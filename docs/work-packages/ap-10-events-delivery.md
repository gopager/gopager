# AP-10 Domain events and Delivery Core

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [Gateway events](../requirements-catalog.md#e-transmission-networks-and-gateways), [subscriptions/Delivery](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [current authorization](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [Unit settings/reactions](../requirements-catalog.md#m-additional-domain-capabilities).
- **Architecture:** [DOM-07 ownership](../architecture.md#domains-and-data-ownership), [transaction/I/O dependencies](../architecture.md#allowed-dependencies), [Delivery coordination](../architecture.md#shared-use-cases).
- **Owner contracts:** [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-06 AuthZ](../contracts/c-06-identity-authorization.md), [C-07 projection](../contracts/c-07-query-live.md); [shared conventions](../contracts/shared-conventions.md).
- **Cases and prerequisite outputs:** [F-02/F-17/F-18/F-23/F-25](../contracts/reference-cases.md), [F-33..F-35](../contracts/ap-02-boundary-cases.md); [AP-05](ap-05-runtime-persistence.md), [AP-06](ap-06-transmission-messages.md), [AP-07](ap-07-identity-authorization.md), [AP-08](ap-08-master-data-correction.md), [AP-09](ap-09-query-live.md), [AP-04 freeze](ap-04-scope-freeze.md). [AP-11](ap-11-plugins.md) consumes the host handover; it is not a prerequisite.

**Owner:** DOM-07. **Contract gate:** C-08/C-09 and agreed C-10 host invocation. **REQ:** REQ-049, REQ-081..082, REQ-084..087, REQ-105..106, REQ-129..130. **Prerequisites:** AP-05..AP-09. **Exclusive target scopes:** `internal/events/`, `internal/delivery/`, domain outbox/subscription/attempt schema objects. Physical lease adapter integrated with AP-05 owner.

- [ ] T034 [AP-10] Implement Domain events, atomic outbox, subscriptions/Delivery targets, consumer-specific idempotency.
- [ ] T035 [AP-10] Implement current Delivery projection per attempt, pause/revocation/age, bounded retry classification, explicit redelivery with separate attempts.
- [ ] T036 [AP-10] Test F-02/F-17/F-18/F-23/F-25, including accepted provider request with lost response, lease loss and ordinary restart; cover host boundary with controlled providers. No application Backup/Restore logic (D-31).

**Acceptance:** No hidden once-per-Message behavior, no old Payload as authorization proof, uncertain effects visible. **Non-goals:** Exactly-once/recall guarantees or arbitrary Domain event fan-out. Real provider evidence only in AP-11 with an explicit access environment.
