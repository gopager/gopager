# GoPager Product Decisions, Scope and Approval

Date: 2026-10-25. This plan owns product decision disposition, delivery scope and approval, not a second technical specification or execution log. See the [authority matrix](README.md#document-authority).

## Product Goal and Priorities

Help users stay informed about relevant pager Messages and trace received evidence.

Evaluate in this order: performance/resources on constrained hosts; maintainable, understandable code with justified dependencies; intuitive domain-oriented operation; flexibility with fine-grained permissions and extensible Delivery. Security is mandatory. Existing code, stack and migration effort receive no preference.

## Confirmed Decisions

The stable D-IDs below record decision topics and supersession. The **owner link is the sole current definition** of the chosen technical behavior, including constraints and exceptions. Follow it rather than inferring behavior from the title. New decisions use the same routing; do not copy owner content into this table.

| ID | Decision topic / disposition | Authoritative detail |
| --- | --- | --- |
| D-01 | <a id="d-01"></a>Confirmed: product identity is GoPager in its dedicated repository; no new naming round. Availability checks are not claimed. | This row; [source provenance](requirements-evidence.md#source-inventory) |
| D-02 | <a id="d-02"></a>Confirmed technology direction; not reopened by implementation experiments | [TECH-01](architecture.md#technology-decisions) |
| D-03 | <a id="d-03"></a>Confirmed shared Lite/Full application direction | [TECH-02 and profile topology](architecture.md#operating-profiles) |
| D-04 | <a id="d-04"></a>Confirmed Full authority/HA boundary | [C-12](contracts/c-12-persistence-operations.md) |
| D-05 | <a id="d-05"></a>Confirmed extension distribution/SDK boundary, elaborated by D-24 | [C-10](contracts/c-10-plugin-host.md) |
| D-06 | <a id="d-06"></a>Confirmed initial Delivery target selection and subscription boundary | [REQ-090](requirements-catalog.md#i-domain-events-extensions-and-deliveries), [C-09](contracts/c-09-delivery.md), [C-10](contracts/c-10-plugin-host.md) |
| D-07 | <a id="d-07"></a>Confirmed permission model | [C-06](contracts/c-06-identity-authorization.md) |
| D-08 | <a id="d-08"></a>Confirmed Unit organization and filter correlation | [C-04](contracts/c-04-master-data.md), [C-07](contracts/c-07-query-live.md) |
| D-09 | <a id="d-09"></a>Confirmed temporal assignment/deletion effects | [C-04](contracts/c-04-master-data.md), [C-05](contracts/c-05-matching-correction.md) |
| D-10 | <a id="d-10"></a>Confirmed suppression boundary | [C-05](contracts/c-05-matching-correction.md), [C-06](contracts/c-06-identity-authorization.md) |
| D-11 | <a id="d-11"></a>Confirmed identity/credential separation and later external-identity boundary | [C-06](contracts/c-06-identity-authorization.md), [C-13](contracts/c-13-account-lifecycle.md) |
| D-12 | <a id="d-12"></a>Confirmed processing minimum and extensibility constraints | [C-11](contracts/c-11-processing-presentation.md) |
| D-13 | <a id="d-13"></a>Confirmed measurement/acceptance policy | [REQ-120](requirements-catalog.md#l-operations-data-volume-and-evolution), [REQ-127](requirements-catalog.md#m-additional-domain-capabilities), [AP-03 authorization procedure](work-packages/ap-03-technical-validation.md) |
| D-14 | <a id="d-14"></a>Confirmed conditional Lite durability allowance | [REQ-116](requirements-catalog.md#l-operations-data-volume-and-evolution), [C-01](contracts/c-01-transmission-acceptance.md), [C-12](contracts/c-12-persistence-operations.md) |
| D-15 | <a id="d-15"></a>Confirmed retention default and exhaustion behavior | [C-12](contracts/c-12-persistence-operations.md) |
| D-16 | <a id="d-16"></a>Confirmed search semantics | [C-07](contracts/c-07-query-live.md) |
| D-17 | <a id="d-17"></a>Confirmed Grouping policy; comparison scope amended by D-30 | [C-02](contracts/c-02-protocol-payload.md), [C-03](contracts/c-03-message-grouping.md) |
| D-18 | <a id="d-18"></a>Confirmed Protocol-scope amendment | [REQ-121](requirements-catalog.md#m-additional-domain-capabilities), [C-02](contracts/c-02-protocol-payload.md) |
| D-19 | <a id="d-19"></a>Confirmed: documentation does not approve product implementation | [Scope](#delivery-scope), [approval gates](#workflow-and-approval-gates) |
| D-20 | <a id="d-20"></a>Confirmed language/i18n direction | [Project language](README.md#project-language), [REQ-097](requirements-catalog.md#j-processing-presentation-and-usability), [responsibility routing](architecture.md#language-and-internationalization) |
| D-21 | <a id="d-21"></a>Confirmed naming authority | [Glossary maintenance](glossary.md#maintenance-rules) |
| D-22 | <a id="d-22"></a>Confirmed internal-time separation; public-time exception superseded by D-28 | [C-03](contracts/c-03-message-grouping.md), [C-07](contracts/c-07-query-live.md) |
| D-23 | <a id="d-23"></a>Confirmed retention/independent-work boundary | [C-12 retention](contracts/c-12-persistence-operations.md), [C-09 input/dependencies](contracts/c-09-delivery.md), [C-08 receipts](contracts/c-08-events-jobs.md) |
| D-24 | <a id="d-24"></a>Confirmed plugin trust/capability model | [C-10](contracts/c-10-plugin-host.md) |
| D-25 | <a id="d-25"></a>Confirmed ingest publication and presentation boundary | [C-11](contracts/c-11-processing-presentation.md), [C-09 predicates](contracts/c-09-delivery.md) |
| D-26 | <a id="d-26"></a>Confirmed deletion model; supersedes anonymity proposals | [C-13](contracts/c-13-account-lifecycle.md) |
| D-27 | <a id="d-27"></a>Storage model superseded by D-29; no independent current definition | [D-29](#d-29); [C-07 diagnostics](contracts/c-07-query-live.md) retains the separate Gateway-filter boundary |
| D-28 | <a id="d-28"></a>Confirmed query-time model; supersedes universal public Message-time exception | [C-07 time algorithm](contracts/c-07-query-live.md#time-selection-and-one-row-message-results), [C-05 stored minima](contracts/c-05-matching-correction.md#target-resolution-message-contributions-and-ingest-storage) |
| D-29 | <a id="d-29"></a>Confirmed logical model replacing D-27; heuristic scope amended by D-30. Physical fields/types/indexes explicitly deferred to owner design/freeze, not approved by diagram. | [Architecture relationships/ownership](architecture.md#logical-relationship-model), [C-01 originals](contracts/c-01-transmission-acceptance.md), [C-03 membership](contracts/c-03-message-grouping.md), [C-05 derivation](contracts/c-05-matching-correction.md), [C-07 projection](contracts/c-07-query-live.md), [C-11 results](contracts/c-11-processing-presentation.md) |
| D-30 | <a id="d-30"></a>Heuristic delivery deferred; supersedes activation/detail proposals under D-17/D-29. Interrupted threshold exploration was not approval. | [Scope](#delivery-scope), [C-02 comparison boundary](contracts/c-02-protocol-payload.md), [C-03 current membership](contracts/c-03-message-grouping.md), [AP-14 later work](work-packages/ap-14-later-extensions.md) |
| D-31 | <a id="d-31"></a>Application Backup/Restore removed; supersedes former application-lifecycle proposals | [Reserved REQ-126](requirements-catalog.md#m-additional-domain-capabilities), [C-12 Operator boundary](contracts/c-12-persistence-operations.md#operator-backup-boundary-and-profile-transition), [C-13 personal-data boundary](contracts/c-13-account-lifecycle.md) |
| D-32 | <a id="d-32"></a>Approved AP-04 delivery amendment: Lite architecture preparation now, runnable Lite later; qualifies D-03 delivery timing without replacing the shared application direction | [AP-04 scope decision](#ap-04-scope-decision) |
| D-33 | <a id="d-33"></a>Approved AP-04 Webhook capability expansion; initial Pushover/Webhook, Telegram/email later; qualifies D-06 timing without technical freeze | [AP-04 scope decision](#ap-04-scope-decision) |
| D-34 | <a id="d-34"></a>Approved AP-04 FLEX type/address clarification of D-18; preserves D-29's logical Message model | [AP-04 FLEX decision](#ap-04-flex-decision), [C-02](contracts/c-02-protocol-payload.md) |
| D-35 | <a id="d-35"></a>Approved AP-04 foundation-only freeze and AP-05 binding; execution waits for an explicit AP-05 start, all other technical slices remain gated | [Foundation freeze decision](#ap-04-foundation-freeze-decision) |

## Operating Assumptions

Typically 150-300 Units and 500-1500 raw Transmissions/day per Network. Lite: usually 1-2 Gateways, approximately 5 at most, 1-3 Networks, usually fewer than 10 users. Full: 1-10 Networks, around 2 remote Gateways/Network, and 10-150 users. Web use ranges from weekly to ten times daily per user; Delivery is mainly provider-based. These are planning profiles, not burst/capacity promises or technical limits.

Pi/thin client/VPS/Proxmox and stronger VMs/home servers/Kubernetes are examples, not minimum hardware or support commitments. Profiling reference-host evidence belongs to the AP-03 execution record when produced.

## Delivery Scope

This is the only delivery-phase classification. It does not repeat REQ acceptance text. The user approved the Phase-1 proposal on 2026-10-25 with the [AP-04 amendments](#ap-04-scope-decision). Rows retaining the original proposal label are included by that approval except where explicitly qualified below; scope approval is not contract freeze or implementation approval.

| REQ IDs / subject reference | Delivery disposition |
| --- | --- |
| REQ-001..052 | Phase-1 proposal |
| REQ-053 | Later |
| REQ-054..064 | Phase-1 proposal |
| REQ-065..068 | Later; prepare only the contracted identity boundary initially |
| REQ-069..079 | Phase-1 proposal |
| REQ-080 | Later |
| REQ-081..082 | Phase-1 proposal |
| REQ-083 | Later |
| REQ-084..087 | Phase-1 proposal |
| REQ-088 | Removed; ID reserved |
| REQ-089 | Phase 1 approved |
| REQ-090 | Initial SDK/Pushover/Webhook approved; Telegram/email later under D-33 |
| REQ-091..100 | Phase 1 approved |
| REQ-101..104 | Later |
| REQ-105..114 | Phase 1 approved |
| REQ-115..116 | Later runtime delivery; architectural preparation in Phase 1 under D-32 |
| REQ-117..118 | Phase 1 approved; D-32 defers Lite-specific runtime evidence |
| REQ-119 | Later executable transition; architectural preparation in Phase 1 under D-32 |
| REQ-120..124 | Phase 1 approved |
| REQ-125 | Later |
| REQ-126 | Removed under D-31; ID reserved |
| REQ-127 | Phase-1 proposal |
| REQ-128 | Later |
| REQ-129..130 | Phase-1 proposal |
| REQ-131 | Later |
| REQ-132 | Phase-1 proposal |
| [D-30 comparison extension](#d-30) | Later, separately approved slice; not a current contract blocker |

Resolve IDs in the [catalog](requirements-catalog.md), not in a parallel feature summary.

### AP-04 Scope Decision

**Date/origin:** 2026-10-25, AP-04 T015, with consequences for T017/T018. **Accountable owner/approver:** Product/integration; the user as sole approving authority. **Status:** approved product scope; this approval alone grants no technical freeze or product implementation. The later D-35 foundation approval is separate. Baseline: repository `1a7132370f86f2f345c1b4a97d0ed00764c15648`, initially clean. Evidence and amendment handover: [AP-04 execution](execution/ap-04.md).

**D-32 question and options:** Deliver runnable Lite with the first release, or retain its architecture while deferring that runtime. The user selected the latter: later Lite support must be achievable through alternative infrastructure adapters rather than restructuring the application. The shared domain/application model remains; no separate Lite product is approved. This changes delivery timing, not the historical AP-03 results or D-29's logical Message model. After approving AP-05 T019 and AP-15 T051/T052, the user explicitly approved the necessary consistency changes to affected architecture, requirements, contracts, not-yet-started downstream APs and incoming references. Implementation starts with Full/PostgreSQL; Lite-specific adapters, runtime/parity demonstrations and executable profile transition are later deliverables. Existing transaction, authorization and logical Message semantics remain unchanged.

**D-33 question and options:** Retain the previously proposed Webhook scope, or explicitly include configurable formats, HTTP methods and authentication. The user selected expansion; the capability requirements are maintained in [REQ-090](requirements-catalog.md#i-domain-events-extensions-and-deliveries) and the draft boundary in [C-09](contracts/c-09-delivery.md#initial-target-capabilities). After an initially masked authentication selection, the user explicitly clarified Bearer-token support. Asked whether to deliver two or four initial targets, the user selected Pushover and Webhook now, Telegram and email later. This qualifies D-06's delivery timing. The user approved the necessary owner-document and not-yet-started downstream AP consistency amendments. These are approved capability intentions, not frozen signing/encoding or authorization rules; C-09/C-10 review remains required.

**D-33 GET resolution, 2026-10-25, AP-04 T017:** A trigger-only GET was offered and rejected: GET must be able to carry configured Message input. Body-only, nonconfidential-query-only and removal of GET were then offered. The user instead expressly selected configurable transport for legacy systems, with the Operator responsible for consciously choosing insecure methods. DOM-07/plugin host own the resulting draft in [C-09](contracts/c-09-delivery.md#initial-target-capabilities). This expands the draft beyond the earlier nonconfidential-query recommendation, not a claim that query transport is safe. Explicit Operator policy, current authorized projections and GoPager's own Secret/log protections remain required; external logging cannot be guaranteed. No silent TLS downgrade or implicit authorization expansion is granted. Exact configuration/signature schemas and fixtures still require review before freeze.

**Consequences and boundaries:** The original full proposal remains approved subject to D-32/D-33 and the existing later/removed classifications. D-32 affects AP-05/AP-06/AP-09/AP-15; D-33 affects AP-11; owner and coverage references are aligned with the approved delivery timing. The user additionally required conformance to the existing AP-01..AP-03 logical Message model and confirmed that the backend defines Gateway API formats. Gateway implementations are outside AP-04: source observations do not define or constrain its API. C-02 remains the owner of Gateway reassembly and submitted target semantics. Historical execution evidence and glossary meanings remain unchanged. Current owner drafts carry explicit AP-04 amendments, not retroactive claims about earlier evidence.

**D-33 configuration-authority clarification, 2026-10-26, AP-04 T017:** The user explicitly requires administrative configuration for every integration, rather than independent User-created integrations. Where the provider supports individually addressable recipients, Users may configure their own recipient credentials and Subscriptions on that administratively configured integration. The Pushover example establishes the intended split. Product/integration (user) approves this clarification of D-06/D-33; DOM-07 owns the boundary in [C-09](contracts/c-09-delivery.md#configuration-authority), with plugin host/integration responsible for its manifest/configuration exposure through [C-10](contracts/c-10-plugin-host.md). Operator responsibility for integration security does not remove current data authorization or GoPager's own Secret protections. This refines the current drafts, not earlier execution evidence or a frozen contract. AP-10/AP-11 consume the linked owner contracts; existing tasks already cover configuration and authorization, so no additional downstream task or implementation approval is needed.

**D-33 manifest follow-up, 2026-10-26, AP-04 T017:** The user explicitly requires plugins to declare which properties Users and administrators may configure in their manifests. Product/integration (user) approves this requirement rather than relying on UI conventions alone; plugin host/integration owns the [C-10 draft field contract](contracts/c-10-plugin-host.md#manifest-configuration-fields). It operationalizes the preceding authority clarification without relaxing C-06/C-09, changing earlier execution evidence, or approving generated SDK artifacts/implementation. Planned cases belong to [F-45](contracts/reference-cases.md#ap-04-plugin-configuration-cases); AP-11 T037/T042 already consume the manifest/SDK contract.

### AP-04 FLEX Decision

**D-34; date/origin:** 2026-10-25, AP-04 T016/T017. **Status:** approved draft amendment, not contract freeze. **Accountable owner/approver:** Product/integration, user; DOM-02 owns C-02 with DOM-01 for submitted originals. **Question/context:** the [direct specification review](execution/ap-04.md#e-06-direct-flex-specification-review) found omitted Short Message cases and ambiguous CAPCODE notation in the earlier C-02 draft.

**Options/resolution:** The user selected `short_numeric=numeric` rather than deferral; `source_code_identity=distinct_signal` rather than tone-only coalescing or deferral; and explicitly approved decimal-address CAPCODE clarification. These decisions preserve distinguishable signal information and avoid treating routing notation as an integer address. Behavior and exact mappings belong solely to [C-02](contracts/c-02-protocol-payload.md#ap-04-flex-clarification), terminology to the [glossary](glossary.md), and examples to [F-42..F-44](contracts/reference-cases.md#ap-04-protocol-cases).

**Approval/impact:** User approval covers the C-02 draft and necessary terminology/case/not-yet-started consumer amendments described in the clarification. It qualifies D-18's type inventory without replacing D-29/D-30 or rewriting AP-01..AP-03 evidence. AP-06 T023/T024 consumes the amended version and cases; AP-09 must preserve its existing authorized projection boundary. No Gateway adapter implementation or decoder adoption is included. No earlier frozen version exists; the draft amendment history remains explicit.

## Open Issues and Risks

P-19 was never assigned. This table owns **remaining gate/status**, not the already-defined contract semantics. A resolved documentary question is not proof of implementation.

| ID | Remaining gate / disposition | Owner detail | Responsible package |
| --- | --- | --- | --- |
| P-01 | Technical durability/crash and commit-uncertainty evidence open | [C-01](contracts/c-01-transmission-acceptance.md), [C-08](contracts/c-08-events-jobs.md), [C-12](contracts/c-12-persistence-operations.md) | AP-03 |
| P-02 | Policy resolved by D-13; experimental authorization still required | [D-13](#d-13), [approval gates](#workflow-and-approval-gates) | AP-03 |
| P-03 | Documentary boundary resolved; backend evidence open | [C-12](contracts/c-12-persistence-operations.md) | AP-03 |
| P-04 | Full failover/driver fencing evidence open | [C-08](contracts/c-08-events-jobs.md), [C-12](contracts/c-12-persistence-operations.md) | AP-03 |
| P-05 | Exact ABI/toolchain/runtime and resource/cancellation evidence open; freeze pending | [C-10](contracts/c-10-plugin-host.md) | AP-03 / AP-04 |
| P-06 | Documentary lifecycle resolved; enforcement/toolchain evidence open | [C-10](contracts/c-10-plugin-host.md) | AP-03 |
| P-07 | Adversarial capability/I/O/cancellation evidence open | [C-09](contracts/c-09-delivery.md), [C-10](contracts/c-10-plugin-host.md) | AP-03 |
| P-08 | Duration/precision/concurrency evidence and FLEX fact verification open; D-30 is not a blocker | [C-01](contracts/c-01-transmission-acceptance.md), [C-02](contracts/c-02-protocol-payload.md), [C-03](contracts/c-03-message-grouping.md), [C-08](contracts/c-08-events-jobs.md) | AP-03 / AP-04 |
| P-09 | Logical-model question resolved by D-29/D-30; physical design at owner gates, not a diagram freeze | [D-29](#d-29), [C-05](contracts/c-05-matching-correction.md) | AP-03 / AP-04 / affected implementation owner |
| P-10 | Documentary projection/revocation boundary resolved; enforcement evidence open | [C-06](contracts/c-06-identity-authorization.md), [C-07](contracts/c-07-query-live.md) | AP-03 |
| P-11 | Documentary contract resolved; worker/provider enforcement evidence and freeze open | [C-08](contracts/c-08-events-jobs.md), [C-09](contracts/c-09-delivery.md), [C-10](contracts/c-10-plugin-host.md) | AP-03 / AP-04 |
| P-12 | Documentary correction model resolved; concurrency/publication evidence open | [C-05](contracts/c-05-matching-correction.md) | AP-03 |
| P-13 | Documentary question closed; implementation/freeze separately gated | [C-04](contracts/c-04-master-data.md) | AP-04 / AP-08 |
| P-14 | Documentary question closed; freeze and lifecycle execution evidence remain | [C-13](contracts/c-13-account-lifecycle.md) | AP-04 / AP-07 |
| P-15 | Unicode implementation pin, indexing and backend parity evidence open | [C-07](contracts/c-07-query-live.md) | AP-03 |
| P-16 | Documentary semantics closed; budgets/installed-step evidence open; library/editor selections belong to implementation owners | [C-11](contracts/c-11-processing-presentation.md) | AP-03 / AP-12 / AP-13 |
| P-17 | Deliberately later, not a current implementation blocker | [C-06](contracts/c-06-identity-authorization.md), [C-13](contracts/c-13-account-lifecycle.md) | AP-14 |
| P-18 | Product scope approved with D-32/D-33 amendments; D-35 freezes only AP-05 foundation, all other technical slices remain gated | [Delivery scope](#delivery-scope), [approval gates](#workflow-and-approval-gates) | AP-04 |
| P-20 | Operations qualification open | [C-12](contracts/c-12-persistence-operations.md) | AP-03 |
| P-21 | Available reference host and measured environment missing | [Operating assumptions](#operating-assumptions), [C-12](contracts/c-12-persistence-operations.md) | AP-03 |

### AP-04 Foundation Freeze Decision

**D-35; date/origin:** 2026-10-26, AP-04 T016..T018. **Accountable owner/approver:** Product/integration, user. **Status:** approved; explicit answers `completion_scope=foundation_ap05`, `foundation_freeze_approval=true`, `consumer_binding_approval=true`. **Question/options:** close the whole initial product's technical contracts, or close only the next implementation foundation while keeping other boundaries gated. The user selected the foundation and separately approved its technical freeze and consumer binding. Product scope and D-32..D-34 remain unchanged.

**Approved resolution:** Freeze only [C-12 foundation 1.0](contracts/c-12-persistence-operations.md#ap-05-foundation-boundary), [C-01 intake-commit 1.0](contracts/c-01-transmission-acceptance.md#ap-05-intake-commit-boundary), [C-08 transaction participation 1.0](contracts/c-08-events-jobs.md#ap-05-transaction-participation), [C-07 platform-errors 1.0](contracts/c-07-query-live.md#ap-05-platform-error-binding), and the referenced [shared foundation conventions](contracts/shared-conventions.md#ap-05-foundation-conventions). Bind [glossary G-1.0](glossary.md) without changing existing meanings and approve the [TECH foundation tuple](architecture.md#ap-05-foundation-version-selection). Exact future domain schemas, Gateway HTTP API, search, auth/account implementation, job engine, plugin ABI, Delivery and frontend contracts remain unfrozen. There is no whole-C-ID or whole-product approval.

**Rationale/evidence:** [AP-04 E-13](execution/ap-04.md#e-13-completion-review-and-freeze-scope) identifies the actual remaining gaps. [AP-03 E-05](execution/ap-03.md#e-05-database-driver-and-temporal-probes) supports the proposed Go/pgx/PostgreSQL tuple and short transaction strategy, not production validation or a security certification. Alternatives are to repeat version experiments without a concrete need, or adopt untested replacement versions; neither is required to prepare this bounded foundation. Observation-time precision is deliberately outside the slice; platform clock precision does not settle it.

**Approval/impact:** The user approves these explicit owner-document addenda and technology selection, not revisions of earlier evidence; separately approves the [T018 handover](execution/ap-04.md#ap-05-consumer-binding) naming the Copilot SDK assistant as sequential AP-05 assignee with the AP-05 write scope and focused PostgreSQL/i18n checks. Product implementation is approved only for this bound AP-05 T019..T021 slice after the user's next explicit AP-05 start instruction; no AP-05 task runs during AP-04. AP-05 instructions already consume these boundaries and need no further baseline amendment. No parallel execution, Gateway code, SQLite implementation, domain schema ownership transfer or generated OpenAPI/SDK artifact is included. A later contract change needs a new version and affected-slice approval. Final exact documentary state and SHA-256 handover are in [AP-04 E-15](execution/ap-04.md#e-15-approved-freeze-and-final-verification).

## Workflow and Approval Gates

Only this section grants scope/execution approval. An approval entry must identify date, approving authority, exact AP/task slice, immutable contract/artifact versions, relevant evidence and permitted write-scope binding. Product scope, technical freeze and experiment authorization are separate approvals; none is implied by another.

| Gate | Approval condition | Current approval record |
| --- | --- | --- |
| Documentation | User authorization for specification maintenance | Authorized documentary work; not product implementation |
| AP-03 baseline clarification | Explicit approval to clarify that AP-03 is pre-implementation planning/feasibility and does not require a completed application or product runtime tests | Approved by the user on 2026-10-25. Narrowly clarifies the AP-03 task descriptions and acceptance; standalone feasible probes remain bounded by the Executable experiments gate. Product-/hardware-dependent validation is documented as future evidence, not an AP-03 completion blocker. No product behavior, production technology, or scope decision is made. |
| Executable experiments | **Approved 2026-10-25 by the user (sole approving authority)** for AP-03 T010..T012 isolated, disposable feasibility/conformance probes using Go 1.26.2; `modernc.org/sqlite` v1.61.0; `github.com/mattn/go-sqlite3` v1.14.52; `github.com/jackc/pgx/v5` v5.11.0; PostgreSQL 18.0 Alpine; `github.com/tetratelabs/wazero` v1.12.0; `golang.org/x/text` v0.43.0. AP-03/C-01..C-03, C-06..C-12 and shared conventions remain at their starting documented versions; no application artifact exists. Permitted: temporary external scratch harnesses, local database/container installation, bounded characterization/benchmark runs, and recording outputs in [AP-03 execution evidence](execution/ap-03.md). Forbidden: product/application code or manifest changes, production technology/contract decisions, AP baseline edits except the separately approved clarification recorded above, real secrets or external-provider effects. T013 is authorized as planning and resource-limited disposable-host characterization only; no Raspberry Pi measurement or hardware guarantee. Exact commands, environment, runtime image digest, limits, and results are in the AP-03 execution record. | User's explicit authorization in this session, 2026-10-25; bounded by AP-03 and the restrictions in this row. |
| Deliberate legacy reuse | Targeted source/ref/symbol/test assessment followed by explicit adoption decision | None recorded; [source observations](requirements-evidence.md) are not adoption |
| Product scope | Explicit user approval of the Phase-1 proposal or a smaller named slice | Approved by the user on 2026-10-25 for AP-04 T015 at baseline `1a7132370f86f2f345c1b4a97d0ed00764c15648`, qualified by [D-32/D-33](#ap-04-scope-decision). The user's subsequent `alignment_approval=true` extends documentary writes to necessary affected architecture, requirement/contract owner drafts, not-yet-started downstream APs and incoming references. Earlier execution evidence and the AP-04 definition stay unchanged. This product-scope approval alone grants no technical freeze, implementation, Gateway changes or parallel execution; D-35 below separately approves the foundation slice. |
| Contract freeze | AP-04 review of owner version, parties, fixtures, technical evidence and [shared completeness checklist](contracts/shared-conventions.md#freeze-requirements) | Approved by the user on 2026-10-26 (`foundation_freeze_approval=true`) for [D-35](#ap-04-foundation-freeze-decision) only: C-12 foundation 1.0, C-01 intake-commit 1.0, C-08 transaction-participation 1.0, C-07 platform-errors 1.0, shared foundation conventions 1.0 and G-1.0; exact owner sections, TECH tuple and evidence linked there. Full-file hashes identify the reviewed state, not a whole-file behavioral freeze. All other technical slices remain gated. |
| Product implementation | Approved slice, frozen/validated inputs, named assignee and runnable validation binding | Approved by the user on 2026-10-26 (`consumer_binding_approval=true`) only for AP-05 T019..T021 under [D-35](#ap-04-foundation-freeze-decision), activated by the user's next explicit AP-05 start instruction. Assignee, exact write-scope/input/validation binding: [AP-04 handover](execution/ap-04.md#ap-05-consumer-binding). No consumer task is executed in AP-04; no other AP or generated API/SDK artifact is approved. |
| Parallel execution | Explicitly disjoint approved scopes and shared-boundary integration coordination | None recorded |

Task progress, documentary completion and actual validation results are maintained only in [execution records](execution/README.md), not here.
