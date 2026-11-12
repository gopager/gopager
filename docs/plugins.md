# Plugins

A GoPager plugin is an executable that implements one processing capability. A workflow step of kind `installed` passes typed input fields to the plugin and receives typed result fields back. Plugins are written in any language that can serve gRPC; Go plugins use the [`pluginsdk`](../pluginsdk/) package.

Related documents: [Workflows](workflows.md) (step definitions, instances, preview, retry) · [Operations](operations.md) (deployment, artifact directory) · [HTTP API](http-api.md).

## Contents

1. [Runtime model](#1-runtime-model)
2. [Wire protocol](#2-wire-protocol)
3. [Invocation request](#3-invocation-request)
4. [Response](#4-response)
5. [Limits](#5-limits)
6. [Packaging and installation](#6-packaging-and-installation)
7. [Writing a plugin in Go](#7-writing-a-plugin-in-go)
8. [Writing a plugin in another language](#8-writing-a-plugin-in-another-language)

## 1. Runtime model

| Aspect | Behavior |
| --- | --- |
| Process model | Child process of the GoPager server, started through [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin), gRPC protocol only. No in-process loading. |
| Where it runs | Every server process with the `api` role (workflow preview) or `worker` role (message processing) can start plugins. Each such process needs the artifact directory. |
| Process ownership | One long-lived process per **instance configuration revision** (a process pool in the server). Two instances of the same plugin never share a process. All live runs that use the same instance revision, including concurrent runs, share its process; preview invocations of that revision use a second pooled process, so an editor's preview never occupies the call slot a live run waits for. A process keeps its state between runs. |
| Calls per process | The server sends a process one `Invoke` at a time: concurrent runs queue on the process (the queue wait counts against the 30 s call deadline). A plugin handler therefore needs no locking against concurrent calls. A step that depends on message targets is invoked once per selected target. |
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

Steps of kind `installed` only accept plugins that declare themselves `pure` (no side effects, no host capabilities). The server cannot enforce purity; it is the author's declaration.

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

The request JSON is the invocation envelope. All fields are always present.

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
| Executable or manifest missing, digest mismatch, not executable, instance disabled, configuration declares a secret | `AUTHORITY_UNAVAILABLE` | yes |
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

## 5. Limits

| Limit | Value | Enforced by |
| --- | --- | --- |
| Call deadline | 30 s per `Invoke`, including waiting for the process to become free and process start; exceeding it is `RESOURCE_LIMIT` | server context + gRPC deadline |
| Handshake | 10 s | go-plugin start timeout |
| Request JSON | ≤ 1 MiB | server (before sending) |
| Response JSON | ≤ 256 KiB | gRPC receive limit + server check |
| Nesting depth | 32 (JSON, values, schemas) | server |
| Collection size | 1000 entries per object, list, schema property map and `fields` map | server |
| Decoded `bytes` | ≤ 1 MiB aggregate per value tree | server |
| Workflow aggregate | Encoded inputs, envelopes and intermediate step results of one run ≤ 1 MiB; final workflow result ≤ 256 KiB | server |
| Built-in compute budget | 1 s per run for built-in steps; time spent waiting for plugins is not charged; exhausting it is `RESOURCE_LIMIT` | server |
| Executable size | ≤ 256 MiB | server (digest check) |
| Manifest size | ≤ 1 MiB | server |
| Memory / CPU | none | use OS or container limits |
| Concurrency | Worker processes 2 candidates in parallel (fixed default); preview requests run in addition. One process per instance revision handles one call at a time, so concurrent runs of the same instance are serialized. | server |

## 6. Packaging and installation

### Artifact directory

The operator provisions executables and manifests in one local directory and passes it with `--processing-artifact-root <dir>` or `GOPAGER_PROCESSING_ARTIFACT_ROOT` (the flag wins; an explicitly empty flag or a path with leading/trailing whitespace is a startup error; a directory that cannot be opened is a startup error). Without a root, built-in processing works and `installed` steps fail with `AUTHORITY_UNAVAILABLE`. The server never downloads, compiles or installs plugins. Deployment details: [Operations](operations.md).

All paths are relative to the root and cannot leave it (`..`, absolute paths and symlinks pointing outside are rejected). A possible layout:

```text
/var/lib/gopager/plugins/
├── upper/
│   ├── 1.0.0/
│   │   ├── manifest.json
│   │   └── upper            # executable, mode 0755
```

`artifact.path` in a manifest is resolved **relative to the root**, not relative to the manifest file. With the layout above, `upper/1.0.0/manifest.json` contains `"path": "upper/1.0.0/upper"`.

### Manifest

JSON, UTF-8, at most 1 MiB. Unknown members and duplicate keys are rejected.

| Field | Type | Required | Rules |
| --- | --- | --- | --- |
| `schemaVersion` | number | yes | `2` |
| `sdkMajor` | number | yes | `2` |
| `sdkMinor` | number | yes | `0` |
| `pluginId` | string | yes | Non-empty, no NUL. Must equal the instance pin. |
| `version` | string | yes | Non-empty, no NUL. Must equal the instance pin. |
| `capabilityId` | string | yes | Non-empty, no NUL. Must equal the instance pin; sent as `capabilityId`. |
| `artifact.path` | string | yes | Relative path (forward slashes) of the executable inside the root. |
| `artifact.digest` | string | yes | Lowercase hex SHA-256 of the executable. Must equal the pin and the file. |
| `pure` | boolean | yes | Must be `true`. |
| `supportsPreview` | boolean | no | `true` if the plugin handles [`mode: "preview"`](#preview-mode) without effects outside its process. Default `false`: previews of workflows using this plugin fail with `AUTHORITY_UNAVAILABLE`. The former `purePreview` member is gone and, like any unknown member, rejected. |
| `inputSchema` | object | yes | Field name → [field schema](#field-schema), ≤ 1000 entries. |
| `resultSchema` | object | yes | Field name → field schema, ≤ 1000 entries. |
| `configurationSchema` | field schema | yes | Kind `object`; no `bytes` anywhere. |
| `configurationPolicy` | policy | yes | See [configuration policy](#configuration-policy). |
| `requiredCoreFields` | array | no | Must be empty or omitted. |
| `requiredOperations` | array | no | Must be empty or omitted. |
| `hostCapabilities` | array | no | Must be empty or omitted. |

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

#### Configuration policy

The policy mirrors `configurationSchema` node by node: the root, every object property (`properties`, exactly the same names) and every list element (`element`). Scalars have neither. Each node requires:

| Member | Values |
| --- | --- |
| `configurationScope` | `admin` or `user` |
| `secret` | `true` or `false` (explicit; omission is invalid) |

Supported today: only non-secret `admin` values. A manifest with any `"secret": true` node loads but cannot be configured or invoked (`AUTHORITY_UNAVAILABLE`). `user`-scoped properties cannot be set through an instance; declare them `optional` or the configuration fails validation. The root node must be `admin`.

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

### Preview

Workflow preview calls the instance's own plugin with `mode: "preview"`; there is no separate preview artifact or `preview` member in an instance. A plugin that does not declare `supportsPreview` is never called in preview, and preview of an `installed` step that uses it returns `AUTHORITY_UNAVAILABLE`. The plugin author is responsible for making preview effect-free; the server cannot enforce it, just as it cannot enforce `pure`.

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
