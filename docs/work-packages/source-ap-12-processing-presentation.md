# AP-12 Processing and Presentation

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [processing/presentation/i18n](../requirements-catalog.md#j-processing-presentation-and-usability); select the REQ IDs below.
- **Architecture:** [DOM-08 ownership](../architecture.md#domains-and-data-ownership), [ingest publication coordination](../architecture.md#shared-use-cases), [i18n responsibilities](../architecture.md#language-and-internationalization).
- **Owner contracts:** [C-11](../contracts/c-11-processing-presentation.md), [C-08](../contracts/c-08-events-jobs.md), [C-09](../contracts/c-09-delivery.md), [C-10 installed steps](../contracts/c-10-plugin-host.md), [C-06 field rights](../contracts/c-06-identity-authorization.md); [shared limits](../contracts/shared-conventions.md).
- **Cases and staged handovers:** [F-21/F-26](../contracts/reference-cases.md), [F-37/F-38](../contracts/ap-02-boundary-cases.md); [AP-05 foundation](ap-05-runtime-persistence.md), [source AP-07 AuthZ](source-ap-07-identity-authorization.md), [source AP-06 ingest consumer](source-ap-06-transmission-messages.md), [source AP-09 read integration](source-ap-09-query-live.md), [source AP-10 reactions](source-ap-10-events-delivery.md), [source AP-13 UI consumer](source-ap-13-frontend.md), [AP-04 freeze](ap-04-scope-freeze.md). Only the slices in **Prerequisites** below are required.

**Owner:** DOM-08. **Contract gate:** C-11, C-08/C-09 reaction boundary. **REQ:** REQ-091..097, REQ-099..100. **Prerequisites by slice:** AP-05/AP-07 for T043's backend evaluator/configuration kernel; AP-09 for read/presentation integration; AP-10 for real Domain event reactions. T043 consumes frozen Transmission/target/result interfaces and fixtures, not an already-built feed. **Exclusive target scopes:** `internal/processing/`, rule/presentation schemas and own tests; UI through AP-13 only.

- [ ] T043 [AP-12] Implement the early backend kernel: multiple selectively triggered, namespaced bounded Workflows/results, extraction/conditions, pure preview/errors, saved/active versions and declared installed-step invocation under C-10. Expose C-11 evaluation/revision/result interfaces for AP-06's ingest publication barrier; no dependency on completed AP-09/feed or frontend.
- [ ] T044 [AP-12] Implement permission-preserving representative-based presentation, source-bound Processing results and safe installation/notice/preference operations under C-11. Hidden fuller evidence cannot supply fallback text/derived fields. Expose built-in Localized text through shared English keys; preserve operator data and independent formatting preferences.
- [ ] T045 [AP-12] Test F-21/F-26/F-37/F-38, regex/Payload bounds, sensitivity, revision conflicts and version selection; prove preview has no real side effects. Kernel fixtures can run before AP-06; source-bound publication/read integration tests follow AP-06/AP-09.

**Acceptance:** Minimum value without original mutation, privilege expansion, or unrestricted scripts; built-in product text prepared for localization, not automatic Payload translation. **Non-goals:** Complete n8n/Node-RED clone or unverified `essen.js` parity.

**Output handover:** T043's standalone evaluator/revision/result interface goes to historical [source AP-06](source-ap-06-transmission-messages.md) before ingest publication wiring. Presentation/query/reaction integration follows source AP-06/AP-09/AP-10. The feed/frontend is not a prerequisite for the kernel.
