# AP-10 Events and Delivery

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-10 T034..T036](source-ap-10-events-delivery.md).
- **Requirements and owners:** [REQ-049/081..082/084..087/105..106/129..130](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Owner:** DOM-07; AP-05 owns the shared physical lease adapter.
- **Architecture:** [event-driven architecture](../architecture.md#core-domain-event-driven-architecture-reassessment), [worker/queue assessment](../architecture.md#ap-12-durable-worker-and-job-queue-assessment), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verified AP-06 through AP-09 handovers and AP-05's fixed lease/transaction foundation; only applicable contract/decision slices already fixed by earlier work. Any C-08/C-09 detail needed by T066-T068 that is not already fixed is planned and defined in this AP, not treated as an external freeze prerequisite.
- **Write scope:** `internal/events/`, `internal/delivery/`, and owner outbox/subscription/attempt schemas and tests.

## Planning Before Coding

Complete this planning as the first part of T066-T068; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Record contract decisions in C-08/C-09 and case expectations at their F-case owners; track the work within those existing task IDs:

- **Later-consumer planning context (not prerequisites):** [AP-11 Plugin Host](ap-11-plugin-host.md), [AP-12 Processing Presentation and Integration](ap-12-processing-presentation-integration.md), and [AP-14 Frontend](ap-14-frontend.md). Use their consumer needs to keep event and delivery boundaries compatible; do not implement their plugin, processing or browser work early.
- For T066, define the event types/envelopes and selected subscriptions, ordering and publication rules, outbox atomicity with owner state, consumer identity and idempotency boundary.
- For T067, define claims/leases/fencing, per-attempt state, current authorization and age checks, retry classification/bounds, pause/revocation and explicit redelivery, including provider uncertainty and restart behavior.
- For T068, bind uncertain outcomes, lease loss, crash/restart and duplicate-risk cases to the assigned cases. Define only the C-10 integration boundary needed by this AP; do not define plugin implementation or assume a provider is available.

- [ ] T066 [AP-10] Implement Domain event selection, atomic outbox publication, subscriptions, and consumer-specific idempotency.
- [ ] T067 [AP-10] Implement per-attempt state, current authorization/age checks, bounded retry classification, pause/revocation, and explicit redelivery.
- [ ] T068 [AP-10] Verify uncertain provider outcomes, lease loss, restart, duplicate risk, and controlled-provider boundaries.

**Acceptance:** Durable event and Delivery state is truthful about acceptance, uncertainty, retry, and current rights.

**Non-goals:** Exactly-once or recall guarantees, arbitrary event fan-out, or real provider access without a separately approved environment.
