# GoPager: Authoritative Product and Architecture Plan

Date: 2026-10-24. Revision: 3.1.

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
| D-08 | Units may belong to multiple Unit collections and have cross-Network identities. Filter dimensions correlate within the same Transmission/assignment. | C-04, C-07 |
| D-09 | Temporal validity applies to Address assignments. Unit deletion rematches history without new Transmissions or regrouping. Organization deletion preserves/detaches Units without rematching and retains effectively applied defaults. | C-04, C-05 |
| D-10 | Suppression retains data but removes affected Transmission targets from normal views/search/details/Delivery; mixed Messages remain usable for other Transmission targets. No separate Payload masking or implicit admin bypass view. | C-05, C-06 |
| D-11 | Stable internal Principals separate from credentials. Later OIDC/SCIM: issuer+subject, never matching email as an account link; explicit User group mappings and separate manual memberships. | C-06, C-13 |
| D-12 | Bounded extraction/regex/structured data/conditions with preview are the workflow minimum. Flexible composition preferred; no arbitrary scripts/loops/privilege expansion or automatically approved universal builder. | C-11 |
| D-13 | Small internal profiling/spikes are allowed. No required quantitative performance/acceptance thresholds, SLA, or RPO/RTO. | [Validation](work-packages.md#ap-03-technical-validation-planning) |
| D-14 | If this substantially simplifies the solution, Lite may lose even acknowledged inputs in a crash window. Conditional allowance, not a goal or defined loss budget. Acceptance status remains honest. | C-01, C-12 |
| D-15 | Unlimited retention until the operator sets a limit, no silent default deletion; handle exhaustion visibly. | C-12 |
| D-16 | Search uses literal substrings, case-insensitive, Unicode-consistent, accent-sensitive; empty text means no text filter. No implicit regex/query parser. | C-07 |
| D-17 | Payload-identical Transmissions with the same Payload form and value (declared by Protocol definitions, e.g. POCSAG/FLEX text, numeric, tone-only) across Protocols and Networks; Address/function are Transmission target attributes; v1 exact canonical match only; sliding window over reported Observation instants with one shared configurable window (default 30 s, inclusive boundary) (0 disables Grouping, no upper limit); no maximum Message age; Messages are chains of Payload-identical Transmissions with gaps <= W, merged on gap closure with alias forwarding of absorbed IDs; late Transmissions link by Observation time without cut-off; configuration changes apply to assignments processed after the change only in v1 (regrouping is a later investigation item); dedicated audited capability for configuration changes (2026-10-25, draft until AP-04). No claim of a common operational incident. | C-02, C-03 |
| D-18 | EAS supplements POCSAG/FLEX/ZVEI in the proposed initial scope. No automatic decoder/audio/video/CAP promise. | C-02 |
| D-19 | The documented initial scope explicitly remains a proposal. The documentation task and its execution do not approve product implementation. | [delivery scope](#delivery-scope) |
| D-20 | English is the authoritative language for the project, planning data, and default product text. Prepare i18n from the first implementation, without requiring additional translations. Formatting locale is separate from UI language. | REQ-097; [Language and internationalization](architecture.md#language-and-internationalization), C-07/C-11, AP-13 |
| D-21 | The [glossary](glossary.md) defines binding domain vocabulary (Transmission, Message, Message target, Payload, Address, Unit, Gateway, Delivery, User group, Unit collection, Group call) and prohibits ambiguous terms listed in the glossary's Prohibited and Restricted Terms section. It lists logical stored information without prescribing a data model. The glossary is the single authoritative source for names: packages add new names there, record collisions/renames in its rename log, and must propagate renames to all referring documents and code (glossary maintenance rules). Draft until AP-04 (2026-10-25). | All |

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
| REQ-126..127 | Phase-1 proposal: backup/restore and qualitative load-profile consideration. |
| REQ-128 | Later: Unit import/export. |
| REQ-129..130 | Phase-1 proposal: Unit-specific target settings and Domain event/duplicate/age rules. |
| REQ-131 | Later: SCIM with explicit mapping. |
| REQ-132 | Phase-1 proposal: delete organization, preserve Units/effective defaults, no rematching. |

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
| P-08 | Decided (D-17, C-01..C-03; draft until AP-04 freeze): shared sliding window and fixed Observation time basis (RFC 3339 with mandatory offset, at least whole seconds, future times beyond a 15-minute default clock-skew tolerance accepted and marked), window default/range, inclusive boundary, chain/merge rule, late linking, configuration-change behavior, policy-revision activation, configuration capability, Payload identity and Payload forms per Protocol, idempotency, rejected-Submission behavior. Idempotency residual risk after retention deletion accepted (F-23). Remaining blocker before the freeze: verification of the FLEX type list and capcode details against the FLEX specification (AP-04 freeze check). Handed over: duration data type (AP-03), Domain event schemas/alias resolution (AP-02), similarity heuristic and regrouping (later phase). Blocks ingest/Grouping freeze. | C-01..C-03 | AP-01 |
| P-09 | Same-Transmission/assignment correlation set; Network resolution decided (C-01: declared, inferred-single, inferred-address, mandatory per-Gateway default Network; only assigned Networks). Concrete Network/Transmission target identifier types and safe query integration open. | C-01, C-02, C-05..C-07 | AP-01 (resolution), AP-02 (types, queries) |
| P-10 | No metadata leaks or implicit suppression bypass; hidden/unassigned projection and live revocation open. | C-05..C-07 | AP-02 |
| P-11 | Current permissions per attempt, finite retries, and uncertain outcomes set; backoff/deadlines/receipt retention and redelivery authorization open. | C-08..C-10 | AP-02 |
| P-12 | No new Transmissions/Messages during rematching; safe revision switching and concurrent ingest/revocation open. Blocks rematching freeze. | C-04..C-07 | AP-02 |
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
2. **Domain contracts:** AP-01 is complete (2026-10-25, user-confirmed; contracts remain 0.1-draft, not frozen); AP-02 resolves the remaining blockers; review examples and the logical model. Do not distribute unresolved shared semantics to independent feature implementations. Include the i18n/API text boundary without requiring translations.
3. **Technical evidence:** AP-03 plans and, after separate approval, executes narrow spikes, not parallel complete products. A negative result requires documented architecture updates.
4. **Legacy assessment:** only after target decisions and only for deliberate reuse; record source/ref/symbol/test per adoption. No automatic code/data migration.
5. **Contract freeze and scope:** AP-04 records user scope approval and, per contract, version, owner, producers/consumers, authorization, errors, time, idempotency, and transactions with positive/negative fixtures.
6. **Implementation:** only packages with resolved prerequisites, named owners, and executable test plans. Shared boundaries have an integration owner. No blanket parallel approval.

## Claims Not Made

No application implemented, contract frozen, or product package approved. No performance/provider/hardware/HA/recovery/RPO/RTO measurement, validated plugin isolation, or complete historical source comparison with `v1.6.0-beta`. Documentary interfaces are not compilable Go or generated OpenAPI artifacts. Database drivers, exact versions, and technical defaults are not fixed by this consolidation. English documentation and i18n planning are not an implemented localization runtime or an additional translated product.