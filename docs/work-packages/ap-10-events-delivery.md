# AP-10 Domain events and Delivery Core

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-05](ap-05-runtime-persistence.md), [AP-09](ap-09-query-live.md), [AP-11](ap-11-plugins.md).

**Owner:** DOM-07. **Contract gate:** C-08/C-09 and agreed C-10 host invocation. **REQ:** REQ-049, REQ-081..082, REQ-084..087, REQ-105..106, REQ-129..130. **Prerequisites:** AP-05..AP-09. **Exclusive target scopes:** `internal/events/`, `internal/delivery/`, domain outbox/subscription/attempt schema objects. Physical lease adapter integrated with AP-05 owner.

- [ ] T034 [AP-10] Implement Domain events, atomic outbox, subscriptions/Delivery targets, consumer-specific idempotency.
- [ ] T035 [AP-10] Implement current Delivery projection per attempt, pause/revocation/age, bounded retry classification, explicit redelivery with separate attempts.
- [ ] T036 [AP-10] Test F-02/F-17/F-18/F-23/F-25, including accepted provider request with lost response, lease loss and ordinary restart; cover host boundary with controlled providers. No application Backup/Restore logic (D-31).

**Acceptance:** No hidden once-per-Message behavior, no old Payload as authorization proof, uncertain effects visible. **Non-goals:** Exactly-once/recall guarantees or arbitrary Domain event fan-out. Real provider evidence only in AP-11 with an explicit access environment.
