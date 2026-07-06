package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
	"github.com/plusclouds/ubuntu-agent/internal/backup/register"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	vmconfig "github.com/plusclouds/ubuntu-agent/internal/config"
)

func newRegisterCmd() *cobra.Command {
	var token, endpoint string
	var showRaw bool

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Exchange a one-time registration token for this agent's bootstrap config",
		Long: "Registers this machine with the platform using a one-time token, then writes\n" +
			"the resulting credentials (agent_uuid, agent_api_key, bucket, access_key) and\n" +
			"initial job list to the config/state files. The token is single-use — losing\n" +
			"the resulting config means requesting a new token and registering again.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRegister(cmd.Context(), token, endpoint, showRaw)
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "One-time registration token (required)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Platform API base URL, e.g. https://api.example.com (required)")
	cmd.Flags().BoolVar(&showRaw, "debug", false, "Print the platform's raw HTTP response body (useful for diagnosing unexpected/missing fields)")
	cmd.MarkFlagRequired("token")    //nolint:errcheck
	cmd.MarkFlagRequired("endpoint") //nolint:errcheck

	return cmd
}

func runRegister(ctx context.Context, token, endpoint string, showRaw bool) error {
	// Do every local, non-consuming step FIRST — resolve the config path and
	// load defaults — so a bug or permission problem here can never burn the
	// one-time token. register.Register() below is the only step the
	// platform actually consumes the token for; nothing before it should be
	// able to fail after that point.
	configPath := cfgFile
	if configPath == "" {
		configPath = backupconfig.DefaultConfigPath
	}
	// Load defaults (Agent/Log settings) rather than hand-building a Config,
	// so registration and hand-edited config files stay consistent.
	cfg, err := backupconfig.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading base config (token not yet used): %w", err)
	}

	resp, raw, err := register.Register(ctx, endpoint, token, backupconfig.AgentVersion)
	if err != nil {
		// The HTTP call may have consumed the token even though it's
		// reporting an error here (e.g. a parse failure on an otherwise-
		// successful response) — always show whatever bytes came back on
		// failure, not just when --debug is passed, since this is exactly
		// the moment you need them most and can least afford to lose them.
		if len(raw) > 0 {
			fmt.Fprintf(os.Stderr, "Raw response body:\n%s\n\n", raw)
		}
		return fmt.Errorf("registration failed: %w", err)
	}

	if showRaw {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, raw, "", "  "); err == nil {
			fmt.Printf("Raw response body:\n%s\n\n", pretty.String())
		} else {
			fmt.Printf("Raw response body:\n%s\n\n", raw)
		}
	}

	// The token is consumed as of the line above — print everything the
	// platform returned right away, before any further local operation that
	// could still fail, so credentials are never lost to a bug downstream of
	// the (one-time, non-retryable) HTTP call.
	fmt.Println("Registration succeeded — this response is shown once, save it now:")
	fmt.Printf("  agent_uuid:  %s\n", resp.AgentUUID)
	fmt.Printf("  agent_api_key: %s\n", resp.AgentAPIKey)
	fmt.Printf("  nats:        %s:%d\n", resp.NATS.Host, resp.NATS.Port)
	fmt.Printf("  bucket:      %s (%s)\n", resp.Bucket.Name, resp.Bucket.Endpoint)
	fmt.Printf("  access_key:  %s\n", resp.AccessKey.AccessKey)
	fmt.Printf("  secret_key:  %s\n", resp.AccessKey.SecretKey)
	fmt.Printf("  jobs:        %d\n", len(resp.Jobs.Jobs))
	fmt.Println()

	// Default to WebSocket: backup.agent runs on arbitrary customer machines,
	// and the platform's raw NATS TCP port is not reachable from outside —
	// only the WebSocket gateway is (same reasoning as the VM agent's
	// configs/agent.yaml default). Both URLs are still recorded in the
	// config file so connection_type can be switched by hand if a given
	// deployment's network actually does allow raw TCP to the NATS port.
	cfg.NATS.ConnectionType = vmconfig.ConnectionTypeWebSocket
	cfg.NATS.URL = fmt.Sprintf("nats://%s:%d", resp.NATS.Host, resp.NATS.Port)
	cfg.NATS.WebSocketURL = fmt.Sprintf("wss://%s:%d", resp.NATS.Host, resp.NATS.Port)
	cfg.NATS.AgentUUID = resp.AgentUUID
	cfg.NATS.APIKey = resp.AgentAPIKey
	cfg.Bucket.Name = resp.Bucket.Name
	cfg.Bucket.Endpoint = resp.Bucket.Endpoint
	cfg.AccessKey.AccessKey = resp.AccessKey.AccessKey
	cfg.AccessKey.SecretKey = resp.AccessKey.SecretKey
	// Conditional, unlike the overwrites above: an empty response value means
	// the platform isn't stating a preference, so an operator's existing
	// choice (or the "rsync" default already in cfg from Load) must survive,
	// not get clobbered by a zero value.
	if resp.SyncEngine != "" {
		cfg.Agent.SyncEngine = resp.SyncEngine
	}
	fmt.Printf("  sync_engine: %s\n\n", cfg.Agent.SyncEngine)

	if err := backupconfig.Save(configPath, cfg); err != nil {
		return fmt.Errorf("writing config file (see the printed values above to save by hand): %w", err)
	}

	st := state.NewStore(cfg.Agent.StateFile)
	// Load is a no-op if the file doesn't exist yet (fresh registration).
	if err := st.Load(); err != nil {
		return fmt.Errorf("loading existing state file: %w", err)
	}
	if err := st.ApplyFullSync(resp.Jobs.Jobs); err != nil {
		return fmt.Errorf("writing initial state file: %w", err)
	}

	fmt.Printf("Saved to config: %s\n", configPath)
	fmt.Printf("Saved to state:  %s\n", cfg.Agent.StateFile)
	fmt.Println("Start the daemon with: backup-agent run")
	return nil
}
