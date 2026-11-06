# Source AP-06 Historical Execution Record

## Baseline

[Original AP-06 assignment at its immutable baseline](https://github.com/gopager/gopager/blob/24318de414b3b35f77b6b288905125c1debf46f4/docs/work-packages/ap-06-transmission-messages.md) is opened from commit `24318de414b3b35f77b6b288905125c1debf46f4` on `main`; the worktree was clean. This is historical acceptance evidence, not a current assignment.

Assignee: Copilot SDK assistant. Accountable approval owner: user / Product and integration. The assignment proposes exclusive target scopes `internal/ingest/`, `internal/protocols/` and `internal/messages/`; no AP-06 implementation scope/version/test binding or implementation approval is recorded in the [plan](../authoritative-plan.md#workflow-and-approval-gates). The [AP-04 handover](ap-04.md) freezes/binds only the AP-05 foundation slice. The user request starts AP-06 but does not replace missing contract freezes, prerequisite handovers or the required execution binding.

**D-95 follow-up baseline, 2026-11-02:** The preceding paragraph preserves the original source AP-06 start state. At the time of this follow-up, source AP-06 remained the coordinating assignment; later approvals separately supplied the identity/matcher and limited T043 prerequisites. [E-31](#e-31-session-repercussion-consolidation) records that session-impact reconciliation under D-95 at the same HEAD with extensive existing uncommitted changes. This is not a clean original baseline or a new product start for the current AP-06.

## Task Outcomes

| Task | State | Output / evidence |
| --- | --- | --- |
| T022 | blocked | C-01 Submission/Gateway freeze and AP-06 execution binding remain absent; the D-59 central identity prerequisite is now available, distinct from Gateway authentication. E-18. |
| T023 | blocked | D-60 matcher/target input and limited T043 prerequisite progress remain available. E-31 links later lifecycle/publication infrastructure evidence, including D-94, but the verified required-kernel handover and AP-06's own remaining freeze/execution are still required. |
| T024 | blocked | T022/T023 and required T043 handover remain outstanding. E-31 distinguishes product infrastructure checks from missing AP-06 acceptance/Grouping/concurrency/crash/publication tests; no AP-06 validation completion is inferred. |

AP-06 intake/Grouping/publication implementation has not started. Actual prerequisite implementation and validation belong to the AP-07/AP-08/AP-12 producer records.

## Evidence

### E-01 Baseline and Assignment

Date: 2026-10-26. In `/Users/leokr/Projects/gopager`, `git status --short --branch` returned `## main...origin/main [ahead 1]` with no modified or untracked files; `git rev-parse HEAD` returned `24318de414b3b35f77b6b288905125c1debf46f4`. Both commands exited 0. AP-06 is present at this baseline and has no local diff.

### E-02 Approval and Prerequisite Review

The [plan approval table](../authoritative-plan.md#workflow-and-approval-gates) records product implementation approval only for AP-05 T019..T021. D-35 and the [AP-04 execution record](ap-04.md#e-13-completion-review-and-freeze-scope) explicitly limit the approved freeze to AP-05; the AP-04 [approved freeze evidence](ap-04.md#e-15-approved-freeze-and-final-verification) says all other technical slices remain gated. Contract owners currently mark C-02 0.2-draft, C-03 0.3-draft, C-05 0.1-draft, C-06 0.1-draft and C-11 0.1-draft. C-01 and C-08 freeze only their named AP-05 sub-boundaries.

The execution-record directory contains AP-01 through AP-05 records and no AP-07, AP-08 or AP-12 execution record. AP-05's [handover](ap-05.md#handover) confirms its foundation is verified but says downstream execution requires its own approval. Therefore the AP-06 prerequisites for AP-07, AP-08's early handover and AP-12 T043 are not evidenced as completed. No AP-06 contract/version/write-scope/test binding exists.

No build, test or database command was run: the implementation is blocked before code changes by missing approvals and prerequisite handovers.

### E-03 AP-04 Follow-up Authorization

Date: 2026-10-26. The user approved preparing/reopening AP-04 documentary T017/T018 work for the AP-06 contract slice. The authorization is recorded in the [plan](../authoritative-plan.md#workflow-and-approval-gates) and [AP-04 follow-up](ap-04.md#follow-up-ap-06-contract-slice). It is limited to review/preparation for C-01/C-02/C-03/C-05/C-06/C-08/C-11 and required glossary/consumer binding. It is not a contract freeze, technical/library decision, AP-06 implementation approval, or prerequisite AP handover. The current task blocker remains until the remaining approvals and handovers are complete.

### E-04 Workload Fit and Remaining Decisions

Date: 2026-10-26. The user stated that most activity is expected to be accepting new Transmissions and producing their resulting Deliveries, while Grouping and Address-filter changes are rare. The plan's existing 500-1500 Transmissions/day/Network is a planning profile, not measured capacity or burst evidence.

For that mix, option C in the [D-40 architecture comparison](../architecture.md#transmission-message-architecture-options) is the strongest initial fit to examine: commit original evidence, exact Grouping, current Address resolution/contributions and required facts consistently in a short relational transaction; dispatch resulting Domain events/Delivery work through the existing durable outbox. The C-11 Processing publication barrier and C-05 correction jobs remain separate; this does not make external Delivery synchronous or permit stale/unready evidence to publish. Rare configuration corrections do not imply full Event Sourcing or historical regrouping. Option B remains relevant if measured processing/latency requirements justify an additional projector/work queue for ordinary intake; it adds per-Transmission progress, deduplication, revision/fencing and readiness-lag behavior. Option A adds authoritative event-history/schema/retention obligations without a stated full-replay requirement. This is a provisional fit assessment, not a selection, benchmark, or evidence that option C meets operational capacity. No architecture contract or downstream AP was changed.

The user clarified that only whole seconds will be stored and finer input should be truncated. This changes the current [C-01](../contracts/c-01-transmission-acceptance.md) draft, which allows finer fractions and says to compare at supplied precision. Truncation would discard original sub-second evidence and could shift a Transmission across C-03's inclusive Grouping boundary by less than one second; e.g. an input at `t+30.9s` becomes `t+30s` and may match at `W=30s`. It would still preserve [F-01a](../contracts/reference-cases.md)'s `.000` equality case. Before recording this as approved, confirm this exact change and decide whether Submission-ID retry equality uses the truncated instant or conflicts when the submitted fractions differ. No input normalization or contract change has been implemented.

### E-05 Approved Decisions and Impact Review

Date: 2026-10-26. The user approved whole-second Observation time by truncation under [D-42](../authoritative-plan.md#ap-06-observation-precision-decision) and selected relational synchronous initial derivation for AP-06 under [D-43](../authoritative-plan.md#ap-06-transmissionmessage-architecture-decision). The C-01/C-03/C-05 edits and [F-47](../contracts/reference-cases.md#ap-04-observation-precision-case) record the precision semantics; [architecture](../architecture.md#ap-06-transmissionmessage-boundary-selection) records the scoped architecture selection. F-01a and all prior AP-03 evidence are unchanged.

Impact review covered the active AP-06 baseline and the not-yet-started AP-08 T029, AP-09 T031..T033, AP-10 T034..T036, AP-11 T037..T042 and AP-12 T043..T045 definitions. They remain consistent when consuming C-01's normalized Observation instant, relationally committed Messages/contributions and the existing C-08/C-11 facts/barrier. AP-08's correction remains asynchronous; AP-10 still owns Delivery/provider I/O; AP-12 still owns Processing. No AP baseline amendment is necessary for these choices. The decisions do not supply the missing AP-07/AP-08/AP-12 implementation handovers, freeze the owner contracts, resolve retry equality for timestamp values that truncate to the same second, or approve AP-06 code.

### E-06 D-42 Retry Clarification

Date: 2026-10-26. The user clarified that Gateway timestamps should preferably be whole seconds and all finer precision is ignored/truncated in every aspect. For an otherwise identical Submission with the same Gateway, Submission ID and UTC offset, differing fractional digits that truncate to the same second are therefore the same retry. [C-01](../contracts/c-01-transmission-acceptance.md) and [F-47](../contracts/reference-cases.md#ap-04-observation-precision-case) now state this. Other content or offset differences remain conflicts. This resolves the D-42 retry question without changing prior cases or AP baselines.

### E-07 AP-04 C-04 Scope Request

Date: 2026-10-26. The user declined to extend the existing AP-04 T017/T018 follow-up authorization to C-04 and chose to keep the approved scope unchanged. C-04 remains outside that follow-up; no C-04 amendment or freeze is authorized by this request. AP-08 T029's contract gate includes C-04, so its standalone matcher handover to AP-06 remains blocked until the required C-04 slice is separately reviewed and approved. No AP-08 task or AP baseline was changed.

### E-08 D-45 Gateway Transport Follow-up

Date: 2026-10-26. D-45 approves direct HTTP support for Gateway requests and records the residual risk. The credential mechanism was still unresolved at that point; no Ed25519 or replay-protection requirement was approved. This resolves the prior TLS-only transport conflict, not the C-01/C-06 freeze, AP-06 execution binding or implementation approval. T022 remains blocked.

### E-09 D-46 Grouping Window Follow-up

Date: 2026-10-26. The user chose no additional product maximum for the AP-06 Grouping window. C-03's current proposed logical representation is whole seconds in `[0, MaxInt64]`, and [F-50](../contracts/reference-cases.md#ap-06-grouping-window-range-case) is planned but not executed. This resolves the product cap question, not P-08 duration/concurrency validation, C-03 freeze, the remaining prerequisite handovers or AP-06 implementation approval.

### E-10 D-47 Gateway Credential Follow-up

Date: 2026-10-26. The user selected a reusable per-Gateway Bearer credential, including over HTTP, after clarifying that replay protection and non-reusable credentials were not requirements. The decision is [D-47](../authoritative-plan.md#ap-06-gateway-credential-decision); F-49 is updated for this choice but remains unexecuted. HTTP token exposure and the lack of general replay protection are explicit. C-06 lifecycle details and the AP-06 execution/freeze gates remain open; T022 is still blocked.

### E-11 C-04 Minimum Review

Date: 2026-10-26. The user approved the limited C-04 review scope in [AP-04 E-22](ap-04.md#e-22-ap-06-c-04-minimum-scope-approval). Review found the approved Unit identity/revision, deletion and C-05 handover minimum already present in C-04; [AP-04 E-23](ap-04.md#e-23-ap-06-c-04-minimum-review) records why no duplicate amendment was made. This is not a C-04/C-05 freeze or the AP-08 T029 handover.

### E-12 D-48 Gateway Credential Lifecycle

Date: 2026-10-26. Gateway credentials do not expire automatically; the old credential is revoked atomically on the replacement's first successfully authenticated request. See [D-48](../authoritative-plan.md#ap-06-gateway-credential-lifecycle-decision) and [C-06](../../docs/http-api.md#gateway-authentication). The contract and F-49 remain unfreezed/unexecuted, so T022 stays blocked.

### E-13 D-49 Grouping Window Precision

Date: 2026-10-26. The user approved whole-second `W` values. C-03 reflects the decision; [F-50](../contracts/reference-cases.md#ap-06-grouping-window-range-case) remains planned and unexecuted. This does not resolve the proposed `MaxInt64` representation or P-08 duration/concurrency validation; T023 remains blocked.

### E-14 D-50 Grouping Validation Timing

Date: 2026-10-26. Product-specific PostgreSQL duration/concurrency validation is a mandatory AP-06 T024 check after implementation, not a pre-freeze blocker; P-08 remains open until it passes. See [D-50](../authoritative-plan.md#ap-06-grouping-validation-timing) and [AP-04 E-27](ap-04.md#e-27-ap-06-grouping-validation-timing). The tests have not run, and T024 remains blocked by its upstream contracts and handovers.

### E-15 D-51 Grouping Window JSON Integer Limit

Date: 2026-10-26. The user explicitly approved a JSON integer `W` in `0..2^53−1` seconds, superseding D-46's no-additional-cap decision. See [D-51](../authoritative-plan.md#ap-06-grouping-window-json-integer-limit-decision), [C-03](../contracts/c-03-message-grouping.md) and planned [F-50](../contracts/reference-cases.md#ap-06-grouping-window-range-case). F-50 must be included in AP-04 T018's T024 validation binding. No AP-06 implementation or test was run; T023 remains blocked on contract freeze and T024 on all required upstream gates.

### E-16 D-52 Application-wide HTTP

Date: 2026-10-26. The user extended HTTP support from Gateway requests to every GoPager application function, including human login, Recovery and Session use. Direct HTTP is supported without a TLS guarantee; Operators may terminate TLS at a reverse proxy and are responsible for protecting exposed transport paths. See [D-52](../authoritative-plan.md#application-wide-http-support-decision), [C-12](../../docs/http-api.md#basics) and [C-06](../../docs/http-api.md#authentication). This is a documentary decision only: no AP baseline or prior evidence changed, owner contracts remain draft, and no AP-06 implementation approval was granted. T022–T024 remain blocked.

### E-17 Original Target Reference and Prerequisite Preparation

Date: 2026-10-26. The user approved the original-target reference in [D-53](../authoritative-plan.md#ap-06-transmission-target-reference-decision) and separately authorized documentary preparation of required prerequisite slices. [AP-04 E-30](ap-04.md#e-30-original-target-reference-and-prerequisite-preparation) records the follow-up. This advances contract preparation, not implementation readiness; F-52 remains planned and T022–T024 remain blocked pending their actual freezes, execution bindings and verified handovers.

### E-18 Narrowed Identity Prerequisite Available

Date: 2026-10-27. The user approved [D-59](../authoritative-plan.md#identity-prerequisite-scope-decision): bring forward only the AP-06-required identity core while retaining the AP-07-oriented ports/model. [AP-07 E-13 and handover](source-ap-07.md#e-13-d-59-narrowed-prerequisite-handover) record actual PostgreSQL/race/HTTP evidence for the resulting core; this is neither whole AP-07 completion nor Gateway credential acceptance. Future general account/grant/key/Recovery management does not gate AP-06.

The AP-08/AP-12/AP-06 consumers still have to compose their real scope references and protected query/projection surfaces, fail closed for absent owners, and obtain their respective contract/execution approvals. AP-08 T028/standalone T029 and AP-12 T043 handovers are not present. Their documentary preparation is already authorized, including necessary C-04 dependencies, by the prerequisite-preparation plan gate; no additional permission to merely prepare those candidates is needed.

### E-19 Minimal Master-data and Matcher Candidates

Date: 2026-10-27. Prepared [D-60](../authoritative-plan.md#early-master-data-and-matcher-prerequisite-proposal) and the exact [AP-04 producer/consumer binding](ap-04.md#ap-08-prerequisite-consumer-binding-proposal) for the next required prerequisite. It includes only Unit/collection identity/current management and current temporal matching, not full Organization/default/template/bulk or historical correction work. Four candidate owner sections identify typed target/rule/Unit interfaces, current authority, atomic configuration facts, failures and focused PostgreSQL/HTTP validation. These are proposed, not frozen or implemented; no AP-08 execution record was created. `git diff --check` and the unchanged AP-baseline check are the applicable documentary checks; earlier product tests certify only the identity slice in E-18.

### E-20 Minimal Prerequisite Contract Freeze

Date: 2026-10-27. The user explicitly approved `minimal_matcher_contract_freeze=approve`. [AP-04 E-35](ap-04.md#e-35-minimal-master-data-and-matcher-freeze-approval) records the exact four frozen owner subsets under D-60. This resolves the next prerequisite's contract-freeze gate, not its separate implementation authorization or actual handover. E-19 preserves the pre-approval state.

### E-21 Verified Minimal Matcher Prerequisite

Date: 2026-10-28. The separate `minimal_matcher_execution=approve-and-start` under D-60 has now been implemented and verified for its exact narrow scope. [The AP-08 handover](ap-08.md#handover) links the actual typed Protocol/Unit/current-rule/fact/receipt/schema/HTTP artifacts and real PostgreSQL/race/proxy evidence. This supersedes the former current blocker about a missing implementation start/handover, without rewriting E-19/E-20's historical observations. Production Network ownership remains AP-06's consumer obligation; neither full AP-08 nor AP-06 acceptance follows.

### E-22 Processing Kernel Preparation and Probe Gate

Date: 2026-10-28. [AP-04 E-36](ap-04.md#e-36-t043-boundary-and-runtime-preparation) records the continued authorized T043 preparation and the actual local/upstream review, not a runtime test. [D-61](../authoritative-plan.md#processing-budget-feasibility-proposal) requests only the missing bounded experiment authorization; it does not weaken the existing budget, reopen Go/single-process choices, or start AP-12/AP-06 product implementation. The actual kernel handover remains outstanding.

### E-23 Executed Budget Proof and Runtime Selection Gate

Date: 2026-10-28. The user approved D-61, and [AP-04 E-38](ap-04.md#e-38-budget-and-memory-probe-results) records actual native counterexamples and the bounded embedded Go/WASI proof, including failed attempts, exact limits and hashes. The evidence supports [D-62's proposed runtime selection](../authoritative-plan.md#core-regex-runtime-selection-proposal), not an implicit production/library approval or AP-12 completion. No AP-06 intake/Grouping/publication artifact was produced; the concrete T043 freeze/binding and handover remain outstanding.

### E-24 Corrected Operator-start Requirement

Date: 2026-11-01. The user explicitly corrected the single-process assumption and required all application dependencies to be supplied. [D-63](../authoritative-plan.md#operator-start-and-bundled-dependency-clarification) owns the approved correction; [AP-04 E-39](ap-04.md#e-39-operator-start-clarification-and-prior-decision-review) records affected source/decision review and preserved history. Child processes are admissible, not automatically validated or selected. D-62 is reopened, and D-64 proposes the missing bounded child comparison; neither request cancellation nor the general clarification selected Wasm or a child runtime. The AP-06 baseline and existing implementation/evidence remain unchanged.

### E-25 Child and Native-alternative Follow-up

Date: 2026-11-01. The user separately approved D-64's isolated child probe and the narrow AP-15 T052 amendment. [AP-04 E-40](ap-04.md#e-40-child-probe-start-and-approved-downstream-amendment) records completed probe compilation, the Docker-daemon failure before runtime tests, and the actual approved baseline amendment. [E-41](ap-04.md#e-41-native-plugin-alternative-review) records the user's D-65 native-alternative research approval and completed documentary comparison, not native implementation. AP-06/T043 still needs the actual runtime choice/freeze/handover; no standalone helper compilation is ingest/publication acceptance.

### E-26 Executed Child Runtime Comparison

Date: 2026-11-01. The user made Docker available and instructed continuation. [AP-04 E-42](ap-04.md#e-42-executed-bundled-child-comparison) now records actual D-64 limit/capture/failure/lifecycle tests. Both authorized memory-limit tuples failed ordinary-input conformance; bounded child failure, parent survival and separate cancellation/wait/reap tests succeeded. This is not a blanket rejection of managed children, a passing whole native evaluator, or an AP-06 runtime test. D-62's selection remains explicit and now has the corrected evidence-based comparison.

### E-27 Approved Regex and Concrete Kernel Preparation

Date: 2026-11-01. The user explicitly selected the embedded Core regex direction under D-62; [AP-04 E-43](ap-04.md#e-43-core-regex-technical-selection) records this narrow approval. The runtime choice is no longer a blocker. [E-44](ap-04.md#e-44-concrete-kernel-owner-candidates) now supplies concrete unfrozen source/predicate/Workflow/installed-call/fact candidates and the remaining D-66/D-67 technical choices. No production intake/Grouping/publication code or AP-12 start follows from documentary preparation; actual freeze/execution/handover remains required.

### E-28 Installed Boundary Selected; Predicate Choice Pending

Date: 2026-11-01. [AP-04 E-45](ap-04.md#e-45-installed-boundary-selection-and-predicate-explanation-request) records the user's separate D-67 selection and request to explain D-66 in the chat before deciding. The installed-step technical direction is no longer open; shared-predicate selection and subsequent freeze/execution/handover remain outstanding. No implementation approval is inferred.

### E-29 Fixed Field Grouping Direction

Date: 2026-11-01. The user selected `field_group_definition=fixed-configurable-groups` after discussion of visibility, beneficiary search and internal authorization criteria. [D-68](../authoritative-plan.md#field-visibility-grouping-direction) records the narrow grouping choice and explicitly distinguishes pending field membership/frozen-contract amendments/address-dependent grants. No grant/action catalog, existing field semantics or product code changed. The concrete C-06/C-07 matrix is the next documentary preparation step; D-66 remains undecided.

### E-30 Resumed AP-06 and Field Matrix Preparation

Date: 2026-11-01. The user explicitly resumed `/execute-work-package AP-06`. Reviewed the unchanged T022/T023/T024 baseline and current prerequisite/gate handover. D-62/D-67 technical selections remain approved; D-66 remains unselected after its chat explanation. Global-view visibility remains explicitly deferred and is not treated as a new AP-06 blocker.

Prepared [C-06 field-group-review 0.1-candidate](../contracts/c-06-identity-authorization.md#planned-and-not-yet-implemented) and C-07's linked query-eligibility note under D-68/documentary authorization. The matrix retains current identifiers, separates readable presentation from rule internals and operation rights, and explicitly marks source/receipt subfield and single-Gateway authorization gaps. No frozen grant model/field semantics, application source, AP-09/AP-10 task or AP baseline changed. This preparation is not a new permission freeze or runtime evidence.

### E-29 Approved T043 Start and Verified Partial Kernel Progress

Date: 2026-11-01. The user's separate five-part freeze and actual-start approvals are recorded at [D-66](../authoritative-plan.md#t043-shared-predicate-binding-proposal) and the [T043 execution binding](ap-04.md#processing-kernel-execution-binding). They are no longer open gates. Subsequent D-71/D-72/D-73 approvals cover current execution-Principal resolution, target-result binding and bounded installed loading.

[source AP-12 E-09..E-20](source-ap-12.md#e-09-target-result-kernel-implementation-and-bounds) own actual runtime/fact/configuration persistence, coordinated management and explicit scope/invocation-context evidence. E-20's full real-primary, root HTTP and Processing Race/Vet checks pass. [E-22](source-ap-12.md#e-22-approved-dedicated-preview-invocation) records the subsequently approved D-76 separate-preview implementation and focused verification; E-20 predates it. T043 remains incomplete: current instance/source adapters, durable candidate/result lifecycle, protected Processing HTTP and fenced publication are required. [E-23/D-77](source-ap-12.md#e-23-reselection-identity-and-terminal-work-review) preserve the same-key reselection review, now resolved and partially implemented in E-24 below. No AP-06 Gateway/intake/Grouping/readiness implementation or product validation is inferred.

[D-74](../authoritative-plan.md#processing-internal-retry-jitter-bound) identifies one newly discovered missing internal retry numeric bound; no Delivery policy or Gateway replay protection is silently adopted. This question does not reopen the already approved CEL/Principal/target/artifact choices.

[D-77](../authoritative-plan.md#processing-reselection-identity-and-terminal-work) subsequently selected revisioned same-key reselection, not a new execution-generation table or key extension. [source AP-12 E-24](source-ap-12.md#e-24-approved-revisioned-candidate-admission-and-primary-clock-claims) records actual admission/reselection/claim/renewal validation; the complete worker and AP-06 source integration remain outstanding.

### E-31 Session Repercussion Consolidation

Date: 2026-11-02; T022..T024 prerequisite coordination, approved by [D-95](../authoritative-plan.md#ap-06-session-repercussion-consolidation) after the AP-01..AP-06 review. Assignee: current sequential GoPager assistant. HEAD `24318de414b3b35f77b6b288905125c1debf46f4`; extensive existing uncommitted source/contract changes and earlier approved AP-15 topology delta are preserved. Writes are limited to the five existing architecture/plan/AP-04/AP-05/AP-06 documents. No AP definition, product source, owner contract meaning, dependency or glossary term changes in this follow-up.

**Impact disposition:** AP-01..AP-03 historical outcomes need no amendment from this session's selected transport. [AP-04 E-53](ap-04.md#e-53-ap-06-publication-and-follow-up-binding) links D-94's exact C-08 freeze/write binding and D-95's documentary scope. The central approval table now routes to both decisions. [AP-05 E-19](ap-05.md#e-19-library-review-and-current-handover-addendum) records the completed standard-library correction, actual unit checks and the current versus historical foundation handover. Architecture now explicitly distinguishes original experiments/candidate statements from [the limited current selection](../architecture.md#proposed-postgresql-integration-slice); its stable proposal heading is retained for incoming links.

**Current producer inputs:** [source AP-12 E-31..E-38](source-ap-12.md#e-31-private-worker-coordination-and-supersession) and subsequent [E-46](source-ap-12.md#e-46-approved-postgresql-watermill-publication-slice) are producer evidence, not execution of whole AP-12. They add private worker/lifecycle/publication groundwork and actual PostgreSQL reference publication for existing identity/master-data/Processing appenders. E-46 includes all five append-family atomicity, migrated storage, restart/group/Ack cases and existing producer regressions, with a timed-out broad Race run explicitly retained. Do not copy those results here or label them AP-06 intake evidence.

**Still required for AP-06:** C-01/C-02/C-03 and the applicable C-05/C-06/C-08/C-11 boundary must pass AP-06's own freeze/execution binding; implement Gateway/Network/Submission and original/Message ownership, typed intake/Grouping facts, initial matching and actual candidate admission/reconciliation/fenced source publication. No existing identity/Processing transport test substitutes for these missing domain producers. Optional post-commit wake hints must never replace durable candidate discovery. T024 must test the actual acceptance transaction, exact/late/bridge Grouping, concurrent/retry/crash outcomes and all required state/facts/work atomicity on PostgreSQL. Retain portable domain cases for later Lite; do not make SQLite renewal or an AP-10 Delivery consumer a current AP-06 blocker.

The separately approved T043 prerequisite is still not a complete verified handover: [its current execution status](source-ap-12.md#task-outcomes) retains protected HTTP/trusted preparation and actual source-owner integration gaps. [D-86](../authoritative-plan.md#processing-cold-start-test-observation) remains proposed for the earlier root process-test artifact. Resolving those is distinct from this documentary reconciliation; no implicit approval to change a test threshold or implement general AP-12/AP-10 functionality follows.

**Verification:** Inspected the new/changed local links and their target headings with focused file reads and `rg`, including D-94/D-95, E-19/E-31/E-46/E-53, current selection, contract and process-test gates. Corrected the new [source AP-12 E-31](source-ap-12.md#e-31-private-worker-coordination-and-supersession) fragment to its actual stable heading. The existing `slices.Contains` role helper and AP-06 protocol-only source boundary were inspected; the preceding review's actual unit results are recorded once in AP-05 E-19. No runtime tests are required or rerun for these documentation-only changes.

From `/Users/leokr/Projects/gopager`, `git diff --check` exited 0; `git diff --stat -- docs/work-packages` showed only the pre-existing AP-15 one-insertion/one-deletion amendment; `git diff --name-only -- docs/execution/ap-01.md docs/execution/ap-02.md docs/execution/ap-03.md` returned no changes. These are targeted documentary checks, not a repository-wide automated Markdown-link audit or a fresh runtime pass. All five identified session repercussions are now documented at their owning locations. Remaining implementation/freeze/test-policy work is explicitly excluded and not reported complete.

## Blockers and Deviations

Current prerequisite/impact disposition is [E-31](#e-31-session-repercussion-consolidation); E-29's approved-start evidence is historical partial progress. The sequential prerequisite findings below preserve earlier stages and their then-pending gates; they are not a second current approval source.

### Unscoped prerequisite replacement preparation

The user approved the bounded D-66 CEL comparison after pausing the initial engine answer. [AP-04 E-50](ap-04.md#e-50-bounded-cel-compatibility-and-cost-comparison) owns executed probe evidence and [architecture](../architecture.md#cel-go-incremental-capability-review) the revised restricted-CEL recommendation. The comparison is complete and the user subsequently selected restricted CEL-Go; [E-51](ap-04.md#e-51-final-predicate-selection-and-kernel-freeze-preparation) records current candidate alignment. T043 freeze/start remain open, not an implemented AP-06 handover.

Subsequent kernel gate: [E-52](ap-04.md#e-52-coordinated-processing-kernel-freeze) records the user's five-part processing-kernel 1.0 freeze. Only the separate [execution binding](ap-04.md#processing-kernel-execution-binding) and its actual start remain open before T043 product work. A frozen contract is not the verified kernel handover.

Date: 2026-11-01, renewed explicit AP-06 start. [AP-04 E-47](ap-04.md#e-47-unscoped-field-replacement-preparation) records actual implementation inspection and the D-70 replacement/transition candidate. This is necessary correction of the prior identity handover under D-69, not whole AP-07 execution or completed AP-06 code. Global field visibility and scoped data rights remain separate; no existing limited field grant is silently promoted. Existing-data transition choice and subsequent replacement freeze/execution are pending.

Current resolution: [AP-04 E-48](ap-04.md#e-48-no-deployment-clarification) records the user's no-deployment clarification. Production transition/reassignment is no longer a blocker or required feature; replacement freeze/execution preparation targets fresh installations. Local data is not silently reset and earlier runtime evidence remains historical.

Replacement freeze: [AP-04 E-49](ap-04.md#e-49-unscoped-field-replacement-freeze) records the explicit identity-fields 1.0 approval. The separate [correction execution binding](ap-04.md#unscoped-identity-correction-consumer-binding) is prepared; actual start/handover is not inferred from the freeze.

Correction handover, 2026-11-01: the separate execution approval was granted and implemented. [AP-07 correction verification](source-ap-07.md#unscoped-field-correction-verification-and-handover) owns the real PostgreSQL/Race/HTTP/Proxy evidence and artifact links. This replaces the earlier scoped-field prerequisite semantics, not the AP-06 baseline. D-66 and the T043 freeze/execution gates still require resolution; AP-06 intake/Grouping/publication remains unimplemented.

Field-visibility correction, 2026-11-01: the user explicitly rejected per-Unit/per-source scopes on field visibility. [D-69](../authoritative-plan.md#unscoped-field-visibility-clarification) records the instructed product direction and [C-06](../../docs/http-api.md#authorization) owns its meaning. Previous candidate/frozen scoped-field interpretation is identified as historical rather than silently propagated. Scope-based data access remains separate; current identity implementation/keys/detail composition and existing grants require a concrete replacement freeze/migration binding. No product code or frozen artifact was changed, and D-66 remains undecided.

User-story follow-up, 2026-11-01: the user explicitly requested five delegated-administration/restricted-view scenarios be added for planning. The canonical [REQ-133..137 addendum](../requirements-catalog.md#n-delegated-administration-and-restricted-views) records goals, positive/negative acceptance and links to existing requirements. The user then explicitly mapped Source to Gateway, Agency to Organization and Unit filters to saved search/view filters, not technical assignment rules. The addendum uses those canonical terms, without a domain rename. Single-Gateway authorization and delegated saved-view/filter management require concrete owner bindings; existing Network-scoped Address-rule management does not satisfy the clarified REQ-133. No new contract freeze, code or AP-baseline change follows.

Saved-view follow-up, 2026-11-01: the user clarified global administratively managed versus strictly personal saved views, independent of mandatory Network ownership; Network remains a possible filter. The requirement owner updates REQ-133's initial Network example and adds REQ-138 for personal views. Global visibility remains a real choice between automatic rights/filter intersection and explicit view grants, not a settled approval. Delivery Subscription GUI filter preview is recorded as a planning candidate, not identical sending authority or dispatch on preview. These consumer findings do not start AP-09/AP-10 or change their baselines; C-06/C-07/C-09 bindings and the initial delegation-scope question remain unresolved.

The user explicitly deferred global-view visibility for later review (`Entscheidung offen lassen, später prüfen`). No default visibility policy is adopted; this pending later-consumer choice is not made an additional blocker for unrelated T043/AP-06 work. Story-only checks `git diff --check` and the unchanged-AP diff excluding the approved AP-15 amendment passed (exit 0); no executable product tests were run or implied.

- Product implementation, technical contract freeze and execution binding for AP-06 are absent. The applicable owner is Product/integration; only the plan approval gate can record them.
- D-42 and D-43 are approved but do not freeze the full C-01/C-02/C-03/C-05/C-06/C-08/C-11 slice; AP-04 T017/T018 review and the AP-06 consumer binding remain incomplete.
- C-04/C-05's narrow prerequisite subsets and their C-02/C-08 inputs are verified under D-60/E-21; unrelated general C-04 and historical correction work stays deferred.
- AP-07's narrowed identity core and AP-08's narrowed matcher are available under D-59/E-18 and D-60/E-21. AP-12 T043 freeze/start are approved and implementation is in progress; its full verified handover is still required. Domain scope/projection composition remains an obligation of the consuming slices, not a reason to implement deferred AP-07 management.
- T022, T023 and T024 remain blocked. No AP baseline, earlier execution result or downstream package was changed.

## Handover

No AP-06 intake/Grouping/publication artifact is produced. D-59 identity and D-60 matcher inputs are available through E-18/E-21, with the verified D-69/D-70 global-field correction linked above. [E-31](#e-31-session-repercussion-consolidation) is the current prerequisite handover, including D-94's implemented publication foundation and remaining source/kernel/root gates. [AP-04 E-53](ap-04.md#e-53-ap-06-publication-and-follow-up-binding) binds the additive approval without starting whole AP-12 or AP-10.

Next permitted step: resolve the remaining required T043/root handover and prepare/obtain AP-06's own exact owner freeze and implementation binding, keeping actual AP-06 source ownership with this package. The documentary session repercussions are not permission to change D-86's test threshold or to start a production event consumer. Resume product changes only under the respective explicit binding; AP-01..AP-06 baselines remain unchanged and no downstream baseline amendment is required by this follow-up.

### E-32 Resliced Execution Path Reconciliation

Date: 2026-11-03. Under the user's explicit [D-96 approval](../authoritative-plan.md#work-package-reslicing-decision), this record remains the historical evidence owner for original AP-06. T022..T024 remain blocked against that source baseline; they are not currently active. Their future execution scope was assigned to historical [AP-18](../work-packages/ap-07-transmission-intake.md) in the D-96 path, after the then-current AP-17 kernel handover. The D-59/D-60 inputs and D-94 publication evidence above remain producer evidence, not AP-18 completion. No prior result or original AP-06 definition is overwritten.

Under [D-97](../authoritative-plan.md#ordered-ap-renumbering-decision), historical AP-18 is now current AP-07, and the same active kernel slice is current AP-06. This is only an ID/navigation change; T022..T024 remain blocked historical outcomes and are not completion evidence for current AP-07.
