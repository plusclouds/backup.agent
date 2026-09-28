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
package kopia

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/fs/localfs"
	"github.com/kopia/kopia/fs/virtualfs"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/upload"

	backupengine "github.com/plusclouds/ubuntu-agent/internal/backup/engine"
)

// Snapshot uploads spec.SourcePaths as a single new Kopia snapshot.
//
// Kopia's bandwidth throttle (throttling.Limits) lives on the blob storage
// connection, not on an individual upload call, and different jobs can
// specify different bandwidth_limit_mbps — so each Snapshot call opens its
// own short-lived repository connection (a throwaway local config file,
// removed afterward) with that job's throttle applied, rather than reusing
// the shared connection EnsureRepository keeps open for Restore/Verify.
func (e *Engine) Snapshot(ctx context.Context, spec backupengine.SnapshotSpec) (backupengine.SnapshotResult, error) {
	bc, err := e.resolve(spec.BucketName)
	if err != nil {
		return backupengine.SnapshotResult{}, err
	}
	cfg := bc.cfg

	if len(spec.SourcePaths) == 0 {
		return backupengine.SnapshotResult{}, fmt.Errorf("no source paths to snapshot")
	}

	st, err := openStorage(ctx, cfg, spec.BandwidthLimitMbps)
	if err != nil {
		return backupengine.SnapshotResult{}, fmt.Errorf("connecting to storage: %w", err)
	}
	defer st.Close(ctx) //nolint:errcheck

	scopedConfigPath := filepath.Join(cfg.CacheDir, fmt.Sprintf("job-%s-%d.kopia-config", spec.JobUUID, time.Now().UnixNano()))
	defer os.Remove(scopedConfigPath) //nolint:errcheck

	if err := repo.Connect(ctx, scopedConfigPath, st, cfg.Password, connectOptions(cfg.CacheDir)); err != nil {
		return backupengine.SnapshotResult{}, fmt.Errorf("connecting to repository for job %s: %w", spec.JobUUID, err)
	}
	defer repo.Disconnect(ctx, scopedConfigPath) //nolint:errcheck

	rep, err := repo.Open(ctx, scopedConfigPath, cfg.Password, nil)
	if err != nil {
		return backupengine.SnapshotResult{}, fmt.Errorf("opening repository for job %s: %w", spec.JobUUID, err)
	}
	defer rep.Close(ctx) //nolint:errcheck

	var entries []fs.Entry
	for _, p := range spec.SourcePaths {
		entry, err := localfs.NewEntry(p)
		if err != nil {
			return backupengine.SnapshotResult{}, fmt.Errorf("reading source path %q: %w", p, err)
		}
		entries = append(entries, entry)
	}
	sourceInfo := snapshot.SourceInfo{
		Host:     kopiaHostname,
		UserName: kopiaUsername,
		Path:     "/backup-agent/jobs/" + spec.JobUUID,
	}
	root := virtualfs.NewStaticDirectory(filepath.Base(sourceInfo.Path), entries)
	pol := buildPolicy(spec)
	policyTree := policy.BuildTree(nil, pol)

	var uploadedBytes int64
	var manifest *snapshot.Manifest

	err = repo.WriteSession(ctx, rep, repo.WriteSessionOptions{
		Purpose: "backup-agent snapshot " + spec.JobUUID,
		OnUpload: func(n int64) {
			atomic.AddInt64(&uploadedBytes, n)
		},
	}, func(ctx context.Context, w repo.RepositoryWriter) error {
		if err := policy.SetPolicy(ctx, w, sourceInfo, pol); err != nil {
			return fmt.Errorf("setting retention policy: %w", err)
		}

		previous, err := snapshot.FindPreviousManifests(ctx, w, sourceInfo, nil)
		if err != nil {
			return fmt.Errorf("finding previous manifests: %w", err)
		}

		u := upload.NewUploader(w)
		man, err := u.Upload(ctx, root, policyTree, sourceInfo, previous...)
		if err != nil {
			return fmt.Errorf("uploading snapshot: %w", err)
		}

		if _, err := snapshot.SaveSnapshot(ctx, w, man); err != nil {
			return fmt.Errorf("saving snapshot manifest: %w", err)
		}
		manifest = man

		if _, err := policy.ApplyRetentionPolicy(ctx, w, sourceInfo, true); err != nil {
			return fmt.Errorf("applying retention policy: %w", err)
		}
		return nil
	})
	if err != nil {
		return backupengine.SnapshotResult{}, err
	}

	uploaded := atomic.LoadInt64(&uploadedBytes)
	deduped := manifest.Stats.TotalFileSize - uploaded
	if deduped < 0 {
		deduped = 0
	}

	return backupengine.SnapshotResult{
		SnapshotID:    string(manifest.ID),
		BytesUploaded: uploaded,
		BytesDeduped:  deduped,
	}, nil
}

// buildPolicy translates a job's retention fields into a Kopia Policy.
//
// KeepLastN maps directly to RetentionPolicy.KeepLatest. KeepForDays has no
// exact Kopia equivalent — Kopia's retention is snapshot-count-based
// (KeepDaily = "keep the N most recent daily snapshots"), not an absolute
// age cutoff — so it's mapped to KeepDaily as the closest native
// approximation. This is accurate for jobs scheduled at most once/day (the
// documented use case) but is an approximation, not an exact age-based
// expiry, for jobs that run more frequently.
func buildPolicy(spec backupengine.SnapshotSpec) *policy.Policy {
	pol := &policy.Policy{}
	if spec.KeepLastN > 0 {
		n := policy.OptionalInt(spec.KeepLastN)
		pol.RetentionPolicy.KeepLatest = &n
	}
	if spec.KeepForDays != nil && *spec.KeepForDays > 0 {
		n := policy.OptionalInt(*spec.KeepForDays)
		pol.RetentionPolicy.KeepDaily = &n
	}
	return pol
}
