# Execution and Ownership Rules

[Document authority](../README.md#document-authority) | [Package directory](../work-packages.md) | [Execution records](../execution/README.md)

## Session Start

1. Open the assigned AP's **Session Context**, tasks, prerequisites, write scope, acceptance and non-goals. Treat its current contents as the baseline, not a scratchpad or progress log.
2. Read the [authority/change rules](../README.md#change-rules), relevant [glossary](../glossary.md) entries and [naming maintenance](../glossary.md#maintenance-rules).
3. Check [scope](../authoritative-plan.md#delivery-scope), [P-ID gates](../authoritative-plan.md#open-issues-and-risks) and [approval](../authoritative-plan.md#workflow-and-approval-gates). Open relevant D-ID owner links and the C-ID versions; creating documentation or marking a task does not grant approval.
4. Read the AP-linked REQ sections, TECH/architecture boundaries, owner contracts, [shared conventions](../contracts/shared-conventions.md) and F-ID cases for the slice. Follow only relevant contract dependencies.
5. Open this AP's [execution record](../execution/README.md#record-location) and those of prerequisite producers. If no record exists, there is no recorded execution handover; do not infer readiness from an AP checkbox. Create the current record before execution, not placeholder records for every future AP.
6. Inspect actual source/manifests/artifacts. Verify named assignee, approved scope/version binding, prerequisite outputs and runnable focused checks. Necessary planning and contract decisions are part of the AP that consumes them; a draft contract is not, by itself, an external prerequisite that blocks the AP. Resolve the required decisions from approved outputs/decisions of preceding APs or obtain user clarification where genuinely needed. Maintain the result in its canonical owner contract and freeze the exact AP-required version before relying on it for implementation. Keep prior AP results and already-frozen slices immutable unless the explicit earlier-result/change gate is satisfied. Executable experiments still require AP-03 authorization. Missing required user input or an unapproved scope expansion remains a blocker; do not invent a default.

## Immutable AP Baseline

Do not edit the AP being executed, including task wording/checkmarks, status, commands, results or write scope. Its task definitions and acceptance remain the reference against which execution is judged.

Keep execution state, actual assignee/scope binding, commands/results, blockers, deviations and output references only in the [execution record](../execution/README.md). Do not copy task descriptions, contract decisions or planned case definitions into it: use T/C/D/F/REQ IDs and links to owners.

If the current AP's baseline cannot be satisfied, record the discrepancy and block the affected work. Resolve contract planning and decisions necessary to satisfy the current AP within its task scope, using preceding AP outputs or user clarification; do not require a separate package merely to plan/freeze the current AP's own contract inputs. Product/integration must explicitly resolve a changed product boundary or obtain approval for any compatible active-AP addendum; use a separate planning revision if the existing acceptance cannot be met. The executing session must not silently rewrite its own acceptance. This also applies when a prerequisite or write-scope assumption proves wrong.

Legacy checked tasks in AP-01/AP-02 remain untouched as historical marks. Their documentary execution outcomes are indexed under [execution records](../execution/README.md), independently of implementation approval.

## Approved Active-AP Addenda

An active AP definition remains immutable. When a concrete finding requires additional scope that is compatible with its existing task IDs, acceptance and non-goals, Product/integration may approve a separate additive scope addendum at `docs/work-packages/addenda/ap-NN-<slug>.md`. The addendum supplements, but never edits or replaces, the original assignment. This procedure applies only to active APs; it does not reopen completed APs or permit a planning/acceptance revision.

The addendum must identify the unchanged AP baseline revision, evidence and owner, affected existing task IDs, the additive behavior/scope, exact approved write paths, frozen owner inputs, verification cases/commands, exclusions, and impacts on prior decisions and downstream consumers. Mark it as a proposal until the plan records explicit approval of that exact artifact and scope. Product decisions, contract freezes, technology approvals and implementation starts remain separate gates; a scope addendum alone grants none of them.

The plan is the canonical approval record, the approved addendum is canonical for the added scope, and the AP execution record links both while maintaining the assignee, exact execution binding, results and handover. Do not copy the addendum's scope into the original AP or treat the original AP as modified. If the proposed addition changes existing acceptance, non-goals, task IDs, prior results or an approved decision, stop and use a separately approved planning revision or decision-supersession process instead. No work covered only by a proposed addendum may start before its explicit approval and all other required gates.

## Downstream Amendments

Only a concrete finding from the current execution justifies changing a following AP:

1. Identify the affected future task/input/interface and its owning contract or architecture decision.
2. Record finding, evidence link, affected AP/task and owner in the current execution record. Store the actual decision/contract detail only in its authoritative source.
3. Amend only the affected **not-yet-started** downstream AP, preserving stable IDs and linking to that owner detail. "Downstream" means an actual consumer, not merely a higher AP number. A concurrently active/completed AP is not editable under this exception.
4. Record the amendment path and rationale reference in the producer's handover, notify integration/consumer, and obtain approval if scope/version changes. The consumer checks this handover before starting.

This rule does not permit implementation of an unapproved extension or a silent expansion of write scope.

## Implementation Decisions and Interface Handover

- Put chosen boundary behavior, complete interface forms, field/error/time/revision/idempotency/transaction rules and compatibility in the owning C-ID file, following [shared conventions](../contracts/shared-conventions.md#freeze-requirements).
- Record technology selection, exact selected versions and rationale in the architecture's owning TECH section; reference executable version pins rather than maintaining competing literals once manifests exist.
- Record product decision disposition/approval in the plan with a direct owner link. New defaults, changed permissions or changed product behavior require that approval, not just an implementation note.
- The AP consuming a contract owns planning and deciding the necessary contract slice, relying on approved prior-AP outputs or user clarification. The domain owner remains accountable for the canonical contract definition; do not turn that ownership assignment into an unlisted prerequisite for a separate AP or independent sign-off. Freeze the exact required slice before implementation, and use the explicit earlier-result gate for any change to an already-frozen prior slice.
- Keep private implementation mechanics in code/tests; document only externally required usage or non-obvious rationale. Do not create an additional interface catalog that copies contract signatures.
- An owner contract must link actual OpenAPI/schema/SDK artifacts when introduced, identify their authority/version and replace duplicated prose schema detail with references. Producers and consumers must agree on the same artifact.
- The execution record links to changed owners, exact approved contract versions and actual output artifacts. It contains no second definition of a decision or interface.

## Execution and Verification

Integration coordinates shared composition/build/schema-registration boundaries under the [architecture](../architecture.md#supporting-responsibilities); domain owners retain their schema objects. Parallel work needs explicit disjoint scope approval.

Use existing actual test/build tooling. Record exact focused commands, versions/environment, observed exit/results and limits in the execution record. Do not invent runnable commands for absent modules/toolchains. Overall Go tests do not replace real PostgreSQL, HTTP/browser/provider or operational evidence.

Document-only verification checks links/anchors, IDs, ownership, preserved acceptance and the current AP's unchanged diff. End with a persisted handover of artifacts/results and unresolved blockers; a chat-only report is not a handover.
