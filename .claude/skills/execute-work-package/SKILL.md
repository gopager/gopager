---
name: execute-work-package
description: "Execute an entire AP or work package to verified completion within its assigned scope. Use when starting, implementing, resuming or completing an AP. The order to execute is the start approval: work in the AP's own worktree and feature branch without asking for confirmation. Continue through all assigned tasks, verification, handover, a local security-review and code-review before every push of code, and an open pull request against main whose Copilot review has no annotations left; never stop before the AP is complete. Never commit AP work to main. Ask the user only for open user decisions, missing information, impossible or contradicting plans and scope crossings; leave items open only with the user's explicit agreement."
argument-hint: "AP ID or assignment path"
---

# Work Package Execution

The rules are in [development/work-packages/shared-rules.md](../../../development/work-packages/shared-rules.md). Read it and the assigned AP file (`development/work-packages/ap-NN-*.md`) first; read nothing else up front. Follow AP links section by section, only when the current task needs them.

The order to execute the AP is its start approval. Do not ask for confirmation to start, to follow the plan or to continue; ask only when the [rules](../../../development/work-packages/shared-rules.md) require a user decision.

1. [Session start](../../../development/work-packages/shared-rules.md#session-start): the user stories of the AP's REQ IDs, the AP's open user decisions (if any) in one structured question, the AP's worktree `.claude/worktrees/ap-NN-<slug>` on branch `ap-NN-<slug>`, worktree baseline, input verification, execution record.
2. Planning before coding: answer the AP's planning questions at the named destinations.
3. Per task: implement within the [scope](../../../development/work-packages/shared-rules.md#scope), verify, [maintain documentation](../../../development/work-packages/shared-rules.md#mandatory-documentation-maintenance), record evidence.
4. Anything outside the scope: [scope change](../../../development/work-packages/shared-rules.md#scope-change) — classify, analyze impact, request approval, then apply it yourself.
5. [Continue](../../../development/work-packages/shared-rules.md#mandatory-continuation-and-stop-conditions) until the AP is complete. **Never stop with open items on your own.** When something blocks an item (missing information, a plan that is impossible or needs a decision), ask the user right away (structured question) and keep working on independent items; an item stays open only if the user explicitly agrees.
6. [Handover](../../../development/work-packages/shared-rules.md#handover): `make`, `go test ./...`, `make docs-check`, diff within scope, handover section complete.
7. [Pull request and review](../../../development/work-packages/shared-rules.md#pull-request-and-review): **before every push that contains a code commit**, run the `security-review` skill and the `code-review` skill with `xhigh ap-NN-<slug> --max-findings all` (never `--comment`, `--fix`, `max` or `ultra`) to find as much as possible before Copilot does. Check and fix their findings like Copilot annotations and run both again after every fix, until a run of both reports no open finding; record rejected findings with the reason in the evidence (they do not count as open). Only then push the branch, open the pull request against `main`, request Copilot's review if it was not requested automatically and wait for it. Check every annotation against the REQ user stories and acceptance criteria and the AP's tasks, fix the valid ones, answer the rest with the reason, repeat the whole local review loop on the fixes, push and re-request the review when the push contains code (a Markdown-only commit needs no new review). Repeat until Copilot's review of the last code commit has no annotations and the required checks pass on the head commit.
8. Report the pull request link, task states, evidence and open points concisely. The user merges the pull request.

**Never commit AP work to `main`** and never merge or push it there, locally or remotely; never merge the AP's pull request yourself.
