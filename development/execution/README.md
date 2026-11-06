# AP Execution Records

This directory owns execution state and evidence, not task definitions, decisions, contract detail or scope approval. The [execution rules](../work-packages/shared-rules.md) define the procedure.

## Record Location

For AP-NN, use `development/execution/ap-NN.md`. Create it when that package starts. An absent record means no recorded execution, not approval or completion. Follow the same path rule for prerequisite producers; do not create an independent status matrix.

Records written before 2026-11-06 use former paths and test identifiers; see [reading older records](#reading-records-written-before-2026-11-06).

The [package directory](../work-packages.md#revised-execution-path) owns order and assignment navigation. Read prerequisite records only for their required handover and evidence. Do not preload unrelated execution history or create records for packages that have not started.

## Required Record Structure

Use these headings in each record; keep task descriptions and technical rules at their owners:

- **Baseline:** Link the AP and identify its starting Git revision plus any relevant uncommitted baseline delta; never claim a clean commit baseline when the AP was already modified. Record the user's start approval and the answers to the AP's open user decisions. Name the assignee; the write scope is the AP's unless an ADR changes it. Missing approval remains explicit.
- **Task Outcomes:** T-ID, state (`not-started`, `in-progress`, `blocked`, `verified`), output links and evidence references. `Verified` requires actual evidence and is the final state; the user accepts results by merging them. Partial slices are identified explicitly.
- **Evidence:** Give each check a local heading such as `E-01`. Record date, exact command/procedure, tool/runtime versions, environment, observed result/exit and limitations. Link logs/artifacts when they exist; do not claim a planned fixture or unexecuted command passed.
- **Blockers and Deviations:** Finding, affected T/REQ/C/P IDs, responsible owner and resolution reference. A changed decision lives at its owner, not in this section. No unresolved record may be presented as success.
- **Handover:** Artifact/API/schema/test paths, producing task, consuming AP/task, evidence links, checked documentation sections and unresolved discrepancies. The handover always describes the **current** state: a follow-up that changes delivered behavior updates it. Consumers link `#handover`, never the whole record.

After closure, a record changes only through a `## Follow-up (D-nnn), <date>` section that names the approving decision and adds new E-IDs, plus the matching update of the **Handover** ([scope-change procedure](../work-packages/shared-rules.md#boundary-crossing-procedure)). Earlier evidence entries are never rewritten.

Source provenance and deliberate reuse assessments stay in [source notes](../requirements-evidence.md); actual AP validation results stay here. Large raw logs may be linked from a suitable artifact location; do not make a second hand-maintained result summary.

## Prerequisite Evidence

These records own unique checks from prerequisite slices; they are evidence, not current assignments or current blockers. Read only the handover/E-ID linked by the consuming task.

| Slice | Evidence | Current continuation |
| --- | --- | --- |
| Identity/AuthZ core and global fields | [Core handover](source-ap-07.md#handover), [field correction](source-ap-07.md#unscoped-field-correction-verification-and-handover) | AP-14 remaining identity/lifecycle |
| Unit/collection/current matcher | [D-60 checks](source-ap-08.md#e-03-postgresql-race-http-and-proxy-validation) | AP-08 remaining master-data/correction |
| Processing/publication prerequisites | Individual E-IDs in [Processing evidence](source-ap-12.md#evidence) | [AP-06 final handover](ap-06.md#handover) |
| Intake planning observations | Individual E-IDs in [intake observations](source-ap-06.md#evidence) | [AP-07 final handover](ap-07.md#handover) |

## Reading Records Written Before 2026-11-06

These records refer to this tree under its former path `docs/` (for example `docs/execution/ap-NN.md` or `git diff -- docs/work-packages` in recorded commands) and to former test identifiers. Those commands and results are preserved as observed; read `docs/` there as today's `development/`. Renamed test identifiers ([D-132](../decisions/09-process-and-documentation.md#d-132)): environment variables `GOPAGER_AP05_PROCESS_HELPER` → `GOPAGER_TEST_PROCESS_HELPER` and `GOPAGER_AP05_TEST_LISTEN` → `GOPAGER_TEST_LISTEN`; test schema prefixes `ap05_`, `ap07_`, `ap08_` → `test_`, `test_cli_`, `test_http_`; trigger `ap07_intake_failpoint` → `test_intake_failpoint`; regex test helper `d61CaptureExpansion` → `captureExpansionCounterexample`. Recorded numbered domain IDs correspond to the fixed [domain names](../architecture.md#domains-and-data-ownership).
