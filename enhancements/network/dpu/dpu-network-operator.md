---
title: DPU Network Operator
authors:
  - "@pliurh"
reviewers:
  - "@zshi-redhat"
  - "@dcbw"
approvers:
  - 
creation-date: 2021-09-07
last-updated: 2021-09-07
status: implementable
---

# DPU Network Operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary

To facilitate the management of Nvidia BlueField-2 DPU, a two-cluster design is being
proposed. Under such design, a BlueField-2 card will be provisioned as a worker
node of the ARM-based infra cluster, whereas the tenant cluster where the normal
user applications run on, is composed of the X86 servers.

The OVN-Kubernetes components are spread over the two clusters. On the tenant
cluster side, the Cluster Network Operator is in charge of the management of the
ovn-kube components. On the infra cluster side, we propose to create a new
operator to be responsible for the life-cycle management of the ovn-kube components
and the necessary host network initialization on DPU cards.

## Motivation

Normally, the Cluster Network Operator watches the Network customer resource,
then provision the ovn-kube components to the Openshift cluster.

In the two-cluster design, some of the ovn-kube components have to run on BF2
DPUs, but they are still part of the ovn-kube cluster running in the tenant
cluster. To allow the ovn-kube components on different clusters to talk with
each other, we need to ensure all the necessary configs and secrets are aligned
with each other.

### Goals

- Design a new operator that
  - manages the ovn-kube components running on the DPU that serve the tenant cluster.
  - enable the switchdev mode on DPUs.
  - add PF representor to the br-ex bridge, so that the x86 host can connect to
  the cluster network of the tenant cluster.
- Define the ovn-kube upgrade mechanism in such a deployment.

### Non-Goals

- Support for network plugins other than OVN-Kubernetes.
- This enhancement does not address the two-cluster installation.
- It is possible to have one infra cluster to serve more than one tenant cluster
in the future. But it is not our initial goal.

## Pre-requisites

- This operator needs to access the apiserver of the tenant cluster, we need to
  make sure that the pods network in the infra cluster can connect to the
  apiserver of the tenant cluster in some way.
- In the infra cluster, one custom MachineConfigPool shall be created for the
  DPUs on which hardware offloading needs to be enabled.

## Proposal

We will create a namespaced CRD OVNKubeConfig.dpu.openshift.io for the DPU
OVNKube Operator. One custom resource of this CRD shall be created for the
tenant cluster in the infra cluster. The operator will take this custom resource
as input to render and create the objects of the ovn-kube DPU components. The
operator will also create a MachineConfig CR which injects a systemd service to
do necessary host configuration on DPUs.

### User Stories

### Risks and Mitigations

## Design Details

The DPU Network Operator will fetch the necessary information from the
tenant cluster, then create the following objects in the infra cluster:

- The ovnkube-node DaemonSet running in the DPU mode
- The ServiceAccount and corresponding RBAC definition
- The ConfigMap that contains ​​ovnkube-config of the tenant cluster
- The ConfigMap and Secret that contains the root CA and signed certificate of
  the tenant cluster OVN
- The ConfigMap that contains the root CA of the tenant cluster API server

The DPU Network Operator will be responsible for the following host
configuration on the DPUs which we want to enable hardware offloading:

- Add PF representor to the br-ex bridge
- Enable switchdev mode on DPU nodes
- Enable OVS hardware offloading: other_config:hw-offload=true
- Apply udev rules for BF2 interfaces

We assume a custom MachineConfigPool shall be created for all the DPUs, and the
DPU Network Operator will also create a MachineConfig CR which injects a systemd
service to do the aforementioned configuration.

### API
Example configurations:

```yaml
apiVersion: dpu.openshift.io/v1
kind: OVNKubeConfig
metadata:
  name: cluster
  namespace: tenant-cluster-1
spec:
  kubeConfigFile: tenant-cluster-1-kubeconf
  poolName: bf2-worker
status:
  conditions:
  - lastTransitionTime: ""
    status: "True"
    type: MachineConfigApplied
  - lastTransitionTime: ""
    status: "True"
    type: OvnKubeReady
  - lastTransitionTime: ""
    status: "False"
    type: TenantClusterApiServerUnreachable
```

1. `kubeConfigFile` stores the secret name of the tenant cluster kubeconfig
   file. The operator uses this to access the api-server of the tenant
   cluster.
2. `poolName` specifies the name of the MachineConfigPool CR which contains all
   the BF2 nodes in the infra cluster. The operator copies the
   `spec.nodeSelector` of the MCP to render the ovnkube-node daemonset. A new
   MachineConfig will be created for this MCP, which injects the systemd
   service.

### Lifecycle Management

The DPU Network Operator will be delivered as an OLM-managed Operator.
Users can install this operator from the embedded operator hub of OpenShift,
after the infra cluster installation is complete.

The OLM shall handle the installation, management, upgrade, and uninstallation
of this operator.

### Test Plan

TBD

### Graduation Criteria

Graduation criteria follows:

#### Dev Preview -> Tech Preview

- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- More testing (upgrade, scale)
- Add CI job at baremetal OCP CI
- Sufficient time for feedback

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy
No Kubernetes object is used, hence we could say that it is not
kubernetes-version sensitive.

The version of this operator shall be tied to the version of the ovn-kubernetes
running in the tenant cluster.

### Version Skew Strategy
N/A

### API Extensions
N/A

### Operational Aspects of API Extensions
N/A

#### Failure Modes
N/A

#### Support Procedures
N/A

## Implementation History
N/A

## Drawbacks
N/A

## Alternatives

Implement the above-mentioned function in the Cluster Network Operator. Enable
this feature when the Cluster Network Operator is running in the infra cluster.


## Infrastructure Needed

One X86 bare-metal cluster with BlueField-2 DPU installed on worker nodes, and
one ARM bare-metal cluster.
