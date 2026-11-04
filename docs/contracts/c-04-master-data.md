# C-04 Units, Organizations, and Unit Collections

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-05](c-05-matching-correction.md), [C-10](c-10-plugin-host.md).

**Traceability:** [REQ-019..020](../requirements-catalog.md#b-search-filters-and-details), [REQ-021..030](../requirements-catalog.md#c-units-organizations-and-defaults), [REQ-095](../requirements-catalog.md#j-processing-presentation-and-usability), [REQ-132](../requirements-catalog.md#m-additional-domain-capabilities). Documentary coverage: [AP-02](../work-packages/ap-02-domain-contracts.md#documentary-completion-and-coverage); freeze: [AP-04](../work-packages/ap-04-scope-freeze.md).

**Version/status:** 0.1-draft. **Owner:** DOM-03. **Parties:** authorized management; DOM-04/DOM-06/DOM-07 consume current master data. **Draft interfaces:** `CreateUnit`, `ChangeUnit`, `DeleteUnit`, `DeleteOrganization`, `ChangeUnitCollectionMembership`, `PreviewBulkChange`; each takes Principal, domain input, and expected revision for changes. Results: new reference/revision or authorized conflict/preview state.

**Defined:** Units have their own identity, may belong to several Unit collections, and are not necessarily subordinate to one Network. Organization, color, icon, templates, and selective overrides remain recognizable; returning to organization defaults is possible. Formation membership alone is not evidence of co-alerting. No unrestricted multi-parent default graph.

**Deletion:** Unit deletion coordinates C-05 Rematching of every potentially affected historical target resolution; no new Transmission records/Messages. Organization deletion preserves/detaches Units and retains previously effective organization defaults; no Rematching. Current organization filters/rights follow the new reference. Deleted Units have a nonselectable deletion marker until correction/reference cleanup completes; no stale visible Unit or scope expansion.

**Authorization/failures:** Creating an organization within a Unit dialog separately requires organization permission. Bulk/deletion checks each target; conflict details do not reveal hidden uses elsewhere. No silent overwrite of concurrent edits. Color/icon-only changes do not rematch.

**Transaction/time:** Consistently capture inherited defaults before detachment; reliably coordinate reference/permission changes and required correction work. C-05 does not require historical color/icon versioning for general master data.

### AP-08 Unit Template Direction

**Owner:** DOM-03. **Status:** standalone ownership approved; [AP-08](../work-packages/ap-08-master-data-correction-after-intake.md) defines exact authorization in C-06 before implementation.

A Unit template is a standalone master-data resource, not owned by an Organization. It may carry an optional Organization reference and the supported Unit creation settings. Creating a Unit from a template copies its current settings; the Unit has no live template relationship, and later template edits do not change it. Any copied Organization reference becomes the Unit's ordinary Organization relationship and follows the inheritance rules above.

Template operations require explicit current C-06 authorization, not inferred `organization.*` or `unit.*` rights.

## AP-02 Concrete Master-data Contract

- Unit: stable ID/revision, required nonempty name (maximum 256 Unicode scalars), optional one Organization, optional color (`#RRGGBB`), icon reference from configured catalog, Function meanings per Protocol/code, zero or more Unit collection memberships, plugin-namespaced Extension fields. No implied Network ownership. Organization/Unit collection/template names use the same name limit; names are not identity keys.
- Color/icon/Function-meaning entries each use either an explicit value or inheritance. Clearing an override restores inheritance; a null effective value means unset, not a fabricated default. A template is a copied creation snapshot, not a live multi-parent inheritance graph.
- `DeleteOrganization` reads the effective defaults and revisions, materializes each still-inherited effective value on its Units, detaches them, deletes the Organization and invalidates affected rights/filters in one transaction. Overrides remain unchanged; no Address-rule/target-resolution mutation or Rematch job. Concurrent Organization/Unit edit conflicts rather than loses a value.
- `PreviewBulkChange` returns caller-visible per-item validation and the exact expected revisions. Execution rechecks each item and permission; all-or-nothing transaction, no partial writes. Hidden shared use produces a generic forbidden/conflict outcome, not foreign identifiers.
- `DeleteUnit` atomically marks the Unit deleted, disables/removes its rules from the current rule set, records affected correction scope/revision and durable Rematch work. Current grants on that ID become ineffective, not global. Physical cleanup follows successful reference correction. Cosmetic edits never schedule Rematching.
- `DeclareExtensionFields` is owned by C-10, storage by DOM-03: definition has plugin namespace/version, type, validation/default, sensitivity/read and write capability. Unknown/new fields are inaccessible by default. Schema incompatibility blocks activation rather than coercing saved values.

## Early Master-data Handover Candidate

**Version/status:** master-data-core 1.0, frozen 2026-10-27 by explicit user approval under [D-60](../authoritative-plan.md#early-master-data-and-matcher-prerequisite-proposal). **Owner:** DOM-03; integration owns composition. **Producer:** the AP-06-required portion of AP-08 T028; **consumers:** standalone T029 and AP-06 target context/authorization. Only Unit identity/name/revision/deletion, Unit collections/membership and their current matcher/authority participation are included for early implementation, subject to separate execution approval. The full master-data contract above is preserved for later extension, not declared implemented or wholly frozen by this subset.

### Data and Operation Forms

The authoritative master-data-core 1.0 DTOs, internal context and validation are [the Unit owner types](../../internal/Units/model.go); [the service](../../internal/Units/service.go) defines the implemented operations. Persistence uses [the owned Ent schemas](../../internal/persistence/ent/schema/assignments.go), [Goose migration 00004](../../internal/persistence/migrations/00004_current_assignments.sql) and [the native-filtered adapter](../../internal/persistence/ent/unitstore/store.go). ID/revision/deletion are server-owned, not writable properties. Unit collection has no implied Network/Organization ownership; membership is the explicit Unit-to-collection relation.

Organization/defaults, color/icon/Function meanings, creation templates, extension-field registry and general bulk operations remain later T028 work. Their meanings and deletion/default-preservation obligations above are not removed or replaced with empty placeholders. Keep the early owner ports separate from presentation fields so their later additive implementation does not require replacing the matcher/authorization boundary or changing existing Unit IDs. The early update operation changes only the named supported fields; it must never erase future unrelated configuration via a whole-row replacement.

`CreateUnit` and `CreateUnitCollection` take authenticated proof, actor-bound `commandId` UUID and typed creation values. Read/list operations return only authorized typed DTOs and ordinary bounded pages. Update/delete operations take ID and `expectedRevision`; updates change supported typed fields, not arbitrary merge-patched JSON. The resulting revision or safe deletion/pending status is returned. Creation retries compare the complete validated command input and return the same nonsecret result; changed content with the same actor/command is `REVISION_CONFLICT`. General command receipt retention/expiry follows shared conventions, not Submission retention.

`ChangeUnitCollectionMembership` takes Unit ID/revision plus the complete desired collection-ID set; it checks current permission/existence of all affected uses and increments the Unit revision atomically. Collection deletion updates affected Unit revisions and membership without changing original evidence. This operation is not a general bulk-edit engine.

The authoritative version-1 HTTP request/response shapes, route registrations, bounded JSON decoding and page validation are [the management adapter](../../cmd/gopager/management.go); [root composition](../../cmd/gopager/main.go) retains one primary store/listener and [owner scope dispatch](../../cmd/gopager/scopes.go). Unit replacement requires an explicit membership list, including an explicit empty list to clear it. Stored/internal entities are never serialized directly. No Organization/template/bulk API is promised by this early slice. Actual HTTP/PostgreSQL evidence belongs to [the AP-08 execution record](../execution/source-ap-08.md#e-03-postgresql-race-http-and-proxy-validation).

### Matcher and Authority Participation

The internal `ReadUnitContext(transaction, unitId)` returns current ID/revision/deletion marker and collection references; no original Payload or authority is returned. `ScopeExists(transaction, kind, id)` handles current Unit/Unit-collection references and returns false for deleted targets. Network existence/Protocol belongs to DOM-01 and is supplied through a typed transaction-participating owner port, not a foreign table query or fabricated Network. A missing owner adapter fails explicitly with `AUTHORITY_UNAVAILABLE`; standalone tests may supply explicit fixture owners but cannot count as completed production Network composition.

All protected operations resolve current C-06 authority in the same live platform transaction, filter before count/page/mutation, and check every affected use before shared-resource mutation. Collection operations cannot infer authority over hidden members; errors do not list those uses. Read DTOs project only permitted fields; no Organization grant dimension or new permission catalog is introduced.

Domain changes, safe audit, applicable access/configuration revisions, command result and required [C-08 configuration facts](c-08-events-jobs.md#early-master-data-fact-candidate) commit together. Before historical owners exist, Unit deletion persists the nonselectable marker, current-rule invalidation and durable affected correction scope/revision atomically; it returns pending correction, never falsely reports historical cleanup completed. Historical scanning/publication belongs to later T029/T030 integration. Organization deletion remains deferred, with its distinct default-preservation/no-rematch semantics unchanged above.

Invalid shape/reference is `INVALID_INPUT`, denied mutation is `FORBIDDEN`, inaccessible/missing reads use indistinguishable `NOT_FOUND`, and stale expected state is `REVISION_CONFLICT`. Resource/storage/authority/uncertain-commit outcomes follow C-07/C-12, without silent empty/default fallback. Operation time comes from the primary clock; Unit revision is not Address-rule domain-validity time. Positive/negative validation is bound in the [AP-04 consumer proposal](../execution/ap-04.md#ap-08-prerequisite-consumer-binding-proposal).
