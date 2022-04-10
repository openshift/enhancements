ifeq (/,${HOME})
GOLANGCI_LINT_CACHE=/tmp/golangci-lint-cache/
else
GOLANGCI_LINT_CACHE=${HOME}/.cache/golangci-lint
endif

.PHONY: help
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  make \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^[A-Z_-]+.*=/ { varend=index($$0, "?") - 1; if ( varend > 0 ) { helpstart=index($$0, "#") + 1; printf "   var \033[36m%-15s\033[0m %s\n", substr($$0, 0, varend), substr($$0, helpstart) } } /^##@/ { printf "\n  \033[1m%s\033[0m\n", substr($$0, 5) } END { printf "\n" }' $(MAKEFILE_LIST)

.PHONY: lint
lint: bin/golangci-lint  ## Run go lint against code
	GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) ./bin/golangci-lint run --exclude=G101

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.31.0

test:  ## Run tests
	go test ./...

cover:  ## Run tests with code coverage output
	go test -coverprofile=cover.out $(GO_TEST_FLAGS) ./...
	go tool cover -func=cover.out
