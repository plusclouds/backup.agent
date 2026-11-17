package dbdump

import (
	"strings"
	"testing"
)

func TestPickInstance(t *testing.T) {
	tests := []struct {
		name      string
		installed []string
		want      string
		wantErr   string
	}{
		{name: "nothing installed keeps the plain connection", installed: nil, want: ""},
		{name: "a lone named instance is used", installed: []string{"SQLEXPRESS"}, want: "SQLEXPRESS"},
		{name: "the default instance needs no name", installed: []string{"MSSQLSERVER"}, want: ""},
		{name: "the default instance wins over named ones", installed: []string{"SQLEXPRESS", "mssqlserver", "REPORTING"}, want: ""},
		{name: "several named instances and no default is refused", installed: []string{"REPORTING", "SQLEXPRESS"},
			wantErr: "several SQL Server instances (REPORTING, SQLEXPRESS)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickInstance(tt.installed)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("instance = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithLocalInstance(t *testing.T) {
	original := localInstances
	t.Cleanup(func() { localInstances = original })
	localInstances = func() []string { return []string{"SQLEXPRESS"} }

	t.Run("an unconfigured connection gets the installed instance", func(t *testing.T) {
		conn, err := withLocalInstance(Conn{})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := (&mssql{conn: conn}).dsn(), "sqlserver://localhost/SQLEXPRESS?app+name=stash&database=master"; got != want {
			t.Errorf("dsn = %q, want %q", got, want)
		}
	})

	t.Run("credentials alone do not stop detection", func(t *testing.T) {
		conn, err := withLocalInstance(Conn{User: "sa", Password: "x", DumpDir: `D:\bak`})
		if err != nil {
			t.Fatal(err)
		}
		if conn.Instance != "SQLEXPRESS" || conn.User != "sa" || conn.DumpDir != `D:\bak` {
			t.Errorf("conn = %+v", conn)
		}
	})

	for name, conn := range map[string]Conn{
		"host":     {Host: "db.internal"},
		"port":     {Port: 1444},
		"instance": {Instance: "OTHER"},
	} {
		t.Run("a connection that sets "+name+" is left alone", func(t *testing.T) {
			got, err := withLocalInstance(conn)
			if err != nil {
				t.Fatal(err)
			}
			if got != conn {
				t.Errorf("conn changed: %+v → %+v", conn, got)
			}
		})
	}

	t.Run("an ambiguous machine fails instead of guessing", func(t *testing.T) {
		localInstances = func() []string { return []string{"A", "B"} }
		if _, err := (&mssql{}).open(); err == nil {
			t.Fatal("open succeeded with two named instances and no default")
		}
	})
}
