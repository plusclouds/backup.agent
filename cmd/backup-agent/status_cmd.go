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
package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the registered agent's job list and last-run status",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	cfg, err := backupconfig.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.NATS.AgentUUID == "" {
		return fmt.Errorf("agent is not registered — run `backup-agent register --token=... --endpoint=...` first")
	}

	st := state.NewStore(cfg.Agent.StateFile)
	if err := st.Load(); err != nil {
		return fmt.Errorf("loading state file: %w", err)
	}

	fmt.Printf("agent_uuid:  %s\n", cfg.NATS.AgentUUID)
	fmt.Printf("bucket:      %s\n", cfg.Bucket.Name)
	fmt.Printf("sync_engine: %s\n\n", cfg.Agent.SyncEngine)

	jobs := st.Jobs()
	if len(jobs) == 0 {
		fmt.Println("No jobs (waiting for the platform's first full_sync).")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSCHEDULE\tPAUSED\tLAST_RUN_STATUS\tLAST_RUN_AT")
	for _, j := range jobs {
		fmt.Fprintf(w, "%s\t%s\t%v\t%s\t%s\n", j.Name, j.Schedule, j.Paused, orDash(j.LastRunStatus), orDash(j.LastRunAt))
	}
	return w.Flush()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
