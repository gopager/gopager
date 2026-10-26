.DEFAULT_GOAL := lint

.PHONY: fmt fmt-check lint

fmt:
	gofmt -w cmd internal

fmt-check:
	@files="$$(gofmt -l cmd internal)" || exit $$?; \
	if [ -n "$$files" ]; then \
		printf 'Go formatting required; run make fmt:\n%s\n' "$$files"; \
		exit 1; \
	fi

lint: fmt-check
	go vet ./...
