# GoPager Documentation

## Document Authority

One fact has one maintained definition. Other documents link to it; they do not reproduce its fields, defaults, algorithms, status lists or acceptance text. Document titles and IDs may recur as navigation, not as competing specifications.

| Information | Single authoritative location | Maintenance responsibility |
| --- | --- | --- |
| User goals and observable outcomes (REQ-IDs) | [Requirements catalog](requirements-catalog.md) | Product |
| Product priorities, decision disposition/supersession (D-IDs), delivery scope, open gates (P-IDs), scope/execution approvals | [Authoritative plan](authoritative-plan.md) | Product / integration |
| Selected technologies and rationale (TECH-IDs), module/data ownership, topology and allowed dependencies | [Architecture](architecture.md) | Architecture / integration |
| Interface signatures, fields/types, state transitions, algorithms, permissions, errors, defaults/limits and compatibility | Respective [C-ID owner file](domain-contracts.md#contract-register); only genuinely cross-contract rules in [shared conventions](contracts/shared-conventions.md) | Contract owner / integration |
| Concrete input/expected-output examples (F-IDs) | [Core cases](contracts/reference-cases.md), [boundary cases](contracts/ap-02-boundary-cases.md) | Case's contract owner |
| Domain term meanings, prohibited terms and rename history | [Glossary](glossary.md) | Integration |
| Stable task definition, REQ/C context, role, prerequisites, proposed write scope, acceptance and non-goals | Respective [AP file](work-packages.md#package-directory) | Planning / integration |
| Named assignee, actual execution state, approved write-scope binding, commands/results, blockers and artifact handovers | Respective [execution record](execution/README.md) | Executing owner / integration |
| Shared session and immutable-AP workflow | [Execution rules](work-packages/shared-rules.md) | Integration |
| REQ-to-package coverage and planned evidence categories | [Requirement mapping](work-packages/requirement-mapping.md) | Integration |
| External source provenance, reuse assessment and documentation-review evidence | [Source/evidence notes](requirements-evidence.md) | Documentation / assessing owner |
| Human navigation | Root [README](../README.md), [package directory](work-packages.md), [contract directory](domain-contracts.md) | Documentation |
| Agent entry route | [AGENTS.md](../AGENTS.md) | Integration; links only, no duplicate rules |

Roles are not named assignees. An assignee or a completed task does not grant scope/execution approval; approvals belong only to the plan.

## Project Language

English is the language of repository artifacts, code identifiers/comments and interface documentation; no parallel translated specification is maintained. Product-language/i18n requirements belong to [REQ-097](requirements-catalog.md#j-processing-presentation-and-usability); responsibility routing belongs to the [architecture](architecture.md#language-and-internationalization).

## Status Terms

- **Confirmed:** explicitly decided; drafts may not reopen it.
- **Selected, validation pending:** technical planning baseline, not successful validation.
- **Draft:** proposed contract representation, not frozen.
- **Open/blocker:** missing decision/input/evidence assigned to an owner.
- **Approved:** recorded authority with exact scope/version/evidence.
- **Documented:** recorded specification, not implemented behavior.

Actual contract state is in its C-ID file, execution state in its execution record and approval in the plan. AP checkmarks predating the immutable-AP workflow are historical planning marks, not a current status source.

## Change Rules

1. Locate the owner in the matrix before editing. Do not add a summary of the same rule to another document, even if that document is a convenient entry point.
2. A product decision gets a stable D-ID/disposition and owner link in the plan; the chosen behavior is specified only in the linked owner. A technology selection/rationale is recorded under its TECH-ID in the architecture. Implementation-local choices are recorded beside the owning contract/interface or code, with rationale when non-obvious, not in a parallel decision diary.
3. Preserve stable REQ/D/P/C/AP/T/F/TECH IDs. Reserve removed IDs. Delivery phase and approval are never inferred from requirements, files or checkmarks.
4. Contract updates identify owner, version/status, changed surface, producers/consumers and affected packages. Keep data/error/time/revision/idempotency/transaction rules at that boundary; shared authorization or commit semantics require integration agreement. Frozen contract changes require renewed approval of affected scope.
5. Treat the current AP as read-only during its execution. Record execution information under [execution records](execution/README.md); adapt only affected not-yet-started downstream APs when a concrete finding requires it, following the [amendment procedure](work-packages/shared-rules.md#downstream-amendments).
6. Update owner content and incoming references in the same change. Conflicting owners block approval; do not silently pick a weaker rule or an unapproved default.
7. Evidence must distinguish planned cases from executed commands and results. No inferred product completion, SLA, capacity, RPO/RTO or hardware certification.
8. Names and propagation follow the [glossary maintenance rules](glossary.md#maintenance-rules). External code adoption follows the [reuse assessment](requirements-evidence.md#evidence-assessment-for-deliberate-reuse).
9. Once generated API/schema/SDK artifacts become authoritative, record their exact path/version in the owning C-ID file and replace corresponding hand-maintained detail with links. Never keep two independently editable definitions of the same schema.

## Reading Order

For assigned work, the AP file is sufficient as the initial context: follow its **Session Context** and the [shared start procedure](work-packages/shared-rules.md#session-start), including the execution record. Do not preload all contracts or rely on prior chat.

For a lookup, choose the owner above. D/P/TECH IDs route decisions and gates; C-ID files expose interface details; F-IDs locate examples. History is provenance, never an alternative specification.
