//go:build windows

package main

import (
	"encoding/binary"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The de-blur module reads its frame rate from this named mapping. Several
// helpers may run for one prefix (multi-instance); the launcher keeps their
// frame rates equal, so a later helper simply writes the same values.
type blurControl struct {
	handle windows.Handle
	view   uintptr
	stop   chan struct{}
	done   chan struct{}
}

func newBlurControl(fps int) (*blurControl, error) {
	name, _ := windows.UTF16PtrFromString("SRUnlocker")
	handle, err := windows.CreateFileMapping(windows.InvalidHandle, nil, windows.PAGE_READWRITE, 0, 4096, name)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return nil, err
	}
	view, err := windows.MapViewOfFile(handle, windows.FILE_MAP_WRITE, 0, 0, 4096)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	control := &blurControl{handle: handle, view: view, stop: make(chan struct{}), done: make(chan struct{})}
	data := unsafe.Slice((*byte)(unsafe.Pointer(view)), 8)
	publish := func() {
		binary.LittleEndian.PutUint32(data[:4], uint32(fps))
		binary.LittleEndian.PutUint32(data[4:], 0) // FOV delta stays at the game's default.
	}
	publish()
	go func() {
		defer close(control.done)
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-control.stop:
				return
			case <-ticker.C:
				publish()
			}
		}
	}()
	return control, nil
}

func (c *blurControl) close() {
	close(c.stop)
	<-c.done
	windows.UnmapViewOfFile(c.view)
	windows.CloseHandle(c.handle)
}
