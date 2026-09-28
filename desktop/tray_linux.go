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
//go:build linux

package main

// runTray on Linux just runs Wails directly, with no tray icon at all.
//
// getlantern/systray's Linux backend runs its own independent GTK main loop
// (gtk_init + a native event loop) on whatever OS thread calls Run. Wails'
// own Linux frontend is also GTK-based (WebKitGTK) and drives its window
// from a separate OS thread via its own internal dispatcher. GTK3 requires
// every GTK call in a process to happen on the single thread that
// initialized it — there's no thread-safety fallback the way GTK2 had.
// Two independent GTK main loops in one process is a hard API violation,
// not a race that better locking fixes: it reproduced as a SIGSEGV in one
// environment and a SIGABRT in another (glib's own internal assertion
// aborting), and persisted with Go's async goroutine preemption disabled
// (GODEBUG=asyncpreemptoff=1), ruling out a Go-runtime-signal explanation.
//
// Until there's a tray approach that doesn't run a second GTK loop (e.g. a
// D-Bus StatusNotifierItem implementation instead of a GTK status icon),
// Linux simply has no tray: wailsAppOptions sets HideWindowOnClose to false
// on this platform, so closing the window quits the app normally instead of
// vanishing with no tray to bring it back from.
func runTray(_ *App, startWails func()) {
	startWails()
}
