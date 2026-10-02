---
title: hosted-cluster-autoscaler-configuration-parity
authors:
  - "@georgelipceanu"
reviewers:
  - # TODO: add these
approvers:
  - # TODO: add these
api-approvers:
  - # TODO: add these
creation-date: 2026-10-02
last-updated: 2026-10-02
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

This enhancement lets hosted cluster administrators configure six remaining
ClusterAutoscaler behaviors through `HostedCluster.spec.autoscaling`: local-storage
node protection, balancing of similar node groups, DaemonSet utilization, cluster-wide
CPU/memory/GPU limits, cordoning during scale-down, and a delay for new pods during
scale-up. HyperShift translates these settings into arguments for its per-hosted-cluster
autoscaler. Omitted settings retain the current HyperShift behavior.

## Motivation

[OCPSTRAT-1806](https://redhat.atlassian.net/browse/OCPSTRAT-1806) and
[HyperShift PR #6236](https://github.com/openshift/hypershift/pull/6236) introduced
`ClusterAutoscaling`, including expanders, scale-down timings, and several global
limits. HyperShift still hardcodes `--skip-nodes-with-local-storage=false` and
`--balance-similar-node-groups=true` in its
[Deployment template](https://github.com/openshift/hypershift/blob/main/control-plane-operator/controllers/hostedcontrolplane/v2/assets/cluster-autoscaler/deployment.yaml).
It exposes neither the other four settings in this proposal nor their flags.

The local-storage setting is particularly visible during migration. The
[standalone ClusterAutoscaler API](https://docs.redhat.com/en/documentation/openshift_container_platform/4.22/html/autoscale_apis/clusterautoscaler-autoscaling-openshift-io-v1)
describes a default of `true`, while HyperShift explicitly sets `false`. The
autoscaler's local-volume rule covers disk-backed `emptyDir` and `hostPath`;
administrators should not infer from this flag alone that every local PersistentVolume
is protected. The upstream
[autoscaler FAQ](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#what-types-of-pods-can-prevent-ca-from-removing-a-node)
describes the local-volume and pod-annotation exceptions.

### User Stories

* As a hosted cluster administrator, I want to protect nodes running pods with
  supported local volumes so that scale-down follows my workload's eviction policy.
* As a platform engineer, I want aggregate resource limits across NodePools so that
  autoscaler-driven growth stays within a capacity budget.
* As a hosted cluster administrator, I want to tune balancing, DaemonSet utilization,
  cordoning, and new-pod delay so that autoscaling follows my workload pattern.
* As an SRE, I want to inspect both requested settings and effective autoscaler
  arguments so that I can diagnose scale-up and scale-down decisions after upgrades.

### Goals

* Make the six settings in
  [CNTRLPLANE-4426](https://redhat.atlassian.net/browse/CNTRLPLANE-4426)
  configurable per hosted cluster.
* Preserve effective behavior for every omitted setting, including on existing clusters.
* Define admission validation, upgrade behavior, documentation, and tests for every
  new setting.
* Deliver GPU limits only after an end-to-end test proves enforcement in the
  Cluster API autoscaler topology.

### Non-Goals

* Changing the existing HyperShift defaults.
* Exposing `logVerbosity` or changing the hardcoded `--v=4`. The parent
  [OCPSTRAT-3600](https://redhat.atlassian.net/browse/OCPSTRAT-3600) lists it, but
  the child feature explicitly excludes it.
* ROSA/OCM API integration, tracked by
  [ROSA-88](https://redhat.atlassian.net/browse/ROSA-88).
* Installing or using the standalone `ClusterAutoscaler` CR or its operator inside
  a hosted cluster.
* Guaranteeing that resource limits constrain manual NodePool changes or capacity
  already present above a configured maximum.

## Proposal

### Overview

Add typed fields to the existing `ClusterAutoscaling` and `ScaleDownConfig` types,
plus `ScaleUpConfig` and `ResourceLimits` types. The same `ClusterAutoscaling` type is
used by `HostedCluster` and `HostedControlPlane`. The HostedCluster controller already
copies `spec.autoscaling` to the HostedControlPlane; the control plane operator (CPO)
will adapt the cluster-autoscaler Deployment arguments from the latter.

The public on/off controls use string enums rather than new Boolean fields, following
the [OpenShift API conventions](../../dev-guide/api-conventions.md#do-not-use-boolean-fields).
Their names describe the action seen by an administrator. This proposal does not copy
the standalone `ClusterAutoscaler` API verbatim: HyperShift already uses its own
`scaling` enum, second-based scale-down delays, and top-level `maxNodesTotal`.

### Workflow Description

1. The administrator configures `HostedCluster.spec.autoscaling` and enables
   autoscaling on one or more NodePools.
2. The HostedCluster controller mirrors autoscaling configuration to the
   HostedControlPlane. CPO reconciles one autoscaler Deployment in that hosted
   control plane's management-cluster namespace.
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

This is a partial `spec` example; a complete HostedCluster also needs its existing
required platform, release, networking, and service fields. The GPU example
requires a GPU-capable NodePool and the acceptance test described below.

### API Extensions

| Proposed HostedCluster field | Type and accepted values | Autoscaler argument | Omitted behavior |
| --- | --- | --- | --- |
| `autoscaling.localStorageScaleDown` | `Skip` or `Consider` | `--skip-nodes-with-local-storage=true` or `false` | `false`, preserving HyperShift's current choice |
| `autoscaling.similarNodeGroupBalancing` | `Balance` or `DoNotBalance` | `--balance-similar-node-groups=true` or `false` | `true`, preserving HyperShift's current choice |
| `autoscaling.daemonSetUtilization` | `Include` or `Exclude` | `--ignore-daemonsets-utilization=false` or `true` | Omit the flag, preserving the autoscaler default of including requests |
| `autoscaling.resourceLimits.cores` | `{min, max}` in CPU cores | `--cores-total=min:max` | Omit the flag; retain the image's existing default |
| `autoscaling.resourceLimits.memory` | `{min, max}` in GiB | `--memory-total=min:max` | Omit the flag; retain the image's existing default |
| `autoscaling.resourceLimits.gpus[]` | `{type, min, max}` for each accelerator label value | Repeated `--gpu-total=type:min:max` | No GPU limits |
| `autoscaling.scaleDown.cordonNodeBeforeTerminating` | `Enabled` or `Disabled` | `--cordon-node-before-terminating=true` or `false` | Omit the flag; retain the image's existing default |
| `autoscaling.scaleUp.newPodScaleUpDelay` | Nonnegative Go duration string, for example `0s` or `30s` | `--new-pod-scale-up-delay=<duration>` | Omit the flag; autoscaler default is `0s` |

The ticket's `skipNodesWithLocalStorage`, `balanceSimilarNodeGroups`, and
`ignoreDaemonsetsUtilization` correspond respectively to
`localStorageScaleDown`, `similarNodeGroupBalancing`, and
`daemonSetUtilization` here. The names differ because the public API uses
action enums instead of Boolean flag names. The first and third policies
are inert while `scaling: ScaleUpOnly`, even though their arguments can be
rendered; cordoning is inside `scaleDown` and cannot be set in that mode.
For typed Go clients, an explicit empty enum value has the same effect as
omission.

The existing `autoscaling.maxNodesTotal` remains the only public
representation of the node-count limit; it is not duplicated under
`resourceLimits`. An explicit enum value always wins over the omitted
behavior. New fields have no CRD default so omission remains observable.

The range objects require both `min` and `max`. `min` may be zero, `max` must be
positive, and `min <= max`. Values are whole cores, whole GiB, and whole GPU
devices respectively. The autoscaler's resource minima are inputs to its
resource limiter; they can constrain scale-down but do not by themselves
create pending pods or force a NodePool above its own minimum. A maximum
blocks autoscaler scale-up that would cross it. These limits neither remove
existing excess nodes nor supersede NodePool `autoScaling.min`/`max`.

`resourceLimits` requires at least one of `cores`, `memory`, or `gpus`.
`gpus`, when present, is a nonempty list with unique `type` keys, represented
as a Kubernetes list-map keyed by `type`. `type` is a nonempty Kubernetes label
value (at most 63 characters), such as `nvidia-tesla-t4`. It is the value of
the guest Node's `cluster-api/accelerator` label, **not** a resource name such
as `nvidia.com/gpu`. Each GPU range has the same min/max rules as CPU and memory.
All three resource kinds are optional independently, so CPO emits only the
configured flags. Because `min: 0` is valid but omitting `min` is invalid,
the Go API must use `*int32` with `omitempty` for required minimum fields.
Positive maximums can use value fields with `omitempty`. This preserves an
explicit zero when clients serialize the configuration.

`scaleUp` requires `newPodScaleUpDelay` if the object is supplied. Admission
rejects an empty object, negative or malformed duration, and a duration that
cannot be parsed as a Go duration. `0s` is valid. Use required fields and
minimum/maximum markers for range endpoints, CEL for `min <= max` and
nonempty objects, and a duration pattern plus a CEL `duration(self)` parse
and nonnegative check. Verify that the CEL rule and Go's
`time.ParseDuration` accept the same boundary cases in envtest. The existing
CEL rule on `ClusterAutoscaling` continues to reject `scaleDown` when
`scaling` is `ScaleUpOnly`; it also covers the new cordon field. No new
webhook is proposed.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The autoscaler and CPO run in the management cluster's hosted-control-plane
namespace. The autoscaler uses its guest kubeconfig for pods, nodes, utilization,
local volumes, and node cordoning. Its Cluster API client discovers and scales
management-cluster CAPI node groups backing NodePools. Node-group balancing
depends on the autoscaler's comparison of CAPI node groups and guest-node
labels; the existing platform-specific `--balancing-ignore-label` arguments
remain in place. These settings add no new network path, credential, RBAC grant,
sidecar, or per-node resource overhead.

The node and pod policies and the new-pod delay are evaluated against
guest-cluster state before a CAPI scale action. Cordon targets the guest Node during
the autoscaler's deletion workflow; CAPI then reduces the corresponding node
group. CPU and memory limits use the autoscaler's cluster-wide resource limiter,
so their maximum is shared across all NodePools, not reset per NodePool.

GPU limits need particular validation in the CAPI topology. The
[OpenShift Cluster API provider](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/clusterapi/clusterapi_provider.go)
uses `cluster-api/accelerator` as its GPU label and currently returns `nil`
from `GetAvailableGPUTypes()`. In the current autoscaler code, that method
feeds GPU **metric labels**; the
[resource limiter](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/context/autoscaling_context.go)
receives `--gpu-total` independently, and the
[GPU resource processor](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/processors/customresources/gpu_processor.go)
accounts for labeled nodes using allocatable or template capacity. The
[CAPI provider guide](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/clusterapi/README.md)
describes `--gpu-total` with matching node labels, while the generic
[autoscaler FAQ](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#what-are-the-parameters-to-ca)
still cautions that the flag works only on GKE. The empty GPU-type list alone
does not prove that CAPI limit enforcement is broken. A GPU end-to-end test
must establish the effective behavior on the shipped image. If it fails,
fix the provider, node template, or image before claiming GPU support.

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

* Add the public fields and enum types to
  `api/hypershift/v1beta1/hostedcluster_types.go`. Use optional string enums
  for the three top-level policies and for cordoning, with the empty value
  allowed in their enum validation so Go zero values remain representable.
  Use `omitzero` for optional nested structs whose required children make
  `{}` invalid, and required pointer fields for zero-valued `min` in JSON.
  Generate the HostedCluster and
  HostedControlPlane schemas, deepcopy code, feature-gated manifests, clients,
  and API reference with `make update`; run the API linter.
* Remove only `--skip-nodes-with-local-storage=false` and
  `--balance-similar-node-groups=true` from
  `v2/assets/cluster-autoscaler/deployment.yaml`. Generate exactly one of each
  in `autoscalerArgs()`, using those same values when fields are omitted.
  Keep the template's `--v=4` because verbosity is outside this proposal.
  Emit the other new arguments only when their fields are present.
* Preserve the existing HostedCluster-to-HostedControlPlane copy of
  `spec.autoscaling`. CPO continues to derive Deployment arguments from
  HostedControlPlane configuration; it must never depend on the standalone
  `ClusterAutoscaler` CR.
* HyperShift defines its own `ClusterAutoscaling` and `ScaleDownConfig` types.
  The older vendored `cluster-autoscaler-operator` types are currently imported
  by standalone test setup, not by the production HCP argument translator.
  Sync that dependency and vendor directory to the target OpenShift release
  so the standalone test setup has current API types, but do not make them
  the source of the HostedCluster API. Their update alone will not implement
  any HCP setting.
* The GPU acceptance test is a delivery dependency of
  [CNTRLPLANE-4429](https://redhat.atlassian.net/browse/CNTRLPLANE-4429).
  A GPU-capable NodePool must publish the matching accelerator label and GPU
  capacity in both real and scale-from-zero template nodes. If the test finds
  a CAPI provider or template gap, fix it in the target autoscaler image
  before documenting the field as a functioning limit.
* Document the fields in
  `hypershift/docs/content/how-to/autoscaling.md` and regenerate
  `docs/content/reference/api.md`. Include omitted defaults, example values,
  the meaning of GPU `type`, the provider prerequisite, and the fact that a
  resource maximum constrains autoscaler scale-up rather than manual changes.

### Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Moving hardcoded flags changes existing clusters | Emit `false` and `true` respectively for omitted values; assert exact effective arguments before and after upgrade. |
| Invalid enum, range, GPU type, or duration stops autoscaler startup | Reject invalid values at CRD admission; test rendered arguments against a supported autoscaler image. |
| GPU flag is accepted but not enforced in a target image | Require a real limit test before claiming GPU support; fix any provider or template gap and document label prerequisites. |
| A tight limit prevents pending workloads from scaling | Validate ranges, document cluster-wide accounting and NodePool bounds, and surface autoscaler events and logs in support guidance. |
| Scale-down settings increase eviction risk | Keep current defaults; document local-volume exceptions, PodDisruptionBudgets, and `safe-to-evict` annotations. |
| New API fields reach an older CPO during version skew | Sequence payload support before announcing availability, and verify applied Deployment arguments after CPO rollout. |

### Drawbacks

HyperShift must maintain another set of public policy enums and a translation
table alongside the standalone API. Because the settings are per HostedCluster,
the CPO restarts that cluster's autoscaler when they change. GPU limits need
GPU-capable CI and might require an autoscaler image change if the acceptance
test reveals a provider or template gap.

## Alternatives (Not Implemented)

* **Copy the standalone `ClusterAutoscaler` CR or install its operator in the
  guest cluster.** HyperShift already owns a different API and deploys the
  autoscaler directly with a management/guest dual-client setup. A second
  authority for autoscaler settings would create conflicting reconciliation.
* **Use new Boolean fields named exactly like the autoscaler flags.** This
  matches the ticket terminology but conflicts with OpenShift's API convention
  against new Boolean fields. The proposed enums also give each value an
  explicit user-facing meaning.
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

The API names and enum values above are the proposed review surface. These may be refined them before merge, but the action semantics, omission
behavior, and validation rules are design requirements. The GPU end-to-end
test determines whether any provider or image change is necessary before
the GPU field is released.

## Test Plan

* **API and unit:** Cover serialization of omitted fields and explicit
  `min: 0`; test N-1/N+1 API deserialization. Extend
  `TestAdaptDeployment` with omitted settings, both values of every policy,
  mixed settings, `0s`, multiple GPU types, and assertions that each argument
  appears once. Verify the template still yields `--v=4` and the two existing
  hardcoded defaults after translation.
* **Envtest:** Use the generated HostedCluster and HostedControlPlane CRDs to
  accept valid enums, ranges, unique GPU types, and duration values. Reject
  unknown enums, empty `resourceLimits`/`scaleUp`, missing range endpoints,
  negative or inverted ranges, duplicate GPU types, invalid label values,
  malformed or negative durations, and `scaleDown` with `ScaleUpOnly`.
* **End to end:** On at least two autoscaled NodePools, verify
  `localStorageScaleDown: Skip` prevents deletion of a node with a qualifying
  disk-backed local volume and `Consider` restores eligibility. Assert
  argument application for balancing and DaemonSet utilization, observe
  cordoning in the guest cluster during a scale-down, and verify a new-pod
  delay holds scale-up until the configured age. Test CPU and memory limits
  at their boundary: a pending workload triggers growth below the maximum
  and cannot grow the cluster beyond it, regardless of which NodePool could
  satisfy it. Verify removal of fields restores baseline arguments.
* **GPU end to end:** Run on a GPU-capable CAPI NodePool with matching
  `cluster-api/accelerator` labels and capacity on real and template nodes.
  Show scale-up below the GPU maximum and rejection at the maximum, including
  scale-from-zero if that platform supports it. This test is required for
  GPU delivery; checking the argument alone is insufficient.
* **Upgrade:** Upgrade an existing hosted cluster with no new fields and
  compare effective arguments before and after. Exercise a new HyperShift
  operator with an old CPO, then a new CPO, and verify the configuration
  becomes effective only when the translator is present. Include an
  operator rollback after first removing the new fields.

## Graduation Criteria

### Dev Preview -> Tech Preview

No separate Dev Preview or Tech Preview API is planned. These optional fields
extend an existing autoscaler configuration API, and the proposal targets
availability by default after the required tests and payload compatibility pass.

### Tech Preview -> GA

The release gate for default availability is: API approval; generated schemas
and API compatibility tests; admission, unit, and multi-NodePool end-to-end
tests; an upgrade test that preserves omitted behavior; published HyperShift
documentation; and a tested CPO/autoscaler image combination in every
advertised hosted-cluster stream. GPU support additionally requires the GPU
end-to-end result and any provider or template fix that result identifies.
No partial GPU parity claim is made before that gate.

### Removing a deprecated feature

This enhancement removes no API field or deprecated feature. Moving the two
hardcoded arguments from the template to generated arguments preserves their
effective values.

## Upgrade / Downgrade Strategy

On upgrade, the new CRD schemas are installed before users can submit the
new fields. Existing HostedClusters omit them; CPO still produces
`--skip-nodes-with-local-storage=false`, `--balance-similar-node-groups=true`,
and `--v=4`. There is no data migration or change to NodePool bounds. A
configured setting takes effect when the HostedControlPlane copy and the new
CPO have reconciled the autoscaler Deployment.

To roll back the HyperShift operator to a version that lacks this API, first
remove the new fields from HostedCluster and wait for baseline autoscaler
arguments to reconcile. Older schemas or typed clients may discard unknown
fields; relying on their preservation is unsupported. Control-plane release
downgrades remain unsupported by HyperShift's existing
[upgrade policy](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/upgrades.md).
An operator rollback does not roll back the CPO or autoscaler image already
selected by the hosted control plane release.

## Version Skew Strategy

The HyperShift operator, CPO, autoscaler image, and guest cluster can move at
different times. The HostedCluster controller's existing spec copy needs no
new translator, but an older CPO cannot act on new autoscaling fields. The
feature is supported only for hosted-control-plane payload streams that contain
both the CPO argument translator and an autoscaler image with the required
flags; GPU additionally needs a passing enforcement test. Support is determined per
advertised release stream and verified by CI, rather than inferred from a
newer management-cluster operator alone.

During mixed-version rollout, old CPO behavior remains in force until the new
CPO reconciles. The two historical arguments stay effective either way.
New CPO emits only flags known to its paired release image. Documentation and
support procedures require checking the Deployment arguments before treating
a requested value as applied. No new kubelet, Machine, or guest operator API
is required.

## Operational Aspects of API Extensions

This extends the existing HostedCluster and HostedControlPlane CRDs. It adds
no new CRD, admission/conversion webhook, finalizer, API server, or network
dependency. Invalid configurations fail admission with field-specific enum,
range, list, or duration messages, before they can roll the autoscaler.
There is no expected change to general API throughput or control-plane
resource consumption from the small additional spec fields.

If an unsupported flag reaches an autoscaler image, its container can fail
to start; the cluster-autoscaler Deployment's `Available` condition and the
existing CPO component availability reporting reveal that failure. Pending
pods may then remain unscheduled because autoscaler-driven NodePool growth
stops; existing nodes and running workloads remain. A valid but overly tight
limit leaves the Deployment healthy while scale-up is blocked. Autoscaler
logs, guest pod events, and the `kube-system/cluster-autoscaler-status`
ConfigMap distinguish this from an unhealthy Deployment. The HyperShift team
owns API/argument translation and CPO availability; the autoscaler/CAPI
provider maintainers own resource accounting failures.

## Support Procedures

1. Read `HostedCluster.spec.autoscaling`, the mirrored
   `HostedControlPlane.spec.autoscaling`, and the autoscaler Deployment's
   container arguments in the hosted-control-plane namespace. An omitted
   local-storage or balancing field must still yield `false` or `true`
   respectively; other omitted new fields yield no new flag.
2. Check the Deployment's `Available` condition and CPO component condition.
   For a failed container, inspect its logs for an unknown flag or parsing
   error and confirm that its image belongs to a supported payload stream.
3. For a healthy autoscaler that does not scale, inspect its logs, guest pod
   events, `kube-system/cluster-autoscaler-status`, NodePool min/max values,
   and aggregate guest-node CPU, memory, and GPU capacity. Check
   `cluster-api/accelerator` labels and CAPI template capacity for GPU limits.
4. To restore pre-enhancement behavior, remove the new HostedCluster fields
   and wait for the autoscaler Deployment to reconcile. This changes
   autoscaler decisions prospectively; it does not delete existing nodes or
   override NodePool bounds.

The user-facing [HyperShift autoscaling guide](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/autoscaling.md)
will include these steps and examples. Support should use the effective
Deployment arguments as the source of truth during mixed-version upgrades.

## Infrastructure Needed 

The existing HyperShift envtest harness and two-NodePool autoscaling test can
host the API, translation, CPU/memory, and node-policy cases. GPU graduation
additionally needs a CI lane with a GPU-capable CAPI NodePool and the target
payload image.
