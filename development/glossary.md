# GoPager Glossary

Date: 2026-10-26. Status: **G-1.0, frozen for the AP-05 foundation slice** under AP-04/D-35; original AP-01/AP-02 meanings and history are preserved. Decisions and provenance: [plan](authoritative-plan.md), [evidence notes](requirements-evidence.md).

**AP-04 version binding:** G-1.0, approved by the user on 2026-10-26 for the D-35 AP-05 foundation slice only. Existing term meanings and naming history are unchanged. Contract-specific types/configuration fields remain at their C-ID owners. Other contract slices do not gain freeze approval from this vocabulary binding; later meaning changes require a new glossary version and affected-consumer review.

**D-40 domain addendum, 2026-10-26:** The user's removal of enduring Message identity/alias requirements updates the Message merge entry and rename history below. This is a domain-only clarification outside the frozen platform vocabulary; the G-1.0 foundation binding and earlier execution evidence remain historical and unchanged.

This glossary owns **domain term meanings**, not interface schemas or implementation algorithms. Code/API/UI names derive from these terms; exact wire identifiers and their behavior are owned together in the [contract registry routes](#ap-02-identifier-registry). UI labels may be localized without introducing new meanings. Terms in the [prohibited terms](#prohibited-and-restricted-terms) list are not domain names.

## Domains

Fixed names of the architecture domains. Ownership and boundaries: [Domains and Data Ownership](architecture.md#domains-and-data-ownership). The code identifier is used where code must name a domain, for example as `producer.domain` in Domain events. Where a domain name coincides with a term below, write "the ... domain" in prose.

| Domain | Code identifier | Scope | Not to be confused with |
| --- | --- | --- | --- |
| **Intake** | `intake` | Gateways, Networks, Submissions, Transmissions and Transmission targets | Ingest package name (`internal/ingest`) |
| **Grouping** | `grouping` | Protocol model, Payload identity and Message Grouping | Grouping (the process), User group, Unit collection |
| **Master data** | `masterdata` | Units, organizations, Unit collections and templates | Configuration in general |
| **Assignment** | `assignment` | Address rules, target resolution, Message contributions, suppression and correction | Assignment as a separate shared entity (prohibited), Unit assignment of a Transmission target |
| **Identity** | `identity` | Principals, credentials, sessions, User groups, grants and central authorization | Gateway identity |
| **Query** | `query` | Search, feed, details and live projections | Projection |
| **Delivery** | `delivery` | Domain event records/outbox, Subscriptions, Delivery targets, Delivery jobs and attempts | Delivery (one outbound send) |
| **Processing** | `processing` | Workflows, Processing results, presentation configuration and the Transmission processing status | Processing of a Transmission in general, Workflow execution |

## Radio and Gateway Side

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Emission** | One physical radio (or SMTP) send event. Not a domain object; it is received by zero or more Gateways, each producing its own Transmission. | Transmission (the record of one Gateway) |
| **Receiver hardware** | Physical radio receiver/SDR used by a Gateway. Not a domain object. | Gateway, Unit |
| **Gateway adapter** | Software type receiving/decoding Emissions and submitting finalized Transmission reports. Responsibility boundary: [C-01](contracts/c-01-transmission-acceptance.md). | Gateway (a running instance) |
| **Gateway** | A registered, authenticated running instance of a Gateway adapter, authorized for one or more Networks. Has its own state (contact, heartbeat, termination). | Receiver hardware, Subscriber |
| **Gateway Bearer credential** | Reusable Bearer secret bound to exactly one Gateway; GoPager derives Gateway identity from the credential. HTTP exposure, authorization and lifecycle are defined by [C-06](contracts/c-06-identity-authorization.md#ap-06-gateway-bearer-credentials). | Gateway token; per-Gateway credential |
| **Network** | A logical radio network or channel with exactly one Protocol, in whose context a Gateway submits Transmissions; also "Radio network". It belongs to exactly one Agency. | Protocol, Agency |
| **Gateway Network identifier** | Gateway-local identifier with which a Submission may declare its Network; the Gateway's configuration maps it to exactly one assigned Network. | Network (the server entity) |
| **Default Network** | Configured Gateway/Protocol fallback in [C-01 Network resolution](contracts/c-01-transmission-acceptance.md). | Network |
| **Network resolution** | Server-side derivation of a Transmission's Network. Precedence, states and rematching boundary: [C-01](contracts/c-01-transmission-acceptance.md). | Grouping |
| **Protocol** | The radio or transport protocol (POCSAG, FLEX, ZVEI, EAS, SMTP-based formats). | Payload form |
| **Protocol definition** | Server-side, versioned description of a Protocol: which submitted fields are Payload, which are Address/Function, and how each protocol message type maps to a Payload form and Canonical payload. | Protocol |
| **FLEX short signal** | A FLEX Short Message carrying a Source code rather than normal text. Protocol meaning and comparison are owned by the Grouping domain in [C-02](contracts/c-02-protocol-payload.md#ap-04-flex-clarification); introduced by AP-04/D-34. | Tone-only call, Gateway/source identity |
| **Group call** | Protocol feature in which one Emission addresses several Addresses (for example FLEX group messaging, Swissphone Express-Alarm (product feature name), SMTP with several targets in the body). Results in one Transmission per receiving Gateway with several Transmission targets. | Message, User group, Unit collection |

## Transmission and Payload

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Submission** | One transport operation delivering a Transmission, identified by a **Submission ID**. A **Retry** repeats that operation; idempotency/retention exceptions belong to [C-01](contracts/c-01-transmission-acceptance.md). | Transmission |
| **Transmission** | Record that one Gateway received one Emission; the name is the same on Gateway and server. Evidence schema/preservation: [C-01](contracts/c-01-transmission-acceptance.md). | Emission, Message |
| **Payload** | Original information carried by a Transmission as submitted. Forms and preservation: [C-02](contracts/c-02-protocol-payload.md). | Message, Canonical payload |
| **Payload form** | Protocol-independent kind of Payload declared by a Protocol definition. Supported forms: [C-02](contracts/c-02-protocol-payload.md). | Protocol |
| **Canonical payload** | Payload form plus normalized value used solely for comparison (for example line breaks normalized, terminators removed). Two Transmissions have equal **Payload identity** if their Canonical payloads are equal. | Payload (original), search normalization |
| **Address** | A target address of an Emission as reported in a Transmission: POCSAG RIC, FLEX capcode, ZVEI five-tone sequence, EAS location code, or protocol equivalent. | Unit, Delivery target |
| **Function** | Address attribute such as POCSAG function bits/subaddress. Its meaning depends on the address rule (function bit or subaddress) and the Unit (**Function meaning**); never part of Payload identity. | Payload |
| **Transmission target** | One original Address/Function reported in a Transmission. Evidence is owned by [C-01/C-02](contracts/c-02-protocol-payload.md); derived resolution is owned by [C-05](contracts/c-05-matching-correction.md). | Message target, Message contribution, Unit |
| **Quality metrics** | Optional, already evaluated and contextualized reception-quality information supplied by the Gateway. | Gateway state |
| **Incomplete marker** | Optional Gateway statement that a Transmission was received incompletely. | — |
| **Synthetic preview source** | Transient server-validated sample with a distinct synthetic identity, never stored original evidence or publication authority. Owner: [C-02](contracts/c-02-protocol-payload.md#synthetic-preview-source-10), D-102. | Transmission, ready source, request-supplied declaration |
| **Bytes value** | Typed binary sequence with canonical Base64 JSON transport, not text; owner: [C-09 shared bytes](contracts/c-09-delivery.md#shared-bytes-value-addendum), D-103. | String, decoded text, Payload comparison hash |

## Messages and Grouping

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Message** | Logical user-facing summary of grouped Transmissions, not a real-world incident. Membership: [C-03](contracts/c-03-message-grouping.md). | Payload, Transmission, Delivery, Localized text |
| **Grouping** | Server process linking Transmissions into Messages. Comparison/membership rules: [C-02](contracts/c-02-protocol-payload.md)/[C-03](contracts/c-03-message-grouping.md). | Rematching, User group, Unit collection, Group call |
| **Grouping window (`W`)** | Maximum gap between Observation times of linked Transmissions. | — |
| **Message membership** | Current relation from a Transmission to its Message. Context/representation: [C-03](contracts/c-03-message-grouping.md). | User group membership, Contribution support |
| **Message merge** | Combining Messages when a Transmission links to several of them. Enduring Message IDs, aliases and old/new ID mappings are not required under D-40; C-03 owns grouping behavior. | — |
| **Message target** | Derived Address/Function summary of Message contributions, not another stored target/Unit mapping. Network/Protocol and resolution context distinguish otherwise equal Addresses. | Transmission target, Delivery target, Unit appearance |
| **Representative Transmission** | Original member selected to supply displayed Payload. Preference/tie-break: [C-03](contracts/c-03-message-grouping.md); readable projection: [C-07](contracts/c-07-query-live.md). | Canonical payload, Message time, permission |
| **Message time** | Internal Message-origin timestamp. Calculation: [C-03](contracts/c-03-message-grouping.md); retention use: [C-12](contracts/c-12-persistence-operations.md). | View time, Acceptance time |
| **Contribution time** | First usable time summarized by a Message contribution. Maintenance algorithm: [C-05](contracts/c-05-matching-correction.md). | Rule Validity time, rule-edit time |
| **Contribution age anchor** | Derived basis for contribution age permissions. Formula/maintenance: [C-05](contracts/c-05-matching-correction.md). | Message time, job creation time |
| **Unit appearance** | Query-derived summary for one Message and Unit. Formation/ordering: [C-07](contracts/c-07-query-live.md#time-selection-and-one-row-message-results). | Message contribution, Transmission target |
| **View time** | Query-dependent displayed/sort time of a Message. Algorithm: [C-07](contracts/c-07-query-live.md#time-selection-and-one-row-message-results). | Stored Message time, most recent reception |

## Units and Assignment

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Agency** | The operating authority of one alerting domain (for example the county fire dispatch centre or the county police), owning that domain's Units and Networks; every Unit and every Network belongs to exactly one Agency. It is the normal grant scope and feed-filter dimension above a Unit and cannot be deleted while Units or Networks belong to it. Introduced under [D-135](authoritative-plan.md#foundation-redesign-after-the-2026-11-06-re-evaluation). | Organization (defaults container), Unit collection (free-form set), Gateway, EAS Area, a single brigade (a Unit) |
| **Unit** | Logical recipient defined in GoPager (for example a fire brigade or a person's pager role) to which Addresses resolve; it belongs to exactly one Agency. | Address, Subscriber, User, Agency |
| **Organization** | Container of Units providing inherited defaults. | User group |
| **Organization default** | Typed Unit setting supplied by an Organization to Units or templates that inherit that setting; materialization on deletion: [C-04](contracts/c-04-master-data.md#ap-08-master-data-management). | Global default |
| **Unit setting override** | Explicit Unit value or explicit unset replacing an inherited Organization default; clearing the override restores inheritance. | Unit template snapshot |
| **Unit collection** | User-defined set of Units for filtering and permissions. | User group, Message |
| **Unit template** | Standalone master-data preset used to create a Unit; copied settings do not form a live template relationship. Ownership and snapshot boundary: [C-04](contracts/c-04-master-data.md#ap-08-unit-template-direction). | Presentation template |
| **Address rule** | Temporal Address/Function-to-Unit rule. Shape, specificity and revision semantics: [C-05](contracts/c-05-matching-correction.md). | Assignment, audit history |
| **Assignment** | Resolution of an original Transmission target to its current Unit, not a separate shared entity. Rules/status: [C-05](contracts/c-05-matching-correction.md). | Address rule, Message contribution, Unit appearance |
| **Message contribution** | Stored, rebuildable address-based Message read model. Identity/fields/derivation: [C-05](contracts/c-05-matching-correction.md#target-resolution-message-contributions-and-ingest-storage). | Original Transmission target, Unit appearance, Message target |
| **Contribution support** | Evidence relationship supporting a Message contribution from original reception evidence. Cardinality/targetless constraints: [C-05](contracts/c-05-matching-correction.md#target-resolution-message-contributions-and-ingest-storage). | Message membership, original evidence |
| **Suppression** | Rule that hides selected Transmission targets from projections, whether assigned or unassigned. | Permission, Payload masking |
| **Rematching** | Re-evaluating original targets at their Observation times after relevant rule/Unit/Network changes, updating stored resolutions, Contribution supports and Message contributions. Never fabricates Transmission evidence or changes Grouping. | Grouping |
| **Assignment revision** | Correction-state marker for current target resolutions and contribution derivation. Concrete revision fields/placement belong to owner contracts; not a mandatory per-entity counter or immutable rule history. | Rule Validity time, Grouping policy revision |
| **Rematch job** | Durable correction work for an affected scope/revision; lifecycle in [C-05](contracts/c-05-matching-correction.md). | Delivery job |

## AP-08 Unit Template Addendum

The [D-125 term addition](authoritative-plan.md#ap-08-unit-template-ownership) supplements G-1.0 with the Unit template term only. It does not change any existing G-1.0 meaning or the separate Presentation template term.

## Identity, Access, and Delivery

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Principal** | An internal identity (human or technical account, Guest, Worker) acting in GoPager. An Access key acts as its owning Principal with additional restrictions, not as an independent source of rights. | Unit, Gateway, Access key |
| **User** | A human Principal with an account. | Unit, Subscriber |
| **User group** | Set of Principals used for granting permissions. | Unit collection, Group call |
| **Grant / Capability** | Permission to see data or perform an action; Capabilities cover actions such as changing the grouping configuration. | — |
| **Field permission** | Global field-visibility permission for a Principal or User group, independent of data/action scopes. Definition and implemented type: [C-06 identity-fields1.0](contracts/c-06-identity-authorization.md#unscoped-field-permission-replacement-candidate). | Scoped data/action Grant |
| **Subscription** | A Principal's standing request to be informed about matching Messages or other Domain events. | — |
| **Subscriber** | The Principal owning a Subscription. | Unit, Address |
| **Delivery target** | A configured destination instance (for example a push device, email address, webhook) used by a delivery plugin. | Message target, Address |
| **Delivery plugin** | Separately provided plugin executable, run by the plugin host as a managed child process (D-131), that performs Deliveries to one kind of Delivery target (for example Pushover, Telegram messenger, email, webhook). | Gateway adapter |
| **Delivery** | One outbound communication to one Delivery target caused by a Subscription; executed through **Delivery jobs** and **Delivery attempts**. | Message, Transmission |
| **Domain event** | Internal, recorded fact about a state change (for example Message created, Transmission joined Message, Messages merged). | EAS event code, operational incident |
| **Operational Domain event** | Domain event about an operational problem that administrators can subscribe Deliveries to, for example rejected Submission, implausible Observation time, or ambiguous Network. | Incident, Gateway state |
| **Operational log** | Administrator-visible record of operational problems (rejected Submissions, implausible Observation times, ambiguous Networks). Separate from the audit trail of configuration changes. | Audit trail, Transmission evidence |
| **Access key** | Revocable bearer credential acting in its owner's name, optionally restricting actions, fields, scopes, age, and lifetime. | Principal, Recovery proof |
| **Personal Subscription** | Subscription acting as its owning User within that User's delegable rights. | Global Subscription |
| **Global Subscription** | Administratively managed Subscription acting as an explicitly assigned technical Principal, within its delegated rights. Never implicitly acts as an unrestricted administrator. | Personal Subscription |
| **Delivery input** | Bounded, materialized, authorized data computed by Core for a Delivery plugin, including plugin-declared fields and rendered template output; independently usable without reading the original Message. | Original Payload, unrestricted object copy |
| **Fact transport reference** | Internal pointer to a canonical Domain event for durable transport, not another fact history or authorization grant. The Delivery domain and integration own [C-08's publication subset](contracts/c-08-events-jobs.md#postgresql-fact-publication); introduced under D-94. | Domain event envelope, Delivery job, consumer receipt |
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
| **Processing instance** | Separately revisioned administrative binding of a provisioned pure processing capability, its execution Principal and configuration; optionally binds an independent pure preview. Owner: [C-10](contracts/c-10-plugin-host.md#installed-processing-invocation). | Workflow, module artifact, Delivery target |
| **Plugin host** | Core runtime that verifies Operator-provisioned plugin executables, starts each configured plugin instance as a managed child process (D-131), invokes it over the published protocol, and records bounded outcomes. Trust does not imply a hostile-code sandbox guarantee. | External plugin process |
| **Plugin instance** | One separately configured and revisioned use of a plugin (for example one Pushover application token, one printer or one SMTP server). Several instances of the same plugin are independent and run in separate processes. | Plugin artifact, Processing instance |
| **Host capability** | SDK-mediated ability declared by a plugin and explicitly approved for an instance, including optional network, filesystem, or command access. | Domain Capability/Grant |
| **Secret reference** | Versioned reference to an instance-bound confidential configuration value in the Core Secret store or an external provider. | Plaintext configuration |
| **Secret store** | Core resolution boundary for encrypted local secrets and externally supplied secrets, including mounted Kubernetes Secret files or environment values; external secrets need not be copied to the database. | Backup |
| **Extension field** | Plugin-namespaced, typed, versioned field attached to a supported Core entity (initially Unit), with explicit read/write permissions and no automatic rights. | Assignment, arbitrary database column |
| **Workflow** | Versioned ingest-processing definition selecting Transmissions and composing bounded parsing/mapping/branching steps or trusted installed code steps. Multiple matching Workflows may run independently. | Subscription, Delivery |
| **Workflow definition version** | Immutable saved selector/step/output-schema definition referenced by a Workflow's active pointer and source-bound Processing results. Saving is not activation or historical reprocessing. | Mutable Workflow revision, Address-rule validity |
| **Processing status** | Per-Transmission marker (`none`, `pending`, `completed`, `failed`) derived from the candidates of the currently selected Workflows. Accepted evidence is visible from intake; Workflow results attach to the Message when they complete, and `processing.completed` is the trigger for result-dependent reactions. Replaces the ingest publication barrier under [D-135](authoritative-plan.md#foundation-redesign-after-the-2026-11-06-re-evaluation). | Provider acceptance, whole-Message retention |
| **Workflow selector** | Explicit trigger and correlated domain/Payload predicate deciding whether a Workflow runs for a particular Transmission. | Grant |
| **Workflow candidate work** | Durable evaluation work for a scope-eligible Workflow; its admission does not prove that the selector matched. The Processing and Delivery domains own its [approved lifecycle](contracts/c-08-events-jobs.md#processing-durable-selection-placement-review), introduced under D-75. | Selected Workflow execution, published Processing result |
| **Execution generation proposal** | Unapproved D-77 alternative for separately identifiable evaluation work under the stable logical candidate key. The Processing and Delivery domains own the [reselection binding review](contracts/c-08-events-jobs.md#processing-same-input-reselection-review); it is not the current adopted schema or a second queue. | Candidate revision, Fencing token, automatically renewed retry budget |
| **Processing result** | Typed, namespaced derived fields and status from one Workflow version and input Transmission. A field such as `level` is data, not a Domain event. | Original Payload, Domain event type |
| **Target-result binding candidate** | Stable historical heading for [C-11's approved target-result binding](contracts/c-11-processing-presentation.md#target-result-binding-candidate): target-correlated field sets inside one Processing result, not a new original target/Message. | Per-target Transmission splitting, field permission scope |
| **Presentation template** | Versioned, permission-filtered rendering definition using an established template engine's constrained logic and escaped data interpolation. | Arbitrary browser script |
| **Text key** | Stable identifier in the English text catalog, mapped to typed parameters and optional plural selection. | Domain event/error code |
| **Field set** | Explicit set of readable domain field categories; computed and plugin-defined fields receive no rights merely by existing. | Scope |
| **Scope** | One explicit permission or filter dimension and its concrete references; an absent/deleted restricting reference never means unrestricted access. | Workflow selector |
| **Page token** | Opaque URL-safe continuation reference, bound to a query and Principal context, never proof of authorization. | SSE resume token |
| **SSE resume token** | Opaque stream-position reference used as SSE `id`/`Last-Event-ID`, bound to a permitted view and current revision checks. | Domain event ID, Page token |
| **Lease / Fencing token** | Time-limited job claim and increasing ownership generation; state writes require the current unexpired generation. | External exactly-once guarantee |

## AP-02 Identifier Registry

Exact identifier spelling, schema and behavior have the same owner. This section is a directory, not another field/enum/operation catalog. Nested wire fields are not new domain entities; names must still follow the vocabulary above.

| Identifier family | Authoritative definition |
| --- | --- |
| Cross-contract transport/reference conventions | [Shared schemas](contracts/shared-conventions.md#ap-02-common-schemas-and-limits) |
| Submission, Gateway operations and acceptance fields | [C-01](contracts/c-01-transmission-acceptance.md), [C-02 Protocol fields](contracts/c-02-protocol-payload.md) |
| Membership/representative and internal Message fields | [C-03](contracts/c-03-message-grouping.md) |
| Target resolution, contribution/support and correction fields | [C-05](contracts/c-05-matching-correction.md) |
| Grant scopes, global field permissions, field sets and exact action names | [C-06](contracts/c-06-identity-authorization.md) |
| Query/live fields, errors/codes and API fallback mappings | [C-07](contracts/c-07-query-live.md) |
| Domain event envelope/type names and job fields | [C-08](contracts/c-08-events-jobs.md) |
| Delivery state and input/attempt fields | [C-09](contracts/c-09-delivery.md) |
| Plugin namespaces and Extension fields | [C-10](contracts/c-10-plugin-host.md) |
| Workflow/result fields, text-key namespaces and English entries | [C-11](contracts/c-11-processing-presentation.md) |
| Account cleanup fields/states | [C-13](contracts/c-13-account-lifecycle.md) |
| Operation names | Respective [C-ID owner](domain-contracts.md#contract-register) |

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
| **Observation time** | Reported time at which the Gateway received the Emission. Format/validation: [C-01](contracts/c-01-transmission-acceptance.md); Grouping use: [C-03](contracts/c-03-message-grouping.md). |
| **Clock-skew tolerance** | Configurable future-time plausibility boundary. Default/effect: [C-01](contracts/c-01-transmission-acceptance.md). |
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

Storage is not a glossary responsibility. Use the [architecture ownership/relationship map](architecture.md#logical-relationship-model) and follow its owner-contract links. This retained heading is a navigation alias for earlier links, not a second storage inventory.

## Maintenance Rules

These rules bind every work package and every agent or person working on GoPager:

1. **Single source.** Domain meanings are defined here. Exact wire identifiers/schema and operation names are defined only in their owner contracts, consistently derived from these terms; do not maintain parallel field/enum registries here.
2. **New names.** Whoever needs a domain or cross-module name that is not listed adds it here with definition and "Not to be confused with" before or together with its first use.
3. **Collisions and renames.** Whoever detects a collision, an ambiguous use, or a need to rename records it in the [rename log](#rename-log) (old name, new name, meaning, reason, date, work package) and updates the entry above. A rename never silently reuses an old name for a different meaning; the old name is added to the prohibited list where appropriate.
4. **Propagation.** After adding or renaming a term, the same work package must search all documents, code, schemas/migrations, API/event definitions, tests, fixtures, and UI texts for the affected names and either update them or record the remaining occurrences as an explicit follow-up with owner. Frozen contracts are changed only through the contract change process (README change rules); the glossary entry then notes the pending contract version.
5. **Historical texts.** The rename log is chronological provenance; later rows may supersede earlier intermediate names. Only the current entries above define terminology. Git history retains obsolete discussions; do not restore them as implementation guidance.

## Rename Log

| Date | Old name(s) | New name | Meaning / reason | Package |
| --- | --- | --- | --- | --- |
| 2026-11-06 | DOM-01..DOM-08 | Intake, Grouping, Master data, Assignment, Identity, Query, Delivery, Processing | Numbered domain IDs replaced by fixed names usable in code (`producer.domain`) and permanent documentation. | Documentation |
| 2026-10-26 | Mandatory Message aliases / enduring Message identity | Message merge without identity-continuity requirement | D-40 withdraws old-link and ID-mapping requirements; authorization-relevant identities and operation-specific deduplication are unaffected. | AP-05 |
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
| 2026-10-25 | FLEX Source-code variants implicitly treated as tone-only | FLEX short signal | D-34 distinguishes signal-bearing Payload from empty tone-only Payload; C-02 owns its wire/form mapping. Earlier execution evidence is preserved. | AP-04 |
