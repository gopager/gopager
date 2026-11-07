# AP-16 Operations and Profile Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** [REQ-111..112/115..120/127](../requirements-catalog.md#l-operations-data-volume-and-evolution). REQ-114 for shutdown, readiness and authority honesty (provider outcomes: AP-12; intake: AP-07). For REQ-115/117 the plugin part (shipping first-party plugins, placement on enabled instances) is AP-12.
- **Inputs:** (those producers own the actual paths/versions and local lifecycle limitations; retention, shutdown and fault participants come from their handovers; earlier APs, so an absent handover is an input gap)
  - [AP-05 handover](../execution/ap-05.md#handover): foundation (store, migrations, runtime).
  - [AP-06 handover](../execution/ap-06.md#handover): kernel.
  - [AP-07 handover](../execution/ap-07.md#handover): intake.
  - [AP-08 handover](../execution/ap-08.md#handover): master data and correction.
  - [AP-09 handover](../execution/ap-09.md#handover): `gateway_state_history`, `gateways.history_removed_before`, ContactMonitor flags; `last_contact_at`, `last_heartbeat_at` and `last_transmission_at` survive retention.
  - [AP-10 handover](ap-10-query-live.md#handover): read-model tables, consumer registry and the SSE replay window constant (default 24 h) that bounds `domain_events` pruning.
  - [AP-11 handover](ap-11-events-delivery.md#handover): Delivery tables and attempts, receipts, the [C-09](../contracts/c-09-delivery.md) 24 h rule and the retention participants.
  - [AP-12 handover](ap-12-plugin-host.md#handover): plugin artifact/version state, the plugin status accessor in the instance read view (used by the status route and the update preflight), packaged first-party plugins (plugin stage in the Dockerfile, plugin directory, Makefile plugin targets).
  - [AP-13 handover](ap-13-processing-presentation-integration.md#handover): installation settings and layout tables where retention-relevant.
  - [AP-14 handover](ap-14-identity-lifecycle.md#handover): account deletion cleanup owners.
  - [AP-15 handover](ap-15-frontend.md#handover): Dockerfile frontend stage, consumed by the compose verification.
  - [AP-20 handover](../execution/ap-20.md#handover): correction and pending-scope state for the status route.
- **Owned contract slices:** [C-12](../contracts/c-12-persistence-operations.md#planned-and-not-yet-implemented-ap-16) planned operations (storage status, `RunRetention`, setup, shutdown, update/migration); supporting slices of [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md) and [C-13](../contracts/c-13-account-lifecycle.md) for retention participants. Do not replace frozen domain semantics. Retention windows and defaults are decided by [D-142](../decisions/09-process-and-documentation.md#d-142); this AP only implements pruning. Operational decisions are recorded in these contracts and evidence procedures in the execution record before implementation.
- **Reference (read when needed):** [technology decisions](../architecture.md#technology-decisions), [operating profiles](../architecture.md#operating-profiles), [supporting responsibilities](../architecture.md#supporting-responsibilities), [D-63](../decisions/01-platform-and-storage.md#d-63), [D-134](../decisions/09-process-and-documentation.md#d-134), [D-135](../decisions/10-foundation-redesign.md#d-135), [F-19/F-23/F-25](../contracts/reference-cases.md), [F-40 and F-41](../contracts/ap-02-boundary-cases.md), [D-142](../decisions/09-process-and-documentation.md#d-142) (retention), [operations](../../docs/operations.md).
- **Write scope:**
  - `internal/operations/` (new) — storage status, retention run, operator commands
  - `internal/messages/`, `internal/ingest/`, `internal/processing/`, `internal/readmodel/`, `internal/delivery/`, `internal/events/` — retention participants
  - `internal/identity/` and `internal/commands/` — retention participants (login attempts, command receipts) via their stores
  - `internal/persistence/ent/schema/`, the stores of the retention participants under `internal/persistence/ent/`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout))
  - `internal/assignments/`, `internal/persistence/ent/assignmentstore/` — bounded Suppression listing ([D-134](../decisions/09-process-and-documentation.md#d-134))
  - transaction call sites in any `internal/` package — only to route them through the readiness-gated transaction capability
  - `internal/httpapi/` — handlers for status/configuration
  - `cmd/gopager/` and `internal/platform/` — runner/bootstrap/shutdown registration only (composition)
  - `deploy/compose/` — verification and fault fixtures (build stages belong to AP-12 and AP-15)
  - `.github/workflows/ci.yml` and `Makefile` — fault and probe targets only
  - `internal/persistence/ent/borrowed/` and `internal/testutil/` — PostgreSQL and fault fixtures
  - operator documentation under `docs/`
- **Compatibility context:** [AP-17](ap-17-integration-evidence.md) receives machine-readable operational APIs, operator procedures, retention tests, topology/fault results and D-63/D-135 limitations (use its evidence expectations, not its unfinished output). No AP-17 sign-off is required.

## Open User Decisions

None.

## Planning Before Coding

Record operational decisions in C-12 and supporting contracts, and evidence procedures in the execution record before implementation.

T081: decisions of [D-142](../decisions/09-process-and-documentation.md#d-142) are fixed input. Define in C-12 and the execution record:
- The retention participant interface, the chunk/checkpoint/restart protocol and the participant list (Messages with targets/originals/Processing/search data, Submission receipts in `internal/ingest`/`ingeststore`, command receipts in `internal/commands`/`commandstore`, `domain_events`, `consumer_offsets`, Delivery receipts, `login_attempts`, `audit_records`, `gateway_state_history`).
- The installation setting: a revisioned single-row table (e.g. `retention_settings`) with the Message and `audit_records` limits (age and count, unlimited by default); the routes `GET/PUT /operations/retention`, `POST /operations/retention/runs` and `GET /operations/status`, all `operations.manage` with scope all (no new read action); a worker-role runner with `--retention-interval` (default 1h, greater than 0); the manual run uses the same path.
- The automatic rules: `domain_events` and `consumer_offsets` per the documented rule (below every consumer offset and older than the replay window, AP-10's SSE replay window, default 24 h), terminal receipts per the C-08 rule, `login_attempts` once older than the longest rate-limit window. Only Messages and `audit_records` need an operator limit.
- The storage-status fields, setup without default credentials, shutdown/drain and update/migration procedures: verify against [docs/operations.md](../../docs/operations.md), record gaps in C-12 and amend the procedures only where a gap is found.
- The readiness-gated transaction capability: today `platform.RunningHandle.WithinTransaction` (`internal/platform/status.go`) exists but is created inside `platform.StartWithAPI`, after `cmd/gopager/main.go` composed the services from `store`. Resolve this construction-order cycle with a late-bound transaction runner handed to every service at composition and bound to the RunningHandle after start (refusing work before binding); record the design in C-12 and the execution record.
- Bounded listings: `assignmentstore.ListSuppressions` returns all rows and `Service.ListSuppressions` filters and pages in memory; replace it with a SQL predicate (target authorization evaluated in SQL) and a page token. A grep of the store `List*` methods found no other in-memory-paginated management list (all others apply a SQL limit); keep a volume test that guards this.

T082: plan a disposable Full environment with writable PostgreSQL, completed API/SSE/worker roles, controlled provider transport and a held authorized stream/attempt. Bind [F-19/F-23/F-25](../contracts/reference-cases.md) and [F-40 and F-41](../contracts/ap-02-boundary-cases.md) to concrete fault injection, expected observations and cleanup: primary unavailability/read-only authority, old lease publication/start, process termination/restart, known rollback versus lost commit acknowledgement, disposable storage/disk-full, migration failure, pending cleanup/retention restart and started-provider uncertainty. Distinguish topology/transport/storage faults from synthetic failpoints; a primary restart is not demonstrated HA failover. Destination: the execution record.

T083: document the Lite profile as built by D-135: one `gopager --roles=api,worker` process with PostgreSQL as a compose sidecar (`deploy/compose/`), PostgreSQL only, no SQLite and no `--profile` flag, and the Lite-to-Full move as `pg_dump`/`pg_restore` into the external primary. An application-managed PostgreSQL child is not planned. State the limitations on hardware, SLA/RPO/RTO and Backup/Restore claims. F-41 is run here (Lite-to-Full with a quiesced source). Destination: `docs/operations.md` and C-12.

## Tasks

- [ ] T081 [AP-16] Implement retention, the readiness-gated transaction capability, bounded management listings and the operations status/settings API; verify (and amend only where a gap is found) setup, shutdown and update procedures.
  - **Done when:**
    - C-12 `RunRetention` follows [D-142](../decisions/09-process-and-documentation.md#d-142): `domain_events`, `consumer_offsets`, terminal receipts and `login_attempts` are pruned automatically by the documented rules; Messages (whole-Message ordering, participant deletion of targets/originals/Processing/search/Submission receipts) and `audit_records` are deleted only under the operator limit, unlimited by default; transaction/chunk/checkpoint/restart is tested ([C-12](../contracts/c-12-persistence-operations.md)).
    - Self-contained Delivery inputs are preserved only within C-09 lifetime/current-rights rules, dependent work ends explicitly unavailable, and only contracted dedup/uncertainty records are kept.
    - The database pool settings stay fixed (10 open, 5 idle, 30 min; [operations](../../docs/operations.md#configuration-reference) "Fixed values"); no pool tuning.
    - The `operations.manage` routes `GET/PUT /operations/retention`, `POST /operations/retention/runs` and `GET /operations/status` (including the plugin state from AP-12's status accessor and the correction state from AP-20), the `--retention-interval` runner, removed-history diagnostics and operator commands exist; storage exhaustion triggers no emergency deletion.
    - All production domain/runner transactions go through the running platform's readiness-gated transaction capability, so workers stop starting new work while not ready (incompatible schema, lost authority, shutdown) and resume after recheck; tested with a schema-incompatibility and authority-loss fixture.
    - Unbounded in-memory listing of retained management records (e.g. `ListSuppressions`) is replaced by authorization-before-pagination SQL predicates and bounded pages, with a volume test ([D-134](../decisions/09-process-and-documentation.md#d-134)).
    - Operators receive truthful machine-readable state.
- [ ] T082 [AP-16] Demonstrate authority loss and recovery, lease fencing, disk/crash/restart and external-effect recovery boundaries.
  - **Done when:** [F-19/F-23/F-25](../contracts/reference-cases.md) and [F-40 and F-41](../contracts/ap-02-boundary-cases.md) are run against concrete fault injection with recorded observations and cleanup; private output/new starts stop under lost authority and recover only after current checks; faults are never injected into a shared/non-disposable system; exact environment and commands are recorded; reproducible evidence exists for exactly these claims: authority loss stops output and starts, stale leases are fenced, a known rollback differs from `COMMIT_UNCERTAIN`, and a restart honors the recorded uncertainty; this includes the C-01 binding failure rules (storage shortage, write failure and uncertain authority stay visible to Gateways without exposing other Gateways or Networks, no deletion to free space) under [D-139](../decisions/02-intake-and-grouping.md#d-139).
- [ ] T083 [AP-16] Verify the compose Lite deployment (startup, health, shutdown, restart against the PostgreSQL sidecar) and document the Lite-to-Full dump/restore procedure and the profile limits.
  - **Done when:** packaging startup/health/shutdown/restart probes pass against the compose PostgreSQL sidecar, the dump/restore procedure is documented, F-41 passes (Lite-to-Full `pg_dump`/`pg_restore` with a quiesced source preserves identities, grants, jobs and Secret references; the active writer is never activated; a missing plugin artifact visibly blocks; an incompatible schema stays not-ready), and no Lite hardware, parity or SLA claim is made (there is no SQLite adapter).

## Documentation to Update

- [C-12](../contracts/c-12-persistence-operations.md) planned section and, where retention participants change, the supporting [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md) and [C-13](../contracts/c-13-account-lifecycle.md) slices; move implemented behavior into `docs/`.
- [docs/operations.md](../../docs/operations.md): `Topology` / `Lite with Docker Compose`, `Configuration Reference`, `Roles`, `Database Migrations` / `Upgrade`, `Event Log Retention`, `First Start and Bootstrap`, `Starting the Server`, `Backup and restore responsibility`, `Health Endpoints`, `Failure Handling`, `Graceful Shutdown`.
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md) `Route reference` for the status/configuration routes.
- [docs/architecture.md](../../docs/architecture.md): `Cross-cutting mechanisms` (`Readiness and lifecycle`, `Transactions and the store abstraction`, `Migrations and Ent code generation`).
- [docs/development.md](../../docs/development.md): `Database Schema and Migrations` and `Tests` for fault fixtures and process tests.
- Root [README.md](../../README.md) `Getting Started` if the compose deployment changes; Go doc comments on exported identifiers; [glossary](../glossary.md) for new terms; [requirement mapping](requirement-mapping.md) if a producer changes.

## Verification

Use existing PostgreSQL integration/race/process tests plus disposable topology procedures and packaging startup/health/shutdown probes; record exact environment and commands. Never inject faults into a shared/non-disposable system. No external provider access is necessary for fault fixtures; AP-12 supplies real-provider evidence. Include the retention volume test and the schema-incompatibility and authority-loss fixture, then `make`, `go test ./...` and `make docs-check`.

## Handover

Hand over to AP-17: machine-readable operational APIs, operator procedures, retention tests, topology/fault results and D-63/D-135 limitations.

## Non-goals

Application Backup/Restore, an application-managed PostgreSQL child, external HA/failover automation, SQLite, plugin work (AP-12), unsupported hardware certification, quantitative SLA/RPO/RTO, or custom consensus.
