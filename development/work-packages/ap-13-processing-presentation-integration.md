# AP-13 Processing Presentation and Integration

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** [REQ-091..100](../requirements-catalog.md#j-processing-presentation-and-usability).
- **Inputs:**
  - [AP-06 handover](../execution/ap-06.md#handover): evaluator and the `processing_candidates` work record.
  - [AP-07 handover](../execution/ap-07.md#handover): source capture.
  - [AP-08 handover](../execution/ap-08.md#handover): correction, suppression, Agencies, defaults.
  - [AP-10](ap-10-query-live.md): authorized projection. Verify `development/execution/ap-10.md#handover` exists at start.
  - [AP-11](ap-11-events-delivery.md): event, Delivery and renderer. Verify `development/execution/ap-11.md#handover` exists at start.
  - [AP-12](ap-12-plugin-host.md): installed host. Verify `development/execution/ap-12.md#handover` exists at start.
- **Owned contract slices:** [C-11](../contracts/c-11-processing-presentation.md); additive integration slices of C-04/C-06/C-07/C-08/C-09/C-10 without changing frozen evaluator/intake/installed ABI behavior; F-21/F-26 and F-37/F-38 coverage. Presentation/Reprocess integration planning belongs here. Processing is the affected code area.
- **Reference (read when needed):** [D-135](../decisions/10-foundation-redesign.md#d-135) item 9 (durable worker and queue), [D-134](../decisions/09-process-and-documentation.md#d-134), [domain ownership](../architecture.md#domains-and-data-ownership), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Write scope:**
  - `internal/processing/` — including historical source/work adapters and the runner
  - `internal/readmodel/` — projection
  - `internal/events/`, `internal/delivery/` — replay/presentation participation
  - `internal/identity/` — account preferences only
  - `internal/units/` — defaults adapters
  - owner Ent stores, schemas, generated output and next migrations
  - `internal/httpapi/`, OpenAPI handlers
  - `cmd/gopager/` — runner/root registration only
  - focused integration tests
- **Compatibility context:** [AP-14](ap-14-identity-lifecycle.md), [AP-15](ap-15-frontend.md) and [AP-16](ap-16-operations.md) consume the management/projection/preferences APIs, renderer-adapter versions and historical work/fact semantics. Their code is not required.

## Open User Decisions

None.

## Planning Before Coding

Record the decisions in C-11 and supporting contract slices before coding; cases belong to their F-ID owners.

**T072.** Define representative-source selection/binding, result sensitivity/projection/revisions, preview isolation, the GUI card adapters over the shared declarative layout definition (REQ-094; rendered by Vue, no raw HTML or script by construction), output limits, installation/Organization/Unit/Subscription layout precedence/conflicts, the static installation banner (REQ-096, no per-revision dismissal), revisioned account locale/time-zone/time-gap/Delivery defaults, authorization/actions/errors and management HTTP. Reuse AP-11's engine; this AP owns the GUI card adapters and layout-definition editor metadata (AP-15 builds the editor). Also implement production stored-Transmission preview capture (currently explicitly unavailable): `workflow.preview` plus current read rights for every stored sample field, source-owner projection without hidden fields, and no persisted state. Return safe failure explanations (C-07 code, safe reason key and failing step ID) for preview and for failed runs, persisting the run's `failure_step` ([D-134](../decisions/09-process-and-documentation.md#d-134)).

**T073.** Bind AP-06 kernel/AP-07 intake/AP-10 projection/AP-11 event-renderer/AP-12 host integration, including trigger/revision/failure/rollback. Historical `Reprocess` uses explicit permitted scope/chosen version, stable scan cutoff/concurrent invalidation, original-preserving source selection, durable `processing_candidates` work/restart/supersession, current authority/Assignment/Suppression/version fencing and atomic result replacement. Old results cannot retain invalid visibility. C-08 replayMode suppresses historical external reactions by default; redelivery remains separately confirmed. Implement work adapters/runner here. Retrying a failed run is not historical Reprocess; reactions that depend on results trigger on `processing.completed`.

**T074.** Settle the end-to-end evidence plan for the cases listed under T074 and the real PostgreSQL/HTTP integration with completed host artifacts.

## Tasks

- [ ] T072 [AP-13] Implement permission-preserving representative presentation, source-bound results, notices, and preference operations.
  - **Done when:** presented results remain bound to their authorized source and current revision; Processing never expands visibility; layout, banner, preferences, preview capture and failure explanations behave as planned.
- [ ] T073 [AP-13] Integrate the frozen kernel with committed intake facts, current query projection, event reactions, installed-step host, and explicit historical Reprocess.
  - **Done when:** Processing never changes original Payloads; historical Reprocess is explicit, fenced and atomically replaces results; old results retain no invalid visibility.
- [ ] T074 [AP-13] Verify end-to-end source binding, preview isolation, sensitivity, revisions, historical Reprocess and presentation cases across completed backend boundaries.
  - **Done when:** [F-21/F-26](../contracts/reference-cases.md) and [F-37/F-38](../contracts/ap-02-boundary-cases.md) pass, covering source-binding/hidden-location fallback, unsafe markup/URL, bounded preview without external effects, retained old versions until explicit reprocessing, concurrent changes, historical replacement rollback/restart, layout-definition rendering parity between card and Delivery, stored-sample preview with hidden/foreign fields denied, step-level failure explanations and no automatic sending; backend locale-independent/English-keyed values are verified (actual browser text is AP-15's evidence).

## Documentation to Update

- Development owners: [C-11](../contracts/c-11-processing-presentation.md) (Presentation and Configuration, Historical Reprocess, Text-key Mapping), supporting C-04/C-06/C-07/C-08/C-09/C-10 slices, the F-37/F-38 and F-21/F-26 cases, and [glossary](../glossary.md).
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md) (Processing: workflows, versions, candidates; preference and projection routes).
- [docs/workflows.md](../../docs/workflows.md) (Preview, Result shape, Lifecycle) and [docs/architecture.md](../../docs/architecture.md) (Processing status, Historical correction).
- [docs/operations.md](../../docs/operations.md) if runner configuration changes; Go doc comments.

## Verification

Run existing Processing/renderer/readmodel/delivery race tests and real PostgreSQL/HTTP integration with completed host artifacts, plus focused vet/build.

## Handover

To AP-14/AP-15/AP-16/AP-17: actual management/projection/preferences APIs, renderer-adapter versions, historical work/fact semantics and executed tests.

## Non-goals

Rebuilding the evaluator kernel, unrestricted workflow scripting, automatic Payload translation, or unapproved browser behavior.
