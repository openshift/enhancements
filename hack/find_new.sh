#!/bin/bash

# Find the enhancements files that are new in the current PR.

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

set -o errexit
set -o pipefail

FILELIST=/tmp/$$.filelist
git ls-tree --name-only -r "${PULL_BASE_SHA:-origin/master}" enhancements > $FILELIST

for f in $(${SCRIPTDIR}/find_changed.sh); do
    if ! grep -q $f $FILELIST; then
        echo $f
    fi
done
