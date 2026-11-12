# Operations

GoPager is a single executable, `gopager`, backed by one PostgreSQL primary. It runs in two modes:

- **Server mode** (`gopager --roles=…`): runs the `api` and/or `worker` roles until it receives a signal.
- **Bootstrap** (`gopager bootstrap …`): creates the initial administrator account and exits.

## Requirements

| Component | Requirement |
| --- | --- |
| Go | The version in [go.mod](../go.mod) (`go 1.27.4`), only for building |
| PostgreSQL | A writable primary. The test suite runs against PostgreSQL 18. Read replicas and hot standbys are rejected as not writable. |
| Database privileges | `apply` mode needs privileges to create tables, indexes and functions in the connection's current schema |
| Plugins (optional) | A local directory with plugin executables and manifests; see [Plugins](plugins.md) |

GoPager does not terminate TLS for HTTP. Put a TLS-terminating reverse proxy in front of the API listener for any exposed deployment.

## Topology

GoPager always runs against PostgreSQL; there is no SQLite and no embedded database. "Lite" and "Full" are deployment profiles, not binary modes: the binary and its flags are identical.

| Profile | Layout | Use |
| --- | --- | --- |
| Lite | One host, one `gopager --roles=api,worker` process, PostgreSQL as a compose sidecar on the same host | Small installations, trials, constrained hosts |
| Full | `api` and `worker` processes split across hosts ([Roles](#roles)), against an external PostgreSQL primary | Larger installations, separate scaling and failure domains |

### Lite with Docker Compose

[deploy/compose/](../deploy/compose/) holds `compose.yaml`, a `Dockerfile` (multi-stage, Go version from `go.mod`, non-root runtime user) and `.env.example`.

```sh
cd deploy/compose
cp .env.example .env          # set POSTGRES_PASSWORD (URL-safe) and GOPAGER_PUBLIC_ORIGIN
docker compose up --build -d
curl -fsS http://localhost:8080/health/ready
printf '%s\n' 'a long initial password' | docker compose run --rm -T gopager bootstrap \
  --username=admin --actions=password.local,session.manage-own,account.read,account.create,account.update,grant.manage \
  --allow-insecure-db
```

The image also contains the two first-party Delivery plugins (Pushover and Webhook) in `/usr/lib/gopager/plugins`, and the compose file sets `GOPAGER_PROCESSING_ARTIFACT_ROOT` to that directory (see [Plugin Directory](#plugin-directory)). To store Secrets in the application, also give the `gopager` service a master key (see [Secrets](#secrets)); the compose file does not set one.

The `gopager` service waits for the PostgreSQL healthcheck (`pg_isready`), then starts with `--migration-mode=apply`. The database host `postgres` is not loopback, so the compose file passes `--allow-insecure-db`; the private compose network is the trust boundary (see [Database TLS](#database-tls)). Data lives in the named volume `pgdata`; `docker compose down -v` deletes it. TLS for browsers still needs a reverse proxy.

- **Upgrade:** take a backup (below), `git pull` (or fetch the new release), `docker compose up --build -d`. The new container applies migrations at start. See [Upgrade](#upgrade).
- **Backup:** `docker compose exec -T postgres pg_dump -U gopager -Fc gopager > gopager.dump`.
- **Restore:** into an empty database, with GoPager stopped: `docker compose stop gopager`, then `docker compose exec -T postgres pg_restore -U gopager -d gopager --clean --if-exists < gopager.dump`, then `docker compose start gopager`.
- **Moving a Lite installation** to a Full one is a `pg_dump` / `pg_restore` into the external PostgreSQL, then starting the roles there. The former SQLite transfer path no longer exists.

## Build

```sh
go build -o gopager ./cmd/gopager
```

The binary embeds all SQL migrations. It needs no other files at runtime except an optional plugin directory.

## Configuration Reference

Flags use Go `flag` syntax (`--name=value` or `-name value`). Positional arguments are rejected. Any configuration error ends the process with `INVALID_INPUT` (see [Exit Status and Logging](#exit-status-and-logging)). The former `--profile` flag was removed (deployment profiles are topologies, see [Topology](#topology)); passing it fails with `INVALID_INPUT` (unknown flag).

### Server mode

| Flag / variable | Default | Required | Meaning |
| --- | --- | --- | --- |
| `--roles` | — | always | Comma-separated, non-empty, no duplicates: `api`, `worker` or `api,worker`. |
| `GOPAGER_DATABASE_URL` (env) | — | always | PostgreSQL connection string (URL or keyword/value form understood by pgx). Read only from the environment so it does not appear in process listings or logs. |
| `--listen-address` | — | with `api` | HTTP listen address `host:port`. Host must be non-empty (use `0.0.0.0` or `[::]` to listen on all interfaces); port 1–65535. |
| `--public-origin` | — | with `api` | Public origin browsers use to reach GoPager, `http://` or `https://` scheme, host, optional port, no path other than `/`, no query/fragment/userinfo. Validated whenever set, even for `worker`. See [Security](#security). |
| `--migration-mode` | `check` | — | `check`: start only if the schema is exactly current. `apply`: run pending migrations first. See [Database Migrations](#database-migrations). |
| `--processing-concurrency` | `2` | — | Processing candidates evaluated in parallel per worker process. Must be > 0. |
| `--processing-poll-interval` | `5s` | — | Fallback interval at which a worker looks for due work, as a Go duration (`500ms`, `5s`, `1m`). Must be > 0. New work normally starts immediately through a PostgreSQL notification (channel `gopager_processing_due`, sent by intake and by Processing readmission after a correction); the poll covers lost notifications and work that becomes due by time. |
| `--event-poll-interval` | `5s` | — | Fallback interval at which event consumers in a worker look for new facts, as a Go duration. Must be > 0. New facts normally wake consumers immediately through a PostgreSQL notification (channel `gopager_domain_events`, sent with every fact); the poll covers lost notifications. |
| `--processing-artifact-root` | — | — | Plugin directory. Must exist and be openable; leading/trailing whitespace and an explicitly empty value are rejected. |
| `GOPAGER_PROCESSING_ARTIFACT_ROOT` (env) | — | — | Plugin directory, used only when `--processing-artifact-root` is not given. Same validation. |
| `--login-attempts-per-account` | `10` | — | Login attempts per minute per normalized username. Must be > 0. |
| `--login-attempts-per-source` | `30` | — | Login attempts per minute per client source, across all usernames. The source is the host part of the connection's remote address. Must be > 0. See [Security](#security). |
| `--login-attempts-per-instance` | `300` | — | Login attempts per minute across the whole instance. Must be > 0. |
| `--search-attempts-per-minute` | `60` | — | Message search requests (`GET /messages` pages) per minute per access key, per Principal (all its sessions) or, for the guest, per client source (the host part of the connection's remote address). Must be >= 1. Rejected requests are `429 RATE_LIMITED` and are not counted. The guest source bucket has the proxy caveat of the login source bucket (see [Security](#security)). |
| `--live-replay-window` | `24h` | — | How old a position may be when a Message live stream resumes from it, as a Go duration. Must be > 0. An older position makes the stream answer `resync-required`. Domain events must be retained at least this long (see [Event Log Retention](#event-log-retention)); the stream relies on the log holding every fact after a client's last position. Streams use the same notification channel as event consumers, through one listener per process. |
| `--gateway-heartbeat-interval` | `60s` | — | Interval in which Gateways are expected to send a heartbeat. Must be > 0. Used to validate the contact timeout. |
| `--gateway-contact-timeout` | `180s` | — | Silence after which the worker sets an enabled `active` or `degraded` Gateway to `contact-lost`, compared on the database clock. Must be at least `--gateway-heartbeat-interval`. |
| `--gateway-contact-scan-interval` | `15s` | — | Pause between the worker's contact-loss scans. Must be > 0 and at most `--gateway-contact-timeout`. Detection may lag the timeout by up to this interval. |
| `--delivery-sends-per-minute` | `60` | — | Delivery sends per minute per subscriber, counting background attempt starts, Subscription creation and redelivery requests. Must be >= 1. A background start over the limit is deferred to the next minute without consuming an attempt; an HTTP request over it is `429 RATE_LIMITED`. |
| `--delivery-target-sends-per-minute` | `600` | — | Delivery sends per minute per target instance; a smaller limit declared by the target lowers it. Must be >= 1. Over the limit, starts are deferred like above. |
| `GOPAGER_SECRET_KEY_FILE` (env) | — | — | Path of a file holding the master key of the encrypted Secret store: standard padded Base64 of exactly 32 bytes, at most 1 KiB, surrounding whitespace ignored. Wins over `GOPAGER_SECRET_KEY`. An unreadable, oversized or invalid file is `INVALID_INPUT` at start (the log names only the variable). See [Secrets](#secrets). |
| `GOPAGER_SECRET_KEY` (env) | — | — | The master key itself, same format, used only when `GOPAGER_SECRET_KEY_FILE` is not set. Prefer the file variant so the key is not in the process environment listing. Invalid is `INVALID_INPUT` at start. |
| `GOPAGER_SECRET_FILE_ROOT` (env) | — | — | Directory that `file:` Secret references are relative to. Must exist and be openable; leading/trailing whitespace is rejected. Without it every `file:` Secret is unavailable. |
| `--allow-insecure-db` | `false` | — | Permit a non-local database connection without certificate-verified TLS. See [Database TLS](#database-tls). |

Fixed values (not configurable):

| Setting | Value |
| --- | --- |
| Database pool | 10 open, 5 idle connections, 30 min connection lifetime |
| Per-call / per-transaction timeout | 30 s |
| Readiness re-check interval | 5 s |
| HTTP `ReadHeaderTimeout` | 5 s |
| HTTP `ReadTimeout` (whole request including body) | 30 s |
| HTTP `WriteTimeout` (whole response) | 60 s |
| HTTP `IdleTimeout` (keep-alive connection) | 120 s |
| HTTP `MaxHeaderBytes` | 64 KiB |
| Shutdown deadline | 30 s |

### Bootstrap

| Flag / variable | Default | Required | Meaning |
| --- | --- | --- | --- |
| `GOPAGER_DATABASE_URL` (env) | — | yes | As in server mode. |
| `--username` | — | yes | Initial account name: 1–128 characters, no leading/trailing whitespace, no control characters. |
| `--actions` | — | yes | Comma-separated actions granted with scope `all`. Each must be a catalog action; no wildcards. |
| `--fields` | — | — | Comma-separated readable fields: `payload`, `address`, `unit`, `gateway`, `quality` or `derived`. |
| `--delegable` | `false` | — | Allow the account to delegate the granted actions. |
| `--migration-mode` | `check` | — | `check` or `apply`, as in server mode. |
| `--allow-insecure-db` | `false` | — | As in server mode. |
| stdin | — | yes | Password from a pipe or file (not a terminal). 12–1024 characters, valid UTF-8. One trailing LF is stripped; all other bytes are kept. |

## Roles

| Role | Runs |
| --- | --- |
| `api` | HTTP listener with health endpoints and all `/api/v1` routes (authentication, management, gateway intake and administration, processing). See [HTTP API](http-api.md). |
| `worker` | Processing dispatcher (executes due workflow work), the assignment correction runner, the Gateway contact-loss monitor, the Delivery fan-out consumer (turns new facts into Deliveries) and the Delivery runner (attempts due Deliveries, polling every 5 s besides fact notifications). No listener. Without a configured Delivery target provider every Subscription target is unavailable and no Delivery starts. |

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

`domain_events` (the sequenced fact log) and `consumer_offsets` (one read position per consumer) grow without bound until a retention job prunes them; no such job is implemented yet. Both tables belong in the planned retention configuration. The rule the job must follow: a `domain_events` row is prunable only when its `(tx_id, seq)` is at or below the lowest `(last_tx_id, last_seq)` of every registered consumer **and** it is older than the replay window, so no consumer can be left with a gap and late-registered consumers still find recent history. The replay window is at least `--live-replay-window`, because a live stream resumes from a position up to that old and reads every fact after it; pruning younger facts would silently drop Message updates for reconnecting clients. A `consumer_offsets` row is removed only when its consumer is decommissioned.

Delivery adds its own retention participants, also without a pruning job yet: `deliveries` and `delivery_attempts` (terminal Deliveries no longer hold input; their history and attempt receipts remain until pruned; prune only terminal Deliveries older than 24 h after they ended, so explicit redelivery and the fan-out deduplication of a fact still recorded within the 24 h fan-out window keep working), and `delivery_admissions` (each admitted send deletes the older windows of its own bucket and up to 100 expired windows of any bucket, so buckets of removed subscribers or instances do not stay behind). A fact older than 24 h never creates a Delivery, so the Delivery fan-out needs no older facts; its consumer `delivery-fanout` still follows the offset rule above.

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
| Processing | `workflow.read`, `workflow.create`, `workflow.update`, `workflow.delete`, `workflow.preview`, `workflow.activate`, `workflow.reprocess`, `presentation.manage`, `plugin.configure` |
| Attribute schemas | `attribute-schema.manage` |
| Other | `notice.manage`, `operations.manage`, `saved-view.manage-global`, `audit.read` |

The `*` notation above is shorthand for this table only; `--actions` requires each action spelled out.

## Starting the Server

Minimal combined deployment behind a TLS-terminating proxy:

```sh
export GOPAGER_DATABASE_URL='postgres://gopager:…@db.internal:5432/gopager?sslmode=verify-full'
gopager \
  --roles=api,worker \
  --listen-address=127.0.0.1:8080 \
  --public-origin=https://pager.example.org \
  --processing-artifact-root=/srv/gopager/plugins
```

Local development against a loopback database:

```sh
GOPAGER_DATABASE_URL='postgres://gopager:secret@127.0.0.1:5432/gopager?sslmode=disable' \
  gopager --roles=api,worker --migration-mode=apply \
  --listen-address=127.0.0.1:8080 --public-origin=http://127.0.0.1:8080
```

Startup order: parse and validate configuration → open the database (TLS rule) → compose roles → ping, check write authority (`pg_is_in_recovery()` false and `transaction_read_only` off), check/apply migrations → open the listener and start workers → log `platform started`. Any failure before the last step ends the process with exit 1.

### Backup and restore responsibility

Backup, restore, secure disposal of copies and the handling of keys, secrets and plugin artifacts belong to the operator; GoPager has no backup or restore feature and no detection of a restored copy. An older database copy can bring back revoked credentials, deleted accounts, or work whose external effect happened after the copy was taken, and the application cannot know that later state existed. Account for these risks outside GoPager before starting a restored installation. Normal start-up still enforces schema compatibility, current stored rights and job leases, and honours the uncertainty recorded for attempts that had started. An upgrade never rolls back data written in between.

## Health Endpoints

Served on the `api` listener only, `GET` only (other methods: `405`, `Allow: GET`), no authentication.

| Endpoint | Response | Meaning |
| --- | --- | --- |
| `GET /health/live` | `200 {"live":true}` | The process is serving HTTP. |
| `GET /health/ready` | `200 {"ready":true}` / `503 {"ready":false}` | The process accepts work. |

The process is ready when the database is reachable, is a writable primary, the schema matches the binary exactly, no role has failed and shutdown has not started. Readiness is re-checked every 5 s (connectivity, write authority and schema together); it returns to ready automatically when the database recovers, unless a role has failed.

While not ready, every non-health route returns `503` with error code `AUTHORITY_UNAVAILABLE` and `retryable: true` (error envelope: [HTTP API](http-api.md)).

The responses contain no other details. Roles, migration state (`current` / `incompatible`) and failure code are kept in-process; failure codes appear in logs.

## Failure Handling

| Situation | Behavior |
| --- | --- |
| Database unreachable or demoted at runtime | Readiness becomes false; log `platform readiness check failed code=…`; recovers automatically. |
| Worker runner hits a transient database failure (`STORAGE_UNAVAILABLE`, `AUTHORITY_UNAVAILABLE`, `COMMIT_UNCERTAIN`, `REVISION_CONFLICT`) | The runner keeps running. It logs a warning with the code, the number of consecutive failures and the delay, and retries after a backoff that doubles up to 30 s: the processing dispatcher starts at its 5 s poll interval (`Processing dispatch selection failed; retrying`), the correction runner at 500 ms (`correction runner storage failure; retrying`), the Gateway contact-loss monitor at its scan interval (`Gateway contact monitor storage failure; retrying`). The first successful attempt logs `Processing dispatch selection recovered`, `correction runner recovered` or `Gateway contact monitor recovered`. No restart is needed. |
| A worker runner exits with any other error | Log `platform role stopped code=…`; the process stays not ready permanently, keeps running, and exits 1 at shutdown. Restart the process. |
| HTTP listener fails | Same as a failed worker runner. |

A `worker`-only process has no health endpoint. Monitor its log for `platform role stopped` and restart it, or run it combined with `api` if an HTTP probe is required. Repeated `…; retrying` warnings indicate a database outage the worker is waiting out.

## Exit Status and Logging

| Exit status | Meaning |
| --- | --- |
| 0 | Normal shutdown after `SIGINT`/`SIGTERM`, or successful bootstrap |
| 1 | Startup failure, failed role, or shutdown failure |

Logs are written to stderr by Go's default `log/slog` handler (`2026/11/05 23:54:28 INFO platform started roles="[api worker]"`). Errors are logged by code only; connection strings, credentials, causes and payloads are never logged. On exit 1 the last line is `ERROR GoPager exited code=<CODE>`.

**Correlating a client error with the log.** Every API error response carries a fresh `requestId` (see [HTTP API](http-api.md#errors)). The same response writes one log line, `API error response code=<CODE> status=<HTTP status> requestId=<UUID>`, at `ERROR` level for 5xx and `INFO` level for 4xx. To find out what happened to a failed request, search the log for the `requestId` the client reports. The line carries only these three values: no request, body, credential or private cause. The cause itself is deliberately not logged (see above), so the `code` is the diagnosis; for `STORAGE_UNAVAILABLE` and `AUTHORITY_UNAVAILABLE` also check the platform readiness lines.

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

Log lines in use: `platform started`, `API error response`, `platform readiness check failed`, `platform role stopped`, `GoPager exited`, the worker retry warnings `Processing dispatch selection failed; retrying` and `correction runner storage failure; retrying` and `Gateway contact monitor storage failure; retrying` with their recovery lines `Processing dispatch selection recovered`, `correction runner recovered` and `Gateway contact monitor recovered`, and the warning `operator approved a remote database connection without certificate-verified TLS`.

## Database TLS

| Database host | Required TLS |
| --- | --- |
| Unix socket path, `localhost`, loopback IP | none |
| Any other host (including every fallback host in a multi-host URL) | certificate-verified TLS: `sslmode=verify-ca` or `sslmode=verify-full` |

`sslmode=disable`, `allow`, `prefer` or `require` to a non-local host fails with `INVALID_INPUT` unless `--allow-insecure-db` is set. With the flag, GoPager connects and logs a warning at every start. Use it only on a trusted private network.

## Plugin Directory

`--processing-artifact-root` (or `GOPAGER_PROCESSING_ARTIFACT_ROOT`) names a local directory of plugin manifests and executables. All file access is confined to this directory. Executables must be regular files with an executable bit, at most 256 MiB, and match the SHA-256 digest in their manifest. Plugin processes are started on demand (verified against the pinned digest at every start), reused by later runs of the same instance revision, stopped after 5 minutes without calls, and receive only `PATH`, `HOME`, `LANG`, `LC_ALL`, `TZ` and `TMPDIR` from GoPager's environment — never `GOPAGER_DATABASE_URL`.

Without a plugin directory GoPager runs normally; creating, inspecting or executing plugin instances fails with `AUTHORITY_UNAVAILABLE`, and no Delivery plugin is available as a Subscription target. Manifest format and installation: [Plugins](plugins.md); configuring instances: [Workflows](workflows.md) (Workflow steps) and [HTTP API](http-api.md#processing-plugin-instances) (Delivery targets).

**Bundled plugins.** The Docker image ships `pushover` and `webhook` 1.0.0 with ready manifests (artifact digests filled in) under `/usr/lib/gopager/plugins/<name>/1.0.0/`, read-only for the `gopager` user. Outside Docker, `make plugins` builds the same layout into `build/plugins/`, and you copy it to the plugin directory of every node. Their properties and result codes: [Plugins: First-party Delivery plugins](plugins.md#9-first-party-delivery-plugins).

**Several nodes.** Every `api` and `worker` process uses its own directory. Provision the same contents on every `worker` node: a worker claims the Deliveries of an instance immediately only if the instance's pinned manifest and executable verify from its own plugin directory (re-checked every 5 seconds, without starting the plugin). A worker without the plugin that fans out a fact still creates the Delivery due at once (outcome code `dependency-unavailable`), so a capable worker claims it without delay. A Delivery that no capable worker claimed within 60 seconds after it became due may be claimed by any worker. A worker that cannot load the artifact defers it for 60 seconds without consuming an attempt; the Delivery keeps its state and shows the outcome code `dependency-unavailable`. A Delivery in that state that stays unserved ends as `expired` when its lifetime runs out. If you see `dependency-unavailable` on Deliveries, check that the pinned plugin files are present on the workers, that the digest matches and that the executable bit is set. The plugin status in the instance view (`running`, `lastStartedAt`, `lastFailureCode`) is kept in memory per node and resets when that node restarts, so compare nodes rather than expecting one cluster-wide answer.

Plugin processes are keyed by instance revision. The node that handles an instance update (a new revision or disabling it) stops the processes of older revisions at once; an attempt running in one of them fails and the Delivery records it as `uncertain`. Other nodes start new attempts in a process of the new revision and let the old one stop after its 5-minute idle timeout.

## Graceful Shutdown

On `SIGINT` or `SIGTERM`:

1. Readiness becomes false; new application requests get `503 AUTHORITY_UNAVAILABLE`.
2. Workers are cancelled; the HTTP server stops accepting connections and waits for in-flight requests.
3. Running work and transactions are drained, then the database pool and plugin processes are closed. Closing a plugin process asks it to shut down, waits up to 2 s and then kills it. A Delivery attempt still running in a plugin at that moment ends as `uncertain` (the provider may have been contacted); it is never sent again automatically.

The total deadline is 30 s. Exceeding it, or any role failure during the run, yields exit 1. Give the process at least 30 s between `SIGTERM` and `SIGKILL` (e.g. Docker `--stop-timeout=35`, Kubernetes `terminationGracePeriodSeconds: 35`). Processing work claimed but not completed before shutdown is recovered by a worker after restart; completed results are not recomputed.

## Secrets

Plugin instances can hold Secrets such as API tokens and signing keys ([Plugins: Secrets](plugins.md#secrets)). They live in one of three places:

- **Encrypted store (`secret:`).** A value an administrator or a User enters through the API is sealed with AES-256-GCM into the database table `configuration_secrets`. Each row has a random 96-bit nonce and is bound to its row ID, instance and owner, so a row cannot be moved to another instance or user. Rows are immutable: a changed value is a new row, hence a new Secret version.
- **File (`file:<relative path>`).** A regular file below `GOPAGER_SECRET_FILE_ROOT`, in a directory named after the plugin ID (`<root>/<plugin ID>/…`), at most 64 KiB, one trailing newline removed. Use it for mounted Kubernetes or Docker Secrets, or files written by tools such as External Secrets Operator or Vault Agent. The file is read at every attempt, so a rotated file takes effect without a restart; a new content digest is a new Secret version.
- **Environment variable (`env:GOPAGER_PLUGIN_SECRET_<ID>__<NAME>`).** A variable of the GoPager process, readable only if its name starts with `GOPAGER_PLUGIN_SECRET_<ID>__` for the instance's plugin (so an instance cannot route `GOPAGER_DATABASE_URL` or the master key into a plugin). Changing it needs a restart of the process.

Name variables and lay out the file root per plugin: plugin `pushover` reads `env:GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN` and `file:pushover/token`; plugin `my-hook` reads `GOPAGER_PLUGIN_SECRET_MY_HOOK__KEY` (`<ID>` is the plugin ID in upper case with `-` as `_`). A reference to another plugin's name or directory is refused, so an administrator with `plugin.configure` cannot route one plugin's Secret into another plugin's instance. Delivery plugin IDs are canonical (lowercase letters and digits in groups joined by single hyphens), so every plugin has its own variable prefix and directory.

`file:` and `env:` Secrets are placed by you and can only be used by administrative values; a User's personal Secret is always a stored value.

**Master key.** The encrypted store needs a 32-byte key, supplied as standard padded Base64 in `GOPAGER_SECRET_KEY_FILE` (a file; preferred, and it wins) or `GOPAGER_SECRET_KEY`:

```sh
openssl rand -base64 32 > /run/secrets/gopager_secret_key     # 44 characters
GOPAGER_SECRET_KEY_FILE=/run/secrets/gopager_secret_key
```

Give every `api` and `worker` process the same key. The key is read once at start and kept only inside the cipher, which prints as `SecretCipher(redacted)` in every textual form. Each row records a key ID (`k1:` and the first 16 hex digits of the key's SHA-256), which is not secret and does not reveal the key. Without a key the process starts normally, but writing a stored Secret is refused with `503 AUTHORITY_UNAVAILABLE` and every Delivery attempt of an instance whose configuration holds any Secret reference (of any source) ends as `transient` with the code `configuration-unavailable` until the key is configured.

**Rotation limits.** There is no re-encryption and only one key is active. Replacing the key makes every stored row unreadable (its key ID no longer matches): the affected instances answer `configuration-unavailable` until each stored Secret, administrative and personal, is entered again; entering it creates a row sealed with the new key. Users must re-enter their own personal Secrets, because only the owner can write them. Rows of the old key stay in the database. If you lose the key you lose the stored Secrets; the key and the database backup must be kept (and protected) separately: a database dump alone cannot be decrypted. To rotate without re-entering values, use `file:` or `env:` Secrets for administrative values and rotate the file or variable instead.

**Never logged.** Secret values and the key do not appear in logs, errors, API responses or command fingerprints (a replayed write is fingerprinted with a keyed digest only). Views show a Secret only as `secret`, `configured`, `redacted` and its source (`stored`, `file` or `env`), never the value, file path or variable name. Startup errors about the key or the file root name only the variable. The plaintext exists only in the request to the plugin during an attempt and in the plugin process itself; the server clears it when the call ends. A trusted plugin that deliberately leaks a Secret cannot be stopped: plugins are trusted code ([Plugins: Runtime model](plugins.md#1-runtime-model)). A plugin receipt that contains a Secret of the attempt is dropped.

**Administrators are trusted with an instance's Secrets.** Whoever holds `plugin.configure` can change an instance's non-Secret settings, including its destination (Webhook `url`, Pushover `endpoint`), while its stored Secrets carry over; the plugin then sends them to the new destination, also when another administrator entered them. GoPager never shows a stored Secret, but it does not stop an administrator from redirecting one. Grant `plugin.configure` only to people you trust with every plugin Secret, and replace a Secret after removing such a grant.

## Security

- **Public origin.** `--public-origin` is the single source for cookie and CSRF origin policy. `Host` and `X-Forwarded-*` headers are ignored. Set it to the exact origin users type into the browser (e.g. `https://pager.example.org`); default ports `80`/`443` are normalized away.
- **Session cookie.** `gopager_session`, host-only, `Path=/`, `HttpOnly`, `SameSite=Lax`, `Secure` when the public origin is `https://`. Behind an HTTPS proxy, use the `https://` origin even though GoPager itself listens on HTTP.
- **Browser requests.** Login and cookie-authenticated state-changing requests require a single `Origin` header equal to the public origin; state-changing requests also require `X-CSRF-Token`. API keys use `Authorization: Bearer …` and are not subject to origin checks. Details: [HTTP API](http-api.md).
- **Login rate limits.** Three per-minute buckets, checked in this order: per instance (`--login-attempts-per-instance`), per client source (`--login-attempts-per-source`) and per normalized username (`--login-attempts-per-account`). The source bucket stops one client from trying many usernames and from exhausting the instance bucket, which would lock every operator out. The account bucket is not keyed by address, so an attacker cannot block a user's login from the user's own address by guessing from another. Rejections return `RATE_LIMITED` with a retry interval; rejected attempts are not counted again. Only hashes of usernames and source addresses are stored.
- **Client source behind a reverse proxy.** The source is the host part of the TCP peer address; `X-Forwarded-For` and similar headers are ignored because a client can set them to pick its own bucket. Behind a proxy that terminates the connection, every client therefore shares the proxy's address and one source bucket. Either configure the proxy to preserve the client address at connection level, or raise `--login-attempts-per-source` (up to the instance limit) to what all users behind the proxy need. Trusted-proxy header parsing is not implemented.
- **HTTP timeouts.** The server bounds header reading (5 s), the whole request (30 s), the whole response (60 s) and idle keep-alive connections (120 s), and rejects headers over 64 KiB. Slow clients are disconnected. A future streaming response (server-sent events) must extend its own write deadline per request instead of raising the global value.
- **Authentication before body.** Gateway intake (`POST /api/v1/transmissions`, `POST /api/v1/gateway/state`) verifies the bearer credential before it reads the request body, so unauthenticated clients cannot make the server buffer payloads.
- **Transport.** GoPager serves plain HTTP. Restrict `--listen-address` to loopback or a private interface and terminate TLS at a proxy.
- **Secrets.** Provide `GOPAGER_DATABASE_URL` from a secret store; never pass passwords as arguments. Bootstrap reads the password from stdin only. Handle the plugin Secret key and files the same way ([Secrets](#secrets)).
- **Database role.** Day-to-day processes can use `check` mode; reserve DDL privileges for the process or job that runs `apply`, if your setup separates roles.

Component overview and data flow: [Architecture](architecture.md). Building and testing: [Development](development.md).
