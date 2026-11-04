# Execution and Ownership Rules

[Document authority](../README.md#document-authority) | [Package directory](../work-packages.md) | [Execution records](../execution/README.md)

## Mandatory AP Self-Containment and Planning

These rules are mandatory for every AP author, planner, implementer, reviewer and handover owner.

1. Define an AP so it can be executed against its unchanged baseline. It may rely only on:
   - outputs and decisions from earlier APs that are already completed and have a verifiable handover;
   - already-fixed, applicable contracts and decisions, identified by their canonical source and version; and
   - contract or decision work explicitly assigned to this AP, plus user clarification where the user stories and authoritative sources do not determine the answer.
2. Name every external prerequisite, its producing AP or canonical owner, and the exact fixed output/version required. Do not rely on unowned assumptions, unverified or incomplete handovers, work assigned to a later AP, or an output with no producing AP or owner. Do not use AP numbering alone as evidence that an output is ready.
3. Before an AP is assigned, give every decision or contract slice needed by its tasks a clear owner and an exact scope in that AP. Specify what must be defined, the required extent, its canonical destination and the cases or acceptance evidence that establish completion. Avoid vague directions such as “as applicable,” “as needed,” or “freeze the contract” without enumerating the slice.
4. Planning and contract-definition work assigned to an AP is part of that AP, not an external prerequisite or blocker. Complete it before coding, using the AP's requirements, user stories, fixed decisions and evidence. Do not ask the user how much to plan or report the AP as blocked merely because its own planning remains; do that work first, then code. Existing explicit scope, implementation and experiment gates still apply; they do not create a separate gate for the AP's in-scope planning or contract/decision work.
5. The AP's execution order authorizes decisions and contract changes that are explicitly within its tasks and write scope. Do not ask for permission or separate approval to do that work. Orient decisions to the user stories and record them at their canonical owners. Ask the user only for genuine clarification or to choose among materially different options when the authoritative context and user stories do not provide enough information to decide.
6. Do not make an AP executable by changing its baseline during execution, by backfilling work that was required from an earlier AP, or by implementing a later AP's work. If a prerequisite handover is incomplete, or a gap cannot be resolved within this AP's explicitly assigned planning scope, stop the affected work and report the exact owner, missing output and evidence; continue only independent work. The AP planner must resolve such readiness gaps before assigning the AP.
7. An AP must link all information needed for its planning tasks: relevant user stories/requirements, fixed decisions, owner contracts, architecture/technology constraints, cases, and—where useful—later consumer APs and their contract expectations. Label later-AP references as planning context, not prerequisites or fixed authority. Consider them to make current boundaries compatible, but never rely on unfinished outputs or draft expectations as fixed inputs.
8. Planning must consider relevant later consumers without doing their work early. A later AP's possible need for a technology or reusable function is not, by itself, a reason to adopt that technology or implement that function now. Make the current AP's choices from its own user stories, approved architecture and verified needs; build a shared seam only when the current AP's scope requires it. Do not add a stub, route, adapter or behavior solely to prepare for a later AP.

## Session Start

1. Open the assigned AP's **Session Context**, tasks, prerequisites, write scope, acceptance and non-goals. Treat its current contents as the baseline, not a scratchpad or progress log. Check it against [mandatory AP self-containment and planning](#mandatory-ap-self-containment-and-planning); do not start an AP whose prerequisite ownership or planning scope is undefined.
2. Read the [authority/change rules](../README.md#change-rules), relevant [glossary](../glossary.md) entries and [naming maintenance](../glossary.md#maintenance-rules).
3. Check [scope](../authoritative-plan.md#delivery-scope), [P-ID gates](../authoritative-plan.md#open-issues-and-risks) and any existing explicit scope, execution or experiment gates. Open relevant D-ID owner links and C-ID versions. These gates do not create an extra permission requirement for decisions or contract updates explicitly assigned to this AP.
4. Read the AP-linked REQ sections, TECH/architecture boundaries, owner contracts, [shared conventions](../contracts/shared-conventions.md) and F-ID cases for the slice. Follow only relevant contract dependencies.
5. Open this AP's [execution record](../execution/README.md#record-location) and those of prerequisite producers. If no record exists, there is no recorded execution handover; do not infer readiness from an AP checkbox. Create the current record before execution, not placeholder records for every future AP.
6. Inspect actual source/manifests/artifacts. Verify the AP's exact fixed inputs, prerequisite handovers and runnable focused checks. Complete the AP's bounded planning and contract-definition work before coding; a draft contract is not an external prerequisite when its required slice belongs to this AP. Decide from the user stories and authoritative context, asking only for genuine clarification or an unresolvable choice. Record the result in its canonical owner and freeze the exact required version before implementation. Do not seek separate permission for decisions or contract changes within the AP's tasks and write scope. Keep prior AP results and already-fixed decisions immutable; report a readiness gap instead of changing them. Executable experiments still require AP-03 authorization. Required user clarification and work outside the approved AP scope remain blockers; do not invent a default or silently expand scope.

## Immutable AP Baseline

Do not edit the AP being executed, including task wording/checkmarks, status, commands, results or write scope. Its task definitions and acceptance remain the reference against which execution is judged.

Keep execution state, actual assignee/scope binding, commands/results, blockers, deviations and output references only in the [execution record](../execution/README.md). Do not copy task descriptions, contract decisions or planned case definitions into it: use T/C/D/F/REQ IDs and links to owners.

Resolve contract planning and decisions necessary to satisfy the current AP within its explicitly scoped tasks, using fixed preceding outputs and user-story evidence; do not require a separate package, sign-off or permission merely to define/freeze the current AP's own contract inputs. Keep prior AP results and fixed decisions unchanged. If the AP's existing scope cannot satisfy its acceptance, do not silently rewrite the baseline, backfill another AP's work, or ask permission to do so: report the exact assignment-readiness gap for the planner to resolve before execution. Product-boundary changes outside the AP and active-AP addenda remain subject to their existing approval/change rules.

Legacy checked tasks in AP-01/AP-02 remain untouched as historical marks. Their documentary execution outcomes are indexed under [execution records](../execution/README.md), independently of implementation approval.

## Approved Active-AP Addenda

An active AP definition remains immutable. When a concrete finding requires additional scope that is compatible with its existing task IDs, acceptance and non-goals, Product/integration may approve a separate additive scope addendum at `docs/work-packages/addenda/ap-NN-<slug>.md`. The addendum supplements, but never edits or replaces, the original assignment. This procedure applies only to active APs; it does not reopen completed APs or permit a planning/acceptance revision.

The addendum must identify the unchanged AP baseline revision, evidence and owner, affected existing task IDs, the additive behavior/scope, exact approved write paths, frozen owner inputs, verification cases/commands, exclusions, and impacts on prior decisions and downstream consumers. Mark it as a proposal until the plan records explicit approval of that exact added scope. The addendum does not authorize work outside the AP or its approved addition, changes to earlier fixed results, or separately gated experiments. Decisions and contract work within the AP's authorized scope do not need a separate permission gate.

The plan is the canonical approval record, the approved addendum is canonical for the added scope, and the AP execution record links both while maintaining the assignee, exact execution binding, results and handover. Do not copy the addendum's scope into the original AP or treat the original AP as modified. If the proposed addition changes existing acceptance, non-goals, task IDs, prior results or an approved decision, stop and use a separately approved planning revision or decision-supersession process instead. No work covered only by a proposed addendum may start before its explicit approval and all other required gates; this does not add a separate approval gate for decisions or contract work already within the AP.

## Downstream Amendments

Only a concrete finding from the current execution justifies raising a possible downstream documentation change. Identify the affected consumer task/input/interface, its owner, and evidence; record the finding in the current handover. Do not edit the downstream AP, do not perform its planning or implementation, and do not ask permission to expand scope. The planner must resolve and record any necessary AP-definition change before that AP starts, following the existing approval/change rules. Until resolved, the downstream AP must not rely on the missing or incompatible input.

## Implementation Decisions and Interface Handover

- Put chosen boundary behavior, complete interface forms, field/error/time/revision/idempotency/transaction rules and compatibility in the owning C-ID file, following [shared conventions](../contracts/shared-conventions.md#freeze-requirements).
- Record technology selection, exact selected versions and rationale in the architecture's owning TECH section; reference executable version pins rather than maintaining competing literals once manifests exist.
- Record each product decision's disposition in the plan with a direct owner link. Decisions and contract changes explicitly assigned to this AP are authorized by its execution; do not seek separate permission to make or record them. A change outside the AP, to an earlier fixed result, or across an existing explicit scope gate remains subject to that gate.
- The AP consuming a contract owns planning and deciding the necessary contract slice, relying on fixed prior-AP outputs or user clarification. The domain owner remains accountable for the canonical contract definition; do not turn that ownership assignment into an unlisted prerequisite for a separate AP or independent sign-off. Freeze the exact required slice before implementation. Do not alter an already-fixed prior slice from the current AP; report a readiness gap for the planner instead.
- Decisions within the consuming AP's tasks and write scope are authorized by execution of that AP; do not request additional permission to make or record them. Use the user stories and fixed context to choose the behavior. User input is needed only for a genuine clarification or materially different choice that the AP's authoritative sources do not resolve.
- Keep private implementation mechanics in code/tests; document only externally required usage or non-obvious rationale. Do not create an additional interface catalog that copies contract signatures.
- An owner contract must link actual OpenAPI/schema/SDK artifacts when introduced, identify their authority/version and replace duplicated prose schema detail with references. Producers and consumers must agree on the same artifact.
- The execution record links to changed owners, exact approved contract versions and actual output artifacts. It contains no second definition of a decision or interface.

## Execution and Verification

Integration coordinates shared composition/build/schema-registration boundaries under the [architecture](../architecture.md#supporting-responsibilities); domain owners retain their schema objects. Parallel work needs explicit disjoint scope approval.

Use existing actual test/build tooling. Record exact focused commands, versions/environment, observed exit/results and limits in the execution record. Do not invent runnable commands for absent modules/toolchains. Overall Go tests do not replace real PostgreSQL, HTTP/browser/provider or operational evidence.

Document-only verification checks links/anchors, IDs, ownership, preserved acceptance and the current AP's unchanged diff. End with a persisted handover of artifacts/results and unresolved blockers; a chat-only report is not a handover.
