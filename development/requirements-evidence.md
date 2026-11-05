# Source Provenance and Documentation Evidence

This file owns source observations, reuse assessment and non-AP documentation checks. Product decisions are routed by [D-ID](authoritative-plan.md#confirmed-decisions); actual AP validation/results belong to [execution records](execution/README.md). Do not copy decision narratives, task progress or contract rules here.

## Source Inventory

| Source / observation date | Provenance coverage | Limits |
| --- | --- | --- |
| User-provided consolidated planning file, 2026-10-23: product plan 2.2 / catalog 1.8 | Origin of stable REQ-001..132 and initial technology rationales | Historical external input, not a second maintained specification; no general legacy-code validation |
| User discussion, 2026-10-23 | D-01/D-02 and initial D-17..D-19 provenance | Later decisions amend the initial discussion; use current owner links |
| Documentation creation/maintenance requests, 2026-10-23..25 | Authorization for repository documentation | Not scope, spike or product implementation approval |
| Language/naming discussions, 2026-10-24..25 | D-20/D-21 provenance | Current terminology and rename sequence are in the [glossary](glossary.md#rename-log) |
| AP-01 clarification and later impact review, 2026-10-25 | D-17/D-18 and C-01..C-03 inputs | Intermediate policy candidates were superseded; current contract only |
| AP-02 clarification, 2026-10-25 | D-22..D-31 provenance | Supersession is indexed in the plan; the contracts own final behavior |
| Documentation execution-policy request, 2026-10-25 | Immutable-current-AP workflow and workspace agent entry | Rules are owned by [shared execution procedure](work-packages/shared-rules.md), not this observation |
| [47 CFR 11.31](https://www.ecfr.gov/current/title-47/chapter-I/subchapter-A/part-11/subpart-B/section-11.31), read in prior planning on 2026-10-23, then current through 2026-10-21 | EAS source reference for [C-02](contracts/c-02-protocol-payload.md) | Not re-fetched here, not a complete Protocol implementation study |

The external German source is historical provenance, not a parallel translated specification. Detailed obsolete policy/naming explorations remain in Git history; they are deliberately absent from current implementation context.

## Protocol Source Checks

The AP-01 historical record reports checks against Wikipedia (POCSAG/EAS) and multimon-ng sources, including correction of numeric glyphs for 0xE/0xF. Exact source revisions and a reproducible check command were not recorded. Do not treat this as complete protocol certification. FLEX type/capcode verification remains assigned by [P-08](authoritative-plan.md#open-issues-and-risks).

## Legacy Source Observation

The prior targeted inspection recorded PagerMon tag `v1.6.0-beta`, commit `c8a934d67100e86789f9d6ee844cb2d47aa44c7f`, `server/essen/index.js` and desktop/mobile card templates. Observed: conditional structured location/destination sections, raw-text fallback and custom parser logic.

This supersedes the initial not-found report. No code adoption, complete historical comparison or functional parity is established. This review preserves the prior source/ref observation; it did not independently rerun that inspection.

Potential later research sources are PagerMon, pagermon-ingest-core and pagermon-ingest-adapter-multimon in their separate repositories. They are not GoPager runtime dependencies or mandatory migration targets.

## Evidence Assessment for Deliberate Reuse

For each proposed adoption, add a record here identifying affected REQ/C version, repository/ref, path/symbol, observed behavior, executed tests and missing invariants. Classify direct fit, partial fit, idea/test-case only or unsuitable, with rationale. Adoption approval belongs in the plan. A source reference alone is neither a passing target test nor permission to reuse.

No adoption assessment is approved yet.

## Processing Kernel Library Sources

Incremental-capability recheck, 2026-11-01: pkgsite metadata/public symbols for [`cel.dev/cel-go/cel` v0.33.2](https://pkg.go.dev/cel.dev/cel-go@v0.33.2/cel) and [`ext`](https://pkg.go.dev/cel.dev/cel-go@v0.33.2/ext), the official [CEL overview](https://cel.dev/overview/cel-overview), and the [published module manifest](https://proxy.golang.org/cel.dev/cel-go/@v/v0.33.2.mod). Verified API availability and declared dependencies only; no adoption, compatibility proof or measured incremental resource cost. The former `google/cel-go` release README URL returned 404 and was not used as release evidence. Recommendation and evaluation scope belong to [architecture](architecture.md#cel-go-incremental-capability-review).

Reviewed 2026-10-28 for AP-04's authorized T043 preparation, not a package adoption or runtime test:

- Local Go 1.26.2 `go doc regexp.Regexp`, `go doc os/exec.CommandContext`; local `src/regexp/regexp.go` Reader implementation and `src/regexp/exec.go` matcher loop. The documented Reader API and observed end-of-input handling warrant testing context-aware Reader cancellation; ordinary string methods do not accept context. Process cancellation is a distinct mechanism, not permission to change Lite topology.
- [CEL-Go upstream entry](https://github.com/google/cel-go), including its canonical-repository/import migration notice and partial-state/unknown-value documentation. Reviewed current documentation, not a pinned executable release or compatibility result.
- [Expr language definition](https://expr-lang.org/docs/language-definition), including operators, optional/nil values, predicates and regex. Language availability is not a demonstrated authorization/classification adapter.
- [AP-03 E-07](execution/ap-03.md#e-07-wasm-runtime-probes) retains the exact historical Wazero tuple, synthetic guest/host observations and limitations; no earlier experiment was rerun or reinterpreted as application evidence.

The [architecture assessment](architecture.md#t043-evaluator-library-and-budget-assessment) owns suitability/rationale. D-61 subsequently authorized the scoped follow-up; actual commands, hashes, successes/failures and limits belong to [AP-04 E-38](execution/ap-04.md#e-38-budget-and-memory-probe-results). Only the disposable scratch module acquired the approved Wazero dependency; no application dependency or implementation artifact was introduced.

**2026-11-01 discovery addendum:** The now-available Go package-discovery tool returned canonical `cel.dev/cel-go/cel` and `common/types` at v0.33.2, and `github.com/expr-lang/expr` plus its `ast`/`vm` packages at v1.17.8. Queries: `cel.dev/cel-go` and `github.com/expr-lang/expr`, three results each. This confirms current package discovery, not adoption, pinned-version execution or a dependency/latency measurement. [The proposed restricted binding assessment](architecture.md#t043-predicate-and-coordination-library-binding) owns the D-36 suitability/exception rationale.

### Managed Child Resource Sources

Reviewed 2026-11-01 after D-63, without running a child experiment:

- Local Go 1.26.2 `go doc os/exec.CommandContext`, `go doc os/exec.Cmd.WaitDelay`: context cancellation kills the configured child; pipe/wait bounds are separate. Neither API is an OS memory limiter or automatic proof of descendant/parent-death cleanup.
- Local `go doc runtime/debug.SetMemoryLimit`: Go's memory target is explicitly soft and excludes some non-Go/runtime external memory. It is not a safe replacement for hard enforcement after the D-61 OOM counterexample.
- Local `go doc syscall.Setrlimit`; [Linux getrlimit/setrlimit manual](https://man7.org/linux/man-pages/man2/getrlimit.2.html): unprivileged processes can lower hard/soft limits; address-space, CPU and data-segment limits have different semantics. Linux `RLIMIT_DATA` also affects `mmap` since 4.7; neither a generic Go API name nor a chosen byte value proves compatibility with Go's startup/address reservations.
- [Linux cgroup v2 documentation](https://docs.kernel.org/admin-guide/cgroup-v2.html): process membership/resource controllers are hierarchical. The outer disposable Docker limit is not proof that a distribution can create a separate per-child resource boundary without delegated OS permissions or extra Operator setup.

Suitability/proposed follow-up belongs to [the architecture assessment](architecture.md#t043-evaluator-library-and-budget-assessment) and D-64. No Linux/macOS/Windows parity or automatic deployment privilege is inferred from these sources.

### Native Plugin Sources

Reviewed 2026-11-01 under the user's `review-native-alternative` instruction:

- Local Go 1.26.2 `go doc plugin`, `go doc plugin.Open`, `go doc plugin.Plugin.Lookup`: supported host limits, exact toolchain/shared-source compatibility warnings, initialization at load, no close/unload and race-detector limitations. These are official API warnings, not a target plugin conformance test.
- Go package-discovery module metadata reports `github.com/ebitengine/purego v0.11.2` (commit time 2026-10-15) and `github.com/hashicorp/go-plugin v1.8.0` (commit time 2026-04-29) as current/latest on the lookup. No package was installed or imported into the application.
- [PureGo upstream README, main](https://github.com/ebitengine/purego/blob/main/README.md): native C-function/dynamic-symbol support, per-platform/architecture constraints and beta warning. This source review is not a verified pinned-release ABI/runtime matrix.
- [HashiCorp go-plugin upstream README, main](https://github.com/hashicorp/go-plugin/blob/main/README.md): application-launched subprocesses with local net/rpc or gRPC, protocol/handshake/lifecycle facilities and controllable stdout/stderr forwarding. Framework crash separation is not a demonstrated resource cap, field-authority engine or secret-redaction guarantee.

The initial guessed HashiCorp documentation URL returned HTTP 404; the canonical upstream repository/README and module metadata were used instead. [Architecture](architecture.md#native-plugin-alternative-review) owns the comparison/rationale. No native code, SDK, artifact load, runtime probe or production adoption follows from this review. The later selection of `go-plugin` v1.8.0 under D-131 is owned by [TECH-08](architecture.md#technology-decisions).

## Documentation Reviews

Historical AP documentary checks/outcomes are in [AP-01 E-01](execution/ap-01.md#e-01-imported-documentary-outcome) and [AP-02 E-01](execution/ap-02.md#e-01-imported-documentation-checks), not duplicated here.

### E-DOC-01 Entry-point Cleanup

Date: 2026-10-25, before the immutable-AP restructuring. A local Python standard-library check resolved 773 relative Markdown links/heading anchors across 43 Markdown files and checked all 16 AP session routes, REQ sections and contract gates. Compared with Git `3be71b4`, 132 catalog/mapping rows, 54 task IDs/check states, D/P rows and 61 F-cases were preserved. `git diff --check` passed.

The temporary checker and full tool/runtime environment were not persisted. These are recorded structural/textual observations, not a renderer/Mermaid check, fresh-agent session trial or runtime evidence.

### E-DOC-02 Authority and Immutable-AP Review

Compared the documentation worktree with Git `3be71b4` after the ownership/execution restructuring. Local tools: Python 3.14.0 standard library and Git 2.54.0 (Apple Git-157) on macOS. Procedure: resolve relative Markdown targets, heading slugs and explicit anchors; compare stable IDs, task states, requirement mapping, delivery classifications, vocabulary and planned cases against the baseline.

- 992 local links/anchors across 47 Markdown files resolved; all 16 AP session routes include their relevant REQ sections, owner contracts and shared start/approval context.
- All 132 REQ IDs, user-goal/provenance cells and mapping rows, 54 task IDs/check states, D/P/TECH IDs, 132 delivery classifications and 61 planned F-cases were preserved. Six acceptance rows intentionally replace repeated technical detail with owner-contract links.
- Glossary term labels, prohibited terms and rename history were preserved; interface identifier spellings were checked at their receiving contracts.
- Root/direct-document responsibilities and execution instructions were reviewed locally. Workspace agent entry is link-only; historical AP execution imports explicitly qualify missing original environments/commands and do not claim new runtime evidence or approval.
- `git diff --check` passed.

The temporary checker was not persisted. These checks establish structural consistency and reviewed ownership, not automatic prevention of future duplication, a fresh-agent session trial, rendered diagrams or implemented product behavior. No runtime tests were executed for this documentation-only change.

### E-DOC-03 Process/Permanent Documentation Separation

Date: 2026-11-06, under [D-132](authoritative-plan.md#documentation-and-naming-separation). The process tree moved from `docs/` to `development/`; `docs/` now holds permanent documentation. In the README, architecture, glossary, requirement/evidence, contract-directory, package-directory, shared-rule and requirement-mapping files, references to the process tree, numbered domain IDs, the Wazero-based plugin design and the package placement table were updated against the code; the C-ID, AP and execution files were maintained separately. A local Python standard-library checker resolved every relative link and heading/explicit anchor in these nine files with zero failures; one stale glossary anchor into C-10 was corrected. This is a structural check, not a rendered-diagram, fresh-agent or runtime check.
