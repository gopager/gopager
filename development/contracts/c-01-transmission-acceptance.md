# C-01 Transmission Acceptance

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-02](c-02-protocol-payload.md), [C-05](c-05-matching-correction.md).

**Implementation planning:** [AP-09](../work-packages/ap-09-gateway-network.md) delivered Gateway/Network administration, diagnostics and contact loss ([handover](../execution/ap-09.md#handover)).

**Traceability:** [REQ-041..050](../requirements-catalog.md#e-transmission-networks-and-gateways), [REQ-108](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [REQ-113..114](../requirements-catalog.md#l-operations-data-volume-and-evolution). Issue references: [P-01/P-03/P-08/P-09](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Status:** Implemented behavior (intake, Network resolution, Gateway state and contact loss, credentials, Network and Gateway administration, diagnostics, re-resolution after mapping changes) is specified once in `docs/` and OpenAPI, linked below; this file keeps only what is still open. **Owner:** Intake; integration owns the shared commit boundary. **Parties:** authorized Gateway/Gateway manager; downstream Grouping, Assignment and Delivery domains.

## Implemented behavior (specified in `docs/` and OpenAPI)

The routes and schemas are defined by [OpenAPI v1](../../api/openapi/v1.yaml) (`info.version` 1.10.0), tags `Ingest` and `Gateways`. Semantics:

- Submission fields, size limits, Network resolution order and priorities, Observation-time rules (offset, truncation, 15-minute plausibility tolerance), idempotency, receipt and rejected submissions: [HTTP API: Ingest, Gateway credential](../../docs/http-api.md#ingest-gateway-credential).
- Protocol targets and Payload identity: [HTTP API: Protocols, targets and payload identity](../../docs/http-api.md#protocols-targets-and-payload-identity).
- Gateway state reports and their facts: [HTTP API: Ingest, Gateway credential](../../docs/http-api.md#ingest-gateway-credential) (Gateway state semantics).
- Network and Gateway lifecycle (create, read, list, rename, enable/disable, soft delete, used-Network refusal), Network replacement, credential issue, status and revoke, diagnostics and state history, with their rights: [HTTP API: Gateway administration](../../docs/http-api.md#gateway-administration); Gateway proof, rotation and blocking: [HTTP API: Gateway authentication](../../docs/http-api.md#gateway-authentication).
- Gateway states including `never-seen`, server-side contact loss (timeout, precedence, `stopped` and disabled exclusion) and one history entry and fact per transition: [HTTP API: Gateway state semantics](../../docs/http-api.md#ingest-gateway-credential), [Architecture: Gateway state and contact loss](../../docs/architecture.md#gateway-state-and-contact-loss-worker-role); configuration: [Operations](../../docs/operations.md#configuration-reference).
- Intake transaction (single commit, no receipt before commit, `COMMIT_UNCERTAIN` for lookup by the same Submission ID): [Architecture: Intake](../../docs/architecture.md#intake-one-transaction).
- Error codes: [HTTP API: Errors](../../docs/http-api.md#errors).
- Effect of a Network-mapping replacement on accepted Transmissions (re-resolution per changed Protocol, Network move, unresolved Transmissions keep their Network; receipts and originals unchanged; [D-139](../decisions/02-intake-and-grouping.md#d-139)): [HTTP API: Gateway administration](../../docs/http-api.md#gateway-administration), [Architecture: Historical correction](../../docs/architecture.md#historical-correction-worker-role); cases F-10e, [F-97..F-100](reference-cases.md#ap-20-gateway-mapping-re-resolution-cases).

Decisions that stay valid and are not repeated elsewhere: a Network has one Protocol and a Gateway may submit for several Networks; an unauthorized declaration is rejected, never silently redirected; the Gateway submits a finalized report (frame, checksum and fragment handling stay in the Gateway adapter, raw RF/audio is not accepted, an optional compact reference/delta format is a possible later optimization); the Gateway never decides Payload equality, Unit mapping or Message membership.

Binding failure rules ([D-139](../decisions/02-intake-and-grouping.md#d-139); evidence: rejection invariant in the [AP-07 record](../execution/ap-07.md#handover), storage and fault cases in AP-16 T082): If the rejection fact cannot be committed, the Submission returns the storage or authority error instead of the rejection code, so a retry records the rejection; no rejection is reported without its record. If the store is unavailable, return the storage or authority error and emit a redacted process diagnostic; do not claim that the fact was committed. Authentication failures never disclose a foreign Gateway or Network. Invalid shape, unauthorized Gateway or Network, conflicts, storage shortage, write failure and uncertain authority stay visible without exposing other Gateways or Networks; no silent deletion to free space; cancellation after a possible commit cannot guarantee absence.
