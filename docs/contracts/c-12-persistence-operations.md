# C-12 Persistence and Operations

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-01](c-01-transmission-acceptance.md), [C-08](c-08-events-jobs.md), [C-09](c-09-delivery.md).

**Traceability:** [REQ-111..112/114..120](../requirements-catalog.md#l-operations-data-volume-and-evolution), [REQ-127 and removed REQ-126](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-01/P-03/P-04/P-15/P-20/P-21](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft except the separately frozen [AP-05 foundation boundary](#ap-05-foundation-boundary). **Owner:** Operations/persistence with integration and domain schema owners. **Parties:** Lite/Full app, operator, authoritative store, roles, restart.

**AP-04 delivery amendment (D-32), topology clarified by D-63:** This draft retains both target profiles below, but [delivery timing](../authoritative-plan.md#ap-04-scope-decision) requires Full/PostgreSQL first and only architectural preparation for Lite. SQLite implementation, cross-backend runtime evidence, single-entry bundled Lite proof and executable Lite-to-Full transfer are deferred. The [architecture](../architecture.md#operating-profiles) owns adapter/composition boundaries. PostgreSQL evidence remains mandatory for the initial slice; no parity claim is made for an absent adapter. Historical AP-03 experiments are unchanged.

Draft interfaces: `ReadOperationalStatus -> safeStatus`; `RunRetention(configuredPolicy) -> progress`; `RequestShutdown -> drainStatus`. Capabilities, not fixed CLI commands/API endpoints. D-31 excludes application Backup/Restore operations entirely.

**Defined:** Shared logical schema, explicit SQLite/PostgreSQL adapters; test critical semantics across backends. Lite has a local SQLite file and the single-start bundled application boundary in REQ-115, not an exact OS-process-count requirement. Full primary is authoritative; established external HA provides fencing/failover. Unclear write authority visibly stops acceptance/Delivery. Replicas/caches do not prove current authorization.

## Managed Child Process Clarification

**D-63; version/status:** operations-topology clarification 1.0, approved 2026-11-01 for the previously draft topology; existing frozen foundation interfaces are unchanged. The Operator starts only the application. Core may launch/manage child processes provided all required executables/runtimes/libraries ship with the application; it must not require separate module startup, orchestration, compiler or runtime-package installation. This is not permission for automatic dependency/code downloads at startup.

Operations/integration owns child lifecycle and bounded IPC, resource enforcement, cancellation, termination and wait/reap, parent shutdown/failure behavior and safe diagnostics. A timeout around a still-running child, an unbounded memory allocation, or a child lost during shutdown is not validated bounded computation. Platform-specific mechanisms must be selected and tested before claiming portability or containment. Configuration must expose required capabilities/failures, not silently fall back to unrestricted in-process execution.

Historical AP-03/AP-05 results and their read-only baselines remain unchanged; their “single-process Lite” wording records the old assumption, not a current exact-count acceptance rule. AP-15 T052 now uses REQ-115/D-63 through the separately approved 2026-11-01 narrow baseline amendment; its Lite timing/remaining acceptance is unchanged and AP-15 has not started. This correction does not deliver Lite/SQLite now, select a regex runtime, or remove the already specified external Full/PostgreSQL infrastructure boundary.

**Retention/storage:** No default deletion without operator limits. Retention unit is a whole Message, aged by stored internal Message time (earliest Observation time); count limits remove oldest whole Messages by `(messageTime, Message ID)`. This is installation-wide and never depends on a User's View time or filters. Delete attached Message contributions, Contribution supports, target resolutions, Transmission evidence/membership, derived processing/search data and linked Submission receipts consistently; do not regroup unaffected Messages or retain dangling references. Lookups of deleted or invalidated Message references return not found; no alias mapping is required under D-40. Accepted C-01 exception remains: a Gateway retry after removal of its Submission receipt can create a new Transmission.

This preserves the earlier explicit whole-Message/first-time retention choice: a continuously extended old Message can be removed even with recent receptions, and an older late member/merge may move it past the cutoff. A new Unit appearance does not extend retention. Expose this policy clearly to the Operator; no implicit switch to last-reception retention or per-target deletion. Storage exhaustion remains visible, never triggers silent emergency deletion.

Prepared self-contained Delivery input can finish under C-09's bounded lifetime and current authorization checks after Message deletion. Jobs requiring deleted data end `dependency-unavailable`; no automatic cancellation solely because a fully materialized job's original Message disappeared. Minimal dedup/uncertainty receipts follow C-08. Retained original evidence is immutable; policy-compliant deletion is distinct. Gateway diagnostics distinguish removed history from never received.

**Acceptance/recovery:** Crash loss of acknowledged Lite inputs is conditionally allowed, not a goal/budget. State commit uncertainty and ordinary restart of pending jobs/attempts honestly. Lite->Full preserves domain identities, accounts/permissions and history, not a domain rebuild. Manual Backup/Restore is entirely Operator-owned under D-31; the application cannot know later state missing from an older database copy.

Initial setup without default credentials, status checks, orderly shutdown, traceable updates are required. Drain/cancellation respects jobs and provider uncertainty. Update is no automatic rollback of intervening data. No quantitative SLA/RPO/RTO/hardware promises. Operator-facing product text uses English by default; machine-readable operational codes remain language-independent.

## Storage Failure and Operational Status

`ReadOperationalStatus` exposes liveness without private data and authorized readiness details: profile/store role, write/access authority, migration state, pending/failed correction/Processing and safe storage/secret/plugin failure codes. Liveness is not write readiness. A read replica/cache cannot substitute for current grants or serve private responses during authority loss.

On failed allocation/write/commit (including disk full/read-only store), roll back where known and return `STORAGE_UNAVAILABLE`; a possible commit is `COMMIT_UNCERTAIN` with caller-owned receipt lookup, never a definite rejection. C-01 acknowledges only its recorded acceptance stage, not memory queueing, completed Processing or provider Delivery. Primary/lease authority uncertainty is `AUTHORITY_UNAVAILABLE`: stop new acceptance writes, private output and Delivery starts; retain pending work for checked recovery. Already started I/O remains potentially uncertain.

Do not delete data or relax durability/permissions to recover capacity. Emit redacted operational status/logging using an available channel; inability to persist a failure event is itself exposed through status/logs, not claimed as a committed outbox fact. Operator capacity alerts may warn before exhaustion but no unapproved percentage threshold or capacity guarantee is specified. Recovery checks the store, authority and current revisions before readiness; expired attempts/leases never become fresh unattempted work.

Shutdown first removes write readiness and rejects new starts, then cancels/drains accepted work within its existing bounded deadlines. Persist jobs/results while authority remains; unfinished internal work resumes idempotently and started external work follows C-09 uncertainty. No promise that termination always finishes a provider effect. Initial setup is local Operator-controlled, one-time and without shipped/default credentials; completion disables the bootstrap path. Never expose an unauthenticated remote first-admin race.

## Operator Backup Boundary and Profile Transition

Backup and Restore are manual Operator responsibilities, not application features. No backup API/command, manifest/export engine, restore detector/hold, restore-time credential invalidation, reconciliation workflow or product restore gate is required. Operators choose tooling, consistency, Secret/key/artifact handling, retention and disposal. Application updates do not promise automatic rollback of intervening data.

An older copy may reintroduce revoked credentials, deleted accounts or work whose external effect happened later. The application has no knowledge of that missing history and promises no automatic prevention/reconciliation. Operators must account for those risks externally before starting the restored application. Normal startup still enforces schema compatibility, current stored rights, job deadlines/fencing and honest uncertainty for recorded started attempts; this is ordinary consistency, not special restore logic.

Lite-to-Full remains a separate required profile transition: stop source writes, transfer a consistent domain state preserving IDs, original evidence/times, accounts/grants, configuration, contribution/result context, jobs and dedup identities, and provision the Secret/artifact dependencies externally. Validate backend parity and target references/counts; rebuild only derived indexes/projections as needed and activate exactly one target write authority. A concrete transition mechanism belongs to AP-15, not a general Backup/Restore subsystem. Do not run two independent writable copies.

## AP-05 Foundation Boundary

**Version/status:** foundation 1.0, frozen for AP-05 under D-35 on 2026-10-26; the rest of C-12 remains 0.1-draft. **Owner:** persistence/operations, integration for registration. **Producers/consumers:** Operator supplies validated startup configuration; composition starts platform API/worker roles; later domain repositories join transactions/register migrations; local status/test consumers inspect safe outcomes. Uses [shared foundation conventions](shared-conventions.md#ap-05-foundation-conventions) and glossary G-1.0. Planned cases: [F-46..F-46f](reference-cases.md#ap-05-foundation-cases).

**Subsequent migration-only supersession:** The original foundation 1.0 text below is retained as historical freeze evidence. Its migration registration/ledger/checksum rules are replaced by [foundation 1.1](#ap-05-foundation-migration-addendum) under D-37; all other foundation interfaces remain unchanged.

### Configuration and Role Startup

`StartPlatform(context, config, registrations) -> runningHandle | codedFailure`. Config contains `profile: full`, nonempty unique `roles` from `api`, `worker`, `migrationMode: check | apply`, a Secret database connection string and an explicit HTTP listen address when `api` is selected. Default migration mode is `check`; profile, roles and connection must be explicitly supplied. Unknown fields/roles, empty/duplicate role sets, unavailable `lite`, or a missing API listen address reject startup with `INVALID_INPUT`; never silently start another profile/role. Parse/bind flags/environment in composition, not domain code; no fixed CLI spelling is frozen. The connection string is never returned/logged. A non-loopback/nonlocal database connection requires certificate-verified TLS; an intentionally insecure private-infrastructure connection requires explicit Operator policy and a redacted warning, never silent downgrade.

`api` binds platform status handlers; `worker` starts registered bounded internal runners. Empty domain registrations are valid for a foundation smoke check, not a claim of a working ingest/Delivery product. API and worker may compose in one process using the same interfaces or run in separate Full processes. No mandatory broker/IPC or separate domain model is introduced for later Lite.

`RunningHandle` provides `Shutdown(context) -> completed | codedFailure` and safe status, with no SQL handle or private configuration in its public result. Startup checks connection, primary write capability and migration compatibility before write readiness. Worker-only composition needs no HTTP listener; its status is available through the same in-process status interface. A read-only/recovery server cannot become writable primary by configuration fiction. Full HA/failover remains external and unvalidated.

### Transaction and Repository Participation

`WithinTransaction(context, callback(TransactionContext) -> error) -> committed | codedFailure`. `TransactionContext` is an opaque, store-neutral participation capability valid only during this callback. Infrastructure repository adapters resolve it to the owning SQL transaction; domain/application code sees neither `*sql.Tx` nor pgx types. A callback must return only after its synchronous store work finishes; no nested independent transaction, leaked handle, background goroutine using it, external provider I/O or cross-store enlistment is permitted.

Use PostgreSQL `READ COMMITTED`, short transactions and explicit database uniqueness/row locks/CAS for invariants. This does not claim that isolation alone serializes domain Grouping. C-01 and C-08 own their participation obligations. Domain state plus required outbox work use the same capability/commit; any callback/constraint failure rolls back the whole unit. Default ordinary-call deadline remains the shared 30 s bound. The boundary performs no automatic callback replay; operation owners may retry a proven rolled-back conflict using their existing identity and bounded policy.

Successful commit returns `committed`; a known rollback preserves the appropriate safe validation/conflict/storage code. Network/cancellation/driver failure after commit may have been sent returns `COMMIT_UNCERTAIN`, not `committed` or a definite rollback. Resolve through the operation owner's receipt, never fabricate absence. Cleanup rollback errors are surfaced in redacted diagnostics without replacing an already uncertain outcome. Infrastructure adapters, not application consumers, own SQL error classification.

### Migration Registration and Compatibility

Integration registers an ordered list of migrations with positive signed-64-bit `version`, nonempty owning module ID and nonempty embedded UTF-8 SQL bytes. Versions are unique and strictly increasing; checksum is lowercase hexadecimal SHA-256 of those exact bytes. Domain owners own their SQL; integration owns ordering/registration. No inferred production domain tables or SQL from AP-03 probes is included.

Platform ledger `platform_schema_migrations` has `version BIGINT PRIMARY KEY CHECK (version > 0)`, `owner TEXT NOT NULL`, `checksum TEXT NOT NULL` (64 lowercase hexadecimal characters), and `applied_at TIMESTAMPTZ NOT NULL`. Bootstrap of this ledger is platform-owned and serialized with migration application. Acquire a session-level PostgreSQL advisory lock with signed-64-bit key `0x47504D4947520001` on one dedicated connection; acquisition respects context. Hold it across compatibility checking and application and release it/close the dedicated connection on all exit paths. No global application traffic lock is implied by that startup lock.

The applied ledger must be an exact prefix of the registered list, including owner/checksum. A changed/unknown applied version, gap, or newer database yields `INCOMPATIBLE_VERSION` and no readiness; never repair or skip silently. `check` requires every registered migration applied and performs no schema changes; a missing ledger/missing suffix is `pending`, not `current`, and startup fails `INCOMPATIBLE_VERSION` until an authorized apply. Only `apply` may bootstrap the ledger, transactionally under the same advisory lock, then run the missing suffix in order, each SQL migration and ledger insert in one transaction; failure rolls back that migration and leaves a visible not-ready state. Earlier successfully applied versions remain recorded; no automatic down migration or data rollback. SQL needing execution outside a transaction is unsupported by this boundary and fails visibly.

Operators must stop/drain older writers before applying incompatible migrations. The startup lock serializes migration runners, not already running application transactions. Rolling compatibility and zero-downtime migration are not promised. No live process gains permission to continue with an incompatible schema.

### Status, Authority and Shutdown

`ReadPlatformStatus() -> {live: boolean, ready: boolean, profile, roles, migrationState, failureCode?}`; `migrationState` is `unchecked | current | pending | incompatible | failed`. The API exposes `GET /health/live` (200 when process responds, otherwise unavailable) and `GET /health/ready` (200 when ready, 503 otherwise), returning only `{"live": boolean}` or `{"ready": boolean}` respectively. Detailed status is available only in-process/Operator diagnostics until a later authorized management API is frozen. Neither endpoint reveals connection strings, database hosts, schema versions, plugin data or accounts. No remote bootstrap route is provided.

Readiness becomes false on migration incompatibility, store/authority failure or shutdown; new platform work is refused with the existing storage/authority code. Recovery rechecks connectivity, write authority and schema before readiness returns; a successful ping alone is insufficient. Each later protected operation still checks its own current authority; a health probe is not an authorization cache.

Shutdown first clears readiness/stops new starts, then cancels/drains registered work under a 30 s default total shutdown deadline, closes the HTTP listener and store handles, and returns completion or a visible coded failure/nonzero process exit. No discarded error or success claim after deadline. Cancellation/uncertain commits preserve their prior classification. Actual Delivery starts/retention/bootstrap are later domain consumers, not placeholder implementations in AP-05.

## Application HTTP Transport

**D-52; status:** approved application-wide behavior, pending AP-04 owner-contract review/freeze. **Owner:** Operations/persistence with integration and domain API owners. **Consumers:** every GoPager application API and browser-facing route.

All application functions must be available through the configured HTTP listener, including protected administrative, authentication, Session, Gateway, query and live routes. GoPager does not require application-level TLS, redirect HTTP to HTTPS or require a separate HTTP setting. A reverse proxy may terminate external TLS and forward requests to GoPager over HTTP. Direct HTTP access also remains supported; it provides no confidentiality, server authentication or response integrity. Operators are responsible for protecting any externally exposed edge and for the proxy-to-application network path. Authentication and Session semantics over HTTP are owned by [C-06](c-06-identity-authorization.md#http-transport-and-human-authentication).

### Application Handler Registration Addendum

**Version/status:** api-registration 1.0, approved 2026-10-26 by the user (`platform_api_registration=approve-additive-registration`). **Owner:** Operations/integration. **Producer:** root application composition; **consumers:** AP-07 and later domain HTTP adapters.

`StartWithAPI(context, config, migrations, store, runners, api http.Handler)` adds application routes on the same configured API listener. Existing `Start` remains the no-application entry point with unchanged behavior. Non-nil API registration requires the API role. Health routes retain priority and their exact boolean-only response/method rules. Domain routes are refused with C-07 `AUTHORITY_UNAVAILABLE` while not ready, stopping or cancelled; admitted requests join shutdown drain accounting. Domain handlers still enforce current authorization and bounded request/projection work. No second pool/listener, application TLS or redirect is introduced; transaction, migration and durability interfaces remain unchanged.

## Local Identity Initialization and Startup

Implementation usage for the approved AP-07 prerequisite is owned by [root composition](../../cmd/gopager/main.go) and the [local bootstrap command](../../cmd/gopager/bootstrap.go). API-role startup now requires `--public-origin` as the deployment origin used by [C-06 Cookie/Origin policy](c-06-identity-authorization.md#http-transport-and-human-authentication); it is not an HTTP opt-in. For example:

```sh
gopager --profile=full --roles=api,worker --listen-address=127.0.0.1:8080 --public-origin=http://localhost:8080 --migration-mode=apply
```

For an HTTPS-terminating reverse proxy, set `--public-origin=https://pager.example` while retaining the HTTP listen address. Worker-only startup does not need a public origin. `GOPAGER_DATABASE_URL` remains the secret connection input; database TLS policy is separate from application HTTP.

`--login-attempts-per-account` and `--login-attempts-per-instance` configure the existing [C-06 per-minute login limits](c-06-identity-authorization.md#ap-02-credentials-and-recovery); both must be positive integers. The [login policy](../../internal/identity/login.go) owns executable defaults, and the [persistence adapter](../../internal/persistence/ent/identitystore/login.go) shares attempt accounting across API processes. Lower limits do not change password/Session semantics; raised limits are an explicit Operator capacity choice, not a capacity claim.

On a fresh installation, initialize the first account locally before login:

```sh
gopager bootstrap --username=operator --actions=password.local,session.manage-own --migration-mode=apply < /protected/path/initial-password
```

The password comes from a pipe/file on stdin, not an argument or echoed terminal prompt. One optional final LF is the input delimiter; other password bytes are preserved. The command prints only the created Principal UUID and exits without opening a listener. The example grants only login/own-session management, not administrative domain rights. Operators must explicitly supply the required catalog actions for their initial administrator; `--fields` selects readable field sets and `--delegable` explicitly enables delegation of the supplied rights. There is no wildcard, implicit administrator role, shipped password or remote bootstrap route. Guest starts without grants. Bootstrap rejects existing accounts/completed initialization and commits its guard, accounts, selected grants, audit and required fact together. Runtime/acceptance limitations remain in [AP-07 evidence](../execution/source-ap-07.md).

## Updates and Remaining Evidence

Updates preflight schema/plugin compatibility, block conflicting writes during incompatible migration, and record migration version/result under the [current migration binding](#ap-05-foundation-migration-addendum), without content checksums. Failure leaves explicit not-ready/maintenance state. SQLite and PostgreSQL share domain semantics, not necessarily identical SQL. No automatic binary downgrade after irreversible schema/data change; recovery from an external backup is the Operator's responsibility. Plugin artifact rollback follows C-10 and does not roll back provider effects.

AP-03 owns exact drivers/runtime versions, schema/index/constraint and isolation experiments, migration feasibility, fsync/crash evidence, Full primary fencing/failover and the available reference host. AP-05/AP-15 implement approved mechanics later. No arbitrary SLA/RPO/RTO, disk threshold, automatic acknowledged-loss policy or successful failover test is inferred from this documentary contract.

## AP-05 Foundation Migration Addendum

**Version/status:** foundation 1.1, approved 2026-10-26 under [D-37](../authoritative-plan.md#ap-05-migration-revision-decision); implementation/conformance evidence is owned by [AP-05](../execution/ap-05.md#e-14-goose-foundation-implementation). **Owner/parties:** unchanged from foundation 1.0. **Supersedes:** only the migration-registration/compatibility section of foundation 1.0 and the obsolete checksum reference in update guidance. Other configuration, transaction, error, status and shutdown rules remain frozen as before. Cases: [F-46a/F-46b addendum](reference-cases.md#ap-05-migration-case-addendum).

Integration owns one central embedded migration collection and global ordering, with positive unique signed-64-bit versions in increasing order. Numeric version intervals need not be contiguous. Domain owners still author/review changes to their objects; a separate human-only module-owner field is not required. Cross-module prerequisites are expressed by putting the required change earlier in the same global sequence, including alternating changes across domains. There are no independently advancing module histories or custom dependency-graph engine.

Use the approved Goose runner and its PostgreSQL version store. The platform-owned tracking table keeps the name `platform_schema_migrations`, with the library's PostgreSQL shape: identity primary key `id`, `version_id BIGINT NOT NULL`, `is_applied BOOLEAN NOT NULL`, and `tstamp TIMESTAMP NOT NULL DEFAULT now()`. Goose's version-zero initialization record is bookkeeping, not an application migration. Application history contains one successful applied record per registered version. Do not add custom owner or checksum columns or validate content hashes.

The registered collection is the authority for allowed versions; the applied application versions must be its exact prefix. Reject duplicate/unknown applied versions, applied holes, negative/non-applied unexpected history or a newer database with `INCOMPATIBLE_VERSION`. Missing ledger or unapplied suffix is `pending`. An old custom checksum/owner ledger is not silently treated as the new format or converted; incompatible shape is a visible incompatibility. No installed production database is presumed by this foundation change.

Only `apply` bootstraps the library history and runs missing migrations; `check` makes no schema or history writes, including when the ledger is absent. Serialize checking/application with a dedicated PostgreSQL session advisory lock using the existing key `0x47504D4947520001`, with bounded context cancellation and release/physical-session cleanup on every exit path. Reuse the library session locker rather than recreating lock acquisition/retry logic. Preserve the 30 s ordinary-call/shutdown bounds; library defaults must not silently extend them.

Embedded SQL may perform schema changes and data changes. Each migration and its successful applied record commit atomically; a failure rolls both back, preserves previous successful versions and leaves a visible not-ready/failure result. Library SQL parsing/execution owns statement handling. Nontransactional migrations, uncontrolled environment substitution and automatic down migrations are unsupported; reject rather than degrade guarantees. Transactional Go callbacks may handle necessary transformations not reasonably expressed in SQL, without using unstable current domain-service behavior for historical migrations.

Under the user's release-generation clarification in D-37, development-time generation is allowed: generated SQL remains manually editable and must be reviewed and fixed in the release's embedded central collection before publication. Deployment executes those release artifacts; it does not inspect the live schema to generate a new diff/migration plan, run ORM schema synchronization, or otherwise synthesize migration contents at startup. A generator is not itself selected by this permission.

Responsible historical content corrections are allowed and trusted. Contents are not compared with prior bytes, and already-applied versions do not run again merely because their contents changed. If an existing database needs repair, add an explicit corrective migration; editing old SQL alone does not repair it. Keep primary-authority/readiness checks, stop/drain requirements for incompatible updates and no rolling-compatibility promise from foundation 1.0. Plugin migration execution and the future SQLite adapter remain outside this approval.

**Executable binding:** [Store/Start](../../internal/platform/status.go) accepts the central release collection as standard-library `fs.FS`; production composition supplies an immutable embedded filesystem, not an Operator directory. SQL files are flat, named `<positive-version>_<description>.sql`, UTF-8 and use Goose's `Up`/statement annotations. [The adapter](../../internal/platform/postgres/migrations.go) owns library configuration and strict history checks; [the manifest](../../go.mod) owns the approved library pin. `nil` denotes an empty registration, not permission to skip ledger compatibility. The foundation command intentionally uses that empty registration until domain owners supply real release migrations; no placeholder domain table is installed. This binding replaces the old owner/checksum registration fields under D-37 and introduces no domain SQL or runtime generator.
