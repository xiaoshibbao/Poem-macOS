//go:build windows

package injector

import (
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

// The entry shim signals after its initial IAT scan. Do not resume game code
// before that scan; a fixed delay would leave another proxy-initialization race.
func waitNetworkEntryReady(process windows.Handle, path string) error {
	local, err := windows.LoadLibraryEx(path, 0, 1) // DONT_RESOLVE_DLL_REFERENCES: never execute local DllMain.
	if err != nil {
		return err
	}
	defer windows.FreeLibrary(local)
	ready, err := windows.GetProcAddress(local, "PoemNetworkReady")
	if err != nil {
		return err
	}
	offset := ready - uintptr(local)
	pid, err := windows.GetProcessId(process)
	if err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE|windows.TH32CS_SNAPMODULE32, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ModuleEntry32{Size: uint32(unsafe.Sizeof(windows.ModuleEntry32{}))}
	var remote uintptr
	for err = windows.Module32First(snapshot, &entry); err == nil; err = windows.Module32Next(snapshot, &entry) {
		if strings.EqualFold(filepath.Base(windows.UTF16ToString(entry.ExePath[:])), filepath.Base(path)) {
			remote = entry.ModBaseAddr
			break
		}
	}
	if remote == 0 {
		return fmt.Errorf("network entry module not found: %s", filepath.Base(path))
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var value uint32
		var read uintptr
		if err := windows.ReadProcessMemory(process, remote+offset, (*byte)(unsafe.Pointer(&value)), 4, &read); err != nil {
			return err
		}
		if read == 4 && value == 1 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("network entry initialization timed out")
}
