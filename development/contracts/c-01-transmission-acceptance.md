# C-01 Transmission Acceptance

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-02](c-02-protocol-payload.md), [C-05](c-05-matching-correction.md).

**Implementation planning:** [AP-09](../work-packages/ap-09-gateway-network.md) owns Gateway/Network administration, diagnostics and contact-loss. Frozen intake/credential/state-report versions remain inputs.

**Traceability:** [REQ-041..050](../requirements-catalog.md#e-transmission-networks-and-gateways), [REQ-108](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [REQ-113..114](../requirements-catalog.md#l-operations-data-volume-and-evolution). Issue references: [P-01/P-03/P-08/P-09](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 1.2 (was 0.1-draft; 1.2 removes the implemented intake, Network resolution, Gateway state report and credential text, which now live in `docs/` and OpenAPI, and drops the D-135 historical blocks). The implemented behavior (AP-05 intake-commit boundary, AP-07 HTTP surface and credential management) is specified once in the pages linked below; this file keeps only what AP-09 still has to deliver. **Owner:** Intake; integration owns the shared commit boundary. **Parties:** authorized Gateway/Gateway manager; downstream Grouping, Assignment and Delivery domains.

## Implemented behavior (specified in `docs/` and OpenAPI)

The routes and schemas are defined by [OpenAPI v1](../../api/openapi/v1.yaml) (`info.version` 1.1.0), tags `Ingest` and `Gateways`. Semantics:

- Submission fields, size limits, Network resolution order and priorities, Observation-time rules (offset, truncation, 15-minute plausibility tolerance), idempotency, receipt and rejected submissions: [HTTP API: Ingest, Gateway credential](../../docs/http-api.md#ingest-gateway-credential).
- Protocol targets and Payload identity: [HTTP API: Protocols, targets and payload identity](../../docs/http-api.md#protocols-targets-and-payload-identity).
- Gateway state reports and their facts: [HTTP API: Ingest, Gateway credential](../../docs/http-api.md#ingest-gateway-credential) (Gateway state semantics).
- Network replacement, credential issue, status and revoke, with their rights: [HTTP API: Gateway administration](../../docs/http-api.md#gateway-administration); Gateway proof and rotation: [HTTP API: Gateway authentication](../../docs/http-api.md#gateway-authentication).
- Intake transaction (single commit, no receipt before commit, `COMMIT_UNCERTAIN` for lookup by the same Submission ID): [Architecture: Intake](../../docs/architecture.md#intake-one-transaction).
- Error codes: [HTTP API: Errors](../../docs/http-api.md#errors).

Decisions that stay valid and are not repeated elsewhere: a Network has one Protocol and a Gateway may submit for several Networks; an unauthorized declaration is rejected, never silently redirected; the Gateway submits a finalized report (frame, checksum and fragment handling stay in the Gateway adapter, raw RF/audio is not accepted, an optional compact reference/delta format is a possible later optimization); the Gateway never decides Payload equality, Unit mapping or Message membership.

## Still planned (AP-09)

- **Administration of Gateways and Networks.** The HTTP API has no route that creates, lists, updates or deletes Gateways or Networks (only Network assignment replacement and credentials exist). AP-09 defines them under the `gateway.*` and `network.*` actions, with `expectedRevision` and actor-bound `commandId` as for other management routes.
- **Gateway state completeness.** `never seen` is a state distinct from `active`, `degraded`, `stopped` and `contact lost`, and contact loss is detected by the server without any caller report, using the heartbeat expectation and timeout of [shared conventions](shared-conventions.md#ap-02-common-schemas-and-limits). Last Transmission time and last heartbeat time stay separate. Missing contact proves no crash cause; removed history does not disprove a previous Transmission.
- **Diagnostics.** Contact, reported diagnostics and the history of state transitions must stay stored or exactly reconstructable, so that authorized managers can see which Gateways produced evidence for a Message and whether a Gateway had problems; projection stays subject to C-06/C-07. Reception diagnostics are defined by [C-07](c-07-query-live.md).
- **Configuration change effects.** A change of the Gateway's Network mapping or of Address rules triggers C-05 Rematching without rewriting the declaration or retroactively treating an accepted Submission as rejected. If the current configuration no longer permits a safe resolution, the affected derivation is invalidated and a pending or failed state is shown to authorized managers; no Network or Unit is fabricated and the original receipt does not change.
- **Rejection invariant.** If the rejection fact cannot be committed, the Submission returns the storage or authority error instead of the rejection code, so a retry records the rejection; no rejection is reported without its record. If the store is unavailable, return the storage or authority error and emit a redacted process diagnostic; do not claim that the fact was committed. Authentication failures never disclose a foreign Gateway or Network.
- **Failure visibility.** Invalid shape, unauthorized Gateway or Network, conflicts, storage shortage, write failure and uncertain authority stay visible without exposing other Gateways or Networks; no silent deletion to free space; cancellation after a possible commit cannot guarantee absence.
