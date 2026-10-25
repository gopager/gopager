# AP-09 Search, REST, and SSE

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-06](ap-06-transmission-messages.md), [AP-08](ap-08-master-data-correction.md).

**Owner:** DOM-06/integration. **Contract gate:** C-06/C-07/C-08. **REQ:** REQ-001..020, REQ-027, REQ-029, REQ-055..060, REQ-097..100, REQ-108/110. **Prerequisites:** AP-06..AP-08. **Exclusive target scopes:** `internal/readmodel/`, `internal/httpapi/`, versioned REST/OpenAPI artifacts; all transport registration by this integration owner, not concurrently rewritten by other domain owners.

- [ ] T031 [AP-09] Implement contribution-first safe query/field projection, deterministic readable representative/fallback and variant-match details, search/filter options, first-time/cursor operations; verify OpenAPI against responses. Use stable error/status codes and parameters with English fallback.
- [ ] T032 [AP-09] Integrate authorized SSE for multiple views/tabs, visible interruption, safe resume/resync, ongoing revocation.
- [ ] T033 [AP-09] Test F-11..F-14/F-17 on both backends and HTTP/live boundaries; hidden Transmission targets affect neither counts, ordering, nor times. Verify C-07/F-26 language independence at the API boundary.

**Acceptance:** Same-Transmission correlation, authorization before pagination, literal Unicode substrings, safe reconnect, stable codes/values independent of display locale. **Non-goals:** Separate search service or WebSocket without new need. Backend parity and real SSE checks are separate evidence, not browser acceptance.

