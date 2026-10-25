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

## AP-02 Shared Assignment and Time Revision on 2026-10-25

The user clarified that repeated receptions from one or many Gateways must not create independent Unit mappings. The primary user operation is finding which Unit was first associated with which Message; original technical Addresses remain necessary for Rematching, while Gateway evidence primarily supports administrative diagnostics.

The revised model uses one shared address-based Assignment per Message/Network/Protocol/Address/Function/effective Address-rule version and contiguous validity segment. Unit resolves through the rule; a separate per-reception Unit mapping is not required. n:m Reception links identify original reported targets without adding a second technical-evidence entity. One multi-target Transmission belongs to one Message, even with Grouping disabled. Temporal corrections can split/coalesce relationships and relink evidence without rewriting the original report.

The user authorized choosing the time semantics and propagating the result. D-28/C-07 choose first-observed Unit appearances after non-time scope/permission filters, time-range selection on those appearances, then one Message row sorted by its earliest surviving appearance (View time). The same rule applies without a Unit filter. Stored Assignment first times avoid ordinary reads scanning reception history; View time is query-derived, not stored as a universal Message field. Internal Message time remains the whole-Message retention origin under the earlier explicit retention decision. A recent Unit appearance does not prevent deletion of an old whole Message.

This supersedes the earlier draft universal Message-time/hidden-member metadata exception and per-Transmission-target Assignment definition. Glossary rename entries record the changes. Requirements and package mappings are adjusted, including Gateway filters confined to administrative diagnostics. F-11a..F-11i are concrete planned contract cases, not executed product tests. No performance measurement, database spike, code adoption, schema migration, product implementation or AP-04 freeze was performed. AP-02 as a whole remains in progress; especially pending C-11/C-13 propagation is not claimed complete.

## AP-02 Fresh Logical Model Approval on 2026-10-25

After comparing alternative models and explicitly reopening the data design, the user approved the new logical model and the subsequent simplification/GUI explanation, requesting propagation to the glossary and planning data. D-29 supersedes the storage assumptions in the preceding historical section; that section is retained as provenance, not current authority.

- Preserve an original Payload per independent Transmission, exact original Addresses/Functions, Gateway/declaration context and both reception/intake times. Repeated Payload storage is acceptable; physical deduplication is optional and lossless.
- Store current Network/Unit/matched Address-rule resolution and explicit status/suppression on each target. Direct Unit repetition is deliberate technical denormalization, not repeated user assignment.
- Maintain shared, rebuildable Message contributions with direct Unit/first time/age anchor and Contribution supports tracing back to originals. Context identity excludes Gateway, immutable rule edit versions and contiguous validity segments; temporal evaluation remains per original target.
- Keep Address-rule domain validity; do not require an immutable version entity merely for Rematching. Configuration audit explains edits; exact replay of every prior target evaluation is not promised.
- Message remains the payload-based logical summary, independent of Unit matching, with no additional MessageGroup layer. Partial/deviating originals remain stored; heuristic membership is explicit and reversible. Thresholds, reconsideration and split/reference/event behavior still require owner-contract elaboration before freeze. No measured heuristic quality is claimed.
- GUI Payload comes from a permitted Representative Transmission, preferring non-heuristic members not marked incomplete, otherwise an identifiable fallback, with deterministic Acceptance-time/ID tie-break. Hidden complete evidence cannot supply text to an otherwise permitted partial view. A better representative does not reset first Unit times.
- The user expressly deferred concrete fields to suitable owner contracts/packages. The illustrative ER fields are not a frozen schema. Gateway/Network/Unit boxes were stubs only: existing configuration, state, authorization, color/icon, organization/collection, templates/defaults/overrides and Protocol-specific behavior are preserved.

The user's final clarification keeps Grouping based on Payload, not Units. The assistant's fixed-span-window suggestion was not approved; existing exact sliding-window cases remain. Explicit Domain-event subscriptions remain; no blanket once-per-Message Delivery rule was introduced. The old exact-only/blanket heuristic deferral is amended, without claiming completed heuristic contracts or executing AP-01/AP-03/AP-04 or implementation packages. Documentation propagation includes glossary, plan, architecture, requirements, contracts/reference cases, package handovers and entry points. AP-02 remains in progress.

Documentation checks for this amendment: local Markdown file links/heading anchors passed across all nine repository Markdown files; all 132 requirement IDs and all 54 task IDs/check states match the Git baseline. Model glossary entries are present; `git diff --check` passed. These are documentation checks, not Mermaid rendering, executable query tests, heuristic calibration or database performance evidence.

## AP-01 Impact Review after D-29

At the user's explicit request, AP-01 contents were checked against the approved new structure. Adjustments were necessary: C-02 still referred to the removed shared Assignment entity; C-01 ambiguously required "completed" reception and a historical Gateway-configuration revision; C-03's unqualified Message-chain/merge/order statements did not distinguish exact components from heuristic attachments; AP-01's handover still deferred every heuristic to a later phase.

The amendment clarifies finalized valid reports versus partial reception, preservation of original targets/Payload, current target resolution and contribution derivation, safe current configuration rematching, exact-only connectivity/alias guarantees and reversible heuristic follow-up. Original exact sliding window, time basis, Protocol forms, initial authorization/idempotency and multi-target indivisibility remain unchanged. F-10c..F-10e add planned boundary cases. AP-01's original checkmarks remain historical completion records; the amended handover does not declare the enlarged heuristic scope complete. New heuristic details remain AP-02 T009 blockers, while configuration-triggered historical regrouping stays deferred. No implementation, AP-03 spike, AP-04 freeze or new heuristic threshold was introduced.

## Heuristic Deferral (D-30)

The user stopped the sequential heuristic clarification round and explicitly deferred heuristics in full. Earlier choices allowing unmarked comparison, per-form configuration, text-on default, prefix comparison and configurable 50% minimum length are historical exploration, not current settings. The proposed 85% similarity threshold was never confirmed.

Current matching is exact canonical Payload equality only. Valid partial originals remain stored, may group with exactly equal canonical originals, and do not join fuller/different values through similarity. Comparison is separated from temporal Grouping decisions; original evidence and match kind `exact` permit a future approved extension without requiring current heuristic scores/configuration, candidate scans, reconsideration jobs, split mechanics or new event types. C-02/C-03, glossary/model, current plan, package handovers and F-10a..F-10d are aligned. Heuristics are no longer AP-02/freeze blockers; AP-14 only records later ownership. Product code and other work packages were not executed.

## Manual Operator Backup Boundary (D-31)

Asked about credential invalidation after restoring an old backup, the user explicitly rejected the premise that the application should implement Backup/Restore at all. Backup creation/restoration is manual Operator work; no application logic is needed. The proposed restore detector/hold, manifests, application backup/restore operations and automatic credential invalidation were removed before implementation. REQ-126 is retained as a reserved removed ID; current plan, glossary, C-08/C-12/C-13 and affected package handovers are aligned.

Ordinary restart/job consistency, schema compatibility and Lite-to-Full identity/history transfer remain separate requirements. The application cannot reconstruct later revocations/deletions or external effects missing from an old database copy; that limitation is stated, not disguised as a restore guarantee. No backup/restore code, experiment or other work package was executed.

## AP-02 Documentary Completion on 2026-10-25

T004..T009 were reviewed against their owner contracts and concrete planned boundary cases. C-11 now specifies typed namespaced Workflows/results, independent selectors, bounded evaluation, current-revision ingest publication, side-effect-free preview, activation/reprocessing, source-bound representative rendering, classified template inputs and English key/fallback mappings. C-13 now propagates D-26's immediate blocking, durable cleanup and field-specific pseudonymization matrix. C-12 defines storage/authority failures, restart/transition/update boundaries and D-31's exclusion. Earlier C-08 envelope/order and plugin-trust contradictions were removed, along with active anonymization/heuristic-blocker wording.

The prior targeted local legacy inspection recorded in this session used tag `v1.6.0-beta`, commit `c8a934d67100e86789f9d6ee844cb2d47aa44c7f`, including `server/essen/index.js` and desktop/mobile card templates. It established conditional structured location/destination sections with raw-text fallback and custom parser logic; no code was copied/adopted and no parity claimed. It supersedes earlier not-found wording in REQ-091; it is not a new general legacy assessment.

Documentation validation passed local Markdown links/heading anchors across 43 Markdown files, 132 sequential catalog/mapping REQ IDs, 54 stable task IDs, 16 AP owners, 13 C owners and 61 unique planned F cases (46 existing plus F-27..F-41). Prior to closure task states matched the baseline; closure deliberately checks only T004..T009 in addition to historical AP-01 T001..T003. Whitespace validation passed. These checks validate documentation structure and traceability, not runtime behavior, performance, provider Delivery, database enforcement or localization execution.

Remaining technical evidence belongs to AP-03 and contract/scope approval to AP-04; user-deferred physical fields/types/indexes and library/UI artifacts belong to their named owner gates. No product code, migration, executable spike, dependency install, staging or commit was performed. Current package status is maintained in the authoritative plan, not inferred from the existence of this evidence note.

The producer/consumer dependency review also made future execution slices explicit: AP-08's early master-data/matcher and AP-12 T043's standalone evaluator precede AP-06's ingest integration; historical/read/presentation/reaction integration follows. This avoids whole-package cycles introduced by mandatory current resolution/Processing publication. Only handover descriptions changed; no task of those packages was executed/checked off. Merge survivor ties now end in stable ID, and same-commit merge/join facts explicitly require no artificial total order.
