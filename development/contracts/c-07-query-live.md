# C-07 REST Queries, Search and Live Updates

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-03](c-03-message-grouping.md), [C-06](c-06-identity-authorization.md), [C-11](c-11-processing-presentation.md).

**Implementation planning:** [AP-10](../work-packages/ap-10-query-live.md) delivered Query/SSE, supporting fields, consumption, admission and saved views. [AP-15](../work-packages/ap-15-frontend.md) consumes them.

**Traceability:** [REQ-001..010](../requirements-catalog.md#a-live-message-view), [REQ-011..020](../requirements-catalog.md#b-search-filters-and-details), [REQ-027/029](../requirements-catalog.md#c-units-organizations-and-defaults), [REQ-055..060](../requirements-catalog.md#f-access-rights-and-guests), [REQ-097..100](../requirements-catalog.md#j-processing-presentation-and-usability). Issue references: [P-08..P-10/P-12/P-15](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Status:** Query, live updates, saved views and search admission are implemented by AP-10 and specified once in `docs/` and OpenAPI; the remaining error codes are planned. **Owner:** Query; integration owns OpenAPI and the Domain event boundary; C-06 exclusively owns permission semantics. **Parties:** Vue and API consumers receive authorized projections, not internal database objects.

## Implemented behavior (specified in `docs/` and OpenAPI)

The HTTP error envelope, code table with status and `retryable`, retry semantics of `REVISION_CONFLICT`, the `RETRY_LIMIT` correction failure code, the safe `fallback` texts and `textKey` mapping, and the shared list and paging conventions of the management routes: [HTTP API: Errors](../../docs/http-api.md#errors), [Conventions](../../docs/http-api.md#conventions) and the `Error` schema of [OpenAPI v1](../../api/openapi/v1.yaml) (`info.version` 1.6.0). Platform failure classification and the PostgreSQL SQLSTATE mapping: [Architecture: Errors and failure codes](../../docs/architecture.md#errors-and-failure-codes).

AP-10 Query, live and saved views (implemented 2026-11-08, [AP-10 record](../execution/ap-10.md#handover)): appearance eligibility, field groups, filters, PostgreSQL text search, order, HMAC keyset page token, snapshot cutoff `position`, projections (Representative Transmission, Unit summaries, encrypted label, correction and processing status, target resolutions, derived results, `networkUnresolved`) and filter options: [HTTP API: Messages: query and detail](../../docs/http-api.md#messages-query-and-detail); positions, frames, fact handling, bounds, replay window and resync: [Messages: live stream](../../docs/http-api.md#messages-live-stream); [Saved views](../../docs/http-api.md#saved-views); [Search rate limiting](../../docs/http-api.md#search-rate-limiting); keyset paging: [Pagination](../../docs/http-api.md#pagination); data flow: [Architecture: Query](../../docs/architecture.md#query-api-role); flags `--search-attempts-per-minute`, `--live-replay-window`: [Operations](../../docs/operations.md#configuration-reference). Cases: [AP-10 query and live cases](reference-cases.md#ap-10-query-and-live-cases).

Binding rules for every response: codes are stable and language independent; Product-generated outcomes carry `error.<CODE>` or `status.<status>` text keys with a redacted English fallback and structured parameters; no client parses prose; UI language and formatting locale change no ID, field name, enum, timestamp, token or authorization outcome; invisible, deleted or invalidated references share one `NOT_FOUND` shape and never expose a replacement ID.

## Planned and not yet implemented

**Error codes not implemented yet** (added by their owning AP together with the English fallback text and the allowed parameters): `DEPENDENCY_UNAVAILABLE` (409, safe dependency kind), `CAPABILITY_UNDECLARED` (403, permitted capability name) and `EXTERNAL_UNCERTAIN` (409, own attempt reference). Plugin handshake and transport failures are `DEPENDENCY_UNAVAILABLE` and a crash is `INTERNAL_ERROR` once those codes exist. Outcome statuses map to `status.<status>`. C-11 owns the text-key registry. Resync is the live `resync-required` event, not an HTTP error code.
