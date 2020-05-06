---
title: infiniband
authors:
  - "@zshi"
reviewers:
  - "@pliurh"
  - "@dougbtv"
  - "@s1061123"
  - "@danwinship"
  - "@squeed"
  - "@dcbw"
  - "@yrobla"
approvers:
  - "@knobunc"
  - "@fepan"
creation-date: 2020-05-06
last-updated: 2020-05-06
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# InfiniBand

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

RDMA (Remote Direct Memory Access) fabrics such as InfiniBand and RDMA over
Converged Ethernet (RoCE) are key to unlocking scalable performance for
demanding Artificial Intelligence (AI) and Machine Learning (ML) applications.
RoCE v2 is supported in SR-IOV Operator as Tech Preview since 4.3, This
enhancement is to introduce native InfiniBand support.

## Motivation

Enable high-performance message transmission via RDMA for distributed workloads
such as AI/ML.

### Goals

- Provide native InfiniBand support on pod additional network interfaces

### Non-Goals

- Support GPUDirect RDMA
- Support InfiniBand on pod primary network interface

## Proposal

Extend SR-IOV Operator to support both Ethernet and InfiniBand devices.

SR-IOV Hardware NIC is managed by SR-IOV Operator and will be provisioned
with a new device type `infiniBand` or `ib`.

A new Container Network Interface (CNI) plugin `ib-sriov-cni` will be added.

InfiniBand support will only be available on bare metal deployment.

#### Hardware

The following Mellanox NICs will be supported:
- Mellanox Technologies MT27700 Family [ConnectX-4]
- Mellanox Technologies MT28908 Family [ConnectX-6]

#### InfiniBand SR-IOV CNI

InfiniBand SR-IOV CNI `ib-sriov-cni` is a 3rd party CNI plugin that moves
SR-IOV InfiniBand device to pod namespace and configures device attributes
such as InfiniBand GUID. It will be delivered in a separate image and copied
to node CNI directory by a DaemonSet.

#### InfiniBand Kubernetes Daemon

InfiniBand Kubernetes Daemon `ib-kubernetes` is a DaemonSet that watches
Pod object changes and manages network partitioning via InfiniBand subnet
manager plugins.

#### SR-IOV Operator

SR-IOV Operator API definition will support `infiniBand` or `ib` device type
in both `SriovNodeNetworkState` and `SriovNodeNetworkPolicy`.

SR-IOV Operator will support generating net-attach-def Custom Resource with
`ib-sriov-cni` as CNI plugin type.

#### SR-IOV Config Daemon

SR-IOV Config Daemon, a sub component managed by SR-IOV Operator, will be
enhanced to change device link type to InfiniBand.

Mellanox Plugin, a Go plugin managed by SR-IOV config daemon for vendor
specific configuration, will support changing device link type to InfiniBand.

Opensm rpm package will be installed in SR-IOV config daemon container image.
Opensm service will be started when InfiniBand device is configured via SR-IOV
Operator device type API.

#### SR-IOV Network Device Plugin

SR-IOV Network Device Plugin, a sub component managed by SR-IOV Operator,
supports selecting SR-IOV devices with link type `ether` or `infiniband`.


## Design Details

### Test Plan

- e2e tests will be added in SR-IOV Operator for InfiniBand device type
- e2e tests will be conducted in baremetal environment as part of SR-IOV tests
- InfiniBand function must work on supported NICs

### Graduation Criteria

##### -> Tech Preview

- SR-IOV InfiniBand device can be provisioned by SR-IOV Operator
- SR-IOV InfiniBand device can be advertised as extended node resource
- Pod can be created with SR-IOV InfiniBand device as additional interface
- SR-IOV document should be updated to include InfiniBand feature
- Should pass e2e and QE tests

### Upgrade / Downgrade Strategy

This enhancement is an addon feature in SR-IOV Operator, it follows the same
upgrade/downgrade strategy as was developed in marketplace operator.

## Implementation History

- SR-IOV Operator is in GA support since 4.3
- RoCE v2 support in SR-IOV Operator is Tech Preview since 4.3

## Infrastructure Needed [optional]

A new github project needs to be created under openshift organization for
infiniBand SR-IOV CNI.
