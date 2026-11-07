# AP-17 Integration and Delivery Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** [REQ-to-package coverage](requirement-mapping.md) for every REQ of the approved delivery slice, including refinements REQ-135..137; see [delivery scope](../authoritative-plan.md#delivery-scope) and [open issues](../authoritative-plan.md#open-issues-and-risks).
- **Inputs:** foundation evidence (AP-01..06) is read from the producer rows of the [requirement mapping](requirement-mapping.md).
  - [AP-07 handover](../execution/ap-07.md#handover), including its D-139 follow-up (rejection-invariant test): intake evidence.
  - [AP-08 handover](../execution/ap-08.md#handover): master-data/correction PostgreSQL/HTTP evidence.
  - [AP-09 handover](../execution/ap-09.md#handover): provisioning/contact-loss evidence.
  - [AP-20 handover](../execution/ap-20.md#handover): Gateway mapping and rematch evidence.
  - [AP-10 handover](ap-10-query-live.md#handover): query/live/search/saved-view evidence, tests and versions.
  - [AP-11 handover](ap-11-events-delivery.md#handover): materialization/retry/rendering evidence.
  - [AP-12 handover](ap-12-plugin-host.md#handover): external Delivery and host/SDK evidence and commands.
  - [AP-13 handover](ap-13-processing-presentation-integration.md#handover): Reprocess/presentation evidence.
  - [AP-14 handover](ap-14-identity-lifecycle.md#handover): identity/cleanup evidence and commands.
  - [AP-15 handover](ap-15-frontend.md#handover): browser/build/assets evidence and the frontend build, lint and test commands.
  - [AP-16 handover](ap-16-operations.md#handover): retention/Full recovery evidence.
- **Owned contract slices:** the evidence mapping and acceptance procedure artifacts only; no contract section is defined here. Domain corrections remain with their owner.
- **Reference (read when needed):** [execution records](../execution/README.md), [repository layout and testing](../architecture.md#repository-layout-and-testing), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies), actual approved contract versions and F-cases.
- **Write scope:**
  - `development/execution/ap-17.md` — the evidence mapping and acceptance procedure; [requirement mapping](requirement-mapping.md) status
  - the documentation targets listed under Documentation to Update
- **Compatibility context:** none; this is the final package.

## Open User Decisions

None. REQ-134 was approved at the AP-09 start and REQ-133/138 are decided by [D-142](../decisions/09-process-and-documentation.md#d-142); T084 maps the recorded [delivery scope](../authoritative-plan.md#delivery-scope) rows without editing the delivery-scope row.

## Planning Before Coding

Produce the evidence mapping and acceptance procedure within the assigned evidence artifacts before verification.

T084: map every in-scope REQ to its producing AP/task, exact evidence artifact and verification type; label planned versus executed, fixture versus real integration/provider/browser/recovery checks, and any unverified claim. Destination: the evidence artifacts and the execution record.

T085: define the compatibility/build/API/SDK/asset checks, relevant version inputs, gate criteria and the exact conditions for reporting a blocker or completion. AP-17 builds no new tooling: OpenAPI/SDK checks are `make generate-check`, `make docs-check` and the commands named in the AP-12, AP-14 and AP-15 handovers; a claim without a producer command is recorded "not verified". Destination: the evidence artifacts and the execution record.

Defect handling: a failing check caused by another AP's code or documents is not repaired in AP-17; it is a boundary crossing under [shared rules](shared-rules.md), so ask the user once (structured question: fix under an approved D-ID in the owner's scope, or record the claim as failed) and continue the independent checks. Only AP-17's own artifacts are repaired.

Create no product behavior and replace no missing owner evidence. Report missing evidence against its producer and keep unverified or deferred claims explicitly unresolved.

## Tasks

- [ ] T084 [AP-17] Complete traceable REQ-to-test/operations evidence for the approved delivery slice, distinguishing fixtures from real integration/provider/browser/recovery checks.
  - **Done when:** every REQ of the approved slice (REQ-133/134/138 included, as approved by D-142 and at the AP-09 start), refinements REQ-135..137 and the C-01 failure rules moved to binding decisions by [D-139](../decisions/02-intake-and-grouping.md#d-139), maps to its producer task and executed acceptance cases; real-provider, PostgreSQL, browser and fault evidence are separate categories; REQs whose delivery-scope row is Later, Removed or Phase-1 proposal (for example REQ-127, REQ-129..130, REQ-132) are listed as "not in the approved slice" without a completion claim; no missing provider environment, domain implementation or producer test is backfilled here.
- [ ] T085 [AP-17] Verify selected contract/version compatibility, endpoint/SDK/build/asset integration, gates, deviations, and remaining blockers without a blanket completion claim.
  - **Done when:** `make generate-check`, `make docs-check`, AP-15's handover commands (build, lint, test) and the AP-12/AP-14 handover commands ran with exact runnable selectors from the completed producers; packaged assets and artifact/version agreement are verified; incompatible/missing inputs block the affected claim without weakened contracts or an invented success fallback; a failure caused by another AP is asked once under the defect handling above and never repaired here; every approved delivery claim is supported by evidence from its owning execution record and no missing or deferred slice is represented as complete.

## Documentation to Update

[Requirement mapping](requirement-mapping.md), the [package directory](../work-packages.md#revised-execution-path) row for AP-17 and the [open issues](../authoritative-plan.md#open-issues-and-risks) rows whose responsible package is AP-17 (P-18 evidence link), as the evidence changes their status.

## Verification

Run `make generate-check`, the commands named in the AP-12, AP-14 and AP-15 handovers (AP-15: build, lint, test) and verify packaged assets and artifact/version agreement. Record exact runnable selectors from the completed producers, then `make`, `go test ./...` and `make docs-check`. A claim without a producer command is recorded "not verified".

## Handover

The evidence mapping, gate results, deviations and remaining blockers recorded in the execution record; no later AP consumes it.

## Non-goals

New product scope, new tooling, repairing another AP's failing code or documents, unapproved quantitative guarantees, required translations, or replacing owner evidence with package checkmarks.
