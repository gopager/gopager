# AP-04 Scope and Contract Freeze

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [proposed delivery scope](../authoritative-plan.md#delivery-scope), [open issues](../authoritative-plan.md#open-issues-and-risks), [approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and naming:** [catalog](../requirements-catalog.md), [coverage mapping](requirement-mapping.md), [glossary](../glossary.md); select the slice being approved.
- **Architecture:** [TECH selections](../architecture.md#technology-decisions), [domain ownership](../architecture.md#domains-and-data-ownership), [dependency rules](../architecture.md#allowed-dependencies).
- **Contracts and fixtures:** [contract directory](../domain-contracts.md#contract-register) (select the C-IDs in the approval slice), [freeze checklist](../contracts/shared-conventions.md#freeze-requirements), [core F-cases](../contracts/reference-cases.md), [F-27..F-41](../contracts/ap-02-boundary-cases.md).
- **Prerequisite outputs and evidence:** [AP-01](ap-01-protocols-grouping.md), [AP-02](ap-02-domain-contracts.md), [AP-03](ap-03-technical-validation.md), [evidence/reuse assessment](../requirements-evidence.md#evidence-assessment-for-deliberate-reuse). Use the [package directory](../work-packages.md#package-directory) to open only the APs in the approval slice.

**Owner:** Product/integration ownership. **Contracts:** Every C ID being approved. **REQ:** Every requirement in the approved slice; REQ-088 and REQ-126 stay removed/reserved. **Write scope:** `docs/authoritative-plan.md`, contract approval metadata, approved downstream AP amendments and execution bindings in `docs/execution/`; shared OpenAPI/SDK artifacts after their gates only. This AP remains read-only during execution.

**Prerequisites:** Relevant AP-01..AP-03 results and evidence; targeted legacy assessment only if reuse is intended.

- [ ] T015 [AP-04] Record user approval of the proposed initial scope or an explicit smaller slice in `docs/authoritative-plan.md`; do not assume technical approval grants scope approval.
- [ ] T016 [AP-04] Review relevant AP-01..AP-03 results, including verification of the C-02 FLEX type list and capcode details against the FLEX specification; only if reuse is intended, first document targeted legacy evidence with source/ref/symbol/test. No artificial legacy-parity requirement when nothing is reused.
- [ ] T017 [AP-04] Freeze owner, producers/consumers, schema/version, permissions/field projection, time/revision, idempotency/errors/transactions, and concrete fixtures per boundary, together with the [glossary](../glossary.md) version the frozen contracts use; frozen contracts and the glossary must not contradict each other. Specify machine-readable OpenAPI/SDK artifacts as separate deliverables only then.
- [ ] T018 [AP-04] Bind named assignees, approved write scopes, required contract versions and focused validation plans in `docs/execution/ap-04.md`, linking the plan's approval entry; consuming execution records reference that handover instead of copying it. Contract files own frozen versions; AP baselines remain unchanged except justified not-yet-started downstream amendments. Approve parallel execution only explicitly for disjoint packages. Include REQ-097/i18n checks without requiring translations.

**Acceptance:** Documented evidence-based approval per slice; no unresolved shared semantics in an approved package. **Non-goals:** Writing code or marking every boundary complete merely because documents exist.
