---
title: Non-Disruptive DPU Infra Cluster Upgrades
authors:
  - "@danwinship"
reviewers:
  - "@dhellmann"
  - "@sdodson"
  - "@wking"
  - "@romfreiman"
  - "@zshi"
approvers:
  - TBD
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - TBD
creation-date: 2021-01-11
last-updated: 2021-01-11
tracking-link:
  - https://issues.redhat.com/browse/SDN-2603
see-also:
  - "/enhancements/network/dpu/overview.md"
---

# Non-Disruptive DPU Infra Cluster Upgrades

## Summary

Our architecture for [using DPUs (eg, BlueField-2 NICs) in
OCP](../network/dpu/overview.md) involves creating one ordinary
"tenant" OCP cluster consisting of x86 hosts, and a second "infra" OCP
cluster consisting of the ARM systems running on the NICs installed in
the x86 hosts.

In the current Dev Preview of DPU support, there is no coordination
between the infra and tenant clusters during upgrades. Thus, when
upgrading the infra cluster, each NIC will end up rebooting at some
point, after it has drained _its own_ pods, but without having made
any attempt to drain the pods of its x86 tenant node. When this
happens, the x86 node (and its pods) will lose network connectivity
until the NIC finishes rebooting.

In order to make upgrades in DPU clusters work smoothly, we need some
way to ensure that tenant nodes do not get rebooted when they are
running user workloads.

## Glossary

(Copied from the [DPU Overview
Enhancement](../network/dpu/overview.md).)

- **DPU** / **Data Processing Unit** - a Smart NIC with a full CPU,
  RAM, storage, etc, running a full operating system, able to offload
  network processing and potentially other tasks from its host system.
  For example, the NVIDIA BlueField-2.

- **DPU NIC** - refers to the DPU and its CPU, OS, etc (as opposed to
  the CPU, OS, etc, of the x86 server that the DPU is installed in). A
  **DPU NIC node** or **DPU NIC worker** is an OCP worker node
  running on a DPU NIC.

- **DPU Host** - refers to an x86 server (and its CPU, OS, etc) which
  contains a DPU. A **DPU Host node** or **DPU Host worker** is an OCP
  worker node running on a DPU Host.

- **Two-Cluster Design** - any architecture for deploying an OCP
  cluster with (some) nodes containing DPUs, where the DPUs are nodes
  in a second OCP cluster. (For purposes of this term, the
  architecture still counts as "two-cluster" even if HyperShift is
  involved and there are actually three clusters.)

- **Infra Cluster** - in a Two-Cluster Design, the OCP cluster which
  contains the DPU NIC workers. (It may also include master nodes
  and/or non-DPU workers, depending on the particular details of the
  two-cluster design.)

- **Tenant Cluster** - in a Two-Cluster Design, the OCP cluster
  containing the DPU Host workers. (It may also include master nodes
  and/or workers without DPUs.)

## Motivation

### Goals

- Make upgrades of DPU Infra Clusters work without interrupting
  network connectivity in their corresponding Tenant Clusters.

### Non-Goals

- Requiring Infra and Tenant Clusters to upgrade at the same time;
  either cluster should be able to do an upgrade without involving the
  other.

- _Allowing_ Infra and Tenant Clusters to upgrade _efficiently_ at the
  same time. If an administrator wants to upgrade both clusters at the
  same time, then we would ideally do this in a way that would only
  drain each Tenant node once. But this would require substantially
  more synchronization between the two clusters, and is not handled by
  this proposal.

## Proposal

### User Stories

#### User Story 1

As the administrator of a cluster using DPUs, I want to be able to do
y-stream and z-stream upgrades without causing unnecessary network outages.

#### User Story 2

As an infrastructure provider of clusters using DPUs, I want to be
able to upgrade the DPU Infra Clusters without having to coordinate
with the administrators of the individual Tenant Clusters.

### API Extensions

TBD

### Implementation Details/Notes/Constraints

#### Setup

We can set some things up at install time (eg, creating credentials to
allow certain operators in the two clusters to talk to each other).

#### Inter-Cluster Version Skew

There is very little explicit communication between the two clusters:

  1. The dpu-operator in the infra cluster talks to the apiserver in
     the tenant cluster.

  2. The ovn-kubernetes components in the two clusters do not
     communicate directly, but they do make some assumptions about
     each other's behavior (which is necessary to coordinate the
     plumbing of pod networking).

The apiserver communication is very ordinary and boring and not likely
to be subject to any interesting version skew problems (even if we
ended up with, say, a 3-minor-version skew between the clusters).

Thus, other than ovn-kubernetes, no OCP component needs to be
concerned about version skew between the two clusters, because the
components in the two clusters are completely unaware of each other.

For ovn-kubernetes, if we ever change the details of the cross-cluster
communication, then we will need to add proper checks to enforce
tolerable cross-cluster skew at that time.

## Design Details

If an Infra Node is being cordoned and drained, then we can assume
that the administrator (or an operator) is planning to do something
disruptive to it. Anything that disrupts the Infra Node will disrupt
its Tenant Node as well.

Therefore, if an Infra Node is being cordoned and drained, we should
respond by cordoning and draining its Tenant Node, and not allowing
the Infra Node to be fully drained until the Tenant has also been
drained.

Given that behavior, when an Infra Cluster upgrade occurs, if it is
necessary to reboot nodes, then each node's tenant should be drained
before the infra node reboots, so there should be no pod network
disruption.

### "Drain Mirroring" Controller

In order to prevent MCO from rebooting an Infra node until its Tenant
has been drained, we will need to run at least one drainable pod on
each node, so that we can refuse to let that pod be drained until the
infra node's tenant is drained.

Annoyingly, although we need this pod to run on every worker node, it
can't be deployed by a DaemonSet, because DaemonSet pods are not
subject to draining. Thus, we will have to have the DPU operator
manage this manually. To minimize privileges and cross-cluster
communication, it would also make sense to have the DPU operator
handle the actual mirroring of infra state to the tenant cluster,
rather than having each infra node manage its own tenant node's state.

So, the "drain mirroring" controller of the DPU operator will:

  - Deploy a "drain blocker" pod to each infra worker node. This pod
    will do nothing (ie, it just runs the "pause" image, or
    equivalent), and will have a
    `dpu-operator.openshift.io/drain-mirroring` finalizer.

  - Whenever an infra worker is marked `Unschedulable`, mark its
    tenant node `Unschedulable` as well, with a `Condition` indicating
    why it was made unschedulable.

  - If a "drain blocker" pod on an unschedulable infra node is marked
    for deletion, then drain the corresponding tenant node as well
    before removing the finalizer from the pod.

  - Whenever an infra worker is marked not-`Unschedulable`:

      - If we were in the process of draining its tenant, then abort.

      - If the tenant node is also `Unschedulable` and has a
        `Condition` indicating that we are the one who made it
        `Unschedulable`, then remove the `Condition` and the
        `Unschedulable` state from the tenant node.

      - If the infra node does not currently have a "drain blocker"
        pod, then deploy a new one to it.

### Open Questions

Do finalizers block draining/eviction, or do we need to set
`TerminationGracePeriodSeconds` too? (If the latter, then this gets
more complicated since we need to control when the "drain blocker"
actually exits.)

### Risks and Mitigations

TBD

### Test Plan

TBD

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

This is a modification to the upgrade process, not something that can
be upgrade or downgraded on its own.

TBD, as the details depend on the eventual design.

### Version Skew Strategy

TBD, as the details depend on the eventual design.

### Operational Aspects of API Extensions

TBD

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

- Initial proposal: 2021-01-11
- Updated for initial feedback: 2021-01-24
- Updated again: 2021-03-02

## Drawbacks

This makes the upgrade process more complicated, which risks rendering
clusters un-upgradeable without manual intervention.

However, without some form of synchronization, it is impossible to
have non-disruptive tenant cluster upgrades.

## Alternatives

### Never Reboot the DPUs

This implies never upgrading OCP on the DPUs. I don't see how this
could work.

### Don't Have an Infra Cluster

If the DPUs were not all part of a single OCP cluster (for example,
they were just "bare" RHCOS hosts, or they were each running
Single-Node OpenShift), then it might be simpler to synchronize the
DPU upgrades with the tenant upgrades, because then each tenant could
coordinate the actions of its own DPU by itself.

The big problem with this is that, for security reasons, we don't want
the tenants to have any control over their DPUs. (For some future use
cases, the DPUs will be used to enforce security policies on their
tenants.)

### Synchronized Infra and Tenant Cluster Upgrades

In [the original proposal], the plan was for infra and tenant cluster
uprades to be synchronized, with the Infra and Tenant MCOs
communicating to ensure that workers were upgraded in the same order
in both clusters, and that reboots of the Infra and Tenant workers
happened at the same time.

After some discussion, we decided we didn't need that much
synchronization.

[the original proposal]: https://github.com/openshift/enhancements/commit/8033a923

### Simultaneous but Asynchronous Infra and Tenant Cluster Upgrades

[The second version of the proposal] took advantage of the fact that
it's presumed safe to reboot infra nodes whenever their tenant nodes
are idle, but still assumed that the infra and tenant clusters would
be upgraded at the same time. In this version, the two clusters
upgraded independently until they reached the MCO upgrade stage. Then
the infra cluster MCO was supposed to queue up RHCOS changes without
actually rebooting nodes, and then the tenant cluster MCO would
proceed with its upgrade, and each infra worker node would watch to
see when its tenant rebooted, and reboot itself at the same time.

This approach also had various problems, and probably would not have
worked well.

[The second version of the proposal]: https://github.com/openshift/enhancements/commit/b4650860