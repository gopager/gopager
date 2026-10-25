# AP-15 Operations and Profile Transition

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-05](ap-05-runtime-persistence.md), [AP-12](ap-12-processing-presentation.md).

**Owner:** Operations/integration and domain schema owners. **Contract gate:** C-12/C-13, C-01/C-08/C-09 restart/idempotency/attempt boundaries. **REQ:** REQ-111..120, REQ-127; REQ-126 removed under D-31. **Prerequisites:** AP-05..AP-12. **Exclusive target scopes:** `internal/operations/`, future `packaging/` and deployment/operator artifacts. Platform/root changes by AP-05 integration owner only; define new operations documents before package approval.

- [ ] T050 [AP-15] Implement/document storage status, configured retention, setup without default credentials, orderly shutdown, update/migration workflows; use English project/operator text and stable machine-readable status codes.
- [ ] T051 [AP-15] Implement/test Lite->Full transition preserving IDs, grants, configuration, history, pending work and Secret references. No application Backup/Restore subsystem (D-31).
- [ ] T052 [AP-15] Demonstrate single-process Lite, Full authority/lease/failover boundaries, disk failure/crash/restart, F-19/F-23/F-25. Ordinary restart must not duplicate recorded external effects; manual old-backup risks belong to the Operator.

**Acceptance:** Traceable actual profile-transition/restart evidence, honest acceptance state, no silent retention. **Non-goals:** Application Backup/Restore or restore detection/reconciliation, custom consensus, quantitative RPO/RTO/SLA, unverified hardware support.
