# GoPager Glossary

Date: 2026-10-25. Status: **draft** (AP-01); binding vocabulary for all documents, frozen together with the contracts in AP-04. Decisions and provenance: [plan](authoritative-plan.md), [evidence notes](requirements-evidence.md#ap-01-terminology-on-2026-10-25).

This glossary is the **single authoritative source for names** (domain terms, and the code identifiers, API fields, event names, and UI terms derived from them). Every domain term below has exactly one meaning. Terms in the [prohibited terms](#prohibited-and-restricted-terms) list must not be used as domain terms. Code identifiers, API fields, and event names follow these terms. UI labels may be localized, but they do not introduce new domain meanings.

## Radio and Gateway Side

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Emission** | One physical radio (or SMTP) send event. Not a domain object; it is received by zero or more Gateways, each producing its own Transmission. | Transmission (the record of one Gateway) |
| **Receiver hardware** | Physical radio receiver/SDR used by a Gateway. Not a domain object. | Gateway, Unit |
| **Gateway adapter** | Software type that decodes Emissions (frames, checksums, fragment reassembly, SMTP parsing) and submits completed Transmissions, for example a multimon-based adapter. Never interprets payload meaning, resolves Units, or groups. | Gateway (a running instance) |
| **Gateway** | A registered, authenticated running instance of a Gateway adapter, authorized for one or more Networks. Has its own state (contact, heartbeat, termination). | Receiver hardware, Subscriber |
| **Network** | A logical radio network or channel with exactly one Protocol, in whose context a Gateway submits Transmissions. | Protocol |
| **Protocol** | The radio or transport protocol (POCSAG, FLEX, ZVEI, EAS, SMTP-based formats). | Payload form |
| **Protocol definition** | Server-side, versioned description of a Protocol: which submitted fields are Payload, which are Address/Function, and how each protocol message type maps to a Payload form and Canonical payload. | Protocol |
| **Group call** | Protocol feature in which one Emission addresses several Addresses (for example FLEX group messaging, Swissphone Express-Alarm (product feature name), SMTP with several targets in the body). Results in one Transmission per receiving Gateway with several Transmission targets. | Message, User group, Unit collection |

## Transmission and Payload

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Submission** | One transport operation by which a Gateway delivers one Transmission to GoPager, identified by a **Submission ID**. A **Retry** repeats the same Submission and never creates a second Transmission. | Transmission |
| **Transmission** | Immutable record that one Gateway received one Emission, with the same name on Gateway and server side: Gateway, Network, Protocol, Observation time, Payload, Transmission targets, optional quality metrics and incomplete marker. One Emission received by three Gateways yields three Transmissions. | Emission, Message |
| **Payload** | The completed original information carried by a Transmission as submitted (text, numeric digits, tone-only, ZVEI double tone or its absence, EAS header fields). Stored unchanged. | Message, Canonical payload |
| **Payload form** | Protocol-independent kind of Payload declared by Protocol definitions: `text`, `numeric`, `tone-only`, `zvei-signal`, `eas-header` (extensible by versioned definition). | Protocol |
| **Canonical payload** | Payload form plus normalized value used solely for comparison (for example line breaks normalized, terminators removed). Two Transmissions have equal **Payload identity** if their Canonical payloads are equal. | Payload (original), search normalization |
| **Address** | A target address of an Emission as reported in a Transmission: POCSAG RIC, FLEX capcode, ZVEI five-tone sequence, EAS location code, or protocol equivalent. | Unit, Delivery target |
| **Function** | Address attribute such as POCSAG function bits/subaddress. Its meaning depends on the address rule (function bit or subaddress) and the Unit (**Function meaning**); never part of Payload identity. | Payload |
| **Transmission target** | One Address (with Function) reported in one Transmission. A Group call has several. | Message target, Unit |
| **Quality metrics** | Optional, already evaluated and contextualized reception-quality information supplied by the Gateway. | Gateway state |
| **Incomplete marker** | Optional Gateway statement that a Transmission was received incompletely. | — |

## Messages and Grouping

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Message** | Server-side entity formed by Grouping: one Canonical payload sent to a set of Message targets, evidenced by one or more Transmissions whose Observation times are chained within the window. The central object for users. Not an operational incident. | Payload, Transmission, Delivery, Localized text |
| **Grouping** | The server process that links payload-identical Transmissions into Messages (C-03). | User group, Unit collection, Group call |
| **Grouping window (`W`)** | Maximum gap between Observation times of linked Transmissions. | — |
| **Message membership** | Link from a Transmission to its Message, recording the policy revision and match kind. | User group membership |
| **Message merge** | Combining Messages when a Transmission links to several of them; absorbed Messages become **Message aliases** forwarding to the surviving Message. | — |
| **Message target** | One distinct Address (with Function) of a Message, aggregated from its Transmission targets, with traceability to the Transmissions that reported it. Answers "which Payload was sent to which Address". | Transmission target, Delivery target |

## Units and Assignment

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Unit** | Logical recipient defined in GoPager (for example a fire brigade or a person's pager role) to which Addresses resolve. | Address, Subscriber, User |
| **Organization** | Container of Units providing inherited defaults. | User group |
| **Unit collection** | User-defined set of Units for filtering and permissions. | User group, Message |
| **Address rule** | Temporal rule that maps an Address (with Function filter and mode) to a Unit. | Assignment |
| **Assignment** | Versioned result linking a Transmission target, and thereby its Message target, to a Unit under an Address-rule revision. | Address rule, Delivery |
| **Suppression** | Rule that hides selected Assignments from projections. | Permission |
| **Rematching** | Recomputing Assignments after Address-rule or Unit changes; never creates Transmissions or Messages and is not Grouping. | Grouping |

## Identity, Access, and Delivery

| Term | Definition | Not to be confused with |
| --- | --- | --- |
| **Principal** | An internal identity (user account, API key, guest, worker) acting in GoPager. | Unit, Gateway |
| **User** | A human Principal with an account. | Unit, Subscriber |
| **User group** | Set of Principals used for granting permissions. | Unit collection, Group call |
| **Grant / Capability** | Permission to see data or perform an action; Capabilities cover actions such as changing the grouping configuration. | — |
| **Subscription** | A Principal's standing request to be informed about matching Messages or other Domain events. | — |
| **Subscriber** | The Principal owning a Subscription. | Unit, Address |
| **Delivery target** | A configured destination instance (for example a push device, email address, webhook) used by a delivery plugin. | Message target, Address |
| **Delivery plugin** | Separately provided Wasm module executed by the plugin host that performs Deliveries to one kind of Delivery target (for example Pushover, Telegram messenger, email, webhook). | Gateway adapter |
| **Delivery** | One outbound communication to one Delivery target caused by a Subscription; executed through **Delivery jobs** and **Delivery attempts**. | Message, Transmission |
| **Domain event** | Internal, recorded fact about a state change (for example Message created, Transmission joined Message, Messages merged). | EAS event code, operational incident |

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
| Observation (as a noun for the report), Reception | Not used; "Observation time" stays. | Transmission |
| Source, Ingest module, Ingester, Receiver | Not used (Receiver collides with recipient and receiver hardware). | Gateway, Gateway adapter |
| Content (as a domain noun) | Not used. | Payload, Canonical payload |
| Message (for UI or error text) | Not used. | Localized text |
| Event (unqualified) | Not used. | Domain event, EAS event code, Incident |
| Notification | Not used as a domain term. | Delivery |

## Stored Information (Logical, Not a Data Model)

The physical data model is not prescribed and may normalize or deduplicate. The following information must be stored or exactly reconstructable:

1. **Transmission evidence (immutable):** Gateway, Network, Protocol, Submission ID, Observation time with its offset, Acceptance time, original Payload, all Transmission targets with Functions, Quality metrics, and Incomplete marker. Original Payload may be stored by reference to an identical Payload if the original remains reconstructable.
2. **Messages:** Canonical payload with identity-rule version, Message memberships (Transmission, policy revision, match kind), Message aliases with merge history.
3. **Message targets:** distinct Addresses with Functions per Message, traceable to the Transmissions that reported them; may be stored or derived from Transmission targets.
4. **Assignments:** Transmission target to Unit with Address-rule revision (C-05).
5. **Gateway state history:** contact, heartbeat, and termination over time, so administrators can tell which authorized Gateways of a Network did or did not produce a Transmission for a Message and whether a Gateway had problems.

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
