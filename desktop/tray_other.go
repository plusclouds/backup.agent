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
//go:build !linux && !darwin

package main

import (
	"github.com/getlantern/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// runTray blocks on systray's own event loop. On Windows this is safe
// to run alongside Wails' own window — the backend there is not GTK-based,
// so there's no shared-main-loop conflict (unlike Linux; see
// tray_linux.go's doc comment for why that platform skips the tray
// entirely, and tray_darwin.go for why macOS does too).
// Wails itself starts from onReady, in its own goroutine.
// Closing the window hides it (HideWindowOnClose, see wailsopts.go); only
// the tray's own "Quit" item stops the app.
func runTray(app *App, startWails func()) {
	systray.Run(func() {
		systray.SetTitle("Stash")
		systray.SetTooltip("Stash Backup Agent")

		open := systray.AddMenuItem("Open", "Show the Stash window")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop Stash and exit")

		go startWails()

		for {
			select {
			case <-open.ClickedCh:
				if app.ctx != nil {
					wailsruntime.WindowShow(app.ctx)
					wailsruntime.WindowUnminimise(app.ctx)
				}
			case <-quit.ClickedCh:
				if app.ctx != nil {
					wailsruntime.Quit(app.ctx)
				}
				systray.Quit()
				return
			}
		}
	}, func() {
		// onExit: nothing extra to do — App.shutdown (wired via Wails'
		// OnShutdown) already stops the embedded daemon.
	})
}
