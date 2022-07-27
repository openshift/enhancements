#!/bin/bash -xe

dnf -y module enable nodejs:14
dnf -y install nodejs
npm install -g markdownlint@v0.25.1 markdownlint-cli2@v0.4.0
