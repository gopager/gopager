# GoPager Work Packages

Date: 2026-10-24. Documentary implementation plan with tasks and REQ mapping, not an instruction to implement product code. Domain requirements: [catalog](requirements-catalog.md). delivery status exclusively: [plan](authoritative-plan.md#delivery-scope). Ownership/technologies: [architecture](architecture.md). Contract references currently refer to [version 0.1-draft](domain-contracts.md).

## Targeted Reading

Open the assigned AP-ID file from the table below and [shared execution rules](work-packages/shared-rules.md). Each package retains its owner, prerequisites, contract gates, tasks, acceptance and non-goals. Follow only the relevant C-ID files in the [contract index](domain-contracts.md). The [requirement mapping](work-packages/requirement-mapping.md) is a separate lookup, not required full context for every task.

Task IDs are stable; check states change only on the owning package's verified completion. Package separation changes navigation/write scopes, not approval. Delivery status remains owned by the [plan](authoritative-plan.md). Maintain task details only in the corresponding AP file.

## Dependency Overview

| Package | Owner role | Prerequisites | Status |
| --- | --- | --- | --- |
| [AP-01](work-packages/ap-01-protocols-grouping.md) Protocols and Grouping contract | DOM-01/DOM-02, integration | Documented requirements/decisions | Original scope closed; D-29 impact amendment recorded; D-30 exact-only scope; no heuristic blockers; not frozen |
| [AP-02](work-packages/ap-02-domain-contracts.md) Remaining domain contracts | Integration and respective contract owner | [AP-01](work-packages/ap-01-protocols-grouping.md) for Transmission-dependent detail completion | Documentary completion recorded in plan; not a freeze or implementation approval |
| [AP-03](work-packages/ap-03-technical-validation.md) Technical validation planning | Persistence/operations/plugin host | Concrete test semantics from AP-01/AP-02; independent runtime questions separately plannable | Validation planned |
| [AP-04](work-packages/ap-04-scope-freeze.md) Scope and contract freeze | Product/integration ownership | Relevant AP-01..AP-03 results; targeted legacy assessment if needed | Approval pending |
| [AP-05](work-packages/ap-05-runtime-persistence.md) Runtime and persistence foundation | Persistence/operations/integration | [AP-04](work-packages/ap-04-scope-freeze.md) for C-01/C-08/C-12 and TECH-01/02/05/06/07 | Blocked |
| [AP-06](work-packages/ap-06-transmission-messages.md) Transmission, Protocols, Messages | DOM-01/DOM-02 | AP-05/AP-07, early AP-08 matcher/master data and AP-12 T043 kernel; frozen C-01..C-03/C-05/C-08/C-11 | Blocked |
| [AP-07](work-packages/ap-07-identity-authorization.md) Local identity and central AuthZ | DOM-05 | AP-05, frozen C-06/C-13 and relevant C-12 boundaries | Blocked |
| [AP-08](work-packages/ap-08-master-data-correction.md) Master data and correction | DOM-03/DOM-04 | Early master-data/matcher: AP-05/AP-07; historical integration: AP-06; frozen C-02/C-04..C-06/C-08 | Blocked |
| [AP-09](work-packages/ap-09-query-live.md) Search, REST, SSE | DOM-06/integration | AP-06..AP-08, frozen C-06/C-07/C-08 | Blocked |
| [AP-10](work-packages/ap-10-events-delivery.md) Domain events and Delivery core | DOM-07 | AP-05..AP-09, frozen C-08/C-09; agreed C-10 | Blocked |
| [AP-11](work-packages/ap-11-plugins.md) SDK, plugin host, four targets | Plugin host/respective plugin owner | AP-10, frozen and validated C-09/C-10 | Blocked |
| [AP-12](work-packages/ap-12-processing-presentation.md) Processing and presentation | DOM-08 | T043 kernel: AP-05/AP-07; presentation/query: AP-09; reactions: AP-10; frozen C-11/C-08/C-09 | Blocked |
| [AP-13](work-packages/ap-13-frontend.md) Vue frontend | Frontend | Versioned APIs from AP-07..AP-12; C-07/C-11 draft design possible, no implementation start | Blocked |
| [AP-14](work-packages/ap-14-later-extensions.md) Later extension boundaries | Respective domain/product | Separate scope decision under plan and later contract approvals | Not approved for initial implementation |
| [AP-15](work-packages/ap-15-operations.md) Operations and profile transition | Operations/integration | AP-05..AP-12, frozen C-12/C-13 and restart/attempt boundary; no application Backup/Restore | Blocked |
| [AP-16](work-packages/ap-16-integration-evidence.md) Integration and delivery evidence | Integration with all owners | Approved delivery scope and all related packages/evidence | Blocked |

Numbers are stable identifiers, not a strictly linear execution order. In particular, AP-07 follows the foundation, not AP-06; AP-14 does not precede AP-15 merely because of numbering. Partial approvals require explicitly documenting the smaller scope.

Dependencies are artifact-level where a package has staged slices: provide AP-08's standalone master-data/matcher and AP-12 T043's evaluator after AP-05/AP-07, wire them in AP-06, then finish historical/read/presentation/reaction integrations. Do not require whole AP-08/AP-12 completion before AP-06 or bypass the publication barrier to hide a cycle. No additional task IDs or execution approvals follow from this handover.
