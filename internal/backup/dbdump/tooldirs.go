package dbdump

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// toolDirPatterns lists glob patterns for the directories database client
// tools are installed into on this platform, for when they are not on PATH.
// A variable so tests can substitute their own.
var toolDirPatterns = installedToolDirPatterns

// findInstalledTool looks for file (the tool's file name, with its extension)
// in the platform's usual install directories and returns its full path, or
// "" when it is nowhere to be found. With several versions installed side by
// side in one place the newest wins.
func findInstalledTool(file string) string {
	// Patterns are tried in the order they are listed, so a platform can
	// put the directory its package manager links the current version into
	// ahead of side-by-side older ones. Version order only decides between
	// the directories a single pattern matches.
	for _, pattern := range toolDirPatterns() {
		dirs, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		sort.SliceStable(dirs, func(i, j int) bool { return newerVersion(dirs[i], dirs[j]) })
		for _, dir := range dirs {
			path := filepath.Join(dir, file)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}

// newerVersion reports whether directory a carries a higher version number
// than b ("MySQL Server 8.4" before "MySQL Server 8.0", "PostgreSQL\17"
// before "PostgreSQL\9.6"). Paths with no number in them sort last.
func newerVersion(a, b string) bool {
	va, vb := versionOf(a), versionOf(b)
	for i := 0; i < len(va) || i < len(vb); i++ {
		var x, y int
		if i < len(va) {
			x = va[i]
		}
		if i < len(vb) {
			y = vb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// versionOf pulls the numbers out of the last path element that has any:
// `…\MySQL Server 8.0\bin` → [8 0].
func versionOf(path string) []int {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	for i := len(parts) - 1; i >= 0; i-- {
		var version []int
		for _, field := range strings.FieldsFunc(parts[i], func(r rune) bool { return r < '0' || r > '9' }) {
			if n, err := strconv.Atoi(field); err == nil {
				version = append(version, n)
			}
		}
		if len(version) > 0 {
			return version
		}
	}
	return nil
}
