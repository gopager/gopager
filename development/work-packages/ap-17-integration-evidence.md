# AP-17 Integration and Delivery Evidence

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** [REQ-to-package coverage](requirement-mapping.md) for every REQ of the approved delivery slice, including explicit disposition of REQ-133/134/138 and refinements REQ-135..137; see [delivery scope](../authoritative-plan.md#delivery-scope) and [open issues](../authoritative-plan.md#open-issues-and-risks).
- **Inputs:** (use the [REQ producer mapping](requirement-mapping.md) to select the exact handovers, evidence and fixed versions for the approved slice; producer records give actual versions, paths, commands and limitations; missing producer evidence is a handover gap, not work to backfill or infer; verify `development/execution/ap-NN.md#handover` exists at start for each open AP)
  - [AP-08 handover](../execution/ap-08.md#handover): master-data/correction PostgreSQL/HTTP evidence.
  - [AP-09](ap-09-gateway-network.md): provisioning/contact-loss evidence.
  - [AP-10](ap-10-query-live.md): query/live/search/saved-view disposition evidence.
  - [AP-11](ap-11-events-delivery.md): materialization/retry/rendering evidence.
  - [AP-12](ap-12-plugin-host.md): actual external Delivery and host/SDK evidence.
  - [AP-13](ap-13-processing-presentation-integration.md): Reprocess/presentation evidence.
  - [AP-14](ap-14-identity-lifecycle.md): identity/cleanup evidence.
  - [AP-15](ap-15-frontend.md): browser/build/assets evidence and the recorded frontend build/test commands.
  - [AP-16](ap-16-operations.md): retention/Full recovery evidence.
- **Owned contract slices:** the evidence mapping and acceptance procedure artifacts only; no contract section is defined here. Domain corrections remain with their owner.
- **Reference (read when needed):** [execution records](../execution/README.md), [repository layout and testing](../architecture.md#repository-layout-and-testing), [supporting responsibilities](../architecture.md#supporting-responsibilities), [allowed dependencies](../architecture.md#allowed-dependencies), actual approved contract versions and F-cases.
- **Write scope:**
  - `development/execution/ap-17.md` — the evidence mapping and acceptance procedure; [requirement mapping](requirement-mapping.md) status
  - shared build/CI, API/SDK compatibility checks and asset/package integration in the repository root, `cmd/gopager/`, `pluginsdk/`, `web/` and packaging — only to integrate already completed behavior; domain corrections remain with their owner
- **Compatibility context:** none; this is the final package.

## Open User Decisions

None. The disposition of REQ-133/138 is decided at the start of [AP-10](ap-10-query-live.md#open-user-decisions) and that of REQ-134 at the start of [AP-09](ap-09-gateway-network.md#open-user-decisions); T084 maps the recorded [delivery scope](../authoritative-plan.md#delivery-scope) rows. A row still unresolved at start is an input gap ([scope change](shared-rules.md#scope-change)), not a decision of this AP.

## Planning Before Coding

Produce the evidence mapping and acceptance procedure within the assigned evidence artifacts before verification.

T084: map every in-scope REQ to its producing AP/task, exact evidence artifact and verification type; label planned versus executed, fixture versus real integration/provider/browser/recovery checks, and any unverified claim. Destination: the evidence artifacts and the execution record.

T085: define the compatibility/build/API/SDK/asset checks, relevant version inputs, gate criteria, deviation handling and the exact conditions for reporting a blocker or completion. Destination: the evidence artifacts and the execution record.

Do not create product behavior or replace missing owner evidence. Report missing evidence against its producer and keep unverified or deferred claims explicitly unresolved.

## Tasks

- [ ] T084 [AP-17] Complete traceable REQ-to-test/operations evidence for the approved delivery slice, distinguishing fixtures from real integration/provider/browser/recovery checks.
  - **Done when:** every approved REQ, including the explicit disposition of REQ-133/134/138 and refinements REQ-135..137, maps to its producer task and executed acceptance cases; real-provider, PostgreSQL, browser and fault evidence are separate categories; no missing provider environment, domain implementation or producer test is backfilled here.
- [ ] T085 [AP-17] Verify selected contract/version compatibility, endpoint/SDK/build/asset integration, gates, deviations, and remaining blockers without a blanket completion claim.
  - **Done when:** actual Go build/vet/test, selected OpenAPI/SDK compatibility commands and AP-15's recorded frontend build/test commands ran with exact runnable selectors from the completed producers; packaged assets and artifact/version agreement are verified; incompatible/missing inputs block the affected claim without weakened contracts or an invented success fallback; every approved delivery claim is supported by evidence from its owning execution record and no missing or deferred slice is represented as complete.

## Documentation to Update

- Root [README.md](../../README.md) `Getting Started` and [docs/development.md](../../docs/development.md) `Build`, `Tests`, `Code Generation` where the integrated build/CI/SDK/asset commands change.
- [docs/operations.md](../../docs/operations.md) `Build` where packaging or artifact/version agreement changes; [docs/plugins.md](../../docs/plugins.md) `6. Packaging and installation` where SDK compatibility checks change.
- [Requirement mapping](requirement-mapping.md), the [package directory](../work-packages.md#revised-execution-path), the [delivery scope](../authoritative-plan.md#delivery-scope) row for REQ-133/134/138 and the [open issues](../authoritative-plan.md#open-issues-and-risks) register as the evidence changes their status.

## Verification

Run actual Go build/vet/test, selected OpenAPI/SDK compatibility commands and AP-15's recorded frontend build/test commands; verify packaged assets and artifact/version agreement. Record exact runnable selectors from the completed producers, then `make`, `go test ./...` and `make docs-check`.

## Handover

The evidence mapping, gate results, deviations and remaining blockers recorded in the execution record; no later AP consumes it.

## Non-goals

New product scope, unapproved quantitative guarantees, required translations, or replacing owner evidence with package checkmarks.
