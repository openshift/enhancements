---
title: kubernetes-nmstate
authors:
  - "@schseba"
  - "@bcrochet"
reviewers:

approvers:
  - TBD
  
creation-date: 2019-12-18
last-updated: 2019-12-18
status: 
---

# kubernetes-nmstate

## Release Signoff Checklist

- [X] Enhancement is `implementable`
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

- Dynamic network interface configuration for OpenShift on Day 2 via an API

### Non-Goals

- Replace SRIOV operator

## Proposal

A new kubernetes-nmstate operator Deployment is deployed to the cluster as part of the the OpenShift
installation, via CVO.

The operator has a single CRD, called `NMstate`. When a custom resource of kind `NMstate`, with 
a name of 'nmstate' is created, it will then create the CRDs for the kubernetes-nmstate handler.
Then, the namespace, RBAC, and finally the DaemonSet are applied. Custom resources of kind `NMstate`
with other names will be ignored. 

The `NMState` CR accepts a node filter, allowing one to add a label to nodes to indicate where the
DaemonSet will be deployed.

A new kubernetes-nmstate handler DaemonSet is deployed in the cluster by the operator.
This DaemonSet contains nmstate package and interacts with the NetworkManager
on the host by mounting the related dbus. The project contains three
Custom Resource Definitions, `NodeNetworkState`, `NodeNetworkConfigurationPolicy` and
`NodeNetworkConfigurationEnactment`. `NodeNetworkState` objects are created per each
node in the cluster and can be used to report available interfaces and network configuration.
These objects are created by kubernetes-nmstate and must not be touched by a user.
`NodeNetworkConfigurationPolicy` objects can be used to specify desired
networking state per node or set of nodes. It uses API similar to `NodeNetworkState`. 
`NodeNetworkConfigurationEnactment` objects will be created per each node, per each matching
`NodeNetworkConfigurationPolicy`.

kubernetes-nmstate DaemonSet creates a custom resource of `NodeNetworkState` type per each node and
updates the network topology from each OpenShift node.

User configures host network changes and apply a policy in `NodeNetworkConfigurationPolicy` custom
resource. Network topology is configured via `desiredState` section in `NodeNetworkConfigurationPolicy`.
Multiple `NodeNetworkConfigurationPolicy` custom resources can be created.

Upon receiving a notification event of `NodeNetworkState` update,
kubernetes-nmstate Daemon verify the correctness of `NodeNetworkState` custom resource and
apply the selected profile to the specific node.

`NodeNetworkConfigurationEnactment` objects is a read-only object that represents Policy exectuion
per each matching Node. It will expose configuration status per each Node.

A new container image (kubernetes-nmstate-handler) will be created.

A new container image (kubernetes-nmstate-operator) will be created. However, the operator
and the handler will co-exist in the same upstream repo.

An upstream API group of nmstate.io is currently used.

### User Stories

#### Bond creation

* As an OpenShift administrator, my customer base will change. Each customer will have different needs,
  network-wise. Most will be able to utilize the typical network. Some customers may need more bandwidth
  than a single pipe can provide. In order to satisfy these customers' neds, I would like to have
  the ability to create a bond on my nodes dynamically, without the need for a reboot.

#### VLAN create

* As an OpenShift administrator, my customer base will change. Each customer will have different needs,
  network-wise. Most will be able to utilize the typical network. Some customers may need more than
  one network, and desire a VLAN setup. In order to satisfy these customers' needs, I would like to have
  the ability to create a VLAN on top of a node interface, without the need for a reboot. 

#### Assign ip address

* As an OpenShift administrator, I have a need to create interfaces, such as VLAN interfaces, and 
  assign either a static or a dynamic IP address to that interface. I would like the ability to configure
  either a static address or dynamic address without the need for a reboot. 

* As an OpenShift administrator, I have the need to create a bridge, add an existing interface to
  the bridge, and move the IP from that interface to the bridge, all without having to reboot the node.

#### Create/Update/Remove network routes

* As an OpenShift administrator, I have a need to create/update/remove network routes for specific
  interfaces. This might include source routing. This is necessary without having to reboot the node.

#### Manage/Configure host SR-IOV network device

* As an OpenShift administrator, I would like to be able to change host Virtual functions configuration
 (those not managed by the sriov-operator) like vlans, mtu, driver etc.

#### Rollback

* As an OpenShift administrator, if a configuration that I apply is somehow invalid, I would like to be
  able to rollback network configuration if network connectivity is lost to the OpenShift api server 
  after a policy is applied. This should be done without my intervention, and restore connectivity as
  it was prior to application of the faulty configuration.

### Implementation Details

https://docs.google.com/document/d/1k7_vWtVRbOvTmOOTFx7qRPvYXJh3YyBPvuwh6H-g_jA/edit#heading=h.cdwyj2vhalzy

## Design Details

Distributed. Pod on every labeled node.

### Test Plan

- Unit tests (implemented)
- e2e tests of kubernetes-nmstate handler (implemented)
- e2e tests of kubernetes-nmstate operator (implemented)

All tests will be automated in CI.

### Graduation Criteria

Initial support for kubernetes-nmstate will be Tech Preview

#### Tech Preview

- kubernetes-nmstate can be installed via container image
- Host network topology can be configured via CRDs

#### Tech Preview -> GA

- Record a session, with slides, explaining usage for sharing with CEE.
- Documented under CNV

### Upgrade / Downgrade Strategy

The kubernetes-nmstate operator will handle upgrades. Downgrades are not specified ATM.

### Version Skew Strategy



## Implementation History

### Version 4.6

Tech Preview

## Infrastructure Needed

This requires a github repo be created under openshift org to hold a clone from kubernetes-nmstate
Any CI system could run the unit tests as-is. There is no need for specialized hardware.
The e2e tests require multiple interfaces on the nodes.

