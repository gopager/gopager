# AP-20 Re-resolution after Gateway Network-Mapping Changes

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** [REQ-043..046](../requirements-catalog.md#e-transmission-networks-and-gateways): this AP's part is re-resolving accepted Transmissions after a Gateway's Network assignments change; the rest of REQ-043..046 is delivered by AP-07, AP-08 and AP-09.
- **Inputs:**
  - [AP-07 handover](../execution/ap-07.md#handover): intake, Network resolution (`ingest.ResolveNetwork`), `transmissions` and `transmission_targets`.
  - [AP-08 handover](../execution/ap-08.md#handover): correction runner, pending scopes, `message_units` rebuild and Processing readmission.
  - [AP-09 handover](../execution/ap-09.md#handover): Gateway assignment replacement and Gateway lifecycle.
- **Owned contract slices:** [C-01](../contracts/c-01-transmission-acceptance.md) and [C-05](../contracts/c-05-matching-correction.md) sections for mapping-change re-resolution; F-10e and new cases at the next free F-numbers in [reference cases](../contracts/reference-cases.md).
- **Reference (read when needed):** [D-139](../decisions/02-intake-and-grouping.md#d-139), [Architecture: Historical correction](../../docs/architecture.md#historical-correction-worker-role).
- **Write scope:**
  - `internal/ingest/` — store the declared Gateway Network identifier; enqueue a correction scope when assignments change.
  - `internal/assignments/` — correction reason and Network re-resolution in the correction batch.
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/ingeststore/`, `internal/persistence/ent/assignmentstore/`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout)).
  - `internal/httpapi/` — correction status fields for unresolved Transmissions, if needed.
  - Added by [D-141](../decisions/03-assignment-and-correction.md#d-141): `internal/events/` (correction fact vocabulary), `cmd/gopager/` (composition only), `internal/persistence/ent/borrowed/` (PostgreSQL test fixtures).
- **Compatibility context:** AP-10 (query visibility follows the corrected Network), AP-11 (no recall of handed-off Deliveries).

## Open User Decisions

None. D-139 fixes the behavior for unresolvable Transmissions.

## Planning Before Coding

T090: settle the scope selector (Gateway and Protocol), which Transmissions are re-resolved (all of the Gateway's Protocol, or only those whose resolution inputs changed), batch order and cursor, the stored declaration column (D-128 in-place edit), the Network move (Transmission, every target, `message_units`, Processing readmission, `assignment.changed` with reason `corrected`), the unresolved report and the interaction with concurrent rule and mapping changes. Destination: C-01, C-05, F-cases.

## Tasks

- [ ] T090 [AP-20] Re-resolve accepted Transmissions after a Gateway Network-mapping change through the correction runner.
  - **Done when:** replacing a Gateway's assignments queues one correction scope; each affected Transmission is re-resolved with its stored declaration, the current assignments and Address rules at its Observation time; a changed Network moves the Transmission and its targets, rebuilds `message_units` and readmits Processing atomically per batch; an unresolvable Transmission keeps its Network and is reported as unresolved; originals, receipts and Message membership never change.
- [ ] T091 [AP-20] Verify F-10e and the new cases on PostgreSQL, including concurrent mapping and rule changes, restart and visibility after the move.
  - **Done when:** the cases exist at their canonical owner and pass with the Verification evidence.

## Documentation to Update

C-01, C-05, reference cases; [docs/architecture.md](../../docs/architecture.md#historical-correction-worker-role), [docs/http-api.md](../../docs/http-api.md#gateway-administration) (effect of a replacement), OpenAPI if correction responses change; Go doc comments.

## Verification

Ent generation, `GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=15m ./internal/persistence/ent/... ./cmd/gopager`, `make`, `go test ./...`.

## Handover

Re-resolution semantics and evidence to AP-10, AP-15 and AP-17.

## Non-goals

Re-grouping Messages, rewriting originals or receipts, recalling Deliveries, re-resolution on Gateway deletion.
