// Package daemon holds backup-agent's full startup/run sequence — extracted
// out of cmd/backup-agent so it can be embedded in-process by more than one
// caller. The CLI (cmd/backup-agent/run_cmd.go) is one such caller; a
// desktop GUI (desktop/) is another — both get identical behavior (the same
// single-instance lock, engine selection, NATS wiring, scheduler) since both
// call the same Run function, just with different lifecycle/shutdown wiring
// around it.
package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
	"github.com/plusclouds/ubuntu-agent/internal/backup/dispatcher"
	"github.com/plusclouds/ubuntu-agent/internal/backup/engine"
	"github.com/plusclouds/ubuntu-agent/internal/backup/engine/kopia"
	"github.com/plusclouds/ubuntu-agent/internal/backup/engine/s3sync"
	"github.com/plusclouds/ubuntu-agent/internal/backup/publisher"
	"github.com/plusclouds/ubuntu-agent/internal/backup/registry"
	"github.com/plusclouds/ubuntu-agent/internal/backup/runner"
	"github.com/plusclouds/ubuntu-agent/internal/backup/scheduler"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	"github.com/plusclouds/ubuntu-agent/internal/executor"
	"github.com/plusclouds/ubuntu-agent/internal/modules/system"
	natsclient "github.com/plusclouds/ubuntu-agent/internal/nats"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
)

const backupAgentType = "backup"

// Handles are the live objects Run wires up, handed to onReady once startup
// succeeds. A caller embedding the daemon in-process (e.g. a desktop GUI)
// uses these directly for actions — store.SetPaused, runner.RunJob,
// registry.Cancel — the exact same objects the scheduler/dispatcher are
// using, so there's no separate IPC layer or second source of truth.
type Handles struct {
	Store    *state.Store
	Runner   *runner.Runner
	Registry *registry.Registry
	Engine   engine.Engine
}

// Run performs backup-agent's full startup sequence (single-instance lock,
// state load, engine selection per cfg.Agent.SyncEngine, NATS connect,
// dispatcher/scheduler/publisher wiring) and blocks until ctx is cancelled —
// by the caller (e.g. an OS signal handler) or internally, via a platform
// "revoke" command. Run itself installs no signal handling: callers that
// want that (the CLI) wire signal.Notify to cancel ctx themselves, so the
// same function is equally usable from a GUI's own lifecycle, which cancels
// ctx from a tray "Quit" action instead.
//
// onReady, if non-nil, is called once after the scheduler starts,
// with Handles for direct in-process control — see Handles' doc comment.
func Run(ctx context.Context, cfg *backupconfig.Config, logger *zap.Logger, onReady func(Handles)) error {
	logger.Info("backup-agent starting", zap.String("version", backupconfig.AgentVersion))

	if cfg.NATS.AgentUUID == "" || cfg.NATS.APIKey == "" {
		return fmt.Errorf("agent is not registered — run `backup-agent register --token=... --endpoint=...` first")
	}

	// ------------------------------------------------------------------ //
	// Single-instance lock — two processes sharing the same state file and
	// Kopia cache dir corrupt both (observed as a nil-pointer panic deep in
	// Kopia's content cache when a second instance opens the same repo
	// concurrently). Held for the lifetime of the process; the OS releases
	// it automatically even on a crash.
	// ------------------------------------------------------------------ //
	lockPath := cfg.Agent.StateFile + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}
	instanceLock := flock.New(lockPath)
	locked, err := instanceLock.TryLock()
	if err != nil {
		return fmt.Errorf("acquiring instance lock %s: %w", lockPath, err)
	}
	if !locked {
		return fmt.Errorf("another backup-agent instance is already running (lock held on %s)", lockPath)
	}
	defer instanceLock.Unlock() //nolint:errcheck

	// ------------------------------------------------------------------ //
	// State (job list + bookkeeping)
	// ------------------------------------------------------------------ //
	st := state.NewStore(cfg.Agent.StateFile)
	if err := st.Load(); err != nil {
		return fmt.Errorf("loading state file: %w", err)
	}

	// ------------------------------------------------------------------ //
	// Engine (Kopia or s3sync, per cfg.Agent.SyncEngine)
	// ------------------------------------------------------------------ //
	repoPassword, err := engine.DeriveRepositoryPassword(cfg.AccessKey.SecretKey, cfg.NATS.AgentUUID)
	if err != nil {
		return fmt.Errorf("deriving repository password: %w", err)
	}
	// Password and CacheDir are Kopia-only (repository encryption password,
	// local content-cache directory) — both inert for s3sync's live mirror,
	// which keeps no local state and needs no repository encryption. Left
	// populated unconditionally since deriving/holding them is cheap and it
	// keeps this struct identical regardless of which engine consumes it.
	repoCfg := engine.RepoConfig{
		BucketName:     cfg.Bucket.Name,
		BucketEndpoint: cfg.Bucket.Endpoint,
		AccessKey:      cfg.AccessKey.AccessKey,
		SecretKey:      cfg.AccessKey.SecretKey,
		Password:       repoPassword,
		CacheDir:       cfg.Agent.KopiaCacheDir,
	}

	var eng engine.Engine
	switch cfg.Agent.SyncEngine {
	case "", "rsync":
		eng = s3sync.New(logger)
	case "kopia":
		eng = kopia.New(logger)
	default:
		return fmt.Errorf("unknown agent.sync_engine %q (want %q or %q)", cfg.Agent.SyncEngine, "rsync", "kopia")
	}

	// daemonCtx is what every internal component watches for shutdown: it
	// ends when either the caller cancels ctx (OS signal, GUI "Quit") or a
	// platform "revoke" command fires daemonCancel below — same collapsed-
	// to-one-channel shape, whichever happens first.
	daemonCtx, daemonCancel := context.WithCancel(ctx)
	defer daemonCancel()

	// ------------------------------------------------------------------ //
	// NATS
	// ------------------------------------------------------------------ //
	nc, err := natsclient.ConnectForAgentType(cfg.NATS, backupAgentType, cfg.NATS.AgentUUID, cfg.NATS.APIKey, logger)
	if err != nil {
		return fmt.Errorf("NATS connection failed: %w", err)
	}
	defer nc.Drain()

	sysMod := system.New(nil)
	reg := registry.New()
	pub := publisher.New(nc, sysMod, reg, cfg.NATS.AgentUUID, cfg.Agent, logger)

	// EnsureRepository failure is surfaced, not fatal: the agent still
	// connects, heartbeats, and reports job failures explicitly (rather than
	// crash-looping on a transient S3/network issue) — matching this
	// project's "surfaced, not inferred" design principle.
	if err := eng.EnsureRepository(daemonCtx, repoCfg); err != nil {
		logger.Error("could not ensure backup repository/bucket access — job runs will fail until this is resolved",
			zap.String("sync_engine", cfg.Agent.SyncEngine), zap.Error(err))
		if alertErr := pub.PublishAlert(daemonCtx, protocol.AlertPayload{
			Severity: "critical",
			Code:     "REPOSITORY_UNAVAILABLE",
			Message:  "could not open or create the backup repository/bucket access: " + err.Error(),
		}); alertErr != nil {
			logger.Warn("could not publish repository-unavailable alert", zap.Error(alertErr))
		}
	}
	defer eng.Close(context.Background()) //nolint:errcheck

	execTimeoutCeiling := 24 * time.Hour // per-job script_timeout_s enforced via context, not this ceiling
	exec := executor.NewWithTimeout(logger, execTimeoutCeiling)
	run := runner.New(eng, reg, st, exec, logger)

	// onReady fires here — before sched.Start() below — so a caller setting
	// run.OnStart/run.OnComplete (see runner.Runner's doc comment) can never
	// miss a run that fires the instant the scheduler starts.
	if onReady != nil {
		onReady(Handles{Store: st, Runner: run, Registry: reg, Engine: eng})
	}

	sched := scheduler.New(st, run, pub, logger)

	shutdown := func(reason string) {
		logger.Info("shutting down due to revoke command", zap.String("reason", reason))
		daemonCancel()
	}
	disp := dispatcher.New(st, sched, run, reg, cfg.NATS.AgentUUID, cfg.Agent.WorkDir, shutdown, logger)

	if err := nc.Subscribe(func(env protocol.Envelope) {
		result := disp.Dispatch(daemonCtx, env)
		// ReplyTo is set by the platform on sync/RPC-style commands
		// (AgentCommandService::send) — the result must go to that specific
		// inbox subject instead of the general evt subject, or the platform
		// has no way to correlate it back to the waiting caller.
		var publishErr error
		if result.ReplyTo != "" {
			publishErr = nc.PublishToSubject(result.ReplyTo, result)
		} else {
			publishErr = nc.Publish(result)
		}
		if publishErr != nil {
			logger.Error("could not publish result", zap.String("command_id", env.ID), zap.Error(publishErr))
		}
	}); err != nil {
		return fmt.Errorf("subscribing to NATS cmd subject: %w", err)
	}

	if err := sched.Reload(st.Jobs()); err != nil {
		logger.Warn("some persisted jobs have invalid schedules", zap.Error(err))
	}
	sched.Start()
	defer sched.Stop()

	pub.Start(daemonCtx)

	logger.Info("backup-agent started",
		zap.String("nats_url", cfg.NATS.ActiveURL()),
		zap.String("cmd_subject", nc.CmdSubject()),
		zap.String("evt_subject", nc.EvtSubject()),
		zap.Int("jobs", len(st.Jobs())),
		zap.String("sync_engine", cfg.Agent.SyncEngine),
	)

	<-daemonCtx.Done()

	logger.Info("backup-agent stopped cleanly")
	return nil
}

// BuildLogger builds the same zap.Logger configuration the CLI has always
// used (JSON or console encoding per cfg.Log.Format, optional file sink at
// cfg.Log.File) — shared so a GUI's own logging matches the daemon's,
// pointed at its own desktop-mode cfg.Log.File.
func BuildLogger(cfg *backupconfig.Config) (*zap.Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var stdoutEnc zapcore.Encoder
	if cfg.Log.Format == "console" {
		consoleEncCfg := zap.NewDevelopmentEncoderConfig()
		consoleEncCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		stdoutEnc = zapcore.NewConsoleEncoder(consoleEncCfg)
	} else {
		stdoutEnc = zapcore.NewJSONEncoder(encCfg)
	}
	stdoutCore := zapcore.NewCore(stdoutEnc, zapcore.AddSync(os.Stdout), level)
	cores := []zapcore.Core{stdoutCore}

	if cfg.Log.File != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.Log.File), 0755); err != nil {
			return nil, fmt.Errorf("creating log directory: %w", err)
		}
		f, err := os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("opening log file %s: %w", cfg.Log.File, err)
		}
		fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.AddSync(f), level)
		cores = append(cores, fileCore)
	}

	return zap.New(zapcore.NewTee(cores...), zap.AddCaller()), nil
}
