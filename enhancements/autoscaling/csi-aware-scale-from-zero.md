---
title: csi-aware-scale-from-zero
authors:
  - "@gnufied"
reviewers:
  - "@elmiko"
approvers:
  - "@joelspeed"
api-approvers:
  - None
creation-date: 2026-09-21
last-updated: 2026-09-22
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/STOR-3099
see-also:
  - "/enhancements/machine-api/autoscaling-from-to-zero.md"
  - "/enhancements/machine-api/cluster-autoscaler-operator.md"
replaces: []
superseded-by: []
---

# CSI volume limits in scheduler and Cluster Autoscaler

## Summary

Adopt [KEP-5030](https://github.com/kubernetes/enhancements/tree/master/keps/sig-autoscaling/5030-attach-limit-autoscaler)
as an OpenShift feature. Cluster Autoscaler in supported
clusters and cloud environments will use appropriate CSI node limits
either from `MachineSet` objects or from existing `CSINode` in the cluster.

In supported environments we will disable placement of pods to nodes
where CSI driver is not installed.


## Motivation

Cluster Autoscaler currently assumes unlimited number of volumes can be attached
to nodes during scheduler simulation. This leads to bugs like https://redhat.atlassian.net/browse/OCPBUGS-42358

Separately, the scheduler currently allows scheduling pods to nodes
where no CSI driver is installed. This leads to pods getting
stuck on a node.

### User Stories

* As a cluster administrator, I want autoscaling to account for CSI attachment
  limits so that it creates enough nodes for pending stateful workloads.
* As a cluster administrator, I want the scheduler to wait for a supported CSI
  driver to register on a new node so that pods are not assigned using an
  unknown attachment limit.

### Goals

* Support CSI-aware simulation for node groups with running nodes and for
  supported groups scaled to zero.
* Opt supported OpenShift-managed CSI drivers into the scheduler behavior only
  when Cluster Autoscaler has matching CSI awareness.

### Non-Goals

* Supporting autoscalers other than Cluster Autoscaler.
* Descheduling pods already assigned to an unsuitable node.
* Deriving attachment limits for arbitrary third-party CSI drivers.
* Changing CSI `NodeGetInfo` or attachment enforcement on real nodes.

## Proposal

Kubernetes 1.37 enables the beta `VolumeLimitScaling` feature by default in the
API server and scheduler. But the behaviour of blocking scheduling is an opt-in
that requires `CSIDriver` object to have `preventPodSchedulingIfMissing` field
set to `true`.

### Autoscaler changes

The Cluster Autoscaler(CA) version shipped in OpenShift
5.1 likewise enables CSI node-aware scheduling by default. For scaling nodes
in nodes with existing nodegroups, the new scheduler behaviour requires no further
changes from underlying Kubernetes distribution(Openshift). 

For scaling from zero, CA requires some sort of help from underlying cloudprovider/platform
to inform it about CSI drivers that will get installed and volumes limits they 
will have.

In Openshift-5.1 on supported platforms (aws, gcp, azure to start with), we will implement
Cluster API based annotations as documented in - https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/cloudprovider/clusterapi/README.md#pre-defined-csi-driver-information-on-nodes-scaled-from-zero 

Each supported Machine API provider's MachineSet controller reconciles this
annotation, extending the existing [scale-from-zero capacity
mechanism](../machine-api/autoscaling-from-to-zero.md). For example:

```yaml
metadata:
  annotations:
    capacity.cluster-autoscaler.kubernetes.io/csi-driver: "ebs.csi.aws.com=27"
```

The value is illustrative. Providers derive conservative limits from the
MachineSet template and supported CSI driver configuration, including for
MachineSets created at zero replicas.

This feature should have no negative interaction and will be introduced as a
techPreview feature(which we aim to remove before 5.1 release) called - `AutoscalerCSILimits`.

### Scheduler changes

The scheduler behavior remains opt-in per driver and is not a requirement
for CA changes. But `CSIDriver.spec.preventPodSchedulingIfMissing` should
only be enabled if CA is correctly configured to calculate volume limits.

In Openshift-5.1 when `AutoscalerCSILimits` featuregate is enabled, we will
set `preventPodSchedulingIfMissing` to `true` on those platforms
and deployments.

### Workflow Description

No additional user workflow is required. Once cluster and machine autoscaling
are configured, CSI limits participate in scale-up simulation automatically.

### API Extensions

Propose `AutoscalerCSILimits` as a new TechPreviewNoUpgrade featuregate in OCP.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift uses Cluster Autoscaler with Cluster API resources backing its
NodePools. Scale-from-zero support requires CSI capacity annotations on those
resources in the management cluster, using limits appropriate to the hosted
cluster's workers. The proposed writer is the HyperShift Operator's NodePool
controller, which already reconciles scale-from-zero capacity annotations on
backing MachineDeployments or MachineSets. It needs access to the hosted
cluster's CSI limits and end-to-end coverage.

#### Standalone Clusters

Standalone clusters using Machine API or Cluster API are the initial target.

#### Single-node Deployments or MicroShift

SNO and MicroShift do not use this machine autoscaling path. The feature adds no
node daemon or steady-state node resource cost.

#### OpenShift Kubernetes Engine

The feature is available when the supported machine autoscaling and CSI driver
operators are present; it has no dependency on an OCP-only workload service.

### Implementation Details/Notes/Constraints

The following downstream integration is required:

* `openshift/machine-api-provider-aws` and the corresponding Azure and GCP
  providers extend their MachineSet capacity annotation controllers to publish
  CSI driver limits and refresh them when relevant configuration changes.
* `openshift/cluster-autoscaler-operator` relies on the operand's default CSI
  node-aware behavior and does not add a command-line flag.
* OpenShift CSI driver operators set
  `preventPodSchedulingIfMissing` for drivers supported in OpenShift 5.1.

### Risks and Mitigations

I think the main risk is if `preventPodSchedulingIfMissing` is set to `true`
and some issue causes Cluster AutoScaler to not take into account CSI attach
limits from Cluster API objects, in whicah case scaling from zero may be broken.

We should also verify if this works correctly in Clusters that use Karpenter.

We will iron out any issues regarding this via TechPreviewNoUpgrade featureGate.

### Drawbacks

Provider-side limit calculations must remain consistent with CSI driver
behavior as drivers and supported instance types change.

## Alternatives (Not Implemented)

None.

## Open Questions [optional]

Should we have a separate featureGate for `preventPodSchedulingIfMissing` feature?

## Test Plan

OpenShift end-to-end tests verify the default configuration:

* scale-up from a nonzero node group creates enough nodes for pending volumes.
  Verify that machinesets created have right annotations that indicate CSI driver limits.
* scale-up in nodegroups with existing node should create enough nodes for pending
  volumes.

## Graduation Criteria

### Dev Preview -> Tech Preview

Not applicable. 

### Tech Preview -> GA

If we don't find any issues around scaling-from-zero with `preventPodSchedulingIfMissing`
flag enabled in `CSIDriver` object, we will consider enabling this feature as GA.


### GA

* Scale from zero works without manual MachineSet annotation, including for
  groups that have never created a node.
* Scale-up failure metrics and scheduler events provide sufficient diagnostics.
* Supported drivers, platforms, and bootstrap limitations are documented.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

On upgrade to OpenShift 5.1, the API server, scheduler, and Cluster Autoscaler
gain their upstream default behavior. CSI driver operators set
`preventPodSchedulingIfMissing` only after the compatible Cluster Autoscaler is
available. Generated MachineSet annotations are inert to older autoscalers.

## Version Skew Strategy

The scheduler opt-in is safe only when the running Cluster Autoscaler supports
CSI-aware templates. Payload rollout must therefore update Cluster Autoscaler
before CSI driver operators set the field, and rollback must reverse that
order. The feature does not require kubelet changes.

## Operational Aspects of API Extensions

Operators can use scheduler events together with Cluster Autoscaler
`failed_scale_ups_total` and `unschedulable_pods_count` metrics. No new API type
or steady-state cloud-provider call is introduced.

## Support Procedures

For a CSI-backed pod that does not scale or schedule, inspect the selected node
group's template `CSINode` data, the real node's `CSINode`, the driver's
`preventPodSchedulingIfMissing` setting, and Cluster Autoscaler logs. A mismatch
between the template and real attachment limit is a product defect for a
supported driver and platform.

## Infrastructure Needed [optional]

Presubmit and periodic jobs need a cloud cluster that can scale a worker group
to zero and enough volumes and quota to exceed one node's attachment limit.
