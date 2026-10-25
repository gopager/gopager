# GoPager Documentation

Date: 2026-10-24. This documentation is the authoritative specification of the new product in its own repository. It describes a target state, not evidence of an existing implementation or qualified deployment.

## Document Authority

| Information | Single authoritative location | Maintenance responsibility |
| --- | --- | --- |
| User goals and observable acceptance outcomes | [Requirements catalog](requirements-catalog.md) | Product ownership |
| Decisions, delivery scope, open issues, and approval gates | [Authoritative plan](authoritative-plan.md) | Product and integration ownership |
| Technologies, domain boundaries, data ownership, and dependencies | [Architecture](architecture.md) | Architecture ownership |
| Contracts, interfaces, invariants, and reference cases | [Contract catalog](domain-contracts.md) | Respective contract owner; integration for shared boundaries |
| Work packages, write scopes, dependencies, and evidence | [Work packages](work-packages.md) | Integration and respective package owner |
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

All contract versions are currently `0.1-draft`. Confirmed invariants remain binding; the concrete interface shape is not frozen. Implementation packages are blocked. Contract clarification and technical validation planning are described as next work, not already completed.

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

Read product goals and delivery status in the [plan](authoritative-plan.md), then the [catalog](requirements-catalog.md), [architecture](architecture.md), [contracts](domain-contracts.md), and [work packages](work-packages.md). The root README is an entry point only; it is not a second scope or task list.