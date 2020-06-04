---
title: minimise-baremetal-footprint
authors:
  - "@hardys"
reviewers:
  - "@avishayt"
  - "@beekhof"
  - "@crawford"
  - "@deads2k"
  - "@dhellmann"
  - "@hexfusion"
  - "@mhrivnak"
approvers:
  - "@crawford"
creation-date: "2020-06-04"
last-updated: "2020-06-05"
status: implementable
see-also: compact-clusters
replaces:
superseded-by:
---

# Minimise Baremetal footprint

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Over recent releases OpenShift has improved support for small-footprint
deployments, in particular with the compact-clusters enhancement which adds
full support for 3-node clusters where the masters are schedulable.

This is a particularly useful deployment option for baremetal PoC environments,
where often the amount of physical hardware is limited, but there is still the
problem of where to run the installer/bootstrap-VM in this environment.

The current solution for IPI baremetal is to require a 4th bootstrap host,
which is a machine physically connected to the 3 master nodes, that runs
the installer and/or the bootstrap VM.  This effectively means the minimum
footprint is 4 nodes, unless you can temporarily connect a provisioning host
to the cluster machines.

A similar constraint exists for UPI baremetal deployments, where although a
3 master cluster is possible, you need to run a 4th bootstrap node somewhere
for the duration of the initial installation.

Even in larger deployments, it is not recommended to host the bootstrap or
controlplane services on a host that will later host user workloads, due
to the risk of e.g stale loadbalancer config resulting in controlplane
traffic reaching that node, so potentially you always need an additional
node (which may need to be dedicated per-cluster for production cases).

## Motivation

This proposal outlines a potential approach to avoid the requirement for a
4th node, leveraging the recent etcd-operator improvements and work to enable
a live-iso replacement for the bootstrap VM.

### Goals

* Enable clusters to be deployed on baremetal with exactly 3 nodes
* Avoid the need for additional nodes to run install/bootstrap components
* Simplify existing IPI baremetal day-1 user experience

### Non-Goals

* Supporting any controlplane topology other than three masters.
* Supporting deployment of a single master or scaling from such a deployment.
* Support for pivoting the bootstrap machine to a worker.

## Proposal

### User Stories

As a user of OpenShift, I should be able to install a fully supportable
3-node cluster in baremetal environments, without the requirement to
temporarily connect a 4th node to host installer/bootstrap services.

As a large-scale production user with multi-cluster deployments I want to avoid
dedicated provisioning nodes per-cluster in addition to the controlplane node
count, and have the ability to redeploy in-place for disaster recovery reasons.

As an existing user of the IPI baremetal platform, I want to simplify my day-1
experience by booting a live-ISO for the bootstrap services, instead of a
host with a bootstrap VM that hosts those services.

### Risks and Mitigations

This proposal builds on work already completed e.g etcd-operator improvements
but we need to ensure any change in deployment topology is well tested and
fully supported, to avoid these deployments being an unreliable
corner-case.

## Design Details

### Enabling three-node clusters on baremetal

OpenShift now provides a bootable RHCOS based installer ISO image, which can
be booted on baremetal, and adapted to install the components normally
deployed on the bootstrap VM.

This means we can run the bootstrap services in-place on one of the target hosts
which we can later reboot to become a master (referred to as master-0 below).

While the master-0 is running the bootstrap services, the two additional hosts
are then provisioned, either with a UPI-like boot-it-yourself method, or via a
variation on the current IPI flow where the provisioning components run on
master-0 alongside the bootstrap services (exactly like we do today on the
bootstrap VM).

When the two masters have deployed, they form the initial OpenShift controlplane
and master-0 then reboots to become a regular master.  At this point it joins
the cluster and bootstrapping is complete, and the result is a full-HA 3-master
deployment without any dependency on a 4th provisioning host.

Note that we will not support pivot of the initial node to a worker role, since
there is concern that network traffic e.g to the API VIP should never reach
a worker node, and there could be a risk e.g if an external load balancer config
was not updated of this happening if the bootstrap host is allowed to pivot
to a worker.


### Test Plan

We should test in baremetal (or emulated baremetal) environments with 3-node
clusters with machines that represent our minimum target and ensure our e2e
tests operate reliably with this new topology.

We should add testing of the controlplane scaling/pivot (not necessarily on
baremetal) to ensure this is reliable.  It may be this overlaps with some
existing master-replacement testing?

### Graduation Criteria

TODO

### Upgrade / Downgrade Strategy

This is an install-time variation so no upgrade/downgrade impact.

## Implementation History

TODO links to existing PoC code/docs/demos

## Drawbacks

The main drawback of this approach is it requires a deployment topology and
controlplane scaling which is not likely to be adopted by any of the existing
cloud platforms, thus moves away from the well-tested path and increases the
risk of regressions and corner-cases not covered by existing platform testing.

Relatedly it seems unlikely that existing cloud-platforms would adopt this
approach, since creating the bootstrap services on a dedicated VM is easy
in a cloud environment, and switching to this solution could potentially
add walltime to the deployment (the additional time for the 3rd master to
pivot/reboot and join the cluster).

## Alternatives

One possible alternative is to have master-0 deploy a single-node controlplane
then provision the remaining two hosts.  This idea has been rejected as it
is likely more risky trying to scale from 1->3 masters than establishing
initial quorum with a 2-node controlplane, which should be similar to the
degraded mode when any master fails in a HA deployment, and thus a more
supportable scenario.
