# GoPager Contract Catalog

Date: 2026-10-25. All contract versions: **0.1-draft**. AP-02 documentary completion and remaining evidence/approval gates are recorded in the [plan](authoritative-plan.md#workflow-and-approval-gates). D-29's model is a confirmed logical design, not a frozen physical field inventory; D-30 defers heuristics and D-31 excludes application Backup/Restore. Not implemented. Owner roles: [architecture](architecture.md#domains-and-data-ownership).

This document records defined domain invariants and describes interfaces as **documentary drafts**. Operation/field names and result types are proposals, not compilable Go signatures, fixed HTTP endpoints, or an approved Wasm ABI. Their semantics must not weaken confirmed product rules. Freeze a versioned contract only after resolving its blockers.

## Targeted Reading

This file is an index, not a second copy of the contracts. Read only the contract(s) for the task, plus [shared conventions and freeze requirements](contracts/shared-conventions.md). Follow referenced contracts only for the boundary being changed. Use [reference cases](contracts/reference-cases.md) by stable F-ID; cases are planned outcomes, not executed tests. Current comparison scope is exact-only under D-30; heuristics are deferred, not outstanding current acceptance criteria.

The glossary remains the naming authority; the plan remains the delivery-status authority. Each contract file owns its existing C-ID and content. File separation does not change owner, scope, approval or version.

## Contract Register

| Contract | Subject | Owner | Producer / consumer | REQ references | Blockers |
| --- | --- | --- | --- | --- | --- |
| [C-01](contracts/c-01-transmission-acceptance.md) | Gateways, Networks, Transmission acceptance | DOM-01 | Gateway/management -> Transmission -> Grouping/assignment | REQ-041..050, REQ-108, REQ-113..114 | P-01, P-03, P-08, P-09 |
| [C-02](contracts/c-02-protocol-payload.md) | Protocol Payload and Transmission targets | DOM-02 with DOM-01 | Protocol boundary -> ingest/Grouping/assignment | REQ-020, REQ-031..032, REQ-037..038, REQ-121..122 | P-08, P-09 |
| [C-03](contracts/c-03-message-grouping.md) | Messages (grouping) | DOM-02 | Transmission -> Message projection/Domain event | REQ-001..002, REQ-113, REQ-122 | P-08 |
| [C-04](contracts/c-04-master-data.md) | Units, organizations, Unit collections | DOM-03 | Management -> master data/query/assignment | REQ-019..030, REQ-095, REQ-132 | AP-02 clarified; AP-04 freeze |
| [C-05](contracts/c-05-matching-correction.md) | Assignment, suppression, rematching | DOM-04 | Rules/evidence -> assignments/projection revision | REQ-021, REQ-031..040, REQ-123..124 | P-08..P-10, P-12 |
| [C-06](contracts/c-06-identity-authorization.md) | Principal, login, central AuthZ | DOM-05 | Credentials/grants -> all protected use cases | REQ-051..061, REQ-063..079, REQ-105, REQ-107..110 | P-09, P-10, P-14, P-17 |
| [C-07](contracts/c-07-query-live.md) | REST queries, search, SSE | DOM-06 | Domain projections/AuthZ -> Vue/API consumers | REQ-001..020, REQ-027, REQ-029, REQ-055..060, REQ-097..100 | P-08..P-10, P-12, P-15 |
| [C-08](contracts/c-08-events-jobs.md) | Domain events, outbox, jobs | DOM-07 / integration | Domain modules -> live/rematch/Delivery workers | REQ-049, REQ-081, REQ-113..114, REQ-117 | P-01, P-04, P-11, P-12 |
| [C-09](contracts/c-09-delivery.md) | Subscriptions, targets, Delivery | DOM-07 | Subscription/Domain event -> plugin host/attempt | REQ-081..087, REQ-105..106, REQ-129..130 | P-07, P-10, P-11 |
| [C-10](contracts/c-10-plugin-host.md) | Plugin SDK and host capabilities | Plugin host with DOM-07 | Operator/module -> host-controlled capabilities | REQ-083..084, REQ-089..090, REQ-107, REQ-129 | P-05..P-07, P-11 |
| [C-11](contracts/c-11-processing-presentation.md) | Payload processing and presentation | DOM-08 | Manager/sample/Transmission -> derived values/view | REQ-091..097, REQ-099..100 | P-16 |
| [C-12](contracts/c-12-persistence-operations.md) | Persistence, roles, operations | Operations / integration | App/store/operator -> status, retention, restart, profile transition; no application Backup/Restore | REQ-111..112, REQ-114..120, REQ-127; REQ-126 removed | P-01, P-03, P-04, P-15, P-20, P-21 |
| [C-13](contracts/c-13-account-lifecycle.md) | Account lifecycle, audit, extension boundaries | DOM-05 / integration | Account deletion/security actions -> all personal-data owners | REQ-061..062, REQ-065..068, REQ-080, REQ-109, REQ-131 | P-14, P-17 |
