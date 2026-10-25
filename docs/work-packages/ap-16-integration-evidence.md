# AP-16 Integration and Delivery Evidence

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [approved delivery slice](../authoritative-plan.md#delivery-scope), [open issues/deviations](../authoritative-plan.md#open-issues-and-risks), [approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and evidence coverage:** [catalog](../requirements-catalog.md), [REQ-to-package mapping](requirement-mapping.md), [actual execution/evidence records](../execution/README.md), [external provenance](../requirements-evidence.md). Review only active IDs in the approved slice.
- **Architecture:** [integration ownership](../architecture.md#supporting-responsibilities), [shared dependencies](../architecture.md#allowed-dependencies), [i18n responsibilities](../architecture.md#language-and-internationalization).
- **Contracts and cases:** [contract directory](../domain-contracts.md#contract-register) (select approved C-ID versions), [shared conventions](../contracts/shared-conventions.md), [core F-cases](../contracts/reference-cases.md), [F-27..F-41](../contracts/ap-02-boundary-cases.md).
- **Prerequisite outputs:** [AP-04 approval](ap-04-scope-freeze.md); use the [package directory](../work-packages.md#package-directory) and REQ mapping to select all product packages contributing to that slice. Open their acceptance/results and actual test/operations artifacts; a package checkmark alone is not evidence.

**Owner:** Integration with all domain owners. **Contract gate:** Approved versions for agreed Delivery scope. **REQ:** Every active identifier in the approved slice, particularly REQ-097..100/105..110/120/127; REQ-126 remains removed/reserved (D-31). **Prerequisites:** All related product packages and AP-04 approval. **Write scope:** Integration/approval evidence and shared build/API/SDK wiring only; domain corrections through respective owners.

- [ ] T053 [AP-16] Complete REQ-to-test/operations evidence for the approved slice; distinguish fixtures, real SQL/HTTP/browser/plugin/provider/recovery checks. Include English/i18n-readiness evidence without requiring a translated second catalog.
- [ ] T054 [AP-16] Integrate contract versions, endpoints/SDK, build/asset compatibility, gate outcomes; record deviations/remaining blockers in the plan, no blanket completion claim.

**Acceptance:** No active delivery scope without traceable evidence; later/removed identifiers remain identifiable. **Non-goals:** Inventing features, required translations, or quantitative promises during acceptance.
