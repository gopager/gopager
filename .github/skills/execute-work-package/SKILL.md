---
name: execute-work-package
description: "Execute an AP or work package within its assigned scope. Use when starting, implementing, resuming or completing an AP. Verify earlier inputs, perform bounded own planning, route readiness gaps to the planner and produce truthful evidence."
argument-hint: "AP ID or assignment path"
---

# Controlled Work Package Execution

## Required Context

Read the assigned AP and relevant sections of:

- [Shared execution rules](../../../docs/work-packages/shared-rules.md)
- [Document authority](../../../docs/README.md#document-authority)
- [Evidence format](../../../docs/execution/README.md)

These are the canonical rules. Follow task-specific links for requirements, fixed contracts, decisions, glossary terms and exact producer handovers. Do not preload source assignments, unrelated contracts or change history.

## Procedure

1. Establish the selected assignment, worktree baseline, current start authority and execution record. Preserve existing changes.
2. Verify every external input against an actual completed earlier producer. Own explicitly assigned planning is work before coding, not an external blocker.
3. Define the assigned contract/decision slices at their canonical owners before using them. Execution authorizes this in-scope work; do not request additional permission or ask how much to plan.
4. Implement only current tasks and supporting write paths. Preserve fixed earlier behavior. Later consumers are compatibility context, not unfinished inputs or work to perform early.
5. Run the smallest checks covering the actual acceptance cases. Distinguish PostgreSQL, HTTP/SSE, browser, real provider and operational evidence from fixtures/compile checks.
6. Apply [mandatory documentation maintenance](../../../docs/work-packages/shared-rules.md#mandatory-documentation-maintenance) in every work step. Record results and hand over artifact paths/versions, producer tasks, consumer inputs, limitations and gaps in the execution record; do not copy definitions or task wording.

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

## Completion Check

- Assigned tasks and required outputs have actual acceptance evidence.
- Diff stays within scope and preserves others' changes and fixed behavior.
- Decisions, terminology and generated API/schema/SDK authority are consistent.
- Producer/consumer references and handovers are complete.
- All affected current owner documentation and targeted links match the actual output; discrepancies remain explicitly unresolved.
- Missing, deferred or unexecuted work is explicitly unresolved.

Report task status, evidence and remaining blockers concisely. No extra planning/result sidecar or blanket completion claim.
