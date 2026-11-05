# Execution and Ownership Rules

[Document authority](../README.md#document-authority) | [Assignments](../work-packages.md#revised-execution-path) | [Evidence format](../execution/README.md)

## Mandatory AP Self-Containment and Planning

1. An AP may rely only on verified completed earlier outputs, applicable fixed contracts/decisions, its own explicitly assigned work, and genuine user clarification. Name each required output, producer and version; numbering or checkmarks do not establish completion.
2. Each AP must state its planning questions, required extent, canonical destinations, references and acceptance cases. Include necessary supporting contracts, persistence/migrations, HTTP/CLI, facts/work and root/lifecycle integration in its tasks and write scope.
3. Complete assigned planning before coding. Execution authorizes its in-scope decisions and contract changes; neither unfinished own planning nor domain/integration ownership creates an external approval gate. Never ask how much to plan or permission to perform assigned planning.
4. Decide from the user stories and fixed context. Ask only for genuine clarification or a materially different choice that the available evidence cannot resolve. Separate independently answerable questions.
5. Later APs are compatibility context only. Do not depend on their unfinished outputs or implement their tasks. Add a library/seam only for a verified current need, not a possible later use.
6. Report missing prior inputs or insufficient scope to the planner; stop only affected work. Do not backfill earlier obligations, revise fixed behavior, enlarge the current assignment or silently choose an unowned default.

## Session Start

1. Read the assigned AP, [delivery scope](../authoritative-plan.md#delivery-scope), relevant [approval](../authoritative-plan.md#workflow-and-approval-gates), [document rules](../README.md#change-rules), [permanent documentation](../README.md#permanent-documentation), [naming](../README.md#naming), [tooling](../README.md#skills-agents-and-mcp-servers) and [library policy](../README.md#library-reuse).
2. Follow only relevant REQ/C/F/TECH/decision links and glossary terms. Use targeted sections; do not load source assignments, decision history or unrelated contracts as startup context.
3. Inspect the worktree and actual artifacts. Preserve others' changes. Verify required producer outputs against their handovers and focused evidence; a draft needed slice belongs to the current AP only when explicitly assigned.
4. Create/continue only this AP's execution record. Record the actual baseline revision/delta, start authority, assignee, task IDs and scope references. Plan, code and verify within that scope.

## Immutable AP Baseline

Executing agents do not edit their assignment or earlier results. User-authorized planning revisions update AP documentation directly; record authority and affected scope in the plan. The resulting assignment is the execution baseline. Keep status, commands/results and blockers only in execution records.

## Approved Active-AP Addenda

Existing approved addenda remain applicable to their named assignments. New planning repairs belong inline in the AP, not in sidecar documents. A planner needs explicit authority to change active scope or fixed decisions; this does not require a second permission for work already assigned.

## Downstream Amendments

Identify affected consumer input/task, evidence, owner and missing output. The planner repairs the consumer assignment before it starts; the executing producer does not perform later planning or implementation. Continue independent in-scope work.

## Implementation Decisions and Interface Handover

- Keep interface behavior, schema/version, authorization, fields, errors, time/revisions, transactions and idempotency in its C-ID owner. Define/freeze the current needed slice before coding.
- Keep product disposition/approval in the plan, technology selection in the architecture, terms in the glossary and actual outcomes in the execution record. Link instead of copying.
- Participating domain/integration roles work inside the owning AP. Each producer supplies complete current behavior and verified API/schema/SDK/store/runner handovers; consumers do not invent future paths or versions.
- Link authoritative generated artifacts from their contract; remove competing handwritten schema definitions. Keep private implementation detail in code/tests.

## Mandatory Documentation Maintenance

Every executing agent must update all documentation affected by its in-scope change in the same work step, before dependent work or handover. Domain ownership assigns accountability, not an external documentation chore or permission gate.

Use the [authority matrix](../README.md#document-authority): update the owning contract and generated artifact paths/versions, affected F-cases, terminology, technology/product decisions when assigned, and execution evidence. Update incoming references and current producer/consumer handover links together. An unchanged source needs no edit; never copy definitions merely to show maintenance.

When a change alters implemented behavior, also update the permanent [docs/](../../docs/README.md) section, OpenAPI/proto artifact and Go doc comments it affects, following the [documentation rules](../../CONTRIBUTING.md#documentation). [Permanent documentation](../README.md#permanent-documentation) and [naming](../README.md#naming) rules apply.

APs contain their exact planning questions, boundaries and cases, or direct section/artifact links to them. Use links to the required handover or E-ID, not a whole historical record or instructions to search. Keep prerequisite material distinct from later compatibility context. Delete obsolete duplicate/redirect documents only after moving every still-required unique input to its owner and updating references; preserve actual evidence.

Execution may maintain documentation for behavior already assigned, but cannot change AP tasks/acceptance, fixed earlier behavior or later assignments. If such a revision is needed, record the exact owner/input gap for authorized planning; do not leave an inaccurate current definition presented as valid. Record remaining affected documentation as unresolved and stop only dependent work.

## Execution and Verification

Use existing tooling and the smallest relevant checks. PostgreSQL, HTTP/SSE, browser, provider and operational evidence are distinct; a compile/unit run does not replace them. Record actual commands, versions/environment, results and limitations. Parallel execution requires explicit disjoint scope.

Verify task coverage, references/anchors, permitted diff, fixed behavior and required output shape. Completion also requires all affected current documentation to agree with the delivered behavior and actual artifact versions; record the checked owner sections and any unresolved discrepancy in the execution record. Hand over actual artifact versions/paths and unresolved inputs at the designated execution owner. Never report unexecuted, missing or deferred work as complete.

## Mandatory Continuation and Stop Conditions

An AP execution assignment requires the whole assigned package: its planning, all tasks and supporting integration, actual acceptance verification, documentation maintenance and handover. An explicitly narrower user assignment limits this obligation to that scope.

After every partial result, reassess all remaining assigned work and continue with the next actionable item. Milestones, foundation work, passing focused tests and updated execution records are progress, not reasons to end execution. Repair in-scope implementation or validation failures and rerun the affected checks; the first failed check is not itself an external blocker.

Ask genuine clarification through a structured question tool when needed, while continuing independent work. Missing earlier inputs, approval or planner repairs block only their dependent work. Resume that work once resolved; do not request a second authorization for already assigned work.

An incomplete execution may end only when:

- The user explicitly pauses, cancels or narrows the assignment.
- Every remaining item is blocked by a verified external input, unresolved clarification, required authority, unavailable access/environment or other concrete impediment that cannot be resolved safely within the assigned scope and available tools.
- A hard runtime/tool limit actually prevents further execution. Preserve a truthful checkpoint at the execution owner when possible; do not present it as completion or an external planning blocker.

Package size, complexity, effort already spent, a preferred response length, voluntary timeboxing or an anticipated context limit are not stop conditions. Nor is the absence of a commit, unless the assignment explicitly requires one. Do not defer actionable assigned work to a future session or ask whether to continue an already authorized AP.

Before ending, check every outstanding task, required output, acceptance case and documentation obligation. If any is actionable, continue. Otherwise record the exact stop condition, affected task IDs, evidence, resolution owner and required input/action for resumption in the execution record. Keep verified slices distinct from blocked or unexecuted work; missing owner acceptance remains pending, never inferred. A remaining-work list or an honest "not complete" statement alone does not justify stopping.
