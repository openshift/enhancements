#!/bin/bash -xe

echo "Running"

if [ ! -f $HOME/.hackmd/cookies.json ]
then
    hackmd-cli login
else
    hackmd-cli whoami
fi

hackmd-cli import $1
