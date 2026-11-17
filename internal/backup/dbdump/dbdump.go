// Package dbdump lists and dumps databases for job_type "database" jobs.
// Each engine is a Driver that writes one dump file per database into a
// directory; the runner then hands that directory to the backup engine like
// any other source path, so upload, retention and reporting are unchanged.
package dbdump

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Engine names, as sent in a job's database.engine field.
const (
	EnginePostgres = "postgres"
	EngineMySQL    = "mysql"
	EngineMSSQL    = "mssql"
)

// DefaultConnection is the connection name used when a job names none.
const DefaultConnection = "default"

// Conn is one named connection from the "databases" section of stash.yaml.
// Every field is optional: the zero value means "the engine's own local
// defaults" (unix socket / peer auth, ~/.my.cnf, Windows integrated auth).
type Conn struct {
	Host   string `mapstructure:"host" yaml:"host,omitempty"`
	Port   int    `mapstructure:"port" yaml:"port,omitempty"`
	Socket string `mapstructure:"socket" yaml:"socket,omitempty"`
	User   string `mapstructure:"user" yaml:"user,omitempty"`
	// Password is handed to client tools through the environment, never as a
	// command-line argument.
	Password string `mapstructure:"password" yaml:"password,omitempty"`
	// Instance is a SQL Server named instance (mssql only).
	Instance string `mapstructure:"instance" yaml:"instance,omitempty"`
	// DumpDir is where SQL Server writes its .bak files (mssql only). The
	// files are written by the SQL Server service account, so the directory
	// must be writable by it and readable by the agent. Empty means the
	// instance's default backup directory.
	DumpDir string `mapstructure:"dump_dir" yaml:"dump_dir,omitempty"`
	// BinDir is where the engine's client tools live (pg_dump, mysqldump…)
	// when they are not on the agent's PATH.
	BinDir string `mapstructure:"bin_dir" yaml:"bin_dir,omitempty"`
}

// Driver lists and dumps the databases of one engine over one connection.
type Driver interface {
	// ListDatabases returns the user databases (system databases excluded).
	ListDatabases(ctx context.Context) ([]string, error)
	// Dump writes database into destDir and returns the dump file's path.
	Dump(ctx context.Context, database, destDir string) (string, error)
}

// DumpDirProvider is implemented by drivers whose dump files are written by
// the database server itself rather than by the agent (SQL Server): the
// runner then dumps under that directory instead of its own work directory.
type DumpDirProvider interface {
	DumpDir(ctx context.Context) (string, error)
}

// UnlistedProvider is implemented by drivers that leave some real databases
// out of ListDatabases because they are rarely worth backing up. Such a
// database is not offered and not part of "all", but a job that names it
// explicitly is still honoured rather than failing as "not found".
type UnlistedProvider interface {
	UnlistedDatabases() []string
}

// Factory builds a Driver. New is the real one; tests substitute their own.
type Factory func(engine string, conn Conn) (Driver, error)

// New returns the Driver for engine.
func New(engine string, conn Conn) (Driver, error) {
	switch strings.ToLower(engine) {
	case EnginePostgres:
		return &postgres{conn: conn}, nil
	case EngineMySQL:
		return &mysql{conn: conn}, nil
	case EngineMSSQL:
		return &mssql{conn: conn}, nil
	case "":
		return nil, fmt.Errorf("database engine is not set")
	default:
		return nil, fmt.Errorf("unknown database engine %q (want %q, %q or %q)",
			engine, EnginePostgres, EngineMySQL, EngineMSSQL)
	}
}

// Select returns the databases a job should dump: every user database when
// all is set, otherwise requested — after checking each one actually exists.
// Nothing reaches a command line or a SQL statement without having come back
// from the server's own database list first.
func Select(ctx context.Context, d Driver, requested []string, all bool) ([]string, error) {
	available, err := d.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing databases: %w", err)
	}
	if all {
		if len(available) == 0 {
			return nil, fmt.Errorf("no user databases found")
		}
		return available, nil
	}
	if len(requested) == 0 {
		return nil, fmt.Errorf("no databases selected")
	}

	known := make(map[string]bool, len(available))
	for _, name := range available {
		known[name] = true
	}
	if u, ok := d.(UnlistedProvider); ok {
		for _, name := range u.UnlistedDatabases() {
			known[name] = true
		}
	}
	seen := make(map[string]bool, len(requested))
	var selected, missing []string
	for _, name := range requested {
		if seen[name] {
			continue
		}
		seen[name] = true
		if !known[name] {
			missing = append(missing, name)
			continue
		}
		selected = append(selected, name)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("database(s) not found on the server: %s", strings.Join(missing, ", "))
	}
	return selected, nil
}

// fileName turns a database name into a safe dump file base name: anything
// outside [A-Za-z0-9._-] becomes '_', so a name can never escape destDir.
func fileName(database string) string {
	var b strings.Builder
	for _, r := range database {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.TrimLeft(b.String(), ".")
	if name == "" {
		return "database"
	}
	return name
}

// splitLines returns the non-empty, trimmed lines of s.
func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
