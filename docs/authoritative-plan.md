# GoPager: Authoritative Product and Architecture Plan

Date: 2026-10-24. Revision: 3.1.

**Status:** The defined specification is documented in the dedicated repository. Go and the modular monolith are decided. Other technology selections are the planning baseline with validation pending. Phase 1 remains a scope proposal; contract drafts and work packages are documented, not approved for product implementation. English is the authoritative project language; i18n preparation is required without additional translations yet.

## Authority and Maintenance

The [authority matrix](README.md#document-authority) defines document ownership. Only this plan maintains delivery status, current product decisions, open issues, and approvals. Detailed REQ text belongs exclusively to the [catalog](requirements-catalog.md); technical details belong to the [architecture](architecture.md) and [contracts](domain-contracts.md).

The consolidated external planning file dated 2026-10-23 is provenance, not parallel product documentation to maintain. Its contradictory open-decision lists and earlier name candidates are superseded. The external file was not modified. Earlier individual planning files were not assumed to exist. Repository planning data is maintained entirely in English; the German source is historical provenance only.

## Product Goal and Priorities

GoPager helps users stay informed about relevant pager alarms and trace what was received. Reception, domain assignment, technical grouping, current and historical search, and explicit event delivery make up the product.

Evaluate in this order: performance/resources on constrained hosts; maintainable, understandable code with justified dependencies; intuitive domain-oriented operation; flexibility with fine-grained permissions and extensible delivery. Security is mandatory. Existing code, stack, and migration effort receive no preference.

## Confirmed Decisions

| ID | Decision | Authoritative elaboration |
| --- | --- | --- |
| D-01 | The product name is GoPager; use only its dedicated repository for the new product. No new naming round. Trademark/domain availability has not been checked. | [Provenance](requirements-evidence.md) |
| D-02 | Go backend as a modular monolith. No parallel complete Node/Rust implementations to reopen the fundamental choice. | [Technology decisions](architecture.md#technology-decisions) |
| D-03 | One codebase for Lite and Full; single-process Lite without a mandatory external runtime, database, or broker. A shared artifact is the selected architecture. | [Operating profiles](architecture.md#operating-profiles) |
| D-04 | Full enables HA/scaling; a combined app container remains possible but promises neither database HA nor single-container HA. No divergent writes under uncertain write authority. | C-12 |
| D-05 | Operators provide separate plugins; an SDK/API with stable models is mandatory. No marketplace, end-user remote installation, or legacy compatibility. Installation is a trust decision; an SDK is not a security guarantee. | C-10 |
| D-06 | Initial targets are separate Pushover, Telegram, email, and webhook plugins. Events, targets, and content are selected explicitly. Later PWA push uses the same authorization. | C-09, C-10 |
| D-07 | Additive principal permissions, current unit assignments even for history, restricting API keys, and no implicit rights from plugin fields/SCIM. Search AND is not grant union. | C-06 |
| D-08 | Units may belong to multiple domain collections and have cross-network identities. Filter dimensions correlate within the same reception/assignment. | C-04, C-07 |
| D-09 | Temporal validity applies to address assignments. Unit deletion rematches history without new receptions or regrouping. Organization deletion preserves/detaches units without rematching and retains effectively applied defaults. | C-04, C-05 |
| D-10 | Suppression retains data but removes affected recipients from normal views/search/details/delivery; mixed messages remain usable for other recipients. No separate content masking or implicit admin bypass view. | C-05, C-06 |
| D-11 | Stable internal principals separate from credentials. Later OIDC/SCIM: issuer+subject, never matching email as an account link; explicit group mappings and separate manual memberships. | C-06, C-13 |
| D-12 | Bounded extraction/regex/structured data/conditions with preview are the workflow minimum. Flexible composition preferred; no arbitrary scripts/loops/privilege expansion or automatically approved universal builder. | C-11 |
| D-13 | Small internal profiling/spikes are allowed. No required quantitative performance/acceptance thresholds, SLA, or RPO/RTO. | [Validation](work-packages.md#ap-03-technical-validation-planning) |
| D-14 | If this substantially simplifies the solution, Lite may lose even acknowledged inputs in a crash window. Conditional allowance, not a goal or defined loss budget. Acceptance status remains honest. | C-01, C-12 |
| D-15 | Unlimited retention until the operator sets a limit, no silent default deletion; handle exhaustion visibly. | C-12 |
| D-16 | Search uses literal substrings, case-insensitive, Unicode-consistent, accent-sensitive; empty text means no text filter. No implicit regex/query parser. | C-07 |
| D-17 | Group content-identical messages of the same protocol across networks; sliding window since the last matching reception and one shared configurable duration. Duration/default remain open. No claim of a common operational incident. | C-02, C-03 |
| D-18 | EAS supplements POCSAG/FLEX/ZVEI in the proposed initial scope. No automatic decoder/audio/video/CAP promise. | C-02 |
| D-19 | The documented initial scope explicitly remains a proposal. The documentation task and its execution do not approve product implementation. | [Delivery scope](#delivery-scope) |
| D-20 | English is the authoritative language for the project, planning data, and default product text. Prepare i18n from the first implementation, without requiring additional translations. Formatting locale is separate from UI language. | REQ-097; [Language and internationalization](architecture.md#language-and-internationalization), C-07/C-11, AP-13 |

Technology selection, alternatives, and validation needs are maintained exclusively in the [architecture](architecture.md#technology-decisions). The old statement that no technologies have been selected no longer applies.

## Operating Assumptions

Typically 150-300 units and 500-1500 raw telegrams/day per network. Lite: usually 1-2 sources, approximately 5 at most, 1-3 networks, usually fewer than 10 users. Full: 1-10 networks, around 2 remote sources/network, and 10-150 users. Web use ranges from weekly to ten times daily per user; delivery is mainly provider-based. These are planning profiles, not burst/capacity promises or technical limits.

Pi/thin client/VPS/Proxmox and stronger VMs/home servers/Kubernetes are examples, not minimum hardware or support commitments. Internal profiling requires an available constrained reference host with a documented environment; do not invent hardware certification.

## Delivery Scope

**Single delivery-status source; every Phase-1 entry is a proposal.** Detailed goals appear only in the [catalog](requirements-catalog.md). Requirements assigned to later delivery are not forgotten work packages. D-20 applies across the project and prepares i18n; it does not require additional translations or change other scope approvals.

| REQ IDs | Delivery status |
| --- | --- |
| REQ-001..020 | Phase-1 proposal: feed/history, search, filters, safe details, and authorized detail actions. |
| REQ-021..050 | Phase-1 proposal: units, organizations, templates/defaults, bulk operations, assignments/rematching, and sources/networks/status. |
| REQ-051..052 | Phase-1 proposal: direct permissions and flat multiple group memberships. |
| REQ-053 | Later: nested user groups. |
| REQ-054..064 | Phase-1 proposal: scopes/fields/guest/history age, local users, and profile functions. |
| REQ-065..068 | Later: OIDC/proxy/login linking; prepare only the principal/credential boundary. |
| REQ-069..079 | Phase-1 proposal: password/session/recovery/rescue and restricted keys. |
| REQ-080 | Later with external identity integration. |
| REQ-081..082 | Phase-1 proposal: explicit event subscriptions, targets, and permitted content. |
| REQ-083 | Later: full cross-plugin event support; do not unnecessarily close the SDK boundary. |
| REQ-084..087 | Phase-1 proposal: target instances, pausing, status, retry, and deliberate redelivery. |
| REQ-088 | Removed and reserved; no legacy compatibility. |
| REQ-089..090 | Phase-1 proposal: operator-managed plugin lifecycle, SDK/API, and four separate delivery plugins. |
| REQ-091..100 | Phase-1 proposal: minimum workflow, presentation, previews/conflicts, usability, and basic branding; English-first i18n preparation under REQ-097. |
| REQ-101..104 | Later: PWA, background push, longer device login, and offline freshness. |
| REQ-105..124 | Phase-1 proposal: current delivery authorization, security, retention, operating profiles, POCSAG/FLEX/ZVEI/EAS, grouping, and suppression. |
| REQ-125 | Later: voluntary registration. |
| REQ-126..127 | Phase-1 proposal: backup/restore and qualitative load-profile consideration. |
| REQ-128 | Later: unit import/export. |
| REQ-129..130 | Phase-1 proposal: unit-specific target settings and event/duplicate/age rules. |
| REQ-131 | Later: SCIM with explicit mapping. |
| REQ-132 | Phase-1 proposal: delete organization, preserve units/effective defaults, no rematching. |

## Open Issues and Risks

Existing P IDs are preserved; P-19 was not assigned in the source. 'Closed' refers only to the named product decision, not technical evidence. Details remain open in the referenced contracts.

| ID | Current status / required outcome | Contracts | Clarification package |
| --- | --- | --- | --- |
| P-01 | Lite loss allowance closed; acceptance point, commit uncertainty, and actual crash window technically open. | C-01, C-08, C-12 | AP-02, AP-03 |
| P-02 | Internal measurement allowed; do not invent SLA/acceptance thresholds. | C-12 | AP-03 |
| P-03 | Unlimited default retention closed; storage warnings, write failures, and idempotency receipt retention open. | C-01, C-12 | AP-02 |
| P-04 | Primary/fencing baseline selected; concrete Full failover/recovery and authority-loss behavior open. | C-08, C-12 | AP-02, AP-03 |
| P-05 | Wasm/Wazero selected; ABI, compatible toolchains/versions, startup/cancellation, and single-process evidence open. | C-10 | AP-02, AP-03 |
| P-06 | Minimum operator lifecycle decided; manifest, SDK versioning, updates/rollback open. | C-10 | AP-02 |
| P-07 | Minimal authorized host capabilities set; secret/I/O/resource contract and adversarial tests open. | C-09, C-10 | AP-02, AP-03 |
| P-08 | Shared sliding window decided; per-protocol content identity, EAS scope, time basis, duration/default, exact boundary, lateness, and configuration changes open. Blocks ingest/grouping freeze. | C-01..C-03 | AP-01 |
| P-09 | Same-reception/assignment correlation set; concrete network/recipient types and safe query integration open. | C-01, C-02, C-05..C-07 | AP-01, AP-02 |
| P-10 | No metadata leaks or implicit suppression bypass; hidden/unassigned projection and live revocation open. | C-05..C-07 | AP-02 |
| P-11 | Current permissions per attempt, finite retries, and uncertain outcomes set; backoff/deadlines/receipt retention and redelivery authorization open. | C-08..C-10 | AP-02 |
| P-12 | No new receptions/groups during rematching; safe revision switching and concurrent ingest/revocation open. Blocks rematching freeze. | C-04..C-07 | AP-02 |
| P-13 | Default preservation on organization deletion closed; technical materialization open. | C-04 | AP-02 |
| P-14 | Anonymization decided; field-specific deletion/retention rules open. Blocks account-deletion freeze. | C-06, C-09, C-13 | AP-02 |
| P-15 | Search semantics/database-integrated approach decided; Unicode form, indexes, and backend parity need validation. | C-07 | AP-02, AP-03 |
| P-16 | Minimum workflow decided; concrete rule/presentation models open. `essen.js` unverified, neither a parity blocker nor a parity promise. | C-11 | AP-02 |
| P-17 | OIDC/SCIM deliberately later; do not bring forward a provisioning platform. External/manual authority details belong to that phase. | C-06, C-13 | AP-14 |
| P-18 | Scope remains proposed; provenance B is not a mandatory feature; technical packages cannot approve scope. | All | AP-04 |
| P-20 | Single container is not an HA promise; boundary documented, operations not qualified. | C-12 | AP-03 |
| P-21 | No hardware/capacity promise; concrete profiling reference and measurement environment open. | C-12 | AP-03 |

## Workflow and Approval Gates

1. **Documentation:** Markdown specification only. Complete requirements, technologies, owners, contracts, and dependent packages in the repository, maintained in English.
2. **Domain contracts:** AP-01/AP-02 resolve blockers; review examples and the logical model. Do not distribute unresolved shared semantics to independent feature implementations. Include the i18n/API-message boundary without requiring translations.
3. **Technical evidence:** AP-03 plans and, after separate approval, executes narrow spikes, not parallel complete products. A negative result requires documented architecture updates.
4. **Legacy assessment:** only after target decisions and only for deliberate reuse; record source/ref/symbol/test per adoption. No automatic code/data migration.
5. **Contract freeze and scope:** AP-04 records user scope approval and, per contract, version, owner, producers/consumers, authorization, errors, time, idempotency, and transactions with positive/negative fixtures.
6. **Implementation:** only packages with resolved prerequisites, named owners, and executable test plans. Shared boundaries have an integration owner. No blanket parallel approval.

## Claims Not Made

No application implemented, contract frozen, or product package approved. No performance/provider/hardware/HA/recovery/RPO/RTO measurement, validated plugin isolation, or complete historical source comparison with `v1.6.0-beta`. Documentary interfaces are not compilable Go or generated OpenAPI artifacts. Database drivers, exact versions, and technical defaults are not fixed by this consolidation. English documentation and i18n planning are not an implemented localization runtime or an additional translated product.