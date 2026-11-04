# AP-16 Integration and Delivery Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [approved delivery scope](../authoritative-plan.md#delivery-scope), [open issues](../authoritative-plan.md#open-issues-and-risks).
- **Source scope:** Historical [source AP-16 T053..T054](source-ap-16-integration-evidence.md).
- **Requirements and evidence:** [REQ-to-package coverage](requirement-mapping.md), [execution records](../execution/README.md), [source evidence](../requirements-evidence.md), approved contract versions and F-cases.
- **Owner:** Integration with all contributing domain owners.
- **Architecture:** [repository layout and testing](../architecture.md#repository-layout-and-testing), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verified AP-06 through AP-15 handovers and evidence from each producing owner; only fixed contract/decision versions used by the approved delivery slice. Missing producer evidence is an explicit handover gap, not work to backfill or infer in AP-16.
- **Write scope:** Integration/evidence artifacts and shared build/API/SDK wiring only; domain corrections remain with their owner.

## Planning Before Coding

Complete this planning as the first part of T084-T085; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Produce the evidence mapping and acceptance procedure within the assigned evidence artifacts and track them within those existing task IDs:

- For T084, map every in-scope REQ to its producing AP/task, exact evidence artifact and verification type; label planned versus executed, fixture versus real integration/provider/browser/recovery checks, and any unverified claim.
- For T085, define the compatibility/build/API/SDK/asset checks, relevant version inputs, gate criteria, deviation handling and the exact conditions for reporting a blocker or completion.
- Do not create product behavior or replace missing owner evidence. Report missing evidence against its producer and keep unverified or deferred claims explicitly unresolved.

- [ ] T084 [AP-16] Complete traceable REQ-to-test/operations evidence for the approved delivery slice, distinguishing fixtures from real integration/provider/browser/recovery checks.
- [ ] T085 [AP-16] Verify selected contract/version compatibility, endpoint/SDK/build/asset integration, gates, deviations, and remaining blockers without a blanket completion claim.

**Acceptance:** Every approved delivery claim is supported by evidence from its owning execution record; no missing or deferred slice is represented as complete.

**Non-goals:** New product scope, unapproved quantitative guarantees, required translations, or replacing owner evidence with package checkmarks.
