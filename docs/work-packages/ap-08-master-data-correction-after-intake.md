# AP-08 Master Data and Correction

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Remaining historical [source AP-08 T028..T030](source-ap-08-master-data-correction.md); the verified early Unit/matcher handover is recorded in the [source AP-08 execution record](../execution/source-ap-08.md).
- **Requirements and owners:** [REQ-013/019/021..040/045/095/123..124/132](../requirements-catalog.md#c-units-organizations-and-defaults), [C-02](../contracts/c-02-protocol-payload.md), [C-04](../contracts/c-04-master-data.md), [C-05](../contracts/c-05-matching-correction.md), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md).
- **Owner:** DOM-03/DOM-04; integration coordinates cross-domain correction.
- **Architecture:** [domain ownership](../architecture.md#domains-and-data-ownership), [logical relationships](../architecture.md#logical-relationship-model), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verified AP-06 and AP-07 handovers; the already verified identity-core handover from historical [source AP-07](../execution/source-ap-07.md); only applicable contract/decision slices already fixed by earlier work. The draft C-04/C-05 text is planning context, not a fixed prerequisite; this AP owns only the exact implementation-bound slices listed below. Do not backfill an incomplete earlier-AP deliverable.
- **Write scope:** `internal/Units/`, `internal/assignments/`, their owner schemas and tests; shared composition changes only under the assigned integration binding.

## Planning Before Coding

Complete this planning as the first part of T060-T062; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Record contract decisions at C-04/C-05 and case expectations at their F-case owners; track the work within those existing task IDs:

- **Planning references:** [AP-02 contract work](ap-02-domain-contracts.md) and its [documentary handover](../execution/ap-02.md), [C-04](../contracts/c-04-master-data.md), [C-05](../contracts/c-05-matching-correction.md), and their linked F-cases. Treat draft text as context, not fixed authority. Decide only the non-fixed implementation-bound behavior needed by T060-T062 from the user stories and fixed decisions; do not redo AP-02's completed documentary work. If a needed contract deliverable assigned to AP-02 is missing, report that handover gap instead.
- **Later-consumer planning context (not prerequisites):** [AP-09 Query and Live API](ap-09-query-live.md), [AP-10 Events and Delivery](ap-10-events-delivery.md), [AP-13 Identity Lifecycle](ap-13-identity-lifecycle.md), [AP-14 Frontend](ap-14-frontend.md), and [C-13 Account Lifecycle](../contracts/c-13-account-lifecycle.md). Use these to avoid correction, query, event, authorization, cleanup and management boundaries that obstruct later consumers; do not implement their tasks or assume their outputs exist.
- For T060, finalize only the implementation contract for the remaining Unit, organization, collection, template and default operations: inputs/state changes, action/field authorization, validation/errors, default preservation, bulk success/failure, conflict behavior, revisions and transaction boundaries.
- For T061, finalize only the implementation contract for historical correction: preserved original evidence, correction eligibility/suppression, current target resolution, contribution/support redistribution, checked rematching, races, atomic publication and recoverable failure outcomes. Do not change any fixed C-05 decision or AP-07's frozen intake boundary.
- Bind each defined behavior to its assigned F-cases and identify the exact producer/consumer handover from AP-07. Do not implement UI or later-AP work.

- [ ] T060 [AP-08] Complete the remaining Unit, organization, collection, template, default, bulk, and conflict operations.
- [ ] T061 [AP-08] Complete historical target correction, contribution/support redistribution, suppression, and checked rematching using the committed intake boundary.
- [ ] T062 [AP-08] Verify deletion/default preservation, correction/revocation races, rollback, and the assigned F-cases against PostgreSQL.

**Acceptance:** Corrections update only permitted derived state, retain original evidence, and leave no stale visible Unit references or unauthorized conflict details.

**Non-goals:** New Transmission/Message creation, Payload masking, historical presentation versions, unapproved imports, or heuristic comparison.
