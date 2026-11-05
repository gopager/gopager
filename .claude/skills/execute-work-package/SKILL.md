---
name: execute-work-package
description: "Execute an entire AP or work package to verified completion within its assigned scope. Use when starting, implementing, resuming or completing an AP. Continue through all assigned tasks, verification and handover; ask genuine clarifications and stop incomplete only under the canonical stop conditions."
argument-hint: "AP ID or assignment path"
---

# Controlled Work Package Execution

## Required Context

Read the assigned AP and relevant sections of:

- [Shared execution rules](../../../development/work-packages/shared-rules.md)
- [Document authority](../../../development/README.md#document-authority)
- [Evidence format](../../../development/execution/README.md)

Apply the [permanent documentation](../../../development/README.md#permanent-documentation), [naming](../../../development/README.md#naming) and [tooling](../../../development/README.md#skills-agents-and-mcp-servers) rules. These are the canonical rules. Follow task-specific links for requirements, fixed contracts, decisions, glossary terms and exact producer handovers. Do not preload source assignments, unrelated contracts or change history.

## Procedure

1. Establish the selected assignment, worktree baseline, current start authority and execution record. Preserve existing changes. The execution unit is the entire assigned AP, not a convenient task or implementation slice, unless the user explicitly narrows it.
2. Verify every external input against an actual completed earlier producer. Own explicitly assigned planning is work before coding, not an external blocker.
3. Define the assigned contract/decision slices at their canonical owners before using them. Execution authorizes this in-scope work; do not request additional permission or ask how much to plan.
4. Implement only current tasks and supporting write paths. Preserve fixed earlier behavior. Later consumers are compatibility context, not unfinished inputs or work to perform early.
5. Run the smallest checks covering the actual acceptance cases. Distinguish PostgreSQL, HTTP/SSE, browser, real provider and operational evidence from fixtures/compile checks.
6. Apply [mandatory documentation maintenance](../../../development/work-packages/shared-rules.md#mandatory-documentation-maintenance) in every work step. Record results and hand over artifact paths/versions, producer tasks, consumer inputs, limitations and gaps in the execution record; do not copy definitions or task wording.
7. After each task, partial slice or check, reconcile the execution record with every assigned task, required output and acceptance case. Select the next actionable in-scope item and repeat planning, implementation, verification and documentation until the completion check passes. Follow the canonical [continuation and stop conditions](../../../development/work-packages/shared-rules.md#mandatory-continuation-and-stop-conditions).

Progress updates and execution checkpoints are intermediate state, not final handovers. Passing focused tests for a foundation slice does not discharge the remaining AP tasks or acceptance checks.

## Decisions

At the canonical decision owner, record:

- Stable ID, date, AP/task, status and accountable owner.
- Question, user-story context, evidence, alternatives, selected option and rationale.
- Authorization and exact scope; current-AP planning needs no separate approval.
- Affected contracts/artifacts/consumers and consequences.
- Supersession reference when explicitly authorized.

Product decisions belong in the plan, technology selections in the architecture, interface behavior in the C-ID contract and terms in the glossary. Link between them; do not maintain parallel definitions.

## Clarification and Gaps

Resolve authoritative context before asking. Ask only for genuine missing information or an unresolvable materially different choice. Use a structured question tool with one field per independent topic, decision-relevant context, source links and consequences. Do not ask the user to perform checks available to the agent.

If scope or a prior input is incomplete, stop the affected step and report its owner, missing output, evidence and consumer task to the planner. Continue independent work. Do not revise the assignment, backfill an earlier AP, change fixed decisions or implement later work from an execution session. Authorized planning revisions are a separate activity.

If clarification is unavailable, leave only affected work blocked; never invent a default. Record the resolved answer at its canonical owner before dependent coding.

An unanswered question or planner gap does not justify ending the AP while independent work remains actionable. Once the answer or required input is available, resume the affected work without requesting renewed permission for the existing assignment.

## Completion Check

- Assigned tasks and required outputs have actual acceptance evidence.
- Diff stays within scope and preserves others' changes and fixed behavior.
- Decisions, terminology and generated API/schema/SDK authority are consistent.
- Producer/consumer references and handovers are complete.
- All affected current owner documentation and targeted links match the actual output; discrepancies remain explicitly unresolved.
- Missing, deferred or unexecuted work is explicitly unresolved.

Before a final response, check the remaining work again. If any in-scope item is actionable, continue execution instead of reporting a partial result.

Report task status and evidence concisely only after verified completion or a canonical stop condition. For an incomplete stop, identify the exact reason, affected tasks, blocker evidence, resolution owner and input/action needed to resume; distinguish verified slices from unresolved work. A list of remaining tasks alone is not a stop justification. No extra planning/result sidecar or blanket completion claim.
