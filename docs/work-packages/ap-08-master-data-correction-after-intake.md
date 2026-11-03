# AP-08 Master Data and Correction

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Remaining historical [source AP-08 T028..T030](source-ap-08-master-data-correction.md); the verified early Unit/matcher handover is recorded in the [source AP-08 execution record](../execution/source-ap-08.md).
- **Requirements and owners:** [REQ-013/019/021..040/045/095/123..124/132](../requirements-catalog.md#c-units-organizations-and-defaults), [C-02](../contracts/c-02-protocol-payload.md), [C-04](../contracts/c-04-master-data.md), [C-05](../contracts/c-05-matching-correction.md), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md).
- **Owner:** DOM-03/DOM-04; integration coordinates cross-domain correction.
- **Prerequisites:** AP-06 and AP-07; the already verified identity-core handover from historical [source AP-07](../execution/source-ap-07.md), produced before this ordered path began; applicable AP-04 freezes and this slice's exact execution binding.
- **Write scope:** `internal/Units/`, `internal/assignments/`, their owner schemas and tests; shared composition changes only under the assigned integration binding.

- [ ] T060 [AP-08] Complete the remaining Unit, organization, collection, template, default, bulk, and conflict operations.
- [ ] T061 [AP-08] Complete historical target correction, contribution/support redistribution, suppression, and checked rematching using the committed intake boundary.
- [ ] T062 [AP-08] Verify deletion/default preservation, correction/revocation races, rollback, and the assigned F-cases against PostgreSQL.

**Acceptance:** Corrections update only permitted derived state, retain original evidence, and leave no stale visible Unit references or unauthorized conflict details.

**Non-goals:** New Transmission/Message creation, Payload masking, historical presentation versions, unapproved imports, or heuristic comparison.
