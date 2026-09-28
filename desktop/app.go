// MIT License
//
// Copyright (c) 2026 PlusClouds
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
	"github.com/plusclouds/ubuntu-agent/internal/backup/daemon"
	"github.com/plusclouds/ubuntu-agent/internal/backup/register"
	"github.com/plusclouds/ubuntu-agent/internal/backup/runner"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	vmconfig "github.com/plusclouds/ubuntu-agent/internal/config"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// changedEvent is emitted (via Wails' Events runtime) whenever something
// GetStatus reflects might have changed — job started/finished (cron,
// run_job_now from either the GUI or the platform), or the daemon itself
// erroring out. The frontend listens for this instead of polling GetStatus
// on a fixed timer; see frontend/src/main.ts.
const changedEvent = "stash:changed"

// App is the Wails-bound struct: its exported methods are callable directly
// from the frontend (Wails' binding mechanism generates the JS/TS glue).
type App struct {
	ctx context.Context

	mu         sync.Mutex
	cfg        *backupconfig.Config
	logger     *zap.Logger
	handles    daemon.Handles
	daemonErr  error // set if the background daemon.Run goroutine exits with an error
	daemonQuit context.CancelFunc
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called once by Wails when the app starts. If a config already
// exists (the user has registered before), it starts the embedded daemon in
// the background — same daemon.Run every CLI `stash run` invocation
// uses, just called in-process instead of from a cobra command.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	path, err := desktopConfigPath()
	if err != nil {
		println("could not resolve desktop config path:", err.Error())
		return
	}
	cfg, err := backupconfig.Load(path)
	if err != nil {
		println("could not load config:", err.Error())
		return
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()

	if cfg.NATS.AgentUUID != "" && cfg.NATS.APIKey != "" {
		a.startDaemon()
	}
}

// emitChanged notifies the frontend that GetStatus should be re-fetched.
// No-op before Wails has assigned a.ctx (e.g. a hook firing during the
// narrow window before OnStartup runs — shouldn't happen in practice, but
// cheap to guard).
func (a *App) emitChanged() {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, changedEvent)
	}
}

// onResume is wired to windows.Options.OnResume (see wailsopts.go), which
// fires on WM_POWERBROADCAST when Windows exits Modern Standby. WebView2's
// GPU compositor has been observed deadlocking against the host's Intel iGPU
// driver a few minutes after such a resume — Application Hang events for
// stash-desktop.exe in the Windows Event Log, timestamped shortly after
// Kernel-Power resume events. Same class of dxgkrnl deadlock as the
// launch-time hang WebviewGpuIsDisabled addresses, just triggered later by
// the next natural repaint instead of startup. Forcing a hide/show cycle a
// couple seconds after resume — once the driver has had a moment to settle —
// makes WebView2 recreate its render surface proactively instead of wedging
// on that later repaint.
func (a *App) onResume() {
	go func() {
		time.Sleep(2 * time.Second)
		if a.ctx == nil {
			return
		}
		wailsruntime.WindowHide(a.ctx)
		time.Sleep(200 * time.Millisecond)
		wailsruntime.WindowShow(a.ctx)
	}()
}

// shutdown is called by Wails when the app is closing (tray "Quit") — stop
// the embedded daemon cleanly before the process exits.
func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	cancel := a.daemonQuit
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// startDaemon launches daemon.Run in the background and records Handles
// once it's ready. Safe to call only when not already running (startup and
// Register are the only two call sites, both single-shot).
func (a *App) startDaemon() {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()

	logger, err := daemon.BuildLogger(cfg)
	if err != nil {
		println("could not build logger:", err.Error())
		return
	}

	daemonCtx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.logger = logger
	a.daemonQuit = cancel
	a.mu.Unlock()

	go func() {
		err := daemon.Run(daemonCtx, cfg, logger, func(h daemon.Handles) {
			h.Runner.OnStart = func(_ state.Job) { a.emitChanged() }
			h.Runner.OnComplete = func(_ state.Job, _ runner.Outcome) { a.emitChanged() }

			a.mu.Lock()
			a.handles = h
			a.mu.Unlock()
		})
		a.mu.Lock()
		a.daemonErr = err
		a.mu.Unlock()
		if err != nil {
			logger.Error("daemon exited with error", zap.Error(err))
		}
		a.emitChanged() // reflect DaemonErr/DaemonUp changing, even on a clean exit
	}()
}

// --- Bound methods, callable from the frontend ---

// JobView is one row of the Jobs table shown in the UI.
type JobView struct {
	UUID          string `json:"uuid"`
	Name          string `json:"name"`
	Schedule      string `json:"schedule"`
	Paused        bool   `json:"paused"`
	LastRunStatus string `json:"lastRunStatus"`
	LastRunAt     string `json:"lastRunAt"`
	Running       bool   `json:"running"`
}

// StatusView is what GetStatus returns to the frontend.
type StatusView struct {
	Registered bool      `json:"registered"`
	AgentUUID  string    `json:"agentUuid"`
	Bucket     string    `json:"bucket"`
	SyncEngine string    `json:"syncEngine"`
	DaemonUp   bool      `json:"daemonUp"`
	DaemonErr  string    `json:"daemonErr,omitempty"`
	Jobs       []JobView `json:"jobs"`
}

// GetStatus reports the agent's identity/bucket/engine and job list — the
// same fields cmd/backup-agent/status_cmd.go already prints, plus a live
// "Running" flag (from the registry, only available while the daemon is up)
// that the CLI's file-only status command can't show.
func (a *App) GetStatus() StatusView {
	a.mu.Lock()
	cfg := a.cfg
	handles := a.handles
	daemonErr := a.daemonErr
	a.mu.Unlock()

	if cfg == nil || cfg.NATS.AgentUUID == "" {
		return StatusView{Registered: false}
	}

	view := StatusView{
		Registered: true,
		AgentUUID:  cfg.NATS.AgentUUID,
		Bucket:     cfg.Bucket.Name,
		SyncEngine: cfg.Agent.SyncEngine,
		DaemonUp:   handles.Store != nil,
		// Jobs must marshal as [] not null when empty — a nil Go slice
		// encodes to JSON null, and the frontend calls .map() on it
		// unconditionally (observed crashing on a freshly registered agent
		// with no jobs yet: "TypeError: null is not an object (evaluating
		// 't.jobs.map')").
		Jobs: []JobView{},
	}
	if daemonErr != nil {
		view.DaemonErr = daemonErr.Error()
	}
	if handles.Store == nil {
		return view
	}

	for _, j := range handles.Store.Jobs() {
		_, running := handles.Registry.IsRunning(j.UUID)
		view.Jobs = append(view.Jobs, JobView{
			UUID:          j.UUID,
			Name:          j.Name,
			Schedule:      j.Schedule,
			Paused:        j.Paused,
			LastRunStatus: j.LastRunStatus,
			LastRunAt:     j.LastRunAt,
			Running:       running,
		})
	}
	return view
}

// RegisterResult is what Register returns — the one-time secrets, shown
// exactly once, same as cmd/backup-agent/register_cmd.go's printed output.
type RegisterResult struct {
	AgentUUID   string `json:"agentUuid"`
	AgentAPIKey string `json:"agentApiKey"`
	Bucket      string `json:"bucket"`
	AccessKey   string `json:"accessKey"`
	SecretKey   string `json:"secretKey"`
	SyncEngine  string `json:"syncEngine"`
}

// Register exchanges a one-time platform token for this desktop install's
// bootstrap config — the same internal/backup/register.Register call
// cmd/backup-agent/register_cmd.go makes, with desktop-mode paths applied
// before saving (see applyDesktopDefaults) and the embedded daemon started
// immediately afterward so the user doesn't need to restart the app.
func (a *App) Register(token, endpoint string) (RegisterResult, error) {
	path, err := desktopConfigPath()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("resolving config path: %w", err)
	}
	cfg, err := backupconfig.Load(path)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("loading base config: %w", err)
	}

	resp, _, err := register.Register(context.Background(), endpoint, token, backupconfig.AgentVersion)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("registration failed: %w", err)
	}

	cfg.NATS.ConnectionType = vmconfig.ConnectionTypeWebSocket
	cfg.NATS.URL = fmt.Sprintf("nats://%s:%d", resp.NATS.Host, resp.NATS.Port)
	cfg.NATS.WebSocketURL = fmt.Sprintf("wss://%s:%d", resp.NATS.Host, resp.NATS.Port)
	cfg.NATS.AgentUUID = resp.AgentUUID
	cfg.NATS.APIKey = resp.AgentAPIKey
	cfg.Bucket.Name = resp.Bucket.Name
	cfg.Bucket.Endpoint = resp.Bucket.Endpoint
	cfg.AccessKey.AccessKey = resp.AccessKey.AccessKey
	cfg.AccessKey.SecretKey = resp.AccessKey.SecretKey
	if resp.SyncEngine != "" {
		cfg.Agent.SyncEngine = resp.SyncEngine
	}

	if err := applyDesktopDefaults(cfg); err != nil {
		return RegisterResult{}, fmt.Errorf("resolving desktop-mode paths: %w", err)
	}
	if err := backupconfig.Save(path, cfg); err != nil {
		return RegisterResult{}, fmt.Errorf("saving config: %w", err)
	}

	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.startDaemon()

	return RegisterResult{
		AgentUUID:   resp.AgentUUID,
		AgentAPIKey: resp.AgentAPIKey,
		Bucket:      resp.Bucket.Name,
		AccessKey:   resp.AccessKey.AccessKey,
		SecretKey:   resp.AccessKey.SecretKey,
		SyncEngine:  cfg.Agent.SyncEngine,
	}, nil
}

// PauseJob/ResumeJob call state.Store.SetPaused directly — the exact same
// Store instance the running scheduler reads, so the change takes effect
// immediately, with no IPC round-trip.
func (a *App) PauseJob(uuid string) error {
	return a.setPaused(uuid, true)
}

func (a *App) ResumeJob(uuid string) error {
	return a.setPaused(uuid, false)
}

func (a *App) setPaused(uuid string, paused bool) error {
	a.mu.Lock()
	store := a.handles.Store
	a.mu.Unlock()
	if store == nil {
		return fmt.Errorf("daemon is not running")
	}
	return store.SetPaused(uuid, paused)
}

// RunJobNow triggers job immediately, the same runner.RunJob call the
// platform's run_job_now command uses — run in the background since a
// backup can take a while; the frontend picks up the outcome on its next
// GetStatus poll (LastRunStatus/LastRunAt) rather than blocking on this call.
func (a *App) RunJobNow(uuid string) error {
	a.mu.Lock()
	handles := a.handles
	logger := a.logger
	a.mu.Unlock()
	if handles.Store == nil {
		return fmt.Errorf("daemon is not running")
	}

	job, ok := handles.Store.GetJob(uuid)
	if !ok {
		return fmt.Errorf("unknown job %q", uuid)
	}

	go func() {
		if _, err := handles.Runner.RunJob(context.Background(), job.Job, protocol.NewUUID()); err != nil && logger != nil {
			logger.Warn("run-now failed", zap.String("job_uuid", uuid), zap.Error(err))
		}
	}()
	return nil
}

// CancelJob cancels a job's in-flight execution, if any — same
// registry.Cancel the platform's cancel_job command uses.
func (a *App) CancelJob(uuid string) error {
	a.mu.Lock()
	handles := a.handles
	a.mu.Unlock()
	if handles.Registry == nil {
		return fmt.Errorf("daemon is not running")
	}
	if !handles.Registry.Cancel(uuid) {
		return fmt.Errorf("no running execution for job %q", uuid)
	}
	return nil
}

// SetAutoStart enables or disables starting this app at login, via the
// per-OS mechanism implemented in autostart_{linux,darwin,windows}.go.
func (a *App) SetAutoStart(enabled bool) error {
	return setAutoStart(enabled)
}

// GetAutoStart reports whether the per-OS login-item is currently installed.
func (a *App) GetAutoStart() (bool, error) {
	return isAutoStartEnabled()
}
