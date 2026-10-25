# C-12 Persistence and Operations

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-01](c-01-transmission-acceptance.md), [C-08](c-08-events-jobs.md), [C-09](c-09-delivery.md).

**Traceability:** [REQ-111..112/114..120](../requirements-catalog.md#l-operations-data-volume-and-evolution), [REQ-127 and removed REQ-126](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-01/P-03/P-04/P-15/P-20/P-21](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft. **Owner:** Operations/persistence with integration and domain schema owners. **Parties:** Lite/Full app, operator, authoritative store, roles, restart.

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

## Updates and Remaining Evidence

Updates preflight schema/plugin compatibility, block conflicting writes during incompatible migration, and record migration version/checksum/result. Failure leaves explicit not-ready/maintenance state. SQLite and PostgreSQL share domain semantics, not necessarily identical SQL. No automatic binary downgrade after irreversible schema/data change; recovery from an external backup is the Operator's responsibility. Plugin artifact rollback follows C-10 and does not roll back provider effects.

AP-03 owns exact drivers/runtime versions, schema/index/constraint and isolation experiments, migration feasibility, fsync/crash evidence, Full primary fencing/failover and the available reference host. AP-05/AP-15 implement approved mechanics later. No arbitrary SLA/RPO/RTO, disk threshold, automatic acknowledged-loss policy or successful failover test is inferred from this documentary contract.
