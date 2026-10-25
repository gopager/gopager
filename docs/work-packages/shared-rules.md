# Execution and Ownership Rules

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-01](ap-01-protocols-grouping.md), [AP-02](ap-02-domain-contracts.md), [AP-03](ap-03-technical-validation.md), [AP-04](ap-04-scope-freeze.md), [AP-09](ap-09-query-live.md), [AP-12](ap-12-processing-presentation.md), [AP-13](ap-13-frontend.md), [AP-14](ap-14-later-extensions.md).

- **Historical clarification closure:** AP-01 closed on 2026-10-25 for its original exact-Grouping scope. The D-29 impact review amended C-01..C-03 and reference cases; D-30 subsequently deferred all heuristic semantics. Exact comparison extensibility remains AP-02 T009; no heuristic freeze blocker is retained.
- **Documentary contract handover:** AP-02 coverage/closure is recorded in its owner file and the plan. D-28 times, D-29 model, D-30 exact-only comparison and D-31 external Backup/Restore boundary are reflected in owner contracts. Diagram stubs never narrow Gateways/Networks/Units. Technical evidence, physical schema details and scope/freeze approval retain their own gates; they are not AP-02 implementation claims.
- **Validation planned:** AP-03 contains a documentary validation plan. Executable spikes require their own authorization; none have been run.
- **Approval pending:** AP-04 is a gate, not an automatic status change after creating documentation.
- **Blocked:** all product implementation packages; freeze prerequisites/contract versions first. AP-14 records deliberately later goals without bringing them forward.
- Roles have no named assignee yet. Integration exclusively owns shared schema/contract/build/composition boundaries. No named or parallel write approval exists yet.
- Target paths below are proposed future write scopes, not existing packages or an already-created Go structure. AP-04 confirms them before execution; no module path/toolchain is predetermined.
- Contracts live in individual `docs/contracts/c-*.md` owner files; `docs/domain-contracts.md` is only their index. Shared conventions and F-ID cases have dedicated files there. Open the assigned C-ID files, then follow only relevant dependencies; do not load the entire contract collection by default. Coordinate shared index/conventions/model changes sequentially. Independent implementation remains gated.
- Do not infer passing tests/product completion from future test files. Acceptance requires executed evidence; currently there is only Markdown.
- **Names (D-21):** the [glossary](../glossary.md) is the single authoritative source for names. Every package that needs a new name adds it to the glossary; every package that detects a collision or needs a rename records it in the glossary rename log (old name, new name, meaning, reason, package) and must then re-check and update all documents, code, schemas/migrations, API/event definitions, tests, fixtures, and UI texts referring to the affected names, or record remaining occurrences as an owned follow-up. A package is not accepted while it introduces names missing from the glossary.
- All project artifacts use English. Under D-20/REQ-097, prepare i18n during initial implementation: stable text keys, English text catalog/fallback, parameters/plurals, locale-aware formatting, and language-independent API codes. Additional translations/language packs are not required. AP-09 owns API boundaries, AP-12 built-in processing Localized text, AP-13 frontend catalog/formatting, and integration shared compatibility.

# Validation Commands and Remaining Gates

There is no Go module, frontend manifest, test configuration, or CI yet. Do not invent currently executable build/test commands. AP-04 records actual focused commands per approved package once available; `go test ./...` is only a future overall check after creating the Go module and replaces neither real PostgreSQL nor browser/provider/HA evidence. I18n checks will validate the English-only catalog, mappings, fallback, interpolation/plurals, formatting separation, and layout, without requiring additional translated files.

The task list is deliberately not labeled a fully approved, immediately executable implementation plan: missing domain decisions, technical evidence, and scope approval are real gates. Documentary completeness is checked through IDs/links/ownership/status, not fictional runtime outcomes. Translation of planning data does not approve any blocked package.
