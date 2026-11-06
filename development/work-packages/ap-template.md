# AP Template

[Execution rules](shared-rules.md) | [Package directory](../work-packages.md#revised-execution-path)

Every work package file `ap-NN-<slug>.md` uses the sections below in this order. The executing agent reads the AP file and the [execution rules](shared-rules.md) and nothing else up front, so the AP must be complete on its own:

- Rely only on completed earlier results, accepted decisions, the AP's own planning and questions to the user. Name each input with its producer and an exact handover or E-ID anchor; never link a whole execution record or a historical assessment.
- Assign every supporting change the tasks need (contracts, persistence and migrations, HTTP, runners, root wiring) to this AP's write scope and owned slices.
- Link decisions to their ADR (`../decisions/<topic>.md#d-nnn`), not to the plan's moved-section redirects.
- Later APs are compatibility context only.

Changes to an AP file after it is approved follow the [scope-change procedure](shared-rules.md#scope-change).

## Template

```markdown
# AP-NN <Title>

[Execution path](../work-packages.md#revised-execution-path) | [Execution rules](shared-rules.md)

## Session Context

- **Requirements:** REQ links to the catalog sections, with any qualification.
- **Inputs:** one line per producer: `[AP-xx handover](../execution/ap-xx.md#handover)`: what is used.
- **Owned contract slices:** C-ID file sections, F-case ranges and glossary terms this AP defines or changes.
- **Reference (read when needed):** fixed contract sections, decisions and architecture sections the tasks rely on.
- **Write scope:**
  - `path/` — what may change there
- **Compatibility context:** later APs and what they will consume. Their code is not required.

## Open User Decisions

Questions asked together with the start approval, each with the destination of the answer. "None" if there are none.

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
