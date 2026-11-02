//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"poem/internal/helperproto"
	"poem/internal/injector"
)

// jadeite.exe starts the game through explorer.exe and may exit before the
// game does, so the game is found as a new process with its executable name.
const wrappedStartTimeout = 2 * time.Minute

func gamePIDs(name string) map[uint32]bool {
	result := map[uint32]bool{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return result
	}
	defer windows.CloseHandle(snap)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			result[entry.ProcessID] = true
		}
	}
	return result
}

func launchWrapped(plan helperproto.Plan) (*injector.Process, error) {
	name := filepath.Base(plan.Game)
	before := gamePIDs(name)
	cmd := exec.Command(plan.Wrapper, append([]string{plan.Game, "--"}, plan.Args...)...)
	// jadeite expects to be run from the game directory, as its documentation shows.
	cmd.Dir = filepath.Dir(plan.Game)
	cmd.Env = os.Environ()
	for key, value := range plan.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("无法运行 jadeite.exe：%w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(wrappedStartTimeout)
	wrapperDone := false
	for time.Now().Before(deadline) {
		for pid := range gamePIDs(name) {
			if !before[pid] {
				return injector.Open(pid)
			}
		}
		if !wrapperDone {
			select {
			case err := <-exited:
				wrapperDone = true
				if err != nil {
					return nil, fmt.Errorf("jadeite.exe 启动游戏失败：%v", err)
				}
				// A clean exit may precede the game appearing; keep looking briefly.
				deadline = time.Now().Add(20 * time.Second)
			default:
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("通过 jadeite.exe 启动后未检测到游戏进程")
}
