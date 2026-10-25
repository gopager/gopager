# GoPager Work Packages

Navigation only. Each AP file is a stable assignment baseline, not a progress log. Product scope/execution approval belong to the [plan](authoritative-plan.md#workflow-and-approval-gates); actual task state, assignee, commands and handovers belong to [execution records](execution/README.md).

## Targeted Reading

An assigned AP file is the sole entry document needed to start a session. Its **Session Context** links the shared start procedure, relevant requirements, architecture, owner contracts, planned cases and package handovers. Follow those links; no earlier conversation or full-document preload is required.

Use the [requirement mapping](work-packages/requirement-mapping.md) for cross-package coverage, not a second task list. Shared execution mechanics belong to the [execution rules](work-packages/shared-rules.md).

## Package Directory

| Package | Focus |
| --- | --- |
| [AP-01](work-packages/ap-01-protocols-grouping.md) | Protocols and Grouping contract |
| [AP-02](work-packages/ap-02-domain-contracts.md) | Remaining domain contracts |
| [AP-03](work-packages/ap-03-technical-validation.md) | Technical validation planning and authorized evidence |
| [AP-04](work-packages/ap-04-scope-freeze.md) | Scope approval and contract freeze |
| [AP-05](work-packages/ap-05-runtime-persistence.md) | Runtime and persistence foundation |
| [AP-06](work-packages/ap-06-transmission-messages.md) | Transmission, Protocols and Messages |
| [AP-07](work-packages/ap-07-identity-authorization.md) | Local identity and central AuthZ |
| [AP-08](work-packages/ap-08-master-data-correction.md) | Master data and correction |
| [AP-09](work-packages/ap-09-query-live.md) | Search, REST and SSE |
| [AP-10](work-packages/ap-10-events-delivery.md) | Domain events and Delivery core |
| [AP-11](work-packages/ap-11-plugins.md) | SDK, plugin host and four targets |
| [AP-12](work-packages/ap-12-processing-presentation.md) | Processing and presentation |
| [AP-13](work-packages/ap-13-frontend.md) | Vue frontend |
| [AP-14](work-packages/ap-14-later-extensions.md) | Later extension boundaries |
| [AP-15](work-packages/ap-15-operations.md) | Operations and profile transition |
| [AP-16](work-packages/ap-16-integration-evidence.md) | Integration and delivery evidence |

Numbers are stable identifiers, not execution order. Follow prerequisites in the owning AP, including the staged handovers in [AP-06](work-packages/ap-06-transmission-messages.md), [AP-08](work-packages/ap-08-master-data-correction.md) and [AP-12](work-packages/ap-12-processing-presentation.md).
