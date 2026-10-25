# AP-11 SDK, Plugin Host, and Four Targets

[Package index](../work-packages.md) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md) | [Requirement mapping](requirement-mapping.md)

## Session Context

- **Start and authority:** [session procedure](shared-rules.md#session-start), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements:** [plugins/targets](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [security](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [Unit target settings](../requirements-catalog.md#m-additional-domain-capabilities).
- **Architecture:** [TECH-08](../architecture.md#technology-decisions), [plugin-host responsibilities](../architecture.md#supporting-responsibilities), [operating profiles](../architecture.md#operating-profiles).
- **Owner contracts:** [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md), [C-06 SDK authorization](../contracts/c-06-identity-authorization.md); [shared limits](../contracts/shared-conventions.md).
- **Cases and prerequisite outputs:** [F-18/F-20](../contracts/reference-cases.md), [F-34/F-36](../contracts/ap-02-boundary-cases.md); [AP-03 runtime/ABI evidence](ap-03-technical-validation.md), [AP-10 invocation boundary](ap-10-events-delivery.md), [AP-04 freeze](ap-04-scope-freeze.md). Record real provider access/results in the [execution record](../execution/README.md).

**Owner:** Plugin host; separate owner per target; integration owns shared SDK. **Contract gate:** Validated C-09/C-10. **REQ:** REQ-084, REQ-089..090, REQ-107, REQ-129. **Prerequisites:** AP-10 and AP-03 evidence. **Exclusive target scopes:** `internal/pluginhost/`, `sdk/`, separate artifact sources `plugins/pushover/`, `plugins/telegram/`, `plugins/email/`, `plugins/webhook/`. Paths do not predetermine programming language/build tools.

- [ ] T037 [AP-11] Implement/document manifest/compatibility/lifecycle, Wazero limits, permitted host calls, secret/I/O boundaries under approved SDK.
- [ ] T038 [AP-11] Implement Pushover as a separate target module with configuration/classified outcomes; test controlled provider fixtures.
- [ ] T039 [AP-11] Implement Telegram as a separate module with separate instances/secrets and bounded I/O; test controlled provider fixtures.
- [ ] T040 [AP-11] Implement email as a separate module through the selected controlled transport; no HTTP-only assumption without an approved mail-provider path.
- [ ] T041 [AP-11] Implement webhook as a separate module under approved target/DNS/redirect/TLS/secret policy; test SSRF/failures.
- [ ] T042 [AP-11] Test F-18/F-20 and disable/update during invocation; verify SDK examples against runtime/ABI. Demonstrate actual external Delivery separately from fixtures.

**Acceptance:** Four separate modules, operator lifecycle, clear compatible API, demonstrated limits. **Non-goals:** Marketplace, hostile-code isolation guarantees, legacy adapter, implicit host privileges. D-24/C-10 allow explicitly Operator-approved trusted network/filesystem/command access; they do not grant implicit Core data rights. T038..T041 may run in parallel only after SDK freeze and disjoint owner assignment.
