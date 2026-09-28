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
	"fmt"
	"os"

	"github.com/plusclouds/ubuntu-agent/internal/backup/engine"
)

// VerifySnapshot restores snapshotID from bucketName (or the default
// bucket, if empty — see state.Job.BucketName) to a fresh temp directory
// under workDir and checksum-verifies it via the engine, per the
// verify_snapshot command (docs/backup.agent/protocol.md advises a long,
// e.g. 1800s, timeout on ctx for this — the caller is responsible for
// bounding ctx accordingly). The temp directory is always cleaned up before
// returning.
func (r *Runner) VerifySnapshot(ctx context.Context, bucketName, snapshotID string, sourcePaths []string, workDir string) (engine.VerifyResult, error) {
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		return engine.VerifyResult{}, fmt.Errorf("creating verify work directory %s: %w", workDir, err)
	}

	dir, err := os.MkdirTemp(workDir, "verify-*")
	if err != nil {
		return engine.VerifyResult{}, fmt.Errorf("creating verify temp directory: %w", err)
	}
	defer os.RemoveAll(dir) //nolint:errcheck — best-effort cleanup

	return r.engine.Verify(ctx, bucketName, snapshotID, sourcePaths, dir)
}
