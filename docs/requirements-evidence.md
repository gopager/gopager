# Source Provenance and Documentation Evidence

This file owns source observations, reuse assessment and non-AP documentation checks. Product decisions are routed by [D-ID](authoritative-plan.md#confirmed-decisions); actual AP validation/results belong to [execution records](execution/README.md). Do not copy decision narratives, task progress or contract rules here.

## Source Inventory

| Source / observation date | Provenance coverage | Limits |
| --- | --- | --- |
| User-provided consolidated planning file, 2026-10-23: product plan 2.2 / catalog 1.8 | Origin of stable REQ-001..132 and initial technology rationales | Historical external input, not a second maintained specification; no general legacy-code validation |
| User discussion, 2026-10-23 | D-01/D-02 and initial D-17..D-19 provenance | Later decisions amend the initial discussion; use current owner links |
| Documentation creation/maintenance requests, 2026-10-23..25 | Authorization for repository documentation | Not scope, spike or product implementation approval |
| Language/naming discussions, 2026-10-24..25 | D-20/D-21 provenance | Current terminology and rename sequence are in the [glossary](glossary.md#rename-log) |
| AP-01 clarification and later impact review, 2026-10-25 | D-17/D-18 and C-01..C-03 inputs | Intermediate policy candidates were superseded; current contract only |
| AP-02 clarification, 2026-10-25 | D-22..D-31 provenance | Supersession is indexed in the plan; the contracts own final behavior |
| Documentation execution-policy request, 2026-10-25 | Immutable-current-AP workflow and workspace agent entry | Rules are owned by [shared execution procedure](work-packages/shared-rules.md), not this observation |
| [47 CFR 11.31](https://www.ecfr.gov/current/title-47/chapter-I/subchapter-A/part-11/subpart-B/section-11.31), read in prior planning on 2026-10-23, then current through 2026-10-21 | EAS source reference for [C-02](contracts/c-02-protocol-payload.md) | Not re-fetched here, not a complete Protocol implementation study |

The external German source is historical provenance, not a parallel translated specification. Detailed obsolete policy/naming explorations remain in Git history; they are deliberately absent from current implementation context.

## Protocol Source Checks

The AP-01 historical record reports checks against Wikipedia (POCSAG/EAS) and multimon-ng sources, including correction of numeric glyphs for 0xE/0xF. Exact source revisions and a reproducible check command were not recorded. Do not treat this as complete protocol certification. FLEX type/capcode verification remains assigned by [P-08](authoritative-plan.md#open-issues-and-risks).

## Legacy Source Observation

The prior targeted inspection recorded PagerMon tag `v1.6.0-beta`, commit `c8a934d67100e86789f9d6ee844cb2d47aa44c7f`, `server/essen/index.js` and desktop/mobile card templates. Observed: conditional structured location/destination sections, raw-text fallback and custom parser logic.

This supersedes the initial not-found report. No code adoption, complete historical comparison or functional parity is established. This review preserves the prior source/ref observation; it did not independently rerun that inspection.

Potential later research sources are PagerMon, pagermon-ingest-core and pagermon-ingest-adapter-multimon in their separate repositories. They are not GoPager runtime dependencies or mandatory migration targets.

## Evidence Assessment for Deliberate Reuse

For each proposed adoption, add a record here identifying affected REQ/C version, repository/ref, path/symbol, observed behavior, executed tests and missing invariants. Classify direct fit, partial fit, idea/test-case only or unsuitable, with rationale. Adoption approval belongs in the plan. A source reference alone is neither a passing target test nor permission to reuse.

No adoption assessment is approved yet.

## Documentation Reviews

Historical AP documentary checks/outcomes are in [AP-01 E-01](execution/ap-01.md#e-01-imported-documentary-outcome) and [AP-02 E-01](execution/ap-02.md#e-01-imported-documentation-checks), not duplicated here.

### E-DOC-01 Entry-point Cleanup

Date: 2026-10-25, before the immutable-AP restructuring. A local Python standard-library check resolved 773 relative Markdown links/heading anchors across 43 Markdown files and checked all 16 AP session routes, REQ sections and contract gates. Compared with Git `3be71b4`, 132 catalog/mapping rows, 54 task IDs/check states, D/P rows and 61 F-cases were preserved. `git diff --check` passed.

The temporary checker and full tool/runtime environment were not persisted. These are recorded structural/textual observations, not a renderer/Mermaid check, fresh-agent session trial or runtime evidence.

### E-DOC-02 Authority and Immutable-AP Review

Compared the documentation worktree with Git `3be71b4` after the ownership/execution restructuring. Local tools: Python 3.14.0 standard library and Git 2.54.0 (Apple Git-157) on macOS. Procedure: resolve relative Markdown targets, heading slugs and explicit anchors; compare stable IDs, task states, requirement mapping, delivery classifications, vocabulary and planned cases against the baseline.

- 992 local links/anchors across 47 Markdown files resolved; all 16 AP session routes include their relevant REQ sections, owner contracts and shared start/approval context.
- All 132 REQ IDs, user-goal/provenance cells and mapping rows, 54 task IDs/check states, D/P/TECH IDs, 132 delivery classifications and 61 planned F-cases were preserved. Six acceptance rows intentionally replace repeated technical detail with owner-contract links.
- Glossary term labels, prohibited terms and rename history were preserved; interface identifier spellings were checked at their receiving contracts.
- Root/direct-document responsibilities and execution instructions were reviewed locally. Workspace agent entry is link-only; historical AP execution imports explicitly qualify missing original environments/commands and do not claim new runtime evidence or approval.
- `git diff --check` passed.

The temporary checker was not persisted. These checks establish structural consistency and reviewed ownership, not automatic prevention of future duplication, a fresh-agent session trial, rendered diagrams or implemented product behavior. No runtime tests were executed for this documentation-only change.
