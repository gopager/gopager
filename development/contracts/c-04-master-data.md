# C-04 Master Data

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-05](c-05-matching-correction.md), [C-10](c-10-plugin-host.md).

**Traceability:** [REQ-019..020](../requirements-catalog.md#b-search-filters-and-details), [REQ-021..030](../requirements-catalog.md#c-units-organizations-and-defaults), [REQ-095](../requirements-catalog.md#j-processing-presentation-and-usability), [REQ-132](../requirements-catalog.md#m-additional-domain-capabilities). Documentary coverage: [AP-02](../work-packages/ap-02-domain-contracts.md#documentary-completion-and-coverage); freeze: [AP-04](../work-packages/ap-04-scope-freeze.md).

**Status:** Implemented behavior (Agencies, Units, Organizations, templates, collections, bulk operations) is specified once in `docs/` and OpenAPI; this file keeps the planned rules. **Owner:** Master data. **Parties:** authorized management; the Assignment, Query and Delivery domains consume current master data.

## Implemented behavior (specified in `docs/` and OpenAPI)

Routes and schemas: [OpenAPI v1](../../api/openapi/v1.yaml) (`info.version` 1.6.0), tags `Agencies`, `Organizations`, `Unit templates`, `Units` and `Unit collections`. Semantics: [HTTP API: Agencies, Organizations, Unit templates, Units, collections](../../docs/http-api.md#agencies-organizations-unit-templates-units-collections) and [Master data semantics](../../docs/http-api.md#master-data-semantics) (Unit identity, exactly one Agency, collection membership, `inherit`/`value`/`unset` settings, color, icon and Function-meaning rules, name limit, templates as copied snapshots, Organization deletion with materialized defaults and no Rematch, Unit deletion marker and pending correction, bulk operations with their 100-item limit, required rights). Authorization of these routes: [HTTP API: Authorization](../../docs/http-api.md#authorization). Correction after Unit deletion: [Architecture: Historical correction](../../docs/architecture.md#historical-correction-worker-role). Typed code: [`internal/units`](../../internal/units/).

Decisions that remain binding: formation membership alone is not evidence of co-alerting and there is no unrestricted multi-parent default graph; a Unit is not subordinate to a Network; no historical color or icon versioning is needed for general master data; conflict details never reveal hidden uses elsewhere; a concurrent Organization and Unit edit conflicts instead of losing a value.

## Planned and not yet implemented

- **Extension fields.** Dropped by [D-135](../decisions/10-foundation-redesign.md#d-135): per-Unit integration values use the per-integration parameter table owned by [C-09](c-09-delivery.md) (AP-11, AP-12).
- **Icon catalog and renderer.** The server stores and copies an icon identifier only. [AP-15](../work-packages/ap-15-frontend.md) (T079/T080) defines the identifier set from a bundled open-licensed icon set chosen under [D-36](../decisions/01-platform-and-storage.md#d-36), the renderer, and the fallback for an unknown identifier (an accessible label, never color alone), and records the result here replacing this line. Unit and Organization color and icon render consistently in management and Message views (REQ-022).
- **Exchange and bulk (REQ-128, REQ-140).** Server-to-server exchange of Units with their Address rules, Suppressions and Unit collections, and bulk changes of collections, are planned and implemented by [AP-15 T089](../work-packages/ap-15-frontend.md#planning-before-coding) ([D-138](../decisions/09-process-and-documentation.md#d-138)); the exchange document and bulk rules are defined here and in [C-05](c-05-matching-correction.md) by that task.
