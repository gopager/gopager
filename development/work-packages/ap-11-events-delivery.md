# AP-11 Events and Delivery

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** server side of the following (screens: AP-15):
  - [REQ-081..082/086..087/105..106/130](../requirements-catalog.md#i-domain-events-extensions-and-deliveries) Domain events, Subscriptions and Delivery.
  - REQ-049: Delivery triggers on AP-09's Gateway state facts.
  - REQ-084: configuration revision and authority in the Core target descriptor; configuring instances is AP-12.
  - REQ-085: users pause or remove their Subscriptions; the effect of owner suspension/deletion is AP-14.
  - REQ-108: sending admission (query: AP-10, login: AP-14, provider limits: AP-12).
  - REQ-129: the per-Unit parameter table and its use in materialization; managing the values is AP-12.
  - [REQ-091..095](../requirements-catalog.md#j-processing-presentation-and-usability): Core Delivery rendering of the layout definition; GUI cards and presentation are AP-13.
  - [REQ-139](../requirements-catalog.md#i-domain-events-extensions-and-deliveries) only as compatibility context (Later).
- **Inputs:**
  - [AP-05 handover](../execution/ap-05.md#handover): transactions and clock.
  - [AP-06 handover](../execution/ap-06.md#handover): kernel.
  - [AP-07 handover](../execution/ap-07.md#handover): intake.
  - [AP-08 handover](../execution/ap-08.md#handover): correction, suppression, Agencies, default facts/lookups.
  - [AP-09](ap-09-gateway-network.md): Gateway state facts; verify `development/execution/ap-09.md#handover` exists at start.
  - [AP-20 handover](../execution/ap-20.md#handover): `assignment.changed.previousNetworkId` with an empty target list and `correction.failed` with the reason `gateway-mapping-change` ([D-141](../decisions/03-assignment-and-correction.md#d-141)).
  - [AP-10](ap-10-query-live.md): authorized projection/filter and its event-consumer framework (the sequenced `domain_events` log consumers); verify `development/execution/ap-10.md#handover` exists at start.
  - Verify actual handovers. No generic lease adapter or later host/renderer is assumed.
- **Owned contract slices:** [C-08](../contracts/c-08-events-jobs.md) and [C-09](../contracts/c-09-delivery.md), plus the required [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-10](../contracts/c-10-plugin-host.md) input/attempt slices and [C-11 shared rendering](../contracts/c-11-processing-presentation.md#presentation-and-configuration-planned), with canonical cases. The shared Core materialization/rendering code under `internal/processing/` is part of this AP. This AP owns Delivery production claims/leases; AP-05 supplies only fixed transactions/primary clock and synthetic fencing evidence.
- **Reference (read when needed):** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope), [approval](../authoritative-plan.md#workflow-and-approval-gates), [D-135](../decisions/10-foundation-redesign.md#d-135) item 10 (one sequenced `domain_events` log) and item 9 as amended by [D-137](../decisions/10-foundation-redesign.md#d-137) (Processing candidates), [allowed dependencies](../architecture.md#allowed-dependencies), [Domain events and outbox](../../docs/architecture.md#domain-events-and-outbox), [Processing (worker role)](../../docs/architecture.md#processing-worker-role).
- **Write scope:**
  - `internal/events/` — event selection, facts, subscriptions.
  - `internal/delivery/` (new) — Delivery claims, attempts, management.
  - `internal/processing/` — shared bounded renderer only.
  - `internal/readmodel/`, `internal/units/`, `internal/authz/` — supporting query/default/authority adapters.
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/eventstore/`, a new Delivery store under `internal/persistence/ent/`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout)) — Subscriptions, attempts, claims, per-Unit parameters.
  - `go.mod`, `go.sum` — only for a library selected under [library reuse](../README.md#library-reuse).
  - `internal/httpapi/` — handlers and OpenAPI.
  - `cmd/gopager/` — runner/root registration only.
- **Compatibility context:** later-consumer planning (not prerequisites): [AP-12 Host](ap-12-plugin-host.md), [AP-13 Presentation](ap-13-processing-presentation-integration.md), [AP-15 Frontend](ap-15-frontend.md). Shared Core input/attempt contracts and rendering are required by current materialization, not speculative preparation. AP-12 later produces real validated manifest/configuration descriptors and target invocations conforming to this AP's contract.

## Open User Decisions

None.

## Planning Before Coding

Record the following decisions in C-08/C-09 and supporting C-06/C-07/C-10/C-11 before coding; cases belong to their F-ID owners.

T066 defines the completed Core target descriptor (typed inputs, classified dependencies, configuration revision/authority, selected output context), attempt invocation/result port and their versions in C-09/C-10. T066's attempt invocation contract carries the triggering fact's C-08 `correlationId`/`causationId` as the envelope correlation reference ([D-134](../decisions/09-process-and-documentation.md#d-134)). Implement the full Core authorized projection/filter/target mapping/split/default/revision/idempotency flow using those contracts. Implement bounded Delivery rendering of the declarative layout definition owned by C-11/C-09 (REQ-094: blocks `text`, `field`, `chip`, `link`, `section`, `conditional` on the C-09 typed predicate, `fallback`; plain text, Markdown or JSON output, never raw HTML; no general template engine; precedence installation default, Unit/Organization layout, Subscription layout) in C-11; GUI-specific cards/editors belong to AP-13/AP-15. No second expression parser or dummy production host.

AP-12 later produces real validated manifest/configuration descriptors and target invocations conforming to this contract. AP-11 exercises the same complete Core flow with controlled providers supplied by its tests. Production with no configured provider is visibly unavailable; do not claim initial targets or external delivery have been implemented here. Subscription/Delivery HTTP operations are real current Core APIs, not stubs, and may reject unavailable targets explicitly.

T066/T067 define event types/envelopes/order, selection/publication atomicity, canonical lookup/unknown-version/missing-reference handling, domain receipts/work commit before Ack, catch-up/retention/lifecycle, replay/correction suppression and unique fan-out. Per-Unit Delivery overrides (REQ-129): T066 creates the per-integration table `integration_unit_parameters(instance_id, unit_id, key, value)` (new migration, `instance_id` references the existing plugin instances), defines it in C-09 and applies the values in materialization; plugin Extension fields are dropped. Authoring the values through the integration's configuration API is AP-12. Define Delivery claims/token/primary-time expiry, per-attempt state, retry classification/bounds, provider uncertainty and restart behavior. Transport offsets are not domain work leases or idempotency.

Keep the event model open for [REQ-139](../requirements-catalog.md#i-domain-events-extensions-and-deliveries) plugin reactions under [D-130](../decisions/04-processing-and-plugins.md#d-130):
- The versioned event-type catalog covers all producing domains, including identity/account lifecycle, master data, Gateways, corrections and operations, not only Messages.
- A Subscription target is a typed reference, so a later plugin reaction capability can be a target kind next to Delivery targets.
- Global Subscriptions run under a technical Principal, and facts carry IDs and classified fields only.

This is design compatibility, not reaction implementation.

Define Subscription CRUD/selection/target mappings, pause/retry/redeliver/status management APIs, current data/field/age/Assignment/Suppression/configuration checks, split identity, minimal retained inputs and terminal/24h expiry. Define sending admission/rate fairness under REQ-108 and machine-readable throttling; provider-specific policy is AP-12's own later work, not a prerequisite for generic admission.

## Tasks

- [ ] T066 [AP-11] Implement Domain event selection (on AP-10's consumer framework), atomic fact writing, subscriptions, Core materialization/shared rendering, and consumer-specific idempotency.
  - **Done when:** the Core flow (authorized projection/filter/target mapping/split/default/revision/idempotency) runs on AP-10's consumer framework with atomic fact writing; bounded shared rendering per REQ-094 works; Subscription/Delivery HTTP operations are real and reject unavailable targets explicitly; production with no configured provider is visibly unavailable; hidden defaults/split conflicts, save-time field validation and input expiry cases hold.
- [ ] T067 [AP-11] Implement production claims/leases/fencing, per-attempt state, current authorization/age checks, bounded retry/sending admission, pause/revocation, and explicit redelivery.
  - **Done when:** Delivery claims/token/primary-time expiry, per-attempt state, retry classification/bounds, restart behavior, current data/field/age/Assignment/Suppression/configuration checks, pause/revocation and explicit redelivery work; fair sending admission under REQ-108 returns machine-readable throttling.
- [ ] T068 [AP-11] Verify uncertain provider outcomes, lease loss, restart, duplicate risk, rendering/materialization and controlled-provider boundaries.
  - **Done when:** [F-02/F-15/F-17..F-19/F-23/F-25](../contracts/reference-cases.md), [F-33..F-35](../contracts/ap-02-boundary-cases.md), duplicate fact/claim/restart/commit rollback, `processing.completed`-triggered reactions, superseded token, revocation before/after started I/O, uncertain outcomes/no blind retry, historical no-send, hidden defaults/split conflicts, save-time field validation, input expiry and fair sending admission pass; durable event and Delivery state is truthful about acceptance, uncertainty, retry, and current rights.

## Documentation to Update

- Development owners: [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), supporting [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-10](../contracts/c-10-plugin-host.md), [C-11](../contracts/c-11-processing-presentation.md#presentation-and-configuration-planned); cases in [reference cases](../contracts/reference-cases.md) and [boundary cases](../contracts/ap-02-boundary-cases.md); [glossary](../glossary.md#maintenance-rules) for new terms.
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md#route-reference) (new Subscription and Delivery section).
- [docs/architecture.md](../../docs/architecture.md#domain-events-and-outbox) for Delivery data flow; [docs/operations.md](../../docs/operations.md#roles) and [Configuration Reference](../../docs/operations.md#configuration-reference) for the Delivery runner and configuration; [docs/workflows.md](../../docs/workflows.md#events) if event behavior shared with Processing changes; Go doc comments of changed exported identifiers.

## Verification

Run focused event/delivery/renderer race tests, real PostgreSQL/HTTP transaction integration and controlled provider process tests; existing generation/vet/build.

## Handover

Hand over actual input/attempt/renderer/OpenAPI versions, claims/stores/facts and executed tests to AP-12..AP-17.

## Non-goals

Exactly-once or recall guarantees, implemented plugin reactions to non-Message events (REQ-139 stays open for later planning), or real provider access without a separately approved environment.
