---
title: kubernetes-nmstate
authors:
  - "@schseba"
reviewers:

approvers:
  - TBD
  
creation-date: 2019-12-18
last-updated: 2019-12-18
status: 
---

# kubernetes-nmstate

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

A proposal to deploy [kubernetes-nmstate](https://github.com/nmstate/kubernetes-nmstate/) on OpenShift.

Node-networking configuration driven by Kubernetes and executed by
[nmstate](https://nmstate.github.io/).

## Motivation

With hybrid clouds, node-networking setup is becoming even more challenging.
Different payloads have different networking requirements, and not everything
can be satisfied as overlays on top of the main interface of the node (e.g.
SR-IOV, L2, other L2).
The [Container Network Interface](https://github.com/containernetworking/cni)
(CNI) standard enables different
solutions for connecting networks on the node with pods. Some of them are
[part of the standard](https://github.com/containernetworking/plugins), and there are
others that extend support for [Open vSwitch bridges](https://github.com/kubevirt/ovs-cni),
[SR-IOV](https://github.com/hustcat/sriov-cni), and more...

However, in all of these cases, the node must have the networks setup before the
pod is scheduled. Setting up the networks in a dynamic and heterogenous cluster,
with dynamic networking requirements, is a challenge by itself - and this is
what this project is addressing.

### Goals

- Deploy kubernetes-nmstate as part of openshift

### Non-Goals

- Replace SRIOV operator

## Proposal

A new kubernetes-nmstate handler DaemonSet is deployed in the cluster part of the OpenShift installation.
This DaemonSet contains nmstate package and interacts with the NetworkManager
on the host by mounting the related dbus. The project contains two
Custom Resource Definitions, `NodeNetworkState` and `NodeNetworkConfigurationPolicy`.
`NodeNetworkState` objects are created per each node in the cluster and can be
used to report available interfaces and network configuration. These objects
are created by kubernetes-nmstate and must not be touched by a user.
`NodeNetworkConfigurationPolicy` objects can be used to specify desired
networking state per node or set of nodes. It uses API similar to `NodeNetworkState`.

kubernetes-nmstate DaemonSet creates a custom resource of `NodeNetworkState` type per each node and
updates the network topology from each OpenShift node.

User configures host network changes and apply a policy in `NodeNetworkConfigurationPolicy` custom
resource. Network topology is configured via `desiredState` section in `NodeNetworkConfigurationPolicy`.
Multiple `NodeNetworkConfigurationPolicy` custom resources can be created.

Upon receiving a notification event of `NodeNetworkState` update,
kubernetes-nmstate Daemon verify the correctness of `NodeNetworkState` custom resource and
apply the selected profile to the specific node.

### User Stories

#### Bond creation

* Be able to create bond interfaces on OpenShift nodes.
* Create a vlan interface on top of the bond inter.

#### Assign ip address

* Assign static and/or dynamic ip address on interfaces
* Assign ipv4 and/or ipv6

#### Create/Update/Remove network routes

* Be able to Create/Update/Remove network routes for different interfaces like (bond,ethernet,sriov vf and sriov pf)

#### Manage/Configure host SR-IOV network device

* Be able to change host Virtual functions configuration (not managed by the sriov-operator) like vlans mtu driver etc..

#### Rollback

* Be able to rollback network configuration 
if we lose connectivity to the openshift api server after applying a policy.

### Implementation Details

The proposal introduces kubernetes-nmstate as Tech Preview.

## Design Details

### Test Plan

- Functional tests will be implemented

### Graduation Criteria

Initial support for kubernetes-nmstate will be Tech Preview

#### Tech Preview

- kubernetes-nmstate can be installed via container image
- Host network topology can be configured via CRDs

#### Tech Preview -> GA

### Upgrade / Downgrade Strategy

### Version Skew Strategy

kubernetes-nmstate runs as a DaemonSet.

## Implementation History

### Version 4.4

Tech Preview

## Infrastructure Needed

This requires a github repo be created under openshift org to hold a clone from kubernetes-nmstate
