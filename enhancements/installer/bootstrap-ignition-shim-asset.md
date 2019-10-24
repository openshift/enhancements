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
to avoid problems with instance metadata size limits. Previously, we were doing
this in Terraform using the terraform Ignition Provider, however the repo has fallen extremely out
of date, making us unable to add additional CA Bundles to our nodes. This means that clients that
use self signed certificates are unable to use our IPI installer, and can not upgrade to openshift 4 on Openstack.
We considered both updating the terraform provider and creating a new installer asset as possible solutions, and ultimately decided
that the best solution for our team is to create the installer asset. This asset would be generated just like the bootstrap
and master ignition assets are currently generated, and other platforms would have the option to pass the asset to their
terraform configs if they choose to use it. Please see
[this working proof of concept](https://github.com/openshift/installer/pull/2544) to see a technical
example of how this would be implemented.

## Motivation

Our motivations for choosing an installer asset over updating the terraform provider are as follows:

- Risk: The terraform ignition provider is extremely stale, and does not seem to be accepting or reviewing any new code
- Time: It is critical for the OpenStack team that we get this fix in 4.3, it prevents a large number of our users from upgrading to OpenShift 4. Terraform provider code is 100% green field for our team, so taking it on with the expectation of finishing by the feature freeze deadline is risky.
- Convenience: It would be convenient for all of our ignition assets to be generated the same way for UPI and IPI, ideally with the command: `openshift-install create ignition-configs`. This garuntees consistency, and lets us reuse the same code and tests.

### Goals

Our goal is to create an installer asset, written in Go, that creates an Ignition
shim for the bootstrap node. This shim should contain only the data
required to succesfully fetch the bootstrap Ignition config from its endpoint.
All further customizations should be added to the bootstrap Ignition config, not
the bootstrap shim. The design of this asset should be similar to the node ignition
asset so that other platforms can opt into using it, and add their own customizations.

## Proposal

We are proposing to create a new installer asset in `installer/pkg/asset/ignition/bootstrap/shim.go`.
This assest would have similar design to [this proof of concept code](https://github.com/openshift/installer/pull/2544),
and will serve as a pointer to the main bootstrap ignition config. In most platforms, that config is stored
in a remote storage solution like s3 or swift in Terraform. They can simply append an ignition config source to the
generated ignition config assset that points to the remote storage location in Terraform for now. We would like to
consider moving that functionality to the installer in later iterations of this design.

### Risks and Mitigations

- This implementation has to be generic enough to be useable by the other platforms
that rely on an ignition shim.
  - To mitigate this, developers from those teams will
need to be included in the design and review process of this feature in order to
make sure that their needs are met.
- Some values, such as the Swift temp url in OpenStack, are generated in Terraform,
and are therefore unable to be moved to the new installer asset.
  - These values can be edited in Terraform, as they are on aws and azure
  - Possible future plans to move this operation to installer code?
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
