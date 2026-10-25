# C-05 Assignments, Suppression, and Rematching

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-01](c-01-transmission-acceptance.md), [C-06](c-06-identity-authorization.md), [C-09](c-09-delivery.md).

**Traceability:** [REQ-021](../requirements-catalog.md#c-units-organizations-and-defaults), [REQ-031..040](../requirements-catalog.md#d-correct-assignment-and-history), [REQ-123..124](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-08..P-10/P-12](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft. **Owner:** DOM-04. **Producers:** temporal Address rules, Transmission targets, relevant master-data Domain events. **Consumers:** DOM-06/DOM-07 using central C-06 evaluation.

| Draft interface | Input / result | Obligation |
| --- | --- | --- |
| `ValidateAddressRuleChange` | Principal, target Networks/Units, expected revision, Protocol variant/validity -> preview/conflict | Expose permitted rules/Units only; reject equally specific or partially overlapping simultaneously valid rules. |
| `MatchTransmissionTarget` | Transmission/target, resolved Network, Observation time, current rules -> stored target resolution and updated Message contribution/Contribution support, or explicit non-success state | Unit is stored on the target and shared contribution as a controlled derivation; repeated Gateway evidence is not another user assignment. |
| `ScheduleRematch` | Relevant change/deletion and affected scope -> correction job/revision | Miss no potentially affected historical Transmission. |
| `ReadRematchStatus` | Authorized Principal, job -> waiting/running/completed/failed with safe progress | Start is not full completion; errors/restart traceable. |
| `ChangeSuppression` | Principal, Transmission target filter, expected revision -> rule revision/projection work | Preserve evidence, correct current visibility/Delivery; lifting suppression is not Transmission. |

**Defined:** Address-to-Unit validity is temporal; general Unit attributes are not. A later-entered rule valid from X corrects affected history using Observation time. When processing is available, rematching starts after 60 seconds of inactivity by default; further changes cannot extend the delay beyond 300 seconds by default. Both are configurable rematching values, not Grouping duration or completion SLA.

Suppression is Transmission-target-specific. No normal Message view without usable permitted evidence; mixed Messages remain partly usable. Originally targetless evidence has the explicit Network/without-Unit path below, not a fabricated non-suppressed target. A hidden assignment is not relabeled 'Without Unit'. No additional Payload masking or implicit admin bypass.

## AP-02 Matching and Correction

**Rule shape:** Stable ID/revision, exactly one Network/its Protocol, one Unit, Address selector, Function selector/mode, and half-open Validity interval `[from, until)`; missing bound is unbounded. Validity is evaluated at Observation time, never Message time. The Unit must exist and all Network uses must be permitted. C-01 always resolves an accepted Transmission to an assigned Network; unknown/unauthorized Network is not a global rule scope.

**Specificity:** Treat each Protocol selector as a set of Address/Function pairs. Proper subset wins over its superset; Function bits participate in specificity. Two simultaneously valid rules in the same Network with equal selectors or overlapping selectors where neither contains the other are rejected, even if both name the same Unit. Disjoint selectors/intervals are valid. No artificial priority score lets Address precision silently defeat Function precision.

- POCSAG selectors: exact RIC or inclusive RIC range and optional Function filter, with explicit `function-bit` or `subaddress` mode. Same exact Address/range selector with overlapping Validity and differing modes conflicts regardless of Function values/Unit. Overlapping broad/narrow selectors also require consistent mode where they overlap. Never infer mode from a Unit name.
- FLEX selectors: exact capcode or validated Protocol-supported range and Function attributes defined by the Protocol definition. The FLEX fact verification remains AP-04; an unsupported variant is rejected, not treated as a wildcard.
- ZVEI: exact five-tone string with preserved leading zeros; signal/double-tone meaning is a Unit attribute, not an invented Address hierarchy.
- EAS: exact submitted location-code string; no automatic geographic containment or expansion to neighboring areas. Multiple Transmission target areas are independently assigned.
- No matching rule means explicit unassigned target resolution with null Unit/matched-rule reference, never a default Unit. Empty target lists yield no synthetic Address/target; targetless contributions preserve a Network-authorized `without-unit.read` path. Ambiguous/failed/pending are distinct from proven unassigned.

### Target Resolution, Message Contributions and Ingest Storage

**Logical structure is confirmed; concrete fields/types are not frozen by the illustrative ER diagram.** Each Transmission retains its own original Payload and exact targets. Store current resolved Network, direct Unit, matched Address-rule reference and explicit resolution/suppression state on each target. Those are replaceable derivations, not new original evidence or independently editable master data.

Address rules retain domain validity intervals and ordinary edit concurrency control. No immutable Address-rule-version entity is required: configuration audit records changes, while Rematching uses current rules at original Observation times. Exact replay of every earlier evaluation is not promised. A removed rule must not leave stale Unit authority; reference cleanup and invalidation are coordinated.

A Message contribution is the stored, rebuildable read model: Message, Network/Protocol, concrete Address/Function, direct Unit or explicit status, Contribution time and Contribution age anchor. Logical identity is those context dimensions, not Gateway, rule edit version or contiguous validity segment. Different Addresses remain different contributions even for the same Unit. Different rules/segments resolving to the same visible context can share one contribution; exact temporal evaluation remains on each original target. Unit changes across a validity boundary produce distinct Unit contributions, without discarding provenance.

Semantic reference names are `contributionId` and `addressRuleId`; these are not a frozen physical field inventory or an immutable rule-version reference.

Contribution supports reference contribution + Transmission + original target, with no target only for a targetless Transmission. Enforce at most one current support per usable target, target ownership by the referenced Transmission, and common Message membership. Suppressed/unready targets do not support the normal read model; unsupported contributions are removed. One Transmission can support many contributions, and many Transmissions can support one. Targetless support uniqueness and nullable-key equality need explicit constraints in the concrete schema.

Ingest operations:

1. Authenticate/idempotency-check; preserve original Payload, exact Addresses/Functions, declaration and both times. Determine membership independently of Unit resolution.
2. Resolve Network and each original target at Observation time using current rules. Store direct Unit/matched filter or explicit status. Never reuse a cached result across incompatible validity context.
3. Upsert matching contributions and supports. Maintain `contributionTime = min(usable Observation times)` and `contributionAgeAnchor = min(min(Observation time, Acceptance time) per usable support)`. Repeats do not advance either value. Rule/suppression/membership correction recomputes affected summaries.
4. Publish target resolutions, contributions, supports and required facts/work consistently. `assignment.changed` describes genuine resolution/first-time changes, not one extra user assignment per Gateway. Complete required ingest Workflows before GUI/Delivery publication.

Ten same-context independent receptions produce ten original target resolutions and ten supports, but one shared contribution. Stored Unit repetition is deliberate denormalization, not ten user appearances. Multi-target input remains one Transmission in one Message. Repeated original Payload storage is allowed; optional physical deduplication must reconstruct every original, including partial/deviating values.

Normal reads start at direct contribution Unit/Network/Protocol and first times, then Message and readable Payload; no rule-version join or routine reception-history aggregate is required. Indexes, physical tables, field types and necessary revision placement remain owner-package/contract work, with backend evidence in AP-03. Contributions cannot be manually edited as a second source of truth.

**Suppression shape:** Network, Protocol, Address/Function selector, optional Gateway and Observation-time interval; target-specific, independent of Unit assignment. Targetless evidence has no target to suppress; it remains subject to explicit Network/without-Unit rights. Enabling/changing suppression updates revision and invalidation atomically; it never waits for background work before denying affected output. Lifting it publishes only current corrected projections. `suppression.changed` is explicit; no new Transmission or automatic Delivery of historical Payload.

**Selected revision algorithm (simple primary-store transactions):**

1. Commit the new rules/deletion, increment Assignment revision, mark the potentially affected scope pending, and enqueue correction together. Calculate a conservative superset of affected Networks/Address/Validity ranges, including previous and new selectors; broader invalidation is safe, omission is not.
2. All new ingest reads the current rules. Affected evidence under correction is not exposed through normal views/Delivery until its applicable revision is ready. Unaffected scopes remain usable.
3. Re-evaluate affected original Addresses/Functions at Observation times; update target Network/Unit/matched-rule/status, redistribute Contribution supports and rebuild affected Message contributions/first times/age anchors. Remove unsupported rows; leave originals and Grouping unchanged. Correcting Address 456 from A to B must not move Address 123 merely because both previously resolved to A. A revision-checked publication covers historical scan and concurrent evidence via an acceptance high-water mark/dirty queue; a one-time scan is insufficient.
4. A newer rule change supersedes old work. Reuse completed batches only if their inputs still match; old Fencing tokens cannot publish. Debounce defaults remain 60/300 s; continuous changes cannot indefinitely postpone starting work, but completion is not promised by that timer.
5. Failed work leaves the affected scope pending/failed, with redacted error and retry state visible only to its managers. Retry uses the current revision; cancelling a job does not reactivate stale projections. Startup resumes current work.

`ReadRematchStatus` returns job/revision/status, affected caller-visible scope, processed/remaining counts only where authorized, and retryable error/progress time. `assignment.changed` records genuine before/after revision transitions, not every retry/no-op. Already created read/Delivery jobs can retain their original domain context; C-09 reuses it only after current access checks. No historical job is rewritten just to match a cosmetic edit.
