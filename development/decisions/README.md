# Decision Records

Architecture decision records (ADRs) of GoPager, grouped by topic. Each decision keeps its stable `D-nn` ID; the ID is the only reference form used in contracts, execution records and `docs/`. Status values: **accepted**, **superseded by D-nnn** (a trailing note names the part that stands or the replacement), **withdrawn** (none so far). Product goal, delivery scope and the approval rule are in the [authoritative plan](../authoritative-plan.md).

Add a decision to the ADR file of its topic with status, date, supersession, the decision in one to five sentences and its consequences, then add a row below. A superseded decision keeps its text; set its status and name the superseding decision in both records. The approval narrative of the decisions before D-135 is not kept in the ADRs; the repository history of `development/authoritative-plan.md` retains it.

## ADR files

| File | Topic | Decisions |
| --- | --- | --- |
| [01-platform-and-storage.md](01-platform-and-storage.md) | Platform and storage | 19 (D-01..D-128) |
| [02-intake-and-grouping.md](02-intake-and-grouping.md) | Intake and Grouping | 34 (D-17..D-139) |
| [03-assignment-and-correction.md](03-assignment-and-correction.md) | Assignment and correction | 10 (D-08..D-145) |
| [04-processing-and-plugins.md](04-processing-and-plugins.md) | Processing and plugins | 36 (D-05..D-159) |
| [05-events-and-delivery.md](05-events-and-delivery.md) | Events and Delivery | 15 (D-06..D-158) |
| [06-identity-and-authorization.md](06-identity-and-authorization.md) | Identity and authorization | 18 (D-07..D-151) |
| [07-query-and-live.md](07-query-and-live.md) | Query and live | 4 (D-16..D-152) |
| [08-operations-and-http.md](08-operations-and-http.md) | Operations and HTTP | 3 (D-52..D-133) |
| [09-process-and-documentation.md](09-process-and-documentation.md) | Process and documentation | 14 (D-19..D-142) |
| [10-foundation-redesign.md](10-foundation-redesign.md) | Foundation redesign | 2 (D-135..D-137) |

## Index

| ID | Title | Status | Date | Record |
| --- | --- | --- | --- | --- |
| D-01 | Confirmed | accepted | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-01) |
| D-02 | Technology direction | accepted | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-02) |
| D-03 | Shared Lite/Full application direction | superseded by D-135 | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-03) |
| D-04 | Full authority/HA boundary | accepted | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-04) |
| D-05 | Extension distribution/SDK boundary | accepted | 2026-10-25 | [04-processing-and-plugins](04-processing-and-plugins.md#d-05) |
| D-06 | Initial Delivery target selection and subscription boundary | accepted | 2026-10-25 | [05-events-and-delivery](05-events-and-delivery.md#d-06) |
| D-07 | Permission model | accepted | 2026-10-25 | [06-identity-and-authorization](06-identity-and-authorization.md#d-07) |
| D-08 | Unit organization and filter correlation | accepted | 2026-10-25 | [03-assignment-and-correction](03-assignment-and-correction.md#d-08) |
| D-09 | Temporal assignment/deletion effects | accepted | 2026-10-25 | [03-assignment-and-correction](03-assignment-and-correction.md#d-09) |
| D-10 | Suppression boundary | accepted | 2026-10-25 | [03-assignment-and-correction](03-assignment-and-correction.md#d-10) |
| D-11 | Identity/credential separation and later external-identity boundary | accepted | 2026-10-25 | [06-identity-and-authorization](06-identity-and-authorization.md#d-11) |
| D-12 | Processing minimum and extensibility constraints | accepted | 2026-10-25 | [04-processing-and-plugins](04-processing-and-plugins.md#d-12) |
| D-13 | Measurement/acceptance policy | accepted | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-13) |
| D-14 | Conditional Lite durability allowance | superseded by D-135 | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-14) |
| D-15 | Retention default and exhaustion behavior | accepted | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-15) |
| D-16 | Search semantics | accepted | 2026-10-25 | [07-query-and-live](07-query-and-live.md#d-16) |
| D-17 | Grouping policy | accepted (amended by D-30) | 2026-10-25 | [02-intake-and-grouping](02-intake-and-grouping.md#d-17) |
| D-18 | Protocol-scope amendment | accepted | 2026-10-25 | [02-intake-and-grouping](02-intake-and-grouping.md#d-18) |
| D-19 | Confirmed | accepted | 2026-10-25 | [09-process-and-documentation](09-process-and-documentation.md#d-19) |
| D-20 | Language/i18n direction | accepted | 2026-10-25 | [09-process-and-documentation](09-process-and-documentation.md#d-20) |
| D-21 | Naming authority | accepted | 2026-10-25 | [09-process-and-documentation](09-process-and-documentation.md#d-21) |
| D-22 | Internal-time separation | superseded by D-135 | 2026-10-25 | [07-query-and-live](07-query-and-live.md#d-22) |
| D-23 | Retention/independent-work boundary | accepted | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-23) |
| D-24 | Plugin trust/capability model | superseded by D-130 and D-135 | 2026-10-25 | [04-processing-and-plugins](04-processing-and-plugins.md#d-24) |
| D-25 | Ingest publication and presentation boundary | superseded by D-135 | 2026-10-25 | [04-processing-and-plugins](04-processing-and-plugins.md#d-25) |
| D-26 | Deletion model | accepted | 2026-10-25 | [06-identity-and-authorization](06-identity-and-authorization.md#d-26) |
| D-27 | Storage model superseded by D-29 | superseded by D-29 | 2026-10-25 | [02-intake-and-grouping](02-intake-and-grouping.md#d-27) |
| D-28 | Query-time model | superseded by D-135 | 2026-10-25 | [07-query-and-live](07-query-and-live.md#d-28) |
| D-29 | Logical model replacing D-27 | accepted (amended by D-30) | 2026-10-25 | [02-intake-and-grouping](02-intake-and-grouping.md#d-29) |
| D-30 | Heuristic delivery deferred | accepted | 2026-10-25 | [02-intake-and-grouping](02-intake-and-grouping.md#d-30) |
| D-31 | Application Backup/Restore removed | superseded by D-135 | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-31) |
| D-32 | AP-04 Scope Decision | superseded by D-135 | 2026-10-25 | [01-platform-and-storage](01-platform-and-storage.md#d-32) |
| D-33 | AP-04 Scope Decision | accepted | 2026-10-25 | [04-processing-and-plugins](04-processing-and-plugins.md#d-33) |
| D-34 | AP-04 FLEX Decision | accepted | 2026-10-25 | [02-intake-and-grouping](02-intake-and-grouping.md#d-34) |
| D-35 | AP-04 Foundation Freeze Decision | accepted | 2026-10-26 | [01-platform-and-storage](01-platform-and-storage.md#d-35) |
| D-36 | Repository-wide Library Policy Decision | accepted | 2026-10-26 | [01-platform-and-storage](01-platform-and-storage.md#d-36) |
| D-37 | AP-05 Migration Revision Decision | accepted | 2026-10-26 | [01-platform-and-storage](01-platform-and-storage.md#d-37) |
| D-38 | AP-05 Data Access Decision | accepted | 2026-10-26 | [01-platform-and-storage](01-platform-and-storage.md#d-38) |
| D-39 | C-03 Reassessment Decision | superseded by D-40 | 2026-10-26 | [03-assignment-and-correction](03-assignment-and-correction.md#d-39) |
| D-40 | Transmission and Message Architecture Reassessment | accepted | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-40) |
| D-41 | Repository Structure and Test Infrastructure Follow-up | accepted | 2026-10-26 | [01-platform-and-storage](01-platform-and-storage.md#d-41) |
| D-42 | AP-06 Observation Precision Decision | accepted | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-42) |
| D-43 | AP-06 Transmission/Message Architecture Decision | accepted | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-43) |
| D-44 | AP-06 Unicode Payload Identity Decision | accepted | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-44) |
| D-45 | AP-06 Gateway Transport Security Decision | accepted (extended by D-52) | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-45) |
| D-46 | AP-06 Grouping Window Decision | superseded by D-51 | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-46) |
| D-47 | AP-06 Gateway Credential Decision | accepted | 2026-10-26 | [06-identity-and-authorization](06-identity-and-authorization.md#d-47) |
| D-48 | AP-06 Gateway Credential Lifecycle Decision | accepted | 2026-10-26 | [06-identity-and-authorization](06-identity-and-authorization.md#d-48) |
| D-49 | AP-06 Grouping Window Precision Decision | accepted | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-49) |
| D-50 | AP-06 Grouping Validation Timing | accepted | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-50) |
| D-51 | AP-06 Grouping Window JSON Integer Limit Decision | accepted | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-51) |
| D-52 | Application-wide HTTP Support Decision | accepted | 2026-10-26 | [08-operations-and-http](08-operations-and-http.md#d-52) |
| D-53 | AP-06 Transmission Target Reference Decision | accepted | 2026-10-26 | [02-intake-and-grouping](02-intake-and-grouping.md#d-53) |
| D-54 | Local Username Equality Decision | accepted | 2026-10-26 | [06-identity-and-authorization](06-identity-and-authorization.md#d-54) |
| D-55 | Detail Field Authorization Decision | accepted | 2026-10-26 | [06-identity-and-authorization](06-identity-and-authorization.md#d-55) |
| D-56 | Identity Fact Append Decision | accepted | 2026-10-26 | [06-identity-and-authorization](06-identity-and-authorization.md#d-56) |
| D-57 | HTTP Credential Selection Decision | accepted | 2026-10-26 | [06-identity-and-authorization](06-identity-and-authorization.md#d-57) |
| D-58 | Ordinary Login Decision | accepted | 2026-10-27 | [06-identity-and-authorization](06-identity-and-authorization.md#d-58) |
| D-59 | Identity Prerequisite Scope Decision | accepted | 2026-10-27 | [06-identity-and-authorization](06-identity-and-authorization.md#d-59) |
| D-60 | Early Master-data and Matcher Prerequisite Proposal | accepted | 2026-10-27 | [03-assignment-and-correction](03-assignment-and-correction.md#d-60) |
| D-61 | Processing Budget Feasibility Proposal | accepted (evidence only) | 2026-10-28 | [04-processing-and-plugins](04-processing-and-plugins.md#d-61) |
| D-62 | Core Regex Runtime Selection Proposal | superseded by D-129 | 2026-10-28 | [04-processing-and-plugins](04-processing-and-plugins.md#d-62) |
| D-63 | Operator Start and Bundled Dependency Clarification | accepted | 2026-11-01 | [01-platform-and-storage](01-platform-and-storage.md#d-63) |
| D-64 | Bundled Child Feasibility Proposal | accepted (evidence only) | 2026-11-01 | [01-platform-and-storage](01-platform-and-storage.md#d-64) |
| D-65 | Native Plugin Alternative Review | superseded by D-131 | 2026-11-01 | [04-processing-and-plugins](04-processing-and-plugins.md#d-65) |
| D-66 | T043 Shared Predicate Binding Proposal | accepted | 2026-11-01 | [04-processing-and-plugins](04-processing-and-plugins.md#d-66) |
| D-67 | Narrow Installed Processing ABI Proposal | superseded by D-131 | 2026-11-01 | [04-processing-and-plugins](04-processing-and-plugins.md#d-67) |
| D-68 | Field Visibility Grouping Direction | accepted | 2026-11-01 | [06-identity-and-authorization](06-identity-and-authorization.md#d-68) |
| D-69 | Unscoped Field Visibility Clarification | accepted | 2026-11-01 | [06-identity-and-authorization](06-identity-and-authorization.md#d-69) |
| D-70 | Unscoped Identity Replacement Binding Proposal | accepted | 2026-11-01 | [06-identity-and-authorization](06-identity-and-authorization.md#d-70) |
| D-71 | Internal Execution-principal Authority Adapter | accepted | 2026-11-01 | [06-identity-and-authorization](06-identity-and-authorization.md#d-71) |
| D-72 | Multi-target Workflow Context Binding | accepted | 2026-11-01 | [04-processing-and-plugins](04-processing-and-plugins.md#d-72) |
| D-73 | Installed Processing Artifact Load Bound | superseded by D-131 | 2026-11-01 | [04-processing-and-plugins](04-processing-and-plugins.md#d-73) |
| D-74 | Processing Internal Retry Jitter Bound | accepted | 2026-11-01 | [04-processing-and-plugins](04-processing-and-plugins.md#d-74) |
| D-75 | Processing Durable Selection Placement | accepted (amended by D-135: current run only) | 2026-11-01 | [04-processing-and-plugins](04-processing-and-plugins.md#d-75) |
| D-76 | Dedicated Installed Preview Artifact Binding | superseded by D-131 and D-135 | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-76) |
| D-77 | Processing Reselection Identity and Terminal Work | superseded by D-135 | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-77) |
| D-78 | Processing Failed-work Retry Action Binding | accepted (amended by D-135: action renamed `workflow.reprocess`) | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-78) |
| D-79 | Superseded Work Terminality Interpretation Confirmation | superseded by D-135 | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-79) |
| D-80 | Processing Source Context Field-group Membership | accepted | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-80) |
| D-81 | Installed Result Requirement Interpretation | accepted | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-81) |
| D-82 | Exhausted Candidate Reselection State | superseded by D-135 | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-82) |
| D-83 | Processing Dispatcher Concurrency Bound | accepted | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-83) |
| D-84 | Processing Dispatcher Idle Wake-up Bound | accepted | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-84) |
| D-85 | Processing Scheduling Configuration Preparation | accepted (amended by D-135: Operator configuration) | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-85) |
| D-86 | Processing Cold-start Test Observation | accepted | 2026-11-02 | [04-processing-and-plugins](04-processing-and-plugins.md#d-86) |
| D-87 | AP-12 River Queue Evaluation | accepted (evidence only) | 2026-11-02 | [01-platform-and-storage](01-platform-and-storage.md#d-87) |
| D-88 | AP-12 Cross-Profile Worker and Pub/Sub Comparison | superseded by D-135 | 2026-11-02 | [05-events-and-delivery](05-events-and-delivery.md#d-88) |
| D-89 | Core-Domain Event-Driven Architecture Reassessment | accepted | 2026-11-02 | [05-events-and-delivery](05-events-and-delivery.md#d-89) |
| D-90 | Core-Domain Event-Driven Architecture Direction and Watermill Prototype | superseded by D-135 | 2026-11-02 | [05-events-and-delivery](05-events-and-delivery.md#d-90) |
| D-91 | Watermill Failure and Concurrency Validation Extension | accepted (evidence only) | 2026-11-02 | [05-events-and-delivery](05-events-and-delivery.md#d-91) |
| D-92 | Isolated SQLite Adapter Renewal Fix Assessment | superseded by D-135 | 2026-11-02 | [05-events-and-delivery](05-events-and-delivery.md#d-92) |
| D-93 | Watermill PostgreSQL-first Gate Clarification | superseded by D-135 | 2026-11-02 | [05-events-and-delivery](05-events-and-delivery.md#d-93) |
| D-94 | Watermill PostgreSQL Publication Integration Proposal | superseded by D-135 | 2026-11-02 | [05-events-and-delivery](05-events-and-delivery.md#d-94) |
| D-95 | AP-06 Session Repercussion Consolidation | accepted | 2026-11-02 | [09-process-and-documentation](09-process-and-documentation.md#d-95) |
| D-96 | Work-package reslicing | accepted | 2026-11-03 | [09-process-and-documentation](09-process-and-documentation.md#d-96) |
| D-97 | Ordered work-package numbering | accepted | 2026-11-03 | [09-process-and-documentation](09-process-and-documentation.md#d-97) |
| D-98 | Coordinated Existing-code Library Reassessment | accepted | 2026-11-02 | [09-process-and-documentation](09-process-and-documentation.md#d-98) |
| D-99 | Synthetic Preview Schema Completion | accepted | 2026-11-03 | [04-processing-and-plugins](04-processing-and-plugins.md#d-99) |
| D-100 | Processing Instance Read Authorization | accepted | 2026-11-03 | [04-processing-and-plugins](04-processing-and-plugins.md#d-100) |
| D-101 | Processing Artifact-root Configuration | accepted | 2026-11-03 | [08-operations-and-http](08-operations-and-http.md#d-101) |
| D-102 | Synthetic Schema Freeze Responses | accepted | 2026-11-03 | [04-processing-and-plugins](04-processing-and-plugins.md#d-102) |
| D-103 | Shared Bytes Binding | accepted (SDK-version mapping superseded by D-131) | 2026-11-03 | [04-processing-and-plugins](04-processing-and-plugins.md#d-103) |
| D-104 | AP-07 Gateway HTTP Submission boundary decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-104) |
| D-105 | AP-07 Gateway-state HTTP boundary decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-105) |
| D-106 | AP-07 Gateway Network configuration HTTP boundary decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-106) |
| D-107 | AP-07 Submission route decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-107) |
| D-108 | AP-07 Gateway-state route decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-108) |
| D-109 | AP-07 Gateway Network configuration route decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-109) |
| D-110 | AP-07 Gateway Network replacement semantics decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-110) |
| D-111 | AP-07 Gateway Network replacement authorization decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-111) |
| D-112 | AP planning and contract decision ownership | accepted | 2026-11-03 | [09-process-and-documentation](09-process-and-documentation.md#d-112) |
| D-113 | AP-07 Gateway degraded-report decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-113) |
| D-114 | AP-07 Gateway recovery decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-114) |
| D-115 | AP-07 Gateway Network replacement request decision | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-115) |
| D-116 | AP-07 Gateway state-fact boundary | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-116) |
| D-117 | AP-07 Gateway state HTTP response | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-117) |
| D-118 | AP-07 Gateway Network HTTP response | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-118) |
| D-119 | AP-07 contract freeze proposal | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-119) |
| D-120 | AP-07 recovered report idempotency | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-120) |
| D-121 | AP-07 Gateway credential-management HTTP addendum | accepted | 2026-11-03 | [02-intake-and-grouping](02-intake-and-grouping.md#d-121) |
| D-122 | AP-07 additive write-scope and execution-binding proposal | accepted | 2026-11-03 | [05-events-and-delivery](05-events-and-delivery.md#d-122) |
| D-123 | AP-07 selected-Workflow worker lifecycle scope extension | superseded by D-135 | 2026-11-04 | [05-events-and-delivery](05-events-and-delivery.md#d-123) |
| D-124 | AP-07 selected-Workflow source and publication scope extension | superseded by D-135 | 2026-11-04 | [05-events-and-delivery](05-events-and-delivery.md#d-124) |
| D-125 | AP-08 Unit-template ownership | accepted | 2026-11-04 | [03-assignment-and-correction](03-assignment-and-correction.md#d-125) |
| D-126 | Open-AP self-containment repair | accepted | 2026-11-04 | [09-process-and-documentation](09-process-and-documentation.md#d-126) |
| D-127 | AP-08 suppression direction | accepted | 2026-11-04 | [03-assignment-and-correction](03-assignment-and-correction.md#d-127) |
| D-128 | Owning-migration consolidation | accepted | 2026-11-05 | [01-platform-and-storage](01-platform-and-storage.md#d-128) |
| D-129 | In-process Core regex | accepted | 2026-11-05 | [04-processing-and-plugins](04-processing-and-plugins.md#d-129) |
| D-130 | Plugin trust clarification | accepted | 2026-11-05 | [04-processing-and-plugins](04-processing-and-plugins.md#d-130) |
| D-131 | Managed plugin process runtime | accepted | 2026-11-05 | [04-processing-and-plugins](04-processing-and-plugins.md#d-131) |
| D-132 | Documentation and naming separation | accepted | 2026-11-05 | [09-process-and-documentation](09-process-and-documentation.md#d-132) |
| D-133 | Command-ID reuse conflict code | accepted | 2026-11-06 | [08-operations-and-http](08-operations-and-http.md#d-133) |
| D-134 | Review-finding AP planning revision | accepted | 2026-11-06 | [09-process-and-documentation](09-process-and-documentation.md#d-134) |
| D-135 | Foundation redesign after the 2026-11-06 re-evaluation | accepted (item 9 amended by D-137, item 15 by D-138) | 2026-11-06 | [10-foundation-redesign](10-foundation-redesign.md#d-135) |
| D-136 | Agent execution rules and boundary crossings | accepted | 2026-11-06 | [09-process-and-documentation](09-process-and-documentation.md#d-136) |
| D-137 | Processing work record as implemented | accepted | 2026-11-07 | [10-foundation-redesign](10-foundation-redesign.md#d-137) |
| D-138 | Open-AP boundary repair and server exchange of Units | accepted | 2026-11-07 | [09-process-and-documentation](09-process-and-documentation.md#d-138) |
| D-139 | AP-09 open C-01 points | accepted | 2026-11-07 | [02-intake-and-grouping](02-intake-and-grouping.md#d-139) |
| D-140 | Not-found versus forbidden for Gateways and Networks | accepted | 2026-11-07 | [06-identity-and-authorization](06-identity-and-authorization.md#d-140) |
| D-141 | AP-20 boundary crossing for Gateway mapping re-resolution | accepted | 2026-11-07 | [03-assignment-and-correction](03-assignment-and-correction.md#d-141) |
| D-142 | Second open-AP boundary repair and start decisions for AP-10..AP-17 | accepted | 2026-11-07 | [09-process-and-documentation](09-process-and-documentation.md#d-142) |
| D-143 | Network re-inference after Address-rule changes | accepted | 2026-11-07 | [03-assignment-and-correction](03-assignment-and-correction.md#d-143) |
| D-144 | Message time in Message-related facts | accepted | 2026-11-08 | [05-events-and-delivery](05-events-and-delivery.md#d-144) |
| D-145 | Encrypted marker on Address rules | accepted | 2026-11-08 | [03-assignment-and-correction](03-assignment-and-correction.md#d-145) |
| D-146 | Shared client source and retry delay for search admission | accepted | 2026-11-09 | [06-identity-and-authorization](06-identity-and-authorization.md#d-146) |
| D-147 | Workflow classification without installed-step preparation | superseded by D-148 | 2026-11-10 | [04-processing-and-plugins](04-processing-and-plugins.md#d-147) |
| D-148 | Result classification stored at result write | accepted | 2026-11-10 | [04-processing-and-plugins](04-processing-and-plugins.md#d-148) |
| D-149 | Per-instance use of Delivery targets belongs to AP-12 | accepted | 2026-11-10 | [05-events-and-delivery](05-events-and-delivery.md#d-149) |
| D-150 | Attribute schemas replace Workflow output schemas | accepted | 2026-11-11 | [04-processing-and-plugins](04-processing-and-plugins.md#d-150) |
| D-151 | Agency-bound Workflows, Layouts and Attribute schemas | accepted | 2026-11-11 | [06-identity-and-authorization](06-identity-and-authorization.md#d-151) |
| D-152 | Field registry, one selection predicate and Subscription health | accepted | 2026-11-11 | [07-query-and-live](07-query-and-live.md#d-152) |
| D-153 | AP-21 write scope: Workflow selection in the intake store and the health fact store | accepted | 2026-11-11 | [04-processing-and-plugins](04-processing-and-plugins.md#d-153) |
| D-154 | Everything on its latest version in the AP-21 pull request | accepted | 2026-11-12 | [01-platform-and-storage](01-platform-and-storage.md#d-154) |
| D-155 | Test run time: test-only costs are configurable | accepted | 2026-11-12 | [09-process-and-documentation](09-process-and-documentation.md#d-155) |
| D-156 | AP-21 follow-ups after the merge | accepted | 2026-11-12 | [04-processing-and-plugins](04-processing-and-plugins.md#d-156) |
| D-157 | AP-22 runs before AP-12 on the AP-11 target-input port | accepted | 2026-11-12 | [05-events-and-delivery](05-events-and-delivery.md#d-157) |
| D-158 | AP-22 write scope: target-input bindings in `00013_events_delivery.sql` | accepted | 2026-11-12 | [05-events-and-delivery](05-events-and-delivery.md#d-158) |
| D-159 | AP-22 review: schema Agency facts and reading references | accepted | 2026-11-12 | [04-processing-and-plugins](04-processing-and-plugins.md#d-159) |
