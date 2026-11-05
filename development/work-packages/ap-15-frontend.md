# AP-15 Frontend

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-001..030/045/047..050/057..064/069..079/082/084..087/089/091..100/109..110](../requirements-catalog.md), [owner contracts](../domain-contracts.md#contract-register), [F-26](../contracts/reference-cases.md).
- **Owner:** Frontend with integration participating in this AP for build/asset registration; backend API versions come from their completed producers.
- **Architecture:** [language and internationalization](../architecture.md#language-and-internationalization), [domain ownership](../architecture.md#domains-and-data-ownership), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verify [AP-08](ap-08-master-data-correction-after-intake.md) master-data/default APIs, [AP-09](ap-09-gateway-network.md) Gateway/Network APIs, [AP-10](ap-10-query-live.md) Query/SSE/approved saved views, [AP-11](ap-11-events-delivery.md) Subscription/Delivery, [AP-12](ap-12-plugin-host.md) configuration, [AP-13](ap-13-processing-presentation-integration.md) rules/presentation/preferences/notices and [AP-14](ap-14-identity-lifecycle.md) identity/recovery/deletion. Consume actual OpenAPI/error/action versions and HTTP evidence. Assets are this AP's output; no operations/sign-off input is required.
- **Write scope:** `web/`, its dependency/lock/build/test manifests, root packaging/build/CI, `cmd/gopager/` and `internal/httpapi/` for built-asset registration on the existing listener. This AP owns frontend toolchain/version selection and generated assets; no replacement backend domain APIs.

## Planning Before Coding

Record the user-story-to-screen/route/API mapping in implementation/tests before coding; do not duplicate server specifications.

- **Later-consumer planning context (not prerequisites):** [AP-16](ap-16-operations.md) and [AP-17](ap-17-integration-evidence.md). Do not create operations APIs here.
- For T078, define feed/search/detail flows, filter and saved-link state, time-anchor behavior, aging indicators, multi-tab reconciliation, and loading/empty/error/interrupted states using the fixed AP-10 API.
- For T079, map each authorized account, master-data, Gateway, subscription, plugin and bounded rule/presentation workflow to its fixed server API and permission outcomes from AP-08..AP-14. Include saved views only according to AP-10's recorded product disposition and visibility contract; no browser-side substitute policy.
- For T080, define English keys/fallback/typed interpolation/plurals without sentence concatenation, independent persisted formatting locale/time zone, desktop/mobile/keyboard/touch accessibility, copy/navigation preservation and non-color-only authorization/interruption states. Bind flows to relevant REQ/F cases; no additional translations or required language selector.

- [ ] T078 [AP-15] Implement feed, search, filters, details, saved links/time anchors, aging, and multi-tab reconciliation.
- [ ] T079 [AP-15] Implement authorized account, master-data, Gateway, subscription, plugin, and bounded rule/presentation management workflows.
- [ ] T080 [AP-15] Implement English-keyed text and independent formatting locale; verify desktop/mobile, keyboard/touch, authorization, accessibility, and interruption states.

## Build, Browser Evidence and Handover

Choose/pin the maintained Vue-compatible toolchain/i18n/test libraries against [C-11](../contracts/c-11-processing-presentation.md), D-36 and the architecture. Define English key/typed-parameter validation, formatting locale/time zone, responsive breakpoints, accessibility expectations and route/API test matrix before coding. Build deployable assets and register them in the actual application distribution; no deployment-time npm install or separately managed web server.

Run the selected manifest's build/type/lint/unit/browser commands and real browser flows against disposable completed backend data: current fields/options/counts, multiple tabs, reconnect/removal/revocation, unknown/hidden references, denied management, desktop/mobile keyboard/touch, notice dismissal and invalid preferences. Test field refinements REQ-135..137 and accepted saved-view/Gateway delegation scenarios. Do not depend on a later operations UI or assert unexecuted browser evidence. Hand over exact toolchain/artifact hashes/build paths, asset registration, API versions and browser results to AP-16/AP-17.

**Acceptance:** Browser behavior reflects current server authority and never presents stale/offline state as fresh.

**Non-goals:** PWA/push, external identity, additional translations, automatic Payload translation, or a separately deployed runtime web server unless approved.
