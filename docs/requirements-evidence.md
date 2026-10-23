# Source Notes and Evidence Boundaries

Date: 2026-10-24. Non-normative. Decisions belong to the [plan](authoritative-plan.md), requirements to the [catalog](requirements-catalog.md).

## Source Inventory

| Source | Observed content | Use / boundary |
| --- | --- | --- |
| User-provided consolidated planning file, 2026-10-23 | Product plan revision 2.2, catalog version 1.8 with REQ-001..132, embedded reference text and evidence note | Read completely as domain baseline. Document-existence/technology-status conflicts resolved; no historical code verification. |
| User statements and answers, 2026-10-23 | GoPager; Go modular monolith; dedicated repository; initial scope still proposed; sliding window with shared configurable duration; EAS in proposed initial scope | Direct provenance for D-01/D-02/D-17..D-19. No specific window duration or audio/video contract decided. |
| Documentation request, 2026-10-23; execution request, 2026-10-24 | Record contracts/interfaces, responsibilities, technologies, work packages, domain boundaries in the repository | Approval of documentation creation, not product implementation or entire initial scope. |
| User language/i18n decision, 2026-10-24 | All planning data in English; English is the authoritative project language; prepare i18n without requiring translations yet | D-20 and REQ-097 clarification; architecture, C-07/C-11, AP-09/AP-12/AP-13, and F-26 updated. No additional language, translation catalog, i18n library, or implemented runtime claimed. |
| [47 CFR 11.31, EAS protocol](https://www.ecfr.gov/current/title-47/chapter-I/subchapter-A/part-11/subpart-B/section-11.31) | Read in prior planning on 2026-10-23; page then current through 2026-10-21. Header, attention signal, message, EOM; message may be audio/video/text; headers/EOM repeat. | Explains why EAS cannot be reduced to a text field. No approved GoPager decoder/media/content-identity contract or complete EAS implementation study. |

The statement that the `essen.js` functional reference was not found comes from earlier planning. No new legacy-source search reconfirmed it during this documentation task. Functional parity is still not claimed.

## Future Research Targets

- PagerMon: existing Nest/Vue application in its separate repository.
- pagermon-ingest-core: previous ingest core.
- pagermon-ingest-adapter-multimon: previous Multimon adapter.
- Historical PagerMon tag `v1.6.0-beta`.

These sources were not analyzed as a codebase in this documentation task. They are not GoPager runtime dependencies. Their existence or naming is neither product evidence nor reuse approval. No trademark/domain check was performed for the new product name. The external German planning source remains historical provenance; the English repository documents are authoritative and are not maintained as a parallel translated specification set.

## Evidence Assessment for Deliberate Reuse

After target contracts, record per affected REQ/contract version: repository/ref, path/symbol, observed behavior, actual tests, missing invariants, status (direct fit, partial fit, idea/test case only, unsuitable). Justify adoption independently of migration effort. Old source references do not replace new authorization/protocol/consistency fixtures.

Technology rationales from the original plan are documented as planning decisions. This transfer is not a new benchmark, API-version, or security validation; required evidence belongs to the [work packages](work-packages.md).

## Initial Documentation Review on 2026-10-24

The initial transfer comprised eight Markdown files, no product sources/dependencies. Native editor search confirmed 132 sequential REQ rows in both catalog and package mapping, REQ-088 still reserved, 13 contract sections, 16 package sections, and 54 tasks (T001..T054). The original 25 reference cases were planned contract checks, not passing tests. The language/i18n update adds F-26 without renumbering the existing cases.

Relative document links and section anchors were checked against the created documents. Editor diagnostics reported no errors. Git whitespace checks on individual new files emitted no whitespace errors; `--no-index` nevertheless returns difference status 1 against `/dev/null`. Nothing was staged or committed.

No Markdown/link checker was preinstalled or subsequently installed. Link/ID review used native search/content inspection, not automated parser/renderer/Mermaid validation. No Go/frontend/database/browser/provider/HA tests ran: no application or test toolchain exists yet. English documentation and planned i18n acceptance checks do not constitute executed localization tests.

## English and I18n Planning Review on 2026-10-24

All eight repository documents were translated into English, including diagram labels, tasks, requirements, and evidence notes. Native search confirmed 132 sequential REQ rows in both the English catalog and mapping, 13 contract sections, 16 work-package sections, 54 unchanged task IDs, and 26 reference cases after adding F-26. English section links were checked against their headings; a search for German planning vocabulary and obsolete anchors found matches only in the historical attachment outside this repository, not in the project documents. Editor diagnostics reported no errors, and Git whitespace checks emitted no whitespace errors. These remain native/documentary checks, not automated language classification or runtime i18n tests. No product code, translation packs, dependencies, staging, or commits were added.