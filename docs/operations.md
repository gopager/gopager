# Operations

GoPager is a single executable, `gopager`, backed by one PostgreSQL primary. It runs in two modes:

- **Server mode** (`gopager --profile=full --roles=…`): runs the `api` and/or `worker` roles until it receives a signal.
- **Bootstrap** (`gopager bootstrap …`): creates the initial administrator account and exits.

## Requirements

| Component | Requirement |
| --- | --- |
| Go | The version in [go.mod](../go.mod) (`go 1.26.2`), only for building |
| PostgreSQL | A writable primary. The test suite runs against PostgreSQL 18. Read replicas and hot standbys are rejected as not writable. |
| Database privileges | `apply` mode needs privileges to create tables, indexes and functions in the connection's current schema |
| Plugins (optional) | A local directory with plugin executables and manifests; see [Plugins](plugins.md) |

GoPager does not terminate TLS for HTTP. Put a TLS-terminating reverse proxy in front of the API listener for any exposed deployment.

## Build

```sh
go build -o gopager ./cmd/gopager
```

The binary embeds all SQL migrations. It needs no other files at runtime except an optional plugin directory.

## Configuration Reference

Flags use Go `flag` syntax (`--name=value` or `-name value`). Positional arguments are rejected. Any configuration error ends the process with `INVALID_INPUT` (see [Exit Status and Logging](#exit-status-and-logging)).

### Server mode

| Flag / variable | Default | Required | Meaning |
| --- | --- | --- | --- |
| `--profile` | — | always | Operating profile. Only `full` is accepted. |
| `--roles` | — | always | Comma-separated, non-empty, no duplicates: `api`, `worker` or `api,worker`. |
| `GOPAGER_DATABASE_URL` (env) | — | always | PostgreSQL connection string (URL or keyword/value form understood by pgx). Read only from the environment so it does not appear in process listings or logs. |
| `--listen-address` | — | with `api` | HTTP listen address `host:port`. Host must be non-empty (use `0.0.0.0` or `[::]` to listen on all interfaces); port 1–65535. |
| `--public-origin` | — | with `api` | Public origin browsers use to reach GoPager, `http://` or `https://` scheme, host, optional port, no path other than `/`, no query/fragment/userinfo. Validated whenever set, even for `worker`. See [Security](#security). |
| `--migration-mode` | `check` | — | `check`: start only if the schema is exactly current. `apply`: run pending migrations first. See [Database Migrations](#database-migrations). |
| `--processing-concurrency` | `2` | — | Processing candidates evaluated in parallel per worker process. Must be > 0. |
| `--processing-poll-interval` | `5s` | — | Fallback interval at which a worker looks for due work, as a Go duration (`500ms`, `5s`, `1m`). Must be > 0. New work normally starts immediately through a PostgreSQL notification (channel `gopager_processing_due`, sent by intake); the poll covers lost notifications and work that becomes due by time. |
| `--processing-artifact-root` | — | — | Plugin directory. Must exist and be openable; leading/trailing whitespace and an explicitly empty value are rejected. |
| `GOPAGER_PROCESSING_ARTIFACT_ROOT` (env) | — | — | Plugin directory, used only when `--processing-artifact-root` is not given. Same validation. |
| `--login-attempts-per-account` | `10` | — | Login attempts per minute per normalized username. Must be > 0. |
| `--login-attempts-per-instance` | `300` | — | Login attempts per minute across the whole instance. Must be > 0. |
| `--allow-insecure-db` | `false` | — | Permit a non-local database connection without certificate-verified TLS. See [Database TLS](#database-tls). |

Fixed values (not configurable):

| Setting | Value |
| --- | --- |
| Database pool | 10 open, 5 idle connections, 30 min connection lifetime |
| Per-call / per-transaction timeout | 30 s |
| Readiness re-check interval | 5 s |
| HTTP `ReadHeaderTimeout` | 5 s |
| Shutdown deadline | 30 s |

### Bootstrap

| Flag / variable | Default | Required | Meaning |
| --- | --- | --- | --- |
| `GOPAGER_DATABASE_URL` (env) | — | yes | As in server mode. |
| `--username` | — | yes | Initial account name: 1–128 characters, no leading/trailing whitespace, no control characters. |
| `--actions` | — | yes | Comma-separated actions granted with scope `all`. Each must be a catalog action; no wildcards. |
| `--fields` | — | — | Comma-separated readable fields: `payload`, `address`, `unit`, `gateway`, `quality`, `derived`, or `extension.<namespace>.<name>`. |
| `--delegable` | `false` | — | Allow the account to delegate the granted actions. |
| `--migration-mode` | `check` | — | `check` or `apply`, as in server mode. |
| `--allow-insecure-db` | `false` | — | As in server mode. |
| stdin | — | yes | Password from a pipe or file (not a terminal). 12–1024 characters, valid UTF-8. One trailing LF is stripped; all other bytes are kept. |

## Roles

| Role | Runs |
| --- | --- |
| `api` | HTTP listener with health endpoints and all `/api/v1` routes (authentication, management, gateway intake and administration, processing). See [HTTP API](http-api.md). |
| `worker` | Processing dispatcher (executes due workflow work) and the assignment correction runner. No listener. |

Both roles load the plugin directory, if configured, because workflow management and preview on the API also execute plugins.

Topologies:

- **Combined**: one process with `--roles=api,worker`.
- **Split**: any number of `--roles=api` and `--roles=worker` processes against the same database. Workers coordinate through the database (selected rows are locked with `SKIP LOCKED`, claims are revision-checked); additional workers wake through the same notifications and otherwise pick up work on their poll (`--processing-poll-interval`, default 5 s). Each worker process holds one extra dedicated database connection for the notification listener, outside the pool.

All processes of one deployment must run the same release (see [Upgrade](#upgrade)).

## Database Migrations

Migrations are embedded in the binary and tracked in the table `platform_schema_migrations` in the connection's current schema. Checking and applying are serialized across processes with a PostgreSQL session advisory lock (key `0x47504D4947520001`).

| Database state | `check` | `apply` |
| --- | --- | --- |
| Empty (no ledger table) | fails `INCOMPATIBLE_VERSION` | creates ledger, applies all migrations |
| Applied history is a strict prefix of the binary's migrations | fails `INCOMPATIBLE_VERSION` | applies the missing suffix in order |
| Applied history equals the binary's migrations | starts | starts |
| History contains unknown/extra versions (database is newer), gaps or failed entries | fails `INCOMPATIBLE_VERSION` | fails `INCOMPATIBLE_VERSION` |
| Ledger table has an unexpected layout | fails `INCOMPATIBLE_VERSION` | fails `INCOMPATIBLE_VERSION` |

Each migration and its ledger row commit in one transaction; a failing migration rolls back and leaves earlier ones applied. Down migrations are never executed. If a commit outcome cannot be determined, the result is `COMMIT_UNCERTAIN`; restart with `apply` to re-check.

Every transaction compares the applied schema version with the version the process started with (one query, no lock). A running process therefore stops accepting work (`INCOMPATIBLE_VERSION`, not ready) as soon as another process applies migrations it does not know.

### Upgrade

1. Take a database backup (GoPager has no built-in backup or rollback).
2. Stop all GoPager processes.
3. Start one server process of the new release with `--migration-mode=apply` and wait until `/health/ready` returns `200` (or, for a `worker`-only process, until `platform started` is logged).
4. Start the remaining processes with the default `--migration-mode=check`.

Rolling upgrades with mixed releases are not supported. Downgrading the binary after a schema change fails with `INCOMPATIBLE_VERSION`; restore the backup instead.

## Event Log Retention

`domain_events` (the sequenced fact log) and `consumer_offsets` (one read position per consumer) grow without bound until a retention job prunes them; no such job is implemented yet. Both tables join the retention list owned by [C-12](../development/contracts/c-12-persistence-operations.md). The rule the job must follow: a `domain_events` row is prunable only when its `seq` is at or below the lowest `last_seq` of every registered consumer **and** it is older than the replay window, so no consumer can be left with a gap and late-registered consumers still find recent history. A `consumer_offsets` row is removed only when its consumer is decommissioned.

## First Start and Bootstrap

A fresh installation has no accounts and no default credentials. Create the initial administrator locally:

```sh
export GOPAGER_DATABASE_URL='postgres://gopager:…@db.internal:5432/gopager?sslmode=verify-full'
printf '%s\n' 'a long initial password' > /run/secrets/gopager-admin   # or any protected file
gopager bootstrap \
  --username=admin \
  --actions=password.local,session.manage-own,account.read,account.create,account.update,grant.manage \
  --migration-mode=apply \
  < /run/secrets/gopager-admin
```

| Outcome | stdout | Exit |
| --- | --- | --- |
| Success | the new account's UUID, one line | 0 |
| Installation already initialized | — (log `code=REVISION_CONFLICT`) | 1 |
| Invalid username, action, field, password, or stdin is a terminal | — (log `code=INVALID_INPUT`) | 1 |
| Schema not current and `--migration-mode=check` | — (log `code=INCOMPATIBLE_VERSION`) | 1 |

Bootstrap opens no listener, completes within 30 s, and can succeed only once per database. Account, grants, field permissions, audit record and the bootstrap fact are committed together. The account receives exactly the listed actions with scope `all`; there is no implicit administrator role. `password.local` is needed to log in with the password, `session.manage-own` to manage the own session. The full action catalog is `authz.Actions()` in [internal/authz/grants.go](../internal/authz/grants.go):

| Group | Actions |
| --- | --- |
| Messages | `message.read`, `message.detail`, `without-unit.read` |
| Master data | `unit.*`, `organization.*`, `unit-collection.*`, `unit-template.*` (each `read`, `create`, `update`, `delete`) |
| Assignment | `address-rule.*`, `suppression.*` (each `read`, `create`, `update`, `delete`), `assignment-correction.read`, `assignment-correction.retry` |
| Gateways | `gateway.*`, `network.*` (each `read`, `create`, `update`, `delete`), `grouping.configure` |
| Accounts | `account.read`, `account.create`, `account.update`, `account.delete`, `account.delete-own`, `grant.manage`, `password.local`, `recovery.request`, `recovery.issue`, `recovery.consume`, `session.manage-own`, `key.create-own`, `key.manage-own`, `key.manage-technical` |
| Subscriptions and delivery | `subscription.manage-own`, `subscription.manage-global`, `delivery.read`, `delivery.retry`, `delivery.redeliver` |
| Processing | `rule.read`, `rule.create`, `rule.update`, `rule.delete`, `rule.preview`, `rule.activate`, `rule.reprocess`, `presentation.manage`, `plugin.configure`, `plugin.internal-read` |
| Other | `notice.manage`, `operations.manage`, `audit.read` |

The `*` notation above is shorthand for this table only; `--actions` requires each action spelled out.

## Starting the Server

Minimal combined deployment behind a TLS-terminating proxy:

```sh
export GOPAGER_DATABASE_URL='postgres://gopager:…@db.internal:5432/gopager?sslmode=verify-full'
gopager \
  --profile=full \
  --roles=api,worker \
  --listen-address=127.0.0.1:8080 \
  --public-origin=https://pager.example.org \
  --processing-artifact-root=/srv/gopager/plugins
```

Local development against a loopback database:

```sh
GOPAGER_DATABASE_URL='postgres://gopager:secret@127.0.0.1:5432/gopager?sslmode=disable' \
  gopager --profile=full --roles=api,worker --migration-mode=apply \
  --listen-address=127.0.0.1:8080 --public-origin=http://127.0.0.1:8080
```

Startup order: parse and validate configuration → open the database (TLS rule) → compose roles → ping, check write authority (`pg_is_in_recovery()` false and `transaction_read_only` off), check/apply migrations → open the listener and start workers → log `platform started`. Any failure before the last step ends the process with exit 1.

## Health Endpoints

Served on the `api` listener only, `GET` only (other methods: `405`, `Allow: GET`), no authentication.

| Endpoint | Response | Meaning |
| --- | --- | --- |
| `GET /health/live` | `200 {"live":true}` | The process is serving HTTP. |
| `GET /health/ready` | `200 {"ready":true}` / `503 {"ready":false}` | The process accepts work. |

The process is ready when the database is reachable, is a writable primary, the schema matches the binary exactly, no role has failed and shutdown has not started. Readiness is re-checked every 5 s (connectivity, write authority and schema together); it returns to ready automatically when the database recovers, unless a role has failed.

While not ready, every non-health route returns `503` with error code `AUTHORITY_UNAVAILABLE` and `retryable: true` (error envelope: [HTTP API](http-api.md)).

The responses contain no other details. Profile, roles, migration state (`current` / `incompatible`) and failure code are kept in-process; failure codes appear in logs.

## Failure Handling

| Situation | Behavior |
| --- | --- |
| Database unreachable or demoted at runtime | Readiness becomes false; log `platform readiness check failed code=…`; recovers automatically. |
| Worker runner hits a transient database failure (`STORAGE_UNAVAILABLE`, `AUTHORITY_UNAVAILABLE`, `COMMIT_UNCERTAIN`, `REVISION_CONFLICT`) | The runner keeps running. It logs a warning with the code, the number of consecutive failures and the delay, and retries after a backoff that doubles up to 30 s: the processing dispatcher starts at its 5 s poll interval (`Processing dispatch selection failed; retrying`), the correction runner at 500 ms (`correction runner storage failure; retrying`). The first successful attempt logs `Processing dispatch selection recovered` or `correction runner recovered`. No restart is needed. |
| A worker runner exits with any other error | Log `platform role stopped code=…`; the process stays not ready permanently, keeps running, and exits 1 at shutdown. Restart the process. |
| HTTP listener fails | Same as a failed worker runner. |

A `worker`-only process has no health endpoint. Monitor its log for `platform role stopped` and restart it, or run it combined with `api` if an HTTP probe is required. Repeated `…; retrying` warnings indicate a database outage the worker is waiting out.

## Exit Status and Logging

| Exit status | Meaning |
| --- | --- |
| 0 | Normal shutdown after `SIGINT`/`SIGTERM`, or successful bootstrap |
| 1 | Startup failure, failed role, or shutdown failure |

Logs are written to stderr by Go's default `log/slog` handler (`2026/11/05 23:54:28 INFO platform started profile=full roles=[api worker]`). Errors are logged by code only; connection strings, credentials, causes and payloads are never logged. On exit 1 the last line is `ERROR GoPager exited code=<CODE>`.

| Code | Typical operational cause |
| --- | --- |
| `INVALID_INPUT` | Bad flag, missing variable, invalid public origin, insecure remote database without `--allow-insecure-db`, invalid plugin directory, bootstrap input rejected |
| `INCOMPATIBLE_VERSION` | Schema not current in `check` mode, or database newer than the binary |
| `STORAGE_UNAVAILABLE` | Database unreachable, timeout, shutdown deadline exceeded |
| `AUTHORITY_UNAVAILABLE` | Database is a standby or read-only |
| `REVISION_CONFLICT` | Concurrent change; bootstrap on an initialized installation |
| `RESOURCE_LIMIT` | Database resource limit or numeric overflow |
| `COMMIT_UNCERTAIN` | Connection lost during commit; outcome unknown |
| `INTERNAL_ERROR` | Any other failure, including listener bind errors |

Log lines in use: `platform started`, `platform readiness check failed`, `platform role stopped`, `GoPager exited`, the worker retry warnings `Processing dispatch selection failed; retrying` and `correction runner storage failure; retrying` with their recovery lines `Processing dispatch selection recovered` and `correction runner recovered`, and the warning `operator approved a remote database connection without certificate-verified TLS`.

## Database TLS

| Database host | Required TLS |
| --- | --- |
| Unix socket path, `localhost`, loopback IP | none |
| Any other host (including every fallback host in a multi-host URL) | certificate-verified TLS: `sslmode=verify-ca` or `sslmode=verify-full` |

`sslmode=disable`, `allow`, `prefer` or `require` to a non-local host fails with `INVALID_INPUT` unless `--allow-insecure-db` is set. With the flag, GoPager connects and logs a warning at every start. Use it only on a trusted private network.

## Plugin Directory

`--processing-artifact-root` (or `GOPAGER_PROCESSING_ARTIFACT_ROOT`) names a local directory of plugin manifests and executables. All file access is confined to this directory. Executables must be regular files with an executable bit, at most 256 MiB, and match the SHA-256 digest in their manifest. Plugin processes are started on demand (verified against the pinned digest at every start), reused by later runs of the same instance revision, stopped after 5 minutes without calls, and receive only `PATH`, `HOME`, `LANG`, `LC_ALL`, `TZ` and `TMPDIR` from GoPager's environment — never `GOPAGER_DATABASE_URL`.

Without a plugin directory GoPager runs normally; creating, inspecting or executing plugin instances fails with `AUTHORITY_UNAVAILABLE`. Configure the same directory contents on every `api` and `worker` process. Manifest format and installation: [Plugins](plugins.md); configuring instances: [Workflows](workflows.md).

## Graceful Shutdown

On `SIGINT` or `SIGTERM`:

1. Readiness becomes false; new application requests get `503 AUTHORITY_UNAVAILABLE`.
2. Workers are cancelled; the HTTP server stops accepting connections and waits for in-flight requests.
3. Running work and transactions are drained, then the database pool and plugin processes are closed.

The total deadline is 30 s. Exceeding it, or any role failure during the run, yields exit 1. Give the process at least 30 s between `SIGTERM` and `SIGKILL` (e.g. Docker `--stop-timeout=35`, Kubernetes `terminationGracePeriodSeconds: 35`). Processing work claimed but not completed before shutdown is recovered by a worker after restart; completed results are not recomputed.

## Security

- **Public origin.** `--public-origin` is the single source for cookie and CSRF origin policy. `Host` and `X-Forwarded-*` headers are ignored. Set it to the exact origin users type into the browser (e.g. `https://pager.example.org`); default ports `80`/`443` are normalized away.
- **Session cookie.** `gopager_session`, host-only, `Path=/`, `HttpOnly`, `SameSite=Lax`, `Secure` when the public origin is `https://`. Behind an HTTPS proxy, use the `https://` origin even though GoPager itself listens on HTTP.
- **Browser requests.** Login and cookie-authenticated state-changing requests require a single `Origin` header equal to the public origin; state-changing requests also require `X-CSRF-Token`. API keys use `Authorization: Bearer …` and are not subject to origin checks. Details: [HTTP API](http-api.md).
- **Login rate limits.** Per normalized username and per instance, per minute (`--login-attempts-per-account`, `--login-attempts-per-instance`). Limits are not keyed by client IP. Rejections return a retry interval.
- **Transport.** GoPager serves plain HTTP. Restrict `--listen-address` to loopback or a private interface and terminate TLS at a proxy.
- **Secrets.** Provide `GOPAGER_DATABASE_URL` from a secret store; never pass passwords as arguments. Bootstrap reads the password from stdin only.
- **Database role.** Day-to-day processes can use `check` mode; reserve DDL privileges for the process or job that runs `apply`, if your setup separates roles.

Component overview and data flow: [Architecture](architecture.md). Building and testing: [Development](development.md).
