# AP-09 Gateway and Network Administration

[Execution path](../work-packages.md#revised-execution-path) | [Shared rules](shared-rules.md) | [Approval](../authoritative-plan.md#workflow-and-approval-gates)

## Session Context

- **Owner:** DOM-01 with DOM-05 and integration participating inside this AP.
- **Requirements:** [REQ-041..050](../requirements-catalog.md#e-transmission-networks-and-gateways), REQ-051/054/055 and [REQ-134](../requirements-catalog.md#n-delegated-administration-and-restricted-views) subject to its explicit delivery disposition.
- **Planning references:** [C-01](../contracts/c-01-transmission-acceptance.md), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-12](../contracts/c-12-persistence-operations.md), [F-60..F-68 intake/credential cases](../contracts/reference-cases.md#ap-07-intake-cases), [domain ownership](../architecture.md#domains-and-data-ownership). Add management/timeout cases at those canonical owners.
- **Prerequisites:** [AP-05](../execution/ap-05.md) transaction/primary clock/lifecycle, [identity/AuthZ handover](../execution/source-ap-07.md#handover), [AP-07](../execution/ap-07.md) Gateway credentials/Network replacement/state reports/intake and [AP-08](ap-08-master-data-correction-after-intake.md)'s completed master-data/correction handover.
- **Write scope:** `internal/ingest/`, `internal/protocols/` Network validation, `internal/authz/`/`internal/identity/` Gateway action/scope/proof adapters, `internal/events/`, owner Ent ingest/event/diagnostic stores/schemas/generated output/next migrations and PostgreSQL tests; `cmd/gopager/`, `internal/httpapi/`, versioned management OpenAPI and contact-timeout runner/root registration. C-01/C-06/C-08 and supporting C-12 planning slices/cases/terms are owned here.
- **Later consumers, context only:** [AP-10 Query](ap-10-query-live.md), [AP-11 Delivery](ap-11-events-delivery.md), [AP-15 Frontend](ap-15-frontend.md), [AP-16 Operations](ap-16-operations.md). Their code is not required.

## Planning Before Coding

T086 owns exact Network/Gateway create/read/list/update/deactivate/delete and assigned-Network lifecycle rules: manageable scopes, revision/conflict/receipt semantics, protocol/default validation, credential prerequisites, used/deleted reference behavior, audit and safe management HTTP. Preserve frozen AP-07 credential proof/rotation/replacement and accepted originals; deletion/deactivation must not manufacture accepted input or silently broaden grants.

T086 owns the single-Gateway detail-versus-credential-rotation action/scope contract for REQ-134 if approved for delivery. Obtain its missing delivery disposition as genuine clarification during planning; do not defer implementation to AP-14 or infer Network-only authority satisfies it. Implement additive versioned authorization without modifying frozen prior proof semantics. Detail grants do not imply rotation or alarm Payload access.

T087 owns server-derived diagnostics and state: never-seen versus no retained history, last authenticated contact/heartbeat/reception, degraded/stopped/contact-lost precedence, reason/history projection and allowed fields. Define configurable finite contact timeout, primary-time comparison, concurrent report/reception fencing, durable state transition/fact atomicity, retry/restart and runner lifecycle in C-01/C-08. A stopped Gateway must not silently become contact-lost merely because it stopped intentionally; settle exact precedence from stories/contracts. Use domain-specific store/time checks, not a later generic queue.

T088 owns concrete cases and verification for management denial/hidden references, used Network deletion, credential exclusion, scoped delegation, never-seen, authenticated recovery, timeout boundary, concurrent contact/timeout, duplicate tick/restart, rollback and exactly one transition fact per revision. Bind new cases at C-01/C-06/C-08's canonical F-case owner before coding.

- [ ] T086 [AP-09] Implement production Network/Gateway provisioning, lifecycle/inventory/diagnostic management API and approved single-Gateway delegation.
- [ ] T087 [AP-09] Implement server-derived contact-loss/recovery state, diagnostic history and atomic transition facts with actual runner registration.
- [ ] T088 [AP-09] Verify PostgreSQL/HTTP, current authority, provisioning/reference rules, timed/contact races and restart/rollback.

## Verification and Handover

Run Ent generation, focused ingest/identity/AuthZ/event/command race tests, `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager`, focused vet/build and real command/HTTP runner lifecycle tests. Use existing disposable PostgreSQL; deterministic boundary-time tests complement actual advancing database-clock/restart tests.

Hand over actual versioned management OpenAPI/actions/errors, Network/Gateway read/scope adapters, diagnostic retention markers, canonical `gateway.state-changed` fact lookup/version and executed evidence to AP-10/AP-11/AP-15/AP-16/AP-17. Complete the producer before those APs start; an assignment is not evidence.

**Acceptance:** Gateways/Networks can be provisioned and administered without test seed data; current authority separates diagnostic/credential/data access; server status and history distinguish never contacted, stopped, lost contact and removed history truthfully.

**Non-goals:** Raw decoder, normal Message query/SSE, Delivery fan-out, generic queue, renderer, general identity completion, frontend, HA/retention implementation or hardware timeout SLA.
