# Contributing

Development setup, tests and conventions are described in [docs/development.md](docs/development.md).

## Commit Messages

Every commit and every pull request title follows [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/). This applies to humans and agents alike. Versions and `CHANGELOG.md` are generated from these messages (see [Releases](docs/development.md#releases)).

```text
<type>[(scope)][!]: <description>

[body]

[footer: BREAKING CHANGE: …, Co-Authored-By: …]
```

| Type | Use for | Version bump |
| --- | --- | --- |
| `feat` | new behavior visible to users, operators or plugin authors | minor |
| `fix` | bug fix | patch |
| `perf` | performance improvement | patch |
| `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `style`, `revert` | everything else | none |

- A breaking change adds `!` after the type or scope, or a `BREAKING CHANGE: <what changed>` footer. Before 1.0.0 it bumps the minor version.
- The scope is optional and names the area, for example a package (`ingest`, `assignments`, `httpapi`) or `web`.
- Pull requests are squash-merged. The pull request title becomes the commit header on `main` and its description the body, so the title must describe the whole change; CI checks it.

Only a missing or unknown type and an empty description are rejected. Header length (100) and a trailing full stop give warnings; case and line lengths are not checked. The rules are in [commitlint.config.mjs](commitlint.config.mjs). Locally, commitlint skips the messages git generates itself (`Merge branch …`, `Revert "…"`, `fixup! …`), since they are squashed away; pull request titles get no such exception ([commitlint.pr-title.config.mjs](commitlint.pr-title.config.mjs)), so the title of a pull request from GitHub's revert button must be changed to `revert: …`. The `commit-msg` hook checks every commit locally once `npm ci` has run (see [Development](docs/development.md#prerequisites)).

## Documentation

When a change alters behavior, update the documentation in the same commit:

- `docs/` describes current behavior only. Change history belongs in commit messages and pull requests; decision records (ADRs), plans and execution evidence are kept in the separate development-process documents and never in `docs/`. Run `make docs-check` after editing Markdown to verify links, heading fragments and identifiers.
- [api/openapi/v1.yaml](api/openapi/v1.yaml) and [api/plugin/v2/plugin.proto](api/plugin/v2/plugin.proto) define the schemas. Prose in `docs/` links to them instead of repeating field lists.
- Every exported Go identifier in hand-written code has a doc comment that is understandable on its own, without referring to external planning documents. This includes the Ent schema definitions in `internal/persistence/ent/schema/`. Generated files (header `Code generated … DO NOT EDIT`, e.g. the Ent client under `internal/persistence/ent/`) are exempt and never edited by hand; change their source and regenerate.
