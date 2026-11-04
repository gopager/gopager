# AP-12 Processing Presentation and Integration

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-12 T044 and the source-bound portion of T045](source-ap-12-processing-presentation.md).
- **Requirements and owners:** [REQ-091..100](../requirements-catalog.md#j-processing-presentation-and-usability), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-11](../contracts/c-11-processing-presentation.md).
- **Owner:** DOM-08.
- **Architecture:** [worker and queue assessment](../architecture.md#ap-12-durable-worker-and-job-queue-assessment), [cross-profile worker/pub-sub comparison](../architecture.md#ap-12-cross-profile-worker-and-pub-sub-comparison), [domain ownership](../architecture.md#domains-and-data-ownership).
- **Prerequisites:** Verified AP-06 through AP-11 handovers; only applicable contract/decision slices already fixed by earlier work. Any C-11 detail needed by T072-T074 that is not already fixed is planned and defined in this AP, not treated as an external freeze prerequisite. The AP-06 kernel and AP-07..AP-11 consumer boundaries are fixed inputs and must not be redefined.
- **Write scope:** `internal/processing/`, Processing-owned schemas/adapters, and focused integration tests; do not rewrite the frozen kernel owner surface.

## Planning Before Coding

Complete this planning as the first part of T072-T074; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Record contract decisions in C-11 and case expectations at their F-case owners; track the work within those existing task IDs:

- **Later-consumer planning context (not prerequisites):** [AP-13 Identity Lifecycle](ap-13-identity-lifecycle.md), [AP-14 Frontend](ap-14-frontend.md), and [AP-15 Operations](ap-15-operations.md). Use these to consider deletion/visibility, presentation-consumer and operational boundaries; do not implement lifecycle, browser or operations tasks early.
- For T072, define representative-source selection, source binding, result/notice fields, sensitivity and permission-preserving projection, revision behavior, preview isolation and preference operations.
- For T073, define only the integration mapping between the fixed AP-06 kernel surface, AP-07 committed intake facts, AP-09 query projection, AP-10 events and AP-11 installed-step host. Specify trigger/revision behavior and failure/rollback boundaries without changing those owner contracts.
- For T074, bind source-binding, preview, sensitivity, revision and presentation cases to the assigned F-cases and verified earlier handovers. Do not add browser behavior or rebuild the kernel.

- [ ] T072 [AP-12] Implement permission-preserving representative presentation, source-bound results, notices, and preference operations.
- [ ] T073 [AP-12] Integrate the frozen kernel with committed intake facts, current query projection, event reactions, and the approved installed-step host boundary.
- [ ] T074 [AP-12] Verify end-to-end source binding, preview isolation, sensitivity, revisions, and presentation cases across the completed backend boundaries.

**Acceptance:** Processing never changes original Payloads or expands visibility; presented results remain bound to their authorized source and current revision.

**Non-goals:** Rebuilding the evaluator kernel, unrestricted workflow scripting, automatic Payload translation, or unapproved browser behavior.
