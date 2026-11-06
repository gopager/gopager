# C-05 Assignment, Suppression and Rematching

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-01](c-01-transmission-acceptance.md), [C-06](c-06-identity-authorization.md), [C-09](c-09-delivery.md).

**Traceability:** [REQ-021](../requirements-catalog.md#c-units-organizations-and-defaults), [REQ-031..040](../requirements-catalog.md#d-correct-assignment-and-history), [REQ-123..124](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-08..P-10/P-12](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 1.0 (was 0.1-draft; matching, target resolution, suppression and selector-scoped correction are implemented and now specified once in `docs/` and OpenAPI; the contribution-support, dirty-capture, barrier re-entry and global-revision text is deleted under D-135). **Owner:** Assignment. **Producers:** temporal Address rules, Transmission targets, relevant master-data Domain events. **Consumers:** the Query and Delivery domains using central C-06 evaluation.

## Implemented behavior (specified in `docs/` and OpenAPI)

- Routes and schemas: [OpenAPI v1](../../api/openapi/v1.yaml) (`info.version` 1.1.0), tags `Address rules`, `Suppressions` and `Assignment corrections`; route table, preview, correction progress and retry: [HTTP API: Address rules, suppressions, corrections](../../docs/http-api.md#address-rules-suppressions-corrections). Right scopes: [HTTP API: Authorization](../../docs/http-api.md#authorization). Matching itself has no public route; the internal ports are in [`internal/assignments`](../../internal/assignments/) and the management adapters in [`internal/httpapi`](../../internal/httpapi/).
- Rule shape, half-open validity at the Observation time, selectors per Protocol, set-subset specificity, rejection of equal or partially overlapping rules and of POCSAG mode conflicts, `assigned` and `unassigned` resolution, targetless handling, suppression semantics, original target reference `(transmissionId, targetPosition)` and the `message_units` cache: [Architecture: Address rule semantics](../../docs/architecture.md#address-rule-semantics).
- Initial admission at intake (Network resolution, target matching, cache upsert, one `assignment.changed` per Transmission): [Architecture: Intake](../../docs/architecture.md#intake-one-transaction).
- Selector-scoped correction (scope per rule version, Unit deletion or single suppressed target; assignment-revision stamps on targets; lease and fencing; batches of 100; five attempts and `correction.failed`; re-admission of Processing; `correcting` cache rows): [Architecture: Historical correction](../../docs/architecture.md#historical-correction-worker-role). Failure code `RETRY_LIMIT`: [HTTP API: Errors](../../docs/http-api.md#errors).

Decisions that remain binding: an Address-to-Unit assignment is temporal while general Unit attributes are not; a later-entered rule valid from X corrects history using Observation time; suppression is Transmission-target specific, mixed Messages stay partly usable, a hidden assignment is not relabelled "Without Unit", and no Payload masking or implicit administrator bypass exists; matching results are internal derivations and grant no read right; a removed rule leaves no stale Unit authority; exact replay of earlier derivations is not promised and no immutable Address-rule-version entity exists; correction never writes originals, Grouping or new Transmissions; failed work stays inspectable and an authorized retry never revives superseded work; correction status and retry require coverage of every affected scope and otherwise look like an unavailable job.

## Planned and not yet implemented

- **Rematch debounce.** Rematching should start after 60 seconds of inactivity by default and further changes may not postpone the start beyond 300 seconds by default; both are configurable values and neither is a completion SLA. The runner currently claims due scopes without such a delay.
- **Delivery reuse.** Already created read or Delivery jobs may keep their original domain context; C-09 reuses it only after current access checks. No historical job is rewritten for a cosmetic edit.
