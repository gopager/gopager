# AP-10 Query and Live API

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-001..020/027/029/055..060/070/097..100/108/110](../requirements-catalog.md#a-live-message-view), [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md).
- **Owner:** DOM-06 and integration.
- **Architecture:** [domain ownership](../architecture.md#domains-and-data-ownership), [shared use cases](../architecture.md#shared-use-cases), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** [AP-06](../execution/ap-06.md)/[AP-07](../execution/ap-07.md) accepted originals/Processing-ready state, [AP-08](ap-08-master-data-correction-after-intake.md) correction/suppression/default reads/facts and [AP-09](ap-09-gateway-network.md) inventory/diagnostics. Verify their actual handovers and the [identity/fields handover](../execution/source-ap-07.md#handover). Fixed C-07 page/SSE semantics remain inputs. Remaining C-06/C-07/C-08 query/live/saved-view planning belongs here.
- **Write scope:** `internal/readmodel/`, `internal/httpapi/`, `internal/authz/` and `internal/identity/` for query fields/saved-view actions only, `internal/events/` and owner eventstore/readmodel/continuation/resume/saved-view Ent stores/schemas/generated output/next migrations; `cmd/gopager/` HTTP/SSE/subscriber/root/lifecycle registration and versioned REST/OpenAPI artifacts/tests. C-06/C-07/C-08 supporting slices and canonical cases are authorized.

## Planning Before Coding

Record query/live decisions in C-07, fields/actions in C-06 and consumption in C-08 before coding.

- **Later-consumer planning context (not prerequisites):** [AP-11](ap-11-events-delivery.md), [AP-13](ap-13-processing-presentation-integration.md), [AP-14](ap-14-identity-lifecycle.md), [AP-15](ap-15-frontend.md), [AP-17](ap-17-integration-evidence.md).

- [ ] T063 [AP-10] Implement authorized contribution-first query, projection, search/filter, cursor, detail and approved saved-view operations against committed domain state.
- [ ] T064 [AP-10] Implement authorized live updates, interruption visibility, resume/resync, and ongoing revocation behavior.
- [ ] T065 [AP-10] Verify PostgreSQL, HTTP, authorization-before-pagination, locale-independent API values, search admission and live-boundary cases.

## Exact Field, Consumer, Saved-View and Evidence Binding

T063 binds query/detail/filter/order/continuation schema/version/errors, authorization-before-pagination and locale-independent wire values. Settle remaining field-group/filter/operator/option eligibility using [C-06](../contracts/c-06-identity-authorization.md) and [REQ-135..137](../requirements-catalog.md#n-delegated-administration-and-restricted-views), preserving fixed global fields and C-07 Unicode/grouping/time/page bounds. Payload-only and selected-Unit queries cannot expose hidden technical/Unit/Organization fields through filters/options/counts/details.

T063 owns server saved-view planning for [REQ-133/138](../requirements-catalog.md): personal/global ownership, CRUD/delegation actions, revisions, private definition access, serialization/validation, unavailable-field handling, current-rights reopening and management HTTP. Obtain genuine clarification of delivery classification and deferred global visibility; record it in the plan/C-06/C-07 before affected coding. Plan the listed server surfaces and bind finite cases for private access, delegation, hidden-field reopening and the selected visibility policy; do not assume a default. If excluded from delivery, record that disposition and omit those operations. Subscription-preview remains optional.

T064 defines/implements its actual live source consumption using D-94 publication: canonical lookup, unknown-version/missing-ref outcomes, revisions/current rights, durable receipt/resume/work state before transport Ack where required, start/catch-up cutoff, retention/resync and lifecycle registration. Do not require AP-11 Delivery consumers or treat transport offsets as domain idempotency. Bind AP-08 correction and AP-09 Gateway facts and supported Processing changes; later AP-13 may add its presentation participation inside its own scope.

T063/T065 own REQ-108 query/search admission, bounded configurable policy, safe error/retry metadata and shared-IP fairness, with real multi-Principal/multi-tab tests; page/deadline limits do not substitute.

T065 covers F-11 family/F-14/F-30/F-32, Unicode literals/empty terms, same-contribution correlation, stable pagination/cutoff, continuation expiry/revocation, heartbeat/slow consumer/reconnect/removal, authority loss, hidden options/details, refinements REQ-135..137 and accepted personal/global saved-view scenarios. Run focused readmodel/AuthZ/event/HTTP race tests and real PostgreSQL/HTTP/SSE integration, generation/vet/build. Hand over actual query/filter/projection/OpenAPI/error versions, canonical consumer/resume semantics, safe store ports and executed evidence to AP-11..AP-17. Versioned output projection is a current Query need; no speculative host/renderer/Delivery implementation.

**Acceptance:** Counts, ordering, times, fields, and live updates disclose only currently authorized data and remain stable across supported query paths.

**Non-goals:** Browser workflows, separate search services, WebSockets without an approved need, or database-dialect parity claims.
