# GoPager Work Packages

Start from the assigned AP and the [execution rules](work-packages/shared-rules.md#session-start). New and amended APs follow the [AP template](work-packages/ap-template.md).

<a id="package-directory"></a>
## Revised Execution Path

Complete each producer and verify its handover before starting a dependent AP. Status `done` means every task is verified in the AP's execution record.

| AP | Assignment | Tasks | Status |
| --- | --- | --- | --- |
| AP-06 | [Processing Kernel](work-packages/ap-06-processing-kernel.md) | T055..T056 | done ([handover](execution/ap-06.md#handover)) |
| AP-07 | [Transmission Intake](work-packages/ap-07-transmission-intake.md) | T057..T059 | done ([handover](execution/ap-07.md#handover)) |
| AP-08 | [Master Data and Correction](work-packages/ap-08-master-data-correction-after-intake.md) | T060..T062 | done ([handover](execution/ap-08.md#handover)) |
| AP-09 | [Gateway and Network Administration](work-packages/ap-09-gateway-network.md) | T086..T088 | done ([handover](execution/ap-09.md#handover)) |
| AP-10 | [Query and Live API](work-packages/ap-10-query-live.md) | T063..T065 | done ([handover](execution/ap-10.md#handover)) |
| AP-11 | [Events and Delivery](work-packages/ap-11-events-delivery.md) | T066..T068 | done ([handover](execution/ap-11.md#handover)) |
| AP-21 | [Attribute Schemas, Agency Binding and Field Registry](work-packages/ap-21-attribute-schemas.md) ([D-150](decisions/04-processing-and-plugins.md#d-150), [D-151](decisions/06-identity-and-authorization.md#d-151), [D-152](decisions/07-query-and-live.md#d-152)) | T092..T094 | done ([handover](execution/ap-21.md#handover)) |
| AP-22 | [Unit Attributes](work-packages/ap-22-unit-attributes.md) ([D-150](decisions/04-processing-and-plugins.md#d-150), before AP-12: [D-157](decisions/05-events-and-delivery.md#d-157)) | T095..T096 | done ([handover](execution/ap-22.md#handover)) |
| AP-12 | [Plugin Host and Initial Targets](work-packages/ap-12-plugin-host.md) | T069..T071 | open |
| AP-13 | [Processing Presentation and Integration](work-packages/ap-13-processing-presentation-integration.md) | T072..T074 | open |
| AP-14 | [Identity Lifecycle Completion](work-packages/ap-14-identity-lifecycle.md) | T075..T077 | open |
| AP-15 | [Frontend](work-packages/ap-15-frontend.md) | T078..T080, T089 | open |
| AP-16 | [Operations and Profile Evidence](work-packages/ap-16-operations.md) | T081..T083 | open |
| AP-17 | [Integration and Delivery Evidence](work-packages/ap-17-integration-evidence.md) | T084..T085 | open |
| AP-20 | [Re-resolution after Gateway Network-Mapping Changes](work-packages/ap-20-gateway-mapping-rematch.md) ([D-139](decisions/02-intake-and-grouping.md#d-139)) | T090..T091 | done ([handover](execution/ap-20.md#handover)) |

## Required Foundation Evidence

Read only the slice consumed by the assigned AP.

| Input | Evidence |
| --- | --- |
| Contracts and cases | [Contract register](domain-contracts.md#contract-register) |
| Contract-planning assignments | [AP-01](work-packages/ap-01-protocols-grouping.md), [AP-02](work-packages/ap-02-domain-contracts.md) |
| Feasibility and exact scope bindings | [AP-03](work-packages/ap-03-technical-validation.md), [AP-04](work-packages/ap-04-scope-freeze.md) |
| Transactions, primary clock, migrations and lifecycle | [AP-05 handover](execution/ap-05.md#handover) |
| Identity/AuthZ core and global fields | [Identity handover](execution/source-ap-07.md#handover) |
| Current Unit/collection/matcher | [Matcher handover](execution/source-ap-08.md#handover) |
| Processing kernel | [AP-06 handover](execution/ap-06.md#handover) |
| Accepted intake and Processing publication | [AP-07 handover](execution/ap-07.md#handover) |

[Requirement mapping](work-packages/requirement-mapping.md) identifies current producers and evidence categories. Implementation assignments link their exact supporting contracts, paths, checks and consumers.
