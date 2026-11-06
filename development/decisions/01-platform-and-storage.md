# ADR: Platform and storage

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-01"></a>
## D-01 Confirmed

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed: product identity is GoPager in its dedicated repository; no new naming round. Availability checks are not claimed.
- **Owner detail:** This row; [source provenance](../requirements-evidence.md#source-inventory)

<a id="d-02"></a>
## D-02 Technology direction

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed technology direction; not reopened by implementation experiments.
- **Owner detail:** [TECH-01](../architecture.md#technology-decisions)

<a id="d-03"></a>
## D-03 Shared Lite/Full application direction

- **Status:** superseded by D-135 (in part: the Lite runtime and its SQLite adapter; the shared-application direction stands)
- **Date:** 2026-10-25
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Confirmed shared Lite/Full application direction.
- **Owner detail:** [TECH-02 and profile topology](../architecture.md#operating-profiles)

<a id="d-04"></a>
## D-04 Full authority/HA boundary

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed Full authority/HA boundary.
- **Owner detail:** [C-12](../contracts/c-12-persistence-operations.md)

<a id="d-13"></a>
## D-13 Measurement/acceptance policy

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed measurement/acceptance policy.
- **Owner detail:** [REQ-120](../requirements-catalog.md#l-operations-data-volume-and-evolution), [REQ-127](../requirements-catalog.md#m-additional-domain-capabilities), [AP-03 authorization procedure](../work-packages/ap-03-technical-validation.md)

<a id="d-14"></a>
## D-14 Conditional Lite durability allowance

- **Status:** superseded by D-135 (in part: the crash-loss allowance of a SQLite Lite)
- **Date:** 2026-10-25
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Confirmed conditional Lite durability allowance.
- **Owner detail:** [REQ-116](../requirements-catalog.md#l-operations-data-volume-and-evolution), [C-01](../contracts/c-01-transmission-acceptance.md), [C-12](../contracts/c-12-persistence-operations.md)

<a id="d-15"></a>
## D-15 Retention default and exhaustion behavior

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed retention default and exhaustion behavior.
- **Owner detail:** [C-12](../contracts/c-12-persistence-operations.md)

<a id="d-23"></a>
## D-23 Retention/independent-work boundary

- **Status:** accepted
- **Date:** 2026-10-25
- **Decision:** Confirmed retention/independent-work boundary.
- **Owner detail:** [C-12 retention](../contracts/c-12-persistence-operations.md), [C-09 input/dependencies](../contracts/c-09-delivery.md), [C-08 receipts](../contracts/c-08-events-jobs.md)

<a id="d-31"></a>
## D-31 Application Backup/Restore removed

- **Status:** superseded by D-135 (in part: the profile transition; the removal of application Backup/Restore stands)
- **Date:** 2026-10-25
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Application Backup/Restore removed; supersedes former application-lifecycle proposals.
- **Owner detail:** [Reserved REQ-126](../requirements-catalog.md#m-additional-domain-capabilities), [C-12 Operator boundary](../../docs/operations.md#backup-and-restore-responsibility), [C-13 personal-data boundary](../contracts/c-13-account-lifecycle.md)

<a id="d-32"></a>
## D-32 AP-04 Scope Decision

- **Status:** superseded by D-135 (in part: runnable-Lite timing; Lite is a compose profile)
- **Date:** 2026-10-25
- **Superseded by:** [D-135](10-foundation-redesign.md#d-135)
- **Decision:** Approved AP-04 delivery amendment: Lite architecture preparation now, runnable Lite later; qualifies D-03 delivery timing without replacing the shared application direction.
- **Owner detail:** This record is the only current definition

<a id="d-35"></a>
## D-35 AP-04 Foundation Freeze Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved AP-04 foundation-only freeze and AP-05 binding; execution waits for an explicit AP-05 start, all other technical slices remain gated. Freeze only [C-12 foundation 1.0](../../docs/architecture.md#cross-cutting-mechanisms), [C-01 intake-commit 1.0](../../docs/architecture.md#intake-one-transaction), [C-08 transaction participation 1.0](../../docs/architecture.md#transactions-and-the-store-abstraction), [C-07 platform-errors 1.0](../../docs/http-api.md#errors), and the referenced [shared foundation conventions](../../docs/architecture.md#cross-cutting-mechanisms). Bind [glossary G-1.0](../glossary.md) without changing existing meanings and approve the [TECH foundation tuple](../architecture.md#ap-05-foundation-version-selection). Exact future domain schemas, Gateway HTTP API, search, auth/account implementation, job engine, plugin ABI, Delivery and frontend contracts remain unfrozen. There is no whole-C-ID or whole-product approval.
- **Owner detail:** This record is the only current definition

<a id="d-36"></a>
## D-36 Repository-wide Library Policy Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved repository-wide library-first policy and review of in-progress AP-05. The normative rule lives only in [Library reuse](../README.md#library-reuse). Avoid unnecessary reinvention without importing disproportionate or unsuitable dependencies. The instruction authorizes that policy section, its agent-entry link, this approval record and AP-05's technical review/corrections. It does not select a new library or silently override D-35's frozen behavior/versions.
- **Consequences:** [AP-05 library assessment](../architecture.md#ap-05-library-assessment) owns technical alternatives and exception rationale; [AP-05 execution](../execution/ap-05.md) records actual lookup limitations, corrections and verification. All later work packages consume the repository-wide rule through the existing documentation authority/agent entry, not copied task amendments.
- **Owner detail:** [policy owner](../README.md#library-reuse)

<a id="d-37"></a>
## D-37 AP-05 Migration Revision Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved central Goose migration sequence and foundation 1.1 replacement binding. Retain the D-35 fixed migration format, or revise it for controlled schema/data migrations and responsible historical corrections. The user explicitly permits format changes and prohibits checksum validation. An intermediate answer selected independent namespaced sequences without a graph. **Subsequent user clarification, 2026-10-26, supersedes that simplification:** if migrations are module-separated, cross-module prerequisites must be possible, including a Message change requiring a prior Gateway change; if this is not sensible at reasonable cost, retain central migrations.
- **Owner detail:** This record is the only current definition

<a id="d-38"></a>
## D-38 AP-05 Data Access Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved Ent native Privacy/Filter for required domain consumers; no Atlas/checksum authoring workflow.
- **Owner detail:** [selected stack](../architecture.md#domain-data-access-selection)

<a id="d-41"></a>
## D-41 Repository Structure and Test Infrastructure Follow-up

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved targeted repository structure, Testcontainers PostgreSQL fixtures, Ginkgo/Gomega scenario suites and formatting/linting entry points.
- **Owner detail:** This record is the only current definition

<a id="d-63"></a>
## D-63 Operator Start and Bundled Dependency Clarification

- **Status:** accepted
- **Date:** 2026-11-01
- **Decision:** Operator starts one application; application-managed children permitted; required runtimes supplied. Replace the literal single-process interpretation with the [REQ-115 bundled single-start boundary](../requirements-catalog.md#l-operations-data-volume-and-evolution), including application-managed children and supplied runtime dependencies. Retaining the exact process limit or allowing Operator-installed/separately orchestrated module dependencies would contradict this clarification. No language/runtime is selected merely because children are now allowed.
- **Owner detail:** This record is the only current definition

<a id="d-64"></a>
## D-64 Bundled Child Feasibility Proposal

- **Status:** accepted (evidence only)
- **Date:** 2026-11-01
- **Decision:** Approved bounded bundled-child feasibility probe; evidence only.
- **Consequences:** Evidence is limited to Linux/arm64 and the effective resource boundaries observed; a data/address limit is not automatically the entire process-RSS cap or portable to other hosts. Failure may show the child option needs a different enforcement mechanism, not permission to raise limits or replace hard controls with `GOMEMLIMIT`. Record exact commands/hashes/results in an AP-04 addendum.
- **Owner detail:** This record is the only current definition

<a id="d-87"></a>
## D-87 AP-12 River Queue Evaluation

- **Status:** accepted (evidence only)
- **Date:** 2026-11-02
- **Decision:** Isolated River prototype only; no production selection. The user approved this prototype only. D-75 and the current AP-12 implementation binding remain in force unless a separate product/contract decision approves a change. Prototype results belong in the AP-12 execution record and architecture assessment. Any production adoption requires a separate explicit decision and any necessary C-08/AP-12/AP-06 binding or downstream amendments.
- **Owner detail:** This record is the only current definition

<a id="d-128"></a>
## D-128 Owning-migration consolidation

- **Status:** accepted
- **Date:** 2026-11-05
- **Decision:** Unreleased migrations change only tables they create; cross-file edits squashed into owning migrations. Until a release fixes migration history, each table's complete definition (columns, defaults, checks, indexes) lives in the migration that creates it; later files create only their own tables and may reference earlier ones. Upgrade-only guards and backfills for unreleased states are removed. The focused per-owner file layout is kept rather than a single monolith, because it preserves owner boundaries and the prefix-upgrade test before `00008_fact_transport.sql`. Applied result: `organizations`, `unit_templates`, Unit Organization/settings, `command_receipts.result_items` and the full correction-work columns moved into `00004_current_assignments.sql`; the `identity_grants.field_names` default into `00001_identity.sql`; `transmissions_unpublished` into `00009_transmission_intake.sql`; correction-owned tables are now `00010_historical_correction.sql`.
- **Consequences:** This revises unreleased AP-05..AP-08 migration artifacts, not their contracts or fixed behavior. Earlier execution evidence remains as observed; current results are in the [AP-08 record](../execution/ap-08.md). Until the first release, a later package that needs a column, index or constraint on an existing table edits that table's owning migration file (for example `00009_transmission_intake.sql`) and regenerates Ent; this applies to AP-09..AP-16 and to the D-135 redesign.
- **Owner detail:** This record is the only current definition
