# AP-13 Processing Presentation and Integration

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** [REQ-091..100](../requirements-catalog.md#j-processing-presentation-and-usability), server side (screens and the layout editor: AP-15). For REQ-091..095 this AP owns the GUI card adapters, the card model, the installation/Organization/Unit layout layers and their precedence; layout definitions, their CRUD, the Subscription layout reference and Core Delivery rendering are AP-11. REQ-096 banner and REQ-097 preferences ([D-142](../decisions/09-process-and-documentation.md#d-142)) are owned here.
- **Inputs:**
  - [AP-06 handover](../execution/ap-06.md#handover): evaluator and the `processing_candidates` work record.
  - [AP-07 handover](../execution/ap-07.md#handover): source capture.
  - [AP-08 handover](../execution/ap-08.md#handover): correction, suppression, Agencies, defaults.
  - [AP-10 handover](ap-10-query-live.md#handover): authorized projection port including derived Processing result fields.
  - [AP-11 handover](ap-11-events-delivery.md#handover): layout definitions, `LayoutResolver` port, renderer versions, `replayMode` vocabulary and fan-out suppression.
  - [AP-12 handover](ap-12-plugin-host.md#handover): installed host calls and plugin status.
  - An absent handover of an earlier open AP is an input gap reported at start.
- **Owned contract slices:** [C-11](../contracts/c-11-processing-presentation.md); additive integration slices without changing frozen evaluator/intake/installed ABI behavior: [C-06](../contracts/c-06-identity-authorization.md) the `preference.manage-own` action, [C-07](../contracts/c-07-query-live.md) card-model response section, [C-10](../contracts/c-10-plugin-host.md) installed-step host calls; F-21/F-26 and F-37/F-38 coverage. Presentation/Reprocess integration planning belongs here. Processing is the affected code area.
- **Reference (read when needed):** [D-135](../decisions/10-foundation-redesign.md#d-135) item 9 as amended by [D-137](../decisions/10-foundation-redesign.md#d-137) (Processing candidates, dispatch), [D-134](../decisions/09-process-and-documentation.md#d-134), [domain ownership](../architecture.md#domains-and-data-ownership), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Write scope:**
  - `internal/processing/` — including historical source/work adapters and the runner
  - `internal/readmodel/` — card-model response only
  - `internal/events/` — Reprocess publication of `processing.completed` with `reprocess`
  - `internal/identity/` — account preferences only
  - `internal/units/` — read-only lookups
  - `internal/authz/` — catalog entry for the preference action only
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/processingstore/`, `internal/persistence/ent/identitystore/` (preferences), a new store for `installation_settings` and `layout_assignments`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout))
  - `internal/httpapi/`, OpenAPI handlers (`/api/v1/installation`, preferences, card model)
  - `cmd/gopager/` — runner/root registration and binding the layout layers into AP-11's resolver only
  - focused integration tests
- **Compatibility context:** [AP-14](ap-14-identity-lifecycle.md), [AP-15](ap-15-frontend.md) and [AP-16](ap-16-operations.md) consume the management/projection/preferences APIs, renderer-adapter versions and historical work/fact semantics. Their code is not required.

## Open User Decisions

None.

## Planning Before Coding

Record the decisions in C-11 and the contract slices above before coding; cases belong to their F-ID owners.

**T072.** Define representative-source selection/binding and the GUI card model (representative binding, fallback, layout input; AP-10 already projects derived Processing result fields), result sensitivity/revisions, preview isolation, the GUI card adapters over the shared declarative layout definition (REQ-094; rendered by Vue, no raw HTML or script by construction), reusing AP-11's limits (C-11 output, nesting and collection bounds). Layout layers: the installation default layout reference lives in `installation_settings`; Organization and Unit assignments live in the new table `layout_assignments(scope_kind organization|unit, scope_id, layout_id)` (no change to C-04 or unitstore); fieldwise precedence installation default < Organization < Unit < Subscription is bound into AP-11's `LayoutResolver` from `cmd/gopager`; conflicts between several Units follow C-11. Editor metadata is served here (AP-15 builds the editor). Banner ([D-135](../decisions/10-foundation-redesign.md#d-135) item 15): installation title and banner are one revisioned `installation_settings` row (title, optional plain-text banner, optional severity, default layout reference), edited with `notice.manage` and `expectedRevision`, shown to everyone while set, with no dismissal, IDs or audience; new migration, `GET/PUT /api/v1/installation` (read is public including guests) in OpenAPI and docs/http-api.md. Preferences ([D-142](../decisions/09-process-and-documentation.md#d-142)): locale, IANA time zone and time-gap threshold in minutes (0 turns it off, range 0..1440), revisioned, no Delivery defaults; action `preference.manage-own` (scope `all`, self-service like `key.manage-own`), one catalog entry in `internal/authz/` and one planned line in C-06. `workflow.preview` already exists. Also implement production stored-Transmission preview capture (currently explicitly unavailable): `workflow.preview` plus current read rights for every stored sample field, source-owner projection without hidden fields, and no persisted state. Return safe failure explanations (C-07 code, safe reason key and failing step ID) for preview and for failed runs, persisting the run's `failure_step` ([D-134](../decisions/09-process-and-documentation.md#d-134)).

**T073.** Bind AP-06 kernel/AP-07 intake/AP-10 projection/AP-11 event-renderer/AP-12 host integration, including trigger/revision/failure/rollback. Historical `Reprocess` scope is the Workflow selection scope (all|network|unit) intersected with an explicit Message-time range and the chosen version; `workflow.reprocess` is authorized over the Unit/Agency scopes of the affected Transmissions; the stable scan cutoff is the maximum accepted Transmission sequence at request time; concurrent invalidation is fenced; original-preserving source selection, durable `processing_candidates` work/restart/supersession, current authority/Assignment/Suppression/version fencing and atomic result replacement. Old results cannot retain invalid visibility. Reprocess publishes `processing.completed` with `replayMode` `reprocess`; AP-11 owns the vocabulary and suppresses automatic Delivery fan-out for it, and redelivery remains separately confirmed. Implement work adapters/runner here. Retrying a failed run is not historical Reprocess; reactions that depend on results trigger on `processing.completed`.

**T074.** Settle the end-to-end evidence plan for the cases listed under T074 and the real PostgreSQL/HTTP integration with completed host artifacts. Destination of the evidence: the execution-record E-IDs, one per F-case.

## Tasks

- [ ] T072 [AP-13] Implement permission-preserving representative presentation, source-bound results, notices, and preference operations.
  - **Done when:** presented results remain bound to their authorized source and current revision; Processing never expands visibility; the card model, layout layers, `installation_settings` banner with `GET/PUT /api/v1/installation`, preferences with `preference.manage-own`, preview capture and failure explanations behave as planned.
- [ ] T073 [AP-13] Integrate the frozen kernel with committed intake facts, current query projection, event reactions, installed-step host, and explicit historical Reprocess.
  - **Done when:** Processing never changes original Payloads; historical Reprocess is explicit, fenced and atomically replaces results; old results retain no invalid visibility.
- [ ] T074 [AP-13] Verify end-to-end source binding, preview isolation, sensitivity, revisions, historical Reprocess and presentation cases across completed backend boundaries.
  - **Done when:** [F-21/F-26](../contracts/reference-cases.md) and [F-37/F-38](../contracts/ap-02-boundary-cases.md) pass, covering source-binding/hidden-location fallback, unsafe markup/URL, bounded preview without external effects, retained old versions until explicit reprocessing, concurrent changes, historical replacement rollback/restart, layout-definition rendering parity between card and Delivery, stored-sample preview with hidden/foreign fields denied, step-level failure explanations and no automatic sending; F-26 is covered for the backend part only (locale-independent wire values, stable codes and keys; the catalog and browser text are AP-15's evidence).

## Documentation to Update

- Development owners: [C-11](../contracts/c-11-processing-presentation.md) (Presentation and Configuration, Historical Reprocess, Text-key Mapping), the C-06/C-07/C-10 slices above, the F-37/F-38 and F-21/F-26 cases, and [glossary](../glossary.md).
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md) (Processing: workflows, versions, candidates; installation, preference and card-model routes).
- [docs/workflows.md](../../docs/workflows.md) (Preview, Result shape, Lifecycle) and [docs/architecture.md](../../docs/architecture.md) (Processing status, Historical correction).
- [docs/operations.md](../../docs/operations.md) if runner configuration changes; Go doc comments.

## Verification

Run existing Processing/renderer/readmodel/delivery race tests and real PostgreSQL/HTTP integration with completed host artifacts, plus focused vet/build.

## Handover

To AP-14/AP-15/AP-16/AP-17: actual management/projection/preferences APIs, renderer-adapter versions, historical work/fact semantics and executed tests. To AP-14: the preferences table with its account key and a deletion function for AP-14's transaction. To AP-16: retention participants, if any.

## Non-goals

Rebuilding the evaluator kernel, unrestricted workflow scripting, automatic Payload translation, or unapproved browser behavior.
