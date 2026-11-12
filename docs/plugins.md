# Plugins

A GoPager plugin is an executable that implements one capability, declared in its manifest:

- **`processing`** (the default): a pure computation. A workflow step of kind `installed` passes typed input fields to the plugin and receives typed result fields back.
- **`delivery`**: a Delivery target. GoPager sends the rendered notification of a Delivery to an external provider through the plugin and records the outcome the plugin reports. Two first-party Delivery plugins ship with GoPager: [Pushover and Webhook](#9-first-party-delivery-plugins).

Plugins are written in any language that can serve gRPC; Go plugins use the [`pluginsdk`](../pluginsdk/) package.

Related documents: [Workflows](workflows.md) (step definitions, instances, preview, retry) · [Operations](operations.md) (deployment, artifact directory, Secrets) · [HTTP API](http-api.md#processing-plugin-instances) (instance, personal configuration and inspection routes).

## Contents

1. [Runtime model](#1-runtime-model)
2. [Wire protocol](#2-wire-protocol)
3. [Invocation request](#3-invocation-request)
4. [Response](#4-response)
5. [Limits](#5-limits)
6. [Packaging and installation](#6-packaging-and-installation)
7. [Writing a plugin in Go](#7-writing-a-plugin-in-go)
8. [Writing a plugin in another language](#8-writing-a-plugin-in-another-language)
9. [First-party Delivery plugins](#9-first-party-delivery-plugins)

## 1. Runtime model

| Aspect | Behavior |
| --- | --- |
| Process model | Child process of the GoPager server, started through [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin), gRPC protocol only. No in-process loading. |
| Where it runs | Every server process with the `api` role (workflow preview) or `worker` role (message processing, Delivery) can start plugins. Each such process needs the artifact directory. |
| Process ownership | One long-lived process per **instance configuration revision** (a process pool in the server). Two instances of the same plugin never share a process. All live runs that use the same instance revision, including concurrent runs, share its process; preview invocations of that revision use a second pooled process, so an editor's preview never occupies the call slot a live run waits for. A process keeps its state between runs. A Delivery plugin has no preview process: its attempts all use the live process of the instance revision, shared by every subscriber of the instance. |
| Calls per process | The server sends a process one `Invoke` at a time: concurrent runs and Delivery attempts queue on the process (the queue wait counts against the 30 s call deadline). A plugin handler therefore needs no locking against concurrent calls. A step that depends on message targets is invoked once per selected target. A Delivery attempt is one `Invoke`. |
| Start | Lazy: on the first `Invoke` after the instance revision is first used, or after the process was stopped. Loading, validating and inspecting an instance never starts the executable. |
| Verified start | The server starts the executable through go-plugin with a `SecureConfig` checksum (the pinned SHA-256 digest). go-plugin hashes the file immediately before it starts it and refuses to start it on a mismatch (`AUTHORITY_UNAVAILABLE`, nothing is executed). The check and the `exec` are two steps; an attacker who can write to the artifact directory can still race them, so keep it writable only by the operator. |
| Handshake timeout | 10 s. A process that does not complete the handshake is killed. |
| Idle timeout | 5 minutes without any call stops the process; the next call starts a new one. |
| Working directory | The directory containing the executable. |
| Environment | Only `PATH`, `HOME`, `LANG`, `LC_ALL`, `TZ`, `TMPDIR` (when set in the server) plus the go-plugin variables (see [§2](#2-wire-protocol)). Server configuration such as `GOPAGER_DATABASE_URL` is never passed. |
| stdout / stderr | Discarded after the handshake line; not forwarded to server logs. |
| Per-call deadline | 30 s. The deadline is also sent in the envelope and as the gRPC deadline. |
| Kill | On deadline expiry, cancellation, a gRPC error returned by `Invoke` (including a handler error) or a broken connection, the process is killed. The next call starts a new process. |
| Stop | A process stops after the idle timeout, when a newer revision of its instance is committed (the older revision's process stops; running calls of that revision fail), and at server shutdown. Stopping closes the connection (go-plugin `Shutdown` controller call), waits up to 2 s for the process to exit, then sends `SIGKILL`. The end of a workflow run does not stop it. |
| Artifact cache | The server remembers that an executable matched its pinned digest, keyed by artifact directory, path and digest, and re-hashes it only when its size, modification time or mode changes or the instance changes. The manifest is re-read on every load. |
| Trust | Plugins are trusted, operator-installed code. There is no sandbox, no memory limit and no seccomp/namespace isolation; use OS or container limits. |
| Status | The server remembers per instance, in memory, whether a process is running, when one last started and the last failure code (`start-failed`, `checksum-mismatch`, `invocation-failed`, `deadline-exceeded`). The instance view shows it ([HTTP API](http-api.md#processing-plugin-instances)). The status is local to the answering node and resets when that process restarts, so it differs between nodes. |

### Capabilities

| Capability | Declared by | Used by | Rules |
| --- | --- | --- | --- |
| `processing` | `capability` absent or `"processing"` | Workflow steps of kind `installed` | Pure: no side effects, no host capabilities, no Secrets. The server cannot enforce purity; it is the author's declaration. |
| `delivery` | `"capability": "delivery"` and a `delivery` object | Delivery targets of Subscriptions | Effects on an external provider are the purpose. Not pure, no preview, no Workflow input or result schema. May hold Secrets and personal (per-subscriber) configuration. |

Steps of kind `installed` only accept `processing` plugins that declare themselves `pure`. A `delivery` instance cannot be a Workflow step (the step fails with `AUTHORITY_UNAVAILABLE`), and a `processing` instance cannot be a Subscription target. A Delivery plugin makes no reads of GoPager data and has no callback into the server; it receives everything it needs in the request.

### Placement across nodes

Each `api` and `worker` process reads its own plugin directory, so nodes may differ. A worker claims the due Deliveries of an instance at once only when the instance's pinned manifest and executable verify from that node's plugin directory (checked without starting a process; the answer is cached for 5 seconds; without a plugin directory a node serves no instance). A Delivery prepared by a node that cannot describe its target carries the outcome code `dependency-unavailable` but stays due at once, so a capable node claims it without delay. A Delivery that no capable node claimed within 60 seconds of becoming due may be claimed by any node. A node that cannot load the artifact then defers the Delivery without consuming an attempt: the Delivery keeps its state (`prepared` or `retry-pending`), its outcome code is `dependency-unavailable`, and it is due again after 60 seconds. It starts when a node with the artifact claims it, or ends as `expired` when its lifetime runs out. Provision the same plugin directory on every worker unless you deliberately want a subset of nodes to serve an instance.

## 2. Wire protocol

The service is published in [`api/plugin/v2/plugin.proto`](../api/plugin/v2/plugin.proto):

```proto
syntax = "proto3";
package gopager.plugin.v2;
import "google/protobuf/wrappers.proto";

service Plugin {
  rpc Invoke(google.protobuf.BytesValue) returns (google.protobuf.BytesValue);
}
```

| Item | Value |
| --- | --- |
| Magic cookie | Environment variable `GOPAGER_PLUGIN=gopager-plugin-runtime-2`. The plugin must refuse to run without it. |
| go-plugin core protocol version | `1` |
| Application protocol version | `2` (also sent as `PLUGIN_PROTOCOL_VERSIONS=2`) |
| Transport | `grpc` only; no net/rpc. No TLS (`PLUGIN_CLIENT_CERT` is not set). |
| Handshake line (stdout) | `1|2|unix|<socket path>|grpc` or `1|2|tcp|127.0.0.1:<port>|grpc`, terminated by a newline. For TCP, pick a port in `PLUGIN_MIN_PORT`..`PLUGIN_MAX_PORT` (default 10000–25000). |
| gRPC method | `/gopager.plugin.v2.Plugin/Invoke` |
| go-plugin dispense name | `gopager` (Go SDK only; not visible on the wire) |
| Health service | `grpc.health.v1.Health`, service name `plugin`, status `SERVING` |
| Message framing | Request and response are `google.protobuf.BytesValue` (field 1, `bytes`). The bytes are one UTF-8 JSON document. |
| Response size on the wire | The server accepts at most 256 KiB of JSON plus 16 bytes of protobuf framing. |

Environment variables set by go-plugin in the plugin process: `GOPAGER_PLUGIN`, `PLUGIN_MIN_PORT`, `PLUGIN_MAX_PORT`, `PLUGIN_PROTOCOL_VERSIONS`.

## 3. Invocation request

The request JSON is the invocation envelope of a `processing` plugin. All fields are always present. A `delivery` plugin receives the [Delivery request](#delivery-request) instead; it is told apart by its `kind` member, which a processing request does not have.

| Field | JSON type | Meaning |
| --- | --- | --- |
| `schemaVersion` | number | Always `2`. |
| `invocationId` | string (UUID) | New for every call. |
| `instanceId` | string (UUID) | The configured instance. Use it to keep per-instance state apart. |
| `artifactDigest` | string | Lowercase hex SHA-256 of the executable that is being called. |
| `capabilityId` | string | `capabilityId` from the manifest. |
| `workflowId` | string (UUID) | Workflow being evaluated. |
| `definitionVersionId` | string (UUID) | Saved workflow version being evaluated. |
| `runId` | string (UUID) | Workflow run of this call. A process serves many runs, so this changes between calls. |
| `executionPrincipalId` | string (UUID) | Principal the instance executes as. |
| `configurationRevision` | string | Instance configuration revision as a decimal string, e.g. `"3"`. |
| `deadline` | string | RFC 3339 timestamp with fractional seconds and the server's local offset, e.g. `"2026-11-05T14:00:30.123456789+01:00"`. Same instant as the gRPC deadline. |
| `input` | object | Field name → [typed value](#typed-values). Contains only names from `inputSchema`. Optional fields without a value are omitted. |
| `configuration` | typed value | The instance configuration: a typed value of kind `object` matching `configurationSchema`. |
| `mode` | string | `"live"` or `"preview"`. See [Preview mode](#preview-mode). |

### Preview mode

`mode` is `"live"` for message processing and `"preview"` when an editor tests a workflow draft (`POST /api/v1/workflows/{id}/preview`). In preview mode the plugin must compute its result **without any effect outside its process** and may return a representative result (for example a plugin that normally calls a lookup service returns sample data). The server sends `"preview"` only to plugins whose manifest sets `supportsPreview` to `true`; for any other plugin a preview of an `installed` step fails with `AUTHORITY_UNAVAILABLE` and the plugin is not called. Because of that gate the field is additive: the envelope `schemaVersion` stays `2`, and a plugin that ignores `mode` is safe as long as it does not declare `supportsPreview`. The server always sends `mode`; treat a missing value as `"live"`. Preview and live calls share the instance's process.

Example:

```json
{
  "schemaVersion": 2,
  "invocationId": "0b6f0a43-5d0e-4a8e-9a43-2f1f3c1b6a10",
  "instanceId": "5f0c6f7e-3a3b-4d43-8f0e-6f8f3c0a1b2c",
  "artifactDigest": "5e718916f839bcdcd66bd15a9ad4eae5ff7228b029898d602df75901f5b1d847",
  "capabilityId": "upper",
  "workflowId": "8a7d9a52-1d3c-4b0e-9a0f-0d4d6b9e8c11",
  "definitionVersionId": "c3f1b2a4-6e5d-4c3b-8a29-1f0e9d8c7b6a",
  "runId": "e2d4c6b8-a0f1-4e3d-9c5b-7a6f8e0d2c4b",
  "executionPrincipalId": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
  "configurationRevision": "1",
  "deadline": "2026-11-05T14:00:30.5+01:00",
  "mode": "live",
  "input": {
    "text": {"kind": "string", "value": "alarm"}
  },
  "configuration": {
    "kind": "object",
    "value": {"prefix": {"kind": "string", "value": "FW: "}}
  }
}
```

### Typed values

Every value is an object with exactly two members, `kind` and `value`:

| `kind` | `value` encoding | Example |
| --- | --- | --- |
| `string` | JSON string | `{"kind":"string","value":"text"}` |
| `boolean` | `true` / `false` | `{"kind":"boolean","value":true}` |
| `integer` | Decimal **string**, signed 64-bit range | `{"kind":"integer","value":"9223372036854775807"}` |
| `number` | JSON number (not a string), finite | `{"kind":"number","value":1.5}` |
| `timestamp` | RFC 3339 string with offset or `Z`; fractional seconds allowed | `{"kind":"timestamp","value":"2026-11-01T12:00:00+01:00"}` |
| `bytes` | Canonical standard Base64 with padding | `{"kind":"bytes","value":"AAEC/w=="}` |
| `object` | JSON object of name → typed value; only declared properties; every non-optional property present | `{"kind":"object","value":{"ids":{"kind":"list","value":[{"kind":"integer","value":"7"}]}}}` |
| `list` | JSON array of typed values of the declared element schema | `{"kind":"list","value":[{"kind":"string","value":"a"}]}` |
| `null` | `null` — only where the schema has `"nullable": true` | `{"kind":"null","value":null}` |

A value whose `kind` differs from its declaration is rejected, including `{"kind":"string","value":null}`. Absence (omitting the field) and explicit null are distinct; absence requires `"optional": true`.

### Delivery request

For one Delivery attempt the server sends a document with `"kind": "delivery"`. The plugin never sees a message, a Unit or a subscriber beyond what this document contains.

| Field | JSON type | Meaning |
| --- | --- | --- |
| `schemaVersion` | number | Always `2`. |
| `kind` | string | Always `"delivery"`. |
| `invocationId` | string (UUID) | The attempt ID, new for every attempt. |
| `instanceId` | string (UUID) | The configured instance. |
| `instanceRevision` | string | Instance configuration revision as a decimal string. |
| `artifactDigest` | string | Lowercase hex SHA-256 of the executable that is being called. |
| `capabilityId` | string | `capabilityId` from the manifest. |
| `deadline` | string | RFC 3339 instant (UTC) by which the attempt must end. The same bound as the gRPC deadline, at most 30 s after the attempt started. |
| `configuration` | typed value | The merged configuration: a typed value of kind `object` matching `configurationSchema`. It holds the administrative values and the subscriber's personal values; every Secret property holds its **plaintext**, as a `string`. |
| `secretVersions` | object | Secret property path (dotted) → the version that was resolved: `secret:<row id>`, `file:<16 hex digits of the content digest>` or `env:<NAME>`. Never a value. |
| `attempt` | object | The notification, below. |

`attempt`:

| Field | JSON type | Meaning |
| --- | --- | --- |
| `deliveryId` | string (UUID) | The Delivery; identical in every attempt of it, so a provider that accepts an idempotency key can use it. |
| `attemptId` | string (UUID) | This attempt (equals `invocationId`). |
| `attemptNumber` | number | 1-based, at most 5. |
| `subscriberId` | string (UUID) | The account whose Subscription produced the Delivery. Personal configuration values in `configuration` are this account's. |
| `correlationId` | string (UUID) | The correlation ID of the originating event; omitted when it has none. |
| `causationId` | string (UUID) | The ID of the event that caused the Delivery. |
| `outputKind` | string | `text`, `markdown` or `json`; one of the manifest's `delivery.outputKinds`. |
| `body` | string | The rendered notification; for `json` it holds the JSON text. |
| `inputs` | object | Input name → [typed value](#typed-values) for the inputs the manifest's `delivery.inputs` declare, after the Core's precedence rules (instance configuration, Subscription constant, Unit attribute binding, per-Unit value). Inputs without a value are absent. |

Example:

```json
{
  "schemaVersion": 2,
  "kind": "delivery",
  "invocationId": "7c1f2a60-3b1e-4d6c-9a0f-5e2b8c4d1a33",
  "instanceId": "5f0c6f7e-3a3b-4d43-8f0e-6f8f3c0a1b2c",
  "instanceRevision": "2",
  "artifactDigest": "5e718916f839bcdcd66bd15a9ad4eae5ff7228b029898d602df75901f5b1d847",
  "capabilityId": "webhook",
  "deadline": "2026-11-05T13:00:30.5Z",
  "configuration": {"kind": "object", "value": {
    "url": {"kind": "string", "value": "https://hooks.example.com/gopager"},
    "hmacSecret": {"kind": "string", "value": "plaintext of the Secret"}
  }},
  "secretVersions": {"hmacSecret": "secret:0e3b7e0a-4f3a-4a55-9c0e-1f5d3f6c2b10"},
  "attempt": {
    "deliveryId": "b8a1c6d2-0f4e-4a77-8d39-6c1e2f9a7b54",
    "attemptId": "7c1f2a60-3b1e-4d6c-9a0f-5e2b8c4d1a33",
    "attemptNumber": 1,
    "subscriberId": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
    "causationId": "2d9f7e41-8c0a-4b36-a1d5-3e7f9c0b6a12",
    "outputKind": "text",
    "body": "Fire alarm, station 3",
    "inputs": {"topic": {"kind": "string", "value": "alarms"}}
  }
}
```

The request is limited to 1 MiB; a larger one is not sent and the attempt ends as `permanent` `request-too-large`. The server keeps the plaintext Secrets only for the duration of the call and clears them afterwards.

## 4. Response

The response JSON is exactly one of:

```json
{"schemaVersion": 2, "fields": {"upper": {"kind": "string", "value": "FW: ALARM"}}}
```

```json
{"schemaVersion": 2, "failure": "evaluation-failed"}
```

Rules:

- `schemaVersion` must be `2`.
- Exactly one of `fields` (an object, may be `{}`) and a non-empty `failure`. Both, or neither, is invalid.
- No other top-level members. Duplicate keys anywhere are invalid. Nesting depth ≤ 32.
- `fields` may contain only names declared in `resultSchema`; every non-optional result field must be present; each value must match its declaration ([typed values](#typed-values)).
- The document must be valid UTF-8 and at most 256 KiB.
- Failure text is never shown to users; only the classified error code is recorded.

`failure` reasons:

| `failure` | Meaning | Error code |
| --- | --- | --- |
| `conversion-failed` | Input could not be converted | `INVALID_INPUT` |
| `evaluation-failed` | Processing failed for this input | `INVALID_INPUT` |
| `resource-limit` | Plugin-side limit exceeded | `RESOURCE_LIMIT` |
| `capability-unavailable` | A dependency of the plugin is temporarily unavailable | `AUTHORITY_UNAVAILABLE` |
| any other string | — | `INVALID_INPUT` |

### Failure classification and retry

A failed step fails the whole workflow run for that transmission; no partial result is published. The worker stores the error code and decides whether to retry automatically:

| Situation | Error code | Automatic retry |
| --- | --- | --- |
| `failure: "capability-unavailable"` | `AUTHORITY_UNAVAILABLE` | yes |
| Process does not start, wrong cookie/protocol version, no handshake within 10 s, invocation service missing | `AUTHORITY_UNAVAILABLE` | yes |
| Executable or manifest missing, digest mismatch, not executable, instance disabled, a `processing` configuration declares a Secret | `AUTHORITY_UNAVAILABLE` | yes |
| `failure: "conversion-failed"` / `"evaluation-failed"` / unknown reason | `INVALID_INPUT` | no |
| Malformed, mixed or non-UTF-8 response, undeclared or missing result field, value/schema mismatch | `INVALID_INPUT` | no |
| Handler returns a gRPC error, process crashes during the call, connection breaks (process is killed) | `INVALID_INPUT` | no |
| Response larger than 256 KiB | `INVALID_INPUT` (rejected by gRPC) or `RESOURCE_LIMIT` | no |
| `failure: "resource-limit"`, input or aggregate limits exceeded | `RESOURCE_LIMIT` | no |
| Manifest `schemaVersion`/`sdkMajor`/`sdkMinor` unsupported | `INCOMPATIBLE_VERSION` | no |
| 30 s call deadline exceeded (process is killed) | `RESOURCE_LIMIT` | no |
| Built-in compute budget of the run exhausted | `RESOURCE_LIMIT` | no |

Worker shutdown or loss of the candidate lease cancels a running call (the process is killed). This is a cancellation, not a failure: nothing is stored for the attempt, and the candidate is recovered when its lease expires (see [Workflows](workflows.md#retries)).

Automatic retry schedule (per workflow run candidate): up to 5 automatic attempts in total. After attempt 1–4 the next attempt is due after 30 s, 2 min, 10 min, 30 min respectively, each with ±20 % random jitter. After the fifth failed attempt the candidate stays `failed`. Failed work can be retried manually through the API (see [Workflows](workflows.md)).

Return a `failure` response instead of a Go/gRPC error for expected failures: any error returned from `Invoke` kills the process.

### Delivery response

A `delivery` plugin answers with an attempt result instead of `fields` / `failure`:

```json
{"schemaVersion": 2, "attempt": {"status": "accepted", "code": "accepted", "receipt": {"status": 200}}}
```

| Member | Rules |
| --- | --- |
| `schemaVersion` | `2`. |
| `attempt.status` | `accepted` (the provider took the notification), `transient` (nothing was delivered, a later attempt may succeed), `permanent` (nothing was delivered, retrying cannot succeed) or `uncertain` (the provider may or may not have acted; GoPager never resends). |
| `attempt.code` | Machine-readable reason matching `[a-z0-9-]{1,64}`. It must never contain configuration, Secrets or input. Shown to users and recorded with the attempt. |
| `attempt.retryAfterSeconds` | Optional, 0 to 86400. A transient result asks GoPager to wait at least this long before the next attempt; the later of this time and GoPager's own retry schedule applies. |
| `attempt.receipt` | Optional JSON, at most 4096 bytes, that identifies the delivery at the provider (for example an HTTP status and a request ID). Shown with the attempt. |

How GoPager maps what it receives (before and after the request reached the plugin):

| Situation | Recorded as |
| --- | --- |
| Instance, artifact, process start or handshake failed before the request was sent | `transient` `plugin-unavailable` |
| Configuration or Secrets cannot be read or resolved (no master key, row of another key, missing file or variable, configuration that does not satisfy the schema) | `transient` `configuration-unavailable` |
| Request over 1 MiB | `permanent` `request-too-large` |
| Transport failure, expired deadline, malformed, non-UTF-8 or oversized (> 256 KiB) response, unknown `status`, `retryAfterSeconds` out of range, after the request was sent | `uncertain` |
| `code` outside `[a-z0-9-]{1,64}` | the plugin's status with code `invalid-code` |
| Receipt over 4096 bytes, not JSON, or containing a resolved Secret of this invocation (in raw bytes or in any decoded string or key) | the receipt is dropped and the code becomes `receipt-redacted` |

The Go SDK maps its own failures to results: a request it cannot decode is `permanent` `invalid-request` (nothing was sent), an invalid result of the handler and a panicking handler are `uncertain` `invalid-result` and `handler-failed`. Failure details never leave the plugin process; only the status, the code and the receipt are recorded. After a `transient` result GoPager retries: up to 5 attempts in total, with the delays 30 s, 2 min, 10 min and 30 min, each ±20 % jitter; `uncertain` Deliveries are never retried automatically (see [HTTP API: Deliveries](http-api.md#deliveries)).

## 5. Limits

| Limit | Value | Enforced by |
| --- | --- | --- |
| Call deadline | 30 s per `Invoke`, including waiting for the process to become free and process start; exceeding it is `RESOURCE_LIMIT` | server context + gRPC deadline |
| Handshake | 10 s | go-plugin start timeout |
| Request JSON | ≤ 1 MiB (a larger Delivery request ends as `permanent` `request-too-large`) | server (before sending) |
| Response JSON | ≤ 256 KiB | gRPC receive limit + server check |
| Nesting depth | 32 (JSON, values, schemas) | server |
| Collection size | 1000 entries per object, list, schema property map and `fields` map | server |
| Decoded `bytes` | ≤ 1 MiB aggregate per value tree | server |
| Workflow aggregate | Encoded inputs, envelopes and intermediate step results of one run ≤ 1 MiB; final workflow result ≤ 256 KiB | server |
| Built-in compute budget | 1 s per run for built-in steps; time spent waiting for plugins is not charged; exhausting it is `RESOURCE_LIMIT` | server |
| Delivery attempt | 30 s from the start of the attempt, like a processing call | server context + gRPC deadline |
| Delivery receipt, retry hint | Receipt ≤ 4096 bytes of JSON; `retryAfterSeconds` 0 to 86400; `code` 1 to 64 characters of `[a-z0-9-]` | server |
| Delivery declaration | ≤ 64 inputs, ≤ 16 dependencies, `sendsPerMinute` 0 to 100000, dependency description ≤ 256 bytes | server (manifest validation) |
| Secrets | A `file:` Secret ≤ 64 KiB; a path ≤ 1024 bytes | server |
| Executable size | ≤ 256 MiB | server (digest check) |
| Manifest size | ≤ 1 MiB | server |
| Memory / CPU | none | use OS or container limits |
| Concurrency | Worker processes 2 candidates in parallel (fixed default); preview requests run in addition. One process per instance revision handles one call at a time, so concurrent runs of the same instance are serialized. | server |

## 6. Packaging and installation

### Artifact directory

The operator provisions executables and manifests in one local directory and passes it with `--processing-artifact-root <dir>` or `GOPAGER_PROCESSING_ARTIFACT_ROOT` (the flag wins; an explicitly empty flag or a path with leading/trailing whitespace is a startup error; a directory that cannot be opened is a startup error). Without a root, built-in processing works, `installed` steps fail with `AUTHORITY_UNAVAILABLE` and no Delivery plugin instance is available as a target. The server never downloads, compiles or installs plugins. Deployment details: [Operations](operations.md).

All paths are relative to the root and cannot leave it (`..`, absolute paths and symlinks pointing outside are rejected). A possible layout:

```text
/var/lib/gopager/plugins/
├── upper/
│   ├── 1.0.0/
│   │   ├── manifest.json
│   │   └── upper            # executable, mode 0755
```

`artifact.path` in a manifest is resolved **relative to the root**, not relative to the manifest file. With the layout above, `upper/1.0.0/manifest.json` contains `"path": "upper/1.0.0/upper"`.

#### First-party plugins in the image

The [Docker Compose image](operations.md#lite-with-docker-compose) ships the two first-party plugins, built with their manifests (digests filled in), under `/usr/lib/gopager/plugins` and sets `GOPAGER_PROCESSING_ARTIFACT_ROOT` to that directory. The directory is owned by root and read-only for the `gopager` user:

```text
/usr/lib/gopager/plugins/
├── pushover/1.0.0/{manifest.json,pushover}
└── webhook/1.0.0/{manifest.json,webhook}
```

An instance pins them with `manifestPath` `pushover/1.0.0/manifest.json` (or `webhook/1.0.0/manifest.json`), `pluginId` and `capabilityId` equal to the plugin name, `version` `1.0.0` and the digest from the manifest. The digest differs per build; read it from the shipped `manifest.json` (`artifact.digest`) or let instance inspection report the manifest. Outside Docker, `make plugins` builds the same layout into `build/plugins/` ([Development](development.md#building-the-first-party-plugins)). A binary installation without Docker must provide the directory itself and copy these files into it on every node that runs the `worker` role.

### Manifest

JSON, UTF-8, at most 1 MiB. Unknown members and duplicate keys are rejected.

| Field | Type | Required | Rules |
| --- | --- | --- | --- |
| `schemaVersion` | number | yes | `2` |
| `sdkMajor` | number | yes | `2` |
| `sdkMinor` | number | yes | `0` or `1`. A `delivery` manifest requires `1`. A `processing` manifest may declare either. An unknown minor or another `sdkMajor` is `INCOMPATIBLE_VERSION`. |
| `capability` | string | no | `"processing"` (default when absent) or `"delivery"`. Any other value is `INVALID_INPUT`. |
| `pluginId` | string | yes | Non-empty, no NUL. Must equal the instance pin. A `delivery` plugin ID must be canonical: lowercase letters and digits in groups joined by single hyphens (`^[a-z0-9]+(-[a-z0-9]+)*$`), so its Secret directory and variable prefix are unique. |
| `version` | string | yes | Non-empty, no NUL. Must equal the instance pin. |
| `capabilityId` | string | yes | Non-empty, no NUL. Must equal the instance pin; sent as `capabilityId`. |
| `artifact.path` | string | yes | Relative path (forward slashes) of the executable inside the root. |
| `artifact.digest` | string | yes | Lowercase hex SHA-256 of the executable. Must equal the pin and the file. |
| `pure` | boolean | `processing`: yes | `processing`: must be `true`. `delivery`: `false` or omitted. |
| `supportsPreview` | boolean | no | `processing` only: `true` if the plugin handles [`mode: "preview"`](#preview-mode) without effects outside its process. Default `false`: previews of workflows using this plugin fail with `AUTHORITY_UNAVAILABLE`. A `delivery` manifest must omit it or set `false`. The former `purePreview` member is gone and, like any unknown member, rejected. |
| `inputSchema` | object | `processing`: yes | Field name → [field schema](#field-schema), ≤ 1000 entries. A `delivery` manifest declares its inputs in `delivery.inputs` and must omit this member (or leave it empty). |
| `resultSchema` | object | `processing`: yes | Field name → field schema, ≤ 1000 entries. Empty or omitted for `delivery`. |
| `configurationSchema` | field schema | yes | Kind `object`; no `bytes` anywhere. |
| `configurationPolicy` | policy | yes | See [configuration policy](#configuration-policy). |
| `requiredCoreFields` | array | no | `processing`: empty or omitted. `delivery`: non-empty is refused with `CAPABILITY_UNDECLARED` (403): a Delivery plugin gets no read access to GoPager data. |
| `requiredOperations` | array | no | Same rule as `requiredCoreFields`. |
| `hostCapabilities` | array | no | Must be empty or omitted. |
| `delivery` | object | `delivery`: yes | The [Delivery declaration](#delivery-declaration). A `processing` manifest must not have it (`INVALID_INPUT`). |
| `resources` | object | no | `delivery` only: informational `{"memoryMiB": <number ≥ 0>, "note": "<≤ 256 bytes>"}` shown by instance inspection. Never enforced; use OS or container limits. |

Example (adapted from `internal/processing/testdata/instance-manifest.json`):

```json
{
  "schemaVersion": 2,
  "sdkMajor": 2,
  "sdkMinor": 0,
  "pluginId": "upper",
  "version": "1.0.0",
  "capabilityId": "upper",
  "artifact": {
    "path": "upper/1.0.0/upper",
    "digest": "5e718916f839bcdcd66bd15a9ad4eae5ff7228b029898d602df75901f5b1d847"
  },
  "pure": true,
  "supportsPreview": true,
  "inputSchema": {"text": {"kind": "string"}},
  "resultSchema": {"upper": {"kind": "string"}},
  "configurationSchema": {
    "kind": "object",
    "properties": {"prefix": {"kind": "string", "optional": true}}
  },
  "configurationPolicy": {
    "configurationScope": "admin",
    "secret": false,
    "properties": {
      "prefix": {"configurationScope": "admin", "secret": false}
    }
  }
}
```

#### Field schema

| Member | Type | Rules |
| --- | --- | --- |
| `kind` | string | `string`, `boolean`, `integer`, `number`, `timestamp`, `bytes`, `object`, `list` |
| `optional` | boolean | Field/property may be absent. Default `false`. |
| `nullable` | boolean | Explicit `null` allowed. Default `false`. |
| `properties` | object | Only for `object`: name → field schema. |
| `element` | field schema | Required for `list`, forbidden otherwise. |

#### Delivery declaration

The `delivery` object of a `delivery` manifest states what the plugin offers as a Delivery target. GoPager presents it as the target's descriptor to Subscriptions and validates the inputs of every Subscription against it.

| Member | Type | Rules |
| --- | --- | --- |
| `outputKinds` | array of string | Required, non-empty, distinct, from `text`, `markdown`, `json`. The output kinds of the Layout the plugin can consume; a Subscription whose Layout renders another kind cannot select the target. |
| `inputs` | array | At most 64 entries `{"name", "kind", "required", "maxBytes"}`. `name` is unique and valid as a local name, `kind` is a scalar: `string`, `boolean`, `integer`, `number`, `timestamp` or `bytes`. `required` defaults to `false`. `maxBytes` bounds the encoded value; `0` or omitted selects the Delivery default. |
| `dependencies` | array | At most 16 entries `{"kind", "description"}`. `kind` is `network`, `secret`, `device` or `other`. `description` is 1 to 256 bytes of UTF-8 and must not reveal a Secret or a destination. Shown to administrators and subscribers. |
| `sendsPerMinute` | number | 0 to 100000. A positive value lowers the sending admission of this instance below the server-wide per-instance limit; `0` adds no limit of its own. |
| `idempotent` | boolean | `true` if the provider deduplicates by the Delivery ID, so a later attempt of the same Delivery cannot produce a second notification. Informational. |

An administrator reaches the declaration of a manifest without starting the plugin through `POST /api/v1/instances/inspection` ([HTTP API](http-api.md#processing-plugin-instances)). A target's `configurationAuthority` is `admin` when every configurable property is administrative and `admin-and-personal` when the manifest also has `user`-scoped properties.

Example (the Webhook manifest template, with the digest filled in):

```json
{
  "schemaVersion": 2,
  "sdkMajor": 2,
  "sdkMinor": 1,
  "pluginId": "webhook",
  "version": "1.0.0",
  "capabilityId": "webhook",
  "capability": "delivery",
  "artifact": {"path": "webhook/1.0.0/webhook", "digest": "<sha256 of the executable>"},
  "configurationSchema": {
    "kind": "object",
    "properties": {
      "url": {"kind": "string"},
      "bearerToken": {"kind": "string", "optional": true},
      "hmacSecret": {"kind": "string", "optional": true},
      "followRedirects": {"kind": "boolean", "optional": true},
      "allowPrivateDestinations": {"kind": "boolean", "optional": true}
    }
  },
  "configurationPolicy": {
    "configurationScope": "admin",
    "secret": false,
    "properties": {
      "url": {"configurationScope": "admin", "secret": false},
      "bearerToken": {"configurationScope": "admin", "secret": true},
      "hmacSecret": {"configurationScope": "admin", "secret": true},
      "followRedirects": {"configurationScope": "admin", "secret": false},
      "allowPrivateDestinations": {"configurationScope": "admin", "secret": false}
    }
  },
  "delivery": {
    "outputKinds": ["json", "text", "markdown"],
    "inputs": [{"name": "topic", "kind": "string", "required": false, "maxBytes": 256}],
    "dependencies": [
      {"kind": "network", "description": "HTTPS POST to the configured URL"},
      {"kind": "secret", "description": "Optional Bearer token and HMAC signing secret"}
    ],
    "sendsPerMinute": 0,
    "idempotent": false
  }
}
```

#### Configuration policy

The policy mirrors `configurationSchema` node by node: the root, every object property (`properties`, exactly the same names) and every list element (`element`). Scalars have neither. Each node requires:

| Member | Values |
| --- | --- |
| `configurationScope` | `admin` or `user` |
| `secret` | `true` or `false` (explicit; omission is invalid) |

`configurationScope` states who sets the value. `admin` values are set by an administrator on the instance (`plugin.configure`). `user` values are set by each subscriber for themselves on a `delivery` instance that admits them ([HTTP API: personal configuration](http-api.md#processing-plugin-instances)); a subscriber's values never reach another subscriber's Deliveries. Every property has exactly one scope, so a personal value never shadows an administrative one: the plugin receives one merged `configuration` object that holds each property from the side that owns it, and a personal value for an `admin` or undeclared property is rejected at write time. `secret: true` marks a value that is stored and shown only as a [Secret](#secrets).

Rules common to all plugins: the root node must be `admin`.

Rules for `delivery` plugins:

- `secret: true` is allowed only on `string` properties that are not inside a list; objects and lists are never Secrets.
- A `user`-scoped property must be `optional` in the schema (the administrative record never holds it), and everything below a `user` container is `user`-scoped. A list element has the scope of its list, and an `admin` list holds no `user`-scoped node at any depth (a list is scope-uniform throughout its element subtree).
- An `admin` object that is `nullable` holds no `user`-scoped descendant: an administrative `null` would hide the personal values, and there is no unambiguous merge.
- Property names contain no `.`, because dotted paths address Secrets.
- A manifest that breaks a rule is refused at inspection and instance save and is `failed` in the instance view.

Rules for `processing` plugins: a manifest with any `"secret": true` node loads but cannot be configured or invoked (`AUTHORITY_UNAVAILABLE`), because a Workflow step has no owner for a Secret. `user`-scoped properties cannot be set through an instance; declare them `optional` or the configuration fails validation.

#### Secrets

A Secret is a `string` property whose policy says `secret: true`. Its value is never part of `configuration` in a request, response or view. It is held as a reference string, one of:

| Reference | Source | Resolved |
| --- | --- | --- |
| `secret:<uuid>` | An immutable row of the encrypted in-app store | At every attempt, decrypted with the master key. |
| `file:<plugin ID>/<path>` | A regular file below the operator's Secret file root (`GOPAGER_SECRET_FILE_ROOT`) in the directory named after the plugin ID, at most 64 KiB, one trailing newline removed, not empty | At every attempt, so a changed file is a new Secret version without restarting GoPager. |
| `env:GOPAGER_PLUGIN_SECRET_<ID>__<NAME>` | An environment variable of the GoPager process; the prefix is mandatory so an administrator cannot route a setting such as `GOPAGER_DATABASE_URL` into a plugin, and `<ID>` (the plugin ID upper-cased, characters outside `A-Z0-9` replaced by `_`) binds it to the plugin, so `pushover` reads `GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN` but not `GOPAGER_PLUGIN_SECRET_WEBHOOK__TOKEN` | At every attempt, but the process environment changes only with a restart. |

Operator-placed Secrets are bound to the plugin: an instance may only reference a file below its own plugin-ID directory and variables in its own `GOPAGER_PLUGIN_SECRET_<ID>__` namespace, so an administrator cannot send another plugin's Secret to a destination they control. Anything else is `INVALID_INPUT` on save, an instance holding it is `failed`, and an attempt answers `configuration-unavailable` without starting the plugin.

Administrators write Secrets with the `secrets` member of an instance write: `{"value": "<plaintext>"}` seals the value into a new `secret:` row, `{"reference": "file:..."}` or `{"reference": "env:..."}` sets an operator-placed Secret, and `null` removes an optional one; a path left out keeps the previous revision's Secret, as long as every object above it is still present in the submitted `configuration` (top-level Secrets always qualify). If an optional object is omitted, its Secrets are dropped with it and the object stays absent; a retained object must still satisfy its required Secrets. Subscribers write their personal Secrets with `{"value": ...}` only. `file:` and `env:` Secrets are operator-placed and apply to administrative values: a personal Secret is always a stored value sealed for its owner, and a `file:` or `env:` reference in a personal write, or found in a personal value when an attempt resolves it, is refused. Details of the routes: [HTTP API](http-api.md#processing-plugin-instances). Key handling, rotation limits and logging: [Operations: Secrets](operations.md#secrets).

Each Secret reaches only the plugin invocation that needs it, as plaintext in the `configuration` of the [Delivery request](#delivery-request), together with its version in `secretVersions`; the server drops it when the call ends. A plugin must not log, echo or put a Secret into a `code` or `receipt`: a receipt that contains a Secret of the invocation is dropped. Without a master key, writing a stored value is refused and an instance whose configuration holds any Secret reference is unavailable (`AUTHORITY_UNAVAILABLE`, attempts end as `transient` `configuration-unavailable`).

### Digest

```sh
shasum -a 256 upper/1.0.0/upper     # macOS
sha256sum upper/1.0.0/upper         # Linux
```

Use the lowercase hex output in `artifact.digest` and in the instance pin. When an artifact is loaded the server streams the file (regular file, at least one execute bit, ≤ 256 MiB) and compares the digest; later loads reuse that result while size, modification time and mode are unchanged. Every process start is verified again by go-plugin (see [Verified start](#1-runtime-model)). Replacing the file changes the digest and makes existing instances fail with `AUTHORITY_UNAVAILABLE` until they are updated. Ship a new version in a new path rather than overwriting the old one.

### Referencing a plugin

An **instance** binds a manifest to configuration and an execution principal (`POST /api/v1/instances`, see [Workflows](workflows.md)):

```json
{
  "schemaVersion": 1,
  "commandId": "6d881468-4f69-4d81-a08a-95ea30bb1653",
  "enabled": true,
  "executionPrincipalId": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
  "main": {
    "manifestPath": "upper/1.0.0/manifest.json",
    "pin": {
      "pluginId": "upper",
      "version": "1.0.0",
      "capabilityId": "upper",
      "digest": "5e718916f839bcdcd66bd15a9ad4eae5ff7228b029898d602df75901f5b1d847"
    }
  },
  "configuration": {"kind": "object", "value": {"prefix": {"kind": "string", "value": "FW: "}}}
}
```

A workflow step references the instance by ID and configuration revision; its `outputSchema` must equal the manifest `resultSchema` exactly, and its `input` assignments must match `inputSchema`:

```json
{
  "id": "upper",
  "kind": "installed",
  "outputSchema": {"upper": {"kind": "string"}},
  "installed": {
    "instanceId": "<instance id>",
    "configurationRevision": "1",
    "input": {"text": {"field": {"namespace": "source", "path": ["payload", "text", "value"]}}}
  }
}
```

A `delivery` instance additionally carries `targetUse` and, for Secret properties, `secrets` (`executionPrincipalId` stays required for every instance; the content of a Delivery is authorized as the Subscription's subscriber):

```json
{
  "schemaVersion": 1,
  "commandId": "0b5e1d3a-77c2-4d5f-9d0e-4f6a1c2b3d44",
  "enabled": true,
  "executionPrincipalId": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
  "main": {
    "manifestPath": "webhook/1.0.0/manifest.json",
    "pin": {"pluginId": "webhook", "version": "1.0.0", "capabilityId": "webhook", "digest": "<manifest artifact.digest>"}
  },
  "configuration": {"kind": "object", "value": {"url": {"kind": "string", "value": "https://hooks.example.com/gopager"}}},
  "secrets": {"hmacSecret": {"value": "choose-a-long-random-secret"}},
  "targetUse": {"everyone": true, "principalIds": []}
}
```

`targetUse` states who may use the instance as a Subscription target: `everyone`, or the listed Principals (at most 1000 distinct IDs). It is checked when a Subscription is saved (a Principal that may not use the instance sees an unavailable target, indistinguishable from a missing one), when an event fans out (no Delivery is created for that target) and again just before every attempt starts (the Delivery is cancelled with `authority-revoked`). A `delivery` instance requires it, a `processing` instance must not carry it. Groups are not supported.

### Preview

Workflow preview calls the instance's own plugin with `mode: "preview"`; there is no separate preview artifact or `preview` member in an instance. A plugin that does not declare `supportsPreview` is never called in preview, and preview of an `installed` step that uses it returns `AUTHORITY_UNAVAILABLE`. The plugin author is responsible for making preview effect-free; the server cannot enforce it, just as it cannot enforce `pure`. A `delivery` plugin has no preview mode.

### Platform requirements

- The executable must run on the OS and CPU architecture of the GoPager server process (inside its container, if containerized). No cross-platform selection exists; provision the matching build.
- Scripts with a `#!` interpreter line are allowed. The digest covers only the script; the interpreter must be present in the server's environment and found via the plugin's `PATH`.
- Static binaries (`CGO_ENABLED=0` for Go) avoid libc mismatches in minimal container images.
- Device or network access needed by the plugin must be granted to the server process or container.

## 7. Writing a plugin in Go

`pluginsdk` provides the handshake, the gRPC service registration and the `Handler` interface:

```go
type Handler interface {
	Invoke(ctx context.Context, request []byte) ([]byte, error)
}
func Serve(handler Handler)          // blocks until the server stops the plugin
type HandlerFunc func(ctx context.Context, request []byte) ([]byte, error)
func RequestMode(request []byte) (string, error) // ModeLive or ModePreview
```

The server keeps the process alive between calls and sends it one request at a time, so a handler may cache state (connections, compiled expressions) across calls but must not assume a fresh process per run, and must tolerate being stopped at any time.

`ctx` carries the call deadline and is cancelled when the server gives up. The SDK passes raw JSON; decoding and typed-value handling are the plugin's job.

### Example

`go.mod`:

```text
module example.com/gopager-upper

go 1.27.4

require github.com/gopager/gopager v0.0.0
```

Until the SDK is published at a tagged version, add `replace github.com/gopager/gopager => /path/to/gopager` and run `go mod tidy`.

`main.go`:

```go
// Command upper is a GoPager processing plugin that upper-cases a text field.
package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gopager/gopager/pluginsdk"
)

// value is the typed wire value {"kind": ..., "value": ...}.
type value struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

type request struct {
	SchemaVersion int              `json:"schemaVersion"`
	InstanceID    string           `json:"instanceId"`
	Mode          string           `json:"mode"`
	Input         map[string]value `json:"input"`
	Configuration value            `json:"configuration"`
}

type response struct {
	SchemaVersion int              `json:"schemaVersion"`
	Fields        map[string]value `json:"fields,omitempty"`
	Failure       string           `json:"failure,omitempty"`
}

func failure(reason string) ([]byte, error) {
	return json.Marshal(response{SchemaVersion: 2, Failure: reason})
}

func handle(ctx context.Context, data []byte) ([]byte, error) {
	var req request
	if err := json.Unmarshal(data, &req); err != nil || req.SchemaVersion != 2 {
		return failure("conversion-failed")
	}
	// Upper-casing has no effects, so "preview" computes the same result as "live" and the
	// manifest can declare supportsPreview. A plugin with effects must branch on req.Mode here
	// (pluginsdk.ModeLive / pluginsdk.ModePreview) and skip them in preview.
	if req.Mode != pluginsdk.ModeLive && req.Mode != pluginsdk.ModePreview {
		return failure("conversion-failed")
	}
	// Configuration is an object of typed values; "prefix" is optional.
	var config map[string]value
	if err := json.Unmarshal(req.Configuration.Value, &config); err != nil {
		return failure("conversion-failed")
	}
	prefix := ""
	if p, ok := config["prefix"]; ok {
		if err := json.Unmarshal(p.Value, &prefix); err != nil {
			return failure("conversion-failed")
		}
	}
	var text string
	if err := json.Unmarshal(req.Input["text"].Value, &text); err != nil {
		return failure("conversion-failed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err // deadline passed; the Core discards the call anyway
	}
	encoded, err := json.Marshal(prefix + strings.ToUpper(text))
	if err != nil {
		return failure("evaluation-failed")
	}
	return json.Marshal(response{SchemaVersion: 2, Fields: map[string]value{
		"upper": {Kind: "string", Value: encoded},
	}})
}

func main() {
	pluginsdk.Serve(pluginsdk.HandlerFunc(handle))
}
```

The matching manifest is the [example above](#manifest). Note that `fields` is never `nil` on success: an empty result must still be `"fields":{}` (use a non-nil map without `omitempty`, or a pointer).

### Build

```sh
CGO_ENABLED=0 go build -trimpath -o upper .                                   # host platform
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o upper .           # Linux x86-64 server
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o upper .           # Linux ARM64 server
sha256sum upper
```

Running the binary directly prints a go-plugin notice and exits with status 1; it only runs when started by the server.

### Test

Unit-test the handler with an envelope literal:

```go
func TestHandle(t *testing.T) {
	request := `{"schemaVersion":2,"invocationId":"0b6f0a43-5d0e-4a8e-9a43-2f1f3c1b6a10",` +
		`"instanceId":"5f0c6f7e-3a3b-4d43-8f0e-6f8f3c0a1b2c","artifactDigest":"00","capabilityId":"upper",` +
		`"workflowId":"8a7d9a52-1d3c-4b0e-9a0f-0d4d6b9e8c11","definitionVersionId":"c3f1b2a4-6e5d-4c3b-8a29-1f0e9d8c7b6a",` +
		`"runId":"e2d4c6b8-a0f1-4e3d-9c5b-7a6f8e0d2c4b","executionPrincipalId":"1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",` +
		`"configurationRevision":"1","deadline":"2026-11-05T14:00:30.5+01:00","mode":"live",` +
		`"input":{"text":{"kind":"string","value":"alarm"}},` +
		`"configuration":{"kind":"object","value":{"prefix":{"kind":"string","value":"FW: "}}}}`
	got, err := handle(context.Background(), []byte(request))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":2,"fields":{"upper":{"kind":"string","value":"FW: ALARM"}}}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
```

Test the built executable over the real protocol with a small host that starts it exactly as the server does (go-plugin client, `pluginsdk.Handshake`, gRPC only, empty host environment):

```go
// Command hostcheck starts a plugin executable and sends one request.
// Usage: hostcheck <plugin executable> <request.json>
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/hashicorp/go-plugin"

	"github.com/gopager/gopager/pluginsdk"
)

func main() {
	request, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  pluginsdk.Handshake,
		Plugins:          plugin.PluginSet{pluginsdk.PluginName: &pluginsdk.GRPCPlugin{}},
		Cmd:              exec.Command(os.Args[1]),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		SkipHostEnv:      true,
	})
	defer client.Kill()
	protocol, err := client.Client()
	if err != nil {
		panic(err)
	}
	raw, err := protocol.Dispense(pluginsdk.PluginName)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := raw.(*pluginsdk.Client).Invoke(ctx, request, 256<<10)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(response))
}
```

```sh
go run ./cmd/hostcheck ./upper request.json
# {"schemaVersion":2,"fields":{"upper":{"kind":"string","value":"ALARM"}}}
```

This does not validate the response against the manifest. For an end-to-end check, provision the artifact directory, create an instance and run a workflow preview (see [Workflows](workflows.md)); the plugin must declare `supportsPreview`. The server's own fixture is `internal/processing/testdata/command`; it answers `mode: "preview"` by prefixing its `text` field with `PREVIEW:`.

### Delivery plugins

A Delivery plugin implements `pluginsdk.DeliveryHandler` and starts with `pluginsdk.ServeDelivery` instead of `Serve`:

```go
type DeliveryHandler interface {
	Deliver(ctx context.Context, request DeliveryRequest) DeliveryResult
}
func ServeDelivery(handler DeliveryHandler)  // blocks until the server stops the plugin
type DeliveryHandlerFunc func(ctx context.Context, request DeliveryRequest) DeliveryResult

func (r DeliveryRequest) DecodeConfiguration(target any) error      // merged configuration into a struct
func (r DeliveryRequest) ConfigurationValues() (map[string]Value, error)

type DeliveryResult struct {
	Status            string          // StatusAccepted, StatusTransient, StatusPermanent, StatusUncertain
	Code              string          // [a-z0-9-]{1,64}, never a Secret
	RetryAfterSeconds int             // 0..86400, 0 = none
	Receipt           json.RawMessage // optional JSON, at most MaxReceiptBytes (4096), never a Secret
}
```

`ServeDelivery` decodes the request strictly (an unknown member, a `kind` other than `delivery` or another `schemaVersion` is answered `permanent` `invalid-request`), gives the handler a context that ends at the request `deadline`, validates the result and encodes the response. The handler does not touch JSON envelopes. `DeliveryRequest.Attempt.Inputs` holds `pluginsdk.Value` entries with `String()`, `Boolean()`, `Integer()`, `Number()` and `Plain()` (typed value to plain JSON); each checks that the encoding matches the kind (an explicit `null` only for kind `null`, finite numbers, RFC 3339 timestamps, canonical Base64 bytes) and returns an error otherwise, so `DecodeConfiguration` never accepts a mistyped value. `DecodeConfiguration` first turns the typed configuration into plain JSON (integers become numbers), then unmarshals it into a struct with `json` tags; an optional property that is absent keeps its zero value.

A minimal handler that POSTs the body to a configured URL with a Secret token:

```go
type config struct {
	URL   string `json:"url"`
	Token string `json:"token"` // Secret property: plaintext here
}

func deliver(ctx context.Context, request pluginsdk.DeliveryRequest) pluginsdk.DeliveryResult {
	var cfg config
	if err := request.DecodeConfiguration(&cfg); err != nil {
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusPermanent, Code: "configuration-invalid"}
	}
	target, err := safehttp.SecureURL(cfg.URL, false)
	if safehttp.IsDenied(err) {
		return safehttp.ErrorResult(err) // a refused scheme is permanent "destination-denied"
	}
	if err != nil {
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusPermanent, Code: "configuration-invalid"}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(),
		strings.NewReader(request.Attempt.Body))
	if err != nil {
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusPermanent, Code: "configuration-invalid"}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+cfg.Token)
	response, err := safehttp.NewClient(safehttp.Policy{}).Do(httpRequest)
	if err != nil {
		return safehttp.ErrorResult(err)
	}
	if response.StatusCode/100 == 2 {
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusAccepted, Code: "accepted"}
	}
	return pluginsdk.DeliveryResult{Status: pluginsdk.StatusPermanent, Code: "rejected"}
}

func main() { pluginsdk.ServeDelivery(pluginsdk.DeliveryHandlerFunc(deliver)) }
```

Choose the status by what you know about the effect: `transient` only if nothing reached the provider, `permanent` if it will never succeed (bad configuration, rejected input), `uncertain` as soon as the request may have been processed (a lost response, an unexpected answer). Never answer `transient` after the provider may have acted: GoPager would send the notification again.

#### safehttp

Plugins that call a destination an administrator configures should use `pluginsdk/safehttp` instead of `net/http` directly. A `Client` built from a `Policy` (the zero value is the strictest):

- resolves the host itself, checks **every** resolved address and dials the address it checked, so a changing DNS answer cannot redirect the connection afterwards; it uses no proxy;
- denies loopback, unspecified, multicast, link-local, private (RFC 1918 and RFC 4193) and carrier-grade NAT addresses, the cloud metadata addresses `169.254.169.254` and `fd00:ec2::254`, the reserved ranges `0.0.0.0/8`, `192.0.0.0/24`, `198.18.0.0/15` and `240.0.0.0/4`, and the IPv6 transition ranges that tunnel an IPv4 destination (`64:ff9b:1::/48`, `2002::/16`, `2001::/32`, `::/96`); IPv4-mapped and NAT64 (`64:ff9b::/96`) addresses are judged by the IPv4 address they carry;
- denies ports below 1024 other than 80 and 443, and verifies TLS (1.2 or later) against the URL host;
- follows no redirect unless `FollowRedirects` is set; then at most 3, never from HTTPS to HTTP, each hop checked again, and `Authorization` plus the headers named in `SensitiveHeaders` dropped when the scheme, host or port changes;
- bounds the call (`Timeout`, default 10 s) and the stored response body (`MaxResponseBytes`, default 64 KiB; `Response.Truncated` tells), and reports through `RequestError.Written` whether the request was completely written.

`Policy.AllowPrivate` (a plugin ties it to an administrator-set `allowPrivateDestinations` property, default `false`) lifts the address rules, including the cloud-metadata rule, and the port rule, for intended internal infrastructure; the other protections stay. `SecureURL` accepts only `https` URLs, or `http` when `AllowPrivate` is set. `ErrorResult` classifies a `Do` error (denied: permanent `destination-denied`; failure before the request was written: transient `connection-failed`; failure after: uncertain `response-lost`) and `RateLimitedResult` turns an HTTP 429 into transient `rate-limited` with the `Retry-After` value (default 60 s, at most 3600 s). A third-party plugin that does not use `safehttp` applies its own policy; that is explicit trust, not a server-side SSRF guarantee.

#### Testing with plugintest

`pluginsdk/plugintest` runs the real, built executable exactly as the server does, against a local fake of the provider. It writes a temporary plugin directory with the manifest (digest filled in), verifies the manifest with the server's rules, starts the plugin through the server's plugin host and performs attempts through the server's Delivery host, so the plugin receives the same request (merged configuration with Secrets resolved, `secretVersions`, typed inputs) and the test sees the same classified outcome, including the receipt checks and the error that makes the server record an attempt as `uncertain`.

```go
func TestDeliversToTheProvider(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer provider.Close()

	h := plugintest.New(t, plugintest.Options{
		Executable:           plugintest.BuildPlugin(t, ".", "."),
		ManifestTemplatePath: "manifest.template.json", // with "@DIGEST@" as artifact.digest
	})
	h.Configure(map[string]any{"url": provider.URL, "allowPrivateDestinations": true},
		map[string]plugintest.Secret{"token": plugintest.SecretValue("s3cret")})
	outcome, err := h.Attempt(plugintest.AttemptInput{Body: "disk full", Inputs: map[string]any{"topic": "alarms"}})
	if err != nil || outcome.Status != pluginsdk.StatusAccepted {
		t.Fatalf("%+v %v", outcome, err)
	}
}
```

The test server listens on loopback, so a plugin that uses `safehttp` needs its private-destinations switch on in the test. `Harness` offers `Configure(values, secrets)` (a new instance revision; `plugintest.SecretValue`, `SecretFile` with `WriteSecretFile`, `SecretEnv` with `SetEnv`; the harness places files and variables in the plugin's namespace, so pass the part after `<plugin ID>/` or `GOPAGER_PLUGIN_SECRET_<ID>__`), `Personal(subscriber, values, secrets)` for user-scoped values (their Secrets are always stored values), `Describe()` for the target descriptor, `Attempt(AttemptInput)` with output kind, body, inputs, attempt number and deadline, and `Subscriber()`/`NewSubscriber()`. The plugin module needs `go.mod` entries for the GoPager module (the first-party plugins use a `replace` directive to the repository root until the SDK is published at a tag).

## 8. Writing a plugin in another language

Follow go-plugin's [non-Go plugin guide](https://github.com/hashicorp/go-plugin/blob/main/docs/guide-plugin-write-non-go.md) with these values:

1. Exit with an error unless `GOPAGER_PLUGIN` equals `gopager-plugin-runtime-2`.
2. Accept application protocol `2` (`PLUGIN_PROTOCOL_VERSIONS` contains `2`).
3. Start a plaintext gRPC server on a Unix socket (e.g. in `TMPDIR`) or on `127.0.0.1` with a port in `PLUGIN_MIN_PORT`..`PLUGIN_MAX_PORT`.
4. Register `gopager.plugin.v2.Plugin` from [`plugin.proto`](../api/plugin/v2/plugin.proto) (method `Invoke`, `google.protobuf.BytesValue` in and out).
5. Register `grpc.health.v1.Health` reporting service `plugin` as `SERVING`.
6. Optionally implement `plugin.GRPCController/Shutdown` to exit on request; otherwise the process is killed 2 s after the server stops it (idle timeout, newer instance revision, shutdown). `plugin.GRPCStdio` and `plugin.GRPCBroker` are not needed.
7. After the server listens, print exactly one line to stdout and flush it: `1|2|unix|/path/to/socket|grpc` or `1|2|tcp|127.0.0.1:PORT|grpc`. Complete this within 10 s of process start.
8. Accept a request message of up to 1 MiB (plus framing); return at most 256 KiB of JSON.
9. Parse the [request envelope](#3-invocation-request), including `mode` (see [Preview mode](#preview-mode)); encode results as [typed values](#typed-values) (integers as decimal strings, bytes as padded standard Base64, timestamps as RFC 3339 with offset).
10. Return a [response document](#4-response) with `schemaVersion` 2 and exactly one of `fields` / `failure`; return `failure` instead of a gRPC error for expected failures.
11. Honour the gRPC deadline (≤ 30 s); work beyond it is discarded and the process is killed. Expect one request at a time and a process that lives across many runs.
12. Do not rely on stdout/stderr for diagnostics after the handshake line; they are discarded.
13. Do not rely on environment variables other than `PATH`, `HOME`, `LANG`, `LC_ALL`, `TZ`, `TMPDIR` and the go-plugin variables.
14. Package as an executable file (`chmod +x`), write the manifest, compute the SHA-256 and provision both in the artifact directory ([§6](#6-packaging-and-installation)).

## 9. First-party Delivery plugins

Two `delivery` plugins ship with GoPager, built from [`plugins/`](../plugins/) (separate Go modules on top of `pluginsdk`, `safehttp` and `plugintest`) and included in the [image](#first-party-plugins-in-the-image). Create an instance of each as described in [Referencing a plugin](#referencing-a-plugin). Both use `safehttp`, so their destinations follow the [address rules](#safehttp); neither retries by itself (GoPager's Delivery retry schedule does).

### Webhook

Posts every notification as a JSON document to a URL.

| Property | Scope | Secret | Meaning |
| --- | --- | --- | --- |
| `url` | admin | no | Required. An `https` URL. `http` is accepted only with `allowPrivateDestinations`. |
| `bearerToken` | admin | yes | Optional. Sent as `Authorization: Bearer <token>`. |
| `hmacSecret` | admin | yes | Optional. Signs every request, see [Verifying the signature](#verifying-the-signature). |
| `followRedirects` | admin | no | Optional, default `false`. Follows at most 3 redirects, never from HTTPS to HTTP; `Authorization`, `X-GoPager-Signature` and `X-GoPager-Timestamp` are dropped when a redirect changes the scheme, host or port. |
| `allowPrivateDestinations` | admin | no | Optional, default `false`. Permits private, loopback and other internal destinations and `http` URLs (also lifts the cloud-metadata and port rules). Enable it only for intended internal infrastructure. |

Output kinds: `json`, `text`, `markdown`. Input: `topic` (string, optional, at most 256 bytes); like every input it is sent in the `inputs` object of the payload, and the plugin does not interpret it. Dependencies: network, secret. It declares no sending limit of its own and is not idempotent.

**Request.** `POST` to `url` with these headers:

| Header | Value |
| --- | --- |
| `Content-Type` | `application/json` |
| `User-Agent` | `GoPager-Webhook/1` |
| `X-GoPager-Delivery` | The Delivery ID; identical in every attempt of the Delivery. Use it to deduplicate. |
| `X-GoPager-Attempt` | The attempt ID; unique per attempt. |
| `Authorization` | `Bearer <bearerToken>`, only when configured. |
| `X-GoPager-Timestamp` | Seconds since the Unix epoch when the request was signed, only when `hmacSecret` is configured. |
| `X-GoPager-Signature` | `sha256=` and the lowercase hex HMAC-SHA256, only when `hmacSecret` is configured. |

The body:

```json
{
  "deliveryId": "b8a1c6d2-0f4e-4a77-8d39-6c1e2f9a7b54",
  "attemptId": "7c1f2a60-3b1e-4d6c-9a0f-5e2b8c4d1a33",
  "attemptNumber": 1,
  "instanceId": "5f0c6f7e-3a3b-4d43-8f0e-6f8f3c0a1b2c",
  "correlationId": "…",
  "causationId": "2d9f7e41-8c0a-4b36-a1d5-3e7f9c0b6a12",
  "outputKind": "text",
  "body": "Fire alarm, station 3",
  "inputs": {"topic": "alarms"}
}
```

`correlationId` is omitted when the originating event has none. For `outputKind` `json` the `body` member is the rendered JSON itself (a JSON value, not a string; rendered output that is not valid JSON ends the attempt as `permanent` `input-invalid`); for `text` and `markdown` it is a JSON string. `inputs` maps each input name to its **plain JSON value**: strings as strings, integers as numbers, timestamps as RFC 3339 strings, bytes as Base64 strings; the typed `kind` envelope of the [wire format](#typed-values) is not sent. The body is not canonicalized: verify the signature over the exact bytes received.

**Result classification.**

| Receiver answer | Status | Code |
| --- | --- | --- |
| 2xx | `accepted` | `accepted` |
| 429 | `transient` | `rate-limited`, with `Retry-After` seconds (default 60, at most 3600) |
| 408, 503 | `transient` | `provider-unavailable` |
| 3xx while `followRedirects` is off | `permanent` | `redirect-refused` |
| other 4xx | `permanent` | `rejected` |
| anything else (for example 500, 502, 504 or an unexpected 1xx/3xx) | `uncertain` | `unexpected-response` |
| Destination refused by the address, port or scheme rules; too many redirects | `permanent` | `destination-denied` |
| Connection failed before the request was written | `transient` | `connection-failed` |
| Failure after the request was written (timeout, reset) | `uncertain` | `response-lost` |
| Invalid configuration, such as an unparsable URL | `permanent` | `configuration-invalid` |
| `json` body that is not JSON, or an input that cannot be converted | `permanent` | `input-invalid` |

Every answer carries a receipt `{"status": <HTTP status>, "requestId": "<X-Request-Id>"}`; `requestId` is present only when the receiver sent a printable `X-Request-Id` of at most 128 characters that contains no Secret.

#### Verifying the signature

With `hmacSecret` set, the signature is `HMAC-SHA256(key = hmacSecret, message = timestamp + "." + raw request body)`, hex-encoded and prefixed with `sha256=`. A receiver should:

1. read the **raw** body before parsing it;
2. recompute the signature from `X-GoPager-Timestamp` and the raw body and compare it with a constant-time comparison;
3. reject a timestamp outside a tolerance (5 minutes is a common choice; every attempt, including a retry, is signed anew with the current time), which bounds how long a captured request stays replayable;
4. deduplicate by `X-GoPager-Delivery`, because a Delivery can arrive more than once: after an `uncertain` answer GoPager does not retry by itself, but an operator can redeliver, and a receiver may have processed an attempt whose answer was lost.

A receiver in Go:

```go
const tolerance = 5 * time.Minute

// verify reports whether the request is correctly signed and fresh.
func verify(secret []byte, header http.Header, body []byte, now time.Time) bool {
	timestamp := header.Get("X-GoPager-Timestamp")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || now.Sub(time.Unix(seconds, 0)).Abs() > tolerance {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(header.Get("X-GoPager-Signature")))
}
```

The bearer token authenticates GoPager to the receiver but is not tied to the body; the signature is. Use the signature when the body matters, and use `https`.

### Pushover

Sends a notification to a Pushover user through the Pushover messages API. The application token is configured by the administrator, the recipient by each subscriber.

| Property | Scope | Secret | Meaning |
| --- | --- | --- | --- |
| `apiToken` | admin | yes | Required. The Pushover application token. |
| `title`, `sound` | admin | no | Optional defaults, overridden by the inputs of the same name. |
| `priority` | admin | no | Optional default, an integer from -2 to 1. |
| `endpoint` | admin | no | Optional, default `https://api.pushover.net/1/messages.json`. Must be `https` unless `allowPrivateDestinations` is set (for a test double or an internal relay). |
| `allowPrivateDestinations` | admin | no | Optional, default `false`. Permits private destinations and an `http` endpoint. |
| `userKey` | user | yes | The subscriber's Pushover user or group key. Without it the attempt ends as `permanent` `recipient-missing`. |
| `device` | user | no | Optional device name to address one device of the subscriber. |

Because it has `user` properties, the target's `configurationAuthority` is `admin-and-personal`: each subscriber sets `userKey` (stored, never shown again) and `device` through the personal configuration route of the instance. Output kind: `text` only. Inputs, all optional: `title` (at most 250 bytes), `priority` (integer from -2 to 1), `url` (512), `urlTitle` (100), `sound` (32). An input wins over the administrator's default of the same name; a priority outside -2..1 (the emergency priority is not supported) or an input of the wrong kind ends the attempt as `permanent` `input-invalid`. The message is the Delivery body, cut to 1024 characters (the receipt then holds `"truncated": true`). The plugin declares a limit of 60 sends per minute for the instance and is not idempotent. It sends a form-encoded `POST` and does not follow redirects.

**Result classification.**

| Pushover answer | Status | Code |
| --- | --- | --- |
| HTTP 200 and `status` 1 | `accepted` | `accepted` |
| 429 | `transient` | `rate-limited`, with `Retry-After` seconds (default 60, at most 3600) |
| 400 with `user` `invalid` | `permanent` | `recipient-invalid` |
| 400 with `status` 0 | `permanent` | `rejected` |
| 5xx | `transient` | `provider-unavailable` |
| anything else | `uncertain` | `unexpected-response` |
| Destination refused (address, port or scheme rules) | `permanent` | `destination-denied` |
| Connection failed before the request was written | `transient` | `connection-failed` |
| Failure after the request was written | `uncertain` | `response-lost` |
| Missing or undecodable configuration, such as an empty `apiToken` | `permanent` | `configuration-invalid` |
| `userKey` not set by the subscriber | `permanent` | `recipient-missing` |

The receipt holds the HTTP `status`, Pushover's `request` ID (when it does not contain a Secret) and `truncated`.
