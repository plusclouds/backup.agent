package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestApplyFullSyncPreservesBookkeepingAndDropsRemoved(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(filepath.Join(dir, "state.json"))
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := st.ApplyFullSync([]Job{
		{UUID: "a", Name: "job-a", Schedule: "0 2 * * *"},
		{UUID: "b", Name: "job-b", Schedule: "0 3 * * *"},
	}); err != nil {
		t.Fatalf("ApplyFullSync (initial): %v", err)
	}

	if err := st.SetPaused("a", true); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}
	if err := st.RecordRun("a", time.Now(), "completed", "snap-1"); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	// Second full_sync: "a" keeps its name but changes schedule (bookkeeping
	// must survive), "b" is dropped (bookkeeping must go with it), "c" is new.
	if err := st.ApplyFullSync([]Job{
		{UUID: "a", Name: "job-a-renamed", Schedule: "0 4 * * *"},
		{UUID: "c", Name: "job-c", Schedule: "0 5 * * *"},
	}); err != nil {
		t.Fatalf("ApplyFullSync (second): %v", err)
	}

	a, ok := st.GetJob("a")
	if !ok {
		t.Fatalf("expected job a to still exist")
	}
	if !a.Paused {
		t.Errorf("expected job a to still be paused after full_sync")
	}
	if a.LastRunStatus != "completed" || a.LastKopiaSnapshotID != "snap-1" {
		t.Errorf("expected job a's bookkeeping to survive full_sync, got %+v", a)
	}
	if a.Name != "job-a-renamed" || a.Schedule != "0 4 * * *" {
		t.Errorf("expected job a's Job fields to be updated, got %+v", a.Job)
	}

	if _, ok := st.GetJob("b"); ok {
		t.Errorf("expected job b to be removed after full_sync")
	}

	c, ok := st.GetJob("c")
	if !ok {
		t.Fatalf("expected new job c to exist")
	}
	if c.Paused {
		t.Errorf("expected brand-new job c to start unpaused")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	st := NewStore(path)
	if err := st.Load(); err != nil {
		t.Fatalf("Load (fresh): %v", err)
	}
	keepForDays := 7
	if err := st.ApplyFullSync([]Job{
		{UUID: "a", Name: "job-a", JobType: JobTypeScript, Schedule: "0 2 * * *", KeepForDays: &keepForDays},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load (reload): %v", err)
	}
	jobs := reloaded.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job after reload, got %d", len(jobs))
	}
	if jobs[0].KeepForDays == nil || *jobs[0].KeepForDays != 7 {
		t.Errorf("expected KeepForDays=7 to round-trip, got %+v", jobs[0].KeepForDays)
	}
}

func TestSetPausedUnknownJobErrors(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := st.SetPaused("nonexistent", true); err == nil {
		t.Errorf("expected error pausing an unknown job")
	}
}

func TestRecordRunUnknownJobIsANoOp(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := st.RecordRun("nonexistent", time.Now(), "completed", "snap"); err != nil {
		t.Errorf("expected RecordRun on unknown job to be a no-op, got error: %v", err)
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err := st.Load(); err != nil {
		t.Fatalf("expected missing file to be a no-op, got: %v", err)
	}
	if len(st.Jobs()) != 0 {
		t.Errorf("expected no jobs from a missing file")
	}
}
