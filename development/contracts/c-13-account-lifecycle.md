# C-13 Account Deletion, Audit, and Future Identity

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-06](c-06-identity-authorization.md), [C-08](c-08-events-jobs.md), [C-09](c-09-delivery.md), [C-10](c-10-plugin-host.md), [C-12](c-12-persistence-operations.md).

**Implementation planning:** [AP-14](../work-packages/ap-14-identity-lifecycle.md) owns account lifecycle and concrete cleanup participants against completed personal-data stores.

**Traceability:** [REQ-061..062/065..068](../requirements-catalog.md#g-accounts-and-login), [REQ-080](../requirements-catalog.md#h-recovery-and-access-keys), [REQ-109](../requirements-catalog.md#k-installed-web-app-and-security-behavior), [REQ-131](../requirements-catalog.md#m-additional-domain-capabilities). Issue references: [P-14/P-17](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft. **Owner:** Identity; integration coordinates deletion; Delivery owns Delivery fields. **Parties:** confirmed account owner/authorized account manager, all personal-data owners.

Operations: `RequestAccountDeletion(Principal, target, expectedRevision, confirmation)` -> deletion reference/state; `ExecuteAccountDeletion(job, fencingToken)` -> progress/completion; `ReadSecurityAudit(Principal, filters, pageToken)` -> permitted redacted evidence. Own deletion requires `account.delete-own` and a fresh authenticated confirmation; management deletion requires target-specific `account.delete`. Guest is undeletable. Repeating the same command identity returns the same deletion result, never creates a new Principal.

## Immediate Boundary and Restart

**AP-07 identity-core 1.0 partial freeze, 2026-10-26:** The user approved this immediate non-authenticating/revocation boundary and the field-handling obligations needed by the [AP-07 prerequisite binding](../execution/ap-04.md#ap-07-prerequisite-consumer-binding-proposal). This does not freeze or accelerate external/plugin/Delivery cleanup implementations, certify whole-account cleanup, or approve OIDC/SCIM. Unimplemented or unacknowledged personal-data owners must not be reported as cleaned up.

The accepting transaction marks the account non-authenticating/deleting, increments access revision, revokes all login/key/recovery paths and grants/memberships, pauses owned Subscriptions, cancels unstarted account-related work, and records `principal.deleted` plus durable cleanup work. Reject further protected output/starts immediately, not after cleanup. Profile/contact/credential fields in Core account records are removed in that transaction; cleanup handles dependent domain-owned copies and external-secret references. No new account may reuse the old internal ID.

State is deleting -> completed, or deleting -> cleanup-failed -> retrying. Access remains blocked through every failure/restart; cleanup is idempotent and C-08 fenced. Managers get safe per-domain progress/error state, not retained profile values. Success is reported only when all registered personal-data owners acknowledge their cleanup. Provider effects authorized before the boundary remain accepted/uncertain under C-09, not falsely cancelled. No distributed transaction or remote provider purge is promised.

## Field Handling (D-26)

| Data class | Required handling |
| --- | --- |
| Principal/profile | Retain only stable old Principal ID, deleted kind/state and deletion time needed for historical attribution. Remove username/display name, contact/email/phone, avatar/profile metadata, preferences and personal Extension fields. Render `Deleted user` plus a stable ID-derived distinguishing suffix; collision handling must keep different deleted actors distinguishable. No login, reactivation or ID recycling. |
| Credentials/external bindings/recovery | Delete password hash/salt, identity issuer/subject bindings, provider tokens, recovery requests/proof material and verified-contact bindings. Remove local personal Secret-store values; release external references without claiming to erase operator-owned external secrets. |
| Sessions/access keys | Revoke immediately; delete bearer hashes/material and device/network/client labels. Historical audit may retain nonsecret session/key operation reference, former owner ID, revocation time and outcome, never reusable authorization. |
| Grants/memberships | Remove effective account grants/User group memberships; keep only classified audit of operation/reference/outcome where needed. Deleting a subject does not broaden anybody else's grant. |
| Personal Subscriptions/Delivery targets | End Subscriptions and delete personal target configuration, contact/destination and Secret fields; remove prepared inputs and pending jobs' personal-data copies. Shared targets/configuration survive only under an already authorized independent owner/technical Principal; no automatic ownership transfer or inheritance of the deleted user's rights. |
| Delivery history | Retain nonsecret operation/event/target references, times, result and uncertainty where needed for dedup/audit. Remove account-specific rendered input/body, contact/destination, provider response bodies and personal target labels. A provider receipt is retained only if nonsecret and not a personal contact; otherwise remove it while preserving `uncertain`/result facts. C-08/C-09 receipt/expiry rules still apply. |
| Workflow/configuration history and audit | Keep actor/target opaque IDs, action, time, revision and safe result. Remove product-maintained actor profile snapshots, credential/contact/Secret values. Shared Unit/Organization/configuration content is not deleted merely because this actor authored it. Never add credential/secret Payload to audit. |
| Received Payload and ordinary logs | Account deletion does not rewrite independent immutable received Payload or promise a blanket purge of ordinary operational logs. Logs must already follow C-12 redaction; historical free text and external records may still identify a person. This is not an anonymity claim. |
| Backups/exported/external data | Operators own manual Backup/Restore, retention, secure disposal and external provider policy (D-31). No application restore detection/hold/cleanup or guarantee that deleting the live account erases prior copies. |

Domain cleanup is durable, visible and retried rather than silently skipped. Inaccessible dependent copies remain blocked while cleanup is pending; C-09's 24 h input ceiling is not permission to retain account secrets until that deadline. A plugin owning a personal-data copy must declare its cleanup responsibility; unavailable required plugin cleanup prevents a completed claim. Secret values must never be needed to retry cleanup.

This is **pseudonymization**: old IDs keep deleted actors distinct and can be correlated with outside knowledge. Do not label them anonymous. No indefinite retention exception for arbitrary personal fields follows from keeping IDs; security/dedup records contain only the stated minimal fields. Security audit access remains scoped `audit.read` with C-06 projection, not ordinary Message access. No quantitative backup/log-retention policy is invented for the operator.

## Future Identity Boundary

OIDC binds `iss`+`sub` to the same internal Principal ID. Linking requires existing login plus fresh external authentication, never matching email alone. Do not accidentally remove the last login path. SCIM is provisioning, not login: explicit User group mapping; unmapped means no rights; removed mappings revoke only their own memberships. External deactivation blocks external login/Delivery; independent local credentials remain possible under the rule; feed disappearance is not deactivation. A deleted Principal is never silently restored by a later identity feed.

Detailed external/manual authority and identity lifecycle belong to AP-14, not an initial generic provisioning platform. C-06 local rescue remains an explicit audited recovery path, not resurrection of deleted credentials. No deletion workflow or external identity integration was executed by this documentation task.
