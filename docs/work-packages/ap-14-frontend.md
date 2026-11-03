# AP-14 Frontend

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-13 T046..T048](source-ap-13-frontend.md).
- **Requirements and owners:** [REQ-001..030/045/047..050/057..064/069..079/082/084..087/089/091..100/109..110](../requirements-catalog.md), [owner contracts](../domain-contracts.md#contract-register), [F-26](../contracts/reference-cases.md#f-26-localized-display-case).
- **Owner:** Frontend; integration owns API/asset versions and built-asset registration.
- **Prerequisites:** AP-06 through AP-13; API/asset versions and applicable AP-04 freezes are bound before implementation.
- **Write scope:** `web/`; integration owns built-asset registration.

- [ ] T078 [AP-14] Implement feed, search, filters, details, saved links/time anchors, aging, and multi-tab reconciliation.
- [ ] T079 [AP-14] Implement authorized account, master-data, Gateway, subscription, plugin, and bounded rule/presentation management workflows.
- [ ] T080 [AP-14] Implement English-keyed text and independent formatting locale; verify desktop/mobile, keyboard/touch, authorization, accessibility, and interruption states.

**Acceptance:** Browser behavior reflects current server authority and never presents stale/offline state as fresh.

**Non-goals:** PWA/push, external identity, additional translations, automatic Payload translation, or a separately deployed runtime web server unless approved.
