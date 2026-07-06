# backup.agent — Protocol

Shares the same envelope and subject convention as every other agent type in this
system (`plusclouds.api.v4/docs/agents/protocol.md`) — `agent_type: "backup"` is
the only thing that differs.

## Envelope (both directions)

```json
{
  "v":          1,
  "id":         "550e8400-e29b-41d4-a716-446655440000",
  "type":       "command|heartbeat|telemetry|job_run|alert|result",
  "agent_type": "backup",
  "agent_uuid": "550e8400-e29b-41d4-a716-446655440001",
  "timestamp":  1748000000,
  "payload":    {}
}
```

## Subjects

| Direction | Subject |
|---|---|
| Platform → agent (commands) | `agent.backup.{uuid}.cmd` |
| Agent → platform (events)   | `agent.backup.{uuid}.evt` |

---

## Platform → Agent (commands)

Published by `NextDeveloper\S3\Services\BackupAgentCommandService`. Envelope
`type` is always `"command"`; `id` becomes `command_id` in the agent's `result`.

| `operation` | Params | Notes |
|---|---|---|
| `full_sync` | `{jobs: [...]}` | Complete desired job list — apply idempotently. Sent on registration and after every job CRUD change. |
| `run_job_now` | `{job_uuid, run_uuid}` | `run_uuid` is pre-created by the platform (see `database.md`) — the agent must echo it back in its `result.output.run_uuid`. |
| `pause_job` | `{job_uuid}` | |
| `resume_job` | `{job_uuid}` | |
| `cancel_job` | `{job_uuid}` | Cancel a currently-running job. |
| `verify_snapshot` | `{job_uuid, snapshot_id}` | Restore the given Kopia snapshot to a temp location and checksum-verify it. Long `timeout_s` (1800s) — a restore+checksum pass takes a while. |
| `revoke` | `{reason}` | Shut down cleanly. Sent before `agent_api_key` is cleared — the agent has a brief window to react before it's rejected on reconnect. |

### `full_sync` job entry shape

Each item in `payload.jobs` (see `BackupJobsService::buildFullSyncPayload()`):

```json
{
  "uuid": "...",
  "name": "nightly-mysql-dump",
  "job_type": "files|script",
  "source_paths": ["/var/backups/mysql"],
  "pre_script": "#!/bin/bash\nmysqldump ... > /var/backups/mysql/dump.sql\n",
  "script_timeout_s": 900,
  "schedule": "0 2 * * *",
  "keep_last_n": 14,
  "keep_for_days": null,
  "bandwidth_limit_mbps": 50,
  "bucket_name": "backup-agent-a1b2c3d4",
  "object_lock_enabled": false
}
```

- `job_type: "files"` — snapshot `source_paths` directly.
- `job_type: "script"` — run `pre_script` first (respecting `script_timeout_s`); a
  non-zero exit or missing/empty output must fail the run *before* the agent ever
  touches Kopia. A failed pre-script is never allowed to look like a successful
  backup (see `overview.md` — this is the direct answer to the "silent failure"
  problem the design research turned up).
- `schedule` is a standard cron expression, evaluated **by the agent**, not the
  platform (see `overview.md`).
- `bucket_name` (added `updates/2026-07-04-per-job-destination-bucket.md`) — the
  job's destination bucket. Omitted/empty means the agent's own default bucket
  from registration; a job with `object_lock_enabled: true` gets a distinct,
  platform-provisioned WORM bucket here instead. **The registration response
  only ever hands out credentials for the default bucket** — a job whose
  `bucket_name` differs from it cannot currently be serviced (the agent
  surfaces this as a failed run, not a crash; see the update doc's open
  question about per-bucket credentials, not yet resolved on the backend side).
- `object_lock_enabled` is informational only — the named bucket already
  enforces WORM server-side; the agent doesn't change write behavior based on
  this flag.

---

## Agent → Platform (events)

Published to `agent.backup.{uuid}.evt`, routed by
`NextDeveloper\S3\Services\BackupAgentEventService::handle()`.

### `heartbeat`

Every ~30s. `{version, uptime_s, tasks_queued}` — same shape as every other agent
type. The first heartbeat after registration triggers a `full_sync` from the
platform (mirrors the "pending server connected" behavior already in
`S3AgentService`).

### `telemetry`

OS-level metrics (disk free, etc). There's no dedicated telemetry table for backup
agents — the thing that actually matters here (did the backup work) travels in
`job_run`/`result` instead, not a periodic snapshot.

### `job_run`

**The core message of this protocol.** Sent when the agent's own cron fires a job
and it finishes — there was no platform command for this, so there's no
`command_id` to reply to.

```json
{
  "type": "job_run",
  "payload": {
    "job_uuid":          "...",
    "run_uuid":          "...",
    "status":            "completed",
    "started_at":        "2026-07-04T02:00:00Z",
    "finished_at":       "2026-07-04T02:03:41Z",
    "bytes_uploaded":    104857600,
    "bytes_deduped":     943718400,
    "kopia_snapshot_id": "k1a2b3c4",
    "error":             null
  }
}
```

The agent generates its own `run_uuid` and the platform creates the
`s3_backup_job_runs` row with that exact uuid (`BackupJobRunsService::recordRun()`)
— a redelivered `job_run` is naturally idempotent rather than creating a duplicate
row with a fresh id.

### `alert`

Same shape as every other agent type — `{severity, code, message, details}` — fired
via `Events::fire("alert:backup.{code}", ...)`.

### `result`

Reply to a platform-initiated command (`run_job_now`, `pause_job`, etc.) — the only
path with a `command_id`, since scheduled runs never go through this method (see
`job_run` above).

```json
{
  "type": "result",
  "payload": {
    "command_id": "...",
    "status":     "completed",
    "message":    "...",
    "output": {
      "run_uuid":          "...",
      "bytes_uploaded":    104857600,
      "bytes_deduped":     943718400,
      "kopia_snapshot_id": "k1a2b3c4"
    }
  }
}
```

`status` values: `completed`, `failed`, `rejected` — same convention as every
other agent type.

## Versioning

Same rule as the rest of the system: `v: 1` today; agents should reject unknown
versions and log a warning. New optional fields can be added without a version
bump; removing or renaming a required field requires a new version.
