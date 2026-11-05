# AP-17 Integration and Delivery Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [delivery scope](../authoritative-plan.md#delivery-scope), [approval](../authoritative-plan.md#workflow-and-approval-gates) and [open issues](../authoritative-plan.md#open-issues-and-risks).
- **Requirements and evidence:** [REQ-to-package coverage](requirement-mapping.md), [execution records](../execution/README.md), actual approved contract versions and F-cases.
- **Owner:** Integration with all contributing domain owners.
- **Architecture:** [repository layout and testing](../architecture.md#repository-layout-and-testing), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Use the [REQ producer mapping](requirement-mapping.md) to select exact required handovers, evidence and fixed versions for the approved slice. Missing producer evidence is a handover gap, not work to backfill or infer here.
- **Write scope:** Integration/evidence artifacts and shared build/API/SDK wiring only; domain corrections remain with their owner.

## Planning Before Coding

Produce the evidence mapping and acceptance procedure within the assigned evidence artifacts before verification:

- For T084, map every in-scope REQ to its producing AP/task, exact evidence artifact and verification type; label planned versus executed, fixture versus real integration/provider/browser/recovery checks, and any unverified claim.
- For T085, define the compatibility/build/API/SDK/asset checks, relevant version inputs, gate criteria, deviation handling and the exact conditions for reporting a blocker or completion.
- Do not create product behavior or replace missing owner evidence. Report missing evidence against its producer and keep unverified or deferred claims explicitly unresolved.

- [ ] T084 [AP-17] Complete traceable REQ-to-test/operations evidence for the approved delivery slice, distinguishing fixtures from real integration/provider/browser/recovery checks.
- [ ] T085 [AP-17] Verify selected contract/version compatibility, endpoint/SDK/build/asset integration, gates, deviations, and remaining blockers without a blanket completion claim.

## Exact Producer Inputs and Verification

Consume AP-08 master-data/correction PostgreSQL/HTTP evidence, AP-09 provisioning/contact-loss evidence, AP-10 query/live/search/saved-view disposition evidence, AP-11 materialization/retry/rendering evidence, AP-12 actual external Delivery and host/SDK evidence, AP-13 Reprocess/presentation evidence, AP-14 identity/cleanup evidence, AP-15 browser/build/assets and AP-16 retention/Full recovery evidence. Use producer records for actual versions, paths, commands and limitations.

T084 maps every approved REQ, including explicit disposition of REQ-133/134/138 and refinements REQ-135..137, to producer task and executed acceptance cases. Real-provider, PostgreSQL, browser and fault evidence are separate categories. No missing provider environment, domain implementation or producer test is backfilled here.

T085 may change shared build/CI, API/SDK compatibility checks and asset/package integration paths in the repository root, `cmd/gopager/`, `sdk/`, `web/` and packaging/evidence artifacts only to integrate already completed behavior. Run actual Go build/vet/test, selected OpenAPI/SDK compatibility commands and AP-15's recorded frontend build/test commands; verify packaged assets and artifact/version agreement. Record exact runnable selectors from the completed producers. Incompatible/missing inputs block the affected claim; do not silently weaken contracts or invent a success fallback.

**Acceptance:** Every approved delivery claim is supported by evidence from its owning execution record; no missing or deferred slice is represented as complete.

**Non-goals:** New product scope, unapproved quantitative guarantees, required translations, or replacing owner evidence with package checkmarks.
