#!/bin/bash

# Find the enhancements files that have changed in the current PR.

set -o errexit
set -o pipefail

for f in $(git log --name-only --pretty= "${PULL_BASE_SHA:-origin/master}.." -- \
               | (grep '^enhancements.*\.md$' || true) \
               | sort -u); do
    # Filter out anything that no longer exists, probably because of a
    # rename in the middle of the PR chain.
    if [ ! -f ${f} ]; then
        echo "Skipping deleted or renamed file ${f}" 1>&2
        continue
    fi

    echo $f
done
