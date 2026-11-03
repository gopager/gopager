# AP-12 Processing Presentation and Integration

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-12 T044 and the source-bound portion of T045](source-ap-12-processing-presentation.md).
- **Requirements and owners:** [REQ-091..100](../requirements-catalog.md#j-processing-presentation-and-usability), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-11](../contracts/c-11-processing-presentation.md).
- **Owner:** DOM-08.
- **Prerequisites:** AP-06 through AP-11; applicable AP-04 freezes and this slice's exact execution binding.
- **Write scope:** `internal/processing/`, Processing-owned schemas/adapters, and focused integration tests; do not rewrite the frozen kernel owner surface.

- [ ] T072 [AP-12] Implement permission-preserving representative presentation, source-bound results, notices, and preference operations.
- [ ] T073 [AP-12] Integrate the frozen kernel with committed intake facts, current query projection, event reactions, and the approved installed-step host boundary.
- [ ] T074 [AP-12] Verify end-to-end source binding, preview isolation, sensitivity, revisions, and presentation cases across the completed backend boundaries.

**Acceptance:** Processing never changes original Payloads or expands visibility; presented results remain bound to their authorized source and current revision.

**Non-goals:** Rebuilding the evaluator kernel, unrestricted workflow scripting, automatic Payload translation, or unapproved browser behavior.
