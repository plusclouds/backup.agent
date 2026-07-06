package registry

import (
	"context"
	"testing"
)

func TestTryStartRejectsConcurrentRun(t *testing.T) {
	r := New()

	ctx1, done1, ok1 := r.TryStart(context.Background(), "job-1", "run-1")
	if !ok1 {
		t.Fatalf("expected first TryStart to succeed")
	}
	defer done1()

	if ctx1 == nil {
		t.Fatalf("expected a non-nil context from TryStart")
	}

	_, _, ok2 := r.TryStart(context.Background(), "job-1", "run-2")
	if ok2 {
		t.Errorf("expected second concurrent TryStart for the same job to fail")
	}

	if runUUID, running := r.IsRunning("job-1"); !running || runUUID != "run-1" {
		t.Errorf("expected job-1 to be running with run-1, got running=%v uuid=%q", running, runUUID)
	}
	if r.RunningCount() != 1 {
		t.Errorf("expected RunningCount()=1, got %d", r.RunningCount())
	}
}

func TestDoneAllowsARestart(t *testing.T) {
	r := New()

	_, done, ok := r.TryStart(context.Background(), "job-1", "run-1")
	if !ok {
		t.Fatalf("expected TryStart to succeed")
	}
	done()

	if _, running := r.IsRunning("job-1"); running {
		t.Errorf("expected job-1 to no longer be running after done()")
	}

	_, _, ok = r.TryStart(context.Background(), "job-1", "run-2")
	if !ok {
		t.Errorf("expected TryStart to succeed again after the previous run finished")
	}
}

func TestCancelCancelsContextAndReportsFound(t *testing.T) {
	r := New()

	ctx, done, ok := r.TryStart(context.Background(), "job-1", "run-1")
	if !ok {
		t.Fatalf("expected TryStart to succeed")
	}
	defer done()

	if !r.Cancel("job-1") {
		t.Errorf("expected Cancel to find a running job-1")
	}
	select {
	case <-ctx.Done():
	default:
		t.Errorf("expected ctx to be cancelled after Cancel")
	}

	if r.Cancel("job-does-not-exist") {
		t.Errorf("expected Cancel on an unknown job to report false")
	}
}
