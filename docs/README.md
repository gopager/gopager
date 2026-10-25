# GoPager Documentation

Date: 2026-10-24. This documentation is the authoritative specification of the new product in its own repository. It describes a target state, not evidence of an existing implementation or qualified deployment.

## Document Authority

| Information | Single authoritative location | Maintenance responsibility |
| --- | --- | --- |
| User goals and observable acceptance outcomes | [Requirements catalog](requirements-catalog.md) | Product ownership |
| Decisions, delivery scope, open issues, and approval gates | [Authoritative plan](authoritative-plan.md) | Product and integration ownership |
| Technologies, domain boundaries, data ownership, and dependencies | [Architecture](architecture.md) | Architecture ownership |
| Contracts, interfaces and invariants | [Contract index](domain-contracts.md) -> individual [C-ID files](contracts/) | Respective contract owner; integration for shared boundaries |
| Shared contract conventions and reference cases | [Shared conventions](contracts/shared-conventions.md), [reference cases](contracts/reference-cases.md) | Integration / respective case owner |
| Work packages, write scopes and dependencies | [Package index](work-packages.md) -> individual [AP-ID files](work-packages/) | Integration and respective package owner |
| Shared execution gates and requirement-to-package traceability | [Execution rules](work-packages/shared-rules.md), [requirement mapping](work-packages/requirement-mapping.md) | Integration / respective package owner |
| Provenance and limits of evidence | [Evidence notes](requirements-evidence.md) | Documentation ownership |
| Binding domain vocabulary and logical stored information | [Glossary](glossary.md) | Integration ownership; draft until AP-04 freeze |

Responsibilities are technical roles, not named people yet. Assign a role to a specific person or agent before assigning its work package. Until then, no exclusive write scope has a named assignee.

## Project Language

English is the authoritative project language for specifications, documentation, code identifiers, code comments, interfaces, and default product text. Maintain these planning documents in English only; no parallel German specification or translated documentation set is required.

Prepare the product for internationalization from the first implementation: separate user-facing Localized text from logic through stable text keys and an English text catalog, support parameterized Localized text and pluralization, and keep UI language separate from locale-dependent formatting. English is the only required language initially. Additional translations, language packs, and a language-selection UI are not required. Details and ownership belong to the [architecture](architecture.md#language-and-internationalization), C-07/C-11, and AP-13; this documentation does not claim an implemented i18n runtime.

## Status Terms

- **Confirmed:** explicit product decision; downstream drafts must not reopen it.
- **Selected, validation pending:** architecture planning decision, not a successful technical validation.
- **Draft:** documented proposal for a boundary or representation, not a frozen contract.
- **Open/blocker:** missing decision or evidence, with affected contracts and packages identified.
- **Approved:** separately documented approval with contract version, scope, and evidence; creating a file does not grant approval.
- **Documented:** the documentation records project content in the repository; this says nothing about product completion.

All contract versions are currently `0.1-draft`. Confirmed invariants remain binding; concrete fields/interface shape are not frozen. Implementation packages are blocked. [AP-02](work-packages/ap-02-domain-contracts.md) records documentary coverage/completion, not runtime or approval evidence; status remains owned by the [plan](authoritative-plan.md#workflow-and-approval-gates). The D-29 model preserves originals, current target resolution and rebuildable Message contributions with permitted representative selection. Diagram stubs never replace full master-data requirements. D-30 defers heuristics entirely; D-31 excludes application Backup/Restore logic.

## Change Rules

1. Record new user decisions in the authoritative plan first, then update affected owner documents in the same change.
2. Keep stable REQ IDs; reserve removed IDs rather than reusing them. Maintain delivery status only in the plan.
3. Each technical statement has one owner. Other documents reference it rather than maintaining divergent copies. A conflict blocks approval of the affected contract until resolved.
4. Contract changes identify version, producers/consumers, impact, and affected packages. No package owner independently changes shared authorization or transaction semantics.
5. Implementation claims require actual tests or operational evidence. Planned examples and validation commands are not test results.
6. Do not infer unapproved defaults, size limits, SLA, RPO/RTO, or hardware promises from architecture assumptions.
7. PagerMon source code remains external provenance. Reuse requires a separate evidence assessment; there is no legacy compatibility promise.
8. Write project artifacts in English and preserve stable technical IDs across language changes. Preparing i18n does not require additional translations or translating received Payload and operator-authored data.
9. Names follow the [glossary](glossary.md), the single authoritative source for names. New names, detected collisions, and renames are recorded there (rename log: old name, new name, meaning, reason, package) by the package that finds them, and that package re-checks and updates all documents, code, schemas, API/event definitions, tests, and UI texts referring to the affected names, or records remaining occurrences as an owned follow-up (glossary maintenance rules).

## Reading Order

Start with the assigned AP file via the [package index](work-packages.md), its [shared execution rules](work-packages/shared-rules.md), and its C-/REQ-/P-IDs. Use the [contract index](domain-contracts.md) to open only relevant C-ID files and [shared conventions](contracts/shared-conventions.md); follow referenced contracts only for the boundary being changed. Use the separate [requirement mapping](work-packages/requirement-mapping.md) for traceability lookups. Read relevant requirement rows, glossary entries and current plan decisions/status rather than every document. The [architecture](architecture.md) owns the shared model/dependencies. Historical [evidence notes](requirements-evidence.md) are needed for provenance questions, not routine implementation context.

The contract index is navigation, not duplicated specification. Each contract has a stable C-ID, descriptive file name, owner/status, searchable headings and links to referenced contracts. [Reference cases](contracts/reference-cases.md) and [AP-02 boundary cases](contracts/ap-02-boundary-cases.md) retain stable F-IDs for targeted lookup. Markdown remains suitable for these prose contracts; actual machine-readable API/schema artifacts belong to their existing approval gates, not a parallel hand-maintained copy of the prose. Other suitable formats may be used when they become the authoritative artifact for that information.

Keep new material in its owner file; link instead of copying it into the index, plan and package descriptions. Split further by cohesive responsibility when a file grows, preserving IDs and updating links/write scopes. Do not combine unrelated contracts merely to minimize file count. The root README is an entry point, not a second scope or task list.