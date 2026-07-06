// Package scheduler owns the agent-side cron loop that fires backup jobs.
// Scheduling runs on the agent, not the platform, because a customer's
// machine may be offline or unreachable at any given moment — see
// docs/backup.agent/overview.md. run_job_now is the one platform-forced
// exception and bypasses this package entirely (the dispatcher calls the
// shared Runner directly).
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/backup/runner"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
)

// EventPublisher is the seam scheduler uses to emit job_run events, so it
// doesn't need to depend on the concrete NATS publisher (or its tests on
// real NATS).
type EventPublisher interface {
	PublishJobRun(ctx context.Context, payload protocol.JobRunPayload) error
}

// Scheduler evaluates each job's cron schedule and fires runs via a shared
// Runner. Paused jobs are skipped at fire time (their cron entry stays
// registered — see docs/backup.agent's registration.md discussion of
// keeping this simple: missed ticks while paused are not backfilled).
type Scheduler struct {
	mu      sync.Mutex
	cron    *cron.Cron
	store   *state.Store
	runner  *runner.Runner
	events  EventPublisher
	logger  *zap.Logger
	entries map[string]cron.EntryID
}

// New creates a Scheduler. Call Reload once with the initial job list (from
// the state file or the first full_sync), then Start.
func New(store *state.Store, run *runner.Runner, events EventPublisher, logger *zap.Logger) *Scheduler {
	c := cron.New(
		cron.WithLocation(time.UTC),
		cron.WithChain(cron.Recover(zapCronLogger{logger})),
	)
	return &Scheduler{
		cron:    c,
		store:   store,
		runner:  run,
		events:  events,
		logger:  logger,
		entries: make(map[string]cron.EntryID),
	}
}

// Start begins evaluating cron schedules. Non-blocking — cron runs its own
// goroutine internally.
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop halts the scheduler and waits for any in-progress cron dispatch (not
// the job runs themselves, which are tracked by the registry) to finish.
func (s *Scheduler) Stop() {
	<-s.cron.Stop().Done()
}

// Reload replaces every cron entry with fresh ones built from jobs — the
// full desired state after a full_sync (or the persisted state at startup).
// Invalid schedule expressions are logged and skipped rather than aborting
// the whole reload; a job with a bad schedule simply never fires until
// corrected by a future full_sync.
func (s *Scheduler) Reload(jobs []state.JobState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range s.entries {
		s.cron.Remove(id)
	}
	s.entries = make(map[string]cron.EntryID, len(jobs))

	var errs []error
	for _, js := range jobs {
		jobUUID := js.UUID
		id, err := s.cron.AddFunc(js.Schedule, func() { s.fire(jobUUID) })
		if err != nil {
			errs = append(errs, fmt.Errorf("job %s: invalid schedule %q: %w", jobUUID, js.Schedule, err))
			continue
		}
		s.entries[jobUUID] = id
	}
	return errors.Join(errs...)
}

// fire runs when a job's cron schedule ticks. It re-reads the job's current
// state (in case a full_sync or pause/resume changed it since Reload) and
// skips if paused, unknown, or already running.
func (s *Scheduler) fire(jobUUID string) {
	js, ok := s.store.GetJob(jobUUID)
	if !ok {
		return // removed by a concurrent full_sync since this tick was scheduled
	}
	if js.Paused {
		s.logger.Debug("scheduler: skipping paused job", zap.String("job_uuid", jobUUID))
		return
	}

	runUUID := protocol.NewUUID()
	ctx := context.Background()

	outcome, err := s.runner.RunJob(ctx, js.Job, runUUID)
	if err != nil {
		if errors.Is(err, runner.ErrAlreadyRunning) {
			s.logger.Debug("scheduler: job still running from a previous tick, skipping", zap.String("job_uuid", jobUUID))
		} else {
			s.logger.Error("scheduler: unexpected error starting run", zap.String("job_uuid", jobUUID), zap.Error(err))
		}
		return
	}

	payload := protocol.JobRunPayload{
		JobUUID:         jobUUID,
		RunUUID:         outcome.RunUUID,
		Status:          outcome.Status,
		StartedAt:       outcome.StartedAt.Format(time.RFC3339),
		FinishedAt:      outcome.FinishedAt.Format(time.RFC3339),
		BytesUploaded:   outcome.BytesUploaded,
		BytesDeduped:    outcome.BytesDeduped,
		KopiaSnapshotID: outcome.SnapshotID,
		Error:           outcome.Error,
	}
	if err := s.events.PublishJobRun(ctx, payload); err != nil {
		s.logger.Error("scheduler: could not publish job_run", zap.String("job_uuid", jobUUID), zap.Error(err))
	}
}

// zapCronLogger adapts *zap.Logger to cron.Logger, used only for cron's own
// internal recover-from-panic diagnostics.
type zapCronLogger struct{ logger *zap.Logger }

func (l zapCronLogger) Info(msg string, keysAndValues ...any) {
	l.logger.Sugar().Debugw("cron: "+msg, keysAndValues...)
}

func (l zapCronLogger) Error(err error, msg string, keysAndValues ...any) {
	l.logger.Sugar().Errorw("cron: "+msg, append(keysAndValues, "error", err)...)
}
