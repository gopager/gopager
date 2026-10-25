# AP-06 Transmission, Protocols, and Messages

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-02](ap-02-domain-contracts.md), [AP-04](ap-04-scope-freeze.md), [AP-05](ap-05-runtime-persistence.md), [AP-07](ap-07-identity-authorization.md).

**Owner:** DOM-01/DOM-02; integration owns commit boundary. **Contract gate:** C-01..C-03/C-08, C-05 current matching, C-06 Gateway authorization and C-11 publication. **REQ:** REQ-020, REQ-031..032, REQ-037..038, REQ-041..050, REQ-113..114, REQ-121..122. **Prerequisites:** AP-05/AP-07 plus AP-08's early master-data/matching slice and AP-12 T043's evaluator interface/kernel; not completion of those whole packages. **Exclusive target scopes:** `internal/ingest/`, `internal/protocols/`, `internal/messages/`, with own tests/schema objects.

- [ ] T022 [AP-06] Implement authorized Gateways/Networks, submission, and Gateway-state operations under frozen C-01, distinguishing inferred/default Network resolution from unauthorized declarations.
- [ ] T023 [AP-06] After freeze, implement original/partial typed Payload preservation, server-owned exact comparison separated from sliding Grouping, match kind `exact` and permitted representative preference. Wire C-05 resolution and C-11's ingest publication barrier through agreed owner interfaces; accepted is not published. No heuristics/settings/scores/reconsideration or split machinery. Preserve the comparison boundary without a speculative framework.
- [ ] T024 [AP-06] Test F-01..F-10e and F-33/F-37 publication boundaries: partial originals retained, canonically different values never fuzzy-joined, representative projection, concurrent acceptance, crash/idempotency and atomic facts across backends. Installed-code adapter/provider evidence remains with AP-11/AP-12, not passing proof from a fixture.

**Acceptance:** Replay is not Transmission; independent repeat remains its own evidence; Protocol/time-correct Messages under concurrency. **Non-goals:** Raw radio decoders or unapproved EAS media processing. Focused Go package tests follow actual structure; freeze exact paths/commands in AP-04.
