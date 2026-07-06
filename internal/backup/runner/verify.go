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
