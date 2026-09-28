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
package dispatcher

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/backup/engine"
	"github.com/plusclouds/ubuntu-agent/internal/backup/engine/fake"
	"github.com/plusclouds/ubuntu-agent/internal/backup/registry"
	"github.com/plusclouds/ubuntu-agent/internal/backup/runner"
	"github.com/plusclouds/ubuntu-agent/internal/backup/scheduler"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	"github.com/plusclouds/ubuntu-agent/internal/executor"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
)

type noopPublisher struct{}

func (noopPublisher) PublishJobRun(context.Context, protocol.JobRunPayload) error { return nil }

func newTestDispatcher(t *testing.T) (*Dispatcher, *fake.Engine, *state.Store, chan string) {
	t.Helper()
	eng := fake.New()
	reg := registry.New()
	st := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	run := runner.New(eng, reg, st, executor.NewWithTimeout(zap.NewNop(), time.Hour), zap.NewNop())
	sched := scheduler.New(st, run, noopPublisher{}, zap.NewNop())

	shutdownCh := make(chan string, 1)
	shutdown := func(reason string) { shutdownCh <- reason }

	disp := New(st, sched, run, reg, "agent-uuid", filepath.Join(t.TempDir(), "verify-work"), shutdown, zap.NewNop())
	return disp, eng, st, shutdownCh
}

func command(op string, params any) protocol.Envelope {
	raw, _ := json.Marshal(params)
	payload := protocol.CommandPayload{Operation: op, Params: raw}
	env, err := protocol.NewForAgentType("agent-uuid", "backup", protocol.TypeCommand, payload)
	if err != nil {
		panic(err)
	}
	return env
}

func decodeResult(t *testing.T, env protocol.Envelope) protocol.ResultPayload {
	t.Helper()
	var rp protocol.ResultPayload
	if err := env.DecodePayload(&rp); err != nil {
		t.Fatalf("decoding result payload: %v", err)
	}
	return rp
}

func backupRepoConfig(bucketName string) engine.RepoConfig {
	return engine.RepoConfig{BucketName: bucketName}
}

func nonEmptyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	return dir
}

func TestDispatchFullSync(t *testing.T) {
	disp, _, st, _ := newTestDispatcher(t)
	env := command("full_sync", map[string]any{
		"jobs": []map[string]any{
			{"uuid": "job-1", "name": "job-one", "job_type": "files", "source_paths": []string{nonEmptyDir(t)}, "schedule": "0 2 * * *"},
		},
	})

	result := disp.Dispatch(context.Background(), env)
	rp := decodeResult(t, result)
	if rp.Status != protocol.StatusCompleted {
		t.Fatalf("expected completed, got %q (msg=%q)", rp.Status, rp.Message)
	}
	if _, ok := st.GetJob("job-1"); !ok {
		t.Errorf("expected full_sync to add job-1 to the store")
	}
}

func TestDispatchRunJobNowEchoesRunUUID(t *testing.T) {
	disp, eng, st, _ := newTestDispatcher(t)
	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "job-one", JobType: state.JobTypeFiles, SourcePaths: []string{nonEmptyDir(t)}, Schedule: "0 2 * * *"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}

	env := command("run_job_now", map[string]any{"job_uuid": "job-1", "run_uuid": "run-abc"})
	result := disp.Dispatch(context.Background(), env)
	rp := decodeResult(t, result)
	if rp.Status != protocol.StatusCompleted {
		t.Fatalf("expected completed, got %q (msg=%q)", rp.Status, rp.Message)
	}

	out, ok := rp.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected output to be a map, got %T", rp.Output)
	}
	if out["run_uuid"] != "run-abc" {
		t.Errorf("expected run_uuid to be echoed back, got %v", out["run_uuid"])
	}
	if len(eng.SnapshotCalls) != 1 {
		t.Errorf("expected exactly 1 Snapshot call, got %d", len(eng.SnapshotCalls))
	}
}

func TestDispatchRunJobNowUnknownJobIsRejected(t *testing.T) {
	disp, _, _, _ := newTestDispatcher(t)
	env := command("run_job_now", map[string]any{"job_uuid": "does-not-exist", "run_uuid": "run-1"})
	result := disp.Dispatch(context.Background(), env)
	rp := decodeResult(t, result)
	if rp.Status != protocol.StatusRejected {
		t.Fatalf("expected rejected, got %q", rp.Status)
	}
	// UNKNOWN_JOB is machine-readable so the platform can detect a missed/stale
	// full_sync and react (e.g. re-sending it) without pattern-matching Message.
	if rp.Code != "UNKNOWN_JOB" {
		t.Errorf("expected code UNKNOWN_JOB, got %q", rp.Code)
	}
}

func TestDispatchResultPropagatesReplyTo(t *testing.T) {
	disp, _, _, _ := newTestDispatcher(t)
	env := command("run_job_now", map[string]any{"job_uuid": "does-not-exist", "run_uuid": "run-1"})
	env.ReplyTo = "_INBOX.abc123"

	result := disp.Dispatch(context.Background(), env)
	if result.ReplyTo != env.ReplyTo {
		t.Errorf("expected result to carry ReplyTo %q forward, got %q", env.ReplyTo, result.ReplyTo)
	}
}

func TestDispatchPauseResumeCancel(t *testing.T) {
	disp, eng, st, _ := newTestDispatcher(t)
	eng.SnapshotDelay = 300 * time.Millisecond
	if err := st.ApplyFullSync([]state.Job{
		{UUID: "job-1", Name: "job-one", JobType: state.JobTypeFiles, SourcePaths: []string{nonEmptyDir(t)}, Schedule: "0 2 * * *"},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}

	pauseResult := disp.Dispatch(context.Background(), command("pause_job", map[string]any{"job_uuid": "job-1"}))
	if decodeResult(t, pauseResult).Status != protocol.StatusCompleted {
		t.Fatalf("expected pause_job to complete")
	}
	if js, _ := st.GetJob("job-1"); !js.Paused {
		t.Errorf("expected job-1 to be paused")
	}

	resumeResult := disp.Dispatch(context.Background(), command("resume_job", map[string]any{"job_uuid": "job-1"}))
	if decodeResult(t, resumeResult).Status != protocol.StatusCompleted {
		t.Fatalf("expected resume_job to complete")
	}
	if js, _ := st.GetJob("job-1"); js.Paused {
		t.Errorf("expected job-1 to be resumed")
	}

	// cancel_job with nothing running should be rejected.
	cancelResult := disp.Dispatch(context.Background(), command("cancel_job", map[string]any{"job_uuid": "job-1"}))
	if decodeResult(t, cancelResult).Status != protocol.StatusRejected {
		t.Fatalf("expected cancel_job with no running execution to be rejected")
	}

	// Start a run in the background, then cancel it while in flight.
	done := make(chan protocol.Envelope, 1)
	go func() {
		env := command("run_job_now", map[string]any{"job_uuid": "job-1", "run_uuid": "run-1"})
		done <- disp.Dispatch(context.Background(), env)
	}()
	time.Sleep(30 * time.Millisecond)

	cancelResult = disp.Dispatch(context.Background(), command("cancel_job", map[string]any{"job_uuid": "job-1"}))
	if decodeResult(t, cancelResult).Status != protocol.StatusCompleted {
		t.Fatalf("expected cancel_job to succeed against an in-flight run")
	}
	<-done
}

func TestDispatchRunJobNowUnknownBucketFailsNotRejects(t *testing.T) {
	// A job pointing at a bucket the agent has no credentials for (see
	// docs/backup.agent/updates/2026-07-04-per-job-destination-bucket.md)
	// is a valid command that ran and failed, not an invalid command — same
	// "completed command, failed job" convention as any other Snapshot error.
	disp, eng, st, _ := newTestDispatcher(t)
	if err := eng.EnsureRepository(context.Background(), backupRepoConfig("default-bucket")); err != nil {
		t.Fatalf("EnsureRepository: %v", err)
	}
	if err := st.ApplyFullSync([]state.Job{
		{
			UUID: "job-1", Name: "worm-job", JobType: state.JobTypeFiles,
			SourcePaths: []string{nonEmptyDir(t)}, Schedule: "0 2 * * *",
			BucketName: "some-other-worm-bucket",
		},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}

	env := command("run_job_now", map[string]any{"job_uuid": "job-1", "run_uuid": "run-1"})
	result := disp.Dispatch(context.Background(), env)
	rp := decodeResult(t, result)
	if rp.Status != protocol.StatusCompleted {
		t.Fatalf("expected the command itself to complete, got %q", rp.Status)
	}
	out, ok := rp.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected output to be a map, got %T", rp.Output)
	}
	if out["status"] != "failed" {
		t.Errorf("expected the run outcome to be failed for an unknown bucket, got %v", out["status"])
	}
}

func TestDispatchVerifySnapshotUnknownBucketIsRejected(t *testing.T) {
	disp, eng, st, _ := newTestDispatcher(t)
	if err := eng.EnsureRepository(context.Background(), backupRepoConfig("default-bucket")); err != nil {
		t.Fatalf("EnsureRepository: %v", err)
	}
	if err := st.ApplyFullSync([]state.Job{
		{
			UUID: "job-1", Name: "worm-job", JobType: state.JobTypeFiles,
			SourcePaths: []string{nonEmptyDir(t)}, Schedule: "0 2 * * *",
			BucketName: "some-other-worm-bucket",
		},
	}); err != nil {
		t.Fatalf("ApplyFullSync: %v", err)
	}

	env := command("verify_snapshot", map[string]any{"job_uuid": "job-1", "snapshot_id": "snap-1"})
	result := disp.Dispatch(context.Background(), env)
	rp := decodeResult(t, result)
	if rp.Status != protocol.StatusRejected {
		t.Fatalf("expected verify_snapshot against an unknown bucket to be rejected, got %q (msg=%q)", rp.Status, rp.Message)
	}
}

func TestDispatchUnknownOperationIsRejected(t *testing.T) {
	disp, _, _, _ := newTestDispatcher(t)
	result := disp.Dispatch(context.Background(), command("does_not_exist", map[string]any{}))
	if decodeResult(t, result).Status != protocol.StatusRejected {
		t.Fatalf("expected unknown operation to be rejected")
	}
}

func TestDispatchRevokeTriggersShutdownAfterGracePeriod(t *testing.T) {
	disp, _, _, shutdownCh := newTestDispatcher(t)
	result := disp.Dispatch(context.Background(), command("revoke", map[string]any{"reason": "test"}))
	if decodeResult(t, result).Status != protocol.StatusCompleted {
		t.Fatalf("expected revoke to complete")
	}

	select {
	case reason := <-shutdownCh:
		if reason != "test" {
			t.Errorf("expected shutdown reason 'test', got %q", reason)
		}
	case <-time.After(revokeGracePeriod + time.Second):
		t.Fatalf("expected shutdown to be invoked after the grace period")
	}
}
