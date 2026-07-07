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
		},
	}
}
