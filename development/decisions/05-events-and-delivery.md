# ADR: Events and Delivery

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-06"></a>
## D-06 Initial Delivery target selection and subscription boundary

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed initial Delivery target selection and subscription boundary.
- **Owner detail:** [REQ-090](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md)

<a id="d-88"></a>
## D-88 AP-12 Cross-Profile Worker and Pub/Sub Comparison

- **Status:** superseded by D-135 (in part: the SQLite comparison; the River evidence stays)
- **Date:** 2026-11-02
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Cross-profile worker and pub/sub comparison; evidence only, SQLite part obsolete under D-135. The user explicitly approved this comparative evaluation. Existing decisions, including D-75, remain authoritative until a distinct product/contract decision approves a replacement. A recommendation may propose an architectural restructuring but does not itself approve implementation or downstream AP changes. Record the actual commands, versions, outcomes and remaining uncertainties in the AP-12 execution record; maintain the comparison and recommendation in architecture.
- **Owner detail:** This record is the only current definition

<a id="d-89"></a>
## D-89 Core-Domain Event-Driven Architecture Reassessment

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** Core-domain event-driven architecture reassessment authorized. The user explicitly requested the reassessment. Prior decisions remain authoritative while it is conducted. Record the comparison, findings and recommendation in [architecture](../architecture.md#core-domain-event-driven-architecture-reassessment); any later selection, contract changes, AP amendments, dependency approval and implementation require their own owner-specific approval.
- **Owner detail:** This record is the only current definition

<a id="d-90"></a>
## D-90 Core-Domain Event-Driven Architecture Direction and Watermill Prototype

- **Status:** superseded by D-135 (in part: the Watermill prototype; the hybrid relational-plus-facts direction stands)
- **Date:** 2026-11-02
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Hybrid direction: relational authoritative state, facts in the same transaction, selective asynchronous reactions; Watermill prototype. Keep relational authoritative domain state; record required Domain facts in the same primary transaction as their state change; use event consumers selectively for independent reactions and projections where the owning consistency contract permits it; retain C-08 durable jobs for work requiring due scheduling, single-claim ownership, bounded attempts, leases/fencing, visible failure and recovery. Event transport is not the job lifecycle and neither replaces domain authorization or publication barriers. This direction explicitly retains D-43's synchronous initial Transmission/Grouping boundary and D-75's one Processing candidate lifecycle; it does not introduce Event Sourcing or supersede those decisions.
- **Owner detail:** This record is the only current definition

<a id="d-91"></a>
## D-91 Watermill Failure and Concurrency Validation Extension

- **Status:** accepted (evidence only)
- **Date:** 2026-11-02
- **Decision:** Watermill failure/concurrency validation extension; evidence only. Extend the session-only module with hard consumer-process termination before and after application commit, restart/redelivery, competing same-group consumers, atomic receipt/result/downstream work and fact publication, and SQLite contention using independent connections. Reuse the pinned adapters, Testcontainers and Go standard-library process/test APIs; synthetic fault injection and domain transaction fixtures are application-specific test glue, not a new queue implementation. Record exact procedures, outcomes and limitations in a new AP-12 evidence entry and link the architectural consequences.
- **Owner detail:** This record is the only current definition

<a id="d-92"></a>
## D-92 Isolated SQLite Adapter Renewal Fix Assessment

- **Status:** superseded by D-135
- **Date:** 2026-11-02
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Isolated SQLite adapter renewal fix assessment; obsolete under D-135.
- **Owner detail:** This record is the only current definition

<a id="d-93"></a>
## D-93 Watermill PostgreSQL-first Gate Clarification

- **Status:** superseded by D-135
- **Date:** 2026-11-02
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** PostgreSQL-first gate: SQLite is not a blocker; obsolete under D-135. [D-32](01-platform-and-storage.md#d-32) already starts implementation with Full/PostgreSQL and defers runnable Lite. The SQLite issue in [E-42](../execution/source-ap-12.md#e-42-watermill-crash-atomicity-and-concurrency-extension), with its isolated correction in [E-43](../execution/source-ap-12.md#e-43-isolated-sqlite-renewal-correction-and-upstream-check), gates later acceptance of that SQLite adapter, not current PostgreSQL Watermill integration. Do not require an upstream SQLite release or maintained patch before PostgreSQL work. Preserve store-neutral domain/application boundaries so Lite remains achievable through alternate infrastructure adapters.
- **Owner detail:** This record is the only current definition

<a id="d-94"></a>
## D-94 Watermill PostgreSQL Publication Integration Proposal

- **Status:** superseded by D-135 (native sequenced event log)
- **Date:** 2026-11-02
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Watermill PostgreSQL fact publication slice; superseded by D-135's native sequenced event log.
- **Owner detail:** This record is the only current definition

<a id="d-122"></a>
## D-122 AP-07 additive write-scope and execution-binding proposal

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved AP-07 additive write scope and execution binding.
- **Owner detail:** This record is the only current definition

<a id="d-123"></a>
## D-123 AP-07 selected-Workflow worker lifecycle scope extension

- **Status:** superseded by D-135 (no publication barrier)
- **Date:** 2026-11-04
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Approved narrow AP-07 selected-Workflow worker lifecycle scope extension. Keep D-122 unchanged and leave T058/F-66 blocked, broadly reopen Processing internals, or approve only the worker lifecycle seam and focused tests. The user approved the narrow extension.
- **Consequences:** Amend the versioned [AP-07 implementation addendum](../work-packages/addenda/ap-07-intake-implementation.md) to list these two paths and link D-123; record implementation and verification only in the [AP-07 execution record](../execution/ap-07.md). No glossary term or downstream AP amendment is required.
- **Owner detail:** This record is the only current definition

<a id="d-124"></a>
## D-124 AP-07 selected-Workflow source and publication scope extension

- **Status:** superseded by D-135 (no publication barrier)
- **Date:** 2026-11-04
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Approved AP-07 selected-Workflow source and atomic publication scope extension. Keep F-66 blocked, broadly reopen Processing/runtime implementation, or authorize only the source/publication composition and focused verification paths needed by T058/T059. The user approved the exact narrow extension.
- **Owner detail:** This record is the only current definition

<a id="d-144"></a>
## D-144 Message time in Message-related facts

- **Status:** accepted
- **Date:** 2026-11-08
- **Decision:** AP-10's live stream filters pushed facts by the client's loaded Message-time range, but no implemented fact carried a Message time (input gap found at the AP-10 start). The user approved extending the producers rather than resolving each fact's Message time from current state. Every fact that names a Message carries the optional envelope field `data.messageTime`, the Message time (`messages.first_observed_at`) after the change: `message.created`, `message.transmission-joined`, `message.merged`, `assignment.changed` (intake `created` and correction `corrected`, including Network moves), `suppression.changed` (which additionally carries the `messageId` of the suppressed target's Transmission) and `processing.completed`. `message.transmission-joined` and `message.merged` also carry `data.previousMessageTime`, the latest Message time among the Messages the change replaced, when it differs from `messageTime`; the Message time never moves later, so `[messageTime, previousMessageTime]` covers every time the affected rows had before and after. `schemaVersion` stays 1 (additive optional fields under the envelope rules); the producers always set them. Resolving the time from current state in the live source was considered and rejected by the user.
- **Scope (AP-10 write scope extension):** `internal/events/` fact types and validation; producers in `internal/persistence/ent/ingeststore/` (Grouping and intake facts, Processing source binding), `internal/processing/` (publication binding and `publication.Service`), `internal/assignments/` (correction and suppression facts) and `internal/persistence/ent/assignmentstore/` (Message times of corrected Transmissions); their tests and the PostgreSQL fixtures in `internal/persistence/ent/borrowed/`.
- **Consequences:** The [AP-06](../execution/ap-06.md), [AP-07](../execution/ap-07.md), [AP-08](../execution/ap-08.md) and [AP-20](../execution/ap-20.md) records get a follow-up with the fact tests rerun; [AP-11](../work-packages/ap-11-events-delivery.md) lists the fields in its compatibility context; [Architecture: Domain events and outbox](../../docs/architecture.md#domain-events-and-outbox) and C-08 describe them.
- **Owner detail:** [C-08](../contracts/c-08-events-jobs.md), [AP-10 record](../execution/ap-10.md)
