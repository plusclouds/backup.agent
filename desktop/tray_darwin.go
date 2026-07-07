//go:build darwin

package main

// runTray on macOS just runs Wails directly, with no tray icon at all.
//
// getlantern/systray's darwin backend compiles its own Objective-C
// AppDelegate class (systray_darwin.m), and Wails v2's darwin runtime
// also defines an AppDelegate — linking both into one binary fails with
// "duplicate symbol '_OBJC_CLASS_$_AppDelegate'". This is a link-time
// conflict, not a runtime race: the two libraries each want to own the
// NSApplication delegate, so there's no flag or ordering that fixes it.
//
// Until the tray moves to a Wails-compatible implementation (e.g. a fork
// like energye/systray that attaches to the existing NSApplication instead
// of installing its own delegate, or Wails v3's native tray support),
// macOS simply has no tray. Note wailsopts.go sets HideWindowOnClose to
// true on non-Linux platforms — on macOS that's still acceptable without
// a tray, because the app stays in the Dock and can be reopened from
// there (unlike a fully hidden window on Windows/Linux).
func runTray(_ *App, startWails func()) {
	startWails()
}
