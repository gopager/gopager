# AP-15 Frontend

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** [REQ-001..030/045/047..050/057..064/069..071/073..079/082/084..087/089/091..100/109..110](../requirements-catalog.md), REQ-128 (Unit import/export, promoted by [D-135](../decisions/10-foundation-redesign.md#d-135)); REQ-135..137 field refinements are tested here.
- **Inputs:** (verify each against the linked handover and the actual OpenAPI/error/action versions and HTTP evidence; assets are this AP's output, no operations/sign-off input is required)
  - [AP-08 handover](../execution/ap-08.md#handover): master-data, default and Agency APIs.
  - [AP-09](ap-09-gateway-network.md): Gateway/Network APIs; verify `development/execution/ap-09.md#handover` exists at start.
  - [AP-10](ap-10-query-live.md): Query/SSE and approved saved views (product disposition and visibility contract); verify `development/execution/ap-10.md#handover` exists at start.
  - [AP-11](ap-11-events-delivery.md): Subscription and Delivery APIs; verify `development/execution/ap-11.md#handover` exists at start.
  - [AP-12](ap-12-plugin-host.md): plugin configuration; verify `development/execution/ap-12.md#handover` exists at start.
  - [AP-13](ap-13-processing-presentation-integration.md): Workflows, presentation, preferences and banner; verify `development/execution/ap-13.md#handover` exists at start.
  - [AP-14](ap-14-identity-lifecycle.md): identity, recovery and deletion; verify `development/execution/ap-14.md#handover` exists at start.
- **Owned contract slices:** the browser side of [C-11](../contracts/c-11-processing-presentation.md#text-key-mapping-and-remaining-technical-evidence) (English text keys, formatting locale), [F-26](../contracts/reference-cases.md), the user-story-to-screen/route/API mapping, the frontend toolchain/version selection and generated assets. No replacement backend domain APIs. Other owner contracts per the [contract register](../domain-contracts.md#contract-register) are consumed, not changed.
- **Reference (read when needed):** [language and internationalization](../architecture.md#language-and-internationalization), [domain ownership](../architecture.md#domains-and-data-ownership), [allowed dependencies](../architecture.md#allowed-dependencies), [C-11](../contracts/c-11-processing-presentation.md), [D-36](../decisions/01-platform-and-storage.md#d-36) (library policy), [delivery scope](../authoritative-plan.md#delivery-scope).
- **Write scope:**
  - `web/` — frontend sources, tests, dependency/lock/build/test manifests
  - repository root packaging, build and CI files — build and asset packaging
  - `internal/httpapi/` — built-asset registration on the existing listener only
  - `cmd/gopager/` — composition only
- **Compatibility context:** [AP-16](ap-16-operations.md) provides the Unit CSV import/export operation that the REQ-128 screens consume; [AP-16](ap-16-operations.md) and [AP-17](ap-17-integration-evidence.md) receive the toolchain, artifact hashes, build paths, asset registration, API versions and browser results. Do not create operations APIs here and do not depend on a later operations UI.

## Open User Decisions

None.

## Planning Before Coding

Record the user-story-to-screen/route/API mapping in implementation/tests before coding; do not duplicate server specifications.

T078: define feed/search/detail flows, Agency and Unit filters, saved-link state, Message-time ordering and per-Unit first times in details, aging indicators, multi-tab reconciliation, and loading/empty/error/interrupted states using the fixed AP-10 API. Destination: the route/API test matrix in `web/`.

T079: map each authorized account, master-data, Gateway, subscription, plugin and bounded Workflow/presentation workflow (including the layout-definition editor of REQ-094 and the Unit CSV import/export screens of REQ-128 over AP-16's operation) to its fixed server API and permission outcomes from AP-08..AP-14. Include saved views only according to AP-10's recorded product disposition and visibility contract; no browser-side substitute policy. Destination: the route/API test matrix in `web/`.

T080: define English keys/fallback/typed interpolation/plurals without sentence concatenation, independent persisted formatting locale/time zone, desktop/mobile/keyboard/touch accessibility, copy/navigation preservation and non-color-only authorization/interruption states. Bind flows to relevant REQ/F cases; no additional translations or required language selector. Destination: the key/typed-parameter validation in `web/` and the C-11 text-key mapping.

Toolchain: choose and pin the maintained Vue-compatible toolchain/i18n/test libraries against [C-11](../contracts/c-11-processing-presentation.md), [D-36](../decisions/01-platform-and-storage.md#d-36) and the architecture. Define English key/typed-parameter validation, formatting locale/time zone, responsive breakpoints, accessibility expectations and the route/API test matrix before coding. Build deployable assets and register them in the actual application distribution; no deployment-time npm install or separately managed web server. Destination: `docs/development.md` and the execution record.

## Tasks

- [ ] T078 [AP-15] Implement feed, search, filters, details, saved links/time anchors, aging, and multi-tab reconciliation.
  - **Done when:** the feed/search/detail flows, Agency and Unit filters, saved-link state, Message-time ordering, per-Unit first times, aging indicators and multi-tab reconciliation work against the fixed AP-10 API in a real browser, including loading/empty/error/interrupted states, reconnect/removal/revocation, unknown/hidden references and field refinements REQ-135..137.
- [ ] T079 [AP-15] Implement authorized account, master-data (including Agencies and Unit import/export), Gateway, subscription, plugin, and bounded Workflow/presentation management workflows.
  - **Done when:** each mapped management workflow works against its AP-08..AP-14 server API with the permission outcomes (denied management included), the accepted saved-view/Gateway delegation scenarios pass, and the Unit import/export screens consume AP-16's operation.
- [ ] T080 [AP-15] Implement English-keyed text and independent formatting locale; verify desktop/mobile, keyboard/touch, authorization, accessibility, and interruption states.
  - **Done when:** [F-26](../contracts/reference-cases.md) passes (complete keys/parameters/plurals, English fallback, formatting independent of UI language), desktop/mobile and keyboard/touch checks pass, the static banner and invalid preferences behave as contracted, and browser behavior reflects current server authority and never presents stale/offline state as fresh.

## Documentation to Update

- [docs/development.md](../../docs/development.md): `Repository Layout`, `Build`, `Tests` (and `Code Generation` if assets or clients are generated) for the frontend toolchain, build, lint, unit and browser commands.
- [docs/operations.md](../../docs/operations.md): `Build` for the built-asset distribution and the absence of a separate web server or deployment-time npm install.
- [docs/architecture.md](../../docs/architecture.md): `Package map` / `Packages` for the `web/` package and asset registration.
- [C-11](../contracts/c-11-processing-presentation.md) text-key mapping section and [F-26](../contracts/reference-cases.md) status; [glossary](../glossary.md) for new terms.
- Root [README.md](../../README.md) `Getting Started` if the build steps change; Go doc comments for the asset-registration identifiers in `internal/httpapi/`.

## Verification

Run the selected manifest's build/type/lint/unit/browser commands and real browser flows against disposable completed backend data: current fields/options/counts, multiple tabs, reconnect/removal/revocation, unknown/hidden references, denied management, desktop/mobile keyboard/touch, the static banner and invalid preferences. Test field refinements REQ-135..137 and accepted saved-view/Gateway delegation scenarios. Record each as an E-ID. Do not assert unexecuted browser evidence.

## Handover

Hand over to AP-16 and AP-17: exact toolchain/artifact hashes/build paths, asset registration, API versions and browser results.

## Non-goals

PWA/push, external identity, additional translations, automatic Payload translation, or a separately deployed runtime web server unless approved.
