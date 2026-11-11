# AP-21 Attribute Schemas, Agency Binding and Field Registry

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:**
  - [REQ-141](../requirements-catalog.md#j-processing-presentation-and-usability) Attribute schemas and Workflow outputs, [REQ-143](../requirements-catalog.md#j-processing-presentation-and-usability) Agency-bound Workflows, Layouts and Attribute schemas, [REQ-144](../requirements-catalog.md#b-search-filters-and-details) typed attribute filters and [REQ-145](../requirements-catalog.md#i-domain-events-extensions-and-deliveries) Subscription health: server side; screens are AP-15.
  - REQ-144 for Unit attributes and all of [REQ-142](../requirements-catalog.md#c-units-organizations-and-defaults) belong to [AP-22](ap-22-unit-attributes.md); this AP provides the catalog subject `unit` and the registry provider seam they use.
  - Changed implemented behavior of REQ-091..095 (AP-06, AP-11) and REQ-011..017/133/138 (AP-10) under [D-150](../decisions/04-processing-and-plugins.md#d-150), [D-151](../decisions/06-identity-and-authorization.md#d-151) and [D-152](../decisions/07-query-and-live.md#d-152).
- **Inputs:**
  - [AP-06 handover](../execution/ap-06.md#handover): Workflow definition, compiler, evaluator, `processing_results` with stored `field_groups`.
  - [AP-07 handover](../execution/ap-07.md#handover): Network resolution at intake and candidate admission.
  - [AP-08 handover](../execution/ap-08.md#handover): Agencies, Unit settings with Organization inheritance, Agency deletion.
  - [AP-10 handover](../execution/ap-10.md#handover): feed SQL, `derived` filter, saved views, live stream, delivery projection port.
  - [AP-11 handover](../execution/ap-11.md#handover): Delivery field catalog, Subscription filter and Layout compilation, fan-out, start check, Layout store and renderer.
- **Owned contract slices:** the D-150..D-152 sections of [C-11](../contracts/c-11-processing-presentation.md#attribute-schemas-and-agency-binding-planned), [C-07](../contracts/c-07-query-live.md#planned-and-not-yet-implemented), [C-09](../contracts/c-09-delivery.md#planned-attribute-references-and-subscription-health), [C-06](../contracts/c-06-identity-authorization.md#planned-and-not-yet-implemented) and the Agency-deletion bullet of [C-04](../contracts/c-04-master-data.md#planned-and-not-yet-implemented); new cases at the next free F-numbers in [reference cases](../contracts/reference-cases.md); glossary terms Attribute schema, Message attribute, Effective attribute value, Field registry, Subscription health and Readable field groups.
- **Reference (read when needed):** [D-128](../decisions/01-platform-and-storage.md#d-128) (in-place migration edits), [D-148](../decisions/04-processing-and-plugins.md#d-148), [D-68](../decisions/06-identity-and-authorization.md#d-68), [D-69](../decisions/06-identity-and-authorization.md#d-69), [D-135](../decisions/10-foundation-redesign.md#d-135) items 3 and 15, [shared use cases](../architecture.md#shared-use-cases), [domain ownership](../architecture.md#domains-and-data-ownership).
- **Write scope:**
  - `internal/attributes/` (new) — Attribute-schema model, validation, compatibility rules, management service, Agency binding rules.
  - `internal/fields/` (new) — field registry: built-in provider (moved from `internal/delivery/fields.go` `staticFields` and `internal/processing/synthetic_declarations.go`, renamed without "synthetic") and attribute provider; implements `processing.Declarations` and `processing.FieldGroupDeclarations`.
  - `internal/processing/` — definition format (`outputs` into Message attributes, `outputSchema` removed), compiler, `ValidateEvaluation`, Workflow scope with Agencies and narrowing, `management` service authorization, Layout block `icon` and label/icon defaults in `layout.go`/`layout_render.go`, `presentation` Layout Agencies.
  - `internal/delivery/` — registry-based field references, effective-value lookup including target results, Layout choice rule, Subscription `health`, compiled-predicate cache, shared projection.
  - `internal/readmodel/` — typed `filter` predicate, saved-view definition, effective attributes with provenance in projections.
  - `internal/units/` — shared icon identifier rule (`canonicalIcon` moves to a package shared with attributes and layouts).
  - `internal/authz/` — `attribute-schema.manage`, `ValidField` without `extension.*`.
  - `internal/events/` — `subscription.health-changed` vocabulary.
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/processingstore/`, `internal/persistence/ent/readstore/`, `internal/persistence/ent/deliverystore/`, `internal/persistence/ent/layoutstore/`, `internal/persistence/ent/unitstore/` (Agency-deletion guard), a new `internal/persistence/ent/attributestore/`, generated Ent code; migrations `00006_processing_kernel.sql`, `00012_query_live.sql`, `00013_events_delivery.sql` edited in place and a new `00014_attribute_schemas.sql` (per [package layout](shared-rules.md#package-layout)).
  - `internal/httpapi/`, `api/openapi/v1.yaml` — Attribute-schema routes, Workflow scope, Layout `agencies` and `icon`, feed `filter`, saved-view format, Subscription `health`.
  - `cmd/gopager/` — composition only.
  - Focused unit and PostgreSQL integration tests and fixtures that create Workflows, results, Layouts or Subscriptions.
- **Compatibility context:** [AP-22](ap-22-unit-attributes.md) adds Unit-attribute values and the `unit` registry provider; [AP-13](ap-13-processing-presentation-integration.md) builds the card model, layout layers and Reprocess on the Agency-bound model; [AP-15](ap-15-frontend.md) builds the schema editor, typed filter editor, Agency assignment and health screens; [AP-16](ap-16-operations.md) receives the projection table as retention participant if T094 adds it. Their code is not required.

## Open User Decisions

None. D-150..D-152 record the user's decisions of 2026-11-10/11.

## Planning Before Coding

**T092.** Settle and record in C-11 and C-06: the Attribute-schema key syntax and limits, the JSON shape of fields with display metadata, the compatibility rules as an executable check, deprecation visibility, delete-reference checks, the `00014_attribute_schemas.sql` tables (`attribute_schemas`, `attribute_schema_agencies`) and the Agency tables of Workflows and Layouts in their owning migrations, the Workflow scope shape and admission query, the authorization of Agency changes (added and removed), the field-registry interfaces (kind, group, filterable, label, icon) and the providers' ownership in the [architecture](../architecture.md#domains-and-data-ownership).

**T093.** Settle and record in C-07 and C-09: the feed `filter` encoding (query parameter and saved-view field), the predicate-to-SQL compiler over effective values and per-target visibility, the saved-view definition version, the provenance fields, the `result` → `attributes` reference change in Subscriptions and Layouts, Layout choice authorization, the Subscription `health` columns, reason codes, event payload and the transactions that set and clear it.

**T094.** Settle the measurement: seeded data volume, the feed queries with attribute filters, the Delivery fan-out query count per event, and the acceptance threshold for adopting the typed projection table, the compiled-predicate cache and the shared projection. Destination: C-07 and C-09 performance bullets and the execution record.

## Tasks

- [ ] T092 [AP-21] Implement the Attribute-schema catalog, the field registry, Agency binding of Workflows, Layouts and Attribute schemas, and Workflow outputs into Message attributes.
  - **Done when:** schemas validate, change only compatibly and keep their keys; display metadata edits need no new version; Workflows compile only against `message` schemas and persist only Message attributes; Workflow scope `all` is gone and admission follows assigned Agencies and narrowing; managing or reassigning a definition needs the right in every Agency involved; an Agency that is a definition's last Agency cannot be deleted; `attribute-schema.manage` exists and `extension.*` field groups are rejected; processing, delivery and read model use the same registry declarations.
- [ ] T093 [AP-21] Switch search, live, saved views, Subscriptions and Layouts to registry fields with the typed predicate, effective values with provenance, Layout label/icon defaults and Subscription health.
  - **Done when:** a typed predicate selects the same Messages in the feed as the equivalent Subscription filter matches; unknown or non-filterable paths and unreadable groups are rejected; two Workflows writing one field give one effective value with provenance in feed, detail, Delivery and Layout; target results reach Delivery; a Workflow version change no longer breaks a Subscription; a Layout field without overrides renders its schema label and icon and `text`/`markdown` ignore icons; a Subscription whose filter stops compiling turns `invalid` with its reason and event, and a valid save clears it; the new F-cases pass.
- [ ] T094 [AP-21] Measure and, where the threshold is met, add the typed projection of effective values, the compiled-predicate cache and the shared Delivery projection.
  - **Done when:** the before/after `EXPLAIN ANALYZE` and query counts are recorded; adopted parts keep all T093 results; parts below the threshold are recorded as not adopted with their numbers.

## Documentation to Update

- Development: [C-11](../contracts/c-11-processing-presentation.md), [C-07](../contracts/c-07-query-live.md), [C-09](../contracts/c-09-delivery.md), [C-06](../contracts/c-06-identity-authorization.md), [C-04](../contracts/c-04-master-data.md), reference cases, [glossary](../glossary.md), [architecture](../architecture.md#domains-and-data-ownership) (registry and catalog ownership), follow-ups in the AP-06, AP-08, AP-10 and AP-11 execution records per the [boundary-crossing procedure](shared-rules.md#boundary-crossing-procedure) step 4.3.
- Permanent: [docs/workflows.md](../../docs/workflows.md) (definition format, outputs, scope, classification limitation), [docs/http-api.md](../../docs/http-api.md) (Attribute schemas, Workflow scope, Layouts, feed filter, saved views, Subscriptions, field groups), [docs/architecture.md](../../docs/architecture.md), [api/openapi/v1.yaml](../../api/openapi/v1.yaml), Go doc comments.

## Verification

`make`, `go test ./...`, `make docs-check`, `make test-race` (PostgreSQL integration of processing, read, delivery and layout stores and HTTP), and the T094 measurements on seeded PostgreSQL data.

## Handover

To AP-22: catalog with subject `unit`, registry provider seam, shared icon rule. To AP-13: Agency-bound Workflow scope for Reprocess, Layout Agencies for layout layers, registry labels and icons for the card model. To AP-15: Attribute-schema, Agency-assignment, typed filter, provenance and Subscription-health APIs. To AP-16: retention participants added by T094, if any. To AP-17: evidence E-IDs.

## Non-goals

Unit-attribute values (AP-22), per-output classification (known limitation in D-150), rights derived from attributes, manually entered Message attributes, automatic reprocessing, rendering icons in text or Markdown output.
