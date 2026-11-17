package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procFindWindowW = windows.NewLazySystemDLL("user32.dll").NewProc("FindWindowW")

// handOverToRunningInstance reports whether another copy of the app is
// running and ready to be told about this launch.
//
// Wails does the actual hand-over inside wails.Run: it finds the first
// copy's message window, sends it this launch's arguments and exits the
// process. But on Windows the tray icon is created before wails.Run (see
// runTray), so a second copy would add its own tray icon and then exit
// without removing it, leaving a dead icon behind on every repeat launch.
// Checking first lets main skip the tray for a copy that is about to exit.
//
// The mutex and window names are the ones Wails derives from the unique id
// (internal/frontend/desktop/windows/single_instance.go). If a future Wails
// names them differently this simply reports false, and the app falls back
// to the hand-over inside wails.Run — correct, with the stray icon.
func handOverToRunningInstance() bool {
	id := "wails-app-" + singleInstanceID

	mutexName, err := windows.UTF16PtrFromString(id + "sim")
	if err != nil {
		return false
	}
	mutex, err := windows.OpenMutex(windows.SYNCHRONIZE, false, mutexName)
	if err != nil {
		return false // no other copy
	}
	windows.CloseHandle(mutex) //nolint:errcheck

	className, err := windows.UTF16PtrFromString(id + "-sic")
	if err != nil {
		return false
	}
	windowName, err := windows.UTF16PtrFromString(id + "-siw")
	if err != nil {
		return false
	}
	// The other copy creates its message window a moment after its mutex.
	// Give one that is still starting a few seconds to get there; without
	// the window Wails would start this copy as a full second instance.
	for range 30 {
		hwnd, _, _ := procFindWindowW.Call(
			uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowName)))
		if hwnd != 0 {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
