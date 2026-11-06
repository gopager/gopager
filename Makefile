.DEFAULT_GOAL := lint

# Tool versions are pinned here and run through "go run", so no global install is
# needed and local runs match CI. Bump deliberately; v1.64.8 is the last release of
# the golangci-lint v1 line, whose .golangci.yml format the repository uses.
GOLANGCI_LINT_VERSION := v1.64.8
GOVULNCHECK_VERSION := v1.1.4
GOTESTSUM_VERSION := v1.13.0
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
GOTESTSUM := go run gotest.tools/gotestsum@$(GOTESTSUM_VERSION)

# Test reports (JUnit XML and the go test JSON stream) written by test-integration and
# test-race, and read by test-slowest. STRESS_COUNT is the number of runs of test-stress.
TEST_REPORT_DIR ?= build/test-reports
STRESS_COUNT ?= 5
SLOWEST_THRESHOLD ?= 1s

# Ginkgo suites (one RunSpecs per package) abort with "go test -count=N", N > 1; test-stress
# skips them and repeats them with the Ginkgo CLI instead.
GINKGO_SUITE_TESTS := TestCredentials|TestMigrationBehavior|TestEvents
GINKGO_SUITE_PACKAGES := ./internal/identity ./internal/events ./internal/platform/postgres

# Pathspec of the files "go generate" rewrites: the Ent output, not its hand-written
# schema input.
GENERATED := internal/persistence/ent ':(exclude)internal/persistence/ent/schema'

.PHONY: fmt fmt-check vet golangci lint test test-integration test-race test-slowest test-stress generate-check vuln

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

# test-integration runs every test including the PostgreSQL ones (one Docker container per
# participating package), shuffled and uncached, and writes junit.xml and test.json to
# TEST_REPORT_DIR. It has no race detector; test-race is the CI tier. There are no automatic
# reruns: a failing test fails the run.
test-integration:
	mkdir -p $(TEST_REPORT_DIR)
	GOPAGER_INTEGRATION=1 $(GOTESTSUM) --format pkgname \
		--junitfile $(TEST_REPORT_DIR)/junit.xml --jsonfile $(TEST_REPORT_DIR)/test.json \
		-- -count=1 -shuffle=on -timeout=10m ./...

# test-race is test-integration with the race detector. CI runs this target.
test-race:
	mkdir -p $(TEST_REPORT_DIR)
	GOPAGER_INTEGRATION=1 $(GOTESTSUM) --format pkgname \
		--junitfile $(TEST_REPORT_DIR)/junit.xml --jsonfile $(TEST_REPORT_DIR)/test.json \
		-- -race -count=1 -shuffle=on -timeout=10m ./...

# test-slowest lists the tests slower than SLOWEST_THRESHOLD from the last test-integration
# or test-race run.
test-slowest:
	$(GOTESTSUM) tool slowest --jsonfile $(TEST_REPORT_DIR)/test.json --threshold $(SLOWEST_THRESHOLD)

# test-stress hunts flaky tests: STRESS_COUNT shuffled runs with the race detector. Ginkgo
# suites reject -count above 1, so they are skipped there and repeated by the Ginkgo CLI
# (--repeat=N reruns a suite N more times) with the same race detector.
test-stress:
	GOPAGER_INTEGRATION=1 go test -race -shuffle=on -count=$(STRESS_COUNT) -timeout=30m \
		-skip '^($(GINKGO_SUITE_TESTS))$$' ./...
	GOPAGER_INTEGRATION=1 go run github.com/onsi/ginkgo/v2/ginkgo --race --randomize-all \
		--repeat=$$(($(STRESS_COUNT) - 1)) --timeout=30m $(GINKGO_SUITE_PACKAGES)

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
