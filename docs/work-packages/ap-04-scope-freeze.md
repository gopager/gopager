# AP-04 Scope and Contract Freeze

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-01](ap-01-protocols-grouping.md), [AP-03](ap-03-technical-validation.md).

**Owner:** Product/integration ownership. **Contracts:** Every C ID being approved. **REQ:** Every requirement in the approved slice; REQ-088 and REQ-126 stay removed/reserved. **Write scope:** `docs/authoritative-plan.md`, contract approval metadata, this package register; shared OpenAPI/SDK artifacts after their gates only.

- [ ] T015 [AP-04] Record user approval of the proposed initial scope or an explicit smaller slice in `docs/authoritative-plan.md`; do not assume technical approval grants scope approval.
- [ ] T016 [AP-04] Review relevant AP-01..AP-03 results, including verification of the C-02 FLEX type list and capcode details against the FLEX specification; only if reuse is intended, first document targeted legacy evidence with source/ref/symbol/test. No artificial legacy-parity requirement when nothing is reused.
- [ ] T017 [AP-04] Freeze owner, producers/consumers, schema/version, permissions/field projection, time/revision, idempotency/errors/transactions, and concrete fixtures per boundary, together with the [glossary](../glossary.md) version the frozen contracts use; frozen contracts and the glossary must not contradict each other. Specify machine-readable OpenAPI/SDK artifacts as separate deliverables only then.
- [ ] T018 [AP-04] Add approved contract versions, named owners, final write scopes, and then-executable focused validation commands to `docs/work-packages.md`; approve parallel execution only explicitly for disjoint packages. Include REQ-097/i18n checks without requiring translations.

**Acceptance:** Documented evidence-based approval per slice; no unresolved shared semantics in an approved package. **Non-goals:** Writing code or marking every boundary complete merely because documents exist.
