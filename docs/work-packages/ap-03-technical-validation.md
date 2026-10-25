# AP-03 Technical Validation Planning

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

**Owner:** Persistence/operations/plugin host; DOM-06 with DOM-05 for search semantics. **Contracts:** C-01/C-03/C-06..C-12. **REQ:** REQ-011, REQ-017, REQ-089..091, REQ-097, REQ-105..108, REQ-113..120, REQ-127; REQ-126 is removed (D-31). **Current write scope:** Validation decisions in `docs/architecture.md`, provenance/evidence in `docs/requirements-evidence.md`, boundaries in `docs/contracts/` (C-ID files; index: `docs/domain-contracts.md`). No spike code yet.

- [ ] T010 [AP-03] Plan SQLite driver/CGO/build comparison and PostgreSQL parity probe: constraints, idempotency, concurrent Grouping, transactions/outbox, leases, migrations with shared inputs. No full implementations.
- [ ] T011 [AP-03] Define Unicode/accent/short-term fixtures for both stores; check normalization/case folding/index candidates against C-07 rather than substituting search syntax. Verify that UI language/formatting preferences do not alter search semantics or wire values.
- [ ] T012 [AP-03] Plan small adversarial tests for Wasm ABI/toolchain, startup/memory, infinite loops, blocking host calls, I/O/Payload limits, and secret redaction.
- [ ] T013 [AP-03] Document an available constrained reference host and reproducible single-process/shutdown/crash/disk-failure/restart experiment. Plan Full authority loss, superseded workers, and API/SSE revocation together. No application Backup/Restore experiment (D-31).
- [ ] T014 [AP-03] Obtain separate authorization for executable experiments; then record commands, exact versions, environment, results, and limits in `docs/requirements-evidence.md`. Feed negative results back into architecture/contracts.

**Acceptance:** First an approved narrow experimental plan; later actual evidence without SLA/hardware promises. A plan alone does not pass runtime validation. **Non-goals:** Infrastructure deployment, competing complete products, code work under this documentation task.
