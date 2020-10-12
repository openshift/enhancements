---
title: User Space Pod Interface and API Library
authors:
  - "@bmcfall"
reviewers:
  - "@dcbw"
  - "@zshi"
approvers:
  - TBD
creation-date: 2020-08-20
last-updated: 2020-08-20
status: implementable
---

# User Space Pod Interface and API Library

Add support for running user space networking applications (i.e. - DPDK
based applications) in pods on OCP.

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Some applications require higher bandwidth and lower latency than can be
provided by kernel networking. One solution is to bypass the kernel network
stack and perform the networking in user space. An example of this is writing an
application that does its own packet processing using the
[DPDK Libraries](https://www.dpdk.org/).

A DPDK based application has additional requirements though. They typically need
hugepage memory, dedicated CPUs, and direct ownership of the networking device
or interface (whether that is a physical NIC, an SR-IOV VF or other sub-function,
or a virtio device). Throughout this document, a user space networking
application is an application running in a container that is performing
networking related functions in user space and has some or all of the
requirements mentioned above.

A typical pod has one and only one kernel interface (one side of a veth pair).
Interface attributes such as IP address and MAC address are applied to the
interface through the kernel. More advanced use cases involve injecting more
than one interface into the pod using meta-CNI plugins such as Multus.

For these advanced use cases, multiple interfaces injected into a pod and a user
space networking application run in one of the containers, additional information
needs to be passed to the pod. Below are types of data that need to be passed
into the pod:

- Multiple interfaces:
  - With multiple interfaces now in a pod, the pod needs context on which
  interface is intended for what purpose.

- Interface is used by a user space networking application in the pod:
  - When the container workload performs packet processing in user space, for
  example a DPDK application, some of the DPDK poll mode drivers (logic that
  polls the interface for packets) require different kernel drivers in order to
  work properly. The interfaces are moved over to these drivers
  (uio_pci_generic, igb_uio, or vfio-pci) before being assigned to a pod. These
  drivers do not have a mechanism to assign an IP address or MAC address from
  host (i.e. from a CNI) and such can not be read from within a pod. So for
  user space networking applications, another mechanism needs to used to pass
  IP address and MAC address values to a pod.
  - In addition to data typically passed via the kernel interface (IP address
  and MAC address), user space networking applications require additional
  information to function properly. Examples of some of the data include:
    - CPUs allocated to the pod
    - Number of hugepages dedicated to the pod

- User space interface is injected into the pod:
  - A user space interface, like a [vhost-user](#vhost-user) (not supported by
  OCP) or a [vDPA interface](#vdpa-interface), which uses the virtio protocol
  for data plane negotiation (support expect in future OCP release, around 4.8
  or 4.9 time frame), requires additional information like Unix socket file and
  virtio mode (master or client). Additional data needed by these types of
  interfaces is not needed by OCP immediately, but must be considered during the
  overall design.

This enhancement has two components:

- How the data should be passed to the pod?
- How to ease the consumption of the data in the pod?

## Motivation

Various system resources are allocated to run high performance network
applications inside a container such as CPU cores, dedicated memory (hugepages),
network devices, etc. However, these resources are presented differently inside
the container and some of them are not visible to applications without
explicitly injecting to the container.

### Goals

- Define what data needs to be passed from Kubernetes to a user space
networking application to enable the application to run properly.

- Define how this data is passed from Kubernetes to a user space networking
application.

- Ease the consumption of the data that was passed into a user space networking
application by providing a library that both provides an example of how the
data can be collected and if desired, integrated directly into the application. 

- Ensure customers using SR-IOV can run user space networking applications in
their deployment.

### Non-Goals

The following items are out of scope for this proposal:

- This could, secondarily, also be used to support user space data plane
technologies like ovs-dpdk, but its not the same, and its not our primary goal.

- Non-SR-IOV devices (vDPA and vhost-user interfaces) require additional
information. However, these devices are not required for this enhancement.
That being said, some of the upstream work this enhancement is pulling from
does include those interfaces. So the API Library may include some of the
additional work for these interfaces.

## Proposal

As described above, there are two parts to this enhancement. Work is needed to
pass additional data to the pod to enable user space networking applications to
run properly. Work is also needed in the pod to ease the consumption of the
data. Each item will be described below.

### Passing Data to the Pod

Currently, data is passed to the pod in several forms:

- **PCI Addresses**: SR-IOV Device Plugin passes the list of PCI Addresses in
an environment variable to the pod.

- **Interface Data**: Multus populates the pod annotation with a
`k8s.v1.cni.cncf.io/network-status` field. This annotation contains the list
of interfaces, and for each interface, the MAC address, list of IP addresses
(not CIDR), interface (net1, net2, etc), name, etc. The pod can use the
Kubernetes Downward API to retrieve this information.

- **CPU List**: The dedicated CPU list can be retrieved by reading
`/sys/fs/cgroup/cpuset/cpuset.cpus` in the pod.

For user space networking applications running in a pod, the following
information is still needed:

- **Hugepage Information**: The type and number of hugepages allocated to the
pod.

- **Additional Interface Data**: The `k8s.v1.cni.cncf.io/network-status`
contains a `name` field, which is the `network` field in the
[Network-Attachment-Definition](#networkattachmentdefinition). The `name`
does indicate how the interface is intended to be used, however, the pod doesn't
have enough information to determine which PCI Address is associated with which
interface. So additional information is needed to make that association within
the pod.
  - One downside to this approach is that the `name` field is just a string, so
  "intended use" of the interface would have to be encoded in the string as a
  well known name (or well known sub-string) that the user space networking
  application would know and leverage.

#### Hugepage Information

To allow a DPDK application running in a container to know how many hugepages
are available to use, hugepage information will be added to the set of supported
fields in the Kubernetes Downward API
[86102](https://github.com/kubernetes/kubernetes/pull/86102).
SR-IOV Network-Resource Injector already ensures the Kubernetes Downward API is
enabled in the pod-spec of pods using SR-IOV interfaces. It will now also need to
ensure the hugepage fields are exposed by the Kubernetes Downward API.

#### Additional Interface Data

Work is being done in the Network Plumbing Working Group (NPWG) to standardize
how data for secondary interfaces is passed to a pod. NPWG is where the standard
is defined that Multus is implemented too. See:
[Kubernetes Network Custom Resource Definition De-facto Standard](https://github.com/k8snetworkplumbingwg/multi-net-spec).
The `k8s.v1.cni.cncf.io/network-status` field is being expanded to include
additional data for each interface, specific to the interface type data. See
Proposal:

[NPWG - Device Information](https://docs.google.com/document/d/1rBm-L1ymXIjoKNA6w2lixwBAjt9-tnbs-ee7AcLRbVE/edit?usp=sharing)

The proposal has been discussed in the NPWG for several months and there is
general agreement on the feature and now work is being done on finalizing the
smaller details. The remaining work includes:

- Updating the specification, defining the new fields in
`k8s.v1.cni.cncf.io/network-status`

- Completing a best practices document which describes how the data is passed
between a Device Plugin or Admission Controller to a CNI, and what Multus needs
to do with the collected data.

Once that is completed, the following projects will need to be updated to add
support (proof of concept already exists for most of these):

- SR-IOV Device Plugin
- SR-IOV CNI
- SR-IOV Network-Resource Injector
- Multus
- appNetUtil

#### Consumption of Data in the Pod

Data passed into a pod can come from several places. Below are some examples:

- For kernel interfaces, data can be applied to the interface via the kernel
itself.

- For SR-IOV VFs, the PCI Address list is passed into the pod via an
environment variable.

- Using Kubernetes Downward API, pod annotation data is exposed within the pod.
Data is written to the pod annotation and consumed by the pod.

To ease the burden of the CNF developer, an API library is being developed that
collects data from all the different sources and exposes the data through a set
of APIs. The API is written in Golang and can be imported into other Golang
programs. Since DPDK is the primary user space networking application being
targeted, and DPDK is written in C, there is also a C API binding for the Golang
library.

The library is located in github at the following URL:
- https://github.com/openshift/app-netutil/

The library will not be released in OCP, but will be available for CNF
developers to include in their projects. If the CNF developers choose not to
include the library, the library will still serve as an example of where to pull
the data from and how to consume the data.

The following APIs will be exposed:

- `GetCPUInfo()`
  - Returns the set of CPUs assigned to the pod.
  - **Note:** When pod is created with non-guaranteed QoS, this API returns all
  available cpus on the node, not just those dedicated to a pod. 

- `GetInterfaces()`
  - Returns the set of interfaces associated with the pod and a structure for
  each interface containing the type of interface and the data specific to that
  interface type.

- `GetHugepageInfo()`
  - Returns the amount of hugepage memory reserved for the pod and page size.

### Implementation Details/Notes/Constraints

### Risks and Mitigations

- **Risk**: One risk with moving this to GA is that there has been no customer
feedback on the `appNetUtil` library.
  - **Mitigation**: Because the `appNetUtil` library is not built into OCP,
  changes could be made to how the library presents the data to the caller post
  OCP 4.7, provided that the data feed into the container is correct.

- **Risk**: One of the items identified is to update Kubernetes to add hugepage
information to the set of fields Downward API exposes. Risk is getting a PR with
the required changes accepted in the Kubernetes 1.20 time frame.
  - **Mitigation**: Work on the PR needs to happen immediately. This will be one
  of the first work items started. Also, this risk has been identified to
  Product Management and this feature could move to GA without it, though it is
  highly desired and all efforts should be made to get it in.

## Design Details

### Open Questions [optional]

### Test Plan

The [appNetUtil](https://github.com/openshift/app-netutil/) repository needs
unit tests and CI added.

### Graduation Criteria

The `appNetUtil` library was introduced as Tech Preview in OCP 4.5. The plan
is to make the library GA in OCP 4.7. To our knowledge, the `appNetUtil`
library is not currently being used. With the additions described in this
enhancement, the library should be more useful to CNF developers. Also by moving
to GA, the library may attract more users and provide feedback on ways to
improve the usefulness of the library.

##### Dev Preview -> Tech Preview

In Tech Preview, the following APIs are defined in
`github.com/openshift/app-netutil/lib/v1alpha`:

- `GetCPUInfo()`
  - Returns the set of CPUs assigned to the pod.

- `GetInterfaces()`
  - Returns the set of interfaces associated with the pod and a structure for
  each interface containing the type of interface and the data specific to that
  interface type.

##### Tech Preview -> GA 

In GA, the following APIs are defined in
`github.com/openshift/app-netutil/lib/v1`:

- `GetCPUInfo()`
  - Returns the set of CPUs assigned to the pod.
  - Same as Tech Preview.

- `GetInterfaces()`
  - Returns the set of interfaces associated with the pod and a structure for
  each interface containing the type of interface and the data specific to that
  interface type.
  - The returned structure will be expanded to include additional data.

- `GetHugepageInfo()`
  - Returns the amount of hugepage memory reserved for the pod and page size.
  - New API.

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

appNetUtil Library:

- Tech Preview: OCP 4.5 - June, 2020
- GA: OCP 4.7 - January, 2021

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

## Footnotes

### vhost user

The vhost user protocol consists of a control path and a data path. All control
information is exchanged via a Unix socket. This includes information for
exchanging memory mappings for direct memory access, as well as kicking/
interrupting the other side if data is put into the virtio queue. The actual
dataplane is implemented via direct memory access. This type of interface is
commonly used by vSwitches (for example OvS DPDK) to connect with other DPDK
based applications running in VMs or containers.

For a user space networking application running in a container to use a vhost
user interface, the container needs the socketfile location for the protocol
messaging, and needs to know if it needs to create the file (master) or if the
file has already been created on the host (client).

### vDPA Interface

vDPA (virtual Data Path Acceleration) utilizes virtio ring compatible devices to
serve the virtio driver directly to enable datapath acceleration (i.e. - vrings
are implemented in NIC instead of in software on host). NICs that support vDPA
behave similar to NICs that support SR-IOV in the fact that the Physical
Function (PF) can be divided up into multiple Virtual Functions (VF). However
vDPA uses the standard virtio specification for ring layout so that generic
drivers can be used in the VM or Container. Whereas in the SR-IOV case, vendor
specific drivers are required.

Like vhost-user interfaces, vDPA interfaces use a Unix socketfile for the
protocol negotiation. This implies that the container in the pod that is running
the user space networking application needs to know the socketfile location to
run properly. It also has to know if it needs to create the file (master) or if
the file has already been created on the host (client).

### NetworkAttachmentDefinition

Kubernetes allows custom resources to be created via a Custom Resource
Definition (CRD). Multus (via the Network Plumbing Working Group specification:
[Kubernetes Network Custom Resource Definition De-facto Standard](https://github.com/k8snetworkplumbingwg/multi-net-spec))
uses a CRD called a NetworkAttachmentDefinition to define a network, and
configuration data about that network. Then multiple networks can be associated
with a pod, hence multiple interfaces.

The `name` field in the `k8s.v1.cni.cncf.io/network-status` annotation refers to
the name of a NetworkAttachmentDefinition.