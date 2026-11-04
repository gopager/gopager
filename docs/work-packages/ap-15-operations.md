# AP-15 Operations and Profile Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-15 T050..T052](source-ap-15-operations.md), preserving its approved topology amendment and deferred Lite timing.
- **Requirements and owners:** [REQ-111..120/127](../requirements-catalog.md#l-operations-data-volume-and-evolution), [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-12](../contracts/c-12-persistence-operations.md), [C-13](../contracts/c-13-account-lifecycle.md).
- **Owner:** Operations/integration and domain schema owners.
- **Architecture:** [technology decisions](../architecture.md#technology-decisions), [operating profiles](../architecture.md#operating-profiles), [supporting responsibilities](../architecture.md#supporting-responsibilities).
- **Prerequisites:** Verified AP-06 through AP-14 handovers and their fixed operational boundaries; only applicable contract/decision slices already fixed by earlier work. Any operational details required by T081-T083 that are not already fixed are planned and documented within AP-15. Do not depend on AP-16 evidence sign-off.
- **Write scope:** `internal/operations/`, packaging/operator documentation, and assigned operational evidence; shared platform changes stay with the approved integration owner.

## Planning Before Coding

Complete this planning as the first part of T081-T083; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Record contract decisions in C-12 and the evidence procedures at their canonical owner; track the work within those existing task IDs:

- **Later-consumer planning context (not prerequisites):** [AP-16 Integration and Delivery Evidence](ap-16-integration-evidence.md). Use its evidence and delivery expectations to make operational results reproducible; do not perform AP-16 sign-off here.
- For T081, define the storage-status fields, configured-retention reporting, setup prerequisites without default credentials, shutdown behavior and update/migration procedure, including observable failures and operator actions.
- For T082, define the evidence procedure and pass/fail observations for approved Full authority, lease/failover, disk/crash/restart and external-effect recovery boundaries; distinguish simulated fixtures from real integration evidence.
- For T083, document only the approved Lite/Full transition boundary and clearly identify behavior deferred from runtime implementation. State the limitations on hardware, SLA/RPO/RTO and Backup/Restore claims.

- [ ] T081 [AP-15] Implement/document storage status, configured retention, setup without default credentials, shutdown, and update/migration workflows.
- [ ] T082 [AP-15] Demonstrate approved Full authority, lease/failover, disk/crash/restart, and external-effect recovery boundaries.
- [ ] T083 [AP-15] Record the approved Lite/Full transition boundary without implementing deferred Lite runtime or executable transition scope.

**Acceptance:** Operators receive truthful machine-readable state and reproducible evidence for approved operational claims.

**Non-goals:** Application Backup/Restore, executable Lite-to-Full transition, unsupported hardware certification, quantitative SLA/RPO/RTO, or custom consensus.
