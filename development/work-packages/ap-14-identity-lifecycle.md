# AP-14 Identity Lifecycle Completion

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-051..052/054..064/069..071/073..079/085/109..110](../requirements-catalog.md#f-access-rights-and-guests) (REQ-072/076 recovery requests and automatic dispatch are dropped by D-135), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-13](../contracts/c-13-account-lifecycle.md).
- **Owner:** Identity; integration and concrete earlier data owners participate inside this AP to implement the deletion steps for their data.
- **Architecture:** [domain ownership](../architecture.md#domains-and-data-ownership), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verify [identity-core](../execution/source-ap-07.md#handover), supporting actions from the [AP-08 handover](../execution/ap-08.md#handover)/[AP-09](ap-09-gateway-network.md), [AP-10](ap-10-query-live.md) saved views/current fields, [AP-11](ap-11-events-delivery.md) personal Subscription/Delivery, [AP-12](ap-12-plugin-host.md) configuration/Secrets and [AP-13](ap-13-processing-presentation-integration.md) preferences/dismissals. C-06/C-13 and supporting deletion planning belongs here; no prior cleanup subsystem or frontend/operations input is required.
- **Write scope:** `internal/identity/`, `internal/authz/`, `internal/delivery/`, `internal/pluginhost/` (if extracted), `internal/readmodel/`, `internal/processing/`, `internal/events/` for exact account-owned copies/work/acknowledgements; owner Ent adapters/schemas/generated output and next ordered migrations; `internal/httpapi/` handlers, versioned management OpenAPI, `cmd/gopager/` local-rescue command and root registration only. Preserve independently owned received Payload/shared configurations.

## Planning Before Coding

Record the following decisions in C-06/C-13 and supporting C-08/C-09/C-10 before coding; cases belong to their F-ID owners.

- **Later-consumer planning context (not prerequisites):** [AP-15](ap-15-frontend.md), [AP-16](ap-16-operations.md), [AP-17](ap-17-integration-evidence.md). Do not implement their workflows.

- [ ] T075 [AP-14] Complete local account/profile/grant/key and Recovery (administrator-issued code, consume, CLI rescue) operations not delivered by the verified identity-core handover.
- [ ] T076 [AP-14] Implement account deletion as one transaction with immediate invalidation, audit, pseudonymization and the `principal.deleted` fact.
- [ ] T077 [AP-14] Verify revocation, Recovery replay/rights, deletion atomicity/rollback, plugin best-effort reaction status, and redacted outcomes.

## Exact Participant, API and Verification Binding

T075 inventories [completed identity-core operations](../execution/source-ap-07.md#handover) and defines remaining account/profile/group/grant/key/session/recovery/rescue operations. Recovery keeps REQ-071 (change), REQ-073 (an administrator issues a 30 min one-time code), REQ-074 (consume with username) and REQ-075 (CLI rescue); recovery request records, approval modes, automatic dispatch and per-account throttles (REQ-072/076) are not built. Access keys keep action and scope restrictions only (REQ-077/078; no field or age subsets): current actions/fields, validation/errors, revisions, lifecycle and safe management/audit/status HTTP. Consume completed action/scope extensions from AP-08/AP-09/AP-10/AP-12 without redefining them. T075 makes Session absolute (30 d) and inactivity (7 d) lifetimes Operator-configurable to shorter values only ([C-06 Session rules](../../docs/http-api.md#sessions-browser-login)); reject longer or invalid values at startup and test both expiry boundaries ([D-134](../authoritative-plan.md#review-finding-ap-planning-revision)).

T076 deletes, in one transaction, the principal's credentials, sessions, keys, grants, memberships, subscriptions (AP-11 personal Subscriptions/Delivery input/history), personal configuration (AP-10 personal saved views, AP-12 personal plugin configuration/Secret values/references, AP-13 preferences) and nulls the profile, tombstones the principal and emits `principal.deleted`. Shared master-data/workflow/audit retains only C-13-safe attribution, not author profiles. Plugins holding copies react to the fact best-effort with visible status; an absent plugin does not block the deletion. Define the transaction, immediate invalidation and redacted status in C-06/C-13. There is no registered-owner acknowledgement protocol, no `cleanup-failed`/`retrying` states, no future operations runner and no blanket log/Payload purge.

T077 executes F-17/F-22/F-24/F-31/F-39 with real PostgreSQL and HTTP: proof replay/expiry/current rights, shared-location login throttling, denied grant/key expansion, immediate output/start stop, rollback of a failed deletion, idempotent repeat, absent plugin, personal versus independently shared configuration, safe distinguishing deleted-user suffixes and redacted progress. Run focused identity/authz/participant race tests, `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager`, focused vet/build and actual local-rescue command tests. Hand over management OpenAPI/action/error versions, deletion paths and executed evidence to AP-15/AP-16/AP-17.

**Acceptance:** Access is blocked immediately on deletion/revocation, the deletion is atomic, and plugin-held copies report a truthful best-effort status.

**Non-goals:** OIDC/SCIM, nested User groups, profile resurrection, anonymous-retention claims, or operator-owned backup cleanup.
