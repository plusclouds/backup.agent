//go:build !windows

package config

func defaultConfigPath() string { return "/etc/plusclouds/stash.yaml" }
func defaultStateFile() string  { return "/var/lib/plusclouds/stash-state.json" }
func defaultCacheDir() string   { return "/var/lib/plusclouds/stash-kopia-cache" }
func defaultWorkDir() string    { return "/var/lib/plusclouds/stash-work" }
func defaultLogFile() string    { return "/var/log/plusclouds/stash.log" }

// restrictToAdmins is a no-op here: the file mode Save writes with already
// keeps the config private.
func restrictToAdmins(string) error { return nil }
