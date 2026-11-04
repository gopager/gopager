# AP-11 Events and Delivery

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-049/081..082/084..087/105..106/108/129..130](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md) and [C-11 shared rendering](../contracts/c-11-processing-presentation.md#presentation-and-configuration).
- **Owner:** DOM-07 with integration and DOM-08 participating in current Core materialization/rendering work. This AP owns Delivery production claims/leases; AP-05 supplies only fixed transactions/primary clock and synthetic fencing evidence.
- **Architecture:** [event-driven architecture](../architecture.md#core-domain-event-driven-architecture-reassessment), [worker/queue assessment](../architecture.md#ap-12-durable-worker-and-job-queue-assessment), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** [AP-05](../execution/ap-05.md) transactions/clock, [AP-06](../execution/ap-06.md)/[AP-07](../execution/ap-07.md) kernel/intake publication, [AP-08](ap-08-master-data-correction-after-intake.md) correction/suppression/default facts/lookups, [AP-09](ap-09-gateway-network.md) Gateway state facts and [AP-10](ap-10-query-live.md) authorized projection/filter. Verify actual handovers and [postgresql-fact-publication 1.0](../contracts/c-08-events-jobs.md#postgresql-fact-publication). No generic lease adapter or later host/renderer is assumed.
- **Write scope:** `internal/events/`, `internal/delivery/`, shared bounded renderer under `internal/processing/`, supporting query/default/authority adapters in `internal/readmodel/`, `internal/Units/`, `internal/authz/`; owner Ent outbox/subscription/attempt/claim/rendering adapters/schemas/generated output/next migrations; `cmd/gopager/`, `internal/httpapi/`/OpenAPI and actual runner/root registration. C-08/C-09 and required C-06/C-07/C-10/C-11 input/attempt/rendering slices, canonical cases and verified necessary library manifests are authorized.

## Planning Before Coding

Record the following decisions in C-08/C-09 and supporting C-06/C-07/C-10/C-11 before coding; cases belong to their F-ID owners.

- **Later-consumer planning context (not prerequisites):** [AP-12 Host](ap-12-plugin-host.md), [AP-13 Presentation](ap-13-processing-presentation-integration.md), [AP-15 Frontend](ap-15-frontend.md). Shared Core input/attempt contracts and rendering are required by current materialization, not speculative preparation.

- [ ] T066 [AP-11] Implement Domain event selection, atomic publication, subscriptions, Core materialization/shared rendering, and consumer-specific idempotency.
- [ ] T067 [AP-11] Implement production claims/leases/fencing, per-attempt state, current authorization/age checks, bounded retry/sending admission, pause/revocation, and explicit redelivery.
- [ ] T068 [AP-11] Verify uncertain provider outcomes, lease loss, restart, duplicate risk, rendering/materialization and controlled-provider boundaries.

## Exact Materialization, Management and Verification

T066 defines the completed Core target descriptor (typed inputs, classified dependencies, configuration revision/authority, selected output context), attempt invocation/result port and their versions in C-09/C-10. Implement the full Core authorized projection/filter/target mapping/split/default/revision/idempotency flow using those contracts. Select/pin an established constrained engine and implement bounded Delivery rendering in C-11; GUI-specific cards/editors belong to AP-13/AP-15. No second expression parser or dummy production host.

AP-12 later produces real validated manifest/configuration descriptors and target invocations conforming to this contract. AP-11 exercises the same complete Core flow with controlled providers supplied by its tests. Production with no configured provider is visibly unavailable; do not claim initial targets or external delivery have been implemented here. Subscription/Delivery HTTP operations are real current Core APIs, not stubs, and may reject unavailable targets explicitly.

T066/T067 define event types/envelopes/order, selection/publication atomicity, canonical lookup/unknown-version/missing-reference handling, domain receipts/work commit before Ack, catch-up/retention/lifecycle, replay/correction suppression and unique fan-out. Define Delivery claims/token/primary-time expiry, per-attempt state, retry classification/bounds, provider uncertainty and restart behavior. Transport offsets are not domain work leases or idempotency.

Define Subscription CRUD/selection/target mappings, pause/retry/redeliver/status management APIs, current data/field/age/Assignment/Suppression/configuration checks, split identity, minimal retained inputs and terminal/24h expiry. Define sending admission/rate fairness under REQ-108 and machine-readable throttling; provider-specific policy is AP-12's own later work, not a prerequisite for generic admission.

T068 covers [F-02/F-15/F-17..F-19/F-23/F-25](../contracts/reference-cases.md), [F-33..F-35](../contracts/ap-02-boundary-cases.md), duplicate fact/claim/restart/commit rollback, superseded token, revocation before/after started I/O, uncertain outcomes/no blind retry, historical no-send, field indeterminacy, hidden defaults/split conflicts, input expiry and fair sending admission. Run focused event/delivery/renderer race tests, real PostgreSQL/HTTP transaction integration and controlled provider process tests; existing generation/vet/build. Hand over actual input/attempt/renderer/OpenAPI versions, claims/stores/facts and executed tests to AP-12..AP-17.

**Acceptance:** Durable event and Delivery state is truthful about acceptance, uncertainty, retry, and current rights.

**Non-goals:** Exactly-once or recall guarantees, arbitrary event fan-out, or real provider access without a separately approved environment.
