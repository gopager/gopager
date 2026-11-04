# AP-14 Frontend

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-13 T046..T048](source-ap-13-frontend.md).
- **Requirements and owners:** [REQ-001..030/045/047..050/057..064/069..079/082/084..087/089/091..100/109..110](../requirements-catalog.md), [owner contracts](../domain-contracts.md#contract-register), [F-26](../contracts/reference-cases.md#f-26-localized-display-case).
- **Owner:** Frontend; integration owns API/asset versions and built-asset registration.
- **Architecture:** [language and internationalization](../architecture.md#language-and-internationalization), [domain ownership](../architecture.md#domains-and-data-ownership), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verified AP-06 through AP-13 handovers, including the exact API/asset versions and contract behavior already provided by those APs. Only applicable contract/decision slices already fixed by earlier work are inputs; any frontend-only decisions needed by T078-T080 are planned and resolved within AP-14. Do not depend on AP-15 or AP-16.
- **Write scope:** `web/`; integration owns built-asset registration.

## Planning Before Coding

Complete this planning as the first part of T078-T080; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Record the user-story-to-screen/route/API mapping in the implementation/test structure and track it within those existing task IDs; do not create a duplicate API specification:

- **Later-consumer planning context (not prerequisites):** [AP-15 Operations](ap-15-operations.md) and [AP-16 Integration and Delivery Evidence](ap-16-integration-evidence.md). Use them to keep status visibility and built-asset/evidence integration compatible; do not implement operations or final delivery evidence early.
- For T078, define feed/search/detail flows, filter and saved-link state, time-anchor behavior, aging indicators, multi-tab reconciliation, and loading/empty/error/interrupted states using the fixed AP-09 API.
- For T079, map each authorized account, master-data, Gateway, subscription, plugin and bounded rule/presentation workflow to its fixed server API and permission outcomes from AP-07..AP-13. Identify missing prior-AP behavior as a handover gap; do not create a replacement API or pull forward server work.
- For T080, define English-keyed text and independent formatting locale, desktop/mobile layouts, keyboard/touch and accessibility expectations, and how current authorization/interruption state is shown. Bind all flows to the relevant REQ/F cases; do not add translations or later-AP operations.

- [ ] T078 [AP-14] Implement feed, search, filters, details, saved links/time anchors, aging, and multi-tab reconciliation.
- [ ] T079 [AP-14] Implement authorized account, master-data, Gateway, subscription, plugin, and bounded rule/presentation management workflows.
- [ ] T080 [AP-14] Implement English-keyed text and independent formatting locale; verify desktop/mobile, keyboard/touch, authorization, accessibility, and interruption states.

**Acceptance:** Browser behavior reflects current server authority and never presents stale/offline state as fresh.

**Non-goals:** PWA/push, external identity, additional translations, automatic Payload translation, or a separately deployed runtime web server unless approved.
