# GoPager Agent Guide

- Architecture, package map and domain model: [docs/architecture.md](docs/architecture.md)
- Build, test, lint, code generation, migrations: [docs/development.md](docs/development.md)
- HTTP API: [docs/http-api.md](docs/http-api.md), schemas in [api/openapi/v1.yaml](api/openapi/v1.yaml)
- Processing workflows and plugins: [docs/workflows.md](docs/workflows.md), [docs/plugins.md](docs/plugins.md)
- Configuration and runtime: [docs/operations.md](docs/operations.md)

Before finishing a change, run `make` and `go test ./...`. Update affected documentation, OpenAPI/proto files and Go doc comments in the same change, as described in [CONTRIBUTING.md](CONTRIBUTING.md#documentation).
