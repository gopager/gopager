# AP-12 Plugin Host and Initial Targets

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope) and [approval](../authoritative-plan.md#workflow-and-approval-gates).
- **Requirements and owners:** [REQ-084/089..090/106..107/129](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [C-06](../contracts/c-06-identity-authorization.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Owner:** Plugin host, with separate owners for each target; integration owns the shared SDK.
- **Architecture:** [technology decisions](../architecture.md#technology-decisions), [native-plugin alternative review](../architecture.md#native-plugin-alternative-review), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verify [AP-06](../execution/ap-06.md)'s pure installed-processing ABI, [identity/AuthZ](../execution/source-ap-07.md#handover), [AP-10](ap-10-query-live.md)'s authorized Core reads and [AP-11](ap-11-events-delivery.md)'s descriptor/input/attempt/renderer handover. [AP-03 E-07/B-01](../execution/ap-03.md#e-07-wasm-runtime-probes) is bounded fixture evidence, not a production host. Full manifest/runtime/target/configuration planning belongs here.
- **Write scope:** `internal/pluginhost/`, `sdk/`, initial Pushover/Webhook artifact sources; supporting `internal/authz/`/`internal/identity/` instance authority, `internal/delivery/` target adapters, owner Ent configuration/Secret/instance adapters/schemas/generated output/next migrations; `cmd/gopager/`, `internal/httpapi/` and versioned configuration OpenAPI, managed invocation/lifecycle/root registration, packaging and necessary dependency manifests under D-36. C-09/C-10 and supporting C-06/C-08/C-12 planning slices are authorized.

## Planning Before Coding

Record the following host/target/configuration decisions in C-09/C-10 and supporting boundaries before coding; cases belong to their canonical owners.

- **Later-consumer planning context (not prerequisites):** [AP-13](ap-13-processing-presentation-integration.md), [AP-14](ap-14-identity-lifecycle.md), [AP-15](ap-15-frontend.md), [AP-16](ap-16-operations.md), [AP-17](ap-17-integration-evidence.md).

- [ ] T069 [AP-12] Implement the approved SDK/host lifecycle, compatibility, bounded invocation, capability, Secret, and I/O boundaries.
- [ ] T070 [AP-12] Implement the separately packaged Pushover and Webhook targets under their frozen contracts.
- [ ] T071 [AP-12] Verify controlled and actual external provider behavior separately, lifecycle during invocation, redaction, limits, and SDK/runtime compatibility.

## Exact Configuration, Integration and Evidence

T069 maps the full manifest to AP-11's completed Core descriptor/attempt contracts, validates type/field/configScope/Secret declarations, chooses/pins the actual production ABI/runtime/toolchain, and preserves the pure installed-processing boundary. Define inspection/activation/update/disable/version pins, limits/cancellation/capabilities/declared Core reads, config storage/encryption or external Secret references, rotation/version resolution and explicit failures. Freeze only the selected mechanism, not every draft ABI option.

Own server-enforced all/delegated-instance configuration authority in C-06 and admin/user recursive separation in C-09/C-10. Define configuration/inspection/status HTTP plus Operator-only install commands; ordinary views never return stored Secrets. Personal values cannot create an integration or overwrite admin/other-User configuration. Implement actual persistence and host/Delivery root wiring; missing runtime/instance must fail explicitly, never fall back to test fixtures.

T070 owns exact Pushover application-token/personal-key and Webhook method/encoding/auth/signing schemas, input mapping/validation, request/receipt classification, SSRF/DNS/redirect/deadline/size/Secret rules and provider-specific rate limits. Use the completed Core materializer/renderer; no later presentation dependency.

T071 executes F-18/F-20/F-34/F-36/F-45 family and method/signature matrix against controlled transports, lifecycle/resource/schema/redaction tests and real PostgreSQL configuration/HTTP/attempt integration. Demonstrate actual external Delivery for both initial targets through an approved disposable destination/environment with operator-provided credentials. Resolve missing destination/risk/credential clarification before those checks; never disclose credentials in evidence. Record observed receipt/uncertainty separately from fixtures.

Use focused host/SDK/target/delivery race tests, existing PostgreSQL fixture, actual artifact build/ABI examples and command-process lifecycle tests; record exact commands, artifact digests and environment. Hand over complete manifest/SDK/input/attempt versions, active configuration/Secret owner interfaces, actual installed-host calls, management APIs and provider evidence to AP-13/AP-14/AP-15/AP-16/AP-17. Cleanup orchestration belongs to AP-14, but this AP identifies every personal copy and how its own configured instance can remove it; no dependency on AP-14 implementation.

**Acceptance:** Initial targets are separate, bounded, and explicit about provider outcomes and operator-approved capabilities.

**Non-goals:** Telegram/email, marketplace distribution, hostile-code isolation guarantees, or implicit host privileges.
