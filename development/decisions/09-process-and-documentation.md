# ADR: Process and documentation

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-19"></a>
## D-19 Confirmed

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed: documentation does not approve product implementation.
- **Owner detail:** [Scope](../authoritative-plan.md#delivery-scope), [approval gates](../authoritative-plan.md#workflow-and-approval-gates)

<a id="d-20"></a>
## D-20 Language/i18n direction

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed language/i18n direction.
- **Owner detail:** [Project language](../README.md#project-language), [REQ-097](../requirements-catalog.md#j-processing-presentation-and-usability), [responsibility routing](../architecture.md#language-and-internationalization)

<a id="d-21"></a>
## D-21 Naming authority

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed naming authority.
- **Owner detail:** [Glossary maintenance](../glossary.md#maintenance-rules)

<a id="d-95"></a>
## D-95 AP-06 Session Repercussion Consolidation

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** AP-06 session repercussion consolidation (documentary). Reconcile current navigation/status through explicit follow-up entries and historical labels, rather than relocate/rewrite prior evidence or reopen settled technology choices. Record the AP-05 standard-library correction, link later approved library selections, bring AP-06's prerequisite handover up to date, and distinguish historical Watermill candidates from D-94's limited current selection.
- **Owner detail:** This record is the only current definition

<a id="d-96"></a>
## D-96 Work-package reslicing

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** The user ordered a reslice of the AP-06+ work packages into an acyclic, strictly ordered path with a clear order-to-package mapping that depends on no later package, and allowed changing packages even where execution had started. The reslice created eleven successor assignments (tasks T055..T085, first numbered 17 to 27) covering the Processing kernel, intake, master data and correction, query/live, events/Delivery, plugin host, Processing presentation, identity lifecycle, frontend, operations and final evidence. AP-01..AP-05 stayed unchanged and the original AP-06..AP-16 files and evidence were kept as source history.
- **Consequences:** The execution path in [work-packages.md](../work-packages.md#revised-execution-path) is the authoritative order. Completed baselines and source evidence stay preserved; the numbering was corrected by D-97.
- **Owner detail:** [execution path](../work-packages.md#revised-execution-path)

<a id="d-97"></a>
## D-97 Ordered work-package numbering

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** The reslice numbering did not communicate execution order, so the user ordered the eleven revised assignments to be replaced under sequential IDs AP-06..AP-16 (old numbers 17 to 27, same order: Processing kernel, intake, master data and correction, query/live, events/Delivery, plugin host, presentation, identity lifecycle, frontend, operations, integration evidence). Task IDs T055..T085 and all scope stay unchanged; AP-01..AP-05 stay untouched.
- **Consequences:** The former AP-06..AP-16 files and the AP-06, AP-07, AP-08 and AP-12 execution records remain as labeled source history (`execution/source-ap-*.md`). References to the old numbers 17 to 27 in historical text mean the current AP-06..AP-16 in the same order.
- **Owner detail:** [execution path](../work-packages.md#revised-execution-path)

<a id="d-98"></a>
## D-98 Coordinated Existing-code Library Reassessment

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** Approved all-task library suitability checks and coordinated multi-session reassessment of existing code. The user requires library suitability to be checked for every task, not just substantial additions, and all previously produced code to be reassessed in separate scoped sessions. Independent repeated selections are rejected: sessions must first centrally report capability needs, then receive the shared selection for implementation. [The existing policy owner](../README.md#library-reuse) now specifies permitted custom-code exceptions and the coordination procedure, extending D-36 rather than creating a parallel instruction source. Root AGENTS already routes every agent to this policy.
- **Owner detail:** [policy owner](../README.md#library-reuse)

<a id="d-112"></a>
## D-112 AP planning and contract decision ownership

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Confirmed AP-owned planning and contract decisions; no separate AP prerequisite for a consuming AP's own contract freeze.
- **Owner detail:** This record is the only current definition

<a id="d-126"></a>
## D-126 Open-AP self-containment repair

- **Status:** accepted
- **Date:** 2026-11-04
- **Decision:** Approved direct open-AP planning repair without sidecars; inserts Gateway producer, preserves completed results and assigns all supporting work before its consumers. Directly maintain current AP documentation and required references, migrate needed source-assignment inputs to their owners and remove redundant source definitions/redirects/duplicate records. Preserve unique evidence and immutable baseline provenance; no product execution or sidecars. This supersedes earlier requirements to retain duplicate source-assignment files, not their behavioral decisions or observed results. The [ordered assignments](../work-packages.md#revised-execution-path) own task allocation, supporting paths and verification; the [mapping](../work-packages/requirement-mapping.md) owns REQ coverage.
- **Owner detail:** This record is the only current definition

<a id="d-132"></a>
## D-132 Documentation and naming separation

- **Status:** accepted
- **Date:** 2026-11-05
- **Decision:** Process documentation separated from permanent documentation; planning IDs only in `development/`; fixed domain names; project tooling; OpenAPI authoritative for implemented HTTP schemas.
- **Consequences:** Execution records written before 2026-11-06 refer to the former `docs/` path and to former test identifiers; see the [execution README](../execution/README.md#reading-records-written-before-2026-11-06). Earlier decisions and evidence are not rewritten. Checks: [AP-08 E-16](../execution/ap-08.md#e-16-follow-up-documentation-naming-and-tooling-separation).
- **Owner detail:** This record is the only current definition

<a id="d-134"></a>
## D-134 Review-finding AP planning revision

- **Status:** accepted
- **Date:** 2026-11-06
- **Decision:** Review-finding planning revision of open AP-11..AP-16 tasks and resolved contract statements (2026-11-06). Findings that need later-AP work were added to the open assignments, in their existing task paragraphs: - [AP-11 T066](../work-packages/ap-11-events-delivery.md#planning-before-coding): the attempt invocation carries the triggering fact's C-08 `correlationId`/`causationId`. - [AP-12 T069](../work-packages/ap-12-plugin-host.md#planning-before-coding): manifest Extension field definitions, storage and C-06 exposure, or an explicit deferral. - [AP-13 T072/T074](../work-packages/ap-13-processing-presentation-integration.md#planning-before-coding): production stored-Transmission preview capture and step-level safe failure explanations (candidate `failure_step`), with their cases. - [AP-14 T075](../work-packages/ap-14-identity-lifecycle.md#planning-before-coding): Operator-configurable shorter Session lifetimes. - [AP-16 T081](../work-packages/ap-16-operations.md#planning-before-coding): readiness-gated production transactions and bounded, authorization-before-pagination management listings.
- **Owner detail:** This record is the only current definition

<a id="d-136"></a>
## D-136 Agent execution rules and boundary crossings

- **Status:** accepted
- **Date:** 2026-11-06
- **Decision:** The [execution rules](../work-packages/shared-rules.md) are the only definition of how an agent executes an AP; the skill, the development README, the package directory and the execution README link to them. The executing agent also performs approved boundary crossings: it analyzes the impact, asks the user, records each approved crossing as an ADR with a D-ID, adds follow-ups to affected completed records and amends affected open APs inline. The task state `accepted` is retired; `verified` with evidence is final and the user accepts by merging. An execution record's handover always describes the current state. Open APs follow the [AP template](../work-packages/ap-template.md).
- **Consequences:** AP-09..AP-17 were restructured to the template; the handovers of AP-05..AP-08 were brought up to date with their D-135 follow-ups; earlier `accepted` task states read as `verified`.
- **Owner detail:** [Execution rules](../work-packages/shared-rules.md), [AP template](../work-packages/ap-template.md), [execution README](../execution/README.md)
