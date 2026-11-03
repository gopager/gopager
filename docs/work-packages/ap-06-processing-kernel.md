# AP-06 Processing Kernel

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [processing-kernel binding](../execution/ap-04.md#processing-kernel-execution-binding).
- **Source scope:** The evaluator/kernel slice of historical [source AP-12 T043](source-ap-12-processing-presentation.md); the prior partial implementation and evidence remain in the [source AP-12 execution record](../execution/source-ap-12.md).
- **Requirements and owners:** [REQ-091..093/095/097](../requirements-catalog.md#j-processing-presentation-and-usability), [C-11](../contracts/c-11-processing-presentation.md), [C-02](../contracts/c-02-protocol-payload.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Owner:** DOM-08 and integration for shared composition.
- **Prerequisites:** AP-05 foundation; only already verified identity and matcher handovers from historical [source AP-07](../execution/source-ap-07.md) and [source AP-08](../execution/source-ap-08.md), produced before this ordered path began. These are existing completed inputs, not dependencies on future current-path APs. No completed feed, plugin target, Delivery consumer, or later package is required.
- **Write scope:** Existing T043 kernel and its owner schemas/adapters/tests, under the exact approved binding. This is a package-boundary relocation, not new product scope.

- [ ] T055 [AP-06] Complete the already approved standalone Processing evaluator/kernel slice and expose its frozen evaluation, revision, result, and invocation boundaries.
- [ ] T056 [AP-06] Complete the kernel's standalone conformance evidence and verified handover; defer source-bound consumer integration to its later path position.

**Acceptance:** The frozen kernel surface is implemented, protected, and verified against its bound cases; no later package output is needed to complete this assignment.

**Non-goals:** Read/feed presentation, external Delivery, general plugin targets, or product behavior beyond the frozen kernel binding.
