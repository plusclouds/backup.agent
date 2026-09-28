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
// Package registry is the single source of truth for "is this job running
// right now" — used by the scheduler (to avoid double-firing), run_job_now
// (to reject a second concurrent run), and cancel_job (to find what to cancel).
package registry

import (
	"context"
	"sync"
)

type entry struct {
	runUUID string
	cancel  context.CancelFunc
}

// Registry tracks in-flight job executions, keyed by job UUID. Safe for
// concurrent use.
type Registry struct {
	mu      sync.Mutex
	running map[string]*entry
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{running: make(map[string]*entry)}
}

// TryStart attempts to mark jobUUID as running with runUUID. On success it
// returns a context derived from parent (cancellable independently via
// Cancel or the returned done func) and ok=true. On failure (jobUUID is
// already running) it returns ok=false and the caller must not proceed.
//
// The caller must call the returned done func exactly once when the run
// finishes (typically via defer) — this both releases the context and
// removes the "running" entry so future TryStart/Cancel calls behave correctly.
func (r *Registry) TryStart(parent context.Context, jobUUID, runUUID string) (ctx context.Context, done func(), ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.running[jobUUID]; exists {
		return nil, nil, false
	}

	ctx, cancel := context.WithCancel(parent)
	r.running[jobUUID] = &entry{runUUID: runUUID, cancel: cancel}

	return ctx, func() {
		cancel()
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.running, jobUUID)
	}, true
}

// Cancel cancels the currently running execution of jobUUID, if any, and
// reports whether one was found. It does not remove the registry entry —
// that happens when the run's own `done` func (from TryStart) runs.
func (r *Registry) Cancel(jobUUID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.running[jobUUID]
	if !ok {
		return false
	}
	e.cancel()
	return true
}

// IsRunning reports whether jobUUID currently has an in-flight execution and,
// if so, its run UUID.
func (r *Registry) IsRunning(jobUUID string) (runUUID string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.running[jobUUID]
	if !ok {
		return "", false
	}
	return e.runUUID, true
}

// RunningCount returns how many jobs currently have an in-flight execution —
// reported as tasks_queued in the heartbeat payload.
func (r *Registry) RunningCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.running)
}
