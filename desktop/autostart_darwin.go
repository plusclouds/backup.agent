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

import (
	"fmt"
	"os"
	"path/filepath"
)

// autoStartPlistPath is the LaunchAgent this app installs so it starts at
// login — per-user (~/Library/LaunchAgents), no root/admin needed, matching
// the per-user model the rest of desktop/ uses (see paths.go's
// applyDesktopDefaults).
func autoStartPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.plusclouds.stash-desktop.plist"), nil
}

const autoStartPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.plusclouds.stash-desktop</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
`

// setAutoStart only ever writes/removes the plist file — never calls
// launchctl load/unload. This toggle is reached from inside the
// already-running app (the user is clicking a checkbox in the live UI), and
// with KeepAlive:true in the plist, this app *is* the job launchctl would
// manage: `launchctl unload` sends it SIGTERM, killing the user's current
// session as a side effect of unchecking "start at login," not just
// preventing the *next* login's start. launchd rescans LaunchAgents at each
// login regardless, so just managing the file achieves "will (not) start
// next time" without touching the current process.
func setAutoStart(enabled bool) error {
	plistPath, err := autoStartPlistPath()
	if err != nil {
		return err
	}

	if !enabled {
		return os.Remove(plistPath)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0o750); err != nil {
		return fmt.Errorf("creating LaunchAgents directory: %w", err)
	}
	plist := fmt.Sprintf(autoStartPlistTemplate, exePath)
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("writing LaunchAgent plist: %w", err)
	}
	return nil
}

func isAutoStartEnabled() (bool, error) {
	plistPath, err := autoStartPlistPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(plistPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
