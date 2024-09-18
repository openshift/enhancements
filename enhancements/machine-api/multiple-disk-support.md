---
title: multiple-disk-support
authors:
  - "@kannon92"
reviewers:
  - "tbd"
approvers:
  - "tbd"
api-approvers:
  - "tbd"
creation-date: 2024-09-17
last-updated: 2024-09-17
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-615"
  - "https://issues.redhat.com/browse/OCPSTRAT-1592"
see-also:
  - "https://github.com/openshift/enhancements/pull/1657"
replaces:
superseded-by:
---

# Disk Support in Openshift

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenShift is traditionally a single-disk system, meaning the OpenShift installer is designed to install the OpenShift filesystem on one disk. However, new customer use cases have highlighted the need for the ability to configure additional disks during installation.

## Motivation

Custormers request the ability to add disks for day 0 and day 1 operations. Some of the common areas include designed disk for etcd, dedicated disk for swap partitions, container runtime filesystem, and a separate filesystem for container images.

All of these features are possible to support through a combination of machine configs and machine API changes.
However, the support varies across the use cases and there is a need to define a general API for disk support so we can have a common experience across different methods.

### Workflow Description

### Goals

TBD

### Non-Goals

- Adding disk support in CAPI providers where it is not supported upstream

## Proposal

### User Stories

#### Designated Disk for ETCD

As a user, I would like to install a cluster with a dedicated disk for etcd.
Our recommended practices for etcd suggest using a dedicated disk for optimal performance.
Managing disk mounting through MCO can be challenging and may introduce additional issues.
Cluster-API supports running etcd on a dedicated disk.

An example of this done via MCO is available on our [documentation pages](https://docs.openshift.com/container-platform/4.13/scalability_and_performance/recommended-performance-scale-practices/recommended-etcd-practices.html#move-etcd-different-disk_recommended-etcd-practices).

#### Dedicated Disk for Swap Partitions

As a user, I would like to install a cluster with swap enabled on each node and utilize a dedicated disk for swap partitions.
A dedicated disk for swap would help prevent swap activity from impacting node performance.

If the swap partition is located on the node filesystem, it is possible to get I/O contention between the system I/O and the user workloads.
Having a separate disk could allow for swap I/O contention to be mitigated.

#### Dedicated Disk for Container Runtime

As a user, I would like to install a cluster and assign a separate disk for the container runtime for each node.

This has been supported in Kubernetes for a long time but it has been poorly documented. Kevin created [container runtime filesystem](https://kubernetes.io/blog/2024/01/23/kubernetes-separate-image-filesystem).

A common KCS article we link is [here](https://access.redhat.com/solutions/4952011).

#### Dedicated Disk for Image Storage

As a user, I would like to install a cluster with images stored on a dedicated disk, while keeping ephemeral storage on the node's main filesystem.

This story was the motivation for [KEP-4191](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/4191-split-image-filesystem/README.md).

#### Container logs

As a user, I would like that my container logs are stored on a separate filesystem from the node filesystem and the container runtime filesystem.

[OCPSTRAT-188](https://issues.redhat.com/browse/OCPSTRAT-188) is one feature request for this.

[RFE-2734](https://issues.redhat.com/browse/RFE-2734) is another ask for separating logs.

### API Extensions

### Implementation Details/Notes/Constraints

### Topology Considerations

#### Hypershift / Hosted Control Planes

This proposal does not affect HyperShift.
HyperShift does not leverage Machine API.

#### Standalone Clusters


#### Single-node Deployments or MicroShift

Single Node and MicroShift do not leverage Machine API.

### Risks and Mitigations

N/A

## Design Details

### Open Questions

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

No features will be removed as a part of this proposal.

## Upgrade / Downgrade Strategy

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

#### Failure Modes


## Support Procedures

## Implementation History

N/A

### Drawbacks

N/A

## Alternatives

### Future implementation
