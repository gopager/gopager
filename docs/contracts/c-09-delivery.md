# C-09 Subscriptions, Targets, and Delivery

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-06](c-06-identity-authorization.md), [C-07](c-07-query-live.md), [C-08](c-08-events-jobs.md), [C-10](c-10-plugin-host.md).

**Traceability:** [REQ-081..087](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [REQ-105..106](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [REQ-129..130](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-07/P-10/P-11](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft. **Owner:** DOM-07; C-10 host executes under control. **Parties:** authorized subscription owner/target manager, Domain event worker, host capability, provider.

| Draft interface | Input -> result |
| --- | --- |
| `ConfigureDeliveryTarget` | Principal, plugin type, expected revision, target-specific configuration/secret references -> separate instance and redacted status |
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

**Target selection:** A Subscription can select one or several configured target instances, including a provider-side channel or collection such as Telegram channel/chat or Pushover group key. A dynamic selector may select from an explicitly delegated configured target pool using permitted structured data; it cannot turn untrusted Payload into an arbitrary undelegated URL/contact/Secret. An admin may explicitly configure a template-driven external destination under the Integration's trusted transport policy and delegated data rights; that is deliberate configuration, not an implicit global Unit mapping.

**Overrides:** Unit-specific Extension fields have plugin-declared type/validation and explicit permission. Mapping/precedence is declared by the Subscription: common target configuration, then Subscription mapping, then explicitly selected Unit fields. Conflicting values from multiple relevant Units must be resolved by an explicit split-per-Unit or explicit configured selection, never map iteration order. Split Deliveries contain only the corresponding permitted data, with split identity in deduplication. Provider/channel/Secret changes remain separately authorized target configuration, not a Unit-field shortcut.

**V1 responsibility split:** `MaterializeDeliveryInput` is Core-owned. After the ingest Processing barrier, Core reads the currently authorized projection, evaluates filters/target selection/templates, validates plugin-declared input fields, and stores bounded Delivery input plus minimal scope/field/access/Assignment/Suppression revision context. Input is not an unbounded object copy. Plugin receives this input and its instance-bound configuration/Secrets; it need not load the original Message. Plugins needing further Core reads declare those SDK capabilities and use a current authorized invocation Principal. Such a read is a new dependency, not hidden unrestricted access.

| Future SDK/input decision | Required behavior now specified |
| --- | --- |
| Materialized declared fields (selected v1) | Prepared self-contained work may finish after Message retention, if current checks can be completed from its minimal authorization context. Remove Payload copy on terminal completion, at latest 24 h from original Delivery creation. |
| Reference-only capability | If the referred evidence/configuration no longer exists or is not permitted, stop with `dependency-unavailable`; no fabricated empty object or fallback to stale unprojected data. |
| Projected object snapshot | Bounded copy with explicit field classification, authorization context and independent expiry, never full private entity serialization; same revocation/deletion checks as materialized input. |
| Plugin asks for other Units/Core data | Manifest declaration plus approved instance capability and current Principal data grants; result projected through C-06. An undeclared call fails `CAPABILITY_UNDECLARED`. |

This matrix is binding even if AP-03 later requires a different SDK mechanism. Do not implement all variants speculatively. Message retention only cancels work whose actual dependencies disappeared; it does not cancel a self-contained Delivery just because the original Message was deleted.

## AP-02 Attempt State, Authorization and Retry

- Automatic identity: Subscription ID, selected Domain event ID, Delivery target ID and optional explicit split identity. Atomic unique creation prevents duplicate fan-out. Merge aliases do not rename earlier Domain event IDs or implicitly resend earlier Deliveries.
- Normal state path: pending -> prepared -> started -> accepted / retry-pending / failed / uncertain. Pending/prepared may expire/cancel/dependency-fail; retry-pending returns to preparation. Terminal states are not reset by a worker restart. Creation/attempt IDs, revision, start/finish time, safe outcome code and provider receipt (where safe/useful) are inspectable only with `delivery.read`.
- Before each external attempt, check current Subscriber/account/Session or key validity where delegated, Subscription/target state, age, and access/Suppression revisions against primary authority. If unchanged, reuse the already materialized input; if changed, re-evaluate only relevant scope/fields. A stale target resolution or contribution may remain as historical context, not as proof of current Unit rights. Affected Rematching scopes wait/fail safely until current. No continuous full historical rematerialization or 100% distributed consistency promise.
- Atomically record `started` with current authorization revision/claim immediately before host invocation; provider I/O is outside SQL. Revocation committed before that authorization point blocks start. Revocation after authorized start requests cancellation where possible but cannot recall the external effect. Losing authority/lease prevents additional starts; an already started effect may remain uncertain.
- Default at most 5 attempts total, at 30 s/2 min/10 min/30 min backoff after each known transient failure; jitter within +/-20%, respecting provider Retry-After without exceeding expiry. Deadline 24 h from Delivery creation or an earlier configured Subscription/permission age deadline. Use the selected fact's relevant Unit appearances/Contribution age anchors for data age, not global Message time, provider-start time or a newly appearing unrelated Unit. A correction to relevant earlier evidence can expire an unsent prepared Delivery; a later reception cannot rejuvenate it. For explicitly subscribed Transmission facts retain their own Observation/Acceptance context, but current Unit-data age rights still apply. Operational facts without Message data use their own occurredAt/recordedAt. Permanent errors end immediately; success means provider acceptance, not User reading.
- Uncertain effects (lost response, crash after start, expired claim during I/O) receive no automatic blind retry. Retry is possible only with demonstrated provider idempotency using the same provider key or an explicit authorized duplicate-risk confirmation. SMTP generally has no exactly-once receipt; ambiguous post-DATA disconnect is uncertain.
- `RequestExplicitRedelivery` requires current `delivery.redeliver`, target/data permissions and explicit duplicate-risk confirmation bound to original attempt plus fresh idempotency identity. Produces a new traceable Delivery; does not rewrite prior outcome. `delivery.retry` restarts eligible known-transient failure within original expiry, not an expired or uncertain effect under a misleading retry button.
- Pending/failed/uncertain work lives in the same store, with explicit authorized retry/cancel actions. Most jobs are expected to finish quickly, but upstream outages must remain correct under these finite limits. Receipts follow C-08; no mandatory separate DLQ service. Key/Subscription/target deletion removes their future authority, not historical fact.
