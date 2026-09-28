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
package main

import (
	"embed"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// wailsAppOptions is shared by both entrypoints (main.go's normal/tray build
// and main_bindings.go's binding-generation build) so they can't drift.
func wailsAppOptions(app *App) *options.App {
	return &options.App{
		Title:  "Stash",
		Width:  1024,
		Height: 768,
		// Hiding on close (rather than quitting) only makes sense where a
		// tray icon exists to bring the window back — Linux has no tray
		// (see tray_linux.go's doc comment: systray's own GTK main loop and
		// Wails' Linux GTK frontend can't safely coexist as two independent
		// loops in one process), so closing the window there must actually
		// quit; otherwise the app would vanish with no way to reach it again
		// short of killing the process.
		HideWindowOnClose: goruntime.GOOS != "linux",
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		// WebviewGpuIsDisabled: observed a dxgkrnl!ProcessDeadlockThread
		// live-kernel watchdog event (GPU driver deadlock, TDR-recovered) in
		// the Windows Application log coinciding with the window hanging on
		// launch — WebView2's GPU compositor deadlocking against the host
		// GPU driver. Rendering falls back to software; still smooth for
		// this app's UI.
		Windows: &windows.Options{
			WebviewGpuIsDisabled: true,
			// OnResume: mitigates a WebView2/Intel-iGPU deadlock that hangs
			// the app a few minutes after resuming from Modern Standby — see
			// App.onResume's doc comment for the diagnosis.
			OnResume: app.onResume,
		},
	}
}
