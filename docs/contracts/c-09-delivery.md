# C-09 Subscriptions, Targets, and Delivery

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-06](c-06-identity-authorization.md), [C-07](c-07-query-live.md), [C-08](c-08-events-jobs.md), [C-10](c-10-plugin-host.md).

**Traceability:** [REQ-081..087](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [REQ-105..106](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [REQ-129..130](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-07/P-10/P-11](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft. **Owner:** DOM-07; C-10 host executes under control. **Parties:** authorized subscription owner/target manager, Domain event worker, host capability, provider.

| Draft interface | Input -> result |
| --- | --- |
| `ConfigureDeliveryTarget` | Authorized administrator/Operator, plugin type, expected revision, administrative configuration/Secret references -> separate instance and redacted status |
| `ConfigurePersonalRecipient` | Owning User, administratively configured target, expected revision, declared personal recipient fields/Secret references -> instance-bound personal recipient and redacted status |
| `ChangeSubscription` | Owner, expected revision, Domain event selection, filters, target, Payload selection -> explicit active/paused configuration |
| `PrepareDeliveryAttempt` | Job/Domain event reference -> currently authorized projection, target configuration, attempt permission or cancellation reason |
| `RecordDeliveryOutcome` | Attempt, controlled provider result -> accepted, retry pending, failed/expired, or uncertain |
| `RequestExplicitRedelivery` | Authorized Principal, original attempt, deliberate confirmation -> separate traceable job, not overwritten result |

**Defined:** Subscriptions explicitly select Domain event types, correlated domain/Processing-result filters, one or more Delivery targets, and permitted template/input fields; multiple instances of one plugin remain separate. There is no global Unit-to-Delivery-target mapping. Declared Unit Extension fields can parameterize a Subscription's filter/template/input, never introduce a destination implicitly. Pause/removal, Principal suspension/deletion, current grant revocation, and age limits prevent new unauthorized attempts.

Before every new attempt, authoritatively check current Principal, subscription, filters, assignments/suppression, and field permissions. A stored old ready-to-send Payload is not authorization. Preparation, execution, and recording are distinguishable; provider acceptance is neither reading nor recallability.

**Uncertainty:** A lost provider response may follow an already executed effect. Do not disguise it as definite failure/success or blindly repeat indefinitely. Bounded retries require failure classification and current permission; permanent/transient/uncertain outcomes remain separate. Deliberate redelivery names duplicate-effect risk. Use provider idempotency only when actually supported.

**Consistency boundary:** No SQL transaction around long external I/O. The attempt section below defines authoritative start/revocation and preparation races; an already started provider effect cannot be recalled.

## AP-02 Subscription and Plugin-input Contract

Subscription: ID/revision, owning Subscriber, personal/global kind, explicit nonempty Domain event type set, filter definition, permitted Delivery target references, template/input mapping version, active/paused state, creation/start time and optional additional maximum age. Global Subscription requires `subscription.manage-global` and an assigned technical Principal with explicit delegated data/target rights. Personal Subscription acts as its User. A creator's temporary admin status is never captured as unrestricted sending authority.

**Filters:** Reuse the C-07 dimension rules and typed predicates: equality, membership, presence, numeric/time comparison, literal substring, AND/OR/NOT with bounded nesting. Values are typed and namespaced; missing/hidden fields do not pass a positive predicate, and negation must not reveal hidden-field presence. Evaluate on an authorized projection, not original data. `processing.completed` with permitted `level >= 2` is a valid explicit selection. Workflow trigger selection uses the same predicate representation, not a second ad-hoc expression language. No arbitrary code in a filter.

Hidden/unreadable operands produce an indeterminate value, propagated through negation and boolean composition; only a definite true matches. In particular `NOT(hiddenField = x)` is not true. For readable optional fields, presence can explicitly test absence; malformed typed comparisons are validation errors, not coerced strings or silent defaults.

### Early Shared Typed Predicate Candidate

**Version/status:** typed-predicate 1.0, explicitly frozen 2026-11-01 with restricted CEL-Go under D-66 as part of the user's `approve-processing-kernel-1.0` approval; execution start remains separate. **Owner:** DOM-07/integration for this shared representation; DOM-08 consumes it for Workflow selectors/conditions. No Delivery/subscription engine or general expression-language API is introduced.

The approved subset's authoritative implemented wire types/validation are [typed values and closed schemas](../../internal/processing/values.go), [structured predicates/classified lookup](../../internal/processing/predicates.go) and [classification union](../../internal/processing/dependencies.go). [AP-12](../execution/source-ap-12.md#e-04-product-predicate-classification-and-supplied-core-guest) owns their actual partial verification; these primitives do not certify the complete Workflow, Subscription, native-search or publication consumer.

Integer values are signed 64-bit decimal strings with exact comparison, finite numbers are JSON numbers without string coercion, timestamps are offset-bearing RFC 3339 instants at C-12 operational precision. Objects have closed declared properties whose values use this same representation; lists have one declared element schema. Absence is no field entry, not `null`, zero or empty text. Schema `optional`/`nullable` are separate; unknown fields/types, invalid nesting and mismatched comparisons are validation errors. Shared collection/input/nesting limits apply before evaluation.

Predicate JSON has `op`, with only its relevant operand fields:

| `op` | Required representation |
| --- | --- |
| `eq` | `field` reference and one typed `value` of its declared type |
| `in` | `field` reference and bounded typed `values` of that same type |
| `present` | `field` reference and boolean `value` specifying present versus absent |
| `lt`, `lte`, `gt`, `gte` | `field` reference and typed numeric or timestamp `value` of a compatible declared type; no string ordering/coercion |
| `contains` | string `field` reference and literal string `value`, never a regex or substring expression |
| `and`, `or` | nonempty bounded `args` of predicates |
| `not` | exactly one predicate in `args` |

Field references are explicit `namespace` plus path-segment list into declared fields, not an expression string or arbitrary reflected Go field. The evaluation owner supplies the permitted field schema/resolver. Selectors reference source/current target fields; conditions additionally reference visible prior local-step outputs. Subscription consumers later bind their own event/result namespaces to this same type. No other Workflow's private intermediates or undeclared keys. Predicate argument/membership lists use the existing 1024-entry filter bound and depth 32; typed-value lists retain the 1000-element collection limit. Encoded evaluation input uses the existing 1 MiB limit.

Field lookup returns distinct present, readable-absent and hidden/indeterminate states, with classified dependency references. Hidden is never replaced by absent. Readable optional absence makes equality/membership/ordered/substring predicates false; `present=false` may match it. Explicit null compares only with declared nullable null. Three-valued composition uses definite false dominance for AND, definite true dominance for OR, and otherwise propagates indeterminate; NOT preserves indeterminate. Only definite true selects or takes a true branch. Invalid runtime types are explicit failures, not indeterminate/false.

Retain the union of declared/read source dependencies even when boolean short-circuiting could omit one. This conservative classification prevents hidden selector/branch operands from becoming public through a derived constant. It grants no rights. Target fields resolve on one correlated target context per predicate, never one target's Network and another's Unit. Transmission-level selection tests relevant target contexts separately, selects at most one run per Workflow and retains the context dependencies. No implicit list-loop expression is exposed.

Under the final D-66 selection, validated structured predicates bind to restricted checked CEL-Go declarations/programs. There is no user-supplied CEL expression string, arbitrary function environment, independent authorization engine or native regex substitution. Standard-library validation retains exact wire types, finite numbers and the explicit absence/null/Hidden adapter; conservative source dependencies are collected independently of CEL short-circuiting. [The library assessment](../architecture.md#cel-go-incremental-capability-review) owns the selection rationale and [E-50](../execution/ap-04.md#e-50-bounded-cel-compatibility-and-cost-comparison) the bounded proof, not full production conformance. Additional library operators/extensions require explicit owner declaration before product use.

### Subscription Target and Input Selection

**Target selection:** A Subscription can select one or several configured target instances, including a provider-side channel or collection such as Telegram channel/chat or Pushover group key. Personal recipient selection follows the [configuration-authority boundary](#configuration-authority); subscription rights never create an integration. A dynamic selector may select from an explicitly delegated configured target pool using permitted structured data; it cannot turn untrusted Payload into an arbitrary undelegated URL/contact/Secret. An admin may explicitly configure a template-driven external destination under the Integration's trusted transport policy and delegated data rights; that is deliberate configuration, not an implicit global Unit mapping.

**Overrides:** Unit-specific Extension fields have plugin-declared type/validation and explicit permission. Mapping/precedence is declared by the Subscription: common target configuration, then Subscription mapping, then explicitly selected Unit fields. Conflicting values from multiple relevant Units must be resolved by an explicit split-per-Unit or explicit configured selection, never map iteration order. Split Deliveries contain only the corresponding permitted data, with split identity in deduplication. Provider/channel/Secret changes remain separately authorized target configuration, not a Unit-field shortcut.

**V1 responsibility split:** `MaterializeDeliveryInput` is Core-owned. After the ingest Processing barrier, Core reads the currently authorized projection, evaluates filters/target selection/templates, validates plugin-declared input fields, and stores bounded Delivery input plus minimal scope/field/access/Assignment/Suppression revision context. Input is not an unbounded object copy. Plugin receives this input and its instance-bound configuration/Secrets; it need not load the original Message. Plugins needing further Core reads declare those SDK capabilities and use a current authorized invocation Principal. Such a read is a new dependency, not hidden unrestricted access.

| Future SDK/input decision | Required behavior now specified |
| --- | --- |
| Materialized declared fields (selected v1) | Prepared self-contained work may finish after Message retention, if current checks can be completed from its minimal authorization context. Remove Payload copy on terminal completion, at latest 24 h from original Delivery creation. |
| Reference-only capability | If the referred evidence/configuration no longer exists or is not permitted, stop with `dependency-unavailable`; no fabricated empty object or fallback to stale unprojected data. |
| Projected object snapshot | Bounded copy with explicit field classification, authorization context and independent expiry, never full private entity serialization; same revocation/deletion checks as materialized input. |
| Plugin asks for other Units/Core data | Manifest declaration plus approved instance capability and current Principal data grants; result projected through C-06. An undeclared call fails `CAPABILITY_UNDECLARED`. |

This matrix is binding even if AP-03 later requires a different SDK mechanism. Do not implement all variants speculatively. Message retention only cancels work whose actual dependencies disappeared; it does not cancel a self-contained Delivery just because the original Message was deleted.

## Configuration Authority

**AP-04 D-33 clarification, 2026-10-26; DOM-07 with plugin-host/integration ownership; draft, not frozen.** Every integration requires configuration by an authorized administrator or Operator before Users can use it. Administrative authority is checked through C-06 (`plugin.configure` for all or a delegated instance); personal subscription rights do not grant it. Installing/updating plugin code remains Operator-only under C-10.

Administrative configuration owns provider/application credentials, enabled capabilities, message format/templates, transport/authentication settings and security policy. The administrator/Operator is responsible for selecting appropriate settings and consciously accepting configured external-transport risks. This responsibility does not grant access to other Principals' data or disable Core authorization, Secret redaction or the existing transport rules.

Where an integration supports individually addressable recipients, its declared schema may expose personal recipient fields. Users can supply/change their own recipient identifier or credential and configure their own Subscriptions using that administratively configured integration, within current data/target permissions. Personal settings are bound to the owning User and target instance; they cannot change administrative settings, another User's recipient or provider/application Secrets. Subscription mappings and Unit-field overrides cannot promote a personal field into an administrative override. Integrations without individual recipient support expose no such personal-recipient configuration; permitted Subscriptions still use administratively configured targets.

Each property's administrative or personal configurability must be declared in the plugin manifest under [C-10's configuration-field contract](c-10-plugin-host.md#manifest-configuration-fields), which also owns SDK/API enforcement and invalid-declaration handling.

**Pushover:** The administrator/Operator supplies the application API token and defines the message format. Each User may supply their own Pushover User key and select their own Subscriptions. The personal User key does not create another application configuration or reveal/replace its application token. Both credentials use C-10 Secret handling; ordinary views return redacted status, not stored Secret values.

Missing/inactive administrative configuration prevents use with a visible configuration failure; no personal credential creates a fallback integration. Personal-recipient changes are revisioned target configuration and participate in the existing current-authorization/configuration checks before each new attempt. Concrete field schemas and validation/error fixtures remain freeze requirements. This clarification does not select an additional persistent entity or physical SQL schema.

## Initial Target Capabilities

**AP-04 D-33 amendment; DOM-07 with plugin-host/integration ownership; draft, not frozen.** Pushover and Webhook are the initial target plugins. The capability set belongs to [REQ-090](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), delivery timing to the [plan](../authoritative-plan.md#ap-04-scope-decision). Neither installed Gateway behavior nor existing provider fixtures define this API.

Webhook target configuration must distinguish the destination, selected method, input encoding, authentication mode and versioned Secret references. All invocations use this contract's current authorized Delivery input and attempt/uncertainty rules plus [C-10 transport protections](c-10-plugin-host.md#ap-02-secrets-and-external-transport); none of the new options relaxes them.

**Configurable GET input (explicit D-33 follow-up):** Support deliberate target-specific selection of query parameters or request body for legacy endpoints. Neither location is inferred from arbitrary Payload or silently changed after a failure. Body compatibility is endpoint-specific; do not promise universal GET-body interoperability. Query transport of sensitive input requires an explicit Operator-approved risk policy and a visible explanation that destinations/proxies may log URLs. Ordinary subscription configuration does not by itself grant that policy. Input still comes only from the current authorized Delivery projection and the configured target's declared fields; operator risk acceptance is not access to another Principal's data. Do not emit dynamic query values, credentials or complete request URLs in GoPager logs/errors/status; use redacted target/attempt references. Explicit bearer/basic modes use authentication headers, not an automatic credential-to-query conversion. Existing C-10 TLS/destination/redirect rules still apply. External endpoint/intermediary logging is outside GoPager's control and cannot be promised safe by this configuration.

Before freeze, specify concrete validation/errors and positive/negative fixtures for method/encoding/input-location combinations, form value typing, multipart fields, authentication headers, and the exact HMAC signed bytes/header/replay policy. GET configuration must explain body compatibility and query exposure; a failed/uncertain attempt never triggers a secret-bearing location/method fallback. No arbitrary default signing algorithm, URL serialization or success-shaped fallback is selected here.

## AP-02 Attempt State, Authorization and Retry

- Automatic identity: Subscription ID, selected Domain event ID, Delivery target ID and optional explicit split identity. Atomic unique creation prevents duplicate fan-out. Replacing Message/read-model IDs does not rename earlier Domain event IDs or implicitly resend earlier Deliveries; D-40 does not remove operation-specific deduplication.
- Normal state path: pending -> prepared -> started -> accepted / retry-pending / failed / uncertain. Pending/prepared may expire/cancel/dependency-fail; retry-pending returns to preparation. Terminal states are not reset by a worker restart. Creation/attempt IDs, revision, start/finish time, safe outcome code and provider receipt (where safe/useful) are inspectable only with `delivery.read`.
- Before each external attempt, check current Subscriber/account/Session or key validity where delegated, Subscription/target state, age, and access/Suppression revisions against primary authority. If unchanged, reuse the already materialized input; if changed, re-evaluate only relevant scope/fields. A stale target resolution or contribution may remain as historical context, not as proof of current Unit rights. Affected Rematching scopes wait/fail safely until current. No continuous full historical rematerialization or 100% distributed consistency promise.
- Atomically record `started` with current authorization revision/claim immediately before host invocation; provider I/O is outside SQL. Revocation committed before that authorization point blocks start. Revocation after authorized start requests cancellation where possible but cannot recall the external effect. Losing authority/lease prevents additional starts; an already started effect may remain uncertain.
- Default at most 5 attempts total, at 30 s/2 min/10 min/30 min backoff after each known transient failure; jitter within +/-20%, respecting provider Retry-After without exceeding expiry. Deadline 24 h from Delivery creation or an earlier configured Subscription/permission age deadline. Use the selected fact's relevant Unit appearances/Contribution age anchors for data age, not global Message time, provider-start time or a newly appearing unrelated Unit. A correction to relevant earlier evidence can expire an unsent prepared Delivery; a later reception cannot rejuvenate it. For explicitly subscribed Transmission facts retain their own Observation/Acceptance context, but current Unit-data age rights still apply. Operational facts without Message data use their own occurredAt/recordedAt. Permanent errors end immediately; success means provider acceptance, not User reading.
- Uncertain effects (lost response, crash after start, expired claim during I/O) receive no automatic blind retry. Retry is possible only with demonstrated provider idempotency using the same provider key or an explicit authorized duplicate-risk confirmation. SMTP generally has no exactly-once receipt; ambiguous post-DATA disconnect is uncertain.
- `RequestExplicitRedelivery` requires current `delivery.redeliver`, target/data permissions and explicit duplicate-risk confirmation bound to original attempt plus fresh idempotency identity. Produces a new traceable Delivery; does not rewrite prior outcome. `delivery.retry` restarts eligible known-transient failure within original expiry, not an expired or uncertain effect under a misleading retry button.
- Pending/failed/uncertain work lives in the same store, with explicit authorized retry/cancel actions. Most jobs are expected to finish quickly, but upstream outages must remain correct under these finite limits. Receipts follow C-08; no mandatory separate DLQ service. Key/Subscription/target deletion removes their future authority, not historical fact.
