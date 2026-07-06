//go:build !bindings

package main

import "github.com/wailsapp/wails/v2"

func main() {
	app := NewApp()

	// systray owns the process's main goroutine (see tray.go's doc comment);
	// Wails itself starts from the tray's onReady callback, in its own
	// goroutine, once the tray icon exists. Excluded from the `bindings`
	// build (see main_bindings.go) — Wails' binding-generation step compiles
	// and runs this program with that tag expecting it to introspect the
	// Bind slice and exit immediately; systray.Run's event loop never
	// returns on its own, which would hang that step forever.
	runTray(app, func() {
		if err := wails.Run(wailsAppOptions(app)); err != nil {
			println("Error:", err.Error())
		}
	})
}
