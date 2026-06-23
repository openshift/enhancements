---
title: add-bare-metal-host-to-vsphere
authors:
  - "rvanderp3"
reviewers: 
  - "gnufied"
  - "JoelSpeed"
approvers: 
  - "JoelSpeed"
api-approvers: 
  - None
creation-date: 2025-02-10
last-updated: 2025-02-10
tracking-link: 
  - https://issues.redhat.com/browse/OCPSTRAT-1917
see-also:
  - None
replaces:
  - None
superseded-by:
  - None
---

# Bare Metal Hosts on vSphere Integrated Cluster

## Summary

As customers begin to use alternate virtualization or deployment patterns in their cluster they may need the ability to provision bare metal hosts along side their cloud integrated nodes. This is needed if a customer is unable to readily migrate their workloads to another OpenShift cluster or if they have a specific need to schedule workloads to both integrated and non-integrated nodes.

## Motivation

### User Stories

As a maintainer of an OpenShift cluster I need the ability to add bare metal hosts to my integrated OpenShift installation so I can migrate workloads to bare metal hosts without having to reinstall the cluster.

### Goals

- Enable a migration path for existing clusters to leverage nodes without cloud provider integration.
- Cluster operators will not be `degraded` or stuck in `progressing`.
- Cluster storage will not be supported in initial implementation.

### Non-Goals

- Migrating the control plane and infrastructure nodes to `platform: none`. Atleast initially, the goal is to add nodes which lack cloud provider integration to an existing `platform: vsphere` cluster.
- Autoscaling or machine/cluster API management of nodes
- Bare metal API integration
- Cluster storage support

## Proposal

Nodes added to a platform vSphere cluster are expected to be initialized by the cloud controller manager. Otherwise, the nodes will remain tainted and will not join the cluster. To ensure the bare metal nodes are initialized, they are ignited with `platform: none`. Once the bare metal nodes join the cluster, the CCM recognizes the node is there but not a part of vCenter. While this results in warnings being logged by the CCM, no events are generated and the cloud controller manager operator remains available. 

The CSI operator will attempt to schedule daemonset pods on all nodes and [tolerates most taints](https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/4479e2418f38cb93b5da4df7e043aff71a20cccc/manifests/vanilla/vsphere-csi-driver.yaml#L565-L569) due to its nodeSelector of any linux node. This results in the cluster storage operator progressing indefinitely since the CSI driver schedules a csi pod onto the bare metal node that crash loops indefinitely. The CSI operator will need to be modified to configure the node daemonset to only target vSphere nodes.

Another issue the CSI operator has with the non vSphere node is that it fails to find the node's VM in vSphere.  This is expected since its not a vSphere node.  We will need to update the vmware csi driver operator's `checkNode` logic to ignore any node that is not a vSphere node.

### Workflow Description

#### Adding a New Node Without Cloud Provider Integration to a Platform vSphere cluster

1. Install a `platform: vsphere` cluster
2. Disable Storage Operator
3. Download the RHCOS Live CD which aligns with the installed version of OpenShift.
4. Obtain or create a worker.ign file. This will be used to bootstrap the bare metal node.
5. Boot the new bare metal host from the RHCOS Live CD.
6. Install RHCOS:
```bash=
coreos-installer install /dev/sdX --insecure-ignition --ignition-url=https://path-to-compute-ignition --platform=metal
```
6. Reboot the node.
7. Approve CSRs for the node

Note: 
1. The [vSphere CSI driver daemonset](https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/4479e2418f38cb93b5da4df7e043aff71a20cccc/manifests/vanilla/vsphere-csi-driver.yaml#L565-L569) tolerates all taints. I was able to disable it by making the operator unmanaged and removing the tolerations.
2. We did attempt to fix the [vSphere CSI Driver Daemonset](https://github.com/openshift/vmware-vsphere-csi-driver-operator/pull/305) to not deploy to BM node, but we decided to make storage generally not supported in hybrid environment

### API Extensions

### Topology Considerations

#### Hypershift / Hosted Control Planes

Does not have an impact.

#### Standalone Clusters

#### Single-node Deployments or MicroShift

Does not have an impact.

#### OpenShift Kubernetes Engine

Does not have an impact.

### Implementation Details/Notes/Constraints

#### Feature Gate Definition

The mixed-node environment behavior is gated behind the `VSphereMixedNodeEnv` feature gate, defined in the OpenShift API. The feature gate allows the new behavior to be introduced incrementally without affecting existing clusters. It is initially available in the `DevPreviewNoUpgrade` feature set and applies to both Hypershift and SelfManagedHA cluster profiles.

#### Cluster Storage Operator

The Cluster Storage Operator will not support hybrid environment.  It will need to be disabled in order for the operator to not go degraded.

##### Upstream CCM: Node Identity Labeling

The upstream vSphere Cloud Controller Manager must be capable of stamping a platform-identity label onto vSphere nodes at registration time. This is the mechanism by which the rest of the stack can distinguish vSphere nodes from non-vSphere bare metal nodes without inspecting vCenter directly.

To accomplish this, the CCM is extended with a configurable node-labels flag. When the `VSphereMixedNodeEnv` feature gate is enabled, the operator passes `node.openshift.io/platform-type=vsphere` to the CCM via this flag. The CCM then applies that label to every vSphere node it initializes, establishing a durable, queryable identity on the node object.

Additionally, the CCM is updated to implement the `InstancesV2` cloud provider interface, which provides a more precise per-node lifecycle model (existence, shutdown state, and metadata). This enables accurate node classification in a mixed environment where some nodes have no corresponding vSphere VM.

##### CCM Operator Integration

The cluster-cloud-controller-manager-operator is responsible for deploying and configuring the vSphere CCM. When the `VSphereMixedNodeEnv` feature gate is enabled, the operator must configure the CCM deployment to pass the `node.openshift.io/platform-type=vsphere` node label. This ensures that every vSphere node joining the cluster receives the platform-identity label automatically, without any manual administrator intervention.

The operator reads the feature gate state at reconciliation time and conditionally includes the node label in the CCM deployment arguments. When the feature gate is disabled, the CCM deployment is left unchanged and no platform-type label is applied, preserving backwards-compatible behavior for clusters that do not require mixed-node support.

An example of a vSphere Node with the new label:
```yaml
apiVersion: v1
kind: Node
metadata:
  annotations:
    ...
  creationTimestamp: "2025-05-27T13:55:12Z"
  labels:
    beta.kubernetes.io/arch: amd64
    beta.kubernetes.io/instance-type: vsphere-vm.cpu-8.mem-16gb.os-unknown
    beta.kubernetes.io/os: linux
    failure-domain.beta.kubernetes.io/region: us-east
    failure-domain.beta.kubernetes.io/zone: us-east-1a
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: ngirard-multi-twxnm-master-2
    kubernetes.io/os: linux
    node-role.kubernetes.io/control-plane: ""
    node-role.kubernetes.io/master: ""
    node-role.kubernetes.io/worker: ""
    node.cluster.x-k8s.io/esxi-host: ci-vmware-host-2.ci.ibmc.devcluster.openshift.com
    node.kubernetes.io/instance-type: vsphere-vm.cpu-8.mem-16gb.os-unknown
    node.openshift.io/os_id: rhel
    node.openshift.io/platform-type: vsphere
    topology.csi.vmware.com/openshift-region: us-east
    topology.csi.vmware.com/openshift-zone: us-east-1a
    topology.kubernetes.io/region: us-east
    topology.kubernetes.io/zone: us-east-1a
  name: ngirard-multi-twxnm-master-2
  resourceVersion: "1701785"
  uid: 418b38e5-fea5-4549-afc1-faf938d4f546
spec:
  providerID: vsphere://42100fe1-0a04-9eb7-1b73-9a3d4578b557
```

##### check_nodes.go

The vSphere controller has a module that checks each node object to make sure it passes all environment / config checks.  As part of the checks, it validates if each node has a virtual machine (VM) found in vSphere.  In the case of the non vsphere bare metal node, the node will not have a VM in vSphere associated with it.  We need to update the check logic to verify that the node is in fact a vSphere node before checking for the VM.

The current plans are to check each node to see if the spec's providerID has been set and the label for provider-type is set.  If both of these are not set, then the node is expected to be ignored by the operator.

When the `VSphereMixedNodeEnv` feature gate is enabled, the node-check logic uses the `node.openshift.io/platform-type=vsphere` label as its gating signal. A node that does not carry this label is treated as a non-vSphere node and skipped entirely — no VM lookup is attempted and no error is raised. This prevents the operator from going degraded simply because a bare metal node has no corresponding VM in vCenter.

### Risks and Mitigations

**!!! TODO !!!**

### Drawbacks

**!!! TODO !!!**

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?

## Open Questions [optional]

**!!! TODO !!!**

## Test Plan

**Note:** *Section not required until targeted at a release.*

**!!! TODO !!!**

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

**!!! TODO !!!**

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

## Version Skew Strategy

**!!! TODO !!!**

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

N/A

## Support Procedures

**!!! TODO !!!**

## Alternatives (Not Implemented)

N/A

## Infrastructure Needed [optional]

N/A
