//go:build windows

package injector

import "golang.org/x/sys/windows"

// The caller must restrict pid to a game session it owns. This only loads the
// bridge; the bridge executes scripts through the game's main-thread Lua API.
func AttachLuaBridge(pid uint32, path string) error {
	handle, err := windows.OpenProcess(windows.PROCESS_CREATE_THREAD|windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_OPERATION|windows.PROCESS_VM_WRITE|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return injectDLL(handle, path)
}
