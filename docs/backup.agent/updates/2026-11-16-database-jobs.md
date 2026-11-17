# Update: database backup jobs (`job_type: "database"`) and `list_databases`

**Date:** 2026-11-16
**Backend:** `nextdeveloper/s3` (same-dated update doc there covers schema and API)
**Affects:** backup.agent (Go binary), backend, panel

## What changed and why

Until now the only way to back up a database was a free-text `pre_script`
that dumped into one of the job's `source_paths`. That works on a Linux box
with someone who can write the script, and not at all for the case that
prompted this change: a Windows machine running SQL Server, where the
customer wants to tick the databases to protect.

The agent now understands databases directly:

- a new job type, `database`, that names an engine and a list of databases;
- a new command, `list_databases`, so the panel can show a picker with the
  databases that actually exist on that machine.

Supported engines: `postgres`, `mysql` (also MariaDB), `mssql`.

Nothing changes for `files` and `script` jobs.

## What changed, concretely

### `full_sync` job entry

```json
{
  "uuid": "...",
  "name": "nightly-sql-server",
  "job_type": "database",
  "schedule": "0 2 * * *",
  "script_timeout_s": 3600,
  "database": {
    "engine": "mssql",
    "connection": "default",
    "databases": ["crm", "erp"],
    "all": false
  }
}
```

| Field | Notes |
|---|---|
| `database.engine` | `postgres`, `mysql` or `mssql`. Required. |
| `database.connection` | Name of a connection in the agent's local config. Optional; empty means `default`. |
| `database.databases` | Databases to back up. Each one is checked against the server's own list at run time. |
| `database.all` | `true` = every user database present at run time; `databases` is then ignored. |
| `script_timeout_s` | Reused as the limit for the whole dump phase. Default for this type: 3600. |
| `source_paths`, `pre_script` | Ignored for this type. |

All other job fields (`keep_last_n`, `bucket_name`, …) behave as before.

### New command `list_databases`

Params: `{"engine": "mssql", "connection": "default"}` (`connection` optional).

Result output: `{"engine": "mssql", "databases": ["crm", "erp"]}`. System
databases are left out (`master`/`tempdb`/`model`/`msdb`; MySQL's
`information_schema`, `performance_schema`, `mysql`, `sys`; Postgres templates).

- No `engine` → `status: "rejected"`.
- Server unreachable, login refused, client tools missing → `status: "failed"`,
  `message` carries the database client's own error text. Show it to the user:
  it is the thing they need to fix.

### What a run does

1. Lists the server's databases and resolves the selection. A selected
   database that no longer exists fails the run with its name in the error.
2. Dumps each database to its own file in a scratch directory:
   `pg_dump` custom format (`<db>.dump`), `mysqldump` SQL (`<db>.sql`), or a
   SQL Server native backup (`<db>.bak`).
3. Snapshots that directory with the agent's normal engine.
4. Deletes the scratch directory, whether the run succeeded or not.

Any failure in steps 1–2 fails the run before anything is uploaded. The
`job_run` event and `run_job_now` result are unchanged; a dump failure shows
up in `error` prefixed with `database dump:`.

Objects land in the bucket under `stash-dump-<job_uuid>/<database>.<ext>`.

### Credentials never leave the machine

The job payload has no host, user or password. Connection settings live in
the agent's own config file, written by whoever administers that machine:

```yaml
databases:
  mssql:
    default:
      host: localhost
      port: 1433
      instance: ""        # named instance, e.g. SQLEXPRESS
      user: ""            # empty = Windows integrated auth (the agent's account)
      password: ""
      dump_dir: 'D:\StashDumps'
  postgres:
    default: { host: localhost, port: 5432, user: postgres, password: "..." }
  mysql:
    default: { host: 127.0.0.1, port: 3306, user: backup, password: "...", bin_dir: /opt/mysql/bin }
```

The whole section is optional. With nothing configured the agent uses each
engine's local defaults: the Postgres/MySQL client defaults (local socket,
`~/.my.cnf`, `PGPASSFILE`) and, for SQL Server, Windows integrated
authentication against `localhost`.

Requirements on the machine:

- **Postgres / MySQL:** the client tools (`psql` + `pg_dump`, `mysql` +
  `mysqldump`) must be installed. If they are not on the agent's `PATH`, set
  `bin_dir`.
- **SQL Server:** no client tools needed. The login needs permission to back
  up the selected databases (`db_backupoperator`, or `sysadmin`). The `.bak`
  files are written by the **SQL Server service account**, not by the agent,
  so `dump_dir` must be writable by that account and readable/deletable by
  the agent's account. If `dump_dir` is empty the instance's default backup
  directory is used, which a non-admin agent usually cannot write to — on
  Windows, set `dump_dir` to a folder both accounts can use.
- SQL Server backups are taken `WITH COPY_ONLY`, so they do not interfere
  with an existing full/differential/log backup schedule.

`stash db list --engine <engine>` and `stash db dump --engine <engine> --out <dir> <db>...`
run the same code without NATS or an upload — the quickest way to check a
machine's setup before scheduling a job.

### Known limits

- With the default `rsync` engine the bucket is a mirror: each run replaces
  the previous dump of the same database. Keeping several days of history
  needs `sync_engine: kopia` (where `keep_last_n` / `keep_for_days` apply).
- Dumps are staged on local disk before upload, so the machine needs free
  space for one full dump of the selected databases.
- No restore command yet. Restore is manual: download the file and use
  `pg_restore`, `mysql < file.sql` or `RESTORE DATABASE`.

---

## Backend and panel

Implemented alongside this change:

- **Backend** (`nextdeveloper/s3`): `s3_backup_jobs` gains `database_engine`,
  `database_connection`, `database_names`, `database_all`; `full_sync` emits
  the `database` object; `POST /s3/backup-agents/{id}/list-databases` relays
  `list_databases` synchronously and always answers 200 with
  `{ok, engine, databases, code, message}`. Note the column is
  `database_engine` because `engine` already means rsync/kopia there.
- **Panel**: the job form has a "Database" type with an engine choice, an
  "All databases" switch and a checklist fed by that endpoint.

An agent older than this change ignores the `database` field and treats the
job like a `files` job with no `source_paths`: every run fails with
"no source_paths configured". The backend reports such an agent as
`AGENT_UNSUPPORTED` when the picker tries to list its databases.
