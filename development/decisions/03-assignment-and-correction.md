# ADR: Assignment and correction

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-08"></a>
## D-08 Unit organization and filter correlation

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed Unit organization and filter correlation.
- **Owner detail:** [C-04](../contracts/c-04-master-data.md), [C-07](../contracts/c-07-query-live.md)

<a id="d-09"></a>
## D-09 Temporal assignment/deletion effects

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed temporal assignment/deletion effects.
- **Owner detail:** [C-04](../contracts/c-04-master-data.md), [C-05](../contracts/c-05-matching-correction.md)

<a id="d-10"></a>
## D-10 Suppression boundary

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed suppression boundary.
- **Owner detail:** [C-05](../contracts/c-05-matching-correction.md), [C-06](../contracts/c-06-identity-authorization.md)

<a id="d-39"></a>
## D-39 C-03 Reassessment Decision

- **Status:** superseded by D-40 (in part: the mandatory-preparation reading)
- **Date:** 2026-10-26
- **Superseded by:** [D-40](02-intake-and-grouping.md#d-40)
- **Decision:** Controlled historical regrouping prepared documentarily only; the mandatory-preparation reading is superseded by D-40's scope correction. Prefer a controlled explicit operation over automatic regrouping on every configuration edit; distinguish new-policy recomputation from faithful repair of existing decisions. Obtain the user's delivery-scope choice before changing C-03 or dependent definitions. A subsequent concrete amendment must identify approved owner-document/fixture changes and any not-yet-started AP amendments; preserve previous decisions/evidence and AP-05's active baseline. No framework, version, experiment or later-AP implementation is selected by this proposal.
- **Owner detail:** This record is the only current definition

<a id="d-60"></a>
## D-60 Early Master-data and Matcher Prerequisite Proposal

- **Status:** accepted
- **Date:** 2026-10-27
- **Decision:** Approved minimal Unit/collection/current-matcher contract freeze and sequential implementation start. The user approved the four narrowly identified owner candidates as 1.0, with their existing linked semantics/fixtures and the exact proposed [consumer binding](../execution/ap-04.md#ap-08-prerequisite-consumer-binding-proposal). Contract approval and execution approval remain distinct. The required implementation start is sequential and limited to that binding; it cannot start on documentary-preparation or contract approval alone. No new library tuple, implicit permission, AP baseline change, historical correction worker, provider/plugin host or frontend follows.
- **Consequences:** Owner freeze metadata and the plan gate record approval; the subsequent explicit implementation authorization activates the exact sequential binding now. Actual schemas/DTOs/ports replace their hand-maintained inventory at the C-ID owners when produced. Existing AP-04/AP-07 evidence is preserved;
- **Owner detail:** This record is the only current definition

<a id="d-125"></a>
## D-125 AP-08 Unit-template ownership

- **Status:** accepted
- **Date:** 2026-11-04
- **Decision:** Approved standalone Unit-template ownership direction; C-06 actions and implementation remain unbound. The user responded `standalone`. A Unit template is therefore an independent master-data resource, not an Organization-owned resource. This permits a template to carry an optional Organization reference without making that Organization the template's owner. The Organization-scoped and read-only alternatives were not selected.
- **Consequences:** [C-04](../../docs/http-api.md#master-data-semantics) owns the snapshot behavior. [AP-08 T060](../work-packages/ap-08-master-data-correction-after-intake.md) owns exact C-06 template actions and implementation; [AP-15](../work-packages/ap-15-frontend.md) consumes the completed API.
- **Owner detail:** This record is the only current definition

<a id="d-127"></a>
## D-127 AP-08 suppression direction

- **Status:** accepted
- **Date:** 2026-11-04
- **Decision:** Selected AP-08 explicit target-specific suppression; no broad selector rules. AP-08 persists one Suppression per immutable `(transmissionId, targetPosition)`. This makes each exclusion, authorization context, audit record, and lift apply only to the manager-selected original target. Broad selectors that silently suppress future or multiple targets are not part of this slice.
- **Consequences:** [C-05 historical-correction 1.0](../../docs/architecture.md#historical-correction-worker-role) owns the exact data and revision contract; [C-06](../../docs/http-api.md#authorization) owns authorization. D-10's confirmed no-deletion/no-implicit-Payload-masking boundary, C-05 initial target admission, original evidence, Message membership, and Delivery non-trigger behavior remain unchanged.
- **Owner detail:** This record is the only current definition

<a id="d-141"></a>
## D-141 AP-20 boundary crossing for Gateway mapping re-resolution

- **Status:** accepted
- **Date:** 2026-11-07
- **Decision:** For [AP-20](../work-packages/ap-20-gateway-mapping-rematch.md) T090/T091 the user approved three paths outside the AP's write scope: (1) `internal/events/correction.go`: `correction.failed` accepts the reason `gateway-mapping-change`, and a corrected `assignment.changed` carries an optional `previousNetworkId`; its target list may be empty only when the Network moved; (2) `cmd/gopager/` composition only: wiring the Network re-resolver into the correction runner and the correction queue into Gateway administration; (3) PostgreSQL test fixtures in `internal/persistence/ent/borrowed/*_test.go`. Deferring AP-20 behind AP-11 was considered and rejected, because it would turn the fact change into a change of a completed AP-11.
- **Consequences:** AP-20's write scope includes these paths. The [AP-08 record](../execution/ap-08.md) gets a follow-up for the extended fact vocabulary and runner, with the correction tests rerun. [AP-11](../work-packages/ap-11-events-delivery.md) lists the new optional field and the reason in its compatibility context. C-08 and [Architecture: Historical correction](../../docs/architecture.md#historical-correction-worker-role) describe the facts.
- **Owner detail:** [C-05](../contracts/c-05-matching-correction.md), [AP-20 record](../execution/ap-20.md)

<a id="d-143"></a>
## D-143 Network re-inference after Address-rule changes

- **Status:** accepted
- **Date:** 2026-11-07
- **Decision:** Because Address rules decide Network inference among a Gateway's Networks of one Protocol, a rule change also re-runs that inference for accepted Transmissions, amending the AP-08 behavior that rule corrections keep the Network. `MarkPending` with reason `rule-change` additionally queues, at the same revision, one `gateway` scope (reason `rule-change`) per Gateway and Protocol whose current mapping contains the rule's Network and at least one other Network of the Protocol. Such a scope selects only undeclared Transmissions with targets; the AP-20 move, unresolved and fact semantics apply. Rule-write authority stays as before: a rule on one Agency's Network of a shared Gateway may move inferred history to or from another Agency, consistent with intake inference. Requiring authority over every affected Agency, or moving only between the writer's Agencies, was considered and rejected by the user. The user agreed that an ambiguous re-inference stays a process warning (no `operation.network-ambiguous` fact, which belongs to intake) and that T091's visibility check (on `message_units`) and restart simulation (failing batch, new runner) are sufficient for AP-20.
- **Consequences:** The [AP-08 record](../execution/ap-08.md) gets a follow-up; the AP-20 write scope covers the paths (`internal/persistence/ent/assignmentstore/`, borrowed fixtures under [D-141](#d-141)). Case F-101; a rule change on a busy shared Gateway re-reads that Gateway's inferred Transmissions in batches.
- **Owner detail:** [C-05](../contracts/c-05-matching-correction.md), [AP-20 record](../execution/ap-20.md)

<a id="d-145"></a>
## D-145 Encrypted marker on Address rules

- **Status:** accepted
- **Date:** 2026-11-08
- **Decision:** REQ-124 needs an Address assignment marked as encrypted, but AP-08 delivered no marker (input gap found at the AP-10 start). The user approved adding it in AP-10: an Address rule carries `encrypted` (boolean, default false), set by the existing rule create and update commands under the rule revision and returned by rule reads. It is a label only: it claims no available decryption, changes no matching, correction or Payload access, and queues no correction. AP-10 projects it on targets assigned by such a rule (`matched_rule_id`) in query and detail results. Leaving REQ-124 open for a later package was considered and rejected by the user.
- **Scope (AP-10 write scope extension):** the Address-rule domain in `internal/assignments/` (rule type, validation, commands), its store in `internal/persistence/ent/assignmentstore/`, the Ent schema and `00004_current_assignments.sql` (edited in place under [D-128](01-platform-and-storage.md#d-128)), the rule HTTP adapter and OpenAPI schema, their tests and PostgreSQL fixtures.
- **Consequences:** The [AP-08 record](../execution/ap-08.md) gets a follow-up with the rule tests rerun; [HTTP API: Address rules](../../docs/http-api.md#address-rules-suppressions-corrections), C-04 and C-05 describe the field. AP-15 can show and edit it (its compatibility context names the field).
- **Owner detail:** [C-05](../contracts/c-05-matching-correction.md), [AP-10 record](../execution/ap-10.md)
