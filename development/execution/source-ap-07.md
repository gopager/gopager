# Source AP-07 Historical Execution Record

## Handover

Verified prerequisite: bootstrap, login/logout/self-Session, current-authority resolution, point authorization/native filtering, restricting-key resolution, global fields and immediate invalidation. [E-02..E-13](#e-02-credential-primitives-and-api-registration) and the [field-correction evidence](#unscoped-field-correction-verification-and-handover) retain exact checks; [C-06](../../docs/http-api.md#authorization) owns executable interface/artifact bindings.

AP-06/AP-07 and each later protected domain compose server-derived scope-existence/filter/projection adapters in the live primary transaction; point authorization never permits raw entity output. No Guest/admin/worker fallback for missing authority.

This does not verify general management, Recovery/rescue, key issuance/rotation or participant cleanup; [AP-14](../work-packages/ap-14-identity-lifecycle.md#planning-before-coding) owns them. Historical AP/task numbers and blockers below describe their observation dates, not current prerequisites.

## Baseline

The [original AP-07 baseline](https://github.com/gopager/gopager/blob/24318de414b3b35f77b6b288905125c1debf46f4/docs/work-packages/ap-07-identity-authorization.md) is unchanged at Git commit `24318de414b3b35f77b6b288905125c1debf46f4`. Execution starts on 2026-10-26 with existing uncommitted AP-04/AP-06 documentary changes, not a clean worktree.

Assignee: Copilot SDK assistant, sequential execution. Exact partial scope, version/library tuple and validation are approved in the [AP-04 prerequisite binding](ap-04.md#ap-07-prerequisite-consumer-binding-proposal) and [plan gate](../authoritative-plan.md#workflow-and-approval-gates). AP-05 is the verified platform prerequisite. This is the central identity/authorization handover required by AP-06/AP-08/AP-12, not whole-package acceptance.

## Task Outcomes

| Task | State | Evidence |
| --- | --- | --- |
| T025 | in-progress | AP-06 prerequisite login/logout/bootstrap/Session slice verified in E-02/E-08..E-13; broader management and Recovery remain deferred under D-59, not completed. |
| T026 | in-progress | Current grant/group/key resolution, point authorization and native Ent filtering verified in E-03/E-07/E-10/E-13. Actual domain scope/projection composition belongs to the consuming prerequisite/ingest slices. |
| T027 | in-progress | Immediate authority/revocation/inactive-account boundary verified; personal-data cleanup and the full deletion workflow are not part of this prerequisite handover. |

## Evidence

### Unscoped Field Correction Start

Date: 2026-11-01. The user explicitly approved D-70 identity-fields 1.0 and separately `unscoped_field_execution=approve-and-start`. Reopen only the required identity field producer/consumers under the [AP-04 binding](ap-04.md#unscoped-identity-correction-consumer-binding). Prior E-01..E-13 evidence remains historical, not validation of the replacement. Current working tree includes earlier uncommitted implementation and documentary changes; no clean-baseline or whole-AP-07 acceptance is claimed.

### Unscoped Field Correction Verification and Handover

Date: 2026-11-01. The separately approved `identity-fields1.0` correction is implemented and reverified; earlier scoped-field evidence remains historical. The authoritative record/behavior owner is [C-06](../../docs/http-api.md#authorization).

Implementation: [global field authority](../../internal/authz/fields.go), [point decisions](../../internal/authz/decision.go), [native query authorization](../../internal/persistence/ent/permissions/filter.go), [current identity resolver](../../internal/identity/authority.go), [Ent authority loader](../../internal/persistence/ent/identitystore/authority.go), [bootstrap](../../internal/persistence/ent/identitystore/bootstrap.go), [generated schema source](../../internal/persistence/ent/schema/identity.go), and [migration 00005](../../internal/persistence/migrations/00005_global_field_permissions.sql). Generation used the existing `go generate ./internal/persistence/ent`; no new dependency or general management endpoint was introduced.

Actual verification from the target repository:

```text
GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/authz ./internal/identity ./internal/persistence/ent/permissions ./internal/persistence/ent/borrowed ./cmd/gopager
go vet ./internal/authz ./internal/identity ./internal/persistence/ent/... ./cmd/gopager
```

Both commands exited 0. Test-package durations were 1.293s, 2.678s, 1.200s, 37.797s and 14.371s respectively. PostgreSQL integration was explicitly enabled, not inferred from a skip-capable ordinary test run. The existing fixture applied the real migration chain including 00005; existing HTTP/login/Reverse-proxy process tests ran in the same command.

Evidence covers global Principal/User-group union, two independently permitted Unit scopes, denial of foreign data despite field permission, read/detail and native-count parity, restricting-key field intersection, delegation without scoped exceptions, invalid/unavailable stored field authority without success-shaped fallback, current membership removal, and explicit bootstrap field selection. The [PostgreSQL group test](../../internal/persistence/ent/borrowed/fields_test.go) proves revision/audit/fact participation and rollback for fixture-driven changes; it does not claim an implemented permissions editor. The [bootstrap test](../../internal/persistence/ent/borrowed/bootstrap_test.go) exercises rollback after fact failure and no field authority for Guest. Focused [authorization tests](../../internal/authz/fields_test.go) and [resolver tests](../../internal/identity/authority_test.go) own the non-SQL cases.

Migration limits: fresh-install execution is verified. The nonempty legacy-development field guard and guarded Down path were not separately exercised in this run; no local existing data was reset, deleted or automatically made globally visible. There is no production-deployment migration claim.

Corrected producer handover: AP-06, the already-started AP-08 matcher slice, and the eventual AP-12 processing consumer must use `Authority.FieldPermissions`/`AuthorizeFields`, not `Grant.Fields` or per-field scope predicates. Data/action scopes, ages and target eligibility remain separate. AP-06 may now consume this correction; D-66 and the AP-12 T043 freeze/execution gates remain unresolved independently. Full AP-07 still remains incomplete outside this approved prerequisite slice.

### E-01 Approved Start and Foundation Inspection

Date: 2026-10-26, macOS, Go `go1.26.2 darwin/arm64`. `git -C /Users/leokr/Projects/gopager rev-parse HEAD` exited 0 with the baseline above. The user separately approved the identity-core freeze and execution start. No earlier execution record or AP baseline was changed.

Read the platform transaction/store/composition, test-fixture and migration-test APIs. The existing platform binds only its Health handler; no application-handler registration exists. Adding identity routes to the same configured listener requires a separately approved additive composition interface rather than a second listener, mutation of a running server or changes to the Health responses.

### E-02 Credential Primitives and API Registration

Date: 2026-10-26, same Go/macOS environment as E-01. The user separately approved the additive [C-12 application handler registration](../../docs/architecture.md#packages). [Platform API composition](../../internal/platform/api.go) and `StartWithAPI` preserve the existing no-API `Start` entry point, Health routes and readiness/draining boundary. [Credential tests](../../internal/identity/credentials_test.go) cover D-54 comparison, byte-preserving password handling, salted Argon2id and canonical random-secret hashing.

`go test ./internal/platform ./internal/identity` exited 0. E-03 additionally executes these packages under the race detector. The API test uses an actual HTTP test server; this is not evidence of a completed login/Recovery/Session HTTP adapter.

### E-03 Native Ent Transaction and Privacy Participation

Date: 2026-10-26. [Ent schemas](../../internal/persistence/ent/schema/identity.go), the [generator directive](../../internal/persistence/ent/generate.go), the pinned module tool, [mandatory privacy policy](../../internal/persistence/ent/guard/policy.go), [borrowed driver](../../internal/persistence/ent/borrowed/driver.go) and [reviewed Goose migration collection](../../internal/persistence/migrations/) were introduced within the approved integration scope. Generation uses the same library/tool version. No Atlas migration generator, `atlas.sum`, second production pool, `Schema.Create` or independently committed Ent transaction is used.

`go generate ./internal/persistence/ent` exited 0. The first compile exposed incorrect variadic EntQL composition; the calls were corrected against the local pinned API. `go test ./internal/identity ./internal/authz ./internal/persistence/ent/... ./internal/platform` then exited 0 before the additional driver tests.

`GOPAGER_INTEGRATION=1 go test -count=1 -v ./internal/persistence/ent/borrowed` initially exited 0 for three PostgreSQL cases. An expanded race run subsequently exposed an isolation defect in the new test helper: pgx `ConnString()` preserves its original input and does not serialize edited runtime parameters. This was verified in the pinned pgx source, corrected by encoding `search_path` into the actual input DSN, and guarded by asserting the current schema before migration. The owned disposable container was terminated; no shared database was used. The initial three-case pass is not relied on as isolated-fixture evidence.

After correction, `GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed ./internal/authz ./internal/identity ./internal/platform` exited 0. PostgreSQL 18.0 was provided by the existing Testcontainers fixture with its pinned image and durability settings. The four [driver cases](../../internal/persistence/ent/borrowed/driver_test.go) cover shared account/audit rollback, expired capability rejection, default-deny queries/creates, native filtering before count/page/update, rejection of `privacy.Allow` overrides, inclusive microsecond age boundaries, independent matching field/age union, and restricting-key intersection. Account rows exercise the predicate algebra; this does not certify Message/Unit-appearance aggregation, projection or search.

`go vet ./internal/authz ./internal/identity ./internal/persistence/ent/... ./internal/platform`, `git diff --check` and `git diff --exit-code -- docs/work-packages` all exited 0. Containers were cleaned up by the fixture. The migration collection is exercised by tests but not yet registered in the production root.

### E-04 Detail Field Clarification

Date: 2026-10-26. The user selected `read-fields-plus-detail-action`; [D-55](../authoritative-plan.md#detail-field-authorization-decision) records the approval for [C-06 identity-core 1.1](../../docs/http-api.md#authorization). The active prerequisite binding consumes this narrow addendum; the 1.0 freeze is preserved as history. [Predicate tests](../../internal/persistence/ent/permissions/filter_test.go) verify that a fieldless detail grant permits the additional view only with field-bearing read authority, and that owner/key action omissions fail closed. `go test ./internal/persistence/ent/permissions ./internal/authz` and the two Git diff checks from E-03 exited 0. This is partial F-53 evidence; real Message-context scope/age/projection tests remain necessary at the consumer boundary.

### E-05 Identity Fact Freeze Gap

Date: 2026-10-26. Before implementing state-changing identity services, checked C-06's mandatory same-transaction invalidation facts against the actual C-08 freeze. Transaction-participation 1.0 covers borrowing/atomicity but explicitly leaves the event envelope/schema draft. The [identity-facts candidate](../../docs/architecture.md#domain-events-and-outbox) prepares the minimal AP-07 producer subset and additive integration write scope for explicit approval. No event/job engine or production event table has been implemented.

### E-06 Approved Identity Fact Append

Date: 2026-10-26, same Go/macOS environment as E-01. The user separately approved the C-08 identity-facts 1.0 freeze and additive implementation binding under [D-56](../authoritative-plan.md#identity-fact-append-decision), resolving E-05's freeze gap without replacing that historical finding. Implemented the [typed identity-fact port/envelope](../../internal/events/identity.go), [Ent append adapter](../../internal/persistence/ent/eventstore/append.go) and [Goose fact migration](../../internal/persistence/migrations/00002_identity_facts.sql). C-08 links these as the canonical typed schema; no dispatch/worker or AP-06 event implementation is included.

`GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed ./internal/events ./internal/authz ./internal/identity ./internal/persistence/ent/permissions ./internal/platform` exited 0 with the owned PostgreSQL 18 fixture. [Fact integration tests](../../internal/persistence/ent/borrowed/events_test.go) cover committed persistence, no precommit external publication, invalid fact rejection and storage-error rollback. The shared driver test rolls back account/audit/fact together. This verifies append participation, not yet complete producing commands, command receipts, invalidation consumers or unknown-commit reconciliation.

### E-07 Current Authority Resolution

Date: 2026-10-26. Implemented the [store-neutral Resolver](../../internal/identity/authority.go) and [Ent identity adapter](../../internal/persistence/ent/identitystore/authority.go). The adapter holds the primary IdentityControl row lock across the protected transaction and loads current account, credential, direct/group grants and existing scope references. Non-identity scope owners are not yet composed and fail explicitly; no unrestricted substitute is returned.

After generation and UUID predicate corrections, the command in E-06 exited 0. An expanded `GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed ./internal/identity` also exited 0. [Resolver unit tests](../../internal/identity/authority_test.go) verify exact expiry/absolute/inactivity rejection boundaries, the preceding microsecond, inactive account kinds/states, consumed/revoked credentials, missing key restrictions, missing scope disablement and propagated authority failure without last-use updates. [PostgreSQL authority tests](../../internal/persistence/ent/borrowed/authority_test.go) verify current direct/group union, explicit Guest versus invalid proof, key restriction retention and deleted restricting references. The concurrent case observes the revoker actually blocked in PostgreSQL `pg_stat_activity` while protected work holds the authority lock; after release and committed revocation, subsequent resolution rejects the credential.

The resolver is not a completed login/issuance/delegation/HTTP service. These tests do not certify downstream Message projections, all account-deletion effects, or whole-package acceptance.

### E-08 HTTP Credential Selection and Session Transport

Date: 2026-10-26. The user approved [D-57](../authoritative-plan.md#http-credential-selection-decision), and [C-06](../../docs/http-api.md#authentication) now freezes explicit bearer precedence without Session/Guest fallback for invalid explicit credentials.

Implemented [HTTP transport/policy helpers](../../internal/identity/http.go) and [tests](../../internal/identity/http_test.go) covering explicit bearer selection, duplicate/malformed Authorization rejection, no fallback to Session/Guest, duplicate/malformed Session-cookie rejection, strict same-origin checks, and Session-bound CSRF checks. The fixture uses real HTTP requests and a TLS-terminating reverse proxy forwarding to an HTTP upstream, and verifies that untrusted `Forwarded`/`X-Forwarded-*` headers do not influence origin/credential decisions.

`go test ./internal/identity ./internal/authz ./internal/events ./internal/platform` and the race/integration command from E-06 exited 0. This verifies transport/selection policy only; it does not yet implement full login/Recovery HTTP endpoints or complete AP-07 acceptance.

### E-09 Root Migration Registration

Date: 2026-10-26. The active prerequisite binding permits central migration wiring for produced identity tables/facts. [Root startup](../../cmd/gopager/main.go) now registers [embedded Goose migrations](../../internal/persistence/migrations/migrations.go) instead of starting with `nil` migration FS, so compatibility checks include identity schema/fact artifacts during platform startup.

`go test ./cmd/gopager ./internal/platform ./internal/platform/postgres ./internal/identity` and the race/integration command from E-08 exited 0. This is composition wiring evidence, not full identity-service endpoint evidence.

### E-10 Ordinary Login Clarification and Point Authorization

Date: 2026-10-27. The user rejected unnecessary login complexity and approved `ordinary_login=approve-ordinary-login` under [D-58](../authoritative-plan.md#ordinary-login-decision). C-06 ordinary-login 1.0 now explicitly excludes login Command IDs/receipts, without removing ordinary password/Session/Origin/CSRF controls. The general administrative-command rule remains unchanged.

Implemented [concrete-target authorization](../../internal/authz/decision.go) with [tests](../../internal/authz/decision_test.go) for correlated scope/field/age union, read-plus-detail action, key intersection, Account targets, proven without-Unit state and integer-safe age calculation. Native Ent predicates reuse the same inclusive age cutoff. `go test ./internal/authz ./internal/persistence/ent/permissions ./internal/identity` exited 0.

### E-11 Ordinary Login and Logout Services

Date: 2026-10-27. Implemented the [login/logout application service](../../internal/identity/login.go), [Ent login persistence](../../internal/persistence/ent/identitystore/login.go), [bounded attempt schema](../../internal/persistence/ent/schema/identity.go) and [reviewed Goose migration](../../internal/persistence/migrations/00003_login_attempts.sql). Reuse the selected Ent/Go transaction and hashing libraries, not a new identity framework. The bounded login-attempt table enforces the existing account/instance defaults across processes without an IP-only bucket; configurable lower limits are not yet wired.

The [PostgreSQL login tests](../../internal/persistence/ent/borrowed/login_test.go) verify independent Sessions on repeated ordinary login, Unicode-equivalent usernames, primary resolution, hash-only Session persistence, atomic Session/audit/fact rollback, generic unknown/excluded/suspended rejection, account throttling surviving service reconstruction, exact-minute reset, independent other-account access and the 300-row bound for unknown-account attempts. `GOPAGER_INTEGRATION=1 go test -race -count=1 ./internal/persistence/ent/borrowed ./internal/identity ./internal/authz ./internal/persistence/ent/permissions` exited 0.

Added [actual login/logout HTTP routes](../../internal/identity/handler.go) and wired them through the existing root `StartWithAPI` listener with the explicitly configured public origin. Tests execute login, rejected CSRF/foreign-Origin logout, successful logout/revocation and subsequent normal login over both direct HTTP and HTTPS-edge/HTTP-upstream proxy. They inspect actual Cookie attributes and ensure login JSON omits the password and Cookie bearer secret. No redirect, application TLS or HTTP opt-in was introduced.

### E-12 Local Bootstrap and Product Composition

Date: 2026-10-27. Implemented [local-only bootstrap](../../internal/identity/bootstrap.go), its [Ent adapter](../../internal/persistence/ent/identitystore/bootstrap.go) and [CLI command](../../cmd/gopager/bootstrap.go). Initial actions, fields and delegability are explicit Operator selections; no implicit administrator bypass, default credential or bootstrap HTTP route exists. Password input is bounded stdin pipe/file input, not a command argument or echoed interactive prompt. Startup usage is documented at [C-12](../../docs/operations.md#first-start-and-bootstrap).

[Bootstrap integration tests](../../internal/persistence/ent/borrowed/bootstrap_test.go) prove rollback of the one-time guard on fact failure, successful ordinary login afterward, Guest without grants, no unspecified administrator/delegation rights and exactly one winner for concurrent local initialization. [CLI input tests](../../cmd/gopager/bootstrap_test.go) verify exact password-byte handling. Existing command process tests retain real readiness/signal shutdown and now supply the public origin for API-role startup; earlier evidence is not rewritten.

`GOPAGER_INTEGRATION=1 go test -race -count=1 ./cmd/gopager ./internal/persistence/ent/borrowed ./internal/identity ./internal/authz ./internal/persistence/ent/permissions ./internal/platform` and `go vet ./cmd/gopager ./internal/identity ./internal/authz ./internal/persistence/ent/...` exited 0. Owned PostgreSQL fixtures were cleaned up. Bootstrap/login/logout are now implemented, but account/grant/key/Recovery management, full domain scope routing and the complete prerequisite handover remain incomplete.

### E-13 D-59 Narrowed Prerequisite Handover

Date: 2026-10-27, same Go/macOS and PostgreSQL fixture as E-12. The user's explicit [D-59 approval](../authoritative-plan.md#identity-prerequisite-scope-decision) narrows only the work brought forward; it does not mark the remaining AP-07 tasks complete. The [AP-04 binding addendum](ap-04.md#ap-07-prerequisite-binding-narrowing) preserves the earlier wider approval as history.

Implemented current self-Session inspection for browser reload and configurable positive account/instance login limits. The [real HTTP/PostgreSQL tests](../../internal/persistence/ent/borrowed/login_test.go) cover self-Session/CSRF recovery over direct HTTP and HTTPS-edge/HTTP-upstream, alongside configured lower persistent throttles and the existing default bound. CLI parsing and actual policy values are checked in [root tests](../../cmd/gopager/main_test.go). No new login command identity, implicit admin, remote bootstrap or management/Recovery subsystem was introduced.

`GOPAGER_INTEGRATION=1 go test -race -count=1 ./cmd/gopager ./internal/persistence/ent/borrowed ./internal/identity ./internal/authz ./internal/persistence/ent/permissions ./internal/events ./internal/platform`, `go vet ./cmd/gopager ./internal/identity ./internal/authz ./internal/persistence/ent/...`, `git diff --check` and `git diff --exit-code -- docs/work-packages` exited 0 after the implementation, and again after adding HTTP no-fallback/logged-out assertions. The latter run took 19.110 s for the PostgreSQL borrowed-adapter suite; owned fixtures were cleaned up. This verifies the narrowed reusable core, not the absent Gateway/Network/Unit/Message consumers.

## Blockers and Deviations

- E-01's application HTTP registration blocker is resolved by the separately approved C-12 addendum and E-02 implementation; no second application listener is required.
- D-59 defers general account/profile/grant administration, Recovery/rescue and personal/technical key issuance/rotation. Their command receipts/delegation checks and full personal-data cleanup remain AP-07 work, not blockers requiring those subsystems before AP-06.
- E-05's identity-fact freeze gap is resolved by D-56/E-06; the implemented bootstrap/login/logout producers append their required facts atomically. Absent future consumers are not claimed operational.
- HTTP credential selection, ordinary login/logout, self-Session inspection and root migration/HTTP registration are implemented and tested; E-05/E-09/E-12 retain their earlier observed limits as history.
- Domain scope existence and native query/projection composition require each actual domain owner. Until wired, unknown owner scopes explicitly return `AUTHORITY_UNAVAILABLE`, not an allow-all fallback. AP-08/AP-06 must provide and test those adapters in the same live platform transaction; this does not require full AP-07 management.
- Whole AP-07 acceptance and unavailable personal-data cleanup remain outside the approved prerequisite scope.

## Historical Handover

The D-59 narrowed identity core is available to the staged prerequisite consumers under the approved binding; no whole AP-07 task is accepted. Reuse [current-authority resolution](../../internal/identity/authority.go), [login/Session services](../../internal/identity/login.go), [HTTP transport](../../internal/identity/http.go), [bootstrap](../../internal/identity/bootstrap.go), [point authorization](../../internal/authz/decision.go), [native Ent constraints](../../internal/persistence/ent/permissions/filter.go), [borrowed transaction driver](../../internal/persistence/ent/borrowed/driver.go), [identity persistence](../../internal/persistence/ent/identitystore/) and [fact append](../../internal/persistence/ent/eventstore/append.go). E-02..E-13 own the actual evidence; the owning contracts link the executable DTO/schema/port artifacts.

AP-08 T028/standalone T029, AP-12 T043 and AP-06 T022/T023 must compose their current scope-existence adapters and protected operations/projections, using server-derived contexts in the same platform transaction. Recheck native filters before count/page/mutation and project fields before public output; a successful point check is not permission to return raw Ent entities. Do not invent a Guest/admin/worker fallback for unimplemented scopes, or certify Gateway authentication before the separate C-06 Gateway freeze/implementation. Required consumer contract freezes/execution bindings remain outstanding; the producer core does not authorize them.

### E-14 Resliced Execution Path Reconciliation

Date: 2026-11-03. Under the user's explicit [D-96 approval](../authoritative-plan.md#work-package-reslicing-decision), E-13 and the global-field correction remain the evidence owners for their verified partial identity/AuthZ handovers. The original T025..T027 `in-progress` entries above describe their recorded state at the time and are no longer active work. Only the evidenced core handovers are consumed earlier in the revised path; remaining account, Recovery/key, scope-management, and full deletion work was assigned to historical AP-24, then AP-13, now [AP-14 under D-126](../work-packages/ap-14-identity-lifecycle.md). This navigation update does not accept or complete whole source AP-07 or change earlier evidence.
