---
name: execute-work-package
description: "Execute an entire AP or work package to verified completion within its assigned scope. Use when starting, implementing, resuming or completing an AP. Continue through all assigned tasks, verification and handover; never stop before the AP is complete. Ask the user whenever information is missing; leave items open only with the user's explicit agreement."
argument-hint: "AP ID or assignment path"
---

# Work Package Execution

The rules are in [development/work-packages/shared-rules.md](../../../development/work-packages/shared-rules.md). Read it and the assigned AP file (`development/work-packages/ap-NN-*.md`) first; read nothing else up front. Follow AP links section by section, only when the current task needs them.

1. [Session start](../../../development/work-packages/shared-rules.md#session-start): the user stories of the AP's REQ IDs, start approval and the AP's open user decisions in one structured question, worktree baseline, input verification, execution record.
2. Planning before coding: answer the AP's planning questions at the named destinations.
3. Per task: implement within the [scope](../../../development/work-packages/shared-rules.md#scope), verify, [maintain documentation](../../../development/work-packages/shared-rules.md#mandatory-documentation-maintenance), record evidence.
4. Anything outside the scope: [scope change](../../../development/work-packages/shared-rules.md#scope-change) — classify, analyze impact, request approval, then apply it yourself.
5. [Continue](../../../development/work-packages/shared-rules.md#mandatory-continuation-and-stop-conditions) until the AP is complete. **Never stop with open items on your own.** When something blocks an item, ask the user right away (structured question) and keep working on independent items; an item stays open only if the user explicitly agrees.
6. [Handover](../../../development/work-packages/shared-rules.md#handover): `make`, `go test ./...`, `make docs-check`, diff within scope, handover section complete.
7. Report task states, evidence and open points concisely.
