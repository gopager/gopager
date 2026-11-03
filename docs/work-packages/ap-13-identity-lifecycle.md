# AP-13 Identity Lifecycle Completion

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Remaining historical [source AP-07 T025..T027](source-ap-07-identity-authorization.md) outside the already verified identity-core and global-field handovers.
- **Requirements and owners:** [REQ-051..080/085/105/109..110](../requirements-catalog.md#f-access-rights-and-guests), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-13](../contracts/c-13-account-lifecycle.md).
- **Owner:** DOM-05; integration coordinates deletion, and each data owner supplies its cleanup acknowledgement.
- **Prerequisites:** AP-06 through AP-12; applicable AP-04 freezes and this slice's exact execution binding.
- **Write scope:** `internal/identity/`, `internal/authz/`, identity-owned schemas/adapters, and agreed cross-domain cleanup participants.

- [ ] T075 [AP-13] Complete local account/profile/grant/key and Recovery/rescue operations not delivered by the verified identity-core handover.
- [ ] T076 [AP-13] Implement account deletion, immediate invalidation, durable cleanup, audit, and pseudonymization across registered data owners.
- [ ] T077 [AP-13] Verify revocation, Recovery replay/rights, deletion progress/restart, cleanup failure, and redacted outcomes.

**Acceptance:** Access is blocked immediately on deletion/revocation, cleanup is durable and honest, and no owner is reported clean before its required acknowledgement.

**Non-goals:** OIDC/SCIM, nested User groups, profile resurrection, anonymous-retention claims, or operator-owned backup cleanup.
