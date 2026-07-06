package main

import (
	"os"
	"path/filepath"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
)

// desktopConfigPath returns where the desktop app's config file lives:
// $HOME/.plusclouds/backup-agent.yaml — a path internal/backup/config.Load
// already searches by default, so the existing CLI (`backup-agent status
// --config ~/.plusclouds/backup-agent.yaml`) can inspect a desktop
// installation with zero code changes there.
func desktopConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".plusclouds", "backup-agent.yaml"), nil
}

// applyDesktopDefaults points state/cache/work/log paths at per-user
// directories (os.UserConfigDir/UserCacheDir — ~/.config, ~/.cache on Linux;
// ~/Library/Application Support, ~/Library/Caches on macOS; %AppData%,
// %LocalAppData% on Windows) instead of config.go's DefaultStateFile/
// DefaultCacheDir/DefaultWorkDir/DefaultLogFile, which are hardcoded
// root-only paths (/var/lib/plusclouds, /var/log/plusclouds) meant for the
// existing systemd/root server deployment. Those defaults are intentionally
// left untouched — this only sets fields explicitly on cfg, the same as any
// hand-edited config file would, so server installs are entirely unaffected.
func applyDesktopDefaults(cfg *backupconfig.Config) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return err
	}

	base := filepath.Join(configDir, "plusclouds", "backup-agent")
	cache := filepath.Join(cacheDir, "plusclouds", "backup-agent")

	if cfg.Agent.StateFile == "" || cfg.Agent.StateFile == backupconfig.DefaultStateFile {
		cfg.Agent.StateFile = filepath.Join(base, "state.json")
	}
	if cfg.Agent.KopiaCacheDir == "" || cfg.Agent.KopiaCacheDir == backupconfig.DefaultCacheDir {
		cfg.Agent.KopiaCacheDir = filepath.Join(cache, "kopia-cache")
	}
	if cfg.Agent.WorkDir == "" || cfg.Agent.WorkDir == backupconfig.DefaultWorkDir {
		cfg.Agent.WorkDir = filepath.Join(cache, "work")
	}
	if cfg.Log.File == "" || cfg.Log.File == backupconfig.DefaultLogFile {
		cfg.Log.File = filepath.Join(base, "backup-agent.log")
	}
	return nil
}
