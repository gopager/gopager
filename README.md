# GoPager

GoPager receives pager transmissions from radio gateways, groups them into messages, assigns them to units, processes them with configurable workflows and plugins, and serves the results through an HTTP API.

## Requirements

- Go (version in [go.mod](go.mod))
- PostgreSQL
- Docker, for the integration test suite

## Getting Started

```sh
go build ./cmd/gopager
make                                                  # format check and go vet
go test ./...                                         # unit tests
GOPAGER_INTEGRATION=1 go test -race -count=1 ./...    # full suite against PostgreSQL containers
```

To configure and run a deployment, see [docs/operations.md](docs/operations.md).

## Documentation

- [Architecture](docs/architecture.md)
- [Operations](docs/operations.md)
- [HTTP API](docs/http-api.md)
- [Workflows](docs/workflows.md)
- [Plugins](docs/plugins.md)
- [Development](docs/development.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
