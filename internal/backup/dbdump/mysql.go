package dbdump

import (
	"context"
	"path/filepath"
	"strconv"
)

// mysqlSystemSchemas are never offered for backup.
var mysqlSystemSchemas = map[string]bool{
	"information_schema": true,
	"performance_schema": true,
	"mysql":              true,
	"sys":                true,
}

// mysql drives the mysql client (listing) and mysqldump (SQL dumps). It
// works unchanged against MariaDB, whose packages ship the same tool names.
type mysql struct {
	conn Conn
}

func (m *mysql) ListDatabases(ctx context.Context) ([]string, error) {
	args := append(m.connArgs(), "--batch", "--skip-column-names", "--execute=SHOW DATABASES")
	out, err := run(ctx, m.conn.BinDir, "mysql", m.env(), args...)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, name := range splitLines(out) {
		if !mysqlSystemSchemas[name] {
			names = append(names, name)
		}
	}
	return names, nil
}

func (m *mysql) Dump(ctx context.Context, database, destDir string) (string, error) {
	path := filepath.Join(destDir, fileName(database)+".sql")
	// --single-transaction: a consistent InnoDB snapshot without locking
	// writers. --databases: the dump carries its own CREATE DATABASE/USE, and
	// the name after it is always read as a database, never as an option.
	args := append(m.connArgs(),
		"--single-transaction", "--routines", "--events", "--triggers",
		"--result-file="+path,
		"--databases", database,
	)
	if _, err := run(ctx, m.conn.BinDir, "mysqldump", m.env(), args...); err != nil {
		return "", err
	}
	return path, nil
}

func (m *mysql) connArgs() []string {
	// Without this the client takes its character set from the operating
	// system's code page; on Windows that can be one MySQL does not know
	// (cp857, Turkish), which adds a warning to every error and leaves
	// non-ASCII database names at the mercy of the fallback.
	args := []string{"--default-character-set=utf8mb4"}
	if m.conn.Socket != "" {
		args = append(args, "--socket="+m.conn.Socket)
	}
	if m.conn.Host != "" {
		args = append(args, "--host="+m.conn.Host)
	}
	if m.conn.Port != 0 {
		// Without --protocol=tcp the client silently ignores the port for
		// host "localhost" and uses the default socket instead.
		args = append(args, "--port="+strconv.Itoa(m.conn.Port), "--protocol=tcp")
	}
	if m.conn.User != "" {
		args = append(args, "--user="+m.conn.User)
	}
	return args
}

func (m *mysql) env() []string {
	if m.conn.Password == "" {
		return nil
	}
	return []string{"MYSQL_PWD=" + m.conn.Password}
}
