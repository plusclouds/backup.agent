package dbdump

// installedToolDirPatterns covers where database client tools live on Linux
// when the agent's PATH does not reach them. Distribution packages normally
// do put them on PATH, but two cases are common enough to matter: a service
// started with a minimal PATH, and PostgreSQL, whose real binaries sit in a
// per-version directory that only some distributions wrap.
func installedToolDirPatterns() []string {
	return []string{
		"/usr/bin",
		"/usr/local/bin",
		"/usr/sbin",
		"/snap/bin",
		// PostgreSQL: Debian/Ubuntu, then the PGDG packages on RHEL-family
		// systems, then a source install.
		"/usr/lib/postgresql/*/bin",
		"/usr/pgsql-*/bin",
		"/usr/local/pgsql/bin",
		// MySQL and MariaDB installed from the vendor tarball.
		"/usr/local/mysql/bin",
		"/usr/local/mysql-*/bin",
		"/usr/local/mariadb*/bin",
		"/opt/mysql*/bin",
		"/opt/mariadb*/bin",
		// Bundled stacks.
		"/opt/lampp/bin",
		"/opt/bitnami/mysql/bin",
		"/opt/bitnami/mariadb/bin",
		"/opt/bitnami/postgresql/bin",
	}
}
