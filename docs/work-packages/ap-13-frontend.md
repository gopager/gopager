# AP-13 Vue Frontend

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-07](ap-07-identity-authorization.md), [AP-12](ap-12-processing-presentation.md).

**Owner:** Frontend; integration owns API/asset versions. **Contract gate:** C-04..C-11/C-13 for the approved slice. **REQ:** REQ-001..030, REQ-045, REQ-047..050, REQ-060..064, REQ-069..079, REQ-084..087, REQ-089, REQ-091..100, REQ-109..110. **Prerequisites:** Related backend contracts/operations from AP-07..AP-12. **Exclusive target scopes:** `web/`, including the future English text catalog and formatting layer; built asset integration `internal/webassets/` by integration only, no mandatory separate runtime web server.

- [ ] T046 [AP-13] Implement feed/search/filters/details, saved links/time anchors, aging highlights, multiple tabs, safe reconciliation; route product-owned UI text through text keys from the start.
- [ ] T047 [AP-13] Implement authorized Unit/organization/Gateway/account/subscription/plugin management and bounded rule/presentation editors; make previews/errors/revisions usable with English keyed validation/status/accessibility text.
- [ ] T048 [AP-13] Select/integrate a suitable Vue i18n facility and English text catalog/fallback; support interpolation/plurals, independent persisted formatting locale, dates/numbers/time zones, and no sentence concatenation. Test F-26, key completeness, copy/navigation preservation, phone/desktop touch/keyboard, layout resilience, and non-color-only states. Require no additional translations or language selector; hidden controls never replace backend authorization.

**Acceptance:** Real desktop/mobile browser workflows with permitted/forbidden/revoked data; offline/live interruption never implies freshness. English-only catalogs render complete user-visible text, fallback/parameters/plurals work, and formatting locale is independent. Original Payloads and operator data remain unchanged. **Non-goals:** Premature PWA/push/OIDC/SCIM UI, additional translated languages, automatic Payload translation. Select actual frontend commands after toolchain choice; do not pretend an npm application already exists.

