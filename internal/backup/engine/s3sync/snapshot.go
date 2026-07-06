package s3sync

import (
	"context"
	"crypto/md5" //nolint:gosec — used as a cheap content-identity check against S3's own MD5-based ETag, not for security
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"

	backupengine "github.com/plusclouds/ubuntu-agent/internal/backup/engine"
)

// objectKey returns the S3 key a local path maps to: {basename(sourceRoot)}/
// {relative path}, always forward-slash-separated regardless of OS (this
// engine isn't targeting Windows source paths the way the rest of this repo
// does — see the "one v1 limitation" notes in this package for why that's an
// acceptable simplification for now).
func objectKey(sourceRoot, path string) (string, error) {
	rel, err := filepath.Rel(sourceRoot, path)
	if err != nil {
		return "", fmt.Errorf("computing relative path for %q under %q: %w", path, sourceRoot, err)
	}
	return filepath.ToSlash(filepath.Join(filepath.Base(sourceRoot), rel)), nil
}

// walkLocal walks sourceRoot and returns one localFile per regular file.
// Symlinks are skipped entirely (not followed, not uploaded as-is) — S3 has
// no symlink concept the way Kopia's snapshot format does, and silently
// uploading a symlink's target under its own name would misrepresent what's
// actually on disk; this is a deliberate, documented v1 limitation, matching
// this package's other "known gaps, not solved now" tradeoffs. Empty
// directories are likewise skipped: S3 objects have no directory entries, so
// an empty local directory has no representation in the bucket at all.
func walkLocal(sourceRoot string) ([]localFile, error) {
	var files []localFile
	err := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		key, err := objectKey(sourceRoot, path)
		if err != nil {
			return err
		}
		sum, err := md5File(path)
		if err != nil {
			return fmt.Errorf("hashing %q: %w", path, err)
		}
		files = append(files, localFile{Key: key, Path: path, Size: info.Size(), MD5: sum})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func md5File(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec — path comes from our own filepath.Walk over a job's configured source_paths
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck

	h := md5.New() //nolint:gosec
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// listRemote lists every object under prefix in bucket.
func listRemote(ctx context.Context, client *minio.Client, bucket, prefix string) ([]remoteObject, error) {
	var objs []remoteObject
	for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("listing objects under %q: %w", prefix, obj.Err)
		}
		objs = append(objs, remoteObject{Key: obj.Key, Size: obj.Size, ETag: obj.ETag})
	}
	return objs, nil
}

// Snapshot mirrors each of spec.SourcePaths into the resolved bucket under
// its own {basename}/ prefix (see objectKey), PUTting new/changed files and
// DELETing extras no longer present locally.
//
// spec.KeepLastN, spec.KeepForDays, and spec.BandwidthLimitMbps are no-ops
// here: retention has no meaning for a live mirror with no history, and
// bandwidth throttling isn't implemented in this v1 (unlike engine/kopia,
// which applies it via Kopia's own throttling.Limits) — logged at debug so
// it's visible without failing the run.
func (e *Engine) Snapshot(ctx context.Context, spec backupengine.SnapshotSpec) (backupengine.SnapshotResult, error) {
	bc, err := e.resolve(spec.BucketName)
	if err != nil {
		return backupengine.SnapshotResult{}, err
	}

	if len(spec.SourcePaths) == 0 {
		return backupengine.SnapshotResult{}, fmt.Errorf("no source paths to snapshot")
	}

	if spec.KeepLastN > 0 || spec.KeepForDays != nil || spec.BandwidthLimitMbps > 0 {
		e.logger.Debug("s3sync: retention/bandwidth options are no-ops for this engine (live mirror, no history)")
	}

	var uploaded, skipped int64
	for _, sourcePath := range spec.SourcePaths {
		local, err := walkLocal(sourcePath)
		if err != nil {
			return backupengine.SnapshotResult{}, fmt.Errorf("reading source path %q: %w", sourcePath, err)
		}

		prefix := filepath.ToSlash(filepath.Base(sourcePath)) + "/"
		remote, err := listRemote(ctx, bc.client, bc.cfg.BucketName, prefix)
		if err != nil {
			return backupengine.SnapshotResult{}, fmt.Errorf("listing existing objects for %q: %w", sourcePath, err)
		}

		diff := computeDiff(local, remote)
		skipped += diff.Skipped

		for _, lf := range diff.ToPut {
			f, err := os.Open(lf.Path) //nolint:gosec — path came from our own walk over a configured source_path
			if err != nil {
				return backupengine.SnapshotResult{}, fmt.Errorf("opening %q: %w", lf.Path, err)
			}
			_, err = bc.client.PutObject(ctx, bc.cfg.BucketName, lf.Key, f, lf.Size, minio.PutObjectOptions{})
			f.Close() //nolint:errcheck
			if err != nil {
				return backupengine.SnapshotResult{}, fmt.Errorf("uploading %q: %w", lf.Key, err)
			}
			uploaded += lf.Size
		}

		for _, key := range diff.ToDelete {
			if err := bc.client.RemoveObject(ctx, bc.cfg.BucketName, key, minio.RemoveObjectOptions{}); err != nil {
				return backupengine.SnapshotResult{}, fmt.Errorf("removing stale object %q: %w", key, err)
			}
		}
	}

	return backupengine.SnapshotResult{
		SnapshotID:    time.Now().UTC().Format(time.RFC3339),
		BytesUploaded: uploaded,
		BytesDeduped:  skipped,
	}, nil
}
