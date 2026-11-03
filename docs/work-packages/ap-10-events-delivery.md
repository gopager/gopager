# AP-10 Events and Delivery

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-10 T034..T036](source-ap-10-events-delivery.md).
- **Requirements and owners:** [REQ-049/081..082/084..087/105..106/129..130](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Owner:** DOM-07; AP-05 owns the shared physical lease adapter.
- **Prerequisites:** AP-06 through AP-09; AP-05 lease/transaction foundation; applicable AP-04 freezes and this slice's exact execution binding.
- **Write scope:** `internal/events/`, `internal/delivery/`, and owner outbox/subscription/attempt schemas and tests.

- [ ] T066 [AP-10] Implement Domain event selection, atomic outbox publication, subscriptions, and consumer-specific idempotency.
- [ ] T067 [AP-10] Implement per-attempt state, current authorization/age checks, bounded retry classification, pause/revocation, and explicit redelivery.
- [ ] T068 [AP-10] Verify uncertain provider outcomes, lease loss, restart, duplicate risk, and controlled-provider boundaries.

**Acceptance:** Durable event and Delivery state is truthful about acceptance, uncertainty, retry, and current rights.

**Non-goals:** Exactly-once or recall guarantees, arbitrary event fan-out, or real provider access without a separately approved environment.
