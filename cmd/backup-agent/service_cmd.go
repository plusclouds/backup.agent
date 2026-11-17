package main

import "github.com/spf13/cobra"

// newServiceCmd exposes the service install that `stash register` already
// does on its own, for the cases register doesn't cover: a machine that
// was registered before service support existed, or by a user who lacked
// the rights to install a service at the time.
func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage stash as a system service",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install and start stash as a system service (systemd unit on Linux, Windows service on Windows)",
		Run: func(*cobra.Command, []string) {
			installSystemService()
		},
	})
	return cmd
}
