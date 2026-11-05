# AP-02 Remaining Domain Contracts

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [current scope and approvals](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [catalog](../requirements-catalog.md), [coverage mapping](requirement-mapping.md); use these for T009's all-active-REQ review, not delivery approval.
- **Architecture:** [domain ownership](../architecture.md#domains-and-data-ownership), [logical model](../architecture.md#logical-relationship-model), [i18n responsibilities](../architecture.md#language-and-internationalization).
- **Owner contracts:** [C-01](../contracts/c-01-transmission-acceptance.md), [C-02](../contracts/c-02-protocol-payload.md), [C-03](../contracts/c-03-message-grouping.md), [C-04](../contracts/c-04-master-data.md), [C-05](../contracts/c-05-matching-correction.md), [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-11](../contracts/c-11-processing-presentation.md), [C-12](../contracts/c-12-persistence-operations.md), [C-13](../contracts/c-13-account-lifecycle.md); [shared conventions](../contracts/shared-conventions.md). Select by T004..T009 and the coverage table below.
- **Cases and handover:** [core F-cases](../contracts/reference-cases.md), [F-27..F-41](../contracts/ap-02-boundary-cases.md); input from [AP-01](ap-01-protocols-grouping.md), evidence handover to [AP-03](ap-03-technical-validation.md), freeze to [AP-04](ap-04-scope-freeze.md).

**Owner:** Respective DOM-03..DOM-08/plugin-host/operations role; integration coordinates cross-contract changes. **Contracts:** C-04..C-13 and C-01 boundaries. **REQ:** All active REQs considered through contract/mapping review; delivery phase remains separate. **Write scope:** assigned C-ID files in `docs/contracts/`, shared conventions/reference cases where affected, index `docs/domain-contracts.md` for navigation only; logical model/ownership in `docs/architecture.md`; status changes by integration only.

**Prerequisites:** AP-01's Transmission contract outputs for Transmission-dependent detail completion; independent domain boundaries can be reviewed separately.

- [x] T004 [AP-02] Extend C-04/C-05 with defaults, temporal rule specificity, bulk failures, suppression and concurrent correction. Preserve exact originals; store current target Unit/matched filter and rebuildable Message contributions/Contribution supports. No mandatory immutable rule-version entity or version/validity-segment contribution key. Owner contracts elaborate concrete fields/revisions; cover F-11..F-17 including F-11a..F-11i.
- [x] T005 [AP-02] Complete C-06/C-07 action/field/grant schemas, local credential/recovery/key boundaries, first-observed Unit appearances/View time distinct from stored internal Message time, age anchor, query/cursor/SSE resume/revocation, and stable API error/status codes; preserve central AuthZ independent of language.
- [x] T006 [AP-02] Complete C-08/C-09 Domain event envelopes/types, claim/fencing, retry/uncertainty/receipt retention, and Delivery-start/revocation boundary; preserve explicit Domain event subscriptions.
- [x] T007 [AP-02] Specify C-10/C-11 SDK/manifest/host capabilities and bounded rule/presentation model; cover email transport, secrets, SSRF/DNS/redirect/cancellation, and English text key/parameter/plural/fallback boundaries for future localization without requiring translations.
- [x] T008 [AP-02] Extend C-12/C-13 with storage failures/retention, acceptance/restart/profile transition, updates, and field-specific account pseudonymization/audit. D-31 excludes application Backup/Restore logic. No blanket retention of personal data as a security default.
- [x] T009 [AP-02] Review producers/consumers, owner, version, permissions, time/revision, errors, idempotency and transactions; align model/reference cases with D-29/D-30. Document exact comparison separate from Grouping, preserving originals and match kind for a later approved extension, without current heuristic configuration/algorithms/correction machinery. Complete C-07/C-11 exact representative/fallback projection. Concrete fields belong to owner contracts/packages, not the illustrative diagram.

**Acceptance:** P-12/P-14 and relevant technical contract questions are resolved per boundary or remain explicitly blocking. No unchecked approval of old projections after revocation. English/i18n readiness has a clear owner and interface contract, not a requirement for a second translation. **Non-goals:** Claiming unimplemented behavior, accelerating OIDC/SCIM/PWA, substituting plans for executed spikes.

## Documentary Completion and Coverage

Historical closure/evidence: [AP-02 execution record](../execution/ap-02.md), 2026-10-25. Existing checkmarks are historical specification/review marks; future execution state is recorded outside the AP. Scope/implementation approval belongs to the [plan](../authoritative-plan.md#workflow-and-approval-gates).

| Task | Owner evidence | Concrete planned cases |
| --- | --- | --- |
| T004 | [C-04](../contracts/c-04-master-data.md), [C-05](../contracts/c-05-matching-correction.md): defaults/bulk, temporal specificity, current resolution/support model, suppression and checked correction publication | F-11..F-17, F-27..F-29 |
| T005 | [C-06](../contracts/c-06-identity-authorization.md), [C-07](../contracts/c-07-query-live.md): actions/fields/grants, recovery/keys, scoped first-time/age, cursor/SSE/current revocation and errors | F-11b..F-11i, F-14/F-17/F-24/F-26, F-30..F-32 |
| T006 | [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md): event types/envelope/order, fencing, explicit subscriptions, materialized input, current start boundary and bounded uncertainty/retries | F-18/F-23/F-25, F-33..F-35 |
| T007 | [C-10](../contracts/c-10-plugin-host.md), [C-11](../contracts/c-11-processing-presentation.md): trusted prebuilt lifecycle/Secrets/I/O, bounded namespaced Workflows, publication/preview/template/sensitivity and English mappings | F-20/F-21/F-26, F-36..F-38 |
| T008 | [C-12](../contracts/c-12-persistence-operations.md), [C-13](../contracts/c-13-account-lifecycle.md): storage/retention/restart/transition/update, external manual Backup/Restore, immediate account block and field-specific pseudonymization | F-19/F-22/F-23, F-39..F-41 |
| T009 | [Shared conventions](../contracts/shared-conventions.md), [glossary](../glossary.md), owner contracts and [model](../architecture.md#logical-relationship-model): producers/consumers, revisions, access/error/transaction/idempotency boundaries; exact comparator separated from Grouping and source-bound readable projection | F-01..F-26 including lettered cases; [F-27..F-41](../contracts/ap-02-boundary-cases.md) |

**Handover, not unfinished AP-02 decisions:** AP-03 owns runtime/driver/ABI/precision/Unicode/concurrency/crash/authority evidence; AP-04 owns FLEX verification and scope/contract freeze. Owner implementation packages elaborate user-deferred physical fields/types/indexes and generate approved REST/SDK artifacts. AP-12/AP-13 select established template/builder/i18n tooling under C-11, not new permission or product semantics. No spike, migration, product code, dependency installation, staging or commit was performed.

Implementation dependencies are maintained in the [current assignments](../work-packages.md#revised-execution-path), not as a second schedule here.
