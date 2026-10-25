---
name: execute-work-package
description: "Execute work packages (APs) systematically and strictly within approved scope. Use when starting, implementing, resuming, or completing an AP or work package. Protects AP definitions and prior results, requires consultation before revisiting earlier decisions or changing downstream APs, and enforces structured decisions, glossary maintenance, and unambiguous information ownership."
argument-hint: "AP ID or path to the work package"
---

# Controlled Work Package Execution

## Purpose and Boundaries

Execute only the tasks of the selected, approved work package (AP).
This skill does not itself grant product, technical, or experimental approval.
Follow the applicable project rules and approval gates.

Mandatory boundaries:

- The AP definition is an immutable baseline. Do not change or add tasks,
  acceptance criteria, checkboxes, status information, or results within it.
- Do not change results or artifacts from earlier APs without prior explicit
  consultation. Supposed corrections are changes too.
- Do not pull forward tasks from later APs, implement speculative improvements,
  or create execution records for APs that have not started.
- Document decisions before using them as the basis for further work.
  A proposal is not an approved decision.
- Record new or changed terms in the canonical glossary before or with their
  first use.
- Each distinct piece of information has exactly one accountable owner and
  one canonical location. Other documents link to that source rather than
  maintaining a second copy.

## 1. Establish the Assignment, Rules, and Baseline

1. If the AP ID is ambiguous, use the clarification tool to ask. Do not select
   an AP yourself or infer execution order solely from its number.
2. Read applicable `AGENTS.md` files, documentation rules, the selected AP,
   its prerequisites, referenced decisions, and handovers.
3. Inspect the working tree and existing local changes. Preserve changes made
   by others; consult before writing if there is a conflict.
4. Verify prerequisites and required approvals against their evidence.
   Do not present historical documentary findings as current runtime validation.
5. In the execution record, reference the AP source and its verifiable state
   (e.g. commit and existing local diff), task IDs, acceptance criteria, allowed
   write scope, non-goals, and approvals. Do not copy task wording.
6. Treat unclear or contradictory assignments as blockers. Do not resolve them
   through implicit assumptions or by changing the AP definition.

## 2. Identify Canonical Information Locations and Owners

In gopager, responsibilities are governed by the
[documentation authority](../../../docs/README.md),
[shared AP rules](../../../docs/work-packages/shared-rules.md), and
[execution rules](../../../docs/execution/README.md).
Read these sources for every execution; do not reproduce their detailed rules here.

Before creating or changing any content:

1. Search for an existing definition, decision, or piece of evidence.
2. Identify the accountable owner (role, person, or responsible module according
   to project conventions) and canonical document location. The executing agent
   and the owner authorized to approve are not automatically the same.
3. If the owner or location is missing or ambiguous, consult before creating
   a new source or an authoritative statement.
4. Maintain content only at its canonical location; use stable IDs and links
   in evidence records and handovers. Do not create parallel decision lists,
   replacement glossaries, or copies of contract definitions.

Normative definitions, execution findings, and historical evidence are different
information types. Preserve historical observations with their original context;
they are not a second current definition. Report existing redundancy or
inconsistency, but do not clean it up outside the assignment or without
required approval.

## 3. Execute the Current AP in Controlled Steps

1. Create or continue only the execution record of the AP actually being started
   or resumed. Use the existing format.
2. Maintain a work plan referencing the AP's task IDs. Derive only necessary
   subtasks within those tasks.
3. Before each change, check:
   - Which current AP task requires it?
   - Is it within the allowed write scope?
   - Does it affect earlier results or decisions?
   - Does it invalidate a downstream AP's instructions?
   - Does it change terms or their meanings?
   - Are the owner and canonical location unambiguous?
4. Execute, verify against the acceptance criterion, and record evidence at the
   designated location. Use existing testing and evidence procedures.
5. Report unrequested improvements and future work only as referenceable findings
   at the designated location; do not implement them.

## 4. Record Structured Decisions

Before making a decision, search for existing decision IDs and their owning
sources. Use the existing decision format. If none exists, first clarify
ownership and location.

Every decision must traceably contain the following information or link to
its canonical source:

- stable ID, date, originating AP, and task ID;
- status: proposed, approved, rejected, or superseded;
- decision question, context, and reliable evidence;
- options considered, selected option, and rationale;
- accountable owner and required approval, identifying approver and scope;
- affected earlier decisions, current artifacts, and downstream APs;
- consequences, required changes, and their canonical locations;
- when superseding a decision, the superseded decision ID and traceable history.

The product or technical definition remains in its owning source.
The decision record documents the resolution, rationale, and approval;
the AP execution record links to it. Do not repeat the same content in all
three places.

## 5. Consultation and Change Gate

### Earlier Results or Decisions

Whenever a change affects results from an earlier AP, or is intended to
reaffirm, reinterpret, restrict, or replace a decision made there:

1. Stop the affected work step before implementation.
2. Identify the existing source and decision ID, current finding, proposed
   change, alternatives, and impacts explicitly.
3. Use the clarification tool to obtain explicit approval from the accountable
   owner. A general instruction such as "implement the AP" is not approval
   to revise earlier results or decisions.
4. Without a response, or if approval is denied, change none of the affected
   material. Document the blocker; continue only independent tasks that remain
   approved.
5. After approval, implement only the approved scope and reference the approval.
   Do not overwrite earlier evidence; use existing reopening, addendum, and
   supersession rules.

### Impacts on Downstream APs

If a finding or proposed decision would make a dependent AP's instructions
inaccurate:

1. Identify the specific APs and tasks affected. Read them only to assess
   impacts, not to execute them.
2. Present the discrepancy, evidence, owner, and required instruction change.
3. Obtain explicit approval for BOTH the decision and the necessary downstream
   AP amendments. Do not infer automatic authority to make changes.
4. Only then amend the approved, not-yet-started downstream APs according to the
   project's change procedure. This is an explicitly approved baseline amendment,
   not ordinary AP execution or implementation of future work.
5. Do not rewrite active or completed APs through this procedure.
   Clarify separate approval and reopening or addendum rules compatible with
   the project rules.
6. Approved follow-up amendments are mandatory: do not report the originating
   decision as fully implemented or the current AP as completed while necessary
   amendments are missing. While approval is pending, the affected part remains
   blocked.

Approvals must be specific and traceable later. Rejection, silence, or a missing
clarification tool is not consent; stop with an explicit blocker report
when necessary.

## 6. Maintain Consistent Terminology

1. Check the canonical glossary before introducing or changing a term.
2. Record new terms, meaning changes, renames, and relevant aliases using its
   format; make the owner and definition source unambiguous. Link technical
   contract details rather than copying them.
3. Assess impacts on usages in documentation, code, schemas, tests, fixtures,
   and user interfaces, where present.
4. Change previously approved meanings and affected earlier results only after
   passing the consultation gate. Do not independently propagate necessary
   changes outside the current AP.
5. If consistent usage cannot be achieved within approved scope, block the
   terminology change rather than introducing it as completed.

## 7. Verify Completion and Handover

Before reporting completion, check the actual diff and evidence:

- Every completed task is traceable to an AP task ID and appropriate evidence;
  unfinished or unverified items are visible.
- The current AP definition is unchanged.
- Earlier results and decisions were not changed without approval;
  authorized addenda preserve history.
- No downstream AP task was implemented.
- All decisions are structured and unambiguously assigned to their owners.
- All approved, necessary downstream AP amendments are complete;
  pending approvals or conflicts prevent completion of the affected work.
- New and changed terms are recorded in the glossary and used consistently.
- Information is maintained at its canonical location; other places link to it.
- Tests, commands, and observations are recorded with their actual results.
  Do not report checks that were not run as passed.

Finish with a concise report: AP and execution status, completed task IDs,
evidence links, decision and approval links, approved downstream amendments,
blockers, and the next permitted step. Do not create a second result
documentation source alongside the canonical execution record.
