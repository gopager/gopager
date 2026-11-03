# AP Execution Records

This directory owns execution state and evidence, not task definitions, decisions, contract detail or scope approval. Follow the [execution procedure](../work-packages/shared-rules.md); AP baselines stay read-only during execution.

## Record Location

For AP-NN, use `docs/execution/ap-NN.md`. Create it when that package starts. An absent record means no recorded execution, not approval or completion. Follow the same path rule for prerequisite producers; do not create an independent status matrix.

The [package directory](../work-packages.md#revised-execution-path) is the authoritative order and assignment index. The former AP-06..AP-16 records are preserved as `source-ap-*` historical evidence; reconciliation notes identify transferred partial outputs. [AP-06](ap-06.md) is the sole current execution record. The former AP-17 record is preserved as [source AP-17](source-ap-17.md). Do not infer current state from legacy `in-progress` rows in source records.

## Required Record Structure

Use these headings in each record; keep task descriptions and technical rules at their owners:

- **Baseline:** Link the AP and identify its starting Git revision plus any relevant uncommitted baseline delta; never claim a clean commit baseline when the AP was already modified. Link the exact scope/experiment approval in the plan. Name the assignee and approved write-scope binding (reference the AP unless approval changes it). Missing approval remains explicit.
- **Task Outcomes:** T-ID, state (`not-started`, `in-progress`, `blocked`, `verified`, `accepted`), output links and evidence references. `Verified` requires actual evidence; `accepted` additionally requires the responsible owner's acceptance. `Accepted` here is task acceptance, not scope approval. Partial slices are identified explicitly.
- **Evidence:** Give each check a local heading such as `E-01`. Record date, exact command/procedure, tool/runtime versions, environment, observed result/exit and limitations. Link logs/artifacts when they exist; do not claim a planned fixture or unexecuted command passed.
- **Blockers and Deviations:** Finding, affected T/REQ/C/P IDs, responsible owner and resolution reference. A changed decision lives at its owner, not in this section. No unresolved record may be presented as success.
- **Handover:** Artifact/API/schema/test paths and versions, producing slice, consuming AP/task, evidence links and outstanding prerequisites. Link any downstream AP amendment and its owner rationale; do not reproduce it.

After closure, update only through an explicit follow-up/reopened execution entry with reason/date and preserved earlier evidence. Do not overwrite an earlier observed result to make it match later behavior.

Source provenance and deliberate reuse assessments stay in [source notes](../requirements-evidence.md); actual AP validation results stay here. Large raw logs may be linked from a suitable artifact location; do not make a second hand-maintained result summary.
