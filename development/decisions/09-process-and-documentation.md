# ADR: Process and documentation

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-19"></a>
## D-19 Confirmed

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed: documentation does not approve product implementation.
- **Owner detail:** [Scope](../authoritative-plan.md#delivery-scope), [approval gates](../authoritative-plan.md#workflow-and-approval-gates)

<a id="d-20"></a>
## D-20 Language/i18n direction

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed language/i18n direction.
- **Owner detail:** [Project language](../README.md#project-language), [REQ-097](../requirements-catalog.md#j-processing-presentation-and-usability), [responsibility routing](../architecture.md#language-and-internationalization)

<a id="d-21"></a>
## D-21 Naming authority

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed naming authority.
- **Owner detail:** [Glossary maintenance](../glossary.md#maintenance-rules)

<a id="d-95"></a>
## D-95 AP-06 Session Repercussion Consolidation

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** AP-06 session repercussion consolidation (documentary). Reconcile current navigation/status through explicit follow-up entries and historical labels, rather than relocate/rewrite prior evidence or reopen settled technology choices. Record the AP-05 standard-library correction, link later approved library selections, bring AP-06's prerequisite handover up to date, and distinguish historical Watermill candidates from D-94's limited current selection.
- **Owner detail:** This record is the only current definition

<a id="d-96"></a>
## D-96 Work-package reslicing

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** The user ordered a reslice of the AP-06+ work packages into an acyclic, strictly ordered path with a clear order-to-package mapping that depends on no later package, and allowed changing packages even where execution had started. The reslice created eleven successor assignments (tasks T055..T085, first numbered 17 to 27) covering the Processing kernel, intake, master data and correction, query/live, events/Delivery, plugin host, Processing presentation, identity lifecycle, frontend, operations and final evidence. AP-01..AP-05 stayed unchanged and the original AP-06..AP-16 files and evidence were kept as source history.
- **Consequences:** The execution path in [work-packages.md](../work-packages.md#revised-execution-path) is the authoritative order. Completed baselines and source evidence stay preserved; the numbering was corrected by D-97.
- **Owner detail:** [execution path](../work-packages.md#revised-execution-path)

<a id="d-97"></a>
## D-97 Ordered work-package numbering

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** The reslice numbering did not communicate execution order, so the user ordered the eleven revised assignments to be replaced under sequential IDs AP-06..AP-16 (old numbers 17 to 27, same order: Processing kernel, intake, master data and correction, query/live, events/Delivery, plugin host, presentation, identity lifecycle, frontend, operations, integration evidence). Task IDs T055..T085 and all scope stay unchanged; AP-01..AP-05 stay untouched.
- **Consequences:** The former AP-06..AP-16 files and the AP-06, AP-07, AP-08 and AP-12 execution records remain as labeled source history (`execution/source-ap-*.md`). References to the old numbers 17 to 27 in historical text mean the current AP-06..AP-16 in the same order.
- **Owner detail:** [execution path](../work-packages.md#revised-execution-path)

<a id="d-98"></a>
## D-98 Coordinated Existing-code Library Reassessment

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** Approved all-task library suitability checks and coordinated multi-session reassessment of existing code. The user requires library suitability to be checked for every task, not just substantial additions, and all previously produced code to be reassessed in separate scoped sessions. Independent repeated selections are rejected: sessions must first centrally report capability needs, then receive the shared selection for implementation. [The existing policy owner](../README.md#library-reuse) now specifies permitted custom-code exceptions and the coordination procedure, extending D-36 rather than creating a parallel instruction source. Root AGENTS already routes every agent to this policy.
- **Owner detail:** [policy owner](../README.md#library-reuse)

<a id="d-112"></a>
## D-112 AP planning and contract decision ownership

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Confirmed AP-owned planning and contract decisions; no separate AP prerequisite for a consuming AP's own contract freeze.
- **Owner detail:** This record is the only current definition

<a id="d-126"></a>
## D-126 Open-AP self-containment repair

- **Status:** accepted
- **Date:** 2026-11-04
- **Decision:** Approved direct open-AP planning repair without sidecars; inserts Gateway producer, preserves completed results and assigns all supporting work before its consumers. Directly maintain current AP documentation and required references, migrate needed source-assignment inputs to their owners and remove redundant source definitions/redirects/duplicate records. Preserve unique evidence and immutable baseline provenance; no product execution or sidecars. This supersedes earlier requirements to retain duplicate source-assignment files, not their behavioral decisions or observed results. The [ordered assignments](../work-packages.md#revised-execution-path) own task allocation, supporting paths and verification; the [mapping](../work-packages/requirement-mapping.md) owns REQ coverage.
- **Owner detail:** This record is the only current definition

<a id="d-132"></a>
## D-132 Documentation and naming separation

- **Status:** accepted
- **Date:** 2026-11-05
- **Decision:** Process documentation separated from permanent documentation; planning IDs only in `development/`; fixed domain names; project tooling; OpenAPI authoritative for implemented HTTP schemas.
- **Consequences:** Execution records written before 2026-11-06 refer to the former `docs/` path and to former test identifiers; see the [execution README](../execution/README.md#reading-records-written-before-2026-11-06). Earlier decisions and evidence are not rewritten. Checks: [AP-08 E-16](../execution/ap-08.md#e-16-follow-up-documentation-naming-and-tooling-separation).
- **Owner detail:** This record is the only current definition

<a id="d-134"></a>
## D-134 Review-finding AP planning revision

- **Status:** accepted
- **Date:** 2026-11-06
- **Decision:** Review-finding planning revision of open AP-11..AP-16 tasks and resolved contract statements (2026-11-06). Findings that need later-AP work were added to the open assignments, in their existing task paragraphs: - [AP-11 T066](../work-packages/ap-11-events-delivery.md#planning-before-coding): the attempt invocation carries the triggering fact's C-08 `correlationId`/`causationId`. - [AP-12 T069](../work-packages/ap-12-plugin-host.md#planning-before-coding): manifest Extension field definitions, storage and C-06 exposure, or an explicit deferral. - [AP-13 T072/T074](../work-packages/ap-13-processing-presentation-integration.md#planning-before-coding): production stored-Transmission preview capture and step-level safe failure explanations (candidate `failure_step`), with their cases. - [AP-14 T075](../work-packages/ap-14-identity-lifecycle.md#planning-before-coding): Operator-configurable shorter Session lifetimes. - [AP-16 T081](../work-packages/ap-16-operations.md#planning-before-coding): readiness-gated production transactions and bounded, authorization-before-pagination management listings.
- **Owner detail:** This record is the only current definition

<a id="d-136"></a>
## D-136 Agent execution rules and boundary crossings

- **Status:** accepted
- **Date:** 2026-11-06
- **Decision:** The [execution rules](../work-packages/shared-rules.md) are the only definition of how an agent executes an AP; the skill, the development README, the package directory and the execution README link to them. The executing agent also performs approved boundary crossings: it analyzes the impact, asks the user, records each approved crossing as an ADR with a D-ID, adds follow-ups to affected completed records and amends affected open APs inline. The task state `accepted` is retired; `verified` with evidence is final and the user accepts by merging. An execution record's handover always describes the current state. Open APs follow the [AP template](../work-packages/ap-template.md).
- **Consequences:** AP-09..AP-17 were restructured to the template; the handovers of AP-05..AP-08 were brought up to date with their D-135 follow-ups; earlier `accepted` task states read as `verified`.
- **Owner detail:** [Execution rules](../work-packages/shared-rules.md), [AP template](../work-packages/ap-template.md), [execution README](../execution/README.md)

<a id="d-138"></a>
## D-138 Open-AP boundary repair and server exchange of Units

- **Status:** accepted
- **Date:** 2026-11-07
- **Amends:** [D-135](10-foundation-redesign.md#d-135) item 15 (REQ-128)
- **Decision:** User-ordered review of AP-09..AP-17 so that every piece of work belongs to exactly one AP, no AP relies on a later AP or an undecided question outside its own planning, and every AP names its user stories and concrete paths.
  - REQ-128 is rewritten: exchange of one, several or all Units with their Address rules, Suppressions and Unit collections between servers through one versioned document with import preview; no spreadsheet format. New REQ-140 adds bulk changes of Address rules, Suppressions and Unit collections like REQ-026 for Units. Both, server and screens, belong to AP-15 (new task T089); AP-16 no longer carries REQ-128.
  - AP-11 creates the per-Unit parameter table of REQ-129 and applies it; AP-12 only manages its values.
  - AP-10 owns the query-side presentation of assignment state (REQ-035/040/123/124, per-Agency unassigned traffic).
  - REQ-072/076 are recorded as dropped in the delivery scope; shared REQs name each AP's part; write scopes name persistence paths and follow D-128 for migrations; the Subscription preview is a non-goal of AP-10.
  - Executing agents read the story rows of their REQ IDs at session start.
- **Consequences:** Changed: requirements catalog (REQ-128, REQ-140), delivery scope, requirement mapping, package directory, C-04, AP-09..AP-17, execution rules and AP template.
- **Owner detail:** [Execution rules](../work-packages/shared-rules.md#session-start), [AP-15](../work-packages/ap-15-frontend.md), [requirement mapping](../work-packages/requirement-mapping.md)

<a id="d-142"></a>
## D-142 Second open-AP boundary repair and start decisions for AP-10..AP-17

- **Status:** accepted
- **Date:** 2026-11-07
- **Amends:** [D-138](#d-138), [D-135](10-foundation-redesign.md#d-135) item 15 (REQ-096/097)
- **Decision:** User-ordered audit of AP-10..AP-17 so that no open AP contains an undecided question, a conditional scope or a planned boundary crossing. The user decided:
  - REQ-133/138: personal and global saved views are delivered in Phase 1; a global view is visible to every Principal that may read Messages, and opening any view yields only the intersection with the viewer's current rights (AP-10 server, AP-15 screens).
  - REQ-062: account deletion removes all Core-held personal data, including personal plugin configuration, Secrets and Delivery targets, in one transaction and emits `principal.deleted`; the result states that copies outside the Core are not purged. No plugin cleanup method is built (REQ-139 stays later).
  - REQ-097: account preferences are locale, time zone and a time-gap threshold in minutes (0 = off, 0..1440); Delivery preferences are the AP-11 Subscriptions, AP-13 stores no Delivery defaults.
  - REQ-111: `domain_events`/`consumer_offsets`, terminal receipts and `login_attempts` are pruned automatically by their documented rules; Messages and `audit_records` only under an operator limit (unlimited by default). Limits are a revisioned installation setting managed with `operations.manage`; a worker-role runner applies them (AP-16).
  - Plugin Secrets: the in-app encrypted store (authenticated encryption, master key from `GOPAGER_SECRET_KEY_FILE` or `GOPAGER_SECRET_KEY`) is mandatory; a Secret field may instead hold an optional external reference `file:` (mounted Kubernetes or Docker Secret) or `env:`; a changed file digest is a new Secret version. Provider API adapters are not built (AP-12).
  - First-party plugins are separate Go modules under `plugins/<name>/` with their own tests against local fakes; AP-12 provides a server-side test harness (`pluginsdk/plugintest`) that simulates how the Core calls a plugin. No real external Delivery check is part of any AP.
- **Ownership repairs:** AP-11 owns layout definitions (block schema, validator, renderer, store, `presentation.manage` CRUD), the Subscription layout reference and the C-08 `replayMode` vocabulary; AP-13 adds the installation default and Organization/Unit layout assignments in its own table, the static installation title/banner and the GUI card model. AP-10 projects derived Processing result fields; AP-13 builds only the card model on top. Plugin hosting stays in `internal/processing/` (no `internal/pluginhost/`); plugin configuration authority is the global `plugin.configure` right ([D-100](04-processing-and-plugins.md#d-100)). AP-17 builds no new tooling and repairs no other AP's defect. Further repairs (inputs, write scopes, F-case ownership, stale references) are recorded inline in the affected APs.
- **Consequences:** Changed: delivery scope, requirement mapping, AP-10..AP-17, C-06..C-13, reference cases, AP template.
- **Owner detail:** [AP template](../work-packages/ap-template.md), [delivery scope](../authoritative-plan.md#delivery-scope)

<a id="d-155"></a>
## D-155 Test run time: test-only costs are configurable

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** At the user's order during the [AP-21](../work-packages/ap-21-attribute-schemas.md) pull request ("tests running about 9 minutes is absolutely inacceptable"; "make it configurable … we shouldn't make it harder for tests"; "check if we can mock very computing intensive tasks"), components expose test-relevant costs as construction options that production leaves at their defaults, and tests use reduced costs: the correction runner's batch size (`assignments.WithCorrectionBatchSize`), the password hashing cost of the command tests (`identityOptions` in `cmd/gopager`, set to `internal/testutil/passwords` in `TestMain`). CI runs the integration tests in parallel with the static checks.
- **Rationale:** A CPU profile of the command tests under the race detector showed Argon2id at production cost on every started login service and login; the slowest test needed 150 sequential submissions only to exceed the fixed batch of 100. Reduced costs keep the same code paths.
- **Consequences:** The AP-21 write scope extends to `internal/assignments/correction_runner.go`, the affected tests, `cmd/gopager/identity_options.go`, `.github/workflows/ci.yml` and `docs/development.md`. Evidence is in the [AP-21 record](../execution/ap-21.md).

<a id="d-161"></a>
## D-161 Minute-window admission tests verify their window

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** CI failed `TestLiveStreamRequestRules` on the AP-12 pull request because its counted requests straddled a minute boundary of the search admission window. The user required that the test must not ship flaky. Every `cmd/gopager` test whose outcome depends on several requests in one admission window (search and live admission of AP-10, Delivery sending admission of AP-11) reads the primary clock before the first counted request and after the last relevant one, applies its assertions only when both readings lie in the same minute, and otherwise repeats the sequence on fresh state (at most three attempts). Freezing the clock was rejected because the same clock drives authorization. Test-only change outside the AP-12 write scope; no production behaviour changes.
- **Rationale:** The check uses the limiters' own clock, so it is exact rather than a timing margin.
- **Consequences:** Follow-ups in the [AP-10](../execution/ap-10.md) and [AP-11](../execution/ap-11.md) records; the AP-12 record links this decision under Blockers and Deviations. No open AP changes.

<a id="d-162"></a>
## D-162 Documentation-only commits need no new Copilot review

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** The user decided that only code changes require a new Copilot review in the pull-request loop. The review gate applies to the **last code commit**, the newest commit that changes any file other than Markdown (`*.md`); a later commit that changes only Markdown (execution records, contracts, decisions, `docs/`) is pushed without re-requesting a review. The required checks still run and must pass on the head commit. Recorded in the [shared rules](../work-packages/shared-rules.md#pull-request-and-review) and the `execute-work-package` skill.
- **Rationale:** Recording a clean review in the execution record changed the head and restarted the review loop although no reviewed code changed.
- **Consequences:** Applies to every open AP from now on, first to the AP-12 pull request ([AP-12 record](../execution/ap-12.md)).

<a id="d-168"></a>
## D-168 Local security and code review before every code push

- **Status:** accepted
- **Date:** 2026-11-12
- **Decision:** The user made a local review mandatory before every push that contains a code commit: the agent runs the `security-review` skill and the `code-review` skill (arguments `xhigh ap-NN-<slug> --max-findings all`, without `--comment`, `--fix`, `max` or `ultra`) on the AP branch, checks each finding like a Copilot annotation, fixes the valid ones and runs both reviews again after every fix until a run reports no open finding; only then it pushes and requests the Copilot review. Copilot annotations fixed with code repeat this whole loop before the next push. Rejected findings are recorded with their reason in the evidence and do not count as open. Pushes of Markdown-only commits need no local review ([D-162](#d-162)). Recorded in the [shared rules](../work-packages/shared-rules.md#pull-request-and-review) and the `execute-work-package` skill.
- **Rationale:** Each Copilot review round costs a paid review and a round trip; as many findings as possible should be found locally before Copilot reviews. `xhigh` with unlimited findings is the highest-recall level that does not start the many reviewer agents of `max` or `ultra`.
- **Consequences:** Applies to every open AP from now on. The Copilot review gate of the pull request is unchanged.

