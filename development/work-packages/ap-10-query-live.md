# AP-10 Query and Live API

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** server side of the following (screens: AP-15):
  - [REQ-001..019/027/029/098](../requirements-catalog.md#a-live-message-view) query, live view, search and details.
  - [REQ-055..060/070](../requirements-catalog.md#f-access-rights-and-guests): enforcement of field-group, unassigned-Message, guest and session-revocation rules in query and live results; managing grants and sessions is AP-14.
  - [REQ-035/040/123/124](../requirements-catalog.md#d-correct-assignment-and-history): query-side presentation of the AP-08 assignment state — correction in progress versus completed, undecidable/failed assignment distinct from "no matching Unit", suppressed Transmission targets absent from views, search, details and counts, the encrypted-assignment label; unassigned traffic visible per Agency ([D-135](../decisions/10-foundation-redesign.md#d-135) item 5).
  - REQ-108 query/search admission (login: AP-14, sending: AP-11, provider limits: AP-12).
  - [REQ-133/138](../requirements-catalog.md#n-delegated-administration-and-restricted-views) saved views (see Open User Decisions) and REQ-135..137 field/query refinements.
- **Inputs:**
  - [AP-06 handover](../execution/ap-06.md#handover): Processing results and `processing.completed`.
  - [AP-07 handover](../execution/ap-07.md#handover): accepted originals, `processing_status`, `message_units` cache.
  - [AP-08 handover](../execution/ap-08.md#handover): correction, suppression, Agencies, default reads, facts.
  - [Identity/fields handover](../execution/source-ap-07.md#handover): identity and field results.
  - [AP-09](ap-09-gateway-network.md): inventory/diagnostics; verify `development/execution/ap-09.md#handover` exists at start.
  - Implemented C-07 error envelope and paging conventions ([C-07 implemented behavior](../contracts/c-07-query-live.md#implemented-behavior-specified-in-docs-and-openapi)) are fixed.
- **Owned contract slices:** [C-06](../contracts/c-06-identity-authorization.md) query field and saved-view actions, the [planned C-07 section](../contracts/c-07-query-live.md#planned-and-not-yet-implemented-ap-10) (query/live/saved-view) and [C-08](../contracts/c-08-events-jobs.md) (consumption) planning slices and canonical cases. This AP builds the event-consumer framework (gap-safe consumers on the sequenced `domain_events` log: reader port, `(tx_id, seq)` read position, `consumer_offsets`, wake-up by notification, lifecycle registration); AP-11 consumes it.
- **Reference (read when needed):** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope), [approval](../authoritative-plan.md#workflow-and-approval-gates), [domain ownership](../architecture.md#domains-and-data-ownership), [shared use cases](../architecture.md#shared-use-cases), [allowed dependencies](../architecture.md#allowed-dependencies), [D-135](../decisions/10-foundation-redesign.md#d-135).
- **Write scope:**
  - `internal/readmodel/` (new) — query, projection and read model.
  - `internal/httpapi/` — HTTP/SSE handlers and versioned REST/OpenAPI artifacts and tests.
  - `internal/authz/`, `internal/identity/` — query fields and saved-view actions only.
  - `internal/events/` — event-consumer framework.
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/eventstore/`, new read-model and saved-view stores under `internal/persistence/ent/`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout)).
  - `cmd/gopager/` — subscriber/root/lifecycle registration only.
- **Compatibility context:** [AP-11](ap-11-events-delivery.md), [AP-13](ap-13-processing-presentation-integration.md), [AP-14](ap-14-identity-lifecycle.md), [AP-15](ap-15-frontend.md), [AP-17](ap-17-integration-evidence.md) (not prerequisites).

## Open User Decisions

- REQ-133/138 saved views: is the feature delivered, and what is the global visibility policy? Asked together with the start approval; do not assume a default. The answer is recorded in [Delivery scope](../authoritative-plan.md#delivery-scope) and in C-06/C-07 before affected coding (T063). If excluded from delivery, record that disposition and omit those operations.

## Planning Before Coding

Record query/live decisions in C-07, fields/actions in C-06 and consumption in C-08 before coding.

T063 binds query/detail/filter/order/page-token schema/version/errors. Ordering and time filters use the stored Message time (the earliest observation of any member); the page token is a stateless HMAC keyset token on `(first_observed_at, message_id)`; per-Unit first times appear in details and age limits use the per-(Message, Unit) anchor (D-135 replaced the per-query View time and server-side continuation state). Filters by Agency replace Network filters; Radio network and Protocol stay technical dimensions. Field filters are validated against the author's readable field groups at save time. T063 also binds authorization-before-pagination and locale-independent wire values. Settle remaining field-group/filter/operator/option eligibility using [C-06](../contracts/c-06-identity-authorization.md) and [REQ-135..137](../requirements-catalog.md#n-delegated-administration-and-restricted-views), preserving fixed global fields and C-07 Unicode/grouping/time/page bounds. Payload-only and selected-Unit queries cannot expose hidden technical/Unit/Organization fields through filters/options/counts/details. Bind the AP-08 assignment state into the projection: suppressed Transmission targets are excluded from views, search, details, counts and live updates (REQ-123); a correction in progress is distinguishable from completed (REQ-035); an undecidable or failed assignment is distinct from "no matching Unit" (REQ-040); the encrypted-assignment label is shown without implying decryption (REQ-124); unassigned traffic is visible to the Agency owning the resolved Network and only with the explicit "without Unit" grant (REQ-056, D-135 item 5).

T063 owns server saved-view planning for [REQ-133/138](../requirements-catalog.md): personal/global ownership, CRUD/delegation actions, revisions, private definition access, serialization/validation, unavailable-field handling, current-rights reopening and management HTTP. The delivery classification and global visibility policy come from the start decision (Open User Decisions); record them in the plan and C-06/C-07 before affected coding. Plan the listed server surfaces and bind finite cases for private access, delegation, hidden-field reopening and the selected visibility policy; do not assume a default. If excluded from delivery, record that disposition and omit those operations.

T064 defines/implements its actual live source consumption on the sequenced `domain_events` log: the stream request carries the client's loaded Message-time range (`from`, optional `until`) and the page's `asOfSeq`, and the server filters pushed events by the `messageTime`/`previousMessageTime` they carry before the view filter (REQ-005, no live-age window); the client reopens the stream with an extended range after loading a further page; resume is by event sequence; canonical lookup, unknown-version/missing-ref outcomes, revisions/current rights, durable receipt/resume/work state before transport Ack where required, start/catch-up cutoff, retention/resync and lifecycle registration. Do not require AP-11 Delivery consumers or treat consumer offsets as domain idempotency. Bind AP-08 correction and AP-09 Gateway facts and supported Processing changes (`processing.completed`); later AP-13 may add its presentation participation inside its own scope.

T063/T065 own REQ-108 query/search admission, bounded configurable policy, safe error/retry metadata and shared-IP fairness, with real multi-Principal/multi-tab tests; page/deadline limits do not substitute.

## Tasks

- [ ] T063 [AP-10] Implement authorized Message-time ordered query, projection, search/filter, stateless page token, detail and saved-view operations against committed domain state.
  - **Done when:** query/detail/filter/order/page-token schema, version and errors are bound; authorization applies before pagination and wire values are locale-independent; hidden technical/Unit/Organization fields are not exposed through filters, options, counts or details; saved-view operations follow the recorded disposition (or are omitted if excluded); suppressed targets, correction state, failed assignments, the encrypted label and per-Agency unassigned visibility behave as planned; REQ-108 query/search admission is in place.
- [ ] T064 [AP-10] Build the event-consumer framework and implement authorized live updates (loaded-range stream filter), interruption visibility, resume/resync, and ongoing revocation behavior.
  - **Done when:** the framework is registered in the lifecycle; the stream filters by loaded range and resumes by event sequence; unknown-version/missing-ref outcomes, retention/resync, interruption visibility and authority loss behave as planned in C-08; AP-08 correction, AP-09 Gateway and `processing.completed` facts are bound.
- [ ] T065 [AP-10] Verify PostgreSQL, HTTP, authorization-before-pagination, locale-independent API values, search admission and live-boundary cases.
  - **Done when:** F-11 family/F-14/F-30/F-32, Unicode literals/empty terms, same-contribution correlation, stable pagination/cutoff, page-token tampering/revocation, loaded-range filtering and extension, heartbeat/slow consumer/reconnect/removal, authority loss, hidden options/details, refinements REQ-135..137 and accepted personal/global saved-view scenarios pass, with real multi-Principal/multi-tab admission tests; counts, ordering, times, fields and live updates disclose only currently authorized data and remain stable across supported query paths.

## Documentation to Update

- Development owners: [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md); cases in [reference cases](../contracts/reference-cases.md); [glossary](../glossary.md#maintenance-rules) for new terms; [Delivery scope](../authoritative-plan.md#delivery-scope) for the REQ-133/138 disposition.
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md#route-reference) (new query, SSE and saved-view section; [Pagination](../../docs/http-api.md#pagination); [Authorization](../../docs/http-api.md#authorization)).
- [docs/architecture.md](../../docs/architecture.md#domain-events-and-outbox) for the consumer framework and live data flow; [docs/operations.md](../../docs/operations.md#configuration-reference) for query admission configuration; Go doc comments of changed exported identifiers.

## Verification

Run focused readmodel/AuthZ/event/HTTP race tests and real PostgreSQL/HTTP/SSE integration, generation/vet/build.

## Handover

Hand over actual query/filter/projection/OpenAPI/error versions, canonical consumer/resume semantics, safe store ports and executed evidence to AP-11..AP-17. Versioned output projection is a current Query need; no speculative host/renderer/Delivery implementation.

## Non-goals

Browser workflows, separate search services, WebSockets without an approved need, database-dialect parity claims, or the Subscription preview (a planning candidate in the [requirements catalog](../requirements-catalog.md), not a requirement).
