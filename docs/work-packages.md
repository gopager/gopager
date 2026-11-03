# GoPager Work Packages

Navigation and execution order. Each AP file is an assignment baseline, not a progress log. Product scope/execution approval belong to the [plan](authoritative-plan.md#workflow-and-approval-gates); actual task state, assignee, commands and handovers belong to [execution records](execution/README.md).

## Targeted Reading

An assigned AP file is the sole entry document needed to start a session. Its **Session Context** links the shared start procedure, relevant requirements, architecture, owner contracts, planned cases and package handovers. Follow those links; no earlier conversation or full-document preload is required.

Use the [requirement mapping](work-packages/requirement-mapping.md) for cross-package coverage, not a second task list. Shared execution mechanics belong to the [execution rules](work-packages/shared-rules.md).

## Historical Source Baselines

AP-01..AP-05 remain unchanged. The former AP-06..AP-16 assignment files are retained under `source-ap-*` names as historical baselines; their task IDs and recorded outputs are provenance, not current assignments. Their verified partial outputs remain in the corresponding `source-ap-*` execution records. Do not infer completion of a whole source AP from a transferred slice.

| Historical source baseline | Focus |
| --- | --- |
| [AP-01](work-packages/ap-01-protocols-grouping.md) | Protocols and Grouping contract |
| [AP-02](work-packages/ap-02-domain-contracts.md) | Remaining domain contracts |
| [AP-03](work-packages/ap-03-technical-validation.md) | Technical validation planning and authorized evidence |
| [AP-04](work-packages/ap-04-scope-freeze.md) | Scope approval and contract freeze |
| [AP-05](work-packages/ap-05-runtime-persistence.md) | Runtime and persistence foundation |
| [Source AP-06](work-packages/source-ap-06-transmission-messages.md) | Transmission, Protocols and Messages |
| [Source AP-07](work-packages/source-ap-07-identity-authorization.md) | Local identity and central AuthZ |
| [Source AP-08](work-packages/source-ap-08-master-data-correction.md) | Master data and correction |
| [Source AP-09](work-packages/source-ap-09-query-live.md) | Search, REST and SSE |
| [Source AP-10](work-packages/source-ap-10-events-delivery.md) | Domain events and Delivery |
| [Source AP-11](work-packages/source-ap-11-plugins.md) | SDK, plugin host and initial targets |
| [Source AP-12](work-packages/source-ap-12-processing-presentation.md) | Processing and presentation |
| [Source AP-13](work-packages/source-ap-13-frontend.md) | Frontend |
| [Source AP-14](work-packages/source-ap-14-later-extensions.md) | Separately gated later extensions; outside the required path |
| [Source AP-15](work-packages/source-ap-15-operations.md) | Operations and profile transition |
| [Source AP-16](work-packages/source-ap-16-integration-evidence.md) | Integration and delivery evidence |
| [Former D-96 AP-17](work-packages/source-ap-17-processing-kernel.md) | Superseded numbering of the active Processing Kernel slice; current assignment is AP-06 |

The `ap-06-transmission-messages.md`, `ap-08-master-data-correction.md` and `ap-12-processing-presentation.md` files are reference-only pointers retained for unchanged AP-02 links; they are not assignments. Use the ordered path below for current APs.

## Revised Execution Path

The AP ID is the required order. Finish each AP and hand over its output before starting the next. Each assignment names only earlier current APs as prerequisites; verified source-history handovers are explicitly identified as such. No parallel branch or later-package output is assumed. The reslicing is approved under [D-96](authoritative-plan.md#work-package-reslicing-decision); sequential AP numbering is approved under [D-97](authoritative-plan.md#ordered-ap-renumbering-decision). Implementation approval remains separately gated as recorded in the plan.

| Order | Current assignment | Resliced historical source scope |
| --- | --- | --- |
| AP-06 | [Processing Kernel](work-packages/ap-06-processing-kernel.md) | Source AP-12 T043 kernel and standalone kernel verification |
| AP-07 | [Transmission Intake](work-packages/ap-07-transmission-intake.md) | Source AP-06 T022..T024 |
| AP-08 | [Master Data and Correction](work-packages/ap-08-master-data-correction-after-intake.md) | Remaining source AP-08 T028..T030 |
| AP-09 | [Query and Live API](work-packages/ap-09-query-live.md) | Source AP-09 T031..T033 |
| AP-10 | [Events and Delivery](work-packages/ap-10-events-delivery.md) | Source AP-10 T034..T036 |
| AP-11 | [Plugin Host and Initial Targets](work-packages/ap-11-plugin-host.md) | Source AP-11 T037/T038/T041/T042 |
| AP-12 | [Processing Presentation and Integration](work-packages/ap-12-processing-presentation-integration.md) | Source AP-12 T044 and source-bound part of T045 |
| AP-13 | [Identity Lifecycle Completion](work-packages/ap-13-identity-lifecycle.md) | Remaining source AP-07 T025..T027 |
| AP-14 | [Frontend](work-packages/ap-14-frontend.md) | Source AP-13 T046..T048 |
| AP-15 | [Operations and Profile Evidence](work-packages/ap-15-operations.md) | Source AP-15 T050..T052 |
| AP-16 | [Integration and Delivery Evidence](work-packages/ap-16-integration-evidence.md) | Source AP-16 T053..T054 |

### Already Verified Historical Inputs

These are slice-level handovers, not whole-source-package completion and not extra active assignments:

| Input | Evidence owner |
| --- | --- |
| Source AP-07 central identity/AuthZ core and global-field correction | [Source AP-07 execution record](execution/source-ap-07.md) |
| Source AP-08 early Unit/collection/current-matcher slice | [Source AP-08 execution record](execution/source-ap-08.md) |
| Source AP-12 partial evaluator, persistence, lifecycle and publication infrastructure | [Source AP-12 execution record](execution/source-ap-12.md); the verified kernel handover remains the current AP-06 outcome |

### Source-to-Current Crosswalk

| Historical source baseline | Current assignment(s) |
| --- | --- |
| Source AP-06 | AP-07 |
| Source AP-07 | Verified identity/AuthZ input above; remaining lifecycle work in AP-13 |
| Source AP-08 | Verified matcher input above; remaining master-data/correction work in AP-08 |
| Source AP-09 | AP-09 |
| Source AP-10 | AP-10 |
| Source AP-11 | AP-11; deferred Telegram/email remain gated |
| Source AP-12 | AP-06 kernel; AP-12 presentation and source-bound integration |
| Source AP-13 | AP-14 |
| Source AP-14 | Historical, separately gated, and not on the required path |
| Source AP-15 | AP-15 |
| Source AP-16 | AP-16 |

AP-01..AP-05 have no revised assignment. For REQ coverage use the [requirement mapping](work-packages/requirement-mapping.md) together with this crosswalk. Shared execution mechanics remain in [execution rules](work-packages/shared-rules.md).
