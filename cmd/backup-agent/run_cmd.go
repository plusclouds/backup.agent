package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
	"github.com/plusclouds/ubuntu-agent/internal/backup/daemon"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the backup agent daemon (also the default command)",
		RunE:  runDaemon,
	}
}

// runDaemon is a thin CLI wrapper around daemon.Run: load config, build the
// logger, wire OS-signal-triggered shutdown, then block on the shared daemon
// logic. daemon.Run itself installs no signal handling — that's a CLI-only
// concern (a desktop GUI cancels its own context from a tray "Quit" action
// instead) — so this is the only place that concern lives.
func runDaemon(_ *cobra.Command, _ []string) error {
	cfg, err := backupconfig.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger, err := daemon.BuildLogger(cfg)
	if err != nil {
		return fmt.Errorf("building logger: %w", err)
	}
	defer logger.Sync() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigCh:
			logger.Info("received shutdown signal", zap.String("signal", sig.String()))
			cancel()
		case <-ctx.Done():
		}
	}()

	return daemon.Run(ctx, cfg, logger, nil)
}
