package dbdump

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"

	_ "github.com/microsoft/go-mssqldb" // registers the "sqlserver" database/sql driver
)

// mssql talks to SQL Server directly with the pure-Go driver — no sqlcmd or
// PowerShell needed on the host — and has the server write its own native
// .bak files with BACKUP DATABASE.
type mssql struct {
	conn Conn
}

func (m *mssql) ListDatabases(ctx context.Context) ([]string, error) {
	db, err := m.open()
	if err != nil {
		return nil, err
	}
	defer db.Close() //nolint:errcheck

	// database_id 1-4 are master/tempdb/model/msdb; state 0 is ONLINE;
	// source_database_id is set for database snapshots, which can't be backed up.
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sys.databases
		 WHERE database_id > 4 AND state = 0 AND source_database_id IS NULL
		 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("mssql: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("mssql: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mssql: %w", err)
	}
	return names, nil
}

// Dump has SQL Server write <destDir>/<database>.bak. The file is created by
// the SQL Server service account, not by the agent, so destDir has to be a
// path that account can write to (see DumpDir).
func (m *mssql) Dump(ctx context.Context, database, destDir string) (string, error) {
	db, err := m.open()
	if err != nil {
		return "", err
	}
	defer db.Close() //nolint:errcheck

	path := filepath.Join(destDir, fileName(database)+".bak")
	// Both the database and the file are passed as parameters — BACKUP
	// accepts variables for each — so no name is ever spliced into the SQL.
	// COPY_ONLY keeps this backup out of the database's own backup chain, so
	// an existing full/differential/log schedule is not disturbed.
	// INIT overwrites a leftover file instead of appending a second backup set.
	_, err = db.ExecContext(ctx,
		`BACKUP DATABASE @db TO DISK = @path WITH INIT, COPY_ONLY`,
		sql.Named("db", database), sql.Named("path", path))
	if err != nil {
		return "", fmt.Errorf("mssql: backing up %q to %s: %w", database, path, err)
	}
	return path, nil
}

// DumpDir is the directory SQL Server should write .bak files into: the
// connection's dump_dir if set, otherwise the instance's default backup
// directory.
func (m *mssql) DumpDir(ctx context.Context) (string, error) {
	if m.conn.DumpDir != "" {
		return m.conn.DumpDir, nil
	}
	db, err := m.open()
	if err != nil {
		return "", err
	}
	defer db.Close() //nolint:errcheck

	var dir sql.NullString
	err = db.QueryRowContext(ctx,
		`SELECT CAST(SERVERPROPERTY('InstanceDefaultBackupPath') AS nvarchar(4000))`).Scan(&dir)
	if err != nil {
		return "", fmt.Errorf("mssql: reading the default backup directory: %w", err)
	}
	if !dir.Valid || dir.String == "" {
		return "", fmt.Errorf("mssql: the server reports no default backup directory — set dump_dir for this connection")
	}
	return dir.String, nil
}

func (m *mssql) open() (*sql.DB, error) {
	conn, err := withLocalInstance(m.conn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlserver", (&mssql{conn: conn}).dsn())
	if err != nil {
		return nil, fmt.Errorf("mssql: %w", err)
	}
	return db, nil
}

// dsn builds a sqlserver:// URL. With no user set the driver falls back to
// Windows integrated authentication (the agent's own Windows account).
func (m *mssql) dsn() string {
	host := m.conn.Host
	if host == "" {
		host = "localhost"
	}
	if m.conn.Port != 0 {
		host = net.JoinHostPort(host, strconv.Itoa(m.conn.Port))
	}

	u := url.URL{Scheme: "sqlserver", Host: host}
	if m.conn.Instance != "" {
		u.Path = "/" + m.conn.Instance
	}
	if m.conn.User != "" {
		u.User = url.UserPassword(m.conn.User, m.conn.Password)
	}
	q := url.Values{}
	q.Set("database", "master")
	q.Set("app name", "stash")
	u.RawQuery = q.Encode()
	return u.String()
}
