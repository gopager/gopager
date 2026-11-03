# AP-09 Search, REST, and SSE

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [live view](../requirements-catalog.md#a-live-message-view), [search/details](../requirements-catalog.md#b-search-filters-and-details), [master-data navigation](../requirements-catalog.md#c-units-organizations-and-defaults), [permissions](../requirements-catalog.md#f-access-rights-and-guests), [i18n/usability](../requirements-catalog.md#j-processing-presentation-and-usability), [security](../requirements-catalog.md#k-installed-web-app-and-security-behavior).
- **Architecture:** [DOM-06 ownership](../architecture.md#domains-and-data-ownership), [query/data dependencies](../architecture.md#allowed-dependencies), [read paths](../architecture.md#time-and-read-paths), [i18n responsibilities](../architecture.md#language-and-internationalization).
- **Owner contracts:** [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md); [shared conventions](../contracts/shared-conventions.md).
- **Cases and prerequisite outputs:** [F-11..F-17/F-26](../contracts/reference-cases.md), [F-30/F-32](../contracts/ap-02-boundary-cases.md); [source AP-06 evidence/Messages](source-ap-06-transmission-messages.md), [source AP-07 AuthZ](source-ap-07-identity-authorization.md), [source AP-08 contributions/correction](source-ap-08-master-data-correction.md), [AP-04 freeze](ap-04-scope-freeze.md).

**Owner:** DOM-06/integration. **Contract gate:** C-06/C-07/C-08. **REQ:** REQ-001..020, REQ-027, REQ-029, REQ-055..060, REQ-097..100, REQ-108/110. **Prerequisites:** AP-06..AP-08. **Exclusive target scopes:** `internal/readmodel/`, `internal/httpapi/`, versioned REST/OpenAPI artifacts; all transport registration by this integration owner, not concurrently rewritten by other domain owners.

- [ ] T031 [AP-09] Implement contribution-first safe query/field projection, deterministic readable representative/fallback and variant-match details, search/filter options, first-time/cursor operations; verify OpenAPI against responses. Use stable error/status codes and parameters with English fallback.
- [ ] T032 [AP-09] Integrate authorized SSE for multiple views/tabs, visible interruption, safe resume/resync, ongoing revocation.
- [ ] T033 [AP-09] Test F-11..F-14/F-17 on PostgreSQL and HTTP/live boundaries; hidden Transmission targets affect neither counts, ordering, nor times. Verify C-07/F-26 language independence at the API boundary. Under [D-32](../authoritative-plan.md#ap-04-scope-decision), preserve backend-neutral expected outcomes for later SQLite parity tests without requiring its initial implementation.

**Acceptance:** Same-Transmission correlation, authorization before pagination, literal Unicode substrings, safe reconnect, stable codes/values independent of display locale. **Non-goals:** Separate search service or WebSocket without new need. Backend parity and real SSE checks are separate evidence, not browser acceptance.
