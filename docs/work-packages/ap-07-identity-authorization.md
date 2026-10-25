# AP-07 Local Identity and Central AuthZ

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

Related package references (not additional prerequisites): [AP-05](ap-05-runtime-persistence.md).

**Owner:** DOM-05. **Contract gate:** C-06/C-13, relevant C-12 secret/rescue boundaries. **REQ:** REQ-051..052, REQ-054..064, REQ-069..079, REQ-105, REQ-107..110. **Prerequisites:** AP-05. **Exclusive target scopes:** `internal/identity/`, `internal/authz/`, own tests/schema objects; CLI/root wiring by integration only.

- [ ] T025 [AP-07] Implement Principal/credentials, local login, profile, sessions, recovery/rescue, and restricting personal/technical keys with explicit target permissions.
- [ ] T026 [AP-07] Implement additive direct/User group grants, guest, field/age limits, safe scope-object deletion, shared query/projection decisions.
- [ ] T027 [AP-07] Integrate account deletion/audit under the frozen field matrix and safe invalidation; test F-17/F-22/F-24, recovery replay, target rights, throttling, and redacted failures.

**Acceptance:** No privilege expansion through keys, guest, recovery, or SQL adapters; revoked access produces no further private output. C-13's pseudonymization matrix resolves the documentary P-14 question; implementation still requires its AP-04 freeze and deletion/restart fixtures. **Non-goals:** OIDC/SCIM or nested User groups.
