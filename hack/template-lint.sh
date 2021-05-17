#!/bin/bash

# Check that the required section headers from the template are
# present in the enhancement document(s).

set -o errexit
set -o nounset
set -o pipefail

TEMPLATE=$(dirname $0)/../guidelines/enhancement_template.md

# We only look at the files that have changed in the current PR, to
# avoid problems when the template is changed in a way that is
# incompatible with existing documents.
CHANGED_FILES=$(git log --name-only --pretty= "${PULL_BASE_SHA:-origin/master}.." -- \
                    | (grep '^enhancements.*\.md$' || true) \
                    | sort -u)

RC=0
for file in $CHANGED_FILES
do
    # Look for files that were renamed.
    if [ ! -f ${file} ]; then
        echo "Skipping deleted file ${file}"
        continue
    fi

    echo "Checking ${file}"

    # Iterate over the required headers in the template. We look for
    # at least 2 # to start the line because the title header will be
    # different from the text in the template, and we check for a
    # title separately.
    missing=$(grep '^##' $TEMPLATE \
        | grep -v '\[optional\]' \
        | while read header_line
    do
        if ! grep -q "^${header_line}" $file
        then
            echo "$file missing \"$header_line\""
        fi
    done)
    if [ -n "$missing" ]; then
        echo "$missing"
        RC=1
    fi

    # Now look for a title, one # followed by a space to start a line.
    if ! grep -q '^# ' $file
    then
        echo "$file is missing a title"
        RC=1
    fi
done

exit $RC
