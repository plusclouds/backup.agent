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
