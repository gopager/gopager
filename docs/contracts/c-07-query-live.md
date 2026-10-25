# C-07 REST, Search, Details, and Live Updates

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-03](c-03-message-grouping.md), [C-06](c-06-identity-authorization.md), [C-11](c-11-processing-presentation.md).

**Traceability:** [REQ-001..010](../requirements-catalog.md#a-live-message-view), [REQ-011..020](../requirements-catalog.md#b-search-filters-and-details), [REQ-027/029](../requirements-catalog.md#c-units-organizations-and-defaults), [REQ-055..060](../requirements-catalog.md#f-access-rights-and-guests), [REQ-097..100](../requirements-catalog.md#j-processing-presentation-and-usability). Issue references: [P-08..P-10/P-12/P-15](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft. **Owner:** DOM-06; integration owns OpenAPI/Domain event boundary; C-06 exclusively owns permission semantics. **Parties:** Vue/API consumers receive authorized projections, not internal database objects.

Draft operations: `QueryMessages(Principal, filters, timeAnchor, cursor) -> page`; `GetMessageDetail(Principal, MessageRef) -> projection`; `ListFilterOptions(Principal, context) -> safeOptions`; `OpenLiveView(Principal, viewDefinition, resumeState) -> authorizedChangesOrResync`; `ResolveMessageAlias(Principal, MessageRef) -> permitted surviving MessageRef or NOT_FOUND`. Concrete endpoint paths/OpenAPI remain AP-09 after AP-04; wire semantics below are complete documentary drafts.

**Query semantics:** OR within a dimension, AND between dimensions on the same permitted Message contribution (direct Unit). Network A on one contribution cannot combine with Unit B only found on another. 'Without Unit' requires proven unassigned/targetless authorized evidence, never hidden or failed resolution. Normal Message queries do not accept a Gateway filter; authorized reception diagnostics provide that separately.

**Text search:** Literal substrings, case-insensitive, Unicode-consistent, accent-sensitive. Empty search text applies no text filter; `.*` or `%` are user text, not regex/SQL wildcards. Only readable Payload may produce matches. Validate Unicode form/case folding and correct short-term index strategy across backends.

**Projection before result formation:** Apply suppression, current grants, and field permissions before filtering/counting/pagination/details. Invisible Transmission targets affect neither Gateway/Unit names nor counts, time anchors, sorting, facets, or change indicators. Message evidence grants no general access to other Transmission targets. Permitted detail actions may prefill Addresses; no blanket management action in the compact list.

**Live:** New/changed permitted Messages update loaded views. Interruptions are visible; reconnect also reconciles deleted/revoked entries. Multiple tabs/filters concurrently, no one-connection-per-IP assumption. Keep loaded history current with reasonable effort; a necessary live-age limit must be visible and reconciled on redisplay. Two hours is not mandatory. Resume does not prove unchanged permissions; safely resynchronize if resume state is insufficient.

**Revocation/failures:** No further private output under revoked sessions/stale permissions. Safe invalidation may Address IDs already known to that consumer without exposing other objects. Payload already sent to a browser is not promised recallable. Unknown/invisible objects must not become an existence oracle.

**Language/i18n boundary:** Product-generated status, error, and validation outcomes use stable machine-readable codes and structured parameters, documented in English; their user-facing wording is Localized text drawn from the frontend text catalog, with an English fallback and no reliance on parsing server prose. Keep Protocol data, operator names, field names, enum values, timestamps, cursor semantics, and search/authorization behavior independent of UI language. User formatting locale is not an API translation switch. Exact code/parameter catalog and fallback mapping belong to AP-02/AP-09/AP-13; no second translation catalog is required.

## AP-02 Query, Time and Resume Schemas

### Time Selection and One-row Message Results

**Decision:** Store `messageTime` for internal origin/whole-Message retention and maintain `contributionTime`/`contributionAgeAnchor` on Message contributions. Derive `appearanceTime` and `viewTime` per query. Never write a user's filter-dependent time back to Message. There is no universal public timestamp exception for hidden evidence. These semantic names do not freeze SQL column layout.

The same query algorithm applies with and without a Unit filter:

1. Select current permitted Message contributions satisfying all non-time filters on that same contribution (Network, Protocol, Address, direct Unit/Organization/Unit collection). Derivation excludes suppressed supports. Pending/failed/ambiguous resolution is not silently unassigned.
2. Group contributions per `(Message, Unit)` into Unit appearances. `appearanceTime` is the minimum permitted Contribution time; its age anchor is the corresponding scoped Contribution age anchor. Several Addresses still yield one first Unit appearance. A proven unassigned contribution uses its own ID as the unassigned appearance key; do not invent a Unit.
3. Apply the optional permitted age and requested `[from, until)` interval to these appearances. Determine the first appearance **before** applying the time interval; a later Address of the same Unit must not masquerade as that Unit's first appearance. Age scope is evaluated using the matching grant/Network context before combining authorized scopes; a broader query cannot grant evidence forbidden in a narrower scope.
4. Apply literal Payload search only to readable Payload/allowed input context, retain matching appearances, and group the remaining appearances by Message. Return exactly one Message row with `viewTime = min(remaining appearanceTime)`. Multiple selected Units are alternatives: one qualifying Unit appearance suffices; an earlier selected Unit outside the interval does not exclude a different selected Unit inside it.
5. Order descending by `(viewTime, Message ID)` with stable UUID-string tie-break, then paginate. The row's primary displayed time and cursor key are that same `viewTime`, never a hidden/global Message minimum. Per-Unit appearance times remain available in details. Other readable co-targets can be shown separately but do not determine why/when the row matched.

Without a Unit filter, step 1 selects all otherwise allowed relationships; it does **not** switch to global Message time. A Message begun at 12:00 for Unit A and first observed for Unit B at 12:05 appears in an unfiltered 12:04..12:06 query once, at 12:05. In a Unit-A query for that interval it does not appear. With no time restriction, a view including both Units shows it once at 12:00. This query-dependent timestamp is deliberate and included in view/cursor identity.

A later Gateway repeat for Unit A never refreshes A's first appearance or moves the row to the top. A newly appearing Unit, older late evidence, changed filter/permission, correction or merge can change the view result/time; SSE repositions/reconciles the same Message ID rather than inserting a duplicate. A rule edit's server time is not an appearance time. Repeated receipt is not automatically a new Message or renewed Unit appearance.

**Targetless evidence:** A targetless Message contribution has no synthetic Address/target or Unit; supports reference only its Transmissions. Its Network/Protocol-authorized without-Unit appearance uses earliest permitted Observation time and capped age anchor, without routine Gateway-history scans. Protocols permitting empty target lists retain this case.

**Suppression and scoped minima:** Suppression invalidates affected contributions immediately; rebuild their supports/minima from remaining usable evidence before publication. Preserve all original targets. A stored minimum is reusable only when its entire support scope is authorized; narrower evidence/age/field permissions may require scoped support evaluation, never an unfiltered hidden minimum. This is an exceptional scoped path, not a full-history aggregate per ordinary read. Hidden evidence affects no counts, time, sort, facets or SSE indicators.

**Displayed Payload:** Select the Representative Transmission within ready evidence readable in this view, using C-03's deterministic preference and tie-break. Search may match another readable representation of the same canonical value; details identify that permitted source. A global representative cannot expose hidden text/derived fields. Canonically different fuller text remains a separate Message under D-30. A better exact representative updates content, not Unit first time. Row projection contains Message ID, View time, permitted matching target/Unit summaries and allowed Payload/derived fields; readable representative/matched-source references appear only with permitted evidence context. C-11 defines source-bound rendering; external endpoint serialization remains AP-09 after freeze.

**Administrative reception diagnostics:** Gateway/Network/time/Address filters apply to actual linked Transmission evidence, not to unrelated Message members. They use each receipt's Observation time and may show every reception; they are explicitly separate from the one-row-per-Message user feed. They provide evidence for coverage/quality comparisons, not proof of an Emission that no Gateway observed or proof that a Unit's device received it.

**Alias/projection:** Authorize before forwarding both alias and survivor; invisible/deleted references return identical `NOT_FOUND`, never another private ID. Filter-option lists/counts use the same current contribution scope; hidden filter IDs disclose no labels/existence. Apply projection before sorting/pagination, never remove unauthorized rows after forming a page.

**Search representation:** NFC, full default Unicode casefold, then NFC, identical for indexed text and query; preserve accents. `Straße` matches `STRASSE`; composed/decomposed `é` match; `é` does not match `e`, `ä` does not match `a`. Literal `%`, `_`, `.*` keep their ordinary characters. Invalid UTF-8 is rejected. Build normalized derived search values once per evidence/relevant processing revision, not by repeatedly normalizing all stored Payload per request. Pin one Unicode-data implementation/version across stores; rebuild derived indexes if it changes. Short queries must remain correct without mandatory external search. Index choice/version/performance evidence belongs to AP-03.

**Pagination:** Follow the opaque continuation-token pattern of [Google AIP-158](https://google.aip.dev/158) and [Microsoft Graph paging](https://learn.microsoft.com/en-us/graph/paging), not an invented client-decoded cursor syntax or offset paging. Request: filters, time anchor, `pageSize`, optional `pageToken`. Response: `items`, optional `nextPageToken`, `viewId`, safe freshness/status. No next token means end; totals are optional and authorized only. A server-issued random 256-bit URL-safe reference points to bounded server-side continuation state: Principal/key restriction, normalized query, direction, last sort tuple, initial acceptance cutoff and relevant access/view revisions. Lifetime 24 h, maximum 20 retained views/Principal; evicted state requires resync. Tokens convey no permission and expose no hidden sort values.

Changing filters/Principal invalidates a token; changed relevant rights/contributions/appearance times/retention invalidates or safely reconciles state. The continuation tuple is `(viewTime, Message ID)` after grouping, not a contribution or receipt row; a page never returns a partial/duplicate Message because its Units straddle an internal page. New accepted data past the initial cutoff belongs to live updates; corrections to summarized first times after that cutoff require resync rather than pretending an old snapshot is reconstructable from overwritten summaries. `RESYNC_REQUIRED` is explicit. No public global revision counter or hidden-change counts. [Relay](https://relay.dev/graphql/connections.htm) confirms opaque cursor semantics but is GraphQL-specific; GoPager does not adopt GraphQL or claim a universal standardized token interior.

**SSE:** Use standard `text/event-stream`, UTF-8, `id` and `Last-Event-ID` per [WHATWG](https://html.spec.whatwg.org/multipage/server-sent-events.html). The SSE resume token is distinct from a Page token and internal Domain event ID. Per-frame data: permitted `upsert` projection, `remove` of an already-known ID, or `resync-required` without private fields. Bind view/filter/Principal/key and relevant revision context. Resume state retained up to 24 h subject to bounded storage; expired/unknown/mismatched/revoked state triggers resync, not raw replay.

The semantic application field name is `resumeToken`; its SSE transport remains `id`/`Last-Event-ID`, not a newly defined transport header.

Initial page and stream handoff share a consistent cutoff/change position so a commit cannot vanish between them. On resync the client atomically replaces/reconciles its loaded ID set, removing revoked/deleted/merged entries; fresh status only after completion. No initial live-age limit. Disconnected/noncurrent views visibly lose freshness. Recheck access at every output batch, bounded work 30 s; heartbeat every 15 s has no private data. Bounded stream queue 256 frames/1 MiB; slow consumer receives safe resync/close, not silent frame loss or unbounded buffering. Multiple tabs and shared IPs are supported within per-Principal rather than one-per-IP limits.

## AP-02 Errors and Localized Text

REST error body: `schemaVersion`, `code`, typed `parameters`, `textKey`, safe English `fallback`, random `requestId`, `retryable`, optional `retryAfterSeconds`. HTTP status is transport semantics, not a parser for fallback text. Validation parameters may name an input field/max/value category, not echo confidential values or invisible IDs. Authenticated detail invisibility and nonexistence have identical response shape.

| Code | HTTP | English fallback / allowed parameters |
| --- | --- | --- |
| `INVALID_INPUT` | 400 | "Check the supplied values." / field, constraint, limit |
| `LOGIN_REQUIRED` | 401 | "Sign in to continue." / none |
| `FORBIDDEN` | 403 | "This action is not permitted." / none |
| `NOT_FOUND` | 404 | "The requested item is not available." / none |
| `REVISION_CONFLICT` | 409 | "This item changed. Reload before saving." / none; current revision only if readable |
| `IDEMPOTENCY_CONFLICT` | 409 | "This operation identifier was already used differently." / own operation reference |
| `PROCESSING_PENDING` | 409 | "Processing is not complete." / own safe state |
| `RESYNC_REQUIRED` | 409 | "Reload this view to continue." / none |
| `DEPENDENCY_UNAVAILABLE` | 409 | "Required data is no longer available." / safe dependency kind |
| `CAPABILITY_UNDECLARED` | 403 | "The integration has not declared this capability." / permitted capability name |
| `INCOMPATIBLE_VERSION` | 409 | "This version is not compatible." / public supported schema range |
| `RESOURCE_LIMIT` | 413 | "The operation exceeds a configured limit." / limit kind, maximum |
| `RATE_LIMITED` | 429 | "Try again later." / retry delay, also Retry-After |
| `STORAGE_UNAVAILABLE` | 503 | "Storage is not available for this operation." / retry hint |
| `AUTHORITY_UNAVAILABLE` | 503 | "Current write or access authority is not available." / retry hint |
| `COMMIT_UNCERTAIN` | 503 | "The operation may have been committed. Check its identifier." / own operation reference |
| `EXTERNAL_UNCERTAIN` | 409 | "The external action may already have occurred." / own attempt reference |
| `INTERNAL_ERROR` | 500 | "The operation could not be completed." / requestId only |

Codes map to `error.<CODE>`. Outcome statuses map to `status.<status>` with explicit English base entries, never mechanically display an untranslated code as prose. Submission accepted/processing pending uses 202; completed receipt uses 200, new immediately completed resource may use 201; recovery request always generic 202. Background job/SSE errors use the same code semantics without pretending a stream frame changes HTTP status. Transient codes alone are retryable; `COMMIT_UNCERTAIN` requires receipt lookup/same operation ID, not blind creation. Fallback remains redacted even if text-key lookup fails.
