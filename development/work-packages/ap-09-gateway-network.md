# AP-09 Gateway and Network Administration

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md) | [Approval](../authoritative-plan.md#workflow-and-approval-gates)

## Session Context

- **Requirements:** [REQ-041..050](../requirements-catalog.md#e-transmission-networks-and-gateways), server side (screens: AP-15); for REQ-049 this AP produces the Gateway state facts, Delivery triggers on them are AP-11. [REQ-134](../requirements-catalog.md#n-delegated-administration-and-restricted-views) single-Gateway delegation, server side, if the start decision includes it (see Open User Decisions).
- **Inputs:**
  - [AP-05 handover](../execution/ap-05.md#handover): transaction, primary clock and lifecycle.
  - [Identity/AuthZ handover](../execution/source-ap-07.md#handover): identity and AuthZ results.
  - [AP-07 handover](../execution/ap-07.md#handover): Gateway credentials, Network replacement, state reports and intake.
  - [AP-08 handover](../execution/ap-08.md#handover): master data, Agencies, correction.
- **Owned contract slices:** [C-01](../contracts/c-01-transmission-acceptance.md), [C-06](../contracts/c-06-identity-authorization.md) and [C-08](../contracts/c-08-events-jobs.md) planning slices, cases and terms for Network/Gateway management and contact-timeout state, plus the supporting [C-12](../contracts/c-12-persistence-operations.md) planning slices. Add management/timeout cases at those canonical owners.
- **Reference (read when needed):** [F-60..F-68 intake/credential cases](../contracts/reference-cases.md#ap-07-intake-cases), [domain ownership](../architecture.md#domains-and-data-ownership), [D-135](../decisions/10-foundation-redesign.md#d-135).
- **Write scope:**
  - `internal/ingest/` — Network/Gateway provisioning and state logic.
  - `internal/protocols/` — Network validation only.
  - `internal/authz/`, `internal/identity/` — Gateway action/scope/proof adapters.
  - `internal/events/` — Gateway state facts.
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/ingeststore/`, `internal/persistence/ent/eventstore/`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout)) — Network/Gateway/diagnostic persistence and PostgreSQL tests.
  - `internal/httpapi/` — handlers and the versioned management OpenAPI.
  - `cmd/gopager/` — contact-timeout runner and root registration only (composition, no HTTP adapters).
- **Compatibility context:** [AP-10 Query](ap-10-query-live.md), [AP-11 Delivery](ap-11-events-delivery.md), [AP-15 Frontend](ap-15-frontend.md), [AP-16 Operations](ap-16-operations.md). Their code is not required.

## Open User Decisions

- REQ-134 single-Gateway delegation: is it delivered in this AP? Asked together with the start approval; the answer is recorded in [Delivery scope](../authoritative-plan.md#delivery-scope) (T086).

## Planning Before Coding

T086 owns exact Network/Gateway create/read/list/update/deactivate/delete and assigned-Network lifecycle rules: manageable scopes, revision/conflict/receipt semantics, protocol/default validation, credential prerequisites, used/deleted reference behavior, audit and safe management HTTP. Network provisioning keeps the C-01 resolution order and the Gateway's Network priority order (`0..n-1`, lower first): declared identifier, the only Network, the one Network whose Address rules match at the Observation time, the highest-priority matching Network (with the `operation.network-ambiguous` fact), and the highest-priority Network when none matches ([D-135](../decisions/10-foundation-redesign.md#d-135) restored address-based inference; there is no default flag). Preserve frozen AP-07 credential proof/rotation/replacement and accepted originals; deletion/deactivation must not manufacture accepted input or silently broaden grants. Destination: C-01/C-06/C-08 and supporting C-12 sections.

T086 owns the single-Gateway detail-versus-credential-rotation action/scope contract for REQ-134 if the start decision includes it; do not defer implementation to AP-14 or infer Network-only authority satisfies it. Implement additive versioned authorization without modifying frozen prior proof semantics. Detail grants do not imply rotation or alarm Payload access. Destination: C-06 and [Delivery scope](../authoritative-plan.md#delivery-scope).

T087 owns server-derived diagnostics and state: never-seen versus no retained history, last authenticated contact/heartbeat/reception, degraded/stopped/contact-lost precedence, reason/history projection and allowed fields. Define in C-01/C-08 the runner's inputs and role: the Operator-configured finite contact timeout (validated at startup, never shorter than the Gateway heartbeat interval), the scan interval, the per-Gateway last authenticated contact/heartbeat/reception times and stopped marker, and the database primary clock for every comparison; the runner runs in the `worker` role (one active evaluation per Gateway through the revision check, so several workers are safe) and not in `api`. Also define concurrent report/reception fencing, durable state transition/fact atomicity, retry/restart and runner lifecycle. A stopped Gateway must not silently become contact-lost merely because it stopped intentionally; settle exact precedence from stories/contracts. Use domain-specific store/time checks, not a later generic queue.

T088 owns concrete cases and verification for management denial/hidden references, used Network deletion, credential exclusion, scoped delegation, never-seen, authenticated recovery, timeout boundary, concurrent contact/timeout, duplicate tick/restart, rollback and exactly one transition fact per revision. Bind new cases at C-01/C-06/C-08's canonical F-case owner before coding.

## Tasks

- [ ] T086 [AP-09] Implement production Network/Gateway provisioning, lifecycle/inventory/diagnostic management API and approved single-Gateway delegation.
  - **Done when:** Gateways/Networks can be provisioned and administered without test seed data; Network/Gateway create/read/list/update/deactivate/delete and assigned-Network lifecycle follow the planned rules; management denial, hidden references, used Network deletion and credential exclusion hold; REQ-134 delegation is implemented as approved with current authority separating diagnostic, credential and data access.
- [ ] T087 [AP-09] Implement server-derived contact-loss/recovery state, diagnostic history and atomic transition facts with actual runner registration.
  - **Done when:** server status and history distinguish never contacted, stopped, lost contact and removed history truthfully; the contact-timeout runner is registered in the `worker` role; each revision produces exactly one transition fact atomically; a stopped Gateway does not become contact-lost merely because it stopped.
- [ ] T088 [AP-09] Verify PostgreSQL/HTTP, current authority, provisioning/reference rules, timed/contact races and restart/rollback.
  - **Done when:** the cases for management denial/hidden references, used Network deletion, credential exclusion, scoped delegation, never-seen, authenticated recovery, timeout boundary, concurrent contact/timeout, duplicate tick/restart, rollback and exactly one transition fact per revision exist at their canonical owners and pass with the Verification evidence.

## Documentation to Update

- Development owners: [C-01](../contracts/c-01-transmission-acceptance.md), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md) and supporting [C-12](../contracts/c-12-persistence-operations.md) sections; new F-cases in [reference cases](../contracts/reference-cases.md); [glossary](../glossary.md#maintenance-rules) for new terms; [Delivery scope](../authoritative-plan.md#delivery-scope) for the REQ-134 disposition.
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md#gateway-administration) (Gateway administration, Authorization, Gateway authentication).
- [docs/operations.md](../../docs/operations.md#configuration-reference) (contact timeout, scan interval) and [Roles](../../docs/operations.md#roles) (runner in `worker`).
- [docs/architecture.md](../../docs/architecture.md#end-to-end-data-flow) and [Domain events and outbox](../../docs/architecture.md#domain-events-and-outbox) for Gateway state facts and the runner; Go doc comments of changed exported identifiers.

## Verification

Run Ent generation, focused ingest/identity/AuthZ/event/command race tests, `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager`, focused vet/build and real command/HTTP runner lifecycle tests. Use existing disposable PostgreSQL; deterministic boundary-time tests complement actual advancing database-clock/restart tests.

## Handover

Hand over actual versioned management OpenAPI/actions/errors, Network/Gateway read/scope adapters, diagnostic retention markers, canonical `gateway.state-changed` fact lookup/version and executed evidence to AP-10/AP-11/AP-15/AP-16/AP-17. Complete the producer before those APs start; an assignment is not evidence.

## Non-goals

Raw decoder, normal Message query/SSE, Delivery fan-out, generic queue, renderer, general identity completion, frontend, HA/retention implementation or hardware timeout SLA.
