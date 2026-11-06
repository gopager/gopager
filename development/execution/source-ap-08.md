# Source AP-08 Historical Execution Record

## Handover

Current as of the D-135 follow-up (AP-08 E-18..E-25); earlier states are in the evidence entries.

Verified D-60 prerequisite: Unit/collection/current-context operations and standalone temporal-rule/current matching only. Exact implemented artifacts are linked from [E-02](#e-02-implemented-approved-prerequisite-slice); actual PostgreSQL/race/HTTP cases are in [E-03](#e-03-postgresql-race-http-and-proxy-validation).

Consumers retain current scope/revision/lock participation and server-derived candidate contexts. Original verification used explicit Network fixtures; current production intake/Network composition is evidenced separately in [AP-07](ap-07.md#handover).

Templates, Organizations, defaults, bulk operations, Suppressions and historical correction were delivered under [AP-08](ap-08.md#handover), which also records the D-135 changes to this slice (Units belong to an Agency, selector-scoped correction, the `message_units` cache instead of supports, Network resolution with address inference and priority); this record verifies neither those operations nor whole original tasks. Historical blockers below are dated observations, not unowned inputs.

## Baseline

The [original AP-08 baseline](https://github.com/gopager/gopager/blob/24318de414b3b35f77b6b288905125c1debf46f4/docs/work-packages/ap-08-master-data-correction.md) remains unchanged at commit `24318de414b3b35f77b6b288905125c1debf46f4`. Execution starts on 2026-10-27 with existing local AP-04/AP-06/AP-07 documentation and identity implementation changes, not a clean worktree.

Assignee: current Copilot SDK assistant, sequential execution. The user separately approved the four input freezes and `minimal_matcher_execution=approve-and-start` under [D-60](../authoritative-plan.md#early-master-data-and-matcher-prerequisite-proposal). Exact versions, narrow write scope, exclusions and required validation belong to the [AP-04 consumer binding](ap-04.md#ap-08-prerequisite-consumer-binding-proposal). Prerequisites: verified AP-05 foundation and [D-59 identity core](source-ap-07.md#handover). No whole T028/T029 or AP-08 acceptance is implied.

## Task Outcomes

| Task | State | Evidence |
| --- | --- | --- |
| T028 | in-progress | Approved early Unit/collection/current-context slice verified in E-02/E-03; other master-data operations remain deferred. |
| T029 | in-progress | Standalone current-rule/matcher boundary verified in E-02/E-03; historical support redistribution/suppression/correction integration remains later work. |
| T030 | not-started | Historical integration requires AP-06; no fixtures are reported passed. |

## Evidence

### E-01 Approved Start

Date: 2026-10-27. The user approved `minimal_matcher_contract_freeze=approve`, then separately `minimal_matcher_execution=approve-and-start`. The four C-ID owner sections identify the exact 1.0 inputs. Existing code, transaction/authorization adapters and focused PostgreSQL fixture conventions are the implementation baseline; no new dependency tuple is authorized.

### E-02 Implemented Approved Prerequisite Slice

Date: 2026-10-27..28. Implemented the D-60 slice with actual artifact bindings at [C-04](../../docs/http-api.md#master-data-semantics), [C-05](../../docs/architecture.md#address-rule-semantics), [C-02](../../docs/architecture.md#address-rule-semantics) and [C-08](../../docs/architecture.md#domain-events-and-outbox). Administrative receipts use [the shared nonsecret owner port](../../internal/commands/receipt.go) and [borrowed adapter](../../internal/persistence/ent/commandstore/receipt.go); [the identity owner](../../internal/persistence/ent/identitystore/configuration.go) retains its shared configuration revision/audit responsibility. No new dependency was introduced.

All current evaluation locks shared access/configuration before Assignment state, retaining both until the caller's transaction completes. This also serializes ordinary Unit/context edits, not just rule edits/deletion. The field-restricted internal Unit context is distinct from the public name-bearing DTO. Native Ent relationship predicates bind current Network/Unit/collection scopes before read/count/page/write; configured disabled rules remain configuration uses, not successful matches. Collection-scoped deletion captures its exact authorized context before detaching its supporting relationships. Revision exhaustion uses the shared `RESOURCE_LIMIT` semantics before increment.

The root registers the approved management adapters with the existing listener/store. It accepts explicit transaction-participating Network owner ports but currently starts production composition without them: operations requiring that owner fail explicitly. The declared test Network owners are fixtures, not a production substitute or evidence of Gateway intake.

### E-03 PostgreSQL Race HTTP and Proxy Validation

Date: 2026-10-28. Environment: Go 1.26.2, macOS arm64, Docker Desktop, the repository-pinned PostgreSQL 18.0 Alpine fixture. Commands run in `/Users/leokr/Projects/gopager`:

```sh
GOPAGER_INTEGRATION=1 go test -race -count=1 ./cmd/gopager ./internal/protocols ./internal/assignments ./internal/Units ./internal/commands ./internal/authz ./internal/events ./internal/identity ./internal/persistence/ent/...
go vet ./cmd/gopager ./internal/protocols ./internal/assignments ./internal/Units ./internal/commands ./internal/events ./internal/persistence/ent/...
git diff --check
git diff --exit-code -- docs/work-packages
```

Final observed result: all commands exited 0; the actual borrowed PostgreSQL suite passed in 35.597 s and the command/HTTP suite in 11.904 s. Earlier iterations caught and corrected a generated enum type mismatch, test syntax/method-name errors and an incorrect fixture column name; those failed attempts are not passing evidence. Database-disabled quick compilation is not counted as PostgreSQL validation.

Executed evidence:

- [Protocol fixtures](../../internal/protocols/targets_test.go) and [matcher fixtures](../../internal/assignments/rules_test.go): typed JSON, class-aware FLEX gaps/ranges, exact preserved ZVEI/EAS strings, proper-subset specificity rather than Address priority, equal/partial/mode conflicts, half-open time bounds, duplicate original positions, explicit unassigned/empty targets and cancellation.
- [Real domain integration](../../internal/persistence/ent/borrowed/assignments_test.go): identical/set-equivalent and conflicting actor-bound retry; operation mismatch; pure preview; expected revisions; state/audit/fact/receipt/access/Assignment/pending-scope rollback; actual revision exhaustion; concurrent conflicting creates; caller-restricted Network candidates; shared hidden-use denial; current Unit deletion and durable unfinished correction; collection detach/member revisions, including collection-self-context authorization.
- The same integration suite compares native SQL filtering/count/page results with point decisions under mixed Network/Unit/collection grants and persisted restricting keys, including disabled configuration and a conflict hidden by the key. It observes actual `pg_blocking_pids` while ordinary Unit edits and deletion wait for a live evaluation, then checks the new context/revision.
- [Actual HTTP tests](../../cmd/gopager/management_test.go) cover protected Unit/collection/rule management and preview, retry, stale delete, missing/null membership refusal, CSRF, explicit malformed-Bearer precedence despite a Session cookie, caller-safe errors, HTTP-only operation and HTTPS reverse-proxy operation with an HTTP upstream. The production unwired-Network case separately returns `AUTHORITY_UNAVAILABLE`; positive rule HTTP tests use an explicitly declared fixture owner.
- [Configuration envelope tests](../../internal/events/configuration_test.go) retain the separate producer vocabulary and safe typed data; [revision tests](../../internal/commands/receipt_test.go) distinguish current exhaustion from a stale supplied number. The same run preserves the existing identity/bootstrap/session/native-permission regressions.

No historical rematching/suppression/publication, original intake, Message/contribution persistence, external Delivery, general master-data completion, SQLite parity or capacity claim follows. Owned PostgreSQL schemas/containers were cleaned up by the fixture lifecycle. AP baselines remain unchanged.

## Blockers and Deviations

- Production Network scope/Protocol ownership is supplied by AP-06; standalone owner fixtures must not be presented as complete production wiring.
- Historical correction and the remaining T028 operations are outside the approved early slice, not implemented placeholders or whole-task completion.

## Historical Handover

The **verified D-60 prerequisite only** is available to AP-06 T022/T023 through the four owner artifact bindings in E-02, with actual evidence in E-03. No whole T028/T029/AP-08 acceptance is claimed. Intake/AP-06 must supply the actual Network existence/Protocol/scope owner, use server-derived candidate context and retain the transaction/lock boundary; it must not expose internal matching as caller-supplied authority. AP-06 still needs the T043 evaluator kernel and its own contract/execution binding.

No downstream AP amendment was needed: the staged producer/consumer boundaries remain those of the unchanged baselines. Later AP-08 work retains its original definitions and required historical integration.

### E-04 Resliced Execution Path Reconciliation

Date: 2026-11-03. Under the user's explicit [D-96 approval](../authoritative-plan.md#work-package-reslicing-decision), E-02/E-03 remain the evidence owners for the verified D-60 early Unit/collection/current-matcher handover. The original T028/T029 `in-progress` and T030 `not-started` entries above are historical snapshots, not current assignments. The remaining master-data and historical-correction scope was assigned to [historical AP-19, now current AP-08](../work-packages/ap-08-master-data-correction-after-intake.md), after the revised Transmission-intake step. No whole source AP-08 completion or new fixture result is inferred.
