# AP-15 Frontend

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:**
  - Browser side of [REQ-001..030/045/047..050/057..064/069..071/073..075/077..079/082/084..087/089/091..100/109..110/132](../requirements-catalog.md); their server behavior comes from the producing APs. REQ-135..137 field refinements are tested here.
  - [REQ-128](../requirements-catalog.md#m-additional-domain-capabilities) exchange of Units, Address rules, Suppressions and Unit collections between servers, and [REQ-140](../requirements-catalog.md#c-units-organizations-and-defaults) bulk changes of Address rules, Suppressions and Unit collections: server operations (T089) and screens, both in this AP ([D-138](../decisions/09-process-and-documentation.md#d-138)). Bulk changes of Units (REQ-026) already exist server-side ([AP-08 handover](../execution/ap-08.md#handover)).
- **Inputs:** (verify each against the linked handover and the actual OpenAPI/error/action versions and HTTP evidence; assets are this AP's output, no operations/sign-off input is required)
  - [AP-08 handover](../execution/ap-08.md#handover): master-data, default and Agency APIs.
  - [AP-09](ap-09-gateway-network.md): Gateway/Network APIs; verify `development/execution/ap-09.md#handover` exists at start.
  - [AP-10](ap-10-query-live.md): Query/SSE and approved saved views (product disposition and visibility contract); verify `development/execution/ap-10.md#handover` exists at start.
  - [AP-11](ap-11-events-delivery.md): Subscription and Delivery APIs; verify `development/execution/ap-11.md#handover` exists at start.
  - [AP-12](ap-12-plugin-host.md): plugin configuration; verify `development/execution/ap-12.md#handover` exists at start.
  - [AP-13](ap-13-processing-presentation-integration.md): Workflows, presentation, preferences and banner; verify `development/execution/ap-13.md#handover` exists at start.
  - [AP-14](ap-14-identity-lifecycle.md): identity, recovery and deletion; verify `development/execution/ap-14.md#handover` exists at start.
- **Owned contract slices:** the browser side of [C-11](../contracts/c-11-processing-presentation.md#text-key-mapping-and-remaining-technical-evidence) (English text keys, formatting locale), [F-26](../contracts/reference-cases.md), the user-story-to-screen/route/API mapping, the frontend toolchain/version selection and generated assets; for T089 the exchange-document and bulk sections of the [planned C-04](../contracts/c-04-master-data.md#planned-and-not-yet-implemented) (Units, collections) and of [C-05](../contracts/c-05-matching-correction.md) (Address rules, Suppressions), with their F-cases. No backend domain APIs except the T089 exchange and bulk operations. Other owner contracts per the [contract register](../domain-contracts.md#contract-register) are consumed, not changed.
- **Reference (read when needed):** [language and internationalization](../architecture.md#language-and-internationalization), [domain ownership](../architecture.md#domains-and-data-ownership), [allowed dependencies](../architecture.md#allowed-dependencies), [C-11](../contracts/c-11-processing-presentation.md), [D-36](../decisions/01-platform-and-storage.md#d-36) (library policy), [delivery scope](../authoritative-plan.md#delivery-scope).
- **Write scope:**
  - `web/` (new) — frontend sources, tests, dependency/lock/build/test manifests
  - T089 only: `internal/units/`, `internal/assignments/` (exchange and bulk operations), `internal/httpapi/` handlers and `api/openapi/v1.yaml` for these operations, `internal/persistence/ent/unitstore/`, `internal/persistence/ent/assignmentstore/`, `internal/persistence/ent/schema/`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout))
  - repository root packaging, build and CI files — build and asset packaging
  - `internal/httpapi/` — built-asset registration on the existing listener, plus the T089 handlers listed above
  - `cmd/gopager/` — composition only
- **Compatibility context:** [AP-16](ap-16-operations.md) and [AP-17](ap-17-integration-evidence.md) receive the toolchain, artifact hashes, build paths, asset registration, API versions and browser results. Do not create operations APIs here.

## Open User Decisions

None.

## Planning Before Coding

Record the user-story-to-screen/route/API mapping in implementation/tests before coding; do not duplicate server specifications.

T078: define feed/search/detail flows, Agency and Unit filters, saved-link state, Message-time ordering and per-Unit first times in details, aging indicators, multi-tab reconciliation, and loading/empty/error/interrupted states using the fixed AP-10 API. Destination: the route/API test matrix in `web/`.

T079: map each authorized account, master-data, Gateway, subscription, plugin and bounded Workflow/presentation workflow (including the layout-definition editor of REQ-094, the bulk screens for Units (REQ-026) and for Address rules, Suppressions and collections (REQ-140), and the exchange screens of REQ-128 over T089's operations) to its fixed server API and permission outcomes from AP-08..AP-14. Include saved views only according to AP-10's recorded product disposition and visibility contract; no browser-side substitute policy. Destination: the route/API test matrix in `web/`.

T080: define English keys/fallback/typed interpolation/plurals without sentence concatenation, independent persisted formatting locale/time zone, desktop/mobile/keyboard/touch accessibility, copy/navigation preservation and non-color-only authorization/interruption states. Bind flows to relevant REQ/F cases; no additional translations or required language selector. Destination: the key/typed-parameter validation in `web/` and the C-11 text-key mapping.

T089 (assigned planning; decide and record in the C-04/C-05 planned sections, cases as F-85 onward in the [reference cases](../contracts/reference-cases.md)):
- **Document:** one JSON document with a `formatVersion` field, its schema in OpenAPI; no separate API version. Export selects per kind (Units with their Address rules, Suppressions, Unit collections) by explicit IDs, by a Unit filter or all items the caller may read.
- **References:** no natural keys exist today (no unique names on Units, Agencies, Radio networks or collections). Agencies and Radio networks are never created by an import; the import request carries a mapping from exported to existing Agencies and Radio networks, and the preview proposes matches by name. Items are created new by default; updating an existing item requires an explicit target-item mapping in the request (the preview proposes name matches within the same Agency) and passes the normal revision check.
- **Execution:** the preview is read-only and lists per item `create`, `update` or `reject` with a reason. Execution repeats every check and is all or nothing in one transaction, like `/units/bulk`: any rejected item (unresolved reference, missing right, revision conflict) rejects the run. Authorization equals the single-item operations; an import never broadens access.
- **Bounds:** one run must cover the planning profile of 300 Units with their rules; choose and record the exact item and size limits.
- **Bulk (REQ-140):** Address rules, Suppressions and Unit collections (fields, deletions and for collections membership changes) with preview and all-or-nothing execution, 1–100 items, per-item errors, as `/units/bulk`.
- **Correction:** each executed import or bulk run that changes Address rules or Suppressions enqueues the existing selector-scoped correction once ([D-135](../decisions/10-foundation-redesign.md#d-135) item 7); no second correction path.

Toolchain: choose and pin the maintained Vue-compatible toolchain/i18n/test libraries against [C-11](../contracts/c-11-processing-presentation.md), [D-36](../decisions/01-platform-and-storage.md#d-36) and the architecture. Define English key/typed-parameter validation, formatting locale/time zone, responsive breakpoints, accessibility expectations and the route/API test matrix before coding. Build deployable assets and register them in the actual application distribution; no deployment-time npm install or separately managed web server. Destination: `docs/development.md` and the execution record.

## Tasks

- [ ] T078 [AP-15] Implement feed, search, filters, details, saved links/time anchors, aging, and multi-tab reconciliation.
  - **Done when:** the feed/search/detail flows, Agency and Unit filters, saved-link state, Message-time ordering, per-Unit first times, aging indicators and multi-tab reconciliation work against the fixed AP-10 API in a real browser, including loading/empty/error/interrupted states, reconnect/removal/revocation, unknown/hidden references and field refinements REQ-135..137.
- [ ] T079 [AP-15] Implement authorized account, master-data (including Agencies, bulk changes and exchange), Gateway, subscription, plugin, and bounded Workflow/presentation management workflows.
  - **Done when:** each mapped management workflow works against its AP-08..AP-14 server API with the permission outcomes (denied management included), the accepted saved-view/Gateway delegation scenarios pass, and the bulk and exchange screens work against the Unit bulk API and T089's operations.
- [ ] T080 [AP-15] Implement English-keyed text and independent formatting locale; verify desktop/mobile, keyboard/touch, authorization, accessibility, and interruption states.
  - **Done when:** [F-26](../contracts/reference-cases.md) passes (complete keys/parameters/plurals, English fallback, formatting independent of UI language), desktop/mobile and keyboard/touch checks pass, the static banner and invalid preferences behave as contracted, and browser behavior reflects current server authority and never presents stale/offline state as fresh.
- [ ] T089 [AP-15] Implement the server operations for exchanging Units, Address rules, Suppressions and Unit collections between servers (REQ-128) and for bulk changes of Address rules, Suppressions and Unit collections (REQ-140).
  - **Done when:** export of one, several, all or a filtered selection produces the versioned document; importing it on a second database previews and then applies creates/updates/rejections as planned, rejects unresolved references and unauthorized items, and triggers exactly one correction per change; bulk preview and execution for the three kinds behave like `/units/bulk`; OpenAPI and the C-04/C-05 F-cases pass with real PostgreSQL and HTTP.

## Documentation to Update

- [docs/development.md](../../docs/development.md): `Repository Layout`, `Build`, `Tests` (and `Code Generation` if assets or clients are generated) for the frontend toolchain, build, lint, unit and browser commands.
- [docs/operations.md](../../docs/operations.md): `Build` for the built-asset distribution and the absence of a separate web server or deployment-time npm install.
- [docs/architecture.md](../../docs/architecture.md): `Package map` / `Packages` for the `web/` package and asset registration.
- [C-11](../contracts/c-11-processing-presentation.md) text-key mapping section and [F-26](../contracts/reference-cases.md) status; for T089 the C-04/C-05 sections, their F-cases, [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md#route-reference) (Agencies, Organizations, Unit templates, Units, collections; Address rules and Suppressions); [glossary](../glossary.md) for new terms.
- Root [README.md](../../README.md) `Getting Started` if the build steps change; Go doc comments for the asset-registration identifiers in `internal/httpapi/`.

## Verification

For T089 run focused unit/assignment race tests and `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager` with an export from one and import into a second database. Run the selected manifest's build/type/lint/unit/browser commands and real browser flows against disposable completed backend data: current fields/options/counts, multiple tabs, reconnect/removal/revocation, unknown/hidden references, denied management, desktop/mobile keyboard/touch, the static banner and invalid preferences. Test field refinements REQ-135..137 and accepted saved-view/Gateway delegation scenarios. Record each as an E-ID. Do not assert unexecuted browser evidence.

## Handover

Hand over to AP-16 and AP-17: exact toolchain/artifact hashes/build paths, asset registration, API versions, the exchange-document version and bulk/exchange routes, and browser results.

## Non-goals

PWA/push, external identity, additional translations, automatic Payload translation, or a separately deployed runtime web server unless approved.
