# Shared Interface Conventions

[Contract index](../domain-contracts.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-01](c-01-transmission-acceptance.md), [C-05](c-05-matching-correction.md), [C-06](c-06-identity-authorization.md), [C-07](c-07-query-live.md), [C-08](c-08-events-jobs.md), [C-11](c-11-processing-presentation.md).

**Version/status:** 1.1 (was 1.0 frozen for AP-05 and the AP-02 value table; the limits and conventions that the code now implements are specified once in `docs/` and OpenAPI, and this file keeps the conventions that still bind planned work plus the limits of work that is not built yet).

## Conventions that bind every contract

The former "AP-05 Foundation Conventions" (context and cancellation, UTC microsecond timestamps, positive 64-bit revisions and fencing tokens, coded platform failures) are merged into this list.

- Each operation identifies context, calling Principal or Gateway, domain input, result and possible errors. Go calls take a cancellable `context.Context`; cancellation and deadlines propagate to store operations, no callback extends its caller's deadline, and `context.Canceled` and `context.DeadlineExceeded` stay distinguishable. Uncertainty after a possible commit is never reduced to a cancellation implying rollback.
- Server entity IDs are opaque UUID strings; the UUID version is not a domain contract. Enduring identity is required for authorization-relevant entities (Principals, Units, Gateways, Networks, Agencies), not for Message or read-model continuity (D-40): Message references may be replaced on merge or recomputation without old-to-new mapping, but a reference must identify its intended object while valid and is never reassigned to unrelated data. Addresses and Functions are Protocol data, not server IDs. Submission IDs are Gateway-owned opaque strings.
- Observation time, Acceptance time, rule validity time, internal Message time, the per-(Message, Unit) age anchor and the first-appearance time of a Unit are distinct. Grouping uses each offset-bearing Observation instant and never Unit matching.
- Mutable configuration carries a visible revision and needs an expected state for conflicts. Immutable Transmission evidence is never corrected by updating it; derived data is versioned separately.
- Authentication is not authorization. Administrative, guest, Gateway, API-key, worker and plugin paths need explicit capabilities. A technical worker is not a global data reader for outbound Payloads. Subscribing to a Domain event type never grants an unprojected original Payload, and field permissions stay bound to the domain data context.
- Errors use the code and envelope of [HTTP API: Errors](../../docs/http-api.md#errors); other users' IDs, secrets and internal SQL or plugin errors never appear in public details.
- English is the authoritative interface and documentation language. Stable machine-readable error and status codes and structured parameters are independent of display text; the English fallback is human-readable only and clients never parse it. UI language and formatting locale change no ID, field name, enum or event value, wire format or authorization.
- Atomicity applies within the authoritative database, not to browser state or external providers. Data and the required Domain event or work commit together; intermediate derivation is never reported as complete state.
- Idempotency is operation specific; it replaces neither authorization nor the distinction of independent Transmissions. Retention and ordinary restart must not regenerate old work. Backup and restore are outside the application ([Operations](../../docs/operations.md#backup-and-restore-responsibility)).
- Store-generated operational timestamps are UTC with microsecond precision, serialized as RFC 3339 with `Z`; authority and lease checks read the primary store's current time. Store and configuration revisions, migration versions and fencing tokens are positive signed 64-bit integers, checked before increment, with exhaustion reported as `RESOURCE_LIMIT`; JSON uses decimal strings (see [HTTP API: Revisions](../../docs/http-api.md#revisions-and-optimistic-concurrency)).
- Platform failures carry a stable code, safe structured parameters, an English fallback and a wrapped private cause; DSNs, credentials, SQL text with values and private callback data never appear in public parameters or logs ([Architecture: Errors and failure codes](../../docs/architecture.md#errors-and-failure-codes)).
- **Uniform interface:** a producer supplies authenticated context plus schema version and domain input; the owning domain validates current C-06 permissions and expected revision, commits domain changes with the C-08 facts and work atomically and returns reference, revision and status or a coded error. Consumers use immutable references and current authorized projections, never foreign table writes. Reads are side-effect free. Side-effect retries use their operation identity; internal consumers deduplicate by Domain event ID plus consumer identity.
- **Revision checks:** one authoritative access and configuration revision plus the revisions of the affected scope, in ordinary primary-store transactions. Unchanged revisions permit reuse of prepared safe projections; changed relevant revisions require re-evaluation. There is no distributed per-field consensus or historical permission engine, and revisions are internal proof, not public counters leaking hidden changes. Instances that lose current authority pause private output and attempts; already handed-off effects are not recallable.

## AP-02 Common Schemas and Limits

### Limits specified elsewhere

The following AP-02 limits are implemented and are stated once in the linked page. Exceeding a limit is `RESOURCE_LIMIT` or `INVALID_INPUT`, never truncation or partial success; configuration may lower a resource limit and raising one is an explicit operator change.

| Limit | Where it is specified |
| --- | --- |
| JSON transport, `schemaVersion`, UTF-8, RFC 3339 timestamps, request and body sizes per route group, nesting, collection sizes | [HTTP API: Basics](../../docs/http-api.md#basics), [Request size limits](../../docs/http-api.md#request-size-limits), [Time, IDs and numbers](../../docs/http-api.md#time-ids-and-numbers) |
| Submission size, target count, `submissionId` and Gateway Network identifier length, required and optional fields | [HTTP API: Ingest, Gateway credential](../../docs/http-api.md#ingest-gateway-credential) and OpenAPI |
| Page defaults and maximum, filter list size | [HTTP API: Pagination](../../docs/http-api.md#pagination) |
| Ordinary call and transaction deadline | [Architecture: Transactions and the store abstraction](../../docs/architecture.md#transactions-and-the-store-abstraction) |
| Plugin process, input, output and deadline limits | [Plugins: Limits](../../docs/plugins.md#5-limits) |
| Workflow step, nesting, collection, regex, input and compute limits | [Workflows: Limits and defaults](../../docs/workflows.md#limits-and-defaults) |
| Job lease, renewal and fencing for Processing and correction | [Workflows: Worker dispatch](../../docs/workflows.md#worker-dispatch), [Architecture: Historical correction](../../docs/architecture.md#historical-correction-worker-role) |
| Command receipts and `expectedRevision` | [HTTP API: Command IDs](../../docs/http-api.md#command-ids-idempotent-creation) and [Revisions and optimistic concurrency](../../docs/http-api.md#revisions-and-optimistic-concurrency) |

### Limits of work that is not built yet

| Boundary | Version 1 value |
| --- | --- |
| Gateway state (AP-09) | Expected heartbeat every 60 s; contact lost after 180 s without an authenticated heartbeat or Submission. Both are configurable positive durations. Explicit termination is distinct and never-seen is distinct. Degraded records the reported failure or processing problem; successful new contact ends contact-lost or stopped without inventing a cause. Last Transmission and last heartbeat stay separate. |
| Jobs of new job families (C-08) | Lease 60 s, renewed every 20 s; every state write needs the current fencing token and an unexpired lease; the authority clock is the primary store's clock, not a worker's wall clock. |
| Create-with-side-effects and bulk commands for future routes | Need a caller-bound idempotency reference retained while dependent work exists (at least 24 h); after expiry an expired-command conflict is returned for known expired commands and an unrecorded external effect is never pretended absent. |
| Delivery, retention, recovery and SSE | Their numeric bounds are in C-07, C-08, C-09 and C-12 until implemented. |

# Freeze Requirements

This is the shared contract-completeness checklist: concrete data/error/version forms, named ownership, producer/consumer agreement, current permission/field projection, time/revision, idempotency/retention, transaction/failure boundaries, and concrete positive/negative fixtures. Include language-independent codes and English text/parameter mapping for C-07/C-11 without requiring additional translations.

Open-issue status and approval conditions are owned by the [plan](../authoritative-plan.md#open-issues-and-risks); [AP-04](../work-packages/ap-04-scope-freeze.md) owns the freeze tasks and machine-readable artifact handover. Satisfying this checklist alone grants no approval.
