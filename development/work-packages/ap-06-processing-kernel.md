# AP-06 Processing Kernel

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [processing-kernel binding](../execution/ap-04.md#processing-kernel-execution-binding).
- **Requirements and owners:** [REQ-091..093/095/097](../requirements-catalog.md#j-processing-presentation-and-usability), [C-11](../contracts/c-11-processing-presentation.md), [C-02](../contracts/c-02-protocol-payload.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Owner:** Processing and integration for shared composition.
- **Prerequisites:** [AP-05 foundation](../execution/ap-05.md#handover), [identity/AuthZ handover](../execution/source-ap-07.md#handover) and [matcher handover](../execution/source-ap-08.md#handover). No feed, target, Delivery consumer or later AP is required.
- **Write scope:** Kernel, owner schemas/adapters/tests and root surfaces in the [exact binding](../execution/ap-04.md#processing-kernel-execution-binding).

## Kernel Scope and Verification

T055 covers selectively triggered, namespaced bounded Workflows/results, extraction/conditions, pure preview/errors, saved/active versions and declared pure installed-step invocation. Follow [C-11 evaluator/coordination](../../docs/workflows.md#worker-dispatch) and [C-10](../contracts/c-10-plugin-host.md); expose frozen evaluation/revision/result interfaces for intake publication (the publication barrier was removed by [D-135](../authoritative-plan.md#foundation-redesign-after-the-2026-11-06-re-evaluation): results attach when done, with `processing_status` on the Message and `processing_runs` as the work record; Workflow actions are `workflow.*`). Scope-existence/source owners may be declared standalone fixtures with explicit missing-production-owner failure, never fabricated readiness.

T056 verifies [F-21 preview](../contracts/reference-cases.md), [F-37/F-38](../contracts/ap-02-boundary-cases.md), regex/Payload/output budgets, sensitivity, version/revision conflicts and no preview effects. The exact binding owns required selectors, PostgreSQL and HTTP checks. Consumer source-bound integration belongs to AP-07/AP-13, not standalone kernel acceptance.

- [ ] T055 [AP-06] Complete the already approved standalone Processing evaluator/kernel slice and expose its frozen evaluation, revision, result, and invocation boundaries.
- [ ] T056 [AP-06] Complete the kernel's standalone conformance evidence and verified handover; defer source-bound consumer integration to its later path position.

**Acceptance:** The frozen kernel surface is implemented, protected, and verified against its bound cases; no later package output is needed to complete this assignment.

**Non-goals:** Read/feed presentation, external Delivery, general plugin targets, or product behavior beyond the frozen kernel binding.
