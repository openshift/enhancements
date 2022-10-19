---
title: virtio-vdpa-ovn-hw-offload
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
creation-date: 2022-07-13
last-updated: 2022-09-23
tracking-link: 
  - https://issues.redhat.com/browse/NHE-13
---


# Virtio/vDPA with OVN HW offload

The purpose of this enhancement is to provide a proposal for integrating Virtio/vDPA in OCP for container workloads.


## Summary
See [VDPA support in OCP (Overview)](https://github.com/openshift/enhancements/blob/master/enhancements/network/vdpa/vdpa-support-overview.md).

Main aspects of this enhancement:
- primary interface in the pod is configured as a Virtio/vDPA device
- NIC configured in switchdev mode
- OVS HW offloading enabled
- Supported HW: NVIDIA ConnectX-6 Dx


## Motivation

The main motivation is to integrate a Virtio/vDPA device in Openshift.


### User Stories

Details are provided in the overview enhancement.

### Goals

- As a user I want to configure an Openshift cluster with Virtio/VDPA support.
- As a user I want to determine how many VFs to create for a given NIC and how each VF should be consumed by workloads (e.g., netdev device in the container network namespace)
- As a user I want the VDPA solution to be integrated with OVN-kubernetes and with HW offloading enabled

### Non-Goals

- The Vhost-vdpa driver won't be supported in CNFs (VMs only). The virtio-vdpa driver will be supported in CNFs, i.e. netdev device to be consumed.
- Scalable IOV and Sub-functions are out of scope, only switchdev SR-IOV will be supported (with OVS HW offloading to the NIC)

## Proposal

The following diagram depicts all the involved components and their interactions:

![alt text](vdpa-architecture.png "VDPA architecture")

### Workflow Description

#### Pre-requirements

- Create the Openshift cluster on a baremetal node
- Deploy OVN-kubernetes
- Deploy the SRIOV network operator

#### SRIOV network operator workflow

- Configure OpenVswitch in HW offload mode
- Configure NIC (switchdev-configuration-before-nm.service and switchdev-configuration-after-nm.service services)
  - Install vendor-independent kernel drivers: vdpa, virtio-vdpa
  - Configure VFs (same as SR-IOV: echo N > /sys/devices/{PF}/sriov_numvfs)
  - Put NIC in switchdev mode (smart NIC)
  - Enable HW offload mode on PF, VFs and port representors
  - Disable Network Manager on VFs and port representors
  - Install vendor-dependent kernel drivers (e.g. mlx5-core for Mellanox cards)
  - Bind the right PCI driver (vendor specific). Some vendors might implement vdpa in a different PCI driver (e.g: Intel’s ifcvf). Others might keep the same pci driver and require extra steps (e.g: in mellanox vdpa, the VF is still bound to “mlx5_core”)
- Create the vdpa device (vendor specific). Some vendors might require no extra steps because they create the vdpa device on a pci driver probe (e.g: ifcvf). Others might need extra steps (e.g: Mellanox requires loading an additional driver and in the future, they might require managing a “virtual bus”(source)). The plan is to extend the govdpa library to provide this functionality.
- Bind the vdpa device to the correct vdpa bus driver (virtio-vdpa driver in the first implementation). This is not vendor specific and uses a sysfs-based API
- Configure the SRIOV device plugin
  - Add the appropriate vdpa flags to the SR-IOV Device Plugin’s configMap.
- Create the Network Attachment Definitions

Note: the SRIOV operator (config-daemon) needs to destroy the vdpa device when policy is removed


### API Extensions

The plan is to extend the SriovNetworkNodePolicy CRD API to support the vDPA feature.

The proposed solution in details:

- Keep deviceType meaning “driver bound to PCI device”. And add a field called **vdpaType** to select the type of vdpa device (vhost/virtio).
- Add a runtime check to verify that a user does not specify { “deviceType”:  “vfio-pci”, “vdpaType”: “virtio” } because that doesn’t make sense. A new rule needs to be added to the Validation webhook.

Benefits:
- Easy to implement
- Backward compatibible

Problems:
- If another vendor requires another driver for vdpa in the future we would need to expand deviceType to add the new driver.
Relies more on knowledge from the user.

Note: there is an alternative option, aiming to change the semantics of the deviceType field to express the way the device is exposed to the user and let the SR-IOV Network Operator in cooperation with the vendor plugins determine what is the driver that must be bound at the PCI bus (and, of course the vdpa bus).
DeviceType would assume one of the following values: netdev/vfio-pci/vdpa-virtio/vdpa-vhost. The problem with this approach is the management of the backward compatibility and the complexity of the change, so we're not going for this option (at least not now).

### Implementation Details/Notes/Constraints

The proposed implementation depends on the CX-6 Dx OVS hardware work which is not fully ready yet in OCP 4.11.

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

Check if the implementation would fit Openshift 4.12 code freeze date

### Drawbacks

None

## Design Details

### Test Plan

- Automated CI tests

### Graduation Criteria

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

None

### Version Skew Strategy

None

### Operational Aspects of API Extensions

None

#### Failure Modes

None

#### Support Procedures

None

## Implementation History

None

## Alternatives

None
