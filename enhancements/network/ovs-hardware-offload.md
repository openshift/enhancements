---
title: ovs-hardware-offload
authors:
  - "@zshi"
reviewers:
  - "@dcbw"
  - "@squeed"
  - "@danwinship"
  - "@pliurh"
  - "@dougbtv"
  - "@s1061123"
approvers:
  - "@knobunc"
  - "@fepan"
creation-date: 2020-04-27
last-updated: 2020-04-27
status: implementable
see-also: []
replaces: []
superseded-by: []

---

# OVS-Hardware-Offload

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

- When will RHCOS uses RHEL-8.3 as base image?
- Any known issues with connction track feature using OVS Hardware Offload?
- Any known issues with IPv6 traffic using OVS Hardware Offload?

## Summary

This enhancement is to introduce OVS hardware offload feature in OpenShift
for high-performance datapath with OVN-Kubernetes. OVS hardware offload
enables offloading of OVS datapath rules to a compatible network card using
linux Traffic Control (TC) tool `tc`. It relies on SR-IOV technology to control
the SR-IOV Virtual Functions (VF) via VF representor port that is plugged
into OpenvSwitch bridge. It improves the utilization of available bandwidth
on underlying NICs by moving CPU intensive network packet processing from
OVS software to hardware.

## Motivation

Enable high-performance datapath on primary pod interface with production-ready
OVN-Kubernetes.

### Goals

Enable OVS hardware offload for OVN datapath

### Non-Goals

Using hardware offload for any part of the OpenShift control plane

## Proposal

OVS Hardware Offload accelerates the OVS software flows with a SR-IOV NIC
that works in `switchdev` mode. It requires configurations in both OpenvSwitch
daemon and SR-IOV hardware NIC.

- OpenvSwitch daemon is managed by Cluster Network Operator (CNO) and runs in
`ovs-node` DaemonSet. A boolean configurable field will be added in CNO
`Network` Custom Resource Definition `spec.defaultNetwork.ovnKubernetesConfig`
to enable or disable OVS Hardware Offload feature for OpenvSwitch daemon.

- SR-IOV hardware NIC is managed by SR-IOV Network Operator and will be
provisioned with a new device type `switchdev`. The initial supported NIC
model will be Mellanox MT27800 Family [ConnectX-5] 25GbE dual-port SFP28.

Besides CNO and SR-IOV Operator changes, there are several other dependencies.

- OVS Hardware Offload will only be supported on bare metal deployment.

- Multus supports overwriting cluster network definition by specifying a
customized default net-attach-def in pod annotation. Customized net-attach-def
will still use `ovn-k8s-cni-overlay` CNI plugin, but with an special annotation
that requests a SR-IOV device. Customized net-attach-def will be created
automatically by SR-IOV Operator when switchdev mode device is configured.

- RHCOS image shall be based on RHEL-8.3 for fully support of OVS Hardware
Offload with connection track feature.

- OpenvSwitch package version 2.13.

- To use OVS Hardware Offload with link redundency, a bond interface needs to
be created and used as geneve endpoint of OVS bridge. Mellanox SR-IOV VF-LAG
provides hardware link-aggregation (LAG) for hardware offloaded ports. It
allows offloading of OVS rules to a linux `bond` network device that combines
two idential and hardware offload capable Physical Functions (PFs). With the
SR-IOV VF LAG capability, the NIC PFs can receive the rules that the OVS tries
to offload to the linux bond net device and offload them to the hardware
e-switch.

#### OVN-Kubernetes

- OVS Hardware Offload is a [feature available](https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/ovs_offload.md) in OVN-Kubernetes upstream.

- `ovn-k8s-cni-overlay` CNI plugin configures the pod interface by moving the VF
to pod namespace and plugging VF representor to `br-int` OVS bridge. The
relevant [patches](https://github.com/openshift/ovn-kubernetes/commit/6c96467d0d3e58cab05641293d1c1b75e5914795) are available downstream.

#### Cluster Network Operator

- Cluster Network Operator will enable hardware offload and configure tc-policy
on node OpenvSwitch instance and restart OpenvSwitch instance for hardware
offload change to take effect.

- Enabling `hw-offload` in CNO `ovs-node` DaemonSet is equivalent to executing
`ovs-vsctl set Open_vSwitch . other_config:hw-offload=true`. The default value
is `false`.

- Setting `tc-policy` in CNO `ovs-node` DaemonSet is equivalent to executing
`ovs-vsctl set Open_vSwitch . other_config:tc-policy=skip_sw`. The default
value is `none`, optional values can be one of `none`, `skip_sw` and `skip_hw`.
This field is only relevant if `hw-offload` is enabled.

#### Multus

- Multus will support specifying net-attach-def with customized annotations for
[default cluster network in pod annotations](https://github.com/intel/multus-cni/blob/master/doc/configuration.md#specify-default-cluster-network-in-pod-annotations).
Customized annotations will contain a resource name requesting SR-IOV device.

#### SR-IOV Operator

- SR-IOV Operator will support new device type `switchdev` in Operator
[SriovNetworkNodePolicy API](https://github.com/openshift/sriov-network-operator/blob/master/pkg/apis/sriovnetwork/v1/sriovnetworknodepolicy_types.go#L33).

- SR-IOV config daemon (a DaemonSet managed by SR-IOV Operator) will support
changing e-switch mode from `legacy` to `switchdev` on the PF device and
exposing VF representor information in `SriovNetworkNodeState` status. The
daemon will retain the names of PF device and VF representors by applying udev
rules. In order to guarantee the orders required by switchdev mode and VF LAG,
a systemd service profile will be used to apply switchdev related configurations.

- SR-IOV network device plugin (a Device Plugin DaemonSet managed by SR-IOV
Operator) supports advertising VF resource to kubelet.

#### Machine Config Operator (MCO) [optional]

- MCO will be used to create linux bond interface on two identical PFs that
are capable of doing OVS hardware offload. The supported bond modes are
`Active-Backup`, `Active-Active` and `LACP`. This is not required if OVS
Hardware Offload is used on PF device directly.

### Implementation Details/Notes/Constraints

- SR-IOV Operator should first provision PFs with desired number of VFs,
then configure PFs to `switchdev` mode.

- When enabling OVS Hardware Offload with SR-IOV VF LAG, MCO needs to apply
linux bond configuration. It's important that SR-IOV Operator (installed day 2)
makes sure `switchdev` configurations are applied before linux bond
configuration, this requires a node rebooting to guarantee the order if linux
bond configuration is applied upfront.

- When enabling OVS `hw-offload` option for OpenvSwitch daemon, it is required
that ovsdb is created first.

### Risks and Mitigations

RHEL-8.3 contains kernel and driver changes to fully support OVS Hardware
Offload with connection track feature. If RHEL-8.3 is not used for RHCOS in
4.6, OVS Hardware Offload feature will not be fully available.

### Test Plan

- The changes in each relevant component must pass full e2e suite.
- Origin e2e tests mast pass on bare metal with OVS Hardware Offload enabled
in OpenvSwitch daemon.
- OVS hardware offload function must work on supported NICs.

### Graduation Criteria

To Tech Preview:
- Bare metal cluster can be brought up when hardware offload is enabled.
- A mixed of pods using offload and not using offload can co-exist on
different nodes and communicate with each other.
- Network policies can be applied to block pod-to-pod, service-to-service
and pod-to-service traffic.

### Upgrade / Downgrade Strategy

- Upgrading an existing cluster to a version that supports OVS Hardware Offload
doesn't require any change in configurations, API use or invocations.

- All the OVS Hardware Offload configuration will need to be removed before
downgrading to a version that doesn't support OVS Hardware Offload.

### Version Skew Strategy

- OVS hardware offload is an addon feature to existing components that involved.
The upgrade tests will be conducted in each individual component.

- In a rolling update, some nodes will have the current version and some will
have the new version. Since all nodes interact with the master and OVS hardware
offload is a node specific configuration, version skew can occur between nodes.

- The update of Multus CNI config file doesn't affect the use of customized
net-attach-def for default cluster network.

## Implementation History

- OpenvSwitch package [version 2.13](https://github.com/openshift/ovn-kubernetes/pull/122) is currently used in OpenShift
- Relevant patches in `ovn-k8s-cni-overlay` CNI plugin are available in OpenShift OVN-Kubernetes
- Multus is in GA support since 4.1 release and contains relevant patches for specifying customized default net-attach-def
- [SR-IOV Operator](https://docs.openshift.com/container-platform/4.3/networking/hardware_networks/about-sriov.html) is in GA support since 4.3 release


## Drawbacks

OVS Hardware Offload leverages TC flower and actions that are available in the
linux kernel for traffic classification. When OVS Hardware Offload is enabled,
any future component that needs apply TC rules directly on primary NIC that
used by OVS for hardware offloading may cause potential issue by setting
conflicted rules.

## Alternatives

Enable OVS-DPDK and use the DPDK generic flow interface (rte_flow) for OVS
Hardware Offload.
