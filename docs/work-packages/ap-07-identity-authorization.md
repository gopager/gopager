# AP-07 Local Identity and Central AuthZ

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [access rights](../requirements-catalog.md#f-access-rights-and-guests), [accounts](../requirements-catalog.md#g-accounts-and-login), [recovery/keys](../requirements-catalog.md#h-recovery-and-access-keys), [security](../requirements-catalog.md#k-installed-web-app-and-security-behavior).
- **Architecture:** [DOM-05 ownership](../architecture.md#domains-and-data-ownership), [dependency rules](../architecture.md#allowed-dependencies), [revocation/deletion coordination](../architecture.md#shared-use-cases).
- **Owner contracts:** [C-06](../contracts/c-06-identity-authorization.md), [C-13](../contracts/c-13-account-lifecycle.md), [C-12](../contracts/c-12-persistence-operations.md); [shared conventions](../contracts/shared-conventions.md).
- **Cases and prerequisite outputs:** [F-17/F-22/F-24](../contracts/reference-cases.md), [F-30/F-31/F-39](../contracts/ap-02-boundary-cases.md), [AP-05 foundation](ap-05-runtime-persistence.md), [AP-04 freeze](ap-04-scope-freeze.md).

**Owner:** DOM-05. **Contract gate:** C-06/C-13, relevant C-12 secret/rescue boundaries. **REQ:** REQ-051..052, REQ-054..064, REQ-069..079, REQ-105, REQ-107..110. **Prerequisites:** AP-05. **Exclusive target scopes:** `internal/identity/`, `internal/authz/`, own tests/schema objects; CLI/root wiring by integration only.

- [ ] T025 [AP-07] Implement Principal/credentials, local login, profile, sessions, recovery/rescue, and restricting personal/technical keys with explicit target permissions.
- [ ] T026 [AP-07] Implement additive direct/User group grants, guest, field/age limits, safe scope-object deletion, shared query/projection decisions.
- [ ] T027 [AP-07] Integrate account deletion/audit under the frozen field matrix and safe invalidation; test F-17/F-22/F-24, recovery replay, target rights, throttling, and redacted failures.

**Acceptance:** No privilege expansion through keys, guest, recovery, or SQL adapters; revoked access produces no further private output. C-13's pseudonymization matrix resolves the documentary P-14 question; implementation still requires its AP-04 freeze and deletion/restart fixtures. **Non-goals:** OIDC/SCIM or nested User groups.
