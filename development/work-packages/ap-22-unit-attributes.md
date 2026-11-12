# AP-22 Unit Attributes

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:**
  - [REQ-142](../requirements-catalog.md#c-units-organizations-and-defaults) Unit attributes: server side; screens are AP-15.
  - [REQ-144](../requirements-catalog.md#b-search-filters-and-details): this AP's part is filtering on Unit attributes; the typed predicate and Message attributes are [AP-21](ap-21-attribute-schemas.md).
  - [REQ-026](../requirements-catalog.md#c-units-organizations-and-defaults): this AP's part is setting Unit attributes in the existing Unit bulk update.
- **Inputs:**
  - [AP-08 handover](../execution/ap-08.md#handover): Units, Organization defaults with `inherit | value | unset`, templates, Organization deletion, Unit bulk operations.
  - [AP-21 handover](ap-21-attribute-schemas.md#handover): Attribute-schema catalog with subject `unit`, field registry and provider seam, typed feed predicate, Agency binding, shared icon rule.
  - [AP-11 handover](../execution/ap-11.md#handover) ([D-157](../decisions/05-events-and-delivery.md#d-157), replaces the AP-12 handover): the target descriptor port `delivery.TargetDescriptor.Inputs` with typed input kinds, `integration_unit_parameters` and the input precedence of materialization.
  - An absent handover of an earlier open AP is an input gap reported at start.
- **Owned contract slices:** the Unit-attributes bullet of [C-04](../contracts/c-04-master-data.md#planned-and-not-yet-implemented), the Unit-attribute part of [C-07](../contracts/c-07-query-live.md#planned-and-not-yet-implemented), the target-input bullet of [C-09](../contracts/c-09-delivery.md#planned-attribute-references-and-subscription-health); new cases at the next free F-numbers; glossary term Unit attribute.
- **Reference (read when needed):** [D-150](../decisions/04-processing-and-plugins.md#d-150), [D-151](../decisions/06-identity-and-authorization.md#d-151), [D-152](../decisions/07-query-and-live.md#d-152), [D-128](../decisions/01-platform-and-storage.md#d-128), [HTTP API: Master data semantics](../../docs/http-api.md#master-data-semantics).
- **Write scope:**
  - `internal/units/` — Unit, Organization and template attribute values, inheritance resolution, Organization-deletion materialization, template copy, bulk update, Agency-change check.
  - `internal/fields/` — `unit` provider (`unit.attributes.<schemaKey>.<field>…` for search and Delivery, `target.unit.attributes.<schemaKey>.<field>…` for Workflows).
  - `internal/attributes/` — reference checks for Unit values (schema Agency removal and schema deletion).
  - `internal/processing/` — source declarations and capture of the effective Unit attributes of a target.
  - `internal/readmodel/` — Unit-attribute filter correlation and projection.
  - `internal/delivery/` — Unit-attribute fields and target-input binding.
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/unitstore/`, `internal/persistence/ent/ingeststore/` (source projection), `internal/persistence/ent/readstore/`, `internal/persistence/ent/deliverystore/`, `internal/persistence/ent/attributestore/`, generated Ent code, `00004_current_assignments.sql` edited in place (per [package layout](shared-rules.md#package-layout)).
  - `internal/httpapi/`, `api/openapi/v1.yaml` — attribute values on Units, Organizations, templates and Unit bulk update; target-input binding in Subscription target configuration.
  - `cmd/gopager/` — composition only.
  - Focused unit and PostgreSQL integration tests.
- **Compatibility context:** [AP-13](ap-13-processing-presentation-integration.md) may show Unit attributes in the card model; [AP-15](ap-15-frontend.md) builds the attribute editors on Unit, Organization, template and bulk screens. Their code is not required.

## Open User Decisions

None. D-150 fixes inheritance, no rights effect and no automatic reprocessing.

## Planning Before Coding

**T095.** Settle and record in C-04: the storage shape of values on Units, Organizations and templates (following the `settings` columns), validation against the `unit` schemas assigned to the Unit's Agency, inheritance resolution, materialization on Organization deletion, template copy, bulk-update semantics, the check on a Unit's Agency change, and the reference checks that block removing a schema's Agency or deleting a schema while Unit values use it.

**T096.** Settle and record in C-07, C-09 and [docs/workflows.md](../../docs/workflows.md#source-fields): the registry declarations and field group `unit`; the feed correlation with the Message's eligible Unit appearances; how Processing captures the effective values at run time without changing the assignment context revision; the target-input binding shape, its precedence against Unit parameters and Subscription values, and the kind compatibility with the plugin-declared input.

## Tasks

- [ ] T095 [AP-22] Implement Unit attribute values with Organization inheritance, templates, bulk update and Agency checks.
  - **Done when:** values validate against `unit` schemas assigned to the Unit's Agency; `inherit | value | unset` resolve like Unit settings, including Organization deletion and template copy; bulk update sets attributes all-or-nothing; a Unit's Agency change with values of unassigned schemas is rejected; removing a schema's Agency or deleting a schema used by Unit values is `409`; edits change no grants and create no correction or Processing work.
- [ ] T096 [AP-22] Expose Unit attributes to search, Subscriptions, Layouts, Workflows and Delivery target inputs.
  - **Done when:** feed, saved views and Subscription filters select by `unit.attributes…` correlated with the same Unit appearance; Layouts render Unit attributes with their labels and icons; Workflows read `target.unit.attributes…` with group `unit` and results keep the values of their run; a target input bound to a Unit attribute receives the Delivery Unit's effective value; the new F-cases pass.

## Documentation to Update

- Development: [C-04](../contracts/c-04-master-data.md), [C-07](../contracts/c-07-query-live.md), [C-09](../contracts/c-09-delivery.md), reference cases, [glossary](../glossary.md), follow-ups in the AP-08 execution record per the [boundary-crossing procedure](shared-rules.md#boundary-crossing-procedure) step 4.3.
- Permanent: [docs/http-api.md](../../docs/http-api.md) (master data, feed filter, Subscription targets), [docs/workflows.md](../../docs/workflows.md) (source fields), [api/openapi/v1.yaml](../../api/openapi/v1.yaml), Go doc comments.

## Verification

`make`, `go test ./...`, `make docs-check`, `make test-race` (PostgreSQL integration of unit, ingest, read and delivery stores and HTTP).

## Handover

To AP-15: Unit, Organization, template and bulk attribute APIs and the target-input binding. To AP-13: Unit attributes in the registry for the card model. To AP-17: evidence E-IDs.

## Non-goals

Rights or Unit collections derived from attributes, automatic reprocessing after attribute edits, attributes on Agencies, Networks or Gateways, value history.
