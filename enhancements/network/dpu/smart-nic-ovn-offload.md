---
title: DPU support in Tenant cluster
authors:
  - "@zshi-redhat"
reviewers:
  - "@Billy99"
  - "@bn222"
  - "@danwinship"
  - "@erig0"
  - "@fabrizio8"
  - "@pliurh"
  - "@trozet"
approvers:
  - "@dcbw"
  - "@knobunc"
creation-date: 2021-04-13
last-updated: 2021-10-13
status: implementable
---

# DPU support in Tenant cluster

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

See [DPU Support in OCP (Overview)](.enhancements/network/dpu/ocp-on-dpu.md). This document describes the
OCP modification in Tenant cluster in details.

## Terminology

- DPU ARM cores: The embedded aarch64 system running on DPU CPU(s)
- Regular OpenShift node(s): x86 server with no DPU provisioned as an OpenShift
node
- SR-IOV: Single Root I/O Virtualization, a NIC technology that allows
isolation of PCIe resources
- SR-IOV Pods: SR-IOV Pods are pods bound to a VF which is used to send and
receive packets on.
- PF: Physical Function, the physical ethernet controller that supports SR-IOV
- VF: Virtual Function, the virtual PCIe device created from PF. A given PF
only supports a limited number of VFs (hardware dependant) and is in the order
to 128 or 256 VFs per PF with NVIDIA BlueField2.
- VF representor: The port representor of the VF. VF and VF representor are
created in pairs where VF netdev appears in DPU host and VF representor
appears in DPU ARM cores

## Motivation

See Overview doc.

### Goals

- Create a "Developer Preview" version of the feature for OCP 4.10
- Only support NVIDIA BlueField-2
- Manage DPU host configurations via OpenShift Operators

### Non-Goals

- DPU configuration management in Infrastructure cluster is discussed in [Introduce DPU OVNKube Operator](.enhancements/network/dpu/dpu-network-operator.md)
- Support multiple DPUs on one OpenShift baremetal worker node
- Support DPUs on OpenShift baremetal master nodes

## Proposal - Dev Preview

- Configure the Tenant cluster with the following major steps:
  - User installs SR-IOV Network Operator (OLM managed)
  - User creates one or more custom MachineConfigPools whose nodes contain DPU hardware
  - User creates DPU related MachineConfigs to the custom MachineConfigPools
  - MachineConfigOperator applies MachineConfigs and reboots DPU hosts
  - User configures DPU host devices via SR-IOV Network Operator
  - User creates OVNKubernetes network-attachment-definition and env-overrides config map
  - User labels DPU host with `network.operator.openshift.io/dpu-host` in custom MachineConfigPools
  - Nodes in custom MachineConfigPools become ready and start serving SR-IOV pod creation request
  - User creates SR-IOV pod by specifying OVNKubernetes network-attachment-definition in pod annotations
  - SR-IOV pod starts with VF device as pod default route interface on DPU host

### User Stories

### API Extensions

### Implementation Details/Notes/Constraints

DPU OVN offload will be enabled with OVN-Kubernetes CNI only.
It is expected that the following changes will be made:

#### OVN-Kubernetes

There are three modes introduced in OVN-Kubernetes for running ovnkube-node:
- **full**: No change to current ovnkube-node logic, runs on regular OpenShift nodes
- **smart-nic-host**: ovnkube-node on DPU host, assigns VFs to pods but does not do any of the other ovnkube-node work
- **smart-nic**: ovnkube-node on DPU ARM cores, creates an OVN bridge and attaches VF representor devices for each DPU host pod

These names use `smart-nic` rather than `dpu` because they come from upstream
kubernetes code that was committed before we settled on a naming scheme.

```html
                                                                         +--------------------------+
                                                                         |     Baremetal worker     |
                                                                         |     (DPU host)           |
+--------------------------+                                             |                          |
|    Baremetal worker      |                                             | +----------------------+ |
| (regular OpenShift Node) |                                             | |  ovnkube-node in     | |
|                          |                     +-----------------------+-+                      | |
| +----------------------+ |                     |                       | |  smart-nic-host mode | |
| |  ovn-controller      | |      +--------------+---------------+       | +----------------------+ |
| +----------------------+ |      | Master       |               |       |                          |
|                          |      |              |               |       +------------+-------------+
| +----------------------+ |      |    +---------v-----------+   |                    |
| |  ovnkube-node in     | |      |    |                     |   |       +------------+-------------+
| |                      +-+------+---->   K8s APIServer     |   |       |           DPU            |
| |  full mode           | |      |    |                     |   |       |                          |
| +----------------------+ |      |    +---------^-----------+   |       | +----------------------+ |
|                          |      |              |               |       | |  ovn-controller      | |
+------------+-------------+      +--------------+---------------+       | +----------------------+ |
                                                 |                       |                          |
                                                 |                       | +----------------------+ |
                                                 |                       | |  ovnkube-node in     | |
                                                 +-----------------------+-+                      | |
                                                                         | |  smart-nic mode      | |
                                                                         | +----------------------+ |
                                                                         |                          |
                                                                         +--------------------------+
```

On DPU, ovn-controller and ovnkube-node are running as containers, OvS is running as system service.
On DPU host, ovnkube-node is running as a container, OvS and ovn-controller are not running.
On regular OpenShift nodes, ovn-controller and ovnkube-node are running as containers, OvS is running as system service.

#### Cluster Network Operator

A new label `network.operator.openshift.io/dpu-host` will be introduced
in Cluster Network Operator (CNO) to allow proper scheduling of OVN-Kubernetes
components on DPU hosts and non-DPU hosts.

Ovnkube-node-dpu-host daemonset will be created in CNO ovn-kubernetes
manifests, which executes ovnkube-node in `smart-nic-host` mode.
ovnkube-node-dpu-host daemonset runs only on DPU hosts.

Ovnkube-node-dpu-host daemonset is mounted with the config map
`env-overrides` which contains node specific ovn-k8s management port
information. This information is used as input parameters to start
ovnkube-node-dpu-host daemonset. The management port is an SR-IOV VF
device and will be renamed to `ovn-k8s-mp0` port by ovnkube-node-dpu-host.

```go
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: env-overrides
  namespace: openshift-ovn-kubernetes
data:
  worker-wsfd-advnetlab52: |
    OVNKUBE_NODE_MGMT_PORT_NETDEV=ens1f0v0
  worker-wsfd-advnetlab51: |
    OVNKUBE_NODE_MGMT_PORT_NETDEV=ens1f0v0
```

Daemonsets of OVN-Kubernetes components other than ovnkube-node-dpu-host
will be modified with `nodeAffinity` to only on regular OpenShift nodes.

#### Manual steps

DPU offload will be enabled for nodes in custom MachineConfigPool.

User will apply DPU related MachineConfigs which include service that reverts
default OVN-Kubernetes ovs-configuration, and dropin service that disables
Open vSwitch services (openvswitch, ovs-vswitchd, ovsdb-server). DPU related
MachineConfigs will be provided in the dev-preview document.

User will create `SriovNetworkNodePolicy` CRs for nodes in the custom
MachineConfigPool. The creation of `SriovNetworkNodePolicy` CRs will trigger
the provision of SR-IOV devices on those nodes and advertise SR-IOV devices
(except VF 0) to kubernetes node status as extended resource. VF with index 0
will be used by OVN-Kubernetes as OVNKubernetes management port `ovn-k8s-mp0`.
User need to ensure VF with index 0 is not added to the advertised resource.

User will configure [DNS Operator placement API](https://github.com/openshift/api/blob/release-4.8/operator/v1/types_dns.go#L64) based on
custom MachineConfigPool specified to isolate DNS pods from running on DPU hosts
(see [Open Questions](#open-questions) for why removing DNS pods from DPU hosts).

User will apply label `network.operator.openshift.io/dpu-host` to nodes in the
custom MachineConfigPool which drives ovn-controller and ovn-ipsec daemonset
pods away from DPU hosts and allows scheduling of ovnkube-node-dpu-host
daemonset pods onto DPU hosts.

User will create the `env-overrides` configmap to pass node specific
OVNKubernetes management port information.

#### SR-IOV Network Operator

There are three supported modes with NVIDIA DPU BlueField2:
- **Separated host (default)**: The embedded ARM system and the function
exposed to the host are both symmetric. Each one of those functions has its
own MAC address and is able to send and receive packets.
- **Embedded CPU**: The embedded ARM system controls the NIC resources and
data path. A network function is still exposed to the host, but it has limited
privileges.
- **Restricted**: Also referred to as isolated mode, for security and isolation
purposes, it is possible to restrict the host from performing operations that
can compromise the DPU.

BlueField2 needs to be configured to **Embedded CPU** or **Restricted** mode
when used for OVN hardware offload. SR-IOV network config daemon, a daemonset
managed by SR-IOV Network Operator, will be updated to use mstflint version >=
`4.15.0-1.el8` which supports BlueField2 mode configuration.

SR-IOV Network Operator identifies the NIC using its device ID, the NVIDIA
BlueField2 device ID will be recoganized in SR-IOV Network Operator.

SR-IOV Network Operator will move to use systemd service for configuring
devices vs configuring devices in SR-IOV config daemonset pods. This ensures
that SR-IOV device with VF index 0 (which is used as ovn management port)
becomes available before ovnkube-node-dpu-host pod starts.

SR-IOV Network Operator will apply udev rule to DPU host VF0 to ensure the VF
interface name is consistant across rebooting and kernel upgrade.

### Risks and Mitigations

#### Setting up initial DPU connectivity

It is required to establish an initial transparent network connection between
DPU host and external network through DPU ARM cores in order to deploy DPU
host as an OpenShift worker node. This will be addressed by deploying DPU
as a worker node in a separate infrastructure cluster and applying network
configurations via MachineConfig in that cluster.

## Design Details

### Open Questions

#### Cluster Pods on DPU host

With current OVN-Kubernetes design, it is only supported to run SR-IOV pods on
the DPU host. Regular pods whose networks require virtual **veth** interfaces
cannot be plugged to OVN network because there will be no Open vSwitch running
on the DPU host.

To create a SR-IOV pod, it is needed to request the SR-IOV resource explicitly
in pod spec, for example:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: sriov-pod
spec:
  containers:
  - name: sriov-container
    image: centos
    resources:
      requests:
        openshift.io/<sriov-resource-name>: 1
      limits:
        openshift.io/<sriov-resource-name>: 1
```

In the above pod yaml file:

`openshift.io/<sriov-resource-name>` is the full resource name that follows
the [extended resource naming scheme](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#extended-resources),
it is advertised by SR-IOV Network Device Plugin and published in Kubernetes
node status. The SR-IOV Network Device Plugin is a component that discovers
and manages the limited SR-IOV devices on the DPU host. The managed
SR-IOV devices, AKA VFs, are created from DPU host PF.

`1` is the number of SR-IOV devices requested for the `sriov-pod`. When the
requested number of SR-IOV devices are not available on the target node, the
`sriov-pod` will be in `Pending` state until resource become available again
(e.g. other SR-IOV pods release the VF device on the target node).

Given that regular **veth** pods cannot be created on the DPU host,
failure would happen if some cluster pods or infra pods using **veth**
interfaces are scheduled on the DPU host, examples of cluster or infra
pods are as below:

- ingress-canary in openshift-ingress-canary namespace
- image-pruner in openshift-image-registry namespace
- network-metrics-daemon in openshift-multus namespace
- network-check-source & network-check-target in openshift-network-diagnostics
- Pods in openshift-marketplace & openshift-monitoring namespaces

There are several possible ways to avoid the failure:

- Modify cluster pod manifests to run with `hostNetwork: true` which doesn't
require OVN-Kubernetes CNI to create pod network, this has drawback that it
gives cluster pod additional privileges which may not be necessarily needed.
- Modify cluster pod manifests to select non-DPU host, this may have
issues for pods that need to run on every worker node, including the DPU
host. This also implies that additional non-DPU worker nodes need be
available
- Find a way for the cluster pods to consume SR-IOV device by default when they
are scheduled on the DPU host. We cannot simply add SR-IOV resource request
in cluster pod manifests, since the cluster pods are usually daemonsets or
deployments, adding SR-IOV resource request in daemonsets or deployments will
result in their pods be scheduled only on nodes that advertising the SR-IOV
resources, this doesn't work for non-DPU host
- Use mutation admission controller to inject SR-IOV resource request in pod
manifest so that pods can use SR-IOV device as their default interface.
The problem with this approach is that admission controller runs before the
pod is scheduled, it cannot know whether the pod is going to end up on a
DPU host or Regular OpenShift node.
- Find a way to enable Open vSwitch for non-SR-IOV pods on DPU host
- Taint the DPU hosts so that pods without corresponding toleration won't
be scheduled to DPU hosts

### Test Plan

- Testing will be done manually by developers, no QE testing for Dev Preview

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

Ovnkube-node-dpu-host on DPU hosts communicates to ovnkube-node on DPU nodes
through Kubernetes APIs (by annotating pod metadata), version skew can occur
when the two ovnkube-node are using different annotation format. We will have
to make sure OVN-Kubernetes has backward compatibility on pod annotations used
for passing DPU device information.

SR-IOV, CNO, MCO and DNS Operators coordinate on DPU configurations,
version skew may occur when CNO, MCO or DNS has API changes that are used by
SR-IOV Network Operator for inter-Operator communications.

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

N/A

## Infrastructure Needed

N/A
