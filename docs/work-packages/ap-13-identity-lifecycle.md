# AP-13 Identity Lifecycle Completion

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Remaining historical [source AP-07 T025..T027](source-ap-07-identity-authorization.md) outside the already verified identity-core and global-field handovers.
- **Requirements and owners:** [REQ-051..080/085/105/109..110](../requirements-catalog.md#f-access-rights-and-guests), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-13](../contracts/c-13-account-lifecycle.md).
- **Owner:** DOM-05; integration coordinates deletion, and each data owner supplies its cleanup acknowledgement.
- **Architecture:** [domain ownership](../architecture.md#domains-and-data-ownership), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verified AP-06 through AP-12 handovers; only applicable contract/decision slices already fixed by earlier work. Any C-06/C-13 detail needed by T075-T077 that is not already fixed is planned and defined in this AP, not treated as an external freeze prerequisite. Do not depend on AP-14 or later work.
- **Write scope:** `internal/identity/`, `internal/authz/`, identity-owned schemas/adapters, and agreed cross-domain cleanup participants.

## Planning Before Coding

Complete this planning as the first part of T075-T077; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Record contract decisions in C-06/C-13 and case expectations at their F-case owners; track the work within those existing task IDs:

- **Later-consumer planning context (not prerequisites):** [AP-14 Frontend](ap-14-frontend.md), [AP-15 Operations](ap-15-operations.md), and [AP-16 Integration and Delivery Evidence](ap-16-integration-evidence.md). Use these to keep account-management, cleanup reporting and evidence boundaries usable by their consumers; do not implement their workflows or operations early.
- For T075, define the remaining local account/profile/grant/key and Recovery/rescue operations, including their authorized actions/fields, validation, stable errors and lifecycle transitions; do not duplicate already verified identity-core behavior.
- For T076, define immediate deletion invalidation, durable cleanup states/retries, audit and field-specific pseudonymization, plus each participating earlier data owner's exact cleanup acknowledgement and failure boundary.
- For T077, bind revocation, Recovery replay/rights, deletion restart/failure and redacted-result cases to the assigned fixtures. Do not claim cleanup completion before the named owner acknowledgements.

- [ ] T075 [AP-13] Complete local account/profile/grant/key and Recovery/rescue operations not delivered by the verified identity-core handover.
- [ ] T076 [AP-13] Implement account deletion, immediate invalidation, durable cleanup, audit, and pseudonymization across registered data owners.
- [ ] T077 [AP-13] Verify revocation, Recovery replay/rights, deletion progress/restart, cleanup failure, and redacted outcomes.

**Acceptance:** Access is blocked immediately on deletion/revocation, cleanup is durable and honest, and no owner is reported clean before its required acknowledgement.

**Non-goals:** OIDC/SCIM, nested User groups, profile resurrection, anonymous-retention claims, or operator-owned backup cleanup.
