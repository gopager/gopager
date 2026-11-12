# AP-14 Identity Lifecycle Completion

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** server side of the following (screens: AP-15):
  - [REQ-051..052/054..064](../requirements-catalog.md#f-access-rights-and-guests), [REQ-069..070](../requirements-catalog.md#g-accounts-and-login) accounts, grants, field groups, guests and sessions; [REQ-071/073..075/077..079](../requirements-catalog.md#h-recovery-and-access-keys) recovery and access keys. For REQ-055..060/070 this AP manages grants and sessions; their enforcement in query and live results is AP-10.
  - REQ-085: owner suspension or deletion stops Deliveries (pausing a Subscription: AP-11).
  - REQ-108: login throttling (query: AP-10, sending: AP-11, provider limits: AP-12).
  - [REQ-109..110](../requirements-catalog.md#k-installed-web-app-and-security-behavior) security audit trail.
  - REQ-062 account deletion removes all Core-held personal data; copies outside the Core are not purged ([D-142](../decisions/09-process-and-documentation.md#d-142)).
  - REQ-072/076 (recovery requests and automatic dispatch) are dropped by [D-135](../decisions/10-foundation-redesign.md#d-135) item 15.
- **Inputs:**
  - [identity-core handover](../execution/source-ap-07.md#handover): completed identity-core operations.
  - [AP-08 handover](../execution/ap-08.md#handover): master-data, correction and Agency actions and scopes to offer in grants and keys.
  - [AP-09 handover](../execution/ap-09.md#handover): Gateway management actions and scopes to offer in grants and keys.
  - [AP-10 handover](ap-10-query-live.md#handover): list of personal-data tables with account key and the deletion function (personal saved views).
  - [AP-11 handover](ap-11-events-delivery.md#handover): list of personal-data tables with account key and the deletion function (Subscriptions, Delivery inputs, history and personal targets), and the current-authority fence for Delivery starts.
  - [AP-12 handover](ap-12-plugin-host.md#handover): list of personal-data tables with account key and the deletion function (personal plugin configuration, Secret values and references).
  - [AP-13 handover](ap-13-processing-presentation-integration.md#handover): list of personal-data tables with account key and the deletion function (preferences).
  - These are earlier open APs in execution order; a missing handover entry is an input gap reported to the user, not something this AP rebuilds.
- **Owned contract slices:** [C-06](../contracts/c-06-identity-authorization.md) and [C-13](../contracts/c-13-account-lifecycle.md) deletion planning; F-17 (session/grant/key revocation part), F-22, F-24, F-31, F-39. Identity is the primary code area; this AP calls the deletion functions named in the AP-10..AP-13 handovers inside its single deletion transaction.
- **Reference (read when needed):** [domain ownership](../architecture.md#domains-and-data-ownership), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies), [D-134](../decisions/09-process-and-documentation.md#d-134), [D-135](../decisions/10-foundation-redesign.md#d-135), [D-142](../decisions/09-process-and-documentation.md#d-142), [Sessions](../../docs/http-api.md#sessions-browser-login).
- **Write scope:**
  - `internal/identity/`, `internal/authz/` — account, recovery, keys, sessions, action catalog and restrictions
  - `internal/readmodel/`, `internal/delivery/`, `internal/processing/` (plugin configuration and Secrets), `internal/events/` (`principal.deleted`, `principal.access-changed`) — the deletion functions for account-owned data and their calls
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/{identitystore,processingstore,eventstore}/` and the stores named in the AP-10..AP-13 handovers, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout))
  - `internal/httpapi/` handlers and versioned management OpenAPI
  - `cmd/gopager/` — flag registration for Session lifetimes and access-key lifetime, the local-rescue command and root registration
  - Preserve independently owned received Payload and shared configurations.
- **Compatibility context:** [AP-15](ap-15-frontend.md), [AP-16](ap-16-operations.md) and [AP-17](ap-17-integration-evidence.md) consume the management OpenAPI/action/error versions and deletion paths. Do not implement their workflows.

## Open User Decisions

None.

## Planning Before Coding

Record the decisions in C-06/C-13 before coding; cases belong to their F-ID owners.

**T075.** Inventory the [completed identity-core operations](../execution/source-ap-07.md#handover) and define the remaining account/profile/group/grant/key/session/recovery/rescue operations.
- Recovery keeps REQ-071 (change), REQ-073 (a manager holding `recovery.issue`, scoped to the target accounts, issues a 30 min one-time code), REQ-074 (consume with username) and REQ-075 (CLI rescue). `recovery.consume` is not a grantable action: consumption is unauthenticated and gated by proof, username and `password.local`. Remove the dead action `recovery.request` from `internal/authz/grants.go` and `docs/operations.md` (Accounts row). Recovery request records, approval modes, automatic dispatch and the per-account issuance throttle (REQ-072/076) are not built; throttle proof consumption at 10/min per normalized username and 60/min per instance through the login-attempt bucket mechanism with a generic error and `RATE_LIMITED` plus `Retry-After`.
- Access keys keep action and scope restrictions only ([D-135](../decisions/10-foundation-redesign.md#d-135) item 15, REQ-077/078). Remove `Fields` and `MaxAgeSeconds` from `authz.Restrictions` and `ValidateRestrictions` (`internal/authz/grants.go`), from `decision.go`, `reach.go` and `fields.go` (today `AuthorizeFields` denies fields missing from `Key.Fields`), and from the `restrictions` JSON; a key's field projection is its owner's field permissions. Update tests and `docs/http-api.md` (access-key restrictions and the Authorization list) to "actions, scopes". Validation, errors, revisions, lifecycle and safe management/audit/status HTTP cover the action catalog and scope kinds, including the extensions from AP-08/AP-09/AP-10/AP-12, which are consumed without redefining them.
- Session absolute and inactivity lifetimes become flags (at most 30 d and 7 d, positive, validated at startup; hard-coded today in `internal/identity/login.go` and `authority.go`), as does the access-key lifetime (default 90 d, 0 = unlimited per [C-06](../contracts/c-06-identity-authorization.md)); document them in `docs/operations.md` Configuration Reference ([Sessions](../../docs/http-api.md#sessions-browser-login), [D-134](../decisions/09-process-and-documentation.md#d-134)). The session list shows id, created, last used, expires and a current-session marker; no User-Agent or IP is stored.
- Suspension and reactivation stop and resume Delivery starts through AP-11's current-authority fence; no Subscription state changes.

**T076.** Define the single transaction ([D-142](../decisions/09-process-and-documentation.md#d-142)) that deletes all Core-held personal data of the principal: credentials, sessions, keys, grants, memberships, Subscriptions, Delivery targets and inputs, personal plugin configuration, Secrets (in-app store and references), saved views and preferences, through the deletion functions of the AP-10..AP-13 handovers (for plugin configuration `pluginconfigstore.Store.DeleteAccountData(ctx, tx, accountID)` deletes the account's `plugin_personal_configurations` and owner-bound `configuration_secrets` rows in the caller's transaction, [D-160](../decisions/05-events-and-delivery.md#d-160); administrative Secrets have no owner and stay). It nulls the profile, tombstones the principal and emits `principal.deleted` (`internal/events/identity.go`). Shared master-data/workflow/audit retains only C-13-safe attribution, not author profiles. Copies outside the Core (plugin-side state, providers, external Secret stores) are not purged, and the result says so; there is no plugin cleanup hook (REQ-139 later). The state value `deleting` stays in the database check for compatibility but is never set; removing it by migration is not worth the churn, and this choice is recorded in C-13. Define the transaction, immediate invalidation and redacted result in C-06/C-13 and rewrite F-22, F-31 and F-39 to this model. There are no deletion request records, no 202, no cleanup states or retry, no fencing, no owner acknowledgement and no blanket log/Payload purge.

**T077.** Evidence cases are F-17 (session/grant/key revocation part), F-22, F-24, F-31 and F-39 as rewritten, plus Delivery start refused for a suspended owner (through the AP-11 port).

## Tasks

- [ ] T075 [AP-14] Complete local account/profile/grant/key and Recovery (manager-issued code, consume, CLI rescue) operations not delivered by the verified identity-core handover.
  - **Done when:** the remaining operations and safe management/audit/status HTTP work; `recovery.request` is gone and proof consumption is throttled as planned; access-key restrictions are actions and scopes only; Session and access-key lifetime flags are validated at startup (invalid values rejected, both expiry boundaries tested) and documented; the session list shows id, created, last used, expires and the current-session marker; suspension and reactivation stop and resume Delivery starts; denied grant/key expansion holds.
- [ ] T076 [AP-14] Implement account deletion as one transaction with immediate invalidation, audit, pseudonymization and the `principal.deleted` fact.
  - **Done when:** access is blocked immediately on deletion/revocation, the deletion is atomic, and the result reports Core-held data removed and states that external copies are not purged.
- [ ] T077 [AP-14] Verify revocation, Recovery replay/rights, deletion atomicity/rollback and redacted outcomes.
  - **Done when:** F-17 (session/grant/key revocation and account-state change facts `principal.access-changed`, immediate invalidation and revision; output and start stop is tested in AP-10 for query/SSE and in AP-11 for Delivery), F-22, F-24, F-31 and F-39 pass with real PostgreSQL and HTTP: proof replay/expiry/current rights, proof-consumption throttling, denied grant/key expansion, rollback of a failed deletion, idempotent repeat, personal versus independently shared configuration, safe distinguishing deleted-user suffixes, the external-copies statement, Delivery start refused for a suspended owner; actual local-rescue command tests pass.

## Documentation to Update

- Development owners: [C-06](../contracts/c-06-identity-authorization.md), [C-13](../contracts/c-13-account-lifecycle.md) (Operations, Immediate Boundary, Field Handling), the F-cases above in [reference cases](../contracts/reference-cases.md) and [boundary cases](../contracts/ap-02-boundary-cases.md), and [glossary](../glossary.md).
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md) (access-key restrictions: actions and scopes; Authentication, Sessions, Bearer access keys, Authorization; new account-management section).
- [docs/operations.md](../../docs/operations.md) (Configuration Reference, Roles, First Start and Bootstrap) for Session and access-key lifetime flags, the action catalog (`recovery.request` removed) and the local-rescue command; [docs/architecture.md](../../docs/architecture.md) (Identity and cross-cutting records, Authorization) for the deletion flow.
- Go doc comments.

## Verification

Run focused identity/authz/participant race tests, `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager`, focused vet/build and actual local-rescue command tests.

## Handover

To AP-15/AP-16/AP-17: management OpenAPI/action/error versions, the deletion function and result, and executed evidence.

## Non-goals

OIDC/SCIM, nested User groups, profile resurrection, anonymous-retention claims, operator-owned backup cleanup, purging copies outside the Core, a plugin cleanup hook (REQ-139) and notice dismissals.
