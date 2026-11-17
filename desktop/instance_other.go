//go:build !windows

package main

// handOverToRunningInstance is only needed on Windows, the one platform
// where a tray icon is created before Wails gets to do the hand-over itself.
func handOverToRunningInstance() bool { return false }
