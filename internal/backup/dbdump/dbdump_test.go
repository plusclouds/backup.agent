package dbdump

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type stubDriver struct {
	names []string
	err   error
}

func (s stubDriver) ListDatabases(context.Context) ([]string, error) { return s.names, s.err }
func (s stubDriver) Dump(context.Context, string, string) (string, error) {
	return "", errors.New("not used")
}

func TestSelect(t *testing.T) {
	drv := stubDriver{names: []string{"app", "shop", "wiki"}}

	tests := []struct {
		name      string
		requested []string
		all       bool
		want      []string
		wantErr   string
	}{
		{name: "all", all: true, want: []string{"app", "shop", "wiki"}},
		{name: "all ignores the requested list", requested: []string{"nope"}, all: true, want: []string{"app", "shop", "wiki"}},
		{name: "subset keeps the requested order", requested: []string{"wiki", "app"}, want: []string{"wiki", "app"}},
		{name: "duplicates collapse", requested: []string{"app", "app"}, want: []string{"app"}},
		{name: "unknown database", requested: []string{"app", "zzz", "aaa"}, wantErr: "aaa, zzz"},
		{name: "nothing selected", wantErr: "no databases selected"},
		{name: "option-looking name is just an unknown database", requested: []string{"--all-databases"}, wantErr: "--all-databases"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Select(context.Background(), drv, tt.requested, tt.all)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected an error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Select: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectAllWithNoDatabasesFails(t *testing.T) {
	if _, err := Select(context.Background(), stubDriver{}, nil, true); err == nil {
		t.Fatal("expected an error when the server has no user databases")
	}
}

func TestSelectSurfacesListError(t *testing.T) {
	_, err := Select(context.Background(), stubDriver{err: errors.New("access denied")}, []string{"app"}, false)
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected the list error to be surfaced, got %v", err)
	}
}

func TestFileNameCannotEscapeDirectory(t *testing.T) {
	tests := map[string]string{
		"app":            "app",
		"my-db_2.bak":    "my-db_2.bak",
		"../../etc/pass": "_.._etc_pass",
		`c:\x`:           "c__x",
		"with space":     "with_space",
		"..":             "database",
		"":               "database",
	}
	for in, want := range tests {
		got := fileName(in)
		if got != want {
			t.Errorf("fileName(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(got, `/\`) || strings.HasPrefix(got, ".") {
			t.Errorf("fileName(%q) = %q is not a safe base name", in, got)
		}
	}
}

func TestNewRejectsUnknownEngine(t *testing.T) {
	for _, engine := range []string{EnginePostgres, EngineMySQL, EngineMSSQL, "MySQL"} {
		if _, err := New(engine, Conn{}); err != nil {
			t.Errorf("New(%q): %v", engine, err)
		}
	}
	for _, engine := range []string{"", "oracle"} {
		if _, err := New(engine, Conn{}); err == nil {
			t.Errorf("New(%q): expected an error", engine)
		}
	}
}

// Passwords must only ever travel in the environment: command-line
// arguments are visible to every local user through the process list.
func TestPasswordNeverInArguments(t *testing.T) {
	conn := Conn{Host: "db", Port: 1234, User: "u", Password: "s3cret"}

	pg := &postgres{conn: conn}
	my := &mysql{conn: conn}
	for name, args := range map[string][]string{"postgres": pg.connArgs(), "mysql": my.connArgs()} {
		if strings.Contains(strings.Join(args, " "), "s3cret") {
			t.Errorf("%s: password leaked into arguments: %v", name, args)
		}
	}
	if got := pg.env(); !reflect.DeepEqual(got, []string{"PGPASSWORD=s3cret"}) {
		t.Errorf("postgres env = %v", got)
	}
	if got := my.env(); !reflect.DeepEqual(got, []string{"MYSQL_PWD=s3cret"}) {
		t.Errorf("mysql env = %v", got)
	}
}

func TestMSSQLDSN(t *testing.T) {
	tests := []struct {
		name string
		conn Conn
		want string
	}{
		{name: "defaults use integrated auth on localhost", conn: Conn{}, want: "sqlserver://localhost?app+name=stash&database=master"},
		{name: "sql login with port", conn: Conn{Host: "10.0.0.5", Port: 1433, User: "sa", Password: "p@ss/word"},
			want: "sqlserver://sa:p%40ss%2Fword@10.0.0.5:1433?app+name=stash&database=master"},
		{name: "named instance", conn: Conn{Host: "srv", Instance: "SQLEXPRESS"}, want: "sqlserver://srv/SQLEXPRESS?app+name=stash&database=master"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&mssql{conn: tt.conn}).dsn(); got != tt.want {
				t.Errorf("dsn = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunReportsMissingTool(t *testing.T) {
	_, err := run(context.Background(), t.TempDir(), "pg_dump", nil)
	if err == nil || !strings.Contains(err.Error(), "bin_dir") {
		t.Fatalf("expected a bin_dir error, got %v", err)
	}
}

type stubDriverWithUnlisted struct {
	stubDriver
	unlisted []string
}

func (s stubDriverWithUnlisted) UnlistedDatabases() []string { return s.unlisted }

func TestSelectHonoursUnlistedDatabases(t *testing.T) {
	drv := stubDriverWithUnlisted{
		stubDriver: stubDriver{names: []string{"app", "shop"}},
		unlisted:   []string{"postgres"},
	}

	// Named explicitly, an unlisted database is still dumped…
	got, err := Select(context.Background(), drv, []string{"postgres", "app"}, false)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if want := []string{"postgres", "app"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// …but it is not part of "all"…
	got, err = Select(context.Background(), drv, nil, true)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if want := []string{"app", "shop"}; !reflect.DeepEqual(got, want) {
		t.Errorf("all = %v, want %v", got, want)
	}

	// …and it does not make other unknown names acceptable.
	if _, err := Select(context.Background(), drv, []string{"template1"}, false); err == nil {
		t.Error("an unknown database was accepted")
	}
}

func TestPostgresUnlistedIsOnlyTheMaintenanceDatabase(t *testing.T) {
	if got, want := (&postgres{}).UnlistedDatabases(), []string{"postgres"}; !reflect.DeepEqual(got, want) {
		t.Errorf("unlisted = %v, want %v", got, want)
	}
}
