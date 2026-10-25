# C-08 Domain events, Outbox, and Jobs

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-01](c-01-transmission-acceptance.md), [C-09](c-09-delivery.md), [C-11](c-11-processing-presentation.md).

**Version/status:** 0.1-draft. **Owner:** DOM-07 for mechanics, producing domain for Domain event meaning, integration for commit boundary. **Producers/consumers:** Gateway/Message/assignment/configuration changes -> explicit live/rematch/Delivery work.

Draft interfaces: `AppendDomainEvent(transaction, producer, DomainEvent) -> domainEventRef`; `ClaimJob(context, role, workerRef) -> leasedWork`; `CompleteJob(leaseRef, result)`; `RescheduleJob(leaseRef, classifiedFailure)`. Proposed Domain event fields: Domain event ID, type/schema version, producer, domain reference/revision, domain/recorded times, causality/idempotency reference, access-classified Payload. Exact envelope/required fields are not frozen.

**Defined:** Domain state and required Domain event/work are atomic in the primary store. Outbox state is truth; wake-up/pubsub is optimization only. Do not deliver a Domain event as committed before commit. The AP-02 table below defines type names and per-aggregate ordering; no hidden once-per-Message default or global provider order.

**Recovery:** Claims/leases distinguish concurrent workers and superseded owners; an expired worker cannot blindly overwrite current job state. Internal restart may duplicate processing and requires idempotent consumers. Provider I/O retains C-09 uncertainty, not exactly-once guaranteed by a job lease. Not all jobs inherit one retry algorithm.

**Permissions:** Recording a Domain event does not replace current consumer/Delivery authorization. Raw Payloads are not browser/plugin grants. Retain rematch/retention/Delivery receipts so ordinary replay/restart does not revive deleted or expired work. Manual restoration of older store state is the Operator's responsibility under C-12/D-31. Domain event type/schema codes are language-independent.

## AP-02 Recorded Facts and Work

Required envelope: `schemaVersion: 1`, `domainEventId` UUID, `type` from glossary registry, producer domain/plugin identity/version, `aggregateRef` type/ID, `aggregateRevision`, `occurredAt` domain fact time, `recordedAt` commit-associated primary time, optional `causationId` and `correlationId`, `data`, `classification`, `replayMode` (normal or explicit historical reprocessing). Domain event data is internal recorded context, not already-authorized outbound Payload. A schema change adding an optional field is backward compatible; new meaning/required field uses a new schema version. Unknown versions are visibly blocked, never skipped as successfully handled.

| Type / producer | Required data and effect |
| --- | --- |
| `message.created` / DOM-02 | Message/initial Transmission references, Message revision/time; explicit initial membership fact, no redundant joined fact for that first member. |
| `message.transmission-joined` / DOM-02 | Message/Transmission references, late-join indicator, resulting revision/time; one per independent additional membership. |
| `message.merged` / DOM-02 | Survivor/absorbed Message references, resulting revision/time; consumers resolve chains, preserve original causality and deduplicate. |
| `assignment.changed` / DOM-04 | Message and affected target/contribution references, Assignment scope revision, `reason` (created, corrected, removed, first-time-changed), `before`/`after` with Unit/matched-rule/status, Contribution time/age anchor as applicable; absent side on creation/removal. Genuine transitions only, not later support alone. No immutable rule-version reference or separate Assignment ID. Read projections hide unauthorized before/after sides; removal cannot disclose hidden new context. |
| `suppression.changed` / DOM-04 | Suppression reference/revision and affected scope, no raw Payload; removing suppression is not membership creation. |
| `gateway.state-changed` / DOM-01 | Gateway reference, old/new state and contact time; no inferred crash diagnosis. |
| `operation.submission-rejected`, `operation.observation-time-implausible`, `operation.network-ambiguous` / DOM-01 | Gateway, Submission ID, stable error/reason code and time; ambiguity additionally resolved Network/method, subject to projection. No Payload copy for rejection. |
| `processing.completed` / DOM-08 | Transmission/Message, Workflow/version, Processing-result reference/status and classified fields such as level; scalar level is not its own type. |
| `configuration.changed`, `principal.access-changed`, `principal.deleted` / owning domain | Entity/revision and nonsecret change kind; central invalidation/maintenance facts, not automatic public broadcasts. |
| `plugin.<plugin-id>.<type>` / declared plugin | Manifest schema, explicit data classification/Principal context, bounded data and causation. Internal publication itself requires a declared/approved SDK capability. |

Ordering is per aggregate revision/causation, not a claim of global provider order. The transaction that changes a state records its fact and required work; a consumer records its result/dedup receipt atomically with downstream jobs. A Transmission may have intake/Grouping facts recorded internally while Processing remains pending: C-11's publication barrier gates GUI/Delivery materialization, without altering the selected subscribed trigger type.

One exact bridge transaction records one `message.merged` fact with survivor and the sorted absorbed-reference list, plus one `message.transmission-joined` for the newly accepted member; no new `message.created` for the survivor. Facts from that atomic change may share the final aggregate revision and have no required order within it. Consumers resolve current aliases, deduplicate each fact and reconcile current authorized state rather than depend on merge-before-join delivery. Across revisions, detect stale/superseded state; a provider can still observe different completion order. No hidden global sequencing field or exactly-once provider order is required.

**Claim/fencing:** Short primary transaction selects available due work, checks role, increments Fencing token and sets lease deadline. Renew/complete/reschedule require matching job/token and nonexpired lease. A lost Worker cannot publish stale target resolutions/contributions or terminal results over a successor. Lease expiry before external start permits reclaim; expiry after a recorded external start produces `uncertain` unless a provider-supported idempotency/receipt proves safe recovery. Never hold SQL transactions across plugin/provider I/O. Store driver locking/isolation/wake-up mechanism is AP-03, not a second semantic job implementation.

**Retry/recovery:** Idempotent internal derivation gets at most 5 automatic attempts (30 s/2 min/10 min/30 min delays with bounded jitter), then visible failed work; an authorized retry targets the current revision. Superseded work is terminal without publishing stale results. Delivery uses C-09's separate policy. Cancellation retains outcome/uncertainty and never makes old work appear unattempted. No separate mandatory dead-letter broker: failed/uncertain primary-store records are the inspection/recovery queue.

**Replay/retention:** Automatic fan-out accepts Domain events for at most 24 h from recordedAt and only within current subscription start/revision/age rules; no automatic backfill when subscribing. Pending unconsumed facts cannot be silently pruned: expiry marks dependent work expired and inspectable. Retain terminal consumer/Delivery dedup receipts until no automatic replay/active job can use them (at least 24 h after terminal state and past the replay deadline). Compact receipts may remain while needed; no indefinite secret Payload retention. Manual replay/reprocessing explicitly suppresses external reactions by default; deliberate redelivery uses a fresh confirmed operation. No special application Backup/Restore logic or knowledge of later lost receipts is promised (C-12/D-31).
