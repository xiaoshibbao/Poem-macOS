package launcher

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"poem/internal/helperproto"
	"poem/internal/wine"
)

// Wine may need to boot its server and the game must reach its window before
// late modules load (the helper itself waits up to 45 seconds for that).
const helperStartTimeout = 3 * time.Minute

// helperRun follows one poem-helper.exe launch until the game exits.
type helperRun struct {
	pid    uint32
	cmd    *exec.Cmd
	events chan helperproto.Event
}

func readEvents(cmd *exec.Cmd, logf func(string, ...any)) (chan helperproto.Event, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("无法运行 Wine：%w", err)
	}
	// Wine's own diagnostics go to stderr; keep them in the launch log only.
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logf("wine: %s", scanner.Text())
		}
	}()
	events := make(chan helperproto.Event, 16)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			var e helperproto.Event
			if !strings.HasPrefix(line, "{") || json.Unmarshal([]byte(line), &e) != nil {
				if line != "" {
					logf("helper: %s", line)
				}
				continue
			}
			if e.Event == helperproto.EventLog {
				logf("helper: %s", e.Message)
				continue
			}
			events <- e
		}
	}()
	return events, nil
}

// startHelper writes the plan, runs the helper in the prefix and returns once
// the game process has resumed with its modules loaded.
func startHelper(cfg wine.Config, helper, runtimeDir string, plan helperproto.Plan, logf func(string, ...any)) (*helperRun, error) {
	b, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	planFile, err := os.CreateTemp(runtimeDir, "launch-*.json")
	if err != nil {
		return nil, err
	}
	planPath := planFile.Name()
	_, err = planFile.Write(b)
	if closeErr := planFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	// The plan contains the proxy environment only; remove it once read.
	defer os.Remove(planPath)
	cmd := cfg.Command(helper, "launch", wine.WindowsPath(cfg.Prefix, planPath))
	cmd.Dir = filepath.Dir(helper)
	events, err := readEvents(cmd, logf)
	if err != nil {
		return nil, err
	}
	timeout := time.NewTimer(helperStartTimeout)
	defer timeout.Stop()
	for {
		select {
		case e, ok := <-events:
			if !ok {
				err := cmd.Wait()
				return nil, fmt.Errorf("Poem 助手意外退出（%v），请检查 Wine 与容器设置", err)
			}
			switch e.Event {
			case helperproto.EventStarted:
				return &helperRun{pid: e.PID, cmd: cmd, events: events}, nil
			case helperproto.EventError:
				_ = cmd.Wait()
				return nil, errors.New(e.Message)
			}
		case <-timeout.C:
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, fmt.Errorf("等待游戏启动超时（%s）", helperStartTimeout)
		}
	}
}

// wait blocks until the helper reports the game's exit code.
func (r *helperRun) wait() (uint32, error) {
	var code uint32
	exited := false
	for e := range r.events {
		if e.Event == helperproto.EventExited {
			code, exited = e.Code, true
		}
	}
	err := r.cmd.Wait()
	if !exited {
		if err == nil {
			err = errors.New("helper ended without an exit report")
		}
		return 0, err
	}
	return code, nil
}

// attachModule loads a module into a game started by this launcher.
func attachModule(cfg wine.Config, helper string, pid uint32, module string, logf func(string, ...any)) error {
	cmd := cfg.Command(helper, "attach", fmt.Sprint(pid), wine.WindowsPath(cfg.Prefix, module))
	cmd.Dir = filepath.Dir(helper)
	events, err := readEvents(cmd, logf)
	if err != nil {
		return err
	}
	var failure error = errors.New("Poem 助手未返回结果")
	for e := range events {
		switch e.Event {
		case helperproto.EventStarted:
			failure = nil
		case helperproto.EventError:
			failure = errors.New(e.Message)
		}
	}
	if err := cmd.Wait(); err != nil && failure == nil {
		failure = err
	}
	return failure
}
