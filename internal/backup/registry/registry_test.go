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
