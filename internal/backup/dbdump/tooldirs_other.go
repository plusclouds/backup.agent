//go:build !windows && !linux && !darwin

package dbdump

// installedToolDirPatterns has no known install locations for this
// platform; the tools have to be on PATH or named with bin_dir.
func installedToolDirPatterns() []string { return nil }
