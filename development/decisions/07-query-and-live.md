# ADR: Query and live

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-16"></a>
## D-16 Search semantics

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed search semantics.
- **Owner detail:** [C-07](../contracts/c-07-query-live.md)

<a id="d-22"></a>
## D-22 Internal-time separation

- **Status:** superseded by D-135 (in part: per-query View time; the internal-time separation stands)
- **Date:** 2026-10-25
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Confirmed internal-time separation; public-time exception superseded by D-28.
- **Owner detail:** [C-03](../contracts/c-03-message-grouping.md), [C-07](../contracts/c-07-query-live.md)

<a id="d-28"></a>
## D-28 Query-time model

- **Status:** superseded by D-135 (in part: per-query View time; Message time orders feeds)
- **Date:** 2026-10-25
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Confirmed query-time model; supersedes universal public Message-time exception.
- **Owner detail:** [C-07 time algorithm](../contracts/c-07-query-live.md#implemented-behavior-specified-in-docs-and-openapi), [C-05 stored minima](../../docs/architecture.md#address-rule-semantics)

<a id="d-152"></a>
## D-152 Field registry, one selection predicate and Subscription health

- **Status:** accepted
- **Date:** 2026-11-11
- **Decision:** One field registry declares every field that Workflows, search, Subscription filters and Layouts may use, with kind, field group, filterability, label and icon; its providers are the built-in fields (replacing the Delivery field catalog, the Processing source declarations and the fixed Query filter dimensions) and the Attribute schemas of [D-150](04-processing-and-plugins.md#d-150). Feed, live stream and saved views accept the typed predicate that Workflows and Subscriptions already use (`eq`, `in`, ordered comparisons, `contains`, `present`, `and`, `or`, `not`), compiled to SQL over registry fields; the simple query parameters stay as shorthand and the `derived` parameter is removed. A Subscription whose filter or Layout no longer compiles becomes `invalid` with a reason code and a Domain event instead of silently matching nothing, and a successful save makes it valid again. The leftover field-group syntax `extension.<plugin>.<field>` is removed and the glossary term "Field set" becomes "Readable field groups". Delivery fan-out builds the Message projection once per event and subscriber authority, adopted only when measurement shows the gain.
- **Rationale:** Three hard-coded catalogs with the same field groups would each have to learn attributes, and search and Subscriptions used different filter languages; the architecture already recommends one declared field and operator model ([Shared use cases](../architecture.md#shared-use-cases)), which also enables the Subscription-preview candidate of the [requirements catalog](../requirements-catalog.md#n-delegated-administration-and-restricted-views). REQ-017 still holds: a predicate that needs an unreadable field group is rejected.
- **Consequences:** The implemented behavior of AP-10 (filters, saved-view format, live) and AP-11 (field catalog, fan-out, Subscription state) changes in [AP-21](../work-packages/ap-21-attribute-schemas.md), with follow-ups in their execution records. AP-15 builds a typed filter editor and shows the Subscription state.
- **Owner detail:** [C-07](../contracts/c-07-query-live.md#planned-and-not-yet-implemented), [C-09](../contracts/c-09-delivery.md#planned-attribute-references-and-subscription-health), [C-06](../contracts/c-06-identity-authorization.md#planned-and-not-yet-implemented)
