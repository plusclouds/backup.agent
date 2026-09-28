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
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// fileFormat is the on-disk shape of the state file.
type fileFormat struct {
	Jobs []JobState `json:"jobs"`
}

// Store holds the current job list (with bookkeeping) in memory and persists
// it to a JSON file on every mutation. All methods are safe for concurrent use.
type Store struct {
	mu    sync.RWMutex
	path  string
	jobs  map[string]JobState
	order []string // job UUIDs in stable (last full_sync) order, for deterministic listing
}

// NewStore creates a Store backed by the file at path. Call Load before use.
func NewStore(path string) *Store {
	return &Store{
		path: path,
		jobs: make(map[string]JobState),
	}
}

// Load reads the state file if it exists. A missing file is not an error —
// the store simply starts empty (e.g. first run right after registration,
// before the platform's first full_sync arrives).
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading state file %s: %w", s.path, err)
	}

	var ff fileFormat
	if err := json.Unmarshal(data, &ff); err != nil {
		return fmt.Errorf("parsing state file %s: %w", s.path, err)
	}

	s.jobs = make(map[string]JobState, len(ff.Jobs))
	s.order = make([]string, 0, len(ff.Jobs))
	for _, js := range ff.Jobs {
		s.jobs[js.UUID] = js
		s.order = append(s.order, js.UUID)
	}
	return nil
}

// saveLocked writes the current state to disk atomically (write to a temp
// file in the same directory, then rename). Caller must hold s.mu.
func (s *Store) saveLocked() error {
	ff := fileFormat{Jobs: make([]JobState, 0, len(s.order))}
	for _, uuid := range s.order {
		ff.Jobs = append(ff.Jobs, s.jobs[uuid])
	}

	data, err := json.MarshalIndent(ff, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling state: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating state directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".backup-agent-state-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp state file: %w", err)
	}
	tmpPath := tmp.Name()
	// If anything below fails before the rename, remove the leftover temp file.
	defer os.Remove(tmpPath) //nolint:errcheck — best-effort cleanup

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck
		return fmt.Errorf("writing temp state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp state file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o640); err != nil {
		return fmt.Errorf("chmod temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("renaming temp state file to %s: %w", s.path, err)
	}
	return nil
}

// Save persists the current in-memory state to disk.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

// ApplyFullSync replaces the desired job list with newJobs, merging by UUID:
// jobs present in both the old and new lists keep their bookkeeping (Paused,
// LastRunAt, etc); brand-new UUIDs start with fresh bookkeeping; UUIDs no
// longer present are dropped entirely, bookkeeping included. Order follows
// newJobs. Persists to disk before returning.
func (s *Store) ApplyFullSync(newJobs []Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	merged := make(map[string]JobState, len(newJobs))
	order := make([]string, 0, len(newJobs))
	for _, job := range newJobs {
		existing, ok := s.jobs[job.UUID]
		if !ok {
			existing = JobState{}
		}
		existing.Job = job
		merged[job.UUID] = existing
		order = append(order, job.UUID)
	}
	s.jobs = merged
	s.order = order
	return s.saveLocked()
}

// Jobs returns a snapshot copy of every job, in full_sync order.
func (s *Store) Jobs() []JobState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]JobState, 0, len(s.order))
	for _, uuid := range s.order {
		out = append(out, s.jobs[uuid])
	}
	return out
}

// GetJob returns the current state of a single job by UUID.
func (s *Store) GetJob(uuid string) (JobState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	js, ok := s.jobs[uuid]
	return js, ok
}

// IsPaused reports whether the given job is currently paused. Unknown job
// UUIDs report false (nothing to pause).
func (s *Store) IsPaused(uuid string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs[uuid].Paused
}

// SetPaused updates a job's paused flag and persists the change. Returns an
// error if the job UUID is unknown.
func (s *Store) SetPaused(uuid string, paused bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	js, ok := s.jobs[uuid]
	if !ok {
		return fmt.Errorf("unknown job %q", uuid)
	}
	js.Paused = paused
	s.jobs[uuid] = js
	return s.saveLocked()
}

// RecordRun updates a job's last-run bookkeeping and persists the change.
// snapshotID may be empty (e.g. a failed run that never reached Kopia).
// Unknown job UUIDs are logged-and-ignored by the caller's perspective —
// RecordRun returns nil so a completed run for a job removed mid-flight by a
// concurrent full_sync doesn't turn into a spurious error.
func (s *Store) RecordRun(uuid string, runAt time.Time, status, snapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	js, ok := s.jobs[uuid]
	if !ok {
		return nil
	}
	js.LastRunAt = runAt.UTC().Format(time.RFC3339)
	js.LastRunStatus = status
	if snapshotID != "" {
		js.LastKopiaSnapshotID = snapshotID
	}
	s.jobs[uuid] = js
	return s.saveLocked()
}
