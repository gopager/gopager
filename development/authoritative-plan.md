# GoPager Product Decisions, Scope and Approval

Date: 2026-10-25, restructured 2026-11-06 (D-135). This page owns product goal, delivery scope and the approval rule. Decisions are recorded as short ADRs in [decisions/](decisions/README.md); the approval narrative of earlier decisions is in the repository history of this file. It is not a second technical specification or execution log. See the [authority matrix](README.md#document-authority).

## Product Goal and Priorities

Help users stay informed about relevant pager Messages and trace received evidence.

Evaluate in this order: performance/resources on constrained hosts; maintainable, understandable code with justified dependencies; intuitive domain-oriented operation; flexibility with fine-grained permissions and extensible Delivery. Security is mandatory. Existing code, stack and migration effort receive no preference.

## Confirmed Decisions

Every decision keeps its stable D-ID. The [ADR index](decisions/README.md) lists title, status and date; each ADR file holds the decisions of one topic in short form. The **owner link inside each ADR is the sole current definition** of the chosen technical behavior. New decisions are added to the ADR file of their topic and to the index; do not copy owner content here. The table below only lets every D-ID and its anchor resolve from this page.

| ID | ADR |
| --- | --- |
| D-01 | <a id="d-01"></a>[Confirmed](decisions/01-platform-and-storage.md#d-01) |
| D-02 | <a id="d-02"></a>[Technology direction](decisions/01-platform-and-storage.md#d-02) |
| D-03 | <a id="d-03"></a>[Shared Lite/Full application direction](decisions/01-platform-and-storage.md#d-03) |
| D-04 | <a id="d-04"></a>[Full authority/HA boundary](decisions/01-platform-and-storage.md#d-04) |
| D-05 | <a id="d-05"></a>[Extension distribution/SDK boundary](decisions/04-processing-and-plugins.md#d-05) |
| D-06 | <a id="d-06"></a>[Initial Delivery target selection and subscription boundary](decisions/05-events-and-delivery.md#d-06) |
| D-07 | <a id="d-07"></a>[Permission model](decisions/06-identity-and-authorization.md#d-07) |
| D-08 | <a id="d-08"></a>[Unit organization and filter correlation](decisions/03-assignment-and-correction.md#d-08) |
| D-09 | <a id="d-09"></a>[Temporal assignment/deletion effects](decisions/03-assignment-and-correction.md#d-09) |
| D-10 | <a id="d-10"></a>[Suppression boundary](decisions/03-assignment-and-correction.md#d-10) |
| D-11 | <a id="d-11"></a>[Identity/credential separation and later external-identity boundary](decisions/06-identity-and-authorization.md#d-11) |
| D-12 | <a id="d-12"></a>[Processing minimum and extensibility constraints](decisions/04-processing-and-plugins.md#d-12) |
| D-13 | <a id="d-13"></a>[Measurement/acceptance policy](decisions/01-platform-and-storage.md#d-13) |
| D-14 | <a id="d-14"></a>[Conditional Lite durability allowance](decisions/01-platform-and-storage.md#d-14) |
| D-15 | <a id="d-15"></a>[Retention default and exhaustion behavior](decisions/01-platform-and-storage.md#d-15) |
| D-16 | <a id="d-16"></a>[Search semantics](decisions/07-query-and-live.md#d-16) |
| D-17 | <a id="d-17"></a>[Grouping policy](decisions/02-intake-and-grouping.md#d-17) |
| D-18 | <a id="d-18"></a>[Protocol-scope amendment](decisions/02-intake-and-grouping.md#d-18) |
| D-19 | <a id="d-19"></a>[Confirmed](decisions/09-process-and-documentation.md#d-19) |
| D-20 | <a id="d-20"></a>[Language/i18n direction](decisions/09-process-and-documentation.md#d-20) |
| D-21 | <a id="d-21"></a>[Naming authority](decisions/09-process-and-documentation.md#d-21) |
| D-22 | <a id="d-22"></a>[Internal-time separation](decisions/07-query-and-live.md#d-22) |
| D-23 | <a id="d-23"></a>[Retention/independent-work boundary](decisions/01-platform-and-storage.md#d-23) |
| D-24 | <a id="d-24"></a>[Plugin trust/capability model](decisions/04-processing-and-plugins.md#d-24) |
| D-25 | <a id="d-25"></a>[Ingest publication and presentation boundary](decisions/04-processing-and-plugins.md#d-25) |
| D-26 | <a id="d-26"></a>[Deletion model](decisions/06-identity-and-authorization.md#d-26) |
| D-27 | <a id="d-27"></a>[Storage model superseded by D-29](decisions/02-intake-and-grouping.md#d-27) |
| D-28 | <a id="d-28"></a>[Query-time model](decisions/07-query-and-live.md#d-28) |
| D-29 | <a id="d-29"></a>[Logical model replacing D-27](decisions/02-intake-and-grouping.md#d-29) |
| D-30 | <a id="d-30"></a>[Heuristic delivery deferred](decisions/02-intake-and-grouping.md#d-30) |
| D-31 | <a id="d-31"></a>[Application Backup/Restore removed](decisions/01-platform-and-storage.md#d-31) |
| D-32 | <a id="d-32"></a>[AP-04 Scope Decision](decisions/01-platform-and-storage.md#d-32) |
| D-33 | <a id="d-33"></a>[AP-04 Scope Decision](decisions/04-processing-and-plugins.md#d-33) |
| D-34 | <a id="d-34"></a>[AP-04 FLEX Decision](decisions/02-intake-and-grouping.md#d-34) |
| D-35 | <a id="d-35"></a>[AP-04 Foundation Freeze Decision](decisions/01-platform-and-storage.md#d-35) |
| D-36 | <a id="d-36"></a>[Repository-wide Library Policy Decision](decisions/01-platform-and-storage.md#d-36) |
| D-37 | <a id="d-37"></a>[AP-05 Migration Revision Decision](decisions/01-platform-and-storage.md#d-37) |
| D-38 | <a id="d-38"></a>[AP-05 Data Access Decision](decisions/01-platform-and-storage.md#d-38) |
| D-39 | <a id="d-39"></a>[C-03 Reassessment Decision](decisions/03-assignment-and-correction.md#d-39) |
| D-40 | <a id="d-40"></a>[Transmission and Message Architecture Reassessment](decisions/02-intake-and-grouping.md#d-40) |
| D-41 | <a id="d-41"></a>[Repository Structure and Test Infrastructure Follow-up](decisions/01-platform-and-storage.md#d-41) |
| D-42 | <a id="d-42"></a>[AP-06 Observation Precision Decision](decisions/02-intake-and-grouping.md#d-42) |
| D-43 | <a id="d-43"></a>[AP-06 Transmission/Message Architecture Decision](decisions/02-intake-and-grouping.md#d-43) |
| D-44 | <a id="d-44"></a>[AP-06 Unicode Payload Identity Decision](decisions/02-intake-and-grouping.md#d-44) |
| D-45 | <a id="d-45"></a>[AP-06 Gateway Transport Security Decision](decisions/02-intake-and-grouping.md#d-45) |
| D-46 | <a id="d-46"></a>[AP-06 Grouping Window Decision](decisions/02-intake-and-grouping.md#d-46) |
| D-47 | <a id="d-47"></a>[AP-06 Gateway Credential Decision](decisions/06-identity-and-authorization.md#d-47) |
| D-48 | <a id="d-48"></a>[AP-06 Gateway Credential Lifecycle Decision](decisions/06-identity-and-authorization.md#d-48) |
| D-49 | <a id="d-49"></a>[AP-06 Grouping Window Precision Decision](decisions/02-intake-and-grouping.md#d-49) |
| D-50 | <a id="d-50"></a>[AP-06 Grouping Validation Timing](decisions/02-intake-and-grouping.md#d-50) |
| D-51 | <a id="d-51"></a>[AP-06 Grouping Window JSON Integer Limit Decision](decisions/02-intake-and-grouping.md#d-51) |
| D-52 | <a id="d-52"></a>[Application-wide HTTP Support Decision](decisions/08-operations-and-http.md#d-52) |
| D-53 | <a id="d-53"></a>[AP-06 Transmission Target Reference Decision](decisions/02-intake-and-grouping.md#d-53) |
| D-54 | <a id="d-54"></a>[Local Username Equality Decision](decisions/06-identity-and-authorization.md#d-54) |
| D-55 | <a id="d-55"></a>[Detail Field Authorization Decision](decisions/06-identity-and-authorization.md#d-55) |
| D-56 | <a id="d-56"></a>[Identity Fact Append Decision](decisions/06-identity-and-authorization.md#d-56) |
| D-57 | <a id="d-57"></a>[HTTP Credential Selection Decision](decisions/06-identity-and-authorization.md#d-57) |
| D-58 | <a id="d-58"></a>[Ordinary Login Decision](decisions/06-identity-and-authorization.md#d-58) |
| D-59 | <a id="d-59"></a>[Identity Prerequisite Scope Decision](decisions/06-identity-and-authorization.md#d-59) |
| D-60 | <a id="d-60"></a>[Early Master-data and Matcher Prerequisite Proposal](decisions/03-assignment-and-correction.md#d-60) |
| D-61 | <a id="d-61"></a>[Processing Budget Feasibility Proposal](decisions/04-processing-and-plugins.md#d-61) |
| D-62 | <a id="d-62"></a>[Core Regex Runtime Selection Proposal](decisions/04-processing-and-plugins.md#d-62) |
| D-63 | <a id="d-63"></a>[Operator Start and Bundled Dependency Clarification](decisions/01-platform-and-storage.md#d-63) |
| D-64 | <a id="d-64"></a>[Bundled Child Feasibility Proposal](decisions/01-platform-and-storage.md#d-64) |
| D-65 | <a id="d-65"></a>[Native Plugin Alternative Review](decisions/04-processing-and-plugins.md#d-65) |
| D-66 | <a id="d-66"></a>[T043 Shared Predicate Binding Proposal](decisions/04-processing-and-plugins.md#d-66) |
| D-67 | <a id="d-67"></a>[Narrow Installed Processing ABI Proposal](decisions/04-processing-and-plugins.md#d-67) |
| D-68 | <a id="d-68"></a>[Field Visibility Grouping Direction](decisions/06-identity-and-authorization.md#d-68) |
| D-69 | <a id="d-69"></a>[Unscoped Field Visibility Clarification](decisions/06-identity-and-authorization.md#d-69) |
| D-70 | <a id="d-70"></a>[Unscoped Identity Replacement Binding Proposal](decisions/06-identity-and-authorization.md#d-70) |
| D-71 | <a id="d-71"></a>[Internal Execution-principal Authority Adapter](decisions/06-identity-and-authorization.md#d-71) |
| D-72 | <a id="d-72"></a>[Multi-target Workflow Context Binding](decisions/04-processing-and-plugins.md#d-72) |
| D-73 | <a id="d-73"></a>[Installed Processing Artifact Load Bound](decisions/04-processing-and-plugins.md#d-73) |
| D-74 | <a id="d-74"></a>[Processing Internal Retry Jitter Bound](decisions/04-processing-and-plugins.md#d-74) |
| D-75 | <a id="d-75"></a>[Processing Durable Selection Placement](decisions/04-processing-and-plugins.md#d-75) |
| D-76 | <a id="d-76"></a>[Dedicated Installed Preview Artifact Binding](decisions/04-processing-and-plugins.md#d-76) |
| D-77 | <a id="d-77"></a>[Processing Reselection Identity and Terminal Work](decisions/04-processing-and-plugins.md#d-77) |
| D-78 | <a id="d-78"></a>[Processing Failed-work Retry Action Binding](decisions/04-processing-and-plugins.md#d-78) |
| D-79 | <a id="d-79"></a>[Superseded Work Terminality Interpretation Confirmation](decisions/04-processing-and-plugins.md#d-79) |
| D-80 | <a id="d-80"></a>[Processing Source Context Field-group Membership](decisions/04-processing-and-plugins.md#d-80) |
| D-81 | <a id="d-81"></a>[Installed Result Requirement Interpretation](decisions/04-processing-and-plugins.md#d-81) |
| D-82 | <a id="d-82"></a>[Exhausted Candidate Reselection State](decisions/04-processing-and-plugins.md#d-82) |
| D-83 | <a id="d-83"></a>[Processing Dispatcher Concurrency Bound](decisions/04-processing-and-plugins.md#d-83) |
| D-84 | <a id="d-84"></a>[Processing Dispatcher Idle Wake-up Bound](decisions/04-processing-and-plugins.md#d-84) |
| D-85 | <a id="d-85"></a>[Processing Scheduling Configuration Preparation](decisions/04-processing-and-plugins.md#d-85) |
| D-86 | <a id="d-86"></a>[Processing Cold-start Test Observation](decisions/04-processing-and-plugins.md#d-86) |
| D-87 | <a id="d-87"></a>[AP-12 River Queue Evaluation](decisions/01-platform-and-storage.md#d-87) |
| D-88 | <a id="d-88"></a>[AP-12 Cross-Profile Worker and Pub/Sub Comparison](decisions/05-events-and-delivery.md#d-88) |
| D-89 | <a id="d-89"></a>[Core-Domain Event-Driven Architecture Reassessment](decisions/05-events-and-delivery.md#d-89) |
| D-90 | <a id="d-90"></a>[Core-Domain Event-Driven Architecture Direction and Watermill Prototype](decisions/05-events-and-delivery.md#d-90) |
| D-91 | <a id="d-91"></a>[Watermill Failure and Concurrency Validation Extension](decisions/05-events-and-delivery.md#d-91) |
| D-92 | <a id="d-92"></a>[Isolated SQLite Adapter Renewal Fix Assessment](decisions/05-events-and-delivery.md#d-92) |
| D-93 | <a id="d-93"></a>[Watermill PostgreSQL-first Gate Clarification](decisions/05-events-and-delivery.md#d-93) |
| D-94 | <a id="d-94"></a>[Watermill PostgreSQL Publication Integration Proposal](decisions/05-events-and-delivery.md#d-94) |
| D-95 | <a id="d-95"></a>[AP-06 Session Repercussion Consolidation](decisions/09-process-and-documentation.md#d-95) |
| D-96 | <a id="d-96"></a>[Work-package reslicing](decisions/09-process-and-documentation.md#d-96) |
| D-97 | <a id="d-97"></a>[Ordered work-package numbering](decisions/09-process-and-documentation.md#d-97) |
| D-98 | <a id="d-98"></a>[Coordinated Existing-code Library Reassessment](decisions/09-process-and-documentation.md#d-98) |
| D-99 | <a id="d-99"></a>[Synthetic Preview Schema Completion](decisions/04-processing-and-plugins.md#d-99) |
| D-100 | <a id="d-100"></a>[Processing Instance Read Authorization](decisions/04-processing-and-plugins.md#d-100) |
| D-101 | <a id="d-101"></a>[Processing Artifact-root Configuration](decisions/08-operations-and-http.md#d-101) |
| D-102 | <a id="d-102"></a>[Synthetic Schema Freeze Responses](decisions/04-processing-and-plugins.md#d-102) |
| D-103 | <a id="d-103"></a>[Shared Bytes Binding](decisions/04-processing-and-plugins.md#d-103) |
| D-104 | <a id="d-104"></a>[AP-07 Gateway HTTP Submission boundary decision](decisions/02-intake-and-grouping.md#d-104) |
| D-105 | <a id="d-105"></a>[AP-07 Gateway-state HTTP boundary decision](decisions/02-intake-and-grouping.md#d-105) |
| D-106 | <a id="d-106"></a>[AP-07 Gateway Network configuration HTTP boundary decision](decisions/02-intake-and-grouping.md#d-106) |
| D-107 | <a id="d-107"></a>[AP-07 Submission route decision](decisions/02-intake-and-grouping.md#d-107) |
| D-108 | <a id="d-108"></a>[AP-07 Gateway-state route decision](decisions/02-intake-and-grouping.md#d-108) |
| D-109 | <a id="d-109"></a>[AP-07 Gateway Network configuration route decision](decisions/02-intake-and-grouping.md#d-109) |
| D-110 | <a id="d-110"></a>[AP-07 Gateway Network replacement semantics decision](decisions/02-intake-and-grouping.md#d-110) |
| D-111 | <a id="d-111"></a>[AP-07 Gateway Network replacement authorization decision](decisions/02-intake-and-grouping.md#d-111) |
| D-112 | <a id="d-112"></a>[AP planning and contract decision ownership](decisions/09-process-and-documentation.md#d-112) |
| D-113 | <a id="d-113"></a>[AP-07 Gateway degraded-report decision](decisions/02-intake-and-grouping.md#d-113) |
| D-114 | <a id="d-114"></a>[AP-07 Gateway recovery decision](decisions/02-intake-and-grouping.md#d-114) |
| D-115 | <a id="d-115"></a>[AP-07 Gateway Network replacement request decision](decisions/02-intake-and-grouping.md#d-115) |
| D-116 | <a id="d-116"></a>[AP-07 Gateway state-fact boundary](decisions/02-intake-and-grouping.md#d-116) |
| D-117 | <a id="d-117"></a>[AP-07 Gateway state HTTP response](decisions/02-intake-and-grouping.md#d-117) |
| D-118 | <a id="d-118"></a>[AP-07 Gateway Network HTTP response](decisions/02-intake-and-grouping.md#d-118) |
| D-119 | <a id="d-119"></a>[AP-07 contract freeze proposal](decisions/02-intake-and-grouping.md#d-119) |
| D-120 | <a id="d-120"></a>[AP-07 recovered report idempotency](decisions/02-intake-and-grouping.md#d-120) |
| D-121 | <a id="d-121"></a>[AP-07 Gateway credential-management HTTP addendum](decisions/02-intake-and-grouping.md#d-121) |
| D-122 | <a id="d-122"></a>[AP-07 additive write-scope and execution-binding proposal](decisions/05-events-and-delivery.md#d-122) |
| D-123 | <a id="d-123"></a>[AP-07 selected-Workflow worker lifecycle scope extension](decisions/05-events-and-delivery.md#d-123) |
| D-124 | <a id="d-124"></a>[AP-07 selected-Workflow source and publication scope extension](decisions/05-events-and-delivery.md#d-124) |
| D-125 | <a id="d-125"></a>[AP-08 Unit-template ownership](decisions/03-assignment-and-correction.md#d-125) |
| D-126 | <a id="d-126"></a>[Open-AP self-containment repair](decisions/09-process-and-documentation.md#d-126) |
| D-127 | <a id="d-127"></a>[AP-08 suppression direction](decisions/03-assignment-and-correction.md#d-127) |
| D-128 | <a id="d-128"></a>[Owning-migration consolidation](decisions/01-platform-and-storage.md#d-128) |
| D-129 | <a id="d-129"></a>[In-process Core regex](decisions/04-processing-and-plugins.md#d-129) |
| D-130 | <a id="d-130"></a>[Plugin trust clarification](decisions/04-processing-and-plugins.md#d-130) |
| D-131 | <a id="d-131"></a>[Managed plugin process runtime](decisions/04-processing-and-plugins.md#d-131) |
| D-132 | <a id="d-132"></a>[Documentation and naming separation](decisions/09-process-and-documentation.md#d-132) |
| D-133 | <a id="d-133"></a>[Command-ID reuse conflict code](decisions/08-operations-and-http.md#d-133) |
| D-134 | <a id="d-134"></a>[Review-finding AP planning revision](decisions/09-process-and-documentation.md#d-134) |
| D-135 | <a id="d-135"></a>[Foundation redesign after the 2026-11-06 re-evaluation](decisions/10-foundation-redesign.md#d-135) |
| D-136 | <a id="d-136"></a>[Agent execution rules and boundary crossings](decisions/09-process-and-documentation.md#d-136) |
| D-137 | <a id="d-137"></a>[Processing work record as implemented](decisions/10-foundation-redesign.md#d-137) |
| D-138 | <a id="d-138"></a>[Open-AP boundary repair and server exchange of Units](decisions/09-process-and-documentation.md#d-138) |

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
| REQ-069..071 | Phase-1 proposal |
| REQ-072 | Dropped by [D-135](decisions/10-foundation-redesign.md#d-135) item 15; ID reserved |
| REQ-073..075 | Phase-1 proposal |
| REQ-076 | Dropped by [D-135](decisions/10-foundation-redesign.md#d-135) item 15; ID reserved |
| REQ-077..079 | Phase-1 proposal |
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
| REQ-128 | Phase 1 approved (server-to-server exchange, revised by [D-138](decisions/09-process-and-documentation.md#d-138)); AP-15 |
| REQ-129..130 | Phase-1 proposal |
| REQ-131 | Later |
| REQ-132 | Phase-1 proposal |
| REQ-133/138 | Phase 1 approved ([D-142](decisions/09-process-and-documentation.md#d-142)): personal and global saved views; global views visible to every Principal that may read Messages, results always intersected with current rights; server AP-10, screens AP-15 |
| REQ-134 | Phase 1 approved at the AP-09 start (2026-11-07); server side AP-09, screens AP-15 |
| REQ-135..137 | Acceptance refinements of already approved field/query goals; verify within AP-10 without broadening frozen authorization |
| REQ-139 | Later; the event, Subscription and plugin model keeps reaction compatibility ([D-131](#managed-plugin-process-runtime), [D-135](#foundation-redesign-after-the-2026-11-06-re-evaluation)) |
| REQ-140 | Phase 1 approved ([D-138](decisions/09-process-and-documentation.md#d-138)); AP-15 |
| REQ-141..145 | Phase 1 approved ([D-150](decisions/04-processing-and-plugins.md#d-150), [D-151](decisions/06-identity-and-authorization.md#d-151), [D-152](decisions/07-query-and-live.md#d-152), 2026-11-11): server AP-21 (REQ-141, REQ-143..145) and AP-22 (REQ-142), screens AP-15 |
| [D-30 comparison extension](#d-30) | Later, separately approved slice; not a current contract blocker |

Resolve IDs in the [catalog](requirements-catalog.md), not in a parallel feature summary.

## Open Issues and Risks

P-19 was never assigned. This table owns **remaining gate/status**, not contract semantics. D-126 routes implementation-dependent work to current APs; AP-03's completed disposable probes are evidence, not an unowned prerequisite or a production guarantee. A resolved documentary question is not proof of implementation.

| ID | Remaining gate / disposition | Owner detail | Responsible package |
| --- | --- | --- | --- |
| P-01 | Foundation/intake evidence recorded; broader crash/fault qualification remains AP-16 work | [AP-05](execution/ap-05.md), [AP-07](execution/ap-07.md), [C-12](contracts/c-12-persistence-operations.md) | AP-16 |
| P-02 | Policy resolved by D-13; separate new experiments retain their approval gate | [D-13](#d-13), [approval gates](#workflow-and-approval-gates) | Current experimenting owner, only if assigned |
| P-03 | Foundation evidence recorded; actual retention/operational evidence remains | [C-12](contracts/c-12-persistence-operations.md) | AP-16 |
| P-04 | Full topology/failover evidence remains; Delivery-specific fencing is AP-11 work | [C-08](contracts/c-08-events-jobs.md), [C-12](contracts/c-12-persistence-operations.md) | AP-11 / AP-16 |
| P-05 | Plugin runtime and protocol selected and implemented for installed processing under [D-131](#managed-plugin-process-runtime); process lifecycle/pooling, Delivery/reaction capabilities and non-Go plugin proof remain host planning | [C-10](contracts/c-10-plugin-host.md) | AP-12 |
| P-06 | General host lifecycle/configuration enforcement evidence remains | [C-10](contracts/c-10-plugin-host.md) | AP-12 |
| P-07 | Current target/host capability, I/O/cancellation evidence remains | [C-09](contracts/c-09-delivery.md), [C-10](contracts/c-10-plugin-host.md) | AP-12 |
| P-08 | Observation/Grouping decisions remain fixed; current intake concurrency/publication evidence is recorded, not an old AP-06 T024 prerequisite | [D-42](#ap-06-observation-precision-decision), [D-49](#ap-06-grouping-window-precision-decision), [AP-07 evidence](execution/ap-07.md) | AP-07 evidence; AP-17 coverage verification |
| P-09 | Logical-model question resolved by D-29/D-30; physical design at owner gates, not a diagram freeze | [D-29](#d-29), [C-05](contracts/c-05-matching-correction.md) | AP-03 / AP-04 / affected implementation owner |
| P-10 | Query/live projection/revocation enforcement remains current consumer work | [C-06](contracts/c-06-identity-authorization.md), [C-07](contracts/c-07-query-live.md) | AP-10 |
| P-11 | Claims/materialization/attempts and real host/provider evidence remain | [C-08](contracts/c-08-events-jobs.md), [C-09](contracts/c-09-delivery.md), [C-10](contracts/c-10-plugin-host.md) | AP-11 / AP-12 |
| P-12 | Historical correction planning and PostgreSQL concurrency/publication evidence recorded in [AP-08](execution/ap-08.md#handover) | [C-05](contracts/c-05-matching-correction.md) | AP-08 |
| P-13 | Master-data/default/template planning and implementation recorded in [AP-08](execution/ap-08.md#handover); template actions consumed by AP-14 | [C-04](contracts/c-04-master-data.md) | AP-08 |
| P-14 | Full account cleanup planning/participants and lifecycle evidence remain | [C-13](contracts/c-13-account-lifecycle.md) | AP-14 |
| P-15 | Query Unicode/index evidence remains; deferred backend runtime parity is not required now | [C-07](contracts/c-07-query-live.md) | AP-10 |
| P-16 | Shared renderer selection belongs to Delivery; GUI/historical work to presentation; frontend editor to browser AP | [C-11](contracts/c-11-processing-presentation.md) | AP-11 / AP-13 / AP-15 |
| P-17 | External identity deliberately later, not a current implementation blocker | [C-06](contracts/c-06-identity-authorization.md), [C-13](contracts/c-13-account-lifecycle.md) | Later separately approved slice |
| P-18 | Approved delivery scope remains; each current AP owns its unfixed supporting contract planning under D-126 | [Delivery scope](#delivery-scope), [approval gates](#workflow-and-approval-gates) | Current producer AP; AP-17 verifies |
| P-20 | Actual Full operations qualification remains | [C-12](contracts/c-12-persistence-operations.md) | AP-16 |
| P-21 | Record actual disposable evidence environment; no unsupported hardware/capacity certification required | [Operating assumptions](#operating-assumptions), [C-12](contracts/c-12-persistence-operations.md) | AP-16 |

## Workflow and Approval Gates

There is one approval gate: **the user approves a work package by ordering its execution.** Read this section for current authority; consult the [ADR index](decisions/README.md) only to verify a named decision.

| Activity | Required authority |
| --- | --- |
| Starting a work package | The user's order to execute that package, which approves its assignment, approved delivery scope and fixed required inputs. This is the only gate; no separate confirmation. |
| Planning and contract details inside the package | Included in the approved package; no further owner sign-off, freeze permission or intermediate approval |
| Fixed behavior or outside-scope change | User approval of the change, recorded as an ADR with a D-ID before the affected implementation; procedure in the [scope-change rules](work-packages/shared-rules.md#scope-change) |
| External effects, experiments, parallel work | Explicit approved environment or slice, or disjoint scope; the package's own acceptance tests remain assigned work |
| Documentary maintenance | User-authorized direct maintenance; no product execution |

Delivery classification is only in [Delivery Scope](#delivery-scope). Results are only in producer execution records. Approval does not establish implementation or successful tests; completion evidence stays mandatory (see [shared rules](work-packages/shared-rules.md)).

## Moved Sections

The owner sections that used to follow the register were condensed into the ADRs. These anchors keep earlier links to this page resolving.

| Former section | ADR |
| --- | --- |
| <a id="ap-04-scope-decision"></a>AP-04 Scope Decision | [D-32](decisions/01-platform-and-storage.md#d-32), [D-33](decisions/04-processing-and-plugins.md#d-33) |
| <a id="ap-04-flex-decision"></a>AP-04 FLEX Decision | [D-34](decisions/02-intake-and-grouping.md#d-34) |
| <a id="ap-04-foundation-freeze-decision"></a>AP-04 Foundation Freeze Decision | [D-35](decisions/01-platform-and-storage.md#d-35) |
| <a id="repository-wide-library-policy-decision"></a>Repository-wide Library Policy Decision | [D-36](decisions/01-platform-and-storage.md#d-36) |
| <a id="ap-05-migration-revision-decision"></a>AP-05 Migration Revision Decision | [D-37](decisions/01-platform-and-storage.md#d-37) |
| <a id="ap-05-data-access-decision"></a>AP-05 Data Access Decision | [D-38](decisions/01-platform-and-storage.md#d-38) |
| <a id="c-03-reassessment-decision"></a>C-03 Reassessment Decision | [D-39](decisions/03-assignment-and-correction.md#d-39) |
| <a id="transmission-and-message-architecture-reassessment"></a>Transmission and Message Architecture Reassessment | [D-40](decisions/02-intake-and-grouping.md#d-40) |
| <a id="repository-structure-and-test-infrastructure-follow-up"></a>Repository Structure and Test Infrastructure Follow-up | [D-41](decisions/01-platform-and-storage.md#d-41) |
| <a id="ap-06-observation-precision-decision"></a>AP-06 Observation Precision Decision | [D-42](decisions/02-intake-and-grouping.md#d-42) |
| <a id="ap-06-transmissionmessage-architecture-decision"></a>AP-06 Transmission/Message Architecture Decision | [D-43](decisions/02-intake-and-grouping.md#d-43) |
| <a id="ap-06-unicode-payload-identity-decision"></a>AP-06 Unicode Payload Identity Decision | [D-44](decisions/02-intake-and-grouping.md#d-44) |
| <a id="ap-06-gateway-transport-security-decision"></a>AP-06 Gateway Transport Security Decision | [D-45](decisions/02-intake-and-grouping.md#d-45) |
| <a id="ap-06-grouping-window-decision"></a>AP-06 Grouping Window Decision | [D-46](decisions/02-intake-and-grouping.md#d-46) |
| <a id="ap-06-gateway-credential-decision"></a>AP-06 Gateway Credential Decision | [D-47](decisions/06-identity-and-authorization.md#d-47) |
| <a id="ap-06-gateway-credential-lifecycle-decision"></a>AP-06 Gateway Credential Lifecycle Decision | [D-48](decisions/06-identity-and-authorization.md#d-48) |
| <a id="ap-06-grouping-window-precision-decision"></a>AP-06 Grouping Window Precision Decision | [D-49](decisions/02-intake-and-grouping.md#d-49) |
| <a id="ap-06-grouping-validation-timing"></a>AP-06 Grouping Validation Timing | [D-50](decisions/02-intake-and-grouping.md#d-50) |
| <a id="ap-06-grouping-window-json-integer-limit-decision"></a>AP-06 Grouping Window JSON Integer Limit Decision | [D-51](decisions/02-intake-and-grouping.md#d-51) |
| <a id="application-wide-http-support-decision"></a>Application-wide HTTP Support Decision | [D-52](decisions/08-operations-and-http.md#d-52) |
| <a id="ap-06-transmission-target-reference-decision"></a>AP-06 Transmission Target Reference Decision | [D-53](decisions/02-intake-and-grouping.md#d-53) |
| <a id="local-username-equality-decision"></a>Local Username Equality Decision | [D-54](decisions/06-identity-and-authorization.md#d-54) |
| <a id="detail-field-authorization-decision"></a>Detail Field Authorization Decision | [D-55](decisions/06-identity-and-authorization.md#d-55) |
| <a id="identity-fact-append-decision"></a>Identity Fact Append Decision | [D-56](decisions/06-identity-and-authorization.md#d-56) |
| <a id="http-credential-selection-decision"></a>HTTP Credential Selection Decision | [D-57](decisions/06-identity-and-authorization.md#d-57) |
| <a id="ordinary-login-decision"></a>Ordinary Login Decision | [D-58](decisions/06-identity-and-authorization.md#d-58) |
| <a id="identity-prerequisite-scope-decision"></a>Identity Prerequisite Scope Decision | [D-59](decisions/06-identity-and-authorization.md#d-59) |
| <a id="early-master-data-and-matcher-prerequisite-proposal"></a>Early Master-data and Matcher Prerequisite Proposal | [D-60](decisions/03-assignment-and-correction.md#d-60) |
| <a id="processing-budget-feasibility-proposal"></a>Processing Budget Feasibility Proposal | [D-61](decisions/04-processing-and-plugins.md#d-61) |
| <a id="core-regex-runtime-selection-proposal"></a>Core Regex Runtime Selection Proposal | [D-62](decisions/04-processing-and-plugins.md#d-62) |
| <a id="operator-start-and-bundled-dependency-clarification"></a>Operator Start and Bundled Dependency Clarification | [D-63](decisions/01-platform-and-storage.md#d-63) |
| <a id="bundled-child-feasibility-proposal"></a>Bundled Child Feasibility Proposal | [D-64](decisions/01-platform-and-storage.md#d-64) |
| <a id="native-plugin-alternative-review"></a>Native Plugin Alternative Review | [D-65](decisions/04-processing-and-plugins.md#d-65) |
| <a id="t043-shared-predicate-binding-proposal"></a>T043 Shared Predicate Binding Proposal | [D-66](decisions/04-processing-and-plugins.md#d-66) |
| <a id="internal-execution-principal-authority-adapter"></a>Internal Execution-principal Authority Adapter | [D-71](decisions/06-identity-and-authorization.md#d-71) |
| <a id="multi-target-workflow-context-binding"></a>Multi-target Workflow Context Binding | [D-72](decisions/04-processing-and-plugins.md#d-72) |
| <a id="installed-processing-artifact-load-bound"></a>Installed Processing Artifact Load Bound | [D-73](decisions/04-processing-and-plugins.md#d-73) |
| <a id="processing-internal-retry-jitter-bound"></a>Processing Internal Retry Jitter Bound | [D-74](decisions/04-processing-and-plugins.md#d-74) |
| <a id="processing-durable-selection-placement"></a>Processing Durable Selection Placement | [D-75](decisions/04-processing-and-plugins.md#d-75) |
| <a id="dedicated-installed-preview-artifact-binding"></a>Dedicated Installed Preview Artifact Binding | [D-76](decisions/04-processing-and-plugins.md#d-76) |
| <a id="processing-reselection-identity-and-terminal-work"></a>Processing Reselection Identity and Terminal Work | [D-77](decisions/04-processing-and-plugins.md#d-77) |
| <a id="processing-failed-work-retry-action-binding"></a>Processing Failed-work Retry Action Binding | [D-78](decisions/04-processing-and-plugins.md#d-78) |
| <a id="superseded-work-terminality-interpretation-confirmation"></a>Superseded Work Terminality Interpretation Confirmation | [D-79](decisions/04-processing-and-plugins.md#d-79) |
| <a id="processing-source-context-field-group-membership"></a>Processing Source Context Field-group Membership | [D-80](decisions/04-processing-and-plugins.md#d-80) |
| <a id="installed-result-requirement-interpretation"></a>Installed Result Requirement Interpretation | [D-81](decisions/04-processing-and-plugins.md#d-81) |
| <a id="exhausted-candidate-reselection-state"></a>Exhausted Candidate Reselection State | [D-82](decisions/04-processing-and-plugins.md#d-82) |
| <a id="processing-dispatcher-concurrency-bound"></a>Processing Dispatcher Concurrency Bound | [D-83](decisions/04-processing-and-plugins.md#d-83) |
| <a id="processing-dispatcher-idle-wake-up-bound"></a>Processing Dispatcher Idle Wake-up Bound | [D-84](decisions/04-processing-and-plugins.md#d-84) |
| <a id="processing-scheduling-configuration-preparation"></a>Processing Scheduling Configuration Preparation | [D-85](decisions/04-processing-and-plugins.md#d-85) |
| <a id="processing-cold-start-test-observation"></a>Processing Cold-start Test Observation | [D-86](decisions/04-processing-and-plugins.md#d-86) |
| <a id="ap-12-river-queue-evaluation"></a>AP-12 River Queue Evaluation | [D-87](decisions/01-platform-and-storage.md#d-87) |
| <a id="ap-12-cross-profile-worker-and-pubsub-comparison"></a>AP-12 Cross-Profile Worker and Pub/Sub Comparison | [D-88](decisions/05-events-and-delivery.md#d-88) |
| <a id="core-domain-event-driven-architecture-reassessment"></a>Core-Domain Event-Driven Architecture Reassessment | [D-89](decisions/05-events-and-delivery.md#d-89) |
| <a id="core-domain-event-driven-architecture-direction-and-watermill-prototype"></a>Core-Domain Event-Driven Architecture Direction and Watermill Prototype | [D-90](decisions/05-events-and-delivery.md#d-90) |
| <a id="watermill-failure-and-concurrency-validation-extension"></a>Watermill Failure and Concurrency Validation Extension | [D-91](decisions/05-events-and-delivery.md#d-91) |
| <a id="isolated-sqlite-adapter-renewal-fix-assessment"></a>Isolated SQLite Adapter Renewal Fix Assessment | [D-92](decisions/05-events-and-delivery.md#d-92) |
| <a id="watermill-postgresql-first-gate-clarification"></a>Watermill PostgreSQL-first Gate Clarification | [D-93](decisions/05-events-and-delivery.md#d-93) |
| <a id="watermill-postgresql-publication-integration-proposal"></a>Watermill PostgreSQL Publication Integration Proposal | [D-94](decisions/05-events-and-delivery.md#d-94) |
| <a id="ap-06-session-repercussion-consolidation"></a>AP-06 Session Repercussion Consolidation | [D-95](decisions/09-process-and-documentation.md#d-95) |
| <a id="work-package-reslicing-decision"></a>Work-Package Reslicing Decision | [D-96](decisions/09-process-and-documentation.md#d-96) |
| <a id="ordered-ap-renumbering-decision"></a>Ordered AP Renumbering Decision | [D-97](decisions/09-process-and-documentation.md#d-97) |
| <a id="narrow-installed-processing-abi-proposal"></a>Narrow Installed Processing ABI Proposal | [D-67](decisions/04-processing-and-plugins.md#d-67) |
| <a id="field-visibility-grouping-direction"></a>Field Visibility Grouping Direction | [D-68](decisions/06-identity-and-authorization.md#d-68) |
| <a id="unscoped-field-visibility-clarification"></a>Unscoped Field Visibility Clarification | [D-69](decisions/06-identity-and-authorization.md#d-69) |
| <a id="unscoped-identity-replacement-binding-proposal"></a>Unscoped Identity Replacement Binding Proposal | [D-70](decisions/06-identity-and-authorization.md#d-70) |
| <a id="coordinated-existing-code-library-reassessment"></a>Coordinated Existing-code Library Reassessment | [D-98](decisions/09-process-and-documentation.md#d-98) |
| <a id="synthetic-preview-schema-completion"></a>Synthetic Preview Schema Completion | [D-99](decisions/04-processing-and-plugins.md#d-99) |
| <a id="processing-instance-read-authorization"></a>Processing Instance Read Authorization | [D-100](decisions/04-processing-and-plugins.md#d-100) |
| <a id="processing-artifact-root-configuration"></a>Processing Artifact-root Configuration | [D-101](decisions/08-operations-and-http.md#d-101) |
| <a id="synthetic-schema-freeze-responses"></a>Synthetic Schema Freeze Responses | [D-102](decisions/04-processing-and-plugins.md#d-102) |
| <a id="shared-bytes-binding"></a>Shared Bytes Binding | [D-103](decisions/04-processing-and-plugins.md#d-103) |
| <a id="ap-07-gateway-http-submission-boundary-decision"></a>AP-07 Gateway HTTP Submission boundary decision | [D-104](decisions/02-intake-and-grouping.md#d-104) |
| <a id="ap-07-gateway-state-http-boundary-decision"></a>AP-07 Gateway-state HTTP boundary decision | [D-105](decisions/02-intake-and-grouping.md#d-105) |
| <a id="ap-07-gateway-network-configuration-http-boundary-decision"></a>AP-07 Gateway Network configuration HTTP boundary decision | [D-106](decisions/02-intake-and-grouping.md#d-106) |
| <a id="ap-07-submission-route-decision"></a>AP-07 Submission route decision | [D-107](decisions/02-intake-and-grouping.md#d-107) |
| <a id="ap-07-gateway-state-route-decision"></a>AP-07 Gateway-state route decision | [D-108](decisions/02-intake-and-grouping.md#d-108) |
| <a id="ap-07-gateway-network-configuration-route-decision"></a>AP-07 Gateway Network configuration route decision | [D-109](decisions/02-intake-and-grouping.md#d-109) |
| <a id="ap-07-gateway-network-replacement-semantics-decision"></a>AP-07 Gateway Network replacement semantics decision | [D-110](decisions/02-intake-and-grouping.md#d-110) |
| <a id="ap-07-gateway-network-replacement-authorization-decision"></a>AP-07 Gateway Network replacement authorization decision | [D-111](decisions/02-intake-and-grouping.md#d-111) |
| <a id="ap-planning-and-contract-decision-ownership"></a>AP planning and contract decision ownership | [D-112](decisions/09-process-and-documentation.md#d-112) |
| <a id="ap-07-gateway-degraded-report-decision"></a>AP-07 Gateway degraded-report decision | [D-113](decisions/02-intake-and-grouping.md#d-113) |
| <a id="ap-07-gateway-recovery-decision"></a>AP-07 Gateway recovery decision | [D-114](decisions/02-intake-and-grouping.md#d-114) |
| <a id="ap-07-gateway-network-replacement-request-decision"></a>AP-07 Gateway Network replacement request decision | [D-115](decisions/02-intake-and-grouping.md#d-115) |
| <a id="ap-07-gateway-state-fact-boundary"></a>AP-07 Gateway state-fact boundary | [D-116](decisions/02-intake-and-grouping.md#d-116) |
| <a id="ap-07-gateway-state-http-response"></a>AP-07 Gateway state HTTP response | [D-117](decisions/02-intake-and-grouping.md#d-117) |
| <a id="ap-07-gateway-network-http-response"></a>AP-07 Gateway Network HTTP response | [D-118](decisions/02-intake-and-grouping.md#d-118) |
| <a id="ap-07-contract-freeze-proposal"></a>AP-07 contract freeze proposal | [D-119](decisions/02-intake-and-grouping.md#d-119) |
| <a id="ap-07-recovered-report-idempotency"></a>AP-07 recovered report idempotency | [D-120](decisions/02-intake-and-grouping.md#d-120) |
| <a id="ap-07-gateway-credential-management-http-addendum"></a>AP-07 Gateway credential-management HTTP addendum | [D-121](decisions/02-intake-and-grouping.md#d-121) |
| <a id="ap-07-additive-write-scope-and-execution-binding-proposal"></a>AP-07 additive write-scope and execution-binding proposal | [D-122](decisions/05-events-and-delivery.md#d-122) |
| <a id="ap-07-selected-workflow-worker-lifecycle-scope-extension"></a>AP-07 selected-Workflow worker lifecycle scope extension | [D-123](decisions/05-events-and-delivery.md#d-123) |
| <a id="ap-07-selected-workflow-source-and-publication-scope-extension"></a>AP-07 selected-Workflow source and publication scope extension | [D-124](decisions/05-events-and-delivery.md#d-124) |
| <a id="ap-08-unit-template-ownership"></a>AP-08 Unit-template ownership | [D-125](decisions/03-assignment-and-correction.md#d-125) |
| <a id="open-ap-self-containment-repair"></a>Open-AP self-containment repair | [D-126](decisions/09-process-and-documentation.md#d-126) |
| <a id="ap-08-suppression-direction"></a>AP-08 suppression direction | [D-127](decisions/03-assignment-and-correction.md#d-127) |
| <a id="owning-migration-consolidation"></a>Owning-migration consolidation | [D-128](decisions/01-platform-and-storage.md#d-128) |
| <a id="in-process-core-regex"></a>In-process Core regex | [D-129](decisions/04-processing-and-plugins.md#d-129) |
| <a id="plugin-trust-clarification"></a>Plugin trust clarification | [D-130](decisions/04-processing-and-plugins.md#d-130) |
| <a id="managed-plugin-process-runtime"></a>Managed plugin process runtime | [D-131](decisions/04-processing-and-plugins.md#d-131) |
| <a id="documentation-and-naming-separation"></a>Documentation and naming separation | [D-132](decisions/09-process-and-documentation.md#d-132) |
| <a id="command-id-reuse-conflict-code"></a>Command-ID reuse conflict code | [D-133](decisions/08-operations-and-http.md#d-133) |
| <a id="review-finding-ap-planning-revision"></a>Review-finding AP planning revision | [D-134](decisions/09-process-and-documentation.md#d-134) |
| <a id="foundation-redesign-after-the-2026-11-06-re-evaluation"></a>Foundation redesign after the 2026-11-06 re-evaluation | [D-135](decisions/10-foundation-redesign.md#d-135) |
