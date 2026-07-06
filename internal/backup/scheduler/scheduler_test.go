package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/backup/engine/fake"
	"github.com/plusclouds/ubuntu-agent/internal/backup/registry"
	"github.com/plusclouds/ubuntu-agent/internal/backup/runner"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	"github.com/plusclouds/ubuntu-agent/internal/executor"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
)

// nonEmptySourceDir returns a temp directory containing one file, so
// runner's "source_paths must be present and non-empty" guard doesn't reject
// the run before the fake engine is ever invoked.
func nonEmptySourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}
	return dir
}

// recordingPublisher captures every job_run payload published, for assertions.
type recordingPublisher struct {
	mu       sync.Mutex
	payloads []protocol.JobRunPayload
}

func (p *recordingPublisher) PublishJobRun(_ context.Context, payload protocol.JobRunPayload) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.payloads = append(p.payloads, payload)
	return nil
}

func (p *recordingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.payloads)
}

func newTestScheduler(t *testing.T) (*Scheduler, *fake.Engine, *state.Store, *recordingPublisher) {
	t.Helper()
	eng := fake.New()
	reg := registry.New()
	st := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	run := runner.New(eng, reg, st, executor.NewWithTimeout(zap.NewNop(), time.Hour), zap.NewNop())
	pub := &recordingPublisher{}
	return New(st, run, pub, zap.NewNop()), eng, st, pub
}

func TestSchedulerFiresAndPublishesJobRun(t *testing.T) {
	sched, _, st, pub := newTestScheduler(t)
	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "every-second", JobType: state.JobTypeFiles, SourcePaths: []string{nonEmptySourceDir(t)}, Schedule: "@every 1s"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	if err := sched.Reload(st.Jobs()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	sched.Start()
	defer sched.Stop()

	deadline := time.After(3 * time.Second)
	for pub.count() == 0 {
		select {
		case <-deadline:
			t.Fatalf("expected at least one job_run to be published within 3s")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestSchedulerSkipsPausedJob(t *testing.T) {
	sched, _, st, pub := newTestScheduler(t)
	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "every-second", JobType: state.JobTypeFiles, SourcePaths: []string{nonEmptySourceDir(t)}, Schedule: "@every 1s"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	if err := st.SetPaused("job-1", true); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}
	if err := sched.Reload(st.Jobs()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	sched.Start()
	defer sched.Stop()

	time.Sleep(1500 * time.Millisecond)
	if pub.count() != 0 {
		t.Errorf("expected a paused job to never fire, got %d job_run events", pub.count())
	}
}

func TestSchedulerDoesNotDoubleFireASlowJob(t *testing.T) {
	sched, eng, st, _ := newTestScheduler(t)
	eng.SnapshotDelay = 2 * time.Second // outlives the 1s cron tick

	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "every-second", JobType: state.JobTypeFiles, SourcePaths: []string{nonEmptySourceDir(t)}, Schedule: "@every 1s"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	if err := sched.Reload(st.Jobs()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	sched.Start()
	defer sched.Stop()

	time.Sleep(2200 * time.Millisecond) // ~2 cron ticks while the first run is still "in flight"
	if len(eng.SnapshotCalls) > 1 {
		t.Errorf("expected the registry to prevent overlapping runs of the same job, got %d Snapshot calls", len(eng.SnapshotCalls))
	}
}

func TestReloadRemovesEntriesForDroppedJobs(t *testing.T) {
	sched, _, st, pub := newTestScheduler(t)
	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "every-second", JobType: state.JobTypeFiles, SourcePaths: []string{nonEmptySourceDir(t)}, Schedule: "@every 1s"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	if err := sched.Reload(st.Jobs()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	// Immediately drop job-1 via a second full_sync before it ever fires.
	if err := st.ApplyFullSync(nil); err != nil {
		t.Fatalf("ApplyFullSync (empty): %v", err)
	}
	if err := sched.Reload(st.Jobs()); err != nil {
		t.Fatalf("Reload (empty): %v", err)
	}
	sched.Start()
	defer sched.Stop()

	time.Sleep(1200 * time.Millisecond)
	if pub.count() != 0 {
		t.Errorf("expected a dropped job's cron entry to be removed by Reload, got %d job_run events", pub.count())
	}
}
