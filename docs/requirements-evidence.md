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

## AP-01 Grouping Policy Decisions on 2026-10-25

User decisions recorded in C-03 and F-05 to F-07b: grouping always uses the reported observation instant with mandatory UTC offset or UTC designator; window default 30 seconds, inclusive boundary; configurable maximum group age, default 180 seconds, measured as member span; late observations join by observation time with no lateness cut-off (fallback 2 hours if unbounded joining proves disproportionately complex); the nearest candidate group wins regardless of direction; groups are never merged; configuration changes apply to future assignments only and never dissolve or regroup existing groups (no per-group revision pinning). Equal-distance tie-break remains a proposal. Not frozen; freeze belongs to AP-04.

## AP-01 Content Identity Decisions on 2026-10-25

User decisions recorded in C-02 and F-04 to F-10a: identity fields and content classes are declared per protocol definition; compatible protocols (POCSAG/FLEX text) group across protocols, incompatible ones never (this changes the former "same protocol" wording of D-17/REQ-122); address and function/subaddress are recipient observation attributes; empty content is content-identical; ZVEI content is the double tone by technical tone name or "no double tone" (meaning-based grouping evaluated and rejected as ambiguous, per the user's fallback); EAS identity excludes station ID and audio/video, areas are recipients, audio/video storage planned later; control characters act semantically with comparison-only canonicalization; v1 groups exact canonical matches only, the similarity heuristic for incomplete/bit-error receptions is drafted and deferred per the user's fallback; equal-distance tie-break confirmed. Not frozen; freeze belongs to AP-04.

## AP-01 Grouping Revision on 2026-10-25

The user allowed merging groups when a later observation closes a gap, provided absorbed group IDs remain resolvable and the merge is published as an event, and offered dropping the maximum group age. Recorded in C-03 and F-04 to F-07b: groups are chains of content-identical observations with gaps <= W, merged on gap closure (earliest group survives, absorbed IDs forward); the 180-second maximum group age is withdrawn, making results independent of acceptance order and removing the candidate tie-break; W has no upper limit beyond the data type and W = 0 disables grouping; configuration changes require a dedicated audited capability; regrouping after configuration changes is a later investigation item; membership changes and merges are new events whose consumer reactions belong to other contracts. Content identity uses per-protocol canonical content forms instead of a fixed class catalog; FLEX supports tone-only messages (correcting the assumption that it does not), so POCSAG address-only and FLEX tone-only calls match, POCSAG/FLEX numeric match each other but never text. Not frozen; freeze belongs to AP-04.

## AP-01 Terminology on 2026-10-25

The user requested unambiguous terminology for all documents, distinguishing radio Addresses, GoPager Units, receiving Sources, and outbound targets, and distinguishing User groups, Unit collections, protocol Group calls, and grouped Messages. User choices: Reception for a Source's report, Message for the grouped entity, Message target for the Address-to-Message relation, Payload for transmitted content, Delivery for outbound communication, Localized text for UI/error text; "Alarm", "Telegram", unqualified "Recipient" and "Group" are not used. "Transmission", "Canonical payload", and "Payload form" were proposed by the assistant as consequences and remain open to user revision. The glossary also lists logical information that must be stored or reconstructable without prescribing a data model, as requested by the user.

Amendment on the same day: the user asked for identical names on Gateway and server side and preferred "Transmission" over "Reception"; "Transmission" now names the per-Gateway record, and the physical radio send event is called "Emission" (not a domain object). "Ingest module"/"Source" were replaced by "Gateway" (running instance) and "Gateway adapter" (software type) at the user's choice; "Receiver" was rejected because it collides with recipient and receiver hardware. "Reception time" used for historical address-rule validity was mapped to Observation time.

Second amendment on the same day: the user asked to state that the glossary is the only authoritative source for names, that packages must add new names and record collisions/renames with a clear old-to-new mapping, and that they must then re-check all referring parts. Recorded in the glossary maintenance rules and rename log, README change rule 9, D-21, the shared work-package rules, and AP-04 T017. A completeness review also corrected wrong substitutions from the bulk rename (persona heading, "functioning Gateways", Observation time in REQ-034, PagerMon source code), unified "target instance" to Delivery target, and added personas, Guest, Worker, and Delivery plugin to the glossary.

## AP-01 Completion on 2026-10-25

- A first closure on the same day was withdrawn after review. AP-01's own tasks were still open: Protocol field definitions (T001), Payload-form mapping and policy-revision activation (T002), idempotency, Network resolution, timestamp format, rejected Submissions, and negative fixtures (T003).
- User decisions:
  - Submission IDs are retained as long as their Transmission. A retry after retention deletion is an accepted residual risk (F-23 adjusted).
  - The Observation time is RFC 3339 with a mandatory offset and at least whole-second precision.
  - Future Observation times are accepted and processed normally. Beyond a clock-skew tolerance (default 15 minutes) they are marked, logged, and announced by a subscribable Operational Domain event.
  - The Gateway adapter decodes character sets.
  - FLEX binary/secure types use the opaque Payload form `binary`.
  - Malformed Submissions are rejected as a whole and raise an Operational Domain event.
  - Gateways may declare their Network by a Gateway Network identifier; otherwise the server infers it (single Network, then by Address rule). The assistant's proposed `unresolved` fallback was replaced after user review. Every non-unique result goes to the Gateway's default Network, which is mandatory for several Networks of a Protocol. Only assigned Networks are used. Candidate Networks are recomputed, not stored. The assistant noted that step 4 also covers several matching Networks, not only none; the user's symmetry argument (none and several are equally non-unique) applies to both.
  - A new Grouping policy revision applies to assignments processed after the change.
  - The canonical numeric glyphs were confirmed.
- Protocol facts were checked against Wikipedia (POCSAG, EAS) and the multimon-ng sources. The check corrected the assistant's earlier wrong glyphs for numeric codes 0xE/0xF. The FLEX type list remains to be verified in AP-04 T016.
- AP-01 is marked complete with user confirmation. This is not a contract freeze or approval.
