# Running stash on Windows

Tested on Windows Server 2025 with SQL Server 2022 Express.

## Install

From an administrator PowerShell or Command Prompt:

```
stash.exe register --token <one-time token> --endpoint <platform API URL>
```

`register` writes the config, then installs and starts the `stash` Windows
service. The service starts with the machine and Windows restarts it if it
exits unexpectedly.

A machine that was registered before service support existed, or from a
shell without administrator rights, can install the service afterwards:

```
stash.exe service install
```

Useful commands:

```
sc query stash      # is it running?
sc stop stash
sc start stash
sc delete stash     # remove the service (stop it first)
```

## Where files live

Everything is under `%ProgramData%\PlusClouds\Stash`
(normally `C:\ProgramData\PlusClouds\Stash`):

| File | Purpose |
|---|---|
| `stash.yaml` | Config: agent identity, bucket, S3 key, database connections |
| `stash-state.json` | Job list and run bookkeeping |
| `stash.log` | Agent log |
| `kopia-cache\`, `work\` | Scratch space |

`stash.yaml` holds secrets. `register` sets its access list so only
Administrators and SYSTEM can read it. A config file you create or copy by
hand keeps whatever access its folder gives it — check it yourself.

Pass `--config <path>` to any command to use a different config file. The
service is installed with the config path that was in effect at the time.

## SQL Server database jobs

The agent connects to SQL Server itself and runs `BACKUP DATABASE`; no
`sqlcmd` or other client tool is needed.

With no `user` in the connection settings it signs in with Windows
authentication, as the account the agent runs under. The service runs as
**Local System**, which SQL Server sees as `NT AUTHORITY\SYSTEM`.

That login needs to be allowed to back up the databases. Whether it already
is depends on how SQL Server was installed — it is when
`BUILTIN\Administrators` was made a SQL Server administrator, and often is
not otherwise. A database job that fails with a permission error from SQL
Server usually means this. To grant it, per database:

```sql
USE [your_database];
CREATE USER [NT AUTHORITY\SYSTEM] FOR LOGIN [NT AUTHORITY\SYSTEM];
ALTER ROLE db_backupoperator ADD MEMBER [NT AUTHORITY\SYSTEM];
```

Or use a SQL Server login instead, by setting `user` and `password` on the
connection.

A named instance such as `SQLEXPRESS` is found automatically: when the
connection settings name no `host`, `port` or `instance`, the agent looks up
the SQL Server instances installed on the machine. It uses the default
instance if there is one, otherwise the only named instance. Connecting to a
named instance relies on the **SQL Server Browser** service running, the
same as for any other SQL Server client.

With several named instances and no default one the agent does not guess —
the job fails with a message listing them. Choose one in the connection
settings:

```yaml
databases:
    mssql:
        default:
            instance: SQLEXPRESS
```

SQL Server writes the `.bak` files itself, into its default backup
directory (or `dump_dir` if set), and the agent removes them after upload.

Check the setup the same way the service will see it:

```
stash.exe db list --engine mssql
```

Run from an administrator shell this uses *your* Windows account, not
Local System, so it can succeed where the service would be refused.

## MySQL, MariaDB and PostgreSQL database jobs

These engines are backed up with their own client tools (`mysql` and
`mysqldump`, `psql` and `pg_dump`). Their Windows installers do not put the
tools on `PATH`, so the agent also looks in the usual install directories
under `Program Files` — `MySQL\MySQL Server <version>\bin`,
`MariaDB <version>\bin`, `PostgreSQL\<version>\bin` — and uses the newest
version it finds. Set `bin_dir` on the connection to use a different one.
(The same search exists on Linux and macOS, over the locations their
package managers and installers use.)

The login cannot be found automatically. Unlike SQL Server there is no
Windows sign-in to fall back on, so the connection needs a user and password:

```yaml
databases:
    mysql:
        default:
            host: 127.0.0.1
            port: 3306
            user: backup
            password: "..."
```

The password is stored in `stash.yaml` in plain text; see the note on that
file's access list above. Use an account that can only read, not `root`.

