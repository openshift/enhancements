---
title: Openshift Installer Bootstrap Ignition Shim
authors:
  - "@iamemilio"
reviewers:
  - TBD
  - "@abhinavdahiya"
  - "@wking"
  - "@mandre"
  - "@tomassedovic"
approvers:
  - TBD
  - "@abhinavdahiya"
  - "@wking"
creation-date: 2019-10-17
last-updated: 2019-10-17
status: provisional
---

# Openshift Installer Bootstrap Ignition Shim Asset

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

There are a few platforms that use an Ignition shim for their bootstrap machine. The shim
is essentially a pointer to the main config that is stored on a remote server, which helps
us avoid problems with instance metadata size limits on our platform. Previously, we were doing
this in Terraform using the terraform Ignition Provider, however the repo has fallen extremely out
of date, making us unable to add additional CA Bundles to our nodes. This means that clients that
use self signed certificates are unable to use our IPI installer, and can not upgrade to openshift 4 on Openstack.
We considered both updating the
terraform provider and creating a new installer asset as possible solutions, and ultimately decided
that the best solution for our team is to create the installer asset. We plan to create an ignition
asset similar to the [node asset](https://github.com/openshift/installer/blob/master/pkg/asset/ignition/machine/node.go)
for bootstrap ignition shims that other platforms can **opt in** to.

## Motivation

We spent a lot of time looking at the Terraform Ignition Provider to determine if we could
get involved in the community there to solve our problem. However we had a number of hangups
with this. First and foremost, that repo has not seen code merged since May 2019. We also noticed
a number of valid and mergable pull requests waiting for approval for months.
It seems as if there aren't any maintainers, and we were also unable to reach the contributers.
The CA Bundle bug is preventing a considerable portion of our IPI users
from being able to upgrade to OpenShift 4 on OpenStack, and we need this fix in 4.3, so this was rather
dissuading. Lastly, we discovered that for our UPI solution, we wont have a way to generate the bootstrap
ignition shim if we continue generating it in terraform. We would like to generate the ignition configs for
UPI with the command `openshift-install create ignition-configs` so that Ignition configs for both UPI and IPI
are created the same way.

### Goals

Our goal is to create an installer asset, written in Go, that creates an Ignition
shim for the bootstrap node. This shim should contain only the data
required to succesfully fetch the bootstrap Ignition config from its endpoint.
All further customizations should be added to the bootstrap Ignition config, not
the bootstrap shim. The design of this asset should be similar to the node ignition
asset so that other platforms can opt into using it, and add their own customizations.

## Proposal

We are proposing to create a new installer asset in `installer/pkg/asset/ignition/bootstrap/shim.go`.
This assest would have similar design to the [node asset] (https://github.com/openshift/installer/blob/master/pkg/asset/ignition/machine/node.go),
and will serve as a pointer to the main bootstrap ignition config. Platforms can further customize this
config with the IP address of the node or service provisioned to host the Bootstrap Config by
editing the `ignitionHost` field in Terraform, which is already the standard for most platforms.

### Risks and Mitigations

- This implementation has to be generic enough to be useable by the other platforms
that rely on an ignition shim.
  - To mitigate this, developers from those teams will
need to be included in the design and review process of this feature in order to
make sure that their needs are met.
- Some values, such as the Swift temp url in OpenStack, are generated in Terraform,
and are therefore unable to be moved to the new installer asset.
  - These values can be edited in Terraform, as they are on aws and azure
- The new Asset would need to know the platform like the node asset does
  - We can make the InstallConfig asset one of the dependancies, and pass it in as an argument

## Design Details

### Test Plan

We noticed that other Ignition assets did not have unit tests. Perhaps we were not looking in the right place?

### Graduation Criteria

- Dev Preview: Code works for most cases, may have bugs or corner cases
  - Ignition shims are consitently and predictably generated
  - Ignition shims reliably fetch bootstrap Ignition
  - CA Bundles are succesfully added to nodes
  - Asset is usable for UPI
- Tech Preview: Code works for most cases, tests allow for iteration and prevent big bugs
  - Ignition shims are consitently and predictably generated
  - Ignition shims reliably fetch bootstrap Ignition
  - CA Bundles are succesfully added to nodes
  - Asset is usable for UPI
  - Testing infrastructure supports new asset
- GA: Codes works as expected, unlikely for user to experience bugs
  - Ignition shims are consitently and predictably generated
  - Ignition shims reliably fetch bootstrap Ignition
  - CA Bundles are succesfully added to nodes
  - Testing infrastructure supports new asset
  - Asset is usable for UPI
  - QE approval

## Drawbacks

Creating an asset in the installer is non trivial work. It also may not be used by other platforms, and would
have to be implemented carefully and tested by all platforms to make sure that it does not affect their code.

## Alternatives

- Please see the Motivation section
  - We could try to update the terraform provider to ignition v2.2
