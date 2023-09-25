---
title: dont-require-registry-during-boot-and-upgrade
authors:
- "@jhernand"
reviewers:
- "@avishayt"   # To ensure that this will be usable with the appliance.
- "@danielerez" # To ensure that this will be usable with the appliance.
- "@mrunalp"    # To ensure that this can be implemented with CRI-O and MCO.
- "@nmagnezi"   # To ensure that this will be usable with the appliance.
- "@oourfali"   # To ensure that this will be usable with the appliance.
approvers:
- "@sdodson"
- "@zaneb"
- "@LalatenduMohanty"
api-approvers:
- "@sdodson"
- "@zaneb"
- "@deads2k"
- "@JoelSpeed"
creation-date: 2023-09-21
last-updated: 2023-09-21
tracking-link:
- https://issues.redhat.com/browse/RFE-4482
see-also:
- https://issues.redhat.com/browse/OCPBUGS-13219
- https://github.com/openshift/enhancements/pull/1481
- https://github.com/openshift/cluster-network-operator/pull/1803
replaces: []
superseded-by: []
---

# Don't require registry during boot and upgrade

## Summary

Ensure that clusters don't require a registry server to boot or upgrade when
all the required images have already been pulled.

## Motivation

Currently during reboots and upgrades clusters may need to contact the image
registry servers, even if the images have already been pulled. This complicates
things for clusters that are completely disconnected or that have an slow or
unreliable connection to the image registry servers.

### User Stories

#### Boot without registry

As the administrator of a cluster that has all the required images already
pulled in all the nodes, I want to be able to reboot it without requiring
access to a registry server.

### Upgrade without registry

As the administrator of a cluster that has all the required images already
pulled in all the nodes, I want to be able to upgrade it without requiring
access to a registry server.

### Goals

Ensure that clusters don't require a registry server to boot or upgrade when
all the required images have already been pulled.

### Non-Goals

It is not the goal of this enhancement to add a mechanism to ensure that
required images are available. That is the subject of the [pin and pre-load
images](https://github.com/openshift/enhancements/pull/1481) enhancement.

## Proposal

### Workflow Description

1. The administrator of a cluster boots a node or performs an upgrade.

1. All the components of the cluster ensure that if the required images have
been already pulled they will not try to contact the registry server.

### API Extensions

None.

### Implementation Details/Notes/Constraints

#### Don't use the `Always` pull policy

Some OCP components currently use the `Always` image pull policy during
upgrades. As a result, the kubelet and CRI-O will try to contact the registry
server, even if the image is already available in the local storage of the
cluster. This blocks upgrades and should be avoided.

Most of these OCP components have been changed in the past to avoid this use of
the `Always` pull policy. Recently the OVN pre-puller has also been changed
(see this [bug](https://issues.redhat.com/browse/OCPBUGS-13219) for details).
To prevent bugs like this happening in the future and make the boots and
upgrades less fragile we should have a test that gates the OpenShift release
and that verifies that boots and upgrades can be performed without a registry
server. One way to ensure this is to run in CI an admission hook that
rejects/warns about any spec that uses the `Always` pull policy.

#### Don't try to contact the image registry server explicitly

Some OCP components explicitly try to contact the registry server without a
fallback alternative. These need to be changed so that they don't do it or so
that they have a fallback mechanism when the registry server isn't available.

For example, in OpenShift 4.1.13 the machine config operator runs the
equivalent of `skopeo inspect` in order to decide what kind of upgrade is in
progress. That fails if there is no registry server, even if the release image
has already been pulled. That needs to be changed so that contacting the
registry server is not required. A possible way to do that is to use the
equivalent of `crictl inspect` instead.

### Risks and Mitigations

None.

### Drawbacks

None.

## Design Details

### Open Questions

None.

### Test Plan

We should have a set of CI tests that verify that boots and upgrades can be
performed in a fully disconnected environment without a registry server, both
for a single node cluster and a cluster with multiple nodes. These tests should
gate the OCP release.

### Graduation Criteria

The feature will ideally be introduced as `Dev Preview` in OpenShift 4.X,
moved to `Tech Preview` in 4.X+1 and declared `GA` in 4.X+2.

#### Dev Preview -> Tech Preview

- Ability to boot clusters in disconnected environments without a registry
server.

- Ability to upgrade clusters in disconnected environments without a registry
server.

- Availability of the tests that verify the boot and upgrade without a registry
server.

- Availability of the tests that verify that no OCP component uses the `Always`
pull policy.

- Obtain positive feedback from at least one customer.

#### Tech Preview -> GA

- User facing documentation created in
[https://github.com/openshift/openshift-docs](openshift-docs).

#### Removing a deprecated feature

Not applicable.

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

Not applicable.

#### Failure Modes

#### Support Procedures

## Implementation History

None.

## Alternatives

None.

## Infrastructure Needed

Infrastructure will be needed to run the tests described in the test plan above.
