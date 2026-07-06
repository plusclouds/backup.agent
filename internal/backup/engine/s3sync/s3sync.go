// Package s3sync implements engine.Engine as a plain, native Go 1:1 file
// mirror to S3 — no deduplication, no client-side encryption — using
// github.com/minio/minio-go/v7 directly. This is the "rsync" engine
// mentioned in docs/backup.agent/overview.md: unlike engine/kopia, objects
// land in the bucket under human-readable keys mirroring the source tree
// exactly, at the cost of losing Kopia's chunked dedup, encryption, and
// incremental-snapshot history (see the package's own diff.go for the
// tradeoffs this accepts).
//
// This package is deliberately independent of engine/kopia — no shared
// internal package between them — so splitEndpoint below is a verbatim copy
// of kopia.go's helper of the same name, not an import; the two engines
// happen to need the same "bare host, TLS as a separate bool" transform
// because both Kopia's s3.Options and minio-go's Client have that shape, but
// there's no other code shared between them.
package s3sync

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"

	backupengine "github.com/plusclouds/ubuntu-agent/internal/backup/engine"
)

// bucketConn is one open S3 client, keyed by bucket name — mirrors
// engine/kopia's bucketConn shape for the same multi-bucket reason (see
// docs/backup.agent/updates/2026-07-04-per-job-destination-bucket.md).
type bucketConn struct {
	cfg    backupengine.RepoConfig
	client *minio.Client
}

// Engine implements backupengine.Engine as a live, unencrypted, undeduplicated
// 1:1 mirror of local files to S3 objects.
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

// splitEndpoint splits a bucket endpoint into the bare host[:port] minio-go's
// Client expects and whether TLS should be used — verbatim copy of
// engine/kopia/kopia.go's helper of the same name; see that file's doc
// comment for why this transform is needed (registration's bucket.endpoint
// comes back as a fully-qualified URL, but minio.New wants a bare host with
// TLS as a separate option).
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

// openClient builds a minio.Client for cfg's S3 bucket.
func openClient(cfg backupengine.RepoConfig) (*minio.Client, error) {
	host, useTLS, err := splitEndpoint(cfg.BucketEndpoint)
	if err != nil {
		return nil, err
	}
	return minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: useTLS,
	})
}

// EnsureRepository builds a client for cfg's bucket, confirms connectivity,
// and registers it under cfg.BucketName. Idempotent per bucket name; the
// first bucket ever registered becomes the default used when a job doesn't
// specify one. Unlike Kopia, there's no repository/manifest to initialize —
// a live mirror needs nothing beyond bucket access. cfg.Password and
// cfg.CacheDir are Kopia-only fields (repository encryption password, local
// content-cache directory); both are inert here — this engine keeps no local
// state and needs no repository encryption, since it stores files as-is.
func (e *Engine) EnsureRepository(ctx context.Context, cfg backupengine.RepoConfig) error {
	client, err := openClient(cfg)
	if err != nil {
		return fmt.Errorf("connecting to bucket %s: %w", cfg.BucketName, err)
	}

	if _, err := client.BucketExists(ctx, cfg.BucketName); err != nil {
		return fmt.Errorf("checking bucket %s: %w", cfg.BucketName, err)
	}

	e.mu.Lock()
	e.buckets[cfg.BucketName] = &bucketConn{cfg: cfg, client: client}
	if e.defaultBucket == "" {
		e.defaultBucket = cfg.BucketName
	}
	e.mu.Unlock()

	e.logger.Info("s3sync: bucket ready", zap.String("bucket", cfg.BucketName), zap.String("endpoint", cfg.BucketEndpoint))
	return nil
}

// Close is a no-op beyond clearing the bucket map: minio-go's Client holds
// only an *http.Client, no persistent connection to release.
func (e *Engine) Close(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.buckets = make(map[string]*bucketConn)
	return nil
}

// resolve applies the "empty means default bucket" rule and looks up the
// connection registered for that bucket — same contract as engine/kopia's
// resolve.
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
