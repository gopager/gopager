# AP-14 Identity Lifecycle Completion

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** server side of the following (screens: AP-15):
  - [REQ-051..052/054..064](../requirements-catalog.md#f-access-rights-and-guests), [REQ-069..071/073..075/077..079](../requirements-catalog.md#g-accounts-and-login) accounts, grants, field groups, guests, sessions, recovery and access keys. For REQ-055..060/070 this AP manages grants and sessions; their enforcement in query and live results is AP-10.
  - REQ-085: owner suspension or deletion stops Deliveries (pausing a Subscription: AP-11).
  - REQ-108: login throttling (query: AP-10, sending: AP-11, provider limits: AP-12).
  - [REQ-109..110](../requirements-catalog.md#k-installed-web-app-and-security-behavior) security audit trail.
  - REQ-072/076 (recovery requests and automatic dispatch) are dropped by [D-135](../decisions/10-foundation-redesign.md#d-135) item 15.
- **Inputs:**
  - [identity-core handover](../execution/source-ap-07.md#handover): completed identity-core operations.
  - [AP-08 handover](../execution/ap-08.md#handover): master-data, correction and Agency actions and scopes to offer in grants and keys.
  - [AP-09](ap-09-gateway-network.md): Gateway management actions and scopes to offer in grants and keys. Verify `development/execution/ap-09.md#handover` exists at start.
  - [AP-10](ap-10-query-live.md): saved views and current fields. Verify `development/execution/ap-10.md#handover` exists at start.
  - [AP-11](ap-11-events-delivery.md): personal Subscription and Delivery. Verify `development/execution/ap-11.md#handover` exists at start.
  - [AP-12](ap-12-plugin-host.md): configuration and Secrets. Verify `development/execution/ap-12.md#handover` exists at start.
  - [AP-13](ap-13-processing-presentation-integration.md): preferences and dismissals. Verify `development/execution/ap-13.md#handover` exists at start.
  - No prior cleanup subsystem or frontend/operations input is required.
- **Owned contract slices:** [C-06](../contracts/c-06-identity-authorization.md) and [C-13](../contracts/c-13-account-lifecycle.md) deletion planning; supporting [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md) and [C-10](../contracts/c-10-plugin-host.md) slices; F-17/F-22/F-24/F-31/F-39. Identity is the primary code area; integration and the concrete earlier data owners participate inside this AP to implement the deletion steps for their data.
- **Reference (read when needed):** [domain ownership](../architecture.md#domains-and-data-ownership), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies), [D-134](../decisions/09-process-and-documentation.md#d-134), [D-135](../decisions/10-foundation-redesign.md#d-135), [Sessions](../../docs/http-api.md#sessions-browser-login).
- **Write scope:**
  - `internal/identity/`, `internal/authz/` — account, recovery, keys, sessions
  - `internal/delivery/`, `internal/pluginhost/` (if extracted), `internal/readmodel/`, `internal/processing/`, `internal/events/` — exact account-owned copies, work and acknowledgements
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/identitystore/` and the stores of the account-owned data listed above, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout))
  - `internal/httpapi/` handlers and versioned management OpenAPI
  - `cmd/gopager/` — local-rescue command and root registration only
  - Preserve independently owned received Payload and shared configurations.
- **Compatibility context:** [AP-15](ap-15-frontend.md), [AP-16](ap-16-operations.md) and [AP-17](ap-17-integration-evidence.md) consume the management OpenAPI/action/error versions and deletion paths. Do not implement their workflows.

## Open User Decisions

None.

## Planning Before Coding

Record the decisions in C-06/C-13 and supporting C-08/C-09/C-10 before coding; cases belong to their F-ID owners.

**T075.** Inventory the [completed identity-core operations](../execution/source-ap-07.md#handover) and define the remaining account/profile/group/grant/key/session/recovery/rescue operations. Recovery keeps REQ-071 (change), REQ-073 (an administrator issues a 30 min one-time code), REQ-074 (consume with username) and REQ-075 (CLI rescue); recovery request records, approval modes, automatic dispatch and per-account throttles (REQ-072/076) are not built. Access keys keep action and scope restrictions only (REQ-077/078; no field or age subsets): current actions/fields, validation/errors, revisions, lifecycle and safe management/audit/status HTTP. Consume completed action/scope extensions from AP-08/AP-09/AP-10/AP-12 without redefining them. Make Session absolute (30 d) and inactivity (7 d) lifetimes Operator-configurable to shorter values only ([C-06 Session rules](../../docs/http-api.md#sessions-browser-login)); reject longer or invalid values at startup and test both expiry boundaries ([D-134](../decisions/09-process-and-documentation.md#d-134)).

**T076.** Define the single transaction that deletes the principal's credentials, sessions, keys, grants, memberships, subscriptions (AP-11 personal Subscriptions/Delivery input/history), personal configuration (AP-10 personal saved views, AP-12 personal plugin configuration/Secret values/references, AP-13 preferences) and nulls the profile, tombstones the principal and emits `principal.deleted`. Shared master-data/workflow/audit retains only C-13-safe attribution, not author profiles. Plugins holding copies react to the fact best-effort with visible status; an absent plugin does not block the deletion. Define the transaction, immediate invalidation and redacted status in C-06/C-13. There is no registered-owner acknowledgement protocol, no `cleanup-failed`/`retrying` states, no future operations runner and no blanket log/Payload purge.

**T077.** Settle the evidence plan for the cases listed under T077.

## Tasks

- [ ] T075 [AP-14] Complete local account/profile/grant/key and Recovery (administrator-issued code, consume, CLI rescue) operations not delivered by the verified identity-core handover.
  - **Done when:** the remaining operations, safe management/audit/status HTTP and the Operator-configurable Session lifetimes (shorter values only, invalid values rejected at startup, both expiry boundaries tested) work; denied grant/key expansion holds.
- [ ] T076 [AP-14] Implement account deletion as one transaction with immediate invalidation, audit, pseudonymization and the `principal.deleted` fact.
  - **Done when:** access is blocked immediately on deletion/revocation, the deletion is atomic, and plugin-held copies report a truthful best-effort status.
- [ ] T077 [AP-14] Verify revocation, Recovery replay/rights, deletion atomicity/rollback, plugin best-effort reaction status, and redacted outcomes.
  - **Done when:** F-17/F-22/F-24/F-31/F-39 pass with real PostgreSQL and HTTP: proof replay/expiry/current rights, shared-location login throttling, denied grant/key expansion, immediate output/start stop, rollback of a failed deletion, idempotent repeat, absent plugin, personal versus independently shared configuration, safe distinguishing deleted-user suffixes and redacted progress; actual local-rescue command tests pass.

## Documentation to Update

- Development owners: [C-06](../contracts/c-06-identity-authorization.md), [C-13](../contracts/c-13-account-lifecycle.md) (Immediate Boundary and Restart, Field Handling), supporting C-08/C-09/C-10 slices, the F-cases above, and [glossary](../glossary.md).
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md) (Authentication, Sessions, Bearer access keys, Authorization; new account-management section).
- [docs/operations.md](../../docs/operations.md) (Configuration Reference, Roles, First Start and Bootstrap) for Session lifetime configuration and the local-rescue command; [docs/architecture.md](../../docs/architecture.md) (Identity and cross-cutting records, Authorization) for the deletion flow.
- Go doc comments.

## Verification

Run focused identity/authz/participant race tests, `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager`, focused vet/build and actual local-rescue command tests.

## Handover

To AP-15/AP-16/AP-17: management OpenAPI/action/error versions, deletion paths and executed evidence.

## Non-goals

OIDC/SCIM, nested User groups, profile resurrection, anonymous-retention claims, or operator-owned backup cleanup.
