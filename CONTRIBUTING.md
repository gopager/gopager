# Contributing

Development setup, tests and conventions are described in [docs/development.md](docs/development.md).

## Documentation

When a change alters behavior, update the documentation in the same commit:

- `docs/` describes current behavior only. Change history, decision records and plans belong in commit messages and pull requests.
- [api/openapi/v1.yaml](api/openapi/v1.yaml) and [api/plugin/v2/plugin.proto](api/plugin/v2/plugin.proto) define the schemas. Prose in `docs/` links to them instead of repeating field lists.
- Every exported Go identifier in hand-written code has a doc comment that is understandable on its own, without referring to external planning documents. This includes the Ent schema definitions in `internal/persistence/ent/schema/`. Generated files (header `Code generated … DO NOT EDIT`, e.g. the Ent client under `internal/persistence/ent/`) are exempt and never edited by hand; change their source and regenerate.
