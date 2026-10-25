# AP-08 Master Data and Correction

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [detail actions](../requirements-catalog.md#b-search-filters-and-details), [master data/defaults](../requirements-catalog.md#c-units-organizations-and-defaults), [assignment/history](../requirements-catalog.md#d-correct-assignment-and-history), [presentation](../requirements-catalog.md#j-processing-presentation-and-usability), [suppression/deletion](../requirements-catalog.md#m-additional-domain-capabilities).
- **Architecture:** [DOM-03/DOM-04 ownership](../architecture.md#domains-and-data-ownership), [correction coordination](../architecture.md#shared-use-cases), [logical model](../architecture.md#logical-relationship-model).
- **Owner contracts:** [C-02](../contracts/c-02-protocol-payload.md), [C-04](../contracts/c-04-master-data.md), [C-05](../contracts/c-05-matching-correction.md), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md); [shared conventions](../contracts/shared-conventions.md).
- **Cases and prerequisite outputs:** [F-11..F-17](../contracts/reference-cases.md), [F-27..F-29](../contracts/ap-02-boundary-cases.md); [AP-05 foundation](ap-05-runtime-persistence.md), [AP-07 AuthZ](ap-07-identity-authorization.md), [AP-06 historical integration](ap-06-transmission-messages.md), [AP-04 freeze](ap-04-scope-freeze.md). Respect the staged prerequisites below.

**Owner:** DOM-03/DOM-04; integration coordinates cross-domain deletion. **Contract gate:** C-02/C-04..C-06/C-08. **REQ:** REQ-019..040, REQ-095, REQ-123..124, REQ-132. **Prerequisites by slice:** AP-05/AP-07 for T028 and T029's standalone current-rule/matching kernel using frozen original-target interfaces; AP-06 for historical correction/ingest integration and T030 integration fixtures. **Exclusive target scopes:** `internal/Units/`, `internal/assignments/`, with tests/schema objects.

- [ ] T028 [AP-08] Implement Units/organizations/Unit collections/templates/defaults and authorized bulk/conflict operations.
- [ ] T029 [AP-08] After freeze, implement temporal Address rules, stored per-target Network/Unit/matched-rule/status, Message contributions/direct Unit and Contribution supports, checked concurrent rematching and suppression. Concrete fields/constraints/indexes follow C-05; no mandatory immutable rule versions. Preserve full Unit/Network/Gateway contracts, including presentation/default/configuration/state capabilities absent from diagram stubs.
- [ ] T030 [AP-08] Test differing Unit/organization deletion effects and default preservation; cover F-11/F-12/F-15/F-16 and concurrent ingest/revocation.

**Acceptance:** No new Transmission/Message from correction, no stale visible Unit references, no secret conflict details, no automatic Payload masking. **Non-goals:** Historical color/icon versioning or unapproved import.

**Output handover:** T028 and T029's standalone current-rule/matcher interface go to [AP-06](ap-06-transmission-messages.md) before intake integration. Historical support redistribution/contribution correction and T030 follow that integration. This is an artifact-level handover, not whole-package completion before AP-06.
