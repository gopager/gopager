# Execution Rules

[Document authority](../README.md#document-authority) | [Assignments](../work-packages.md#revised-execution-path) | [Record format](../execution/README.md#required-record-structure) | [AP template](ap-template.md)

This file is the only definition of how an agent executes a work package (AP). Other documents link here instead of restating these rules. The approval rule itself is defined in the [plan](../authoritative-plan.md#workflow-and-approval-gates): the user approves a package once, before it starts; that approval covers all of the package's assigned work.

Roles: the **user** approves packages and boundary crossings and accepts results by merging them. The **executing agent** does everything else, including the planning a package assigns and approved [scope changes](#scope-change). Domain names in an AP (Intake, Delivery, …) describe the affected code area, not a person.

## Session Start

1. Read the assigned AP file and this file. Do not preload anything else: follow an AP link only when the current task needs it, and read only the linked section.
2. Read the user stories of the AP's REQ IDs: only their rows in the [requirements catalog](../requirements-catalog.md) (story and acceptance), for example `grep -E '^\| REQ-(041|042|134) \|' development/requirements-catalog.md`. They explain what the tasks are for; the AP's tasks and done criteria define what is built. Where the AP names its part of a shared REQ, the rest belongs to the AP named there.
3. Ask the user for the package start approval if it is missing. Ask the AP's **Open user decisions** in the same structured question; record the answers at the owner named there (usually [delivery scope](../authoritative-plan.md#delivery-scope) or an ADR) before dependent work.
4. Inspect the worktree (`git status`, `git log -1`). Preserve changes you did not make.
5. Verify every **Input** against the linked handover and the actual artifacts (files, routes, migrations, tests). A missing or contradicting input is a [scope change](#scope-change) of class *input gap*, not a reason to backfill it.
6. Create or continue only this AP's execution record (`development/execution/ap-NN.md`) with the [required structure](../execution/README.md#required-record-structure): baseline revision and uncommitted delta, start approval, assignee, task IDs.

## Scope

An AP defines its boundaries in fixed sections ([template](ap-template.md)):

- **Write scope:** the paths the AP may change. Before handover, `git diff --name-only <baseline>` must list only these paths plus the documentation targets of [documentation maintenance](#mandatory-documentation-maintenance). Generated output of a listed source (Ent, OpenAPI client) belongs to the same scope.
- **Owned contract slices:** the C-ID sections, F-cases and glossary terms the AP defines or changes. The AP's own planning ("Planning before coding") is assigned work: do it, record it at the named destination, then code. It needs no further approval.
- **Inputs:** completed earlier results the AP builds on. They are fixed; changing them is a boundary crossing.
- **Compatibility context:** later APs that will consume this AP's output. Keep their needs possible; do not implement their tasks or depend on their unfinished output.
- **Non-goals:** explicitly excluded work.

Add a library or seam only for a verified current need ([library reuse](../README.md#library-reuse)).

### Package Layout

Domain code lives in `internal/<domain>/` (for example `internal/units/`, `internal/assignments/`, `internal/ingest/`), HTTP adapters and handlers in `internal/httpapi/`, and `cmd/gopager/` holds composition and process wiring only. Persistence: Ent schemas in `internal/persistence/ent/schema/`, store implementations in `internal/persistence/ent/<name>store/`, generated Ent code under `internal/persistence/ent/` (regenerate, never edit), SQL migrations in `internal/persistence/migrations/`. Until the first release, a new table gets a new migration file and a change to an existing table edits the migration that creates it ([D-128](../decisions/01-platform-and-storage.md#d-128)). The architecture's [allowed dependencies](../architecture.md#allowed-dependencies) govern imports.

## Execution and Verification

1. Work task by task: plan (record decisions at their owner), implement, verify, update documentation, update the execution record.
2. Run the smallest checks that cover the task's done criteria, then the AP's **Verification** commands. PostgreSQL, HTTP/SSE, browser, real provider and operational evidence are distinct; a compile or unit run does not replace them.
3. Record every check as an E-ID in the execution record: date, exact command, versions, environment, observed result, limitations. Never record a planned or unexecuted check as passed.
4. Repair failures inside the scope and rerun the affected checks; a failing check is not a blocker.
5. Before handover run `make`, `go test ./...` and `make docs-check`.
6. Parallel execution (subagents, worktrees) requires disjoint write scopes.

## Mandatory Documentation Maintenance

Update all documentation affected by a change in the same work step, before dependent work. An unaffected document needs no edit; never copy a definition into a second place. [Permanent documentation](../README.md#permanent-documentation) rules apply to everything outside `development/`.

| Change | Update |
| --- | --- |
| Planned interface behavior (fields, states, errors, permissions, limits) | The C-ID file section; concrete cases in [reference cases](../contracts/reference-cases.md) or [boundary cases](../contracts/ap-02-boundary-cases.md); a new case takes the next free F-number across `development/contracts/` |
| Implemented HTTP route or schema | [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and the [docs/http-api.md](../../docs/http-api.md) section; the C-ID file links to it instead of repeating fields |
| Plugin wire protocol or SDK | [api/plugin/v2/plugin.proto](../../api/plugin/v2/plugin.proto), `pluginsdk/`, [docs/plugins.md](../../docs/plugins.md) |
| Workflow behavior | [docs/workflows.md](../../docs/workflows.md) |
| Configuration, roles, runners, operation | [docs/operations.md](../../docs/operations.md) |
| Implemented data flow, packages, persistence | [docs/architecture.md](../../docs/architecture.md) |
| Planned module/data ownership or allowed dependencies | The [development architecture](../architecture.md) section |
| Build, test, lint, generation, migrations | [docs/development.md](../../docs/development.md) |
| Exported Go identifier | Its Go doc comment |
| New or renamed domain term | [Glossary](../glossary.md#maintenance-rules) |
| Product decision or approved boundary crossing | ADR in the topic file plus index row ([decision records](../decisions/README.md)) |
| Delivery disposition of a requirement | [Delivery scope](../authoritative-plan.md#delivery-scope) |
| Technology selection | TECH-ID in the [development architecture](../architecture.md) |
| Producer or consumer of a requirement changed | [Requirement mapping](requirement-mapping.md), [package directory](../work-packages.md#revised-execution-path) |
| Results, commands, blockers, handover | The AP's execution record |

The AP's **Documentation to update** section names the expected targets; the table decides for anything else. Record the checked documentation sections in the handover.

## Scope Change

Classify every need that the AP does not plainly cover:

| Class | Examples | Action |
| --- | --- | --- |
| In scope | Path in the write scope, owned contract slice, assigned planning question | Do it. Do not ask. |
| Clarification | Missing product information, several materially different options the stories and fixed context cannot decide | Ask a structured question; block only the affected work |
| Input gap | A listed input is missing, incomplete or contradicts the code | Boundary-crossing procedure |
| Boundary crossing | Path outside the write scope; contract slice owned by another AP or already implemented; an accepted ADR; a completed AP's behavior or output; a later AP's task; delivery scope (except the AP's listed open user decisions) | Boundary-crossing procedure |

### Boundary-crossing Procedure

1. **Stop only the affected work.** Continue independent tasks.
2. **Analyze impact.** Search for the affected IDs (`C-nn`, `D-nnn`, `REQ-nnn`, `F-nn`), file anchors, routes, event names and code paths:
   - Backward, completed work: `grep -rln '<id-or-anchor>' development/execution/ development/contracts/` for handovers and evidence that rely on the current behavior; `grep -rln '<symbol-or-route>' internal/ cmd/ api/ docs/` for code, tests and permanent documentation.
   - Forward, open work: `grep -ln '<id-or-anchor>' development/work-packages/ap-*.md` for APs that list it as an input or compatibility context; the [requirement mapping](requirement-mapping.md) rows of the affected REQ-IDs.
3. **Request approval** with a structured question tool. State in the question:
   - the change and why it is needed (evidence, affected task);
   - the alternatives, always including "stay within the scope" and its consequence;
   - the affected documentation (development and permanent);
   - retroactive checks: completed APs whose behavior, handover or evidence changes, and the tests to rerun;
   - affected open APs and how their assignment changes.
4. **After approval**, in this order:
   1. Record an ADR with the next free D-ID (last row of the [ADR index](../decisions/README.md#index)) in the topic file from the [ADR file table](../decisions/README.md#adr-files) and add the index row: AP/task, the exact approved scope, and as consequences the affected APs and records.
   2. Implement the change and update documentation per the [table](#mandatory-documentation-maintenance).
   3. For each affected completed AP: add `## Follow-up (D-nnn), <date>` to its execution record with new E-IDs and the rerun evidence, and update its **Handover** to the current state.
   4. For each affected open AP: amend its sections inline (inputs, tasks, scope, compatibility context) and link the D-ID. Do not execute its tasks.
   5. Update the requirement mapping and package directory if a producer changed.
   6. In this AP's record, add a *Blockers and Deviations* entry that links the D-ID.
   7. Resume the affected work. The approval covers it; do not ask again.
5. **After rejection:** record the rejection in *Blockers and Deviations* and continue within the scope. If the scope cannot reach a task's done criteria, ask the user how to proceed with that task; it stays open only with their agreement ([continuation](#mandatory-continuation-and-stop-conditions)).

Never change fixed behavior, an earlier result, an AP assignment or a later AP without this procedure, and never silently choose an unowned default.

<a id="approved-active-ap-addenda"></a>
Existing addenda (for example the [AP-07 intake addendum](addenda/ap-07-intake-implementation.md)) remain valid for their completed packages; new planning changes are made inline in the AP.

## Mandatory Continuation and Stop Conditions

Executing an AP means the whole package: planning, all tasks, verification, documentation and handover, unless the user explicitly narrows it. After each partial result, reconcile the execution record with every task, done criterion and documentation obligation, and continue with the next actionable item. Milestones, passing focused tests and progress updates are not completion.

**The agent stops only when the AP is complete** ([Handover](#handover)). It never ends a session on its own initiative while a task, done criterion, documentation obligation or handover item is open.

When an item cannot proceed (missing information, a pending boundary crossing, a contradiction, unavailable access or environment), the agent **asks the user immediately** in a structured question, continues all independent work in the meantime and resumes the item with the answer. Writing a question into the execution record is not asking it; an impediment recorded but not asked is not a reason to stop.

An item may stay open only with the user's explicit agreement for that item. Ask for it the same way: name the item, the impediment, what would resolve it and the proposal to leave it open. Without that agreement the item is not done and the agent continues or asks.

An incomplete execution may end only when:

- the user pauses, cancels or narrows the assignment, or explicitly agrees to leave the remaining items open;
- a hard runtime or tool limit actually terminates the session (leave a truthful checkpoint in the record).

Package size, effort spent, response length, anticipated context limits, a missing commit, a passing milestone or a recorded blocker are not stop conditions. Do not defer actionable work to a later session, do not ask whether to continue, and do not end with a report of open points instead of a question about them.

When ending incomplete, record in the execution record: the reason (with the user's agreement or the limit hit), affected task IDs, evidence, who resolves each open item and what is needed to resume. Keep verified work distinct from blocked or unexecuted work.

## Handover

The package is complete when every task is `verified` with evidence, the diff is within scope, documentation matches the delivered behavior and the record's **Handover** lists: artifact/API/schema/test paths, consuming APs, evidence E-IDs, checked documentation sections and unresolved discrepancies. The user accepts the result by merging it.

The **Handover** section always describes the current state and is updated by later follow-ups. Evidence entries are never rewritten. Report to the user only the task states, evidence and open points; no separate summary documents.
