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

Customers request the ability to add disks to their OpenShift clusters. Some of the common areas include designed disk for etcd, dedicated disk for swap partitions, container runtime filesystem, and a separate filesystem for container images.

All of these features are possible to support through a combination of machine configs and machine API changes.
However, the support varies across the use cases and there is a need to define a general API for disk support so we can have a common experience across different methods.

### Goals

- Define a common interface for infrastructure platforms to implement to use additional disks for a defined set of specific uses
- Implement common behaviour to safely use the above disks when they have been presented by the infrastructure platform

### Non-Goals

- Adding generic support for mounting arbitrary additional disks

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

### Workflow Description

At a high level, customers would create a machine and specify the disk layout on node creation. For day 0 operations, this would mean that they are creating the cluster with this disk layout. For day 1 operations, we mean that the machine is created on an existing cluster.

The main workflow in this case would be a customer requests the creation of a machine with the targeted disk layout. Once the disk is created, Machine Config Operation is able to setup the disk for the application.

In the next section, we will walk through the workflows for each user story.

#### ETCD Workflow

@matt please help fill this out.

#### Container Runtime Workflow

CRI-O contains a location where images/containers are stored. The location is set to `/var/lib/containers/storage`. 
By default, this is stored in the same partition as `/var`.
Customers would like to add a disk to an openshift node and store images/containers on this disk.

In order to do this, there are some gotchas for setting up your filesystem. 
The main one is that the filesystem must have the correct selinux labels.
In order to achieve this, we must be able to detect that the customer is changing filesystem.
MCO relabels `/var/lib/containers/storage` to satisfy this rules so changing the location requires reapplying that logic. 

The following steps could satisfy this request:

- Customer requests a machine with a filesystem for container runtimes
- The filesystem is labeled with `CONTAINER_RUNTIME`
- MCO will detect that a machine has a `CONTAINER_RUNTIME` label.
  - Given the label, the filesystem is labeled to satisfy selinux rules

#### Image Store Workflow

A new feature would allow for separating containers and images to separate filesystems.
Due to a technical limitation in Kubernetes, we want to keep containers on the same filesystem as kubelet.
But we want to be store images on a separate disk.

The following steps could satisfy this request:
- Customer requests a machine with a filesystem for images
- The filesystem is labeled with `IMAGES`
- MCO will detect that the machine has a `IMAGES` label.
- Given the label, MCO will prepare the node for this.
  - Container storage should set `IMAGESTORE`
  - selinux relabeling

#### Container Logs Workflow

Upstream work is required for this to track storage metrics if logs are separated to a separate disk.
But the workflow is still worth going through for this feature.
Log location can be controlled via a KubeletConfig.

The following steps could satisfy this request:
- Customer requests a machine with a filesystem for logs
- The filesystem is labeled with `CONTAINER_LOGS`
- MCO detects that the machine has a `CONTAINER_LOGS` label.
- Given this label, KubeletConfig is changed for the container logs.
- Kubelet is restarted

#### Swap Workflow

The following steps could satisfy this request:
- Customer requests a machine with a filesystem for swap.
- The filesystem is labeled with `SWAP`.
- MCO detects that the machine has a `SWAP` label.
- Given this label, Swap is set up on the filesystem.

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
