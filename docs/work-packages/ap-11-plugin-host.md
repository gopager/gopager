# AP-11 Plugin Host and Initial Targets

[Execution path](../work-packages.md#revised-execution-path) | [Contract index](../domain-contracts.md) | [Shared execution rules](shared-rules.md)

## Session Context

- **Authority:** [session procedure](shared-rules.md#session-start), [D-97 ordered AP renumbering approval](../authoritative-plan.md#ordered-ap-renumbering-decision), [scope/approval record](../authoritative-plan.md#workflow-and-approval-gates).
- **Source scope:** Historical [source AP-11 T037/T038/T041/T042](source-ap-11-plugins.md); separately deferred T039/T040 remain gated and are not moved into this assignment.
- **Requirements and owners:** [REQ-084/089..090/106..107/129](../requirements-catalog.md#i-domain-events-extensions-and-deliveries), [C-06](../contracts/c-06-identity-authorization.md), [C-09](../contracts/c-09-delivery.md), [C-10](../contracts/c-10-plugin-host.md).
- **Owner:** Plugin host, with separate owners for each target; integration owns the shared SDK.
- **Prerequisites:** AP-06 through AP-10 and AP-03 runtime/ABI evidence; applicable AP-04 freezes and this slice's exact execution binding.
- **Write scope:** `internal/pluginhost/`, `sdk/`, initial Pushover/Webhook artifact sources, and their tests.

- [ ] T069 [AP-11] Implement the approved SDK/host lifecycle, compatibility, bounded invocation, capability, Secret, and I/O boundaries.
- [ ] T070 [AP-11] Implement the separately packaged Pushover and Webhook targets under their frozen contracts.
- [ ] T071 [AP-11] Verify controlled provider behavior, lifecycle during invocation, redaction, limits, and SDK/runtime compatibility.

**Acceptance:** Initial targets are separate, bounded, and explicit about provider outcomes and operator-approved capabilities.

**Non-goals:** Telegram/email, marketplace distribution, hostile-code isolation guarantees, or implicit host privileges.
