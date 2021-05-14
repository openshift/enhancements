#!/bin/bash -xe

IS_CONTAINER=${IS_CONTAINER:-false}

if $IS_CONTAINER; then
    if hackmd-cli whoami | grep -q "not logged in"; then
        hackmd-cli login
    fi

    # Extract the page ID from a hackmd URL like:
    # https://hackmd.io/7Dw2wvyKQH660TGTRwWGpQ?both
    ID=$(echo $1 | sed -e 's|https://hackmd.io/||' -e 's/\?.*//')

    REPORT_FILE=/workdir/this-week/$(date +%F).md

    hackmd-cli export $ID ${REPORT_FILE}
else
    podman run --interactive --tty --rm=true \
           --env IS_CONTAINER=true \
           -v $(pwd):/workdir \
           -v $HOME:/home \
           --entrypoint='["/workdir/hack/hackmd-download.sh", "'$1'"]' \
           enhancements-hackmd-cli:latest
fi
