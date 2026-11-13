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

<a id="d-149"></a>
## D-149 Per-instance use of Delivery targets belongs to AP-12

- **Status:** accepted
- **Date:** 2026-11-10
- **Decision:** An automated security review of the AP-11 pull request found that every holder of `subscription.manage-own` (and every global Subscription) may select any enabled processing instance with a Delivery descriptor as a target; only the subscriber's own permitted data reaches it (REQ-082 lets users build Subscriptions on administratively configured integrations), but an administrator cannot restrict an instance, for example an internal webhook, to some users. No target is usable in production before AP-12 supplies providers. The user assigned the restriction to AP-12's configuration authority instead of adding a usage action in AP-11 or accepting unrestricted use: the administrative configuration of each Delivery instance declares who may use it as a target, checked at Subscription save, at fan-out and before each start, so revoking it stops future sends.
- **Scope:** AP-12 T069 (configuration authority) and its C-06/C-09 slices; AP-11 is unchanged.
- **Consequences:** [AP-12](../work-packages/ap-12-plugin-host.md) T069 amended; [C-09 Configuration Authority](../contracts/c-09-delivery.md#configuration-authority) notes the open rule; the [AP-11 record](../execution/ap-11.md) links this decision.
- **Owner detail:** [C-09](../contracts/c-09-delivery.md), AP-12


<a id="d-157"></a>
## D-157 AP-22 runs before AP-12 on the AP-11 target-input port

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** [AP-22](../work-packages/ap-22-unit-attributes.md) runs before [AP-12](../work-packages/ap-12-plugin-host.md). Its input "AP-12 handover" (plugin-declared Delivery target inputs, per-Unit parameter API) is replaced by the completed [AP-11 handover](../execution/ap-11.md#handover): the target descriptor port `delivery.TargetDescriptor.Inputs` (typed `InputField` kinds), `integration_unit_parameters` and the input precedence of materialization. AP-22 verifies the target-input binding with fake targets, as AP-11 verified its input path. AP-12 maps its manifests onto the same descriptor and keeps the binding working; real-plugin evidence of the binding belongs to AP-17.
- **Rationale:** AP-12 was open without a handover, but AP-22 consumes only contracts that AP-11 already fixed (AP-12 T069 maps manifests onto AP-11's descriptor and does not change input kinds), and AP-22 does not author Unit parameters. The overlapping write scopes (OpenAPI, HTTP adapters, Ent schema and generated code, composition) require the two APs to run one after the other; the smaller AP-22 lands first, directly on [AP-21](../work-packages/ap-21-attribute-schemas.md). Running AP-12 first, or leaving the binding open until AP-12, were considered and rejected by the user.
- **Consequences:** The AP-22 inputs and the AP-12 compatibility context are amended; the [execution path](../work-packages.md#revised-execution-path) lists AP-22 before AP-12. No completed AP changes; producers in the requirement mapping are unchanged.

<a id="d-158"></a>
## D-158 AP-22 write scope: target-input bindings in `00013_events_delivery.sql`

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** [AP-22](../work-packages/ap-22-unit-attributes.md) T096 adds the column `input_bindings jsonb NOT NULL DEFAULT '{}'` to `subscription_targets` by editing `internal/persistence/migrations/00013_events_delivery.sql` in place ([D-128](01-platform-and-storage.md#d-128)); the store reads and writes it with `inputs`.
- **Rationale:** A Subscription target stores, per input, a binding to a Unit attribute next to its constant inputs, and the table is created by AP-11's migration, which the AP-22 write scope did not list. A separate table in a new migration was considered and rejected by the user as more joins and cascade handling for one small map; staying in scope would have left the target-input binding of T096 unbuilt.
- **Consequences:** The [AP-11 record](../execution/ap-11.md) receives a follow-up with the rerun evidence; the AP-22 record links this decision under Blockers and Deviations. No open AP's assignment changes.

<a id="d-160"></a>
## D-160 AP-12 write scope: target use, personal values and placement in the Delivery Core

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** The user approved three limited changes outside the [AP-12](../work-packages/ap-12-plugin-host.md) write scope in `internal/delivery/` and `internal/persistence/ent/deliverystore/` (AP-11 code), with their tests: (a) the per-instance target use of [D-149](#d-149), declared in the instance's administrative configuration, is checked at Subscription save (`Subscriptions.checkTargets`), at fan-out (`render`) and at the start check (`Runner.current`); (b) `AttemptInvocation` gains the additive `subscriberId` (version 1 stays), so the plugin host hands the subscriber's personal configuration and Secrets to the plugin; (c) the static claim filter `RunnerOptions.Instances` becomes a function listing the instances whose pinned artifact is loadable on this node now, and `ClaimDue` gets a fallback: a Delivery unclaimed past a short grace period may be claimed by any node, which defers it visibly with outcome code `dependency-unavailable` when it cannot load the artifact.
- **Rationale:** T069 and T070 require all three, but the AP listed only the plugin-host paths. Without (a) the D-149 security finding stays open, without (b) a personal Pushover User key cannot reach the plugin, and without (c) placement is fixed at startup and a Delivery no node can serve stays `prepared` without a visible status. Staying within the scope was offered and rejected.
- **Consequences:** [AP-11 record](../execution/ap-11.md) follow-up with rerun evidence of `internal/delivery`, `deliverystore` and the `cmd/gopager` Delivery tests (including the AP-22 binding tests); [C-09](../contracts/c-09-delivery.md) Core Binding (claims, attempt invocation) and [C-10](../contracts/c-10-plugin-host.md); [AP-14](../work-packages/ap-14-identity-lifecycle.md) T076 also deletes the personal configuration and Secret rows by owner key; [AP-17](../work-packages/ap-17-integration-evidence.md) consumes the changed port. The AP-12 record links this decision under Blockers and Deviations.

<a id="d-163"></a>
## D-163 Deliveries show the provider's safe code

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** The user decided that a plugin's safe reason code (for example `recipient-missing`, `destination-denied`, `rate-limited`) must be visible on the Delivery, not only in the node-local plugin status. Each attempt row stores `provider_code` (the `code` of the provider's result when it matches `[a-z0-9-]{1,64}`; none when the call failed or the code is invalid), the attempt view shows it as `providerCode`, and the Delivery carries the provider code of its latest finished attempt as `providerCode`. The enumerated `outcomeCode` set of the Delivery and its attempts is unchanged. This changes `delivery_attempts` and `deliveries` in `00013_events_delivery.sql`, `internal/delivery` and `deliverystore` (AP-11 code), outside the AP-12 write scope.
- **Rationale:** Without it a subscriber or administrator sees only `permanent` or `transient` and cannot tell a missing personal key from a refused destination.
- **Consequences:** [AP-11 record](../execution/ap-11.md) follow-up with rerun evidence; [C-09](../contracts/c-09-delivery.md) Core Binding (Outcome) and [HTTP API: Deliveries](../../docs/http-api.md#deliveries); [AP-15](../work-packages/ap-15-frontend.md) screens can show the code; the AP-12 record links this decision under Blockers and Deviations.

<a id="d-164"></a>
## D-164 AP-13 write scope: Units in the layout request and a shared Message field reader

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** The user approved two changes in `internal/delivery` (AP-11 code), outside the [AP-13](../work-packages/ap-13-processing-presentation-integration.md) write scope. (1) `processing.LayoutRequest` gains `UnitIDs`, and Delivery materialization fills it with the Delivery's relevant Units when the Subscription has no explicit Layout, so the installation, Organization and Unit layout layers that AP-13 binds into the `LayoutResolver` apply to Deliveries. (2) `delivery.MessageFieldReader` exports the existing Delivery value reader for one Message projection without event fields, so the GUI card model renders a Layout with exactly the Delivery value semantics.
- **Rationale:** The resolver request carried only the Layout ID and output kind, so no layer below the Subscription could reach Delivery rendering; a second value reader for cards would drift from the Delivery one that the card/Delivery parity case compares against.
- **Consequences:** [AP-11 record](../execution/ap-11.md) follow-up with rerun evidence of `internal/delivery` and the `cmd/gopager` Delivery tests; [C-11](../contracts/c-11-processing-presentation.md#ap-13-binding-planning-before-coding) precedence binding; [docs/architecture.md](../../docs/architecture.md) Delivery section; the AP-13 record links this decision under Blockers and Deviations. No open AP's assignment changes.

<a id="d-166"></a>
## D-166 Retracted Processing results notify live views

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** After the Copilot review of the [AP-13](../work-packages/ap-13-processing-presentation-integration.md) pull request the user approved a new fact `processing.retracted` (producer `processing`, not deliverable, Message projection; `data` carries `messageId`, `messageTime`, `transmissionId`, `workflowId` and the retracted result IDs). Historical Reprocess appends it in the transaction that retracts results without publishing a replacement (the Workflow no longer selects the Transmission, or the re-run is not selected), and live Message views re-evaluate the Message on it. This extends the `internal/events` catalog and the live fact handling in `internal/readmodel`, outside the AP-13 write scope.
- **Rationale:** Without a fact an open live stream kept withdrawn attributes, cards and filter matches until reload; a replacement publication already appends `processing.completed`.
- **Consequences:** [C-08](../contracts/c-08-events-jobs.md) fact list, [docs/architecture.md](../../docs/architecture.md) Domain events, [docs/workflows.md](../../docs/workflows.md); follow-ups in the [AP-10](../execution/ap-10.md) record (live views) and the [AP-11](../execution/ap-11.md) record (catalog, fan-out ignores it); the AP-13 record links this decision under Blockers and Deviations. No open AP's assignment changes.
