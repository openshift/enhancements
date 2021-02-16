#!/bin/bash -xe

dnf -y module enable nodejs:12
dnf -y install nodejs
npm install -g markdownlint markdownlint-cli2
