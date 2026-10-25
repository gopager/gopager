# GoPager: Authoritative Product and Architecture Plan

Date: 2026-10-25. Revision: 3.4.

**Status:** The defined specification is documented in the dedicated repository. Go and the modular monolith are decided. Other technology selections are the planning baseline with validation pending. Phase 1 remains a scope proposal; contract drafts and work packages are documented, not approved for product implementation. English is the authoritative project language; i18n preparation is required without additional translations yet.

## Authority and Maintenance

The [authority matrix](README.md#document-authority) defines document ownership. Only this plan maintains delivery status, current product decisions, open issues, and approvals. Detailed REQ text belongs exclusively to the [catalog](requirements-catalog.md); technical details belong to the [architecture](architecture.md) and [contracts](domain-contracts.md).

The consolidated external planning file dated 2026-10-23 is provenance, not parallel product documentation to maintain. Its contradictory open-decision lists and earlier name candidates are superseded. The external file was not modified. Earlier individual planning files were not assumed to exist. Repository planning data is maintained entirely in English; the German source is historical provenance only.

## Product Goal and Priorities

GoPager helps users stay informed about relevant pager Messages and trace what was received. Transmission, domain assignment, Grouping, current and historical search, and explicit Domain event-triggered Delivery make up the product.

Evaluate in this order: performance/resources on constrained hosts; maintainable, understandable code with justified dependencies; intuitive domain-oriented operation; flexibility with fine-grained permissions and extensible Delivery. Security is mandatory. Existing code, stack, and migration effort receive no preference.

## Confirmed Decisions

| ID | Decision | Authoritative elaboration |
| --- | --- | --- |
| D-01 | The product name is GoPager; use only its dedicated repository for the new product. No new naming round. Trademark/domain availability has not been checked. | [Provenance](requirements-evidence.md) |
| D-02 | Go backend as a modular monolith. No parallel complete Node/Rust implementations to reopen the fundamental choice. | [Technology decisions](architecture.md#technology-decisions) |
| D-03 | One codebase for Lite and Full; single-process Lite without a mandatory external runtime, database, or broker. A shared artifact is the selected architecture. | [Operating profiles](architecture.md#operating-profiles) |
| D-04 | Full enables HA/scaling; a combined app container remains possible but promises neither database HA nor single-container HA. No divergent writes under uncertain write authority. | C-12 |
| D-05 | Operators provide separate plugins; an SDK/API with stable models is mandatory. No marketplace, end-user remote installation, or legacy compatibility. Installation is a trust decision; an SDK is not a security guarantee. | C-10 |
| D-06 | Initial targets are separate Pushover, Telegram, email, and webhook plugins. Domain events, Delivery targets, and Payloads are selected explicitly. Later PWA push uses the same authorization. | C-09, C-10 |
| D-07 | Additive Principal permissions, current Unit assignments even for history, restricting API keys, and no implicit rights from plugin fields/SCIM. Search AND is not grant union. | C-06 |
| D-08 | Units may belong to multiple Unit collections and have cross-Network identities. Normal filter dimensions correlate within the same Message contribution; Gateway correlation is confined to administrative reception diagnostics. | C-04, C-07 |
| D-09 | Temporal validity applies to Address assignments. Unit deletion rematches history without new Transmissions or regrouping. Organization deletion preserves/detaches Units without rematching and retains effectively applied defaults. | C-04, C-05 |
| D-10 | Suppression retains data but removes affected Transmission targets from normal views/search/details/Delivery; mixed Messages remain usable for other Transmission targets. No separate Payload masking or implicit admin bypass view. | C-05, C-06 |
| D-11 | Stable internal Principals separate from credentials. Later OIDC/SCIM: issuer+subject, never matching email as an account link; explicit User group mappings and separate manual memberships. | C-06, C-13 |
| D-12 | Bounded extraction/regex/structured data/conditions with preview are the workflow minimum. Flexible composition preferred; no arbitrary scripts/loops/privilege expansion or automatically approved universal builder. | C-11 |
| D-13 | Small internal profiling/spikes are allowed. No required quantitative performance/acceptance thresholds, SLA, or RPO/RTO. | [Validation](work-packages/ap-03-technical-validation.md#ap-03-technical-validation-planning) |
| D-14 | If this substantially simplifies the solution, Lite may lose even acknowledged inputs in a crash window. Conditional allowance, not a goal or defined loss budget. Acceptance status remains honest. | C-01, C-12 |
| D-15 | Unlimited retention until the operator sets a limit, no silent default deletion; handle exhaustion visibly. | C-12 |
| D-16 | Search uses literal substrings, case-insensitive, Unicode-consistent, accent-sensitive; empty text means no text filter. No implicit regex/query parser. | C-07 |
| D-17 | Exact Grouping compares compatible Payload forms and canonical values across Protocols and Networks, never Units or Address rules. Sliding Observation-time window, default 30 s, inclusive boundary; 0 disables combining independent Transmissions. No maximum Message age or lateness cut-off; exact bridges merge with alias forwarding. Configuration changes apply to subsequent membership decisions, not automatic historical regrouping; dedicated audited capability required. D-30 fixes current scope to exact-only matching; original partial/deviating Payloads remain stored. Heuristics are later work, not contract blockers. No fixed-span window was approved by the fresh-design discussion. | C-02, C-03; D-29 |
| D-18 | EAS supplements POCSAG/FLEX/ZVEI in the proposed initial scope. No automatic decoder/audio/video/CAP promise. | C-02 |
| D-19 | The documented initial scope explicitly remains a proposal. The documentation task and its execution do not approve product implementation. | [delivery scope](#delivery-scope) |
| D-20 | English is the authoritative language for the project, planning data, and default product text. Prepare i18n from the first implementation, without requiring additional translations. Formatting locale is separate from UI language. | REQ-097; [Language and internationalization](architecture.md#language-and-internationalization), C-07/C-11, AP-13 |
| D-21 | The [glossary](glossary.md) defines binding domain vocabulary (Transmission, Message, Message target, Payload, Address, Unit, Gateway, Delivery, User group, Unit collection, Group call) and prohibits ambiguous terms listed in the glossary's Prohibited and Restricted Terms section. It lists logical stored information without prescribing a data model. The glossary is the single authoritative source for names: packages add new names there, record collisions/renames in its rename log, and must propagate renames to all referring documents and code (glossary maintenance rules). Draft until AP-04 (2026-10-25). | All |
| D-22 | AP-02 decisions are documentary 0.1-draft contracts, not implementation approval; current closure is recorded in Workflow and Approval Gates. Message time is the stored earliest member Observation time for internal Grouping/whole-Message retention, not a universally public sort/age key. D-28 defines the permission- and filter-dependent View time. This supersedes the earlier single-time/shared-metadata exception. | C-03/C-06/C-07/C-09/C-12 |
| D-23 | Retention deletes whole Messages and attached Transmission evidence, not arbitrary member Transmissions. A self-contained authorized Delivery input can outlive Message deletion for its bounded lifetime; missing dependencies cannot be fabricated. Old jobs may retain historical Assignment context, but current access/suspension/suppression checks still apply before new external attempts. Best-effort cancellation does not recall started effects. | C-05/C-08/C-09/C-12 |
| D-24 | Plugins are trusted operator-installed prebuilt artifacts, without build/package installation at deployment or Core rebuild. They may use libraries and approved host/container execution, files, arbitrary external protocols and instance-bound secrets, including externally supplied secrets. All SDK access to Core data must be declared in the manifest and explicitly authorized. This is not a hostile-code sandbox: broad host access can bypass application isolation, so protection against an intentionally malicious operator/plugin is not promised. Core remains one application process; optional plugin-created subprocesses are a deliberate exception, not mandatory Lite infrastructure. | C-10; TECH-08 |
| D-25 | Parsing is part of ingest publication: selected Workflows finish before the new state is shown or delivered. Multiple format-selective Workflows and trusted installed code steps are supported by the contract; no additional JS/Python runtime selected merely because it is permissible. Derived fields such as level are Subscription predicates, not Domain event types. GUI/Delivery templates share established logic syntax; admin-defined links, buttons, sanitized SVG/icons and configured map tiles may deliberately pass permitted parameters to external systems. No arbitrary browser script execution or implicit field permission. | C-09/C-11 |
| D-26 | Account deletion pseudonymizes historical references: retain the old non-authenticating Principal ID to distinguish deleted actors, remove profile/credentials/contact/secret fields, and display Deleted user with a distinguishing suffix. No assertion of anonymity or blanket purge of ordinary logs. Operators own backup retention/deletion; C-12 defines only the product's consistency/restore boundary. This supersedes the former anonymization requirement, not immutable received Payload. | REQ-062; C-13 |
| D-27 | Superseded in storage design by D-29. The earlier rule-version-keyed shared Assignment/Reception-link model and prohibition of per-target Unit columns are withdrawn. Gateway filters remain administrative diagnostics, not normal Message-search filters. | D-29; C-05/C-07 |
| D-28 | Query time is first-observed, not latest reception: authorize/suppress and apply non-time filters on Message contributions; combine matching contributions per Message and Unit into Unit appearances, taking their earliest allowed Observation time; apply optional age and the half-open requested time interval to those appearances, then show each Message once at the earliest remaining appearance time (View time). Without a Unit filter use the same rule over all allowed appearances, including explicitly allowed unassigned evidence. Sort descending by View time then stable Message ID. Message time is internal; View time is query-derived. Repeated receptions do not refresh Unit appearances. | C-05..C-09; F-11a..F-11i |
| D-29 | User-approved fresh logical model: immutable Transmission evidence with an original Payload per independent reception and exact original Transmission targets; mutable, stored Network/Unit/Address-rule resolution on each target; payload-based Message membership independent of Unit matching; shared, denormalized Message contributions with direct Unit references, first times and age anchors; Contribution supports trace contributions back to original targets, or to targetless Transmissions. Contributions group by Message/Network/Protocol/Address/Function/Unit or explicit status, not rule version or validity segment; rules remain temporal and are evaluated per original Observation time. No mandatory immutable Address-rule-version entity; configuration audit explains edits, while exact historical evaluation replay is not promised. Message uses a Representative Transmission for deterministic permitted GUI Payload selection: prefer exact members not marked incomplete, otherwise visibly qualify the fallback; tie-break by Acceptance time then ID, within readable non-suppressed evidence. Partial/deviating originals are preserved; D-30 defers heuristic membership and its correction mechanics entirely. No separate MessageGroup layer. Concrete fields, SQL types, indexes and necessary revision placement are elaborated in owner contracts/packages, not frozen by the illustrative ER diagram. Diagram stubs do not narrow existing Gateway/Network/Unit configuration, status, authorization, color/icon, organization, collection, template, default/override or Protocol-specific capabilities. | Glossary; architecture logical model; C-01..C-08/C-11; AP-02 documentation only |
| D-30 | Defer heuristics entirely from current delivery and AP-02 blockers. Current comparison is exact canonical Payload equality only, including accepted partial originals. Keep comparison separate from Grouping decisions and preserve match kind `exact` plus original evidence, so a later approved comparator can extend behavior. No active similarity settings/defaults, scores, fuzzy candidate scans, reconsideration or split machinery now. Supersedes heuristic activation and detail requirements in D-17/D-29; does not change original preservation, contribution storage, representative projection, temporal window or Unit Rematching. | C-02/C-03; AP-02/AP-06/AP-14 |
| D-31 | Backup/Restore is entirely manual Operator work, not application logic. No application backup API/manifest/export, restore detector/hold, restore-time credential invalidation or reconciliation workflow. Operator owns consistency, disposal, later revocations/deletions and external-effect risks of old copies. Normal startup/job idempotency, schema compatibility and the required Lite-to-Full transition remain separate. Supersedes earlier application Backup/Restore proposals; REQ-126 remains reserved as removed product scope. | C-12/C-13; REQ-126; affected AP handovers |

The interrupted eligibility/per-form/default/prefix/threshold clarification round is historical exploration, not active configuration. D-30 supersedes it; the suggested 85% threshold was never confirmed.

Technology selection, alternatives, and validation needs are maintained exclusively in the [architecture](architecture.md#technology-decisions). The old statement that no technologies have been selected no longer applies.

## Operating Assumptions

Typically 150-300 Units and 500-1500 raw Transmissions/day per Network. Lite: usually 1-2 Gateways, approximately 5 at most, 1-3 Networks, usually fewer than 10 users. Full: 1-10 Networks, around 2 remote Gateways/Network, and 10-150 users. Web use ranges from weekly to ten times daily per user; Delivery is mainly provider-based. These are planning profiles, not burst/capacity promises or technical limits.

Pi/thin client/VPS/Proxmox and stronger VMs/home servers/Kubernetes are examples, not minimum hardware or support commitments. Internal profiling requires an available constrained reference host with a documented environment; do not invent hardware certification.

## Delivery Scope

**Single delivery-status source; every Phase-1 entry is a proposal.** Detailed goals appear only in the [catalog](requirements-catalog.md). Requirements assigned to later delivery phases are not forgotten work packages. D-20 applies across the project and prepares i18n; it does not require additional translations or change other scope approvals.

| REQ IDs | delivery status |
| --- | --- |
| REQ-001..020 | Phase-1 proposal: feed/history, search, filters, safe details, and authorized detail actions. |
| REQ-021..050 | Phase-1 proposal: Units, organizations, templates/defaults, bulk operations, assignments/rematching, and Gateways/Networks/status. |
| REQ-051..052 | Phase-1 proposal: direct permissions and flat multiple User group memberships. |
| REQ-053 | Later: nested User groups. |
| REQ-054..064 | Phase-1 proposal: scopes/fields/guest/history age, local users, and profile functions. |
| REQ-065..068 | Later: OIDC/proxy/login linking; prepare only the Principal/credential boundary. |
| REQ-069..079 | Phase-1 proposal: password/session/recovery/rescue and restricted keys. |
| REQ-080 | Later with external identity integration. |
| REQ-081..082 | Phase-1 proposal: explicit Domain event subscriptions, targets, and permitted Payload. |
| REQ-083 | Later: full cross-plugin Domain event support; do not unnecessarily close the SDK boundary. |
| REQ-084..087 | Phase-1 proposal: Delivery targets, pausing, status, retry, and deliberate redelivery. |
| REQ-088 | Removed and reserved; no legacy compatibility. |
| REQ-089..090 | Phase-1 proposal: operator-managed plugin lifecycle, SDK/API, and four separate Delivery plugins. |
| REQ-091..100 | Phase-1 proposal: minimum workflow, presentation, previews/conflicts, usability, and basic branding; English-first i18n preparation under REQ-097. |
| REQ-101..104 | Later: PWA, background push, longer device login, and offline freshness. |
| REQ-105..124 | Phase-1 proposal: current Delivery authorization, security, retention, operating profiles, POCSAG/FLEX/ZVEI/EAS, Grouping, and suppression. |
| REQ-125 | Later: voluntary registration. |
| REQ-126 | Removed product scope (D-31): manual Operator Backup/Restore, no application feature. |
| REQ-127 | Phase-1 proposal: qualitative load-profile consideration. |
| REQ-128 | Later: Unit import/export. |
| REQ-129..130 | Phase-1 proposal: Unit-specific target settings and Domain event/duplicate/age rules. |
| REQ-131 | Later: SCIM with explicit mapping. |
| REQ-132 | Phase-1 proposal: delete organization, preserve Units/effective defaults, no rematching. |

## Open Issues and Risks

Existing P IDs are preserved; P-19 was not assigned in the source. 'Closed' refers only to the named product decision, not technical evidence. Details remain open in the referenced contracts.

| ID | Current status / required outcome | Contracts | Clarification package |
| --- | --- | --- | --- |
| P-01 | Acceptance/processing stages and commit-uncertainty response are documented. Lite's conditional loss allowance remains; actual durability/crash window needs technical evidence, not another AP-02 product decision. | C-01, C-08, C-12 | AP-03 validation |
| P-02 | Internal measurement allowed; do not invent SLA/acceptance thresholds. | C-12 | AP-03 |
| P-03 | Documentary scope resolved: whole-Message retention uses internal first Message time, even with recent evidence; unlimited default and visible exhaustion unchanged. C-12 defines failure/restart/profile transfer. D-31 excludes application Backup/Restore. Backend evidence remains open. | C-01, C-12 | AP-03 validation |
| P-04 | Primary/fencing and authority-loss stop behavior specified; concrete Full failover/driver enforcement still needs evidence. | C-08, C-12 | AP-03 validation |
| P-05 | Wasm/Wazero selected; bounded manifest/invocation semantics documented. Exact ABI exports, compatible toolchains/runtime versions, startup/cancellation and single-process evidence remain technical gates. | C-10 | AP-03 validation; AP-04 freeze |
| P-06 | Documentary lifecycle complete: prebuilt installation, compatibility, pinned invocation, disable/update/rollback. ABI/toolchain evidence remains; no installation-time build. | C-10 | AP-03 validation |
| P-07 | Trusted host-capability/Secret/HTTP/SMTP/DNS/redirect/resource policy defined under D-24. Adversarial enforcement and cancellation evidence remain; no malicious privileged-plugin isolation claim. | C-09, C-10 | AP-03 validation |
| P-08 | Exact sliding-window/Observation-time/merge/idempotency decisions remain D-17. D-30 defers heuristic matching entirely; it is not a current freeze blocker. C-02/C-03 define the exact comparison extension boundary and retain partial originals. FLEX verification remains AP-04; duration/concurrency evidence remains AP-03. | C-01..C-03/C-08 | AP-01 impact amended; AP-02 exact-only scope |
| P-09 | D-29/D-30 logical structure, resolution/support invariants and exact-only membership handover complete. C-01 Network precedence and EAS/POCSAG rules remain intact; no mandatory immutable rule/mapping-version entities. Concrete SQL fields/types/indexes follow owner implementation/freeze gates, as explicitly deferred by the user; diagram stubs do not remove master-data capabilities. | C-01..C-03, C-05..C-07 | AP-02 documentary closure; AP-03/AP-04 gates remain |
| P-10 | Hidden/unassigned projection, first-observed View time, age scoping, revocation, SSE resync and partial-suppression behavior specified; former shared Message-time exception withdrawn. Enforcement needs evidence. | C-05..C-07 | AP-03 validation |
| P-11 | Documentary contract complete: materialized inputs/dependencies, current authorization/start boundary, finite retry/uncertainty, explicit triggers and relevant appearance age. Provider/worker enforcement still requires validation/approved implementation. | C-08..C-10 | AP-03 validation; AP-04 freeze |
| P-12 | Rematch original targets at Observation time, replace current target resolutions, redistribute Contribution supports and rebuild affected Message contributions/first times. Rule changes do not regroup Messages. Rule validity is not edit-version history. Checked publication includes concurrent ingest and immediate invalidation. | C-04..C-07 | AP-02 model decided; AP-03 validation |
| P-13 | Documentary question resolved: atomic fieldwise default materialization/detachment and all-or-nothing bulk/revision semantics, with positive/conflict cases. | C-04 | Closed in AP-02; implementation separately gated |
| P-14 | Documentary question resolved: C-13 field matrix implements pseudonymization with distinct non-authenticating old IDs, immediate block and durable cleanup; no anonymity/log-purge/backup-erasure promise. Actual lifecycle tests remain implementation work. | C-06, C-09, C-13 | Closed in AP-02; AP-04/AP-07 gates remain |
| P-15 | NFC/full casefold/NFC search, scoped first-time query path and source-bound exact representative/result projection specified. Indexes, pinned Unicode implementation and backend parity still need evidence. | C-07 | AP-03 validation |
| P-16 | C-11 documentary contract complete: typed namespaced Workflows/results, selectors, publication barrier, preview/activation/reprocessing, sensitivity and shared constrained presentation/i18n semantics. Concrete established engine/builder/Vue i18n selection is owned by AP-12/AP-13 under those constraints, not an unresolved product choice. Runtime budgets/installed steps need AP-03 evidence. No legacy parity/adoption claimed. | C-11 | AP-03 validation; AP-12/AP-13 implementation selections |
| P-17 | OIDC/SCIM deliberately later; do not bring forward a provisioning platform. External/manual authority details belong to that phase. | C-06, C-13 | AP-14 |
| P-18 | Scope remains proposed; provenance B is not a mandatory feature; technical packages cannot approve scope. | All | AP-04 |
| P-20 | Single container is not an HA promise; boundary documented, operations not qualified. | C-12 | AP-03 |
| P-21 | No hardware/capacity promise; concrete profiling reference and measurement environment open. | C-12 | AP-03 |

## Workflow and Approval Gates

1. **Documentation:** Markdown specification only. Complete requirements, technologies, owners, contracts, and dependent packages in the repository, maintained in English.
2. **Domain contracts:** AP-02 completed its documentary scope on 2026-10-25: T004..T009 coverage and handover are recorded in [AP-02](work-packages/ap-02-domain-contracts.md). C-04..C-13 and C-01..C-03 dependencies align with D-24..D-31, the glossary and concrete planned cases. No AP-02 product question remains unassigned/open. Heuristics are deferred; application Backup/Restore is excluded. User-deferred physical field inventories are not silently frozen. All contracts remain 0.1-draft pending AP-03 technical evidence and AP-04 approval; no runtime/implementation success is implied.
3. **Technical evidence:** AP-03 plans and, after separate approval, executes narrow spikes, not parallel complete products. A negative result requires documented architecture updates.
4. **Legacy assessment:** only after target decisions and only for deliberate reuse; record source/ref/symbol/test per adoption. No automatic code/data migration.
5. **Contract freeze and scope:** AP-04 records user scope approval and, per contract, version, owner, producers/consumers, authorization, errors, time, idempotency, and transactions with positive/negative fixtures.
6. **Implementation:** only packages with resolved prerequisites, named owners, and executable test plans. Shared boundaries have an integration owner. No blanket parallel approval.

## Claims Not Made

No application implemented, contract frozen, or product package approved. No performance/provider/hardware/HA/recovery/RPO/RTO measurement, validated plugin isolation, or complete historical source comparison with `v1.6.0-beta`. Documentary interfaces are not compilable Go or generated OpenAPI artifacts. Database drivers, exact versions, and technical defaults are not fixed by this consolidation. English documentation and i18n planning are not an implemented localization runtime or an additional translated product.