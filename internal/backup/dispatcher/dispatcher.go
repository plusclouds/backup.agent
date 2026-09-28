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
// Package dispatcher routes inbound backup.agent command envelopes
// (full_sync, run_job_now, pause_job, resume_job, cancel_job,
// verify_snapshot, revoke) to the appropriate handler and returns a result
// envelope. Unlike the VM agent's dispatcher, there is no allowed_operations
// gate — every command in this fixed, spec'd set is always enabled.
package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/backup/engine"
	"github.com/plusclouds/ubuntu-agent/internal/backup/registry"
	"github.com/plusclouds/ubuntu-agent/internal/backup/runner"
	"github.com/plusclouds/ubuntu-agent/internal/backup/scheduler"
	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
)

const agentType = "backup"

// revokeGracePeriod is how long the dispatcher waits after handing back a
// `revoke` result before invoking Shutdown, so the result envelope has time
// to actually reach the wire before the process exits (see
// docs/backup.agent/registration.md: "the agent has a brief window to react
// before it's rejected on reconnect").
const revokeGracePeriod = 1 * time.Second

// Shutdown is called (after revokeGracePeriod) when a revoke command is
// received. Typically wired to cancel the agent's root context.
type Shutdown func(reason string)

// rejectedError marks a run() failure as a rejection (bad reference,
// precondition not met, unknown operation) rather than an execution failure.
// code is an optional machine-readable reason (see protocol.ResultPayload.Code)
// for cases the platform can react to programmatically — "" otherwise.
type rejectedError struct {
	msg  string
	code string
}

func (e *rejectedError) Error() string { return e.msg }

func rejected(format string, args ...any) error {
	return &rejectedError{msg: fmt.Sprintf(format, args...)}
}

// rejectedCode is rejected with an added machine-readable code — e.g.
// "UNKNOWN_JOB" when a command references a job the agent doesn't have
// locally (a missed or stale full_sync), so the platform can detect this
// specific case and react (e.g. re-sending full_sync) without pattern-
// matching the free-text message.
func rejectedCode(code, format string, args ...any) error {
	return &rejectedError{msg: fmt.Sprintf(format, args...), code: code}
}

// Dispatcher routes command envelopes to job/state/scheduler operations.
type Dispatcher struct {
	store     *state.Store
	scheduler *scheduler.Scheduler
	runner    *runner.Runner
	registry  *registry.Registry
	agentUUID string
	workDir   string
	shutdown  Shutdown
	logger    *zap.Logger
}

// New creates a Dispatcher. workDir is scratch space for verify_snapshot restores.
func New(
	store *state.Store,
	sched *scheduler.Scheduler,
	run *runner.Runner,
	reg *registry.Registry,
	agentUUID, workDir string,
	shutdown Shutdown,
	logger *zap.Logger,
) *Dispatcher {
	return &Dispatcher{
		store:     store,
		scheduler: sched,
		runner:    run,
		registry:  reg,
		agentUUID: agentUUID,
		workDir:   workDir,
		shutdown:  shutdown,
		logger:    logger,
	}
}

// Dispatch handles a single command envelope and returns the result envelope.
// It always returns a valid result — errors become failed/rejected statuses.
func (d *Dispatcher) Dispatch(ctx context.Context, env protocol.Envelope) protocol.Envelope {
	start := time.Now()

	var cmd protocol.CommandPayload
	if err := env.DecodePayload(&cmd); err != nil {
		return d.reject(env, "could not decode command payload: "+err.Error(), "")
	}
	op := cmd.Operation

	d.logger.Info("→ command received", zap.String("command_id", env.ID), zap.String("operation", op))

	if cmd.TimeoutS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cmd.TimeoutS)*time.Second)
		defer cancel()
	}

	output, err := d.run(ctx, op, cmd.Params)
	elapsed := time.Since(start)

	if err != nil {
		var re *rejectedError
		if errors.As(err, &re) {
			d.logResult(env.ID, op, protocol.StatusRejected, re.msg, elapsed)
			return d.reject(env, re.msg, re.code)
		}
		d.logResult(env.ID, op, protocol.StatusFailed, err.Error(), elapsed)
		return d.fail(env, err.Error())
	}

	d.logResult(env.ID, op, protocol.StatusCompleted, "", elapsed)
	return d.ok(env, output)
}

func (d *Dispatcher) logResult(commandID, op, status, msg string, elapsed time.Duration) {
	fields := []zap.Field{
		zap.String("command_id", commandID),
		zap.String("operation", op),
		zap.String("status", status),
		zap.Duration("elapsed", elapsed),
	}
	if msg != "" {
		fields = append(fields, zap.String("message", msg))
	}
	switch status {
	case protocol.StatusCompleted:
		d.logger.Info("← result: completed", fields...)
	case protocol.StatusFailed:
		d.logger.Error("← result: failed", fields...)
	case protocol.StatusRejected:
		d.logger.Warn("← result: rejected", fields...)
	}
}

// run executes op and returns its output, or a *rejectedError for
// precondition/reference failures vs. a plain error for execution failures.
func (d *Dispatcher) run(ctx context.Context, op string, raw json.RawMessage) (any, error) {
	switch op {
	case "full_sync":
		return d.fullSync(raw)
	case "run_job_now":
		return d.runJobNow(ctx, raw)
	case "pause_job":
		return d.setPaused(raw, true)
	case "resume_job":
		return d.setPaused(raw, false)
	case "cancel_job":
		return d.cancelJob(raw)
	case "verify_snapshot":
		return d.verifySnapshot(ctx, raw)
	case "revoke":
		return d.revoke(raw)
	default:
		return nil, rejected("unknown operation %q", op)
	}
}

func (d *Dispatcher) fullSync(raw json.RawMessage) (any, error) {
	var p struct {
		Jobs []state.Job `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, rejected("could not decode full_sync params: %s", err)
	}
	if err := d.store.ApplyFullSync(p.Jobs); err != nil {
		return nil, fmt.Errorf("applying full_sync: %w", err)
	}
	if err := d.scheduler.Reload(d.store.Jobs()); err != nil {
		// Invalid schedules are logged and the affected jobs simply never
		// fire — not a reason to fail the whole full_sync.
		d.logger.Warn("full_sync: some jobs have invalid schedules", zap.Error(err))
	}
	return map[string]any{"jobs": len(p.Jobs)}, nil
}

func (d *Dispatcher) runJobNow(ctx context.Context, raw json.RawMessage) (any, error) {
	var p struct {
		JobUUID string `json:"job_uuid"`
		RunUUID string `json:"run_uuid"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, rejected("could not decode run_job_now params: %s", err)
	}
	js, ok := d.store.GetJob(p.JobUUID)
	if !ok {
		return nil, rejectedCode("UNKNOWN_JOB", "unknown job %q", p.JobUUID)
	}

	outcome, err := d.runner.RunJob(ctx, js.Job, p.RunUUID)
	if err != nil {
		if errors.Is(err, runner.ErrAlreadyRunning) {
			return nil, rejected("job %q is already running", p.JobUUID)
		}
		return nil, err
	}

	// The command itself completed (the agent did attempt and report the
	// run) even if the backup run's own outcome was "failed" — the failure
	// detail travels in the output, same as the VM agent's service-action
	// results (see docs/api.md's start/stop 200-with-success:false pattern).
	return map[string]any{
		"run_uuid":          outcome.RunUUID,
		"status":            outcome.Status,
		"bytes_uploaded":    outcome.BytesUploaded,
		"bytes_deduped":     outcome.BytesDeduped,
		"kopia_snapshot_id": outcome.SnapshotID,
		"error":             outcome.Error,
	}, nil
}

func (d *Dispatcher) setPaused(raw json.RawMessage, paused bool) (any, error) {
	var p struct {
		JobUUID string `json:"job_uuid"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, rejected("could not decode params: %s", err)
	}
	if _, ok := d.store.GetJob(p.JobUUID); !ok {
		return nil, rejectedCode("UNKNOWN_JOB", "unknown job %q", p.JobUUID)
	}
	if err := d.store.SetPaused(p.JobUUID, paused); err != nil {
		return nil, rejected("%s", err.Error())
	}
	return map[string]any{"job_uuid": p.JobUUID, "paused": paused}, nil
}

func (d *Dispatcher) cancelJob(raw json.RawMessage) (any, error) {
	var p struct {
		JobUUID string `json:"job_uuid"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, rejected("could not decode params: %s", err)
	}
	if !d.registry.Cancel(p.JobUUID) {
		return nil, rejected("no running execution for job %q", p.JobUUID)
	}
	return map[string]any{"job_uuid": p.JobUUID, "cancelled": true}, nil
}

func (d *Dispatcher) verifySnapshot(ctx context.Context, raw json.RawMessage) (any, error) {
	var p struct {
		JobUUID    string `json:"job_uuid"`
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, rejected("could not decode params: %s", err)
	}
	job, ok := d.store.GetJob(p.JobUUID)
	if !ok {
		return nil, rejectedCode("UNKNOWN_JOB", "unknown job %q", p.JobUUID)
	}

	result, err := d.runner.VerifySnapshot(ctx, job.BucketName, p.SnapshotID, job.SourcePaths, d.workDir)
	if err != nil {
		if errors.Is(err, engine.ErrUnknownBucket) {
			return nil, rejected("%s", err.Error())
		}
		return nil, fmt.Errorf("verify_snapshot: %w", err)
	}
	return map[string]any{
		"ok":             result.OK,
		"bytes_restored": result.BytesRestored,
		"detail":         result.Detail,
	}, nil
}

func (d *Dispatcher) revoke(raw json.RawMessage) (any, error) {
	var p struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(raw, &p) // reason is informational only; absence isn't an error

	if d.shutdown != nil {
		go func() {
			time.Sleep(revokeGracePeriod)
			d.shutdown(p.Reason)
		}()
	}
	return map[string]any{"acknowledged": true}, nil
}

// --- result helpers -----------------------------------------------------------

func (d *Dispatcher) ok(src protocol.Envelope, output any) protocol.Envelope {
	env, err := protocol.ResultForType(src, d.agentUUID, agentType, protocol.StatusCompleted, "", "", output)
	if err != nil {
		d.logger.Error("failed to build result envelope", zap.Error(err))
	}
	return env
}

func (d *Dispatcher) fail(src protocol.Envelope, msg string) protocol.Envelope {
	env, err := protocol.ResultForType(src, d.agentUUID, agentType, protocol.StatusFailed, msg, "", nil)
	if err != nil {
		d.logger.Error("failed to build result envelope", zap.Error(err))
	}
	return env
}

func (d *Dispatcher) reject(src protocol.Envelope, msg, code string) protocol.Envelope {
	env, err := protocol.ResultForType(src, d.agentUUID, agentType, protocol.StatusRejected, msg, code, nil)
	if err != nil {
		d.logger.Error("failed to build result envelope", zap.Error(err))
	}
	return env
}
