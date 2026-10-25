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

## Operating Assumptions

Typically 150-300 Units and 500-1500 raw Transmissions/day per Network. Lite: usually 1-2 Gateways, approximately 5 at most, 1-3 Networks, usually fewer than 10 users. Full: 1-10 Networks, around 2 remote Gateways/Network, and 10-150 users. Web use ranges from weekly to ten times daily per user; Delivery is mainly provider-based. These are planning profiles, not burst/capacity promises or technical limits.

Pi/thin client/VPS/Proxmox and stronger VMs/home servers/Kubernetes are examples, not minimum hardware or support commitments. Profiling reference-host evidence belongs to the AP-03 execution record when produced.

## Delivery Scope

This is the only delivery-phase classification. It does not repeat REQ acceptance text. Every Phase-1 assignment remains a **proposal**, not approval.

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
| REQ-089..100 | Phase-1 proposal |
| REQ-101..104 | Later |
| REQ-105..124 | Phase-1 proposal |
| REQ-125 | Later |
| REQ-126 | Removed under D-31; ID reserved |
| REQ-127 | Phase-1 proposal |
| REQ-128 | Later |
| REQ-129..130 | Phase-1 proposal |
| REQ-131 | Later |
| REQ-132 | Phase-1 proposal |
| [D-30 comparison extension](#d-30) | Later, separately approved slice; not a current contract blocker |

Resolve IDs in the [catalog](requirements-catalog.md), not in a parallel feature summary.

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
| P-18 | Product-scope approval missing | [Delivery scope](#delivery-scope) | AP-04 |
| P-20 | Operations qualification open | [C-12](contracts/c-12-persistence-operations.md) | AP-03 |
| P-21 | Available reference host and measured environment missing | [Operating assumptions](#operating-assumptions), [C-12](contracts/c-12-persistence-operations.md) | AP-03 |

## Workflow and Approval Gates

Only this section grants scope/execution approval. An approval entry must identify date, approving authority, exact AP/task slice, immutable contract/artifact versions, relevant evidence and permitted write-scope binding. Product scope, technical freeze and experiment authorization are separate approvals; none is implied by another.

| Gate | Approval condition | Current approval record |
| --- | --- | --- |
| Documentation | User authorization for specification maintenance | Authorized documentary work; not product implementation |
| Executable experiments | Separate narrow authorization under AP-03 T014 | None recorded |
| Deliberate legacy reuse | Targeted source/ref/symbol/test assessment followed by explicit adoption decision | None recorded; [source observations](requirements-evidence.md) are not adoption |
| Product scope | Explicit user approval of the Phase-1 proposal or a smaller named slice | None recorded |
| Contract freeze | AP-04 review of owner version, parties, fixtures, technical evidence and [shared completeness checklist](contracts/shared-conventions.md#freeze-requirements) | None recorded |
| Product implementation | Approved slice, frozen/validated inputs, named assignee and runnable validation binding | None recorded |
| Parallel execution | Explicitly disjoint approved scopes and shared-boundary integration coordination | None recorded |

Task progress, documentary completion and actual validation results are maintained only in [execution records](execution/README.md), not here.
