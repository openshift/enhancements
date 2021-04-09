#!/bin/bash -xe

echo "Running"

if hackmd-cli whoami | grep -q "not logged in"; then
    hackmd-cli login
fi

hackmd-cli import $1
