#!/bin/bash

# Ensure the enhancement metadata includes the required information

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "${BASH_SOURCE}")/ignore_lib.sh"

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# We only look at the files that have changed in the current PR, to
# avoid problems when the template is changed in a way that is
# incompatible with existing documents.
NEW_FILES=$(${SCRIPTDIR}/find_new.sh)
NEW_FILES=$(filter_ignored_paths $NEW_FILES)

if [ -z "$NEW_FILES" ]; then
    echo "OK, no changed enhancements found"
    exit 0
fi

(cd tools && go build -o ../enhancement-tool -mod=mod ./main.go)

./enhancement-tool metadata-lint ${NEW_FILES}
