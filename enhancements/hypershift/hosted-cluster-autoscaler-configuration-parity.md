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
last-updated: 2026-10-28
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

### Goals

* Make the seven settings in
  [CNTRLPLANE-4426](https://redhat.atlassian.net/browse/CNTRLPLANE-4426)
  configurable per hosted cluster.
* Preserve effective behavior for every omitted setting, including on existing clusters.
* Define admission validation, upgrade behavior, and documentation for every
  new setting.
* Deliver GPU limits only once enforcement in the Cluster API autoscaler topology is
  verified.

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
      cores: # whole CPU cores
        min: 0
        max: 128
      memory: # whole GiB
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
// ScaleDownConfig gains one new field. delayAfterDeleteSeconds's doc comment also gains a
// cross-reference to the new resourceLimits validation rule below.
type ScaleDownConfig struct {
	// ... other existing fields are unchanged ...

	// delayAfterDeleteSeconds ... (existing field, doc comment only)
	//
	// Required to be explicitly set to at least 60 seconds whenever resourceLimits configures
	// a minimum > 0 for cores, memory, or any gpu type: with too short a delay, the autoscaler
	// can delete a second node before Cluster API accounts for a prior deletion, dropping
	// capacity below the floor. 60 seconds is the value confirmed sufficient by live testing,
	// not a value derived from a documented upper bound on Cluster API convergence time; see
	// the matching entry in Risks and Mitigations and in Future Work for why this remains a
	// heuristic rather than a provable guarantee.
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=86400
	// +optional
	DelayAfterDeleteSeconds *int32 `json:"delayAfterDeleteSeconds,omitempty"`

	// cordonNodeBeforeTerminating controls whether the autoscaler cordons a guest-cluster node
	// before draining and terminating it during scale-down. When omitted, HyperShift explicitly
	// renders the flag as false (not cordoning), matching both upstream Cluster Autoscaler's and
	// standalone OCP's documented default, rather than deferring to the deployed autoscaler
	// image's own unflagged behavior (confirmed by live testing to diverge from that documented
	// default).
	//
	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	CordonNodeBeforeTerminating CordonNodeBeforeTerminatingPolicy `json:"cordonNodeBeforeTerminating,omitempty"`
}

// +kubebuilder:validation:Enum="";Skip;Consider
type LocalStorageScaleDownPolicy string

const (
	LocalStorageScaleDownSkip     LocalStorageScaleDownPolicy = "Skip"
	LocalStorageScaleDownConsider LocalStorageScaleDownPolicy = "Consider"
)

// +kubebuilder:validation:Enum="";Balance;DoNotBalance
type SimilarNodeGroupBalancingPolicy string

const (
	SimilarNodeGroupBalancingBalance      SimilarNodeGroupBalancingPolicy = "Balance"
	SimilarNodeGroupBalancingDoNotBalance SimilarNodeGroupBalancingPolicy = "DoNotBalance"
)

// +kubebuilder:validation:Enum="";Include;Exclude
type DaemonSetUtilizationPolicy string

const (
	DaemonSetUtilizationInclude DaemonSetUtilizationPolicy = "Include"
	DaemonSetUtilizationExclude DaemonSetUtilizationPolicy = "Exclude"
)

// +kubebuilder:validation:Enum="";Enabled;Disabled
type CordonNodeBeforeTerminatingPolicy string

const (
	CordonNodeBeforeTerminatingEnabled  CordonNodeBeforeTerminatingPolicy = "Enabled"
	CordonNodeBeforeTerminatingDisabled CordonNodeBeforeTerminatingPolicy = "Disabled"
)

// ScaleUpConfig configures the behavior of the Cluster Autoscaler scale up operation.
type ScaleUpConfig struct {
	// newPodScaleUpDelay delays scale-up evaluation for pods younger than this duration,
	// measured from the pod's creation timestamp on the guest cluster. A shorter per-pod
	// "cluster-autoscaler.kubernetes.io/pod-scale-up-delay" annotation cannot override this
	// value; the autoscaler keeps the longer of the two delays.
	//
	// +required
	// +kubebuilder:validation:Pattern=^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:XValidation:rule="duration(self) <= duration('1h')",message="newPodScaleUpDelay must not exceed 1h"
	NewPodScaleUpDelay string `json:"newPodScaleUpDelay,omitempty"`
}

// ResourceLimits specifies cluster-wide ceilings on aggregate guest-cluster node resources,
// across all NodePools.
// +kubebuilder:validation:MinProperties=1
type ResourceLimits struct {
	// cores bounds the total CPU cores, in whole cores, across all guest-cluster nodes.
	// This affects the "--cores-total" flag on cluster-autoscaler.
	//
	// When omitted, the autoscaler's own default applies (effectively unlimited).
	//
	// +optional
	Cores ResourceRange `json:"cores,omitzero"`

	// memory bounds the total memory, in whole GiB, across all guest-cluster nodes. This
	// affects the "--memory-total" flag on cluster-autoscaler, which the upstream autoscaler
	// converts internally to bytes as GiB * 1024^3; HyperShift passes the value through
	// unconverted.
	//
	// When omitted, the autoscaler's own default applies (effectively unlimited).
	//
	// +optional
	Memory ResourceRange `json:"memory,omitzero"`

	// gpus bounds GPU devices of each listed type. Each entry's type must match the
	// "cluster-api/accelerator" label administrators set on the GPU-capable NodePool.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	GPUs []GPULimit `json:"gpus,omitempty"`
}

// ResourceRange specifies a minimum and maximum bound, in whole units of whichever resource
// the embedding field documents (cores or GiB of memory). Both bounds are required so an
// explicit zero minimum stays distinct from an omitted range.
// +kubebuilder:validation:XValidation:rule="self.min <= self.max",message="min must not exceed max"
type ResourceRange struct {
	// +required
	// +kubebuilder:validation:Minimum=0
	Min *int32 `json:"min,omitempty"`

	// +required
	// +kubebuilder:validation:Minimum=1
	Max *int32 `json:"max,omitempty"`
}

// GPULimit specifies a minimum and maximum bound on GPU devices of a given accelerator type.
// +kubebuilder:validation:XValidation:rule="self.min <= self.max",message="min must not exceed max"
type GPULimit struct {
	// type matches the "cluster-api/accelerator" label value on the guest Nodes it applies
	// to. "cpu" and "memory" are reserved: the autoscaler's resource limiter keys its limit
	// maps by those names, so either would overwrite the corresponding resource limit.
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self.matches('^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$')",message="type must be a valid label value"
	// +kubebuilder:validation:XValidation:rule="self != 'cpu' && self != 'memory'",message="type must not be \"cpu\" or \"memory\""
	Type string `json:"type,omitempty"`

	// +required
	// +kubebuilder:validation:Minimum=0
	Min *int32 `json:"min,omitempty"`

	// max: a value of 0 blocks all scale-up of this GPU type.
	//
	// +required
	// +kubebuilder:validation:Minimum=0
	Max *int32 `json:"max,omitempty"`
}

// ClusterAutoscaling gains six new fields and one cross-field validation rule requiring
// scaleDown.delayAfterDeleteSeconds >= 60 whenever resourceLimits configures any minimum > 0.
// 60 is the floor confirmed sufficient by live testing (see Risks and Mitigations and Future
// Work): it is not derived from any documented upper bound on Cluster API convergence time, so
// it narrows but does not eliminate the underlying race.
// +openshift:validation:FeatureGateAwareXValidation:featureGate=HostedClusterAutoscalerConfiguration,rule="!((has(self.resourceLimits) && ((has(self.resourceLimits.cores) && self.resourceLimits.cores.min > 0) || (has(self.resourceLimits.memory) && self.resourceLimits.memory.min > 0) || (has(self.resourceLimits.gpus) && self.resourceLimits.gpus.exists(g, g.min > 0))))) || (has(self.scaleDown) && has(self.scaleDown.delayAfterDeleteSeconds) && self.scaleDown.delayAfterDeleteSeconds >= 60)",message="scaleDown.delayAfterDeleteSeconds must be explicitly set to at least 60 seconds when resourceLimits configures a minimum greater than 0 for cores, memory, or any gpu type"
type ClusterAutoscaling struct {
	// ... existing fields are unchanged ...

	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	LocalStorageScaleDown LocalStorageScaleDownPolicy `json:"localStorageScaleDown,omitempty"`

	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	SimilarNodeGroupBalancing SimilarNodeGroupBalancingPolicy `json:"similarNodeGroupBalancing,omitempty"`

	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	DaemonSetUtilization DaemonSetUtilizationPolicy `json:"daemonSetUtilization,omitempty"`

	// resourceLimits bounds aggregate CPU/memory/GPU across all guest-cluster nodes, in
	// addition to maxNodesTotal and each NodePool's own autoScaling.max.
	//
	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	ResourceLimits ResourceLimits `json:"resourceLimits,omitzero"`

	// +optional
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	ScaleUp ScaleUpConfig `json:"scaleUp,omitzero"`

	// logVerbosity sets the autoscaler's klog verbosity level, independent of scaling,
	// scaleDown, and scaleUp. An explicit 0 is distinct from omission.
	//
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration
	LogVerbosity *int32 `json:"logVerbosity,omitempty"`
}
```

| Field | Flag | When omitted |
| --- | --- | --- |
| `localStorageScaleDown` | `--skip-nodes-with-local-storage` | `Consider` (`false`), current behavior |
| `similarNodeGroupBalancing` | `--balance-similar-node-groups` | `Balance` (`true`), current behavior |
| `daemonSetUtilization` | `--ignore-daemonsets-utilization` | no flag rendered; autoscaler default (include) applies |
| `resourceLimits.cores`/`.memory` | `--cores-total`/`--memory-total` | no flag rendered; effectively unlimited |
| `resourceLimits.gpus[]` | `--gpu-total` (once per entry) | no flag rendered; no GPU limit |
| `scaleDown.cordonNodeBeforeTerminating` | `--cordon-node-before-terminating` | explicit `false` rendered (not cordoning), matching standalone OCP's documented default |
| `scaleUp.newPodScaleUpDelay` | `--new-pod-scale-up-delay` | no flag rendered; autoscaler default (`0s`) applies |
| `logVerbosity` | `--v` | `4`, current behavior |

### Topology Considerations

#### Hypershift / Hosted Control Planes

The autoscaler and CPO run in the management cluster's hosted-control-plane namespace. The
autoscaler uses its guest kubeconfig for pods, nodes, utilization, local volumes, and node
cordoning, and its Cluster API client to discover and scale management-cluster CAPI node
groups backing NodePools. These settings add no new network path, credential, RBAC grant,
sidecar, or per-node resource overhead.

- **Cordon/taint/deletion ordering:** scale-down always applies the autoscaler's deletion
  taint and waits on workload drain before the
  [CAPI provider](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/clusterapi/clusterapi_nodegroup.go)
  marks the Machine for deletion; cordoning only adds `spec.unschedulable` ahead of that
  drain (see the
  [autoscaler actuator](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/core/scaledown/actuation/actuator.go)).
  DaemonSet eviction is best-effort either way, and empty nodes skip the drain wait entirely.
- **Log verbosity** changes the pod template and rolls the autoscaler; see the
  [autoscaler FAQ](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#how-can-i-increase-the-information-that-the-ca-is-logging)
  for the cost of high verbosity.
- **GPU limits** require the GPU-capable NodePool's guest Nodes to carry a
  `cluster-api/accelerator` label matching `resourceLimits.gpus[].type`; HyperShift does not
  infer this from the instance type:

  ```yaml
  spec:
    nodeLabels:
      cluster-api/accelerator: nvidia-tesla-t4
  ```

  Real guest Nodes already receive this label through the existing NodePool and Node
  controllers, but the scale-from-zero template path drops it whenever the infrastructure
  template carries native `Status.Capacity`; see
  [Implementation Details](#implementation-detailsnotesconstraints) for the required fix.

#### Standalone Clusters

Not applicable. Standalone OCP keeps its own `ClusterAutoscaler` CR and operator; this
proposal changes neither.

#### Single-node Deployments or MicroShift

Not applicable. MicroShift does not consume HostedCluster or this CPO Deployment. A
single-NodePool hosted cluster uses these fields normally, just with little room for a
scale-down decision to act on.

#### OpenShift Kubernetes Engine

No OKE-specific component or setting; the fields apply identically to any supported
hosted-cluster/NodePool autoscaling combination, including OKE.

### Implementation Details/Notes/Constraints 

The ticket split is: [CNTRLPLANE-4427](https://redhat.atlassian.net/browse/CNTRLPLANE-4427)
tracks this enhancement proposal itself; [CNTRLPLANE-4428](https://redhat.atlassian.net/browse/CNTRLPLANE-4428)
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
* HyperShift defines its own `ClusterAutoscaling` and `ScaleDownConfig`
  types. The older vendored `cluster-autoscaler-operator` types are only
  imported by standalone test setup, not the production HCP translator; sync
  that dependency to the target release for current test-setup types, but
  updating it alone implements nothing on the HostedCluster API.
* The NodePool scale-from-zero label fix is a delivery dependency of
  [CNTRLPLANE-4429](https://redhat.atlassian.net/browse/CNTRLPLANE-4429) (see
  [Topology Considerations](#hypershift--hosted-control-planes) for the
  label-sync and scale-from-zero mechanics): extend the scale-from-zero path
  to retain the accelerator label and GPU extended-resource capacity on
  AWS/Azure templates regardless of capacity source, fixing the provider or
  template if a gap is found. Deliver this before documenting the GPU field
  as a functioning limit.
* Document every field's semantics as specified above and the cordoning/taint
  and GPU label mechanics from Topology Considerations. Regenerate
  `docs/content/reference/api.md` and update
  `hypershift/docs/content/how-to/autoscaling.md`, explaining how to select
  management-cluster `TechPreviewNoUpgrade` at install time and its support
  limits, that the hosted cluster's own `spec.configuration.featureGate`
  does not enable these fields, and that a Default CRD may prune unknown
  keys on read-back.

### Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Moving hardcoded flags changes existing clusters | Emit `false` and `true` respectively for omitted values, preserving exact effective arguments across upgrade. |
| The deployed autoscaler image's unflagged behavior cordons nodes by default, diverging from upstream Cluster Autoscaler's and standalone OCP's documented "disabled by default" behavior (confirmed by live testing) | Always render an explicit `--cordon-node-before-terminating`; default the omitted case to `false`, matching the documented/standalone-OCP default rather than the image's own unflagged behavior. |
| Invalid enum, range, GPU type, or duration stops autoscaler startup | With the gate enabled, reject invalid values at CRD admission. |
| GPU flag is accepted but label or template capacity is missing | Document the NodePool label, propagate it to zero-replica templates, and withhold GPU support claims until enforcement is verified on the target image. |
| A tight limit prevents pending workloads from scaling | Validate ranges, document cluster-wide accounting and NodePool bounds, and surface autoscaler events and logs in support guidance. |
| Scale-down settings increase eviction risk | Keep current defaults; document local-volume exceptions, PodDisruptionBudgets, and `safe-to-evict` annotations. |
| High `logVerbosity` increases log volume and autoscaler overhead | Bound the API to the documented `0` through `10` range; describe the operational cost and how to restore `--v=4` by removing the field. |
| New API fields reach an older CPO during version skew | Sequence payload support before announcing availability; see [Version Skew Strategy](#version-skew-strategy) for how administrators confirm rollout in the absence of a dedicated signal. |
| A gated setting is submitted while the management gate is off | Omit it from Default HC/HCP CRDs and document read-back, since Kubernetes may prune unknown keys without rejecting the request. A current CPO ignores retained gated values and continues emitting historical arguments. |
| An older HyperShift operator or CRD schema drops configured fields during emergency rollback | Document that rollback can silently revert to historical arguments; require checking live Deployment arguments rather than assuming spec reflects reality. |
| `localStorageScaleDown`/`daemonSetUtilization` appear inactive under `scaling: ScaleUpOnly`, since scale-down itself is disabled | Document that both fields remain stored and silently take effect immediately on switching to `ScaleUpAndScaleDown`; administrators should review stored values before that switch. |
| With a `resourceLimits` minimum configured and `delayAfterDeleteSeconds` left at its `0` default, the autoscaler can delete a second node before Cluster API accounts for a prior deletion, transiently breaching the configured floor | Reject the combination at admission with a feature-gated CEL rule requiring `scaleDown.delayAfterDeleteSeconds` to be explicitly set to at least `60` seconds whenever any `resourceLimits` minimum is greater than `0`, matching the value confirmed sufficient by live testing. |
| The `60`-second floor is an empirically-tested value, not a value derived from a documented upper bound on Cluster API convergence time: a sufficiently slow cloud-provider deletion (API throttling, degraded infrastructure) could still exceed it and transiently breach the configured `resourceLimits` floor. The autoscaler itself resumes scale-down evaluation on a fixed delay without confirming the prior node is actually gone, so the admission-time check narrows but does not close this race | Document the floor as a heuristic, not a guarantee, in both the API field and this proposal. Closing the race fully requires the autoscaler to account for in-flight CAPI deletions when evaluating `resourceLimits`, which is outside HyperShift's CPO and tracked as [Future Work](#future-work), not delivered by this proposal. |

### Drawbacks

- HyperShift must maintain another set of public policy enums and a
  translation table alongside the standalone API.
- Because the settings are per HostedCluster, CPO restarts that cluster's
  autoscaler when they change, and GPU limits might require an autoscaler
  image change if an enforcement gap is found.

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

## Future Work

A dedicated `AutoscalerConfigurationApplied` status condition on HostedCluster and
HostedControlPlane, confirming that a requested setting reached the running autoscaler, was
considered during design and is out of scope for this proposal (see
[Operational Aspects](#operational-aspects-of-api-extensions) and
[Version Skew Strategy](#version-skew-strategy) for how administrators confirm rollout
without it). It remains a candidate for a follow-on enhancement under the
[CNTRLPLANE-4426](https://redhat.atlassian.net/browse/CNTRLPLANE-4426) epic.

Fully closing the `resourceLimits`/`delayAfterDeleteSeconds` race described in
[Risks and Mitigations](#risks-and-mitigations) requires the cluster-autoscaler itself to treat
nodes it has already asked Cluster API to delete as already removed when evaluating
`--cores-total`/`--memory-total`/`--gpu-total`, instead of relying solely on a fixed
post-deletion delay. That accounting lives in the autoscaler's own scale-down/resource-limiter
logic (`openshift/kubernetes-autoscaler`, ideally upstreamed to `kubernetes/autoscaler` since
any Cluster API-based consumer hits the same race), not in HyperShift's CPO, which only
translates API fields into CLI flags on an image it does not otherwise modify. This proposal's
`>= 60` second floor is a stopgap validated by live testing, not a fix for the race; the
autoscaler-side change is out of scope here and remains a candidate follow-on, tracked under the
[CNTRLPLANE-4426](https://redhat.atlassian.net/browse/CNTRLPLANE-4426) epic.

## Open Questions 

The API names and enum values above are the proposed review surface. They may
be refined before merge, but the action semantics, omission behavior, and
validation rules are design requirements.

## Graduation Criteria

### Dev Preview -> Tech Preview

HyperShift's management-cluster feature-gate system has no Dev Preview equivalent, per
[Feature Gates](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/feature-gates.md),
all management-cluster gates sit under the single `TechPreviewNoUpgrade` feature set, so
this feature has no Dev Preview phase by construction. 

Entry requires:
- API approval and generated gated HC/HCP schemas.
- A CPO/autoscaler payload that recognizes every emitted flag.
- Documentation covering gate enablement, support limits, and the fleet-continuity caveat
  above.
- For the GPU field: the CAPI label/template changes, with enforcement verified on the
  target image, this gate does not ship an unverified GPU field.

### Tech Preview -> GA

Promote `HostedClusterAutoscalerConfiguration` to `Default` only after:
- The gated API and its behavior work reliably across supported platforms, and default/gated
  CRD-API compatibility holds.
- Both resource-limit bounds are verified across every supported CPO/autoscaler image
  combination, including multi-NodePool and GPU scenarios.
- The `delayAfterDeleteSeconds >= 60` floor (see [Risks and Mitigations](#risks-and-mitigations)
  and [Future Work](#future-work)) is reassessed against current CAPI deletion latency on every
  supported platform, or superseded by the autoscaler-side fix; GA must not carry forward an
  unexamined, live-testing-derived heuristic as a resource-floor guarantee.
- Upgrade behavior (omitted and configured settings, including old-CPO states) and
  operator-rollback behavior (with and without CRD rollback) are verified.
- Per HyperShift's
  [feature-gate promotion criteria](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/feature-gates.md#promoting-a-feature-gated-api-field):
  impact analysis for ROSA, ARO, IBM Cloud, and MCE; verified on-creation/on-update
  admission-failure UX; published documentation; validated day-2 changes and failure
  handling.

Then enable the gate in `Default` manifests and runtime registries, regenerate CRDs, and
verify the formerly gated fields are present by default without changing omission
behavior. Until promotion, there is no default availability or partial GPU parity claim.

### Removing a deprecated feature

Not applicable. This is a new feature with no deprecated predecessor.

## Upgrade / Downgrade Strategy

**Upgrade:** New CRD fields install only when the management-cluster feature set is
`TechPreviewNoUpgrade`; Default schemas keep omitting them. Existing HostedClusters keep
producing the historical `--skip-nodes-with-local-storage=false`,
`--balance-similar-node-groups=true`, and `--v=4`. No data migration or NodePool-bounds
change. The gated mixed-version rollout for this feature is a controlled compatibility
exercise, not a promised `TechPreviewNoUpgrade` upgrade path; at GA the same upgrade path is
fully supported.

**Planned rollback:** remove the new fields from HostedCluster and wait for baseline
arguments to reconcile before changing the operator binary.

**Emergency rollback while fields are configured:** the older operator's typed API and
HC-to-HCP copy can't carry the new fields, so a full-object write through old types removes
them, and an older CRD schema can independently prune them on write
([field-pruning rules](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#field-pruning)).
Neither path guarantees the values survive in spec or effective arguments, see
[Version Skew Strategy](#version-skew-strategy). If `localStorageScaleDown: Skip` can't be
confirmed and scale-down must stop immediately, fall back to the already-supported
`scaling: ScaleUpOnly` (removing any `scaleDown` block) and verify
`--scale-down-enabled=false`. After restoring a compatible operator and CRDs, re-apply the
previously configured fields from your own saved record, HyperShift keeps no separate
recovery copy. Control-plane release downgrades remain unsupported by HyperShift's existing
[upgrade policy](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/upgrades.md).

Reinstalling with the Default feature set while fields are still configured is also a
schema change that can prune them on write; treat it as a compatibility concern, not a
supported upgrade/downgrade procedure, and re-apply values explicitly afterward.

## Version Skew Strategy

The HyperShift operator, CPO, autoscaler image, and guest cluster can move independently.
The HC-to-HCP spec copy needs no new translator, but an older CPO can't act on new fields.
The feature needs the management-cluster gate enabled and a payload stream carrying both
the CPO translator and an autoscaler image with the required flags (GPU additionally needs
verified enforcement); support is determined per advertised release stream, not inferred
from a newer operator or guest-cluster feature set alone.

During mixed-version rollout, old CPO behavior stays in force until the new CPO reconciles
and the three historical arguments remain effective, and an old CPO keeps `--v=4` regardless
of a stored `logVerbosity`. Per [Operational Aspects](#operational-aspects-of-api-extensions),
there is no dedicated signal distinguishing "stored but not rolled out" from "fully
applied": check the live Deployment arguments directly rather than trusting the stored spec.
The same applies after a rollback to older operator/CPO binaries, previously rolled-out
arguments can persist without further reconciliation, and an older schema may have pruned
fields along the way, so compare against your own record of the intended configuration.

If a current CPO sees retained new HCP fields while the management gate is off, it renders
historical arguments; a Default CRD can instead prune the fields outright. There is no
status signal distinguishing a disabled gate from a field that was never set, check the
stored specs and feature set directly. No new kubelet, Machine, or guest operator API is
required.

## Operational Aspects of API Extensions

Extends the existing HostedCluster/HostedControlPlane CRDs, no new CRD,
admission/conversion webhook, finalizer, API server, or network dependency, and negligible
API overhead. With the gate enabled, invalid configurations fail admission with
field-specific messages; with it disabled, Default CRDs omit the fields and Kubernetes may
prune unknown keys instead of rejecting the request.

There is no dedicated status signal confirming a requested setting reached the running
autoscaler. Administrators confirm application the way they do for any other
Deployment-rolling field change: inspect the live autoscaler Deployment's container
arguments and its generic `Available`/`Progressing` conditions against the stored spec. CPO
rechecks and repairs the live Deployment after spec changes, but a valid resource maximum
can still block growth while the Deployment reports healthy.

An unsupported flag can fail the autoscaler container to start (visible via `Available` and
CPO availability reporting), leaving pending pods unscheduled. An overly tight but valid
limit instead leaves the Deployment healthy while scale-up is blocked, autoscaler logs,
guest pod events, and the `kube-system/cluster-autoscaler-status` ConfigMap distinguish the
two. HyperShift owns API/argument translation and CPO availability; the autoscaler/CAPI
provider maintainers own resource-accounting failures.

## Support Procedures

The user-facing [HyperShift autoscaling guide](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/autoscaling.md)
will include steps and examples of support procedures, starting with
inspecting the live autoscaler Deployment's effective arguments against
the stored spec, including after an operator rollback.

## Infrastructure Needed 

No new infrastructure, external dependency, or hardware is required: the feature extends
the existing HostedCluster/HostedControlPlane CRDs and the existing CPO/autoscaler
Deployment reconciliation.
