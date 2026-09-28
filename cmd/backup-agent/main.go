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
// Command stash is the PlusClouds Stash backup agent daemon. It registers
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
		Use:     "stash",
		Short:   "PlusClouds Stash backup agent",
		Version: backupconfig.AgentVersion,
		RunE:    runDaemon,
	}
	root.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Path to config file (default: /etc/plusclouds/stash.yaml)")

	root.AddCommand(newRegisterCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newStatusCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
