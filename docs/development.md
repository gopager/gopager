# Development

Contribution and documentation rules: [CONTRIBUTING.md](../CONTRIBUTING.md). Running a deployment: [Operations](operations.md).

## Prerequisites

| Tool | Needed for |
| --- | --- |
| Go, version from [go.mod](../go.mod) (`go 1.27.4`) | everything |
| Make | all `make` targets below |
| Network access | the first run of a tool downloads its pinned module; `make vuln` also queries the vulnerability database |
| Docker (running daemon, pull access to `postgres:18.6-alpine`) | integration tests, unless an external test database is supplied |
| Node.js ≥ 26.13.1 with npm | the commit message hook: run `npm ci` once per clone |

`npm ci` installs the repository tooling pinned in [package.json](../package.json) and [package-lock.json](../package-lock.json): [commitlint](https://commitlint.js.org/) and [lefthook](https://lefthook.dev/), whose install step registers the git hooks from [lefthook.yml](../lefthook.yml) (it skips that when `CI` is set). The `commit-msg` hook rejects messages that break the [commit message rules](../CONTRIBUTING.md#commit-messages). Git worktrees below the clone, such as `.claude/worktrees/`, share the hooks and find the tooling in the clone's `node_modules`.

All other tools are pinned and run through `go tool`, offline once downloaded; nothing needs a global install. The Ent code generator, the Ginkgo CLI and gotestsum are `tool` entries in [go.mod](../go.mod). golangci-lint (`v2.15.0`) and govulncheck (`v1.9.0`) each have a dedicated module file ([golangci-lint.mod](../golangci-lint.mod), [govulncheck.mod](../govulncheck.mod)), as golangci-lint's install docs advise, so their dependencies cannot change the application's. Bump one with `go get -tool -modfile=<tool>.mod <module>@<version>`; never edit the dependencies in those files by hand.

## Repository Layout

One Go module, `github.com/gopager/gopager`.

| Path | Contents |
| --- | --- |
| `cmd/gopager/` | Executable: flag parsing (`main.go`), `bootstrap` subcommand, composition of stores and services (`compose.go` builds the shared set once; `processing_runtime.go`, `processing_worker.go`, `processing_adapters.go`, `processing_preparation.go`, `scopes.go` compose Processing and its ports; `query_runtime.go` composes the Message read side) |
| `internal/httpapi/` | HTTP adapters for the application routes: `management_*.go` (one file per resource), `messages.go`, `ingest.go`, `gateway_credentials.go`, `processing*.go`, and the shared `route.go` (route helper with pre-authentication), `json.go`, `request.go`, `errors.go` |
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
| `internal/delivery/` | Subscriptions, Delivery fan-out consumer, materialization, Delivery runner, sending admission, Delivery management, the Core target port and built-in layouts |
| `internal/processing/presentation/` | Layout definitions with immutable versions, assigned to Agencies, and the layout resolver |
| `internal/attributes/` | Attribute schemas: model, validation, compatibility rules, service and ports |
| `internal/readmodel/` | Message feed, detail and filter-option service, filters, page tokens, projections and the ports they need |
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
| `internal/persistence/ent/deliverystore/` | PostgreSQL implementation of the Delivery store in raw SQL: Subscriptions, Deliveries with fenced claims, attempts, admission windows and Unit parameters |
| `internal/persistence/ent/layoutstore/` | Layout definitions, versions and Agencies in raw SQL |
| `internal/persistence/ent/attributestore/` | Attribute schemas, Agencies and the catalog revision in raw SQL |
| `internal/persistence/ent/agencyscope/` | Translation of an `authz.AgencyVisibility` into a SQL condition |
| `internal/persistence/ent/readstore/` | PostgreSQL implementation of the `readmodel` store in raw SQL: eligibility, filters, text search, keyset paging and the page token key |
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
make docs-check         # Markdown links, heading anchors and configured ID references (tools/doccheck)
make lint               # fmt-check, vet, golangci, docs-check
make test               # quick tier: go test ./...
make test-integration   # all tests incl. PostgreSQL, shuffled, with JUnit and JSON reports (gotestsum, pinned)
make test-race          # test-integration with the race detector (the CI tier)
make test-slowest       # tests over 1 s from the last test-integration / test-race run
make test-stress        # STRESS_COUNT (default 5) shuffled race runs, to hunt flaky tests
make generate-check     # go generate ./..., then fail if the Ent output differs from git
make vuln               # govulncheck ./... (pinned version, needs network)
```

`gofmt` covers `cmd/` and `internal/` (including generated Ent code and tests). Only the `test-integration`, `test-race` and `test-stress` targets need Docker.

`make docs-check` runs `go run ./tools/doccheck`. It checks every Markdown file for relative links and heading anchors. Further rules come from `.doccheck.json` files, which the tool discovers anywhere in the scanned tree; each declares, relative to its own directory, families of numbered references with the register, file names or headings that define the valid numbers, and a list of accepted findings with a reason. The schema is documented on the `config` type in `tools/doccheck/config.go`; a configuration error exits with status 2. Without a configuration file only links and anchors are checked. The rules for the development documents live in `development/.doccheck.json` and disappear with that directory. An ignore entry that no longer matches is reported as `stale-ignore`, so the list cannot hide new problems.

[.golangci.yml](../.golangci.yml) enables a conservative set: `govet`, `staticcheck`, `errcheck`, `ineffassign`, `unused`, `misspell`. Generated code is skipped by golangci-lint's generated-file detection. The config uses the golangci-lint v2 format (migrated with `golangci-lint migrate`; the `std-error-handling`, `comments`, `common-false-positives` and `legacy` exclusion presets keep the v1 default exclusions). In v2, `staticcheck` also covers the former `gosimple` and `stylecheck` checks (for example ST1005 lower-case error strings). Three known findings in processing code are excluded by path and text (two ineffectual assignments in `cmd/gopager/processing_worker.go` and `internal/processing/workflow_test.go`, and an overwritten argument in `internal/processing/workflow.go`); delete the matching `linters.exclusions.rules` entry when you fix one. Do not add new exclusions to hide findings in code you change.

`make generate-check` compares only the Ent output (`internal/persistence/ent`, not its hand-written `schema/`) with git, so commit or stage regenerated files before expecting it to pass locally; it also fails on generated files that git does not track.

`make vuln` reports no vulnerability in the code (Go 1.27.4, the latest module versions) and is a required CI check. golangci-lint v1 cannot read Go 1.27 export data, which is why the pin is v2.

### Continuous integration

[.github/workflows/ci.yml](../.github/workflows/ci.yml) runs on pull requests and pushes to `main`. Job `static`: `make fmt-check`, `make vet`, `make golangci`, `make test`, `make generate-check`. Job `integration` (in parallel with `static`, 15 minute limit): `make test-race` on the runner's Docker daemon, then it uploads `build/test-reports` (`junit.xml`, `test.json`) as the `test-reports` artifact and appends `make test-slowest` to the job summary, both even when tests fail. Job `image` builds the container image from `deploy/compose/Dockerfile` without pushing it. Job `vulnerabilities` runs `make vuln` as non-blocking information. The Go version comes from `go.mod`.

[.github/workflows/commits.yml](../.github/workflows/commits.yml) checks the pull request title with commitlint on every title edit and push, because the title becomes the squash commit header on `main`. It uses [commitlint.pr-title.config.mjs](../commitlint.pr-title.config.mjs), the shared rules without commitlint's default ignores for git-generated messages.

The repository ruleset `main` makes changes to the default branch go through pull requests, allows squash merges only, blocks force pushes and deletion, and requires the checks `Static checks and quick tests`, `Integration tests`, `Container image` and `Conventional pull request title` (`Vulnerability scan` stays informational). A pull request must be up to date with `main` before it merges, so the squash commit is exactly the tree the checks passed on. Nobody can bypass the ruleset; in an emergency an admin disables it temporarily.

### Releases

[release-please](https://github.com/googleapis/release-please) ([.github/workflows/release.yml](../.github/workflows/release.yml), configured in [release-please-config.json](../release-please-config.json) and [.release-please-manifest.json](../.release-please-manifest.json)) reads the Conventional Commits on `main` since the last release and keeps one release pull request open that raises the version and updates `CHANGELOG.md`. Each merge to `main` only updates that pull request, so changes from any number of pull requests accumulate; nothing is released until a maintainer merges the release pull request, which tags `vX.Y.Z` and publishes a GitHub release. The whole repository has one version: the Go module (whose tags `pluginsdk` users import) and, once it exists, the frontend in `web/`, which is embedded in the same binary and image. Until 1.0.0 a breaking change bumps the minor version (`bump-minor-pre-major`). Commits before `bootstrap-sha` are ignored, as they predate the commit rules.

When `web/package.json` is added, add it to the `"."` package's `extra-files` as `{"type": "json", "path": "web/package.json", "jsonpath": "$.version"}` so its version follows the release.

A release is cut only from a tree on which every required check passed: the release pull request runs the same checks as any other pull request and cannot be merged before they pass. Pull requests opened with the workflow's `GITHUB_TOKEN` start no workflows, so release-please uses a token of a GitHub App created with [actions/create-github-app-token](https://github.com/actions/create-github-app-token). The App needs the repository permissions Contents, Pull requests and Issues (read and write) and is installed on this repository only; its client ID is the repository variable `RELEASE_APP_CLIENT_ID` and its private key the repository secret `RELEASE_APP_PRIVATE_KEY`. Without them the release workflow fails, and no release pull request is created.

## Tests

### Tiers

| Tier | Command | Needs |
| --- | --- | --- |
| Unit | `make test` (`go test ./...`) | Go only. Cacheable. Database tests are skipped with `PostgreSQL integration disabled; run with GOPAGER_INTEGRATION=1`. |
| Integration | `make test-integration` | Docker. `GOPAGER_INTEGRATION=1`, `-count=1 -shuffle=on`, no race detector; writes `junit.xml` and `test.json` to `build/test-reports` (`TEST_REPORT_DIR`). |
| Race (CI) | `make test-race` | Docker. Integration plus `-race`; this is what CI runs. |
| Slowest | `make test-slowest` | The `test.json` of the last integration or race run. Lists tests over `SLOWEST_THRESHOLD` (1 s). |
| Stress | `make test-stress [STRESS_COUNT=5]` | Docker. `-race -shuffle=on -count=N` for flaky-test hunting. Ginkgo suites reject `-count` above 1, so they are skipped there and repeated with the Ginkgo CLI (`ginkgo --repeat`, the documented way). |
| External database | `GOPAGER_TEST_DATABASE_URL='postgres://…' make test-race` | A disposable PostgreSQL server with a role that has `CREATEDB` |

Local equals CI: the same make targets, the same pinned PostgreSQL image digest and one container per package, with tool versions pinned in the Makefile. There are no automatic reruns (no `--rerun-fails`): a flaky test is a bug to find with `make test-stress`. The integration targets pass `-shuffle=on`, which disables Go's test cache, so only `make test` is cacheable.

The container is started by `internal/testutil/postgres` per test package, not by make or compose. That needs no setup beyond Docker, isolates concurrent runs and worktrees (no shared fixed port or server), is cleaned up by Testcontainers' Ryuk reaper even after a crash, and runs identical code locally and in CI. `GOPAGER_TEST_DATABASE_URL` stays the escape hatch for a server you provide.

### Where a test belongs

- Domain rules are tested with fakes in the domain package. Same speed as any unit test; no PostgreSQL.
- Integration tests (mostly `internal/persistence/ent/borrowed`) cover only what PostgreSQL decides: SQL, constraints, locks, isolation, `NOTIFY`, snapshot visibility and migrations. A store test that repeats a domain rule is trimmed to its PostgreSQL-only assertions.
- E2E tests in `cmd/gopager` cover wiring only, one test per external surface or path. The transport matrix (plain HTTP, TLS reverse proxy, forged forwarding headers, Origin and CSRF) lives at the HTTP unit tier, `internal/identity/http_test.go`; `cmd/gopager/tls_proxy_test.go` is the single E2E test of the TLS proxy path, and feature E2E tests use plain HTTP.
- Production costs appear only in tests that verify them (Argon2id parameters, real timers). Every other test injects a cheap hasher (`internal/testutil/passwords`) or a short interval (see [Keeping tests off the wall clock](#keeping-tests-off-the-wall-clock)).

Environment variables:

| Variable | Values | Effect |
| --- | --- | --- |
| `GOPAGER_INTEGRATION` | unset or `1`; anything else fails the run | `1` starts one PostgreSQL container per participating test package |
| `GOPAGER_TEST_DATABASE_URL` | DSN | Use this server instead of a container; setting it alone also enables database tests. Tests create and drop databases next to the one named in the DSN, so use a disposable server, never a production one. |

The integration targets use `-count=1` so Go's test cache does not report stale results. In a requested integration run, a missing Docker daemon or unreachable database fails the run; it is not skipped.

Packages with database tests (their `TestMain` calls `testpostgres.Run`): `cmd/gopager`, `internal/platform/postgres`, `internal/persistence/ent/borrowed`. Each test gets its own database (random name), dropped with `WITH (FORCE)` on cleanup, so tests share nothing: schemas, `LISTEN/NOTIFY` channels and advisory locks are all database-scoped. `internal/testutil/postgres` provides `EmptyDatabase(t)` (cloned from `template0`) and `NewTemplate(name, build).Database(t)`, which builds a template database once per process (for example with all migrations applied), marks it `IS_TEMPLATE` with connections disallowed, and clones it with `CREATE DATABASE … TEMPLATE`. Clones of one template are serialized because PostgreSQL refuses to copy a template that other sessions are using. `Run` drops the process's templates after the tests. The owned container is deliberately non-durable (`fsync=off`, `synchronous_commit=off`, `full_page_writes=off`, data directory on tmpfs, `max_connections=500`); it is a throwaway fixture and says nothing about crash safety. An external server may have any durability settings.

### Parallel tests

Every test runs in parallel. `paralleltest` and `tparallel` (configured in `.golangci.yml`, run by `make`) fail the build otherwise. Tests do not pay production costs that only make them slow: passwords use the reduced Argon2id cost of `internal/testutil/passwords` (the command tests set it once in `TestMain` through `identityOptions`), and batch sizes and intervals are options of their component (for example `assignments.WithCorrectionBatchSize`) so that a test can exercise multi-batch behaviour with little data.

- A new top-level test calls `t.Parallel()` as its first statement. A subtest calls it too, so it needs its own setup: its own database (`newStore(t)`, `testpostgres.EmptyDatabase(t)`), `t.TempDir()`, server and fixtures. Siblings must not share mutable state or depend on each other's order; a case that mutates shared fixtures gets a fixture of its own.
- A parent with parallel subtests releases its resources with `t.Cleanup`, never `defer`: a deferred call runs when the parent function returns, before the parallel subtests resume.
- Tests must not touch process-global state: no `t.Setenv`/`os.Setenv` (Go panics on `t.Setenv` in a parallel test), `os.Chdir`, `slog.SetDefault`, swapped package variables, fixed TCP ports (listen on `127.0.0.1:0`) or fixed temp paths. Inject the value instead. A process-wide value that no test changes can be set once in `TestMain`; `internal/processing/main_test.go` does this for `GOPAGER_TEST_CORE_SECRET`.
- A test that cannot be parallel stays serial and says why: `//nolint:paralleltest // <concrete reason>` on the `func` line (and on the `for`/`t.Run` line that `paralleltest` reports for its subtests; add `tparallel` when a parallel parent has sequential subtests). Go runs serial top-level tests first, one at a time, and only then releases the parallel ones, so a serial test can safely change process-global state. Current exceptions: tests that swap `computeAllowance`, `pluginIdleTimeout` or `installedInvocationTimeout` in `internal/processing`; tests that replace the `slog` default logger or set `LANG`; the re-exec helper `TestRunProcessHelper`; tests that assert against the wall-clock compute allowance or elapsed time (CPU contention would exhaust it); and the event-log tests that read through `Log.ReadAfter`, whose visibility horizon (`pg_snapshot_xmin`) is server-wide, so a transaction open in any parallel test's database hides committed events (with an external server, other packages' transactions hide them as well, so those tests can then need a quiet server).
- Ginkgo suites (`RunSpecs`) are one parallel test per package; their specs run in order, because a suite shares its `BeforeEach` state and the global Gomega fail handler. Ginkgo rejects `go test -count=N` with N > 1 for them, so `make test-stress` skips them and repeats them with `ginkgo --repeat`.

Isolation model: one PostgreSQL server per test package (separate packages never share a server), one database per test (see above), so schemas, `LISTEN/NOTIFY` channels and advisory locks are per test. Server-wide state is the only shared thing: transaction ids and the snapshot horizon (see the event-log exception) and the connection limit. Each store pool opens at most 10 connections and the owned container allows 500, with up to `GOMAXPROCS` tests running per package. A run with `GOMAXPROCS=32` peaked at about 45 connections; on a much larger machine lower the parallelism with `go test -parallel=N`.

`make test-integration` and `make test-race` shuffle the test order to expose hidden dependencies; `make test-stress` repeats the run.

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

### Keeping tests off the wall clock

Tests must not wait on production timers or pay production costs they are not testing:

- Password hashing: pass `passwords.Option()` (`internal/testutil/passwords`) to `identity.NewLoginService` and `identity.Bootstrap`, and use `passwords.Hasher().Hash` for fixture hashes. It uses cheap Argon2id parameters and still verifies production-parameter hashes. The production verifier (`identity.VerifyPassword`) accepts only the production parameters, and one test pins them. Packages that cannot import `testutil/passwords` (`internal/identity` itself) build a hasher with `identity.NewPasswordHasher`.
- Candidate lease renewal: `worker.Service.WithRenewInterval` shortens the renew period (default 20 s; it must stay at most a third of the 60 s lease). Tests that observe renewal against a real database use it; the `synctest` tests in `internal/processing/worker` use fake time instead.
- Prefer injection over package-level overrides so tests can run in parallel.

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
