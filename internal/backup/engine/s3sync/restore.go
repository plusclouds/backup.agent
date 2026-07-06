package s3sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/minio/minio-go/v7"

	backupengine "github.com/plusclouds/ubuntu-agent/internal/backup/engine"
)

// Restore downloads the current mirror state for each of sourcePaths into
// destPath (recreating each "{basename}/{relative path}" key under destPath,
// exactly as it's laid out in the bucket). snapshotID is ignored: this
// engine has no snapshot identity — a live mirror only has "whatever's in
// the bucket right now" — see engine.Engine's doc comment on why sourcePaths
// exists for this engine's benefit where Kopia doesn't need it.
func (e *Engine) Restore(ctx context.Context, bucketName, _ string, sourcePaths []string, destPath string) error {
	bc, err := e.resolve(bucketName)
	if err != nil {
		return err
	}

	for _, sourcePath := range sourcePaths {
		prefix := filepath.ToSlash(filepath.Base(sourcePath)) + "/"
		remote, err := listRemote(ctx, bc.client, bc.cfg.BucketName, prefix)
		if err != nil {
			return fmt.Errorf("listing objects for %q: %w", sourcePath, err)
		}

		for _, obj := range remote {
			if err := downloadObject(ctx, bc.client, bc.cfg.BucketName, obj.Key, filepath.Join(destPath, filepath.FromSlash(obj.Key))); err != nil {
				return fmt.Errorf("downloading %q: %w", obj.Key, err)
			}
		}
	}
	return nil
}

func downloadObject(ctx context.Context, client *minio.Client, bucket, key, destFile string) error {
	obj, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer obj.Close() //nolint:errcheck

	if err := os.MkdirAll(filepath.Dir(destFile), 0o750); err != nil {
		return fmt.Errorf("creating directory for %q: %w", destFile, err)
	}
	f, err := os.Create(destFile) //nolint:gosec — destFile is derived from our own bucket listing under a job's configured destination
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	if _, err := io.Copy(f, obj); err != nil {
		return fmt.Errorf("writing %q: %w", destFile, err)
	}
	return nil
}

// Verify re-runs the same diff Snapshot would, without performing any
// writes: OK reports whether the bucket already matches the local tree
// exactly for every source path. workDir is unused — unlike Kopia's
// download-and-checksum approach, there's nothing to restore-and-compare;
// the diff against the live bucket listing already is the comparison, and
// is both cheaper and more meaningful for a live mirror with no snapshot
// history. BytesRestored is repurposed to mean "bytes that would need
// re-uploading if synced now" (0 when already in sync).
func (e *Engine) Verify(ctx context.Context, bucketName, _ string, sourcePaths []string, _ string) (backupengine.VerifyResult, error) {
	bc, err := e.resolve(bucketName)
	if err != nil {
		return backupengine.VerifyResult{}, err
	}

	var pendingBytes int64
	var toPutCount, toDeleteCount int
	for _, sourcePath := range sourcePaths {
		local, err := walkLocal(sourcePath)
		if err != nil {
			return backupengine.VerifyResult{}, fmt.Errorf("reading source path %q: %w", sourcePath, err)
		}

		prefix := filepath.ToSlash(filepath.Base(sourcePath)) + "/"
		remote, err := listRemote(ctx, bc.client, bc.cfg.BucketName, prefix)
		if err != nil {
			return backupengine.VerifyResult{}, fmt.Errorf("listing objects for %q: %w", sourcePath, err)
		}

		diff := computeDiff(local, remote)
		toPutCount += len(diff.ToPut)
		toDeleteCount += len(diff.ToDelete)
		for _, lf := range diff.ToPut {
			pendingBytes += lf.Size
		}
	}

	ok := toPutCount == 0 && toDeleteCount == 0
	detail := fmt.Sprintf("%d file(s) out of date, %d stale object(s) in bucket", toPutCount, toDeleteCount)
	return backupengine.VerifyResult{OK: ok, BytesRestored: pendingBytes, Detail: detail}, nil
}
