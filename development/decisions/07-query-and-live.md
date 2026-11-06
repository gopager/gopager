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
- **Owner detail:** [C-07 time algorithm](../contracts/c-07-query-live.md#planned-and-not-yet-implemented-ap-10), [C-05 stored minima](../../docs/architecture.md#address-rule-semantics)
