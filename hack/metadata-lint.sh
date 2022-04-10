#!/bin/bash

set -x

# Ensure the enhancement metadata includes the required information

set -o errexit
set -o nounset
set -o pipefail

# We only look at the files that have changed in the current PR, to
# avoid problems when the template is changed in a way that is
# incompatible with existing documents.
CHANGED_FILES=$(git log --name-only --pretty= "${PULL_BASE_SHA:-origin/master}.." -- \
                    | (grep '^enhancements.*\.md$' || true) \
                    | sort -u)

(cd tools && go build -o ../enhancement-tool -mod=mod ./main.go)

./enhancement-tool metadata-lint ${CHANGED_FILES}
