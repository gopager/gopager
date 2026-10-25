# AP-05 Runtime and Persistence Foundation

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-04](ap-04-scope-freeze.md).

**Owner:** Persistence/operations/integration. **Contract gate:** C-01/C-08/C-12. **REQ:** REQ-107, REQ-113..120, REQ-127. **Prerequisites:** AP-04 for this slice. **Proposed exclusive write scopes:** `go.mod`, `go.sum`, `cmd/gopager/`, `internal/platform/`, future central build/migration registration. Domain schema objects remain with domain owners; integration owns registration.

- [ ] T019 [AP-05] Implement approved Go/store versions and composition/role startup; Lite combines the same capabilities in one process, Full uses the same artifact contract.
- [ ] T020 [AP-05] Implement dialect adapters, shared transaction/migration boundary, cancellation/shutdown, safe operational failures under C-12; no universal ORM without benefit.
- [ ] T021 [AP-05] Provide controlled clock/store/provider test boundaries; distinguish real database evidence. Verify foundation against approved C-12 fixtures on both backends.

**Acceptance/evidence:** Compilable startup, backend-parallel tests, single-process proof; no claim of Full HA without operational evidence. **Non-goals:** Inventing domain modules or custom external database HA. **Commands:** Only available after module/toolchain setup; later `go test ./...` for Go integration, not claimed executable now.

