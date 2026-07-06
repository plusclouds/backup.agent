//go:build bindings

package main

import "github.com/wailsapp/wails/v2"

// main, under the `bindings` build tag, is what Wails' own binding-generation
// step actually compiles and runs (see GenerateBindings in
// github.com/wailsapp/wails/v2/pkg/commands/bindings) — it needs a plain
// wails.Run call it can introspect and that returns on its own, not the
// tray-owned entrypoint in main.go, which blocks forever in systray's event
// loop and would hang this step indefinitely.
func main() {
	app := NewApp()
	if err := wails.Run(wailsAppOptions(app)); err != nil {
		println("Error:", err.Error())
	}
}
