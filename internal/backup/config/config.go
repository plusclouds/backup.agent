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
// Package config handles loading, and — unlike the VM agent — writing,
// backup.agent's configuration. backup.agent has no ISO config drive: its
// entire identity (agent_uuid, agent_api_key, bucket, access_key) is written
// once by `stash register` and read on every subsequent start.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	vmconfig "github.com/plusclouds/ubuntu-agent/internal/config"
)

// AgentVersion is set at build time via -ldflags, independent of the VM
// agent's own version line.
const AgentVersion = "0.1.0"

// Default file paths, mirroring this repo's existing /etc/plusclouds
// (config) vs. /var/lib/plusclouds (state/cache) convention.
const (
	DefaultConfigPath = "/etc/plusclouds/stash.yaml"
	DefaultStateFile  = "/var/lib/plusclouds/stash-state.json"
	DefaultCacheDir   = "/var/lib/plusclouds/stash-kopia-cache"
	DefaultWorkDir    = "/var/lib/plusclouds/stash-work"
	DefaultLogFile    = "/var/log/plusclouds/stash.log"
)

// Config is the top-level configuration structure for stash.
//
// yaml tags (in addition to mapstructure) are required on every field here
// and below because Save writes this struct out with yaml.Marshal directly
// — without matching yaml tags, yaml.v3's default lowercased field names
// (e.g. AgentUUID -> agentuuid) don't match the mapstructure keys Load reads
// back (agent_uuid), silently producing a config file the agent can never
// re-read its own credentials from. See config_test.go's round-trip test.
type Config struct {
	NATS      vmconfig.NATSConfig `mapstructure:"nats" yaml:"nats"`
	Bucket    BucketConfig        `mapstructure:"bucket" yaml:"bucket"`
	AccessKey AccessKeyConfig     `mapstructure:"access_key" yaml:"access_key"`
	Agent     AgentConfig         `mapstructure:"agent" yaml:"agent"`
	Log       vmconfig.LogConfig  `mapstructure:"log" yaml:"log"`
}

// BucketConfig holds the agent's dedicated S3-compatible bucket, returned at
// registration time.
type BucketConfig struct {
	Name     string `mapstructure:"name" yaml:"name"`
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`
}

// AccessKeyConfig holds the scoped IAM credentials for Bucket, returned at
// registration time. SecretKey is shown exactly once by the platform.
type AccessKeyConfig struct {
	AccessKey string `mapstructure:"access_key" yaml:"access_key"`
	SecretKey string `mapstructure:"secret_key" yaml:"secret_key"`
}

// AgentConfig holds stash's behaviour settings.
type AgentConfig struct {
	// HeartbeatInterval is how often the agent publishes a heartbeat event.
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval" yaml:"heartbeat_interval"`
	// TelemetryInterval is how often the agent publishes a telemetry event.
	TelemetryInterval time.Duration `mapstructure:"telemetry_interval" yaml:"telemetry_interval"`
	// StateFile is where the job list + run bookkeeping is persisted.
	StateFile string `mapstructure:"state_file" yaml:"state_file"`
	// KopiaCacheDir is Kopia's own required local metadata cache directory.
	KopiaCacheDir string `mapstructure:"kopia_cache_dir" yaml:"kopia_cache_dir"`
	// WorkDir is scratch space for verify_snapshot restores, cleaned per run.
	WorkDir string `mapstructure:"work_dir" yaml:"work_dir"`
	// SyncEngine selects which backup engine backs job_type "files"/"script":
	// "rsync" (default — native Go S3 mirror, no dedup/encryption, see
	// engine/s3sync) or "kopia" (deduplicated/encrypted snapshots, see
	// engine/kopia). Registration can also set this (BootstrapResponse.SyncEngine,
	// see register_cmd.go) — an explicit value there overwrites whatever's
	// here; an absent one leaves it untouched.
	SyncEngine string `mapstructure:"sync_engine" yaml:"sync_engine"`
}

// Load reads the configuration from the given file path (or the default
// search paths if empty), overlays PLUSCLOUDS_STASH_* environment
// variables, and applies built-in defaults for anything missing.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("stash")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/plusclouds")
		v.AddConfigPath("$HOME/.plusclouds")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		// viper only returns ConfigFileNotFoundError for the SetConfigName+
		// AddConfigPath search path above; an explicit SetConfigFile (the
		// --config flag case, e.g. the first `register` before the file
		// exists) surfaces a raw os.PathError instead — both mean "no config
		// yet," which is expected and not an error here.
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	v.SetEnvPrefix("PLUSCLOUDS_STASH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	return &cfg, nil
}

// Save writes cfg as YAML to cfgFile (default DefaultConfigPath if empty),
// creating the parent directory if needed. Used once, by `stash
// register`, to persist the credentials the platform shows exactly once.
func Save(cfgFile string, cfg *Config) error {
	if cfgFile == "" {
		cfgFile = DefaultConfigPath
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	dir := filepath.Dir(cfgFile)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	if err := os.WriteFile(cfgFile, data, 0o640); err != nil {
		return fmt.Errorf("writing config file %s: %w", cfgFile, err)
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("nats.connection_type", vmconfig.ConnectionTypeNATS)
	v.SetDefault("nats.max_reconnects", -1)
	v.SetDefault("nats.reconnect_wait", 5*time.Second)

	v.SetDefault("agent.heartbeat_interval", 30*time.Second)
	v.SetDefault("agent.telemetry_interval", 60*time.Second)
	v.SetDefault("agent.state_file", DefaultStateFile)
	v.SetDefault("agent.kopia_cache_dir", DefaultCacheDir)
	v.SetDefault("agent.work_dir", DefaultWorkDir)
	v.SetDefault("agent.sync_engine", "rsync")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("log.file", DefaultLogFile)
}
