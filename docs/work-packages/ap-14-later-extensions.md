# AP-14 Later Extension Boundaries

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [later delivery scope](../authoritative-plan.md#delivery-scope), [D-30 deferral](../authoritative-plan.md#confirmed-decisions), [approval gates](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [nested groups](../requirements-catalog.md#f-access-rights-and-guests), [external identity](../requirements-catalog.md#g-accounts-and-login), [external recovery boundary](../requirements-catalog.md#h-recovery-and-access-keys), [cross-plugin events](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [PWA](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [registration/import/SCIM](../requirements-catalog.md#m-additional-domain-capabilities).
- **Architecture:** [extension ownership](../architecture.md#domains-and-data-ownership), [dependency rules](../architecture.md#allowed-dependencies).
- **Prepared contracts:** [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-13](../contracts/c-13-account-lifecycle.md), [C-02](../contracts/c-02-protocol-payload.md)/[C-03](../contracts/c-03-message-grouping.md) for deferred comparison; [shared conventions](../contracts/shared-conventions.md).
- **Cases and approval handover:** Existing [F-10a..F-10e](../contracts/reference-cases.md) protect exact-only scope, not future heuristic acceptance. Define extension-specific cases only after separate [AP-04 approval](ap-04-scope-freeze.md).

**Owner:** Respective domain with product/integration ownership. **Contracts:** Prepared C-06/C-07/C-09/C-10/C-13 boundaries; concrete extension contracts later. **REQ:** REQ-053, REQ-065..068, REQ-080, REQ-083, REQ-101..104, REQ-125, REQ-128, REQ-131. **Current write scope:** Referenced contract boundaries only; no implementation paths approved.

**Prerequisites:** Separate later scope decision in the plan and approval of the specific extension contracts; no initial implementation is authorized by this package.

- [ ] T049 [AP-14] After separate later scope approval, specify nested User groups, external identity/proxy/SCIM, cross-plugin Domain events, PWA/push, registration, or import/export as their own concrete contract/package slices.

**Acceptance now:** Requirements/prepared boundaries are preserved without claiming implementation. **Non-goals:** Implementing these extensions under this documentation task. delivery status remains exclusively in the plan. I18n readiness is initial preparation, not deferred to AP-14; additional translations remain unrequired.

**Deferred comparison extension (D-30):** Non-exact Payload heuristics require a later separately approved C-02/C-03 slice, including candidate scope, metrics/thresholds, ambiguity, correction/reference/event behavior and authorization. Current scope preserves originals and an exact comparison boundary only. The interrupted text-default/prefix/length-ratio discussion is not an active configuration contract. This records a handover, not authorization to execute this package.
