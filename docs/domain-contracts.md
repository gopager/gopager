# GoPager Contract Catalog

Date: 2026-10-24. All contract versions: **0.1-draft**. Owner roles and module dependencies: [architecture](architecture.md#domains-and-data-ownership). Open decisions and approval status: [plan](authoritative-plan.md#open-issues-and-risks).

This document records defined domain invariants and describes interfaces as **documentary drafts**. Operation/field names and result types are proposals, not compilable Go signatures, fixed HTTP endpoints, or an approved Wasm ABI. Their semantics must not weaken confirmed product rules. Freeze a versioned contract only after resolving its blockers.

## Shared Interface Conventions

- Each operation identifies context, calling principal/source, domain input, result, and possible errors. Go uses cancellable context; concrete deadlines and size limits remain to be specified, not assumed unlimited.
- IDs are opaque stable references. UUID/integer format, transport serialization, and schema/OpenAPI version are not dictated by this notation.
- Source reception time, server acceptance time, domain validity time, and projection revision are distinct concepts. Time format, trust/plausibility limits, grouping time basis, and age anchors remain to be specified. Never silently substitute one time for another.
- Mutable configuration needs a visible revision/expected state for conflicts. Immutable reception evidence is not 'corrected' by updating it; derived data is versioned separately.
- Authentication is not authorization. Administrative, guest, source, API-key, worker, and plugin paths require explicit capabilities. A technical worker is not a global data reader for outbound payloads.
- Draft error classes: invalid/incomplete, login required, forbidden/invisible, revision/idempotency conflict, temporarily unavailable, storage/authority problem, internal error, uncertain external effect. Define HTTP codes/envelopes, retry hints, and redaction before OpenAPI freeze. Other users' IDs, secrets, and internal SQL/plugin errors do not belong in public details.
- English is the authoritative interface/documentation language. Define stable machine-readable error/status codes and structured message parameters independently of localized display text. Any human-readable fallback is English; clients do not parse it. UI language and formatting locale do not change IDs, field names, enum/event values, wire formats, or authorization.
- Atomicity applies within the authoritative database, not to browser state or external providers. Commit data and required domain outbox work together. Intermediate derivation must not be reported as complete state.
- Idempotency is operation-specific; it replaces neither AuthZ nor distinguishing independent receptions. Retention/restore must not unintentionally regenerate old work.
- Subscribing to an event type never grants an event/plugin/SSE consumer unprojected original content. Field permissions remain bound to the domain data context.

## Contract Register

| Contract | Subject | Owner | Producer / consumer | REQ references | Blockers |
| --- | --- | --- | --- | --- | --- |
| C-01 | Sources, networks, reception acceptance | DOM-01 | Source/management -> reception -> grouping/assignment | REQ-041..050, REQ-108, REQ-113..114 | P-01, P-03, P-08, P-09 |
| C-02 | Protocol content and observed recipients | DOM-02 with DOM-01 | Protocol boundary -> ingest/grouping/assignment | REQ-020, REQ-031..032, REQ-037..038, REQ-121..122 | P-08, P-09 |
| C-03 | Technical alarm groups | DOM-02 | Reception -> alarm projection/event | REQ-001..002, REQ-113, REQ-122 | P-08 |
| C-04 | Units, organizations, collections | DOM-03 | Management -> master data/query/assignment | REQ-019..030, REQ-095, REQ-132 | P-12, P-13 |
| C-05 | Assignment, suppression, rematching | DOM-04 | Rules/evidence -> assignments/projection revision | REQ-021, REQ-031..040, REQ-123..124 | P-08..P-10, P-12 |
| C-06 | Principal, login, central AuthZ | DOM-05 | Credentials/grants -> all protected use cases | REQ-051..061, REQ-063..079, REQ-105, REQ-107..110 | P-09, P-10, P-14, P-17 |
| C-07 | REST queries, search, SSE | DOM-06 | Domain projections/AuthZ -> Vue/API consumers | REQ-001..020, REQ-027, REQ-029, REQ-055..060, REQ-097..100 | P-08..P-10, P-12, P-15 |
| C-08 | Domain events, outbox, jobs | DOM-07 / integration | Domain modules -> live/rematch/delivery workers | REQ-049, REQ-081, REQ-113..114, REQ-117 | P-01, P-04, P-11, P-12 |
| C-09 | Subscriptions, targets, delivery | DOM-07 | Subscription/event -> plugin host/attempt | REQ-081..087, REQ-105..106, REQ-129..130 | P-07, P-10, P-11 |
| C-10 | Plugin SDK and host capabilities | Plugin host with DOM-07 | Operator/module -> host-controlled capabilities | REQ-083..084, REQ-089..090, REQ-107, REQ-129 | P-05..P-07, P-11 |
| C-11 | Message processing and presentation | DOM-08 | Manager/sample/reception -> derived values/view | REQ-091..097, REQ-099..100 | P-16 |
| C-12 | Persistence, roles, operations | Operations / integration | App/store/operator -> status, backup, recovery | REQ-111..112, REQ-114..120, REQ-126..127 | P-01, P-03, P-04, P-15, P-20, P-21 |
| C-13 | Account lifecycle, audit, extension boundaries | DOM-05 / integration | Account deletion/security actions -> all personal-data owners | REQ-061..062, REQ-065..068, REQ-080, REQ-109, REQ-131 | P-14, P-17 |

## C-01 Reception Acceptance

**Version/status:** 0.1-draft; not approved for implementation. **Owner:** DOM-01; integration owns the shared commit boundary. **Parties:** authorized source/source manager; downstream DOM-02/DOM-04/DOM-07.

| Draft interface | Input | Result / effect |
| --- | --- | --- |
| `SubmitReception` | Context, source identity, submission identity, C-02 protocol content, observed time, optional explicit network and reception metadata | Acceptance receipt/reception reference and an honest acceptance/processing state |
| `ReportSourceState` | Context, source identity, heartbeat or explicit termination | Separate contact/reception/state evidence, state event if appropriate |
| `ConfigureSourceNetworks` | Administrative principal, expected revision, allowed network contexts | Authorized new revision without retroactive mutation of reception evidence |

**Defined:** A network has one domain protocol; a source may submit for multiple allowed networks. An authorized unambiguous declaration precedes inference; reject unauthorized declarations rather than silently redirecting. Unresolved network, absent declaration, and unauthorized submission are different states. Simple single-network sources need no artificial frequency ranges.

Keep supplied final original content, observed recipients, and reception context unchanged. This does not automatically require recording raw radio signals. A transport retry of the same operation creates no new reception; an independent repeat reception does. Proposal: source plus submission ID as idempotency scope, with payload comparison; another payload under that ID is a conflict, not an update. Exact scope and retention remain open.

**Time/transaction:** Acceptance point and crash contract must fit Lite/Full. An uncertain commit is not a definite failure. Queries/resubmissions must respect idempotency and current source permissions. Acceptance status distinguishes original intake, group/assignment derivation, and pending jobs; exact synchronous/asynchronous boundaries remain open. Do not acknowledge 'fully processed' when work has only been scheduled.

**Failure/cancellation:** Invalid protocol, unauthorized source/network, conflicts, storage shortage/write failure, and uncertain write authority are visible without exposing other sources/networks. No silent deletion to free space. Cancellation after a possible commit cannot guarantee absence.

**Source state:** Never seen, active, degraded, deliberately stopped, and contact lost remain distinguishable. Reception and heartbeat have separate times. Missing contact proves no specific crash cause; removed history does not disprove previous reception.

**Open:** Complete submission format/limits, timestamp plausibility, network inference, receipt/idempotency retention, acceptance/derivation boundary, and source timeouts. AP-01/AP-02 clarify; AP-03 provides technical evidence.

## C-02 Protocol Content and Content Identity

**Version/status:** 0.1-draft. **Owner:** DOM-02; DOM-01 owns submitted evidence. **Parties:** source/protocol boundary produces, grouping/assignment consume. **Failure:** Incomplete/invalid content is neither an empty message nor a certain match.

Draft `DescribeProtocolContent` result: protocol identifier and definition version; unchanged supplied content; typed observed recipients/address variants; separate function/signal characteristics; recognized content kind/completeness; versioned derived content identity. Exact types and serialization remain undecided. A hash alone does not define domain equality; canonical representation and collision handling belong to the freeze.

| Protocol | Defined requirement | Resolve before approval |
| --- | --- | --- |
| POCSAG | Preserve final text/data and address/function characteristics; distinguish receptions/recipients | Text/numeric/tone-only/empty cases; which function belongs to content identity versus recipient variant? Do not automatically equate empty content. |
| FLEX | Preserve final content, multiple recipients, and characteristics | Content types, completeness evidence, recipient/function boundary; no unsolicited partial-telegram reassembly in the product core. |
| ZVEI | Treat tones/function characteristics according to the protocol, not as missing text | Which tone sequence is content, address, or both? Positive/negative examples and repeat normalization needed. |
| EAS | Included in proposed initial scope; structured content semantics possible | Does a source provide SAME/header data only or the actual message? Define repeated headers/EOM, station, issue time, validity, target areas, and media reference. No automatic CAP/audio/video/decoder scope. |

**Identity:** Equal display text is not a general equality definition. Do not infer Unicode/whitespace/case normalization for grouping from search semantics. Originals remain unchanged; later presentation/workflow and master-data correction do not change identity. Source/network are reception context, not blanket group keys. A station identifier inside protocol content cannot be dropped to establish equality without a contract.

**Permissions:** Protocol data is not public metadata. All text/address/function fields receive C-06 projection; unknown new fields receive no automatic grant. Project i18n never translates original protocol content before comparison.

**Open/fixtures:** Complete discriminator/payload per protocol, exact identity representation/versioning, invalid-data treatment, EAS-region-to-unit boundary. Do not define EAS area codes as pager addresses for convenience. AP-01 resolves relevant P-08/P-09 issues.

## C-03 Technical Alarm Grouping

**Version/status:** 0.1-draft. **Owner:** DOM-02. **Producer:** stored reception evidence with C-02 identity. **Consumers:** DOM-06 and C-08 events. **Draft interface:** `AssignReceptionToGroup(context, receptionRef, contentIdentity, groupingPolicyRevision) -> membership, groupRef, changeKind`.

**Defined:** Content-identical receptions of the same protocol may group across networks/sources. Sliding window from the last matching reception; one shared configurable duration. No confirmed duration/default. Adding a reception differs from creating a group. A technical group is not an operational incident.

Each further chronologically matching reception advances the window. Repeated identical content can therefore continue a group indefinitely. No additional maximum span has been decided. Transport retries do not extend the window by inventing a reception.

**Transaction/concurrency:** Membership, group revision, and required event must remain consistent; simultaneous matching receptions must not accidentally create duplicate groups. Validate SQL serialization/claim strategy and C-01 derivation boundary on both backends. Rematching, lifting suppression, and workflow changes are not regrouping.

**Projection:** Global last-reception time, group size, and source list are not automatically visible. DOM-06 projects from permitted receptions; hidden receptions must not leak through counts, ordering, or change indicators.

**Open:** Reception versus server acceptance time; exact W boundary; out-of-order delivery; selection among multiple candidate groups; duration/default; configuration changes, existing membership, and age of indefinitely continued groups. No silent merging of old groups. AP-01 resolves these with fixtures.

## C-04 Units, Organizations, and Domain Collections

**Version/status:** 0.1-draft. **Owner:** DOM-03. **Parties:** authorized management; DOM-04/DOM-06/DOM-07 consume current master data. **Draft interfaces:** `CreateUnit`, `ChangeUnit`, `DeleteUnit`, `DeleteOrganization`, `ChangeCollectionMembership`, `PreviewBulkChange`; each takes principal, domain input, and expected revision for changes. Results: new reference/revision or authorized conflict/preview state.

**Defined:** Units have their own identity, may belong to several collections, and are not necessarily subordinate to one network. Organization, color, icon, templates, and selective overrides remain recognizable; returning to organization defaults is possible. Formation membership alone is not evidence of co-alerting. No unrestricted multi-parent default graph.

**Deletion:** Unit deletion coordinates C-05 rematching of every potentially affected historical assignment; no new reception records/groups. Organization deletion preserves/detaches units and retains previously effective organization defaults; no rematching. Current organization filters/grants follow the new reference. Physical deletion versus tombstone/intermediate status remains open; unsafe intermediate states must not be visible.

**Authorization/failures:** Creating an organization within a unit dialog separately requires organization permission. Bulk/deletion checks each target; conflict details do not reveal hidden uses elsewhere. No silent overwrite of concurrent edits. Color/icon-only changes do not rematch.

**Transaction/time:** Consistently capture inherited defaults before detachment; reliably coordinate reference/permission changes and required correction work. C-05 does not require historical color/icon versioning for general master data.

**Open:** Fields/types, default materialization, bulk atomicity/partial failures, tombstones, transaction boundary to rematch invalidation. AP-02; P-12/P-13.

## C-05 Assignments, Suppression, and Rematching

**Version/status:** 0.1-draft. **Owner:** DOM-04. **Producers:** temporal address rules, reception recipients, relevant master-data events. **Consumers:** DOM-06/DOM-07 using central C-06 evaluation.

| Draft interface | Input / result | Obligation |
| --- | --- | --- |
| `ValidateAddressRuleChange` | Principal, target networks/units, expected revision, protocol variant/validity -> preview/conflict | Expose permitted rules/units only; reject equally specific or partially overlapping simultaneously valid rules. |
| `MatchRecipient` | Reception/recipient reference, rule revision -> assignment or explicit unassigned/uncertain/failed state | Most specific unambiguous rule; reception time for historical validity, no cross-group correlation. |
| `ScheduleRematch` | Relevant change/deletion and affected scope -> correction job/revision | Miss no potentially affected historical reception. |
| `ReadRematchStatus` | Authorized principal, job -> waiting/running/completed/failed with safe progress | Start is not full completion; errors/restart traceable. |
| `ChangeSuppression` | Principal, recipient filter, expected revision -> rule revision/projection work | Preserve evidence, correct current visibility/delivery; lifting suppression is not reception. |

**Defined:** Address-to-unit validity is temporal; general unit attributes are not. A later-entered rule valid from X corrects affected history using reception time. When processing is available, rematching starts after 60 seconds of inactivity by default; further changes cannot extend the delay beyond 300 seconds by default. Both are configurable rematching values, not grouping duration or completion SLA.

Suppression is recipient-specific. No normal message view without non-suppressed recipients; mixed messages remain partly usable. A hidden assignment is not relabeled 'Without unit'. No additional content masking or implicit admin bypass.

**Transaction/revision:** New assignments are derived; originals/group memberships remain unchanged. Unit deletion must not cause stale visible units or excessive permissions during long correction. Candidate for evaluation: separate new revision and consistent switch, failing closed on affected projections during security-relevant invalidation. This is not yet a selected implementation. Continuing to show an old projection is not automatically safe after permission changes.

**Open:** Interval boundaries and protocol specificity order, conflict model, unknown-network rules, invalidation/revision transaction, concurrent ingest and repeatedly superseded jobs, retry/cancellation, safe progress details, suppression before/during rematching. AP-01 for protocol examples, AP-02 for P-09/P-10/P-12.

## C-06 Identity, Login, and Authorization

**Version/status:** 0.1-draft. **Owner:** DOM-05. **Parties:** every protected administrative/query/source/live/worker path. OIDC/SCIM remain extension boundaries, not accelerated delivery.

| Draft interface | Input -> result |
| --- | --- |
| `AuthenticateLocal` / `ResolveSession` | Credentials/session proof, context -> internal principal or redacted rejection |
| `ResolveAccessKey` | Key proof -> principal plus restricting key capabilities; never exceeds owner |
| `AuthorizeAction` | Principal, operation, concrete target, current domain state -> decision with internally explainable provenance |
| `BuildAuthorizedReadScope` / `ProjectFields` | Principal, query/recipient context, current revisions -> safe query predicate and permitted fields |
| `ChangeGrants` / `RevokeSession` / `RevokeKey` | Administrative principal, target, expected revision -> new state and required invalidation |
| `RequestRecovery` / `IssueRecoveryProof` / `ConsumeRecoveryProof` | Authorized request/issuance/username plus proof -> request status or controlled password-setting path |
| `LocalRescue` | Local administrative access -> explicit recovery action; no silently replaced account |

**Defined:** Stable internal principal separate from credentials. Direct/multiple group grants are additive; read scope A and write scope B remain distinct. API keys can only restrict further. Deleting a restricting scope object never makes a grant unrestricted. No default access for new principals/guest; guest cannot be deleted; explicitly warn about public write grants.

Evaluate network/unit/collection/account scopes and field rights centrally; search AND is separate. Current unit assignments apply to history. Matching email establishes neither identity nor account linkage. New plugin/processing fields are not implicitly readable. Authorization age limits details/delivery as well as feed; exact age anchor remains open.

**Credentials/recovery:** Local password only with permission; setting/changing/recovery do not bypass its exclusion. Changes check old and confirmed new password. Administrators neither know nor set a user's new password; use short-lived one-time proof plus correct username. Do not disclose unknown accounts. Without secure delivery, recovery creates a request. Only the authorized owner creates personal keys; technical-account keys require target permission. Rotation/revocation/last use remain traceable.

**Consistency/failures:** All output paths check current authoritative state. SQL predicates and field projection share AuthZ semantics; no filtering after already unauthorized pagination. Login/search/sending throttling must not disproportionately block shared locations solely by shared IP. Audit without secrets, rejection without others' details. Display-language selection changes no authorization outcomes.

**Open:** Complete action/field catalog, grant representation, age anchor, session/CSRF/cookie contract, password/hash algorithm and parameters, key/rescue transport, recovery TTL/receipt retention, rate limits, safe cache/revision invalidation. These are not defaults inferred from documentation. AP-02/AP-03; deletion fields in C-13.

## C-07 REST, Search, Details, and Live Updates

**Version/status:** 0.1-draft. **Owner:** DOM-06; integration owns OpenAPI/event boundary; C-06 exclusively owns permission semantics. **Parties:** Vue/API consumers receive authorized projections, not internal database objects.

Draft operations: `QueryAlarms(principal, filters, timeAnchor, cursor) -> page`; `GetAlarmDetail(principal, alarmRef) -> projection`; `ListFilterOptions(principal, context) -> safeOptions`; `OpenLiveView(principal, viewDefinition, resumeState) -> authorizedChangesOrResync`. Response envelopes, endpoints, error codes, and cursor format remain open until versioned OpenAPI freeze.

**Query semantics:** OR within a dimension, AND between dimensions. Source/network/unit/collection conditions must match the same permitted reception/assignment. A group cannot fabricate a match from network A of reception 1 and unit B of reception 2. 'Without unit' requires genuinely unassigned authorized evidence, not hidden assignments.

**Text search:** Literal substrings, case-insensitive, Unicode-consistent, accent-sensitive. Empty search text applies no text filter; `.*` or `%` are user text, not regex/SQL wildcards. Only readable content may produce matches. Validate Unicode form/case folding and correct short-term index strategy across backends.

**Projection before result formation:** Apply suppression, current grants, and field permissions before filtering/counting/pagination/details. Invisible recipients affect neither source/unit names nor counts, time anchors, sorting, facets, or change indicators. Group evidence grants no general access to other recipients. Permitted detail actions may prefill addresses; no blanket management action in the compact list.

**Live:** New/changed permitted alarms update loaded views. Interruptions are visible; reconnect also reconciles deleted/revoked entries. Multiple tabs/filters concurrently, no one-connection-per-IP assumption. Keep loaded history current with reasonable effort; a necessary live-age limit must be visible and reconciled on redisplay. Two hours is not mandatory. Resume does not prove unchanged permissions; safely resynchronize if resume state is insufficient.

**Revocation/failures:** No further private output under revoked sessions/stale permissions. Safe invalidation may address IDs already known to that consumer without exposing other objects. Content already sent to a browser is not promised recallable. Unknown/invisible objects must not become an existence oracle.

**Language/i18n boundary:** Product-generated status/error/validation messages have stable machine-readable codes and structured parameters, documented in English. The frontend maps them to its English source catalog, prepared for future localization, with an English fallback and no reliance on parsing server prose. Keep protocol data, operator names, field names, enum values, timestamps, cursor semantics, and search/authorization behavior independent of UI language. User formatting locale is not an API translation switch. Exact code/parameter catalog and fallback mapping belong to AP-02/AP-09/AP-13; no second translation catalog is required.

**Open:** Stable sorting/cursor/time anchor, age anchor for long-lived groups, revocation linearization, live-age rules, resume/resync envelope, filter-option permissions, cost limits, concrete error/message code mapping. AP-02/AP-03; P-08..P-10/P-12/P-15.

## C-08 Events, Outbox, and Jobs

**Version/status:** 0.1-draft. **Owner:** DOM-07 for mechanics, producing domain for event meaning, integration for commit boundary. **Producers/consumers:** Source/group/assignment/configuration changes -> explicit live/rematch/delivery work.

Draft interfaces: `AppendDomainEvent(transaction, producer, event) -> eventRef`; `ClaimJob(context, role, workerRef) -> leasedWork`; `CompleteJob(leaseRef, result)`; `RescheduleJob(leaseRef, classifiedFailure)`. Proposed event fields: event ID, type/schema version, producer, domain reference/revision, domain/recorded times, causality/idempotency reference, access-classified payload. Exact envelope/required fields are not frozen.

**Defined:** Domain state and required event/work are atomic in the primary store. Outbox state is truth; wake-up/pubsub is optimization only. Do not deliver an event as committed before commit. Event types at least distinguish group creation, reception including repeat reception, assignment/correction, and subscribable source states; no hidden once-per-alarm default. Exact type names/order remain open.

**Recovery:** Claims/leases distinguish concurrent workers and superseded owners; an expired worker cannot blindly overwrite current job state. Internal restart may duplicate processing and requires idempotent consumers. Provider I/O retains C-09 uncertainty, not exactly-once guaranteed by a job lease. Not all jobs inherit one retry algorithm.

**Permissions:** Recording an event does not replace current consumer/delivery authorization. Raw payloads are not browser/plugin grants. Retain rematch/retention/delivery receipts so replay/restore does not revive deleted or expired work. Event type/schema codes are language-independent.

**Open:** Event types/envelope, revision order/replay scope, claim/fencing form, TTL/heartbeats, cancellation/dead-letter/retention, backend wake-up. AP-02/AP-03; P-01/P-04/P-11/P-12.

## C-09 Subscriptions, Targets, and Delivery

**Version/status:** 0.1-draft. **Owner:** DOM-07; C-10 host executes under control. **Parties:** authorized subscription owner/target manager, event worker, host capability, provider.

| Draft interface | Input -> result |
| --- | --- |
| `ConfigureTargetInstance` | Principal, plugin type, expected revision, target-specific configuration/secret references -> separate instance and redacted status |
| `ChangeSubscription` | Owner, expected revision, event selection, filters, target, content selection -> explicit active/paused configuration |
| `PrepareDeliveryAttempt` | Job/event reference -> currently authorized projection, target configuration, attempt permission or cancellation reason |
| `RecordDeliveryOutcome` | Attempt, controlled provider result -> accepted, retry pending, failed/expired, or uncertain |
| `RequestExplicitRedelivery` | Authorized principal, original attempt, deliberate confirmation -> separate traceable job, not overwritten result |

**Defined:** Subscriptions select event, domain filters, target, and permitted content; multiple instances of one plugin remain separate. Unit overrides affect relevant deliveries only and reveal no other secrets. Pause/removal, principal suspension/deletion, current grant revocation, and age limits prevent new unauthorized attempts.

Before every new attempt, authoritatively check current principal, subscription, filters, assignments/suppression, and field permissions. A stored old ready-to-send payload is not authorization. Preparation, execution, and recording are distinguishable; provider acceptance is neither reading nor recallability.

**Uncertainty:** A lost provider response may follow an already executed effect. Do not disguise it as definite failure/success or blindly repeat indefinitely. Bounded retries require failure classification and current permission; permanent/transient/uncertain outcomes remain separate. Deliberate redelivery names duplicate-effect risk. Use provider idempotency only when actually supported.

**Consistency boundary:** No SQL transaction around long external I/O. Revocation after provider transmission starts cannot recall its effect. Define authoritative start/revocation time and preparation/send races before freeze rather than promise impossible immediate recall.

**Open:** Target/unit override schema and precedence, age anchor, event/attempt idempotency, retry limits/backoff, retention/DLQ/recovery triggers, deliberate redelivery proof, result visibility permissions. AP-02/AP-03; P-07/P-10/P-11.

## C-10 Plugin SDK and Host

**Version/status:** 0.1-draft. **Owner:** Plugin host; DOM-07 owns delivery/subscription semantics, integration owns SDK contract. **Producers/consumers:** Operator supplies artifacts; host checks/activates; modules produce/handle explicit events or provide allowed targets.

Draft interfaces: `InspectModule(artifact) -> compatibility`; `ActivateModule(moduleRef, configurationRevision) -> lifecycleStatus`; `InvokeCapability(invocationContext, projectedInput) -> boundedResult`; `DisableModule(moduleRef) -> status`. Documentary manifest: module identity/version, SDK/host compatibility, offered capabilities/event schemas, configuration schema, required host capabilities. Field names, ABI/calling convention, and versioning rules remain open.

**Defined:** Separate Wasm artifacts from operator sources or local provision; Wazero embedded in Lite. Only operators install/update code. Web administrators configure permitted capabilities; users own allowed subscriptions. Status distinguishes incompatible/disabled/active/failed; define state machine and update/rollback before approval. No marketplace, native legacy ABI, or arbitrary-language guarantee. English documentation and stable machine codes do not prevent a language-neutral SDK.

**Host boundary:** No general filesystem/network grant. Event fields follow C-06; secret access is instance/target-bound and not redisclosed through results/logs. Outgoing actions use controlled capabilities with target configuration, failure classification, and time/memory/I/O bounds. Host calls, oversized results, fan-out, and blocking I/O also need limits/cancellation, not just Wasm instructions.

Four first-party targets are separate plugins. An HTTP-only host API does not automatically support email: provide controlled SMTP or explicitly select a mail provider path. For configurable webhooks/hosts, internal-target protection, DNS/redirect/TLS behavior, and secret forwarding belong to the security contract, not a blanket network grant.

**Transaction/failures:** Plugin failures cannot change another domain's data/grants or bypass attempt recording. Wasm adds isolation but is no complete security guarantee. Disable/update during invocation and uncertain provider results need the future C-09/C-10 contract.

**Open:** Manifest/ABI/SDK version, compatible toolchains, error/event models, limits/cancellation/host rights, secret storage/rotation, update/rollback/invocation binding. AP-02/AP-03; P-05..P-07/P-11.

## C-11 Processing, Presentation, and User Configuration

**Version/status:** 0.1-draft. **Owner:** DOM-08; frontend consumes permitted configuration/results. **Producers:** authorized administrative rules and sample/reception content. **Consumers:** presentation and explicit event reactions.

Draft interfaces: `PreviewRules(principal, draft, sample) -> derivedFields, explanation, errors` without live effects; `SaveRuleVersion(principal, expectedRevision, definition) -> savedVersion`; `ActivateRuleVersion` / `SelectPreviousVersion` -> traceable active state; `EvaluateRules(context, version, content) -> boundedDerivedResult`; `RenderAuthorizedContent(projectedFields, presentationConfig) -> safeViewModel`.

**Defined:** Parsing, bounded regex extraction, administratively structured data, conditions/branches, preview/errors. Flexible composition preferred; basic branches are an effort fallback. No arbitrary scripts, loops, or expanded permissions. Saving and taking effect are distinct without a mandated multistage approval. Concurrent edits produce conflicts rather than silent overwrites.

Preserve original content; processing/presentation version changes create no reception/regrouping. Derived fields retain required classification; regex extraction does not make hidden message text public. Rendering/templates allow permitted arrangement/highlighting without mandatory HTML/scripts. Preview silently changes neither real alarms nor targets and sends nothing.

Installation title/notices, dismissal rule, locale/time format/gaps, and preferences have clear persistence/revision/permissions. Text/touch/keyboard, readable states independent of color, and context-preserving copy/details are frontend acceptance goals.

**Internationalization preparation:** Under REQ-097/D-20, English is the initial UI language and source catalog. Isolate all product-owned user-facing text, including editor validation, statuses, accessibility labels, and built-in presentation messages, behind stable message keys. Support parameters/plurals instead of concatenated sentences, and define English fallback. Keep UI language separate from persisted formatting preferences and locale-aware dates/numbers/time zones. Do not translate received content, unit names, operator templates, or extracted values automatically. Message keys and parameters do not grant data access; fallback/error rendering must retain field permissions and redaction. No additional translation catalog, language pack, or selector is required yet. Select a suitable Vue i18n facility during AP-02/AP-13; none is chosen or implemented here.

**Open:** Rule/field/presentation schema, sensitivity propagation, precedence/order, resource limits/regex dialect, reprocessing/activation boundary, builder library by benefit comparison, concrete message-key/parameter catalog and i18n tooling. `essen.js` remains an unverified functional reference, not a parity goal. AP-02; P-16.

## C-12 Persistence and Operations

**Version/status:** 0.1-draft. **Owner:** Operations/persistence with integration and domain schema owners. **Parties:** Lite/Full app, operator, authoritative store, roles, restart.

Draft interfaces: `ReadOperationalStatus -> safeStatus`; `RunRetention(configuredPolicy) -> progress`; `CreateBackup -> manifest`; `RestoreBackup(manifest, targetProfile) -> verifiedState`; `RequestShutdown -> drainStatus`. Capabilities, not fixed CLI commands/API endpoints.

**Defined:** Shared logical schema, explicit SQLite/PostgreSQL adapters; test critical semantics across backends. Lite is one OS process with a local SQLite file. Full primary is authoritative; established external HA provides fencing/failover. Unclear write authority visibly stops acceptance/delivery. Replicas/caches do not prove current authorization.

**Retention/storage:** No default deletion without operator limits; configurable age/count. Storage status/exhaustion visible. Limited retention must not create dangling recipient/group/delivery references or resurrection from lost idempotency/replay records. Retained evidence is not mutated; policy-compliant retention deletion is distinct. Source status distinguishes removed history from never received.

**Acceptance/recovery:** Crash loss of acknowledged Lite inputs is conditionally allowed, not a goal/budget. State commit uncertainty and restart of pending jobs/attempts honestly. Backup/restore includes domain data/configuration, relevant IDs/revisions, pending work, and an explained secret path. Lite->Full preserves domain identities, accounts/permissions, and history, not a domain rebuild. Restore cannot treat already executed external delivery as certainly pending again.

Initial setup without default credentials, status checks, orderly shutdown, traceable updates are required. Drain/cancellation respects jobs and provider uncertainty. Update is no automatic rollback of intervening data. No quantitative SLA/RPO/RTO/hardware promises. Operator-facing product text uses English by default; machine-readable operational codes remain language-independent.

**Open:** Driver/version/migration contract and atomic schema evolution, SQL constraints/isolation, crash/fsync/commit contract, disk thresholds/failure response, retention unit for long-lived groups, backup consistency/secret encryption/export, restore validation, failover/fencing, lease recovery, concrete reference host. AP-02/AP-03; P-01/P-03/P-04/P-15/P-20/P-21.

## C-13 Account Deletion, Audit, and Future Identity

**Version/status:** 0.1-draft. **Owner:** DOM-05; integration coordinates deletion; DOM-07 owns delivery fields. **Parties:** confirmed account owner/authorized security manager, all personal-data owners.

Draft operations: `RequestAccountDeletion` -> confirmed deletion workflow; `ExecuteAccountDeletion` -> access/bindings/subscriptions ended and data classes handled; `ReadSecurityAudit` -> permitted redacted evidence. Atomic immediate access blocking and possible deferred anonymization need clear status/restart semantics.

**Defined:** Anonymize personal historical references; account deletion does not delete units/organizations. Never audit secrets. Security history is accessible where authorized with who/what/when, not a justification for indefinite retention of every personal field.

| Data class | Defined effect | Decide per field before implementation |
| --- | --- | --- |
| Principal/profile | Account ends; anonymize personal historical references | Removed fields, remaining nonpersonal reference, demonstrable anonymity; do not call mere pseudonymization anonymization. |
| Credentials/external bindings/recovery | No further login/recovery for deleted account | Physical deletion/invalidation, blocking evidence/deadlines without retaining secrets. |
| Sessions/API keys | Access ends | Revocation/last-use receipts, redaction/deletion times. |
| Grants/memberships | No effective account permissions | Audit reference and safe removal without expanding others' scopes. |
| Subscriptions/targets | No further account-related delivery | Shared versus personal target instances, secret/contact removal and retention. |
| Audit/delivery history | Anonymize personal references; do not falsify uncertain external effects | Fields, justified remaining receipts/deadlines, free text/payload/provider responses and relinking risks. |

This is a clarification framework, not completed privacy/retention policy. AP-02 resolves P-14 before account-deletion implementation.

**Future identity:** OIDC binds `iss`+`sub` to the same internal principal ID. Linking requires existing login plus fresh external authentication, never matching email alone. Do not accidentally remove the last login path. SCIM is provisioning, not login: explicit group mapping; unmapped means no rights; removed mappings revoke only their own memberships. External deactivation blocks external login/delivery; independent local credentials remain possible under the rule; feed disappearance is not deactivation. Detailed authority/lifecycle belongs to later AP-14, not a generic Phase-1 platform.

## Reference Cases and Acceptance Preparation

These are planned tests/decision examples, not executed tests. AP-01/AP-02 complete concrete inputs/expected outputs for open cases; AP-03 checks backend/runtime behavior. An unresolved outcome is not a passing case. F-01..F-25 are preserved; F-26 adds the explicit language/i18n requirement.

| Case | Input / situation | Expected result or explicitly open decision |
| --- | --- | --- |
| F-01 | Same source and identical submission repeated | No second reception or reception event. Specify payload conflict handling. |
| F-02 | Independent further reception of identical content | Own reception; same technical group possible within window; explicit reception subscription may fire. |
| F-03 | Same protocol and contract-identical content from two allowed networks | Network alone does not split groups; separate evidence/recipients retained. |
| F-04 | Same display text, different protocol | No cross-protocol grouping. |
| F-05 | Chronological identical content at t0, t0+W-epsilon, t0+2W-2epsilon | With chosen time basis, same group through sliding window; no fixed boundary from t0. |
| F-06 | Matching reception exactly at lastTime+W or beyond W | Exact boundary open; beyond expiry creates a new group. No invented comparison operator. |
| F-07 | Late reception potentially matching multiple groups | Decide time basis/candidate selection and effect on existing memberships before implementation. |
| F-08 | Empty/tone-only POCSAG/FLEX; different function with same text | Define content/address identity per variant with concrete positive/negative examples, not all empty values grouped together. |
| F-09 | Same/different ZVEI tones, same display text | Define protocol-correct content/recipient boundary; presentation is not equality evidence. |
| F-10 | Tripled EAS header, EOM, rebroadcast, different station/areas/issue time | Decide fragment/repeat/independent reception and equality under C-02 scope; do not group every part as empty text. |
| F-11 | Reception 1 network A/unit X, reception 2 network B/unit Y; filter network A AND unit Y | No cross-reception match. Test corresponding cross-assignment case. |
| F-12 | Group contains permitted and hidden/suppressed recipient | Permitted projection only; no foreign counts/names/time/change metadata, no artificial 'Without unit'. |
| F-13 | Group contains only suppressed/unreadable recipients | No normal result/detail/live/delivery content. |
| F-14 | Search `.*`, `%`, `_`, empty/short terms, letter case, accented characters | Literal substrings; empty means no text filter; accents distinct. Shared defined expectations for Unicode composition/case folding on both backends. |
| F-15 | Unit deleted or address rule changed retroactively | Rematch history, no new reception/group, no stale permissions during correction. Specify revision/resume. |
| F-16 | Organization deleted, unit partly inherits defaults and partly overrides | Unit survives; applied defaults retained; no rematch; current organization filters/rights follow detachment. |
| F-17 | Session/grant/subscription revoked before new query/SSE event/retry | No new unauthorized output; old cache/payload is not evidence. Test in-flight boundary separately. |
| F-18 | Provider accepts, response lost, job lease expires | Uncertain external outcome; no exactly-once claim or blind endless retries. |
| F-19 | Disk full / primary authority uncertain / crash after possible commit | Visible honest acceptance/job state; no silent default deletion or divergent write. |
| F-20 | Wasm infinite loop, blocking host call, oversized result, secret in error | Bounded cancellation and redacted failure; host stays controlled. Concrete limits after validation. |
| F-21 | Rule preview with hidden text or reaction with external target | No unauthorized extracted output or real delivery through preview. |
| F-22 | Account deletion with pending attempts, audit, multiple credentials | Access/subscriptions end; personal references anonymized under approved field rules; uncertain external effects represented honestly. |
| F-23 | Retention/restore with deleted receptions and old idempotency/delivery jobs | No resurrection or unintended new delivery; consistent state. |
| F-24 | Grant scope object deleted, API key restricted, guest without grants | No scope expansion, key never exceeds owner, guest has no implicit access. |
| F-25 | Several Full workers claim same work; old worker writes after lease loss | No unauthorized overwrite by superseded claim; external I/O remains separately uncertain. |
| F-26 | English UI/catalog, parameterized/plural messages, missing display-message mapping, independent formatting locale | English fallback under the defined mapping, complete keys/parameters/plurals, localized formatting independent of UI language; stable API codes/permissions/search and unchanged original/operator content. No second translated catalog required. |

## Freeze Requirements

Per contract: concrete data/error/version forms, named ownership, producer/consumer agreement, current permission/field projection, time/revision, idempotency/retention, transaction/failure boundaries, and concrete positive/negative fixtures. Resolve P-08/P-12/P-14 and relevant technical validation before affected implementation packages. Define language-independent codes and English message/parameter mapping for C-07/C-11 without requiring additional translations. External REST/SDK contracts get machine-readable artifacts only after these gates; this documentation does not pretend they exist.