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
  - [AP-06 handover](../execution/ap-06.md#handover) and [AP-06 E-21](../execution/ap-06.md#e-21-plugin-process-pool-secureconfig-artifact-cache-preview-mode): the plugin process pool, lazy restart and shutdown, `SecureConfig`, artifact cache.
  - [identity/AuthZ handover](../execution/source-ap-07.md#handover): identity-core, authorization, `plugin.configure` and `subscription.manage-own`.
  - [AP-11 handover](ap-11-events-delivery.md#handover): target descriptor, input, attempt invocation/result port, renderer, `integration_unit_parameters` and the claim filter by instance. An absent handover is an input gap reported at start.
- **Owned contract slices:** [C-10](../contracts/c-10-plugin-host.md) host, manifest, lifecycle, instance, configuration and Secrets; [C-09](../contracts/c-09-delivery.md) Configuration Authority and Initial Target Capabilities; F-20, F-36, F-45 and the plugin-process variants of F-18/F-34 through the Webhook plugin, plus the POST JSON/Bearer/HMAC cases ([D-135](../decisions/10-foundation-redesign.md#d-135) item 15). The plugin host owns the host; each target owns its schemas; the shared SDK, `pluginsdk/plugintest` and `pluginsdk/safehttp` are owned by this AP. Decisions: [D-142](../decisions/09-process-and-documentation.md#d-142).
- **Reference (read when needed):** [technology decisions](../architecture.md#technology-decisions), [allowed dependencies](../architecture.md#allowed-dependencies), [D-130](../decisions/04-processing-and-plugins.md#d-130) (plugins are fully trusted), [D-131](../decisions/04-processing-and-plugins.md#d-131), [D-135](../decisions/10-foundation-redesign.md#d-135), [shared-rules package layout](shared-rules.md#package-layout), [docs/plugins.md](../../docs/plugins.md).
- **Write scope:**
  - `internal/processing/` (`plugin_runtime.go`, `artifacts.go`, `installed.go`, `instance*.go`) and `internal/processing/management/` — plugin host, instance configuration, Secret resolution
  - `internal/testutil/pluginfixture/` — fixture plugins
  - `pluginsdk/` including `pluginsdk/plugintest/` (server-side test harness) and `pluginsdk/safehttp/` (SSRF-safe HTTP transport); `api/plugin/` — public Go SDK and protocol
  - `plugins/pushover/`, `plugins/webhook/` (new, each its own Go module with `go.mod` using `replace` to the root module) — first-party target plugins; `go.mod`/`go.sum` for libraries selected under [D-36](../decisions/01-platform-and-storage.md#d-36)
  - `deploy/compose/Dockerfile` (plugin build stage and plugin directory only), `deploy/compose/compose.yaml` (artifact root only), `Makefile` (plugin build, test and format targets)
  - `internal/persistence/ent/schema/`, `internal/persistence/ent/processingstore/` (instances), a new configuration/Secret store under `internal/persistence/ent/`, generated Ent code and `internal/persistence/migrations/` (per [package layout](shared-rules.md#package-layout))
  - `internal/httpapi/` and the versioned configuration OpenAPI
  - `cmd/gopager/` — managed invocation, lifecycle and root registration only
  - `docs/` — plugin, operations, HTTP API and development documentation (`docs/development.md`)
- **Compatibility context:** [AP-13](ap-13-processing-presentation-integration.md), [AP-14](ap-14-identity-lifecycle.md), [AP-15](ap-15-frontend.md), [AP-16](ap-16-operations.md) and [AP-17](ap-17-integration-evidence.md) consume the manifest/SDK/input/attempt versions, configuration/Secret owner interfaces, installed-host calls, management APIs and provider evidence. Their code is not required.

## Open User Decisions

None. No real external Delivery check runs in any AP ([D-142](../decisions/09-process-and-documentation.md#d-142)); both first-party plugins are verified against local fakes.

## Planning Before Coding

Record the host/target/configuration decisions in C-09/C-10 before coding; cases belong to their canonical owners.

**T069.** Map the full manifest to AP-11's completed Core descriptor/attempt contracts and validate type/field/configScope/Secret declarations. The runtime is fixed by D-131: managed executables over `go-plugin`/gRPC, one process per configured instance. Add only the Delivery-target capability kind; event-reaction and Core-read capabilities are not implemented here (REQ-139 is later), and a manifest declaring `requiredOperations` or `requiredCoreFields` for a Delivery capability is rejected with `CAPABILITY_UNDECLARED`. Keep the pure Processing-step semantics. Reuse the existing pool, lazy restart and shutdown ([Plugins §1](../../docs/plugins.md#1-runtime-model), AP-06 E-21) without a new backoff: AP-11's attempt policy bounds retries. Expose plugin status (last start, last failure code, running) in the instance read view (`GET /instances/{id}`). Placement (REQ-117): a node claims Delivery work of an instance only if the pinned artifact (path and digest) is loadable from its plugin directory, using AP-11's claim filter; otherwise it skips the work; with no such node the attempt stays pending with the visible status `dependency-unavailable`. Define inspection/activation/update/disable/version pins, limits/cancellation and explicit failures. Freeze only the selected mechanism. Per-Unit Delivery overrides (REQ-129): add management of AP-11's `integration_unit_parameters` values through the integration's configuration API; there are no Extension fields ([D-135](../decisions/10-foundation-redesign.md#d-135) item 15). Configuration authority is the global `plugin.configure` action ([D-100](../decisions/04-processing-and-plugins.md#d-100), no instance scope), recorded in C-06/C-09, with admin/user recursive separation in C-09/C-10. Personal recipient values are written by the owning User under `subscription.manage-own`, bound to (instance, owner), and cannot create an integration or overwrite admin or other-User configuration. Target use ([D-148](../decisions/05-events-and-delivery.md#d-148)): the administrative configuration of each Delivery instance declares who may use it as a Subscription target; AP-11's Subscription save, fan-out and start check enforce it, so revoking it stops future sends. Define configuration/inspection/status HTTP; ordinary views never return stored Secrets. Operators install plugins by placing artifacts in the plugin directory; there are no install commands.

**Secrets ([D-142](../decisions/09-process-and-documentation.md#d-142)).** An in-app encrypted store is mandatory: authenticated encryption (AES-256-GCM), versioned key ID, master key from `GOPAGER_SECRET_KEY_FILE` or `GOPAGER_SECRET_KEY`, never logged; without a key, Secret-bearing instances fail visibly with `AUTHORITY_UNAVAILABLE`. A Secret field may instead hold an external reference `file:<path>` (mounted Kubernetes or Docker Secret, files written by External Secrets Operator or Vault Agent) or `env:<NAME>`. File references are resolved per invocation; a changed content digest is a new Secret version and a running invocation stays pinned. There are no provider API adapters. Record in the C-10 Secrets section and document in Operations (Configuration Reference) and Plugins.

**Delivery of runtime pieces.** Implement the Delivery capability, host and Delivery root wiring and persistence; a missing runtime or instance fails explicitly, never falls back to fixtures. First-party plugins are separate Go modules under `plugins/<name>/` tested on their own against local fakes. `pluginsdk/plugintest` simulates how the Core calls a plugin (manifest validation, configuration and Secret delivery, Delivery invocation, result mapping). The compose image contains both plugin executables and manifests under the plugin directory and the default artifact root points to it. `make` runs the plugin modules' tests.

**T070.** Settle the exact Pushover application-token/personal-key and Webhook schemas (Webhook scope per [D-135](../decisions/10-foundation-redesign.md#d-135) item 15: POST JSON, optional Bearer, optional HMAC-SHA256), input mapping/validation, request/receipt classification, deadline/size/Secret rules and provider-specific rate limits. HMAC: header `X-GoPager-Signature: sha256=<hex>` over timestamp plus raw body, header `X-GoPager-Timestamp`, receiver-side replay tolerance documented. SSRF, DNS and redirect protection lives in `pluginsdk/safehttp`, used by both plugins; relaxation is the per-instance admin field `allowPrivateDestinations` (default false). Use the completed Core materializer/renderer; no later presentation dependency.

**T071.** Settle the plugin-module tests against local fakes (httptest Pushover and webhook receivers), the `plugintest` harness, lifecycle/resource/schema/redaction tests, the two-process PostgreSQL placement test and real PostgreSQL configuration/HTTP/attempt integration. First-party plugins hold no personal copy outside the Core store; personal values are deleted by AP-14 T076; there is no plugin removal hook.

## Tasks

- [ ] T069 [AP-12] Implement the approved SDK/host lifecycle, compatibility, bounded invocation, capability, Secret, and I/O boundaries.
  - **Done when:** several configured instances of one plugin (REQ-084) run independently; required first-party plugins ship with the distribution (REQ-115): the compose image contains both plugin executables and manifests under the plugin directory and the default artifact root points to it; the host, configuration persistence, encrypted Secret store and external references, management HTTP, plugin status in `GET /instances/{id}` and root wiring work, ordinary views never return stored Secrets, a Delivery manifest declaring Core reads is rejected with `CAPABILITY_UNDECLARED`, and a missing runtime/instance fails explicitly; `pluginsdk/plugintest` exists and `make` runs the plugin modules' tests.
- [ ] T070 [AP-12] Implement the separately packaged Pushover and Webhook targets under their frozen contracts.
  - **Done when:** Pushover and Webhook are separately packaged plugins in `plugins/`; their schemas, input mapping, request/receipt classification, SSRF/DNS/redirect/deadline/size/Secret rules and provider rate limits are recorded in [C-09 Initial Target Capabilities](../contracts/c-09-delivery.md#initial-target-capabilities) and enforced; every provider outcome is classified as delivered, rejected or uncertain, never assumed.
- [ ] T071 [AP-12] Verify plugin behavior against local fakes, lifecycle during invocation, redaction, limits, placement and SDK/runtime compatibility.
  - **Done when:** F-20, F-36, F-45, the plugin-process variants of F-18/F-34 via the Webhook plugin and the POST JSON/Bearer/HMAC cases pass in the plugin modules and through `plugintest`; lifecycle/resource/schema/redaction tests, the two-process PostgreSQL placement test and real PostgreSQL configuration/HTTP/attempt integration pass; no personal copy outside the Core store exists (T076 of AP-14 deletes personal values).

## Documentation to Update

- Development owners: [C-10](../contracts/c-10-plugin-host.md), [C-09](../contracts/c-09-delivery.md) (Initial Target Capabilities, Configuration Authority), the F-cases above in the [reference cases](../contracts/reference-cases.md) or [boundary cases](../contracts/ap-02-boundary-cases.md), and [glossary](../glossary.md) terms for plugin host/instance.
- [api/plugin/v2/plugin.proto](../../api/plugin/v2/plugin.proto), `pluginsdk/` and [docs/plugins.md](../../docs/plugins.md) (Runtime model, Wire protocol, Manifest, Configuration policy, Writing a plugin in Go).
- [api/openapi/v1.yaml](../../api/openapi/v1.yaml) and [docs/http-api.md](../../docs/http-api.md) (Processing: plugin instances; new configuration/status section).
- [docs/operations.md](../../docs/operations.md) (Plugin Directory, Graceful Shutdown, Configuration Reference including the Secret key variables) and [docs/architecture.md](../../docs/architecture.md) (Package map, Domain events and outbox) for the pool, status and packages; [docs/development.md](../../docs/development.md) for the plugin modules, `plugintest` and the `make` targets.
- Go doc comments for exported host, SDK and target code.

## Verification

`make` (including the plugin modules' own tests), focused host/SDK/target/delivery race tests, the existing PostgreSQL fixture with the two-process placement test, fixture plugins built from source at test time (`internal/testutil/pluginfixture`), `plugintest` harness runs and process lifecycle tests. Record exact commands, artifact digests and environment. No real external Delivery check.

## Handover

To AP-13/AP-14/AP-15/AP-16/AP-17: complete manifest/SDK/input/attempt versions, active configuration/Secret owner interfaces, actual installed-host calls, management APIs and plugin-module test evidence. To AP-14: personal plugin configuration, Secrets and Delivery targets live only in the Core store, with the owner key for AP-14's deletion transaction.

## Non-goals

Telegram/email, marketplace distribution, hostile-code isolation guarantees (plugins are fully trusted under D-130), event-reaction and Core-read plugin capabilities (REQ-139), provider API Secret adapters and any real external Delivery check.
