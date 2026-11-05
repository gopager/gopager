# AP-07 Transmission Intake

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [scope/approval](../authoritative-plan.md#workflow-and-approval-gates), [exact implementation binding](addenda/ap-07-intake-implementation.md).
- **Requirements and owners:** [REQ-020/031..032/037..050/113..114/121..122](../requirements-catalog.md#e-transmission-networks-and-gateways), [C-01](../contracts/c-01-transmission-acceptance.md), [C-02](../contracts/c-02-protocol-payload.md), [C-03](../contracts/c-03-message-grouping.md), [C-05](../contracts/c-05-matching-correction.md), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-11](../contracts/c-11-processing-presentation.md).
- **Owner:** Intake/Grouping; integration owns the commit and publication boundary.
- **Prerequisites:** [AP-06 kernel](../execution/ap-06.md#handover), [identity-core](../execution/source-ap-07.md#handover) and [current matcher](../execution/source-ap-08.md#handover); exact versions/paths/checks are in the implementation binding.
- **Write scope:** `internal/ingest/`, `internal/protocols/`, `internal/messages/`, with owner schemas and focused tests.

- [ ] T057 [AP-07] Implement Gateway/Network authorization, Submission acceptance, and Gateway state operations under the frozen owner contracts.
- [ ] T058 [AP-07] Implement typed original preservation, exact comparison, current target admission, Grouping, and the frozen Processing publication barrier.
- [ ] T059 [AP-07] Verify the complete acceptance transaction, retries/concurrency/crash behavior, facts, Grouping boundaries, and publication atomicity against the assigned PostgreSQL cases.

T057/T058 preserve inferred/default versus declared Network authorization, partial typed originals, exact-only comparison separated from sliding Grouping and permitted representative preference. Wire current matching and the frozen publication barrier in the same coordinated transaction; accepted is not published. T059 covers [F-01..F-10e](../contracts/reference-cases.md), [F-42..F-44](../contracts/reference-cases.md#ap-04-protocol-cases), partial originals, representative projection, retries/concurrent intake/crash/atomic facts and the binding's publication cases. Retain backend-neutral expected outcomes without claiming SQLite parity.

**Acceptance:** A valid accepted Submission produces the contractually correct Transmission/Message state atomically; repeats, concurrency, and failures preserve the approved boundary.

**Non-goals:** Raw radio decoders, unapproved EAS media processing, historical correction jobs, external Delivery effects, or plugin/provider implementation.
