# AP-08 Master Data and Correction

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-05](ap-05-runtime-persistence.md), [AP-07](ap-07-identity-authorization.md).

**Owner:** DOM-03/DOM-04; integration coordinates cross-domain deletion. **Contract gate:** C-02/C-04..C-06/C-08. **REQ:** REQ-019..040, REQ-095, REQ-123..124, REQ-132. **Prerequisites by slice:** AP-05/AP-07 for T028 and T029's standalone current-rule/matching kernel using frozen original-target interfaces; AP-06 for historical correction/ingest integration and T030 integration fixtures. **Exclusive target scopes:** `internal/Units/`, `internal/assignments/`, with tests/schema objects.

- [ ] T028 [AP-08] Implement Units/organizations/Unit collections/templates/defaults and authorized bulk/conflict operations.
- [ ] T029 [AP-08] After freeze, implement temporal Address rules, stored per-target Network/Unit/matched-rule/status, Message contributions/direct Unit and Contribution supports, checked concurrent rematching and suppression. Concrete fields/constraints/indexes follow C-05; no mandatory immutable rule versions. Preserve full Unit/Network/Gateway contracts, including presentation/default/configuration/state capabilities absent from diagram stubs.
- [ ] T030 [AP-08] Test differing Unit/organization deletion effects and default preservation; cover F-11/F-12/F-15/F-16 and concurrent ingest/revocation.

**Acceptance:** No new Transmission/Message from correction, no stale visible Unit references, no secret conflict details, no automatic Payload masking. **Non-goals:** Historical color/icon versioning or unapproved import.

**AP-02 dependency handover:** Provide the early master-data/current-rule/matcher slice before AP-06 intake integration; finish historical support redistribution/contribution correction against AP-06 afterward. Artifact-level staging avoids AP-06 -> whole AP-08 -> AP-06 circular execution without adding task IDs or executing this package.
