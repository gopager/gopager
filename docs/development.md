# Development

Contribution and documentation rules: [CONTRIBUTING.md](../CONTRIBUTING.md). Running a deployment: [Operations](operations.md).

## Prerequisites

| Tool | Needed for |
| --- | --- |
| Go, version from [go.mod](../go.mod) (`go 1.27.3`) | everything |
| Make | all `make` targets below |
| Network access | `make lint` and `make vuln` fetch their pinned tools through `go run` the first time |
| Docker (running daemon, pull access to `postgres:18.0-alpine`) | integration tests, unless an external test database is supplied |

All other tools are pinned and run through `go tool` / `go run`; nothing needs a global install: the Ent code generator and Ginkgo CLI via `go.mod`, golangci-lint (`v2.15.0`) and govulncheck (`v1.9.0`) via the version variables at the top of the [Makefile](../Makefile). Change a pin there only.

## Repository Layout

One Go module, `github.com/gopager/gopager`.

| Path | Contents |
| --- | --- |
| `cmd/gopager/` | Executable: flag parsing (`main.go`), `bootstrap` subcommand, composition of stores and services (`compose.go` builds the shared set once; `processing_runtime.go`, `processing_worker.go`, `processing_adapters.go`, `processing_preparation.go`, `scopes.go` compose Processing and its ports) |
| `internal/httpapi/` | HTTP adapters for the application routes: `management_*.go` (one file per resource), `ingest.go`, `gateway_credentials.go`, `processing*.go`, and the shared `route.go` (route helper with pre-authentication), `json.go`, `request.go`, `errors.go` |
| `api/openapi/v1.yaml` | HTTP API schema |
| `api/plugin/v2/plugin.proto` | Plugin gRPC service definition |
| `pluginsdk/` | Public Go SDK for plugin executables (handshake, `Serve`, gRPC client/server) |
| `docs/` | Product and code documentation |
| `internal/platform/` | Store-neutral foundation: `Config`/roles, `ErrorCode`/`Failure`, `Clock`, `TransactionContext`/`TxFunc`, health handlers, process lifecycle (`StartWithAPI`, readiness, shutdown) |
| `internal/platform/postgres/` | PostgreSQL implementation of `platform.Store` and `platform.Clock`: pgx pool, TLS rule, Goose migration runner, transactions, SQLSTATE → error code mapping |
| `internal/authz/` | Action catalog, grants, scopes, field permissions, authorization decisions |
| `internal/identity/` | Accounts, credentials, password hashing (Argon2id), sessions, login rate limits, bootstrap, HTTP auth transport (cookie/origin/CSRF) and `/api/v1/auth/` handler |
| `internal/units/` (package `units`) | Units, organizations, Unit collections, Unit templates |
| `internal/assignments/` | Address rules, Unit assignment of transmission targets, historical correction runner |
| `internal/ingest/` | Gateway transmission intake, Gateway/Network administration and credentials |
| `internal/protocols/` | Protocol and transmission payload validation |
| `internal/messages/` | Grouping of transmissions into messages |
| `internal/commands/` | Administrative command receipts |
| `internal/events/` | Domain fact types appended to the event store |
| `internal/processing/` | Workflow model, evaluation, budgets, CEL predicates, regex, plugin host and installed artifacts |
| `internal/processing/management/` | Workflow and instance management commands, reads, retry, preview |
| `internal/processing/worker/` | Dispatcher and worker service executing due processing work |
| `internal/processing/publication/` | Publication of processing results to the source owner |
| `internal/processing/execution/` | Identity binding for installed plugin execution |
| `internal/processing/synthetic/` | Synthetic transmissions used as preview sources |
| `internal/processing/testdata/` | Plugin fixture sources (`command/`, `preview/`) and instance manifests |
| `internal/persistence/migrations/` | Embedded Goose SQL migrations (`migrations.Files`) |
| `internal/persistence/ent/schema/` | Ent schema definitions (hand-written) |
| `internal/persistence/ent/` (top level and one subpackage per entity, `enttest/`, `migrate/`, `predicate/`, `privacy/`, `runtime/`, `hook/`) | Ent-generated code — do not edit |
| `internal/persistence/ent/generate.go` | `go:generate` directive for Ent |
| `internal/persistence/ent/guard/` | Ent privacy policy: deny unless an adapter bound an access decision |
| `internal/persistence/ent/permissions/` | Translation of authz decisions into Ent predicates |
| `internal/persistence/ent/borrowed/` | Ent driver that runs inside an existing platform transaction; hosts most store integration tests |
| `internal/persistence/ent/*store/` | Repository adapters per domain: `identitystore`, `unitstore`, `assignmentstore`, `ingeststore`, `processingstore`, `eventstore` (sequenced domain event log), `commandstore` |
| `internal/testutil/postgres/` | Test PostgreSQL lifecycle (`Run(m)`) |
| `internal/testutil/pluginfixture/` | Builds plugin fixtures for tests (`Binary(t, name)`) |

Architecture and data flow: [Architecture](architecture.md).

## Build

```sh
go build ./...                          # all packages
go build -o gopager ./cmd/gopager       # the executable
```

## Formatting and Linting

```sh
make                    # same as make lint
make fmt                # gofmt -w cmd internal
make fmt-check          # fail and list files that need gofmt
make vet                # go vet ./...
make golangci           # golangci-lint run ./... (pinned version)
make lint               # fmt-check, vet, golangci
make test               # quick tier: go test ./...
make test-integration   # GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=20m ./...
make generate-check     # go generate ./..., then fail if the Ent output differs from git
make vuln               # govulncheck ./... (pinned version, needs network)
```

`gofmt` covers `cmd/` and `internal/` (including generated Ent code and tests). Only `test-integration` needs Docker.

[.golangci.yml](../.golangci.yml) enables a conservative set: `govet`, `staticcheck`, `errcheck`, `ineffassign`, `unused`, `misspell`. Generated code is skipped by golangci-lint's generated-file detection. The config uses the golangci-lint v2 format (migrated with `golangci-lint migrate`; the `std-error-handling`, `comments`, `common-false-positives` and `legacy` exclusion presets keep the v1 default exclusions). In v2, `staticcheck` also covers the former `gosimple` and `stylecheck` checks (for example ST1005 lower-case error strings). Three known findings in processing code are excluded by path and text (two ineffectual assignments in `cmd/gopager/processing_worker.go` and `internal/processing/workflow_test.go`, and an overwritten argument in `internal/processing/workflow.go`); delete the matching `linters.exclusions.rules` entry when you fix one. Do not add new exclusions to hide findings in code you change.

`make generate-check` compares only the Ent output (`internal/persistence/ent`, not its hand-written `schema/`) with git, so commit or stage regenerated files before expecting it to pass locally; it also fails on generated files that git does not track.

`make vuln` reports no vulnerability reachable from the code (Go 1.27.3, `klauspost/compress` v1.20.2, `moby/go-archive` v0.3.0) and is a required CI check. golangci-lint v1 cannot read Go 1.27 export data, which is why the pin is v2.

### Continuous integration

[.github/workflows/ci.yml](../.github/workflows/ci.yml) runs on pull requests and pushes to `main`. Job `static`: `make fmt-check`, `make vet`, `make golangci`, `make test`, `make generate-check`. Job `integration` (after `static`): `make test-integration` on the runner's Docker daemon. Job `vulnerabilities` runs `make vuln` as non-blocking information. The Go version comes from `go.mod`.

## Tests

### Tiers

| Tier | Command | Needs |
| --- | --- | --- |
| Quick | `make test` (`go test ./...`) | Go only. Database tests are skipped with `PostgreSQL integration disabled; run with GOPAGER_INTEGRATION=1`. |
| Full, owned containers | `make test-integration` (`GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=20m ./...`) | Docker |
| Full, external database | `GOPAGER_TEST_DATABASE_URL='postgres://…' go test -race -count=1 ./...` | A disposable PostgreSQL server with a role that has `CREATEDB` |

Environment variables:

| Variable | Values | Effect |
| --- | --- | --- |
| `GOPAGER_INTEGRATION` | unset or `1`; anything else fails the run | `1` starts one PostgreSQL container per participating test package |
| `GOPAGER_TEST_DATABASE_URL` | DSN | Use this server instead of a container; setting it alone also enables database tests. Tests create and drop databases next to the one named in the DSN, so use a disposable server, never a production one. |

Use `-count=1` for integration runs so Go's test cache does not report stale results. In a requested integration run, a missing Docker daemon or unreachable database fails the run; it is not skipped.

Packages with database tests (their `TestMain` calls `testpostgres.Run`): `cmd/gopager`, `internal/platform/postgres`, `internal/persistence/ent/borrowed`. Each test gets its own database (random name), dropped with `WITH (FORCE)` on cleanup, so tests share nothing: schemas, `LISTEN/NOTIFY` channels and advisory locks are all database-scoped. `internal/testutil/postgres` provides `EmptyDatabase(t)` (cloned from `template0`) and `NewTemplate(name, build).Database(t)`, which builds a template database once per process (for example with all migrations applied), marks it `IS_TEMPLATE` with connections disallowed, and clones it with `CREATE DATABASE … TEMPLATE`. Clones of one template are serialized because PostgreSQL refuses to copy a template that other sessions are using. `Run` drops the process's templates after the tests. The owned container is deliberately non-durable (`fsync=off`, `synchronous_commit=off`, `full_page_writes=off`, data directory on tmpfs, `max_connections=500`); it is a throwaway fixture and says nothing about crash safety. An external server may have any durability settings.

Run a subset:

```sh
GOPAGER_INTEGRATION=1 go test -count=1 ./internal/platform/postgres ./cmd/gopager
go test -run TestCredentials ./internal/identity
```

### Frameworks

| Framework | Use |
| --- | --- |
| `testing`, `net/http/httptest`, `testing/fstest` | Default for unit, table and HTTP handler tests |
| Ginkgo v2 + Gomega | Scenario/conformance suites (`internal/platform/postgres/migrations_behavior_test.go`, `internal/identity/credentials_test.go`, `internal/events/identity_test.go`). They run under plain `go test`. Labels in use: `migrations`, `unit`, `integration`. |
| Testcontainers-Go (postgres module) | Disposable PostgreSQL, used only from `internal/testutil/postgres` |

The Ginkgo CLI is available from the module pin for label filtering and parallel processes:

```sh
go run github.com/onsi/ginkgo/v2/ginkgo --label-filter=unit --fail-on-empty ./internal/platform/postgres
GOPAGER_INTEGRATION=1 go run github.com/onsi/ginkgo/v2/ginkgo --procs=2 --label-filter=migrations --fail-on-empty ./internal/platform/postgres
```

Suite conventions: qualified imports (`ginkgo.Describe`), no focused/pending specs, no automatic retries, per-spec cleanup, helpers take `testing.TB` and are passed `ginkgo.GinkgoTB()`. Do not add Testify.

### Process tests

`cmd/gopager/process_test.go` re-executes the test binary as a real `gopager` process (selected by an environment variable the test sets for the child; `TestMain` in `testmain_test.go` skips fixture setup in that child) to verify startup, health endpoints and signal shutdown.

### Plugin fixtures

Plugin executables for tests are built from source, never committed:

- Sources: `internal/processing/testdata/<name>/main.go` (`command`, `preview`), built on the `pluginsdk`.
- `pluginfixture.Binary(t, "<name>")` runs `go build -trimpath` (`CGO_ENABLED=0`) once per test process into a temp directory and returns the bytes. Packages using it call `pluginfixture.Cleanup()` in `TestMain` after `m.Run()`.
- Manifests: `internal/processing/testdata/instance-*.json`. Their `artifact.digest` is a placeholder; tests copy the binary to a temp plugin directory as `plugin` (mode `0755`) and rewrite the digest with the SHA-256 of the freshly built binary (see `pluginFixtureRoot` in `cmd/gopager/processing_management_test.go`).

To add a fixture, create `internal/processing/testdata/<name>/main.go` with `package main` calling `pluginsdk.Serve`, then call `pluginfixture.Binary(t, "<name>")`. Plugin authoring: [Plugins](plugins.md).

## Code Generation

Ent is the only code generator. After changing `internal/persistence/ent/schema/`:

```sh
go generate ./internal/persistence/ent
```

This runs `go tool ent generate --feature privacy,entql,sql/modifier,sql/lock ./schema`. Commit the generated files together with the schema change. Generated code is never edited by hand.

## Database Schema and Migrations

The SQL migrations are the only source of DDL. Ent's generated `migrate` package and auto-migration are not used at runtime; Ent only generates the typed query client.

| Artifact | Role |
| --- | --- |
| `internal/persistence/migrations/NNNNN_<description>.sql` | Creates and changes tables, indexes, constraints, functions |
| `internal/persistence/ent/schema/*.go` | Describes the same tables for the query client; `entsql.Annotation{Table: "…"}` names the SQL table |

Adding a migration:

1. Create `internal/persistence/migrations/<next version>_<snake_case_description>.sql`, zero-padded to 5 digits like the existing files (`00011_…`). Versions must be positive and strictly increasing; gaps are allowed.
2. Start with `-- +goose Up`. Use `-- +goose StatementBegin` / `-- +goose StatementEnd` around statements containing semicolons (functions, `DO` blocks). A `-- +goose Down` section is optional and never executed by GoPager.
3. Do not use `-- +goose NO TRANSACTION` or `-- +goose ENVSUB`; they are rejected. Each file runs in one transaction together with its ledger row.
4. The directory must contain only flat `.sql` files; it is embedded via `//go:embed *.sql` in `migrations.go`, so no registration step is needed.
5. Never change the effect of an already released migration; add a corrective migration instead. Applied migrations are identified by version only and are not re-run when their content changes.
6. Update the Ent schema to match, run `go generate ./internal/persistence/ent`, and add a guard policy (`func (X) Policy() ent.Policy { return guard.Policy("X") }`) to every new entity.
7. Run the integration tests: the store tests in `internal/persistence/ent/borrowed` apply the real migration collection to a fresh schema and execute the Ent queries against it, which catches schema/client drift.

Runtime check/apply semantics: [Operations](operations.md#database-migrations).

## Code Conventions

### Errors

- Platform-facing errors are `*platform.Failure` created with `platform.NewFailure(code, cause)`. Codes: `INVALID_INPUT`, `LOGIN_REQUIRED`, `FORBIDDEN`, `NOT_FOUND`, `REVISION_CONFLICT`, `IDEMPOTENCY_CONFLICT`, `INCOMPATIBLE_VERSION`, `RESOURCE_LIMIT`, `STORAGE_UNAVAILABLE`, `AUTHORITY_UNAVAILABLE`, `COMMIT_UNCERTAIN`, `INTERNAL_ERROR`. `platform.NewPermanentFailure` is the same with `Retryable()` forced to false.
- `Failure.Error()` returns only a fixed English fallback; the cause is reachable through `errors.Unwrap`/`errors.Is` but never printed. `LogValue` logs code and fallback only. Use `platform.FailureCode(err)` for logging; never log causes, DSNs, secrets or payloads.
- `Retryable()` is true for `STORAGE_UNAVAILABLE`, `AUTHORITY_UNAVAILABLE`, `REVISION_CONFLICT`. `COMMIT_UNCERTAIN` must not be retried blindly; callers look up the operation's receipt.
- Domain packages define sentinel errors (`authz.ErrInvalidInput`, `authz.ErrForbidden`, `authz.ErrNotFound`, `authz.ErrRevisionConflict`, `identity.ErrInvalidInput`, `identity.ErrLoginRequired`, `commands.ErrIdempotencyConflict`). Those listed are `*platform.Failure` values (a nil cause), so their code survives a transaction boundary and `errors.Is` still matches. Define new cross-layer sentinels the same way. HTTP adapters map codes to statuses with `platform.HTTPStatus`.
- PostgreSQL errors are classified centrally in `internal/platform/postgres` (`40001`/`40P01` → retryable `REVISION_CONFLICT`, `23505` → non-retryable `REVISION_CONFLICT`, `25006` → `AUTHORITY_UNAVAILABLE`, `22003`/`53100`/`53200`/`53300`/`53400` → `RESOURCE_LIMIT`, others → `STORAGE_UNAVAILABLE`). Any callback error that already is a `*platform.Failure` keeps its code.
- Every API error response logs `code`, `status` and `requestId` through `platform.LogErrorResponse` (never the cause or request data); use it in any new error writer.

### Transactions

- Services receive a transaction runner (`WithinTransaction(ctx, func(platform.TransactionContext) error) error`), not a database handle. All repositories called inside one callback share that transaction.
- The PostgreSQL store runs `READ COMMITTED`, verifies write authority at transaction start, compares the applied schema version with the one it started with, bounds every transaction to 30 s, rejects nested transactions and never retries an uncertain commit. A callback that fails with `40001` or `40P01` (rolled back by the server) is rerun once in a new transaction, so callbacks must be safe to run twice: keep side effects inside the transaction and reset captured outer variables at the start of the callback.
- Repository adapters obtain the SQL executor with `store.Resolve(tx)` (or an Ent client via `borrowed.New(store, tx)`); a `TransactionContext` is invalid after its callback returns.
- No plugin calls, provider I/O or long computation inside a write transaction.

### Time

- Do not call `time.Now()` for persisted or authorization-relevant timestamps (local monotonic measurements such as evaluation budgets are fine). Take a `platform.Clock` and call `clock.Now(ctx, tx)` inside the transaction; the PostgreSQL implementation returns the primary's `clock_timestamp()` in UTC, truncated to microseconds.
- Tests inject a fixed clock (e.g. a `struct{ now time.Time }` implementing `Now`).

### Package boundaries

- Domain packages (`authz`, `identity`, `units`, `assignments`, `ingest`, `protocols`, `messages`, `commands`, `events`, `processing/...`) depend only on `internal/platform` and other domain packages, never on `internal/persistence/...`, `internal/platform/postgres`, pgx or Ent. They declare the repository interfaces they need.
- Persistence adapters in `internal/persistence/ent/*store` implement those interfaces. All Ent queries go through the guard privacy policy; adapters bind an access decision with `guard.Bind`. Privacy overrides (`privacy.DecisionContext`) are rejected by the borrowed driver.
- `cmd/gopager` composes stores and services once (`composeCore`) and registers the routes of `internal/httpapi`; it contains no domain rules and no HTTP handlers.
- `internal/httpapi` holds all HTTP adapters for `/api/v1` application routes (the auth handler stays in `internal/identity`). It imports domain packages only, never a store.
- Services that run work under a caller's authority share `identity.WithAuthority`; a service that serializes its writes passes `identity.LockMutations` instead of copying the transaction wrapper.
- Configuration is parsed only in `cmd/gopager`; library packages never read flags or environment variables.
- `internal/testutil/...` is imported only from tests.
- Test-only inputs live in the owning package's `testdata/`; there is no global test tree.
- Every exported identifier has a doc comment; `go doc ./internal/<package>` is the reference for identifier-level behavior.
