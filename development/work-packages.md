# GoPager Work Packages

Start from the assigned AP. Read its required inputs and planning references, not the entire development-process tree or the permanent [docs/](../docs/README.md). [Shared rules](work-packages/shared-rules.md) govern execution; the [plan](authoritative-plan.md#delivery-scope) owns delivery scope and the [approval rule](authoritative-plan.md#workflow-and-approval-gates); [execution records](execution/README.md) own results.

<a id="package-directory"></a>
## Revised Execution Path

Complete each producer and verify its handover before starting a dependent AP. An AP's own planning is work before coding, not an external prerequisite. There is one approval gate per package: the user approves a package before it starts. No intermediate approval gates exist inside a package, and its completion evidence stays mandatory.

| AP | Assignment | Tasks |
| --- | --- | --- |
| AP-06 | [Processing Kernel](work-packages/ap-06-processing-kernel.md) | T055..T056 |
| AP-07 | [Transmission Intake](work-packages/ap-07-transmission-intake.md) | T057..T059 |
| AP-08 | [Master Data and Correction](work-packages/ap-08-master-data-correction-after-intake.md) | T060..T062 |
| AP-09 | [Gateway and Network Administration](work-packages/ap-09-gateway-network.md) | T086..T088 |
| AP-10 | [Query and Live API](work-packages/ap-10-query-live.md) | T063..T065 |
| AP-11 | [Events and Delivery](work-packages/ap-11-events-delivery.md) | T066..T068 |
| AP-12 | [Plugin Host and Initial Targets](work-packages/ap-12-plugin-host.md) | T069..T071 |
| AP-13 | [Processing Presentation and Integration](work-packages/ap-13-processing-presentation-integration.md) | T072..T074 |
| AP-14 | [Identity Lifecycle Completion](work-packages/ap-14-identity-lifecycle.md) | T075..T077 |
| AP-15 | [Frontend](work-packages/ap-15-frontend.md) | T078..T080 |
| AP-16 | [Operations and Profile Evidence](work-packages/ap-16-operations.md) | T081..T083 |
| AP-17 | [Integration and Delivery Evidence](work-packages/ap-17-integration-evidence.md) | T084..T085 |

## Required Foundation Evidence

Read only the slice consumed by the assigned AP.

| Input | Evidence |
| --- | --- |
| Contracts and cases | [Contract register](domain-contracts.md#contract-register) |
| Contract-planning assignments | [AP-01](work-packages/ap-01-protocols-grouping.md), [AP-02](work-packages/ap-02-domain-contracts.md) |
| Feasibility and exact scope bindings | [AP-03](work-packages/ap-03-technical-validation.md), [AP-04](work-packages/ap-04-scope-freeze.md) |
| Transactions, primary clock, migrations and lifecycle | [AP-05](execution/ap-05.md) |
| Identity/AuthZ core and global fields | [Identity handover](execution/source-ap-07.md#handover) |
| Current Unit/collection/matcher | [Matcher handover](execution/source-ap-08.md#handover) |
| Processing kernel | [AP-06](execution/ap-06.md) |
| Accepted intake and Processing publication | [AP-07](execution/ap-07.md) |

[Requirement mapping](work-packages/requirement-mapping.md) identifies current producers and evidence categories. Implementation assignments link their exact supporting contracts, paths, checks and consumers.
