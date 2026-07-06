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
	return filepath.Join(home, ".config", "systemd", "user", "backup-agent-desktop.service"), nil
}

const autoStartUnitTemplate = `[Unit]
Description=PlusClouds Backup Agent (desktop)

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
