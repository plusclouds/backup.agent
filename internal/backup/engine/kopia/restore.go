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
	"math"
	"os"

	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/restore"
	"github.com/kopia/kopia/snapshot/snapshotfs"

	backupengine "github.com/plusclouds/ubuntu-agent/internal/backup/engine"
)

// fullDepthRestoreOptions requests a full, deep restore of every nested file
// rather than Kopia's zero-value default. restore.Options{}'s
// RestoreDirEntryAtDepth defaults to 0, which only fully restores entries at
// the snapshot root and turns everything nested deeper into an empty
// ".kopia-entry" placeholder instead of real content (confirmed against
// Kopia's own CLI, which explicitly sets math.MaxInt32 for this reason — see
// cli/command_restore.go's unlimitedDepth). Restore and Verify below both
// need every file's actual bytes, not placeholders.
func fullDepthRestoreOptions() restore.Options {
	return restore.Options{RestoreDirEntryAtDepth: math.MaxInt32}
}

// Restore fetches snapshotID's contents from bucketName (or the default
// bucket, if empty) to destPath. sourcePaths is unused: Kopia's snapshotID
// already fully identifies the source root(s) via its manifest — the
// parameter exists for engines with no snapshot identity (see
// engine.Engine's doc comment).
func (e *Engine) Restore(ctx context.Context, bucketName, snapshotID string, _ []string, destPath string) error {
	bc, err := e.resolve(bucketName)
	if err != nil {
		return err
	}
	rep := bc.rep

	man, err := snapshot.LoadSnapshot(ctx, rep, manifest.ID(snapshotID))
	if err != nil {
		return fmt.Errorf("loading snapshot %s: %w", snapshotID, err)
	}
	entry, err := snapshotfs.SnapshotRoot(rep, man)
	if err != nil {
		return fmt.Errorf("resolving snapshot %s root: %w", snapshotID, err)
	}

	if err := os.MkdirAll(destPath, 0o750); err != nil {
		return fmt.Errorf("creating destination %s: %w", destPath, err)
	}

	output := &restore.FilesystemOutput{
		TargetPath:           destPath,
		OverwriteDirectories: true,
		OverwriteFiles:       true,
		OverwriteSymlinks:    true,
	}
	// Init assigns FilesystemOutput's internal stream copier — required
	// before use per its own doc comment; skipping it leaves the copier nil
	// and panics the first time a real file's content is written (only
	// masked here by the fullDepthRestoreOptions fix above, since a shallow
	// restore never reaches this code path).
	if err := output.Init(ctx); err != nil {
		return fmt.Errorf("initializing restore output: %w", err)
	}
	if _, err := restore.Entry(ctx, rep, output, entry, fullDepthRestoreOptions()); err != nil {
		return fmt.Errorf("restoring snapshot %s: %w", snapshotID, err)
	}
	return nil
}

// Verify restores snapshotID to workDir and reports whether it succeeded
// cleanly. A restore that completes without error already checksum-verifies
// the snapshot's content: Kopia's content-addressable storage derives every
// content ID from a hash of its (decrypted, decompressed) bytes, so content
// that didn't match its recorded hash would fail to decode during restore —
// there's no separate manual hashing pass to layer on top of that.
func (e *Engine) Verify(ctx context.Context, bucketName, snapshotID string, _ []string, workDir string) (backupengine.VerifyResult, error) {
	bc, err := e.resolve(bucketName)
	if err != nil {
		return backupengine.VerifyResult{}, err
	}
	rep := bc.rep

	man, err := snapshot.LoadSnapshot(ctx, rep, manifest.ID(snapshotID))
	if err != nil {
		return backupengine.VerifyResult{}, fmt.Errorf("loading snapshot %s: %w", snapshotID, err)
	}
	entry, err := snapshotfs.SnapshotRoot(rep, man)
	if err != nil {
		return backupengine.VerifyResult{}, fmt.Errorf("resolving snapshot %s root: %w", snapshotID, err)
	}

	if err := os.MkdirAll(workDir, 0o750); err != nil {
		return backupengine.VerifyResult{}, fmt.Errorf("creating verify work directory %s: %w", workDir, err)
	}

	output := &restore.FilesystemOutput{
		TargetPath:           workDir,
		OverwriteDirectories: true,
		OverwriteFiles:       true,
		OverwriteSymlinks:    true,
	}
	if err := output.Init(ctx); err != nil {
		return backupengine.VerifyResult{}, fmt.Errorf("initializing restore output: %w", err)
	}

	stats, err := restore.Entry(ctx, rep, output, entry, fullDepthRestoreOptions())
	if err != nil {
		return backupengine.VerifyResult{OK: false, Detail: err.Error()}, nil
	}

	ok := stats.IgnoredErrorCount == 0
	detail := fmt.Sprintf(
		"restored %d files (%d bytes), %d dirs, %d symlinks, %d ignored errors",
		stats.RestoredFileCount, stats.RestoredTotalFileSize, stats.RestoredDirCount,
		stats.RestoredSymlinkCount, stats.IgnoredErrorCount,
	)
	return backupengine.VerifyResult{OK: ok, BytesRestored: stats.RestoredTotalFileSize, Detail: detail}, nil
}
