# AP-14 Identity Lifecycle Completion

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-051..080/085/105/109..110](../requirements-catalog.md#f-access-rights-and-guests), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-13](../contracts/c-13-account-lifecycle.md).
- **Owner:** Identity; integration and concrete earlier data owners participate inside this AP to implement cleanup adapters and acknowledgements.
- **Architecture:** [domain ownership](../architecture.md#domains-and-data-ownership), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verify [identity-core](../execution/source-ap-07.md#handover), supporting actions from [AP-08](ap-08-master-data-correction-after-intake.md)/[AP-09](ap-09-gateway-network.md), [AP-10](ap-10-query-live.md) saved views/current fields, [AP-11](ap-11-events-delivery.md) personal Subscription/Delivery, [AP-12](ap-12-plugin-host.md) configuration/Secrets and [AP-13](ap-13-processing-presentation-integration.md) preferences/dismissals. C-06/C-13 and supporting cleanup planning belongs here; no prior cleanup subsystem or frontend/operations input is required.
- **Write scope:** `internal/identity/`, `internal/authz/`, `internal/delivery/`, `internal/pluginhost/`, `internal/readmodel/`, `internal/processing/`, `internal/events/` for exact account-owned copies/work/acknowledgements; owner Ent adapters/schemas/generated output and next ordered migrations; `cmd/gopager/`, `internal/httpapi/`, versioned management OpenAPI and cleanup-runner/local-rescue registration. Preserve independently owned received Payload/shared configurations.

## Planning Before Coding

Record the following decisions in C-06/C-13 and supporting C-08/C-09/C-10 before coding; cases belong to their F-ID owners.

- **Later-consumer planning context (not prerequisites):** [AP-15](ap-15-frontend.md), [AP-16](ap-16-operations.md), [AP-17](ap-17-integration-evidence.md). Do not implement their workflows.

- [ ] T075 [AP-14] Complete local account/profile/grant/key and Recovery/rescue operations not delivered by the verified identity-core handover.
- [ ] T076 [AP-14] Implement account deletion, immediate invalidation, durable cleanup, audit, and pseudonymization across registered data owners.
- [ ] T077 [AP-14] Verify revocation, Recovery replay/rights, deletion progress/restart, cleanup failure, and redacted outcomes.

## Exact Participant, API and Verification Binding

T075 inventories [completed identity-core operations](../execution/source-ap-07.md#handover) and defines remaining account/profile/group/grant/key/session/recovery/rescue operations: current actions/fields, validation/errors, revisions, lifecycle and safe management/audit/status HTTP. Consume completed action/scope extensions from AP-08/AP-09/AP-10/AP-12 without redefining them. T075 makes Session absolute (30 d) and inactivity (7 d) lifetimes Operator-configurable to shorter values only ([C-06 Session rules](../contracts/c-06-identity-authorization.md#ap-02-credentials-and-recovery)); reject longer or invalid values at startup and test both expiry boundaries ([D-134](../authoritative-plan.md#review-finding-ap-planning-revision)).

T076 registers concrete participants for identity credentials/grants/sessions, AP-10 personal saved views, AP-11 personal Subscriptions/Delivery input/history, AP-12 personal plugin configuration/Secret values/references and plugin-owned copies, and AP-13 preferences/dismissals. Shared master-data/workflow/audit retains only C-13-safe attribution, not author profiles. Define transactional immediate invalidation, durable per-owner work/receipts, claims/fencing/retries, dependency unavailability and redacted status in C-06/C-13 and supporting C-08/C-09/C-10. Implement the participant adapters here against completed stores; unavailable required plugin cleanup prevents completion. No independent owner permission gate, future operations runner or blanket log/Payload purge.

T077 executes F-17/F-22/F-24/F-31/F-39 with real PostgreSQL and HTTP: proof replay/expiry/current rights, shared-location login throttling, denied grant/key expansion, immediate output/start stop, crash before/after cleanup acknowledgement, idempotent retry, absent plugin, personal versus independently shared configuration, safe distinguishing deleted-user suffixes and redacted progress. Run focused identity/authz/participant race tests, `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager`, focused vet/build and actual local-rescue command tests. Hand over management OpenAPI/action/error versions, registry/participant paths and executed evidence to AP-15/AP-16/AP-17.

**Acceptance:** Access is blocked immediately on deletion/revocation, cleanup is durable and honest, and no owner is reported clean before its required acknowledgement.

**Non-goals:** OIDC/SCIM, nested User groups, profile resurrection, anonymous-retention claims, or operator-owned backup cleanup.
