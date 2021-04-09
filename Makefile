RUNTIME ?= podman
DAYSBACK ?= 7

.PHONY: help
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: image
image:  ## Build local container image
	$(RUNTIME) image build -f ./hack/Dockerfile.markdownlint --tag enhancements-markdownlint:latest

lint:  ## run the markdown linter
	$(RUNTIME) run \
		--rm=true \
		--env RUN_LOCAL=true \
		--env VALIDATE_MARKDOWN=true \
		-v $$(pwd):/workdir:Z \
		enhancements-markdownlint:latest

REPORT_FILE=this-week/$(shell date +%F).md
.PHONY: report report-gen
report: report-gen lint report-upload  ## run weekly newsletter report tool

report-gen:
	(cd ./tools; go run ./main.go report --days-back $(DAYSBACK) > ../$(REPORT_FILE))

HACKMD_IMAGE=enhancements-hackmd-cli:latest

.PHONY: report-upload
report-upload: report-image
	$(RUNTIME) run --interactive --tty --rm=true \
		-v $$(pwd):/workdir \
		-v $$HOME:/home \
		--entrypoint='["/workdir/hack/hackmd-cli.sh", "'$(REPORT_FILE)'"]' \
		$(HACKMD_IMAGE)

.PHONY: report-image
report-image:
	$(RUNTIME) build -f ./hack/Dockerfile.hackmd-cli --tag $(HACKMD_IMAGE)
