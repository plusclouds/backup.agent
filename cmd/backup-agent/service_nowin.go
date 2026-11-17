//go:build !windows

package main

import "context"

// runAsSystemService is only meaningful on Windows, where a service has to
// talk to the Service Control Manager itself (see service_windows.go).
// systemd and launchd just run the process and signal it.
func runAsSystemService(func(context.Context) error) (handled bool, err error) {
	return false, nil
}
