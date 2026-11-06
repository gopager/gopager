# ADR: Operations and HTTP

Decisions of this topic in short form. IDs are stable and referenced from contracts, execution records and `docs/`. The approval narrative is not kept here (the history of `development/authoritative-plan.md` retains it). Technical behavior is defined by the linked owner documents, not by this summary. See the [index](README.md).

<a id="d-52"></a>
## D-52 Application-wide HTTP Support Decision

- **Status:** accepted
- **Date:** 2026-10-26
- **Decision:** Approved HTTP support for the entire application, including human authentication and sessions; extends D-45. Every GoPager application function, including browser/API routes, protected administrative routes, Gateway routes, human login, Recovery and Session use, must function over HTTP. GoPager does not require TLS termination, add an HTTP opt-in, or redirect HTTP to HTTPS. An Operator may terminate TLS at a reverse proxy; HTTP remains supported between that proxy and GoPager. Direct HTTP is also supported and carries no GoPager transport-security guarantee.
- **Owner detail:** [C-12](../../docs/http-api.md#basics), [C-06](../../docs/http-api.md#authentication)

<a id="d-101"></a>
## D-101 Processing Artifact-root Configuration

- **Status:** accepted
- **Date:** 2026-11-03
- **Decision:** Approved explicit optional local Processing artifact-root startup option. The rooted local artifact loader exists but lacks a product startup setting. Add explicit optional `--processing-artifact-root`, or choose a different binding. The user selected that option without a guessed default path. No downloads, automatic installation or deployment compiler.
- **Owner detail:** This record is the only current definition

<a id="d-133"></a>
## D-133 Command-ID reuse conflict code

- **Status:** accepted
- **Date:** 2026-11-06
- **Decision:** Command-ID reuse with different content is `IDEMPOTENCY_CONFLICT` (409, not retryable), replacing `REVISION_CONFLICT` in frozen slices. Every actor-bound command whose `commandId` is reused with different content returns `IDEMPOTENCY_CONFLICT` (HTTP 409, `retryable: false`). An identical repeat still returns the recorded result. `REVISION_CONFLICT` remains the code for a stale expected revision.
- **Owner detail:** This record is the only current definition
