//go:build windows

// Package injector launches a Windows process suspended, injects a DLL via
// LoadLibraryW, then resumes it. The returned Process value exposes the raw
// process handle so the caller can WaitForSingleObject on it — that is the
// only reliable way to detect game exit when the target may spawn or
// re-parent helper processes.
package injector

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createSuspended = 0x00000004

var (
	modKernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procVirtualAllocEx    = modKernel32.NewProc("VirtualAllocEx")
	procVirtualFreeEx     = modKernel32.NewProc("VirtualFreeEx")
	procCreateRemoteTh    = modKernel32.NewProc("CreateRemoteThread")
	procGetExitCodeThread = modKernel32.NewProc("GetExitCodeThread")
)

const (
	memCommit     = 0x00001000
	memReserve    = 0x00002000
	memRelease    = 0x00008000
	pageReadWrite = 0x04
)

// Process is a launched (and possibly DLL-injected) Windows process.
// The caller owns the process handle and must call Close when done.
type Process struct {
	handle windows.Handle
	pid    uint32
}

// Pid returns the OS process id.
func (p *Process) Pid() uint32 { return p.pid }

// Wait blocks until the process exits.
func (p *Process) Wait() {
	if p.handle == 0 {
		return
	}
	_, _ = windows.WaitForSingleObject(p.handle, windows.INFINITE)
}

// ExitCode reports the Windows process status while the handle is still owned.
func (p *Process) ExitCode() (uint32, error) {
	var code uint32
	err := windows.GetExitCodeProcess(p.handle, &code)
	return code, err
}

// Close releases the process handle.
func (p *Process) Close() {
	if p.handle != 0 {
		_ = windows.CloseHandle(p.handle)
		p.handle = 0
	}
}

// Launch starts exePath suspended, optionally injects dllPath via
// LoadLibraryW, then resumes the main thread. extraEnv entries are added to
// (or override) the parent's environment for the new process.
//
// dllPath may be empty — in that case the process is just launched with the
// extended environment, no injection.
func Launch(exePath, dllPath string, extraEnv map[string]string) (*Process, error) {
	dlls := []string{}
	if dllPath != "" {
		dlls = append(dlls, dllPath)
	}
	return LaunchWithDLLs(exePath, dlls, extraEnv)
}

// LaunchWithDLLs loads each module before resuming the game's main thread.
func LaunchWithDLLs(exePath string, dllPaths []string, extraEnv map[string]string) (*Process, error) {
	return launchWithStages(exePath, dllPaths, nil, extraEnv, nil)
}

// LaunchStaged keeps game patches early, then loads the proxy after the game
// window is ready so the two modules do not initialise their hooks together.
func LaunchStaged(exePath string, earlyDLLs, lateDLLs []string, extraEnv map[string]string) (*Process, error) {
	return launchWithStages(exePath, earlyDLLs, lateDLLs, extraEnv, waitForGameWindow)
}

func launchWithStages(exePath string, dllPaths, lateDLLs []string, extraEnv map[string]string, ready func(windows.Handle, uint32) error) (*Process, error) {
	absExe, err := filepath.Abs(exePath)
	if err != nil {
		return nil, fmt.Errorf("resolve exe: %w", err)
	}

	exeUTF16, err := syscall.UTF16PtrFromString(absExe)
	if err != nil {
		return nil, fmt.Errorf("encode exe path: %w", err)
	}
	cmdLineUTF16, err := syscall.UTF16PtrFromString(`"` + absExe + `"`)
	if err != nil {
		return nil, fmt.Errorf("encode cmd line: %w", err)
	}
	workDir := filepath.Dir(absExe)
	workDirUTF16, err := syscall.UTF16PtrFromString(workDir)
	if err != nil {
		return nil, fmt.Errorf("encode work dir: %w", err)
	}

	envBlock := buildEnvBlock(extraEnv)
	var envPtr *uint16
	if len(envBlock) > 0 {
		envPtr = &envBlock[0]
	}

	si := windows.StartupInfo{}
	si.Cb = uint32(unsafe.Sizeof(si))
	pi := windows.ProcessInformation{}

	// CREATE_UNICODE_ENVIRONMENT (0x00000400) is required since envBlock is UTF-16.
	const createUnicodeEnv = 0x00000400
	if err := windows.CreateProcess(
		exeUTF16,
		cmdLineUTF16,
		nil, nil,
		false,
		createSuspended|createUnicodeEnv,
		envPtr,
		workDirUTF16,
		&si,
		&pi,
	); err != nil {
		return nil, fmt.Errorf("CreateProcess: %w", err)
	}

	defer windows.CloseHandle(pi.Thread)

	for _, dllPath := range dllPaths {
		if err := injectDLL(pi.Process, dllPath); err != nil {
			_ = windows.TerminateProcess(pi.Process, 1)
			_ = windows.CloseHandle(pi.Process)
			return nil, err
		}
	}

	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		_ = windows.CloseHandle(pi.Process)
		return nil, fmt.Errorf("ResumeThread: %w", err)
	}

	if len(lateDLLs) > 0 {
		if ready != nil {
			err = ready(pi.Process, pi.ProcessId)
		}
		if err == nil {
			for index, path := range lateDLLs {
				if index > 0 {
					time.Sleep(1500 * time.Millisecond)
				}
				if err = injectDLL(pi.Process, path); err != nil {
					break
				}
			}
		}
		if err != nil {
			_ = windows.TerminateProcess(pi.Process, 1)
			_ = windows.CloseHandle(pi.Process)
			return nil, fmt.Errorf("加载后续功能模块失败：%w", err)
		}
	}
	return &Process{handle: pi.Process, pid: pi.ProcessId}, nil
}

// The visible Unity main window belonging to this process is the readiness
// boundary. Give its message loop time to settle before installing more hooks.
func waitForGameWindow(process windows.Handle, pid uint32) error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	enumWindows := user32.NewProc("EnumWindows")
	getPID := user32.NewProc("GetWindowThreadProcessId")
	isVisible := user32.NewProc("IsWindowVisible")
	getClassName := user32.NewProc("GetClassNameW")
	getClientRect := user32.NewProc("GetClientRect")
	inputIdle := user32.NewProc("WaitForInputIdle")
	deadline := time.Now().Add(45 * time.Second)
	var visibleSince time.Time
	found := false
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var owner uint32
		getPID.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		visible, _, _ := isVisible.Call(hwnd)
		if owner == pid && visible != 0 {
			var class [256]uint16
			length, _, _ := getClassName.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
			if length == 0 || windows.UTF16ToString(class[:]) != "UnityWndClass" {
				return 1
			}
			var rect struct{ Left, Top, Right, Bottom int32 }
			ok, _, _ := getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
			if ok == 0 || rect.Right <= rect.Left || rect.Bottom <= rect.Top {
				return 1
			}
			found = true
			return 0
		}
		return 1
	})
	for time.Now().Before(deadline) {
		state, err := windows.WaitForSingleObject(process, 0)
		if err != nil {
			return err
		}
		if state == windows.WAIT_OBJECT_0 {
			return fmt.Errorf("游戏在代理加载前退出")
		}
		found = false
		enumWindows.Call(callback, 0)
		if found {
			if visibleSince.IsZero() {
				visibleSince = time.Now()
			}
			if time.Since(visibleSince) >= 1500*time.Millisecond {
				inputIdle.Call(uintptr(process), 1000)
				return nil
			}
		} else {
			visibleSince = time.Time{}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("等待 Unity 游戏主窗口超时（45 秒）")
}

// injectDLL writes dllPath into the target process's memory and calls
// LoadLibraryW via CreateRemoteThread. The remote LoadLibraryW call is
// awaited so the DLL is loaded (and DllMain has run) before we return.
func injectDLL(hProcess windows.Handle, dllPath string) error {
	absDLL, err := filepath.Abs(dllPath)
	if err != nil {
		return fmt.Errorf("resolve dll: %w", err)
	}

	dllUTF16, err := syscall.UTF16FromString(absDLL)
	if err != nil {
		return fmt.Errorf("encode dll path: %w", err)
	}
	bufLen := uintptr(len(dllUTF16) * 2)

	r, _, e := procVirtualAllocEx.Call(
		uintptr(hProcess),
		0,
		bufLen,
		memReserve|memCommit,
		pageReadWrite,
	)
	if r == 0 {
		return fmt.Errorf("VirtualAllocEx: %w", e)
	}
	remoteAddr := r
	freeBuffer := true
	defer func() {
		if freeBuffer {
			procVirtualFreeEx.Call(uintptr(hProcess), remoteAddr, 0, memRelease)
		}
	}()

	var written uintptr
	if err := windows.WriteProcessMemory(
		hProcess,
		remoteAddr,
		(*byte)(unsafe.Pointer(&dllUTF16[0])),
		bufLen,
		&written,
	); err != nil {
		return fmt.Errorf("WriteProcessMemory: %w", err)
	}

	kernel32, err := windows.LoadLibrary("kernel32.dll")
	if err != nil {
		return fmt.Errorf("LoadLibrary(kernel32): %w", err)
	}
	loadLibraryW, err := windows.GetProcAddress(kernel32, "LoadLibraryW")
	if err != nil {
		return fmt.Errorf("GetProcAddress(LoadLibraryW): %w", err)
	}

	hThread, _, e := procCreateRemoteTh.Call(
		uintptr(hProcess),
		0, 0,
		loadLibraryW,
		remoteAddr,
		0, 0,
	)
	if hThread == 0 {
		return fmt.Errorf("CreateRemoteThread: %w", e)
	}
	remoteHandle := windows.Handle(hThread)
	defer windows.CloseHandle(remoteHandle)

	result, err := windows.WaitForSingleObject(remoteHandle, 30000)
	if err != nil {
		return fmt.Errorf("WaitForSingleObject(remote): %w", err)
	}
	if result != windows.WAIT_OBJECT_0 {
		freeBuffer = false
		// The target is terminated by LaunchWithDLLs on failure; do not free a
		// buffer still being read by a timed-out loader thread before termination.
		return fmt.Errorf("DLL loading timed out: %s", filepath.Base(absDLL))
	}
	var exitCode uint32
	if ok, _, err := procGetExitCodeThread.Call(uintptr(remoteHandle), uintptr(unsafe.Pointer(&exitCode))); ok == 0 {
		return fmt.Errorf("GetExitCodeThread: %w", err)
	}
	if exitCode == 0 {
		return fmt.Errorf("LoadLibraryW failed: %s", absDLL)
	}
	if strings.EqualFold(filepath.Base(absDLL), "Poem.Network.dll") {
		return waitNetworkEntryReady(hProcess, absDLL)
	}
	return nil
}

// buildEnvBlock returns a UTF-16 environment block (each entry NULL-terminated,
// double-NULL at the end) merging os.Environ with overrides.
func buildEnvBlock(overrides map[string]string) []uint16 {
	return mergeEnvBlock(syscall.Environ(), overrides)
}

// Windows environment names are case insensitive and the block must be sorted.
func mergeEnvBlock(parent []string, overrides map[string]string) []uint16 {
	merged := map[string]string{}
	for _, entry := range parent {
		key, value, ok := splitEnv(entry)
		if ok {
			merged[strings.ToUpper(key)] = value
		}
	}
	for key, value := range overrides {
		merged[strings.ToUpper(key)] = value
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []uint16
	for _, key := range keys {
		out = append(out, utf16.Encode([]rune(key+"="+merged[key]))...)
		out = append(out, 0)
	}
	if len(out) == 0 {
		out = append(out, 0)
	}
	return append(out, 0)
}

func splitEnv(e string) (string, string, bool) {
	for i := 0; i < len(e); i++ {
		if e[i] == '=' && i > 0 {
			return e[:i], e[i+1:], true
		}
	}
	return "", "", false
}
