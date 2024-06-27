#!/bin/bash -xe

cat /etc/redhat-release || echo "No /etc/redhat-release"

if grep -q 'Red Hat Enterprise Linux' /etc/redhat-release; then
    # install the config file for the RPM repository with node 14
    # steps taken from https://rpm.nodesource.com/setup_14.x
    yum module disable -y nodejs
    curl -sL -o '/tmp/nodesource.rpm' 'https://rpm.nodesource.com/pub_14.x/el/8/x86_64/nodesource-release-el8-1.noarch.rpm'
    rpm -i --nosignature --force /tmp/nodesource.rpm
    yum -y install nodejs
else
    # Fedora has a module we can use
    dnf -y module enable nodejs:16
    dnf -y install nodejs
fi

npm install -g markdownlint@v0.25.1 markdownlint-cli2@v0.4.0
