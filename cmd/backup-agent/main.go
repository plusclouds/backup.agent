// Command backup-agent is the PlusClouds backup agent daemon. It registers
// once with the platform (token -> bootstrap config), then connects to NATS
// as agent_type "backup" and runs Kopia-backed backup jobs on a cron
// schedule it owns itself. See docs/backup.agent/{overview,protocol,registration}.md.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
)

var cfgFile string

func main() {
	root := &cobra.Command{
		Use:     "backup-agent",
		Short:   "PlusClouds backup agent",
		Version: backupconfig.AgentVersion,
		RunE:    runDaemon,
	}
	root.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Path to config file (default: /etc/plusclouds/backup-agent.yaml)")

	root.AddCommand(newRegisterCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newStatusCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
