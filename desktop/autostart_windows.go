//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// runKeyValueName is the value written under HKCU\...\Run so this app
// starts at login. No admin/elevation needed (HKCU, not HKLM) — matching
// the per-user model the rest of desktop/ uses. A real Windows Service is
// deliberately not built here (see the desktop feature's design notes) —
// building one needs admin rights, which contradicts a per-user desktop
// install with nothing else to elevate for.
const runKeyValueName = "PlusCloudsBackupAgent"

func setAutoStart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("opening Run registry key: %w", err)
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(runKeyValueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("removing autostart entry: %w", err)
		}
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	if err := key.SetStringValue(runKeyValueName, exePath); err != nil {
		return fmt.Errorf("writing autostart entry: %w", err)
	}
	return nil
}

func isAutoStartEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false, fmt.Errorf("opening Run registry key: %w", err)
	}
	defer key.Close()

	_, _, err = key.GetStringValue(runKeyValueName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
