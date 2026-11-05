# Architecture

## System overview

GoPager receives decoded pager transmissions (POCSAG, FLEX, ZVEI, EAS) from authenticated gateways and stores each one as immutable evidence. Transmissions with the same payload identity that arrive within a grouping window are combined into one Message. Each transmission target (address/function) is resolved to a Unit through temporal address rules; rule, Unit and suppression changes are applied to stored history by a correction worker. Workflows evaluate transmissions in a worker process, can call operator-installed plugins, and gate the publication of new evidence until their results are ready.

GoPager is a single Go binary, `gopager` ([cmd/gopager](../cmd/gopager/)), backed by PostgreSQL. The `--roles` flag selects which roles a process runs:

| Role | Runs | Started from |
| --- | --- | --- |
| `api` | HTTP listener: gateway intake, login/session, management routes, health endpoints | `composeApplicationAPI` in [main.go](../cmd/gopager/main.go) |
| `worker` | Processing dispatcher (workflow candidates) and correction runner (rematching, publication re-entry) | `composeProcessingWorker`, `composeCorrectionRunner` |
| `bootstrap` subcommand | One-shot local creation of the first account; no listener | [bootstrap.go](../cmd/gopager/bootstrap.go) |

One process can run both roles (`--roles api,worker`), or the roles can run in separate processes against the same database. Role processes coordinate only through PostgreSQL (row locks, leases, fencing tokens). They do not talk to each other directly.

```mermaid
flowchart LR
    GW[Gateways] -- "Bearer credential<br/>POST /api/v1/transmissions" --> API
    Browser[Browser / API clients] -- "Session cookie or access key" --> API
    subgraph gopager
        API[api role]
        Worker[worker role]
    end
    API <--> PG[(PostgreSQL)]
    Worker <--> PG
    Worker -- "go-plugin / gRPC" --> Plugins[Plugin processes]
    API -- "go-plugin / gRPC (preview, instance inspection)" --> Plugins
```

| External actor | Interaction |
| --- | --- |
| Gateway | Submits transmissions and state reports with a per-gateway bearer credential. See [http-api.md](http-api.md). |
| Browser / API client | Authenticates with a session cookie (human accounts) or an access key, or acts as the guest account when anonymous. Manages master data, rules, gateways, workflows and instances. |
| Plugin | Operator-provisioned executable started as a managed child process and called over gRPC. See [plugins.md](plugins.md). |
| PostgreSQL | The only store: domain state, domain events, outbox, work queues, migration ledger, and the clock used for domain time. See [operations.md](operations.md). |

## End-to-end data flow

```mermaid
flowchart TD
    A[Gateway submission] --> B[Authenticate gateway, check idempotency]
    B --> C[Validate payload and targets per protocol]
    C --> D[Resolve Network]
    D --> E[Match targets to Units]
    E --> F[Group into Message]
    F --> G[Store transmission, targets, contributions, facts]
    G --> H{Workflows in scope?}
    H -- no --> P[Mark ready / publish]
    H -- yes --> I[Insert processing candidates]
    I --> J[Worker: claim, evaluate, call plugins]
    J --> K[Store result]
    K --> L{All selected workflows done?}
    L -- yes --> P
    R[Rule / Unit / suppression change] --> S[Pending correction scope]
    S --> T[Correction runner: rematch, rebuild contributions]
    T --> P
```

### Intake (one transaction)

`POST /api/v1/transmissions` runs as a single primary transaction. If any step fails, nothing is stored. The only exception is a separate transaction that records a rejected submission (`operation.submission-rejected`).

| Step | What happens | Package |
| --- | --- | --- |
| HTTP decoding | Bearer extraction, `application/json`, size limit, JSON decoding | `cmd/gopager` ([ingest.go](../cmd/gopager/ingest.go)) |
| Gateway authentication | The bearer secret hash identifies exactly one gateway. A pending replacement credential takes over on first use. | `identity.GatewayAuthenticator`, `ingeststore` |
| Validation | Submission shape, observation time, payload form per protocol, target address/function ranges | `ingest` (`Submission.Validate`), `protocols` |
| Submission idempotency | `(gateway, submissionId)` plus an input fingerprint. A matching retry returns the original receipt; a different input is rejected. | `ingest`, `ingeststore` |
| Payload identity | Each payload form maps to a comparison-only identity (form, subtype, normalized value). The submitted payload is stored unchanged. | `protocols.TransmissionPayload.Identity` |
| Network resolution | In order: declared gateway network identifier (`declared`), the gateway's only network for the protocol (`inferred-single`), the only network whose rules match the targets (`inferred-address`), the gateway default (`default`). If the address match is ambiguous, the default is used and `operation.network-ambiguous` is emitted. | `ingest.ResolveNetwork`, `assignments.Service.NetworksWithMatches` |
| Target matching | Each target position is resolved against the current address rules at its observation time: `assigned` (Unit plus rule) or `unassigned`. Overlapping rules resolve to the most specific selector. Ambiguity is a conflict. | `assignments.Resolve`, `assignments.Service.MatchTargets` |
| Grouping | Find Messages with equal payload identity that have a member within the grouping window (`grouping_control.window_seconds`). None found: create a Message. One found: join it. Several found: merge them into the lowest ID and move their transmissions and contributions. | `ingeststore.groupMessage` (decision type from `messages`) |
| Persistence | Transmission, ordered targets with their resolution, Message contributions (one per Message and Unit, or one Unit-less row for targetless input), contribution supports | `ingeststore.AcceptTransmission` |
| Correction capture | If a correction for the current assignment revision is queued or running, the transmission is recorded in `correction_dirty_transmissions` | `ingeststore.captureCorrectionDirty` |
| Facts | `gateway.state-changed`, `message.created` / `message.transmission-joined` / `message.merged`, `assignment.changed` (reason `created`) per target, `operation.observation-time-implausible` | `events`, `eventstore` |
| Processing admission | Select enabled workflows with an active version and scope `all`, the matching Network, or one of the matched Units. Insert one `processing_candidates` row per selected workflow. If none are selected, the transmission is marked ready immediately. | `ingeststore.admitProcessing` |

The response is `200` when processing is already complete and `202` when workflows are pending.

### Processing (worker role)

| Step | What happens | Package |
| --- | --- | --- |
| Due selection | Pages through due candidates with bounded concurrency, a poll interval and wake-ups | `processing/worker.Dispatcher` |
| Reconcile | Supersede candidates whose workflow, version, source or assignment context changed. Admit current bindings. Recover expired leases. | `ingeststore.ProcessingSourceOwner.ReconcileCandidate` |
| Capture and claim | In one transaction: capture a detached source projection (payload fields, targets, Units, Network) and claim the candidate (lease, fencing token, worker ID) | `worker.Service`, `ProcessingSourceOwner.CaptureCandidate`, `processingstore` |
| Evaluate | Outside any SQL transaction: compile the definition (CEL predicates, Core regex extraction, map, branch, installed plugin steps) and evaluate within a compute budget | `processing` (`WorkflowProgram.EvaluateScoped`, `PluginHost`) |
| Complete or fail | Recheck source and authority, then write a `processing_results` row or a classified failure with automatic retry scheduling (at most 5 attempts) | `worker.Service.finish`, `processingstore` |
| Publish | Once every selected candidate of the transmission is finished: apply results, emit `processing.completed`, and set transmission, Message and contribution readiness atomically | `processing/publication.Service`, `ProcessingSourceOwner.ApplyReadiness` |

Workflow definitions, versions, activation and processing instances are described in [workflows.md](workflows.md). The plugin protocol is described in [plugins.md](plugins.md).

### Publication barrier

New evidence counts as published only after processing. The flags below hold the publication state.

| Record | Field | Meaning |
| --- | --- | --- |
| `transmissions` | `processing_status` (`pending`/`completed`/`failed`), `publication_status` (`pending`/`ready`/`failed`) | Whether the selected workflows finished and the transmission is published |
| `messages` | `publication_status` | `ready` when all member transmissions are ready. Reset to `pending` when a transmission joins. |
| `message_contributions` | `ready` | Contribution visible to readers |
| `processing_results` | `published_at`, `message_id` | Result applied by publication |

### Historical correction (worker role)

Creating, updating or deleting a rule, deleting a Unit, or changing a suppression does three things in the same transaction. It increments `assignment_control.revision`, stores an `assignment_pending_scopes` row (reason `rule-change`, `unit-deleted` or `suppression-change`, affected Networks/Unit), and returns that scope's status to the caller.

`assignments.CorrectionRunner` then processes the queue:

1. It claims the oldest due scope with a lease and fencing token (`assignmentstore.ClaimCorrection`).
2. It reads affected transmissions in batches of 100, ordered by `(accepted_at, id)` up to a high-water mark, plus any dirty transmissions. Each batch runs `MatchTargets` again at the original observation time.
3. `ApplyCorrectionBatch` updates changed target resolutions and rebuilds the Message's contributions and supports. Suppressed targets are excluded. Each change emits `assignment.changed` (reason `corrected`).
4. On failure, the scope is rescheduled with backoff and a failure code. It can be retried through `POST /api/v1/assignment-corrections/{id}/retry`.
5. When idle, the runner re-enters publication for unpublished transmissions that have no unfinished candidate and no overlapping correction (`ProcessingSourceOwner.ReenterPublication`).

Correction never creates or deletes transmissions or Messages and never changes grouping.

## Package map

### Dependency layering

Derived from `go list -f '{{.ImportPath}} {{.Imports}}' ./...` (internal imports only; test-only packages and transitively implied edges omitted):

```mermaid
flowchart BT
    platform
    authz
    protocols
    pluginsdk
    postgres[platform/postgres] --> platform
    commands --> authz & platform
    messages --> protocols
    processing --> authz & commands & platform & protocols & pluginsdk
    events --> platform & processing
    identity --> authz & events & platform
    units[Units] --> identity & commands & events
    assignments --> units & identity & protocols
    ingest --> assignments & identity & protocols
    pexec[processing/execution] --> identity & authz
    pworker[processing/worker] --> processing & identity
    ppub[processing/publication] --> processing & events
    pmgmt[processing/management] --> processing & pexec & identity & events
    psyn[processing/synthetic] --> pmgmt & assignments & units
    stores[persistence/ent/*store] --> postgres & ingest & assignments & units & pworker & ppub & pmgmt
    cmd[cmd/gopager] --> stores & ingest & pmgmt & psyn
```

The layering rules that the code follows:

- Domain and application packages (`authz`, `protocols`, `messages`, `commands`, `events`, `identity`, `Units`, `assignments`, `ingest`, `processing/...`) never import `platform/postgres`, Ent, or any store package. They declare small consumer interfaces (`Repository`, `Clock`, `Transactions`, owner ports) and receive `platform.TransactionContext`.
- `authz`, `protocols`, `platform` and `pluginsdk` are leaves with no internal imports.
- Store adapters under `internal/persistence/ent/` implement those interfaces and are the only importers of `platform/postgres` besides `cmd/gopager`.
- `cmd/gopager` is the composition root. It constructs stores, wires them into services, and registers HTTP routes.
- `events` imports `processing` to validate `processing.completed` result payloads, so `processing` cannot import `events`.
- The generated Ent package and its `schema` import domain types (`Units`, `assignments`, `authz`, `commands`, `processing`) for typed JSON columns.

### Packages

| Package | Responsibility |
| --- | --- |
| `cmd/gopager` | Flag/env configuration, role composition, HTTP route registration and JSON mapping for management, intake, gateway and processing routes, `bootstrap` subcommand |
| `internal/platform` | Store-neutral runtime: `Config`/roles, `Store` and `Runner` interfaces, `TransactionContext`, `Clock`, `Failure` error codes, readiness/health handlers, graceful shutdown (`StartWithAPI`, `RunningHandle`) |
| `internal/platform/postgres` | PostgreSQL `Store`: connection/TLS policy, primary-only transactions, failure classification, database clock, goose migration check/apply under an advisory lock |
| `internal/authz` | Grant/scope/action catalog, field permissions, access-key restrictions, `Authorize` point decisions, age cutoffs |
| `internal/identity` | Accounts, password hashing (Argon2id), secrets, login with rate limits, sessions/CSRF/origin policy (`HTTPTransport`), `Resolver` (proof to current authority), gateway authentication, bootstrap |
| `internal/commands` | Actor-bound command receipts, input fingerprints, optimistic revision increments |
| `internal/events` | Domain event fact types, envelope construction and validation, appender interfaces |
| `internal/protocols` | Protocol enum, address/target model and ranges, payload forms, payload identity |
| `internal/messages` | Grouping decision type and the pure grouping function `Decide` |
| `internal/Units` (package `units`) | Units, Unit collections, Organizations with setting defaults, Unit templates, bulk Unit operations, setting inheritance |
| `internal/assignments` | Address rules (selectors, validity intervals, conflict validation), target matching, suppressions, correction status/retry, `CorrectionRunner` |
| `internal/ingest` | Submission validation, Network resolution, intake service, gateway state reports, gateway administration (network assignment, credentials) |
| `internal/processing` | Workflow definition model and compiler, CEL predicates, Core regex, value/schema codec, candidate state machine, retry schedule, source capture, plugin host and installed-step execution, processing instances |
| `internal/processing/management` | Authorized workflow/version/instance commands and reads, candidate status reads, manual retry, preview |
| `internal/processing/worker` | Candidate dispatcher and per-candidate execution (claim, evaluate, complete/fail, publish) |
| `internal/processing/publication` | Applies completed results and source readiness in the caller's transaction |
| `internal/processing/execution` | Binds a request proof to preview authority for installed steps |
| `internal/processing/synthetic` | In-memory synthetic transmissions used as the source for workflow previews |
| `internal/persistence/migrations` | Embedded goose SQL migrations (`migrations.Files`) |
| `internal/persistence/ent` | Generated Ent client (do not edit). `schema/` holds the entity definitions. |
| `internal/persistence/ent/borrowed` | Ent driver that runs on an existing platform transaction and cannot commit or close it |
| `internal/persistence/ent/guard` | Mandatory Ent privacy policy: deny unless an adapter-bound access decision is in context |
| `internal/persistence/ent/permissions` | Translates `authz` grants into EntQL row predicates (`ReadScope`) |
| `internal/persistence/ent/identitystore` | Accounts, credentials, grants, field permissions, login attempts, audit, access revision |
| `internal/persistence/ent/unitstore` | Units, collections, Organizations, templates |
| `internal/persistence/ent/assignmentstore` | Address rules, assignment revision, pending scopes, suppressions, correction worker queries |
| `internal/persistence/ent/ingeststore` | Gateways, Networks, credentials, transmission intake, grouping, contributions, processing admission. `ProcessingSourceOwner` provides the source, reconciliation and publication readiness. Uses SQL directly through `postgres.Executor`. |
| `internal/persistence/ent/processingstore` | Workflows, versions, scope references, instances, candidates, results, publication items |
| `internal/persistence/ent/commandstore` | Command receipt persistence |
| `internal/persistence/ent/eventstore` | Appends domain events and their outbox references |
| `pluginsdk` | Public Go SDK for plugins (`Serve`, `Handler`) |
| `internal/testutil/postgres`, `internal/testutil/pluginfixture` | Test-only: disposable PostgreSQL, plugin fixture binary |

Repository layout, tests and code generation are covered in [development.md](development.md).

## Domain model

### Intake, grouping and assignment

```mermaid
erDiagram
    GATEWAY ||--o{ GATEWAY_NETWORK : "assigned to"
    NETWORK ||--o{ GATEWAY_NETWORK : ""
    GATEWAY ||--o{ GATEWAY_CREDENTIAL : authenticates
    GATEWAY ||--o{ TRANSMISSION : submits
    NETWORK ||--o{ TRANSMISSION : "resolved to"
    MESSAGE ||--|{ TRANSMISSION : groups
    TRANSMISSION ||--o{ TRANSMISSION_TARGET : "ordered targets"
    ADDRESS_RULE |o--o{ TRANSMISSION_TARGET : "matched rule"
    UNIT |o--o{ TRANSMISSION_TARGET : "resolved Unit"
    NETWORK ||--o{ ADDRESS_RULE : scopes
    UNIT ||--o{ ADDRESS_RULE : targets
    MESSAGE ||--o{ MESSAGE_CONTRIBUTION : summarizes
    UNIT |o--o{ MESSAGE_CONTRIBUTION : "per Unit"
    MESSAGE_CONTRIBUTION ||--|{ CONTRIBUTION_SUPPORT : "supported by"
    TRANSMISSION ||--o{ CONTRIBUTION_SUPPORT : evidence
    TRANSMISSION ||--o{ TARGET_SUPPRESSION : "hides target"
    PENDING_ASSIGNMENT_SCOPE ||--o{ CORRECTION_DIRTY_TRANSMISSION : tracks
    TRANSMISSION ||--o{ CORRECTION_DIRTY_TRANSMISSION : ""
```

### Master data and processing

```mermaid
erDiagram
    ORGANIZATION |o--o{ UNIT : contains
    ORGANIZATION |o--o{ UNIT_TEMPLATE : contains
    UNIT ||--o{ UNIT_COLLECTION_MEMBERSHIP : ""
    UNIT_COLLECTION ||--o{ UNIT_COLLECTION_MEMBERSHIP : ""
    WORKFLOW ||--o{ WORKFLOW_VERSION : versions
    WORKFLOW |o--o| WORKFLOW_VERSION : "active version"
    WORKFLOW ||--o{ WORKFLOW_SCOPE_REFERENCE : "scope (Network or Unit)"
    WORKFLOW_VERSION }o..o{ PROCESSING_INSTANCE : "installed steps reference"
    PROCESSING_INSTANCE ||--|{ PROCESSING_INSTANCE_VERSION : revisions
    PROCESSING_INSTANCE_VERSION }o..|| ACCOUNT : "execution principal"
    TRANSMISSION ||--o{ PROCESSING_CANDIDATE : "evaluated by"
    WORKFLOW ||--o{ PROCESSING_CANDIDATE : ""
    WORKFLOW_VERSION ||--o{ PROCESSING_CANDIDATE : "bound version"
    PROCESSING_CANDIDATE ||--o{ PROCESSING_RESULT : produces
```

Dotted relations are references stored inside JSON columns (workflow definition, instance configuration).

### Identity and cross-cutting records

```mermaid
erDiagram
    ACCOUNT ||--o{ CREDENTIAL : "session / accessKey / recovery"
    ACCOUNT ||--o{ MEMBERSHIP : ""
    USER_GROUP ||--o{ MEMBERSHIP : ""
    ACCOUNT ||--o{ GRANT : "subject principal"
    USER_GROUP ||--o{ GRANT : "subject userGroup"
    ACCOUNT ||--o{ FIELD_PERMISSION : "subject principal"
    USER_GROUP ||--o{ FIELD_PERMISSION : "subject userGroup"
    ACCOUNT ||--o{ COMMAND_RECEIPT : actor
    ACCOUNT ||--o{ AUDIT_RECORD : actor
```

`DOMAIN_EVENT`, `LOGIN_ATTEMPT` and the single-row control tables (`identity_control`, `assignment_control`, `grouping_control`, `processing_control`) have no foreign relations.

Most cross-aggregate references are UUID columns without Ent edges. The schema is in [internal/persistence/ent/schema](../internal/persistence/ent/schema/) and the authoritative DDL is in [internal/persistence/migrations](../internal/persistence/migrations/).

### Glossary

| Term | Code | Definition |
| --- | --- | --- |
| Gateway | `gateways`, `ingest` | Registered receiver instance that submits transmissions under its own bearer credential. It has a state (`active`, `degraded`, `stopped`, `contact-lost`) and contact timestamps. |
| Gateway credential | `gateway_credentials` | Hashed bearer secret bound to one gateway. At most one active and one pending replacement per gateway. |
| Network | `networks` | Logical radio network with exactly one protocol. Context for network resolution and address rules. |
| Gateway network | `gateway_networks` | Assignment of a Network to a gateway under a gateway-local identifier, optionally the default for its protocol |
| Protocol | `protocols.Protocol` | `pocsag`, `flex`, `zvei` or `eas`. Determines valid payload forms and address ranges. |
| Submission | `ingest.Submission` | One gateway request carrying one transmission, identified by `submissionId` for idempotent retries |
| Transmission | `transmissions` | Immutable record that one gateway observed one emission: payload, ordered targets, observation and acceptance time, resolved Network, quality metrics, incomplete flag |
| Payload / payload form | `protocols.TransmissionPayload` | Submitted content in a closed per-protocol form (`text`, `numeric`, `tone-only`, `binary`, `flex-short-signal`, `zvei-signal`, `eas-header`) |
| Payload identity | `protocols.PayloadIdentity` | Normalized form, subtype and value used only to compare transmissions for grouping |
| Transmission target | `transmission_targets` | One original address/function at a position, with its current resolution status, Unit, matched rule and assignment revision |
| Message | `messages` | Group of transmissions with equal payload identity within the grouping window. Has first/last observation time and publication status. |
| Grouping window | `grouping_control.window_seconds` | Maximum observation-time distance for joining an existing Message. `0` disables grouping. |
| Message contribution | `message_contributions` | Derived per-Message, per-Unit read row (or Unit-less for targetless input) with contribution time, age anchor and `ready` flag |
| Contribution support | `contribution_supports` | Link from a contribution to the transmission/target position that supports it |
| Unit | `units` | Logical recipient that addresses resolve to. Optional Organization, settings, collection membership. Deletion is soft (`deleted`). |
| Organization | `organizations` | Container of Units and templates that supplies inherited setting defaults |
| Unit collection | `unit_collections` | Named set of Units used as an authorization scope |
| Unit template | `unit_templates` | Standalone settings preset for creating Units. Copied settings keep no link to the template. |
| Address rule | `address_rules`, `assignments.Rule` | Mutable rule mapping a selector (address value or range, function set/mode) on a Network and protocol to a Unit, with optional `valid_from`/`valid_until` |
| Assignment | `assignments.Resolution` | Result of resolving one target against current rules at its observation time: `assigned` or `unassigned` |
| Assignment revision | `assignment_control.revision` | Global counter incremented by every rule, Unit or suppression change. Stamped on targets and candidates. |
| Suppression | `target_suppressions` | Active marker that excludes one transmission target from contributions |
| Correction (pending scope) | `assignment_pending_scopes` | Durable rematch work for one assignment revision and affected Networks/Unit, with lease, cursor, attempts and failure code |
| Principal / Account | `identity_accounts` | Internal identity of kind `human`, `technical` or `guest`. Grants and credentials attach to it. |
| User group | `identity_user_groups` | Set of accounts used as a grant subject |
| Credential | `identity_credentials` | Hashed `session`, `accessKey` or `recovery` secret of an account. Access keys carry restrictions. |
| Grant | `identity_grants`, `authz.Grant` | Actions on a scope (`all`, `network`, `unit`, `unitCollection`, `account`) for a principal or group, with optional maximum data age and delegability |
| Field permission | `identity_field_permissions` | Global permission to read field groups (`payload`, `address`, `unit`, `gateway`, `quality`, `derived`, `extension.<ns>.<name>`), independent of scope |
| Access key restrictions | `authz.Restrictions` | Subset of actions, scopes, fields and age that limits an access key below its owner's grants |
| Authority | `authz.Authority` | Current grants, field permissions and key restrictions of a principal, loaded once per transaction |
| Workflow | `processing_workflows` | Named, scoped (`all`, `network`, `unit`), enable-able processing configuration that points at an active version |
| Workflow version | `processing_workflow_versions` | Immutable definition: selector predicate, steps (`map`, `extract`, `branch`, `installed`), output schema, outputs |
| Processing instance | `processing_instances`, `processing_instance_versions` | Revisioned binding of a provisioned plugin artifact (manifest path and pin), its configuration, and the execution principal it runs as |
| Processing candidate | `processing_candidates` | Durable work item for one transmission and workflow version at given source/context revisions. States: `pending`, `running`, `completed`, `failed`, `superseded`. Has lease, fencing token and attempts. |
| Processing result | `processing_results` | Typed evaluation output of a completed candidate, applied to the Message on publication |
| Domain event | `domain_events`, `events` | Validated JSON envelope recording a committed state change |
| Command receipt | `command_receipts` | Stored outcome of an idempotent administrative command per actor and command ID |

## Cross-cutting mechanisms

### Transactions and the store abstraction

- `platform.Store` (implemented by `postgres.Store`) offers `WithinTransaction(ctx, func(platform.TransactionContext) error)`. Services receive it as `identity.Transactions`.
- `TransactionContext` is an opaque capability. Only store adapters can turn it into SQL access, through `postgres.Store.Resolve`. Resolving fails after the callback returns or for a different store.
- Every transaction runs at `READ COMMITTED` on a writable primary (checked with `pg_is_in_recovery`). It has a 30-second timeout and cannot be nested. If the store has a registered migration set, the schema version is checked before the transaction starts.
- All participants in one operation share one transaction: authority resolution, domain writes, command receipt, audit and domain events. Plugin calls and workflow evaluation run outside SQL transactions. The worker rechecks state with fencing before writing results.
- Ent repositories run on the borrowed transaction (`borrowed.Driver`). Every Ent entity has the `guard.Policy`, which denies all queries and mutations unless the adapter has bound an `Access` with per-entity EntQL predicates (from `permissions.ReadScope`) for the live transaction. Hot paths (`ingeststore`, correction and candidate queries) use SQL directly through `postgres.Executor`.
- Concurrency control uses row locks on single-row control tables, in a fixed order: `identity_control` (access revision), then `assignment_control`, then `grouping_control`, then `processing_control`. Aggregates use `revision` columns with expected-revision checks (`commands.NextRevision`). Background work uses leases and fencing tokens (`platform.NextFencingToken`).

### Clock

`platform.Clock.Now(ctx, tx)` returns the database's `clock_timestamp()` through the caller's transaction, truncated to microseconds. Acceptance times, leases, due times, age checks and event timestamps all use this clock, never a process wall clock.

### Domain events and outbox

- Producers build typed facts (`events.IntakeFact`, `IdentityFact`, `MasterDataFact`, `ConfigurationFact`, `CorrectionFact`, `Processing*Fact`). `Envelope()` validates each fact into a versioned envelope with `domainEventId`, `type`, `producer`, `aggregateRef`, `aggregateRevision`, `occurredAt`, `recordedAt`, `data`, `classification` and `replayMode`. Envelopes contain no original payload and no secrets.
- `eventstore` inserts the envelope into `domain_events` and, in the same transaction, publishes a `{schemaVersion, domainEventId}` reference to topic `domain_facts` through the watermill-sql v4 publisher (`watermill_domain_facts`, `watermill_offsets_domain_facts`). The event and the state change commit or roll back together.
- The application does not subscribe to `domain_facts`. Workers find their work by querying their own tables.
- `producer.domain` names the producing domain:

| `producer.domain` | Domain | Events |
| --- | --- | --- |
| `intake` | Gateways, Networks, Transmissions and their targets | Transmission and Gateway facts |
| `grouping` | Payload identity and Message grouping | `message.*` |
| `masterdata` | Organizations, Units, collections, templates | Unit and Organization configuration facts |
| `assignment` | Address rules, target resolution, suppression, historical correction | `assignment.*`, rule configuration and correction facts |
| `identity` | Principals, credentials, sessions, grants | Identity facts |
| `processing` | Workflows, processing instances, candidates, results | Processing facts |

| Event type | Emitted when |
| --- | --- |
| `message.created`, `message.transmission-joined`, `message.merged` | Grouping outcome at intake |
| `assignment.changed` | Target resolution set at intake (`created`) or changed by correction (`corrected`) |
| `suppression.changed` | Suppression created or lifted |
| `gateway.state-changed` | Effective gateway state change |
| `operation.submission-rejected`, `operation.observation-time-implausible`, `operation.network-ambiguous` | Operational problems at intake |
| `configuration.changed` | Account/group/grant, Unit/collection/Organization/template, rule, workflow or processing-instance configuration changed |
| `principal.access-changed`, `principal.deleted` | Effective access of a principal changed or the account is being deleted |
| `processing.completed` | A processing result was published |

### Idempotency

| Mechanism | Scope | Behavior |
| --- | --- | --- |
| Command receipt (`commands`, `command_receipts`) | Create-style management commands carrying `commandId` | Unique per `(actor, commandId)`. Stores the operation, a SHA-256 fingerprint of the canonical input and the result. A retry with the same input returns the stored result. A different input returns `REVISION_CONFLICT`. |
| Submission receipt | Gateway intake | Unique per `(gateway_id, submission_id)` on `transmissions` with `submission_fingerprint`. A retry returns the original receipt. A conflicting retry is rejected and recorded. |
| Expected revision | Updates and deletes | `expectedRevision` must equal the stored `revision` |

### Authorization

1. The transport selects a proof (`identity.HTTPTransport.SelectProof`): an `Authorization` header (access key) takes precedence over the session cookie. Without either, the request is anonymous.
2. `identity.Resolver.Resolve` locks the access revision, validates the credential (expiry; sessions also have idle and absolute limits), loads the account (anonymous requests map to the `guest` account), and builds the `authz.Authority` from direct and group grants and field permissions. Access-key scopes are checked for current existence through the owning store (`applicationScopeOwners`). Cookie-authenticated state changes additionally require CSRF and Origin checks.
3. Decisions:
   - Point decisions use `authz.Authorize(authority, action, fields, target, now)`. Grants must contain the action, match the target's Network/Unit/Unit collection/account scope, and satisfy the maximum age against the target's age anchor. Access-key restrictions must also allow the request. `message.detail` additionally requires `message.read`.
   - List and read queries use `permissions.ReadScope`, which compiles the same grant semantics into EntQL predicates. The guard policy applies them before counting and paging, so hidden rows are never loaded.
   - Field reads are allowed only for fields in the union of field permissions (and key fields).
4. Actions form a closed catalog (`authz.Actions()`), and each action allows only certain scope kinds. There are no wildcards. The initial account receives explicit actions and fields through `gopager bootstrap`.
5. Background executions (workflow plugin steps) run as the processing instance's execution principal (`Resolver.ResolvePrincipalAuthority`), which is checked again at claim and at completion.

### Errors and failure codes

`platform.Failure` wraps a cause with one of these codes: `INVALID_INPUT`, `REVISION_CONFLICT`, `INCOMPATIBLE_VERSION`, `RESOURCE_LIMIT`, `STORAGE_UNAVAILABLE`, `AUTHORITY_UNAVAILABLE`, `COMMIT_UNCERTAIN`, `INTERNAL_ERROR`. `Error()` returns only an English fallback text, so wrapped causes never reach clients or logs (`LogValue`). PostgreSQL errors are classified as follows:

| SQLSTATE | Code |
| --- | --- |
| serialization failure, deadlock, unique violation | `REVISION_CONFLICT` |
| read-only transaction | `AUTHORITY_UNAVAILABLE` |
| out-of-range values and resource exhaustion | `RESOURCE_LIMIT` |
| lost connection during commit | `COMMIT_UNCERTAIN` (never retried automatically) |

`STORAGE_UNAVAILABLE`, `AUTHORITY_UNAVAILABLE` and `REVISION_CONFLICT` are retryable. Domain sentinel errors (`authz.ErrForbidden`, `ErrNotFound`, …) are mapped to HTTP responses in `cmd/gopager`. The response envelope (`code`, `textKey`, `fallback`, `requestId`, `retryable`) is documented in [http-api.md](http-api.md).

### Readiness and lifecycle

`platform.StartWithAPI` checks connectivity, write authority and the migration state before starting any role. It then serves `/health/live` and `/health/ready`, rechecks every 5 seconds, and rejects application requests and new transactions with `503 AUTHORITY_UNAVAILABLE` while not ready. The processing dispatcher and the correction runner do not exit on transient database failures (`STORAGE_UNAVAILABLE`, `AUTHORITY_UNAVAILABLE`, `COMMIT_UNCERTAIN`, `REVISION_CONFLICT`); they log the code and retry with a backoff that doubles up to 30 seconds, so readiness returns once the recheck succeeds. A runner that exits early for any other reason marks the process not ready until restart. On SIGINT or SIGTERM the process stops accepting work, drains in-flight operations and runners, and closes the plugin host and the store. Configuration flags are listed in [operations.md](operations.md).

### Migrations and Ent code generation

| Concern | Source | Applied by |
| --- | --- | --- |
| Database schema (DDL) | Hand-written goose SQL files in `internal/persistence/migrations/NNNNN_name.sql`, embedded through `//go:embed` | `postgres.Store.Migrate`: `--migration-mode=check` (default) refuses to start on missing or unknown versions; `apply` applies pending ones. Runs under an advisory lock, with ledger table `platform_schema_migrations`. `NO TRANSACTION` and `ENVSUB` directives are rejected. |
| Go data-access code | Ent schema in `internal/persistence/ent/schema`, generated with `go generate ./internal/persistence/ent` (features `privacy`, `entql`, `sql/modifier`, `sql/lock`) | Compile time only. The generated `migrate` package is never executed. |

A schema change needs both a new migration file and a matching Ent schema change followed by regeneration. Ent never creates or alters tables at runtime. See [development.md](development.md).
