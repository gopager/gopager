# HTTP API

Request and response schemas for every route are defined in
[`api/openapi/v1.yaml`](../api/openapi/v1.yaml). This page covers authentication,
conventions and semantics that the schema cannot express.

Related: [operations](operations.md) (flags, bootstrap, deployment),
[workflows](workflows.md), [plugins](plugins.md), [architecture](architecture.md),
[development](development.md).

## Basics

| Item | Value |
| --- | --- |
| Listener | Only a process started with the `api` role (`-roles api` or `api,worker`) opens an HTTP listener, on `-listen-address`. A `worker`-only process serves no HTTP. |
| Application base path | `/api/v1` |
| Request body | `Content-Type: application/json` (parameters such as `; charset=utf-8` are allowed). Any other media type returns `400 INVALID_INPUT`. |
| Response body | `application/json`, always with `Cache-Control: no-store`. |
| Envelope version | Every JSON request body must contain `"schemaVersion": 1`; every JSON response carries `"schemaVersion": 1`. |
| Unknown request fields | Rejected with `400 INVALID_INPUT`. Trailing data after the JSON document is also rejected. Processing routes additionally reject duplicate object keys. |
| Unmatched path / method | Go `net/http` defaults: `404` / `405` with a `text/plain` body (not the JSON error envelope). |

### Platform endpoints

Served outside `/api/v1` and outside the readiness gate.

| Method | Path | Response |
| --- | --- | --- |
| GET | `/health/live` | `200 {"live": true}` |
| GET | `/health/ready` | `200 {"ready": true}` or `503 {"ready": false}` |

Other methods return `405` with `Allow: GET`. There is no separate status endpoint.

While the process is not ready or is shutting down, every application route returns
`503 AUTHORITY_UNAVAILABLE` (`retryable: true`) without touching the database.

### Request size limits

| Routes | Limit | Exceeded |
| --- | --- | --- |
| `POST /api/v1/auth/login` | 16 KiB | `413 RESOURCE_LIMIT` |
| Master-data, assignment, correction and Gateway administration routes | 64 KiB | `413 RESOURCE_LIMIT` |
| Workflow, instance and preview routes | 1 MiB | `413 RESOURCE_LIMIT` |
| `POST /api/v1/transmissions` | 1 MiB | `413 RESOURCE_LIMIT` |
| `POST /api/v1/gateway/state` | 4 KiB | `413 RESOURCE_LIMIT` |

## Authentication

Every application request is resolved to exactly one caller identity. The credential is
selected from the request as follows:

1. **`Authorization` header present** → it must be exactly one `Bearer <secret>` value.
   The secret is a 43-character unpadded base64url string (256 bits). Anything else
   (duplicate header, other scheme, malformed secret) is `401 LOGIN_REQUIRED`. A
   session cookie sent alongside is ignored; there is no fallback.
2. **Otherwise, `gopager_session` cookie present** → session authentication. A duplicate,
   empty or malformed session cookie is `401 LOGIN_REQUIRED`; it never falls back to
   anonymous.
3. **Neither** → anonymous. The request runs as the installation's guest account, which
   has no rights unless an administrator granted it some. If no guest account exists,
   the result is `401 LOGIN_REQUIRED`.

The credential, the account state and the caller's grants are re-read from the primary
database in the same transaction as the operation. Revocations take effect on the next
request.

### Sessions (browser login)

| Method | Path | Body | Success |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/login` | `{"schemaVersion":1,"username":"…","password":"…"}` | `200`, sets the session cookie |
| GET | `/api/v1/auth/session` | — | `200` current session |
| POST | `/api/v1/auth/logout` | none | `204`, expires the cookie |

Login response:

```json
{"schemaVersion":1,"sessionId":"6f0c…","csrfToken":"Qk3…","expiresAt":"2026-12-05T10:00:00.123456Z"}
```

`GET /api/v1/auth/session` returns the same fields plus `principalId`. Use it to recover
the CSRF token after a page reload. It returns `401 LOGIN_REQUIRED` if the request is not
authenticated by a valid session (including bearer-authenticated requests).

Login rules:

- The request must carry exactly one `Origin` header equal to `-public-origin`;
  otherwise `403 FORBIDDEN`. This check runs before the body is read.
- Only active human accounts with a local password and the `password.local` right can
  log in. Unknown user, wrong password, inactive account and missing right all return
  the same `401 LOGIN_REQUIRED`.
- Usernames are compared after Unicode NFC normalization and case folding. They are not
  trimmed; leading or trailing whitespace is `400 INVALID_INPUT`.
- Logout requires the `session.manage-own` right, plus the Origin and CSRF checks
  described below.

Session lifetime: 30 days after login (absolute), or 7 days after the last
authenticated use, whichever comes first.

Session cookie attributes:

| Attribute | Value |
| --- | --- |
| Name | `gopager_session` |
| Path | `/` |
| Domain | none (host-only) |
| `HttpOnly` | yes |
| `SameSite` | `Lax` |
| `Secure` | set only when `-public-origin` uses `https`. The scheme of the hop between a reverse proxy and GoPager does not matter. |
| `Expires` | the session's absolute expiry |

### Origin and CSRF policy

Session-authenticated requests with a method other than `GET`, `HEAD` or `OPTIONS` must
send:

- exactly one `Origin` header that, after normalization, equals `-public-origin`.
  Normalization lower-cases the host, converts IDNs to ASCII and drops the default port.
  `Host`, `Forwarded` and `X-Forwarded-*` headers are never consulted.
- exactly one `X-CSRF-Token` header equal to the session's `csrfToken`.

This applies to every such route, including the read-only previews
`POST /units/bulk/preview`, `POST /address-rules/preview` and
`POST /workflows/{id}/preview`.

A failed check returns `403 FORBIDDEN`. These checks do not apply to bearer-authenticated
or anonymous requests. No CORS headers are emitted.

### Login rate limiting

Login attempts are counted per minute in three buckets: per normalized username, per
client source (the host of the connection's remote address, across all usernames) and per
instance. The defaults are 10, 30 and 300. Change them with `-login-attempts-per-account`,
`-login-attempts-per-source` and `-login-attempts-per-instance`. Attempts are counted
whether or not they succeed; attempts that are rejected for exceeding a limit are not.
Behind a reverse proxy that does not preserve the client address, all clients share one
source bucket (see [Operations](operations.md#security)).
When a limit is exceeded, the response is:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 37
```
```json
{"schemaVersion":1,"code":"RATE_LIMITED","parameters":{},"textKey":"error.RATE_LIMITED",
 "fallback":"Try again later.","requestId":"…","retryable":true,"retryAfterSeconds":37}
```

### Bearer access keys

A bearer secret in `Authorization` that is not a Gateway credential is resolved as an
access key of a non-guest account. The key's restrictions (actions, scopes, fields,
maximum age) can only narrow the owning account's rights. The HTTP API has no endpoint
that issues access keys.

### Gateway authentication

`POST /api/v1/transmissions` and `POST /api/v1/gateway/state` accept only a Gateway
credential: `Authorization: Bearer <secret>`, with the same secret format as above. The
Gateway identity is derived from the credential, so a request cannot name a Gateway.
Missing, malformed, unknown or revoked credentials return `401 LOGIN_REQUIRED`.
Sessions, cookies and Origin/CSRF are not used on these routes.

Gateway credential rotation:

- `POST /api/v1/gateways/{id}/credentials` returns the secret once, in `secret`. If the
  Gateway has no current credential, the new one is `current` immediately. Otherwise it
  is `pending`, and the old credential keeps working.
- The first Gateway request authenticated with a `pending` credential promotes it to
  `current` and revokes the previous one. That promotion also increments the Gateway
  revision.
- Only one `pending` credential can exist at a time. Issuing another one returns
  `409 REVISION_CONFLICT`.
- Status reads never return secrets or hashes.

### First administrator

The HTTP API cannot create accounts. Create the first local account with
`gopager bootstrap`; see [operations](operations.md).

## Authorization

What callers can do depends on these concepts:

- **Grants** combine an action from a fixed catalog (for example `unit.read`,
  `address-rule.update`, `gateway.update`, `rule.preview`, `plugin.configure`) with a
  scope: `all`, or a list of `agency`, `unit`, `unitCollection` or `account` IDs. An Agency is
  the operating authority that owns Units and Networks (for example a county's fire dispatch
  centre); a grant on it covers everything of that Agency. A
  grant can carry a maximum data age. Grants from the account and from its user groups
  add up.
- **Field permissions** are global (unscoped) and cover the field groups `payload`,
  `address`, `unit`, `gateway`, `quality`, `derived` and `extension.<plugin>.<field>`.
  They make no rows readable on their own. Reads of Units, Organizations and Unit
  collections also require the `unit` field group. Address-rule reads require both
  `address` and `unit`.
- **Access-key restrictions** intersect with the owning account's rights.

Errors:

| Status | Meaning for the caller |
| --- | --- |
| `401 LOGIN_REQUIRED` | No valid credential, or the credential has expired or been revoked. Log in again. |
| `403 FORBIDDEN` | The caller is authenticated (or the guest), but the action is not permitted. Also returned when an Origin/CSRF check fails, when no grant covers the action at all (for example a list read without any `*.read` grant), or when a mutation touches an object outside the caller's write scope (for example a Unit's collection membership, or a Network assigned to a Gateway). |
| `404 NOT_FOUND` | The item does not exist, is deleted, or is outside the caller's read scope. These cases are indistinguishable. |

Lists are filtered in the database before counting and paging: `total` counts only rows
the caller may see. A response never includes data from objects the caller cannot read.
There is no partial redaction of fields within master-data DTOs.

Scope evaluation per resource:

| Resource | Evaluated against |
| --- | --- |
| Agency | The Agency itself; creating and deleting Agencies needs an `all` grant. |
| Unit | The Unit's Agency, the Unit, and its collections. One matching grant suffices. |
| Organization | Every member Unit. An Organization without members needs an `all` grant. |
| Unit template | Only `all` scope. Referencing an Organization also requires `organization.read` on it. |
| Address rule | The Agency of the rule's Network, and the rule's Unit with its collections. |
| Suppression / correction | The Agency of the affected transmission target's Network, and the target's Unit. |
| Gateway administration | The Agency of every Network currently assigned to the Gateway, and for network replacement of every newly proposed Network. A Gateway without Networks needs an `all` grant. |
| Workflow | The workflow's own selection scope (`all`, `network` or `unit`), with each Network resolved to its Agency and each Unit to its Agency and collections. |
| Processing instance | The global `plugin.configure` right. |

## Conventions

### Response envelope

Most successful responses wrap the result:

```json
{"schemaVersion":1,"data":{ … }}
```

These routes return an unwrapped body instead:

- `/api/v1/auth/*`
- `/api/v1/gateways/*`, for example `{"schemaVersion":1,"revision":"7"}`

`204` responses have no body.

### Errors

Every application error, except for unmatched routes, has this shape:

```json
{
  "schemaVersion": 1,
  "code": "REVISION_CONFLICT",
  "parameters": {},
  "textKey": "error.REVISION_CONFLICT",
  "fallback": "This item changed. Reload before saving.",
  "requestId": "0b6c3f8e-…",
  "retryable": true
}
```

- `code` is stable and language-independent; branch on it.
- `textKey` is `error.<code>`, for client-side localization.
- `fallback` is a redacted English message. It never contains internal causes, SQL or
  identifiers of hidden objects.
- `parameters` is currently always `{}`.
- `requestId` is a fresh UUID per error response. The server logs the same `requestId` with the
  code and HTTP status (`API error response`), so an operator can correlate a client's report with
  the log; see [Operations](operations.md#exit-status-and-logging).
- `retryAfterSeconds` appears only with `RATE_LIMITED`.

| Code | HTTP | `retryable` | Meaning |
| --- | --- | --- | --- |
| `INVALID_INPUT` | 400 | false | Malformed body, unknown field, bad UUID, bad paging parameter, invalid value, or an unresolvable Gateway Network |
| `LOGIN_REQUIRED` | 401 | false | See [Authentication](#authentication) |
| `FORBIDDEN` | 403 | false | See [Authorization](#authorization) |
| `NOT_FOUND` | 404 | false | Absent, deleted or not visible |
| `REVISION_CONFLICT` | 409 | true | Stale `expectedRevision`, conflicting rule, already active suppression, or second pending Gateway credential. Reload the current state, then retry. A database serialization failure or deadlock is retried once by the server before it is reported with this code. A duplicate-key violation also reports `REVISION_CONFLICT` but with `retryable: false`: repeating the same request meets the same conflict. |
| `IDEMPOTENCY_CONFLICT` | 409 | false | A `commandId` was reused by the same caller for a different operation or input, or a Gateway `submissionId` was reused with different content. Retrying the same request cannot succeed; send the original request again or use a new identifier. |
| `INCOMPATIBLE_VERSION` | 409 | false | Incompatible version |
| `RESOURCE_LIMIT` | 413 | false | Body or collection size limit exceeded, or revision counter exhausted |
| `RATE_LIMITED` | 429 | true | Login throttled; see `Retry-After` |
| `STORAGE_UNAVAILABLE` | 503 | true | Database unavailable |
| `AUTHORITY_UNAVAILABLE` | 503 | true | Process not ready, or a required component is unavailable |
| `COMMIT_UNCERTAIN` | 503 | false | The write may have committed. Re-send with the **same** command ID or `submissionId`, or read the resource, instead of creating a new operation. |
| `INTERNAL_ERROR` | 500 | false | Unexpected failure |

`RETRY_LIMIT` is not an error `code`. It appears only as the `failureCode` of a failed
assignment correction (`GET /assignment-corrections/{id}`) when a scope used up its five
attempts; other `failureCode` values there are the codes above. Retry such a correction with
`POST /assignment-corrections/{id}/retry`.

### Command IDs (idempotent creation)

Creating operations take a client-generated `commandId` (UUID, not nil). These are:

- `POST` on organizations, unit-templates, units, `units/from-template`, `units/bulk`,
  unit-collections, address-rules and suppressions
- `POST /assignment-corrections/{id}/retry`
- `POST` gateway credentials and `DELETE` gateway credentials
- `POST /workflows`, `POST /workflows/{id}/versions`
- `POST /instances`, `PUT /instances/{id}`

Rules:

- A receipt is stored per **(caller principal, commandId)** in the same transaction as
  the change. Receipts do not expire.
- Re-sending the same operation with the same input returns the original result,
  including the original `id`/`revision`, and makes no second change. The HTTP status is
  the route's normal success status. The exception is a replayed Gateway credential
  issue: it returns `200` instead of `201` and omits `secret`.
- The same `commandId` with a different operation or input returns
  `409 IDEMPOTENCY_CONFLICT` with `retryable: false`.
- Revision-based updates (`PUT`, most `DELETE`s) and `POST /units/bulk/preview` and
  `POST /address-rules/preview` must **not** send a `commandId`. Sending a non-nil value
  returns `400`.
- `POST /suppressions` checks the receipt before `expectedAssignmentRevision` and the
  active-suppression rule. A successful create advances the assignment revision, but
  re-sending the identical request (for example after `COMMIT_UNCERTAIN`) still returns
  the stored result and makes no second change.

Gateway submissions use `submissionId` instead; see [Transmissions](#ingest-gateway-credential).

### Revisions and optimistic concurrency

- Every mutable resource has a positive `revision`, serialized as a **decimal string**
  (`"revision":"3"`).
- Updates and deletes send `expectedRevision` (also a string) in the JSON body. There are
  no `ETag`/`If-Match` headers.
- A mismatch returns `409 REVISION_CONFLICT`. On create, `expectedRevision` must be
  absent or `"0"`.
- `DELETE` requests carry a JSON body: `{"schemaVersion":1,"expectedRevision":"N"}`,
  plus `commandId` for Gateway credentials.
- Suppressions use the global assignment revision as well:
  - Create sends `expectedAssignmentRevision`.
  - Lift (`DELETE`) sends both `expectedRevision` and `expectedAssignmentRevision`.
  - The current assignment revision is returned as `assignmentRevision` by suppression
    commands and correction reads.
- The Gateway revision covers Network assignments and credentials. It also changes when
  a pending credential is promoted.

### Pagination

List routes use offset paging with query parameters only:

| Parameter | Default | Range |
| --- | --- | --- |
| `offset` | 0 | ≥ 0 |
| `limit` | 50 | 1–200 |

Any other query parameter, a repeated parameter or a non-integer value is
`400 INVALID_INPUT`. Responses have the shape `{"items":[…],"total":N}`, where `total` is
the caller-visible count. There are no cursors.

### Time, IDs and numbers

- Timestamps are RFC 3339 strings in UTC. Responses can include fractional seconds.
- Gateway `observationTime` accepts any RFC 3339 offset. Fractional seconds are
  truncated to whole seconds, not rounded. The truncated value is used for comparison
  and idempotency.
- All resource IDs are UUIDs, generated by the server except for `commandId`. A nil UUID
  (`00000000-…`) is always invalid.
- Revisions are decimal strings (see above). Other integers, such as `targetPosition`
  and counts, are JSON numbers.

## Route reference

All paths are relative to `/api/v1`. "Right" is the grant action that is checked. All
mutating routes reached via session also require Origin + `X-CSRF-Token`.

### Auth

| Method | Path | Right | Success | Purpose |
| --- | --- | --- | --- | --- |
| POST | `/auth/login` | `password.local` | 200 | Password login; sets cookie |
| GET | `/auth/session` | valid session | 200 | Current session and CSRF token |
| POST | `/auth/logout` | `session.manage-own` | 204 | Revoke current session |

### Agencies, Organizations, Unit templates, Units, collections

| Method | Path | Right | Success | Purpose |
| --- | --- | --- | --- | --- |
| GET | `/agencies` | `agency.read` | 200 | List |
| POST | `/agencies` | `agency.create` (`all`) | 201 | Create (`commandId`, `name`) |
| GET | `/agencies/{id}` | `agency.read` | 200 | Read |
| PUT | `/agencies/{id}` | `agency.update` | 200 | Rename |
| DELETE | `/agencies/{id}` | `agency.delete` (`all`) | 200 | Delete; `409 REVISION_CONFLICT` while Units or Networks still belong to it |
| GET | `/organizations` | `organization.read` | 200 | List |
| POST | `/organizations` | `organization.create` | 201 | Create (`commandId`, `name`, `defaults`) |
| GET | `/organizations/{id}` | `organization.read` | 200 | Read |
| PUT | `/organizations/{id}` | `organization.update` | 200 | Replace name and defaults |
| DELETE | `/organizations/{id}` | `organization.delete` | 200 | Delete; members and templates keep their effective values |
| GET | `/unit-templates` | `unit-template.read` | 200 | List |
| POST | `/unit-templates` | `unit-template.create` | 201 | Create |
| GET | `/unit-templates/{id}` | `unit-template.read` | 200 | Read |
| PUT | `/unit-templates/{id}` | `unit-template.update` | 200 | Replace; `organizationId` is required (may be `null`) |
| DELETE | `/unit-templates/{id}` | `unit-template.delete` | 200 | Delete |
| GET | `/units` | `unit.read` | 200 | List |
| POST | `/units` | `unit.create` (+ `organization.create` with `newOrganization`) | 201 | Create; `agencyId` required; optionally creates an Organization in the same transaction |
| POST | `/units/from-template` | `unit.create`, `unit-template.read` | 201 | Create from a snapshot of the template's current values |
| GET | `/units/{id}` | `unit.read` | 200 | Read |
| PUT | `/units/{id}` | `unit.update` | 200 | Full replacement; `agencyId` and `organizationId` required (the latter may be `null`); moving a Unit to another Agency needs `unit.update` on both; `newOrganization` not allowed |
| DELETE | `/units/{id}` | `unit.delete` | **202** | Unit becomes non-selectable; `data.status` is `"pending"` while assignment correction runs |
| POST | `/units/bulk/preview` | `unit.update` / `unit.delete` | 200 | Dry run with per-item `valid`/`code`; no `commandId` |
| POST | `/units/bulk` | `unit.update` / `unit.delete` | 200 | 1–100 items, all or nothing; per-item `status` `updated` or `pending` |
| GET | `/unit-collections` | `unit-collection.read` | 200 | List |
| POST | `/unit-collections` | `unit-collection.create` | 201 | Create (`commandId`, `name`) |
| GET | `/unit-collections/{id}` | `unit-collection.read` | 200 | Read |
| PUT | `/unit-collections/{id}` | `unit-collection.update` | 200 | Rename |
| DELETE | `/unit-collections/{id}` | `unit-collection.delete` | 200 | Delete |

Changing a Unit's `collectionIds` also requires `unit-collection.update` on every
collection that is added or removed. Settings (`color`, `icon`, `functionMeanings`) take
the form `{"mode":"inherit"|"unset"}` or `{"mode":"value","value":…}`.
`effectiveSettings` reports the resolved value and its `source` (`unit`, `organization`
or `unset`). Create and update commands return `{"id","revision"}`.

### Address rules, suppressions, corrections

| Method | Path | Right | Success | Purpose |
| --- | --- | --- | --- | --- |
| GET | `/address-rules` | `address-rule.read` | 200 | List |
| POST | `/address-rules` | `address-rule.create` | 201 | Create rule (Network, Unit, protocol, selector, validity, enabled) |
| GET | `/address-rules/{id}` | `address-rule.read` | 200 | Read |
| PUT | `/address-rules/{id}` | `address-rule.update` | 200 | Full replacement |
| DELETE | `/address-rules/{id}` | `address-rule.delete` | **202** | Delete; `data.status` is `"pending"` |
| POST | `/address-rules/preview` | `address-rule.create` / `.update` | 200 | Validate without saving; `{"valid":true}` or an error. For an update, send `id` and `expectedRevision`. |
| GET | `/suppressions` | `suppression.read` | 200 | List |
| POST | `/suppressions` | `suppression.create` | **202** | Suppress one transmission target (`transmissionId`, `targetPosition`) |
| GET | `/suppressions/{id}` | `suppression.read` | 200 | Read |
| DELETE | `/suppressions/{id}` | `suppression.delete` | **202** | Lift a suppression |
| GET | `/assignment-corrections/{id}` | `assignment-correction.read` | 200 | Progress: `state` (`pending`, `running`, `completed`, `failed`, `superseded`), counts |
| POST | `/assignment-corrections/{id}/retry` | `assignment-correction.retry` | **202** | Reschedule a failed correction (`commandId`) |

On create and update, `selector`, `enabled` and `protocol` are required, and `id` must
not be sent. Overlapping rules return `409 REVISION_CONFLICT`. Rule, Unit and suppression
changes re-evaluate stored transmissions asynchronously. The `202` responses return the
`correctionId` (suppressions) or `status: "pending"`. Follow progress with
`GET /assignment-corrections/{id}`.

### Ingest, Gateway credential

| Method | Path | Auth | Success | Purpose |
| --- | --- | --- | --- | --- |
| POST | `/transmissions` | Gateway bearer | 202 / 200 | Submit one transmission |
| POST | `/gateway/state` | Gateway bearer | 204 | Report `heartbeat`, `degraded`, `recovered` or `terminated` |

Submission:

```json
{
  "schemaVersion": 1,
  "submissionId": "rx-2026-11-05-000123",
  "protocol": "pocsag",
  "observationTime": "2026-11-05T09:30:12Z",
  "gatewayNetworkIdentifier": "north",
  "payload": {"form": "text", "value": "FIRE ALARM"},
  "transmissionTargets": [{"address": 1234567, "function": 3}],
  "qualityMetrics": {"rssi": -71},
  "incomplete": false
}
```

Field rules:

- `protocol` is one of `pocsag`, `flex`, `zvei`, `eas`. The payload `form` must be valid
  for that protocol:
  - `text`, `numeric`, `tone-only`: POCSAG and FLEX
  - `binary`: FLEX, with `subtype` and base64 `data`
  - `flex-short-signal`
  - `zvei-signal`
  - `eas-header`
- `submissionId` is 1–128 bytes of UTF-8, without leading or trailing whitespace.
- `transmissionTargets` holds up to 1024 targets. `address` is a JSON number or string,
  depending on the protocol.
- The optional fields `gatewayNetworkIdentifier`, `qualityMetrics` and `incomplete` must
  be omitted rather than set to `null`.
- Network resolution uses only the Networks assigned to the Gateway, in this order:
  1. the declared `gatewayNetworkIdentifier`
  2. the only Network for this protocol
  3. the only Network with a matching address rule
  4. the protocol's default Network
  
  If none applies, the result is `400 INVALID_INPUT`.
- Idempotency is keyed on (Gateway, `submissionId`). An identical re-submission returns
  the stored receipt. Different content under the same ID returns
  `409 IDEMPOTENCY_CONFLICT`. Rejected submissions (`400`, `409`, `413`) create no
  transmission.

Receipt (`data`): `submissionId`, `transmissionId`, `acceptanceTime`,
`processingStatus`, `messageId`. `processingStatus` is `"none"` when no Workflow was
selected, `"pending"` while Workflow candidates are open, `"completed"` or `"failed"`.
The HTTP status is `202` for `"pending"` and `200` otherwise. Intake has committed in
every case; the accepted evidence is visible to readers while Processing runs.

Gateway state: `{"schemaVersion":1,"operation":"degraded","reasonCode":"LOW_SIGNAL","detail":"…"}`.
`reasonCode` is required for `degraded` and only allowed there: printable ASCII,
1–64 bytes. `detail` is optional, up to 512 bytes. Other operations take no extra fields.

### Gateway administration

| Method | Path | Right | Success | Purpose |
| --- | --- | --- | --- | --- |
| PUT | `/gateways/{id}/networks` | `gateway.update` | 200 | Replace the full Network assignment set |
| GET | `/gateways/{id}/credentials` | `gateway.read` | 200 | Credential IDs, `status` (`current`, `pending`, `revoked`) and timestamps |
| POST | `/gateways/{id}/credentials` | `gateway.update` | 201 (replay: 200) | Issue a credential; the secret is returned once |
| DELETE | `/gateways/{id}/credentials/{credentialId}` | `gateway.update` | 200 | Revoke |

Network replacement body:

```json
{"schemaVersion":1,"expectedRevision":"4","networks":[
  {"gatewayNetworkIdentifier":"north","networkId":"…","default":true}]}
```

- 1–1024 entries. Identifiers and Network IDs must be unique.
- Each protocol in the set needs exactly one `default`.
- The responses are `{"schemaVersion":1,"revision":"N"}` and, for issuing a credential,
  `{"schemaVersion":1,"credentialId","secret","status","revision"}`.

The HTTP API has no routes that create, list or delete Gateways or Networks.

### Processing: workflows, versions, candidates

| Method | Path | Right | Success | Purpose |
| --- | --- | --- | --- | --- |
| GET | `/workflows` | `rule.read` | 200 | List |
| POST | `/workflows` | `rule.create` | 201 | Create (`commandId`, `name`, `scope`, `enabled`) |
| GET | `/workflows/{id}` | `rule.read` | 200 | Read; includes `activeVersionId` |
| PUT | `/workflows/{id}` | `rule.update` | 200 | Update metadata |
| DELETE | `/workflows/{id}` | `rule.delete` | 200 | Delete |
| GET | `/workflows/{id}/versions` | `rule.read` | 200 | List saved definitions |
| POST | `/workflows/{id}/versions` | `rule.update` | 201 | Save an immutable definition (`commandId`, `expectedRevision` of the workflow, `definition`); does not activate it |
| GET | `/workflows/{id}/versions/{version}` | `rule.read` | 200 | Read a definition |
| POST | `/workflows/{id}/versions/{version}/activate` | `rule.activate` | 200 | Activate (`expectedRevision`) |
| POST | `/workflows/{id}/preview` | `rule.preview` | 200 | Evaluate a definition without publishing |
| GET | `/workflows/{id}/candidates` | `rule.read` | 200 | Processing candidate states |
| GET | `/workflows/{id}/candidates/{candidate}` | `rule.read` | 200 | One candidate: `state`, `selected`, `failureCode` |
| GET | `/workflows/{id}/candidates/{candidate}/result` | `rule.read` | 200 | Result metadata only (`id`, `revision`, `completedAt`) |
| POST | `/workflows/{id}/candidates/{candidate}/retry` | `rule.reprocess` | 200 | Retry a failed candidate (`expectedRevision`) |

Workflow `scope.kind` is `all`, `network` or `unit`. Creating, updating and deleting a
workflow, saving a version and activating one all advance the workflow revision.

Preview body:

- exactly one of `versionId` (a saved version) or `definition` (inline)
- exactly one of `syntheticJson` (raw input document) or `storedTransmissionId`

The result is `{"selected","fields","targetResults"}`, restricted to fields the caller
may read. A `syntheticJson` input is evaluated against the current network named by its
`networkId`: an unknown network returns `404 NOT_FOUND`, and a deleted network or one
whose protocol differs from the input returns `503 AUTHORITY_UNAVAILABLE`.
`storedTransmissionId` is not supported and returns `503 AUTHORITY_UNAVAILABLE`.

Candidate retry requeues a `failed`, unpublished candidate and returns
`{"candidateId","revision"}` in `data`. If the candidate's source, workflow version or
revision, or the processing configuration changed since it was admitted, the retry
returns `409 REVISION_CONFLICT`. Details: [workflows](workflows.md#retries).

Definition format: [workflows](workflows.md).

### Processing: plugin instances

| Method | Path | Right | Success | Purpose |
| --- | --- | --- | --- | --- |
| GET | `/instances` | `plugin.configure` (all) | 200 | List instance summaries |
| GET | `/instances/{id}` | `plugin.configure` (all) | 200 | Read; configuration redacted according to the plugin manifest |
| POST | `/instances` | `plugin.configure` (all) | 201 | Create (`commandId`, `enabled`, `executionPrincipalId`, `main`, optional `preview`, `configuration`) |
| PUT | `/instances/{id}` | `plugin.configure` (all) | 200 | Update; requires both `commandId` and `expectedRevision` (> 0) |

Artifacts are resolved under `-processing-artifact-root`. Without that directory,
instance inspection returns `503 AUTHORITY_UNAVAILABLE`. Plugin and manifest details:
[plugins](plugins.md).
