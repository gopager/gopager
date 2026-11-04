# AP-11 Plugin Host and Initial Targets

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-11 T037/T038/T041/T042](source-ap-11-plugins.md); separately deferred T039/T040 remain gated and are not moved into this assignment.
- **Requirements and owners:** [REQ-084/089..090/106..107/129](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [C-06](../contracts/c-06-identity-authorization.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Owner:** Plugin host, with separate owners for each target; integration owns the shared SDK.
- **Architecture:** [technology decisions](../architecture.md#technology-decisions), [native-plugin alternative review](../architecture.md#native-plugin-alternative-review), [allowed dependencies](../architecture.md#allowed-dependencies).
- **Prerequisites:** Verified AP-06 through AP-10 handovers and AP-03 standalone runtime/ABI evidence that is already available and directly required by T069/T071; only applicable contract/decision slices already fixed by earlier work. Any C-10 detail needed by these tasks that is not already fixed is planned and defined in this AP, not treated as an external freeze prerequisite. Do not rely on AP-dependent or otherwise deferred AP-03 evidence.
- **Write scope:** `internal/pluginhost/`, `sdk/`, initial Pushover/Webhook artifact sources, and their tests.

## Planning Before Coding

Complete this planning as the first part of T069-T071; it is required AP work, not a separate prerequisite or permission gate. It does not waive any implementation gate recorded in the plan. Record contract decisions in C-10 and case expectations at their F-case owners; track the work within those existing task IDs:

- **Later-consumer planning context (not prerequisites):** [AP-12 Processing Presentation and Integration](ap-12-processing-presentation-integration.md), [AP-14 Frontend](ap-14-frontend.md), [AP-15 Operations](ap-15-operations.md), and [AP-16 Integration and Delivery Evidence](ap-16-integration-evidence.md). Use these to keep the installed-step host boundary, target configuration, packaging and evidence compatible; do not implement their workflows, operations or sign-off early.
- For T069, define the SDK/manifest and host lifecycle surface, compatibility/version rules, capability and Secret boundaries, invocation/resource limits, cancellation, I/O, redaction and explicit failure outcomes.
- For T070, define the Pushover and Webhook target inputs, configuration/Secret use, request/response and receipt semantics, timeout/retry behavior, and SSRF/DNS/redirect controls against the approved target requirements.
- For T071, bind controlled-provider and lifecycle/redaction/limit cases to the assigned fixtures and available AP-03 evidence. Do not claim hostile-code isolation or rely on Telegram/email or later provider work.

- [ ] T069 [AP-11] Implement the approved SDK/host lifecycle, compatibility, bounded invocation, capability, Secret, and I/O boundaries.
- [ ] T070 [AP-11] Implement the separately packaged Pushover and Webhook targets under their frozen contracts.
- [ ] T071 [AP-11] Verify controlled provider behavior, lifecycle during invocation, redaction, limits, and SDK/runtime compatibility.

**Acceptance:** Initial targets are separate, bounded, and explicit about provider outcomes and operator-approved capabilities.

**Non-goals:** Telegram/email, marketplace distribution, hostile-code isolation guarantees, or implicit host privileges.
