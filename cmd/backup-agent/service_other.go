//go:build !linux && !windows

package main

import "fmt"

// installSystemService is a no-op here: there's no service-manager
// integration built for the headless binary on macOS yet (Linux has
// service_linux.go, Windows has service_windows.go; unlike
// the desktop app, which self-manages its own per-user autostart on every
// OS — see desktop/autostart_windows.go and desktop/autostart_darwin.go).
func installSystemService() {
	fmt.Println("Automatic service install isn't available for stash on this OS yet — start manually with: stash run")
}
