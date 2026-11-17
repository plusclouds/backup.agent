//go:build windows

package config

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// baseDir is %ProgramData%\PlusClouds\Stash — machine-wide, so the agent
// finds the same files whether it runs from an administrator's shell or as
// the Windows service.
func baseDir() string {
	root := os.Getenv("ProgramData")
	if root == "" {
		root = `C:\ProgramData`
	}
	return filepath.Join(root, "PlusClouds", "Stash")
}

func defaultConfigPath() string { return filepath.Join(baseDir(), "stash.yaml") }
func defaultStateFile() string  { return filepath.Join(baseDir(), "stash-state.json") }
func defaultCacheDir() string   { return filepath.Join(baseDir(), "kopia-cache") }
func defaultWorkDir() string    { return filepath.Join(baseDir(), "work") }
func defaultLogFile() string    { return filepath.Join(baseDir(), "stash.log") }

// restrictToAdmins replaces the file's inherited access list with one that
// lets only SYSTEM and the Administrators group in. %ProgramData% is
// readable by every local user by default, and this file holds secrets.
func restrictToAdmins(path string) error {
	// D:PAI — a protected DACL (drops inherited entries) with full access
	// for SY (Local System) and BA (Built-in Administrators).
	sd, err := windows.SecurityDescriptorFromString("D:PAI(A;;FA;;;SY)(A;;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
}
