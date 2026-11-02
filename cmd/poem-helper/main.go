//go:build windows

// poem-helper.exe runs inside the Wine prefix. The macOS launcher cannot create
// or inject Windows processes itself, so it delegates those steps here:
//
//	poem-helper.exe launch <plan.json>   start the game, report its PID, wait for exit
//	poem-helper.exe attach <pid> <dll>   load a module into a running game
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"poem/internal/helperproto"
	"poem/internal/injector"
)

func emit(e helperproto.Event) {
	b, _ := json.Marshal(e)
	fmt.Fprintf(os.Stdout, "%s\n", b)
}

func logf(format string, args ...any) {
	emit(helperproto.Event{Event: helperproto.EventLog, Message: fmt.Sprintf(format, args...)})
}

func fail(format string, args ...any) int {
	emit(helperproto.Event{Event: helperproto.EventError, Message: fmt.Sprintf(format, args...)})
	return 1
}

func main() {
	if len(os.Args) < 2 {
		os.Exit(fail("usage: poem-helper.exe launch <plan.json> | attach <pid> <dll>"))
	}
	switch os.Args[1] {
	case "launch":
		if len(os.Args) != 3 {
			os.Exit(fail("launch 需要计划文件路径"))
		}
		os.Exit(launch(os.Args[2]))
	case "attach":
		if len(os.Args) != 4 {
			os.Exit(fail("attach 需要 PID 和模块路径"))
		}
		pid, err := strconv.ParseUint(os.Args[2], 10, 32)
		if err != nil {
			os.Exit(fail("无效 PID：%s", os.Args[2]))
		}
		if err := injector.AttachLuaBridge(uint32(pid), os.Args[3]); err != nil {
			os.Exit(fail("%v", err))
		}
		emit(helperproto.Event{Event: helperproto.EventStarted, PID: uint32(pid)})
	default:
		os.Exit(fail("未知命令：%s", os.Args[1]))
	}
}

func launch(planPath string) int {
	b, err := os.ReadFile(planPath)
	if err != nil {
		return fail("读取启动计划失败：%v", err)
	}
	var plan helperproto.Plan
	if err := json.Unmarshal(b, &plan); err != nil {
		return fail("启动计划格式错误：%v", err)
	}
	if plan.TrustCert != "" {
		if err := installTrust(plan.TrustCert); err != nil {
			return fail("代理证书安装到 Wine 容器失败：%v", err)
		}
		logf("proxy CA trusted in prefix")
	}
	var blur *blurControl
	if plan.BlurFPS > 0 {
		if blur, err = newBlurControl(plan.BlurFPS); err != nil {
			return fail("去虚化控制初始化失败：%v", err)
		}
		defer blur.close()
	}
	var proc *injector.Process
	if len(plan.Late) > 0 {
		logf("staged early=%v late=%v", plan.Early, plan.Late)
		proc, err = injector.LaunchStaged(plan.Game, plan.Early, plan.Late, plan.Env)
	} else {
		logf("modules=%v", plan.Early)
		proc, err = injector.LaunchWithDLLs(plan.Game, plan.Early, plan.Env)
	}
	if err != nil {
		return fail("%v", err)
	}
	defer proc.Close()
	emit(helperproto.Event{Event: helperproto.EventStarted, PID: proc.Pid()})
	proc.Wait()
	code, err := proc.ExitCode()
	if err != nil {
		logf("exit code unavailable: %v", err)
	}
	emit(helperproto.Event{Event: helperproto.EventExited, PID: proc.Pid(), Code: code})
	return 0
}
