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

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// autoStartUnitPath is the systemd --user unit this app installs so it
// starts at login — GNOME (and any other systemd --user-capable desktop)
// runs it without needing root, matching the per-user model the rest of
// desktop/ uses (see paths.go's applyDesktopDefaults).
func autoStartUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", "stash-desktop.service"), nil
}

const autoStartUnitTemplate = `[Unit]
Description=Stash Backup Agent (desktop)

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`

// setAutoStart only ever enables/disables the unit — never --now. This
// toggle is reached from inside the already-running app (the user is
// clicking a checkbox in the live UI), and this app *is* the process
// systemd would start/stop with --now: disabling "with --now" was observed
// killing the user's current session as a side effect of unchecking "start
// at login," not just preventing the *next* login's start. enable/disable
// alone only affects future logins, leaving the current process alone.
func setAutoStart(enabled bool) error {
	unitPath, err := autoStartUnitPath()
	if err != nil {
		return err
	}

	if !enabled {
		_ = exec.Command("systemctl", "--user", "disable", filepath.Base(unitPath)).Run() //nolint:errcheck — best-effort
		return os.Remove(unitPath)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(unitPath), 0o750); err != nil {
		return fmt.Errorf("creating systemd user directory: %w", err)
	}
	unit := fmt.Sprintf(autoStartUnitTemplate, exePath)
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl --user daemon-reload: %w", err)
	}
	if err := exec.Command("systemctl", "--user", "enable", filepath.Base(unitPath)).Run(); err != nil {
		return fmt.Errorf("systemctl --user enable: %w", err)
	}
	return nil
}

func isAutoStartEnabled() (bool, error) {
	unitPath, err := autoStartUnitPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(unitPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
