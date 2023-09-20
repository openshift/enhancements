#!/usr/bin/env sh

function filter_ignored_paths() {
    ignore_paths=$(cat ./hack/lint-ignores)

    ret=$1
    for path in ${ignore_paths[@]}; do
        ret=${ret//$path/}
    done

    echo $ret
}
