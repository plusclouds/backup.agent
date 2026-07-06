# Update: per-job destination bucket

**Date:** 2026-07-04
**Backend:** `nextdeveloper/s3` v1.1.15
**Affects:** panel (frontend) and backup.agent (Go binary)

## What changed and why

`s3_backup_jobs` had no bucket reference at all. The agent's registration
response gives it *one* bucket, and every job silently wrote there — but the
schema already had a per-job `object_lock_enabled` toggle, which is
meaningless without a bucket to actually apply Object Lock to (WORM is a
bucket-level setting; you can't retroactively turn it on for a bucket that
already has other jobs' data in it).

Fix: jobs now carry an explicit `s3_bucket_id`.
- Not supplied + `object_lock_enabled: false` → defaults to the agent's own
  bucket from registration (the common case, unchanged behavior).
- Not supplied + `object_lock_enabled: true` → the backend **provisions a
  brand-new dedicated WORM bucket** automatically, on the same account/server
  as the agent's default bucket. No manual bucket creation step.
- Supplied explicitly → validated to belong to the same account, used as-is.

Full details: `../database.md` (`s3_backup_jobs.s3_bucket_id`) and
`../protocol.md` (`full_sync` job entry shape).

---

## For the panel team

**API surface changes on `s3_backup_jobs`** (`/s3/backup-jobs`):

- Create/update payloads now accept an optional `s3_bucket_id` (bucket UUID).
  Leave it out in the normal case — the backend picks the right bucket
  automatically based on `object_lock_enabled`.
- `s3_bucket_id` is **immutable after creation**, same rule as
  `s3_backup_agent_id`/`job_type` — don't show it as editable in the edit
  form (see `backup-agent-ui-specification.md` §5.3, "Not editable" list —
  add `s3_bucket_id` to it).
- The list/show response for a job now includes `s3_bucket_id` (resolved to
  the bucket's UUID, not the raw bigint). Useful if you want to show which
  bucket a job writes to on the job detail page — not required for v1, but
  now available if you want it (e.g. "Backing up to: `backup-agent-a1b2c3d4`").
- New filter: `GET /s3/backup-jobs?s3BucketId={uuid}`.

**Copy suggestion for the "Object Lock" toggle on the job create form**
(§5.2 of the UI spec): make clear that turning it on creates a **new, separate
bucket automatically** — e.g. "Enabling this creates a dedicated, immutable
bucket for this job's backups. You don't need to create it yourself."
This is a slightly stronger claim than the existing bucket-create-form WORM
copy, since here it's not just "irreversible," it's also "provisions
infrastructure you didn't explicitly ask to create" — worth being explicit
about in the UI so it isn't a surprise.

No changes needed to the registration flow (§4.2) — the agent's default
bucket is still delivered the same way.

---

## For the agent (Go binary) team

**This is the important one — a behavior assumption needs to change.**

Previously, nothing in the protocol told the agent which bucket a job's
backups belonged in, so it was reasonable to assume "the one bucket from
registration" for everything. **That assumption is no longer safe.** Each job
entry in the `full_sync` command payload now includes its own destination:

```json
{
  "uuid": "...",
  "name": "nightly-mysql-dump",
  "job_type": "files",
  "source_paths": ["/var/backups/mysql"],
  "schedule": "0 2 * * *",
  "bucket_name": "backup-agent-a1b2c3d4",
  "object_lock_enabled": false,
  ...
}
```

**What the agent needs to do:**

1. **Read `bucket_name` per job, not once at startup.** Don't cache "the"
   bucket from the registration response bootstrap payload and reuse it for
   every job — that value is still the *default*, but individual jobs
   (specifically ones with `object_lock_enabled: true`) may point at a
   different bucket.
2. **Initialize a Kopia repository connection per unique `bucket_name`
   encountered across the job list**, not one global connection. Most agents
   will only ever see one bucket (their default), so this is a small change,
   but a job with a dedicated WORM bucket needs its own repository
   connection against that different bucket name.
3. **Credentials**: the access key/secret from the registration response are
   scoped to the agent's *default* bucket only. A job routed to a different
   (WORM) bucket needs credentials scoped to *that* bucket — this isn't in
   the `full_sync` payload yet. **Open question, not yet resolved on the
   backend side:** how the agent gets credentials for a non-default bucket.
   Don't build against an assumption here — flag this back before
   implementing multi-bucket support, since the current registration
   response only hands out one access key/secret pair.
4. `object_lock_enabled: true` on a job is informational for the agent (the
   bucket already enforces the lock server-side via SeaweedFS) — the agent
   doesn't need to do anything differently when writing, just needs to be
   pointed at the right bucket.

If the current agent implementation (or its design docs) assumed a single
global Kopia repository per agent process, that assumption needs to be
revisited before this ships — the credentials gap in point 3 above is the
part most likely to require a follow-up backend change (e.g. including a
per-bucket-scoped access key in the `full_sync` payload for jobs that don't
use the default bucket) before a real multi-bucket agent can be built.
