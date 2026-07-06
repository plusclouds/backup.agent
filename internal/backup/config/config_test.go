package config

import (
	"path/filepath"
	"testing"
)

// TestSaveLoadRoundTrip guards against the yaml/mapstructure tag mismatch
// bug found during real-world testing: yaml.Marshal defaults to lowercasing
// Go field names with no separators (AgentUUID -> agentuuid), which then
// silently fails to match viper's mapstructure-based Load (agent_uuid),
// producing a config file that "saves successfully" but can never be read
// back — the agent would appear unregistered on every subsequent start.
func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup-agent.yaml")

	var cfg Config
	cfg.NATS.ConnectionType = "websocket"
	cfg.NATS.WebSocketURL = "wss://nats.example.com:443"
	cfg.NATS.AgentUUID = "agent-uuid-1"
	cfg.NATS.APIKey = "api-key-1"
	cfg.Bucket.Name = "backup-agent-abc123"
	cfg.Bucket.Endpoint = "https://s3.example.com"
	cfg.AccessKey.AccessKey = "AKIA..."
	cfg.AccessKey.SecretKey = "secret"
	cfg.Agent.SyncEngine = "kopia"

	if err := Save(path, &cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.NATS.AgentUUID != "agent-uuid-1" {
		t.Errorf("expected NATS.AgentUUID to round-trip, got %q", loaded.NATS.AgentUUID)
	}
	if loaded.NATS.APIKey != "api-key-1" {
		t.Errorf("expected NATS.APIKey to round-trip, got %q", loaded.NATS.APIKey)
	}
	if loaded.NATS.ConnectionType != "websocket" {
		t.Errorf("expected NATS.ConnectionType to round-trip, got %q", loaded.NATS.ConnectionType)
	}
	if loaded.NATS.WebSocketURL != "wss://nats.example.com:443" {
		t.Errorf("expected NATS.WebSocketURL to round-trip, got %q", loaded.NATS.WebSocketURL)
	}
	if loaded.Bucket.Name != "backup-agent-abc123" || loaded.Bucket.Endpoint != "https://s3.example.com" {
		t.Errorf("expected Bucket to round-trip, got %+v", loaded.Bucket)
	}
	if loaded.AccessKey.AccessKey != "AKIA..." || loaded.AccessKey.SecretKey != "secret" {
		t.Errorf("expected AccessKey to round-trip, got %+v", loaded.AccessKey)
	}
	if loaded.Agent.SyncEngine != "kopia" {
		t.Errorf("expected Agent.SyncEngine to round-trip, got %q", loaded.Agent.SyncEngine)
	}
}

// TestLoadDefaultsSyncEngineToRsync guards the "rsync by default" rule: a
// config file that doesn't mention sync_engine at all (or no config file at
// all) must still resolve to "rsync", not an empty string that run_cmd.go's
// switch would reject as unknown.
func TestLoadDefaultsSyncEngineToRsync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist-yet.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agent.SyncEngine != "rsync" {
		t.Errorf("expected Agent.SyncEngine to default to %q, got %q", "rsync", cfg.Agent.SyncEngine)
	}
}

// TestLoadToleratesMissingExplicitConfigFile guards against the other
// register bug found during testing: passing an explicit --config path
// that doesn't exist yet (the normal case for a first-time `register`) must
// not be treated as an error — only viper's own auto-search path returns a
// typed ConfigFileNotFoundError; an explicit SetConfigFile surfaces a raw
// os.PathError for the same "no file yet" situation.
func TestLoadToleratesMissingExplicitConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist-yet.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected Load to tolerate a missing explicit config file, got: %v", err)
	}
	if cfg.Agent.HeartbeatInterval == 0 {
		t.Errorf("expected defaults to still apply when the config file is missing")
	}
}
