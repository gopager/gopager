# Source AP-17 Processing Kernel

Historical assignment baseline from the D-96 execution path. Its current, sequentially numbered replacement is [AP-06 Processing Kernel](ap-06-processing-kernel.md), with task IDs T055/T056 retained and current execution state in [AP-06's execution record](../execution/source-ap-06.md).

[Historical execution record](../execution/source-ap-17.md) | [Historical AP-12 source baseline](source-ap-12-processing-presentation.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-96 reslicing approval](../authoritative-plan.md#work-package-reslicing-decision), [processing-kernel binding](../execution/ap-04.md#processing-kernel-execution-binding).
- **Source scope:** The evaluator/kernel slice of historical [AP-12 T043](source-ap-12-processing-presentation.md); the prior partial implementation and evidence remain in the [source AP-12 execution record](../execution/source-ap-12.md).
- **Requirements and owners:** [REQ-091..093/095/097](../requirements-catalog.md#j-processing-presentation-and-usability), [C-11](../contracts/c-11-processing-presentation.md), [C-02](../contracts/c-02-protocol-payload.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Owner:** DOM-08 and integration for shared composition.
- **Prerequisites:** AP-05 foundation; the already verified AP-07 identity and AP-08 matcher handovers linked by the source execution record. No completed feed, plugin target, Delivery consumer, or later package is required.
- **Write scope:** Existing T043 kernel and its owner schemas/adapters/tests, under the exact approved binding. This is a package-boundary relocation, not new product scope.

- [ ] T055 [AP-17] Complete the already approved standalone Processing evaluator/kernel slice and expose its frozen evaluation, revision, result, and invocation boundaries.
- [ ] T056 [AP-17] Complete the kernel's standalone conformance evidence and verified handover; defer source-bound consumer integration to its later path position.

**Acceptance:** The frozen kernel surface is implemented, protected, and verified against its bound cases; no later package output is needed to complete this assignment.

**Non-goals:** Read/feed presentation, external Delivery, general plugin targets, or product behavior beyond the frozen kernel binding.
