# AP-07 Transmission Intake

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-06 T022..T024](source-ap-06-transmission-messages.md); prior blocked outcomes remain in the [source AP-06 execution record](../execution/source-ap-06.md).
- **Requirements and owners:** [REQ-020/031..032/037..050/113..114/121..122](../requirements-catalog.md#e-transmission-networks-and-gateways), [C-01](../contracts/c-01-transmission-acceptance.md), [C-02](../contracts/c-02-protocol-payload.md), [C-03](../contracts/c-03-message-grouping.md), [C-05](../contracts/c-05-matching-correction.md), [C-06](../contracts/c-06-identity-authorization.md), [C-08](../contracts/c-08-events-jobs.md), [C-11](../contracts/c-11-processing-presentation.md).
- **Owner:** DOM-01/DOM-02; integration owns the commit and publication boundary.
- **Prerequisites:** AP-06; the already verified identity-core and current-matcher handovers from historical [source AP-07](../execution/source-ap-07.md) and [source AP-08](../execution/source-ap-08.md), both produced before this ordered path began; applicable AP-04 freezes and this slice's exact execution binding.
- **Write scope:** `internal/ingest/`, `internal/protocols/`, `internal/messages/`, with owner schemas and focused tests.

- [ ] T057 [AP-07] Implement Gateway/Network authorization, Submission acceptance, and Gateway state operations under the frozen owner contracts.
- [ ] T058 [AP-07] Implement typed original preservation, exact comparison, current target admission, Grouping, and the frozen Processing publication barrier.
- [ ] T059 [AP-07] Verify the complete acceptance transaction, retries/concurrency/crash behavior, facts, Grouping boundaries, and publication atomicity against the assigned PostgreSQL cases.

**Acceptance:** A valid accepted Submission produces the contractually correct Transmission/Message state atomically; repeats, concurrency, and failures preserve the approved boundary.

**Non-goals:** Raw radio decoders, unapproved EAS media processing, historical correction jobs, external Delivery effects, or plugin/provider implementation.
