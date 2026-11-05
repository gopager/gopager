# AP-16 Operations and Profile Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-111..120/127](../requirements-catalog.md#l-operations-data-volume-and-evolution), [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-12](../contracts/c-12-persistence-operations.md), [C-13](../contracts/c-13-account-lifecycle.md).
- **Owner:** Operations/integration and domain schema owners.
- **Architecture:** [technology decisions](../architecture.md#technology-decisions), [operating profiles](../architecture.md#operating-profiles), [supporting responsibilities](../architecture.md#supporting-responsibilities).
- **Prerequisites:** Verify [foundation](../execution/ap-05.md), [kernel](../execution/ap-06.md), [intake](../execution/ap-07.md) and [AP-08..AP-15](../work-packages.md#revised-execution-path) store/work/API/host/asset handovers for retention, shutdown and fault participants. Those producers own actual paths/versions and local lifecycle limitations. Operational planning belongs here; no AP-17 sign-off is required.
- **Write scope:** `internal/operations/`, retention participants in `internal/messages/`, `internal/ingest/`, `internal/processing/`, `internal/readmodel/`, `internal/delivery/`, `internal/events/`, owner Ent adapters/schemas/generated output and next ordered migrations; `cmd/gopager/`, `internal/platform/` and `internal/httpapi/` for status/configuration/runner/bootstrap/shutdown registration; packaging, CI/deployment/operator documentation and disposable fault fixtures. C-12 and supporting C-01/C-08/C-09/C-13 slices are this AP's planning destinations; do not replace frozen domain semantics.

## Planning Before Coding

Record operational decisions in C-12 and supporting contracts, and evidence procedures in the execution record before implementation.

- **Later-consumer planning context (not prerequisites):** [AP-17](ap-17-integration-evidence.md). Use its evidence expectations, not its unfinished output.
- For T083, document only the approved Lite/Full transition boundary and clearly identify behavior deferred from runtime implementation. State the limitations on hardware, SLA/RPO/RTO and Backup/Restore claims.

- [ ] T081 [AP-16] Implement/document storage status, configured retention execution, setup without default credentials, shutdown, and update/migration workflows.
- [ ] T082 [AP-16] Demonstrate approved Full authority, lease/failover, disk/crash/restart, and external-effect recovery boundaries.
- [ ] T083 [AP-16] Record the approved Lite/Full transition boundary without implementing deferred Lite runtime or executable transition scope.

## Exact Retention and Full Evidence Scope

T081 defines storage-status fields, setup without default credentials, shutdown/drain, update/migration procedures and observable failures/operator actions. Implement C-12 `RunRetention`: configured age/count limits, no default deletion, whole-Message ordering, transaction/chunk/checkpoint/restart and participant deletion of targets/supports/originals/Processing/search/Submission receipts. Preserve self-contained Delivery inputs only within C-09 lifetime/current-rights rules; dependent work ends explicitly unavailable. Keep only contracted dedup/uncertainty records. Expose authorized configuration/run/status HTTP or operator commands, removed-history diagnostics and runner registration. Storage exhaustion must not trigger emergency deletion. T081 routes all production domain/runner transactions through the running platform's readiness-gated transaction capability, so workers stop starting new work while not ready (incompatible schema, lost authority, shutdown) and resume after recheck; test with a schema-incompatibility and authority-loss fixture. T081 replaces unbounded in-memory listing of retained management records (e.g. `ListSuppressions`) with authorization-before-pagination SQL predicates and bounded pages, with a volume test ([D-134](../authoritative-plan.md#review-finding-ap-planning-revision)).

T082 plans a disposable Full environment with writable PostgreSQL, completed API/SSE/worker roles, controlled provider transport and a held authorized stream/attempt. Bind [F-19/F-23/F-25](../contracts/reference-cases.md) and applicable [F-39..F-41](../contracts/ap-02-boundary-cases.md) to concrete fault injection, expected observations and cleanup: primary unavailability/read-only authority, old lease publication/start, process termination/restart, known rollback versus lost commit acknowledgement, disposable storage/disk-full, migration failure, pending cleanup/retention restart and started-provider uncertainty. Private output/new starts stop under lost authority and recover only after current checks. Distinguish topology/transport/storage faults from synthetic failpoints; a primary restart is not demonstrated HA failover.

Use existing PostgreSQL integration/race/process tests plus disposable topology procedures and packaging startup/health/shutdown probes; record exact environment and commands. Never inject faults into a shared/non-disposable system. No external provider access is necessary for fault fixtures; AP-12 supplies real-provider evidence. Hand over machine-readable operational APIs, operator procedures, retention tests, topology/fault results and D-32/D-63 limitations to AP-17. T083 does not implement Lite/SQLite or executable transfer.

**Acceptance:** Operators receive truthful machine-readable state and reproducible evidence for approved operational claims.

**Non-goals:** Application Backup/Restore, executable Lite-to-Full transition, unsupported hardware certification, quantitative SLA/RPO/RTO, or custom consensus.
