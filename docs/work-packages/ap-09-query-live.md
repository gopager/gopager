# AP-09 Query and Live API

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-09 T031..T033](source-ap-09-query-live.md).
- **Requirements and owners:** [REQ-001..020/027/029/055..060/070/097..100/108/110](../requirements-catalog.md#a-live-message-view), [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md).
- **Owner:** DOM-06 and integration.
- **Architecture:** [domain ownership](../architecture.md#domains-and-data-ownership), [shared use cases](../architecture.md#shared-use-cases), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verified AP-06, AP-07 and AP-08 handovers; only applicable contract/decision slices already fixed by earlier work. Any C-07 detail needed by T063-T065 that is not already fixed is planned and defined in this AP, not treated as an external freeze prerequisite.
- **Write scope:** `internal/readmodel/`, `internal/httpapi/`, versioned REST/OpenAPI artifacts, and focused tests; transport registration stays with the assigned integration owner.

## Planning Before Coding

Complete this planning as the first part of T063-T065; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Record contract decisions in C-07 and case expectations at their F-case owners; track the work within those existing task IDs:

- **Later-consumer planning context (not prerequisites):** [AP-10 Events and Delivery](ap-10-events-delivery.md), [AP-14 Frontend](ap-14-frontend.md), and [AP-16 Integration and Delivery Evidence](ap-16-integration-evidence.md). Use these to keep event/live boundaries, API semantics and evidence traceable for their consumers; do not implement delivery, browser workflows or final delivery evidence here.
- Define the authorized query and detail surface: filters, ordering, cursor shape/version/expiry, field projection, stable errors and bounds, including authorization-before-pagination and locale-independent wire values.
- Define the live-update contract: event contents, ordering/sequence, resume and resync behavior, interruption visibility, current-rights revalidation/revocation, and the HTTP/SSE error and reset boundaries.
- Bind the API surface to the already fixed domain states and assigned F-cases. Identify API/OpenAPI version ownership and transport-registration handover; do not depend on frontend work or add a separate search service.

- [ ] T063 [AP-09] Implement authorized contribution-first query, projection, search/filter, cursor, and detail operations against the committed domain state.
- [ ] T064 [AP-09] Implement authorized live updates, interruption visibility, resume/resync, and ongoing revocation behavior.
- [ ] T065 [AP-09] Verify PostgreSQL, HTTP, authorization-before-pagination, locale-independent API values, and live-boundary cases.

**Acceptance:** Counts, ordering, times, fields, and live updates disclose only currently authorized data and remain stable across supported query paths.

**Non-goals:** Browser workflows, separate search services, WebSockets without an approved need, or database-dialect parity claims.
