# AP-07 Transmission Intake Implementation Addendum

**Status:** Approved by the user with `approve_exact_set` on 2026-11-03 under [D-122](../../authoritative-plan.md#ap-07-additive-write-scope-and-execution-binding-proposal), with the worker lifecycle extension under [D-123](../../authoritative-plan.md#ap-07-selected-workflow-worker-lifecycle-scope-extension) and selected-Workflow source/publication extension under [D-124](../../authoritative-plan.md#ap-07-selected-workflow-source-and-publication-scope-extension) approved on 2026-11-04. **Baseline:** AP-07 remains unchanged from `e12759e8883a96dc6af964f05ddbd03f24718512`. Its original write scope, task IDs, acceptance and non-goals remain in force. This addendum is additive only.

## Purpose and Affected Tasks

This proposal binds the additional implementation paths needed by AP-07 T057..T059. It includes the three frozen Gateway operations, the approved credential-management API addendum, original-preserving Transmission acceptance, Network assignment, Gateway state, target admission, Grouping, facts, and the C-11 Processing publication barrier, including selected-Workflow source capture/reconciliation and atomic publication wiring. It does not change T057..T059 or their acceptance.

## Frozen Inputs

- [AP-07 assignment](../ap-07-transmission-intake.md), unchanged.
- [D-119](../../authoritative-plan.md#ap-07-contract-freeze-proposal): the AP-07 slices of C-01, C-02, C-03, C-05, C-06, C-08 and C-11, version 1.0.
- [D-121](../../authoritative-plan.md#ap-07-gateway-credential-management-http-addendum): additive C-01/C-06 Gateway credential-management HTTP slices, version 1.1.
- [F-60..F-68](../../contracts/reference-cases.md#ap-07-intake-cases), planned; none is execution evidence.
- Existing approved C-07 error mapping and C-12 transaction behavior; AP-05 transaction/fact infrastructure; AP-06 Processing kernel and publication ports; the verified historical identity and current-matcher handovers.

## Proposed Write Paths

The original AP-07 paths remain authorized by the assignment:

- `internal/ingest/`
- `internal/protocols/`
- `internal/messages/`
- Focused tests in those packages.

Approved additive paths:

- `cmd/gopager/main.go`, `cmd/gopager/management.go`, and `cmd/gopager/management_test.go`.
- New `cmd/gopager/ingest.go`, `cmd/gopager/ingest_test.go`, `cmd/gopager/gateway_credentials.go`, and `cmd/gopager/gateway_credentials_test.go`.
- New `internal/identity/gateway.go` and `internal/identity/gateway_test.go` for the Gateway-specific credential proof/authentication boundary; do not broaden the existing Principal/API-key proof.
- New `internal/events/intake.go` and `internal/events/intake_test.go` for the frozen AP-07 fact vocabulary.
- New `internal/persistence/ent/schema/ingest.go`.
- Ent-generated output under `internal/persistence/ent/`, produced only by `go generate ./internal/persistence/ent`; no hand edits to generated files.
- New `internal/persistence/ent/ingeststore/` persistence adapters.
- New `internal/persistence/ent/eventstore/intake.go` and `internal/persistence/ent/eventstore/intake_test.go`.
- New `internal/persistence/ent/borrowed/ingest_test.go` for real PostgreSQL transaction, rollback, concurrency, retry, and publication checks.
- New `internal/persistence/migrations/00009_transmission_intake.sql`; existing migration embedding already includes `*.sql`.
- `internal/processing/worker/service.go` and new `internal/processing/worker/service_lifecycle_test.go`, limited to releasing owner-captured run resources and focused lifecycle verification under D-123.
- `cmd/gopager/processing.go` and `cmd/gopager/processing_management.go`, limited to production worker/source composition and run-resource ownership for selected-Workflow execution/publication under D-124.
- `internal/processing/worker/service.go`, extended under D-124 only to invoke the existing publication participant atomically with successful candidate completion, and new `internal/processing/worker/service_publication_test.go` for focused verification.

No other path is approved by this proposal. In particular, do not edit the AP-07 baseline, `internal/assignments/`, `internal/authz/`, any `internal/processing/` path other than `internal/processing/worker/service.go`, `internal/processing/worker/service_lifecycle_test.go` and `internal/processing/worker/service_publication_test.go`, existing identity proof-selection files, migration infrastructure, dependency manifests, or downstream AP assignments. Reuse the existing `assignments.NetworkOwner`, `gateway.update`/`gateway.read` action and scope catalog, C-12 transactions/receipts, event transport, Ent, Goose, pgx and testcontainers. If any named interface cannot satisfy the frozen contracts, or implementation requires a new dependency or any other excluded path, stop and request a separately approved scope change.

## Verification Binding

Run from the Go module root after implementation:

```sh
go generate ./internal/persistence/ent
go test -race -count=1 -timeout=5m ./internal/identity ./internal/authz ./internal/events ./internal/assignments ./internal/protocols ./internal/messages ./internal/ingest ./internal/processing/worker ./cmd/gopager
GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=10m ./cmd/gopager ./internal/protocols ./internal/assignments ./internal/Units ./internal/commands ./internal/authz ./internal/events ./internal/identity ./internal/ingest ./internal/messages ./internal/persistence/ent/... ./internal/platform/postgres
go vet ./cmd/gopager ./internal/protocols ./internal/assignments ./internal/Units ./internal/commands ./internal/authz ./internal/events ./internal/identity ./internal/ingest ./internal/messages ./internal/persistence/ent/... ./internal/platform/postgres
git diff --check
```

The integration command must run with the repository's real disposable PostgreSQL fixture; a database-disabled compile is not T059 evidence. Record actual tool/runtime versions, environment, commands, results and limitations in the AP-07 execution record. The acceptance run covers the existing D-119 F-IDs plus F-60..F-68, including authorization over complete Network sets, credential issue/status/revoke and secret non-replay, transaction rollback/uncertain commit, concurrent idempotent submissions, Grouping convergence/merge, typed facts, and atomic Processing publication.

## Exclusions and Impact

- No raw radio decoder, EAS media processing, historical correction/rematching, external Delivery effect, or downstream AP execution.
- No HTTP credential-management route beyond the three operations approved by D-121; no out-of-band credential provisioner or secret-display/recovery channel.
- No change to D-45..D-48, the frozen C-01/C-06 1.0 text, D-119, previous AP results, or the AP-07 baseline.
- No downstream AP amendment is identified. If a consumer instruction becomes inaccurate, stop and use the approved downstream-amendment gate.

## Approval

The plan is the canonical approval record. The user approved this addendum and execution binding with `approve_exact_set` on 2026-11-03 under [D-122](../../authoritative-plan.md#ap-07-additive-write-scope-and-execution-binding-proposal), the exact worker lifecycle extension with `approve_worker_lifecycle_extension` on 2026-11-04 under [D-123](../../authoritative-plan.md#ap-07-selected-workflow-worker-lifecycle-scope-extension), and the exact selected-Workflow source/publication extension with `approve_exact_extension` on 2026-11-04 under [D-124](../../authoritative-plan.md#ap-07-selected-workflow-source-and-publication-scope-extension). Assignee: the current sequential Copilot SDK assistant, with no parallel worker. T057..T059 may proceed only within these paths and validation commands. Any other excluded path, dependency, or contract change requires a separately approved scope change.
