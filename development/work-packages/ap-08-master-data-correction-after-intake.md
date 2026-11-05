# AP-08 Master Data and Correction

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-013/019/021..040/045/095/123..124/132](../requirements-catalog.md#c-units-organizations-and-defaults), [C-02](../contracts/c-02-protocol-payload.md), [C-04](../contracts/c-04-master-data.md), [C-05](../contracts/c-05-matching-correction.md), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md).
- **Owner:** DOM-03/DOM-04; integration coordinates cross-domain correction.
- **Architecture:** [domain ownership](../architecture.md#domains-and-data-ownership), [logical relationships](../architecture.md#logical-relationship-model), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** [AP-05](../execution/ap-05.md#handover) transactions/primary clock, [AP-06](../execution/ap-06.md#handover) kernel/publication, [AP-07](../execution/ap-07.md#handover) committed intake, [identity/AuthZ handover](../execution/source-ap-07.md#handover) and [Unit/collection/current-matcher handover](../execution/source-ap-08.md#handover). Required unfixed C-04/C-05/C-06/C-08 slices are defined below.
- **Write scope:** `internal/Units/`, `internal/assignments/`; supporting master-data/suppression action and scope adapters in `internal/authz/` and `internal/identity/`; correction-source/publication participation in `internal/processing/` without evaluator/ABI replacement; additive facts in `internal/events/` and `internal/persistence/ent/eventstore/`; owner adapters in `internal/persistence/ent/assignmentstore/`, `internal/persistence/ent/ingeststore/` and Unit/correction stores; owner Ent schemas/generated output, next ordered Goose migrations and PostgreSQL transaction tests; `cmd/gopager/` management/root/correction-runner composition, `internal/httpapi/` and versioned OpenAPI artifacts. C-04/C-05/C-06/C-08 and canonical cases/terms/selections are authorized supporting planning destinations. Necessary library manifest changes follow D-36, not speculative later infrastructure.

## Planning Before Coding

Record the following implementation-bound decisions at C-04/C-05/C-06/C-08 and cases at their F-ID owners before coding:
- **Later-consumer planning context (not prerequisites):** [AP-09 Gateway/Network](ap-09-gateway-network.md), [AP-10 Query](ap-10-query-live.md), [AP-11 Delivery](ap-11-events-delivery.md), [AP-13 Presentation](ap-13-processing-presentation-integration.md), [AP-14 Identity](ap-14-identity-lifecycle.md), [AP-15 Frontend](ap-15-frontend.md), and [C-13](../contracts/c-13-account-lifecycle.md). Use these to avoid incompatible boundaries, not to implement their tasks.
- T060 defines Unit/Organization/collection/template/default CRUD inputs/state changes, validation/errors, authorization/projection, bulk atomicity, conflict/default preservation and transaction/revision boundaries. Templates are standalone copied snapshots under [C-04](../contracts/c-04-master-data.md#ap-08-unit-template-direction); define explicit actions/scopes/delegation/reference deletion in C-06. Define revisioned management HTTP operations. Defaults are typed stored/copied values; storing/materializing them requires no renderer.
- T060/T061 own additive Organization/template/suppression fact schemas, versions, aggregate revisions, canonical lookups and command/state/audit/receipt atomicity in C-08. Preserve master-data-facts 1.0; introduce a compatible versioned slice.
- T061 defines correction eligibility/suppression, target resolution/support redistribution, claims/expiry/fencing, cutoff/dirty capture, supersession/retries/failure and current Assignment/Suppression/access/source revision checks using AP-05 transactions/primary clock. Bind target/contribution/support/readiness mutations and Processing publication participation, preserving originals, Message membership and fixed intake/kernel semantics.
- T062 covers [F-27..F-29](../contracts/ap-02-boundary-cases.md), [F-11..F-16](../contracts/reference-cases.md), template authorization/snapshot independence, bulk rollback, suppression, concurrent intake/correction, lease loss/supersession/restart, Processing/revocation races, stale publication refusal and real HTTP safe errors.
- Validation: existing Ent generation; `go test -race -count=1 -timeout=10m ./internal/Units ./internal/assignments ./internal/authz ./internal/events ./internal/ingest ./internal/processing/... ./cmd/gopager`; `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager`; focused vet and `git diff --check`. Include actual newly created correction-package selectors. Database-disabled tests are not PostgreSQL evidence.
- Handover: actual versioned master-data/default management APIs, safe read adapters, suppression/correction states, facts/lookups, store/migration paths and executed case evidence to the above consumers. No Gateway administration, Query/SSE, Delivery, host, renderer, historical Reprocess, account cleanup or browser implementation.

- [ ] T060 [AP-08] Complete the remaining Unit, organization, collection, template, default, bulk, and conflict operations.
- [ ] T061 [AP-08] Complete historical target correction, contribution/support redistribution, suppression, and checked rematching using the committed intake boundary.
- [ ] T062 [AP-08] Verify deletion/default preservation, correction/revocation races, rollback, and the assigned F-cases against PostgreSQL.

**Acceptance:** Corrections update only permitted derived state, retain original evidence, and leave no stale visible Unit references or unauthorized conflict details.

**Non-goals:** New Transmission/Message creation, Payload masking, historical presentation versions, unapproved imports, or heuristic comparison.
