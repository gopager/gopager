---
name: execute-work-package
description: "Execute work packages (APs) systematically and strictly within approved scope. Use when starting, implementing, resuming, or completing an AP or work package. Protects AP definitions and prior results, routes readiness gaps to the planner, and enforces structured decisions, glossary maintenance, and unambiguous information ownership."
argument-hint: "AP ID or path to the work package"
---

# Controlled Work Package Execution

## Purpose and Boundaries

Execute only the tasks of the selected, approved work package (AP).
The AP execution order authorizes its defined tasks; this skill does not enlarge
that scope. Follow existing gates for work that the AP does not authorize,
including separately gated experiments. Never revise earlier fixed results
from the current AP; report a readiness gap instead.

Mandatory boundaries:

- An AP may depend only on verified outputs/decisions from completed earlier
  APs, already-fixed applicable contracts/decisions, and planning/contract/
  decision work explicitly assigned to itself. Never rely on unowned assumptions,
  incomplete handovers, later AP work, or an output with no owner.
- Planning assigned to the current AP is work to do before coding, not an
  external prerequisite or blocker. Its exact scope must be stated by the AP;
  do not ask the user how much to plan.
- The AP must link all information needed for its planning tasks. Consider
  relevant later-consumer APs and contracts as planning context only: use them
  to avoid incompatible boundaries, never as prerequisites or a reason to
  implement later work early.
- Do not choose a technology or add a function, route, adapter, or behavior
  solely because a later AP might need it. Follow the current AP's user stories,
  fixed architecture, and verified needs; add a shared seam only when required
  by the current scope.
- Execution of the AP authorizes decisions and contract changes explicitly
  within its tasks and write scope. Do not ask for permission or separate
  approval for that work. Use user stories and authoritative context to decide;
  ask only for genuine clarification or when materially different options
  cannot be resolved from that context.
- Do not edit the AP during execution, backfill work assigned to an earlier AP,
  or implement later-AP work. If the AP's own planning scope is missing or an
  earlier handover is incomplete, report the exact readiness gap rather than
  silently changing the assignment or expanding scope.
- The AP definition is an immutable baseline. Do not change or add tasks,
  acceptance criteria, checkboxes, status information, or results within it.
- Do not change results or artifacts from earlier APs. If a needed earlier
  result is incomplete or incompatible, record the exact readiness gap; do not
  ask permission to backfill or revise it.
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

1. If the AP ID remains ambiguous after checking the assignment and its
   references, use the clarification procedure in section 5. Do not select
   an AP yourself or infer execution order solely from its number.
2. Read applicable `AGENTS.md` files, documentation rules, the selected AP,
   its prerequisites, referenced decisions, and handovers.
3. Inspect the working tree and existing local changes. Preserve changes made
   by others; do not overwrite a conflicting change. If the conflict leaves
   the intended scope or behavior ambiguous, ask only for that clarification.
4. Verify each AP-listed prerequisite against the exact earlier handover or
   fixed decision/contract version it names. Planning and contract decisions
   owned by the current AP are not prerequisites; do that work before coding.
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
   and the information owner may differ; the AP authorizes in-scope decisions
   and contract updates without a separate permission request.
3. Use the AP's named owner and canonical location. If either is missing or
   ambiguous, resolve it from repository conventions; if it remains unresolved,
   report the exact assignment-readiness gap instead of inventing an owner or
   asking for permission. Ask the user only when genuine clarification is needed.
4. Maintain content only at its canonical location; use stable IDs and links
   in evidence records and handovers. Do not create parallel decision lists,
   replacement glossaries, or copies of contract definitions.

Normative definitions, execution findings, and historical evidence are different
information types. Preserve historical observations with their original context;
they are not a second current definition. Report existing redundancy or
inconsistency, but do not clean it up outside the assignment.

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
sources. Use the existing decision format and the owner/location assigned by
the AP and repository rules. If no decision record exists, create one at the
canonical owner when the AP requires it; do not seek permission for that
in-scope work.

Every decision must traceably contain the following information or link to
its canonical source:

- stable ID, date, originating AP, and task ID;
- status: proposed, approved, rejected, or superseded;
- decision question, context, and reliable evidence;
- options considered, selected option, and rationale;
- accountable owner and required approval, if any; identify the source of
  authorization and exact scope. The AP itself authorizes its in-scope contract
  and decision work; do not seek additional permission;
- affected earlier decisions, current artifacts, and downstream APs;
- consequences, required changes, and their canonical locations;
- when superseding a decision, the superseded decision ID and traceable history.

The product or technical definition remains in its owning source.
The decision record documents the resolution and rationale, plus any required
approval or the AP authority for in-scope work; the AP execution record links to
it. Do not repeat the same content in all three places.

## 5. Consultation and Change Gate

### Resolve Context Before Asking

Before asking for input, check the selected AP and its relevant referenced
sources, including existing decisions and approvals. Do not ask questions whose
answers are already explicit or clearly implicit in that context. Apply the
established answer and reference its source; do not reopen a settled decision.
A clearly implied answer does not substitute for a required explicit approval
where an existing gate applies.

Ask the user only for genuine clarification or a choice among materially
different options when the AP's user stories and authoritative sources do not
determine the answer. Do not ask permission to make a decision or update a
contract explicitly within the AP's tasks and write scope, or ask the user to
perform checks the agent can carry out itself. Existing approval gates still
govern work outside that scope and separately gated experiments. Never seek
permission to alter an earlier fixed result from the current AP; report the
readiness gap instead.

### Structure Necessary Questions

Use the clarification tool to present structured questions:

- Each question must address exactly one topic or decision, with its own answer
  field. Never bundle independently answerable topics or decisions into one
  question, field, or combined option.
- Combine aspects only when they are so tightly coupled that separating them
  would make the answer less meaningful. A shared AP, artifact, or approval
  request alone does not justify combining independent decisions.
- For each question, identify the unresolved point, relevant source references,
  why input is needed, and which work depends on the answer.
- Always provide enough context with each question to make the decision, wherever
  possible, without consulting other documents. Include the relevant current
  state, constraints, options, and consequences or trade-offs; do not assume the
  user remembers earlier discussion or knows the referenced IDs.
- If the necessary context is too extensive to include with the question,
  summarize the decision-relevant facts inline and provide direct links to the
  specific source sections containing the remaining context. Bare IDs, unlinked
  file names, or a generic instruction to read the documentation are insufficient.
- Provide focused options and a recommendation with rationale where useful;
  allow another answer.
- Independent questions may share one structured form, but must remain separate
  questions and answer fields. Ask dependent follow-up questions only after the
  prerequisite answer is known.
- Record the clarified answer or selected option at its canonical location and
  link it from the execution record. If clarification is unavailable, block
  only the affected work and continue independent tasks within the AP; never
  invent an answer.

### Earlier Results or Decisions

Whenever a finding suggests that results from an earlier AP need to change, or
that a decision made there should be reaffirmed, reinterpreted, restricted, or
replaced:

1. Stop the affected work step before implementation.
2. Identify the existing source and decision ID, current finding, proposed
   change, alternatives, and impacts explicitly.
3. Do not change the earlier material or ask permission to do so. Record the
   exact missing or conflicting input as a readiness gap for the planner to
   resolve through the applicable planning/change process before dependent work
   proceeds.
4. Continue only independent tasks that remain within the current AP.
   Clarification may be requested only when the authoritative context and user
   stories leave a genuine ambiguity or materially different choice.

### Impacts on Downstream APs

If a finding or decision would make a dependent AP's instructions inaccurate:

1. Identify the specific APs and tasks affected. Read them only to assess
   impacts, not to execute them.
2. Record the discrepancy, evidence, owner, and required downstream input in
   the current handover. Do not edit the downstream AP, perform its planning,
   or request permission to expand scope.
3. The downstream AP's planner must resolve any required assignment change
   before that AP starts. Until then, the downstream AP cannot rely on the
   affected input.
4. Ask the user only if the current decision itself remains ambiguous or
   requires choosing among materially different options that the stories and
   authoritative sources do not resolve.

## 6. Maintain Consistent Terminology

1. Check the canonical glossary before introducing or changing a term.
2. Record new terms, meaning changes, renames, and relevant aliases using its
   format; make the owner and definition source unambiguous. Link technical
   contract details rather than copying them.
3. Assess impacts on usages in documentation, code, schemas, tests, fixtures,
   and user interfaces, where present.
4. Do not change previously fixed meanings or earlier results from the current
   AP. Record an out-of-scope terminology dependency as a readiness gap for its
   owner/planner.
5. If consistent usage cannot be achieved within approved scope, block the
   terminology change rather than introducing it as completed.

## 7. Verify Completion and Handover

Before reporting completion, check the actual diff and evidence:

- Every completed task is traceable to an AP task ID and appropriate evidence;
  unfinished or unverified items are visible.
- The current AP definition is unchanged.
- No earlier results or decisions were changed; authorized addenda preserve
  history.
- No downstream AP task was implemented.
- All decisions are structured and unambiguously assigned to their owners.
- Any downstream readiness gap is recorded for its planner; no downstream AP
  was amended or executed by this AP.
- New and changed terms are recorded in the glossary and used consistently.
- Information is maintained at its canonical location; other places link to it.
- Tests, commands, and observations are recorded with their actual results.
  Do not report checks that were not run as passed.

Finish with a concise report: AP and execution status, completed task IDs,
evidence links, decision links, readiness gaps, blockers, and the next
permitted step. Do not create a second result
documentation source alongside the canonical execution record.
