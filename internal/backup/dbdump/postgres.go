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

func (p *postgres) ListDatabases(ctx context.Context) ([]string, error) {
	args := append(p.connArgs(),
		"--dbname=postgres", "--no-psqlrc", "--tuples-only", "--no-align",
		"--command=SELECT datname FROM pg_database WHERE NOT datistemplate AND datallowconn ORDER BY datname",
	)
	out, err := run(ctx, p.conn.BinDir, "psql", p.env(), args...)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
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
