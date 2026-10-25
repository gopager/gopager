# C-04 Units, Organizations, and Unit Collections

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-05](c-05-matching-correction.md), [C-10](c-10-plugin-host.md).

**Traceability:** [REQ-019..020](../requirements-catalog.md#b-search-filters-and-details), [REQ-021..030](../requirements-catalog.md#c-units-organizations-and-defaults), [REQ-095](../requirements-catalog.md#j-processing-presentation-and-usability), [REQ-132](../requirements-catalog.md#m-additional-domain-capabilities). Documentary coverage: [AP-02](../work-packages/ap-02-domain-contracts.md#documentary-completion-and-coverage); freeze: [AP-04](../work-packages/ap-04-scope-freeze.md).

**Version/status:** 0.1-draft. **Owner:** DOM-03. **Parties:** authorized management; DOM-04/DOM-06/DOM-07 consume current master data. **Draft interfaces:** `CreateUnit`, `ChangeUnit`, `DeleteUnit`, `DeleteOrganization`, `ChangeUnitCollectionMembership`, `PreviewBulkChange`; each takes Principal, domain input, and expected revision for changes. Results: new reference/revision or authorized conflict/preview state.

**Defined:** Units have their own identity, may belong to several Unit collections, and are not necessarily subordinate to one Network. Organization, color, icon, templates, and selective overrides remain recognizable; returning to organization defaults is possible. Formation membership alone is not evidence of co-alerting. No unrestricted multi-parent default graph.

**Deletion:** Unit deletion coordinates C-05 Rematching of every potentially affected historical target resolution; no new Transmission records/Messages. Organization deletion preserves/detaches Units and retains previously effective organization defaults; no Rematching. Current organization filters/rights follow the new reference. Deleted Units have a nonselectable deletion marker until correction/reference cleanup completes; no stale visible Unit or scope expansion.

**Authorization/failures:** Creating an organization within a Unit dialog separately requires organization permission. Bulk/deletion checks each target; conflict details do not reveal hidden uses elsewhere. No silent overwrite of concurrent edits. Color/icon-only changes do not rematch.

**Transaction/time:** Consistently capture inherited defaults before detachment; reliably coordinate reference/permission changes and required correction work. C-05 does not require historical color/icon versioning for general master data.

## AP-02 Concrete Master-data Contract

- Unit: stable ID/revision, required nonempty name (maximum 256 Unicode scalars), optional one Organization, optional color (`#RRGGBB`), icon reference from configured catalog, Function meanings per Protocol/code, zero or more Unit collection memberships, plugin-namespaced Extension fields. No implied Network ownership. Organization/Unit collection/template names use the same name limit; names are not identity keys.
- Color/icon/Function-meaning entries each use either an explicit value or inheritance. Clearing an override restores inheritance; a null effective value means unset, not a fabricated default. A template is a copied creation snapshot, not a live multi-parent inheritance graph.
- `DeleteOrganization` reads the effective defaults and revisions, materializes each still-inherited effective value on its Units, detaches them, deletes the Organization and invalidates affected rights/filters in one transaction. Overrides remain unchanged; no Address-rule/target-resolution mutation or Rematch job. Concurrent Organization/Unit edit conflicts rather than loses a value.
- `PreviewBulkChange` returns caller-visible per-item validation and the exact expected revisions. Execution rechecks each item and permission; all-or-nothing transaction, no partial writes. Hidden shared use produces a generic forbidden/conflict outcome, not foreign identifiers.
- `DeleteUnit` atomically marks the Unit deleted, disables/removes its rules from the current rule set, records affected correction scope/revision and durable Rematch work. Current grants on that ID become ineffective, not global. Physical cleanup follows successful reference correction. Cosmetic edits never schedule Rematching.
- `DeclareExtensionFields` is owned by C-10, storage by DOM-03: definition has plugin namespace/version, type, validation/default, sensitivity/read and write capability. Unknown/new fields are inaccessible by default. Schema incompatibility blocks activation rather than coercing saved values.
