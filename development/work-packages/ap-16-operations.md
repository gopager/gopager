# AP-16 Operations and Profile Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** [REQ-111..112/115..120/127](../requirements-catalog.md#l-operations-data-volume-and-evolution), REQ-128 (Unit import/export, promoted by [D-135](../decisions/10-foundation-redesign.md#d-135)).
- **Inputs:** (those producers own the actual paths/versions and local lifecycle limitations; retention, shutdown and fault participants come from their handovers)
  - [AP-05 handover](../execution/ap-05.md#handover): foundation (store, migrations, runtime).
  - [AP-06 handover](../execution/ap-06.md#handover): kernel.
  - [AP-07 handover](../execution/ap-07.md#handover): intake.
  - [AP-08 handover](../execution/ap-08.md#handover): master data and correction.
  - [AP-09](ap-09-gateway-network.md) to [AP-15](ap-15-frontend.md): store/work/API/host/asset handovers; verify `development/execution/ap-NN.md#handover` exists at start for each of AP-09..AP-15.
- **Owned contract slices:** [C-12](../contracts/c-12-persistence-operations.md#planned-and-not-yet-implemented-ap-16) planned operations (storage status, `RunRetention`, setup, shutdown, update/migration); supporting slices of [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md) and [C-13](../contracts/c-13-account-lifecycle.md) for retention participants. Do not replace frozen domain semantics. Operational decisions are recorded in these contracts and evidence procedures in the execution record before implementation.
- **Reference (read when needed):** [technology decisions](../architecture.md#technology-decisions), [operating profiles](../architecture.md#operating-profiles), [supporting responsibilities](../architecture.md#supporting-responsibilities), [D-32](../decisions/01-platform-and-storage.md#d-32), [D-63](../decisions/01-platform-and-storage.md#d-63), [D-134](../decisions/09-process-and-documentation.md#d-134), [D-135](../decisions/10-foundation-redesign.md#d-135), [F-19/F-23/F-25](../contracts/reference-cases.md), [F-39..F-41](../contracts/ap-02-boundary-cases.md), [operations](../../docs/operations.md).
- **Write scope:**
  - `internal/operations/` — storage status, retention run, operator commands
  - `internal/messages/`, `internal/ingest/`, `internal/processing/`, `internal/readmodel/`, `internal/delivery/`, `internal/events/` — retention participants
  - owner Ent adapters/schemas/generated output and the next ordered migrations
  - `internal/units/` — Unit CSV import/export
  - `internal/httpapi/` — handlers for status/configuration
  - `cmd/gopager/` and `internal/platform/` — runner/bootstrap/shutdown registration only (composition)
  - `deploy/compose/`, packaging, CI/deployment/operator documentation and disposable fault fixtures
- **Compatibility context:** [AP-15](ap-15-frontend.md) screens consume the Unit CSV import/export operation; [AP-17](ap-17-integration-evidence.md) receives machine-readable operational APIs, operator procedures, retention tests, topology/fault results and D-32/D-63 limitations (use its evidence expectations, not its unfinished output). No AP-17 sign-off is required.

## Open User Decisions

None.

## Planning Before Coding

Record operational decisions in C-12 and supporting contracts, and evidence procedures in the execution record before implementation.

T081: define storage-status fields, setup without default credentials, shutdown/drain, update/migration procedures and observable failures/operator actions. Destination: C-12 and the execution record.

T082: plan a disposable Full environment with writable PostgreSQL, completed API/SSE/worker roles, controlled provider transport and a held authorized stream/attempt. Bind [F-19/F-23/F-25](../contracts/reference-cases.md) and applicable [F-39..F-41](../contracts/ap-02-boundary-cases.md) to concrete fault injection, expected observations and cleanup: primary unavailability/read-only authority, old lease publication/start, process termination/restart, known rollback versus lost commit acknowledgement, disposable storage/disk-full, migration failure, pending cleanup/retention restart and started-provider uncertainty. Distinguish topology/transport/storage faults from synthetic failpoints; a primary restart is not demonstrated HA failover. Destination: the execution record.

T083: document the Lite profile as built by D-135: one `gopager --roles=api,worker` process with PostgreSQL as a compose sidecar (`deploy/compose/`), PostgreSQL only, no SQLite and no `--profile` flag, and the Lite-to-Full move as `pg_dump`/`pg_restore` into the external primary. An application-managed PostgreSQL child is not planned. State the limitations on hardware, SLA/RPO/RTO and Backup/Restore claims. Destination: `docs/operations.md` and C-12.

## Tasks

- [ ] T081 [AP-16] Implement/document storage status, configured retention execution, setup without default credentials, shutdown, and update/migration workflows.
  - **Done when:**
    - C-12 `RunRetention` works with configured age/count limits, no default deletion, whole-Message ordering, transaction/chunk/checkpoint/restart, and participant deletion of targets/originals/Processing/search/Submission receipts; the retention list also covers `domain_events`, `consumer_offsets`, receipts, login attempts and audit records ([C-12](../contracts/c-12-persistence-operations.md)).
    - Self-contained Delivery inputs are preserved only within C-09 lifetime/current-rights rules, dependent work ends explicitly unavailable, and only contracted dedup/uncertainty records are kept.
    - The existing database pool settings (10 open, 5 idle, 30 min connection lifetime; [operations](../../docs/operations.md)) stay configurable as they are; no pool tuning is added.
    - The Unit CSV import/export operation (REQ-128) exists with the AP-15 screens as consumer.
    - Authorized configuration/run/status HTTP or operator commands, removed-history diagnostics and runner registration exist; storage exhaustion triggers no emergency deletion.
    - All production domain/runner transactions go through the running platform's readiness-gated transaction capability, so workers stop starting new work while not ready (incompatible schema, lost authority, shutdown) and resume after recheck; tested with a schema-incompatibility and authority-loss fixture.
    - Unbounded in-memory listing of retained management records (e.g. `ListSuppressions`) is replaced by authorization-before-pagination SQL predicates and bounded pages, with a volume test ([D-134](../decisions/09-process-and-documentation.md#d-134)).
    - Operators receive truthful machine-readable state.
- [ ] T082 [AP-16] Demonstrate approved Full authority, lease/failover, disk/crash/restart, and external-effect recovery boundaries.
  - **Done when:** [F-19/F-23/F-25](../contracts/reference-cases.md) and applicable [F-39..F-41](../contracts/ap-02-boundary-cases.md) are run against concrete fault injection with recorded observations and cleanup; private output/new starts stop under lost authority and recover only after current checks; faults are never injected into a shared/non-disposable system; exact environment and commands are recorded; reproducible evidence exists for the approved operational claims.
- [ ] T083 [AP-16] Verify the compose Lite deployment (startup, health, shutdown, restart against the PostgreSQL sidecar) and document the Lite-to-Full dump/restore procedure and the profile limits.
  - **Done when:** packaging startup/health/shutdown/restart probes pass against the compose PostgreSQL sidecar, the dump/restore procedure is documented, and no Lite hardware, parity or SLA claim is made (there is no SQLite adapter).

## Documentation to Update

- [C-12](../contracts/c-12-persistence-operations.md) planned section and, where retention participants change, the supporting [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md) and [C-13](../contracts/c-13-account-lifecycle.md) slices; move implemented behavior into `docs/`.
- [docs/operations.md](../../docs/operations.md): `Topology` / `Lite with Docker Compose`, `Configuration Reference`, `Roles`, `Database Migrations` / `Upgrade`, `Event Log Retention`, `First Start and Bootstrap`, `Starting the Server`, `Backup and restore responsibility`, `Health Endpoints`, `Failure Handling`, `Graceful Shutdown`.
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md) `Route reference` for the status/configuration and Unit import/export routes (`Agencies, Organizations, Unit templates, Units, collections`).
- [docs/architecture.md](../../docs/architecture.md): `Cross-cutting mechanisms` (`Readiness and lifecycle`, `Transactions and the store abstraction`, `Migrations and Ent code generation`).
- [docs/development.md](../../docs/development.md): `Database Schema and Migrations` and `Tests` for fault fixtures and process tests.
- Root [README.md](../../README.md) `Getting Started` if the compose deployment changes; Go doc comments on exported identifiers; [glossary](../glossary.md) for new terms; [requirement mapping](requirement-mapping.md) if a producer changes.

## Verification

Use existing PostgreSQL integration/race/process tests plus disposable topology procedures and packaging startup/health/shutdown probes; record exact environment and commands. Never inject faults into a shared/non-disposable system. No external provider access is necessary for fault fixtures; AP-12 supplies real-provider evidence. Include the retention volume test and the schema-incompatibility and authority-loss fixture, then `make`, `go test ./...` and `make docs-check`.

## Handover

Hand over to AP-17: machine-readable operational APIs, operator procedures, retention tests, topology/fault results and D-32/D-63 limitations. Hand over to AP-15: the Unit CSV import/export operation.

## Non-goals

Application Backup/Restore, an application-managed PostgreSQL child, SQLite, unsupported hardware certification, quantitative SLA/RPO/RTO, or custom consensus.
