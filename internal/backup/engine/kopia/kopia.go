// Package kopia implements engine.Engine on top of github.com/kopia/kopia,
// embedded as a Go library per docs/backup.agent/overview.md's design
// decision — no external `kopia` binary is installed or shelled out to.
//
// Exact API signatures used here were confirmed via `go doc` against the
// vendored kopia@v0.23.1 before writing this file, per the implementation
// plan — nothing here is guessed from memory of Kopia's CLI.
package kopia

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/s3"
	"github.com/kopia/kopia/repo/content"

	backupengine "github.com/plusclouds/ubuntu-agent/internal/backup/engine"
)

// kopiaHostname/kopiaUsername identify this agent's snapshots within each
// repository (Kopia's SourceInfo is keyed by host+username+path). A fixed
// pair is fine across every bucket this agent connects to.
const (
	kopiaHostname = "backup-agent"
	kopiaUsername = "backup-agent"
)

// kopiaContentCacheSizeBytes/kopiaMetadataCacheSizeBytes size Kopia's local
// on-disk content/metadata caches. These must be non-zero: a zero
// ContentCacheSizeBytes tells Kopia to disable local caching, but
// repo.Connect's config serialization (LocalConfig.writeToFile) still
// unconditionally round-trips the (supposedly empty) CacheDirectory through
// filepath.Rel and writes back a bogus non-empty relative path — which then
// looks "enabled" again on the next repo.Open, except NewStorageOrNil still
// returns a nil cache storage for a zero size, so the resulting
// *PersistentCache is nil with no error, and the first content/manifest read
// that misses the cache dereferences that nil pointer (observed as a
// SIGSEGV deep inside Kopia's content cache during a job's first snapshot).
// Kopia's own CLI defaults to 5000MB for both; 500MB is plenty for this
// agent's per-job workloads while keeping the on-disk footprint modest on
// arbitrary customer machines.
const (
	kopiaContentCacheSizeBytes  = 500 << 20 // 500 MB
	kopiaMetadataCacheSizeBytes = 500 << 20 // 500 MB
)

// bucketConn is one open repository connection, keyed by bucket name. As of
// docs/backup.agent/updates/2026-07-04-per-job-destination-bucket.md, jobs
// can specify a destination bucket other than the agent's default one, so
// the Engine holds a connection per unique bucket it has credentials for —
// in practice just one (the default) until the platform starts delivering
// per-bucket credentials (see backupengine.ErrUnknownBucket).
type bucketConn struct {
	cfg        backupengine.RepoConfig
	rep        repo.Repository
	configPath string
}

// Engine implements backupengine.Engine using one or more embedded Kopia
// repository connections, one per destination bucket.
type Engine struct {
	logger *zap.Logger

	mu            sync.Mutex
	buckets       map[string]*bucketConn
	defaultBucket string
}

// New creates an Engine. Call EnsureRepository at least once (for the
// agent's default bucket) before any other method.
func New(logger *zap.Logger) *Engine {
	return &Engine{logger: logger, buckets: make(map[string]*bucketConn)}
}

// openStorage builds a Kopia blob.Storage backed by cfg's S3 bucket.
// bandwidthLimitMbps, when > 0, is applied as a symmetric upload/download
// throttle via Kopia's native throttling.Limits (embedded in s3.Options) —
// confirmed present on the vendored version, so no custom rate-limiting
// wrapper is needed.
func openStorage(ctx context.Context, cfg backupengine.RepoConfig, bandwidthLimitMbps int) (blob.Storage, error) {
	host, useTLS, err := splitEndpoint(cfg.BucketEndpoint)
	if err != nil {
		return nil, err
	}

	opts := &s3.Options{
		BucketName:      cfg.BucketName,
		Endpoint:        host,
		DoNotUseTLS:     !useTLS,
		AccessKeyID:     cfg.AccessKey,
		SecretAccessKey: cfg.SecretKey,
	}
	if bandwidthLimitMbps > 0 {
		bytesPerSecond := float64(bandwidthLimitMbps) * 1024 * 1024 / 8
		opts.Limits.UploadBytesPerSecond = bytesPerSecond
		opts.Limits.DownloadBytesPerSecond = bytesPerSecond
	}
	// isCreate is not about repository initialization (that's repo.Initialize
	// below) — it only affects certain provider-specific bucket-creation
	// semantics we don't need. The bucket itself is never provisioned by this
	// agent: per updates/2026-07-05-no-bucket-auto-provisioning.md, every
	// bucket a job (or the agent's default) points at is a customer-owned
	// bucket that already existed before registration/full_sync.
	return s3.New(ctx, opts, false)
}

// splitEndpoint splits a bucket endpoint into the bare host[:port] Kopia's
// s3.Options.Endpoint expects and whether TLS should be used. Confirmed
// necessary against the real platform: registration's bucket.endpoint comes
// back as a fully-qualified URL (e.g. "https://dc.s3.plusclouds.com"), but
// the underlying S3 client rejects a scheme in Endpoint ("Endpoint url
// cannot have fully qualified paths") — Secure/TLS is a separate option, not
// part of the same string. A bare host with no scheme is also accepted as-is
// and assumed to want TLS, per client-setup-guide.md's "always https://,
// never http://" convention.
func splitEndpoint(endpoint string) (host string, useTLS bool, err error) {
	if !strings.Contains(endpoint, "://") {
		return endpoint, true, nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("parsing bucket endpoint %q: %w", endpoint, err)
	}
	if u.Host == "" {
		return "", false, fmt.Errorf("bucket endpoint %q has no host", endpoint)
	}

	switch u.Scheme {
	case "https":
		useTLS = true
	case "http":
		useTLS = false
	default:
		return "", false, fmt.Errorf("unsupported scheme %q in bucket endpoint %q", u.Scheme, endpoint)
	}
	return u.Host, useTLS, nil
}

func connectOptions(cacheDir string) *repo.ConnectOptions {
	return &repo.ConnectOptions{
		ClientOptions: repo.ClientOptions{
			Hostname: kopiaHostname,
			Username: kopiaUsername,
		},
		CachingOptions: content.CachingOptions{
			CacheDirectory:         cacheDir,
			ContentCacheSizeBytes:  kopiaContentCacheSizeBytes,
			MetadataCacheSizeBytes: kopiaMetadataCacheSizeBytes,
		},
	}
}

// EnsureRepository opens the repository described by cfg if it already
// exists, or creates it otherwise (repo.Connect fails with
// ErrRepositoryNotInitialized on a bucket that has never been used as a
// Kopia repository — that's the exact, documented signal to fall back to
// repo.Initialize), and registers the connection under cfg.BucketName.
// Idempotent per bucket name; the first bucket ever registered becomes the
// default used when a job doesn't specify one.
func (e *Engine) EnsureRepository(ctx context.Context, cfg backupengine.RepoConfig) error {
	if err := os.MkdirAll(cfg.CacheDir, 0o750); err != nil {
		return fmt.Errorf("creating kopia cache directory %s: %w", cfg.CacheDir, err)
	}
	// Each bucket gets its own config file within the shared cache dir —
	// bucket names are already unique identifiers, safe to use directly.
	configPath := filepath.Join(cfg.CacheDir, "repository-"+cfg.BucketName+".config")

	st, err := openStorage(ctx, cfg, 0)
	if err != nil {
		return fmt.Errorf("connecting to bucket %s: %w", cfg.BucketName, err)
	}
	defer st.Close(ctx) //nolint:errcheck — transient handle, only needed for the connect handshake below

	opts := connectOptions(cfg.CacheDir)
	if err := repo.Connect(ctx, configPath, st, cfg.Password, opts); err != nil {
		if !errors.Is(err, repo.ErrRepositoryNotInitialized) {
			return fmt.Errorf("connecting to repository: %w", err)
		}

		e.logger.Info("kopia: bucket has no repository yet, initializing", zap.String("bucket", cfg.BucketName))
		if err := repo.Initialize(ctx, st, &repo.NewRepositoryOptions{}, cfg.Password); err != nil {
			return fmt.Errorf("initializing repository: %w", err)
		}
		if err := repo.Connect(ctx, configPath, st, cfg.Password, opts); err != nil {
			return fmt.Errorf("connecting to newly initialized repository: %w", err)
		}
	}

	rep, err := repo.Open(ctx, configPath, cfg.Password, nil)
	if err != nil {
		return fmt.Errorf("opening repository: %w", err)
	}

	e.mu.Lock()
	// Close any previously-open connection for this bucket (EnsureRepository
	// is meant to be called once per bucket, but stay correct if it's ever
	// retried after a failure).
	prev := e.buckets[cfg.BucketName]
	e.buckets[cfg.BucketName] = &bucketConn{cfg: cfg, rep: rep, configPath: configPath}
	if e.defaultBucket == "" {
		e.defaultBucket = cfg.BucketName
	}
	e.mu.Unlock()

	if prev != nil {
		prev.rep.Close(ctx) //nolint:errcheck
	}

	e.logger.Info("kopia: repository ready", zap.String("bucket", cfg.BucketName), zap.String("endpoint", cfg.BucketEndpoint))
	return nil
}

// Close releases every open repository connection.
func (e *Engine) Close(ctx context.Context) error {
	e.mu.Lock()
	buckets := e.buckets
	e.buckets = make(map[string]*bucketConn)
	e.mu.Unlock()

	var firstErr error
	for _, bc := range buckets {
		if err := bc.rep.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// resolve applies the "empty means default bucket" rule and looks up the
// connection registered for that bucket, returning an error wrapping
// backupengine.ErrUnknownBucket if none has been registered (e.g. a job
// pointing at a bucket the agent has no credentials for — see
// docs/backup.agent/updates/2026-07-04-per-job-destination-bucket.md).
func (e *Engine) resolve(bucketName string) (*bucketConn, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	name := bucketName
	if name == "" {
		name = e.defaultBucket
	}
	bc, ok := e.buckets[name]
	if !ok {
		return nil, fmt.Errorf("bucket %q: %w", name, backupengine.ErrUnknownBucket)
	}
	return bc, nil
}
