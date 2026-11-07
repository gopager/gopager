# ADR: Identity and authorization

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-07"></a>
## D-07 Permission model

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed permission model.
- **Owner detail:** [C-06](../contracts/c-06-identity-authorization.md)

<a id="d-11"></a>
## D-11 Identity/credential separation and later external-identity boundary

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed identity/credential separation and later external-identity boundary.
- **Owner detail:** [C-06](../contracts/c-06-identity-authorization.md), [C-13](../contracts/c-13-account-lifecycle.md)

<a id="d-26"></a>
## D-26 Deletion model

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed deletion model; supersedes anonymity proposals.
- **Owner detail:** [C-13](../contracts/c-13-account-lifecycle.md)

<a id="d-47"></a>
## D-47 AP-06 Gateway Credential Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved reusable per-Gateway Bearer credential, including over direct HTTP. Use a reusable Bearer credential unique to each Gateway, sent in the HTTP `Authorization` header over HTTP or HTTPS. The credential authenticates only its associated Gateway; current Gateway Network assignments and C-06 permissions still authorize operations. No signed-request scheme, timestamp/nonce requirement or replay cache is approved by this decision.
- **Consequences:** C-06 owns credential lifecycle/storage and generic authentication failure behavior; C-01 derives Gateway identity from the credential, never a caller-supplied Gateway ID. [D-48](06-identity-and-authorization.md#d-48) records expiry and rotation.
- **Owner detail:** [C-06](../contracts/c-06-identity-authorization.md)

<a id="d-48"></a>
## D-48 AP-06 Gateway Credential Lifecycle Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved non-expiring Gateway credentials with first-use rotation cutover. Gateway credentials do not expire automatically; they remain valid until explicitly revoked. During rotation, one replacement credential may be issued while the current credential remains valid. On the replacement credential's first successfully authenticated request, the server atomically promotes it and revokes its predecessor before authorizing that request. Until that cutover, both credentials can authenticate. A Gateway cannot have more than one pending replacement; another rotation requires resolving the pending one first.
- **Consequences:** C-06 owns issuance, hashing, rotation and revocation mechanics; C-01 consumes the resulting Gateway identity. [F-49](../contracts/reference-cases.md#ap-06-gateway-authentication-case) covers rotation and revocation.
- **Owner detail:** [C-06](../contracts/c-06-identity-authorization.md)

<a id="d-54"></a>
## D-54 Local Username Equality Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved NFC/Unicode-casefold equality for local usernames, preserving display spelling. Preserve the submitted display spelling and use one NFC/full-default-Unicode-casefold/NFC comparison key for account uniqueness, login, Recovery and account-keyed throttling. Accents remain significant; canonically equivalent and casefold-equivalent usernames identify the same account. Passwords retain their existing no-normalization rule.
- **Owner detail:** [C-06](../../docs/http-api.md#authorization)

<a id="d-55"></a>
## D-55 Detail Field Authorization Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved read-field rights plus a separate detail-action gate. The user selected field authorization through `message.read`, with a separate `message.detail` action gate on the same concrete data context. The alternative required duplicate field declarations on both actions. The selected option separates data visibility from entry into the additional view without widening existing readable scope.
- **Owner detail:** [C-06](../../docs/http-api.md#authorization)

<a id="d-56"></a>
## D-56 Identity Fact Append Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved minimal identity-fact freeze and append integration. Freeze the proposed [C-08 identity-facts 1.0 subset](../../docs/architecture.md#domain-events-and-outbox) and implement its minimal append integration now. The alternative was to hold this integration, which prevents the required identity-core handover. Only the existing three identity/configuration fact types and nonsecret context are included.
- **Owner detail:** [C-08](../../docs/architecture.md#domain-events-and-outbox)

<a id="d-57"></a>
## D-57 HTTP Credential Selection Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved explicit Authorization bearer precedence without Session/Guest fallback. Use the explicitly supplied Authorization credential without fallback to Session or Guest if that credential is invalid. The alternative rejected every mixed request. The chosen rule supports deliberate API-key calls from an already signed-in browser without silently escalating a failed key request to Session authority.
- **Owner detail:** [C-06](../../docs/http-api.md#authentication)

<a id="d-58"></a>
## D-58 Ordinary Login Decision

- **Status:** accepted
- **Date:** 2026-10-27
- **Decision:** Approved ordinary login without Command IDs or login receipts. Implement ordinary login without a Command ID or login receipts. Each successful login may create a new Session. The alternative was to add a client Command ID and safe retry-status lookup, which the user rejected as unnecessary complexity. Keep the existing password, Session, throttle, Origin/CSRF and application-wide HTTP rules.
- **Owner detail:** [C-06](../../docs/http-api.md#sessions-browser-login)

<a id="d-59"></a>
## D-59 Identity Prerequisite Scope Decision

- **Status:** accepted
- **Date:** 2026-10-27
- **Decision:** Approved AP-06-required identity handover only, retaining AP-07-compatible boundaries. Narrow the active prerequisite to ordinary login/logout, local one-time bootstrap, current account/Session/Grant resolution and denial after revocation/inactivation, direct/group scope/field/age/key-restriction algebra, and protected integration needed by Gateway/Network management. Preserve typed store-neutral ports, the existing identity model, Ent participation and owner scope-reference injection for later AP-07 extension. Do not introduce throwaway authentication, implicit administrator authority or weaker rights checking.
- **Owner detail:** This record is the only current definition

<a id="d-68"></a>
## D-68 Field Visibility Grouping Direction

- **Status:** accepted
- **Date:** 2026-11-01
- **Decision:** Fixed product-defined field groups released through permissions. GoPager supplies fixed understandable field groups; administrators configure their release through permissions. The user selected this over administrator-defined arbitrary groups of individual fields. This approves the grouping direction only, not a new exhaustive field catalog, splitting the existing frozen `gateway`/`unit` groups, or address-dependent grant implementation.
- **Consequences:** Prepare the concrete group/field/search eligibility matrix at C-06/C-07 under documentary authorization, retaining earlier frozen owner definitions until their separate amendment approval. Internal matching dependencies do not imply visibility of addresses, Functions or Address-rule details. Beneficiary-controlled search must not provide a direct query oracle for unreadable fields; permission-filtered options and outputs retain C-07's existing protections.
- **Owner detail:** This record is the only current definition

<a id="d-69"></a>
## D-69 Unscoped Field Visibility Clarification

- **Status:** accepted
- **Date:** 2026-11-01
- **Decision:** Field visibility is unscoped (global per Principal/group), separate from scoped data eligibility. Separate scope-based data eligibility from unscoped effective field-group visibility. The normative product clarification is owned by [C-06](../../docs/http-api.md#authorization). Retaining conditional per-Unit field visibility is rejected. D-68 fixed product-defined groups remain applicable; no administrator-defined arbitrary groups are introduced.
- **Owner detail:** This record is the only current definition

<a id="d-70"></a>
## D-70 Unscoped Identity Replacement Binding Proposal

- **Status:** accepted
- **Date:** 2026-11-01
- **Decision:** identity-fields 1.0 replacement freeze and sequential correction start; fresh-installation schema. The user explicitly states that no deployed running instances exist and permits the model to be freely changed at this development stage. The production compatibility/field-grant transfer question is therefore withdrawn as an unnecessary gate; neither A nor B is selected. Prepare a fresh-installation replacement schema without a product upgrade/reassignment workflow for nonexistent deployments. The user-provided deployment state is not an independently verified fleet inventory.
- **Owner detail:** This record is the only current definition

<a id="d-71"></a>
## D-71 Internal Execution-principal Authority Adapter

- **Status:** accepted
- **Date:** 2026-11-01
- **Decision:** Narrow internal execution-Principal authority adapter in the Identity resolver. The user selected A's exact narrow adapter and focused tests after its impact on the prior producer was explained. Extend the active T043 binding only to this additive Identity method, related artifact link and validation. Existing authentication operations and C-10's separate configuration/Principal authorization remain unchanged. Record implementation/evidence in the current AP-12 producer-consumer integration, with the prior AP-07 evidence preserved.
- **Owner detail:** This record is the only current definition

<a id="d-140"></a>
## D-140 Not-found versus forbidden for Gateways and Networks

- **Status:** accepted
- **Date:** 2026-11-07
- **Decision:** For an existing Gateway or Network, a caller that holds no right on it (no `gateway.*` or `network.*` action covering it, including `gateway.diagnostics.read` and `gateway.credential.rotate`) gets `404 NOT_FOUND`, indistinguishable from a missing object, so IDs cannot be probed. A caller that holds some right on it but not the requested one gets `403 FORBIDDEN`; a Gateway right must cover every Agency of the Gateway's Networks (or name the Gateway) to count. Creation and list routes without any grant for the action stay `403`. This changes the AP-07 credential and Network-replacement routes (403 becomes 404 for an invisible Gateway) and the AP-09 lifecycle routes (404 becomes 403 for a visible Gateway or Network).
- **Consequences:** `docs/http-api.md` (Gateway administration, Authorization), OpenAPI descriptions; AP-07 follow-up rerunning F-67/F-68; AP-09 follow-up with new evidence. Executed by AP-09 (`internal/ingest/`).
- **Owner detail:** [HTTP API: Gateway administration](../../docs/http-api.md#gateway-administration)
