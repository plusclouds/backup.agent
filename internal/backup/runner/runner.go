// Package runner executes a single backup job run: for job_type "script" it
// runs pre_script first and fails fast — before Kopia is ever touched — on a
// non-zero exit or on missing/empty source data, per the "silent failure"
// motivation in docs/backup.agent/overview.md. It's shared by the cron
// scheduler (scheduled runs) and the dispatcher's run_job_now handler
// (platform-forced runs), so both paths get identical guarantees.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/backup/dbdump"
	"github.com/plusclouds/ubuntu-agent/internal/backup/engine"
	"github.com/plusclouds/ubuntu-agent/internal/backup/registry"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	"github.com/plusclouds/ubuntu-agent/internal/executor"
)

// ErrAlreadyRunning is returned by RunJob when the job is already executing
// (per the registry) — callers must not queue or retry, only reject.
var ErrAlreadyRunning = errors.New("job already running")

const defaultScriptTimeout = 5 * time.Minute

// defaultDumpTimeout bounds a database job's whole dump phase when the job
// sets no script_timeout_s — far longer than a script's default, since
// dumping a real database routinely takes more than five minutes.
const defaultDumpTimeout = time.Hour

// Outcome is the result of one job run, regardless of trigger (cron or
// run_job_now) — the shape both the job_run event and the run_job_now
// result payload are built from.
type Outcome struct {
	RunUUID       string
	Status        string // protocol.StatusCompleted or protocol.StatusFailed
	StartedAt     time.Time
	FinishedAt    time.Time
	BytesUploaded int64
	BytesDeduped  int64
	SnapshotID    string
	Error         *string
}

// Runner orchestrates a single job run against an Engine.
type Runner struct {
	engine   engine.Engine
	registry *registry.Registry
	store    *state.Store
	exec     *executor.Executor
	logger   *zap.Logger

	// OnStart/OnComplete, if set, are called synchronously from RunJob at the
	// obvious points (nil-checked — no-op for callers that don't set them,
	// e.g. the CLI daemon). They exist so a caller embedding the daemon
	// in-process (the desktop GUI) can react to every run regardless of what
	// triggered it — cron schedule, run_job_now (platform or GUI) — without
	// polling the state store on a timer. Set these before the scheduler
	// starts (i.e. right after New) to avoid missing an early run.
	OnStart    func(job state.Job)
	OnComplete func(job state.Job, outcome Outcome)

	// Database support (job_type "database"), set by the caller right after
	// New, same as the hooks above. All three are optional:
	//   - DBConn resolves a job's (engine, connection name) to local
	//     connection settings; nil means engine defaults for everything.
	//   - DumpDir is the parent of the per-job scratch directory dumps are
	//     written to; empty means the OS temp directory.
	//   - NewDriver builds the engine driver; nil means dbdump.New.
	DBConn    func(engine, connection string) dbdump.Conn
	DumpDir   string
	NewDriver dbdump.Factory
}

// New creates a Runner. exec should be built with a generous timeout ceiling
// (e.g. NewWithTimeout(logger, time.Hour)) — the per-job ScriptTimeoutS is
// enforced via context, not the Executor's own timeout, so the ceiling here
// must never be shorter than the longest expected script_timeout_s.
func New(eng engine.Engine, reg *registry.Registry, store *state.Store, exec *executor.Executor, logger *zap.Logger) *Runner {
	return &Runner{engine: eng, registry: reg, store: store, exec: exec, logger: logger}
}

// RunJob executes job under runUUID, guarded by the registry so the same job
// UUID never runs twice concurrently. It always records the outcome to the
// state store before returning (except when rejected as already-running,
// where there's nothing new to record).
func (r *Runner) RunJob(ctx context.Context, job state.Job, runUUID string) (Outcome, error) {
	runCtx, done, ok := r.registry.TryStart(ctx, job.UUID, runUUID)
	if !ok {
		return Outcome{}, ErrAlreadyRunning
	}
	defer done()

	if r.OnStart != nil {
		r.OnStart(job)
	}

	outcome := Outcome{RunUUID: runUUID, StartedAt: time.Now().UTC()}

	if job.JobType == state.JobTypeScript {
		if err := r.runPreScript(runCtx, job); err != nil {
			return r.fail(job, outcome, fmt.Errorf("pre_script: %w", err)), nil
		}
	}

	sourcePaths := job.SourcePaths
	if job.JobType == state.JobTypeDatabase {
		dumpDir, cleanup, err := r.dumpDatabases(runCtx, job)
		// The dumps are scratch data: gone once the run ends, whether the
		// snapshot below succeeded or not.
		defer cleanup()
		if err != nil {
			return r.fail(job, outcome, fmt.Errorf("database dump: %w", err)), nil
		}
		sourcePaths = []string{dumpDir}
	}

	if err := ensureSourcePathsPresent(sourcePaths); err != nil {
		return r.fail(job, outcome, err), nil
	}

	result, err := r.engine.Snapshot(runCtx, engine.SnapshotSpec{
		JobUUID:            job.UUID,
		SourcePaths:        sourcePaths,
		KeepLastN:          job.KeepLastN,
		KeepForDays:        job.KeepForDays,
		BandwidthLimitMbps: job.BandwidthLimitMbps,
		BucketName:         job.BucketName,
	})
	if err != nil {
		return r.fail(job, outcome, fmt.Errorf("snapshot: %w", err)), nil
	}

	outcome.FinishedAt = time.Now().UTC()
	outcome.Status = "completed"
	outcome.BytesUploaded = result.BytesUploaded
	outcome.BytesDeduped = result.BytesDeduped
	outcome.SnapshotID = result.SnapshotID

	if err := r.store.RecordRun(job.UUID, outcome.FinishedAt, "completed", result.SnapshotID); err != nil {
		r.logger.Warn("could not record successful run", zap.String("job_uuid", job.UUID), zap.Error(err))
	}
	if r.OnComplete != nil {
		r.OnComplete(job, outcome)
	}
	return outcome, nil
}

func (r *Runner) fail(job state.Job, outcome Outcome, cause error) Outcome {
	outcome.FinishedAt = time.Now().UTC()
	outcome.Status = "failed"
	msg := cause.Error()
	outcome.Error = &msg

	if err := r.store.RecordRun(job.UUID, outcome.FinishedAt, "failed", ""); err != nil {
		r.logger.Warn("could not record failed run", zap.String("job_uuid", job.UUID), zap.Error(err))
	}
	if r.OnComplete != nil {
		r.OnComplete(job, outcome)
	}
	return outcome
}

// runPreScript writes job.PreScript to a temp file and executes it, bounded
// by job.ScriptTimeoutS (or defaultScriptTimeout if unset). A non-zero exit
// fails the run outright — the caller never reaches Snapshot in that case.
func (r *Runner) runPreScript(ctx context.Context, job state.Job) error {
	timeout := time.Duration(job.ScriptTimeoutS) * time.Second
	if timeout <= 0 {
		timeout = defaultScriptTimeout
	}
	scriptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	path, cleanup, err := writeTempScript(job.PreScript)
	if err != nil {
		return fmt.Errorf("writing pre_script to temp file: %w", err)
	}
	defer cleanup()

	if runtime.GOOS == "windows" {
		_, _, err = r.exec.Execute(scriptCtx, "cmd", "/C", path)
	} else {
		_, _, err = r.exec.Execute(scriptCtx, path)
	}
	if err != nil {
		return err
	}
	return nil
}

// driver builds the dump driver for a (engine, connection name) pair using
// the Runner's DBConn/NewDriver hooks.
func (r *Runner) driver(engineName, connection string) (dbdump.Driver, error) {
	var conn dbdump.Conn
	if r.DBConn != nil {
		conn = r.DBConn(engineName, connection)
	}
	factory := r.NewDriver
	if factory == nil {
		factory = dbdump.New
	}
	return factory(engineName, conn)
}

// ListDatabases returns the user databases visible over the given local
// connection — what the platform's list_databases command shows in its
// database picker.
func (r *Runner) ListDatabases(ctx context.Context, engineName, connection string) ([]string, error) {
	drv, err := r.driver(engineName, connection)
	if err != nil {
		return nil, err
	}
	return drv.ListDatabases(ctx)
}

// dumpDatabases dumps every database job.Database selects into a fresh
// per-job directory and returns that directory. Any failure — unknown
// database, failed dump, timeout — is returned before the backup engine is
// touched, the same guarantee pre_script gives. cleanup is always non-nil
// and safe to call, including after an error.
func (r *Runner) dumpDatabases(ctx context.Context, job state.Job) (dir string, cleanup func(), err error) {
	cleanup = func() {}

	spec := job.Database
	if spec == nil {
		return "", cleanup, fmt.Errorf("job has no database settings")
	}

	timeout := time.Duration(job.ScriptTimeoutS) * time.Second
	if timeout <= 0 {
		timeout = defaultDumpTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	drv, err := r.driver(spec.Engine, spec.Connection)
	if err != nil {
		return "", cleanup, err
	}
	databases, err := dbdump.Select(ctx, drv, spec.Databases, spec.All)
	if err != nil {
		return "", cleanup, err
	}

	base := r.DumpDir
	if base == "" {
		base = os.TempDir()
	}
	// SQL Server writes the dump files itself, so they have to go where its
	// service account can write, not into the agent's own work directory.
	if p, ok := drv.(dbdump.DumpDirProvider); ok {
		if base, err = p.DumpDir(ctx); err != nil {
			return "", cleanup, err
		}
	}

	dir = filepath.Join(base, "stash-dump-"+job.UUID)
	// A crashed earlier run may have left files behind; they must not be
	// mistaken for, or uploaded alongside, this run's dumps.
	if err := os.RemoveAll(dir); err != nil {
		return "", cleanup, fmt.Errorf("clearing dump directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", cleanup, fmt.Errorf("creating dump directory: %w", err)
	}
	// A copy, not the named result: error returns below set dir to "", and
	// the cleanup must still remove the real directory.
	dumpDir := dir
	cleanup = func() {
		if err := os.RemoveAll(dumpDir); err != nil {
			r.logger.Warn("could not remove dump directory", zap.String("dir", dumpDir), zap.Error(err))
		}
	}

	for _, name := range databases {
		started := time.Now()
		path, err := drv.Dump(ctx, name, dir)
		if err != nil {
			return "", cleanup, fmt.Errorf("%s: %w", name, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", cleanup, fmt.Errorf("%s: dump file missing after dump: %w", name, err)
		}
		if info.Size() == 0 {
			return "", cleanup, fmt.Errorf("%s: dump file is empty", name)
		}
		r.logger.Info("database dumped",
			zap.String("job_uuid", job.UUID),
			zap.String("engine", spec.Engine),
			zap.String("database", name),
			zap.Int64("bytes", info.Size()),
			zap.Duration("elapsed", time.Since(started)),
		)
	}
	return dir, cleanup, nil
}

// writeTempScript writes content to a new temp file, executable on
// non-Windows platforms (the shebang, if any, is honored by the OS).
func writeTempScript(content string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "backup-agent-prescript-*")
	if err != nil {
		return "", nil, err
	}
	defer f.Close() //nolint:errcheck

	if _, err := f.WriteString(content); err != nil {
		os.Remove(f.Name()) //nolint:errcheck
		return "", nil, err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(f.Name(), 0o700); err != nil {
			os.Remove(f.Name()) //nolint:errcheck
			return "", nil, err
		}
	}
	return f.Name(), func() { os.Remove(f.Name()) }, nil //nolint:errcheck
}

// ensureSourcePathsPresent fails the run if source_paths is empty, or any
// entry doesn't exist / is an empty file / is an empty directory. This is
// the doc's "missing/empty output" check, applied uniformly to both job
// types as a belt-and-suspenders guard against a silently-empty backup.
func ensureSourcePathsPresent(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("no source_paths configured")
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("source path %q: %w", p, err)
		}
		if info.IsDir() {
			nonEmpty, err := dirHasEntries(p)
			if err != nil {
				return fmt.Errorf("reading source path %q: %w", p, err)
			}
			if !nonEmpty {
				return fmt.Errorf("source path %q is an empty directory", p)
			}
			continue
		}
		if info.Size() == 0 {
			return fmt.Errorf("source path %q is an empty file", p)
		}
	}
	return nil
}

func dirHasEntries(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close() //nolint:errcheck

	_, err = f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
