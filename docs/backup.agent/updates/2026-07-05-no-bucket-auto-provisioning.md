# Update: backup.agent never auto-provisions buckets

**Date:** 2026-07-05
**Backend:** `nextdeveloper/s3`
**Affects:** panel (frontend) — agent (Go binary) is unaffected by this one

## What changed and why

Both agent registration and WORM job creation used to silently create a new
bucket behind the scenes — `BackupAgentsService::register()` provisioned a
dedicated bucket for every agent, and `BackupJobsService` provisioned a
separate dedicated WORM bucket for any job with `object_lock_enabled: true`.
That's been removed. **backup.agent never creates a bucket on the customer's
behalf, full stop.**

Why: bucket creation is a billable, visible action tied to a specific S3
account and server placement — customers should make that choice explicitly
through the normal Buckets flow, not have it happen implicitly as a side
effect of registering a machine or ticking a checkbox on a job form. This
also removes a chunk of infrastructure-placement logic
(`resolveOrCreateS3Account()`, `pickServerForNewBucket()`) that was a
placeholder policy anyway (see the removed code's own comment: "just the
first non-deleted server").

## What changed, concretely

- `POST /s3/backup-agents` (registration token issuance) now **requires**
  `s3_bucket_id` — an existing bucket the customer already owns. No bucket,
  no token.
- `BackupJobsService::resolveBucketForJob()` no longer provisions a WORM
  bucket for `object_lock_enabled: true` jobs. Instead, it validates that the
  resolved bucket (explicit `s3_bucket_id`, or the agent's default if not
  given) **already has** `s3_buckets.object_lock_enabled = true`, and rejects
  the job with a clear error if not.
- Jobs without `object_lock_enabled` are unaffected — they still default to
  the agent's own bucket, which is now customer-chosen rather than
  system-created, but the resolution logic from the agent's perspective is
  identical (`bucket_name` still shows up the same way in `full_sync`).

Full details: `../registration.md` (§1), `../database.md`
(`s3_backup_agents.s3_bucket_id`, `s3_backup_jobs.object_lock_enabled`).

---

## For the panel team

**Registration form (`backup-agent-ui-specification.md` §4.2) now needs a
bucket picker.** This wasn't there before — the form used to have "nothing to
fill in" besides the reveal-modal step. Now `s3_bucket_id` is required.
Fetch options from `GET /s3/buckets-perspective`. If the customer has zero
buckets, don't let them hit a 403 — show an empty state pointing at bucket
creation first.

**WORM job creation (`backup-agent-ui-specification.md` §5.2) flips from
"toggle creates a bucket" to "toggle requires an existing one."** When
`object_lock_enabled` is turned on in the job create form, force the
`s3_bucket_id` select to appear, filtered to buckets that already have
Object Lock enabled. Submitting with a non-WORM bucket (or no bucket, if the
agent's default isn't a WORM bucket) now gets rejected server-side with a
message naming the problem directly — but the UI should prevent that path
being reachable at all, not just surface the resulting error.

Both spec updates are already applied to `backup-agent-ui-specification.md`
directly (§4.2 and §5.2) — this note is the "what changed and why," that
file has the concrete field-by-field spec.

## For the agent (Go binary) team

No action needed for this specific change. The `full_sync` payload shape is
unchanged (`bucket_name`, `object_lock_enabled` per job, as of the previous
update note) — only *how* the backend decides which bucket to put in that
field changed, not the field itself. The open item from the previous update
note (credentials for a job routed to a non-default bucket) is still open
and untouched by this change — see
`2026-07-04-per-job-destination-bucket.md` point 3 if you're implementing
multi-bucket support.
