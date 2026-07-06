# backup.agent — Registration

Every other agent type in this system (`vm`, `storage`, `compute`, `network`, `s3`)
is pre-provisioned: the member row and its `agent_api_key` exist before the agent
process ever starts, delivered via a config-drive ISO or written directly at
VM-creation time. backup.agent can't work that way — it runs on a customer's own
arbitrary machine (a laptop, an on-prem server, a VM at another provider) that has
no pre-existing relationship with this platform at all.

So it needs a real bootstrap handshake. This is modeled on the one already built
for the in-VM "Managed Services" agent
(`App\Http\Controllers\ManagedServices\AgentRegistrationController` — one-time
token → `POST /agents/register` → session credentials), not on the SSE/REST design
that was tried and abandoned for the S3 agent (see storaged's own history: it
shipped with a REST/SSE design, then explicitly moved to NATS in v0.2.0).

## Flow

### 1. Customer issues a registration token

```
POST /s3/backup-agents/
```

IAM-authenticated, standard dashboard/API call. **Requires an existing
`s3_bucket_id`** the customer already owns — as of
`updates/2026-07-05-no-bucket-auto-provisioning.md`, this endpoint no longer
auto-provisions a bucket; no bucket, no token. Maps to
`BackupAgentsService::create()`, which does **not** create an active agent — it
creates a `pending` row with a one-time `registration_token` (48-char random,
URL-safe) and a 1-hour expiry.

Response: the pending `BackupAgents` record, including the `registration_token`
the customer copies into the install command on their machine (e.g.
`backup-agent register --token=<token> --endpoint=https://api.example.com`).

### 2. Agent calls the public register endpoint

```
POST /backup-agents/register
{
  "token": "<registration_token>",
  "hostname": "db01.internal",
  "os": "linux",
  "arch": "amd64",
  "machine_fingerprint": "...",
  "agent_version": "1.0.0"
}
```

This route is **not** part of this package's IAM-authenticated route group
(`src/Http/api.routes.php`, mounted under the `s3` prefix). It's registered
directly in the host app's `routes/api.php`, inside a
`Route::prefix('/backup-agents')->withoutMiddleware($iamMiddleware)` group — the
same pattern already used for `/agents/register` (VM managed-services agent).
Auth here is the one-time token in the body, not a bearer/session credential.

Maps to `BackupAgentsService::register()`, which:

1. Looks up the `pending` row by `registration_token`, rejects if not found or
   expired.
2. Resolves the S3 tenant `Accounts` row for the agent's owning IAM account —
   done explicitly from the pending row's own `iam_account_id`/`iam_user_id`,
   never via `UserHelper::currentAccount()`/`me()`, which have nothing to
   resolve outside an authenticated request.
3. Uses the customer-owned bucket named at token-issuance time
   (`s3_bucket_id`, required as of
   `updates/2026-07-05-no-bucket-auto-provisioning.md`) — the agent's default
   bucket is now always something the customer explicitly created and chose,
   never something the registration flow provisions on their behalf.
4. Creates a scoped IAM access key for that bucket via the existing
   `AccessKeysService::create()`.
5. Generates the NATS credential (`agent_api_key`, same 48-char random format),
   flips `status` to `active`, clears the token.

Response — this **is** the agent's entire bootstrap config, written to its local
config file and used for every NATS connection from then on:

```json
{
  "agent_uuid": "...",
  "agent_api_key": "...",
  "nats": { "host": "...", "port": 4222 },
  "bucket": { "name": "backup-agent-a1b2c3d4", "endpoint": "..." },
  "access_key": { "access_key": "AKIA...", "secret_key": "..." },
  "jobs": { "jobs": [ /* full_sync payload — see protocol.md */ ] },
  "sync_engine": "rsync"
}
```

The `secret_key` and `agent_api_key` are shown exactly once, the same rule that
already applies to every other S3 access key in this system — losing them means
re-registering, not recovering them.

`sync_engine` (optional, added in
`updates/<today>-agent-sync-engine-selection.md`) selects which backup engine
the agent uses: `"rsync"` (a plain, unencrypted, undeduplicated 1:1 file
mirror — browsable objects, matching the source tree exactly) or `"kopia"`
(deduplicated, encrypted, incremental snapshots — opaque pack/index blobs,
not directly browsable). This is registration-time only, not something
`full_sync` can change later. Omit it (or send an empty string) to leave the
agent's existing default (`"rsync"`) alone — it does **not** need to be sent
on every registration, only when the platform wants to state an explicit
preference.

### 3. From here on, it's just another agent

Once bootstrapped, backup.agent behaves like every other agent type: connect to
NATS with `nats.UserInfo(agent_uuid, agent_api_key)`, subscribe to
`agent.backup.{uuid}.cmd`, publish heartbeat/telemetry/job outcomes to
`agent.backup.{uuid}.evt`. `NatsAuthCalloutService` validates the credential
against `s3_backup_agents.agent_api_key` exactly like it does for
`s3_servers.agent_api_key` — one entry in `AGENT_TABLES`, no special-casing.

## Revocation

```
POST /s3/backup-agents/{uuid}/revoke
```

IAM-authenticated. `BackupAgentsService::revoke()` sends the `revoke` command
*first* — while the agent can still receive it — then clears `agent_api_key` and
sets `status = revoked`. The agent is rejected on its next NATS reconnect attempt,
same rule as every other agent type in this system.
