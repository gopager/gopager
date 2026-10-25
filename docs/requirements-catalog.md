# GoPager Domain Requirements Catalog

Date: 2026-10-24. Version: 2.0. 132 stable identifiers; REQ-088 is removed and reserved.

This document owns user goals and observable acceptance outcomes only. delivery status belongs exclusively to the [authoritative plan](authoritative-plan.md#delivery-scope). Technology choices and domain boundaries belong to the [architecture](architecture.md); contract details to the [contract catalog](domain-contracts.md). Documented requirements are not implementation or operational evidence.

## Reading Rules and Provenance

U = explicit user request; B = originating from existing behavior/documentation, not automatically mandatory or production evidence; Z = later goal. These historical provenance codes remain stable and are not delivery status. 'Authorized users' includes guests, people, and technical users where permissions allow. 'May' presupposes a granted permission, not unrestricted administrator authority.

Imported from the consolidated planning state dated 2026-10-23 with stable IDs and domain statements preserved; product name updated. REQ-121 incorporates the explicit addition of EAS. REQ-122 includes the confirmed shared configurable sliding Grouping window. REQ-097 now includes English-first internationalization preparation per D-20. Historical freedom of technology choice describes the product requirement, not a reopening of the selected architecture. English is the authoritative language of this catalog; no parallel translated catalog is required.

## Personas and Main Goals

- Observers, including guests: promptly find and read relevant Messages and recognize changes.
- Unit/Gateway managers: establish correct Transmission target assignments and visibly functioning Gateways.
- Delivery users: explicitly select which Domain events go where with which permitted Payload.
- Account/permission managers: grant, restrict, and revoke access transparently.
- Operators: resource-efficient simple operation or coherent, scalable HA operation.
- Extension authors: provide additional Domain events, processing, and Delivery capabilities.

## A. Live Message View

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-001 | As an observer, I have a continuously current Message view. | Newly received permitted Messages appear without manual reload; live operation is the normal state. | U/B |
| REQ-002 | As an observer, I recognize changes to already displayed Messages. | New Units or further visible Transmission information also update older loaded Messages. | U |
| REQ-003 | As an observer, I clearly recognize new and changed entries. | Highlighting distinguishes the change type and fades continuously with its age, independently of whether it has been read. | U |
| REQ-004 | As an observer, I can browse further back in history. | Older results can be loaded continuously without losing the selection. | U |
| REQ-005 | As an observer reading older Payload, I do not unknowingly lose its freshness. | Already loaded older Messages remain updated with reasonable effort. A configurable live-age limit is permitted if needed; non-live history must then be identifiable and reconciled when shown again. Two hours is typical change practice, not a mandatory limit. | U, clarified |
| REQ-006 | As an observer, I recognize interrupted updates. | The view indicates connection/freshness loss rather than continuing to imply it is current. | U/B |
| REQ-007 | As an observer, I regain a correct state after interruption. | Reconnection reconciles the selection, including entries deleted or no longer permitted in the meantime. | U |
| REQ-008 | As a user, I can keep several views open simultaneously. | Multiple tabs with different filters work concurrently, including multiple users sharing an internet connection. | U |
| REQ-009 | As an observer, I can interpret the time and age of a Message. | Permitted time information is unambiguous; larger gaps can be shown. | U/B |
| REQ-010 | As an observer, I can navigate to a time and back to the current view. | A time anchor or returning to the latest entries preserves selected filters. | B |

## B. Search, Filters, and Details

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-011 | As an observer, I find Messages by their Payload text. | Literal search text matches substrings case-insensitively in permitted Payload. Unicode characters are compared consistently; accents remain distinct. Empty search text applies no Payload-text restriction. Search exposes nothing beyond current permissions. | U/B, clarified |
| REQ-012 | As an observer, I select one or more Units. | Messages associated with at least one selected Unit are found. | U/B |
| REQ-013 | As an observer, I find Messages without an assigned Unit. | One installation-wide 'Without Unit' selection works, including in combination with Network filters. | U |
| REQ-014 | As an observer, I filter by Networks, organizations, Protocols, and permitted Gateways. | Selecting visible characteristics restricts results understandably. | U/B |
| REQ-015 | As an observer, I combine selections. | Alternatives within a dimension mean 'any of these'; different dimensions must match together. | U |
| REQ-016 | As a user, I can revisit or link to a view. | Reopening restores recognizable selection and time context where still permitted. | U/B |
| REQ-017 | As an observer, search attempts reveal no hidden information. | Unreadable characteristics cannot be reconstructed through selection, sorting, counts, or repeated searches. | U |
| REQ-018 | As an observer, I can inspect a Message in detail. | Details coherently show permitted Payload, Units, and Transmission information. | U/B |
| REQ-019 | As an authorized manager, I can edit or create a matching Unit from Message details. | The observed Address may be prefilled; this action is not additionally shown in the compact list. | U |
| REQ-020 | As a manager, I can create a more precise assignment from a broad match. | Where supported by the Protocol, I select the appropriate domain variant; the application explains its effect before saving. | U |

## C. Units, Organizations, and Defaults

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-021 | As a manager, I maintain Units that can be alerted. | I can create, name, edit, and delete where authorized. Deleting a Unit rematches all affected historical Messages using remaining assignment rules; old references do not remain stale. Transmission evidence is retained; rematching is neither a new Transmission nor Grouping. | U, confirmed 2026-10-23 |
| REQ-022 | As a manager, I recognize Units by name, organization, color, and icon. | These characteristics are used consistently in management and Message views. | U/B |
| REQ-023 | As a manager creating a Unit, I can also create its organization. | I do not leave the workflow; missing permissions are explained. | U |
| REQ-024 | As a manager, I use shared defaults. | Units may inherit organization defaults and override them selectively; provenance and returning to the default are visible. | U |
| REQ-025 | As a manager, I create new Units from templates. | Templates may supply organization, color, icon, and appropriate Protocol-specific defaults. | U |
| REQ-026 | As a manager, I efficiently change multiple Units. | Suitable shared fields and deletions can be handled in bulk, with clear previews and error feedback. | U |
| REQ-027 | As a manager, I navigate from a Unit to its Messages. | The Message view opens with the appropriate permitted selection. | U |
| REQ-028 | As a manager, I organize Units by location, organization, type, and special formation. | A Unit may belong to multiple Unit collections, including across Networks. | U |
| REQ-029 | As a user, I independently combine domain characteristics. | Search supports, for example, all fire engines across all towns or all fire-service vehicles in one town: AND between dimensions, alternatives within a dimension. Gateway/Unit conditions must match the same individual Transmission; different Transmissions of one Message are not mixed across dimensions. Unit collections are also available as additive permission scopes; combined intersection scopes under REQ-054 are optional reuse, not mandatory. No unrestricted multi-parent default graph. | U, search and permissions separated |
| REQ-030 | As a manager, I can represent a formation of multiple Units. | Members are discoverable; membership alone does not claim they were actually alerted together. | U/Z |

## D. Correct Assignment and History

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-031 | As an operator, I obtain traceably correct Unit matches for Transmission Addresses. | Supported Protocols follow their respective domain rules. | U/B |
| REQ-032 | As a manager, I combine broad and specific assignment rules. | The most specific unambiguous match wins; equally specific or partially overlapping simultaneously valid rules are rejected with a conflict explanation. | U |
| REQ-033 | As a manager, I limit technical Address-to-Unit assignments in time. | The same technical Address may belong to different Units sequentially; boundary times are unambiguous. Temporal validity concerns this assignment, not historical versions of colors, icons, or general master data. | U, narrowed |
| REQ-034 | As an operator, historical assignments remain correct. | Relevant changes rematch affected existing Messages using their Observation time. | U |
| REQ-035 | As a manager, I can make several changes in succession. | When processing is available, relevant rematching is triggered after 60 seconds of inactivity by default; further changes must not extend this delay beyond 300 seconds by default. Both are configurable; starting and fully completing are distinguishable. | U, clarified |
| REQ-036 | As an operator, relevant changes or deletion correct affected historical assignments. | A color-only change does not alter assignments; relevant creation/change/deletion misses no potentially affected Message. Unit deletion rematches all affected Messages. Start/completion/failure are visible; rematching creates neither Transmission evidence nor regrouping. | U, confirmed |
| REQ-037 | As a manager, I assign my own meanings to function/signal codes. | Different organizations/Units may interpret the same received codes differently. | U/B |
| REQ-038 | As a manager, I use Protocol-specific defaults for multiple Protocols. | Appropriate function/signal labels are offered without assuming a false nationwide standard meaning. | U |
| REQ-039 | As a manager, I maintain assignments only within permitted Networks. | Other filters are not exposed; hidden additional use prevents unauthorized deletion and is explained generically. | U |
| REQ-040 | As an operator, technical ambiguity is not presented as a certain match. | An undecidable/failed assignment attempt is distinguishable from 'no matching Unit'. | B |

## E. Transmission, Networks, and Gateways

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-041 | As an operator, I connect multiple submitting Gateways. | Each Gateway is identifiable, authorized, and individually blockable. | U/B |
| REQ-042 | As an operator, a Gateway can monitor one or more Networks. | Allowed Networks are explicitly restricted; each Network has exactly one domain Protocol. | U |
| REQ-043 | As an operator, a known Network declaration is respected. | A valid unambiguous Gateway declaration takes precedence over inference; an unauthorized declaration is not silently replaced. | U |
| REQ-044 | As an operator, I can use Gateways without certain frequency/Network knowledge. | Assignment may use the allowed context; a simple single-Network Gateway does not require unnecessary range information. | U |
| REQ-045 | As a manager, I recognize and resolve assignment conflicts. | Conflicts identify permitted affected rules/Units and explain how to resolve them. | U |
| REQ-046 | As an operator, unknown assignments are not silently lost. | Unresolved Networks/Units are visible and separately authorizable; unauthorized submission remains rejected. Rejected Submissions, implausible Observation times, and ambiguous Networks appear in the Operational log and as subscribable Operational Domain events (C-01). | U |
| REQ-047 | As an operator, I know whether a Gateway is still working. | Management shows never seen, active, degraded, deliberately stopped, or connection lost, with appropriate times. | U |
| REQ-048 | As an operator, I distinguish planned termination from unexpected failure. | Deliberate termination differs from missing contact; no specific crash cause is claimed without evidence. | U |
| REQ-049 | As an operator, I react to Gateway-state changes. | Authorized users may use suitable state Domain events as Delivery triggers. | U/Z |
| REQ-050 | As an operator, I see the last known Transmission. | The display distinguishes Transmission, heartbeat, and missing or already removed history. | U |

## F. Access Rights and Guests

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-051 | As a permission manager, I grant permissions to people and User groups. | Direct exceptions are visible for a user without requiring an artificial helper User group. | U |
| REQ-052 | As a user, I inherit permissions from multiple User groups. | All matching grants combine; reading everywhere and editing only a particular scope is possible. | U |
| REQ-053 | As a permission manager, I can later nest related User groups. | Initial implementation uses flat multiple User group memberships plus direct grants; later inheritance remains explainable and cycle-free. Unit collections are independent. | U/Z, deliberately later |
| REQ-054 | As a permission manager, I restrict permissions to domain scopes. | Matching grants are additive; I need not grant a right in every dimension. Individual/all Networks, Units, Unit collections, and specific accounts are distinguishable where meaningful. Combined intersection scopes are an optional low-cost reusable addition, not initially mandatory. | U, clarified |
| REQ-055 | As a permission manager, I determine which Payload is visible. | Message, Address, Gateway information, and other co-alerted Units can, for example, be granted separately. | U |
| REQ-056 | As a permission manager, I can expose or hide unassigned Messages. | 'Without Unit' requires an explicit grant, not automatic access. | U |
| REQ-057 | As an operator, I configure fully usable guest access. | Without login, live views, search, and filters work under assigned permissions; initially none are granted. | U |
| REQ-058 | As an operator, I may also grant guests write capabilities. | The same domain permissions are available; public access receives an explicit warning; the guest subject cannot be deleted. | U |
| REQ-059 | As a permission manager, I can limit access to recent Message history. | One shared age limit restricts the Message and associated Delivery details; expired data is not newly delivered. | U, optionally simplified |
| REQ-060 | As a user, I understand my permissions and their provenance. | Effective permissions and direct/User group assignments are explainable. Access follows current Unit assignments, including history, without an extra warning/special audit. Deleting a restricting scope object must not unintentionally make the remainder of a grant unrestricted. | U, clarified |

## G. Accounts and Login

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-061 | As an operator, I manage local and technical accounts without manually duplicating external users. | An internal profile may have local and multiple external login identities; OIDC-only/SCIM-managed profiles appear as one user, with visible attribute/lifecycle provenance. Suspension affects access paths and Delivery according to their source of authority. | U, OIDC/SCIM later |
| REQ-062 | As a user, I can delete my own account. | After confirmation, login, identity bindings, and subscriptions end. Personal historical references are anonymized; concrete receipt/audit fields and retention are specified before implementation. Unit/organization deletion has a separate lifecycle. | U, confirmed |
| REQ-063 | As a user, I maintain my own profile. | Permitted names/contact details can be changed; contact information alone is not proof of identity. | U/B |
| REQ-064 | As a user, I can log in with a local password. | Permitted valid login works; a suspended account or incorrect password does not. | U/B |
| REQ-065 | As a user, I can log in locally, through OpenID Connect, or both. | OIDC is not Phase 1, but the identity model is prepared. Verified issuer+subject identities map to an internal GoPager user ID; email/username are not identity keys. Permissions/actions reference the internal ID, not raw claims or external tokens. An OIDC-only Principal can be created automatically under controlled minimal rules without a manually maintained duplicate. Adding a local password later extends the same Principal, not a second user. | U/Z, boundary prepared |
| REQ-066 | As an operator, I can later use trusted upstream authentication. | Proxy authentication is deferred from the initial release; later it uses trusted assertions only, not a direct identity bypass. | B/Z |
| REQ-067 | As an authenticated user, I can later securely link my OIDC identity to my existing account. | Linking requires confirmed authentication to the existing account and fresh successful OIDC authentication. Matching email alone never links accounts. Multiple login identities then belong to exactly one internal user. | U/Z |
| REQ-068 | As an authorized user, I can later unlink my external login. | Removal requires appropriate permission and must not unknowingly remove the last working login path; explicit administrative revocation and IdP outage are distinguishable. | U/Z |
| REQ-069 | As a permission manager, I allow or exclude local password login. | The same permission covers setting/changing one's password; recovery does not bypass excluded local login. | U |
| REQ-070 | As a user, I recognize and revoke logged-in devices/sessions. | A revoked device cannot continue fetching new private data under old authorization. | B/U |

## H. Recovery and Access Keys

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-071 | As an authenticated user, I change my password through the normal workflow. | The old password and confirmed new password are checked; administrators cannot inspect password values. | U/B |
| REQ-072 | As a user who forgot my password, I can request recovery. | Without a suitable secure Delivery path, a request is created; unknown accounts are not disclosed. | U |
| REQ-073 | As an authorized manager, I enable recovery without knowing the new password. | A short-lived one-time link/code can be passed on; administrators do not set a known password. | U |
| REQ-074 | As a user, I can use a recovery proof only for my own account. | The correct username is additionally required; reuse and expired proofs fail. | U |
| REQ-075 | As an operator, I can initiate recovery and emergency access locally. | Local administrative rescue remains possible without any remaining web administrator; existing accounts are not silently replaced. | U/B |
| REQ-076 | As a permission manager, I allow recovery directly or by request only. | Own permissions and target accounts/User groups determine access; direct administrative issuance and approved requests follow the same domain process. | U |
| REQ-077 | As a user with permission, I create my own access keys. | Others do not create personal keys for me; a key can never exceed its owner's permissions. | U |
| REQ-078 | As a manager, I create targeted keys for technical accounts. | Individual technical accounts or all can be authorized; different keys may have different restrictions. | U |
| REQ-079 | As an authorized user, I rotate or revoke access keys. | Permissions are target-specific; creation and last use remain traceable. | U |
| REQ-080 | As an operator, I can later manage external identities and access transparently. | OIDC/SCIM lifecycle prevents externally disabled/deleted external-only accounts from retaining access/Deliveries indefinitely. Removing an external identity does not automatically end a still-valid local login for the same internal user. No Phase-1 requirement for OIDC reauthentication timers or warning stages. | U/Z |

## I. Domain events, Extensions, and Deliveries

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-081 | As a user, I select which Domain events should trigger a Delivery. | Subscriptions select permitted Domain events, such as Message creation, individual Transmissions including repeats/multiple Gateways, assignments, or corrections. No hidden default of only Message creation or once per Message. Domain filters restrict the selected Domain event. | U, clarified |
| REQ-082 | As a user, I select Delivery targets and Payload. | Payload and Delivery target are configurable, never beyond my own data permissions. | U/FIX |
| REQ-083 | As a user, I receive one extension's Domain events through another Delivery target. | For example, an externally determined state can trigger a permitted push/webhook Delivery. | U/Z |
| REQ-084 | As a manager, I configure multiple instances of the same Delivery target. | Different accounts/targets/settings remain separate; secrets are not disclosed again. | B/U |
| REQ-085 | As a user, I pause or remove my subscriptions. | New Deliveries stop; suspension/deletion of the owner has the same effect. | U/B |
| REQ-086 | As an authorized user, I see Delivery results/problems and can retry transient failures. | Success, pending retry, failure/expiry, and uncertain outcomes are distinguishable. Retries are wanted; compare recovery triggers versus dead-letter/retention before technical approval. No endless retries of permanent failures or blind duplicate effects after uncertainty. | U/B |
| REQ-087 | As an authorized user, I can deliberately trigger Delivery again. | Potential duplicate effects are explained; uncertain effects are not repeated without permission. | B |
| REQ-088 | REMOVED: promised compatibility with old integrations. | Removed on 2026-10-23, including a required legacy adapter and old special trigger logic. The ID remains reserved. New plugins distinguish Domain events; permitted subscriptions control reactions. | explicitly withdrawn |
| REQ-089 | As an operator, I deliberately install and activate trusted extension code. | Extensions are separate modules provided from suitable package sources/repositories or locally. Operators recognize incompatibility, enable/disable modules, configure them, and see status/errors. Only operators install/update code; web administrators configure offered capabilities; users manage permitted subscriptions. No programming-language/npm mandate or mandatory untrusted-code sandbox. Update/rollback details are technical contracts; no in-app marketplace or uncontrolled remote installation is assumed. | U |
| REQ-090 | As an extension author, I use a defined SDK/API to offer Domain events, reactions, and Delivery capabilities. | Data models, interfaces, versioning, errors, and permitted host capabilities are clearly documented and testable. Initial usable Delivery includes separate Pushover, Telegram, email, and webhook plugins. Multiple implementation languages are evaluated as an option, not yet a guaranteed capability. | U |

## J. Processing, Presentation, and Usability

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-091 | As a manager, I process text and reactions in freely composed automations. | Minimum value: parse Payloads, extract by regex, produce administratively configurable structured data, and react using conditions/branches. Flexible composition like n8n/Node-RED/Home Assistant is preferred; basic branches are acceptable initially if effort would be disproportionate. `essen.js` is a functional reference, not a mandate to adopt code/runtime. It was not found in previous planning, so no functional parity is claimed. Regex execution must not block processing without bounds. Evaluate suitable builder libraries during technology/frontend selection; define the contract before package approval. | U |
| REQ-092 | As a manager, I test processing rules using examples. | Preview explains results/errors without silently changing real Message data or settings. | B |
| REQ-093 | As a manager, I recognize the active processing version and can return to a known version. | Saving and actual activation are clear; selecting an earlier version is traceable. No mandatory multistage approval workflow. | B |
| REQ-094 | As a manager, I design Message presentation. | permitted Payload can be arranged, highlighted, and displayed conditionally; presentation grants no additional data permissions. | U/B |
| REQ-095 | As an editor, I receive helpful errors and do not lose others' changes. | Invalid input is explained at the relevant field; concurrent editing produces a conflict instead of silent overwriting. | B |
| REQ-096 | As an operator, I maintain installation title and notices. | Users see published notices and can dismiss them under an explained rule; no hidden requirement to execute HTML. | U/B |
| REQ-097 | As a user, I choose time/locale formatting, time gaps, and Delivery preferences in an English-first product prepared for internationalization. | Preferences persist across reopening. English is the authoritative project and initial UI language. User-facing product text is prepared for localization through stable keys and an English text catalog, with parameter/plural handling and a defined English fallback. Formatting locale is independent of interface language. No additional translations, language packs, or language-selection UI are required yet; received Payload text and operator-authored data are not automatically translated. | U/B; explicit language/i18n decision |
| REQ-098 | As a user, I copy Payload and open/close details without losing my selection. | Copying does not change the Message; navigation preserves the working context. | B |
| REQ-099 | As a user, I operate feed and management on phone and desktop. | Text/controls do not overlap; essential workflows are accessible by touch and keyboard. | U/B |
| REQ-100 | As a user, I do not depend solely on color or a mouse. | Changes, errors, and actions are readable/labeled and keyboard-accessible. | B, quality proposal |

## K. Installed Web App and Security Behavior

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-101 | As a user, I can use the application as an installed web app. | A familiar launch point and standalone use on supported devices are possible. | U/Z |
| REQ-102 | As a user, I receive configured Deliveries without an open feed. | Supported devices show permitted configured Payload; an open tab is not required. | U/FIX/Z |
| REQ-103 | As a user, I need not log in at every launch on my own device. | Login remains usable across launches but is individually revocable and not guaranteed indefinitely. | U/Z |
| REQ-104 | As a user without connectivity, I know private Message data is not currently available. | Offline mode does not present old state as current; full offline operation is not assumed. | U/Z |
| REQ-105 | As a user, my Delivery target receives only Payload that remains permitted. | Deferred Delivery/retries consider revoked permissions, account/device state, and subscription changes. | U/FIX |
| REQ-106 | As an operator, I recognize external Delivery limits. | Delivery-path acceptance is not confirmation of reading; already delivered Payload is not promised to be safely recallable. | B |
| REQ-107 | As a user, my credentials and confidential Payload are protected. | Normal views, errors, logs, and exports expose no unauthorized secrets. | U/B |
| REQ-108 | As an operator, abusive login/search/sending attempts remain bounded. | High attempt rates cause explained throttling without unnecessarily blocking a shared location. | U |
| REQ-109 | As a manager, I can trace security-relevant changes. | Authorized users see who changed what and when, without recording passwords or secret Payload. | B/U |
| REQ-110 | As a user, I understand denied access without learning others' data. | Login required, action forbidden, and invisible individual object are handled sensibly. | U |

## L. Operations, Data Volume, and Evolution

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-111 | As an operator, I limit stored Message history. | Retention by age or maximum count is configurable; until a limit is set, retention is unlimited and nothing is automatically deleted. | U |
| REQ-112 | As a user, I recognize potentially incomplete history caused by retention. | For example, empty Gateway history explains that earlier history might no longer exist. | U |
| REQ-113 | As an operator, I distinguish retransmission from independent Transmission. | Resubmitting the same operation creates no unintended copy; an independent further Transmission remains traceable. | B/U |
| REQ-114 | As an operator, promised acceptance status remains honest. | An acknowledged operation has reached the storage/processing stage explained for its mode; errors are verifiable. | U/B |
| REQ-115 | As a small-installation operator, I can run the application simply and locally. | Lite is the same GoPager as Full and must share its codebase; two separately developed applications are prohibited. Selecting both modes in the same compiled artifact is preferred. Lite runs in one container and as one OS process bare-metal with required core functions retained. The product requirement itself dictates no programming language, plugin format, or storage, but permits no mandatory separate daemon for bare-metal. | U/FIX |
| REQ-116 | As a small-installation operator, I accept explained limits rather than unnecessary complexity. | No multiple-instance requirement. If substantially simpler, a crash may lose even acknowledged inputs in the crash window; this is not a loss goal and no quantitative loss budget is defined. | U |
| REQ-117 | As a large-installation operator, I can tolerate failures and scale components as needed. | HA deployment, load balancing, and separate/replicated components of the same application are possible without treating conflicting permission/data states as normal. Full shares Lite's codebase; a common compiled artifact is preferred. Storage, workers, live updates, and other relevant components are qualified together, not just API-instance count. | U/FIX |
| REQ-118 | As an operator, I can still run the application together simply. | All application capabilities may run in one container without complex orchestration; supplementary services are separate. | U/FIX |
| REQ-119 | As an operator, I can move from Lite to Full without rebuilding the installation's domain setup. | Accounts/permissions, configuration, and history remain traceable; pending work and secrets have an explained transfer path. | U |
| REQ-120 | As an operator, I receive installable, testable, maintainable operation. | Initial setup without default credentials, status checks, orderly shutdown, and traceable updates exist. No quantitative performance promises or formal measurement/acceptance thresholds are required. | U/B |

## M. Additional Domain Capabilities

| ID | User goal / story | Observable acceptance outcome | Provenance |
| --- | --- | --- | --- |
| REQ-121 | As an operator, I use POCSAG, FLEX, ZVEI, and EAS protocols with their respective characteristics. | Final Payload or completely received data, multiple Transmission targets, and associated function/signal codes are not distorted. For EAS, header fields are supplied as completed Payload and areas as Transmission targets; audio/video is not supported initially and is planned for a later version (filesystem/object storage). Further Protocols are not an implicit task. | U/B; EAS explicitly added |
| REQ-122 | As an observer, I see technically combined Transmissions as one Message. | Payload-identical Transmissions with the same Payload form (declared per Protocol definition; compatible Protocols such as POCSAG/FLEX text, numeric, or tone-only may match) within the Grouping window may appear together across independent Networks. The window slides along reported Observation instants of the Message members; its duration is shared and configurable (default 30 seconds, inclusive boundary, 0 disables Grouping); Messages bridged by a later Transmission are merged, with absorbed Message IDs forwarding to the surviving Message. Payload identity is defined per Protocol definition (C-02). The Message does not claim a common operational incident; individual permitted Transmissions, Networks, and Transmission targets remain distinguishable. | U, current clarification |
| REQ-123 | As a manager, I exclude unwanted Transmission targets from normal operation using suppression filters. | Data remains stored. Suppressed Transmission targets appear in neither normal views, search, details, nor Delivery; mixed Messages retain other usable Transmission targets. With no non-suppressed Transmission targets, the Message does not appear there. No deletion/discarding or separate Payload masking. Lifting suppression can expose existing data again but is not a new Transmission. | U |
| REQ-124 | As an observer, I recognize an Address assignment marked as encrypted. | A domain label claims neither available decryption nor automatic access to secret Payload. | B |
| REQ-125 | As an operator, I can later offer voluntary account registration. | Deferred from the initial release. Later requests create no automatic data permissions. | B/Z |
| REQ-126 | As an operator, I can back up an installation and restore operation after failure. | Configuration and domain data can be verifiably restored through a documented procedure; an update promises no automatic undo of all intervening data. | B |
| REQ-127 | As an observer/operator, I can use the application meaningfully with large history or a constrained host. | Technology decisions consider the described weak Lite hosts and differing Full profiles. No quantitative performance promises or measurement/acceptance thresholds are required. | U |
| REQ-128 | As a manager, I can later import or export many Units together. | Simple Unit import/export is deferred from the initial release. No implicit legacy CSV/database compatibility scope. | B/Z |
| REQ-129 | As a manager, I use per-Unit overrides for a Delivery target. | Authorized Unit-specific settings affect only relevant Deliveries and reveal no other target's secrets. | B/U |
| REQ-130 | As a user, I receive neither inappropriately old nor unintentionally repeated Deliveries. | Age limits, selected Domain event trigger, and duplicate handling also apply after replay, late Transmission, and reprocessing. A deliberately subscribed correction is not a duplicate of the original Delivery; an uncertain external outcome is not presented as guaranteed exactly-once Delivery. | U/B |
| REQ-131 | As an operator, I later synchronize users/User groups from a central identity directory. | The SCIM backchannel provisions, updates, and deactivates users/User groups and memberships; SCIM is not login. Administrators explicitly map external SCIM User groups to local GoPager User groups. Unmapped User groups grant no rights. Changing/removing a mapping revokes only memberships originating from it, not manual assignments. External deactivation blocks external login/Delivery; an independent local credential remains possible under the user decision. Disappearing from a feed is not deactivation/deletion. | U/Z |
| REQ-132 | As a manager, I delete an organization without deleting its Units. | Affected Units remain and are detached; their Message Transmission target assignments are not rematched as on Unit deletion. Previously effective organization defaults are preserved for detached Units. Current organization filters/permissions follow the remaining relationships. | U |