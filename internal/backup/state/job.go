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
// Package state persists backup.agent's desired job list (received via the
// full_sync command) plus agent-owned runtime bookkeeping (pause flag, last
// run outcome) across restarts.
package state

// JobType is the job_type field of a Job — see docs/backup.agent/protocol.md.
type JobType string

// Job type constants.
const (
	JobTypeFiles  JobType = "files"
	JobTypeScript JobType = "script"
)

// Job is the desired-state job definition as sent by the platform in a
// full_sync command's payload.jobs entries.
type Job struct {
	UUID               string   `json:"uuid"`
	Name               string   `json:"name"`
	JobType            JobType  `json:"job_type"`
	SourcePaths        []string `json:"source_paths"`
	PreScript          string   `json:"pre_script,omitempty"`
	ScriptTimeoutS     int      `json:"script_timeout_s,omitempty"`
	Schedule           string   `json:"schedule"`
	KeepLastN          int      `json:"keep_last_n,omitempty"`
	KeepForDays        *int     `json:"keep_for_days,omitempty"`
	BandwidthLimitMbps int      `json:"bandwidth_limit_mbps,omitempty"`
	// BucketName is the job's destination bucket, added in
	// docs/backup.agent/updates/2026-07-04-per-job-destination-bucket.md.
	// Empty means "the agent's default bucket from registration" — the only
	// case the agent can actually service today, since a job routed to a
	// non-default (e.g. dedicated WORM) bucket needs credentials the
	// registration response doesn't hand out yet (see engine.ErrUnknownBucket).
	BucketName string `json:"bucket_name,omitempty"`
	// ObjectLockEnabled is informational only — the destination bucket
	// enforces WORM server-side, the agent doesn't change write behavior
	// based on this flag. Preserved here for round-tripping/visibility only.
	ObjectLockEnabled bool `json:"object_lock_enabled,omitempty"`
}

// JobState is a Job plus agent-owned runtime bookkeeping that survives
// restarts. These fields are never sent by the platform and are never reset
// by full_sync — ApplyFullSync preserves them across a job UUID's full_sync
// updates and only initializes them fresh for brand-new UUIDs.
type JobState struct {
	Job

	Paused              bool   `json:"paused"`
	LastRunAt           string `json:"last_run_at,omitempty"`
	LastRunStatus       string `json:"last_run_status,omitempty"`
	LastKopiaSnapshotID string `json:"last_kopia_snapshot_id,omitempty"`
}
