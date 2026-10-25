# GoPager Glossary

Date: 2026-10-25. Status: **draft** (AP-01/AP-02); binding vocabulary for all documents, frozen together with the contracts in AP-04. Decisions and provenance: [plan](authoritative-plan.md), [evidence notes](requirements-evidence.md).

This glossary is the **single authoritative source for names** (domain terms, and the code identifiers, API fields, event names, and UI terms derived from them). Every domain term below has exactly one meaning. Terms in the [prohibited terms](#prohibited-and-restricted-terms) list must not be used as domain terms. Code identifiers, API fields, and event names follow these terms. UI labels may be localized, but they do not introduce new domain meanings.

## Radio and Gateway Side

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Emission** | One physical radio (or SMTP) send event. Not a domain object; it is received by zero or more Gateways, each producing its own Transmission. | Transmission (the record of one Gateway) |
| **Receiver hardware** | Physical radio receiver/SDR used by a Gateway. Not a domain object. | Gateway, Unit |
| **Gateway adapter** | Software type decoding Emissions (frames, checksums, fragment reassembly, SMTP parsing) and submitting finalized Transmission reports, for example a multimon-based adapter. A finalized report may describe partial reception; it is not raw decoder-fragment input. Never interprets domain Payload meaning, resolves Units or groups independent reports. | Gateway (a running instance) |
| **Gateway** | A registered, authenticated running instance of a Gateway adapter, authorized for one or more Networks. Has its own state (contact, heartbeat, termination). | Receiver hardware, Subscriber |
| **Network** | A logical radio network or channel with exactly one Protocol, in whose context a Gateway submits Transmissions. | Protocol |
| **Gateway Network identifier** | Gateway-local identifier with which a Submission may declare its Network; the Gateway's configuration maps it to exactly one assigned Network. | Network (the server entity) |
| **Default Network** | The one Network per Gateway and Protocol used when Network resolution finds no unique Network; mandatory if the Gateway has several Networks of that Protocol. No global fallback Network exists. | Network |
| **Network resolution** | Server-side, derived determination of a Transmission's Network: `declared`, `inferred-single`, `inferred-address`, or `default` (C-01). Only Networks assigned to the Gateway are used. Recomputed by Rematching; never changes Transmission evidence. | Grouping |
| **Protocol** | The radio or transport protocol (POCSAG, FLEX, ZVEI, EAS, SMTP-based formats). | Payload form |
| **Protocol definition** | Server-side, versioned description of a Protocol: which submitted fields are Payload, which are Address/Function, and how each protocol message type maps to a Payload form and Canonical payload. | Protocol |
| **Group call** | Protocol feature in which one Emission addresses several Addresses (for example FLEX group messaging, Swissphone Express-Alarm (product feature name), SMTP with several targets in the body). Results in one Transmission per receiving Gateway with several Transmission targets. | Message, User group, Unit collection |

## Transmission and Payload

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Submission** | One transport operation by which a Gateway delivers one Transmission to GoPager, identified by a **Submission ID**. A **Retry** repeats the same Submission and never creates a second Transmission. | Transmission |
| **Transmission** | Immutable record that one Gateway received one Emission, with the same name on Gateway and server side: Gateway, Protocol, optional Gateway Network identifier, Observation time, Payload, Transmission targets, optional quality metrics and incomplete marker. Its resolved Network is derived (Network resolution). One Emission received by three Gateways yields three Transmissions. | Emission, Message |
| **Payload** | Original information carried by a Transmission as submitted (text, numeric digits, tone-only, ZVEI double tone or its absence, EAS header fields), including an accepted partial/deviating value. Stored unchanged per independent reception; physical deduplication is optional only if every original remains losslessly reconstructable. | Message, Canonical payload |
| **Payload form** | Protocol-independent kind of Payload declared by Protocol definitions: `text`, `numeric`, `tone-only`, `zvei-signal`, `eas-header`, `binary` (extensible by versioned definition). | Protocol |
| **Canonical payload** | Payload form plus normalized value used solely for comparison (for example line breaks normalized, terminators removed). Two Transmissions have equal **Payload identity** if their Canonical payloads are equal. | Payload (original), search normalization |
| **Address** | A target address of an Emission as reported in a Transmission: POCSAG RIC, FLEX capcode, ZVEI five-tone sequence, EAS location code, or protocol equivalent. | Unit, Delivery target |
| **Function** | Address attribute such as POCSAG function bits/subaddress. Its meaning depends on the address rule (function bit or subaddress) and the Unit (**Function meaning**); never part of Payload identity. | Payload |
| **Transmission target** | One exact original Address/Function reported in a Transmission. Original evidence is immutable; current Network, Unit, matched Address-rule reference and explicit resolution/suppression status are stored derived attributes. A Group call has several targets. A usable target supports at most one current Message contribution. | Message target, Message contribution, Unit |
| **Quality metrics** | Optional, already evaluated and contextualized reception-quality information supplied by the Gateway. | Gateway state |
| **Incomplete marker** | Optional Gateway statement that a Transmission was received incompletely. | — |

## Messages and Grouping

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Message** | Logical user-facing summary of original Transmissions sharing exact canonical Payload identity under C-03, with shared Message contributions and a Representative Transmission. Canonically different partial originals remain separate. No mandatory second original Payload copy or MessageGroup parent. D-30 defers heuristic membership. Not an incident. | Payload, Transmission, Delivery, Localized text |
| **Grouping** | Server process linking Transmissions by exact canonical Payload equality and time, never Unit/Address-rule matching. Comparison is a separable extension boundary, but heuristics are not active or required now (D-30). | Rematching, User group, Unit collection, Group call |
| **Grouping window (`W`)** | Maximum gap between Observation times of linked Transmissions. | — |
| **Message membership** | Current relation from a Transmission to its Message with Grouping-policy context and match kind `exact`. May be fields on Transmission, not a mandatory entity. Supports refer to contributions of the same Message. The match-kind distinction allows a later separately approved extension, not current heuristic fields/behavior. | User group membership, Contribution support |
| **Message merge** | Combining Messages when a Transmission links to several of them; absorbed Messages become **Message aliases** forwarding to the surviving Message. | — |
| **Message target** | Derived Address/Function summary of Message contributions, not another stored target/Unit mapping. Network/Protocol and resolution context distinguish otherwise equal Addresses. | Transmission target, Delivery target, Unit appearance |
| **Representative Transmission** | Preferred original exact member supplying GUI Payload, not synthetic reconstruction. Within readable, non-suppressed members prefer those not marked incomplete; otherwise visibly qualify fallback. Tie-break earliest Acceptance time then ascending stable ID. Stored preference grants no access; use same rule for readable alternatives. Canonically different fuller text is another Message, not a replacement. Missing Incomplete marker proves no correctness. | Canonical payload, Message time, permission |
| **Message time** | Stored earliest Observation time of all Message members, corrected by an older late join or Message merge. Internal origin and whole-Message retention key; not the normal user query/sort time and not automatically visible through hidden evidence. | View time, Acceptance time |
| **Contribution time** | Stored earliest Observation time of currently usable Contribution supports. Recalculated after correction, suppression or membership changes; later repeated reception never refreshes it. | Rule Validity time, rule-edit time |
| **Contribution age anchor** | Minimum of `min(Observation time, Acceptance time)` over usable Contribution supports. Stored derived age limit basis, preventing future Gateway clocks from extending permission. | Message time, job creation time |
| **Unit appearance** | Query-derived summary of permitted matching Message contributions for one Message and Unit, using earliest Contribution time. Not another stored Message-to-Unit entity. First time is determined before age/time-range filtering; multiple Addresses do not duplicate a Unit. Explicit unassigned/targetless appearances use no synthetic Unit/Address. | Message contribution, Transmission target |
| **View time** | Earliest time of the Unit appearances remaining after current authorization, non-time filters, age and requested time range. One displayed/sort/cursor timestamp per Message in that view, derived at query time; different filters may legitimately yield different View times. | Stored Message time, most recent reception |

## Units and Assignment

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Unit** | Logical recipient defined in GoPager (for example a fire brigade or a person's pager role) to which Addresses resolve. | Address, Subscriber, User |
| **Organization** | Container of Units providing inherited defaults. | User group |
| **Unit collection** | User-defined set of Units for filtering and permissions. | User group, Message |
| **Address rule** | Temporal filter mapping Address/Function within one Network/Protocol to exactly one Unit, with selector/mode and validity interval. Rule edits trigger current rematching and configuration audit; no mandatory immutable version entity or exact historical evaluation replay. Domain validity and edit concurrency revision are distinct. | Assignment, audit history |
| **Assignment** | The result/process of resolving an original Transmission target to its current Unit using Address rules at Observation time, including explicit non-success states. Stored on the target and summarized in Message contributions; no separately mandated Assignment entity. | Address rule, Message contribution, Unit appearance |
| **Message contribution** | Stored, rebuildable, shared address-based read model keyed logically by Message, Network/Protocol, Address/Function and Unit or explicit status. Direct Unit reference, Contribution time and age anchor deliberately denormalize target resolutions. Gateway, rule edit version and validity segment are not identity keys. Different Addresses remain distinct even for one Unit; targetless contributions have no invented Address. | Original Transmission target, Unit appearance, Message target |
| **Contribution support** | Reference-only evidence relationship from a Message contribution to its Transmission and original target; target reference absent only for a targetless Transmission. Many receptions may support one contribution, and one Transmission may support several. At most one current support per usable target; Transmission and contribution must belong to the same Message. | Message membership, original evidence |
| **Suppression** | Rule that hides selected Transmission targets from projections, whether assigned or unassigned. | Permission, Payload masking |
| **Rematching** | Re-evaluating original targets at their Observation times after relevant rule/Unit/Network changes, updating stored resolutions, Contribution supports and Message contributions. Never fabricates Transmission evidence or changes Grouping. | Grouping |
| **Assignment revision** | Correction-state marker for current target resolutions and contribution derivation. Concrete revision fields/placement belong to owner contracts; not a mandatory per-entity counter or immutable rule history. | Rule Validity time, Grouping policy revision |
| **Rematch job** | Durable correction work for an affected scope and Assignment revision, with explicit pending/running/completed/failed/superseded status. | Delivery job |

## Identity, Access, and Delivery

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Principal** | An internal identity (human or technical account, Guest, Worker) acting in GoPager. An Access key acts as its owning Principal with additional restrictions, not as an independent source of rights. | Unit, Gateway, Access key |
| **User** | A human Principal with an account. | Unit, Subscriber |
| **User group** | Set of Principals used for granting permissions. | Unit collection, Group call |
| **Grant / Capability** | Permission to see data or perform an action; Capabilities cover actions such as changing the grouping configuration. | — |
| **Subscription** | A Principal's standing request to be informed about matching Messages or other Domain events. | — |
| **Subscriber** | The Principal owning a Subscription. | Unit, Address |
| **Delivery target** | A configured destination instance (for example a push device, email address, webhook) used by a delivery plugin. | Message target, Address |
| **Delivery plugin** | Separately provided Wasm module executed by the plugin host that performs Deliveries to one kind of Delivery target (for example Pushover, Telegram messenger, email, webhook). | Gateway adapter |
| **Delivery** | One outbound communication to one Delivery target caused by a Subscription; executed through **Delivery jobs** and **Delivery attempts**. | Message, Transmission |
| **Domain event** | Internal, recorded fact about a state change (for example Message created, Transmission joined Message, Messages merged). | EAS event code, operational incident |
| **Operational Domain event** | Domain event about an operational problem that administrators can subscribe Deliveries to, for example rejected Submission, implausible Observation time, or ambiguous Network. | Incident, Gateway state |
| **Operational log** | Administrator-visible record of operational problems (rejected Submissions, implausible Observation times, ambiguous Networks). Separate from the audit trail of configuration changes. | Audit trail, Transmission evidence |
| **Access key** | Revocable bearer credential acting in its owner's name, optionally restricting actions, fields, scopes, age, and lifetime. | Principal, Recovery proof |
| **Personal Subscription** | Subscription acting as its owning User within that User's delegable rights. | Global Subscription |
| **Global Subscription** | Administratively managed Subscription acting as an explicitly assigned technical Principal, within its delegated rights. Never implicitly acts as an unrestricted administrator. | Personal Subscription |
| **Delivery input** | Bounded, materialized, authorized data computed by Core for a Delivery plugin, including plugin-declared fields and rendered template output; independently usable without reading the original Message. | Original Payload, unrestricted object copy |
| **Delivery job** | Durable work to create or execute a Delivery, retaining trigger identity, Subscriber, selected Delivery target, revision context, and expiry. | Delivery, Delivery attempt |
| **Delivery attempt** | One recorded authorization/start and external execution attempt for a Delivery, with an explicit result including possible uncertainty. | Provider acceptance, confirmation of reading |
| **Recovery request** | Account-bound request entering the common local password recovery workflow, whether automatically fulfilled through a trusted integration or approved by an authorized manager. | Subscription |
| **Recovery proof** | Short-lived, single-use secret bound to one account and consumed with the correct username to set a password. | Access key, Session |
| **Session** | Revocable authenticated browser state bound to a Principal, lifetime, and current access state. | Access key |
| **Deleted Principal** | Non-authenticating record retaining the old stable Principal ID so historical actions remain distinguishable, without profile/credential/contact fields. This is pseudonymization, not guaranteed anonymity. | User, reusable account |
| **Audit trail** | Authorized security-change history recording actor, action, target, time, and result without credentials or secret Payload. Deleted actors may remain distinguishable through Deleted Principal references. | Operational log |

## Processing and Extension Vocabulary

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Core** | GoPager's application and authoritative domain services, excluding operator-installed extension code. | Plugin host |
| **Integration** | Configured use of an operator-installed plugin and its declared capabilities; not an additional permission bypass or global Unit-to-Delivery-target mapping. | Integration ownership role |
| **Plugin manifest** | Versioned declaration of an artifact's identity, SDK compatibility, capabilities, internal data access, configuration/Secret fields, extension fields, and input/result schemas. | Grant |
| **Plugin host** | Core runtime that loads prebuilt plugin artifacts, binds declared/approved SDK capabilities, invokes code, and records bounded outcomes. Trust does not imply a hostile-code sandbox guarantee. | External plugin process |
| **Host capability** | SDK-mediated ability declared by a plugin and explicitly approved for an instance, including optional network, filesystem, or command access. | Domain Capability/Grant |
| **Secret reference** | Versioned reference to an instance-bound confidential configuration value in the Core Secret store or an external provider. | Plaintext configuration |
| **Secret store** | Core resolution boundary for encrypted local secrets and externally supplied secrets, including mounted Kubernetes Secret files or environment values; external secrets need not be copied to the database. | Backup |
| **Extension field** | Plugin-namespaced, typed, versioned field attached to a supported Core entity (initially Unit), with explicit read/write permissions and no automatic rights. | Assignment, arbitrary database column |
| **Workflow** | Versioned ingest-processing definition selecting Transmissions and composing bounded parsing/mapping/branching steps or trusted installed code steps. Multiple matching Workflows may run independently. | Subscription, Delivery |
| **Workflow definition version** | Immutable saved selector/step/output-schema definition referenced by a Workflow's active pointer and source-bound Processing results. Saving is not activation or historical reprocessing. | Mutable Workflow revision, Address-rule validity |
| **Ingest publication barrier** | Requirement that selected ingest Workflows and current target/contribution derivation are ready before their new evidence/results enter normal GUI/search/Delivery projections. It does not delay honest original-intake acknowledgement or erase previously ready evidence. | Provider acceptance, whole-Message retention |
| **Workflow selector** | Explicit trigger and correlated domain/Payload predicate deciding whether a Workflow runs for a particular Transmission. | Grant |
| **Processing result** | Typed, namespaced derived fields and status from one Workflow version and input Transmission. A field such as `level` is data, not a Domain event. | Original Payload, Domain event type |
| **Presentation template** | Versioned, permission-filtered rendering definition using an established template engine's constrained logic and escaped data interpolation. | Arbitrary browser script |
| **Text key** | Stable identifier in the English text catalog, mapped to typed parameters and optional plural selection. | Domain event/error code |
| **Field set** | Explicit set of readable domain field categories; computed and plugin-defined fields receive no rights merely by existing. | Scope |
| **Scope** | One explicit permission or filter dimension and its concrete references; an absent/deleted restricting reference never means unrestricted access. | Workflow selector |
| **Page token** | Opaque URL-safe continuation reference, bound to a query and Principal context, never proof of authorization. | SSE resume token |
| **SSE resume token** | Opaque stream-position reference used as SSE `id`/`Last-Event-ID`, bound to a permitted view and current revision checks. | Domain event ID, Page token |
| **Lease / Fencing token** | Time-limited job claim and increasing ownership generation; state writes require the current unexpired generation. | External exactly-once guarantee |

## AP-02 Identifier Registry

The following identifiers derive from the terms above. This registry owns their spelling; the contract catalog owns their schemas and behavior. Nested transport fields retain their schema context rather than becoming new domain entities.

| Identifier family | Binding names and meaning |
| --- | --- |
| Shared wire fields | `schemaVersion`, `id`, `revision`, `expectedRevision`, `status`, `code`, `parameters`, `textKey`, `fallback`, `requestId`, `retryable`, `retryAfterSeconds`: version, reference, concurrency, state, and language-independent outcome metadata. |
| Submission / state | `submissionId`, `protocol`, `gatewayNetworkIdentifier`, `observationTime`, `payload`, `transmissionTargets`, `address`, `function`, `qualityMetrics`, `incomplete`, `transmissionId`, `messageId`, `acceptanceTime`, `messageTime`, `processingStatus`: evidence/references and internal time. `contributionId`, `addressRuleId`, `contributionTime`, `contributionAgeAnchor`, `representativeTransmissionId`, `viewTime`, `appearanceTime`: contribution summaries, matched rule, representative and query time. These are semantic spellings, not a frozen field inventory. `heartbeat`, `termination`: Gateway contact operations. |
| Grants | `subject`, `actions`, `scope`, `fields`, `maxAgeSeconds`, `all`, `network`, `unit`, `unitCollection`, `account`, `withoutUnit`: subject, restrictions and the only initial scope kinds/explicit unassigned permission. |
| Read fields | `payload`, `address`, `unit`, `gateway`, `quality`, `derived`: readable Field sets. `extension.<plugin-id>.<field>`: namespaced Extension field. |
| Read / live | `pageSize`, `pageToken`, `nextPageToken`, `items`, `viewId`, `resumeToken`, `upsert`, `remove`, `resync-required`, `fresh`, `processing-pending`: query/stream fields and view statuses; no raw internal Domain event feed. |
| Domain event envelope | `domainEventId`, `type`, `producer`, `aggregateRef`, `aggregateRevision`, `occurredAt`, `recordedAt`, `causationId`, `correlationId`, `data`, `classification`, `replayMode`: recorded fact, ordering, causality and permitted data context. |
| Core Domain event types | `message.created`, `message.transmission-joined`, `message.merged`, `assignment.changed`, `suppression.changed`, `gateway.state-changed`, `operation.submission-rejected`, `operation.observation-time-implausible`, `operation.network-ambiguous`, `processing.completed`, `configuration.changed`, `principal.access-changed`, `principal.deleted`: distinct facts, not scalar field values. Plugin types use `plugin.<plugin-id>.<type>`. |
| Delivery states | `pending`, `prepared`, `started`, `retry-pending`, `accepted`, `failed`, `expired`, `cancelled`, `uncertain`, `dependency-unavailable`: distinguish scheduling, materialization, I/O and terminal outcomes; `accepted` never means read. |
| Action namespace | `<resource>.read`, `.create`, `.update`, `.delete`, `.manage`, plus `message.read`, `message.detail`, `without-unit.read`, `grouping.configure`, `rule.preview`, `rule.activate`, `rule.reprocess`, `subscription.manage-own`, `subscription.manage-global`, `delivery.read`, `delivery.retry`, `delivery.redeliver`, `key.create-own`, `key.manage-own`, `key.manage-technical`, `password.local`, `recovery.request`, `recovery.issue`, `recovery.consume`, `session.manage-own`, `grant.manage`, `audit.read`, `operations.manage`, `plugin.install`, `plugin.configure`, `plugin.internal-read`, `presentation.manage`, `notice.manage`, `account.delete-own`: exact applicable resources/actions are listed in C-06, not generated permissions for every suffix. |
| Error / text namespaces | Uppercase `INVALID_INPUT`, `LOGIN_REQUIRED`, `FORBIDDEN`, `NOT_FOUND`, `REVISION_CONFLICT`, `IDEMPOTENCY_CONFLICT`, `RATE_LIMITED`, `PROCESSING_PENDING`, `RESYNC_REQUIRED`, `DEPENDENCY_UNAVAILABLE`, `CAPABILITY_UNDECLARED`, `INCOMPATIBLE_VERSION`, `RESOURCE_LIMIT`, `STORAGE_UNAVAILABLE`, `AUTHORITY_UNAVAILABLE`, `COMMIT_UNCERTAIN`, `EXTERNAL_UNCERTAIN`, `INTERNAL_ERROR`; text keys `error.<CODE>`, `status.<status>`, `validation.<constraint>`, `processing.<reason>`, `delivery.retryPending`, `account.deleted`, `history.retainedPartially`, `workflow.failed`: stable meanings and English mappings in C-07/C-11. |
| Existing contract operation names | C-01..C-13 operation identifiers such as `SubmitTransmission`, `MatchTransmissionTarget`, `AuthorizeAction`, `QueryMessages`, `ClaimJob`, `PrepareDeliveryAttempt`, `InvokeCapability`, `PreviewRules` and `ExecuteAccountDeletion` retain English verb + glossary term spelling in their owner sections. AP-02 operations include `ResolveMessageAlias`, `DeclareExtensionFields`, `MaterializeDeliveryInput` and `Reprocess`. No application Backup/Restore operations (D-31). |
| Workflow/deletion states and fields | `pending`, `running`, `completed`, `failed`, `superseded` for Processing results; `deleting`, `cleanup-failed`, `retrying`, `completed` for account cleanup. `reason`, `before`, `after` describe classified assignment transitions; they are not independently authorized snapshots. |

## Personas and Roles

Personas describe who uses GoPager in requirements; they are not permissions. Permissions are always Grants/Capabilities.

| Term | Definition |
| --- | --- |
| **Observer** | Persona reading Messages in the live view, search, and details; may be a Guest. |
| **Guest** | Restricted Principal without a personal account, for example a shared display. |
| **Manager** | Persona maintaining Units, Organizations, Unit collections, Address rules, Gateways, and Networks within granted scope. |
| **Permission manager** | Persona granting and revoking Grants and User group memberships. |
| **Operator** | Persona installing and running a GoPager instance (Lite or Full). |
| **Extension author** | Persona providing Gateway adapters or Delivery plugins. |
| **Worker** | Technical Principal executing jobs; never a global data reader. |

## Time and Revisions

| Term | Definition |
| --- | --- |
| **Observation time** | Time at which the Gateway received the Emission, reported with mandatory UTC offset or UTC designator; the only time basis for Grouping. |
| **Clock-skew tolerance** | Configurable amount by which an Observation time may lie after server time before the Transmission is marked `observation-time-implausible` (default 15 minutes); it never causes rejection. | Grouping window |
| **Acceptance time** | Time at which GoPager accepted the Submission. Never substitutes Observation time. |
| **Validity time** | Domain validity period of configuration such as Address rules. |
| **Revision** | Version of mutable configuration or derived projection state. |

## Other Terms

| Term | Definition |
| --- | --- |
| **Localized text** | Human-readable UI, error, or status text resolved from a stable text key and the English base catalog (the **text catalog**). |
| **Projection** | Permission-filtered view of domain data for a Principal. |
| **Incident** | Real-world operational event. Not modeled by GoPager; a Message never claims to be one. |

## Prohibited and Restricted Terms

| Term | Rule | Use instead |
| --- | --- | --- |
| Recipient / receiver | Not a domain term (ambiguous between Address, Unit, Gateway, Subscriber, Delivery target). | The specific term |
| Group (unqualified) | Never alone. | Message, User group, Unit collection, Group call |
| Alarm, alarm group, technical group | Not used. | Message |
| Telegram | Not used (except as the name of the Telegram messenger delivery plugin). | Emission (physical), Transmission (record), Payload (content) |
| Observation (as a noun for the report), Reception | Not standalone report names. "Observation time" remains allowed. | Transmission |
| Reception link, Address-rule version (as a mandatory entity), Assignment (as a separate shared entity) | Superseded AP-02 model names; historical rename/evidence entries retain their original meaning. Ordinary edit revisions and rule validity remain allowed. | Contribution support; Address rule; Message contribution |
| Source, Ingest module, Ingester, Receiver | Not used (Receiver collides with recipient and receiver hardware). | Gateway, Gateway adapter |
| Content (as a domain noun) | Not used. | Payload, Canonical payload |
| Message (for UI or error text) | Not used. | Localized text |
| Event (unqualified) | Not used. | Domain event, EAS event code, Incident |
| Notification | Not used as a domain term. | Delivery |

## Stored Information (Logical, Not a Data Model)

The physical data model is not prescribed and may normalize or deduplicate. The following information must be stored or exactly reconstructable:

1. **Transmission evidence (immutable):** Gateway, Protocol, Gateway Network identifier if declared, Submission ID, Observation time with its offset, Acceptance time, original Payload per independent reception, all exact Transmission targets/Functions, Quality metrics and Incomplete marker. Repeated Payload storage is allowed; deduplication must preserve every original.
2. **Messages:** exact memberships with policy context/match kind, Representative Transmission preference, publication state and aliases/merge history. Canonical values are derived, not replacements for originals. Heuristic membership is deferred (D-30), not required storage.
3. **Message targets:** derived Address/Function summary of Message contributions; no additional copy.
4. **Target resolution, Message contributions and Contribution supports:** current target Network/Unit/matched Address rule/status/suppression; shared contribution context with direct Unit, first time and age anchor; reference-only supports back to exact original targets or targetless Transmissions. Contributions are rebuildable, not independently edited truth. Rule validity is evaluated per original time; no mandatory immutable rule-version or validity-segment contribution key (C-05).
5. **Derived Transmission attributes:** resolved Network with resolution method (candidate Networks are recomputed, not stored); `observation-time-implausible` marker.
6. **Gateway state history:** contact, heartbeat, and termination over time, so administrators can tell which authorized Gateways of a Network did or did not produce a Transmission for a Message and whether a Gateway had problems.
7. **Operational log:** rejected Submissions (Gateway, Submission ID, error code, time; no Payload content) and other operational problems. Rejected Submissions are not Transmissions.
8. **AP-02 derived/runtime information:** internal Message time, necessary correction/publication revisions and scopes, Processing results/active Workflow versions, current authorization revisions, explicit Subscriptions and Delivery inputs/attempts, Secret references, Extension field definitions and Deleted Principal references. Unit appearances/View time are query results, not extra stored entities. Exact fields, SQL types and indexes remain owner-contract/package work.

Gateway, Network and Unit boxes in the AP-02 illustrative diagram were relationship stubs, not complete master-data schemas. All existing configuration, status/contact, authorization, organization/collection, color/icon, template/default/override, Function meaning and Extension-field requirements remain in force. Absence from that diagram never removes a capability.

Users primarily see Messages with their Message targets and resolved Units; administrators additionally see Transmissions per Gateway. Projection rules (C-06/C-07) apply to all of them.

## Maintenance Rules

These rules bind every work package and every agent or person working on GoPager:

1. **Single source.** Names are defined only here. Other documents, code, APIs, events, and UI use these terms and do not define competing ones.
2. **New names.** Whoever needs a domain or cross-module name that is not listed adds it here with definition and "Not to be confused with" before or together with its first use.
3. **Collisions and renames.** Whoever detects a collision, an ambiguous use, or a need to rename records it in the [rename log](#rename-log) (old name, new name, meaning, reason, date, work package) and updates the entry above. A rename never silently reuses an old name for a different meaning; the old name is added to the prohibited list where appropriate.
4. **Propagation.** After adding or renaming a term, the same work package must search all documents, code, schemas/migrations, API/event definitions, tests, fixtures, and UI texts for the affected names and either update them or record the remaining occurrences as an explicit follow-up with owner. Frozen contracts are changed only through the contract change process (README change rules); the glossary entry then notes the pending contract version.
5. **Historical texts.** The [evidence notes](requirements-evidence.md) keep their original wording; the rename log maps old to new names.

## Rename Log

| Date | Old name(s) | New name | Meaning / reason | Package |
| --- | --- | --- | --- | --- |
| 2026-10-25 | Reception, Observation (noun) | Transmission | Record that one Gateway received one Emission; same name on Gateway and server side. | AP-01 |
| 2026-10-25 | Transmission (physical send) | Emission | Physical radio/SMTP send event, freed "Transmission" for the record. | AP-01 |
| 2026-10-25 | Source, Ingest module, Ingester | Gateway / Gateway adapter | Running instance / software type; "Receiver" rejected (collides with recipient and receiver hardware). | AP-01 |
| 2026-10-25 | Recipient, observed recipient, recipient observation | Transmission target, Message target, Unit, Subscriber, Delivery target | "Recipient" was ambiguous; replaced by the specific term by meaning. | AP-01 |
| 2026-10-25 | Alarm, alarm group, technical (alarm) group, group (of receptions) | Message | Grouped entity; "Alarm" prohibited. | AP-01 |
| 2026-10-25 | Group membership (of receptions) | Message membership | Avoids collision with User group membership. | AP-01 |
| 2026-10-25 | Group (unqualified) | User group, Unit collection, Group call, Message | Unqualified "group" prohibited. | AP-01 |
| 2026-10-25 | Telegram (radio) | Emission, Transmission, Payload | "Telegram" kept only as messenger plugin name. | AP-01 |
| 2026-10-25 | Content, message content, content identity, content class/form | Payload, Payload identity, Payload form, Canonical payload | Avoids collision with Message entity. | AP-01 |
| 2026-10-25 | Message (UI/error text), message catalog/keys | Localized text, text catalog/keys | Avoids collision with Message entity. | AP-01 |
| 2026-10-25 | Notification | Delivery | Single outbound term. | AP-01 |
| 2026-10-25 | Event (unqualified) | Domain event | Distinguish from EAS event code and Incident. | AP-01 |
| 2026-10-25 | Target instance | Delivery target | One name for the outbound destination. | AP-01 |
| 2026-10-25 | Reception time | Observation time | Time basis for Grouping and historical Address-rule validity. | AP-01 |
| 2026-10-25 | User groups (persona heading in requirements) | Personas | Avoids collision with permission User groups. | AP-01 |
| 2026-10-25 | API key as independent Principal | Access key acting as owning Principal | Key restrictions cannot create independent rights. Existing generic API-key wording means Access key, not a second identity. | AP-02 |
| 2026-10-25 | Anonymized account history | Pseudonymized account history / Deleted Principal | Explicit AP-02 user decision retains distinguishable old IDs; no claim of anonymity. Historical evidence wording remains provenance. | AP-02 |
| 2026-10-25 | Unit-to-target Delivery mapping / Unit override selecting destinations | Subscription filter and Extension field | No global mapping; Subscriptions explicitly select Delivery targets and may use declared Unit fields. | AP-02 |
| 2026-10-25 | Assignment per Transmission target | Shared Assignment with Reception links | Address-based Message relationship shared across receptions; original targets remain evidence, not duplicated Unit mappings. | AP-02 |
| 2026-10-25 | Message target as separately stored mapping | Derived Message target summary | Shared Assignments already retain concrete technical Addresses; no additional target/Unit entity. | AP-02 |
| 2026-10-25 | Universal Message time / Message age anchor | Internal Message time; Assignment time/age anchor; View time | Separate storage/retention origin from first-observed user query time. Remove the hidden-member timestamp exception; no stored query-dependent Message timestamp. | AP-02 |
| 2026-10-25 | Shared Assignment entity; Reception link | Message contribution; Contribution support | D-29 supersedes the earlier normalized model: store per-target current Unit resolution and a rebuildable shared read model. Assignment now denotes resolution, not another entity. | AP-02 |
| 2026-10-25 | Address-rule version as mandatory entity | Address rule with validity and configuration audit | Domain validity remains required; immutable edit-version storage and exact old evaluation replay are not required. | AP-02 |
| 2026-10-25 | Assignment time/age anchor; assignmentId; addressRuleVersion | Contribution time/age anchor; contributionId; addressRuleId | Naming follows the new read model and mutable matched filter. Historical entries above remain provenance. | AP-02 |
| 2026-10-25 | Restore hold; CreateBackup/RestoreBackup/AcknowledgeRestoreHold | No application feature; manual Operator Backup/Restore | D-31 excludes the former application lifecycle and credential-invalidation proposal, without removing ordinary restart/job consistency. | AP-02 |
