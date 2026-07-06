//go:build linux

package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
)

//go:embed stash.service
var systemdUnitTemplate []byte

const systemdUnitPath = "/etc/systemd/system/stash.service"

// installSystemService writes, enables, and starts the systemd unit right
// after a successful register, so there's no separate `make install`/manual
// systemctl step between "register this machine" and "it's running as a
// service" — the same self-managing spirit as the desktop app's own
// per-user autostart toggle (see desktop/autostart_linux.go), just at the
// system level since this is the headless/server binary.
//
// Root-only: silently skipped (with guidance printed instead) when not
// running as root, e.g. a developer running `register` locally as their own
// user to inspect the resulting config. The .deb/.rpm packages don't do this
// at install time either — the unit can't do anything useful until a
// register has actually produced credentials, so tying it to register
// (which always runs after install, and always as whatever user will
// operate the service — root, for this system-wide unit) is the one point
// that's guaranteed to have both root and a valid config at once.
func installSystemService() {
	if os.Geteuid() != 0 {
		fmt.Println("Not running as root — skipping automatic systemd service install.")
		fmt.Println("Run `stash register` as root (or install the .deb/.rpm package and re-run register as root) to have stash run as a service, or start it manually with: stash run")
		return
	}

	if err := os.WriteFile(systemdUnitPath, systemdUnitTemplate, 0o644); err != nil {
		fmt.Printf("Warning: could not write systemd unit %s: %v\n", systemdUnitPath, err)
		fmt.Println("Start manually with: stash run")
		return
	}
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		fmt.Printf("Warning: systemctl daemon-reload failed: %v\n", err)
		return
	}
	if err := exec.Command("systemctl", "enable", "--now", "stash").Run(); err != nil {
		fmt.Printf("Warning: systemctl enable --now stash failed: %v\n", err)
		fmt.Println("Start manually with: stash run")
		return
	}
	fmt.Println("Installed and started stash.service — check with: systemctl status stash")
}
