# AP-09 Query and Live API

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-09 T031..T033](source-ap-09-query-live.md).
- **Requirements and owners:** [REQ-001..020/027/029/055..060/070/097..100/108/110](../requirements-catalog.md#a-live-message-view), [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md).
- **Owner:** DOM-06 and integration.
- **Prerequisites:** AP-06, AP-07, and AP-08; applicable AP-04 freezes and this slice's exact execution binding.
- **Write scope:** `internal/readmodel/`, `internal/httpapi/`, versioned REST/OpenAPI artifacts, and focused tests; transport registration stays with the assigned integration owner.

- [ ] T063 [AP-09] Implement authorized contribution-first query, projection, search/filter, cursor, and detail operations against the committed domain state.
- [ ] T064 [AP-09] Implement authorized live updates, interruption visibility, resume/resync, and ongoing revocation behavior.
- [ ] T065 [AP-09] Verify PostgreSQL, HTTP, authorization-before-pagination, locale-independent API values, and live-boundary cases.

**Acceptance:** Counts, ordering, times, fields, and live updates disclose only currently authorized data and remain stable across supported query paths.

**Non-goals:** Browser workflows, separate search services, WebSockets without an approved need, or database-dialect parity claims.
