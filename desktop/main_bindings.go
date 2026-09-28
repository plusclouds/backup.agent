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
