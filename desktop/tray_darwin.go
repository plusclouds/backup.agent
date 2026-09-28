// MIT License
//
// Copyright (c) 2026 PlusClouds
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
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
