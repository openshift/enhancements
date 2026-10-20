---
title: hosted-cluster-autoscaler-configuration-parity
authors:
  - "@georgelipceanu"
reviewers:
  - "@csrwng"
  - "@sdminonne"
  - "@devguyio"
  - "@jparill"
  - "@mehabhalodiya"
  - "@JoelSpeed"
approvers:
  - "TBD"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-10-02
last-updated: 2026-10-13
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3600
  - https://redhat.atlassian.net/browse/CNTRLPLANE-4426
see-also:
  - https://redhat.atlassian.net/browse/OCPSTRAT-1806
  - https://github.com/openshift/hypershift/pull/6236
  - https://redhat.atlassian.net/browse/CNTRLPLANE-4427
  - https://redhat.atlassian.net/browse/CNTRLPLANE-4428
  - https://redhat.atlassian.net/browse/CNTRLPLANE-4429
  - https://redhat.atlassian.net/browse/CNTRLPLANE-4430
replaces: []
superseded-by: []
---

# ClusterAutoscaler configuration parity for hosted control planes

## Summary

This enhancement lets hosted cluster administrators configure seven remaining
ClusterAutoscaler settings through `HostedCluster.spec.autoscaling`: local-storage
node protection, balancing of similar node groups, DaemonSet utilization,
cluster-wide CPU/memory/GPU limits, cordoning during scale-down, a delay for
new pods during scale-up, and autoscaler log verbosity. HyperShift translates
these settings into arguments for its per-hosted-cluster autoscaler. Omitted
settings retain the current HyperShift behavior. 

## Motivation

[OCPSTRAT-1806](https://redhat.atlassian.net/browse/OCPSTRAT-1806) and
[HyperShift PR #6236](https://github.com/openshift/hypershift/pull/6236) introduced
`ClusterAutoscaling`, including expanders, scale-down timings, and several global
limits. HyperShift still hardcodes `--skip-nodes-with-local-storage=false`,
`--balance-similar-node-groups=true`, and `--v=4` in its
[Deployment template](https://github.com/openshift/hypershift/blob/main/control-plane-operator/controllers/hostedcontrolplane/v2/assets/cluster-autoscaler/deployment.yaml).
The other four settings in this proposal have no public fields or generated
flags yet which administrators need for configuring `ClusterAutoscaling`.

### User Stories

1. As a hosted cluster administrator, I want to protect nodes running pods with
  supported local volumes so that scale-down follows my workload's eviction policy.
2. As a platform engineer, I want aggregate resource limits across NodePools so that
  autoscaler-driven growth stays within a capacity budget.
3. As a hosted cluster administrator, I want to tune balancing, DaemonSet utilization,
  cordoning, and new-pod delay so that autoscaling follows my workload pattern.
4. As an SRE, I want to change the hosted cluster autoscaler's log verbosity so
  that I can inspect scaling decisions and restore normal log volume afterward.
5. As an SRE, I want HostedCluster status to show whether requested autoscaler
  settings reached the running autoscaler, so I can detect unapplied settings
  during upgrades without inspecting management-cluster Deployment arguments.

### Goals

* Make the seven settings in
  [CNTRLPLANE-4426](https://redhat.atlassian.net/browse/CNTRLPLANE-4426)
  configurable per hosted cluster.
* Preserve effective behavior for every omitted setting, including on existing clusters.
* Define admission validation, upgrade behavior, documentation, and tests for every
  new setting.
* Deliver GPU limits only after an end-to-end test proves enforcement in the
  Cluster API autoscaler topology.

### Non-Goals

* Changing the existing HyperShift defaults.
* ROSA/OCM API integration, tracked by
  [ROSA-88](https://redhat.atlassian.net/browse/ROSA-88).
* Installing or using the standalone `ClusterAutoscaler` CR or its operator inside
  a hosted cluster.
* Guaranteeing that resource limits constrain manual NodePool changes or capacity
  already present above a configured maximum.

## Proposal

Add typed fields to the existing `ClusterAutoscaling` and `ScaleDownConfig` types,
plus `ScaleUpConfig` and `ResourceLimits` types. The same `ClusterAutoscaling` type is
used by `HostedCluster` and `HostedControlPlane`. The HostedCluster controller already
copies `spec.autoscaling` to the HostedControlPlane; the control plane operator (CPO)
will adapt the cluster-autoscaler Deployment arguments from the latter.

### Workflow Description

1. On a management cluster with the gate enabled, the administrator configures
   `HostedCluster.spec.autoscaling` and enables autoscaling on one or more
   NodePools.
2. The HostedCluster controller mirrors autoscaling configuration to the
   HostedControlPlane. CPO reconciles one autoscaler Deployment in that
   hosted control plane's management-cluster namespace.
3. The autoscaler reads guest-cluster pods and nodes using its kubeconfig. With
   `--cloud-provider=clusterapi`, it discovers and scales the backing CAPI node
   groups in the management cluster. A global resource limit therefore applies
   to the aggregate guest-cluster nodes, across all autoscaled NodePools.
4. Updating or removing a field reconciles the arguments and rolls the Deployment.
   Removing all new fields restores the effective pre-enhancement configuration.
5. CPO reports whether the desired arguments have reached a completed autoscaler
   Deployment rollout. The HostedCluster controller exposes this result in
   HostedCluster status when any new setting is requested.

Example proposed HostedCluster configuration:

```yaml
apiVersion: hypershift.openshift.io/v1beta1
kind: HostedCluster
metadata:
  name: example
  namespace: clusters
spec:
  autoscaling:
    scaling: ScaleUpAndScaleDown
    localStorageScaleDown: Skip
    similarNodeGroupBalancing: Balance
    daemonSetUtilization: Exclude
    logVerbosity: 6
    resourceLimits:
      cores:
        min: 0
        max: 128
      memory:
        min: 0
        max: 512
      gpus:
        - type: nvidia-tesla-t4
          min: 0
          max: 8
    scaleDown:
      cordonNodeBeforeTerminating: Enabled
    scaleUp:
      newPodScaleUpDelay: 30s
```

**NOTE:** This is a partial `spec` example; a complete HostedCluster also needs its existing
required platform, release, networking, and service fields.

### API Extensions

Add typed fields and enums to `api/hypershift/v1beta1/hostedcluster_types.go`:

```go
// ScaleDownConfig gains one new field.
type ScaleDownConfig struct {
	// ... existing fields are unchanged ...

	// cordonNodeBeforeTerminating controls whether the autoscaler cordons a guest-cluster node, marking it
	// unschedulable, before draining and terminating it during scale-down (see Topology Considerations for
	// the cordon/taint/Machine-deletion ordering). This affects the "--cordon-node-before-terminating" flag
	// on cluster-autoscaler.
	//
	// When omitted, the autoscaler's own default of not cordoning applies.
	//
	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	CordonNodeBeforeTerminating CordonNodeBeforeTerminatingPolicy `json:"cordonNodeBeforeTerminating,omitempty"`
}

// LocalStorageScaleDownPolicy controls whether nodes running pods with local storage are
// eligible for scale-down.
// +kubebuilder:validation:Enum="";Skip;Consider
type LocalStorageScaleDownPolicy string

const (
	// LocalStorageScaleDownSkip protects nodes running pods with local storage from scale-down.
	LocalStorageScaleDownSkip LocalStorageScaleDownPolicy = "Skip"

	// LocalStorageScaleDownConsider allows nodes running pods with local storage to be
	// considered for scale-down like any other node.
	LocalStorageScaleDownConsider LocalStorageScaleDownPolicy = "Consider"
)

// SimilarNodeGroupBalancingPolicy controls whether the autoscaler balances similar node
// groups when scaling up.
// +kubebuilder:validation:Enum="";Balance;DoNotBalance
type SimilarNodeGroupBalancingPolicy string

const (
	// SimilarNodeGroupBalancingBalance directs the autoscaler to keep similar node groups at
	// similar sizes when scaling up.
	SimilarNodeGroupBalancingBalance SimilarNodeGroupBalancingPolicy = "Balance"

	// SimilarNodeGroupBalancingDoNotBalance allows the autoscaler to scale up similar node
	// groups without balancing their sizes.
	SimilarNodeGroupBalancingDoNotBalance SimilarNodeGroupBalancingPolicy = "DoNotBalance"
)

// DaemonSetUtilizationPolicy controls whether DaemonSet pod resource requests are included
// when calculating node utilization for scale-down eligibility.
// +kubebuilder:validation:Enum="";Include;Exclude
type DaemonSetUtilizationPolicy string

const (
	// DaemonSetUtilizationInclude counts DaemonSet pod resource requests toward node
	// utilization for scale-down eligibility.
	DaemonSetUtilizationInclude DaemonSetUtilizationPolicy = "Include"

	// DaemonSetUtilizationExclude excludes DaemonSet pod resource requests from node
	// utilization for scale-down eligibility.
	DaemonSetUtilizationExclude DaemonSetUtilizationPolicy = "Exclude"
)

// CordonNodeBeforeTerminatingPolicy controls whether the autoscaler cordons a node, marking it
// unschedulable, before draining and terminating it during scale-down.
// +kubebuilder:validation:Enum="";Enabled;Disabled
type CordonNodeBeforeTerminatingPolicy string

const (
	// CordonNodeBeforeTerminatingEnabled directs the autoscaler to cordon a node before draining and
	// terminating it during scale-down.
	CordonNodeBeforeTerminatingEnabled CordonNodeBeforeTerminatingPolicy = "Enabled"

	// CordonNodeBeforeTerminatingDisabled directs the autoscaler not to cordon a node before draining
	// and terminating it during scale-down.
	CordonNodeBeforeTerminatingDisabled CordonNodeBeforeTerminatingPolicy = "Disabled"
)

// ScaleUpConfig configures the behavior of the Cluster Autoscaler scale up operation.
type ScaleUpConfig struct {
	// newPodScaleUpDelay delays scale-up evaluation for pods younger than this duration, measured from
	// the pod's creation timestamp on the guest cluster. This is useful for short-lived, bursty, or
	// batch workloads that should not trigger scale-up before they have a chance to be scheduled on
	// existing capacity. Expressed as a Go duration string, for example "30s". This affects the
	// "--new-pod-scale-up-delay" flag on cluster-autoscaler.
	//
	// A shorter per-pod "cluster-autoscaler.kubernetes.io/pod-scale-up-delay" annotation on an
	// individual pod cannot override this value; the autoscaler keeps the longer of the two delays.
	//
	// +required
	// +kubebuilder:validation:Pattern=^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:XValidation:rule="duration(self) <= duration('1h')",message="newPodScaleUpDelay must not exceed 1h"
	NewPodScaleUpDelay string `json:"newPodScaleUpDelay,omitempty"`
}

// ResourceLimits specifies cluster-wide ceilings on aggregate guest-cluster node resources
// that the autoscaler may grow the hosted cluster to, across all NodePools.
// +kubebuilder:validation:MinProperties=1
type ResourceLimits struct {
	// cores bounds the total CPU cores, in whole cores, across all guest-cluster nodes.
	// This affects the "--cores-total" flag on cluster-autoscaler.
	//
	// When omitted, the autoscaler's own default applies (effectively unlimited).
	//
	// +optional
	Cores ResourceRange `json:"cores,omitzero"`

	// memory bounds the total memory, in whole GiB, across all guest-cluster nodes.
	// This affects the "--memory-total" flag on cluster-autoscaler.
	//
	// When omitted, the autoscaler's own default applies (effectively unlimited).
	//
	// +optional
	Memory ResourceRange `json:"memory,omitzero"`

	// gpus bounds the total number of GPU devices of each listed type across all
	// guest-cluster nodes. Each entry's type must match the value of the
	// "cluster-api/accelerator" label that administrators set on the GPU-capable NodePool
	// and that is applied to its guest Nodes. This affects the "--gpu-total" flag on
	// cluster-autoscaler, rendered once per entry.
	//
	// When omitted, no GPU limits are applied.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	GPUs []GPULimit `json:"gpus,omitempty"`
}

// ResourceRange specifies a minimum and maximum bound, in whole units, for a cluster-wide
// CPU or memory resource limit. Both bounds are required so an explicit zero minimum stays
// distinct from an omitted range.
// +kubebuilder:validation:XValidation:rule="self.min <= self.max",message="min must not exceed max"
type ResourceRange struct {
	// min is the minimum amount of this resource that scale-down must preserve across all
	// guest-cluster nodes. A configured minimum prevents scale-down from reducing aggregate
	// capacity below this floor; it does not trigger scale-up to reach it.
	//
	// +required
	// +kubebuilder:validation:Minimum=0
	Min *int32 `json:"min,omitempty"`

	// max is the maximum amount of this resource allowed across all guest-cluster nodes.
	// The autoscaler will not scale up beyond this ceiling.
	//
	// +required
	// +kubebuilder:validation:Minimum=1
	Max *int32 `json:"max,omitempty"`
}

// GPULimit specifies a minimum and maximum bound on the number of GPU devices of a given
// accelerator type across all guest-cluster nodes.
// +kubebuilder:validation:XValidation:rule="self.min <= self.max",message="min must not exceed max"
type GPULimit struct {
	// type identifies the GPU accelerator type this limit applies to, matching the value of
	// the "cluster-api/accelerator" label on the guest Nodes it applies to. It is a label
	// value, not a resource name. The values "cpu" and "memory" are reserved and rejected,
	// even though both satisfy label-value syntax: the autoscaler's resource limiter stores
	// CPU, memory, and GPU limits in maps keyed by those names, so a GPU entry using either
	// reserved name would overwrite the corresponding resource limit.
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$
	// +kubebuilder:validation:XValidation:rule="self != 'cpu' && self != 'memory'",message="type must not be \"cpu\" or \"memory\""
	Type string `json:"type,omitempty"`

	// min is the minimum number of GPU devices of this type that scale-down must preserve
	// across all guest-cluster nodes. A configured minimum prevents scale-down from
	// reducing aggregate capacity below this floor; it does not trigger scale-up to reach
	// it.
	//
	// +required
	// +kubebuilder:validation:Minimum=0
	Min *int32 `json:"min,omitempty"`

	// max is the maximum number of GPU devices of this type allowed across all
	// guest-cluster nodes. A value of 0 blocks all scale-up of this GPU type. The
	// autoscaler will not scale up beyond this ceiling.
	//
	// +required
	// +kubebuilder:validation:Minimum=0
	Max *int32 `json:"max,omitempty"`
}

// ClusterAutoscaling gains six new fields.
type ClusterAutoscaling struct {
	// ... existing fields (scaling, scaleDown, balancingIgnoredLabels, maxNodesTotal,
	// maxPodGracePeriod, maxNodeProvisionTime, maxFreeDifferenceRatioPercent,
	// podPriorityThreshold, expanders) are unchanged ...

	// localStorageScaleDown controls whether the autoscaler considers nodes running pods
	// with local storage, such as EmptyDir or HostPath volumes, as eligible for scale-down.
	// Skip protects such nodes from scale-down. Consider allows the autoscaler to remove them
	// like any other node. This affects the "--skip-nodes-with-local-storage" flag on
	// cluster-autoscaler.
	//
	// When omitted, this defaults to Consider, preserving HyperShift's current behavior.
	//
	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	LocalStorageScaleDown LocalStorageScaleDownPolicy `json:"localStorageScaleDown,omitempty"`

	// similarNodeGroupBalancing controls whether the autoscaler tries to keep similar node
	// groups, meaning NodePools with the same instance type and labels, at similar sizes when
	// scaling up. This affects the "--balance-similar-node-groups" flag on cluster-autoscaler.
	//
	// When omitted, this defaults to Balance, preserving HyperShift's current behavior.
	//
	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	SimilarNodeGroupBalancing SimilarNodeGroupBalancingPolicy `json:"similarNodeGroupBalancing,omitempty"`

	// daemonSetUtilization controls whether DaemonSet pod resource requests are included when
	// the autoscaler calculates node utilization for scale-down eligibility. Include counts
	// DaemonSet pod requests toward utilization. Exclude ignores them. This affects the
	// "--ignore-daemonsets-utilization" flag on cluster-autoscaler.
	//
	// When omitted, no flag is rendered and the autoscaler's own default behavior of including
	// DaemonSet pod requests applies.
	//
	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	DaemonSetUtilization DaemonSetUtilizationPolicy `json:"daemonSetUtilization,omitempty"`

	// resourceLimits bounds the aggregate CPU, memory, and GPU resources across all
	// guest-cluster nodes that the autoscaler may grow the hosted cluster to. The
	// autoscaler counts resources from all guest-cluster nodes read through its guest
	// kubeconfig and blocks scale-up of any NodePool once a configured maximum would be
	// exceeded. maxNodesTotal and each configured resourceLimits maximum are independent,
	// cluster-wide ceilings that apply in addition to each NodePool's own
	// autoScaling.max; the core or memory budget is shared across NodePools, not a
	// separate allowance for each one.
	//
	// When omitted, no additional resource limits are applied beyond maxNodesTotal and
	// NodePool bounds.
	//
	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	ResourceLimits ResourceLimits `json:"resourceLimits,omitzero"`

	// scaleUp configures the behavior of the Cluster Autoscaler scale up operation.
	//
	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	ScaleUp ScaleUpConfig `json:"scaleUp,omitzero"`

	// logVerbosity sets the hosted cluster autoscaler's klog verbosity level. This is a top-level field,
	// independent of scaling, scaleDown, and scaleUp: it changes only the autoscaler container's own log
	// output, not scaling policy, the control plane operator's logs, or guest control-plane logs. This
	// affects the "--v" flag on cluster-autoscaler.
	//
	// When omitted, this defaults to 4, preserving HyperShift's current behavior. An explicit 0 is
	// distinct from omission and is honored as a valid, lower-verbosity choice. Higher values increase
	// log volume and can add processing overhead.
	//
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	LogVerbosity *int32 `json:"logVerbosity,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

The autoscaler and CPO run in the management cluster's hosted-control-plane
namespace. The autoscaler uses its guest kubeconfig for pods, nodes, utilization,
local volumes, and node cordoning. Its Cluster API client discovers and scales
management-cluster CAPI node groups backing NodePools. Node-group balancing
depends on the autoscaler's comparison of CAPI node groups and guest-node
labels; the existing platform-specific `--balancing-ignore-label` arguments
remain in place. Balancing acts during scale-up and does not guarantee exactly
equal NodePool sizes. These settings add no new network path, credential, RBAC
grant, sidecar, or per-node resource overhead.

The node and pod policies and the new-pod delay are evaluated against
guest-cluster state before a CAPI scale action. In normal scale-down, the
[autoscaler actuator](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/core/scaledown/actuation/actuator.go)
applies its deletion `NoSchedule` taint to the guest Node and, with cordoning
enabled, sets `spec.unschedulable` before starting asynchronous deletion. For
a node with movable workload Pods, the
[deletion scheduler](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/core/scaledown/actuation/group_deletion_scheduler.go)
waits for required evictions and the drain delay before putting the node in
the deletion batch; a failed drain aborts that deletion. Only then does the
[CAPI provider](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/clusterapi/clusterapi_nodegroup.go)
mark the management-cluster Machine for deletion and reduce replicas. The
autoscaler's DaemonSet eviction can be best effort, so completion of its drain
does not promise every DaemonSet Pod has disappeared. Empty nodes have no
workload drain to await. Disabling cordon leaves `spec.unschedulable` unchanged
but still applies the deletion taint and follows the same workload-drain
ordering.

Updating `logVerbosity` changes the Deployment pod template and rolls the
autoscaler; see the
[autoscaler FAQ](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#how-can-i-increase-the-information-that-the-ca-is-logging)
for the log-volume and performance cost of high verbosity.

GPU limits need particular validation in the CAPI topology. The
[OpenShift Cluster API provider](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/clusterapi/clusterapi_provider.go)
uses `cluster-api/accelerator` as its GPU label but currently returns `nil`
from `GetAvailableGPUTypes()`, which only feeds GPU metric labels; the
[resource limiter](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/context/autoscaling_context.go)
receives `--gpu-total` independently, and the
[GPU resource processor](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/processors/customresources/gpu_processor.go)
accounts for labeled nodes using allocatable or template capacity. HyperShift
does not infer the accelerator type from the instance type: the administrator
sets `NodePool.spec.nodeLabels["cluster-api/accelerator"]` to the matching
`resourceLimits.gpus[].type`, for example:

```yaml
spec:
  nodeLabels:
    cluster-api/accelerator: nvidia-tesla-t4
```

The [NodePool controller](https://github.com/openshift/hypershift/blob/f6237324195976af37bf133a7033b92a545c8c77/hypershift-operator/controllers/nodepool/capi.go#L560-L600)
copies `nodeLabels` to management-cluster CAPI Machines with a managed prefix,
and the [hosted-cluster Node controller](https://github.com/openshift/hypershift/blob/f6237324195976af37bf133a7033b92a545c8c77/control-plane-operator/hostedclusterconfigoperator/controllers/node/node.go#L64-L98)
strips that prefix onto guest Nodes once after creation. In Replace mode,
Machine labels are patched after creation, so GPU delivery must verify the
label lands before that one-time sync; existing guest GPU Nodes must already
carry it or be relabeled, since changing `NodePool.spec.nodeLabels` does not
backfill already-synced Nodes. On real guest Nodes, the GPU device plugin
supplies the extended-resource capacity (for example `nvidia.com/gpu`);
admission can validate the GPU type's syntax but not its match with a live
NodePool or Node.

For scale-from-zero, the current [AWS/Azure NodePool path](https://github.com/openshift/hypershift/blob/f6237324195976af37bf133a7033b92a545c8c77/hypershift-operator/controllers/nodepool/scale_from_zero.go#L66-L159)
puts `nodeLabels` in the scalable resource's
`capacity.cluster-autoscaler.kubernetes.io/labels` annotation only when the
infrastructure template has no `Status.Capacity`; when native capacity is
present, it drops those annotations and the Replace template carries no
unprefixed accelerator label. GPU delivery therefore needs a NodePool
controller change to publish the accelerator label in the scale-from-zero
template regardless of capacity source, and to confirm GPU extended-resource
capacity reaches the autoscaler through native capacity or annotations (the
[CAPI provider guide](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/clusterapi/README.md)
describes `--gpu-total`, though the generic
[autoscaler FAQ](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#what-are-the-parameters-to-ca)
notes it's documented only for GKE). An empty GPU-type list alone does not
prove enforcement is broken, the required end-to-end test establishes
effective behavior on the shipped image for both real-node and
scale-from-zero accounting, and any gap must be fixed in the NodePool
template, provider, or image before GPU support ships.

#### Standalone Clusters

Standalone OCP continues to use its `ClusterAutoscaler` CR and operator. This
proposal changes neither. The standalone API is the field and behavior
reference, while HyperShift retains its own API shape and direct Deployment.

#### Single-node Deployments or MicroShift

A hosted cluster with one autoscaled worker NodePool still uses these fields,
but NodePool bounds can leave no room for a scale decision; no replica or
resource adjustment is needed for the new configuration. MicroShift does not
consume HostedCluster or this CPO Deployment, so no MicroShift API or binary
changes are required.

#### OpenShift Kubernetes Engine

The fields configure the HyperShift-managed autoscaler, not an optional guest
OpenShift operator. They apply to any supported hosted-cluster and NodePool
autoscaling combination, including OKE where that combination is offered.
This proposal adds no OKE-specific component or setting.

### Implementation Details/Notes/Constraints 

The ticket split is: [CNTRLPLANE-4427](https://redhat.atlassian.net/browse/CNTRLPLANE-4427)
for this enhancement and its cross-cutting design, API, upgrade, and test
requirements; [CNTRLPLANE-4428](https://redhat.atlassian.net/browse/CNTRLPLANE-4428)
for node and pod behavior; [CNTRLPLANE-4429](https://redhat.atlassian.net/browse/CNTRLPLANE-4429)
for resource limits; and [CNTRLPLANE-4430](https://redhat.atlassian.net/browse/CNTRLPLANE-4430)
for cordoning and new-pod delay.

* Add the fields and enum types shown in API Extensions above to
  `api/hypershift/v1beta1/hostedcluster_types.go`. Because `ClusterAutoscaling`
  is shared, generate both HostedCluster and HostedControlPlane schemas with
  the fields present only in the Tech Preview variant. Generate the schemas,
  deepcopy code, feature-gated manifests, clients, and API reference with
  `make update`; run the API linter.
* Register `HostedClusterAutoscalerConfiguration` in the HyperShift operator
  and CPO feature-gate registries: enabled in the `Hypershift` and
  `SelfManagedHA` `TechPreviewNoUpgrade` manifests, disabled in their
  `Default` manifests, driven by the management-cluster
  `HYPERSHIFT_FEATURESET`, never the guest cluster's own feature-gate
  setting.
* Remove `--skip-nodes-with-local-storage=false`,
  `--balance-similar-node-groups=true`, and `--v=4` from
  `v2/assets/cluster-autoscaler/deployment.yaml`, and generate exactly one of
  each in `autoscalerArgs()` instead, using those same values when fields are
  omitted (rendering `logVerbosity` directly, including `--v=0`, when
  present). Keep the unrelated `--alsologtostderr` flag, and emit the other
  new arguments only when their fields are present.
* Preserve the existing HostedCluster-to-HostedControlPlane copy of
  `spec.autoscaling`; CPO continues to derive Deployment arguments from
  HostedControlPlane configuration and must never depend on the standalone
  `ClusterAutoscaler` CR. Guard translation of every new field with the CPO's
  management-cluster gate check, emitting historical arguments when the gate
  is off even if an old gated schema left new fields in the HCP spec. Gate
  the GPU-specific NodePool label/template changes as part of this feature.
* Add an `AutoscalerConfigurationApplied` condition to HostedControlPlane and
  HostedCluster (semantics summarized in
  [Operational Aspects](#operational-aspects-of-api-extensions)). New CPO
  sets the HCP condition with
  `observedGeneration: HostedControlPlane.metadata.generation`, deriving
  expected values from the field-to-flag mapping and reading the live
  container arguments, never treating previously rendered arguments, or a
  merely `Available` Deployment, as proof of application. Recompute on every
  HCP reconciliation, including Deployment events and periodic requeues, even
  without a generation change, so drift after a previous `True` is caught.
  With the management gate off, remove the condition if no new field
  remains, or report `False` with `FeatureGateDisabled` for a value retained
  by an old schema; never report `True` for a gated value that was ignored.
* When any new autoscaling field is requested with the gate enabled, the
  HostedCluster controller copies the HCP condition into HostedCluster
  status only if the HCP's autoscaling spec matches the HC request and the
  HCP condition's `observedGeneration` equals the HCP generation (resulting
  states are in the [Operational Aspects](#operational-aspects-of-api-extensions)
  table); otherwise set `Unknown` with an unconfirmed-settings message, and
  set `observedGeneration` to the HC generation either way. The existing
  HCP-to-HC condition-copy loop needs this freshness check added, it
  currently overwrites the copied `observedGeneration` unconditionally.
  Remove the HC condition once all new fields are removed; a request of only
  `logVerbosity` must still create, update, and later remove it.
* HyperShift defines its own `ClusterAutoscaling` and `ScaleDownConfig`
  types. The older vendored `cluster-autoscaler-operator` types are only
  imported by standalone test setup, not the production HCP translator; sync
  that dependency to the target release for current test-setup types, but
  updating it alone implements nothing on the HostedCluster API.
* The GPU acceptance test is a delivery dependency of
  [CNTRLPLANE-4429](https://redhat.atlassian.net/browse/CNTRLPLANE-4429); see
  [Topology Considerations](#hypershift--hosted-control-planes) for the
  label-sync and scale-from-zero mechanics it must verify. Extend the
  NodePool scale-from-zero path to retain the accelerator label and GPU
  extended-resource capacity on AWS/Azure templates regardless of capacity
  source, and fix the provider or template if the acceptance test shows a
  gap. Deliver and test this before documenting the GPU field as a
  functioning limit.
* Document every field's semantics as specified above, the cordoning/taint
  and GPU label mechanics from Topology Considerations, and the
  `AutoscalerConfigurationApplied` condition behavior. Regenerate
  `docs/content/reference/api.md` and update
  `hypershift/docs/content/how-to/autoscaling.md`, explaining how to select
  management-cluster `TechPreviewNoUpgrade` at install time and its support
  limits, that the hosted cluster's own `spec.configuration.featureGate`
  does not enable these fields, and that a Default CRD may prune unknown
  keys on read-back.

### Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Moving hardcoded flags changes existing clusters | Emit `false` and `true` respectively for omitted values; assert exact effective arguments before and after upgrade. |
| Invalid enum, range, GPU type, or duration stops autoscaler startup | With the gate enabled, reject invalid values at CRD admission; test rendered arguments against a supported autoscaler image. |
| GPU flag is accepted but label or template capacity is missing | Document the NodePool label, propagate it to zero-replica templates, and require a real limit test on the target image before claiming GPU support. |
| A tight limit prevents pending workloads from scaling | Validate ranges, document cluster-wide accounting and NodePool bounds, and surface autoscaler events and logs in support guidance. |
| Scale-down settings increase eviction risk | Keep current defaults; document local-volume exceptions, PodDisruptionBudgets, and `safe-to-evict` annotations. |
| High `logVerbosity` increases log volume and autoscaler overhead | Bound the API to the documented `0` through `10` range; describe the operational cost and how to restore `--v=4` by removing the field. |
| New API fields reach an older CPO during version skew | Sequence payload support before announcing availability. On initial rollout the old CPO leaves `AutoscalerConfigurationApplied` absent; the HostedCluster reports `Unknown` until a compatible CPO confirms the rolled-out arguments. |
| A gated setting is submitted while the management gate is off | Omit it from Default HC/HCP CRDs, test that unknown fields cannot persist or affect arguments, and document read-back because Kubernetes may prune unknown keys without rejecting the request. A current CPO ignores retained gated values and reports `FeatureGateDisabled` instead of `True`. |
| An older HyperShift operator or CRD schema drops configured fields during emergency rollback | Document that rollback can silently revert to historical arguments; require checking live Deployment arguments and the `AutoscalerConfigurationApplied` condition rather than assuming spec reflects reality, and test this against old typed-controller writes and old CRD schemas. |
| `localStorageScaleDown`/`daemonSetUtilization` appear inactive under `scaling: ScaleUpOnly`, since scale-down itself is disabled | Document that both fields remain stored and silently take effect immediately on switching to `ScaleUpAndScaleDown`; administrators should review stored values before that switch. |

### Drawbacks

- HyperShift must maintain another set of public policy enums and a
  translation table alongside the standalone API.
- Because the settings are per HostedCluster, CPO restarts that cluster's
  autoscaler when they change, and GPU limits need GPU-capable CI and might
  require an autoscaler image change if the acceptance test reveals a gap.

## Alternatives (Not Implemented)

* **Copy the standalone `ClusterAutoscaler` CR or install its operator in the
  guest cluster.** HyperShift already owns a different API and deploys the
  autoscaler directly with a management/guest dual-client setup. A second
  authority for autoscaler settings would create conflicting reconciliation.
* **Reuse `operatorConfiguration.*.logLevel` for autoscaler logging.** That
  API configures other hosted control-plane components with four intent-based
  levels. The autoscaler belongs to `spec.autoscaling`; its standalone API
  already has numeric `logVerbosity`, and direct integers retain the full
  documented `--v` range.
* **Expose an arbitrary list of autoscaler flags.** That would bypass schema
  validation, create duplicate or conflicting arguments, and tie users to the
  exact autoscaler image's CLI surface.
* **Change the local-storage default to match standalone OCP.** That would
  change scale-down behavior for existing hosted clusters. Administrators can
  opt in with `localStorageScaleDown: Skip`.
* **Publish GPU limits based only on flag rendering.** The current CAPI provider
  does not enumerate GPU types for metrics, and a rendered flag alone does not
  demonstrate that labeled real and template nodes are counted correctly.

## Open Questions 

The API names and enum values above are the proposed review surface. They may
be refined before merge, but the action semantics, omission behavior, and
validation rules are design requirements. The NodePool scale-from-zero label
change above is required for GPU delivery; the GPU end-to-end test determines
whether a further provider or image change is necessary before the GPU field
is released.

## Test Plan
### TODO: Discuss which test paths are necessary

* **Feature gate:** Verify `HostedClusterAutoscalerConfiguration` is disabled
  in both `Hypershift` and `SelfManagedHA` Default feature sets and enabled
  in `TechPreviewNoUpgrade`; the Default CRDs omit all seven settings while
  Tech Preview CRDs contain them with validation. With the gate off, verify
  CPO keeps historical arguments and reports `FeatureGateDisabled` (never
  `True`) for a value retained from an older gated schema. Run the
  behavioral e2e cases below in a `TechPreviewNoUpgrade` job, and keep a
  Default job checking unchanged arguments and no application condition.
* **API and unit:** Cover serialization of omitted fields and explicit
  zero-like values (`logVerbosity: 0`, `min: 0`, GPU `max: 0`) distinct from
  omission, plus N-1/N+1 deserialization. Extend `TestAdaptDeployment` with
  omitted settings, both values of every policy, mixed settings, multiple GPU
  types, and verify each argument (including `--v`) appears exactly once and
  the two other historical defaults survive translation. Test CPO/HC
  condition transitions for matching, missing/duplicate, pending-rollout, and
  stale-observation cases, including a request of only `logVerbosity: 0`.
* **Envtest:** On Tech Preview CRDs, accept valid enums, `logVerbosity: 0`
  and `10`, ranges including GPU `{min: 0, max: 0}`, 1-64 unique GPU types,
  and durations from `0s` to `1h`. Reject unknown enums, out-of-range
  `logVerbosity`, empty `resourceLimits`/`scaleUp`, missing range endpoints,
  negative/inverted ranges, CPU/memory `max: 0`, duplicate or reserved
  (`cpu`/`memory`) GPU types, malformed or >1h durations, 0 or 65-entry GPU
  lists, and `scaleDown` with `ScaleUpOnly`. Repeat with Default CRDs to
  verify new fields cannot be stored, whether rejected or pruned on
  read-back.
* **Log verbosity end to end:** Verify the omission baseline is one `--v=4`
  argument. Set `logVerbosity: 0` and `6` in turn, waiting for
  `AutoscalerConfigurationApplied=True` each time, and verify exactly one
  matching `--v` argument and that log volume (checked via a known V(4)
  message) tracks the level. Remove the field and verify `--v=4` and the
  condition's removal, assuming no other new field remains configured.
* **Local storage and delay end to end:** With at least two autoscaled
  NodePools, verify `localStorageScaleDown: Skip` prevents deletion of a node
  with a qualifying local volume and `Consider` restores eligibility. With
  `newPodScaleUpDelay: 30s` and a Pod that fits a NodePool template but not
  existing Nodes, verify CAPI growth is withheld until the Pod is 30 seconds
  old, that a Pod held behind a scheduling gate past that age does not reset
  the wait on release, and that a shorter per-pod
  `cluster-autoscaler.kubernetes.io/pod-scale-up-delay` annotation (e.g.
  `10s`) is ignored in favor of the longer cluster-wide delay while a longer
  one (`60s`) is honored.
* **DaemonSet utilization end to end:** Choose DaemonSet requests that put an
  otherwise removable node above the scale-down threshold with `Include` and
  below it with `Exclude`; verify the same node is ineligible with `Include`
  and becomes eligible with `Exclude`.
* **Similar NodePool balancing end to end:** With two matching NodePools
  started at unequal sizes with scale-up headroom, create pending pods that
  fit either pool and verify `Balance` directs growth toward the smaller
  pool while `DoNotBalance` still scales up without converging sizes.
* **Cordoning end to end:** With a schedulable guest Node running a movable
  workload Pod whose termination briefly holds the drain open, verify
  `Enabled` sets `Node.spec.unschedulable` before eviction and before the
  CAPI Machine is marked for deletion, while `Disabled` leaves it false;
  under both, verify the deletion taint appears and required workload Pods
  leave first, without requiring best-effort DaemonSet Pods to disappear.
* **CPU and memory end to end:** Test `cores.max`/`memory.max` independently
  (a pending workload triggers growth below the maximum but not beyond it)
  and `cores.min`/`memory.min` independently with scale-down enabled and
  otherwise-removable nodes: verify scale-down stops at the floor, retains
  the next node whose removal would cross it, can remove it once the minimum
  is lowered, and does not trigger scale-up from raising a minimum alone.
  Removing fields restores baseline arguments.
* **GPU end to end:** Create a GPU-capable CAPI NodePool with
  `spec.nodeLabels["cluster-api/accelerator"]` set before any Nodes join; verify
  the label and GPU extended-resource capacity reach guest Nodes and the
  scale-from-zero template, covering both native `Status.Capacity` and
  fallback-annotation paths. Verify scale-up below the GPU maximum, rejection
  at it, and the same min/floor behavior as CPU/memory for `gpus[].min`.
  Configure `{type: nvidia-tesla-t4, min: 0, max: 0}` with a pending GPU
  workload and verify no growth; raising `max` above zero must then allow it.
  This run is required for GPU delivery, the rendered argument alone is
  insufficient proof.
* **Upgrade:** With the Default feature set, upgrade an existing hosted
  cluster with no new fields and compare effective arguments before and
  after. In a Tech Preview compatibility test, install the new operator and
  gated CRDs while the old CPO still runs, configure
  `localStorageScaleDown: Skip`, `logVerbosity: 0`, and
  `resourceLimits.cores: {min: 0, max: 64}`, and verify the fields are stored
  but the old CPO keeps `--v=4` with its HCP condition absent and the HC
  condition `Unknown` despite an `Available` Deployment. Upgrade the payload
  without editing HC again; verify the new CPO renders the three flags,
  reports `False` during rollout and `True` only once replicas are updated,
  and that clearing the fields restores `--v=4` and removes the condition.
  Verify drifting the live Deployment after `True` flips the condition back
  to `False` until CPO repairs it.
* **Operator rollback with configured fields:** Covering the rollback
  mechanics in [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy),
  roll back only the HyperShift operator binary in the gated compatibility
  test with new HC/HCP CRDs installed, then repeat with older CRD schemas
  reapplied, starting only after HC `AutoscalerConfigurationApplied=True`.
  Assert the old operator does not crash, that full-object writes through its
  old types and writes under an old CRD schema both drop the new fields, and
  that a retained `True` condition is not treated as proof after rollback.
  Verify the release-selected new CPO renders historical arguments (including
  `--v=4`) once fields are gone, and separately that an old CPO ignores
  retained fields on an HCP without crashing. Verify `scaling: ScaleUpOnly`
  still yields `--scale-down-enabled=false` as an emergency fallback. After
  restoring a compatible operator and CRDs and re-applying the dropped
  fields, verify flags and the `True` condition return. Also test a planned
  rollback where fields are removed first.

## Graduation Criteria

### Dev Preview -> Tech Preview

HyperShift's management-cluster feature-gate system has no Dev Preview
equivalent: per
[Feature Gates](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/feature-gates.md),
all management-cluster gates are grouped under the single
`TechPreviewNoUpgrade` feature set, with no separate experimental bucket
before it. This feature therefore has no Dev Preview phase by construction,
not by choice.

That same feature set carries a caveat stronger than the name "Tech Preview"
implies: HyperShift documents "no guarantees that your fleet continues to be
operational long-term" once a management-cluster gate is enabled. Unlike the
hosted-cluster in-cluster `TechPreviewNoUpgrade` feature set, which blocks
upgrades for one hosted cluster but leaves it otherwise supported, this gate
affects the HyperShift Operator and CPO installation itself with no fleet
continuity guarantee. Administrators should treat enabling it with the same
caution as a Dev Preview feature at the fleet level, even though the fields
themselves are expected to be stable and are covered by the full test plan
below.

Entry into Tech Preview requires API approval, generated gated HC and HCP
schemas with admission tests, a CPO/autoscaler payload that recognizes every
emitted flag, the `AutoscalerConfigurationApplied` status checks, and
documentation that explains how to enable the gate, its support limits, and
the fleet-continuity caveat above. The GPU field also requires the CAPI
label/template changes and a passing end-to-end enforcement test before the
gate ships, this gate does not expose a GPU field whose behavior is
unverified. A periodic gate-enabled CI job runs the behavioral suite, while a
Default job checks omission and historical arguments.

### Tech Preview -> GA

Promote `HostedClusterAutoscalerConfiguration` to `Default` only after the
gated API/behavior tests run reliably across supported platforms, default
and gated CRD/API compatibility checks pass, and the full multi-NodePool and
GPU end-to-end tests prove both upper and lower resource limits across every
supported CPO/autoscaler image combination. Complete the upgrade test for
omitted and configured settings (including old-CPO and rollout condition
states) and the operator-rollback tests with and without CRD rollback. Per
HyperShift's
[feature-gate promotion criteria](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/feature-gates.md#promoting-a-feature-gated-api-field),
analyze the impact on each downstream product, ROSA, ARO, IBM Cloud, and
MCE, and cover on-creation and on-update admission-failure UX with e2e
tests, not only envtest. Publish documentation, validate day-2 changes and
failure handling, then enable the gate in `Default` manifests and runtime
registries, regenerate CRDs, and verify the formerly gated fields are
present by default without changing omission behavior. Until promotion,
there is no default availability or partial GPU parity claim.

### Removing a deprecated feature

Not applicable. This is a new feature with no deprecated predecessor.

## Upgrade / Downgrade Strategy

On the initial Tech Preview release, the new CRD fields are installed only
when the management-cluster HyperShift feature set is
`TechPreviewNoUpgrade`; Default schemas continue to omit them. Existing
HostedClusters omit the new fields, and CPO still produces the historical
`--skip-nodes-with-local-storage=false`, `--balance-similar-node-groups=true`,
and `--v=4`. There is no data migration or change to NodePool bounds. The
gated mixed-version rollout exercised in the test plan is a controlled
compatibility test, not a promised `TechPreviewNoUpgrade` upgrade path. At GA
promotion, the new fields enter the Default schemas and the same test covers
a normal supported upgrade.

For a planned HyperShift operator rollback to a version without this API,
remove the new fields from HostedCluster and wait for baseline arguments to
reconcile before changing the operator binary. An emergency rollback can
happen while fields are still configured: the older operator's typed API and
HostedCluster-to-HostedControlPlane copy cannot carry the new fields, so a
full-object write through the old types removes them, and an older installed
CRD schema can independently prune them on write under
[Kubernetes CRD field-pruning rules](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#field-pruning).
Neither path guarantees the values survive in spec or in the effective
arguments, see [Version Skew Strategy](#version-skew-strategy) for how CPO
and the `AutoscalerConfigurationApplied` condition behave once that happens.
If `localStorageScaleDown: Skip` cannot be confirmed and scale-down must
stop immediately, set the already-supported `scaling: ScaleUpOnly` policy
(removing any `scaleDown` block, as validation requires) and verify
`--scale-down-enabled=false`. After restoring a compatible operator and
CRDs, re-apply the previously configured fields from your own saved record
(manifest or GitOps source), HyperShift does not persist a separate
recovery copy. Control-plane release downgrades remain unsupported by
HyperShift's existing
[upgrade policy](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/upgrades.md).

Reinstalling with the Default feature set while configured fields still
exist is also a schema change that can prune them on write; restore a
compatible gated or GA installation and re-apply the values explicitly
rather than assuming they survived. This feature-set transition is a
compatibility test, not a supported Tech Preview upgrade or downgrade
procedure.

## Version Skew Strategy

The HyperShift operator, CPO, autoscaler image, and guest cluster can move at
different times. The HostedCluster controller's spec copy needs no new
translator, but an older CPO cannot act on new autoscaling fields. The
feature is available only when the management-cluster
`HostedClusterAutoscalerConfiguration` gate is enabled and the
hosted-control-plane payload stream contains both the CPO translator and an
autoscaler image with the required flags; GPU additionally needs a passing
enforcement test. Support is determined per advertised release stream and
verified by CI, not inferred from a newer operator or guest-cluster feature
set alone.

During mixed-version rollout, old CPO behavior remains in force until the
new CPO reconciles; the three historical arguments stay effective either
way, and an older CPO keeps `--v=4` even with `logVerbosity` stored in HC and
HCP (skew for this field is entirely in CPO translation, since `--v` already
exists on the autoscaler image). An old CPO never sets the HCP condition, so
the HC reports `Unknown` rather than implying a copied spec field took
effect. `False` means the new CPO has seen the request but not confirmed a
completed rollout; `True` means the HCP spec matches the HC request and the
arguments are present in the rolled-out Deployment, but it is observational,
not enforcement: while it is not `True`, an old autoscaler still scales down
local-storage nodes per the historical `false` default, so administrators
should wait for `True` before relying on a setting like
`localStorageScaleDown: Skip`. After a rollback to older operator or CPO
binaries, a previous `True` can persist without being recomputed; verify the
live Deployment arguments directly rather than trusting it. Once compatible
code returns, a new `True` confirms only the currently stored HC spec, if an
older schema pruned fields along the way, compare that spec against your own
record of the intended configuration before treating it as complete.

If a current CPO sees retained new HCP fields while the management gate is
off, it renders historical arguments and reports `False` with
`FeatureGateDisabled`; a Default CRD can instead prune the fields outright,
leaving no request to report. Check the stored HC/HCP specs and feature set
before relying on status. No new kubelet, Machine, or guest operator API is
required.

## Operational Aspects of API Extensions

This extends the existing HostedCluster and HostedControlPlane CRDs, adding
no new CRD, admission/conversion webhook, finalizer, API server, or network
dependency, and negligible API overhead. With the gate enabled, invalid
configurations fail admission with field-specific messages before they can
roll the autoscaler; with it disabled, the Default CRD variants omit the
fields and Kubernetes may prune unknown keys instead of rejecting a request.

When any new autoscaling field is requested, the
`AutoscalerConfigurationApplied` entry in HCP and HC `status.conditions`
provides a programmatic application signal:

| Observation | HCP condition | HC condition |
| --- | --- | --- |
| Old CPO does not report the condition, or the HCP observation is stale | Absent or stale | `Unknown` |
| Gate disabled while a new request remains in an older gated schema | `False` with `FeatureGateDisabled` from a current CPO | `False` once the HCP observation is current |
| Requested flags are missing, duplicated, or different; rollout is pending; or the autoscaler is unavailable | `False` | `False` once the HCP observation is current |
| Live flags match the request and the updated Deployment rollout is complete | `True` | `True` once the HCP spec matches the HC request |

CPO rechecks the live Deployment after changes, so a later drift changes a
previous `True` to `False` until repaired. Current HC `True` confirms that the
requested arguments were applied; a valid resource maximum can still block
growth as configured. After rollback to an operator or CPO that predates
this condition, a retained `True` can be stale, see
[Version Skew Strategy](#version-skew-strategy).

If an unsupported flag reaches an autoscaler image, its container can fail to
start, revealed by `AutoscalerConfigurationApplied=False`, the Deployment's
`Available` condition, and CPO component availability reporting, leaving
pending pods unscheduled while existing nodes and workloads remain. A valid
but overly tight limit instead leaves the Deployment healthy while scale-up
is blocked; autoscaler logs, guest pod events, and the
`kube-system/cluster-autoscaler-status` ConfigMap distinguish the two. The
HyperShift team owns API/argument translation and CPO availability; the
autoscaler/CAPI provider maintainers own resource accounting failures.

## Support Procedures

The user-facing [HyperShift autoscaling guide](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/autoscaling.md)
will include steps and examples of support procedures, starting with 
the HostedCluster condition and inspecting effective Deployment 
arguments whenever it is not `True`, including after an operator 
rollback.

## Infrastructure Needed 

The existing HyperShift envtest harness and two-NodePool autoscaling test can
host the API, translation, CPU/memory, and node-policy cases. GPU graduation
additionally needs a CI lane with a GPU-capable CAPI NodePool and the target
payload image.
