# AP-05 Runtime and Persistence Foundation

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [security](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [operations](../requirements-catalog.md#l-operations-data-volume-and-evolution), [load-profile consideration](../requirements-catalog.md#m-additional-domain-capabilities); select the REQ IDs below.
- **Architecture:** [TECH-01/02/05/06/07](../architecture.md#technology-decisions), [operating profiles](../architecture.md#operating-profiles), [supporting responsibilities](../architecture.md#supporting-responsibilities), [dependency rules](../architecture.md#allowed-dependencies).
- **Owner contracts:** [C-01](../contracts/c-01-transmission-acceptance.md), [C-08](../contracts/c-08-events-jobs.md), [C-12](../contracts/c-12-persistence-operations.md); [shared conventions](../contracts/shared-conventions.md).
- **Cases and prerequisite outputs:** [F-19/F-23/F-25](../contracts/reference-cases.md), [F-40/F-41](../contracts/ap-02-boundary-cases.md), [AP-03 technical evidence](ap-03-technical-validation.md), [AP-04 freeze](ap-04-scope-freeze.md).

**Owner:** Persistence/operations/integration. **Contract gate:** C-01/C-08/C-12. **REQ:** REQ-107, REQ-113..120, REQ-127. **Prerequisites:** AP-04 for this slice. **Proposed exclusive write scopes:** `go.mod`, `go.sum`, `cmd/gopager/`, `internal/platform/`, future central build/migration registration. Domain schema objects remain with domain owners; integration owns registration.

- [ ] T019 [AP-05] Implement approved Go/store versions and composition/role startup; Lite combines the same capabilities in one process, Full uses the same artifact contract.
- [ ] T020 [AP-05] Implement dialect adapters, shared transaction/migration boundary, cancellation/shutdown, safe operational failures under C-12; no universal ORM without benefit.
- [ ] T021 [AP-05] Provide controlled clock/store/provider test boundaries; distinguish real database evidence. Verify foundation against approved C-12 fixtures on both backends.

**Acceptance/evidence:** Compilable startup, backend-parallel tests, single-process proof; no claim of Full HA without operational evidence. **Non-goals:** Inventing domain modules or custom external database HA. **Commands:** Record actual focused commands/results in the [execution record](../execution/README.md), following the [AP-04 T018 binding](ap-04-scope-freeze.md); do not edit this AP during implementation.
