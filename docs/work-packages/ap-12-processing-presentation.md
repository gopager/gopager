# AP-12 Processing and Presentation

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-07](ap-07-identity-authorization.md), [AP-09](ap-09-query-live.md), [AP-10](ap-10-events-delivery.md), [AP-13](ap-13-frontend.md).

**Owner:** DOM-08. **Contract gate:** C-11, C-08/C-09 reaction boundary. **REQ:** REQ-091..097, REQ-099..100. **Prerequisites by slice:** AP-05/AP-07 for T043's backend evaluator/configuration kernel; AP-09 for read/presentation integration; AP-10 for real Domain event reactions. T043 consumes frozen Transmission/target/result interfaces and fixtures, not an already-built feed. **Exclusive target scopes:** `internal/processing/`, rule/presentation schemas and own tests; UI through AP-13 only.

- [ ] T043 [AP-12] Implement the early backend kernel: multiple selectively triggered, namespaced bounded Workflows/results, extraction/conditions, pure preview/errors, saved/active versions and declared installed-step invocation under C-10. Expose C-11 evaluation/revision/result interfaces for AP-06's ingest publication barrier; no dependency on completed AP-09/feed or frontend.
- [ ] T044 [AP-12] Implement permission-preserving representative-based presentation, source-bound Processing results and safe installation/notice/preference operations under C-11. Hidden fuller evidence cannot supply fallback text/derived fields. Expose built-in Localized text through shared English keys; preserve operator data and independent formatting preferences.
- [ ] T045 [AP-12] Test F-21/F-26/F-37/F-38, regex/Payload bounds, sensitivity, revision conflicts and version selection; prove preview has no real side effects. Kernel fixtures can run before AP-06; source-bound publication/read integration tests follow AP-06/AP-09.

**Acceptance:** Minimum value without original mutation, privilege expansion, or unrestricted scripts; built-in product text prepared for localization, not automatic Payload translation. **Non-goals:** Complete n8n/Node-RED clone or unverified `essen.js` parity.

**AP-02 dependency handover:** The ingest barrier must not create AP-06 -> whole AP-12 -> AP-09 -> AP-06 circular execution. Schedule the standalone T043 kernel first, wire its frozen interface in AP-06, then integrate presentation/query/reactions. This clarifies future execution order only; no AP-12 task was executed or checked off.
