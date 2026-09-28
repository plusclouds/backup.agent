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
	"os"
	"path/filepath"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
)

// desktopConfigPath returns where the desktop app's config file lives:
// $HOME/.plusclouds/stash.yaml — a path internal/backup/config.Load
// already searches by default, so the existing CLI (`stash status
// --config ~/.plusclouds/stash.yaml`) can inspect a desktop
// installation with zero code changes there.
func desktopConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".plusclouds", "stash.yaml"), nil
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

	base := filepath.Join(configDir, "plusclouds", "stash")
	cache := filepath.Join(cacheDir, "plusclouds", "stash")

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
		cfg.Log.File = filepath.Join(base, "stash.log")
	}
	return nil
}
