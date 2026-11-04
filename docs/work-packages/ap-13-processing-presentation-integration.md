# AP-13 Processing Presentation and Integration

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-091..100](../requirements-catalog.md#j-processing-presentation-and-usability), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-11](../contracts/c-11-processing-presentation.md).
- **Owner:** DOM-08.
- **Architecture:** [worker and queue assessment](../architecture.md#ap-12-durable-worker-and-job-queue-assessment), [cross-profile worker/pub-sub comparison](../architecture.md#ap-12-cross-profile-worker-and-pubsub-comparison), [domain ownership](../architecture.md#domains-and-data-ownership).
- **Prerequisites:** Verify [AP-06](../execution/ap-06.md) evaluator/publication, [AP-07](../execution/ap-07.md) source capture, [AP-08](ap-08-master-data-correction-after-intake.md) correction/suppression/defaults, [AP-10](ap-10-query-live.md) authorized projection, [AP-11](ap-11-events-delivery.md) event/Delivery/renderer and [AP-12](ap-12-plugin-host.md) installed-host handovers. Presentation/Reprocess integration planning belongs here.
- **Write scope:** `internal/processing/` including historical source/work adapters, `internal/readmodel/` projection, `internal/events/`/`internal/delivery/` replay/presentation participation, `internal/identity/` for account preferences only, `internal/Units/` defaults adapters, owner Ent stores/schemas/generated output/next migrations; `cmd/gopager/` runner/root/management registration, `internal/httpapi/`/OpenAPI, focused integration tests. C-11 and supporting C-04/C-06/C-07/C-08/C-09/C-10 additive integration slices are authorized, without changing frozen evaluator/intake/installed ABI behavior.

## Planning Before Coding

Record the following decisions in C-11 and supporting contract slices before coding; cases belong to their F-ID owners.

- **Later-consumer planning context (not prerequisites):** [AP-14](ap-14-identity-lifecycle.md), [AP-15](ap-15-frontend.md), [AP-16](ap-16-operations.md).

- [ ] T072 [AP-13] Implement permission-preserving representative presentation, source-bound results, notices, and preference operations.
- [ ] T073 [AP-13] Integrate the frozen kernel with committed intake facts, current query projection, event reactions, installed-step host, and explicit historical Reprocess.
- [ ] T074 [AP-13] Verify end-to-end source binding, preview isolation, sensitivity, revisions, historical Reprocess and presentation cases across completed backend boundaries.

## Exact Planning, APIs and Evidence

T072 defines representative-source selection/binding, result sensitivity/projection/revisions, preview isolation, GUI card/sanitization/output limits, installation/Organization/Unit/view precedence/conflicts, title/notices/publication/dismissal, revisioned account locale/time-zone/time-gap/Delivery defaults, authorization/actions/errors and management HTTP. Reuse AP-11's engine; this AP owns GUI output/sanitization and template editor metadata.

T073 binds AP-06 kernel/AP-07 intake/AP-10 projection/AP-11 event-renderer/AP-12 host integration, including trigger/revision/failure/rollback. Historical `Reprocess` uses explicit permitted scope/chosen version, stable scan cutoff/concurrent invalidation, original-preserving source selection, durable work/restart/supersession, current authority/Assignment/Suppression/version fencing and atomic replacement publication. Old results cannot retain invalid visibility. C-08 replayMode suppresses historical external reactions by default; redelivery remains separately confirmed. Implement work adapters/runner here. Failed unpublished-candidate retry is not historical Reprocess.

T074 covers [F-21/F-26](../contracts/reference-cases.md) and [F-37/F-38](../contracts/ap-02-boundary-cases.md), source-binding/hidden-location fallback, unsafe markup/URL, bounded preview without external effects, retained old versions until explicit reprocessing, concurrent changes, historical replacement rollback/restart and no automatic sending. Verify backend locale-independent/English-keyed values; actual browser text is AP-15's evidence. Run existing Processing/renderer/readmodel/delivery race tests and real PostgreSQL/HTTP integration with completed host artifacts; focused vet/build. Hand over actual management/projection/preferences APIs, renderer-adapter versions, historical work/fact semantics and executed tests to AP-14/AP-15/AP-16/AP-17.

**Acceptance:** Processing never changes original Payloads or expands visibility; presented results remain bound to their authorized source and current revision.

**Non-goals:** Rebuilding the evaluator kernel, unrestricted workflow scripting, automatic Payload translation, or unapproved browser behavior.
