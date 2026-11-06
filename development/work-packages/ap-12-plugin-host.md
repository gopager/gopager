# AP-12 Plugin Host and Initial Targets

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** server side of the following (screens: AP-15):
  - [REQ-084/089..090/106..107](../requirements-catalog.md#i-domain-events-extensions-and-deliveries) plugin host, instances, initial targets.
  - REQ-129: managing per-Unit values through the integration's configuration API; the table and its use in materialization are AP-11.
  - REQ-108: provider-specific rate limits (generic sending admission: AP-11).
  - [REQ-114/115/117](../requirements-catalog.md#l-operations-data-volume-and-evolution), plugin part only: honest provider outcomes, first-party plugins shipped with the distribution, plugin placement on enabled instances in Full; the rest is AP-16.
  - [REQ-139](../requirements-catalog.md#i-domain-events-extensions-and-deliveries) only as compatibility context (Later per [delivery scope](../authoritative-plan.md#delivery-scope)).
- **Inputs:**
  - [AP-08 E-15](../execution/ap-08.md#e-15-managed-plugin-process-runtime): the managed plugin process runtime of [D-131](../decisions/04-processing-and-plugins.md#d-131), which replaced the WASI installed-processing ABI. Planning of manifest, lifecycle, target and configuration belongs here; the runtime itself is fixed by D-131.
  - [identity/AuthZ handover](../execution/source-ap-07.md#handover): identity-core and authorization.
  - [AP-10](ap-10-query-live.md): authorized Core reads. Verify `development/execution/ap-10.md#handover` exists at start.
  - [AP-11](ap-11-events-delivery.md): descriptor, input, attempt and renderer handover. Verify `development/execution/ap-11.md#handover` exists at start.
- **Owned contract slices:** [C-10](../contracts/c-10-plugin-host.md) host, manifest, lifecycle, instance and configuration planning; [C-09](../contracts/c-09-delivery.md) target and configuration-authority slices; supporting [C-06](../contracts/c-06-identity-authorization.md) instance-authority, C-08 and C-12 planning slices; F-18/F-20/F-34/F-36/F-45 and the POST JSON/Bearer/HMAC cases. The plugin host owns the host; each target owns its schemas; the shared SDK is owned by this AP.
- **Reference (read when needed):** [technology decisions](../architecture.md#technology-decisions), [allowed dependencies](../architecture.md#allowed-dependencies), [D-130](../decisions/04-processing-and-plugins.md#d-130) (plugins are fully trusted), [D-131](../decisions/04-processing-and-plugins.md#d-131), [D-135](../decisions/10-foundation-redesign.md#d-135), [shared-rules package layout](shared-rules.md#package-layout), [docs/plugins.md](../../docs/plugins.md).
- **Write scope:**
  - `internal/processing/plugin_runtime.go` or an extracted `internal/pluginhost/` (when Delivery/reaction capabilities need it) — plugin process runtime
  - `pluginsdk/`, `api/plugin/` — public Go SDK and protocol
  - `plugins/pushover/`, `plugins/webhook/` (new) — initial target plugin sources and packaging; `go.mod`/`go.sum` for libraries selected under [D-36](../decisions/01-platform-and-storage.md#d-36)
  - `internal/authz/`, `internal/identity/` — instance authority (supporting)
  - `internal/delivery/` — target adapters
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/processingstore/` (instances), a new configuration/Secret store under `internal/persistence/ent/`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout))
  - `internal/httpapi/` and the versioned configuration OpenAPI
  - `cmd/gopager/` — managed invocation, lifecycle and root registration only
- **Compatibility context:** [AP-13](ap-13-processing-presentation-integration.md), [AP-14](ap-14-identity-lifecycle.md), [AP-15](ap-15-frontend.md), [AP-16](ap-16-operations.md) and [AP-17](ap-17-integration-evidence.md) consume the manifest/SDK/input/attempt versions, configuration/Secret owner interfaces, installed-host calls, management APIs and provider evidence. Their code is not required.

## Open User Decisions

- Destination, risk and operator-provided credentials for the actual external Delivery checks of both initial targets (an approved disposable destination/environment): answer is needed before T071 and recorded in the [execution record](../execution/README.md#required-record-structure); never disclose credentials in evidence.

## Planning Before Coding

Record the host/target/configuration decisions in C-09/C-10 and supporting boundaries before coding; cases belong to their canonical owners.

**T069.** Map the full manifest to AP-11's completed Core descriptor/attempt contracts and validate type/field/configScope/Secret declarations. The runtime is fixed by D-131: managed executables over `go-plugin`/gRPC, one process per configured instance. Add the Delivery-target and event-reaction capabilities as protocol methods or capability kinds and keep the pure Processing-step semantics. Add a process pool across runs, health/status in the operational view, restart/backoff for crashed plugins and ordered shutdown with the Core. Placement of device-bound plugins in multi-instance Full deployments (REQ-117: only instances where a plugin instance is enabled claim its work) belongs here too. Plugins read Core data through the versioned HTTP API under their technical Principal (REQ-139); no second host-callback API. Define inspection/activation/update/disable/version pins, limits/cancellation/capabilities/declared Core reads, config storage/encryption or external Secret references, rotation/version resolution and explicit failures. Freeze only the selected mechanism, not every draft ABI option. Per-Unit Delivery overrides (REQ-129): add management of AP-11's `integration_unit_parameters` values through the integration's configuration API; the Extension-field framework (`DeclareExtensionFields`, `extension.<plugin-id>.<field>`) is dropped ([D-135](../decisions/10-foundation-redesign.md#d-135)). Own server-enforced all/delegated-instance configuration authority in C-06 and admin/user recursive separation in C-09/C-10. Define configuration/inspection/status HTTP plus Operator-only install commands; ordinary views never return stored Secrets. Personal values cannot create an integration or overwrite admin/other-User configuration. Implement actual persistence and host/Delivery root wiring; a missing runtime/instance must fail explicitly, never fall back to test fixtures.

**T070.** Settle the exact Pushover application-token/personal-key and Webhook schemas (REQ-090 starts with POST and JSON, an optional Bearer header and an optional HMAC-SHA256 signature; other methods, encodings and auth modes are later plugin-side additions), input mapping/validation, request/receipt classification, SSRF/DNS/redirect/deadline/size/Secret rules and provider-specific rate limits. Use the completed Core materializer/renderer; no later presentation dependency.

**T071.** Settle the controlled transports, lifecycle/resource/schema/redaction tests and real PostgreSQL configuration/HTTP/attempt integration. Resolve missing destination/risk/credential clarification (see Open User Decisions) before the actual external checks. Identify every personal copy and how its own configured instance can remove it; cleanup orchestration belongs to AP-14, with no dependency on AP-14 implementation.

## Tasks

- [ ] T069 [AP-12] Implement the approved SDK/host lifecycle, compatibility, bounded invocation, capability, Secret, and I/O boundaries.
  - **Done when:** several configured instances of one plugin (REQ-084) run independently; required first-party plugins ship with the distribution (REQ-115); the host, configuration persistence, management HTTP and root wiring work, ordinary views never return stored Secrets, and a missing runtime/instance fails explicitly.
- [ ] T070 [AP-12] Implement the separately packaged Pushover and Webhook targets under their frozen contracts.
  - **Done when:** Pushover and Webhook are separately packaged plugins in `plugins/`; their schemas, input mapping, request/receipt classification, SSRF/DNS/redirect/deadline/size/Secret rules and provider rate limits are recorded in [C-09 Initial Target Capabilities](../contracts/c-09-delivery.md#initial-target-capabilities) and enforced; every provider outcome is classified as delivered, rejected or uncertain, never assumed.
- [ ] T071 [AP-12] Verify controlled and actual external provider behavior separately, lifecycle during invocation, redaction, limits, and SDK/runtime compatibility.
  - **Done when:** F-18/F-20/F-34/F-36/F-45 and the POST JSON/Bearer/HMAC cases pass against controlled transports; lifecycle/resource/schema/redaction tests and real PostgreSQL configuration/HTTP/attempt integration pass; actual external Delivery for both initial targets is demonstrated through the approved disposable destination with operator-provided credentials; observed receipt/uncertainty is recorded separately from fixtures.

## Documentation to Update

- Development owners: [C-10](../contracts/c-10-plugin-host.md), [C-09](../contracts/c-09-delivery.md) (Initial Target Capabilities, Configuration Authority), [C-06](../contracts/c-06-identity-authorization.md), the F-cases above in the [reference cases](../contracts/reference-cases.md) or [boundary cases](../contracts/ap-02-boundary-cases.md), and [glossary](../glossary.md) terms for plugin host/instance.
- [api/plugin/v2/plugin.proto](../../api/plugin/v2/plugin.proto), `pluginsdk/` and [docs/plugins.md](../../docs/plugins.md) (Runtime model, Wire protocol, Manifest, Configuration policy, Writing a plugin in Go).
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md) (Processing: plugin instances; new configuration/status section).
- [docs/operations.md](../../docs/operations.md) (Plugin Directory, Graceful Shutdown, Configuration Reference) and [docs/architecture.md](../../docs/architecture.md) (Package map, Domain events and outbox) for the pool, health/status and packages.
- Go doc comments for exported host, SDK and target code.

## Verification

Focused host/SDK/target/delivery race tests, the existing PostgreSQL fixture, fixture plugins built from source at test time (`internal/testutil/pluginfixture`) and process lifecycle tests. Record exact commands, artifact digests and environment. Actual external Delivery checks run only against the approved disposable destination.

## Handover

To AP-13/AP-14/AP-15/AP-16/AP-17: complete manifest/SDK/input/attempt versions, active configuration/Secret owner interfaces, actual installed-host calls, management APIs and provider evidence.

## Non-goals

Telegram/email, marketplace distribution, or hostile-code isolation guarantees (plugins are fully trusted under D-130).
