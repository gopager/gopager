# ADR: Intake and Grouping

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-17"></a>
## D-17 Grouping policy

- **Status:** accepted (amended by D-30)
- **Date:** 2026-10-25
- **Decision:** Confirmed Grouping policy; comparison scope amended by D-30.
- **Owner detail:** [C-02](../contracts/c-02-protocol-payload.md), [C-03](../contracts/c-03-message-grouping.md)

<a id="d-18"></a>
## D-18 Protocol-scope amendment

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed Protocol-scope amendment.
- **Owner detail:** [REQ-121](../requirements-catalog.md#m-additional-domain-capabilities), [C-02](../contracts/c-02-protocol-payload.md)

<a id="d-27"></a>
## D-27 Storage model superseded by D-29

- **Status:** superseded by D-29
- **Date:** 2026-10-25
- **Superseded by:** [D-29](02-intake-and-grouping.md#d-29)
- **Decision:** Storage model superseded by D-29; no independent current definition.
- **Owner detail:** [D-29](02-intake-and-grouping.md#d-29); [C-07 diagnostics](../contracts/c-07-query-live.md) retains the separate Gateway-filter boundary

<a id="d-29"></a>
## D-29 Logical model replacing D-27

- **Status:** accepted (amended by D-30)
- **Date:** 2026-10-25
- **Decision:** Confirmed logical model replacing D-27; heuristic scope amended by D-30. Physical fields/types/indexes explicitly deferred to owner design/freeze, not approved by diagram.
- **Owner detail:** [Architecture relationships/ownership](../architecture.md#logical-relationship-model), [C-01 originals](../contracts/c-01-transmission-acceptance.md), [C-03 membership](../contracts/c-03-message-grouping.md), [C-05 derivation](../contracts/c-05-matching-correction.md), [C-07 projection](../contracts/c-07-query-live.md), [C-11 results](../contracts/c-11-processing-presentation.md)

<a id="d-30"></a>
## D-30 Heuristic delivery deferred

- **Status:** accepted
- **Date:** 2026-10-25
- **Supersedes:** [D-17](02-intake-and-grouping.md#d-17), [D-29](02-intake-and-grouping.md#d-29)
- **Decision:** Heuristic delivery deferred; supersedes activation/detail proposals under D-17/D-29. Interrupted threshold exploration was not approval.
- **Owner detail:** [Scope](../authoritative-plan.md#delivery-scope), [C-02 comparison boundary](../contracts/c-02-protocol-payload.md), [C-03 current membership](../contracts/c-03-message-grouping.md)

<a id="d-34"></a>
## D-34 AP-04 FLEX Decision

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Approved AP-04 FLEX type/address clarification of D-18; preserves D-29's logical Message model. The user selected `short_numeric=numeric` rather than deferral; `source_code_identity=distinct_signal` rather than tone-only coalescing or deferral; and explicitly approved decimal-address CAPCODE clarification. These decisions preserve distinguishable signal information and avoid treating routing notation as an integer address. Behavior and exact mappings belong solely to [C-02](../contracts/c-02-protocol-payload.md#ap-04-flex-clarification), terminology to the [glossary](../glossary.md), and examples to [F-42..F-44](../contracts/reference-cases.md#ap-04-protocol-cases).
- **Owner detail:** [C-02](../contracts/c-02-protocol-payload.md)

<a id="d-40"></a>
## D-40 Transmission and Message Architecture Reassessment

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Enduring Message identity, alias and ID-mapping requirements removed; neutral Transmission/Message architecture assessment. Shared conventions own the identity rule; C-03 0.3-draft and C-07 remove survivor/alias/old-link requirements, with matching C-08/C-09/C-12, REQ-122, F-07 and glossary updates. This supersedes D-39's retained initial alias requirement and the corresponding identity portion of D-17/D-29, not their historical evidence. Correct retry/deduplication and reference validity during an operation are not enduring Message-link guarantees.
- **Owner detail:** This record is the only current definition

<a id="d-42"></a>
## D-42 AP-06 Observation Precision Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved whole-second Observation precision by truncation; supersedes finer-precision preservation for the AP-06 boundary. The considered approaches were rejecting finer input, retaining finer instants with a lossless cross-store representation, or truncating fractional seconds to the whole second. The user selected truncation: discard fractional digits, do not round, and store/use the resulting whole-second instant; preserve the supplied UTC offset. Gateways should preferably send whole-second values. A zero-only fraction such as `.000` remains equal to its whole-second form.
- **Owner detail:** [C-01](../contracts/c-01-transmission-acceptance.md)

<a id="d-43"></a>
## D-43 AP-06 Transmission/Message Architecture Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved synchronous relational Transmission/Message derivation for the AP-06 boundary only. The user selected C for the AP-06 Transmission/Message boundary: atomically persist original intake, exact Grouping, current target resolution/contributions and required facts in relational state; durable outbox consumers perform Delivery/provider work outside that transaction. C-11's publication barrier and C-05's revision-checked correction jobs remain in force. This keeps the frequent path consistent while rare corrections remain separately scheduled, without promising capacity or forbidding later evidence-based optimization.
- **Owner detail:** [architecture owner](../architecture.md#ap-06-transmissionmessage-boundary-selection)

<a id="d-44"></a>
## D-44 AP-06 Unicode Payload Identity Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved NFC normalization for text Payload identity before exact comparison. The considered options were exact code-point comparison without normalization, NFC, or NFKC. The user selected NFC. After Protocol-specific terminator/padding handling and the C-02 line-break/leading-trailing whitespace canonicalization, normalize text Payload comparison values to Unicode NFC before exact comparison. Do not case-fold or apply compatibility normalization.
- **Owner detail:** [C-02](../contracts/c-02-protocol-payload.md)

<a id="d-45"></a>
## D-45 AP-06 Gateway Transport Security Decision

- **Status:** accepted (extended by D-52)
- **Date:** 2026-10-26
- **Decision:** Approved direct HTTP support for Gateway requests; its application-wide scope is extended by D-52. Gateway requests must be authenticated and tied to the Gateway identity before C-01 authorization. This decision allows Gateway requests over HTTP without an application opt-in or mandatory GoPager TLS termination; it does not prescribe or prohibit a reusable credential. D-47 records the separately approved credential mechanism.
- **Consequences:** C-06 owns the credential and authorization boundary; C-01 references it for Gateway identity and Submission handling. [F-49](../contracts/reference-cases.md#ap-06-gateway-authentication-case) covers credential binding, invalid credentials, authorization, revocation and C-01 idempotency; replay protection is not an approved requirement.
- **Owner detail:** [D-52](08-operations-and-http.md#d-52), [C-01](../contracts/c-01-transmission-acceptance.md), [C-06](../contracts/c-06-identity-authorization.md)

<a id="d-46"></a>
## D-46 AP-06 Grouping Window Decision

- **Status:** superseded by D-51
- **Date:** 2026-10-26
- **Superseded by:** [D-51](02-intake-and-grouping.md#d-51)
- **Decision:** Approved no additional product cap for the configurable AP-06 Grouping window; superseded by D-51. The configurable Grouping window has no additional product-imposed maximum. Its whole-second precision is separately approved under [D-49](02-intake-and-grouping.md#d-49). The C-03 draft represents `W` as a nonnegative whole number of seconds, `0` through the maximum signed 64-bit integer. This numeric representation is a proposed technical contract detail owned by C-03 and remains subject to the AP-04 owner-contract freeze; it is not a separate user approval of a physical database field or implementation.
- **Consequences:** This records the original decision only; D-51 supersedes its maximum-value rule. C-03 owns window semantics;
- **Owner detail:** [D-51](02-intake-and-grouping.md#d-51), [C-03](../contracts/c-03-message-grouping.md)

<a id="d-49"></a>
## D-49 AP-06 Grouping Window Precision Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved whole-second precision for the configurable AP-06 Grouping window. `W` is configurable only in whole-second increments. `W=0` remains the separate rule that disables Grouping entirely; positive whole-second values use C-03's inclusive sliding-window comparisons.
- **Consequences:** C-03 owns the configuration and comparison semantics; F-50 covers the range/boundary. This does not close P-08 duration/concurrency validation or approve implementation.
- **Owner detail:** [C-03](../contracts/c-03-message-grouping.md)

<a id="d-50"></a>
## D-50 AP-06 Grouping Validation Timing

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved deferring product-specific Grouping duration/concurrency validation to AP-06 T024. Carry product-specific PostgreSQL Grouping duration/concurrency validation as a mandatory AP-06 T024 acceptance check after implementation. It remains open until the real T024 tests pass, but does not by itself block AP-04 T017/T018 contract freeze or execution binding.
- **Consequences:** P-08 remains open until T024 evidence is recorded. This decision changes only validation timing; it does not waive the test, select a transaction implementation beyond D-43, freeze C-03, or approve AP-06 implementation.
- **Owner detail:** [P-08](../authoritative-plan.md#open-issues-and-risks)

<a id="d-51"></a>
## D-51 AP-06 Grouping Window JSON Integer Limit Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Supersedes:** [D-46](02-intake-and-grouping.md#d-46)
- **Decision:** Approved JSON integer Grouping window capped at 2^53−1 seconds; supersedes D-46. Represent `W` as a JSON integer in the inclusive range `0..9,007,199,254,740,991` (`2^53−1`) seconds. `W=0` continues to disable Grouping; the default remains 30 seconds. Reject values outside the range or non-integer values.
- **Consequences:** C-03 owns the wire/range contract; [F-50](../contracts/reference-cases.md#ap-06-grouping-window-range-case) covers zero, boundaries and rejected out-of-range values. AP-06 T023/T024 consume the rule.
- **Owner detail:** [C-03](../contracts/c-03-message-grouping.md), [F-50](../contracts/reference-cases.md#ap-06-grouping-window-range-case)

<a id="d-53"></a>
## D-53 AP-06 Transmission Target Reference Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved Transmission ID plus immutable zero-based target position instead of a separate target UUID. The user selected the Transmission reference plus immutable zero-based target position. This distinguishes even identical Address/Function entries without introducing a separate entity. C-05 owns the concrete reference and targetless-support semantics; shared conventions route to that definition.
- **Consequences:** AP-06 T023/T024 and AP-08 T029 use this reference; C-08 facts and C-11 target context consume it. Planned F-52 covers duplicates, bounds, cross-Transmission ownership and targetless support.
- **Owner detail:** [C-05](../contracts/c-05-matching-correction.md#original-target-reference)

<a id="d-104"></a>
## D-104 AP-07 Gateway HTTP Submission boundary decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved Gateway-authenticated HTTP Submission as the intended AP-07 product boundary; no baseline or implementation approval. Determine whether AP-07 is intended to deliver an externally reachable, Gateway-authenticated HTTP Submission endpoint or only an internal Submission service with the HTTP route deferred to another package. The user selected the externally reachable Gateway HTTP endpoint as the intended AP-07 product boundary.
- **Owner detail:** This record is the only current definition

<a id="d-105"></a>
## D-105 AP-07 Gateway-state HTTP boundary decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved Gateway-authenticated HTTP Gateway-state reporting in AP-07; no baseline or implementation approval. Determine whether the AP-07 Gateway HTTP boundary also includes the C-01 `ReportGatewayState` operation, or whether Gateway-state HTTP reporting is deferred. The user selected a Gateway-authenticated HTTP endpoint for heartbeat and explicit termination reporting in AP-07.
- **Owner detail:** This record is the only current definition

<a id="d-106"></a>
## D-106 AP-07 Gateway Network configuration HTTP boundary decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved administrator-authenticated HTTP Gateway Network configuration in AP-07; no baseline or implementation approval. Determine whether C-01 `ConfigureGatewayNetworks`, an Administrative-Principal operation, is exposed through an HTTP management endpoint in AP-07 or remains an internal service/deferred route. The user selected an administrator-authenticated HTTP endpoint in the AP-07 intended boundary.
- **Owner detail:** This record is the only current definition

<a id="d-107"></a>
## D-107 AP-07 Submission route decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved POST /api/v1/transmissions route for SubmitTransmission; no contract freeze or implementation approval. Select the HTTP method/path for the approved Gateway-authenticated `SubmitTransmission` operation. The user selected `POST /api/v1/transmissions`; the Gateway is identified through authentication rather than a caller-supplied Gateway ID in the path.
- **Owner detail:** This record is the only current definition

<a id="d-108"></a>
## D-108 AP-07 Gateway-state route decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved POST /api/v1/gateway/state route for ReportGatewayState; no contract freeze or implementation approval. Select the HTTP method/path for the approved Gateway-authenticated `ReportGatewayState` operation. The user selected `POST /api/v1/gateway/state`; the Gateway is identified through authentication rather than a caller-supplied Gateway ID in the path.
- **Owner detail:** This record is the only current definition

<a id="d-109"></a>
## D-109 AP-07 Gateway Network configuration route decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved PUT /api/v1/gateways/{id}/networks route for ConfigureGatewayNetworks; no contract freeze or implementation approval. Select the HTTP method/path for the approved Administrative-Principal `ConfigureGatewayNetworks` operation. The user selected `PUT /api/v1/gateways/{id}/networks`, targeting the Gateway with its ID and using administrative authentication.
- **Owner detail:** This record is the only current definition

<a id="d-110"></a>
## D-110 AP-07 Gateway Network replacement semantics decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved complete-replacement semantics for Gateway Network PUT; no contract freeze or implementation approval. Determine whether `PUT /api/v1/gateways/{id}/networks` replaces the complete allowed-Network set or partially mutates it. The user approved complete-set replacement under `expectedRevision`; do not add PATCH. Reject an empty or invalid set without mutation and report stale revisions as a revision conflict.
- **Owner detail:** This record is the only current definition

<a id="d-111"></a>
## D-111 AP-07 Gateway Network replacement authorization decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved Network-scope authorization over current and proposed Gateway assignments; no contract freeze or implementation approval. Determine the Network scope required for an Administrative Principal to replace a Gateway's complete Network assignment set. The user approved requiring `gateway.update` coverage for every Network in the union of the current and proposed assignments; an `all`-scope grant also covers the operation. Current-only coverage is insufficient to add an unauthorized Network, and no partial mutation is allowed.
- **Owner detail:** This record is the only current definition

<a id="d-113"></a>
## D-113 AP-07 Gateway degraded-report decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved bounded Gateway degraded reports with opaque reason codes and optional detail. Determine how `ReportGatewayState` represents a Gateway-reported failure/processing problem. The approved draft allows a `degraded` report with a required opaque ASCII `reasonCode` bounded to 64 bytes and optional UTF-8 `detail` bounded to 512 bytes. These values are Gateway-reported diagnostics only; they do not authorize actions or independently drive automated behavior. The report records contact time and the degraded state fact.
- **Owner detail:** This record is the only current definition

<a id="d-114"></a>
## D-114 AP-07 Gateway recovery decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved explicit recovered operation for Gateway degraded-state recovery. Determine how an authenticated Gateway reports recovery after a degraded report. The user approved a distinct `recovered` operation. It explicitly records contact and transitions `degraded` to `active`; heartbeat remains contact-only and does not clear degraded status.
- **Owner detail:** This record is the only current definition

<a id="d-115"></a>
## D-115 AP-07 Gateway Network replacement request decision

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved complete Gateway Network replacement request shape. Determine the request representation for complete-set replacement. The user approved a closed request with `schemaVersion`, `expectedRevision` and a complete `networks` array. Each entry contains `gatewayNetworkIdentifier`, `networkId` and a `default` boolean. Local identifiers and Network IDs are unique.
- **Owner detail:** This record is the only current definition

<a id="d-116"></a>
## D-116 AP-07 Gateway state-fact boundary

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved Gateway state facts only for actual state transitions. Determine whether `gateway.state-changed` is emitted for every authenticated report or only for a state transition. The user approved transitions only. Heartbeat and repeated reports of the current state may update authenticated contact/report data but do not append `gateway.state-changed` unless the effective state changes.
- **Owner detail:** This record is the only current definition

<a id="d-117"></a>
## D-117 AP-07 Gateway state HTTP response

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved 204 success response for Gateway state reports. Select the successful HTTP response for `ReportGatewayState`. The user approved `204 No Content` after the authenticated report and required state/fact changes commit. Failures use the common C-07 error envelope/status semantics.
- **Owner detail:** This record is the only current definition

<a id="d-118"></a>
## D-118 AP-07 Gateway Network HTTP response

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved revision-returning success response for Gateway Network replacement. Select the successful HTTP response for `ConfigureGatewayNetworks`. The user approved `200 OK` with `{schemaVersion:1, revision:string}` containing the newly committed Gateway revision, allowing the caller to use it as the next `expectedRevision`. Failures use the common C-07 error envelope/status semantics.
- **Owner detail:** This record is the only current definition

<a id="d-119"></a>
## D-119 AP-07 contract freeze proposal

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved AP-07 owner-contract slice freeze; implementation approval remains separate. Which exact owner-contract inputs govern AP-07 implementation? T057..T059 require Gateway HTTP acceptance and state operations, complete Network replacement, original-preserving Protocol comparison, initial target admission/Grouping, required facts, and the Processing publication barrier. D-104..D-118 and D-120 record the user-approved product decisions; [D-112](09-process-and-documentation.md#d-112) places remaining necessary planning in AP-07. The user approved the exact owner-slice set below for 1.0 freeze.
- **Owner detail:** This record is the only current definition

<a id="d-120"></a>
## D-120 AP-07 recovered report idempotency

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved idempotent active-state semantics for `recovered` reports. Determine how an authenticated `recovered` report behaves if the Gateway is no longer degraded. The user approved it as an idempotent request to set state `active`. It records contact; if the effective state already is active, it emits no `gateway.state-changed` fact under D-116. Any different state transitions to active and emits the transition fact.
- **Owner detail:** This record is the only current definition

<a id="d-121"></a>
## D-121 AP-07 Gateway credential-management HTTP addendum

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved additive AP-07 Gateway credential-management HTTP contract; implementation approval remains separate. Keep provisioning blocked pending an identified provisioner; add an operator-side non-HTTP mechanism; or add explicit protected HTTP operations. The user approved the exact additive HTTP contract proposal below. The existing three C-01 routes and all existing C-01/C-06 1.0 text remain unchanged. The new AP-07 credential-management slice is an additive 1.1 amendment in each canonical contract owner file.
- **Owner detail:** This record is the only current definition
