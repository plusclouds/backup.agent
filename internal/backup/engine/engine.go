// Package engine defines the seam between backup.agent's job/scheduling
// logic and the Kopia-backed backup engine. Everything upstream (dispatcher,
// scheduler, runner) is written and tested against this interface; only
// engine/kopia.go depends on the real Kopia library.
package engine

import (
	"context"
	"errors"
)

// ErrUnknownBucket is returned (wrapped) by Snapshot/Restore/Verify when a
// job's resolved bucket has no registered RepoConfig — i.e. no
// EnsureRepository call has ever succeeded for that bucket name. As of
// docs/backup.agent/updates/2026-07-04-per-job-destination-bucket.md, jobs
// can specify a non-default destination bucket, but the registration
// response only ever hands out credentials for the agent's default bucket —
// so any job pointing elsewhere hits this until the platform starts
// delivering per-bucket credentials. Callers should treat this as a
// configuration problem to surface clearly, not a transient failure to retry.
var ErrUnknownBucket = errors.New("no credentials registered for this bucket")

// RepoConfig describes one Kopia repository backend: an S3-compatible
// bucket, scoped access key, repository encryption password, and local
// metadata cache directory. An Engine may hold more than one RepoConfig at
// once — one per unique bucket a job references (see BucketName below).
type RepoConfig struct {
	BucketName     string
	BucketEndpoint string
	AccessKey      string
	SecretKey      string
	// Password is the Kopia repository's own encryption password. See
	// internal/backup/register's derivation — no dedicated field for this
	// exists in the platform's registration response today.
	Password string
	CacheDir string
}

// SnapshotSpec describes one backup run's inputs.
type SnapshotSpec struct {
	JobUUID     string
	SourcePaths []string
	KeepLastN   int
	KeepForDays *int

	BandwidthLimitMbps int

	// BucketName is the job's destination bucket (state.Job.BucketName).
	// Empty means "use the agent's default bucket" — the bucket registered
	// by the first EnsureRepository call.
	BucketName string
}

// SnapshotResult is what a successful Snapshot call reports back — these
// numbers flow directly into the job_run event's payload.
type SnapshotResult struct {
	SnapshotID    string
	BytesUploaded int64
	BytesDeduped  int64
}

// VerifyResult is what a Verify call reports back, for the verify_snapshot
// command's result payload.
type VerifyResult struct {
	OK            bool
	BytesRestored int64
	Detail        string
}

// Engine is the backup engine seam. A real implementation embeds Kopia as a
// Go library (engine/kopia.go); tests use engine/fake.
type Engine interface {
	// EnsureRepository opens the repository described by cfg if it already
	// exists, or creates it otherwise, and registers it under cfg.BucketName
	// for future Snapshot/Restore/Verify calls. Idempotent per bucket name.
	// The first bucket name ever registered becomes the default used when a
	// job doesn't specify BucketName/bucketName explicitly. Must be called
	// at least once (for the agent's default bucket) at startup, before any
	// job can run.
	EnsureRepository(ctx context.Context, cfg RepoConfig) error

	// Snapshot uploads spec.SourcePaths as a new snapshot to spec.BucketName
	// (or the default bucket, if empty), applying a retention policy
	// (KeepLastN / KeepForDays) and bandwidth limit. Returns an error
	// wrapping ErrUnknownBucket if spec.BucketName has no registered RepoConfig.
	Snapshot(ctx context.Context, spec SnapshotSpec) (SnapshotResult, error)

	// Restore fetches a snapshot's contents from bucketName (or the default
	// bucket, if empty) to destPath. Returns an error wrapping
	// ErrUnknownBucket if bucketName has no registered RepoConfig.
	//
	// sourcePaths is the job's source_paths (state.Job.SourcePaths). Kopia's
	// snapshotID already fully identifies the source root(s) via its
	// manifest, so its implementation ignores sourcePaths — it exists for
	// engines with no snapshot identity (e.g. engine/s3sync's live mirror,
	// which only has per-source-path bucket prefixes to restore from).
	Restore(ctx context.Context, bucketName, snapshotID string, sourcePaths []string, destPath string) error

	// Verify restores snapshotID from bucketName (or the default bucket, if
	// empty) to a temp directory under workDir, checksums the result, and
	// cleans up afterward. Long-running by design (protocol.md advises a
	// 1800s timeout for the originating command). Returns an error wrapping
	// ErrUnknownBucket if bucketName has no registered RepoConfig.
	//
	// sourcePaths — see Restore's doc comment; same reasoning applies here.
	Verify(ctx context.Context, bucketName, snapshotID string, sourcePaths []string, workDir string) (VerifyResult, error)

	// Close releases any resources held by the engine (e.g. every open
	// Kopia repository connection). Safe to call once during graceful shutdown.
	Close(ctx context.Context) error
}
