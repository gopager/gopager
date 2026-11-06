# ADR: Processing and plugins

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-05"></a>
## D-05 Extension distribution/SDK boundary

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed extension distribution/SDK boundary, elaborated by D-24.
- **Owner detail:** [C-10](../contracts/c-10-plugin-host.md)

<a id="d-12"></a>
## D-12 Processing minimum and extensibility constraints

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed processing minimum and extensibility constraints.
- **Owner detail:** [C-11](../contracts/c-11-processing-presentation.md)

<a id="d-24"></a>
## D-24 Plugin trust/capability model

- **Status:** superseded by D-130 and D-135 (in part: the capability-approval vocabulary)
- **Date:** 2026-10-25
- **Superseded by:** [D-130](04-processing-and-plugins.md#d-130), [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Confirmed plugin trust/capability model.
- **Owner detail:** [C-10](../contracts/c-10-plugin-host.md)

<a id="d-25"></a>
## D-25 Ingest publication and presentation boundary

- **Status:** superseded by D-135 (in part: the ingest publication barrier)
- **Date:** 2026-10-25
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Confirmed ingest publication and presentation boundary.
- **Owner detail:** [C-11](../contracts/c-11-processing-presentation.md), [C-09 predicates](../contracts/c-09-delivery.md)

<a id="d-33"></a>
## D-33 AP-04 Scope Decision

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Approved AP-04 Webhook capability expansion; initial Pushover/Webhook, Telegram/email later; qualifies D-06 timing without technical freeze.
- **Owner detail:** [AP-04 scope decision](01-platform-and-storage.md#d-32)

<a id="d-61"></a>
## D-61 Processing Budget Feasibility Proposal

- **Status:** accepted (evidence only)
- **Date:** 2026-10-28
- **Decision:** Approved bounded disposable Processing budget probe; evidence only.
- **Consequences:** The user's explicit approval adds this exact application-stage experiment authorization; it does not reopen or supersede an earlier D-ID, select a production dependency, freeze C-11/C-09/C-10/C-02/C-08, start AP-12 or AP-06 implementation, or authorize a full plugin/Delivery engine. The concrete kernel contract and execution binding follow the evidence and retain their separate approval gates. AP-12 T043/T045 and AP-06 T023/T024 consume that future handover; no AP-baseline amendment is presently proposed.
- **Owner detail:** This record is the only current definition

<a id="d-62"></a>
## D-62 Core Regex Runtime Selection Proposal

- **Status:** superseded by D-129 (for Core regex)
- **Date:** 2026-10-28
- **Superseded by:** [D-129](04-processing-and-plugins.md#d-129)
- **Decision:** Embedded Wasm Core regex selected after comparison; superseded for Core regex by D-129. The user selected the tested embedded Go/Wasm regex option after the actual child comparison. This authorizes the narrow technical direction/tuple and corresponding TECH-08 Core-runtime ownership clarification, not a prohibition on children/native plugins. Previous pre-approval statements above remain proposal history. Preserve limits and supplied artifact/runtime requirement; concrete kernel/C-ID freeze and execution authorization remain separate.
- **Owner detail:** This record is the only current definition

<a id="d-65"></a>
## D-65 Native Plugin Alternative Review

- **Status:** superseded by D-131
- **Date:** 2026-11-01
- **Superseded by:** [D-131](04-processing-and-plugins.md#d-131)
- **Decision:** Native plugin alternatives reviewed documentarily; resolved by D-131.
- **Owner detail:** This record is the only current definition

<a id="d-66"></a>
## D-66 T043 Shared Predicate Binding Proposal

- **Status:** accepted
- **Date:** 2026-11-01
- **Decision:** Restricted CEL-Go selected for the shared typed predicate; Processing kernel 1.0 freeze and start. After the measured comparison and revised chat recommendation, the user selected `predicate_engine_final=restricted-cel-go`. Choose B: CEL-Go as the internal checked evaluator of the closed structured representation, with constrained declarations/operators and separate absence/classification/current-authority adapters. Initial production binding targets the tested v0.33.2, with the actual product dependency tuple and integration verified only after the required freeze/start. The custom-engine exception is not adopted.
- **Owner detail:** This record is the only current definition

<a id="d-67"></a>
## D-67 Narrow Installed Processing ABI Proposal

- **Status:** superseded by D-131 (in part: runtime; the pure-step declaration stays)
- **Date:** 2026-11-01
- **Superseded by:** [D-131](04-processing-and-plugins.md#d-131)
- **Decision:** Pure WASI-command installed-processing subset; runtime part superseded by D-131, pure-step declaration retained. The user selected A, the narrow pure WASI-command processing boundary. Apply that selection to the candidate binding without presenting the general callback SDK or effectful/native capabilities as implemented. D-66 remains undecided: the user requested an explanation in the chat before choosing its recommendation. Neither the D-67 answer nor that explanation request authorizes a shared predicate implementation, contract freeze or AP-12 start.
- **Owner detail:** This record is the only current definition

<a id="d-72"></a>
## D-72 Multi-target Workflow Context Binding

- **Status:** accepted
- **Date:** 2026-11-01
- **Decision:** Per-target Workflow results (workflow-target-results 1.1) replace the ambiguous post-selection binding.
- **Owner detail:** This record is the only current definition

<a id="d-73"></a>
## D-73 Installed Processing Artifact Load Bound

- **Status:** superseded by D-131
- **Date:** 2026-11-01
- **Superseded by:** [D-131](04-processing-and-plugins.md#d-131)
- **Decision:** Fixed 32 MiB Wasm artifact load bound; superseded by D-131. The user selected the exact fixed 32 MiB option after its larger-module exclusion and non-RAM-guarantee were explained. Extend only the C-10 installed-processing load boundary and implement/test it under current T043. No new runtime configurability or wider code-execution capability follows.
- **Owner detail:** This record is the only current definition

<a id="d-74"></a>
## D-74 Processing Internal Retry Jitter Bound

- **Status:** accepted
- **Date:** 2026-11-01
- **Decision:** Symmetric ±20% jitter for internal Processing retries. The user selected `approve-symmetric-20-percent-and-implement` after the exact resulting intervals, earlier-than-nominal retry trade-off and separation from Delivery/Gateway behavior were explained. Apply only the selected narrow C-08 jitter completion in the already approved kernel; no additional execution or permission gate is implied.
- **Owner detail:** This record is the only current definition

<a id="d-75"></a>
## D-75 Processing Durable Selection Placement

- **Status:** accepted (amended by D-135: current run only)
- **Date:** 2026-11-01
- **Decision:** Scoped candidate work: one durable evaluation item per eligible Workflow version, selector and body in one evaluation; run-row simplification under D-135. The user selected `approve-candidate-work-and-implement` after the no-match durable-row overhead and simpler single lifecycle were explained. Bind and implement only scoped candidate admission with the existing complete evaluator, preserving selector/body budget and source-publication/lease semantics. No separate selector queue or split-budget protocol is selected.
- **Owner detail:** This record is the only current definition

<a id="d-76"></a>
## D-76 Dedicated Installed Preview Artifact Binding

- **Status:** superseded by D-131 and D-135 (preview mode replaces the preview artifact)
- **Date:** 2026-11-02
- **Superseded by:** [D-131](04-processing-and-plugins.md#d-131), [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Separately pinned pure preview artifact; runtime part superseded by D-131, replaced by a preview mode under D-135. The user selected `approve-separate-preview-artifact-and-implement`. Apply option A under the existing execution binding: optional separately provisioned/pinned preview WASM and manifest, unchanged command tuple/bounds, no normal-artifact fallback. The entrypoint/ABI alternative is not selected.
- **Owner detail:** This record is the only current definition

<a id="d-77"></a>
## D-77 Processing Reselection Identity and Terminal Work

- **Status:** superseded by D-135 (run model)
- **Date:** 2026-11-02
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Revisioned same-key candidate reselection (option A); obsolete under D-135's run model.
- **Owner detail:** This record is the only current definition

<a id="d-78"></a>
## D-78 Processing Failed-work Retry Action Binding

- **Status:** accepted (amended by D-135: action renamed `workflow.reprocess`)
- **Date:** 2026-11-02
- **Decision:** Failed-work retry authorized by existing scoped `rule.reprocess`.
- **Owner detail:** This record is the only current definition

<a id="d-79"></a>
## D-79 Superseded Work Terminality Interpretation Confirmation

- **Status:** superseded by D-135 (run model)
- **Date:** 2026-11-02
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Terminal superseded work means the old execution revision; obsolete under D-135's run model.
- **Owner detail:** This record is the only current definition

<a id="d-80"></a>
## D-80 Processing Source Context Field-group Membership

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** Source-context field-group memberships: Protocol/form under `payload`, resolved Network under `gateway`.
- **Owner detail:** This record is the only current definition

<a id="d-81"></a>
## D-81 Installed Result Requirement Interpretation

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** Installed result requirements are the result schema (option B); no per-output field-group declaration.
- **Owner detail:** This record is the only current definition

<a id="d-82"></a>
## D-82 Exhausted Candidate Reselection State

- **Status:** superseded by D-135 (run model)
- **Date:** 2026-11-02
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Exhausted candidate reselection is immediately `failed`; obsolete under D-135's run model.
- **Owner detail:** This record is the only current definition

<a id="d-83"></a>
## D-83 Processing Dispatcher Concurrency Bound

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** Processing dispatcher concurrency 2 per Worker process.
- **Owner detail:** This record is the only current definition

<a id="d-84"></a>
## D-84 Processing Dispatcher Idle Wake-up Bound

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** Processing fallback poll 5 s with local wake-ups.
- **Owner detail:** This record is the only current definition

<a id="d-85"></a>
## D-85 Processing Scheduling Configuration Preparation

- **Status:** accepted (amended by D-135: Operator configuration)
- **Date:** 2026-11-02
- **Decision:** Scheduling values prepared as injectable configuration; Operator exposure added under D-135. Prepare validated injected scheduling configuration, retaining D-83/D-84's selected initial defaults. Do not scatter fixed scheduling literals through runner mechanics. Operator flags/environment/UI and a final centrally fixed policy remain later choices, not implied by this instruction. [C-08](../../docs/workflows.md#worker-dispatch) owns this scheduling boundary; executable configuration lives in [the dispatcher](../../internal/processing/worker/dispatcher.go).
- **Consequences:** Additive to the approved in-progress dispatcher/root/test scope. No AP baseline, earlier platform API, lease/renewal/retry or external Delivery policy change. This does not automatically reopen existing fixed contractual resource/security limits.
- **Owner detail:** This record is the only current definition

<a id="d-86"></a>
## D-86 Processing Cold-start Test Observation

- **Status:** accepted
- **Date:** 2026-11-02
- **Decision:** Process test cold-start observation window widened to the call deadline plus one second. The user approves option A: only [the process test](../../cmd/gopager/process_test.go) and current root verification/evidence are affected. No AP definition, earlier observed outcome, runtime selection, regex/guest budget, readiness bypass, silent lazy initialization or weakened shutdown assertion. Apply the 31-second cold-start observation and rerun both actual process modes, retaining the five-second shutdown check. Approval is not a successful current test or a product startup SLA; current AP-06 owns the new execution evidence.
- **Owner detail:** This record is the only current definition

<a id="d-99"></a>
## D-99 Synthetic Preview Schema Completion

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved preparation of complete synthetic-preview form schemas; exact schema/context freeze remains separate. C-02 processing-source 1.0 provides the trusted typed projection, but does not bind public synthetic input field paths/form schemas/context identity. Choose a deliberately restricted POCSAG/FLEX first version or prepare the complete form catalog. The user selected the complete catalog. Prepare the versioned closed DTO, field/type/form mappings, synthetic identity/context and authorization rules and obtain their separate specific approval before implementing that schema.
- **Consequences:** Additive clarification at C-02/C-11 with actual artifact links after approval; preserve frozen source-envelope semantics, classifications and historical evidence. This does not start AP-07 original intake, require later source ownership, approve a partial preview as complete or amend AP baselines. Exact schema approval and implementation evidence remain pending.
- **Owner detail:** This record is the only current definition

<a id="d-100"></a>
## D-100 Processing Instance Read Authorization

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved global plugin.configure for redacted Processing-instance reads/listing. Instance changes already require global `plugin.configure`, while instance read/list authorization is not concretely bound. Use the same existing global action for redacted configuration reads/listing, or separately design delegated instance rights. The user selected global `plugin.configure`; no new action or instance scope. C-06/C-10 own this narrow read binding.
- **Owner detail:** This record is the only current definition

<a id="d-102"></a>
## D-102 Synthetic Schema Freeze Responses

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved synthetic schema/context and null-or-name ZVEI; subsequent general bytes approval is D-103. User selected `schema_context_freeze=approve-schema-context`: closed versioned full-form synthetic input, existing Network selection/current matcher for Unit eligibility, owner-generated synthetic identity excluded from work/publication, separate current action/scope/global field checks, existing whole-second Observation handling without a new age cap, no live writes and separate pure installed preview. Schema details are being transferred to C-02/C-11 with actual artifact links when implemented; the session proposal is not a second canonical contract.
- **Consequences:** Continue independent approved instance/root and schema preparation work. Frozen shared C-09/C-11 value kinds, WASI wire/manifest compatibility and stored source semantics cannot be silently extended. Prepare the exact bytes/serialization/operator/resource/classification proposal and obtain specific affected-owner approval; assess downstream consumers without executing or changing their AP baselines.
- **Owner detail:** This record is the only current definition

<a id="d-103"></a>
## D-103 Shared Bytes Binding

- **Status:** accepted (SDK-version mapping superseded by D-131)
- **Date:** 2026-11-03
- **Decision:** Approved bounded shared bytes values and SDK 1.1 bytes compatibility; SDK-version mapping superseded by D-131. Approve the narrowly specified general bytes value or revise its behavior. The user approves a distinct canonical padded standard-Base64 typed bytes value, exact equality/bounded membership/presence with existing three-valued boolean composition, identity copying and explicitly declared pure installed processing. No implicit text/number conversion, regex, byte indexing/ordering/concatenation or new predicate operator. Preserve absent/null/hidden/empty distinctions, all inherited source classifications and `derived` for output visibility.
- **Owner detail:** This record is the only current definition

<a id="d-129"></a>
## D-129 In-process Core regex

- **Status:** accepted
- **Date:** 2026-11-05
- **Supersedes:** [D-62](04-processing-and-plugins.md#d-62)
- **Decision:** Core regex runs in-process with deterministic admission bounds; supersedes D-62 for Core regex. Core regex uses Go's standard `regexp` in process ([adapter](../../internal/processing/regex.go)). Before compilation, a parse-tree estimate bounds the program at 20,000 instructions and instructions x capture slots x 16 bytes at 32 MiB; the exact compiled program is checked against the same bounds. Matching reads through a cancellable reader inside the existing 1 s call budget; cancellation or expiry is a failure, never a no-match. Pattern (4096 B) and text (1 MiB) limits are unchanged. Executable proof (512 MiB container, no swap, D-61 counterexample at 1 KiB and 1 MiB, plus an admitted near-bound worst case): no OOM, peak process memory 28 MB; details in [AP-08 E-13](../execution/ap-08.md#e-13-in-process-core-regex-proof).
- **Consequences:** The `regexguest` command and `regex.wasm` are removed; `WASMHost` serves only installed artifacts (C-10). Processing consumers take a `RegexEvaluator`.
- **Owner detail:** This record is the only current definition

<a id="d-130"></a>
## D-130 Plugin trust clarification

- **Status:** accepted
- **Date:** 2026-11-05
- **Decision:** Plugins are fully trusted and may access host, devices and network directly; D-67 is only a first subset.
- **Consequences:** This confirms D-24 and corrects any reading of [D-67](04-processing-and-plugins.md#d-67)'s pure, capability-free WASI command as a general plugin restriction; D-67 remains only the first implemented Processing-step subset. The current WASI runtime cannot provide direct device or network access, so the plugin runtime (TECH-08) and the C-10 SDK need a planning revision.
- **Owner detail:** This record is the only current definition

<a id="d-131"></a>
## D-131 Managed plugin process runtime

- **Status:** accepted
- **Date:** 2026-11-05
- **Decision:** Plugins run as Operator-provisioned executables managed through `go-plugin`/gRPC, one process per configured instance; WASI/Wazero removed. - **Runtime.** A plugin is an Operator-provisioned executable below the configured artifact root (`--processing-artifact-root` or `GOPAGER_PROCESSING_ARTIFACT_ROOT`). The Core starts it as an application-managed child process through HashiCorp `go-plugin` v1.8.0 using its gRPC transport. - **Protocol.** One versioned service, `gopager.plugin.v2.Plugin/Invoke`, carries the existing JSON envelope as a `google.protobuf.BytesValue`. It is published as [`api/plugin/v2/plugin.proto`](../../api/plugin/v2/plugin.proto), so plugins can be written in any gRPC-capable language. A Go SDK ([`pluginsdk`](../../pluginsdk)) wraps handshake and serving. - **Instances.** Every configured instance runs in its own process. The envelope carries the instance ID, so plugins can keep per-instance connections or devices apart. - **Manifest and limits.** Manifest schema version 2 with SDK 2.0 (bytes included).
- **Consequences:** - Supersedes the runtime parts of D-67, D-73, D-76 and the SDK-version mapping of D-103, and resolves D-65. D-67's pure/pure-preview Processing-step semantics stay as trusted manifest declarations. - Reaction and Delivery capabilities (REQ-139, REQ-090) and Core reads through the versioned HTTP API under a technical Principal remain AP-11/AP-12 planning. A process pool across Workflow runs is also left to AP-12; processes currently live for one run. - C-10, C-11, shared conventions, architecture TECH-08 and AP-11/AP-12 were updated accordingly.
- **Owner detail:** This record is the only current definition
