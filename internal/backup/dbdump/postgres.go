package dbdump

import (
	"context"
	"path/filepath"
	"strconv"
)

// postgres drives psql (listing) and pg_dump (custom-format dumps,
// restorable with pg_restore).
type postgres struct {
	conn Conn
}

// postgresMaintenanceDB is the database every cluster is created with, for
// tools to connect to. It is not a system database — people can and do keep
// tables in it — but on most servers it is empty.
const postgresMaintenanceDB = "postgres"

// listDatabasesSQL lists the databases worth offering for backup. The
// maintenance database is left out while it holds nothing of the user's
// own: it otherwise shows up in every picker, and "all databases" uploads
// an empty dump of it on every run. The query runs while connected to that
// database, so pg_class here describes exactly its contents; anything
// outside the pg_* and information_schema schemas counts as the user's,
// except what an extension brought with it (pg_stat_statements' views are
// in half the clusters out there and hold nothing to back up).
const listDatabasesSQL = `SELECT datname FROM pg_database
WHERE NOT datistemplate AND datallowconn
  AND (datname <> 'postgres' OR EXISTS (
        SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname !~ '^pg_' AND n.nspname <> 'information_schema'
          AND NOT EXISTS (SELECT 1 FROM pg_depend d
                          WHERE d.classid = 'pg_class'::regclass
                            AND d.objid = c.oid AND d.deptype = 'e')))
ORDER BY datname`

func (p *postgres) ListDatabases(ctx context.Context) ([]string, error) {
	args := append(p.connArgs(),
		"--dbname="+postgresMaintenanceDB, "--no-psqlrc", "--tuples-only", "--no-align",
		"--command="+listDatabasesSQL,
	)
	out, err := run(ctx, p.conn.BinDir, "psql", p.env(), args...)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// UnlistedDatabases names the maintenance database: it may be missing from
// ListDatabases, but it exists and a job that names it can still dump it.
func (p *postgres) UnlistedDatabases() []string {
	return []string{postgresMaintenanceDB}
}

func (p *postgres) Dump(ctx context.Context, database, destDir string) (string, error) {
	path := filepath.Join(destDir, fileName(database)+".dump")
	args := append(p.connArgs(), "--format=custom", "--file="+path, "--dbname="+database)
	if _, err := run(ctx, p.conn.BinDir, "pg_dump", p.env(), args...); err != nil {
		return "", err
	}
	return path, nil
}

// connArgs never includes the password, and always passes --no-password so
// a missing credential fails immediately instead of waiting on a prompt
// nobody will ever answer.
func (p *postgres) connArgs() []string {
	args := []string{"--no-password"}
	switch {
	case p.conn.Socket != "":
		args = append(args, "--host="+p.conn.Socket) // libpq: a host starting with / is a socket directory
	case p.conn.Host != "":
		args = append(args, "--host="+p.conn.Host)
	}
	if p.conn.Port != 0 {
		args = append(args, "--port="+strconv.Itoa(p.conn.Port))
	}
	if p.conn.User != "" {
		args = append(args, "--username="+p.conn.User)
	}
	return args
}

func (p *postgres) env() []string {
	if p.conn.Password == "" {
		return nil
	}
	return []string{"PGPASSWORD=" + p.conn.Password}
}
