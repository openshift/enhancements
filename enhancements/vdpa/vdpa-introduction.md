---
title: vdpa-enhancement
authors:
  - Leonardo Milleri, Adrian Moreno Zapata, Ariel Adam
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers:
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2022-07-13
last-updated: 2022-07-13
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
see-also:
  - TBD
replaces:
  - TBD
superseded-by:
  - TBD
---

# VDPA / Openshift integration

Virtual data path acceleration (vDPA) in essence is an approach to standardize the NIC SRIOV data plane using the virtio ring layout and placing a single standard virtio driver in the guest. It’s decoupled from any vendor implementation while adding a generic control plane and SW infrastructure to support it. Given that it’s an abstraction layer on top of SRIOV (Single Root I/O Virtualization) it is also future proof to support emerging technologies such as scalable IOV.

Virtio (see [spec](https://docs.oasis-open.org/virtio/virtio/v1.1/csprd01/virtio-v1.1-csprd01.html)) is used for the data plane while vDPA is used to simplify the control plane thus this solution in general is called virtio/VDPA.

Virtio/vDPA can provide acceleration capabilities replacing SRIOV both for VMs and containers. In this proposal we focus on adding virtio/vDPA capabilities to Openshift SDN as dev-preview for container workloads focusing on Nvidia connectx-6 DX NICs. In the future support for Nvidia Bluefield2/Bluefield3 is planned to be provided as well as support for VMs in openshift (kubevirt and kata containers).

Virtio/vDPA can be consumed in two different ways by the user:
- virtio-vdpa for container workloads (our focus for this enhancement is the virtio-vdpa solution)
- vhost-vdpa for DPDK applications, kata containers etc (any VM based app or user space workload). This solution will be addressed in a future enhancement.


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

Virtio/vDPA provides standard open virtio drivers to be used by VMs and container workloads decoupled from vendor specific NIC interfaces (assuming the NIC supports virtio/vDPA). The Nvidia connectx-6 DX card is one example of a card supporting virtio/VDPA which we will focus on supporting first. Given this standard interface, virtio/vDPA can significantly reduce the work of certifying CNF (container networking function) workloads especially in the case of service chaining where multiple CNFs are required to interact with each other. 

For additional details on this use case see [How vDPA can help network service providers simplify CNF/VNF certification](https://www.redhat.com/en/blog/how-vdpa-can-help-network-service-providers-simplify-cnfvnf-certification).

#### **Use case #2: Hyperscale**

Hyperscale is a use case where storage, networking and security are offloaded to smartNICs (programmable accelerators) to free up the host server's resources to run workloads such as VMs and containers. For granting these workloads the "look & feel" that they are running as usual on the host server while in practice their storage, networking and security is running on a PCI connected smartNIC, resource mapping from the host server to the smartNIC is required. 

This is where vendor specific closed solutions and in contrast virtio/vDPA technologies come into play providing the resource mapping and enabling the hyperscale use case to come to life. The key value virtio/vDPA brings on top of it being an open and standard interface is in this context a single data plane / control plane for all workloads.  This means that by using virtio/vDPA for the hyperscale use case all workloads (VMs, containers, Linux processes) can share the exact same control plane / data plane which significantly simplifies maintaining, troubleshooting and controlling resources. 

For additional details on this use case see [Hyperscale virtio/vDPA introduction: One control plane to rule them all](https://www.redhat.com/en/blog/hyperscale-virtiovdpa-introduction-one-control-plane-rule-them-all).

#### **Use case #3: Confidential containers and hardening**

As smartNICs are emerging and their corresponding devices and SW drivers, new attack surfaces are being created in the kernel and host. This is since historically we assume devices are “dumb” while drivers are “smart” thus we focus is on protecting the device from the driver. With smartNICs however you can now run malicious software on the smartNIC transforming the device to a smart entity that can attack the driver. This is especially critical when looking at emerging use cases such as confidential computing/confidential containers which also require using devices and drivers. Virtio/vDPA addresses this problem by hardening the open standard framework in the kernel and it’s corresponding drivers while closed source solutions are much more vulnerable to such attacks (virtio has a large community supporting it). 
 
For additional details on this use case see [Hardening Virtio for emerging security use cases](https://www.redhat.com/en/blog/hardening-virtio-emerging-security-usecases).

### Goal

- As a user I want to configure an Openshift cluster with Virtio/VDPA support (given that the NIC card supports VDPA).
- As a user I want to determine how many VFs to create for a given NIC and how each VF should be consumed by workloads (e.g., netdev device in the container network namespace)
- As a user I want the VDPA solution to be integrated with OVN-kubernetes and with HW offloading enabled (given that the NIC card supports HW offloading)

### Non-Goals

- The Vhost-vdpa driver won't be supported in CNFs (VMs only). The virtio-vdpa driver will be supported in CNFs, i.e. netdev device to be consumed.
- Scalable IOV and Sub-functions are out of scope, only legacy SRIOV will be supported for partitioning the NIC into multiple VFs.

## Proposal

The following diagram depicts all the involved components and their interactions:

![alt text](vdpa-architecure.png "VDPA architecture")

### Workflow Description

#### Pre-requirements

- Create the Openshift cluster on a baremetal node
- Deploy OVN-kubernetes
- Deploy the SRIOV network operator

#### SRIOV network operator workflow

 - Deploy the SRIOV device plugin
 - Deploy OpenVswitch in HW offload mode
 - Configure the VFs (same as SR-IOV: echo N > /sys/devices/{PF}/sriov_numvfs)
 - Install vendor-independent kernel drivers: vdpa, virtio-vdpa
 - Install vendor-dependent kernel drivers, e.g. mlx5-vdpa for Mellanox cards
 - Put NIC in switchdev mode (smart NIC)
 - (Vendor specific) Bind the right PCI driver. Some vendors might implement vdpa in a different PCI driver (e.g: Intel’s ifcvf). Others might keep the same pci driver and require extra steps (e.g: in mellanox vdpa, the VF is still bound to “mlx5_core”)
 - Enable HW offload mode on PF, VFs and port representors
 - (Vendor specific) Create the vdpa device. Some vendors might require no extra steps because they create the vdpa device on a pci driver probe (e.g: ifcvf). Others might need extra steps (e.g: Mellanox requires loading an additional driver and in the future, they 	might require managing a “virtual bus”(source)). The plan is to extend the govdpa library to provide such functionality.
- Bind the vdpa device to the correct vdpa bus driver (virtio-vdpa driver in the first implementation). This is not vendor specific and uses a sysfs-based API.
- Add the appropriate vdpa flags to the SR-IOV Device Plugin’s configMap.
- Create the Network Attachment Definitions

Note: the operator will have to support the tear-down of resources during the undeploying phase.


### API Extensions

The plan is to extend the SriovNetworkNodePolicy CRD API to support the vDPA feature.

There are two proposals:

#### **Proposal A**
Change the semantics of the deviceType field to express the way the device is exposed to the user and let the SR-IOV Network Operator in cooperation with the vendor plugins determine what is the driver that must be bound at the PCI bus (and, of course the vdpa bus).
**deviceType** would assume one of the following values: netdev/vfio-pci/vdpa-virtio/vdpa-vhost.
The problem with this approach:
**isRdma** goes in another direction. isRdma changes the way the device is exposed to the user but is only compatible with netdev type (and only one vendor implements it AFAIK).
So for this proposal to be complete, deviceType would also include “rdma”. This could become a compatibility issue but backwards compatibility should be manageable.
Benefits of this proposal: Easier for the user.
#### **Proposal B**
Keep deviceType meaning “driver bound to PCI device”. And add a field called **vdpaType** to select the type of vdpa device (vhost/virtio).
We need to add a runtime check to verify that a user does not specify { “deviceType”:  “vfio-pci”, “vdpaType”: “virtio” } because that doesn’t make sense.
Benefits:
Easier to implement
Problems:
If another vendor requires another driver for vdpa in the future we would need to expand deviceType to add the new driver.
Relies more on knowledge from the user.
Runtime errors (user creates a combination of fields that are wrong) are more difficult to return to the user.


### Implementation Details/Notes/Constraints

The work can be split into 4 main repos: [ovn-kubernetes](https://github.com/ovn-org/ovn-kubernetes), [sriov-network-device-plugin](https://github.com/k8snetworkplumbingwg/sriov-network-device-plugin), [sriov-networking-operator](https://github.com/k8snetworkplumbingwg/sriov-network-operator) and [govdpa](https://github.com/k8snetworkplumbingwg/govdpa).

#### **Ovn-kubernetes**

Revive and merge the PR: https://github.com/ovn-org/ovn-kubernetes/pull/2664

Note: currently OVN CNI doesn't support the reading of the DeviceInfo. It is recommended to support this functionality for a better and consistent design.

This PR works in combination with [this PR in the SR-IOV Network Device Plugin](https://github.com/k8snetworkplumbingwg/sriov-network-device-plugin/pull/306) to provide support for vDPA devices.
This functionality is based on [OVS Hardware Offload](https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/ovs_offload.md) and allows ovn-kubernetes to expose an open standard netdev such as a virtio-net to the pod.
Most of the heavy-lifting (detecting vdpa devices, ensuring they are bound to the right driver, etc) is done by the SR-IOV Network Device Plugin. From an ovn-kubernetes perspective, it's just a matter of selecting the right netdev to move to the pod's namespace.

#### **SRIOV-network-device-plugin**

The relevant PR has been already merged to github: https://github.com/k8snetworkplumbingwg/sriov-network-device-plugin/pull/306

Additional unit tests to be implemented.

#### **SRIOV-network operator**

Implement the changes as described in the above workflow section.


#### **Govdpa library**

Github repository: https://github.com/k8snetworkplumbingwg/govdpa

The library needs to be extended with the following functionalities:
- Create a vdpa device for a given vdpa management device
- Delete a vdpa device
- Unit testing


### Risks and Mitigations

Openshit 4.12 code freeze date too close

### Drawbacks

None


## Design Details

### Open Questions [optional]

TBD

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
