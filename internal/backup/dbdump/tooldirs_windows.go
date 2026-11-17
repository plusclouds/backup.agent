package dbdump

import (
	"os"
	"path/filepath"
)

// installedToolDirPatterns covers where the MySQL, MariaDB and PostgreSQL
// installers put their client tools. None of them adds that directory to
// PATH, so without this a stock install needed bin_dir set by hand.
//
// Only the server packages are listed: MySQL Workbench ships its own
// mysql.exe and mysqldump.exe too, but those belong to a GUI tool that can
// be older than the server and is not always installed.
func installedToolDirPatterns() []string {
	seen := map[string]bool{}
	var patterns []string
	for _, name := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		root := os.Getenv(name)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		patterns = append(patterns,
			filepath.Join(root, "MySQL", "MySQL Server *", "bin"),
			filepath.Join(root, "MariaDB *", "bin"),
			filepath.Join(root, "PostgreSQL", "*", "bin"),
		)
	}
	return patterns
}
