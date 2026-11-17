//go:build windows

package config

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDefaultPathsAreUnderProgramData(t *testing.T) {
	t.Setenv("ProgramData", `D:\Data`)
	if got, want := defaultConfigPath(), `D:\Data\PlusClouds\Stash\stash.yaml`; got != want {
		t.Fatalf("defaultConfigPath() = %q, want %q", got, want)
	}
	for _, path := range []string{defaultStateFile(), defaultCacheDir(), defaultWorkDir(), defaultLogFile()} {
		if !strings.HasPrefix(path, `D:\Data\PlusClouds\Stash\`) {
			t.Errorf("%q is not under the Stash data directory", path)
		}
	}
}

func TestSaveLeavesConfigReadableOnlyByAdminsAndSystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stash.yaml")
	if err := Save(path, &Config{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("reading the file's security descriptor: %v", err)
	}
	// The temp directory's own access list grants the current user; if any
	// of it were still inherited, the SDDL would have more than these two.
	if got, want := sd.String(), "D:PAI(A;;FA;;;SY)(A;;FA;;;BA)"; got != want {
		t.Fatalf("config file DACL = %q, want %q", got, want)
	}
}
