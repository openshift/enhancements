ifeq (/,${HOME})
GOLANGCI_LINT_CACHE=/tmp/golangci-lint-cache/
else
GOLANGCI_LINT_CACHE=${HOME}/.cache/golangci-lint
endif

# Run go lint against code
.PHONY: lint
lint: bin/golangci-lint
	GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) ./bin/golangci-lint run --exclude=G101

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.31.0
