# GoPager Development-Process Documentation

This tree holds the working documents of the agent-assisted delivery process: plan, work packages, contract drafts and execution evidence. Permanent product and code documentation is a separate tree in [docs/](../docs/README.md) with its own [documentation rules](../CONTRIBUTING.md#documentation). This tree is temporary and will be deleted. Do not copy content between the trees; see [Permanent Documentation](#permanent-documentation) and [Naming](#naming).

## Document Authority

One fact has one maintained definition. Other documents link to it; they do not reproduce its fields, defaults, algorithms, status lists or acceptance text. Document titles and IDs may recur as navigation, not as competing specifications.

| Information | Single authoritative location | Maintenance responsibility |
| --- | --- | --- |
| User goals and observable outcomes (REQ-IDs) | [Requirements catalog](requirements-catalog.md) | Product |
| Decisions (D-IDs): status, supersession, short-form decision and consequences | [Decision records](decisions/README.md) (ADR index and topic files) | Product / integration |
| Product priorities, delivery scope, open gates (P-IDs) and the approval rule | [Authoritative plan](authoritative-plan.md) | Product / integration |
| Selected technologies and rationale (TECH-IDs), module/data ownership, topology and allowed dependencies | [Architecture](architecture.md) | Architecture / integration |
| Interface signatures, fields/types, state transitions, algorithms, permissions, errors, defaults/limits and compatibility | Respective [C-ID owner file](domain-contracts.md#contract-register); only genuinely cross-contract rules in [shared conventions](contracts/shared-conventions.md) | Contract owner / integration |
| Concrete input/expected-output examples (F-IDs) | [Core cases](contracts/reference-cases.md), [boundary cases](contracts/ap-02-boundary-cases.md) | Case's contract owner |
| Domain term meanings, prohibited terms and rename history | [Glossary](glossary.md) | Integration |
| Stable task definition, REQ/C context, role, prerequisites, supporting write scope, acceptance and non-goals | Respective [AP file](work-packages.md#revised-execution-path) | Planning / integration |
| Authorized assignment revision | Respective AP file; authority in the plan | Planning / integration |
| Named assignee, actual execution state, approved write-scope binding, commands/results, blockers and artifact handovers | Respective [execution record](execution/README.md) | Executing owner / integration |
| Shared session and immutable-AP workflow | [Execution rules](work-packages/shared-rules.md) | Integration |
| REQ-to-package coverage and planned evidence categories | [Requirement mapping](work-packages/requirement-mapping.md) | Integration |
| External source provenance, reuse assessment and documentation-review evidence | [Source/evidence notes](requirements-evidence.md) | Documentation / assessing owner |
| Implemented behavior for operators, API clients, plugin authors and contributors: architecture and data flow, operations/configuration, HTTP usage, Workflows, plugins, build/test/lint commands, repository layout and code conventions | [docs/](../docs/README.md) | Implementing owner, in the same change as the code |
| Implemented HTTP routes and request/response schemas | [api/openapi/v1.yaml](../api/openapi/v1.yaml) | Implementing owner; the owning C-ID file records it as authoritative ([change rule 9](#change-rules)) |
| Implemented plugin wire protocol and Go plugin SDK | [api/plugin/v2/plugin.proto](../api/plugin/v2/plugin.proto), [pluginsdk/](../pluginsdk/) | Implementing owner; [C-10](contracts/c-10-plugin-host.md) records them as authoritative (change rule 9) |
| Identifier-level behavior of Go code | Go doc comments (`go doc`) | Implementing owner |
| Project description and getting started; contribution and documentation rules | Root [README](../README.md); [CONTRIBUTING.md](../CONTRIBUTING.md) | Integration / documentation |
| Agent entry route for code work | [AGENTS.md](../AGENTS.md), imported by [CLAUDE.md](../CLAUDE.md) | Integration; links into `docs/` only, no duplicate rules |
| Project agent tooling: plugins, hooks, skills, MCP servers | [.claude/settings.json](../.claude/settings.json), `.claude/skills/`, [.mcp.json](../.mcp.json); list and purpose in [Skills, Agents and MCP Servers](#skills-agents-and-mcp-servers) | Integration |
| Boundary between this tree and permanent documentation; naming rules | [Permanent Documentation](#permanent-documentation), [Naming](#naming); decision [D-132](decisions/09-process-and-documentation.md#d-132) | Integration / documentation |
| Process navigation | [Package directory](work-packages.md), [contract directory](domain-contracts.md) | Documentation |
| Repository-wide library reuse policy | [Library reuse](#library-reuse) | Architecture / integration |

Permanent documentation and generated/published artifacts describe implemented behavior and are authoritative for implemented schemas once recorded under change rule 9. Contracts in this tree remain planning contracts: they own planned behavior, gates and boundaries, link to those artifacts instead of repeating their fields, and a disagreement between code and a contract is reported here, not resolved by editing permanent documentation to match the contract.

Roles are not named assignees. An assignee or a completed task does not grant scope/execution approval. There is one approval gate: the user approves a work package before it starts ([approval rule](authoritative-plan.md#workflow-and-approval-gates)); no intermediate approval gates exist inside a package, and completion evidence stays mandatory.

## Project Language

English is the language of repository artifacts, code identifiers/comments and interface documentation; no parallel translated specification is maintained. Product-language/i18n requirements belong to [REQ-097](requirements-catalog.md#j-processing-presentation-and-usability); responsibility routing belongs to the [architecture](architecture.md#language-and-internationalization).

## Library Reuse

This mandatory policy applies to every repository activity and every work package, including tests, tooling and infrastructure. Before implementing any functionality, always check whether suitable libraries exist and prefer suitable, well-maintained libraries over rebuilding that functionality. The standard library counts as reuse.

Write only the necessary custom functionality when no suitable library exists, candidates are outdated, insufficiently maintained or demonstrably faulty, the task is too small to justify their dependency/integration cost, or application-specific domain behavior or module/library integration requires that code. Assess suitability against the actual required behavior, maintenance, dependency footprint and integration cost; do not assume that fewer dependencies or custom code is inherently better.

For Go, prefer the `go-pkgs` MCP tools for package discovery and current package/version facts. If unavailable or inconclusive, record that limitation and use authoritative upstream documentation/source and local toolchain documentation; do not invent package facts or treat an unavailable lookup as proof that no library exists.

Record substantial selections/exceptions with alternatives and evidence at the technology/implementation owner; link from execution evidence. Reassess unnecessary reinvention. Library adoption must remain within assigned scope and fixed contracts.

For a repository-wide library reassessment, use disjoint scopes and separate generated artifacts. Coordinate repeated needs at the architecture owner before choosing shared replacements; validate behavior within approved write scopes.

## Permanent Documentation

Decision: [D-132](decisions/09-process-and-documentation.md#d-132). Permanent documentation is everything outside this tree: `docs/`, `api/` (OpenAPI, proto), Go doc comments, `README.md`, `AGENTS.md`, `CLAUDE.md` and `CONTRIBUTING.md`. Rules:

1. It describes implemented behavior only: no history, decision rationale, alternatives, status, plans or approvals.
2. READMEs follow ordinary README conventions: project description, requirements, getting started, links. No meta prose about audience, authority or how the documentation is maintained. Contributor rules go in `CONTRIBUTING.md`, agent routing in `AGENTS.md`.
3. Every registered HTTP route is in `api/openapi/v1.yaml`. Prose links to the schema instead of repeating fields. Every exported Go identifier in hand-written code, including the Ent schema definitions in internal/persistence/ent/schema/, has a self-contained doc comment. Generated files (header `Code generated … DO NOT EDIT`) are exempt and are never edited by hand.
4. Update it in the same change as the code it describes. Verify statements against code; when this tree and the code disagree, the code is documented and the disagreement is reported here.
5. It never references this tree, by link, path, document name or planning ID. This also applies to code identifiers, test names, environment variables, database object names, persisted values and hyphenless forms such as `AP05` or `ap07_`.

## Naming

1. Planning IDs (REQ, D, P, C, AP, T, F, E, TECH and similar) are used exclusively inside this tree.
2. Domains have fixed names, listed in [Domains and Data Ownership](architecture.md#domains-and-data-ownership): Intake, Grouping, Master data, Assignment, Identity, Query, Delivery, Processing. This tree uses those names. Code uses the lowercase identifier from that table where a domain must be named, for example `producer.domain` in Domain events. Numbered domain IDs are not used.
3. New concepts get descriptive names that also work in code and permanent documentation. Terms follow the [glossary](glossary.md#maintenance-rules).

## Skills, Agents and MCP Servers

1. Use the available skills, subagents, plugins, hooks and MCP servers wherever they help with the task.
2. When a recurring task would benefit from a skill, subagent, MCP server or hook that does not exist yet, create or install it at project scope (`.claude/skills/`, `.claude/agents/`, `.claude/settings.json`, `.mcp.json`) instead of repeating manual steps, and add it to the list below. Tools that encode this delivery process may reference this tree and are removed with it; all others must not reference it.
3. Prefer reviewed, maintained sources (for example the official Anthropic plugin marketplace). Review third-party skills before adoption, as with [library reuse](#library-reuse).

Current project setup:

| Tool | Purpose |
| --- | --- |
| `go-pkgs` MCP server (`.mcp.json`) | Go package discovery and version facts for library reuse |
| `gopls-lsp` plugin (`.claude/settings.json`) | Go diagnostics, definitions and references; requires `gopls` on `PATH` |
| gofmt hook (`.claude/settings.json`) | Formats every edited `.go` file |
| `claude-security` plugin (`.claude/settings.json`) | On-demand vulnerability scanning with verified findings and patch proposals |
| `execute-work-package` skill | Executes an AP under these rules (process tool) |

## Status Terms

- **Confirmed:** explicitly decided; drafts may not reopen it.
- **Selected, validation pending:** technical planning baseline, not successful validation.
- **Draft:** proposed contract representation, not frozen.
- **Open/blocker:** missing decision/input/evidence assigned to an owner.
- **Approved:** recorded authority with exact scope/version/evidence.
- **Documented:** recorded specification, not implemented behavior.

Contract state is in its C-ID file, execution state in its execution record and decisions in the ADRs; the approval rule is in the plan. Assignment checkmarks are not evidence.

## Change Rules

1. Locate the owner in the matrix before editing. Do not add a summary of the same rule to another document, even if that document is a convenient entry point.
2. A product decision gets a stable D-ID, an entry in the ADR file of its topic (status, date, supersession, the decision in one to five sentences, consequences) and a row in the [ADR index](decisions/README.md); the chosen behavior is specified only in the linked owner. A superseded decision keeps its record and names its successor. A technology selection/rationale is recorded under its TECH-ID in the architecture. Implementation-local choices are recorded beside the owning contract/interface or code, with rationale when non-obvious, not in a parallel decision diary.
3. Preserve stable REQ/D/P/C/AP/T/F/TECH IDs. Reserve removed IDs. Delivery phase and approval are never inferred from requirements, files or checkmarks.
4. Contract updates identify owner, version/status, changed surface, producers/consumers and affected packages. Keep data/error/time/revision/idempotency/transaction rules at that boundary; shared authorization or commit semantics require integration agreement. Necessary planning and contract decisions belong to the AP that consumes them, based on approved prior-AP outputs or user clarification; do not add a separate AP or owner sign-off gate unless the approved scope/change rules require it. Freeze the exact required slice before implementation. Changes to frozen prior-AP results still require the explicit earlier-result gate and renewed approval of affected scope.
5. Executing agents cannot enlarge their assignment. Authorized planners amend AP documentation directly, with scope/authority recorded in the plan; do not create sidecars. Execution information stays in execution records.
6. Every executing agent follows [mandatory documentation maintenance](work-packages/shared-rules.md#mandatory-documentation-maintenance). Conflicting owners block dependent work; do not silently pick a weaker rule or unapproved default.
7. Evidence must distinguish planned cases from executed commands and results. No inferred product completion, SLA, capacity, RPO/RTO or hardware certification.
8. Names and propagation follow the [glossary maintenance rules](glossary.md#maintenance-rules). External code adoption follows the [reuse assessment](requirements-evidence.md#evidence-assessment-for-deliberate-reuse).
9. Once generated API/schema/SDK artifacts become authoritative, record their exact path/version in the owning C-ID file and replace corresponding hand-maintained detail with links. Never keep two independently editable definitions of the same schema.

## Reading Order

For assigned work, start from the AP's **Session Context** and the [shared procedure](work-packages/shared-rules.md#session-start). Read only linked sections needed by its tasks and the specific producer handovers. Do not preload decision history, source assignments, whole contracts or prior chat.

For a lookup, choose the owner above. D/P/TECH IDs route decisions and gates; C-ID files expose interface details; F-IDs locate examples. History is provenance, never an alternative specification.
