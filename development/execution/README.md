# AP Execution Records

This directory owns execution state and evidence, not task definitions, decisions, contract detail or scope approval. Follow the [execution procedure](../work-packages/shared-rules.md); AP baselines stay read-only during execution.

## Record Location

For AP-NN, use `development/execution/ap-NN.md`. Create it when that package starts. An absent record means no recorded execution, not approval or completion. Follow the same path rule for prerequisite producers; do not create an independent status matrix.

Records written before 2026-11-06 refer to this tree under its former path `docs/` (for example `docs/execution/ap-NN.md` or `git diff -- docs/work-packages` in recorded commands) and to former test identifiers. Those commands and results are preserved as observed; read `docs/` there as today's `development/`. Renamed test identifiers ([D-132](../authoritative-plan.md#documentation-and-naming-separation)): environment variables `GOPAGER_AP05_PROCESS_HELPER` → `GOPAGER_TEST_PROCESS_HELPER` and `GOPAGER_AP05_TEST_LISTEN` → `GOPAGER_TEST_LISTEN`; test schema prefixes `ap05_`, `ap07_`, `ap08_` → `test_`, `test_cli_`, `test_http_`; trigger `ap07_intake_failpoint` → `test_intake_failpoint`; regex test helper `d61CaptureExpansion` → `captureExpansionCounterexample`. Recorded numbered domain IDs correspond to the fixed [domain names](../architecture.md#domains-and-data-ownership).

The [package directory](../work-packages.md#revised-execution-path) owns order and assignment navigation. Read prerequisite records only for their required handover and evidence. Do not preload unrelated execution history or create records for packages that have not started.

## Required Record Structure

Use these headings in each record; keep task descriptions and technical rules at their owners:

- **Baseline:** Link the AP and identify its starting Git revision plus any relevant uncommitted baseline delta; never claim a clean commit baseline when the AP was already modified. Link the exact scope/experiment approval in the plan. Name the assignee and approved write-scope binding (reference the AP unless approval changes it). Missing approval remains explicit.
- **Task Outcomes:** T-ID, state (`not-started`, `in-progress`, `blocked`, `verified`, `accepted`), output links and evidence references. `Verified` requires actual evidence; `accepted` additionally requires the responsible owner's acceptance. `Accepted` here is task acceptance, not scope approval. Partial slices are identified explicitly.
- **Evidence:** Give each check a local heading such as `E-01`. Record date, exact command/procedure, tool/runtime versions, environment, observed result/exit and limitations. Link logs/artifacts when they exist; do not claim a planned fixture or unexecuted command passed.
- **Blockers and Deviations:** Finding, affected T/REQ/C/P IDs, responsible owner and resolution reference. A changed decision lives at its owner, not in this section. No unresolved record may be presented as success.
- **Handover:** Artifact/API/schema/test paths and versions, producing slice, consuming AP/task, evidence links, checked documentation owner sections and unresolved discrepancies/prerequisites. Link any authorized downstream amendment and its rationale; do not reproduce it. Completion requires [documentation maintenance](../work-packages/shared-rules.md#mandatory-documentation-maintenance), not only passing code tests.

After closure, update only through an explicit follow-up/reopened execution entry with reason/date and preserved earlier evidence. Do not overwrite an earlier observed result to make it match later behavior.

Source provenance and deliberate reuse assessments stay in [source notes](../requirements-evidence.md); actual AP validation results stay here. Large raw logs may be linked from a suitable artifact location; do not make a second hand-maintained result summary.

## Prerequisite Evidence

These records own unique checks from prerequisite slices; they are evidence, not current assignments or current blockers. Read only the handover/E-ID linked by the consuming task.

| Slice | Evidence | Current continuation |
| --- | --- | --- |
| Identity/AuthZ core and global fields | [Core handover](source-ap-07.md#handover), [field correction](source-ap-07.md#unscoped-field-correction-verification-and-handover) | AP-14 remaining identity/lifecycle |
| Unit/collection/current matcher | [D-60 checks](source-ap-08.md#e-03-postgresql-race-http-and-proxy-validation) | AP-08 remaining master-data/correction |
| Processing/publication prerequisites | Individual E-IDs in [Processing evidence](source-ap-12.md#evidence) | [AP-06 final handover](ap-06.md#handover) |
| Intake planning observations | Individual E-IDs in [intake observations](source-ap-06.md#evidence) | [AP-07 final handover](ap-07.md#handover) |
