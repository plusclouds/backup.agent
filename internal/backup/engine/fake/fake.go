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
// Package fake provides an in-memory Engine implementation for tests, so
// dispatcher/scheduler/runner logic can be exercised without real Kopia or S3.
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/plusclouds/ubuntu-agent/internal/backup/engine"
)

// Engine is a fully in-memory, configurable fake of engine.Engine.
type Engine struct {
	mu sync.Mutex

	// Canned errors — nil means "succeed."
	EnsureRepositoryErr error
	SnapshotErr         error
	RestoreErr          error
	VerifyErr           error

	// SnapshotFunc, if set, is called instead of the default canned
	// behavior — use it for tests that need custom per-call results.
	SnapshotFunc func(ctx context.Context, spec engine.SnapshotSpec) (engine.SnapshotResult, error)

	// SnapshotDelay makes Snapshot block until ctx is cancelled or the delay
	// elapses, whichever comes first — used to test that cancel_job actually
	// stops an in-flight run rather than merely discarding its result.
	SnapshotDelay time.Duration

	// VerifyResult overrides the default canned Verify result if non-nil.
	VerifyResult *engine.VerifyResult

	// Call recordings, for assertions.
	EnsureRepositoryCalls []engine.RepoConfig
	SnapshotCalls         []engine.SnapshotSpec
	RestoreCalls          []RestoreCall
	VerifyCalls           []VerifyCall
	Closed                bool

	seen        map[string]int64
	snapshotSeq int

	// registeredBuckets/defaultBucket implement the same "unknown bucket"
	// guard the real Kopia engine enforces (see engine.ErrUnknownBucket and
	// docs/backup.agent/updates/2026-07-04-per-job-destination-bucket.md).
	// Left empty (the zero value), the fake is permissive — every call site
	// that predates per-job buckets and never calls EnsureRepository keeps
	// working unchanged. Once EnsureRepository has been called at least
	// once, bucket resolution is enforced like the real engine.
	registeredBuckets map[string]bool
	defaultBucket     string
}

// RestoreCall records one Restore invocation.
type RestoreCall struct {
	BucketName  string
	SnapshotID  string
	SourcePaths []string
	DestPath    string
}

// VerifyCall records one Verify invocation.
type VerifyCall struct {
	BucketName  string
	SnapshotID  string
	SourcePaths []string
	WorkDir     string
}

// New returns a ready-to-use fake Engine with default (successful) behavior.
func New() *Engine {
	return &Engine{seen: make(map[string]int64), registeredBuckets: make(map[string]bool)}
}

// EnsureRepository records the call, registers cfg.BucketName as known, and
// returns EnsureRepositoryErr. The first call's bucket becomes the default.
func (e *Engine) EnsureRepository(_ context.Context, cfg engine.RepoConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.EnsureRepositoryCalls = append(e.EnsureRepositoryCalls, cfg)
	if e.EnsureRepositoryErr != nil {
		return e.EnsureRepositoryErr
	}
	if e.defaultBucket == "" {
		e.defaultBucket = cfg.BucketName
	}
	e.registeredBuckets[cfg.BucketName] = true
	return nil
}

// resolveBucket applies the "empty means default" rule and, once at least
// one EnsureRepository call has happened, enforces that the resolved bucket
// is known — mirroring the real engine's ErrUnknownBucket behavior.
func (e *Engine) resolveBucket(bucketName string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	name := bucketName
	if name == "" {
		name = e.defaultBucket
	}
	if len(e.registeredBuckets) > 0 && !e.registeredBuckets[name] {
		return "", fmt.Errorf("bucket %q: %w", name, engine.ErrUnknownBucket)
	}
	return name, nil
}

// Snapshot records the call, optionally blocks for SnapshotDelay (respecting
// ctx cancellation), then returns either SnapshotFunc's result, SnapshotErr,
// or a canned result. The canned result simulates dedup: the second call for
// a given JobUUID reports the first call's byte count as BytesDeduped.
func (e *Engine) Snapshot(ctx context.Context, spec engine.SnapshotSpec) (engine.SnapshotResult, error) {
	if _, err := e.resolveBucket(spec.BucketName); err != nil {
		return engine.SnapshotResult{}, err
	}

	e.mu.Lock()
	e.SnapshotCalls = append(e.SnapshotCalls, spec)
	fn := e.SnapshotFunc
	delay := e.SnapshotDelay
	callErr := e.SnapshotErr
	e.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return engine.SnapshotResult{}, ctx.Err()
		case <-time.After(delay):
		}
	}

	if fn != nil {
		return fn(ctx, spec)
	}
	if callErr != nil {
		return engine.SnapshotResult{}, callErr
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.snapshotSeq++
	const uploadedBytes = 1024 * 1024
	deduped := e.seen[spec.JobUUID]
	e.seen[spec.JobUUID] = uploadedBytes
	return engine.SnapshotResult{
		SnapshotID:    fmt.Sprintf("fake-snap-%d", e.snapshotSeq),
		BytesUploaded: uploadedBytes,
		BytesDeduped:  deduped,
	}, nil
}

// Restore records the call and returns RestoreErr.
func (e *Engine) Restore(_ context.Context, bucketName, snapshotID string, sourcePaths []string, destPath string) error {
	resolved, err := e.resolveBucket(bucketName)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.RestoreCalls = append(e.RestoreCalls, RestoreCall{BucketName: resolved, SnapshotID: snapshotID, SourcePaths: sourcePaths, DestPath: destPath})
	return e.RestoreErr
}

// Verify records the call and returns VerifyResult (or a canned OK result)
// and VerifyErr.
func (e *Engine) Verify(_ context.Context, bucketName, snapshotID string, sourcePaths []string, workDir string) (engine.VerifyResult, error) {
	resolved, err := e.resolveBucket(bucketName)
	if err != nil {
		return engine.VerifyResult{}, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.VerifyCalls = append(e.VerifyCalls, VerifyCall{BucketName: resolved, SnapshotID: snapshotID, SourcePaths: sourcePaths, WorkDir: workDir})
	if e.VerifyErr != nil {
		return engine.VerifyResult{}, e.VerifyErr
	}
	if e.VerifyResult != nil {
		return *e.VerifyResult, nil
	}
	return engine.VerifyResult{OK: true, BytesRestored: 1024 * 1024, Detail: "checksum matched"}, nil
}

// Close records that it was called.
func (e *Engine) Close(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Closed = true
	return nil
}
