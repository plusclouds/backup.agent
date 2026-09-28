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
package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/plusclouds/ubuntu-agent/internal/config"
)

// --- Defaults ---

func TestLoad_Defaults(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.NATS.URL != "nats://localhost:4222" {
		t.Errorf("NATS URL default: got %q, want nats://localhost:4222", cfg.NATS.URL)
	}
	if cfg.NATS.MaxReconnects != -1 {
		t.Errorf("NATS max_reconnects default: got %d, want -1", cfg.NATS.MaxReconnects)
	}
	if cfg.NATS.ReconnectWait != 5*time.Second {
		t.Errorf("NATS reconnect_wait default: got %v, want 5s", cfg.NATS.ReconnectWait)
	}
	if cfg.Agent.HeartbeatInterval != 30*time.Second {
		t.Errorf("heartbeat interval default: got %v, want 30s", cfg.Agent.HeartbeatInterval)
	}
	if cfg.Agent.TelemetryInterval != 30*time.Second {
		t.Errorf("telemetry interval default: got %v, want 30s", cfg.Agent.TelemetryInterval)
	}
	if len(cfg.Agent.AllowedOperations) == 0 {
		t.Error("allowed_operations should have defaults")
	}
	if cfg.ISO.MountPath != "/media/plusclouds-config" {
		t.Errorf("ISO mount path default: got %q", cfg.ISO.MountPath)
	}
	if !cfg.ISO.FallbackEnv {
		t.Error("ISO fallback_env should default to true")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("log level default: got %q, want info", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("log format default: got %q, want json", cfg.Log.Format)
	}
	if !cfg.Autoheal.Enabled {
		t.Error("autoheal.enabled should default to true")
	}
	if cfg.Autoheal.RestartDelay != 10*time.Second {
		t.Errorf("autoheal restart_delay default: got %v, want 10s", cfg.Autoheal.RestartDelay)
	}
}

// --- Config file ---

func TestLoad_FromYAMLFile(t *testing.T) {
	yaml := `
nats:
  url: nats://nats.example.com:4222
  reconnect_wait: 10s
log:
  level: debug
  format: console
agent:
  heartbeat_interval: 60s
  telemetry_interval: 120s
`
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(cfgFile, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.NATS.URL != "nats://nats.example.com:4222" {
		t.Errorf("NATS URL: got %q", cfg.NATS.URL)
	}
	if cfg.NATS.ReconnectWait != 10*time.Second {
		t.Errorf("NATS reconnect_wait: got %v, want 10s", cfg.NATS.ReconnectWait)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log level: got %q, want debug", cfg.Log.Level)
	}
	if cfg.Log.Format != "console" {
		t.Errorf("log format: got %q, want console", cfg.Log.Format)
	}
	if cfg.Agent.HeartbeatInterval != 60*time.Second {
		t.Errorf("heartbeat interval: got %v, want 60s", cfg.Agent.HeartbeatInterval)
	}
	if cfg.Agent.TelemetryInterval != 120*time.Second {
		t.Errorf("telemetry interval: got %v, want 120s", cfg.Agent.TelemetryInterval)
	}
}

// --- Environment variable overrides ---

func TestLoad_EnvVarOverridesDefault(t *testing.T) {
	t.Setenv("PLUSCLOUDS_AGENT_NATS_URL", "nats://env-server:4222")
	t.Setenv("PLUSCLOUDS_AGENT_LOG_LEVEL", "warn")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.NATS.URL != "nats://env-server:4222" {
		t.Errorf("NATS URL via env: got %q, want nats://env-server:4222", cfg.NATS.URL)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("log level via env: got %q, want warn", cfg.Log.Level)
	}
}

func TestLoad_EnvVarOverridesFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(cfgFile, []byte("nats:\n  url: nats://file-server:4222\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLUSCLOUDS_AGENT_NATS_URL", "nats://env-wins:4222")

	cfg, err := config.Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.NATS.URL != "nats://env-wins:4222" {
		t.Errorf("env should override file: got %q, want nats://env-wins:4222", cfg.NATS.URL)
	}
}

// --- Error cases ---

func TestLoad_InvalidYAMLFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(cfgFile, []byte(": invalid: yaml: [\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(cfgFile)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestLoad_NonExistentFileIsOK(t *testing.T) {
	_, err := config.Load("/nonexistent/path/agent.yaml")
	if err == nil {
		t.Error("expected error for explicit non-existent config file path")
	}
}
