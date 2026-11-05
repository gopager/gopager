# GoPager Contract Catalog

Navigation only. Each C-ID file owns its contract owner, version/status, producers/consumers, REQ references, invariants and boundary-specific remaining gates. Approval and open-issue status belong to the [plan](authoritative-plan.md#workflow-and-approval-gates); role responsibilities belong to the [architecture](architecture.md#domains-and-data-ownership).

This file contains no interface definitions. Follow the C-ID owner for representation, version/status and actual API/schema/SDK artifacts, when introduced.

## Targeted Reading

Read only the contract(s) for the task, plus [shared conventions and freeze requirements](contracts/shared-conventions.md). Follow referenced contracts only for the boundary being changed. Use [reference cases](contracts/reference-cases.md) by stable F-ID; actual execution evidence is in [execution records](execution/README.md).

The [glossary](glossary.md) remains the naming authority. Shared wire conventions/limits belong to [shared conventions](contracts/shared-conventions.md); concrete planned inputs/outcomes belong to the F-ID case files, not duplicated case tables in every contract. File separation does not change scope, approval or version.

## Contract Register

| Contract | Subject |
| --- | --- |
| [C-01](contracts/c-01-transmission-acceptance.md) | Gateways, Networks, Transmission acceptance |
| [C-02](contracts/c-02-protocol-payload.md) | Protocol Payload and Transmission targets |
| [C-03](contracts/c-03-message-grouping.md) | Message Grouping |
| [C-04](contracts/c-04-master-data.md) | Units, organizations, Unit collections |
| [C-05](contracts/c-05-matching-correction.md) | Assignment, suppression, Rematching |
| [C-06](contracts/c-06-identity-authorization.md) | Principal, login, central AuthZ |
| [C-07](contracts/c-07-query-live.md) | REST queries, search, SSE |
| [C-08](contracts/c-08-events-jobs.md) | Domain events, outbox, jobs |
| [C-09](contracts/c-09-delivery.md) | Subscriptions, targets, Delivery |
| [C-10](contracts/c-10-plugin-host.md) | Plugin SDK and host capabilities |
| [C-11](contracts/c-11-processing-presentation.md) | Payload processing and presentation |
| [C-12](contracts/c-12-persistence-operations.md) | Persistence, roles, operations |
| [C-13](contracts/c-13-account-lifecycle.md) | Account lifecycle, audit, extension boundaries |
