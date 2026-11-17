package dbdump

// installedToolDirPatterns covers where database client tools end up on
// macOS without being on the agent's PATH. That is the normal case here, not
// an exception: a launchd service or a desktop app started from Finder gets
// a bare system PATH that leaves out Homebrew entirely, and several of
// these packages never link their tools into a PATH directory at all.
func installedToolDirPatterns() []string {
	var patterns []string
	// Homebrew: /opt/homebrew on Apple Silicon, /usr/local on Intel.
	for _, prefix := range []string{"/opt/homebrew", "/usr/local"} {
		patterns = append(patterns,
			prefix+"/bin",
			// Versioned and keg-only formulae (mysql@8.0, postgresql@16,
			// libpq, mysql-client) are installed but not linked into bin.
			prefix+"/opt/mysql*/bin",
			prefix+"/opt/mariadb*/bin",
			prefix+"/opt/postgresql*/bin",
			prefix+"/opt/libpq*/bin",
		)
	}
	return append(patterns,
		// Oracle's MySQL .pkg installer.
		"/usr/local/mysql/bin",
		"/usr/local/mysql-*/bin",
		// Postgres.app and the EnterpriseDB installer.
		"/Applications/Postgres.app/Contents/Versions/*/bin",
		"/Library/PostgreSQL/*/bin",
		// MacPorts.
		"/opt/local/bin",
		"/opt/local/lib/mysql*/bin",
		"/opt/local/lib/mariadb*/bin",
		"/opt/local/lib/postgresql*/bin",
		// Local development bundles.
		"/Users/Shared/DBngin/mysql/*/bin",
		"/Users/Shared/DBngin/mariadb/*/bin",
		"/Users/Shared/DBngin/postgresql/*/bin",
		"/Applications/MAMP/Library/bin",
	)
}
