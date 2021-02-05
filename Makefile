.PHONY: help
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: image
image:  ## Build local container image
	podman image build -f ./hack/Dockerfile.markdownlint --tag enhancements-markdownlint:latest

lint:  ## run the markdown linter
	podman run \
		--rm=true \
		--env RUN_LOCAL=true \
		--env VALIDATE_MARKDOWN=true \
		-v $$(pwd):/workdir:Z \
		enhancements-markdownlint:latest

REPORT_FILE=this-week/$(shell date +%F).md
.PHONY: report report-gen
report: report-gen lint  ## run weekly newsletter report tool

report-gen:
	(cd ./tools; go run ./main.go report > ../$(REPORT_FILE))
