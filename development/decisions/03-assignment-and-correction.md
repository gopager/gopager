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
- **Consequences:** [C-04](../contracts/c-04-master-data.md#ap-08-unit-template-direction) owns the snapshot behavior. [AP-08 T060](../work-packages/ap-08-master-data-correction-after-intake.md) owns exact C-06 template actions and implementation; [AP-15](../work-packages/ap-15-frontend.md) consumes the completed API.
- **Owner detail:** This record is the only current definition

<a id="d-127"></a>
## D-127 AP-08 suppression direction

- **Status:** accepted
- **Date:** 2026-11-04
- **Decision:** Selected AP-08 explicit target-specific suppression; no broad selector rules. AP-08 persists one Suppression per immutable `(transmissionId, targetPosition)`. This makes each exclusion, authorization context, audit record, and lift apply only to the manager-selected original target. Broad selectors that silently suppress future or multiple targets are not part of this slice.
- **Consequences:** [C-05 historical-correction 1.0](../contracts/c-05-matching-correction.md#ap-08-historical-correction-and-suppression) owns the exact data and revision contract; [C-06](../contracts/c-06-identity-authorization.md#ap-08-master-data-authorization) owns authorization. D-10's confirmed no-deletion/no-implicit-Payload-masking boundary, C-05 initial target admission, original evidence, Message membership, and Delivery non-trigger behavior remain unchanged.
- **Owner detail:** This record is the only current definition
