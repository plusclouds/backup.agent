package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plusclouds/ubuntu-agent/internal/backup/dbdump"
	"github.com/plusclouds/ubuntu-agent/internal/backup/engine"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
)

// fakeDriver stands in for a database engine: Dump writes a small file per
// database unless told to fail or to produce an empty file.
type fakeDriver struct {
	names     []string
	failOn    string
	emptyOn   string
	dumped    []string
	dumpDirIs string // non-empty: also act as a dbdump.DumpDirProvider
}

func (f *fakeDriver) ListDatabases(context.Context) ([]string, error) { return f.names, nil }

func (f *fakeDriver) Dump(_ context.Context, database, destDir string) (string, error) {
	if database == f.failOn {
		return "", errors.New("permission denied")
	}
	content := []byte("dump of " + database)
	if database == f.emptyOn {
		content = nil
	}
	path := filepath.Join(destDir, database+".dump")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", err
	}
	f.dumped = append(f.dumped, database)
	return path, nil
}

type fakeServerSideDriver struct{ *fakeDriver }

func (f fakeServerSideDriver) DumpDir(context.Context) (string, error) { return f.dumpDirIs, nil }

type dbTest struct {
	runner   *Runner
	driver   *fakeDriver
	snapshot *engine.SnapshotSpec // what the engine was asked to back up
	files    []string             // dump files present at snapshot time
	gotConn  string               // "engine/connection" the runner resolved
}

func newDBTest(t *testing.T, drv *fakeDriver) *dbTest {
	t.Helper()
	r, eng, _ := newTestRunner(t)
	dt := &dbTest{runner: r, driver: drv}

	r.DumpDir = t.TempDir()
	r.DBConn = func(engineName, connection string) dbdump.Conn {
		dt.gotConn = engineName + "/" + connection
		return dbdump.Conn{}
	}
	r.NewDriver = func(string, dbdump.Conn) (dbdump.Driver, error) {
		if drv.dumpDirIs != "" {
			return fakeServerSideDriver{drv}, nil
		}
		return drv, nil
	}
	// The dump directory only exists during the run, so capture its content
	// at the moment the engine would upload it.
	eng.SnapshotFunc = func(_ context.Context, spec engine.SnapshotSpec) (engine.SnapshotResult, error) {
		dt.snapshot = &spec
		for _, src := range spec.SourcePaths {
			entries, err := os.ReadDir(src)
			if err != nil {
				return engine.SnapshotResult{}, err
			}
			for _, e := range entries {
				dt.files = append(dt.files, e.Name())
			}
		}
		return engine.SnapshotResult{SnapshotID: "snap-1"}, nil
	}
	return dt
}

func databaseJob(spec *state.DatabaseSpec) state.Job {
	return state.Job{
		UUID: "job-db", Name: "db-job", JobType: state.JobTypeDatabase,
		Schedule: "@every 1h", Database: spec,
		// Must be ignored for database jobs.
		SourcePaths: []string{"/nonexistent/should/be/ignored"},
	}
}

func TestRunJobDatabaseDumpsSelectedAndCleansUp(t *testing.T) {
	dt := newDBTest(t, &fakeDriver{names: []string{"app", "shop", "wiki"}})
	job := databaseJob(&state.DatabaseSpec{Engine: "postgres", Connection: "replica", Databases: []string{"shop", "app"}})

	outcome, err := dt.runner.RunJob(context.Background(), job, "run-1")
	if err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	if outcome.Status != "completed" {
		t.Fatalf("expected completed, got %q (err=%v)", outcome.Status, deref(outcome.Error))
	}
	if dt.gotConn != "postgres/replica" {
		t.Errorf("runner resolved connection %q, want postgres/replica", dt.gotConn)
	}
	if got := strings.Join(dt.files, ","); got != "app.dump,shop.dump" {
		t.Errorf("engine saw files %q, want only the two selected databases", got)
	}
	if dt.snapshot == nil || len(dt.snapshot.SourcePaths) != 1 {
		t.Fatalf("expected exactly one source path, got %+v", dt.snapshot)
	}
	dumpDir := dt.snapshot.SourcePaths[0]
	if filepath.Dir(dumpDir) != dt.runner.DumpDir {
		t.Errorf("dump directory %s is not under DumpDir %s", dumpDir, dt.runner.DumpDir)
	}
	if _, err := os.Stat(dumpDir); !os.IsNotExist(err) {
		t.Errorf("dump directory should be removed after the run, stat err = %v", err)
	}
}

func TestRunJobDatabaseAll(t *testing.T) {
	dt := newDBTest(t, &fakeDriver{names: []string{"app", "shop"}})

	outcome, _ := dt.runner.RunJob(context.Background(), databaseJob(&state.DatabaseSpec{Engine: "mysql", All: true}), "run-1")
	if outcome.Status != "completed" {
		t.Fatalf("expected completed, got %q (err=%v)", outcome.Status, deref(outcome.Error))
	}
	if got := strings.Join(dt.driver.dumped, ","); got != "app,shop" {
		t.Errorf("dumped %q, want every database", got)
	}
}

func TestRunJobDatabaseFailuresNeverTouchEngine(t *testing.T) {
	tests := []struct {
		name    string
		driver  *fakeDriver
		spec    *state.DatabaseSpec
		wantErr string
	}{
		{
			name:    "unknown database",
			driver:  &fakeDriver{names: []string{"app"}},
			spec:    &state.DatabaseSpec{Engine: "postgres", Databases: []string{"app", "gone"}},
			wantErr: "gone",
		},
		{
			name:    "dump fails midway",
			driver:  &fakeDriver{names: []string{"app", "shop"}, failOn: "shop"},
			spec:    &state.DatabaseSpec{Engine: "postgres", All: true},
			wantErr: "shop: permission denied",
		},
		{
			name:    "empty dump file",
			driver:  &fakeDriver{names: []string{"app"}, emptyOn: "app"},
			spec:    &state.DatabaseSpec{Engine: "postgres", All: true},
			wantErr: "empty",
		},
		{
			name:    "nothing selected",
			driver:  &fakeDriver{names: []string{"app"}},
			spec:    &state.DatabaseSpec{Engine: "postgres"},
			wantErr: "no databases selected",
		},
		{
			name:    "no database settings at all",
			driver:  &fakeDriver{names: []string{"app"}},
			spec:    nil,
			wantErr: "no database settings",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt := newDBTest(t, tt.driver)

			outcome, err := dt.runner.RunJob(context.Background(), databaseJob(tt.spec), "run-1")
			if err != nil {
				t.Fatalf("RunJob returned an unexpected top-level error: %v", err)
			}
			if outcome.Status != "failed" || outcome.Error == nil {
				t.Fatalf("expected a failed outcome with an error, got %+v", outcome)
			}
			if !strings.Contains(*outcome.Error, tt.wantErr) {
				t.Errorf("error %q does not mention %q", *outcome.Error, tt.wantErr)
			}
			if dt.snapshot != nil {
				t.Errorf("engine must not be reached after a failed dump, got %+v", dt.snapshot)
			}
			entries, _ := os.ReadDir(dt.runner.DumpDir)
			if len(entries) != 0 {
				t.Errorf("dump directory left behind after a failed run: %v", entries)
			}
		})
	}
}

func TestRunJobDatabaseRemovesLeftoversFromEarlierRun(t *testing.T) {
	dt := newDBTest(t, &fakeDriver{names: []string{"app"}})
	stale := filepath.Join(dt.runner.DumpDir, "stash-dump-job-db")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "old.dump"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	outcome, _ := dt.runner.RunJob(context.Background(), databaseJob(&state.DatabaseSpec{Engine: "postgres", All: true}), "run-1")
	if outcome.Status != "completed" {
		t.Fatalf("expected completed, got %q (err=%v)", outcome.Status, deref(outcome.Error))
	}
	if got := strings.Join(dt.files, ","); got != "app.dump" {
		t.Errorf("engine saw %q — a stale dump from an earlier run was uploaded", got)
	}
}

func TestRunJobDatabaseUsesServerSideDumpDir(t *testing.T) {
	serverDir := t.TempDir()
	dt := newDBTest(t, &fakeDriver{names: []string{"app"}, dumpDirIs: serverDir})

	outcome, _ := dt.runner.RunJob(context.Background(), databaseJob(&state.DatabaseSpec{Engine: "mssql", All: true}), "run-1")
	if outcome.Status != "completed" {
		t.Fatalf("expected completed, got %q (err=%v)", outcome.Status, deref(outcome.Error))
	}
	if got := filepath.Dir(dt.snapshot.SourcePaths[0]); got != serverDir {
		t.Errorf("dumped under %s, want the driver's own dump directory %s", got, serverDir)
	}
	entries, _ := os.ReadDir(serverDir)
	if len(entries) != 0 {
		t.Errorf("server-side dump directory not cleaned up: %v", entries)
	}
}

func TestListDatabases(t *testing.T) {
	dt := newDBTest(t, &fakeDriver{names: []string{"app", "shop"}})

	names, err := dt.runner.ListDatabases(context.Background(), "mysql", "")
	if err != nil {
		t.Fatalf("ListDatabases: %v", err)
	}
	if strings.Join(names, ",") != "app,shop" {
		t.Errorf("got %v", names)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// The backend stores source_paths as [] for database jobs (the column is
// NOT NULL), so that is what full_sync delivers. It must not trip the
// "no source_paths configured" check that applies to files jobs.
func TestRunJobDatabaseWithEmptySourcePaths(t *testing.T) {
	for name, paths := range map[string][]string{"empty": {}, "absent": nil} {
		t.Run(name, func(t *testing.T) {
			dt := newDBTest(t, &fakeDriver{names: []string{"app"}})
			job := databaseJob(&state.DatabaseSpec{Engine: "mssql", All: true})
			job.SourcePaths = paths

			outcome, err := dt.runner.RunJob(context.Background(), job, "run-1")
			if err != nil {
				t.Fatalf("RunJob: %v", err)
			}
			if outcome.Status != "completed" {
				t.Fatalf("expected completed, got %q (err=%v)", outcome.Status, deref(outcome.Error))
			}
			if got := strings.Join(dt.files, ","); got != "app.dump" {
				t.Errorf("engine saw %q, want app.dump", got)
			}
		})
	}
}
