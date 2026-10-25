# AP-15 Operations and Profile Transition

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates), [planning load profiles](../authoritative-plan.md#operating-assumptions).
- **Requirements:** [operations/profile transition](../requirements-catalog.md#l-operations-data-volume-and-evolution), [load-profile consideration and removed Backup/Restore scope](../requirements-catalog.md#m-additional-domain-capabilities); select the REQ IDs below, preserving removed REQ-126.
- **Architecture:** [operating profiles](../architecture.md#operating-profiles), [TECH-01/02/05/06/07](../architecture.md#technology-decisions), [operations/schema ownership](../architecture.md#supporting-responsibilities).
- **Owner contracts:** [C-12](../contracts/c-12-persistence-operations.md), [C-13](../contracts/c-13-account-lifecycle.md), [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md); [shared conventions](../contracts/shared-conventions.md).
- **Cases and prerequisite outputs:** [F-19/F-23/F-25](../contracts/reference-cases.md), [F-39..F-41](../contracts/ap-02-boundary-cases.md); [AP-05](ap-05-runtime-persistence.md), [AP-06](ap-06-transmission-messages.md), [AP-07](ap-07-identity-authorization.md), [AP-08](ap-08-master-data-correction.md), [AP-09](ap-09-query-live.md), [AP-10](ap-10-events-delivery.md), [AP-11](ap-11-plugins.md), [AP-12](ap-12-processing-presentation.md), [AP-04 freeze](ap-04-scope-freeze.md); [execution/evidence records](../execution/README.md).

**Owner:** Operations/integration and domain schema owners. **Contract gate:** C-12/C-13, C-01/C-08/C-09 restart/idempotency/attempt boundaries. **REQ:** REQ-111..120, REQ-127; REQ-126 removed under D-31. **Prerequisites:** AP-05..AP-12. **Exclusive target scopes:** `internal/operations/`, future `packaging/` and deployment/operator artifacts. Platform/root changes by AP-05 integration owner only; define new operations documents before package approval.

- [ ] T050 [AP-15] Implement/document storage status, configured retention, setup without default credentials, orderly shutdown, update/migration workflows; use English project/operator text and stable machine-readable status codes.
- [ ] T051 [AP-15] Implement/test Lite->Full transition preserving IDs, grants, configuration, history, pending work and Secret references. No application Backup/Restore subsystem (D-31).
- [ ] T052 [AP-15] Demonstrate single-process Lite, Full authority/lease/failover boundaries, disk failure/crash/restart, F-19/F-23/F-25. Ordinary restart must not duplicate recorded external effects; manual old-backup risks belong to the Operator.

**Acceptance:** Traceable actual profile-transition/restart evidence, honest acceptance state, no silent retention. **Non-goals:** Application Backup/Restore or restore detection/reconciliation, custom consensus, quantitative RPO/RTO/SLA, unverified hardware support.
