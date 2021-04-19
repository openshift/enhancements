---
title: DPU Support in OCP
authors:
  - "@danwinship"
reviewers:
  - "@derekwaynecarr"
  - "@dcbw"
  - "@zshi-redhat"
approvers:
  - "@derekwaynecarr"
  - "@dcbw"
creation-date: 2021-09-16
last-updated: 2021-04-15
status: provisional
---

# DPU Support in OCP

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

See [DPU Support in OCP (Overview)]. This document describes the
overall architecture, installation, and upgrades.

[DPU Support in OCP (Overview)]: ./overview.md

## Motivation

See Overview doc.

### Goals

- Create a "Developer Preview" version of the feature for OCP 4.10
  with the following limitations:

  - This will be difficult to install as there will be no explicit
    support anywhere for installing the infra and tenant clusters
    as a unit.

  - Upgrades to the infra cluster will result in network outages in
    the tenant cluster, as there will be no coordination between
    infra cluster and tenant cluster reboots.

  - The infra cluster will require separate ARM master nodes. We may
    support running these as VMs on an x86 host.

  - There will be no support for running customer workloads of any
    sort in the infra cluster.

  - It will not necessarily be possible to upgrade from Dev Preview
    to Tech Preview without reinstalling one or both clusters.

- Create further Tech Preview and GA versions of the feature with
  improved functionality.

### Non-Goals

- ...

## Proposal

### User Stories

See Overview doc.

### API Extensions

None for now

### Risks and Mitigations

#### Hardware Availability

We are currently developing using pre-GA BlueField-2 hardware, with a
plan based on official descriptions of how the final GA BlueField-2
hardware will work. So far (presumably due to the standard global chip
supply chain problems?) we have been unable to get GA cards (either
for free or by paying for them). If we are unable to get GA cards
soon, it may not be possible to ship the Dev Preview as designed for
OCP 4.10.

#### OCP on ARM

Some parts of OCP-on-bare-metal are not currently available for ARM.
In particular, I think we don't have "ironic" images, which are needed
for an IPI install.

## Design Details

### Design Details - Dev Preview

The initial dev preview will be a very limited implementation, to
demonstrate the overall architecture.

#### Overall Cluster Architecture

FIXME: fill this in:

- DPU Host and DPU NIC management interfaces
- How to connect the DPU to the network
  - How infra cluster and tenant cluster traffic is distinguished;
    VLANs? Parallel subnets?
- Difference between network setup for provisioning vs network setup
  at runtime?

#### Installation

Installing a cluster with BF-2 cards is potentially tricky, since the
cards can be configured in multiple modes, and in some modes, may not
allow their host to access the network by default.

To install the cluster then, it will be necessary to:

- Install the tenant cluster masters and at least one worker node that
  is not a DPU Host. Install the SR-IOV Operator in the tenant
  cluster, with the proper configuration (FIXME: what is the proper
  configuration?).

- Create a MachineConfigPool in the tenant cluster for the DPU Host
  workers, and update the Cluster Network Operator configuration to
  indicate that this pool is for DPU Host workers.

- Power on, but don't yet install, the DPU Hosts in the tenant
  cluster (so that the BF-2s themselves will be powered on).

- Install the infra cluster.

- Create a MachineConfigPool in the infra cluster for the DPU NIC
  workers, and update the Cluster Network Operator configuration to
  indicate that this pool is for DPU NIC workers.

- Install the DPU Operator (see below) onto the infra cluster,
  providing it with an appropriate tenant-cluster kubeconfig.
  Initially, this will just ensure that each of the cards is
  configured in "passthrough" mode, allowing the tenant hosts
  unrestricted network access.

- FIXME something must ensure that standalone kube-proxy runs on infra
  cluster BF-2 workers to provide service resolution. (Maybe CNO does
  this based on the infra DPU NIC MCP?)

- Add the DPU Host workers to the tenant cluster. The combination of
  the CNO and SR-IOV Operator config in the tenant cluster and the DPU
  Operator in the infra cluster will cause each DPU Host worker to be
  configured to run ovn-kubernetes in "`dpu-host`" mode with its
  corresponding DPU NIC worker running ovn-kubernetes in "`dpu`" mode.

  Setting this up correctly requires that DPU NICs can match
  themselves with their corresponding DPU Hosts. We have not yet
  determined how this will happen. It may require explicit
  configuration in Dev Preview.

#### Upgrade

When upgrading to a new OCP release, each BF-2 node in the infra
cluster will need to reboot at some point. This will cause a temporary
(~ 2 minute) network outage on the corresponding tenant cluster host.

In the GA product, we will need to coordinate the upgrades between the
two clusters, so that each NIC reboots just before its corresponding
host, after the host has already been drained of pods. However, this
functionality will not be implemented in the Dev Preview; upgrades
between the two clusters will be uncoordinated, resulting in
networking outages.

#### OCP Modifications in the Tenant Cluster

##### ovn-kubernetes (DPU Host mode)

The modifications to ovn-kubernetes itself to support "DPU Host" mode
have already been made upstream.

##### cluster-network-operator

CNO will need to know when to configure ovn-kubernetes in "DPU Host"
mode. For Dev Preview, we are not yet ready to add proper
`Network.operator.openshift.io` API for this, so we will instead have
CNO react to an optional ConfigMap.

##### Special Handling of BF-2 Host Nodes

When running in "DPU Host" mode, ovn-kubernetes will configure all
pod-network pods on a node to have SR-IOV interfaces rather than veths
as their primary network interfaces. Because of how SR-IOV currently
works in Kubernetes, this requires setting certain fields in the
`spec` of any pods that will be scheduled on those nodes. This means
that "ordinary" SR-IOV-unaware pod-network pods cannot be scheduled to
BF-2 host nodes.

While this will eventually need to be fixed, this means that for Dev
Preview, we need to ensure that OCP does not attempt to run any of its
own pod-network pods on BF-2 host nodes. (eg, `dns-default` pods
cannot run on these nodes). This may involve some combination of
special configuration, and tainting of the nodes in the BF-2 host
MachineConfigPool.

#### OCP Modifications in the Infra Cluster

##### ovn-kubernetes (DPU NIC mode)

As with the tenant side, the changes to ovn-kubernetes itself to
support "DPU NIC" mode have already been made upstream.

Infra cluster masters and non-DPU worker nodes will run ovn-kubernetes
(in "`full`" mode) as their CNI plugin. However, DPU NIC nodes will
not be able to do this, since it is not currently possible to run
ovn-kubernetes in both "`full`" mode and "`dpu-nic`" mode on the same
host.

The current plan for Dev Preview is that DPU NIC worker nodes will not
run any CNI plugin, and will only support host-network pods.
(Actually, they need _some_ CNI in order for the node to become Ready
at all, so they will run a dummy CNI plugin that indicates to
Kubelet/CRI-O that node networking is ready, but then fails every
`CNI_ADD` call.)

##### DPU Operator

We will write a "[DPU Operator]" to handle coordination between the
infra and tenant clusters, and setting up the special behavior for the
BF-2 cards in the infra cluster. This will be available via OLM and
installable on "day 2". See that enhancement for more details.

[DPU Operator]: https://github.com/openshift/enhancements/pull/890

##### Special Handling of BF-2 NIC Nodes

Similarly to the tenant cluster, we will need to prevent OCP-internal
pod-network pods from being scheduled to BF-2 NIC nodes, since there
will not be a functioning CNI plugin / pod network on those nodes.
Again, this may involve some combination of configuration and
tainting.

### Design Details - Tech Preview

TBD

### Design Details - GA

TBD

### Open Questions

...

### Test Plan

...

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

When downgrading to a version of OCP that does not support this
feature, the administrator must first disable the feature and return
the cluster to fully-non-offloaded operation.

### Version Skew Strategy

For Dev Preview, we do not promise to support y-stream version skew
between DPU NICs and DPU Hosts (which implies that it may not be
possible to update a cluster using the Dev Preview feature to the next
y-stream release, since the clusters cannot be upgraded perfectly in
sync). Z-stream version skew should hopefully not be a problem.

In the final product, we will need to ensure we can handle some skew
between the clusters at least during upgrades.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

- 2021-04-19: Initial proposal
- 2021-09-14: Split out overview and made this mostly just about Dev Preview for now.

## Drawbacks

## Alternatives

## Infrastructure Needed

Infrastructure both for initial development and for later CI will be
provided as part of the Hardware Enablement project of the Network
Services team. QE may eventually need some hardware of their own as
well.
