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
   the result is `401 LOGIN_REQUIRED`. Guests are read-only: they may view and filter, and
   an anonymous request with any method except `GET`, `HEAD` and `OPTIONS` is
   `401 LOGIN_REQUIRED` before the body is read, so a guest can never create or change
   anything, whatever grants the guest account holds.

On every authenticated route (master data, assignments, corrections, Gateway administration,
Processing) a session or bearer credential is checked before the request body is read: an
unknown, revoked, expired or inactive-account credential is `401 LOGIN_REQUIRED` without the
body being consumed. Anonymous `GET`, `HEAD` and `OPTIONS` requests have no credential to check and proceed to the
operation. Expiry is uniform: every service, login-adjacent path and the early check reject a
session or key whose expiry has passed with `401 LOGIN_REQUIRED`. This early check is advisory; the credential, the account state and the caller's
grants are re-read from the primary database in the same transaction as the operation.
Revocations take effect on the next request.

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
Missing, malformed, unknown or revoked credentials, and credentials of a disabled or deleted
Gateway, return `401 LOGIN_REQUIRED`.
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
  `address-rule.update`, `gateway.update`, `workflow.preview`, `plugin.configure`) with a
  scope: `all`, or a list of `agency`, `unit`, `unitCollection`, `account` or `gateway` IDs. An Agency is
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

Scope kinds each action accepts (`internal/authz/grants.go` is the source; a grant with another kind is invalid):

| Actions | Scope kinds |
| --- | --- |
| `message.read`, `message.detail` | `all`, `agency`, `unit`, `unitCollection` |
| `without-unit.read` | `all`, `agency` |
| `unit.*`, `organization.*`, `unit-collection.*`, `address-rule.*`, `suppression.*`, `assignment-correction.read`, `assignment-correction.retry` | `all`, `agency`, `unit`, `unitCollection` |
| `unit-template.*`, `agency.create`, `agency.delete`, `grouping.configure`, `notice.manage`, `plugin.configure`, `operations.manage` | `all` only |
| `agency.read`, `agency.update`, `gateway.*`, `network.*` | `all`, `agency` |
| `account.*`, `grant.manage`, `password.local`, `recovery.*`, `session.manage-own`, `key.*`, `subscription.manage-*` | `all`, `account` |
| `gateway.diagnostics.read`, `gateway.credential.rotate` | `all`, `agency`, `gateway` |
| `delivery.read`, `delivery.retry`, `delivery.redeliver` | `all`, `agency`, `unit`, `unitCollection`, `account` |
| `workflow.*`, `presentation.manage` | `all`, `agency`, `unit` |
| `audit.read` | `all`, `agency`, `account` |

Grant semantics:

- Grants from the account and its groups are additive; read and write scopes are separate grants. Actions are an explicit catalog (see [Operations](operations.md#first-start-and-bootstrap)); there are no wildcards and no implicit administrator. A grant whose scope object was deleted stops applying and never becomes unrestricted. A new account and the guest start with no rights.
- A restricting access key lists actions, scopes, fields and an optional shorter maximum age; the effective right is the owner's rights intersected with the key, and the key can never add a right.
- The maximum data age is optional (absent means unlimited). The largest permitted age among matching grants wins; a key's age is intersected. It is evaluated against the `message_units.age_anchor` of the Unit appearance within the permitted scope, inclusively (`now - anchor <= maxAgeSeconds`), identically for lists, details and Delivery. A later repeat for the same Unit does not renew eligibility.
- Authorization has two independent steps: current action, scope and age rights first establish which rows are eligible, then the global field permissions decide which fields of those rows are projected. A field permission never makes a row readable, and `message.detail` additionally needs `message.read`. Hidden co-targets need their own rights.
- Changing global field permissions needs installation-wide `grant.manage` plus the delegable field groups; a scoped `grant.manage` cannot create global field access, and delegation never exceeds the actor's own delegable rights.
- Passwords are Argon2id hashes (19 MiB, 2 iterations, parallelism 1, random salt, parameters encoded in the hash), 12–1024 characters, valid UTF-8, with no truncation or composition rules. Session, key and gateway secrets are 256-bit random values stored only as hashes and shown once.

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
| Address rule | Writes need authority over the Agency of the rule's Network and, additionally, over the rule's Unit or its collections (for an update, also over the existing rule's Network Agency); a Unit or collection grant alone never reaches another Agency's Network. Reads keep their existing predicate. |
| Suppression / correction | The Agency of the affected transmission target's Network, and the target's Unit. A `gateway` correction needs every Agency of the Gateway's current Networks of its Protocol and of the Networks its Transmissions reference (none: an `all` grant). |
| Network | The Network's Agency. Protocol and Agency are fixed at creation. |
| Gateway administration | The Agency of every Network currently assigned to the Gateway, and for creation and network replacement of every proposed Network. A Gateway without Networks needs an `all` grant. Lists show only Gateways whose every Agency is readable. |
| Gateway delegation | `gateway.diagnostics.read` and `gateway.credential.rotate` with a `gateway` scope reach exactly the listed Gateways; with `agency` scope they need every Agency of the Gateway. Diagnostics grant only the diagnostics and history routes; rotation grants only credential status, issue and revoke. Neither implies the other, any Message or Payload access, or Gateway administration. A grant on a deleted Gateway stops applying. |
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
- `POST /suppressions` checks the receipt before the
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
- Suppressions need no assignment revision token: a suppression concerns exactly one target,
  so a rule change elsewhere never invalidates the command.
  - Create sends `transmissionId` and `targetPosition`.
  - Lift (`DELETE`) sends the suppression's `expectedRevision`.
  - The assignment revision a correction was recorded at is returned as `assignmentRevision`
    by suppression commands and correction reads.
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
| PUT | `/units/{id}` | `unit.update` | 200 | Full replacement; `agencyId` and `organizationId` required (the latter may be `null`); moving a Unit to another Agency needs `unit.update` with authority over both Agencies (a grant on the Unit alone does not suffice) and takes the Unit's Message history along; `newOrganization` not allowed |
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

#### Master data semantics

- **Units** have their own identity, belong to exactly one Agency, may belong to several collections and are not subordinate to a Network. Names (Unit, Organization, template, collection, Agency) are 1–256 Unicode scalars, valid UTF-8, not blank and not trimmed; names are not identity keys.
- **Settings** cover `color` (`#RRGGBB`, stored upper case), `icon` (1–128 ASCII letters, digits, `.`, `_`, `-`; the server stores the identifier and has no icon catalog) and `functionMeanings` (POCSAG function `0..3` mapped to a label of 1–256 scalars). A Unit setting is `inherit`, `value` or `unset`; `inherit` resolves to the Organization's current default, or to `unset` without an Organization. A null effective value means unset, never a fabricated default. An Organization change reaches only Units that still inherit that setting. Color and icon changes never cause correction work.
- **Templates** are standalone: creating a Unit from a template copies the template's current Organization reference and effective settings in one transaction, requires an explicit Unit name and collection list, and leaves no link, so later template or Organization edits do not change the Unit.
- **Organization deletion** requires `organization.delete` plus `unit.update` for every Unit and `unit-template.update` for every template it detaches, checked against the complete set before the change. In one transaction it materializes each still-inherited effective value on the Unit or template as `value` or `unset`, detaches them, keeps explicit overrides and deletes the Organization. A stale revision of any affected Unit or template rolls back everything; an unmanageable reference returns a generic `FORBIDDEN` or `REVISION_CONFLICT` without foreign IDs. No correction work is scheduled.
- **Unit deletion** marks the Unit non-selectable, disables its rules and records correction work atomically; the response is `pending` until the correction ran. Current grants on the ID stop applying. Unit updates carry the complete supported field set and collection list, so they cannot erase fields owned by later features.
- **Bulk** operations take 1–100 distinct Unit IDs, each with its `expectedRevision`. Preview is read-only and reveals only caller-safe per-item results. Execution repeats all checks under ordered row locks and is all or nothing: one stale, unauthorized, invalid, duplicate or over-limit item aborts every item. Bulk update changes only the explicitly supported shared settings (not name, Organization, membership, templates or rules); bulk delete behaves like single Unit deletion. One compact receipt covers the command; an identical retry returns it.
- **Rights.** Template operations need explicit `unit-template.*` rights with `all` scope. Creating a Unit from a template needs `unit.create` and `unit-template.read` (plus `organization.read` for a referenced Organization); an inline new Organization needs `organization.create` and `unit.create`. An Organization without members needs `all` scope. References (Organization, template) never grant access to anything.

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
| GET | `/assignment-corrections/{id}` | `assignment-correction.read` | 200 | Progress: `state` (`pending`, `running`, `completed`, `failed`, `superseded`), `scope` (kind `address-rules`, `unit`, `target` or `gateway`), the processed/remaining Transmission counts and, for `gateway`, `unresolved` |
| POST | `/assignment-corrections/{id}/retry` | `assignment-correction.retry` | **202** | Reschedule a failed correction (`commandId`) |

On create and update, `selector`, `enabled` and `protocol` are required, and `id` must
not be sent. Overlapping rules return `409 REVISION_CONFLICT`. Rule, Unit and suppression
changes re-evaluate stored transmissions asynchronously. A rule change on a Network that shares a
Gateway with other Networks of its Protocol also re-runs the Network inference of that Gateway's
undeclared Transmissions with targets, like a mapping replacement (see
[Gateway administration](#gateway-administration)): such a Transmission can move to another
Network and, with it, to another Agency's visibility. The `202` responses return the
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
- Network resolution uses only the Networks assigned to the Gateway that serve the
  submission's protocol, in this order:
  1. the declared `gatewayNetworkIdentifier` (an unknown identifier is `400 INVALID_INPUT`)
  2. the only Network for this protocol
  3. the only Network with an address rule matching the targets at the observation time
  4. the highest-priority Network among several that match; the intake fact
     `operation.network-ambiguous` is appended
  5. when none matches, including a submission without targets, the highest-priority Network
     for this protocol

  A Gateway without a Network for the protocol gets `400 INVALID_INPUT`. The stored resolution is
  `declared`, `inferred-single`, `inferred-address`, `address-priority` or `priority`.
- `observationTime` is mandatory and must carry an offset or `Z`; the server never
  substitutes the acceptance time. Fractional seconds are truncated (see
  [Time, IDs and numbers](#time-ids-and-numbers)); the supplied offset is stored with the
  truncated instant. Times in the past are accepted without limit. A time more than 15
  minutes after the acceptance time (a fixed tolerance, not configurable) is still
  accepted and grouped normally, but the intake fact `operation.observation-time-implausible`
  is appended and a warning is logged.
- Idempotency is keyed on (Gateway, `submissionId`). The submitted content is compared after
  truncating `observationTime`, so a retry that differs only in fractional digits returns the
  stored receipt, as does any identical re-submission. A different truncated instant, a
  different offset or any other different content under the same ID returns
  `409 IDEMPOTENCY_CONFLICT` and never changes the accepted transmission. Rejected submissions
  (`400`, `409`, `413`) create no transmission; a rejection is recorded as the intake fact
  `operation.submission-rejected` (Gateway, `submissionId`, reason code, no payload) in its own
  transaction.
- The submission is a finalized Gateway report, not raw radio data. The server keeps the
  payload and the ordered targets exactly as submitted, including a partial reception with
  `incomplete: true`; it never merges transmissions, interprets payloads or maps addresses to
  Units on the Gateway's behalf. Quality metrics are optional, already evaluated by the
  Gateway and stored without interpretation.

#### Protocols, targets and payload identity

Targets (`address`, `function`) are validated per protocol and never rewritten; order, duplicates
and positions are preserved.

| Protocol | Address | Function | Payload forms |
| --- | --- | --- | --- |
| `pocsag` | JSON integer 0–2097151 (RIC) | required, 0–3; the Address rule decides whether it is a function bit or a subaddress | `text`, `numeric`, `tone-only` |
| `flex` | JSON integer in a supported address class, not CAPCODE notation | forbidden | `text`, `numeric`, `tone-only`, `binary`, `flex-short-signal` |
| `zvei` | string of exactly 5 digits; leading zeros kept | forbidden | `zvei-signal`; at least one target |
| `eas` | string of exactly 6 digits (`PSSCCC` location code); leading zeros kept, no geographic expansion | forbidden | `eas-header`; 1–31 targets |

FLEX addresses must lie in one of the classes of ARIB RCR STD-43A Appendix A, Table A-1 that
can address a message: short `1..1933312`, information service `2009088..2025471`, temporary
`2029568..2029583`, operator messaging `2029584..2029599`, long `2101249..3923326750` (split
into the sub-ranges 1-2, 1-3, 1-4 and 2-3) and long information service `4280000001..4291000000`.
Network-control, unused, illegal and reserved addresses are rejected. Address-rule ranges must
consist of supported addresses only (a range may not span a gap). A temporary-group call is
submitted with the known member addresses as targets, or with the group address if the Gateway
does not know the members. The server never asserts group completeness.

Payload forms and their comparison identity. Two transmissions are payload-identical only when
their form (and subtype) and canonical value are exactly equal, whatever the protocol;
hashes never replace the equality check. Identity ignores Gateway, Network, address, function,
quality metrics and the `incomplete` marker. The stored payload is always the unchanged
original.

| Form | Submitted | Canonical identity |
| --- | --- | --- |
| `text` | Unicode string; protocol terminators, padding and character-set decoding (for example the German DIN 66003 variant) are the Gateway's job | CRLF and CR become LF, leading and trailing whitespace is removed, then Unicode NFC; no case folding, no compatibility normalization |
| `numeric` | the 4-bit code sequence as `0-9`, `.` (0xA), `U` (0xB), space (0xC), `-` (0xD), `)` (0xE), `(` (0xF); other characters are invalid | the string with leading and trailing spaces removed; never equal to a `text` payload |
| `tone-only` | no payload (`value` omitted) | the empty value; an address-only POCSAG call and a FLEX tone-only call are identical |
| `binary` (FLEX) | `subtype` `transparent` or `secure`, `encoding` `base64`, `data` | subtype plus the exact bytes; never shown as text, never decrypted |
| `flex-short-signal` (FLEX) | `sourceCode` 0–7 | the source code; matches neither `tone-only`, `numeric` nor `text` |
| `zvei-signal` | `double_tone`: technical tone name or `null` for no double tone (`"none"` is an ordinary name) | the name or `null` |
| `eas-header` | `originator` and `eventCode` (3 characters `A-Z0-9`), `purgeTime` (`TTTT`, minutes 00–59), `issueTime` (`JJJHHMM`, day 1–366) | the four values; the station identifier and the location areas (targets) are not identity, so a rebroadcast by another station joins the original Message |

Short numeric FLEX messages map to `numeric`; short-signal source codes `01` and `10` map to
`flex-short-signal`. Pure network-control data stays with the Gateway and is not submitted.
Audio and video payloads are not supported. A shorter or different canonical value never joins a
fuller one by similarity; only exact equality groups.

Receipt (`data`): `submissionId`, `transmissionId`, `acceptanceTime`,
`processingStatus`, `messageId`. `processingStatus` is `"none"` when no Workflow was
selected, `"pending"` while Workflow candidates are open, `"completed"` or `"failed"`.
The HTTP status is `202` for `"pending"` and `200` otherwise. Intake has committed in
every case; the accepted evidence is visible to readers while Processing runs.

Gateway state: `{"schemaVersion":1,"operation":"degraded","reasonCode":"LOW_SIGNAL","detail":"…"}`.
`reasonCode` is required for `degraded` and only allowed there: printable ASCII,
1–64 bytes. `detail` is optional, up to 512 bytes. Other operations take no extra fields.

Gateway state semantics:

- The states are `never-seen`, `active`, `degraded`, `stopped` and `contact-lost`. A caller
  can report only `heartbeat`, `degraded`, `recovered` and `terminated`; `never-seen`,
  `active` and `contact-lost` are server-derived. A new Gateway is `never-seen` until its
  first authenticated contact.
- Every authenticated Submission or report is contact. A `heartbeat` does not clear
  `degraded`. `recovered` sets `active` (idempotent). `terminated` sets `stopped`. An
  authenticated Submission or heartbeat from a `never-seen`, `stopped` or `contact-lost`
  Gateway makes it `active` without inventing a cause.
- Contact loss: the worker role sets an enabled `active` or `degraded` Gateway to
  `contact-lost` when its last contact is at or before `now - contact timeout` on the
  database clock (see [operations](operations.md#configuration-reference)). `contact-lost`
  takes precedence over `degraded`. `stopped`, `never-seen` and disabled Gateways are never
  set `contact-lost`; no crash cause is claimed.
- Every effective state change, whatever its cause, records one history entry and one
  `gateway.state-changed` fact with the new Gateway revision in the same transaction. A
  contact-only heartbeat or a repeated report records nothing. The `degraded` fields are
  diagnostic only and drive no authorization or automation.

### Gateway administration

| Method | Path | Right | Success | Purpose |
| --- | --- | --- | --- | --- |
| GET | `/networks` | `network.read` | 200 | List Networks |
| POST | `/networks` | `network.create` | 201 | Create a Network (`name`, `protocol`, `agencyId`) |
| GET | `/networks/{id}` | `network.read` | 200 | Read |
| PUT | `/networks/{id}` | `network.update` | 200 | Rename |
| DELETE | `/networks/{id}` | `network.delete` | 200 | Soft delete |
| GET | `/gateways` | `gateway.read` | 200 | List Gateways |
| POST | `/gateways` | `gateway.create` | 201 | Create a Gateway (`name`, optional initial `networks`) |
| GET | `/gateways/{id}` | `gateway.read` | 200 | Configuration, state and contact times |
| PUT | `/gateways/{id}` | `gateway.update` | 200 | Change `name` and `enabled` |
| DELETE | `/gateways/{id}` | `gateway.delete` | 200 | Soft delete |
| GET | `/gateways/{id}/diagnostics` | `gateway.read` or `gateway.diagnostics.read` | 200 | State, contact times, last report, history retention marker |
| GET | `/gateways/{id}/history` | `gateway.read` or `gateway.diagnostics.read` | 200 | State transitions, newest first |
| PUT | `/gateways/{id}/networks` | `gateway.update` | 200 | Replace the full Network assignment set |
| GET | `/gateways/{id}/credentials` | `gateway.read` or `gateway.credential.rotate` | 200 | Credential IDs, `status` (`current`, `pending`, `revoked`) and timestamps |
| POST | `/gateways/{id}/credentials` | `gateway.update` or `gateway.credential.rotate` | 201 (replay: 200) | Issue a credential; the secret is returned once |
| DELETE | `/gateways/{id}/credentials/{credentialId}` | `gateway.update` or `gateway.credential.rotate` | 200 | Revoke |

The Network and Gateway lifecycle routes and the diagnostics and history reads use the
management data envelope, `commandId` on creation and `expectedRevision` on change, like
master data. The assignment and credential routes return unwrapped bodies.

Network and Gateway rules:

- A Network's Protocol and Agency cannot change; another Protocol or Agency is a new Network.
  Deleting a Network is a soft delete: accepted Transmissions keep their reference, reads
  return 404 and it cannot be assigned. A Network that is still assigned to a Gateway or
  referenced by an Address rule or an undeleted Workflow cannot be deleted
  (`409 REVISION_CONFLICT`); the error names no reference.
- A new Gateway is enabled, `never-seen` and has no credential; issue one with the credential
  route. Its initial `networks` follow the replacement rules below but may be empty.
- `enabled: false` blocks the Gateway: its credentials authenticate nothing and it is not
  evaluated for contact loss. Enabling it again restores its unrevoked credentials.
- Deleting a Gateway revokes every unrevoked credential, removes its Network assignments and
  hides it. Accepted Transmissions and its state history are unchanged.
- The Gateway revision also advances on state transitions and credential cutover, so a
  change may need a reload (`409 REVISION_CONFLICT`).
- Not found versus forbidden: a caller holding no right on an existing Network or Gateway
  (no `network.*` or `gateway.*` action covering it, including the delegation actions) gets
  `404 NOT_FOUND`, exactly as for a missing one, so IDs cannot be probed. A caller holding
  some right on it but not the requested one gets `403 FORBIDDEN`. A Gateway right counts only
  if it covers every Agency of the Gateway's Networks or names the Gateway. This applies to
  all Gateway and Network routes, including assignment and credential routes.
- Diagnostics: `lastContactAt` null means never seen. `lastTransmissionAt` stays after the
  Transmission itself is removed. `historyRemovedBefore` is set when retention removed older
  transitions, so a short history is not proof that nothing happened. History entries carry
  `revision`, `oldState`, `newState`, `cause` (`report`, `submission`, `timeout`),
  `occurredAt`, `contactAt` (for `timeout` the last contact) and the degraded
  `reasonCode`/`detail`.

Network replacement body:

```json
{"schemaVersion":1,"expectedRevision":"4","networks":[
  {"gatewayNetworkIdentifier":"north","networkId":"…","priority":0}]}
```

- 1–1024 entries. Identifiers and Network IDs must be unique.
- Every entry has an explicit `priority`. Across the set the priorities must be exactly
  `0..n-1`: unique, contiguous, in any request order. `0` is the highest priority. A duplicate,
  a gap or a negative value is `400 INVALID_INPUT`. Priority decides between Networks of one
  protocol at intake (see the submission resolution order above); two districts that share
  one alarm channel are two Networks of the Gateway, one per Agency.
- The replacement response is `{"schemaVersion":1,"revision":"N","correctionIds":[…]}`; issuing a
  credential returns `{"schemaVersion":1,"credentialId","secret","status","revision"}` and revoking
  `{"schemaVersion":1,"revision":"N"}`.
- Effect on accepted Transmissions: for every Protocol whose candidates (identifiers and Networks in
  priority order) changed, the replacement queues one `gateway` correction, listed in
  `correctionIds`; an unchanged Protocol queues none. The correction re-resolves every Transmission
  of that Gateway and Protocol like intake does (a rule change queues the same correction for the
  Gateways that share its Network, limited to undeclared Transmissions with targets), with its original declaration, the current
  mapping and the current Address rules at its Observation time. A Transmission whose Network
  changes moves with its targets: Unit resolution, query visibility (the cache's Agency) and
  Processing follow the new Network. One the current mapping cannot resolve (declared identifier
  no longer mapped, no Network of its Protocol left) keeps its Network and is counted as
  `unresolved` on the correction. Receipts, originals, Payload and Message membership never change.
  Follow progress with `GET /assignment-corrections/{id}`.


### Processing: workflows, versions, candidates

| Method | Path | Right | Success | Purpose |
| --- | --- | --- | --- | --- |
| GET | `/workflows` | `workflow.read` | 200 | List |
| POST | `/workflows` | `workflow.create` | 201 | Create (`commandId`, `name`, `scope`, `enabled`) |
| GET | `/workflows/{id}` | `workflow.read` | 200 | Read; includes `activeVersionId` |
| PUT | `/workflows/{id}` | `workflow.update` | 200 | Update metadata |
| DELETE | `/workflows/{id}` | `workflow.delete` | 200 | Delete |
| GET | `/workflows/{id}/versions` | `workflow.read` | 200 | List saved definitions |
| POST | `/workflows/{id}/versions` | `workflow.update` | 201 | Save an immutable definition (`commandId`, `expectedRevision` of the workflow, `definition`); does not activate it |
| GET | `/workflows/{id}/versions/{version}` | `workflow.read` | 200 | Read a definition |
| POST | `/workflows/{id}/versions/{version}/activate` | `workflow.activate` | 200 | Activate (`expectedRevision`) |
| POST | `/workflows/{id}/preview` | `workflow.preview` | 200 | Evaluate a definition without publishing |
| GET | `/workflows/{id}/candidates` | `workflow.read` | 200 | Processing candidate states |
| GET | `/workflows/{id}/candidates/{candidate}` | `workflow.read` | 200 | One candidate: `state`, `selected`, `failureCode` |
| GET | `/workflows/{id}/candidates/{candidate}/result` | `workflow.read` | 200 | Result metadata only (`id`, `revision`, `completedAt`) |
| POST | `/workflows/{id}/candidates/{candidate}/retry` | `workflow.reprocess` | 200 | Retry a failed candidate (`expectedRevision`) |

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
