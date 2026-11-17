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

	"github.com/plusclouds/ubuntu-agent/internal/backup/dbdump"
	vmconfig "github.com/plusclouds/ubuntu-agent/internal/config"
)

// AgentVersion is set at build time via -ldflags, independent of the VM
// agent's own version line.
var AgentVersion = "0.1.0"

// Default file paths. Linux and macOS follow this repo's existing
// /etc/plusclouds (config) vs. /var/lib/plusclouds (state/cache) convention;
// Windows keeps everything under %ProgramData%\PlusClouds\Stash. The values
// are set per OS in paths_unix.go and paths_windows.go.
var (
	DefaultConfigPath = defaultConfigPath()
	DefaultStateFile  = defaultStateFile()
	DefaultCacheDir   = defaultCacheDir()
	DefaultWorkDir    = defaultWorkDir()
	DefaultLogFile    = defaultLogFile()
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
	// Databases holds local connection settings for job_type "database" jobs,
	// keyed by engine ("postgres", "mysql", "mssql") then by connection name
	// ("default" unless the job names another). It is written by the
	// operator, never by the platform: database credentials stay on this
	// machine. Entirely optional — a missing entry means the engine's own
	// local defaults.
	Databases map[string]map[string]dbdump.Conn `mapstructure:"databases" yaml:"databases,omitempty"`
}

// DBConn returns the local connection settings for engine/name, or the zero
// Conn (engine defaults) if none is configured. An empty name means
// dbdump.DefaultConnection. Lookups are case-insensitive because viper
// lowercases every key it reads.
func (c *Config) DBConn(engine, name string) dbdump.Conn {
	if name == "" {
		name = dbdump.DefaultConnection
	}
	return c.Databases[strings.ToLower(engine)][strings.ToLower(name)]
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
		v.AddConfigPath(filepath.Dir(DefaultConfigPath))
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
	// The file holds the agent's API key and S3 secret. The mode above is
	// what keeps it private on Linux and macOS; Windows ignores it, so the
	// access list is set separately there.
	if err := restrictToAdmins(cfgFile); err != nil {
		return fmt.Errorf("restricting access to config file %s: %w", cfgFile, err)
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
