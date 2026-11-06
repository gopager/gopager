# C-02 Protocol Payload and Payload identity

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-01](c-01-transmission-acceptance.md), [C-03](c-03-message-grouping.md), [C-05](c-05-matching-correction.md), [C-06](c-06-identity-authorization.md), [C-07](c-07-query-live.md).

**Traceability:** [REQ-020](../requirements-catalog.md#b-search-filters-and-details), [REQ-031..032/037..038](../requirements-catalog.md#d-correct-assignment-and-history), [REQ-121..122](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-08/P-09](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Status:** Protocol, Payload form, target and identity rules are implemented and specified once in `docs/`; this file keeps the planned rules. **Owner:** Grouping; Intake owns submitted originals. **Parties:** Gateway submits finalized Transmission reports; ingest, Assignment and Grouping consume them.

## Implemented behavior (specified in `docs/`)

- Protocols, target address domains (POCSAG RIC and function, FLEX address classes of ARIB RCR STD-43A Appendix A Table A-1, ZVEI and EAS strings), Payload forms, validation and the canonical Payload identity of each form (text canonicalization, numeric code alphabet, binary subtype, FLEX short signal, ZVEI double tone, EAS header fields): [HTTP API: Protocols, targets and payload identity](../../docs/http-api.md#protocols-targets-and-payload-identity). Typed code: [`internal/protocols`](../../internal/protocols/).
- Original target reference `(transmissionId, targetPosition)`, ordered and never renumbered or deduplicated: [Architecture: Address rule semantics](../../docs/architecture.md#address-rule-semantics).
- How the identity is used: [Architecture: Grouping rules](../../docs/architecture.md#grouping-rules). The processing source projection is described in [Workflows: Source fields](../../docs/workflows.md#source-fields), the synthetic preview source in [Workflows: Preview](../../docs/workflows.md#preview).

Decisions that remain binding and are not covered by those pages:

- The Gateway carries Protocol, original Payload, zero or more targets with Function or signal attributes, context and optional quality metrics. Finalizing a report does not certify complete reception; a partial or erroneous report is retained unchanged (D-29) with its optional `incomplete` marker, while missing required fields or invalid declared-form data are rejected. No decoder, frame, checksum or reassembly internals enter the server contract.
- Gateway and Network are Transmission context, not Grouping keys. Payload equality is never decided from display labels, and search normalization (C-07) is not reused for Grouping. The identity-rule version increases with every change of a form's identity or canonicalization, which is a deliberate versioned amendment and not an additive field.
- Character-set decoding (for example national POCSAG variants) is Protocol handling by the Gateway adapter; the server does not reinterpret characters per Network.
- EAS audio and video, storing it in a filesystem or object store, and SMTP-based or other Protocols are out of scope until a Protocol definition is approved for them.
- A multi-target Transmission belongs to exactly one Message and is never split by target or Unit.

## Planned and not yet implemented

- **Comparison extension boundary (D-30).** Canonical identity construction and candidate comparison stay separate from membership and merge decisions (C-03). The current comparison accepts compatible form and subtype with exactly equal canonical values; hashes only accelerate lookup. A later non-exact comparator would need a separately approved contract for candidate discovery, temporal constraints, ambiguity, policy and versioning, correction and authorization. No heuristic, threshold, score or per-form activation is required now; the membership match kind `exact` is the only one reserved.
- **Permissions.** Protocol Payload and Transmission target fields are not public metadata; every field passes the C-06 projection and unknown new fields receive no automatic grant (see [HTTP API: Authorization](../../docs/http-api.md#authorization)). i18n never translates original Payload before comparison.
- **Compatibility of the FLEX mapping.** The D-34 FLEX form mapping is an identity-rule amendment; planned cases are [F-42..F-44](reference-cases.md#ap-04-protocol-cases).
