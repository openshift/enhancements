#!/bin/bash -ex

# trap errors, including the exit code from the command failed
trap 'handle_exit $?' EXIT

source "$(dirname "${BASH_SOURCE}")/ignore_lib.sh"

function handle_exit() {
    # If the exit code we were given indicates an error, suggest that
    # the author run the linter locally.
    if [ "$1" != "0" ]; then
    cat - <<EOF

To run the linter on a Linux system with podman, run "make lint" after
committing your changes locally.

EOF
    fi
}

# ignore_paths should only be used for markdowns that are not enhancements

lint_files=$(find enhancements -type f -name "*.md")

lint_files=$(filter_ignored_paths $lint_files)

lint_files=$(echo "$lint_files" | tr '\n' ',')

markdownlint-cli2 '{'"$lint_files"'}'

$(dirname $0)/template-lint.sh

$(dirname $0)/metadata-lint.sh
