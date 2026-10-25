# GoPager Work Packages

Date: 2026-10-24. Documentary implementation plan with tasks and REQ mapping, not an instruction to implement product code. Domain requirements: [catalog](requirements-catalog.md). delivery status exclusively: [plan](authoritative-plan.md#delivery-scope). Ownership/technologies: [architecture](architecture.md). Contract references currently refer to [version 0.1-draft](domain-contracts.md).

## Execution and Ownership Rules

- **Clarification complete:** AP-01 closed on 2026-10-25 with user confirmation; its results are 0.1-draft contract text, not a freeze or implementation approval.
- **Clarification planned:** AP-02 defines the remaining concrete contract work. Unresolved domain decisions must be submitted for confirmation, not guessed by implementation agents.
- **Validation planned:** AP-03 contains a documentary validation plan. Executable spikes require their own authorization; none have been run.
- **Approval pending:** AP-04 is a gate, not an automatic status change after creating documentation.
- **Blocked:** all product implementation packages; freeze prerequisites/contract versions first. AP-14 records deliberately later goals without bringing them forward.
- Roles have no named assignee yet. Integration exclusively owns shared schema/contract/build/composition boundaries. No named or parallel write approval exists yet.
- Target paths below are proposed future write scopes, not existing packages or an already-created Go structure. AP-04 confirms them before execution; no module path/toolchain is predetermined.
- Integrate documentation changes in shared owner files sequentially. Independent domain implementations may work in parallel after their gates, never while shared contract questions remain open.
- Do not infer passing tests/product completion from future test files. Acceptance requires executed evidence; currently there is only Markdown.
- **Names (D-21):** the [glossary](glossary.md) is the single authoritative source for names. Every package that needs a new name adds it to the glossary; every package that detects a collision or needs a rename records it in the glossary rename log (old name, new name, meaning, reason, package) and must then re-check and update all documents, code, schemas/migrations, API/event definitions, tests, fixtures, and UI texts referring to the affected names, or record remaining occurrences as an owned follow-up. A package is not accepted while it introduces names missing from the glossary.
- All project artifacts use English. Under D-20/REQ-097, prepare i18n during initial implementation: stable text keys, English text catalog/fallback, parameters/plurals, locale-aware formatting, and language-independent API codes. Additional translations/language packs are not required. AP-09 owns API boundaries, AP-12 built-in processing Localized text, AP-13 frontend catalog/formatting, and integration shared compatibility.

## Dependency Overview

| Package | Owner role | Prerequisites | Status |
| --- | --- | --- | --- |
| AP-01 Protocols and Grouping contract | DOM-01/DOM-02, integration | Documented requirements/decisions | Clarification complete (2026-10-25); not frozen |
| AP-02 Remaining domain contracts | Integration and respective contract owner | AP-01 for Transmission-dependent detail completion; other drafts may proceed | Clarification planned |
| AP-03 Technical validation planning | Persistence/operations/plugin host | Concrete test semantics from AP-01/AP-02; independent runtime questions separately plannable | Validation planned |
| AP-04 Scope and contract freeze | Product/integration ownership | Relevant AP-01..AP-03 results; targeted legacy assessment if needed | Approval pending |
| AP-05 Runtime and persistence foundation | Persistence/operations/integration | AP-04 for C-01/C-08/C-12 and TECH-01/02/05/06/07 | Blocked |
| AP-06 Transmission, Protocols, Messages | DOM-01/DOM-02 | AP-05, AP-07, frozen C-01..C-03/C-08 | Blocked |
| AP-07 Local identity and central AuthZ | DOM-05 | AP-05, frozen C-06/C-13 and relevant C-12 boundaries | Blocked |
| AP-08 Master data and correction | DOM-03/DOM-04 | AP-05..AP-07, frozen C-02/C-04..C-06/C-08 | Blocked |
| AP-09 Search, REST, SSE | DOM-06/integration | AP-06..AP-08, frozen C-06/C-07/C-08 | Blocked |
| AP-10 Domain events and Delivery core | DOM-07 | AP-05..AP-09, frozen C-08/C-09; agreed C-10 | Blocked |
| AP-11 SDK, plugin host, four targets | Plugin host/respective plugin owner | AP-10, frozen and validated C-09/C-10 | Blocked |
| AP-12 Processing and presentation | DOM-08 | AP-07/AP-09, frozen C-11 and C-08/C-09 reaction boundary | Blocked |
| AP-13 Vue frontend | Frontend | Versioned APIs from AP-07..AP-12; C-07/C-11 draft design possible, no implementation start | Blocked |
| AP-14 Later extension boundaries | Respective domain/product | Separate scope decision under plan and later contract approvals | Not approved for initial implementation |
| AP-15 Operations and profile transition | Operations/integration | AP-05..AP-12, frozen C-12/C-13 and restore/attempt boundary | Blocked |
| AP-16 Integration and delivery evidence | Integration with all owners | Approved delivery scope and all related packages/evidence | Blocked |

Numbers are stable identifiers, not a strictly linear execution order. In particular, AP-07 follows the foundation, not AP-06; AP-14 does not precede AP-15 merely because of numbering. Partial approvals require explicitly documenting the smaller scope.

## AP-01 Protocols and Grouping Contract

**Owner:** DOM-01/DOM-02, shared integration owner. **Contracts:** C-01/C-02/C-03, Transmission side of C-05. **REQ:** REQ-020, REQ-031..032, REQ-037..038, REQ-041..046, REQ-113..114, REQ-121..122. **Write scope:** Protocol/Transmission sections and F-01..F-10 in `docs/domain-contracts.md`; derived plan status changes by integration only.

- [x] T001 [AP-01] Define the completed-Transmission input boundary and server-relevant Payload/Transmission targets for POCSAG/FLEX/ZVEI/EAS in `docs/domain-contracts.md` C-02 with examples, including the required EAS header and actual Payload. Decoder, frame/checksum, quality-metric evaluation, and fragment-reassembly internals belong to Gateway adapters, not the server contract.
- [x] T002 [AP-01] Elaborate server-owned Payload identity and C-03 Grouping, the fixed reported Observation time basis (mandatory timestamp with explicit timezone offset), exact window boundary, out-of-order/candidate selection, remaining configurable policy, and configuration changes in `docs/domain-contracts.md`; preserve the confirmed sliding window. Any unresolved policy values remain explicit blockers.
- [x] T003 [AP-01] Specify C-01 submission/idempotency/Network/acceptance state and Gateway-versus-server responsibilities in `docs/domain-contracts.md`; complete F-01..F-10 with positive/negative expected results. Gateways submit completed Transmissions and do not interpret domain Payload, resolve Unit assignments, or perform Grouping of independent Transmissions.

**Acceptance:** Every relevant P-08/P-09 subquestion has a decision or explicit remaining blocker; no unapproved default. The completed-Transmission boundary, mandatory offset-bearing Observation timestamp and its fixed use for Grouping, server ownership of Payload comparison/Address-to-Unit assignment/Grouping, multiple Transmission targets, optional contextualized quality metrics, sliding chains, cross-Gateway/Network Grouping, empty Payload, retry versus Transmission, and the EAS header-plus-Payload requirement are unambiguous. Technical receiving and Protocol framing/reassembly details are explicitly outside the server contract. A compact reference/delta transport remains optional later work, not an AP-01 prerequisite. **Result (2026-10-25):** Acceptance met in C-01..C-03, F-01..F-10a (including F-01a..F-01c and F-08a), D-17, D-21, P-08/P-09, and the [glossary](glossary.md). Handed over with owner: wire format, size limits, and identifier types; query correlation (P-09); EAS-area-to-Unit boundary; Domain event schemas and alias resolution; Gateway contact timeouts (all AP-02); acceptance/derivation boundary and the duration data type (AP-03); FLEX fact verification and the freeze (AP-04); similarity heuristic and regrouping (later phase). **Non-goals:** Decoder/audio implementation, Gateway adapter implementation, code adoption, silent scope freeze.

## AP-02 Remaining Domain Contracts

**Owner:** Respective DOM-03..DOM-08/plugin-host/operations role; integration coordinates the shared file. **Contracts:** C-04..C-13 and C-01 boundaries. **REQ:** All active REQs considered through contract/mapping review; delivery phase remains separate. **Write scope:** `docs/domain-contracts.md`, logical model/ownership details in `docs/architecture.md`; status changes by integration only.

- [ ] T004 [AP-02] Extend C-04/C-05 with default materialization, rule intervals/specificity, bulk failures, suppression, rematch revision, and concurrent ingest; define safe revocation/intermediate states with F-11..F-17.
- [ ] T005 [AP-02] Complete C-06/C-07 action/field/grant schemas, local credential/recovery/key boundaries, age anchor, query/cursor/SSE resume/revocation, and stable API error/status codes with structured parameters; preserve central AuthZ semantics independent of language.
- [ ] T006 [AP-02] Complete C-08/C-09 Domain event envelopes/types, claim/fencing, retry/uncertainty/receipt retention, and Delivery-start/revocation boundary; preserve explicit Domain event subscriptions.
- [ ] T007 [AP-02] Specify C-10/C-11 SDK/manifest/host capabilities and bounded rule/presentation model; cover email transport, secrets, SSRF/DNS/redirect/cancellation, and English text key/parameter/plural/fallback boundaries for future localization without requiring translations.
- [ ] T008 [AP-02] Extend C-12/C-13 with storage failures/retention, acceptance/restore/profile transition, updates, and field-specific account anonymization/audit. No blanket retention of personal data as a security default.
- [ ] T009 [AP-02] Review producers/consumers, owner, version, permissions, time/revision, errors, idempotency, and transactions for every interface; make the logical model and F-11..F-26 in `docs/domain-contracts.md` consistent and concrete.

**Acceptance:** P-12/P-14 and relevant technical contract questions are resolved per boundary or remain explicitly blocking. No unchecked approval of old projections after revocation. English/i18n readiness has a clear owner and interface contract, not a requirement for a second translation. **Non-goals:** Claiming unimplemented behavior, accelerating OIDC/SCIM/PWA, substituting plans for executed spikes.

## AP-03 Technical Validation Planning

**Owner:** Persistence/operations/plugin host; DOM-06 with DOM-05 for search semantics. **Contracts:** C-01/C-03/C-06..C-12. **REQ:** REQ-011, REQ-017, REQ-089..091, REQ-097, REQ-105..108, REQ-113..120, REQ-126..127. **Current write scope:** Validation decisions in `docs/architecture.md`, provenance/evidence in `docs/requirements-evidence.md`, boundaries in `docs/domain-contracts.md`. No spike code yet.

- [ ] T010 [AP-03] Plan SQLite driver/CGO/build comparison and PostgreSQL parity probe: constraints, idempotency, concurrent Grouping, transactions/outbox, leases, migrations with shared inputs. No full implementations.
- [ ] T011 [AP-03] Define Unicode/accent/short-term fixtures for both stores; check normalization/case folding/index candidates against C-07 rather than substituting search syntax. Verify that UI language/formatting preferences do not alter search semantics or wire values.
- [ ] T012 [AP-03] Plan small adversarial tests for Wasm ABI/toolchain, startup/memory, infinite loops, blocking host calls, I/O/Payload limits, and secret redaction.
- [ ] T013 [AP-03] Document an available constrained reference host and reproducible single-process/shutdown/crash/disk-failure/restore experiment. Plan Full authority loss, superseded workers, and API/SSE revocation together.
- [ ] T014 [AP-03] Obtain separate authorization for executable experiments; then record commands, exact versions, environment, results, and limits in `docs/requirements-evidence.md`. Feed negative results back into architecture/contracts.

**Acceptance:** First an approved narrow experimental plan; later actual evidence without SLA/hardware promises. A plan alone does not pass runtime validation. **Non-goals:** Infrastructure deployment, competing complete products, code work under this documentation task.

## AP-04 Scope and Contract Freeze

**Owner:** Product/integration ownership. **Contracts:** Every C ID being approved. **REQ:** Every requirement in the approved slice; REQ-088 stays removed. **Write scope:** `docs/authoritative-plan.md`, contract approval metadata, this package register; shared OpenAPI/SDK artifacts after their gates only.

- [ ] T015 [AP-04] Record user approval of the proposed initial scope or an explicit smaller slice in `docs/authoritative-plan.md`; do not assume technical approval grants scope approval.
- [ ] T016 [AP-04] Review relevant AP-01..AP-03 results, including verification of the C-02 FLEX type list and capcode details against the FLEX specification; only if reuse is intended, first document targeted legacy evidence with source/ref/symbol/test. No artificial legacy-parity requirement when nothing is reused.
- [ ] T017 [AP-04] Freeze owner, producers/consumers, schema/version, permissions/field projection, time/revision, idempotency/errors/transactions, and concrete fixtures per boundary, together with the [glossary](glossary.md) version the frozen contracts use; frozen contracts and the glossary must not contradict each other. Specify machine-readable OpenAPI/SDK artifacts as separate deliverables only then.
- [ ] T018 [AP-04] Add approved contract versions, named owners, final write scopes, and then-executable focused validation commands to `docs/work-packages.md`; approve parallel execution only explicitly for disjoint packages. Include REQ-097/i18n checks without requiring translations.

**Acceptance:** Documented evidence-based approval per slice; no unresolved shared semantics in an approved package. **Non-goals:** Writing code or marking every boundary complete merely because documents exist.

## AP-05 Runtime and Persistence Foundation

**Owner:** Persistence/operations/integration. **Contract gate:** C-01/C-08/C-12. **REQ:** REQ-107, REQ-113..120, REQ-127. **Prerequisites:** AP-04 for this slice. **Proposed exclusive write scopes:** `go.mod`, `go.sum`, `cmd/gopager/`, `internal/platform/`, future central build/migration registration. Domain schema objects remain with domain owners; integration owns registration.

- [ ] T019 [AP-05] Implement approved Go/store versions and composition/role startup; Lite combines the same capabilities in one process, Full uses the same artifact contract.
- [ ] T020 [AP-05] Implement dialect adapters, shared transaction/migration boundary, cancellation/shutdown, safe operational failures under C-12; no universal ORM without benefit.
- [ ] T021 [AP-05] Provide controlled clock/store/provider test boundaries; distinguish real database evidence. Verify foundation against approved C-12 fixtures on both backends.

**Acceptance/evidence:** Compilable startup, backend-parallel tests, single-process proof; no claim of Full HA without operational evidence. **Non-goals:** Inventing domain modules or custom external database HA. **Commands:** Only available after module/toolchain setup; later `go test ./...` for Go integration, not claimed executable now.

## AP-06 Transmission, Protocols, and Messages

**Owner:** DOM-01/DOM-02; integration owns commit boundary. **Contract gate:** C-01..C-03/C-08 and C-06 Gateway authorization. **REQ:** REQ-020, REQ-031..032, REQ-037..038, REQ-041..050, REQ-113..114, REQ-121..122. **Prerequisites:** AP-05/AP-07. **Exclusive target scopes:** `internal/ingest/`, `internal/protocols/`, `internal/messages/`, with own tests/schema objects.

- [ ] T022 [AP-06] Implement authorized Gateways/Networks, submission, and Gateway-state operations under frozen C-01, distinguishing inferred/default Network resolution from unauthorized declarations.
- [ ] T023 [AP-06] Implement typed completed-Transmission Payloads and server-owned Payload identity under C-02, plus technical sliding Grouping under C-03; no wire/frame/checksum decoder or display-text heuristic.
- [ ] T024 [AP-06] Test F-01..F-10, concurrent acceptance, crash/idempotency, and Message/Domain event atomicity across backends.

**Acceptance:** Replay is not Transmission; independent repeat remains its own evidence; Protocol/time-correct Messages under concurrency. **Non-goals:** Raw radio decoders or unapproved EAS media processing. Focused Go package tests follow actual structure; freeze exact paths/commands in AP-04.

## AP-07 Local Identity and Central AuthZ

**Owner:** DOM-05. **Contract gate:** C-06/C-13, relevant C-12 secret/rescue boundaries. **REQ:** REQ-051..052, REQ-054..064, REQ-069..079, REQ-105, REQ-107..110. **Prerequisites:** AP-05. **Exclusive target scopes:** `internal/identity/`, `internal/authz/`, own tests/schema objects; CLI/root wiring by integration only.

- [ ] T025 [AP-07] Implement Principal/credentials, local login, profile, sessions, recovery/rescue, and restricting personal/technical keys with explicit target permissions.
- [ ] T026 [AP-07] Implement additive direct/User group grants, guest, field/age limits, safe scope-object deletion, shared query/projection decisions.
- [ ] T027 [AP-07] Integrate account deletion/audit under the frozen field matrix and safe invalidation; test F-17/F-22/F-24, recovery replay, target rights, throttling, and redacted failures.

**Acceptance:** No privilege expansion through keys, guest, recovery, or SQL adapters; revoked access produces no further private output. **Non-goals:** OIDC/SCIM or nested User groups. Account deletion stays blocked while P-14 remains open.

## AP-08 Master Data and Correction

**Owner:** DOM-03/DOM-04; integration coordinates cross-domain deletion. **Contract gate:** C-02/C-04..C-06/C-08. **REQ:** REQ-019..040, REQ-095, REQ-123..124, REQ-132. **Prerequisites:** AP-05..AP-07. **Exclusive target scopes:** `internal/Units/`, `internal/assignments/`, with tests/schema objects.

- [ ] T028 [AP-08] Implement Units/organizations/Unit collections/templates/defaults and authorized bulk/conflict operations.
- [ ] T029 [AP-08] Implement temporal rules, unambiguous matches/failure states, rematch scheduling/revision, suppression under C-05.
- [ ] T030 [AP-08] Test differing Unit/organization deletion effects and default preservation; cover F-11/F-12/F-15/F-16 and concurrent ingest/revocation.

**Acceptance:** No new Transmission/Message from correction, no stale visible Unit references, no secret conflict details, no automatic Payload masking. **Non-goals:** Historical color/icon versioning or unapproved import.

## AP-09 Search, REST, and SSE

**Owner:** DOM-06/integration. **Contract gate:** C-06/C-07/C-08. **REQ:** REQ-001..020, REQ-027, REQ-029, REQ-055..060, REQ-097..100, REQ-108/110. **Prerequisites:** AP-06..AP-08. **Exclusive target scopes:** `internal/readmodel/`, `internal/httpapi/`, versioned REST/OpenAPI artifacts; all transport registration by this integration owner, not concurrently rewritten by other domain owners.

- [ ] T031 [AP-09] Implement central safe query/field projection, search/filter options, detail/time-anchor/cursor operations; verify OpenAPI against real responses. Supply language-independent error/status codes and parameters with English fallback, not client-parsed prose.
- [ ] T032 [AP-09] Integrate authorized SSE for multiple views/tabs, visible interruption, safe resume/resync, ongoing revocation.
- [ ] T033 [AP-09] Test F-11..F-14/F-17 on both backends and HTTP/live boundaries; hidden Transmission targets affect neither counts, ordering, nor times. Verify C-07/F-26 language independence at the API boundary.

**Acceptance:** Same-Transmission correlation, authorization before pagination, literal Unicode substrings, safe reconnect, stable codes/values independent of display locale. **Non-goals:** Separate search service or WebSocket without new need. Backend parity and real SSE checks are separate evidence, not browser acceptance.

## AP-10 Domain events and Delivery Core

**Owner:** DOM-07. **Contract gate:** C-08/C-09 and agreed C-10 host invocation. **REQ:** REQ-049, REQ-081..082, REQ-084..087, REQ-105..106, REQ-129..130. **Prerequisites:** AP-05..AP-09. **Exclusive target scopes:** `internal/events/`, `internal/delivery/`, domain outbox/subscription/attempt schema objects. Physical lease adapter integrated with AP-05 owner.

- [ ] T034 [AP-10] Implement Domain events, atomic outbox, subscriptions/Delivery targets, consumer-specific idempotency.
- [ ] T035 [AP-10] Implement current Delivery projection per attempt, pause/revocation/age, bounded retry classification, explicit redelivery with separate attempts.
- [ ] T036 [AP-10] Test F-02/F-17/F-18/F-23/F-25, including accepted provider request with lost response, lease loss, restore; cover host boundary with controlled providers.

**Acceptance:** No hidden once-per-Message behavior, no old Payload as authorization proof, uncertain effects visible. **Non-goals:** Exactly-once/recall guarantees or arbitrary Domain event fan-out. Real provider evidence only in AP-11 with an explicit access environment.

## AP-11 SDK, Plugin Host, and Four Targets

**Owner:** Plugin host; separate owner per target; integration owns shared SDK. **Contract gate:** Validated C-09/C-10. **REQ:** REQ-084, REQ-089..090, REQ-107, REQ-129. **Prerequisites:** AP-10 and AP-03 evidence. **Exclusive target scopes:** `internal/pluginhost/`, `sdk/`, separate artifact sources `plugins/pushover/`, `plugins/telegram/`, `plugins/email/`, `plugins/webhook/`. Paths do not predetermine programming language/build tools.

- [ ] T037 [AP-11] Implement/document manifest/compatibility/lifecycle, Wazero limits, permitted host calls, secret/I/O boundaries under approved SDK.
- [ ] T038 [AP-11] Implement Pushover as a separate target module with configuration/classified outcomes; test controlled provider fixtures.
- [ ] T039 [AP-11] Implement Telegram as a separate module with separate instances/secrets and bounded I/O; test controlled provider fixtures.
- [ ] T040 [AP-11] Implement email as a separate module through the selected controlled transport; no HTTP-only assumption without an approved mail-provider path.
- [ ] T041 [AP-11] Implement webhook as a separate module under approved target/DNS/redirect/TLS/secret policy; test SSRF/failures.
- [ ] T042 [AP-11] Test F-18/F-20 and disable/update during invocation; verify SDK examples against runtime/ABI. Demonstrate actual external Delivery separately from fixtures.

**Acceptance:** Four separate modules, operator lifecycle, clear compatible API, demonstrated limits. **Non-goals:** Marketplace, arbitrary untrusted-code guarantees, legacy adapter, unrestricted host privileges. T038..T041 may run in parallel only after SDK freeze and disjoint owner assignment.

## AP-12 Processing and Presentation

**Owner:** DOM-08. **Contract gate:** C-11, C-08/C-09 reaction boundary. **REQ:** REQ-091..097, REQ-099..100. **Prerequisites:** AP-07/AP-09, AP-10 for real Domain event reactions. **Exclusive target scopes:** `internal/processing/`, rule/presentation schemas and own tests; UI through AP-13 only.

- [ ] T043 [AP-12] Implement bounded extraction, structured fields, conditions, previews/errors, and saved/active revisions.
- [ ] T044 [AP-12] Implement permission-preserving presentation and safe installation/notice/preference operations under C-11. Expose built-in Localized text through the shared English key/parameter contract; preserve operator-authored data and independent formatting preferences.
- [ ] T045 [AP-12] Test F-21, regex/Payload bounds, sensitivity propagation, concurrent conflicts, returning to an earlier version, and F-26 text-parameter/locale boundaries; prove preview has no real side effects.

**Acceptance:** Minimum value without original mutation, privilege expansion, or unrestricted scripts; built-in product text prepared for localization, not automatic Payload translation. **Non-goals:** Complete n8n/Node-RED clone or unverified `essen.js` parity.

## AP-13 Vue Frontend

**Owner:** Frontend; integration owns API/asset versions. **Contract gate:** C-04..C-11/C-13 for the approved slice. **REQ:** REQ-001..030, REQ-045, REQ-047..050, REQ-060..064, REQ-069..079, REQ-084..087, REQ-089, REQ-091..100, REQ-109..110. **Prerequisites:** Related backend contracts/operations from AP-07..AP-12. **Exclusive target scopes:** `web/`, including the future English text catalog and formatting layer; built asset integration `internal/webassets/` by integration only, no mandatory separate runtime web server.

- [ ] T046 [AP-13] Implement feed/search/filters/details, saved links/time anchors, aging highlights, multiple tabs, safe reconciliation; route product-owned UI text through text keys from the start.
- [ ] T047 [AP-13] Implement authorized Unit/organization/Gateway/account/subscription/plugin management and bounded rule/presentation editors; make previews/errors/revisions usable with English keyed validation/status/accessibility text.
- [ ] T048 [AP-13] Select/integrate a suitable Vue i18n facility and English text catalog/fallback; support interpolation/plurals, independent persisted formatting locale, dates/numbers/time zones, and no sentence concatenation. Test F-26, key completeness, copy/navigation preservation, phone/desktop touch/keyboard, layout resilience, and non-color-only states. Require no additional translations or language selector; hidden controls never replace backend authorization.

**Acceptance:** Real desktop/mobile browser workflows with permitted/forbidden/revoked data; offline/live interruption never implies freshness. English-only catalogs render complete user-visible text, fallback/parameters/plurals work, and formatting locale is independent. Original Payloads and operator data remain unchanged. **Non-goals:** Premature PWA/push/OIDC/SCIM UI, additional translated languages, automatic Payload translation. Select actual frontend commands after toolchain choice; do not pretend an npm application already exists.

## AP-14 Later Extension Boundaries

**Owner:** Respective domain with product/integration ownership. **Contracts:** Prepared C-06/C-07/C-09/C-10/C-13 boundaries; concrete extension contracts later. **REQ:** REQ-053, REQ-065..068, REQ-080, REQ-083, REQ-101..104, REQ-125, REQ-128, REQ-131. **Current write scope:** Referenced contract boundaries only; no implementation paths approved.

- [ ] T049 [AP-14] After separate later scope approval, specify nested User groups, external identity/proxy/SCIM, cross-plugin Domain events, PWA/push, registration, or import/export as their own concrete contract/package slices.

**Acceptance now:** Requirements/prepared boundaries are preserved without claiming implementation. **Non-goals:** Implementing these extensions under this documentation task. delivery status remains exclusively in the plan. I18n readiness is initial preparation, not deferred to AP-14; additional translations remain unrequired.

## AP-15 Operations and Profile Transition

**Owner:** Operations/integration and domain schema owners. **Contract gate:** C-12/C-13, C-01/C-08/C-09 restore/idempotency/attempt boundaries. **REQ:** REQ-111..120, REQ-126..127. **Prerequisites:** AP-05..AP-12. **Exclusive target scopes:** `internal/operations/`, future `packaging/` and deployment/operator artifacts. Platform/root changes by AP-05 integration owner only; define new operations documents before package approval.

- [ ] T050 [AP-15] Implement/document storage status, configured retention, setup without default credentials, orderly shutdown, update/migration workflows; use English project/operator text and stable machine-readable status codes.
- [ ] T051 [AP-15] Implement/test consistent backup/restore and Lite->Full with IDs, grants, configuration, pending work, and secrets.
- [ ] T052 [AP-15] Demonstrate single-process Lite, Full authority/lease/failover boundaries, disk failure/crash/restart, F-19/F-23/F-25. No automatic external-Delivery replay after restore.

**Acceptance:** Traceable actual restore/profile-transition evidence, honest acceptance state, no silent retention. **Non-goals:** Custom consensus, quantitative RPO/RTO/SLA, unverified hardware support.

## AP-16 Integration and Delivery Evidence

**Owner:** Integration with all domain owners. **Contract gate:** Approved versions for agreed Delivery scope. **REQ:** Every active identifier in the approved slice, particularly REQ-097..100/105..110/120/126..127. **Prerequisites:** All related product packages and AP-04 approval. **Write scope:** Integration/approval evidence and shared build/API/SDK wiring only; domain corrections through respective owners.

- [ ] T053 [AP-16] Complete REQ-to-test/operations evidence for the approved slice; distinguish fixtures, real SQL/HTTP/browser/plugin/provider/recovery checks. Include English/i18n-readiness evidence without requiring a translated second catalog.
- [ ] T054 [AP-16] Integrate contract versions, endpoints/SDK, build/asset compatibility, gate outcomes; record deviations/remaining blockers in the plan, no blanket completion claim.

**Acceptance:** No active delivery scope without traceable evidence; later/removed identifiers remain identifiable. **Non-goals:** Inventing features, required translations, or quantitative promises during acceptance.

## Requirement Mapping and Planned Evidence

This is traceability, **not a second delivery-status table**. Package assignment means intended contribution, not approval/completion. All evidence below is future; none of these implementations exists yet. AP-01..AP-04 additionally govern respective contract/approval prerequisites; AP-16 governs approved delivery evidence.

| REQ | Packages | Planned evidence |
| --- | --- | --- |
| REQ-001 | AP-06, AP-09, AP-13 | Authorized live feed in HTTP/browser workflow |
| REQ-002 | AP-06, AP-09, AP-13 | Further Transmission updates a loaded Message |
| REQ-003 | AP-13 | Change type and time-based fading |
| REQ-004 | AP-09, AP-13 | Cursor history without losing selection |
| REQ-005 | AP-09, AP-13 | Live history or visible limit/reconciliation |
| REQ-006 | AP-09, AP-13 | Visible freshness loss |
| REQ-007 | AP-09, AP-13 | Reconnect with revocation/deletion |
| REQ-008 | AP-09, AP-13 | Concurrent tabs/filters and shared IP |
| REQ-009 | AP-09, AP-13 | Safe time/age projection |
| REQ-010 | AP-09, AP-13 | Time anchor/latest preserving filters |
| REQ-011 | AP-03, AP-09 | Backend-parallel literal Unicode substrings |
| REQ-012 | AP-09, AP-13 | Unit OR filter |
| REQ-013 | AP-08, AP-09 | Genuinely permitted unassigned data with Network filter |
| REQ-014 | AP-09, AP-13 | Authorized dimensions/filter options |
| REQ-015 | AP-09 | OR within, correlated AND between dimensions |
| REQ-016 | AP-09, AP-13 | Link/selection/time restoration |
| REQ-017 | AP-07, AP-09 | F-11..F-14, no metadata oracles |
| REQ-018 | AP-09, AP-13 | Authorized detail projection |
| REQ-019 | AP-08, AP-09, AP-13 | Authorized prefilled detail action |
| REQ-020 | AP-01, AP-06, AP-08, AP-13 | Protocol variant with effect preview |
| REQ-021 | AP-08 | Unit deletion and historical rematch F-15 |
| REQ-022 | AP-08, AP-13 | Consistent Unit presentation |
| REQ-023 | AP-08, AP-13 | Inline organization with separate authorization |
| REQ-024 | AP-08, AP-13 | Defaults/overrides/provenance/reset |
| REQ-025 | AP-08, AP-13 | Protocol-correct Unit template |
| REQ-026 | AP-08, AP-13 | Bulk preview/errors/deletion |
| REQ-027 | AP-09, AP-13 | Unit-to-Message navigation |
| REQ-028 | AP-08, AP-13 | Multiple cross-Network Unit collections |
| REQ-029 | AP-07..AP-09 | Same-Transmission/assignment fixture F-11 |
| REQ-030 | AP-08, AP-13 | Formation without claimed co-alerting |
| REQ-031 | AP-01, AP-06, AP-08 | Protocol-specific matching fixtures |
| REQ-032 | AP-01, AP-08 | Specificity and conflict rejection |
| REQ-033 | AP-08 | Temporal Address intervals/boundaries |
| REQ-034 | AP-08 | Retroactive matching using Observation time |
| REQ-035 | AP-08 | Rematch debounce/maximum delay with controlled clock |
| REQ-036 | AP-08 | Complete impact scope, no color-only rematch |
| REQ-037 | AP-06, AP-08 | Separate function meanings per Unit/organization |
| REQ-038 | AP-06, AP-08 | Protocol defaults without false national standard |
| REQ-039 | AP-07, AP-08 | Network-scoped editing/hidden uses |
| REQ-040 | AP-08 | Unassigned versus uncertain/failed |
| REQ-041 | AP-06, AP-07 | Identifiable authorized blockable Gateway |
| REQ-042 | AP-06 | Gateway Networks and Network/Protocol boundary |
| REQ-043 | AP-06 | Network precedence/reject unauthorized declaration |
| REQ-044 | AP-06 | Single-Network Gateway without unnecessary ranges |
| REQ-045 | AP-08, AP-13 | Safely redacted conflict explanation |
| REQ-046 | AP-06..AP-09 | Unknown context separately authorizable |
| REQ-047 | AP-06, AP-13 | Gateway state/times |
| REQ-048 | AP-06 | Explicit termination versus lost contact |
| REQ-049 | AP-06, AP-10 | Subscribable Gateway Domain event |
| REQ-050 | AP-06, AP-13, AP-15 | Distinct Transmission/contact/removed history |
| REQ-051 | AP-07, AP-13 | Visible direct/User group permissions |
| REQ-052 | AP-07 | Additive multiple grants |
| REQ-053 | AP-14 | Separate later nesting contract |
| REQ-054 | AP-07 | Additive domain scopes |
| REQ-055 | AP-07, AP-09 | Field/Transmission target projection |
| REQ-056 | AP-07, AP-09 | Explicit unassigned-data permission |
| REQ-057 | AP-07, AP-09, AP-13 | Usable guest without default rights |
| REQ-058 | AP-07, AP-13 | Guest writes/warning/not deletable |
| REQ-059 | AP-07, AP-09, AP-10 | Shared age limit across query/details/Delivery |
| REQ-060 | AP-07..AP-09, AP-13 | Grant provenance/current assignment/safe scope deletion |
| REQ-061 | AP-07, AP-14 | Principal/credential boundary, later bindings |
| REQ-062 | AP-02, AP-07 | Field matrix/account deletion F-22 |
| REQ-063 | AP-07, AP-13 | Profile maintenance without contact-as-identity |
| REQ-064 | AP-07, AP-13 | Local login/suspension |
| REQ-065 | AP-07, AP-14 | Prepared internal ID; later OIDC without duplicate account |
| REQ-066 | AP-14 | Separate trusted-proxy boundary |
| REQ-067 | AP-14 | Linking through both authentications |
| REQ-068 | AP-14 | Controlled external-identity unlinking |
| REQ-069 | AP-07 | No password/recovery bypass of excluded login |
| REQ-070 | AP-07, AP-09, AP-13 | Session revocation stops new private output |
| REQ-071 | AP-07, AP-13 | Old/new/confirmed password checks |
| REQ-072 | AP-07, AP-13 | Recovery request without account oracle |
| REQ-073 | AP-07, AP-13 | One-time proof, no administrator-known password |
| REQ-074 | AP-07 | Username/expiry/one-time use |
| REQ-075 | AP-07, AP-15 | Local rescue without silent account replacement |
| REQ-076 | AP-07, AP-13 | Recovery/approval target rights |
| REQ-077 | AP-07 | Own restricted keys |
| REQ-078 | AP-07 | Targeted technical-account keys |
| REQ-079 | AP-07, AP-13 | Rotation/revocation/last use |
| REQ-080 | AP-14 | Later separate external/local lifecycle |
| REQ-081 | AP-06, AP-10 | Explicit Domain event selection including Transmission/correction |
| REQ-082 | AP-10, AP-13 | Chosen Delivery targets/Payload within current rights |
| REQ-083 | AP-14 | Later cross-plugin contract without SDK dead end |
| REQ-084 | AP-10, AP-11, AP-13 | Separate instances/secrets |
| REQ-085 | AP-07, AP-10, AP-13 | Pause/deletion/suspension stops new Delivery |
| REQ-086 | AP-10, AP-13 | Retry/final/uncertain statuses |
| REQ-087 | AP-10, AP-13 | Authorized deliberate repeat with duplicate risk |
| REQ-088 | No implementation package | Reserved identifier, no legacy adapter |
| REQ-089 | AP-11, AP-13 | Operator lifecycle/compatibility/error status |
| REQ-090 | AP-11 | Versioned SDK and four separate target modules |
| REQ-091 | AP-12, AP-13 | Bounded extraction/structure/conditions |
| REQ-092 | AP-12, AP-13 | Preview without real side effects |
| REQ-093 | AP-12, AP-13 | Saved/active/previous version |
| REQ-094 | AP-12, AP-13 | Permission-preserving presentation |
| REQ-095 | AP-08, AP-12, AP-13 | Field errors/revision conflicts |
| REQ-096 | AP-12, AP-13 | Title/notices/safe dismissal rule |
| REQ-097 | AP-02, AP-03, AP-09, AP-12, AP-13 | F-26: English catalog/fallback, keys/parameters/plurals, independent persistent locale preferences; no additional translations |
| REQ-098 | AP-09, AP-13 | Copy/detail navigation preserves selection |
| REQ-099 | AP-13, AP-16 | Mobile/desktop touch/keyboard |
| REQ-100 | AP-13, AP-16 | Not color/mouse only |
| REQ-101 | AP-14 | Later PWA installation |
| REQ-102 | AP-14 | Later authorized background push |
| REQ-103 | AP-14 | Later longer-lived revocable device login |
| REQ-104 | AP-14 | Later PWA offline freshness |
| REQ-105 | AP-07, AP-10 | Current rights before every attempt F-17 |
| REQ-106 | AP-10, AP-11 | Provider acceptance is not reading/recall |
| REQ-107 | AP-05, AP-07, AP-11 | Secret/error/log/Payload redaction |
| REQ-108 | AP-06, AP-07, AP-09, AP-10 | Throttling without blanket location/IP block |
| REQ-109 | AP-07, AP-13 | Authorized secret-free security audit |
| REQ-110 | AP-07, AP-09, AP-13 | Safe login/forbidden/invisible-object errors |
| REQ-111 | AP-15 | Configured retention, no default deletion |
| REQ-112 | AP-06, AP-15 | Potentially removed history visible |
| REQ-113 | AP-05, AP-06, AP-10 | F-01/F-02 retry versus independent Transmission |
| REQ-114 | AP-05, AP-06, AP-15 | Honest acceptance point/commit uncertainty |
| REQ-115 | AP-05, AP-15 | Shared Lite process/artifact |
| REQ-116 | AP-05, AP-15 | Documented conditional crash-loss behavior |
| REQ-117 | AP-05, AP-10, AP-15 | Joint Full roles/database/live/worker qualification |
| REQ-118 | AP-05, AP-15 | Combined app container, separate supplementary services |
| REQ-119 | AP-15 | Demonstrated Lite->Full transition |
| REQ-120 | AP-05, AP-15, AP-16 | Setup without default credentials, status/shutdown/update |
| REQ-121 | AP-01, AP-06 | Four Protocol models/fixtures |
| REQ-122 | AP-01, AP-06 | Sliding cross-Network Payload Grouping |
| REQ-123 | AP-08, AP-09, AP-10 | F-12/F-13 suppression without discarding data |
| REQ-124 | AP-08, AP-09 | Encryption label without decryption claim |
| REQ-125 | AP-14 | Later voluntary registration without default permissions |
| REQ-126 | AP-15, AP-16 | Actual backup/restore verification |
| REQ-127 | AP-03, AP-15, AP-16 | Documented resource/history profiling without SLA |
| REQ-128 | AP-14 | Later import/export without legacy mandate |
| REQ-129 | AP-10, AP-11 | Target-specific Unit override without secret leak |
| REQ-130 | AP-10 | Domain event/age/duplicate/correction rules during recovery |
| REQ-131 | AP-14 | Later explicit SCIM mapping with origin-specific revocation |
| REQ-132 | AP-08 | Detach organization, preserve defaults, no rematch F-16 |

## Validation Commands and Remaining Gates

There is no Go module, frontend manifest, test configuration, or CI yet. Do not invent currently executable build/test commands. AP-04 records actual focused commands per approved package once available; `go test ./...` is only a future overall check after creating the Go module and replaces neither real PostgreSQL nor browser/provider/HA evidence. I18n checks will validate the English-only catalog, mappings, fallback, interpolation/plurals, formatting separation, and layout, without requiring additional translated files.

The task list is deliberately not labeled a fully approved, immediately executable implementation plan: missing domain decisions, technical evidence, and scope approval are real gates. Documentary completeness is checked through IDs/links/ownership/status, not fictional runtime outcomes. Translation of planning data does not approve any blocked package.