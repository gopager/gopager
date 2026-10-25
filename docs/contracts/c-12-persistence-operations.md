# C-12 Persistence and Operations

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-01](c-01-transmission-acceptance.md), [C-08](c-08-events-jobs.md), [C-09](c-09-delivery.md).

**Traceability:** [REQ-111..112/114..120](../requirements-catalog.md#l-operations-data-volume-and-evolution), [REQ-127 and removed REQ-126](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-01/P-03/P-04/P-15/P-20/P-21](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft except the separately frozen [AP-05 foundation boundary](#ap-05-foundation-boundary). **Owner:** Operations/persistence with integration and domain schema owners. **Parties:** Lite/Full app, operator, authoritative store, roles, restart.

**AP-04 delivery amendment (D-32):** This draft retains both target profiles below, but [delivery timing](../authoritative-plan.md#ap-04-scope-decision) requires Full/PostgreSQL first and only architectural preparation for Lite. SQLite implementation, cross-backend runtime evidence, single-process Lite proof and executable Lite-to-Full transfer are deferred. The [architecture](../architecture.md#operating-profiles) owns adapter/composition boundaries. PostgreSQL evidence remains mandatory for the initial slice; no parity claim is made for an absent adapter. Historical AP-03 experiments are unchanged.

Draft interfaces: `ReadOperationalStatus -> safeStatus`; `RunRetention(configuredPolicy) -> progress`; `RequestShutdown -> drainStatus`. Capabilities, not fixed CLI commands/API endpoints. D-31 excludes application Backup/Restore operations entirely.

**Defined:** Shared logical schema, explicit SQLite/PostgreSQL adapters; test critical semantics across backends. Lite is one OS process with a local SQLite file. Full primary is authoritative; established external HA provides fencing/failover. Unclear write authority visibly stops acceptance/Delivery. Replicas/caches do not prove current authorization.

**Retention/storage:** No default deletion without operator limits. Retention unit is a whole Message, aged by stored internal Message time (earliest Observation time); count limits remove oldest whole Messages by `(messageTime, Message ID)`. This is installation-wide and never depends on a User's View time or filters. Delete attached Message contributions, Contribution supports, target resolutions, Transmission evidence/membership, derived processing/search data and linked Submission receipts consistently; do not regroup unaffected Messages or retain dangling references. Alias lookups to deleted Messages return not found. Accepted C-01 exception remains: a Gateway retry after removal of its Submission receipt can create a new Transmission.

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

## Updates and Remaining Evidence

Updates preflight schema/plugin compatibility, block conflicting writes during incompatible migration, and record migration version/checksum/result. Failure leaves explicit not-ready/maintenance state. SQLite and PostgreSQL share domain semantics, not necessarily identical SQL. No automatic binary downgrade after irreversible schema/data change; recovery from an external backup is the Operator's responsibility. Plugin artifact rollback follows C-10 and does not roll back provider effects.

AP-03 owns exact drivers/runtime versions, schema/index/constraint and isolation experiments, migration feasibility, fsync/crash evidence, Full primary fencing/failover and the available reference host. AP-05/AP-15 implement approved mechanics later. No arbitrary SLA/RPO/RTO, disk threshold, automatic acknowledged-loss policy or successful failover test is inferred from this documentary contract.
