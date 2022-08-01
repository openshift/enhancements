---
title: vdpa-support-overview
authors:
  - Leonardo Milleri
  - Adrian Moreno Zapata
  - Ariel Adam
reviewers:
  - "@mandre"
  - "@tjungblu"
  - "@zshi-redhat"
  - "@SchSeba"
  - "@bn222"
approvers:
  - "@dcbw"
  - "@knobunc"
api-approvers:
  - "@dcbw"
  - "@knobunc"
creation-date: 2022-07-20
last-updated: 2022-07-29
tracking-link: 
  - https://issues.redhat.com/browse/NP-19
---

# VDPA support (Overview)

Virtual data path acceleration (vDPA) in essence is an approach to standardize the NIC SRIOV data plane using the virtio ring layout and placing a single standard virtio driver in the guest. It’s decoupled from any vendor implementation while adding a generic control plane and SW infrastructure to support it.
Given that it’s an abstraction layer on top of SRIOV (Single Root I/O Virtualization) it is also future proof to support emerging technologies such as scalable IOV.

Virtio (see [spec](https://docs.oasis-open.org/virtio/virtio/v1.1/csprd01/virtio-v1.1-csprd01.html)) is used for the data plane while vDPA is used to simplify the control plane thus this solution in general is called virtio/VDPA.

Virtio/vDPA can provide acceleration capabilities replacing SRIOV both for VMs and containers. In this proposal we focus on adding virtio/vDPA capabilities to OVN-Kubernetes as dev-preview for container workloads focusing on Nvidia connectx-6 DX NICs. In the future support for Nvidia Bluefield2/Bluefield3 is planned to be provided as well as support for VMs in openshift (kubevirt and kata containers).

Virtio/vDPA can be consumed in two different ways by the user:
- virtio-vdpa for container workloads
- vhost-vdpa for DPDK applications, kata containers etc (any VM based app or user space workload).


## Summary

VDPA technology will be integrated into OpenShift to provide the ability
to standardize the NIC control plane and data plane.
This enhancement (and future enhancements) are likely to span to multiple Openshift releases.

## Motivation

The main motivation to integrate VDPA in Openshift is to provide the ability to handle the following use cases:
- Run Cloud-Native Network Functions (CNFs) with wirespeed access to NIC
- Reduce considerably the CNF certifcation effort by poviding standard open virtio drivers decoupled from vendor specific NIC interfaces
- Hyperscale the solution by using the exact same control plane / data plane for all workloads (VMs, containers, etc)
- Increase the security in the host by hardening the open standard framework in the kernel and its corresponding drivers

A number of blogs have been published on virtio/vDPA for readers interested in additional details on the technology (such as [Achieving network wirespeed in an open standard manner: introducing vDPA](https://www.redhat.com/en/blog/achieving-network-wirespeed-open-standard-manner-introducing-vdpa) and [Introduction to vDPA kernel framework](https://www.redhat.com/en/blog/introduction-vdpa-kernel-framework)).

### User Stories

#### **Use case #1: CNF certification**

Virtio/vDPA provides standard open virtio drivers to be used by VMs and container workloads decoupled from vendor specific NIC interfaces (assuming the NIC supports virtio/vDPA). The Nvidia connectx-6 DX card is one example of a card supporting virtio/VDPA which we will focus on supporting first.
Given this standard interface, virtio/vDPA can significantly reduce the work of certifying CNF (container networking function) workloads especially in the case of service chaining where multiple CNFs are required to interact with each other.

For additional details on this use case see [How vDPA can help network service providers simplify CNF/VNF certification](https://www.redhat.com/en/blog/how-vdpa-can-help-network-service-providers-simplify-cnfvnf-certification).

#### **Use case #2: Hyperscale**

Hyperscale is a use case where storage, networking and security are offloaded to smartNICs (programmable accelerators) to free up the host server's resources to run workloads such as VMs and containers.
For granting these workloads the "look & feel" that they are running as usual on the host server while in practice their storage, networking and security is running on a PCI connected smartNIC, resource mapping from the host server to the smartNIC is required.

This is where vendor specific closed solutions and in contrast virtio/vDPA technologies come into play providing the resource mapping and enabling the hyperscale use case to come to life. The key value virtio/vDPA brings on top of it being an open and standard interface is in this context a single data plane / control plane for all workloads.
This means that by using virtio/vDPA for the hyperscale use case all workloads (VMs, containers, Linux processes) can share the exact same control plane / data plane which significantly simplifies maintaining, troubleshooting and controlling resources.

For additional details on this use case see [Hyperscale virtio/vDPA introduction: One control plane to rule them all](https://www.redhat.com/en/blog/hyperscale-virtiovdpa-introduction-one-control-plane-rule-them-all).

#### **Use case #3: Confidential containers and hardening**

As smartNICs are emerging and their corresponding devices and SW drivers, new attack surfaces are being created in the kernel and host. This is since historically we assume devices are “dumb” while drivers are “smart” thus we focus is on protecting the device from the driver.
With smartNICs however you can now run malicious software on the smartNIC transforming the device to a smart entity that can attack the driver. This is especially critical when looking at emerging use cases such as confidential computing/confidential containers which also require using devices and drivers.
Virtio/vDPA addresses this problem by hardening the open standard framework in the kernel and it’s corresponding drivers while closed source solutions are much more vulnerable to such attacks (virtio has a large community supporting it).
 
For additional details on this use case see [Hardening Virtio for emerging security use cases](https://www.redhat.com/en/blog/hardening-virtio-emerging-security-usecases).

### Goals

- As a user I want to configure an Openshift cluster with VDPA support

### Non-Goals


### Supported Hardware

We want to support the following NVIDIA NIC cards:
- CTX-6 DX
- BlueField-2

## Proposal

The details will be figured out in further enhancements.

### Workflow Description

### API Extensions

### Risks and Mitigations

None

### Drawbacks

None


## Design Details


### Test Plan

**Note:** *Section not required until targeted at a release.*

TBD

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

None

### Upgrade / Downgrade Strategy

TBD

### Version Skew Strategy

TBD

### Operational Aspects of API Extensions

TBD

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

TBD

## Alternatives

TBD
