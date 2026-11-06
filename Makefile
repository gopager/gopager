.DEFAULT_GOAL := lint

# Tool versions are pinned here and run through "go run", so no global install is
# needed and local runs match CI. Bump deliberately; golangci-lint v2 is required
# for Go 1.27 (v1 cannot read its export data).
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION := v1.9.0
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# Pathspec of the files "go generate" rewrites: the Ent output, not its hand-written
# schema input.
GENERATED := internal/persistence/ent ':(exclude)internal/persistence/ent/schema'

.PHONY: fmt fmt-check vet golangci lint test test-integration generate-check vuln

fmt:
	gofmt -w cmd internal

fmt-check:
	@files="$$(gofmt -l cmd internal)" || exit $$?; \
	if [ -n "$$files" ]; then \
		printf 'Go formatting required; run make fmt:\n%s\n' "$$files"; \
		exit 1; \
	fi

vet:
	go vet ./...

golangci:
	$(GOLANGCI_LINT) run ./...

# lint is the full static-analysis tier: formatting, go vet and golangci-lint.
lint: fmt-check vet golangci

# test is the quick tier: no Docker, database tests are skipped.
test:
	go test ./...

# test-integration runs every test including the PostgreSQL ones (one Docker
# container per participating package) with the race detector.
test-integration:
	GOPAGER_INTEGRATION=1 go test -race -count=1 -timeout=20m ./...

# generate-check fails when the committed generated code differs from what the
# generators produce, including generated files that are missing from git.
generate-check:
	go generate ./...
	@git diff --exit-code -- $(GENERATED) || { echo 'Generated code is stale; run go generate ./... and commit.'; exit 1; }
	@untracked="$$(git ls-files --others --exclude-standard -- $(GENERATED))"; \
	if [ -n "$$untracked" ]; then \
		printf 'Untracked generated files; run go generate ./... and commit:\n%s\n' "$$untracked"; \
		exit 1; \
	fi

# vuln reports known vulnerabilities reachable from the code and the Go toolchain.
# It needs network access to the vulnerability database.
vuln:
	$(GOVULNCHECK) ./...
