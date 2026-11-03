# AP-15 Operations and Profile Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-15 T050..T052](source-ap-15-operations.md), preserving its approved topology amendment and deferred Lite timing.
- **Requirements and owners:** [REQ-111..120/127](../requirements-catalog.md#l-operations-data-volume-and-evolution), [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-12](../contracts/c-12-persistence-operations.md), [C-13](../contracts/c-13-account-lifecycle.md).
- **Owner:** Operations/integration and domain schema owners.
- **Prerequisites:** AP-06 through AP-14; operational boundaries and applicable AP-04 approvals remain authoritative.
- **Write scope:** `internal/operations/`, packaging/operator documentation, and assigned operational evidence; shared platform changes stay with the approved integration owner.

- [ ] T081 [AP-15] Implement/document storage status, configured retention, setup without default credentials, shutdown, and update/migration workflows.
- [ ] T082 [AP-15] Demonstrate approved Full authority, lease/failover, disk/crash/restart, and external-effect recovery boundaries.
- [ ] T083 [AP-15] Record the approved Lite/Full transition boundary without implementing deferred Lite runtime or executable transition scope.

**Acceptance:** Operators receive truthful machine-readable state and reproducible evidence for approved operational claims.

**Non-goals:** Application Backup/Restore, executable Lite-to-Full transition, unsupported hardware certification, quantitative SLA/RPO/RTO, or custom consensus.
