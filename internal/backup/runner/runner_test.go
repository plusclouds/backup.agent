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
package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/backup/engine/fake"
	"github.com/plusclouds/ubuntu-agent/internal/backup/registry"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	"github.com/plusclouds/ubuntu-agent/internal/executor"
)

func newTestRunner(t *testing.T) (*Runner, *fake.Engine, *state.Store) {
	t.Helper()
	eng := fake.New()
	reg := registry.New()
	st := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	exec := executor.NewWithTimeout(zap.NewNop(), time.Hour)
	return New(eng, reg, st, exec, zap.NewNop()), eng, st
}

func writeSourceFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}
	return path
}

func TestRunJobFilesSuccess(t *testing.T) {
	r, eng, st := newTestRunner(t)
	src := writeSourceFile(t, "hello world")

	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "files-job", JobType: state.JobTypeFiles, SourcePaths: []string{src}, Schedule: "@every 1h"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	job, _ := st.GetJob("job-1")

	outcome, err := r.RunJob(context.Background(), job.Job, "run-1")
	if err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	if outcome.Status != "completed" {
		t.Errorf("expected status completed, got %q (err=%v)", outcome.Status, outcome.Error)
	}
	if len(eng.SnapshotCalls) != 1 {
		t.Errorf("expected exactly 1 Snapshot call, got %d", len(eng.SnapshotCalls))
	}

	updated, _ := st.GetJob("job-1")
	if updated.LastRunStatus != "completed" {
		t.Errorf("expected state to record completed run, got %q", updated.LastRunStatus)
	}
}

func TestRunJobMissingSourcePathFailsBeforeSnapshot(t *testing.T) {
	r, eng, st := newTestRunner(t)

	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "files-job", JobType: state.JobTypeFiles, SourcePaths: []string{"/nonexistent/path/xyz"}, Schedule: "@every 1h"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	job, _ := st.GetJob("job-1")

	outcome, err := r.RunJob(context.Background(), job.Job, "run-1")
	if err != nil {
		t.Fatalf("RunJob returned an unexpected top-level error: %v", err)
	}
	if outcome.Status != "failed" {
		t.Errorf("expected status failed for a missing source path, got %q", outcome.Status)
	}
	if len(eng.SnapshotCalls) != 0 {
		t.Errorf("expected Snapshot to never be called when source_paths is missing, got %d calls", len(eng.SnapshotCalls))
	}
}

func TestRunJobScriptFailureNeverTouchesEngine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pre_script test uses a POSIX shebang")
	}
	r, eng, st := newTestRunner(t)

	if err := st.ApplyFullSync([]state.Job{
		{
			UUID: "job-1", Name: "script-job", JobType: state.JobTypeScript,
			PreScript: "#!/bin/sh\nexit 1\n", ScriptTimeoutS: 5,
			SourcePaths: []string{t.TempDir()}, Schedule: "@every 1h",
		},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	job, _ := st.GetJob("job-1")

	outcome, err := r.RunJob(context.Background(), job.Job, "run-1")
	if err != nil {
		t.Fatalf("RunJob returned an unexpected top-level error: %v", err)
	}
	if outcome.Status != "failed" || outcome.Error == nil {
		t.Errorf("expected a failed outcome with an error message, got %+v", outcome)
	}
	if len(eng.SnapshotCalls) != 0 {
		t.Errorf("expected Snapshot to never be called after a failing pre_script, got %d calls", len(eng.SnapshotCalls))
	}
}

func TestRunJobScriptSuccessProducesSnapshottableOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pre_script test uses a POSIX shebang")
	}
	r, eng, st := newTestRunner(t)

	outDir := t.TempDir()
	outFile := filepath.Join(outDir, "dump.sql")

	if err := st.ApplyFullSync([]state.Job{
		{
			UUID: "job-1", Name: "script-job", JobType: state.JobTypeScript,
			PreScript:      "#!/bin/sh\necho 'dump data' > " + outFile + "\n",
			ScriptTimeoutS: 5,
			SourcePaths:    []string{outFile},
			Schedule:       "@every 1h",
		},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	job, _ := st.GetJob("job-1")

	outcome, err := r.RunJob(context.Background(), job.Job, "run-1")
	if err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	if outcome.Status != "completed" {
		t.Errorf("expected status completed, got %q (err=%v)", outcome.Status, outcome.Error)
	}
	if len(eng.SnapshotCalls) != 1 {
		t.Errorf("expected exactly 1 Snapshot call, got %d", len(eng.SnapshotCalls))
	}
}

func TestRunJobAlreadyRunningIsRejected(t *testing.T) {
	r, eng, st := newTestRunner(t)
	eng.SnapshotDelay = 200 * time.Millisecond
	src := writeSourceFile(t, "hello world")

	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "files-job", JobType: state.JobTypeFiles, SourcePaths: []string{src}, Schedule: "@every 1h"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	job, _ := st.GetJob("job-1")

	done := make(chan struct{})
	go func() {
		_, _ = r.RunJob(context.Background(), job.Job, "run-1")
		close(done)
	}()
	time.Sleep(20 * time.Millisecond) // let the first run acquire the registry slot

	_, err := r.RunJob(context.Background(), job.Job, "run-2")
	if err != ErrAlreadyRunning {
		t.Errorf("expected ErrAlreadyRunning, got %v", err)
	}
	<-done
}

func TestRunJobCancellationStopsInFlightSnapshot(t *testing.T) {
	r, eng, st := newTestRunner(t)
	eng.SnapshotDelay = time.Hour // effectively "never", must be cut short by cancellation
	src := writeSourceFile(t, "hello world")

	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "files-job", JobType: state.JobTypeFiles, SourcePaths: []string{src}, Schedule: "@every 1h"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}
	job, _ := st.GetJob("job-1")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	outcome, err := r.RunJob(ctx, job.Job, "run-1")
	if err != nil {
		t.Fatalf("RunJob returned an unexpected top-level error: %v", err)
	}
	if outcome.Status != "failed" {
		t.Errorf("expected a cancelled run to be reported as failed, got %q", outcome.Status)
	}
}
