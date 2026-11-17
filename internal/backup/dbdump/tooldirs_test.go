package dbdump

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installTool creates an empty file standing in for a client tool.
func installTool(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(parts...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func usePatterns(t *testing.T, patterns ...string) {
	t.Helper()
	original := toolDirPatterns
	t.Cleanup(func() { toolDirPatterns = original })
	toolDirPatterns = func() []string { return patterns }
}

func TestFindInstalledToolPrefersNewestVersion(t *testing.T) {
	root := t.TempDir()
	installTool(t, root, "MySQL", "MySQL Server 5.7", "bin", "mysqldump")
	installTool(t, root, "MySQL", "MySQL Server 8.0", "bin", "mysqldump")
	want := installTool(t, root, "MySQL", "MySQL Server 8.4", "bin", "mysqldump")
	// 10 must beat 9: a plain string sort would put "9.6" first.
	installTool(t, root, "PostgreSQL", "9.6", "bin", "pg_dump")
	wantPg := installTool(t, root, "PostgreSQL", "10", "bin", "pg_dump")
	usePatterns(t,
		filepath.Join(root, "MySQL", "MySQL Server *", "bin"),
		filepath.Join(root, "PostgreSQL", "*", "bin"))

	if got := findInstalledTool("mysqldump"); got != want {
		t.Errorf("mysqldump = %q, want %q", got, want)
	}
	if got := findInstalledTool("pg_dump"); got != wantPg {
		t.Errorf("pg_dump = %q, want %q", got, wantPg)
	}
}

func TestFindInstalledToolSkipsDirectoriesWithoutTheTool(t *testing.T) {
	root := t.TempDir()
	// The newest install has no mysqldump; the older one does.
	installTool(t, root, "MySQL Server 9.0", "bin", "mysql")
	want := installTool(t, root, "MySQL Server 8.0", "bin", "mysqldump")
	usePatterns(t, filepath.Join(root, "MySQL Server *", "bin"))

	if got := findInstalledTool("mysqldump"); got != want {
		t.Errorf("mysqldump = %q, want %q", got, want)
	}
	if got := findInstalledTool("psql"); got != "" {
		t.Errorf("psql = %q, want nothing", got)
	}
}

func TestLookToolFallsBackToInstallDirectories(t *testing.T) {
	root := t.TempDir()
	want := installTool(t, root, "MySQL Server 8.0", "bin", toolFile("stash-test-tool"))
	usePatterns(t, filepath.Join(root, "MySQL Server *", "bin"))

	got, err := lookTool("", "stash-test-tool")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("tool = %q, want %q", got, want)
	}

	_, err = lookTool("", "stash-test-absent")
	if err == nil || !strings.Contains(err.Error(), "bin_dir") {
		t.Fatalf("expected an error pointing at bin_dir, got %v", err)
	}
}

func TestLookToolDoesNotSearchWhenBinDirIsSet(t *testing.T) {
	root := t.TempDir()
	installTool(t, root, "MySQL Server 8.0", "bin", toolFile("stash-test-tool"))
	usePatterns(t, filepath.Join(root, "MySQL Server *", "bin"))

	// bin_dir names a directory without the tool: that is an error, not a
	// reason to quietly use a different install.
	if got, err := lookTool(t.TempDir(), "stash-test-tool"); err == nil {
		t.Fatalf("lookTool found %q despite bin_dir not containing it", got)
	}
}

func TestFindInstalledToolTriesPatternsInOrder(t *testing.T) {
	root := t.TempDir()
	// The linked "current" directory carries no version in its path; an
	// older side-by-side install does. Listed first, the linked one wins.
	want := installTool(t, root, "bin", "mysqldump")
	installTool(t, root, "opt", "mysql@5.7", "bin", "mysqldump")
	usePatterns(t, filepath.Join(root, "bin"), filepath.Join(root, "opt", "mysql*", "bin"))

	if got := findInstalledTool("mysqldump"); got != want {
		t.Errorf("mysqldump = %q, want %q", got, want)
	}
}

func TestInstalledToolDirPatternsAreValidGlobs(t *testing.T) {
	for _, pattern := range installedToolDirPatterns() {
		if _, err := filepath.Match(pattern, ""); err != nil {
			t.Errorf("pattern %q: %v", pattern, err)
		}
	}
}
