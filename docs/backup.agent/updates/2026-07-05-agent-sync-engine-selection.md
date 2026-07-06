# Update: agent-selectable backup engine (`sync_engine`)

**Date:** 2026-07-05
**Backend:** `nextdeveloper/s3`
**Affects:** panel (frontend), backend (`BackupAgentsService::register()`), and backup.agent (Go binary)

## What changed and why

backup.agent has gained a second backup engine alongside Kopia: a native Go S3
mirror (`engine/s3sync`) that copies files into the bucket 1:1 — plain,
unencrypted, undeduplicated objects at their original relative paths — instead
of Kopia's deduplicated, encrypted, opaque pack/index blobs. Motivation: a
customer browsing their bucket directly (e.g. mounting it via s3fs) expects to
see their actual files, not backup-tool internals.

This is the new **default** for the agent, not a replacement for Kopia — both
engines ship, selectable via a local `agent.sync_engine` config value
(`"rsync"` or `"kopia"`) or, new as of this change, via an optional field in
the registration response.

## What changed, concretely

- The registration response (`POST /backup-agents/register`) now accepts an
  optional `sync_engine` field: `"rsync"` or `"kopia"`. Absent or empty means
  "don't state a preference" — the agent keeps whatever it already had (an
  operator's local config setting, or its own `"rsync"` built-in default).
- This is **registration-time only**. It is not part of `full_sync`'s payload
  and there is no live "change an already-running agent's engine" mechanism —
  changing it requires a new registration.
- No other part of the registration/full_sync contract changed.

Full details: `../registration.md` (response schema), `../overview.md`
("Engine" design decision).

---

## For the panel team

No UI action required for this to function — an absent `sync_engine` is a
fully valid response (the agent defaults to `"rsync"` on its own). If/when the
panel wants to let customers choose an engine at agent-creation time (e.g. "1:1
file mirror" vs. "encrypted, deduplicated snapshots" as a plan/tier choice),
that's a separate follow-up: this change only adds the field to the contract,
it doesn't yet wire any UI to set it.

## For the backend team

If/when there's a reason to send a non-default value (e.g. a plan tier that
should always use Kopia), add `"sync_engine": "kopia"` to the registration
response `BackupAgentsService::register()` builds. Leaving the field out
entirely (or `null`/`""`) is always safe — it explicitly means "agent decides,"
not "agent gets a broken engine." There is no equivalent field needed in
`full_sync`; this backend team's job list payload is unaffected by this change.

## For the agent (Go binary) team

Already implemented in this change:
- `internal/backup/register.BootstrapResponse` gained `SyncEngine string
  \`json:"sync_engine,omitempty"\``.
- `register_cmd.go`'s `runRegister` only overwrites
  `cfg.Agent.SyncEngine` when the response's value is non-empty — an
  operator's existing local choice (or the `"rsync"` default already applied
  by `config.Load`) survives an empty response untouched.
- `run_cmd.go`'s `runDaemon` picks between `engine/s3sync.New(logger)` and
  `engine/kopia.New(logger)` based on `cfg.Agent.SyncEngine`, defaulting to
  `s3sync` for `""`/`"rsync"`; an unrecognized value fails startup with a
  clear error rather than silently picking one.
