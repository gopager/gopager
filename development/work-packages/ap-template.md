# AP Template

[Execution rules](shared-rules.md) | [Package directory](../work-packages.md#revised-execution-path)

Every work package file `ap-NN-<slug>.md` uses the sections below in this order. The executing agent reads the AP file and the [execution rules](shared-rules.md) and nothing else up front, so the AP must be complete on its own:

- Rely only on completed earlier results, accepted decisions, the AP's own planning and questions to the user. Name each input with its producer and an exact handover or E-ID anchor; never link a whole execution record or a historical assessment.
- Assign every supporting change the tasks need (contracts, persistence and migrations, HTTP, runners, root wiring) to this AP's write scope and owned slices. Each piece of work belongs to exactly one AP; a REQ shared by several APs names each AP's part.
- Never rely on an undecided question, a definition gap or a later AP unless resolving it is this AP's own planning task. A product question the stories cannot answer goes to **Open user decisions**.
- An open AP contains no planned boundary crossing and no conditional scope ("if needed", "if excluded", "may", "or an extracted …", "do not assume a default"). Before an AP is approved, research every question first: answer it from code, ADRs, contracts or the stories where they decide it, and decide technical choices in **Planning before coding**. Only a product question no source decides stays in **Open user decisions**, written as a concrete question with options, a researched recommendation and the destination of the answer, so the agent asks it once at start and then works through without stopping.
- Name write-scope paths concretely (see [package layout](shared-rules.md#package-layout)); no "supporting" or "owner" placeholders.
- Link decisions to their ADR (`../decisions/<topic>.md#d-nnn`), not to the plan's moved-section redirects.
- Later APs are compatibility context only.

Changes to an AP file after it is approved follow the [scope-change procedure](shared-rules.md#scope-change).

## Template

```markdown
# AP-NN <Title>

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** every REQ ID this AP serves (ranges allowed), linked to its catalog section. For a REQ shared with another AP, state this AP's part and name the AP that owns the rest. The executing agent reads these story rows at start.
- **Inputs:** one line per producer: `[AP-xx handover](../execution/ap-xx.md#handover)`: what is used.
- **Owned contract slices:** C-ID file sections, F-case ranges and glossary terms this AP defines or changes.
- **Reference (read when needed):** fixed contract sections, decisions and architecture sections the tasks rely on.
- **Write scope:**
  - `path/` — what may change there
- **Compatibility context:** later APs and what they will consume. Their code is not required.

## Open User Decisions

Questions asked in one structured question at session start, each with the destination of the answer. "None" if there are none.

## Planning Before Coding

One paragraph per task: the questions to settle and the destination (C-ID section, F-cases, ADR) where the answer is recorded before coding.

## Tasks

- [ ] TNNN [AP-NN] <one-line task>
  - **Done when:** observable criteria and F-cases.

## Documentation to Update

Expected targets: development owners (C-ID sections, glossary, requirement mapping) and permanent documentation (`docs/` sections, OpenAPI, proto, Go doc comments).

## Verification

Commands and environments that produce the acceptance evidence.

## Handover

What is handed to which later AP.

## Non-goals

Explicitly excluded work.
```
