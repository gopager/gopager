# Source AP-12 Historical Execution Record

## Baseline

Source: unchanged [original AP-12 at its baseline](https://github.com/gopager/gopager/blob/24318de414b3b35f77b6b288905125c1debf46f4/docs/work-packages/ap-12-processing-presentation.md), starting HEAD `24318de414b3b35f77b6b288905125c1debf46f4`. Extensive earlier uncommitted changes exist in command composition, contracts/architecture/plan, identity/authorization, matching/Units, Ent/persistence/events/protocols and their tests; no clean worktree or committed-only prerequisite baseline is claimed. Preserve them. Only the previously approved AP-15 topology amendment exists in the AP baseline diff.

Assignee: current GoPager executing agent, sequentially, for T043 and its required standalone T045 tests. Approved scope/input/write/verification/non-goal binding: [AP-04](ap-04.md#processing-kernel-execution-binding); explicit owner freeze and separate actual start: [D-66](../authoritative-plan.md#t043-shared-predicate-binding-proposal). Five owner subsets are frozen at 1.0. Prerequisite evidence: [AP-05](ap-05.md), [AP-07 global-field correction](ap-07.md#unscoped-field-correction-verification-and-handover), [AP-08 matcher](ap-08.md); regex/CEL feasibility evidence remains AP-04 history, not kernel production validation.

## Task Outcomes

| Task | State | Artifact/evidence |
| --- | --- | --- |
| T043 | in-progress | Runtime/fact/configuration persistence through E-17; coordinated management/context through E-20; dedicated preview E-22; lifecycle primitives through E-27; D-80 context classification E-28; administrative instance persistence/inspection E-30; private worker/dispatch primitives E-31; approved exhausted reselection E-33; actual periodic renewal/joined cancellation E-34; fenced publication participant E-35; current installed execution/lifetime adapter E-36; configurable dispatcher/native recovery and partial root registration E-38; PostgreSQL Watermill publication foundation E-46. D-86 root verification, protected HTTP, trusted declaration/preview composition and actual AP-06 source ownership remain incomplete. |
| T044 | not-started | Outside this prerequisite binding. |
| T045 | in-progress | Only T043's necessary standalone checks are included; source-bound AP-06/AP-09/presentation integration remains later work. |

## Evidence

### E-01 Approved Start and Source Inspection

Date: 2026-11-01. Read the AP baseline, document authority/execution rules, root AGENTS, frozen owner inputs and producer evidence; inspected actual identity Ent/privacy/borrowed transaction/event append/Unit context adapters. `git rev-parse HEAD` and `git status --short` exited 0 from `/Users/leokr/Projects/gopager`, yielding the baseline and extensive local delta described above. Actual-start approval was recorded before kernel changes. No kernel runtime test or production artifact is claimed from this inspection.

### E-02 Codec Validation and Execution-authority Port Finding

Date: 2026-11-01. Implemented [typed values/closed declarations/bounded decoder](../../internal/processing/values.go) and [focused codec tests](../../internal/processing/values_test.go). `gofmt -w internal/processing && go test ./internal/processing` exited 0 (0.564s). Actual cases cover int64/string distinctions and overflow, finite numbers/no numeric-string coercion, RFC3339 offsets, nullable null versus absence, unknown/duplicate envelope keys, required/closed object/list declarations, UTF-8 rejection, trailing JSON, exact 1 MiB/+1 byte and depth 32/33. This is only codec evidence, not a completed kernel or PostgreSQL/module/HTTP proof.

Inspected `identity.Resolver` public/private methods and `AuthorityRepository`: only `Resolve` accepts transport proofs, while current direct/group/scope/global-field assembly is private `accountAuthority`. No existing current-by-Principal adapter exists. The frozen C-10 execution-Principal participant therefore needs the exact additive producer extension proposed in [D-71](../authoritative-plan.md#internal-execution-principal-authority-adapter), not a second authority engine, invented proof or key issuance. Earlier identity APIs are not changed before explicit consultation. Only this real invocation/root participant is blocked pending input; remaining T043 work stays approved.

### E-03 Current-principal Adapter and Dependency Integration Start

Date: 2026-11-01. D-71's approved [internal authority adapter](../../internal/identity/authority.go), [unit tests](../../internal/identity/execution_authority_test.go), [real-primary lock/suspension test](../../internal/persistence/ent/borrowed/execution_authority_test.go) and stored group-removal checks were implemented without a new HTTP/authentication route. From the target repository:

```text
GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/processing ./internal/identity ./internal/persistence/ent/borrowed ./cmd/gopager
go vet ./internal/processing ./internal/identity ./internal/persistence/ent/borrowed ./cmd/gopager
```

Both exited 0; test durations 1.396s, 2.797s, 44.114s, 15.354s. The new PostgreSQL test proves actual authority-lock blocking, current suspension rejection and live-primary-transaction requirement; existing HTTP/proxy authentication regressions still pass. Group removal affects internal execution authority and normal proof resolution consistently. No installed runtime or kernel publication is certified by this adapter check.

Started selected dependency integration in the existing [manifest](../../go.mod). The CVE skill's generic `appmod-cve-assessment` accepts Maven/NuGet/npm only, so it cannot assess Go; no false ecosystem mapping was attempted. Supplemented the best-effort fallback with current official pkgsite Go vulnerability queries: CEL-Go v0.33.2 and Wazero v1.12.0 each returned an empty applicable-vulnerability list. These are direct-module results, not yet a complete resolved-transitive/code-reachability or independent security audit. No vulnerability-driven unrelated upgrade is authorized; actual imports/tuple build and transitive self-check remain in progress.

### E-04 Product Predicate, Classification and Supplied Core Guest

Date: 2026-11-01. Added the product [restricted CEL compiler/evaluator](../../internal/processing/predicates.go), [classification union](../../internal/processing/dependencies.go), [isolated WASI host](../../internal/processing/wasm.go), [supplied Go/WASI regex command](../../internal/processing/regexguest/main.go) and their [predicate](../../internal/processing/predicates_test.go)/[guest](../../internal/processing/wasm_test.go) tests. The embedded generated artifact is `internal/processing/regex.wasm`; the host's `go:generate` directive reproduces it. No native host regex compiler/matcher is used.

From the target repository:

```text
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -trimpath -buildvcs=false -o internal/processing/regex.wasm ./internal/processing/regexguest
go mod tidy
gofmt -w internal/processing
go test ./internal/processing
go test -race -count=1 ./internal/processing
go vet ./internal/processing
git diff --check
go generate ./internal/processing
go mod verify
shasum -a 256 internal/processing/regex.wasm
```

Initial build/tidy/ordinary tests exited 0 (0.618s). The first expanded Race run failed because a long large-capture fixture reached the real one-second guest deadline before output overflow; it did not produce accepted partial output. Replaced that fixture with a shorter control-character capture whose JSON escaping actually exceeds the output boundary without extending the production budget. A subsequent formatting attempt exposed a misplaced test declaration; corrected it before proceeding. The resulting complete Race run exited 0 (17.022s); targeted vet and diff check exited 0. These checks precede the additional dedicated CEL low-limit classification test; its final result is recorded separately below, not inferred here.

Actual cases include exact int64 extrema, numeric/timestamp and recursive object/list equality, literal binding rather than expression interpolation, unchanged Unicode comparison, explicit null, ordered-null failure, readable absence versus hidden values, three-valued NOT/AND/OR, all operand dependencies despite short-circuiting, dependency deduplication/group union without pointer aliasing, mixed-target rejection, 1024 arguments/+1, concurrent prepared-program reuse, cancellation and an exactly measured 1 MiB classified input/+1. Guest checks cover inspection, named/unmatched captures, safe no-match, unsupported RE2 syntax, 4096-byte pattern/+1, real escaped-output overflow, sticky writer overflow, cancellation, closed-host rejection and survival after rejected calls. This is not a full installed-artifact, accumulated Workflow budget, durable-run, HTTP or publication proof.

The subsequent `go generate`, module verification and artifact hashing all exited 0; embedded artifact SHA-256 `92931e146dffdba84af0a946bc4bf68f14ff38ef8fcdcad2ba3cb85901757d45`. Do not delete this required product artifact as scratch output. Module verification proves checksum integrity, not absence of vulnerabilities.

Extended official pkgsite vulnerability checks to the resolved CEL runtime dependencies: `cel.dev/expr v0.25.1`, `github.com/antlr4-go/antlr/v4 v4.13.1`, `google.golang.org/genproto/googleapis/api v0.0.0-20261005182115-fad411399dd8` and `google.golang.org/protobuf v1.36.12` returned empty applicable lists. Exact pseudo-version queries for `golang.org/x/exp` and `google.golang.org/genproto/googleapis/rpc` returned HTTP 404, not clean results. Those lookup gaps require a verified authoritative Go-vulnerability alternative; no complete transitive/reachability security claim is made.

### E-05 Multi-target Context Binding Finding

Date: 2026-11-01. Inspected the frozen [C-02 projection](../contracts/c-02-protocol-payload.md#early-processing-source-projection-candidate), [C-09 predicates](../contracts/c-09-delivery.md#early-shared-typed-predicate-candidate), [C-11 Workflow schema/results](../contracts/c-11-processing-presentation.md#early-workflow-kernel-candidate) and F-37/F-38. They specify ordered multi-target input, one selected Workflow run and later `target` references, but no resolution of those later references when several target contexts match. Existing predicate correlation is not a Workflow winner policy. No normative choice is inferred from input order or boolean short-circuiting. Resolution/approval lives at [D-72](../authoritative-plan.md#multi-target-workflow-context-binding), with a separate C-11 review rather than an unapproved edit of the frozen subset. Block only affected post-selection target binding.

### E-06 Final Primitive Checks and Verified Vulnerability Fallback

Date: 2026-11-01. After adding the dedicated low-cost/low-tracked-memory CEL tests, ran:

```text
gofmt -w internal/processing
go test -race -count=1 ./internal/processing
go vet ./internal/processing
go test -run '^$' ./cmd/gopager
git diff --check
git status --short --untracked-files=all -- internal/processing
```

All exited 0; Processing Race 16.857s, root compile-only check 0.437s (`[no tests to run]`). The real CEL limit-1 programs now demonstrate typed cost/memory cancellation mapped to safe `RESOURCE_LIMIT`, with no partial result/private input in the public error. These test limits exercise error classification, not the production limit thresholds or a host-RAM guarantee. The root check proves dependency-tuple compilation only, not a new HTTP/PostgreSQL or root Processing integration. Required sources and generated `regex.wasm` are present as untracked product files; none were committed or discarded.

After the E-04 HTTP 404s, verified the canonical [Go vulnerability database API](https://go.dev/doc/security/vuln/database) before another request. The documented `/index/modules.json` endpoint has no version query and lists all database modules/reports. Ran `go list -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}' -deps ./internal/processing | sort -u` to establish the actual standalone runtime closure. With shell pipe failure enabled, fetched the complete official module index using `curl --fail --silent --show-error https://vuln.go.dev/index/modules.json` and filtered only the eight CEL/Wazero/expr/antlr/exp/genproto/protobuf public module paths with `jq`; command exited 0. The index contains no reports for either formerly failing pseudo-version module path. Of the eight checked paths, only protobuf has reports: GO-2023-1631 (latest fix 1.29.1) and GO-2024-2611 (latest fix 1.33.0), both older than resolved 1.36.12 and consistent with its empty version-specific pkgsite result.

The runtime closure also contains existing `github.com/google/uuid v1.6.0`, `go.yaml.in/yaml/v3 v3.0.5` and `golang.org/x/sys v0.49.0`; their version-specific official pkgsite checks each returned an empty applicable list. Thus E-04's two lookup gaps have a verified module-index fallback, not a guessed version or suppressed 404. This is a current public-database module/version self-check for the standalone Processing dependency closure, not whole-application/standard-library scanning, code-reachability analysis, a future-vulnerability guarantee or an independent security review. No unrelated dependency was upgraded for remediation.

### E-07 Exact Membership Boundary and Owner Artifact Links

Date: 2026-11-01. Re-read C-09's frozen distinct bounds: filter arguments/membership have 1024 entries; typed-value collections have 1000. Corrected the newly implemented membership check to the former, without changing the owner contract or an earlier producer. Added an exact 1024/+1 membership test and linked the implemented wire primitives from C-09 as the authoritative artifact forms. An initially misplaced test declaration failed formatting; inspected/corrected its actual file placement before rerunning.

```text
gofmt -w internal/processing
go test -race -count=1 ./internal/processing -run 'TestPredicate|TestDependency'
go vet ./internal/processing
git diff --check
```

All exited 0; focused Race run 2.421s. E-06's guest tests remain the unchanged guest/host evidence; this last change only affects predicates and their owner links. AP baseline inspection still shows only the previously approved AP-15 amendment. D-72 remains proposed, with no post-selection winner or result-shape policy adopted.

### E-08 User-selected Target-result Preparation

Date: 2026-11-01. The user answered D-72 with `prepare-target-bound-result-sets`, authorizing C's concrete preparation, not an unseen result-schema freeze. Prepared the separate [C-11 candidate](../contracts/c-11-processing-presentation.md#target-result-binding-candidate) and glossary reference; no target-dependent evaluator/result schema was implemented against the proposal. Inspected actual matcher `Resolution` (one current Unit reference per original position) and AP-06 T023/T024, AP-09 T031/T033, AP-10 T034/T035, AP-13 T046/T047. These tasks have no scalar-only result requirement, so no downstream baseline amendment is currently necessary. The candidate identifies the coupled C-02/C-09/C-10/C-08 consumer implications and keeps one durable run/publication transition.

The proposed static target-dependence rule, result alternatives, targetless behavior, conservative selection dependencies, all-or-nothing publication and unchanged aggregate budgets are new concrete details requiring explicit owner approval. Do not infer them from the preparation answer or silently treat the recommendation as accepted.

### E-09 Target-result Kernel Implementation and Bounds

Date: 2026-11-01. Following the user's exact `approve-target-results-1.1-and-implement` approval, implemented the detached trusted-owner [source/context port](../../internal/processing/capture.go), [immutable definition/compiler/evaluator/result primitives](../../internal/processing/workflow.go), [target/source/branch/extraction tests](../../internal/processing/workflow_test.go) and [exact result/step/aggregate/conversion tests](../../internal/processing/workflow_bounds_test.go). C-02/C-11 link these actual typed artifacts. The owner source port explicitly fails when AP-06's stored-source owner is missing; arbitrary caller-provided Protocol/form/declarations are not a validated public synthetic-input path.

The approved D-72 primitive behavior now preserves original target positions, performs separate selected-target bodies, retains a conservative selection-dependency union, produces either shared fields or target-result entries, treats hidden/failed selected contexts as failure without partial success and never fabricates an original target. Source-only runs execute once. A reference in an unused branch still marks the definition target-dependent, as approved. Detached source/schema/literal/result data do not alias callers or future evaluations.

Implemented explicit mapping/conversion, guest-only RE2 extraction and lexical branch outputs with globally unique local IDs/no forward or cross-branch references. All source/branch/selector dependencies propagate conservatively, including into constant final fields. A whole-run built-in deadline and aggregate input/intermediate/result limits span target evaluations; they are not reset per step/context. Empty successful field containers and empty explicit map configurations survive JSON preparation. No installed step is silently emulated: the incomplete installed participant explicitly rejects preparation as authority unavailable.

Actual commands from target repository:

```text
gofmt -w internal/processing
go test -race -count=1 ./internal/processing -run 'TestWorkflow|TestTarget|TestSource'
go test -race -count=1 ./internal/processing
go vet ./internal/processing
git diff --check
stat -f '%z bytes' internal/processing/regex.wasm
go doc os.OpenRoot
go doc github.com/tetratelabs/wazero.Runtime.CompileModule
```

Initial target/branch/source Race run exited 0 (15.888s). First expanded full run failed at the 128-step fixture because `omitempty` dropped an explicitly empty map configuration during detached preparation; this was a real codec bug, not an accepted invalid fixture. Preserved explicit emptiness in `Step.MarshalJSON`, then the full Processing Race run exited 0 (32.384s), followed by successful vet/diff checks. Cases cover targetless absence, context-specific values, selector dependencies from nonmatching contexts, hidden second-context failure, later conversion failure without first-context success, definition/source/output mutation isolation, concurrent extract/branch reuse, duplicate/forward step rejection, exact complete output 256 KiB/+1, total steps 128/+1, multi-target aggregate intermediate exhaustion and explicit conversion boundaries/no trimming/locale coercion.

Metadata/API lookup exited 0: supplied Core guest is 3,879,470 bytes. `os.OpenRoot` provides an existing standard-library directory-root API; Wazero exposes precompilation separately from invocation. C-10's current bounds govern call input/output/guest memory, not the byte size of a provisioned Wasm artifact. The new installed loader therefore needs an explicit bounded artifact policy before allocating/compiling an arbitrary local binary; no existing call-input limit is silently repurposed as an artifact-size cap.

This is not a completed T043 handover: installed artifacts/current invocation authority, configuration/revision/version persistence, protected HTTP/preview/status APIs, durable leases/fencing and actual publication integration remain required. Whole AP-12/T044 and AP-06 intake remain unimplemented/unverified by these primitive tests.

### E-10 Approved Bounded Artifact Loader

Date: 2026-11-01. Following D-73 approval, implemented [rooted artifact loading and manifest/import inspection](../../internal/processing/artifacts.go) and [loader tests](../../internal/processing/artifacts_test.go). From the target repository, `go test -race -count=1 ./internal/processing -run 'TestInstalled'`, `go vet ./internal/processing` and `git diff --check` exited 0; the final loader-only Race check took 15.477s. This selector preceded the newer invocation tests and must not be interpreted as their evidence. An initial fixture mutation panicked because an empty `omitempty` map was nil after detachment; corrected the fixture to mutate a declared property and Secret pointer.

Actual checks include exact 32 MiB/+1, regular-file/root/symlink boundaries, digest mismatch, detached metadata, explicit recursive configuration classification and rejection of unsupported capabilities/import signatures. A supplied minimal trapping start-section fixture proves precompilation does not execute initialization; actual instantiation traps. Local operator-managed storage is the trust boundary, not a guarantee against a malicious operator concurrently replacing an already checked file with a device/FIFO. No production instance repository or lifecycle is certified.

### E-11 Installed Command Wire

Date: 2026-11-01. Built a separate original [Go/WASI test command](../../internal/processing/testdata/command/main.go), embedded its generated fixture, and implemented the package-private [typed command wire](../../internal/processing/installed.go) with [actual invocation tests](../../internal/processing/installed_test.go).

```text
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -trimpath -buildvcs=false -o internal/processing/testdata/command.wasm ./internal/processing/testdata/command
gofmt -w internal/processing
go test -race -count=1 ./internal/processing -run 'TestInstalled'
go vet ./internal/processing
```

All exited 0; Race took 42.063s. Actual command output, mixed success/failure, unknown/private failure text, declared failure, caller cancellation, later host reuse, unavailable pure-preview capability and unresolved Secret configuration were tested. This internal wire does not accept an HTTP envelope as Principal/configuration authority. No installed administrative service or general SDK/language parity is claimed.

### E-12 Separate Typed DOM-08 Fact Factories

Date: 2026-11-01. Implemented [Processing configuration/completion factories](../../internal/events/processing.go), [factory tests](../../internal/events/processing_test.go) and the separate [caller-transaction append participant](../../internal/persistence/ent/eventstore/processing.go). They were initially implemented under the Processing package and later moved to the approved event-producer location in E-14; the historical command paths below reflect the actual original check, not current locations. Existing identity/master-data vocabularies and methods remain unchanged. `gofmt -w internal/processing/facts.go internal/processing/facts_test.go internal/persistence/ent/eventstore/processing.go`, `go test -race -count=1 ./internal/processing -run 'TestProcessing'`, `go vet ./internal/processing ./internal/persistence/ent/eventstore` and `git diff --check` exited 0; Race took 1.705s.

Factories require a real publication participant's Message/result references, immutable output declaration and whole successful result; hidden/missing/undeclared/type-mismatched fields, substituted source revisions and duplicate/unordered targets fail. Completion aggregates use the Processing-result ID/revision, not unrelated Workflow revision ordering. Typed source classification and historical replay mode survive serialization; configuration data contains only safe change kind. This first check did not exercise PostgreSQL or establish source readiness.

### E-13 Installed Workflow and Accumulated Compute, Full Regression and PostgreSQL

Date: 2026-11-01. Added [trusted installed preparation/current invocation ports and Workflow execution](../../internal/processing/installed_workflow.go), [accumulated compute accounting](../../internal/processing/budget.go), [budget](../../internal/processing/budget_test.go)/[actual installed Workflow tests](../../internal/processing/installed_workflow_test.go), and [PostgreSQL fact participation tests](../../internal/persistence/ent/borrowed/processing_facts_test.go).

The first expanded test failed because Go guest `time.Sleep` did not produce the claimed real elapsed wait under Wazero's default fake clock. Verified this using local official `go doc` for `ModuleConfig.WithNanotime`, `WithSysNanotime` and `WithSysNanosleep`; did not silently grant a new real host-clock capability. Replaced that invalid timing fixture with an actual guest loop that survives the one-second built-in allowance and is terminated by the real 1.2-second caller deadline. Separately tested the accounting method across an actually elapsed 1.1-second excluded wait, resumed finite allowance and accumulated 600/450-ms host consumption. A misplaced test declaration was caught by formatting and corrected before the final run. Neither failed check is reported as passing.

Final commands from the target repository:

```text
go generate ./internal/processing
gofmt -w internal/processing internal/persistence/ent/borrowed/processing_facts_test.go
go test -race -count=1 ./internal/processing
GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed -run 'TestProcessingFact'
go vet ./internal/processing ./internal/persistence/ent/eventstore ./internal/persistence/ent/borrowed
go test ./cmd/gopager -run '^$'
git diff --check
```

All final checks exited 0. Full Processing Race: 128.953s; real PostgreSQL Race: 8.865s; root compile-only: 0.532s. Generation rebuilt both supplied guests before the final suite. The installed Workflow uses the actual separately supplied command, conservatively retains input/selector classification, reauthorizes every selected target through an explicit fixture owner, rejects projection substitution and second-context revocation without partial success, and survives earlier deadline/rejection. Only runtime instantiation/execution is excluded from accumulated compute; preparation, authority/input/output checks, cleanup and later built-ins remain bounded. No new allowance is allocated per step/target.

PostgreSQL uses the existing isolated real-primary fixture/all migrations. Invalid completion and duplicate facts roll back both earlier appended facts and a synthetic caller-state marker; expired transaction capabilities fail; persisted producer/result/status/replay metadata matches the typed factories. This demonstrates the append participant, not Workflow configuration atomicity or real AP-06 publication. Actual instance/current source authority, administrative/version/run repositories, leases/fencing, protected HTTP and production source-publication integration still require implementation.

### E-14 Fact Owner Location and Retry-bound Finding

Date: 2026-11-01. Aligned the new DOM-08 fact factories/tests with the approved [execution binding](ap-04.md#processing-kernel-execution-binding): moved them into `internal/events/processing.go`/`processing_test.go`, reused shared event Producer/EntityRef types and exposed the existing classified-field validator rather than duplicate its type/state checks. No existing identity/master-data factory or vocabulary changed. A misplaced wrapper declaration was caught by formatting and corrected before validation.

```text
gofmt -w internal/events/processing.go internal/events/processing_test.go internal/processing/capture.go internal/persistence/ent/eventstore/processing.go internal/persistence/ent/borrowed/processing_facts_test.go
go test -race -count=1 ./internal/events
go test -race -count=1 ./internal/processing -run 'TestSource'
GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed -run 'TestProcessingFact'
go vet ./internal/events ./internal/processing ./internal/persistence/ent/eventstore
go test ./cmd/gopager -run '^$'
```

All exited 0: event tests 1.542s, source tests 1.418s, real-primary participation 6.590s and root compile-only 0.507s. E-13 remains the preceding full Processing regression; it is not relabeled as a full-suite run after relocation.

For required durable retry implementation, inspected C-08's frozen Processing participation and retained internal retry rules. Five automatic attempts and four nominal delays are specified, but the internal jitter's exact range is not. C-09 specifies +/-20% only for Delivery's separate policy. A repository-wide bounded search found no other internal jitter binding. Do not silently inherit the whole Delivery policy or invent a new internal retry default. [D-74](../authoritative-plan.md#processing-internal-retry-jitter-bound) proposes the missing narrow bound; only this numeric retry-policy implementation requires the user's resolution.

### E-15 Approved Internal Retry Bound

Date: 2026-11-01. The user explicitly selected `approve-symmetric-20-percent-and-implement`; [D-74](../authoritative-plan.md#processing-internal-retry-jitter-bound) owns that resolution. Implemented [the internal retry scheduler](../../internal/processing/retry.go) and [its exact bound/exhaustion tests](../../internal/processing/retry_test.go), reusing standard-library `math/rand/v2` rather than an extra dependency. Primary-time microsecond precision and UTC due times are retained.

```text
gofmt -w internal/processing/retry.go internal/processing/retry_test.go
go test -race -count=1 ./internal/processing -run 'TestProcessingRetry'
go vet ./internal/processing
git diff --check
```

All exited 0; Race 1.685s. Tests measure both closed interval endpoints for every nominal delay, actual random samples inside each bound, explicit terminal exhaustion at five completed attempts, and rejection of invalid attempts/time/sample sources. This is due-time policy only, not durable claim/retry/fencing implementation or an external Delivery/Gateway feature.

### E-16 Durable Selection Placement Review

Date: 2026-11-01. Inspected C-08's frozen Processing participation and C-11's frozen Workflow/budget/publication semantics before choosing durable work shape. C-08 calls for selected runs and caller-transaction work creation; C-11 prohibits evaluation/module execution with a primary transaction open and gives each evaluation one accumulated selector/body budget. No concrete admission/selector job form is bound yet. Putting CEL selection in the acceptance transaction, creating only nondurable post-commit work, or giving split selector/body stages independent fresh compute allowances would silently weaken those requirements.

[D-75](../authoritative-plan.md#processing-durable-selection-placement) proposes a single scoped candidate-work lifecycle using the actual existing evaluator: selected runs execute their bodies in the same evaluation; false selectors finish candidate work without a Processing result/completion fact. The alternative splits durable selection and body work with additional persisted budget/revision proof. No candidate schema/state or separate selection queue is implemented before the concrete owner amendment is approved.

### E-17 Approved Candidate Placement and Configuration Persistence

Date: 2026-11-01. The user explicitly selected `approve-candidate-work-and-implement`; [D-75](../authoritative-plan.md#processing-durable-selection-placement) owns the approved resolution. E-16 remains the historical pre-approval finding. Added [Processing-owned Ent schemas](../../internal/persistence/ent/schema/processing.go), [central migration](../../internal/persistence/migrations/00006_processing_kernel.sql), [store-neutral configuration forms](../../internal/processing/configuration.go), [native configuration/version repository](../../internal/persistence/ent/processingstore/configuration.go) and [real-primary tests](../../internal/persistence/ent/borrowed/processing_configuration_test.go). Reused existing Ent generation, borrowed transactions, shared scope predicates, configuration audit and command/fact participants; no additional dependency or SQL evaluator.

Prior implementation checks:

```text
gofmt -w internal/persistence/ent/schema/processing.go
go generate ./internal/persistence/ent
go test ./internal/persistence/ent/... ./cmd/gopager -run '^$'
go test ./internal/processing ./internal/events ./internal/persistence/ent/processingstore -run '^$'
gofmt -w internal/processing/configuration.go internal/persistence/ent/processingstore/configuration.go internal/persistence/ent/borrowed/processing_configuration_test.go
GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed -run 'TestProcessingConfiguration|TestProcessingSchema'
go vet ./internal/processing ./internal/persistence/ent/processingstore ./internal/persistence/ent/borrowed
```

All exited 0; prior real-primary Race 7.596s. Subsequently extended the activation test to actually commit all participants, attempt a stale expected-revision overwrite, and reread committed state/receipt/counters:

```text
gofmt -w internal/persistence/ent/borrowed/processing_configuration_test.go
GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed -run 'TestProcessingConfiguration|TestProcessingSchema'
git diff --check
```

All exited 0; real-primary Race 8.425s. Native filtering precedes count/paging and covers every Workflow reference through grant union intersected with key restrictions. Hidden Workflow/version lookup is indistinguishable from absence. Saving a version does not activate it. Tentative activation state, receipt, own/shared revisions, audit and fact roll back together; committed activation persists each exactly once, and a stale write changes none. Deferred active-version ownership rejects a different Workflow's version with known SQLSTATE `23503`, not commit uncertainty. Unguarded queries fail.

Candidate/result tables are schema groundwork only: no claim/renew/retry/publication lifecycle is certified. Configuration mutations still need the coordinating current-authority/reference-validating service; tests use explicit server-side authority fixtures, not a production proof adapter. No whole-suite HTTP regression, production source readiness or completed T043 handover is inferred.

### E-18 Coordinated Management and Current Preparation Binding

Date: 2026-11-02. Added [the coordinating management service](../../internal/processing/management/service.go) using existing proof resolution, shared lock order, primary time, reference owners, actor-bound receipts, audit and separate Processing facts. [Actual-primary tests](../../internal/persistence/ent/borrowed/processing_management_test.go) use the existing bootstrapped/login fixture, not caller-supplied authority. Definition preparation happens after a preauthorization transaction has expired; the commit phase resolves proof/current scope again and checks both expected Workflow revision and unchanged shared configuration revision. Preparation cannot substitute the validated definition.

Removed the newly introduced, unrequested 256-character Workflow-name restriction from the in-progress metadata schema/validator/migration: the frozen owner specifies bounded requests, not this additional per-name default. Metadata commands use the existing strict 1 MiB decoder. The real-primary test stores a 300-character name and requires an explicitly supplied enabled boolean. No previous AP schema or deployed-data migration is rewritten.

```text
gofmt -w internal/processing/configuration.go internal/processing/workflow.go internal/processing/management/service.go internal/persistence/ent/schema/processing.go internal/persistence/ent/processingstore/configuration.go
go generate ./internal/persistence/ent
go test ./internal/processing/... ./internal/persistence/ent/processingstore -run '^$'
gofmt -w internal/processing/management/service.go internal/persistence/ent/borrowed/processing_management_test.go
GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed -run 'TestProcessingManagement|TestProcessingConfiguration|TestProcessingSchema'
go vet ./internal/processing/... ./internal/persistence/ent/processingstore ./internal/persistence/ent/borrowed
```

Generation/compile checks exited 0. The first management test run failed because its assertion expected absent-object `NotFound` when the principal had no read/update action at all; the existing central permission planner correctly returned `Forbidden`, preserved through the transaction wrapper. Corrected the assertion rather than weakening authorization. Inspected the actual event schema and changed the test's nonexistent aggregate column to the canonical envelope reference. The final targeted Race/Vet run exited 0 (9.611s).

Actual cases cover actor-bound creation/version replay without duplicate state, save versus activation, activation/select-previous with activate-only grants and no public definition-read bypass, stale expected revisions, disable without changing the selected version, logical deletion with retained history, every Network use of a shared Unit, explicit missing-reference/transport failure, authority revocation during compilation, and actual invalid final fact append rolling back earlier metadata/audit/revision writes. Additional shared-configuration-change/substitution/missing-owner cases are included in E-20's final regression, not retroactively claimed by the earlier targeted run. Installed-instance management, worker lifecycle/HTTP composition remain incomplete.

### E-19 Explicit Invocation Context and Scoped Evaluation

Date: 2026-11-02. Inspection showed that identical mapped inputs, including a union of selector dependencies from multiple targets, do not identify the actual body target. Added [immutable source-reference metadata](../../internal/processing/capture.go), explicit host-only [invocation context](../../internal/processing/installed_workflow.go) carried through nested execution frames, and [owner-scoped evaluation](../../internal/processing/workflow.go) while preserving the complete original capture and original target positions. This implements existing C-10/D-72 scope/correlation requirements; no guest wire, field grant, selector language, or input-controlled source schema is added.

```text
gofmt -w internal/processing/capture.go internal/processing/workflow.go internal/processing/workflow_test.go internal/processing/installed_workflow.go internal/processing/installed_workflow_test.go
go test -race -count=1 ./internal/processing -run 'TestWorkflowScoped|TestWorkflowChecked|TestInstalledWorkflow'
go vet ./internal/processing/... ./internal/persistence/ent/processingstore ./internal/persistence/ent/borrowed
git diff --check
```

An initial formatting check caught a test inserted inside a helper; moved it to the correct declaration boundary before running tests. The final commands exited 0; Race 83.558s. The actual separately supplied guest receives byte-identical mapped inputs for two contexts, but its authorization participant receives distinct original target positions and correct source/revision references. Participant pointer mutation does not change original/result positions. Scoped evaluation skips an excluded hidden context without treating it as a false predicate or renumbering the eligible target. Duplicate/out-of-order/out-of-range eligibility positions fail explicitly. Ineligible source evaluation produces no namespace. Checked-definition copies cannot mutate the executable.

These are explicit fixture-owner checks, not a production current-source/instance adapter or publication handover. C-11's required real owner must supply scope eligibility; permissive inference from request fields is not introduced.

### E-20 Full Affected Regression After Management and Context Changes

Date: 2026-11-02. From the target repository:

```text
gofmt -w internal/processing/management/service.go internal/persistence/ent/borrowed/processing_management_test.go
GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed ./cmd/gopager
go test -race -count=1 ./internal/processing/...
go vet ./internal/processing/... ./internal/persistence/ent/processingstore ./internal/persistence/ent/borrowed ./cmd/gopager
git diff --check
git --no-pager diff -- docs/work-packages
```

All exited 0. Full real-primary borrowed regression: 52.701s; root HTTP/composition Race: 15.437s; full Processing Race: 152.382s. The latter includes E-19's actual installed command and accumulated-budget tests. Management tests additionally reject shared configuration changes during preparation, substituted compiled definitions and missing declaration owners, with no persisted version. All central migrations execute in the existing isolated real-primary fixture. Baseline review shows only the previously approved AP-15/D-63 amendment; AP-06/AP-12 definitions remain unchanged.

Passing existing HTTP/proxy regressions does not certify missing Processing HTTP endpoints or AP-06 intake. No production source/publication owner, candidate/result lifecycle, current installed-instance adapter or completed T043/AP-06 is claimed.

### E-21 Dedicated Preview Binding Gap

Date: 2026-11-02. Inspected C-10's frozen installed-processing subset, [manifest/pin types](../../internal/processing/artifacts.go), [invocation gate](../../internal/processing/installed.go) and [Workflow preparation/invocation ports](../../internal/processing/installed_workflow.go). A `PurePreview` boolean can reject undeclared preview but does not identify an independently supplied preview artifact or dedicated entrypoint. The current Workflow port prepares one artifact. Actual pure-command tests therefore do not implement/certify the separately declared pure-preview implementation required by C-10.

[D-76](../authoritative-plan.md#dedicated-installed-preview-artifact-binding) proposes the missing narrow artifact/configuration binding, rather than weakening the requirement or expanding the command ABI silently. No new preview artifact/entrypoint/configuration shape is implemented before this owner resolution. Only this extension is blocked; durable candidate/result/current-owner/HTTP/publication work remains outstanding. Existing single-artifact fixture results are preserved as partial evidence.

### E-22 Approved Dedicated Preview Invocation

Date: 2026-11-02. The user selected `approve-separate-preview-artifact-and-implement`; [D-76](../authoritative-plan.md#dedicated-installed-preview-artifact-binding) owns approval. Added [the optional independent preview preparation port/schema-policy checks](../../internal/processing/installed_workflow.go), compiled preview binding and explicit runtime routing, and [a separate supplied pure preview command](../../internal/processing/testdata/preview/main.go). Its generated `testdata/preview.wasm` is embedded by the installed Workflow tests and reproduced by the existing Processing generation command. The guest wire and supported command ABI remain unchanged.

```text
gofmt -w internal/processing/installed_workflow.go internal/processing/installed_workflow_test.go internal/processing/workflow.go internal/processing/testdata/preview/main.go
go generate ./internal/processing
go test -race -count=1 ./internal/processing -run 'TestInstalledWorkflowUsesDedicatedPreview'
gofmt -w internal/processing/installed_workflow_test.go
go test -race -count=1 ./internal/processing -run 'TestInstalledWorkflow'
go vet ./internal/processing/... ./internal/persistence/ent/borrowed ./cmd/gopager
go test ./cmd/gopager ./internal/persistence/ent/borrowed -run '^$'
git diff --check
```

All exited 0; dedicated preview Race 40.064s, complete installed Workflow selector 119.124s; root/borrowed compile-only 0.372s/0.381s. Actual guest output distinguishes dedicated preview (`PREVIEW:LEVEL=7`) from ordinary processing (`LEVEL=7`). Missing dedicated preview fails rather than invoking the main artifact. An alias of the same prepared main artifact, missing pure-preview declaration, incompatible input schema and changed configuration classification are rejected. Existing per-target/context/revocation/substitution and installed deadline checks remain in that final selector.

Normal preparation explicitly distinguishes an absent optional preview from an invalid configured one. Real administrative instance persistence/current-source authority and source publication are still outstanding; the port and fixture are not a fabricated production configuration owner. E-20 remains the prior whole-package PostgreSQL/HTTP/Processing regression, not a full-suite rerun after this preview change.

### E-23 Reselection Identity and Terminal Work Review

Date: 2026-11-02. Inspected C-08's approved candidate identity and supersession; C-11's scope/current-configuration/no-historical-sweep semantics; the actual [candidate schema](../../internal/persistence/ent/schema/processing.go) and [migration constraints](../../internal/persistence/migrations/00006_processing_kernel.sql). The current unique key excludes Workflow metadata/configuration revision, while one candidate row also owns lifecycle/attempts/fence.

An unpublished original may have candidate work superseded when its Workflow is disabled/out of scope. Reenabling/restoring eligibility can retain the same immutable definition, source and target-context revisions. Inserting replacement work with the same key would collide. The sources do not explicitly define permanent terminality of the logical candidate or same-key reselection into a newer candidate revision; claiming the former as already frozen would be incorrect. Attempts/fences and immutable prior evidence must not be erased. This is a schema/contract inspection, not a reproduced worker failure: no production worker is implemented yet.

[D-77](../authoritative-plan.md#processing-reselection-identity-and-terminal-work) requests the concrete revisioned same-key reselection binding, separate execution-generation representation or explicit identity extension. No reselection transition, new generation schema, key widening, source revision fabrication, retry-policy reset or historical sweep is implemented before that resolution. Independent completed configuration/context/preview evidence remains E-17..E-22.

### E-24 Approved Revisioned Candidate Admission and Primary-clock Claims

Date: 2026-11-02. The user selected `approve-revisioned-same-candidate`; [D-77](../authoritative-plan.md#processing-reselection-identity-and-terminal-work) and [C-08](../contracts/c-08-events-jobs.md#processing-same-input-reselection-review) own the resolution. Implemented [store-neutral candidate/binding/lease transitions](../../internal/processing/candidate.go) and [borrowed primary admission/reselection/claim/renewal](../../internal/persistence/ent/processingstore/candidate.go), reusing Ent, existing transaction/guard/control-lock/primary-clock helpers and `commands.NextRevision`. No schema, migration, deduplication-key or AP-baseline change.

Admission checks current Processing configuration and active enabled owning Workflow under shared control locks. Same pins are idempotent, including failed/superseded states. Changed current pins advance candidate revision and fence, retaining the ID, cumulative attempts and private result rows. Lease claims reserve actual attempts; renewals keep the ownership fence/attempts but return a newer revision. Exact due/expiry boundaries and counter exhaustion are explicit errors, not resets. Source eligibility/current source/Principal authority and exclusion of published originals remain real caller-owner obligations; these repository primitives are not an unrestricted public API or a working end-to-end worker.

Final commands, from `/Users/leokr/Projects/gopager`:

```sh
go test -race ./internal/processing -run '^TestCandidate' -count=1
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessingCandidateNativeReselection' -count=1
go vet ./internal/processing ./internal/persistence/ent/processingstore
git diff --check
```

All exited 0: model Race 1.578s; final actual PostgreSQL 18 fixture Race 4.353s. [Model tests](../../internal/processing/candidate_test.go) cover stable identity, monotonic pins/revision/fence, immutable capture, no unchanged-state revival, published/substituted/stale rejection, int64 exhaustion, microsecond primary time, exact 60-second lease/20-second renewal interval, expiry equality, five-versus-six automatic attempts and retained automatic history for a preauthorized manual attempt. [Native tests](../../internal/persistence/ent/borrowed/processing_candidate_test.go) cover disabled/stale configuration rejection, rollback before committed reselection, concurrent deduplication, stale CAS/fence write rejection, retained private result identity/fence, real claim/renewal and a valid synthetic expired lease evaluated against the real primary clock without waiting a minute.

Initial formatting used the unrelated session CWD and failed before tests; reran from the target repository. Formatting then caught a misplaced top-level test, and the first lease-extended PG test failed because the verification block was accidentally inside the disable helper. Corrected those exact scopes before the final checks above. These failed intermediate attempts are not successful validation. No source publication, completion/failure/recovery worker, manual-retry authorization or HTTP/root composition acceptance follows from E-24.

### E-25 Scoped Manual Retry and Terminal-rule Inspection Correction

Date: 2026-11-02. The user selected `approve-existing-rule-reprocess-for-failed-retry`; [D-78](../authoritative-plan.md#processing-failed-work-retry-action-binding) owns approval and C-11 owns the action binding. Added [expected-state manual retry](../../internal/processing/candidate.go), [native action-filtered work read/retry and safe audit](../../internal/persistence/ent/processingstore/candidate.go), and [current-proof/source-owner coordination](../../internal/processing/management/retry.go). No new action/catalog/AP-07 change or automatic retry-cycle reset.

Final commands from the target repository:

```sh
go test -race ./internal/processing -run '^TestCandidate' -count=1
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessing(CandidateNativeReselection|RetryRechecksProof)' -count=1
go vet ./internal/processing ./internal/processing/management ./internal/persistence/ent/processingstore
git diff --check
```

All exited 0; model Race 1.823s, real-primary Race 9.529s. Tightened the native rollback assertion to require that the pending retry/audit participants were actually reached before the exact sentinel abort; reran the same combined primary selector and `git diff --check`, both exit 0, primary Race 6.291s. Tests verify failed/expected state, no implicit CRUD/read retry, native audit/dedup/unchanged configuration revision, retained five-attempt automatic history and one manual claim, fresh real login proof after grant changes, required mutation check, explicit missing/unavailable source-owner failure and recovery through an expressly synthetic owner. [Native retry coordination tests](../../internal/persistence/ent/borrowed/processing_retry_test.go) do not certify production source/execution-Principal ownership, HTTP or a complete worker.

Subsequent full reading of C-08's final Retry/recovery paragraph found the explicit sentence “Superseded work is terminal.” E-23's review omitted this sentence; it must not be used as evidence that terminal work was unspecified. D-77 A interprets terminality at the old execution revision rather than forever at the logical candidate. [D-79](../authoritative-plan.md#superseded-work-terminality-interpretation-confirmation) requests explicit confirmation of that existing-rule interpretation before further affected integration; previously tested code remains uncomposed, and no completed T043/AP-06 acceptance is claimed.

### E-26 Shared Evaluation Validation, Private Completion and Safe Failures

Date: 2026-11-02. The user explicitly confirmed D-79's existing terminal-rule interpretation by `approve-terminal-old-revision-with-new-logical-candidate-revision`; the E-25 consultation pause is resolved without hiding its inspection correction.

Extracted existing completion field/schema/dependency/whole-JSON validation into [the shared Processing validator](../../internal/processing/evaluation.go), reused by [the unchanged publication-fact interface](../../internal/events/processing.go). Private validation does not require or fabricate a Message ID. Workers supply the actual exclusive original-target bound; the publication fact retains the shared ceiling and its actual source-owner publication check.

Added [private-result port](../../internal/processing/result.go), [native atomic private completion](../../internal/persistence/ent/processingstore/result.go) and candidate completion/safe-failure transitions. Completion validates the actually saved definition schema and current configuration/lease, persists selected evidence with result revision/producing fence, or records not-selected without a result namespace. It never sets publication readiness or appends `processing.completed`. The [native transition writer](../../internal/persistence/ent/processingstore/candidate.go) additionally checks PostgreSQL `clock_timestamp() < lease_until` in running-state write predicates, so a deadline expiring after an earlier check cannot authorize the eventual update. Failure scheduling uses the approved remaining-attempt/jitter policy; exhausted/permanent errors stay explicitly failed.

Final commands from `/Users/leokr/Projects/gopager`:

```sh
go test -race ./internal/processing ./internal/events -run '^(TestCandidate|TestEvaluation|TestProcessing)' -count=1
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessing(CandidateNativeReselection|RetryRechecksProof)' -count=1
go vet ./internal/processing ./internal/events ./internal/processing/management ./internal/persistence/ent/processingstore
git diff --check
```

All exited 0; Processing Race 2.017s, Events Race 1.779s, real-primary Race 10.057s. [Shared-validator tests](../../internal/processing/evaluation_test.go) measure exactly 256 KiB versus +1 byte, reject not-selected namespaces, repeated/out-of-original-range results/dependencies and validate ordered original positions. Existing fact tests still pass. Expanded native tests verify private completion with no barrier/Message/fact, stale completion rejection, definite-false completion with no result row, and primary-clock safe failure scheduling/stale failure rejection. The first completion formatting run caught a misplaced method; corrected its exact top-level location before the successful checks.

These are actual lifecycle primitives, not a complete worker/recovery loop, current installed/source adapters, protected HTTP composition or source-publication handover. Root-source readiness and complete T043/AP-06 acceptance remain outstanding.

### E-27 Expired Pure-compute Claim Recovery

Date: 2026-11-02. Added [expired-claim recovery](../../internal/processing/candidate.go) and its [native current-configuration transition](../../internal/persistence/ent/processingstore/candidate.go), reusing the existing safe-failure scheduler. Recovery requires matching stored ownership and actual lease expiry, advances revision/fence, retains consumed attempts and schedules only remaining automatic attempts; the fifth expired automatic attempt stays failed. Completed private evidence is not reevaluated or reclaimed. The SQL write additionally requires `clock_timestamp() >= lease_until`; ordinary renewal/failure/completion retain the opposite unexpired-lease guard.

From `/Users/leokr/Projects/gopager`, Go 1.26.2 darwin/arm64:

```sh
gofmt -w internal/processing/candidate.go internal/processing/candidate_test.go internal/persistence/ent/processingstore/candidate.go internal/persistence/ent/borrowed/processing_candidate_test.go
go test -race ./internal/processing -run '^TestCandidate' -count=1
gofmt -w internal/persistence/ent/processingstore/result.go
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessing(CandidateNativeReselection|RetryRechecksProof)' -count=1
go vet ./internal/processing ./internal/processing/management ./internal/persistence/ent/processingstore
git diff --check
```

Final checks exited 0: model Race 1.840s, real-primary Race 10.681s. The first primary build found the completion participant's remaining call to the extended private transition signature; updated that call without weakening its lease guard, then reran the failed primary/Vet/diff checks successfully. Model cases measure exact expiry, retry-delay bounds, monotonic successor fences, retained attempts, stale completion and counter exhaustion. The real-primary fixture races two recoverers: exactly one commits; the old worker cannot record failure over the recovered state. Existing manual-retry/audit/private-completion checks remain in that same actual test. Source IDs remain synthetic, not production readiness or a completed dispatcher/publication handover.

### E-28 Approved Source-context Classification

Date: 2026-11-02. Applied both independently selected [D-80 memberships](../authoritative-plan.md#processing-source-context-field-group-membership) at the [C-06 owner](../contracts/c-06-identity-authorization.md#fixed-field-group-review-candidate), preserving the still-unfrozen remainder of D-68. Added [the trusted projection context builder](../../internal/processing/capture.go) and [actual evaluator tests](../../internal/processing/source_context_test.go). The builder does not resolve a Network, infer current rights, replace a hidden value or supply the missing real original owner.

```sh
gofmt -w internal/processing/capture.go internal/processing/source_context_test.go
go test -race ./internal/processing -run '^(TestCandidate|TestSourceContext)' -count=1
go vet ./internal/processing ./internal/processing/management ./internal/persistence/ent/processingstore
git diff --check
```

All exited 0; Race 1.661s. Evaluator outputs retain the approved context classifications and source/context identity, including selector dependency union. Tests prove classified absent subtype, explicit hidden-context rejection, duplicate context rejection without overwriting and invalid Unicode rejection without replacement. `MissingSourceOwner` still fails explicitly. These are typed projection/evaluator fixtures, not a production AP-06 source adapter. `git --no-pager diff --stat -- docs/work-packages` exited 0 and showed only the existing approved AP-15 topology amendment; AP-06/AP-12 baselines remain unchanged.

### E-29 Installed Result Requirement Interpretation

Date: 2026-11-02. Read the frozen C-10 narrow invocation/manifest sections, C-11 classification requirement and actual [manifest](../../internal/processing/artifacts.go)/[Workflow invocation](../../internal/processing/installed_workflow.go)/fixture forms. Result schemas express types/requiredness; no distinct manifest result field-group surface exists. [D-81](../authoritative-plan.md#installed-result-requirement-interpretation) requests interpretation rather than silently treating this as either an implementation defect or permission to extend the frozen manifest.

Only the dependent current-instance/result-authorization step awaits this answer. No new result field groups, manifest format or permission semantics have been implemented. Existing fixture evidence remains unchanged. After the E-27/E-28 document updates, `git diff --check` and `go test ./internal/processing/... ./internal/persistence/ent/processingstore -run '^$'` exited 0; the latter is only a compile check, not a test-suite run.

### E-30 Administrative Instance Persistence and Inspection

Date: 2026-11-02. User selected `schema-only-preserve-inherited-visibility`; D-81 is approved and the C-11 interpretation applied. E-29's affected preparation pause is resolved; no manifest field-group extension was introduced.

Added [immutable instance types](../../internal/processing/instance.go), [native snapshots/current pointer](../../internal/persistence/ent/processingstore/instance.go), [Ent owner schema](../../internal/persistence/ent/schema/processing.go) and additive migration 00007. Earlier migrations are unchanged. Added [actual rooted module/configuration inspection](../../internal/processing/instance_artifacts.go) and [two-phase fresh-proof management](../../internal/processing/management/instance.go), actor-bound command receipts, safe configuration audit/shared-revision invalidation and separate nonsecret instance configuration facts through the existing event store. Inspection runs after the first SQL transaction closes; commit checks current proof, configuration revision and execution Principal again. Inspection receives a detached copy, not mutable authority over the persisted configuration.

Commands from the target repository, Go 1.26.2 darwin/arm64:

```sh
gofmt -w internal/processing/instance.go internal/persistence/ent/schema/processing.go internal/persistence/ent/processingstore/instance.go
go generate ./internal/persistence/ent
go test ./internal/processing/... ./internal/persistence/ent/processingstore -run '^$'
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessing(Instance|CandidateNativeReselection|RetryRechecksProof)' -count=1
gofmt -w internal/processing/instance_artifacts.go internal/processing/instance_test.go internal/processing/management/instance.go internal/events/processing.go internal/persistence/ent/eventstore/processing.go
go test -race ./internal/processing ./internal/events -run '^(TestInstance|TestCandidate|TestSourceContext|TestProcessing)' -count=1
gofmt -w internal/persistence/ent/borrowed/processing_instance_management_test.go
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessing(Instance|CandidateNativeReselection|RetryRechecksProof)' -count=1
go vet ./internal/processing/... ./internal/events ./internal/persistence/ent/processingstore ./internal/persistence/ent/eventstore
git diff --check
```

All exited 0. First native instance/lifecycle run 12.033s; actual module-inspection/model Race 15.293s, Events 1.475s; final fresh-proof/native/lifecycle Race 9.179s. Actual trapping module initialization is never executed by inspection. Optional absence, independent bit-identical preview and invalid configured preview are distinguished; unsupported Secrets and supplied personal properties fail explicitly. Native tests apply migration 00007, retain old snapshots, reject stale writes/old-revision starts, preserve disable, and roll back pointer/history/audit together. Management fixtures prove closed SQL during inspection, command replay without duplicate inspection/state, no inspector substitution, proof revocation during inspection, missing mutation/execution-Principal rejection and final invalid-fact rollback with no surviving receipt. The management inspector is explicitly synthetic; actual rooted inspection is tested separately, not misrepresented as a full composition test.

Current installed invocation/source authorization, artifact preparation lifetime/draining, protected HTTP/root and complete worker/publication remain required. These checks do not certify new disabled-version cancellation or a completed T043/AP-06 handover.

### E-31 Private Worker Coordination and Supersession

Date: 2026-11-02. Added [the worker coordinator](../../internal/processing/worker/service.go), current-source capture/check ports and [real-primary worker fixtures](../../internal/persistence/ent/borrowed/processing_worker_test.go). Capture and claim are atomic; compilation/evaluation run after SQL closes; renewal uses the updated Candidate revision; final private result/failure requires current source checks. Cancellation cancels and joins computation rather than leaving a helper running. Failure retryability derives from actual platform errors, not public input. Source-owner checks remain explicitly synthetic in this fixture.

Added [native bounded primary-clock work selection and supersession](../../internal/persistence/ent/processingstore/work.go) and [model transitions/tests](../../internal/processing/candidate_test.go). Supersession advances revision/fence, retains attempts/private evidence, excludes already-applied publication and does not require the obsolete Workflow configuration still to be active.

Final commands from the target repository:

```sh
gofmt -w internal/processing/worker/service.go internal/processing/candidate.go internal/persistence/ent/processingstore/work.go internal/processing/candidate_test.go internal/persistence/ent/borrowed/processing_worker_test.go
go test -race ./internal/processing -run '^TestCandidate' -count=1
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessing(Worker|Instance|CandidateNativeReselection|RetryRechecksProof)' -count=1
go vet ./internal/processing/... ./internal/events ./internal/persistence/ent/processingstore ./internal/persistence/ent/eventstore
git diff --check
```

Final checks exited 0; model Race 1.553s, combined actual-primary Race 23.890s. Earlier first worker test run failed because the installed-only compiler requires an artifact participant even for a builtin-only definition. The coordinator now explicitly chooses the builtin compiler when no artifacts are supplied; actual installed steps still fail without their required owner. The next combined primary run passed (23.762s), followed by the final supersession/work-selection run above. Tests prove selected private evidence, false without a namespace, durable safe compilation failure, no stale-source result, closed capture transaction, native due paging excluding terminal/unexpired work, fenced supersession and rejection of old renewal.

After the final E-30 fact change-kind distinction, repeated actual module/model/Events Race exited 0 (15.437s/1.541s) and instance/lifecycle primary Race exited 0 (8.741s), with Vet/diff successful. E-30's earlier observations are preserved.

These are actual one-item worker and dispatch/transition primitives, not a fully registered runner. Periodic 20-second renewal during a long evaluation, cancellation/heartbeat integration, installed invocation ownership/artifact draining and fenced source publication still need focused runtime verification/composition. The exhausted-reselection state finding below is not hidden by these passing tests.

### E-32 Exhausted Reselection State Finding

Date: 2026-11-02. Tracing the real due-work path against existing model/native tests found that same-key reselection preserves five consumed automatic attempts but unconditionally changes state to pending. `Claim` correctly refuses a sixth automatic attempt; protected `RetryFailed` correctly requires failed state. Consequently this exhausted reselection is neither runnable nor manually retryable. Existing E-24 tests explicitly expect that pending state; they are historical evidence, not proof this integrated state is recoverable.

[D-82](../authoritative-plan.md#exhausted-candidate-reselection-state) requests the narrow owning lifecycle correction before changing this previously recorded behavior. The five-attempt ceiling, no reset, same-key identity, retained results and current manual-retry action remain settled. No counter/status behavior has been changed for this finding yet.

### E-33 Approved Exhausted-reselection Correction

Date: 2026-11-02. User selected `approve-immediate-failed-with-preserved-attempts`; D-82/C-08 own the narrow state addendum. Changed the [model](../../internal/processing/candidate.go), [native admission transition](../../internal/persistence/ent/processingstore/candidate.go) and their existing/new tests. E-24's original pending-state observations remain history; the new current behavior is not a counter reset or a new retry action.

```sh
gofmt -w internal/processing/candidate.go internal/processing/candidate_test.go internal/persistence/ent/processingstore/candidate.go internal/persistence/ent/borrowed/processing_candidate_test.go
go test -race ./internal/processing -run '^TestCandidate' -count=1
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessing(Worker|Instance|CandidateNativeReselection|RetryRechecksProof)' -count=1
go vet ./internal/processing/... ./internal/events ./internal/persistence/ent/processingstore ./internal/persistence/ent/eventstore
git diff --check
```

Final checks exited 0; model Race 1.549s, combined actual-primary Race 23.251s. The first primary run caught a clear-and-set mutation of the same nullable failure column. Read generated Ent mutation semantics, replaced that with mutually exclusive set/clear branches, and reran the failed primary/Vet/diff checks successfully. Actual checks distinguish four/five consumed automatic attempts, exclude exhausted reselection from automatic due dispatch, and prove that the existing scoped manual retry reaches an eighth total claim while keeping five automatic attempts and increasing the fence. No attempt reset or sixth automatic claim.

### E-34 Actual Periodic Renewal and Joined Cancellation

Date: 2026-11-02. Extended [the real-primary worker fixture](../../internal/persistence/ent/borrowed/processing_worker_test.go) with an expressly synthetic blocking artifact-preparation owner. It waits for the real 20-second heartbeat check before releasing preparation; this is actual wall time, not the WASI fake clock or a shortened test-only renewal setting. Completion of the resulting classified failure uses the renewed Candidate revision, retains the same ownership fence and consumes only the original attempt.

```sh
gofmt -w internal/persistence/ent/borrowed/processing_worker_test.go
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessingWorker' -count=1
go vet ./internal/processing/... ./internal/persistence/ent/processingstore
git diff --check
```

All exited 0; actual-primary Race 38.558s. The cancellation case cancels the real worker context after artifact preparation starts and requires preparation's completion signal before `Run` returns; it reports cancellation, not result success. This verifies heartbeat during long preparation and joined cancellation with real store fencing. It is not a real long-running installed guest, registered runner/shutdown or production-source proof. E-31's prior missing-heartbeat observation remains historical.

### E-35 Fenced Whole-source Publication Participant

Date: 2026-11-02. Added [source-publication binding/participant ports](../../internal/processing/publication.go), [borrowed-primary publication repository](../../internal/persistence/ent/processingstore/publication.go), [transaction coordinator and explicit missing-owner failure](../../internal/processing/publication/service.go), and [actual-primary publication tests](../../internal/persistence/ent/borrowed/processing_publication_test.go). No new product default, lease or permission is introduced. This is the C-08/C-11 kernel participant; actual Original/Message/target/contribution ownership remains AP-06's required implementation.

The test SourceOwner is expressly synthetic, with a real SQL fixture barrier/fence rather than a production Original or fabricated production-ready Message. It supplies the complete current set, locks/checks actual fixture source/context/Network/Message/fence pins, and CASes readiness in the same live transaction. Tests prove selected completion/fact/readiness atomicity; all-false and no-candidate barriers still require the source owner's actual CAS and produce no completion namespace/fact. Incomplete work, changed source/context/Network, changed configuration, supersession, revoked fixture authority, duplicate Workflow bindings and stale source fences abort. Injected failures after an actual fact insert or after the readiness update roll back every participant and preserve private result/candidate revision/fence. A concurrent pair has exactly one winning publication; retries do not duplicate results/facts. A pending separate reception for the same fixture Message leaves the already published reception/evidence intact. A detached owner check cannot inject fields into stored results or facts.

```sh
gofmt -w internal/processing/publication.go internal/processing/publication/service.go internal/persistence/ent/processingstore/publication.go internal/persistence/ent/borrowed/processing_publication_test.go
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessingPublication' -count=1
go vet ./internal/processing/... ./internal/events ./internal/persistence/ent/processingstore ./internal/persistence/ent/eventstore
git diff --check
git diff --stat -- docs/work-packages
```

Final checks exited 0; actual-primary Race 6.606s. Initial compilation found an existing loop-scope error-variable redeclaration in the new coordinator; corrected before actual-primary validation. AP baseline diff remains only the previously approved AP-15 topology amendment. These checks do not certify a production source owner, real target-mask/Principal authority, GUI visibility, registered dispatch or protected HTTP.

### E-36 Current Installed Execution and Per-run Artifact Lifetime

Date: 2026-11-02. Added [per-run rooted execution/preparation and lifecycle ownership](../../internal/processing/instance_execution.go), the [current editor-proof adapter](../../internal/processing/execution/preview.go), required post-commit lifecycle notification in [instance management](../../internal/processing/management/instance.go), and [actual-primary/actual supplied guest checks](../../internal/persistence/ent/borrowed/processing_execution_test.go). No new source-read action, configuration group, result permission, general installer or Secret/personal configuration fallback is introduced.

Each call resolves the configured Principal through the existing current-authority adapter, rechecks the exact current enabled instance, validates classified input/global groups and requires its SourceOwner's current scoped source/configuration checks. Preview separately resolves the requesting editor's current proof and requires the source owner's preview/sample authorization. The source participant receives no editor proof. Calls use detached input/current configuration and the selected main or independently pinned preview digest after SQL closes. Pure nonsecret administrative configuration uses the existing policy; unsupported Secret/personal configuration stays unavailable.

Rooted module preparation occurs outside SQL with a fresh current-revision check afterward. Partial/failed preparation remains a failure on reuse, retains compilations for explicit cleanup and never substitutes the main artifact for a missing/malformed preview. Per-run context must be used for compilation/evaluation; committed later revisions cancel affected older runs, but a delayed older notification does not cancel a newer run. Stop prevents new runs and cancels existing contexts; the composing owner must join work, close each run, then close the shared host/root. Instance management notifies only for an actual successful write/commit, not a rolled-back fact/receipt or a receipt replay.

```sh
gofmt -w internal/processing/instance_execution.go internal/processing/execution/preview.go internal/processing/management/instance.go internal/persistence/ent/borrowed/processing_instance_management_test.go internal/persistence/ent/borrowed/processing_execution_test.go
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessing(Execution|InstanceManagement|Publication)' -count=1
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed -run '^TestProcessingExecutionCurrentAuthorityAndArtifactLifetime$/(main|preview)$' -count=1
go vet ./internal/processing/... ./internal/persistence/ent/processingstore ./internal/persistence/ent/borrowed
git diff --check
```

Final checks exited 0: combined actual-primary Race 237.153s; subsequent focused main/preview Race 88.512s. The latter adds actual global-field revocation after a successful call and a separate configured execution account without editor preview rights, while the requesting editor alone supplies `rule.preview`. Other cases cover actual guest main/preview output, absent/invalid preview and failed-preparation reuse, hidden input groups, changed current revision without a notification, revoked execution grants, revoked editor preview rights, closed compilation rejection and stop. All source scope/Original identities remain explicit fixtures, not a production source implementation. These tests do not certify active-guest cancellation caused by a real HTTP configuration change, registered runner/root shutdown, complete activation preparation, saved/synthetic preview HTTP or AP-06 publication.

The first build exposed the identity → events → Processing dependency cycle; replaced the attempted concrete identity dependency with narrow current-Principal/transaction/editor ports and the separate proof adapter. No identity API or earlier AP output was changed.

### E-37 Dispatcher Operational Bounds Review

Date: 2026-11-02. Inspected [platform Runner/start lifecycle](../../internal/platform/status.go), [current platform configuration](../../internal/platform/config.go), [private worker](../../internal/processing/worker/service.go), [native due/expired selection](../../internal/persistence/ent/processingstore/work.go) and C-08's claim/retry/Processing sections. They establish role/cancellation, candidate leases and bounded page selection, but contain no initial simultaneous-candidate count or Processing fallback-poll interval. The platform status-check interval is not a Processing scheduling policy.

[D-83](../authoritative-plan.md#processing-dispatcher-concurrency-bound) and [D-84](../authoritative-plan.md#processing-dispatcher-idle-wake-up-bound) separately propose these operational choices for explicit owner input. No guessed runtime default, new configuration field or dispatcher implementation follows from the proposals. Only the affected actual dispatcher/root scheduling step awaits these choices; the approved predicate, publication and invocation work is not reopened.

### E-38 Configurable Dispatcher, Native Recovery and Root Registration

Date: 2026-11-02. User selected D-83/D-84's proposed initial values and then explicitly required preparation for configurable policy, recorded under [D-85](../authoritative-plan.md#processing-scheduling-configuration-preparation). Added [validated injected scheduling and bounded dispatch](../../internal/processing/worker/dispatcher.go), [scheduler tests](../../internal/processing/worker/dispatcher_test.go), [failure-fairness checks](../../internal/processing/worker/fairness_test.go), current-owner reconciliation/recovery in [the worker](../../internal/processing/worker/service.go), and [root Worker composition](../../cmd/gopager/processing.go). [Startup](../../cmd/gopager/main.go) now registers the runner for the Worker role and retains the compute host until shutdown. Root source ownership remains explicitly unavailable until AP-06 supplies its producer; no synthetic production Original is introduced.

The scheduler reuses Go contexts/channels/WaitGroup/tickers and existing native page/claim/lease transitions; a general worker framework would duplicate the already approved private durable lifecycle. Injected values are validated, defaulted only when omitted, and separate from any future Operator flags/environment/UI. Selection pages remain bounded; configured concurrency can cross the repository page boundary without becoming an implicit 200-item limit. A rotating page cursor prevents repeatedly rejected early work from starving later candidates; in-flight identities are not launched twice locally. Work errors log only candidate ID and safe code, retain a bounded cooldown until wake/poll, and do not become a tight retry loop. Selection infrastructure errors stop the runner and surface through platform role readiness. Cancellation joins dispatched work.

```sh
gofmt -w internal/processing/worker internal/persistence/ent/borrowed/processing_worker_test.go cmd/gopager/main.go cmd/gopager/processing.go cmd/gopager/process_test.go
go test -race ./internal/processing/worker -count=1
GOPAGER_INTEGRATION=1 go test -race ./internal/persistence/ent/borrowed ./cmd/gopager -run '^(TestProcessingWorkerPrivateEvaluationAndCurrentSourceRecheck|TestFullCommandProcessStartupAndSignalShutdown)$/(dispatch|dispatch-recovery|api,worker|worker)$' -count=1
go vet ./internal/processing/... ./cmd/gopager
git diff --check
```

The recorded run exited 0: dispatcher Race 1.396s; actual-primary dispatch/recovery Race 19.060s; trial real command API+Worker/Worker-only process startup and signal shutdown Race 35.222s. The trial process-test change described below is withdrawn pending explicit approval, so the passing trial is not verification of the currently restored historical startup assertion. Native checks exercise actual private selected completion and expired-compute recovery with retained attempts, advanced revision/fence and a future primary retry, not immediate recomputation. Scheduler tests use standard `testing/synctest` virtual time, including exact fallback threshold, immediate/coalesced wake, custom interval/concurrency, page traversal, bounded simultaneous work, failure fairness and joined shutdown. They are not elapsed-wall-time performance evidence.

Initial fixture failures included a nil transaction context, unsynchronized fixture observations found by Race, and a misplaced added test declaration; these were corrected. Verified `go doc testing/synctest.Wait` and used fixture mutex snapshots. An initial combined selector ran no borrowed tests; the exact subtest selector above subsequently ran them. The first actual command test failed its historical five-second startup observation window because root now performs cold Core-WASM preparation. Root preparation now uses the existing platform call deadline. A trial test observed that bounded startup plus one second; its five-second post-signal drain assertion was unchanged. Restored the historical startup test window and requested the required explicit earlier-artifact change approval under [D-86](../authoritative-plan.md#processing-cold-start-test-observation). No current full root verification or new startup SLA is claimed.

Actual production source reconciliation, post-commit intake wake-up wiring, installed run lifecycle composition, protected Processing HTTP and trusted definition/preview preparation remain required. These checks are a registered private runner/root slice, not a complete T043 or AP-06 handover.

After restoring the historical process-test window, final dispatcher Race exited 0 (1.340s); targeted Processing/root Vet and diff check also exited 0. No repeated cold-start process run was needed to rediscover the already observed five-second failure. AP baseline diff remains only the previously approved AP-15 topology amendment.

### E-39 Isolated River Queue Evaluation

Date: 2026-11-02. Scope was limited by [D-87](../authoritative-plan.md#ap-12-river-queue-evaluation) to an isolated River `v0.51.0` prototype; D-75, AP-12 bindings, product source, dependency manifests, contracts and migration files were not changed. This decision, architecture assessment and execution evidence are documented in their canonical locations. The scratch module and integration test live only in session storage at `/Users/leokr/.copilot/session-state/f3db121c-a671-4d82-8fb7-6ea0ec4a70ab/files/river-eval/`.

The prototype used the approved PostgreSQL `18.0` image, Go `1.26.2`, pgx `v5.11.0`, Goose `v3.29.0`, and River `v0.51.0`. A registered Goose Go migration used River's `riverdatabasesql` migrator to the pinned release's latest migration version. Runtime used `riverdatabasesql.NewWithPgxListener` with the same `*sql.DB` used by candidate storage and a dedicated pgx listener pool. The test verified that `InsertTx` on the same `*sql.Tx` persisted or rolled back the candidate and River job together; a transient failure completed on attempt two with `MaxAttempts: 5`; two jobs ran concurrently at configured `MaxWorkers: 2`; graceful client stop succeeded; and Goose down removed River's tables.

```sh
cd /Users/leokr/.copilot/session-state/f3db121c-a671-4d82-8fb7-6ea0ec4a70ab/files/river-eval
DATABASE_URL='postgres://postgres:river_eval_pw@127.0.0.1:55439/river_eval?sslmode=disable' go test -v -count=1 ./...
```

Final run exited 0 (`TestRiverWithGooseAndPostgresWorker`, 0.56s; package 1.015s). An earlier prototype assertion failed because its test IDs did not match its concurrency-gate prefix; the harness was corrected before the passing run. The temporary PostgreSQL container was stopped and removed. This establishes queue mechanics and Goose/database/sql integration, not compatibility with C-08's source authority, revision, lease/fencing, retry exhaustion, publication, multi-process recovery or resource requirements. River has its own durable job lifecycle; therefore this evidence does not select it or authorize a second queue under D-75. Production adoption or AP/contract changes require a separate explicit decision.

### E-40 Isolated Watermill SQLite Comparison

Date: 2026-11-02. Scope was limited by [D-88](../authoritative-plan.md#ap-12-cross-profile-worker-and-pubsub-comparison) to the isolated SQLite comparison. Product source, manifests, migrations, contracts, D-75 and AP baselines were not changed. The scratch module and Goose migration remain in session storage at `/Users/leokr/.copilot/session-state/f3db121c-a671-4d82-8fb7-6ea0ec4a70ab/files/watermill-eval/`.

The prototype used Go `1.26.2`, Watermill `v1.5.3`, `wmsqlitemodernc v0.1.2`, ModernC SQLite `v1.60.1` and Goose `v3.29.0`. Goose created the adapter-compatible topic/offset schema, with runtime adapter schema initialization disabled. The tests verified same-transaction candidate/message commit and rollback, `Nack` redelivery of the same UUID, an unconsumed message and acknowledged offset across closing/reopening the SQLite database, unacknowledged-message recovery after closing/restarting a subscriber and waiting for its lock to expire, and Goose down removal of the Watermill tables. A two-subscriber, same-topic/same-group case received no second message before the first was acknowledged, then delivered the next message after Ack.

The first fixture used four open SQLite connections and encountered `SQLITE_BUSY`; the adapter's own test fixture uses one open connection. Aligning the prototype with that single-connection fixture resolved the issue. This is a test-configuration observation, not a production pool-size recommendation. Since the adapter's per-topic schema initializer is not a public migration API, the prototype's Goose migration manually matches the pinned adapter schema and must be reviewed on adapter upgrades.

```sh
cd /Users/leokr/.copilot/session-state/f3db121c-a671-4d82-8fb7-6ea0ec4a70ab/files/watermill-eval
gofmt -w watermill_sqlite_test.go
go test -race -count=2 ./...
go vet ./...
go mod verify
```

Final validation exited 0: both race-enabled test repetitions passed (package 5.340s), `go vet` passed and all modules verified. The Watermill SQLite adapter's upstream suite also passed with `GOWORK=off go test -count=1 ./...` from its module source (47.338s). The earlier ordinary verbose prototype run passed before the final additional persisted-pending-message assertions; the final repeated Race run includes those assertions.

This proves bounded adapter behavior only. It does not test a hard process kill, production SQLite concurrency/resource characteristics, throughput, complete C-08 mapping, candidate source authorization/revision/fencing/attempt exhaustion/terminal state, or an end-to-end GoPager Lite/Full composition. Delivery is at-least-once, and the observed single-group test is not a throughput benchmark. For the comparison and recommendation, see the [architecture assessment](../architecture.md#ap-12-cross-profile-worker-and-pubsub-comparison); PostgreSQL Watermill evidence remains [AP-05 E-13](ap-05.md#e-13-event-infrastructure-required-behavior-probes).

### E-41 Isolated Watermill Evented-core Prototype

Date: 2026-11-02. Scope was limited by [D-90](../authoritative-plan.md#core-domain-event-driven-architecture-direction-and-watermill-prototype) to a session-local synthetic source-to-read-model prototype. Scratch module and tests: `/Users/leokr/.copilot/session-state/f3db121c-a671-4d82-8fb7-6ea0ec4a70ab/files/evented-core-eval/`. Changes attributable to this prototype are limited to this evidence entry and the linked architecture update; no product source, dependency manifest, migration, contract, D-75 decision or AP baseline changed.

The fixture used Go `1.26.2`, Watermill `v1.5.3`, `watermill-sql/v4 v4.1.5`, `wmsqlitemodernc v0.1.2`, ModernC SQLite `v1.60.1`, pgx `v5.11.0`, Testcontainers `v0.44.0`, and the repository-pinned PostgreSQL `18.0-alpine` OCI digest. SQLite used a temporary WAL database with one open connection and a 1500 ms adapter lock timeout; neither is a production pool or tuning recommendation.

Passing cases verified same-transaction source/fact commit and rollback; independent consumer groups and readiness gating; duplicate fact delivery guarded by durable receipts; a committed projection redelivered after subscriber shutdown before Ack and deduplicated on recovery; stale revision rejection and rejection of an unauthorized fact at the current revision; replay without creating external-work rows; and PostgreSQL visibility ordered by transaction commit when a later publisher transaction commits before an earlier one. The acknowledgment test waits for the persisted transport offset before deliberately shutting down the earlier acknowledged consumer, so normal Ack persistence is not mistaken for the separate commit-before-Ack recovery case. These are synthetic adapter-boundary cases, not full GoPager contract integration.

The first runs exposed a missing pgx `database/sql` driver registration and insufficient SQLite lock timeout; the next run exposed a SQLite positional-placeholder count mismatch in the fixture. A later run showed the acknowledged stale message could be redelivered when the test closed the subscriber before its offset write completed. The harness registered pgx, raised its SQLite timeout, supplied the repeated positional argument and waited for the stored offset before shutdown. No product behavior or adapter source was changed.

```sh
cd /Users/leokr/.copilot/session-state/f3db121c-a671-4d82-8fb7-6ea0ec4a70ab/files/evented-core-eval
go mod tidy
gofmt -w evented_core_test.go
GOPROXY=off go test -v -count=1 ./...
GOPROXY=off go test -race -count=1 ./...
GOPROXY=off go vet ./...
GOPROXY=off go mod verify
```

Final ordinary tests exited 0 (package 15.465s), Race tests exited 0 (15.773s), `go vet` exited 0, and `go mod verify` reported `all modules verified`. PostgreSQL used Testcontainers with Docker Server `28.0.4`; temporary containers were terminated by the test harness. The ordinary and Race runs each covered both SQLite and PostgreSQL atomicity/recovery/authority/readiness/replay scenarios; reverse commit-order visibility was PostgreSQL-only. This evidence does not establish hard-process-kill behavior, complete C-08/C-11 source publication, multi-process domain fencing, production SQLite resource/concurrency characteristics, throughput, or Lite/Full product composition. Watermill remains a candidate, not an adopted product dependency.

### E-42 Watermill Crash, Atomicity and Concurrency Extension

Date: 2026-11-02; AP-12 T043/T045 infrastructure assessment under [D-91](../authoritative-plan.md#watermill-failure-and-concurrency-validation-extension). Executing owner: current GoPager integration agent, sequentially. Starting HEAD remains `24318de414b3b35f77b6b288905125c1debf46f4` with the extensive existing uncommitted baseline. Prior E-39..E-41 results are preserved. Write scope: session-only `evented-core-eval/{evented_core_test.go,failure_test.go}`, this evidence, the approval record and architecture interpretation; no product code, dependency manifest, migration, contract or AP baseline changed. The existing AP-15 baseline delta was inspected, not edited.

The pinned toolchain/adapters/container tuple is E-41's, verified here with Go `1.26.2 darwin/arm64` and Docker Server `28.0.4`. Added only standard-library process/concurrency testing and synthetic application transaction glue, reusing Watermill, pgx, ModernC and Testcontainers. New SQLite fixtures use documented per-connection `_pragma` DSN options for WAL, `busy_timeout(5000)`, `synchronous(FULL)` and foreign keys, without shared cache; actual WAL/busy/synchronous values are asserted. This replaces the earlier scratch fixture's unverified generic URI options; it is not a production configuration selection.

| Executed case | Observed outcome and boundary |
| --- | --- |
| Hard consumer-process termination before application commit | Passed on PostgreSQL and SQLite. A child of the test binary subscribes and stages receipt/result/work/follow-up fact; the parent waits for an explicit transaction checkpoint, kills that specific process with `os.Process.Kill`, reaps it and checks all application writes are absent. A restarted consumer receives the same UUID and commits all participants once. The database server itself is not killed. |
| Hard termination after commit, before transport Ack | Passed on both stores. All four application participants survive; transport offset remains unacknowledged. Redelivery uses the durable receipt and does not create a second result, work item or follow-up fact. |
| Atomic receipt/result/work/follow-up publication | Passed on both stores. Explicit rollback removes every participant; Nack redelivery commits them together and the follow-up message is actually delivered. |
| Competing same-group consumers | Passed on both stores using independent pools. The second consumer cannot pass the held message during a 250 ms observation; after the first subscriber closes without Ack, the second recovers the same UUID and then advances. This is bounded exclusion/failover evidence, not multi-process domain fencing. |
| SQLite independent writers and active projection consumer | Four independent writer pools each committed 12 source/fact transactions while the consumer wrote receipts/results/work/readiness. All 48 unique facts and corresponding state were verified, with no application retry in this bounded case. This is not a workload/throughput benchmark. |
| Forced SQLite write contention | A separate held write transaction exhausts the peer's five-second busy timeout and returns typed `SQLITE_BUSY`. No source/fact partial write remains. An explicit caller retry after release commits both. No automatic application retry policy is claimed. |
| Slow handler, two-second consumer lock, independent competitor | PostgreSQL passed with subscriber pools of one and four connections. SQLite failed with both sizes: after holding delivery for four seconds, Ack did not persist within the five-second observation. The final Race run reproduced each SQLite failure twice. Increasing the pool is not a demonstrated remedy. |
| Isolated SQLite `UPDATE ... RETURNING` lifecycle | Passed as a diagnostic reproduction: calling `QueryRowContext` and `Row.Err` without `Scan` retains the connection and leaves the renewal invisible to another connection; consuming `Scan` releases it and makes the update visible. |

**SQLite renewal blocker:** Inspected pinned `wmsqlitemodernc v0.1.2` module source `subscription.go:113..121` and `subscriber.go:273..278`: `ExtendLock` executes a renewal `UPDATE ... RETURNING`, checks `Row.Err`, but never scans/closes the returned row or verifies a returned lock. Go `database/sql.Row.Scan` closes its rows; `Row.Err` merely returns the stored error (`/usr/local/go/src/database/sql/sql.go:3486..3528`). The focused driver reproduction and observed pool retention support this as the cause of the stalled renewal/Ack path. This is an adapter integration blocker, not proof of data loss, a core Watermill defect across all adapters or a reason to waive domain fencing. No upstream module/cache code was patched, and no larger pool/timeout workaround is selected. Architecture/integration owns resolution before cross-profile adoption.

The first competing-consumer fixture mistakenly read the offset through the same single-connection PostgreSQL pool held by its subscriber transaction. Reading via the independent observer pool resolved that harness deadlock. Earlier scratch duplicate/replay tests were strengthened to assert the exact delivered UUID, persisted Ack where needed and actual replay result, so a preceding message's redelivery cannot satisfy the replay assertions. E-41's historical observations are not overwritten. The later diagnostic test initially had a misplaced closing brace caught by `gofmt`; it was corrected before the final test/vet runs.

Ordering clarification from the pinned PostgreSQL query and retained reverse-finish test: it waits for an earlier open publishing transaction, then delivers by transaction ID and offset even when the later transaction committed first. This is not wall-clock commit-order delivery or a global domain ordering guarantee; E-41's phrase "ordered by transaction commit" must not be used to infer either.

Commands from the session-only module:

```sh
gofmt -w evented_core_test.go failure_test.go
GOPROXY=off go test -v -count=1 -timeout=180s ./...
GOPROXY=off go test -race -count=3 -timeout=240s ./...
GOPROXY=off go test -v -count=1 -timeout=90s -run '^TestSlowConsumerLockRenewal$' ./...
GOPROXY=off go test -v -count=1 -timeout=90s -run '^TestSQLiteReturningRequiresScan$' ./...
GOPROXY=off go test -race -v -count=2 -timeout=240s ./...
GOPROXY=off go vet ./...
GOPROXY=off go mod verify
```

The ordinary full suite **before adding slow-handler renewal** passed (22.553s), as did three Race repetitions of that earlier test set (66.099s). The initial slow-handler test failed on SQLite with the earlier fractional lock timeout; the full-second/two-pool isolation then failed on SQLite with both pool sizes (31.388s), while PostgreSQL passed. The focused RETURNING reproduction passed (2.268s). **Final full Race validation exited 1 (102.255s): both repetitions failed only the SQLite slow-handler cases for pools of one and four connections; all other cases passed.** The helper test is intentionally skipped in the parent and executed in the crash children. No race-detector report occurred; this is a runtime correctness failure, not a passing suite. Final `go vet` and `go mod verify` exited 0 (`all modules verified`); test-owned child processes were reaped and PostgreSQL containers terminated. Repository `git diff --check` exited 0; the AP baseline diff still contains only its pre-existing AP-15 amendment.

Handover: the failing regression remains runnable in session storage. The next step is a separately approved, isolated assessment of an upstream-corrected adapter/version or narrow candidate fix, followed by the same renewal/crash/concurrency tests. No production adoption or downstream AP amendment follows from these results. Database-process/power-loss durability, full C-08/C-11 integration, stale-worker domain fencing, capacity/resource acceptance and Lite/Full composition remain unverified.

### E-43 Isolated SQLite Renewal Correction and Upstream Check

Date: 2026-11-02; AP-12 T043/T045 infrastructure assessment under [D-92](../authoritative-plan.md#isolated-sqlite-adapter-renewal-fix-assessment). Executing owner: current integration agent, sequentially. E-42's failing baseline remains unchanged and runnable. No product implementation, dependency, migration, contract or AP baseline was changed; the existing AP-15 delta remains untouched.

**Upstream/version check:** Preferred pkgsite version discovery again failed at initialization with HTTP 503; used public authoritative GitHub APIs instead. `git/matching-refs/tags/wmsqlitemodernc` returned no version beyond `v0.1.2`, whose commit is `a2a915319684f030296ccb15403b37f292640ad2`. Inspected current `master` at `843d10d38a356dde0c794325c7d215b589e443c2` (2026-10-28), the renewal file's commit history, and all five issue/PR entries returned by the repository list. No published renewal fix was found; `master` still has the unconsumed row (`subscription.go` Git blob `06ed3991ad7924a92d201e6c840e150db8a7bdd1`). Sources: [current revision](https://github.com/ThreeDotsLabs/watermill-sqlite/commit/843d10d38a356dde0c794325c7d215b589e443c2), [pinned upstream renewal source](https://github.com/ThreeDotsLabs/watermill-sqlite/blob/a2a915319684f030296ccb15403b37f292640ad2/wmsqlitemodernc/subscription.go#L113). These observations are point-in-time, not a future release guarantee. No public issue, PR or code upload was created.

**Experimental delta:** Copied the pinned module to session-only `files/wmsqlitemodernc-renewal-eval/`, preserving the MIT license and copyright notice. Changed only `subscription.go:ExtendLock`: consume `UPDATE ... RETURNING` with `Row.Scan`, return wrapped scan errors (including `sql.ErrNoRows` for an expired/missing matching lock), and reject a zero returned lock before updating local progress/timer. Added `renewal_regression_test.go` for active and expired conditional updates, verifying persisted offset/expiry, explicit failure, unchanged local state on rejection and zero retained pool connections. Recursive comparison against the cache copy confirms only this source file and the additional regression test differ.

The original scratch `go.mod`/`go.sum` remain unchanged and resolve the released adapter. Separate `go.fixed.mod`/`go.fixed.sum` select the licensed copy through one local `replace`; checksum files are identical, and effective ModernC remains `v1.60.1`. `GOWORK=off` prevents the copied upstream workspace file from changing dependency resolution. No module cache file or installed dependency was modified. Initial regression-file creation hit the copied directory's read-only permission; only the scratch directory was made writable, then the file was added.

Commands from `files/evented-core-eval/` after creating the source copy and alternate manifests:

```sh
go mod edit -modfile=go.fixed.mod -replace=github.com/ThreeDotsLabs/watermill-sqlite/wmsqlitemodernc=../wmsqlitemodernc-renewal-eval
GOWORK=off GOPROXY=off go test -modfile=go.fixed.mod -v -count=1 -timeout=90s -run '^TestSlowConsumerLockRenewal$' ./...
GOWORK=off GOPROXY=off go test -modfile=go.fixed.mod -race -v -count=3 -timeout=60s github.com/ThreeDotsLabs/watermill-sqlite/wmsqlitemodernc -run '^TestRenewalConsumesReturning$'
GOWORK=off GOPROXY=off go test -modfile=go.fixed.mod -race -count=3 -timeout=240s ./...
GOWORK=off GOPROXY=off go test -modfile=go.fixed.mod -race -count=1 -timeout=300s github.com/ThreeDotsLabs/watermill-sqlite/wmsqlitemodernc
GOWORK=off GOPROXY=off go vet -modfile=go.fixed.mod ./... github.com/ThreeDotsLabs/watermill-sqlite/wmsqlitemodernc
GOWORK=off GOPROXY=off go mod verify -modfile=go.fixed.mod
```

**Results:** Every listed test/vet/verify command exited 0. Slow-handler renewal passed on PostgreSQL and SQLite with subscriber pools of one and four (21.812s package); SQLite in-use connections at the four-second checkpoint dropped from the E-42 failure's one/two to zero in both configurations. Active/expired adapter regressions passed all three Race repetitions (1.376s). The complete crash/atomicity/recovery/contention/replay/renewal suite passed three Race repetitions (116.133s). All copied upstream adapter tests, including its non-short file/memory implementation acceptance cases, passed with Race enabled (271.297s). Those tests used the evaluated application's driver tuple, not the upstream module's older standalone driver pin. `go mod verify` reported `all modules verified`; `gofmt -l` reported no formatting changes. PostgreSQL test containers were terminated and crash children reaped.

Source SHA-256: original `subscription.go` `c596a74c6f2f100d9110fb6c4a297c6b54d061d908d6a511f6fdda1fe987cbfa`; experimental copy `10a751c3ae41f88f8eb790284b05acfbfe65c5eb694e69a4e068a4e14624b8f1`. The expected `diff` exits of 1 identify precisely the deliberate source/manifest deltas, not test failures.

**Handover/limit:** The experiment verifies a narrow correction for E-42; it does not fix the released adapter or select a maintained fork. Upstream delivery or an explicitly approved pinned-patch maintenance strategy remains required before product adoption. Application receipts, authorization, revision checks and domain fencing remain necessary; rejection by this renewal method is not proof of every higher-level stale-handler path. E-42's broader durability, capacity, complete C-08/C-11 integration and profile-composition limitations remain. No automatic River fallback or downstream AP amendment is introduced.

### E-44 PostgreSQL-first Gate Classification

Date: 2026-11-02; AP-12 T043/T045 documentary follow-up. Checked the user's clarification against D-32's existing Full-first/Lite-later scope. [D-93](../authoritative-plan.md#watermill-postgresql-first-gate-clarification) confirms that the SQLite adapter finding is not a blocker for PostgreSQL integration. E-42/E-43 observations and original handovers remain historical; their unqualified "before product adoption" wording must not impose SQLite readiness on current Full delivery.

Updated the architecture's current gate and this record's blocker classification. No runtime test was rerun or newly certified; PostgreSQL evidence remains E-41..E-43. No product dependency/code/contract, AP baseline, SQLite patch strategy or public upstream artifact was changed or approved. Next permitted step is preparation of the concrete Full/PostgreSQL integration binding, independently of later SQLite delivery; implementation retains its separate approval gate.

### E-45 Concrete PostgreSQL Publication Preparation

Date: 2026-11-02; AP-12 T043/T045 infrastructure assessment following D-93 and the user's request to proceed. Read current typed facts, all existing append implementations, Ent DomainEvent schema, borrowed primary transaction resolution, central Goose migration ownership, Processing dispatcher/service/root composition and AP-06/AP-10 bindings. Working tree remains the extensive uncommitted baseline described above; no product file or AP definition changed in this preparation.

Observed that current appenders store canonical envelopes only; no custom event subscriber/router exists to remove. Processing's dispatcher is native candidate due/recovery scheduling, not a generic fact fan-out engine. The platform exposes a validated SQL executor, not raw transaction ownership; Watermill's pinned `ContextExecutor` has distinct return interfaces, so direct assignment is not valid and a narrow infrastructure adapter is needed. Library source inspected: `watermill-sql/v4@v4.1.5/pkg/sql/{publisher.go,sql.go,adapters_std_sql.go}`. The official adapter requires runtime schema initialization disabled with caller transactions.

Recorded the concrete [architecture proposal](../architecture.md#proposed-postgresql-integration-slice) and pending [D-94](../authoritative-plan.md#watermill-postgresql-publication-integration-proposal): canonical fact plus same-transaction Watermill reference publication for existing producers, with no fake production consumer or second Processing queue. Existing fact data is not automatically replayed. Proposed validation binds actual platform/Ent/Goose participation and earlier producer regressions rather than relabeling scratch results as product proof.

This is documentary preparation only; no new runtime/build/test was run and no dependency was installed. Product changes to earlier append artifacts and the additive contract/dependency binding remain blocked pending the specific D-94 approval. SQLite is not that blocker. No AP-06/AP-10 implementation or downstream baseline amendment is proposed.

### E-46 Approved PostgreSQL Watermill Publication Slice

Date: 2026-11-02; AP-12 T043/T045 infrastructure follow-up. User explicitly selected `approve-publication-slice`; [D-94](../authoritative-plan.md#watermill-postgresql-publication-integration-proposal) records the exact earlier-producer/contract/dependency/test write approval. The additive [C-08 postgresql-fact-publication 1.0](../contracts/c-08-events-jobs.md#postgresql-fact-publication) is bound without changing existing producer meanings or AP baselines. Assignee: current sequential executing agent; same extensive local baseline/HEAD as above.

Implemented [caller-transaction Watermill integration](../../internal/persistence/ent/eventstore/transport.go) for all five existing append methods and [central migration 00008](../../internal/persistence/migrations/00008_fact_transport.sql), using the approved [manifest tuple](../../go.mod). No platform transaction interface, old migration or generated Ent artifact changed. No root subscriber, alternate worker lifecycle or SQLite dependency was added. The existing root obtains the augmented eventstore without another composition path. Source/evaluation rationale remains at [the architecture owner](../architecture.md#proposed-postgresql-integration-slice); wire/storage definitions are linked from C-08. Added the internal reference term at the glossary owner.

The [product integration tests](../../internal/persistence/ent/borrowed/transport_test.go) verify every append family with Ent-guarded caller state, canonical fact and reference committed together; callback rollback; forced transport constraint rejection with its PostgreSQL cause preserved; duplicate fact rollback; exact reference-only body/metadata and canonical ID; foreign/expired capability rejection. Actual migrated-table subscribers demonstrate committed-fact lookup, no uncommitted/rolled-back input, redelivery on subscriber restart without Ack, independent groups, and observed durable Ack offsets rather than merely the in-memory Ack signal. Migration tests upgrade the previous release without replay/removal of legacy facts, reject a pending suffix, and fail append explicitly on missing table/incompatible column with no orphan fact or runtime DDL. [Executor guard tests](../../internal/persistence/ent/eventstore/transport_test.go) reject invalid ownership on both SQL methods rather than falling back to a connection.

Commands from `/Users/leokr/Projects/gopager` (Go 1.26.2 darwin/arm64; disposable PostgreSQL 18 fixture and Docker versions as E-43):

```text
GOWORK=off GOPROXY=off go mod tidy
GOWORK=off GOPROXY=off GOPAGER_INTEGRATION=1 go test -count=1 -timeout=180s -run '^TestFactTransport' ./internal/persistence/ent/borrowed
GOWORK=off GOPROXY=off GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=300s ./internal/persistence/ent/borrowed ./internal/events ./internal/platform/postgres
GOWORK=off GOPROXY=off GOPAGER_INTEGRATION=1 go test -count=1 -timeout=90s -run '^Test(Bootstrap|Management)' ./cmd/gopager
GOWORK=off GOPROXY=off go test -race -count=1 ./internal/persistence/ent/eventstore
GOWORK=off GOPROXY=off go vet ./internal/persistence/ent/eventstore ./internal/persistence/ent/borrowed ./internal/events ./cmd/gopager
GOWORK=off GOPROXY=off go mod verify
GOWORK=off GOPROXY=off GOPAGER_INTEGRATION=1 go test -count=1 -timeout=180s ./internal/persistence/ent/borrowed
GOWORK=off GOPROXY=off GOPAGER_INTEGRATION=1 go test -race -count=3 -timeout=120s -run '^Test(FactTransport|BorrowedDriver|IdentityFact|GlobalField|LocalBootstrap|ConcurrentLocalBootstrap|OrdinaryLogin|ProcessingFact|ProcessingInstanceImmutable)' ./internal/persistence/ent/borrowed ./internal/persistence/ent/eventstore
GOWORK=off GOPROXY=off go test -run '^$' ./...
GOWORK=off GOPROXY=off GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=90s -run '^Test(FactTransportMigrationUpgradeAndMissingStorage|TransportExecutorNeverFallsBackFromInvalidCapability)$' ./internal/persistence/ent/borrowed ./internal/persistence/ent/eventstore
git diff --check
git diff --stat -- docs/work-packages
```

Tidy succeeded using cached modules; an initial invocation outside the target working directory failed with no `go.mod` before any modification, then was corrected with explicit `cd`. First focused integration run passed (9.273s). **The broad Race command failed:** borrowed tests hit the 300s suite timeout while `TestProcessingWorkerPrivateEvaluationAndCurrentSourceRecheck` was compiling the existing WASM guest in Wazero; the stack was in `CompileModule`, not the publisher/SQL adapter. No race report was emitted; this is not a full Race pass or a proven unrelated root-cause diagnosis. The other packages in that command passed (events 1.354s; postgres 7.402s).

Root bootstrap/management tests passed (6.239s); the executor Race unit test passed (1.530s); vet, module verification and whitespace checks passed. The entire borrowed suite then passed without Race (66.540s), including existing real producer, worker/publication and command-receipt regressions. Three focused Race repetitions passed (39.873s), including the added incompatible-column case. The regex did not select the separately verified executor unit test in that multi-package command (`no tests to run` for eventstore). The all-package command compiled every package with no tests selected; it is not a full-suite runtime pass. Final review fixed the previous-release fixture to exclude migration 00008 and all future suffixes; the migration and executor Race checks then passed again (4.148s/1.517s). AP diff remains only the earlier approved AP-15 one-line amendment.

**Handover/limits:** The PostgreSQL publication foundation is implemented and verified within D-94, not a completed T043/T045 or AP-10 reaction. No hard process/database crash or power-loss durability, capacity, full dependency security, production subscriber lifecycle or real downstream domain outcome is certified by these product checks. Prior isolated crash evidence remains E-42/E-43. Operation dedup/uncertain commit still belongs to the unchanged producer/platform boundary; existing real command-receipt regressions and platform tests pass, but no lost-COMMIT-response injection was added to the product. Consumer receipt/result/work atomicity, start/catch-up, retention, current authorization and readiness/lifecycle binding remain prerequisites for a real production consumer. No consumer or downstream AP baseline is changed/started by this slice.

## Blockers and Deviations

- E-42's SQLite renewal/Ack failure and E-43's isolated correction remain later Lite adapter prerequisites, not blockers for current Full/PostgreSQL integration under D-93/D-32. No SQLite production patch or fork is selected. PostgreSQL-specific product integration and approval gates remain.
- D-94's bounded PostgreSQL publication approval is implemented in E-46. A real production consumer and its domain/lifecycle/retention binding remain separately gated; neither the worker replacement nor a full AP-10 handover follows from it.
- E-02's earlier-producer write gate is resolved by the user's exact D-71 approval. Implementation/validation of the adapter is now permitted, not yet a completed installed-runtime handover.
- Original Transmission/Message/source-readiness owners are intentionally still AP-06 inputs. Implement typed ports and declared fixtures with explicit missing-owner failure, not fabricated ready production sources.
- Whole AP-12/T044, feed/frontend/Delivery/general SDK and full identity management remain outside the approved binding.
- No deviation or additional product/operator/permission decision is adopted.
- D-72's concrete workflow-target-results 1.1 replacement was explicitly approved after E-08 preparation. Its implementation in the already started T043 kernel is now permitted; no runtime handover is inferred from approval.
- E-04's two pseudo-version lookup gaps are resolved by E-06's verified canonical module-index fallback, with its explicitly limited audit scope.
- D-73's exact 32 MiB load extension was explicitly approved; implementation/tests are now permitted, not inferred completed from approval.
- D-74's exact internal retry jitter is approved and its scheduler/bound tests are verified in E-15; durable lease/retry integration is still required.
- D-75's candidate placement is approved; E-17 records schema/configuration persistence. Durable candidate/result execution and fenced source publication remain required, not blocked on a repeated admission-choice question.
- D-76's separate preview representation is approved and its invocation/fixture verified in E-22. Real administrative instance/current-source adapters remain required; the earlier E-21 gap is preserved as history.
- D-77 option A admission/reselection and primary-clock claim/renewal are verified in E-24. Complete worker/source/failure/recovery/result/publication integration remains required.
- D-78's existing scoped `rule.reprocess` binding and standalone retry coordination are verified in E-25; real owners/HTTP/worker integration remain required.
- D-79's explicit existing C-08 terminal-rule interpretation/addendum is approved by `approve-terminal-old-revision-with-new-logical-candidate-revision`; the temporary affected-integration pause is resolved. The E-25 inspection error/history remains recorded.
- D-80 requests only D-68's still-unresolved Protocol/form and resolved Network field-group memberships needed by real source declarations. Existing global visibility/source eligibility mechanisms remain unchanged; no membership is guessed or new group added before the owner resolution.

## Handover

No kernel handover yet. Produce and verify T043 interfaces/runtime/persistence/HTTP/fixtures under the exact binding, then hand them to AP-06 T023/T024. A frozen contract and approved start are not implementation completion.

### E-47 Resliced Execution Path Reconciliation

Date: 2026-11-03. Under the user's explicit [D-96 approval](../authoritative-plan.md#work-package-reslicing-decision), this record remains the historical evidence owner for partial T043/T045 checks; its earlier `in-progress` markers are observations, not concurrent current assignments. The same kernel scope continued as historical AP-17, the only active assignment at that time; the start/check observations are retained in [AP-06 E-01/E-02](ap-06.md#e-01-resliced-assignment-start). T044/source-bound T045 were assigned to historical AP-23, not accepted. Intake was still blocked/not-started. No runtime result is changed or duplicated.

Current results are in [AP-06](ap-06.md#handover); remaining presentation/source-bound integration is [AP-13](../work-packages/ap-13-processing-presentation-integration.md). This record owns historical evidence only.
