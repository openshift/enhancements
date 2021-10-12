##@ General

RUNTIME ?= podman # container command for linter and report-upload
PULL_BASE_SHA ?= origin/master # Allow the template check base ref to be overriden

.PHONY: help
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  make \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^[A-Z_-]+.*=/ { varend=index($$0, "?") - 1; if ( varend > 0 ) { helpstart=index($$0, "#") + 1; printf "   var \033[36m%-15s\033[0m %s\n", substr($$0, 0, varend), substr($$0, helpstart) } } /^##@/ { printf "\n  \033[1m%s\033[0m\n", substr($$0, 5) } END { printf "\n" }' $(MAKEFILE_LIST)

##@ Linter

.PHONY: image
image:  ## Build local container image
	$(RUNTIME) image build -f ./hack/Dockerfile.markdownlint --tag enhancements-markdownlint:latest

lint: image  ## run the markdown linter
	$(RUNTIME) run \
		--rm=true \
		--env RUN_LOCAL=true \
		--env VALIDATE_MARKDOWN=true \
		--env PULL_BASE_SHA=$(PULL_BASE_SHA) \
		-v $$(pwd):/workdir:Z \
		enhancements-markdownlint:latest

##@ This Week in Enhancements

DAYSBACK ?= 7 # number of days to include in report

REPORT_FILE=this-week/$(shell date +%F).md

.PHONY: report report-gen
report: report-gen lint  ## run weekly newsletter report tool

report-gen:
	(cd ./tools; go run ./main.go report --days-back $(DAYSBACK) > ../$(REPORT_FILE))

HACKMD_IMAGE=enhancements-hackmd-cli:latest # hackmd-cli image

.PHONY: report-image
report-image:
	$(RUNTIME) build -f ./hack/Dockerfile.hackmd-cli --tag $(HACKMD_IMAGE)
