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
  - "@csrwng"
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
settings retain the current HyperShift behavior. The new settings start behind
the `HostedClusterAutoscalerConfiguration` management-cluster feature gate in
`TechPreviewNoUpgrade`; they become available by default only after graduation.

## Motivation

[OCPSTRAT-1806](https://redhat.atlassian.net/browse/OCPSTRAT-1806) and
[HyperShift PR #6236](https://github.com/openshift/hypershift/pull/6236) introduced
`ClusterAutoscaling`, including expanders, scale-down timings, and several global
limits. HyperShift still hardcodes `--skip-nodes-with-local-storage=false`,
`--balance-similar-node-groups=true`, and `--v=4` in its
[Deployment template](https://github.com/openshift/hypershift/blob/main/control-plane-operator/controllers/hostedcontrolplane/v2/assets/cluster-autoscaler/deployment.yaml).
The other four settings in this proposal have no public fields or generated
flags yet.

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
* As an SRE, I want to change the hosted cluster autoscaler's log verbosity so
  that I can inspect scaling decisions and restore normal log volume afterward.
* As an SRE, I want HostedCluster status to show whether requested autoscaler
  settings reached the running autoscaler, so I can detect unapplied settings
  during upgrades without inspecting management-cluster Deployment arguments.

### Goals

* Make the seven settings in
  [CNTRLPLANE-4426](https://redhat.atlassian.net/browse/CNTRLPLANE-4426)
  configurable per hosted cluster.
* Preserve effective behavior for every omitted setting, including on existing clusters.
* Define admission validation, upgrade behavior, documentation, and tests for every
  new setting.
* Report whether requested settings have reached the autoscaler Deployment in
  HostedControlPlane and HostedCluster status.
* Preserve an operator-managed recovery snapshot of new settings so an
  emergency operator or CRD rollback has a deterministic restoration source.
* Deliver GPU limits only after an end-to-end test proves enforcement in the
  Cluster API autoscaler topology.
* Introduce the settings behind a HyperShift management-cluster feature gate
  and keep the Default feature set's autoscaler behavior unchanged.

### Non-Goals

* Changing the existing HyperShift defaults.
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
Their names describe the action seen by an administrator. Log verbosity uses
the standalone autoscaler's numeric field name so administrators can select
a precise `--v` level. This proposal does not copy the standalone
`ClusterAutoscaler` API verbatim: HyperShift already uses its own
`scaling` enum, second-based scale-down delays, and top-level `maxNodesTotal`.

### Feature Gate and Availability

The dedicated `HostedClusterAutoscalerConfiguration` gate initially belongs to
HyperShift's [management-cluster `TechPreviewNoUpgrade` feature set](https://hypershift.pages.dev/how-to/feature-gates/)
and is disabled in `Default`. An administrator enables that set when installing
HyperShift with `hypershift install --tech-preview-no-upgrade`. This selects
the feature-gated HostedCluster and HostedControlPlane CRD schemas and sets
`HYPERSHIFT_FEATURESET` for the HyperShift operator and CPO. The gate applies
to the seven settings in this proposal, including `logVerbosity`; existing
`spec.autoscaling` fields and their behavior remain available with the gate
off. A single management-cluster selection exposes the new API to its hosted
clusters, while each HostedCluster opts in to behavior by setting a field.

This is distinct from `HostedCluster.spec.configuration.featureGate`, which
controls features inside one guest cluster and does not enable these API
fields. The proposed example below requires a management cluster installed
with the Tech Preview feature set. The Default CRD variant omits the new
fields on both HostedCluster and HostedControlPlane. Kubernetes can prune
unknown CRD fields rather than reject a request, so administrators must read
back the stored object and confirm the management feature set before assuming
a new setting was accepted. CPO also checks the gate before rendering any
new argument, including if a stale field survives a feature-set or binary
change. It keeps the historical effective arguments when the gate is off.
With the gate off and no retained request, there is no
`AutoscalerConfigurationApplied` condition. If a new field survives in an
older feature-gated schema while the gate is off, a current CPO reports
`AutoscalerConfigurationApplied=False` with reason `FeatureGateDisabled`
instead of claiming it applied the request. A condition retained from an
older binary is not proof of application after the gate or operator changes.

### Workflow Description

1. On a management cluster with the gate enabled, the administrator configures
   `HostedCluster.spec.autoscaling` and enables autoscaling on one or more
   NodePools.
2. The HostedCluster controller first appends a recovery revision of the new
   settings on HostedCluster metadata, then mirrors autoscaling configuration
   to the HostedControlPlane. CPO reconciles one autoscaler Deployment in that
   hosted control plane's management-cluster namespace.
3. The autoscaler reads guest-cluster pods and nodes using its kubeconfig. With
   `--cloud-provider=clusterapi`, it discovers and scales the backing CAPI node
   groups in the management cluster. A global resource limit therefore applies
   to the aggregate guest-cluster nodes, across all autoscaled NodePools.
4. Updating or removing a field reconciles the arguments and rolls the Deployment.
   Removing all new fields restores the effective pre-enhancement configuration.
5. CPO reports whether the desired arguments have reached a completed autoscaler
   Deployment rollout. The HostedCluster controller exposes this result in
   HostedCluster status when any new setting is requested and its latest
   recovery revision matches the request.

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

This is a partial `spec` example; a complete HostedCluster also needs its existing
required platform, release, networking, and service fields. The GPU example
requires a GPU-capable NodePool whose
`spec.nodeLabels["cluster-api/accelerator"]` is `nvidia-tesla-t4`, plus the
acceptance test described below.

### API Extensions

| Proposed HostedCluster field | Type and accepted values | Autoscaler argument | Omitted behavior |
| --- | --- | --- | --- |
| `autoscaling.localStorageScaleDown` | `Skip` or `Consider` | `--skip-nodes-with-local-storage=true` or `false` | `false`, preserving HyperShift's current choice |
| `autoscaling.similarNodeGroupBalancing` | `Balance` or `DoNotBalance` | `--balance-similar-node-groups=true` or `false` | `true`, preserving HyperShift's current choice |
| `autoscaling.daemonSetUtilization` | `Include` or `Exclude` | `--ignore-daemonsets-utilization=false` or `true` | Omit the flag, preserving the autoscaler default of including requests |
| `autoscaling.logVerbosity` | Integer from `0` through `10` | `--v=<level>` | `--v=4`, preserving HyperShift's current choice |
| `autoscaling.resourceLimits.cores` | `{min, max}` in CPU cores | `--cores-total=min:max` | Omit the flag; retain the image's existing default |
| `autoscaling.resourceLimits.memory` | `{min, max}` in GiB | `--memory-total=min:max` | Omit the flag; retain the image's existing default |
| `autoscaling.resourceLimits.gpus[]` | 1–64 unique `{type, min, max}` entries; `type` excludes `cpu` and `memory`; `max: 0` is valid | Repeated `--gpu-total=type:min:max` | No GPU limits |
| `autoscaling.scaleDown.cordonNodeBeforeTerminating` | `Enabled` or `Disabled` | `--cordon-node-before-terminating=true` or `false` | Omit the flag; retain the image's existing default |
| `autoscaling.scaleUp.newPodScaleUpDelay` | Go duration from `0s` through `1h`, for example `30s` | `--new-pod-scale-up-delay=<duration>` | Omit the flag; autoscaler default is `0s` |

The ticket's `skipNodesWithLocalStorage`, `balanceSimilarNodeGroups`, and
`ignoreDaemonsetsUtilization` correspond respectively to
`localStorageScaleDown`, `similarNodeGroupBalancing`, and
`daemonSetUtilization` here. The names differ because the public API uses
action enums instead of Boolean flag names. `localStorageScaleDown` and
`daemonSetUtilization` affect only scale-down. With `scaling: ScaleUpOnly`,
their configured values are retained and their arguments can be rendered,
but no scale-down occurs. If `scaling` later changes to
`ScaleUpAndScaleDown`, those stored values take effect after reconciliation
without another edit to the policy fields. For example, a previously set
`localStorageScaleDown: Skip` starts protecting qualifying local-volume nodes
when scale-down becomes active. Administrators should review both values
before enabling scale-down. Cordoning is inside `scaleDown` and cannot be
set with `ScaleUpOnly`.

For typed Go clients, an explicit empty enum value has the same effect as
omission.

The existing `autoscaling.maxNodesTotal` remains the only public
representation of the node-count limit; it is not duplicated under
`resourceLimits`. An explicit enum value always wins over the omitted
behavior. New fields have no CRD default so omission remains observable.

`logVerbosity` is a top-level `ClusterAutoscaling` field, independent of
`scaling` and the control-plane components' `operatorConfiguration.*.logLevel`.
It changes only the hosted cluster autoscaler's klog verbosity, including
when `scaling` is `ScaleUpOnly`; it does not change scaling policy. The
[standalone ClusterAutoscaler documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/4.22/html/machine_management/applying-autoscaling)
describes levels `0` through `10`; the current standalone
[CRD schema](https://github.com/openshift/cluster-autoscaler-operator/blob/49d03c60e05e8856dde5a416e01c8512cc824a59/install/01_clusterautoscaler.crd.yaml)
enforces only the lower bound. HyperShift enforces the documented inclusive
range with CRD minimum and maximum markers. Its omitted value remains
`--v=4`, unlike the standalone operator's default of `1`. Use `*int32` with
`omitempty` in the Go API: `nil` means the historical level `4`, while an
explicit `0` must serialize and render as `--v=0`. No CRD default is added.

`maxNodesTotal` and each configured `resourceLimits` maximum are independent,
cluster-wide ceilings. A scale-up must stay within both the node-count limit
and the resource limits, as well as the target NodePool's `autoScaling.max`.
For example, six 8-core nodes in a CPU NodePool and one 8-core, one-GPU node
in a GPU NodePool use seven nodes and 56 cores. With `maxNodesTotal: 10`,
`resourceLimits.cores.max: 64`, and room in both NodePool maxima, one more GPU
node brings the cluster to eight nodes and 64 cores. A subsequent CPU node
would bring it to 72 cores, so the autoscaler blocks that scale-up even though
the node count would still be below ten. The core budget is shared across
NodePools; it is not a separate 64-core allowance for each pool.

The range objects require both `min` and `max`. `min` may be zero and must not
exceed `max`. CPU and memory maxima must be positive; GPU maxima may be zero.
Values are whole cores, whole GiB, and whole GPU devices respectively. A
configured minimum prevents autoscaler scale-down that would take aggregate
capacity below that resource floor. It does not create pending pods or trigger
scale-up to provision capacity up to the floor.
A maximum blocks autoscaler scale-up that would cross it. These limits neither
remove existing excess nodes nor supersede NodePool `autoScaling.min`/`max`.
In particular, `gpus: [{type: nvidia-tesla-t4, min: 0, max: 0}]` means the
autoscaler must not grow capacity for that accelerator type; it does not remove
an existing GPU NodePool or force deletion of existing GPU nodes. This matches
the [pinned autoscaler's parser](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/config/flags/flags.go)
accepting `--gpu-total=nvidia-tesla-t4:0:0` and its
[resource limiter](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/resource_limiter.go)
retaining a zero maximum. It lets an administrator retain the NodePool
configuration while pausing autoscaler-driven GPU growth.

`resourceLimits` requires at least one of `cores`, `memory`, or `gpus`.
`gpus`, when present, has 1–64 entries with unique `type` keys, represented
as a Kubernetes list-map keyed by `type`. Use
`+kubebuilder:validation:MinItems=1` and
`+kubebuilder:validation:MaxItems=64` on the gated `gpus` field. The upper
bound limits validation work and generated `--gpu-total` arguments while
allowing many accelerator types in one hosted cluster. `type` is a nonempty
Kubernetes label value (at most 63 characters), such as `nvidia-tesla-t4`.
It is the value of the guest Node's `cluster-api/accelerator` label, **not**
a resource name such as `nvidia.com/gpu`. The values `cpu` and `memory` are
reserved and rejected,
even though both satisfy Kubernetes label-value syntax. The
[autoscaler's resource-limiter constructor](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/context/autoscaling_context.go)
stores CPU, memory, and GPU limits in maps keyed by those names, so a GPU
entry with either reserved type would overwrite the corresponding resource
limit. For example, `cores.max: 128` alongside a GPU entry with `type: cpu`
and `max: 8` would replace the CPU maximum with 8. Enforce the exclusion with
a field-level CEL rule on `gpus[].type`:
`self != 'cpu' && self != 'memory'`. Generate this validation in both the
HostedCluster and HostedControlPlane CRD schemas. Each GPU range has the same
nonnegative minimum and `min <= max` rule as CPU and memory, but its maximum
uses a lower bound of zero rather than one.

All three resource kinds are optional independently, so CPO emits only the
configured flags. Because `min: 0` is valid but omitting `min` is invalid,
the Go API must use `*int32` with `omitempty` for required minimum fields.
GPU `max` must also use `*int32` with `omitempty`: an explicit zero must survive
typed-client serialization while an omitted maximum fails validation. CPU and
memory maxima can use value fields with `omitempty` because zero is invalid.
The generated CRD validation must preserve these different lower bounds.

`scaleUp` requires `newPodScaleUpDelay` if the object is supplied. Admission
rejects an empty object, a negative or malformed duration, and a duration
greater than `1h`. Both `0s` and `1h` are valid. This one-hour cap limits how
long a cluster-wide setting can exclude a Pod based on its age. The
[autoscaler's `filterOutYoungPods` function](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/core/static_autoscaler.go)
measures the delay from the Pod's `metadata.creationTimestamp`, not from when
it first becomes unschedulable. A Pod already older than the effective delay
can be considered for scale-up on the next autoscaler scan, even if it has
only just become unschedulable. The existing
`cluster-autoscaler.kubernetes.io/pod-scale-up-delay` annotation can extend
the global delay for an individual Pod. If the annotation specifies a shorter
delay, the autoscaler rejects that override and keeps the global delay; the
annotation cannot shorten it. The one-hour API cap applies to the global
field and does not change the annotation's existing behavior.

Use required fields and minimum/maximum markers for range endpoints, CEL for
`min <= max` and nonempty objects, and a duration pattern plus CEL validation
with `duration(self) >= duration('0s') && duration(self) <= duration('1h')`.
Validate the parsed duration rather than its string representation, so `60m`
is valid and `1h1ns` is rejected. In envtest, verify that accepted boundary
values parse to the same duration in Go and that admission rejects values
just above the cap. The existing CEL rule on `ClusterAutoscaling` continues
to reject `scaleDown` when `scaling` is `ScaleUpOnly`; it also covers the new
cordon field. No new webhook is proposed.

### Rollback Recovery Snapshot

The HyperShift operator writes a versioned JSON recovery record to the
`hypershift.openshift.io/autoscaler-configuration-recovery` annotation on the
HostedCluster. It contains the HostedCluster UID and an append-only list of
revisions. Each revision has a source spec generation and the new autoscaling
fields present at that generation, plus the existing `scaling` value and
complete `scaleDown` block when present. This also preserves existing
scale-down timings if an emergency `ScaleUpOnly` change must remove the block.
Explicit zero values survive serialization. No other existing autoscaling
fields or secrets are copied. For example:

```json
{
  "version": 1,
  "hostedClusterUID": "<uid>",
  "revisions": [
    {
      "sourceGeneration": 12,
      "settings": {
        "scaling": "ScaleUpAndScaleDown",
        "localStorageScaleDown": "Skip",
        "logVerbosity": 0
      }
    }
  ]
}
```

Nested `scaleDown` in each `settings` object contains the whole existing block,
including the new cordon field when set. Restoration of new fields merges
with the current block, unless an emergency `ScaleUpOnly` update removed it;
then the administrator can restore the saved block with the previous scaling
mode. The 64-entry GPU cap bounds each revision; a revision with 64 GPU
entries is at most approximately 3 KiB of JSON. The history itself retains
at most the 20 most recent revisions: before appending a new one, the
operator drops the oldest revisions in excess of that count. Twenty
worst-case revisions therefore stay near 60 KiB, well under the
[256 KiB per-object annotation ceiling](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/)
even alongside other annotations on the same HostedCluster. This bound is
independent of how often an administrator changes settings, so routine
tuning, for example repeatedly changing `logVerbosity` for a debugging
session, cannot by itself exhaust the annotation and block an unrelated
future change. Recovery always targets the latest revision before an
unplanned change; retaining history further back would not change what
emergency rollback restores, so the operator does not need to preserve the
oldest (baseline) revision once it ages out of the most recent 20. If a
single new revision would still exceed the size limit on its own, the
operator does not apply the new request and reports `RecoverySnapshotFailed`,
since no amount of pruning the existing history can make room for it. The
annotation is metadata, so replacing the HC or HCP CRD schema does not prune
it. The rollback test verifies that older typed operators preserve
`metadata.annotations` on their full-object writes and that many sequential
configuration changes roll older revisions off without ever blocking a new
one. Neither CPO nor the HostedCluster controller derives desired autoscaler
arguments from the recovery record or automatically restores spec fields
from it.

With the management feature gate enabled, the HostedCluster controller
appends a revision before mirroring a changed nonempty request to HCP. It
patches only this annotation with optimistic concurrency, reads it back, and
verifies that its UID and latest revision still match the backed-up HC fields.
If the request matches the latest revision, no new entry is needed. On a
conflict it retries; if an annotation-size failure persists, it does not
advance the HCP spec. The HC `AutoscalerConfigurationApplied` condition
cannot become `True` until this
matching latest revision exists as well as the existing HCP and Deployment
checks. While the record is missing or stale, report `Unknown` with reason
`RecoverySnapshotPending`, or `False` with `RecoverySnapshotFailed` for a
nonretryable write error such as the annotation size limit. A change to any
backed-up setting appends another revision before that change is applied,
including when an older schema preserved only some new fields. The prior complete
revision remains available. If every new field is removed, the controller
restores baseline arguments and removes the HC application condition, but
retains the recovery history, subject only to the 20-revision cap described
above. It never erases the history merely because the current spec looks
empty, because the same empty spec can result from CRD pruning during
rollback.

After emergency rollback, an administrator restores a compatible gated or
GA operator and CRD, reads the annotation, checks its version and HC UID,
selects the last desired revision before the rollback, and explicitly
patches only the missing new fields back into `HostedCluster.spec.autoscaling`.
The API server validates the restored
values; other supported autoscaling fields remain untouched. If emergency
protection set `scaling: ScaleUpOnly`, restore the selected revision's full
`scaleDown` block, including its old timings, in the same HC update that
switches `scaling` back to `ScaleUpAndScaleDown`; the revision write and HCP
copy then precede renewed scale-down. The administrator then waits for a
current `AutoscalerConfigurationApplied=True`
and checks the live Deployment arguments. There is no automatic replay:
the history can include intentionally removed settings and a partial spec
observed after rollback. The guarantee covers settings that reached HC
`True`; a request accepted immediately before the new controller can append
its revision has not been applied and may still be lost on rollback. If the
annotation is absent, there is no in-cluster copy from which to reconstruct
that unobserved request.

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
ordering. CPU and memory limits use the autoscaler's cluster-wide resource
limiter, so their maximum is
shared across all NodePools, not reset per NodePool.

`logVerbosity` changes the log output of the management-cluster autoscaler
container only. It does not change the CPO's logs, guest control-plane logs,
or the inputs to an autoscaling decision. Updating it changes the Deployment
pod template and rolls the autoscaler. The
[autoscaler FAQ](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#how-can-i-increase-the-information-that-the-ca-is-logging)
describes the additional log volume and possible performance cost at high
verbosity.

GPU limits need particular validation in the CAPI topology. The
[OpenShift Cluster API provider](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/clusterapi/clusterapi_provider.go)
uses `cluster-api/accelerator` as its GPU label and currently returns `nil`
from `GetAvailableGPUTypes()`. In the current autoscaler code, that method
feeds GPU **metric labels**; the
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
copies `nodeLabels` to management-cluster CAPI Machines with a managed prefix;
the [hosted-cluster Node controller](https://github.com/openshift/hypershift/blob/f6237324195976af37bf133a7033b92a545c8c77/control-plane-operator/hostedclusterconfigoperator/controllers/node/node.go#L64-L98)
removes that prefix and applies the label to guest Nodes once after creation.
In Replace mode, Machine labels are currently patched after Machine creation,
so GPU delivery must verify they are present before that one-time Node sync;
otherwise the label must be provisioned on new Machines earlier.
Existing guest GPU Nodes must already have this label or be relabeled before
the limit is enabled; changing `NodePool.spec.nodeLabels` does not backfill
labels on Nodes that have already been synced. On real guest Nodes, the GPU
device plugin supplies the separate extended-resource capacity, such as
`nvidia.com/gpu`. HostedCluster admission can validate the GPU type's syntax,
but cannot verify its match with a separate NodePool or a live guest Node.

For scale-from-zero, the current [AWS/Azure NodePool path](https://github.com/openshift/hypershift/blob/f6237324195976af37bf133a7033b92a545c8c77/hypershift-operator/controllers/nodepool/scale_from_zero.go#L66-L159)
puts `NodePool.spec.nodeLabels` in the scalable MachineDeployment or MachineSet's
`capacity.cluster-autoscaler.kubernetes.io/labels` annotation and records a GPU
count in `machine.openshift.io/GPU` when the infrastructure template has no
`Status.Capacity`. When native `Status.Capacity` is present, it removes those
annotations; the Replace MachineDeployment template does not itself carry the
unprefixed accelerator label. Therefore GPU delivery includes a NodePool
controller change to publish the matching accelerator label in the CAPI
scale-from-zero template regardless of the capacity source. The template must
also expose the correct GPU extended-resource count through native capacity or
annotations understood by the target autoscaler image. The
[CAPI provider guide](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/cloudprovider/clusterapi/README.md)
describes `--gpu-total` with matching node labels, while the generic
[autoscaler FAQ](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#what-are-the-parameters-to-ca)
still cautions that the flag works only on GKE. The empty GPU-type list alone
does not prove that CAPI limit enforcement is broken. A GPU end-to-end test
must establish the effective behavior on the shipped image, including both
real-node and scale-from-zero accounting. If it fails, fix the NodePool
template, provider, or image before claiming GPU support.

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
  Add `ClusterAutoscaling.logVerbosity` as `*int32` with `omitempty`, minimum
  `0`, and maximum `10`; do not add a CRD default, so an omitted field stays
  distinct from an explicit `0`. Use `omitzero` for optional nested structs
  whose required children make `{}` invalid, and required pointer fields for
  zero-valued `min` in JSON. Mark each new `ClusterAutoscaling` field and
  `ScaleDownConfig.cordonNodeBeforeTerminating` with
  `+openshift:enable:FeatureGate=HostedClusterAutoscalerConfiguration` at the
  field that introduces it; the `scaleUp` and `resourceLimits` markers gate
  their nested schemas. Gate any new validation whose parent field remains in
  the Default schema. Because `ClusterAutoscaling` is shared, generate
  both HostedCluster and HostedControlPlane schemas with the fields present
  only in the Tech Preview variant. Generate the schemas, deepcopy code,
  feature-gated manifests, clients, and API reference with `make update`;
  run the API linter. Give `resourceLimits.gpus` both `MinItems=1` and
  `MaxItems=64` validation markers on its list-map field.
* Register `HostedClusterAutoscalerConfiguration` in the HyperShift operator
  and CPO feature-gate registries. Add it to the enabled list in the
  `Hypershift` and `SelfManagedHA` `TechPreviewNoUpgrade` gate manifests and
  to the disabled list in their `Default` manifests. The management-cluster
  `HYPERSHIFT_FEATURESET` controls runtime behavior; do not use the guest
  cluster's feature-gate setting to make this decision.
* Remove `--skip-nodes-with-local-storage=false`,
  `--balance-similar-node-groups=true`, and `--v=4` from
  `v2/assets/cluster-autoscaler/deployment.yaml`. Generate exactly one of each
  in `autoscalerArgs()`, using those same values when fields are omitted. If
  `logVerbosity` is present, render its value directly, including `--v=0`;
  otherwise render `--v=4`. Keep the template's unrelated logging flag
  `--alsologtostderr`. Emit the other new arguments only when their fields
  are present.
* Preserve the existing HostedCluster-to-HostedControlPlane copy of
  `spec.autoscaling`. CPO continues to derive Deployment arguments from
  HostedControlPlane configuration; it must never depend on the standalone
  `ClusterAutoscaler` CR. Guard translation of every new field with the CPO's
  management-cluster gate check. With the gate off, emit the historical
  effective arguments even if old gated schemas leave new fields in the HCP
  spec. Gate the GPU-specific NodePool label/template changes as part of this
  feature as well.
* With the management gate enabled, before mirroring any new setting to HCP,
  have the HostedCluster controller append to and re-read the versioned HC
  recovery annotation defined above.
  Patch only metadata with a resource-version precondition and retry on
  conflict. Compare the decoded latest revision with the backed-up fields in
  the latest HC spec, including explicit zeros and current scale-down timings.
  Do not copy a newer request to HCP while the
  record is missing or mismatched. Cap the revision list at the 20 most
  recent entries, dropping the oldest revisions before appending a new one
  once that count would be exceeded. Retain older revisions up to that cap
  when a partial request follows, and never erase the history for any other
  reason, including when current new fields are absent. Recheck the record on HC metadata
  events and periodic reconciles; if it is removed after a prior `True`,
  report `RecoverySnapshotPending` until it is rewritten. No new API object
  or RBAC grant is needed because the operator already updates HC metadata.
* Add an `AutoscalerConfigurationApplied` condition to HostedControlPlane and
  HostedCluster. New CPO sets the HCP condition with
  `observedGeneration: HostedControlPlane.metadata.generation`. It is `True`
  only after comparing the autoscaling-related flags in the live autoscaler
  Deployment with the HCP spec, finding every requested value exactly once
  (or once per GPU type), including the single effective `--v` argument,
  and observing a completed rollout with updated
  replicas available. Missing, duplicate, or conflicting arguments report
  `False` with a mismatch reason. Derive expected values from the
  field-to-flag mapping and read the live container arguments; do not treat
  previously rendered arguments as proof of application. A Deployment that
  is merely `Available` can still have old pods running, so availability
  alone does not make the condition `True`. Report `False` with a reason and
  message while the rollout is pending, the autoscaler is unavailable, or the
  component is disabled. New CPO recomputes this condition on every HCP
  reconciliation, including Deployment change events and periodic requeues,
  even when HCP generation has not changed. If arguments drift after a
  previous `True`, report `False`
  until CPO restores them and the rollout completes. This condition reports
  argument application; it does not assert that a configured limit will be
  enforced without the GPU acceptance test. With the management gate off,
  a current CPO removes the condition if no new field remains, or reports
  `False` with `FeatureGateDisabled` when a new field is retained by an old
  schema; it never reports `True` for gated values it ignored.
* When any new autoscaling field is requested with the management gate
  enabled, the HostedCluster controller copies the HCP condition into
  HostedCluster status only if the HCP's autoscaling spec matches the
  HostedCluster request and the HCP condition's
  `observedGeneration` equals the HCP generation, and the recovery annotation's
  latest revision matches the current HC fields it backs up. If the record is missing or
  mismatched, set `Unknown` with `RecoverySnapshotPending` even if the HCP condition is
  `True`. If writing the record fails for a nonretryable reason, use `False` with
  `RecoverySnapshotFailed`. If the HCP or condition is absent, or either
  freshness check fails, set the HC condition to `Unknown` with a
  message that the settings have not been confirmed as applied. Set the HC
  condition's `observedGeneration` to the HC generation after these checks.
  Remove the HC condition when all new fields are removed. The existing
  HCP-to-HC condition-copy loop needs this freshness check specifically for
  this condition; it currently overwrites the copied `observedGeneration`.
  A request containing only `logVerbosity` must create and update this
  condition; removing that sole request restores `--v=4` and removes the
  condition while retaining the recovery history. With the gate off,
  skip snapshot writes and report `FeatureGateDisabled` for retained new
  fields rather than `RecoverySnapshotPending`.
* HyperShift defines its own `ClusterAutoscaling` and `ScaleDownConfig` types.
  The older vendored `cluster-autoscaler-operator` types are currently imported
  by standalone test setup, not by the production HCP argument translator.
  Sync that dependency and vendor directory to the target OpenShift release
  so the standalone test setup has current API types, but do not make them
  the source of the HostedCluster API. Their update alone will not implement
  any HCP setting.
* The GPU acceptance test is a delivery dependency of
  [CNTRLPLANE-4429](https://redhat.atlassian.net/browse/CNTRLPLANE-4429).
  Require `NodePool.spec.nodeLabels["cluster-api/accelerator"]` to match the
  configured GPU `type`. Ensure the managed label reaches new CAPI Machines
  before the hosted Node controller's one-time sync. On AWS/Azure, extend
  the NodePool scale-from-zero path to retain the accelerator label in
  `capacity.cluster-autoscaler.kubernetes.io/labels` on
  MachineDeployments and MachineSets even when native `Status.Capacity` is
  present. Verify that the template exposes GPU extended-resource capacity;
  if native capacity or the current `machine.openshift.io/GPU` count does not
  supply it to the paired autoscaler, add its supported GPU type/count
  annotations or fix the provider. Deliver and test this path before
  documenting the GPU field as a functioning limit.
* Document the fields in
  `hypershift/docs/content/how-to/autoscaling.md` and regenerate
  `docs/content/reference/api.md`. Include omitted defaults, example values,
  the `logVerbosity` range, the hosted default of `4`, the meaning of an
  explicit `0`, the autoscaler-only scope, and the log-volume cost of high
  levels. Also include the one-hour new-pod delay maximum, its use of the Pod
  creation timestamp, per-Pod annotation precedence, the meaning of GPU `type` and its
  reserved `cpu` and `memory` values, GPU `max: 0` behavior, the required GPU
  NodePool label and its once-on-creation behavior, the template capacity
  prerequisite, the `AutoscalerConfigurationApplied` condition and its
  version-skew behavior, how configured scale-down policies become effective
  after switching from `ScaleUpOnly` to `ScaleUpAndScaleDown`, the distinction
  between cordoning and the deletion `NoSchedule` taint, guest-node drain
  before CAPI Machine scale-down, and the fact that a resource minimum
  constrains scale-down without causing scale-up,
  while a maximum constrains autoscaler scale-up rather than manual changes.
  Include the emergency operator-rollback procedure for clusters
  that still have new fields configured. Explain that the feature initially
  requires management-cluster `TechPreviewNoUpgrade`, how to select it at
  HyperShift install time, its Tech Preview support limits, and why setting the
  hosted cluster's own `spec.configuration.featureGate` does not enable these
  fields. Instruct administrators to read back new fields because a Default
  CRD may prune unknown keys. Document the annotation's owner, revision
  format, persistence after deliberate field removal, the 20-revision
  retention cap and its automatic pruning of the oldest entries, size-limit
  behavior, and explicit restoration steps.

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
| An older HyperShift operator or CRD schema drops configured fields during emergency rollback | Append a recovery revision in HC metadata before applying new settings and require it for HC `AutoscalerConfigurationApplied=True`. Test preservation through old typed-controller writes and old CRD schemas; restore fields explicitly from the last desired revision after reinstalling compatible code. Check live arguments because old conditions may be stale. |
| The recovery annotation cannot be written because HC annotations are near the Kubernetes size limit or the write conflicts | Retry conflicts. Bound GPU entries per revision and cap the history at the 20 most recent revisions so routine configuration changes cannot exhaust the annotation; report `RecoverySnapshotFailed` only when a persistent conflict occurs or a single revision cannot fit even after dropping all older ones, and test this failure before reporting application. |

### Drawbacks

HyperShift must maintain another set of public policy enums and a translation
table alongside the standalone API. Because the settings are per HostedCluster,
the CPO restarts that cluster's autoscaler when they change. GPU limits need
GPU-capable CI and might require an autoscaler image change if the acceptance
test reveals a provider or template gap. The recovery annotation adds a
metadata write before each new configuration can reach HCP and retains up
to the 20 most recent historical revisions, automatically dropping older
ones; an administrator can still remove the annotation entirely to clear
it sooner.

## Alternatives (Not Implemented)

* **Copy the standalone `ClusterAutoscaler` CR or install its operator in the
  guest cluster.** HyperShift already owns a different API and deploys the
  autoscaler directly with a management/guest dual-client setup. A second
  authority for autoscaler settings would create conflicting reconciliation.
* **Use new Boolean fields named exactly like the autoscaler flags.** This
  matches the ticket terminology but conflicts with OpenShift's API convention
  against new Boolean fields. The proposed enums also give each value an
  explicit user-facing meaning.
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

* **Feature gate:** Verify that `HostedClusterAutoscalerConfiguration` is
  disabled in both `Hypershift` and `SelfManagedHA` Default feature sets and
  enabled in their `TechPreviewNoUpgrade` sets. Compare generated HC and HCP
  CRDs: the Default variants omit all seven new settings while preserving
  existing autoscaling fields; the Tech Preview variants contain the fields
  and their validation. With the gate off, verify CPO keeps historical
  arguments even when an HCP read from an older gated schema still contains
  new values; it reports `FeatureGateDisabled`, never `True`, for that
  retained request. Verify the HC does not treat an old `True` condition as
  current. Run the behavioral end-to-end cases below in a management-cluster
  `TechPreviewNoUpgrade` job; keep a Default job that checks unchanged
  arguments and no new application condition for an omitted request.
* **API and unit:** Cover serialization of omitted fields, explicit
  `logVerbosity: 0`, explicit `min: 0`, and explicit GPU `max: 0` distinct from
  omission; test N-1/N+1 API deserialization. Extend `TestAdaptDeployment`
  with omitted settings, both values of every policy, mixed settings, `0s`,
  multiple GPU types, and `--gpu-total=nvidia-tesla-t4:0:0`; assert that each
  argument appears once. Verify omission yields one `--v=4`, explicit `0` and
  `10` yield `--v=0` and `--v=10` respectively, and the two other historical
  hardcoded defaults still appear after translation. Test CPO condition
  transitions for matching arguments, missing or duplicate flags, a pending
  or failed rollout, an unavailable autoscaler, and `--v` drift or duplication.
  Test HC condition propagation for absent, stale, and current HCP
  observations, including a request containing only `logVerbosity: 0`.
* **Recovery snapshot:** On a gated HC with new fields, verify the annotation
  stores a version, matching UID, and ordered revisions with source
  generations, the new fields, `scaling`, and the complete `scaleDown` block,
  including explicit zeros and multiple GPU types. Assert the HCP spec does
  not advance and HC never reports `True`
  until the latest revision is persisted and read back. Inject a
  resource-version conflict and a nonretryable
  annotation write failure; verify retry or `RecoverySnapshotFailed` and no
  new HCP arguments. Remove the annotation after HC reaches `True` and
  verify the HC condition becomes `RecoverySnapshotPending` until the
  operator recreates it. Change the request and verify a new revision is
  appended before HCP, then remove every new field and verify baseline
  arguments and removal of the HC application condition while the history
  remains. Simulate a partial schema rollback that removes one new field but
  leaves another; after reinstalling compatible code, verify the prior full
  revision survives even if a partial revision is appended. Verify no
  automatic replay. Restore only the missing fields from the selected prior
  revision, confirm admission and a fresh `True`, and confirm unrelated
  autoscaling fields are unchanged. If emergency protection removed an
  existing `scaleDown` block, restore its timings and cordon setting in the
  same update that returns `scaling` to `ScaleUpAndScaleDown`. Apply more
  than 20 sequential distinct configuration changes on one HostedCluster and
  verify the annotation retains only the most recent 20 revisions, every
  change still reaches HC `True`, and no change is ever rejected due to
  annotation size. Confirm the oldest revisions were dropped in order and
  that the revision used for emergency restoration is always the most
  recent one, not an earlier one that aged out of the cap.
* **Envtest:** Use the Tech Preview HostedCluster and HostedControlPlane CRDs to
  accept valid enums, `logVerbosity: 0` and `10`, ranges including GPU
  `{min: 0, max: 0}`, unique GPU types, and durations including `0s`, `60m`,
  and `1h`. Reject unknown enums, `logVerbosity: -1` and `11`, empty
  `resourceLimits`/`scaleUp`, missing range endpoints including GPU
  `max`, negative or inverted ranges, CPU and memory `max: 0`, duplicate GPU
  types, invalid label values, reserved GPU types `cpu` and `memory`, malformed
  or negative durations, durations above `1h` such as `1h1ns` and `8760h`, and
  `scaleDown` with `ScaleUpOnly`. For each CRD, reject both reserved types in
  GPU-only configurations and alongside configured `cores` and `memory`
  ranges, including `cores.max: 128` with a `type: cpu`, `max: 8` GPU entry.
  For both CRDs, accept 1 and 64 unique GPU types and reject an empty or
  65-entry list, verifying the generated `minItems` and `maxItems` constraints.
  Repeat with Default CRDs to verify new fields cannot be stored,
  whether the API server rejects them or prunes them. Do not claim admission
  rejected a field merely because it disappeared on read-back.
* **Log verbosity end to end:** On a hosted cluster, verify the omission
  baseline is one `--v=4` argument. Set `logVerbosity: 0`, wait for the
  autoscaler rollout and `AutoscalerConfigurationApplied=True`, and verify the
  running Pod has exactly one `--v=0` argument. Repeat at `6` and verify one
  `--v=6` argument. During a controlled scan, the
  [pinned image's V(4) `Starting main loop` message](https://github.com/openshift/kubernetes-autoscaler/blob/a819db459f3e0a88becba4c58bf053c6dbda28f2/cluster-autoscaler/core/static_autoscaler.go)
  must appear at level `6` and be absent from the new Pod's logs at level `0`.
  Remove the field and verify the rollout restores one `--v=4` argument and
  removes the HC application condition if no other new field is configured.
* **Local storage and delay end to end:** With at least two autoscaled
  NodePools, verify `localStorageScaleDown: Skip` prevents deletion of a node
  with a qualifying disk-backed local volume and `Consider` restores
  eligibility. With `newPodScaleUpDelay: 30s`, run each delay case from a known
  NodePool capacity baseline, with no other pending Pods and a test Pod that
  cannot fit existing Nodes but fits a NodePool template. Verify a newly
  created unschedulable Pod does not trigger CAPI growth before it is 30
  seconds old and does trigger growth on a subsequent autoscaler scan. Hold
  another Pod behind a scheduling gate until it is older than 30 seconds,
  then release it into the unschedulable set; verify no new 30-second wait
  begins at that transition. On a fresh Pod, set the
  `cluster-autoscaler.kubernetes.io/pod-scale-up-delay` annotation to `10s`.
  Verify the shorter override is ignored: no CAPI growth between Pod ages 10
  and 30 seconds, then growth on a scan after 30 seconds. Set the annotation
  to `60s` on another fresh Pod and verify no growth between ages 30 and 60
  seconds, then growth on a subsequent scan.
* **DaemonSet utilization end to end:** Choose DaemonSet requests that put an
  otherwise removable node above the scale-down utilization threshold with
  `Include` and below it with `Exclude`; keep non-DaemonSet requests below the
  threshold. Keep NodePool `autoScaling.min` below the current size, avoid
  local volume and eviction blockers, and wait past scale-down delays. Verify
  the same node is ineligible with `Include` and becomes eligible for removal
  with `Exclude`.
* **Similar NodePool balancing end to end:** Use two NodePools with matching
  effective capacity, labels, taints, and comparable allocatable and free
  resources, apart from topology labels already ignored by the autoscaler.
  Start them at unequal sizes with scale-up headroom, then create pending pods
  that can run in either pool. With `Balance`, verify from the scale-up decision
  and CAPI replica changes that it recognizes the pools as similar and directs
  growth toward the smaller pool. With `DoNotBalance`, verify scale-up still
  works without requiring the resulting sizes to diverge.
* **Cordoning end to end:** Start with schedulable guest Nodes carrying a
  movable workload Pod whose graceful termination briefly holds the drain
  open. Under both `Enabled` and `Disabled`, watch guest Nodes and Pods alongside
  their management-cluster CAPI Machines and scalable-resource replicas.
  Assert that `Enabled` sets `Node.spec.unschedulable` before pod eviction and
  before the Machine is marked for deletion or replicas decrease; `Disabled`
  leaves it false. Under both values, observe the autoscaler's deletion
  `NoSchedule` taint and verify that required workload Pods have left the node
  before the CAPI Machine is marked or replicas decrease. While the workload
  Pod is terminating, neither CAPI deletion step may begin. Do not require
  best-effort DaemonSet Pods to disappear or use new-pod placement on the
  terminating node as the negative assertion.
* **CPU and memory end to end:** Test CPU and memory maxima
  independently: a pending workload triggers growth below the maximum and
  cannot grow the cluster beyond it, regardless of which NodePool could
  satisfy it. Test `cores.min` and `memory.min` independently, with scale-down
  enabled, no pending workloads, otherwise removable nodes, nonbinding limits
  on other resources, and NodePool `autoScaling.min` low enough to permit one
  more removal. Choose node capacities and floors so scale-down can reach each
  minimum exactly. Verify that it removes nodes down to the floor but retains
  the next otherwise eligible node because removing it would cross the floor.
  Lower or remove that resource minimum without changing other constraints and
  verify the same node can then be removed. With a valid resource maximum and
  NodePool scale-up headroom, setting either minimum above current capacity
  while no workloads are pending must not cause scale-up. Verify removal of
  fields restores baseline arguments.
* **GPU end to end:** Create a GPU-capable CAPI NodePool with
  `spec.nodeLabels["cluster-api/accelerator"]` set before any Nodes join.
  Verify the matching label on guest Nodes immediately after their first
  label sync and in the generated CAPI scale-from-zero template, and verify
  GPU extended-resource capacity on both. Cover native `Status.Capacity` and
  fallback annotation paths in NodePool controller tests. Show scale-up below
  the GPU maximum and rejection at the maximum, including scale-from-zero if
  that platform supports it. With otherwise removable GPU nodes, nonbinding
  CPU and memory limits, and NodePool `autoScaling.min` low enough to permit
  another removal, verify scale-down reaches `gpus[].min` and retains the next
  GPU node whose removal would cross it.
  Lower or remove that GPU minimum and verify the same node can then be
  removed. With a valid GPU maximum and NodePool scale-up headroom, raising
  the GPU minimum above current capacity with no pending workloads must not
  cause scale-up. Configure `{type: nvidia-tesla-t4, min: 0, max: 0}` with
  NodePool scale-up headroom and a pending GPU workload; verify no growth of
  that accelerator type. Raise `max` above zero without changing the NodePool
  and verify the workload can then trigger growth. This test is required for
  GPU delivery; checking the argument alone is insufficient.
* **Upgrade:** With the Default management feature set, upgrade an existing
  hosted cluster with no new fields and compare effective arguments before
  and after. In a controlled Tech Preview compatibility test, install the new
  HyperShift operator and gated CRDs while the old CPO still runs, then
  configure
  `localStorageScaleDown: Skip`, `logVerbosity: 0`, and
  `resourceLimits.cores: {min: 0, max: 64}`. Assert that all three fields are
  stored on HC and HCP, the old CPO keeps `--v=4` and leaves its HCP condition
  absent, and the HC condition is `Unknown` even though the old autoscaler
  Deployment is `Available`. Upgrade the hosted control-plane payload without
  editing HC again. Assert that the new CPO renders
  `--skip-nodes-with-local-storage=true`, `--v=0`, and
  `--cores-total=0:64`, reports `False` during rollout, and reports `True`
  only after all updated replicas are available. Verify the configured
  local-storage behavior. Change the HC spec again and verify that neither a
  stale HCP condition nor a pending HCP spec copy is reported as applied.
  Clear all new fields and verify that
  `--v=4` returns and the HC condition is removed. After reaching `True`,
  alter the live Deployment arguments without changing HCP spec; verify the
  condition becomes `False`, then returns to `True` only after CPO repairs the
  Deployment and its rollout completes.
* **Operator rollback with configured fields:** Roll back only the HyperShift
  operator binary in the gated compatibility test while leaving the new HC
  and HCP CRDs installed; repeat with the older CRD schemas reapplied. Do not
  remove the new fields first. Start only after a matching recovery annotation
  is persisted and HC `AutoscalerConfigurationApplied=True`.
  Exercise both a normal reconcile and full-object writes to HC and HCP by
  the old typed controller. Use unstructured API reads to compare HC and HCP
  specs before rollback, immediately after old schemas are applied, and after
  each write; also compare the CPO image, autoscaler Deployment arguments,
  both application conditions, and the HC recovery annotation. Assert that
  old typed-controller full-object writes preserve that annotation and the
  old CRD schema does not prune it. Assert that the old operator does not
  crash, full HC and HCP updates through its old types lose the new fields,
  and a write under an old CRD schema prunes them. Verify that the
  release-selected new CPO then renders historical arguments, including
  `--v=4`, for omitted fields. Separately, run an old CPO against an HCP still
  carrying new fields and verify that it ignores them, retains historical
  arguments including `--v=4`, and does not crash. Do not treat a retained
  `True` condition from the newer operator as proof after
  rollback. Set the existing `scaling: ScaleUpOnly` policy, removing any
  `scaleDown` block
  first, and verify `--scale-down-enabled=false` as emergency protection
  when `Skip` can no longer be confirmed. Restore the compatible operator
  and CRDs, explicitly patch missing new fields from the annotation, and
  verify the desired flags and current `True` condition return. Also test a
  planned rollback after first
  removing the new fields: leave the historical annotation in place, verify
  baseline arguments, and do not automatically restore its contents.

## Graduation Criteria

### Dev Preview -> Tech Preview

No separate Dev Preview phase is planned. The first release exposes the new
fields only through the `HostedClusterAutoscalerConfiguration` management
feature gate in `TechPreviewNoUpgrade`; `Default` omits them. Entry into Tech
Preview requires API approval, generated gated HC and HCP schemas with
admission tests, a CPO/autoscaler payload that recognizes every emitted flag,
the persisted recovery snapshot and `AutoscalerConfigurationApplied` status
checks, and documentation that
explains how to enable the gate and confirms its support limits. The GPU
field also requires the CAPI label/template changes and a passing end-to-end
enforcement test before the gate ships; this single gate does not expose a
GPU field whose behavior is unverified. A periodic gate-enabled CI job runs
the behavioral suite, while a Default job checks omission and historical
arguments. Existing `spec.autoscaling` remains available in both sets.

### Tech Preview -> GA

Promote `HostedClusterAutoscalerConfiguration` to `Default` only after the
gated API and behavior tests have run reliably across supported platforms,
the default and gated CRD variants and API compatibility checks pass, and
the full multi-NodePool and GPU end-to-end tests prove both upper and lower
resource limits. Verify supported CPO/autoscaler image combinations in every
advertised hosted-cluster stream. Complete the upgrade test for omitted and
configured settings, including old-CPO and rollout condition states, and
the operator-rollback and snapshot-restoration tests with and without CRD
rollback. Publish the
HyperShift documentation and validate day-2 changes and failure handling.
Then enable the gate in the `Default` manifests and runtime registries,
regenerate CRDs, and verify the formerly gated fields are present by default
without changing omission behavior. Until this promotion, there is no
default availability or partial GPU parity claim.

### Removing a deprecated feature

This enhancement removes no API field or deprecated feature. Moving the three
hardcoded arguments from the template to generated arguments preserves their
effective values.

## Upgrade / Downgrade Strategy

On the initial Tech Preview release, the new CRD fields are installed only
when the management-cluster HyperShift feature set is
`TechPreviewNoUpgrade`; Default schemas continue to omit them. Existing
HostedClusters omit the new fields; CPO still produces
`--skip-nodes-with-local-storage=false`, `--balance-similar-node-groups=true`,
and one `--v=4`. There is no data migration or change to NodePool bounds. A
configured setting takes effect only with the gate enabled, after the
HostedCluster recovery revision is persisted, the HostedControlPlane copy is
current, and a compatible CPO has reconciled the autoscaler Deployment. HC
reports application only after all three are confirmed. The gated
mixed-version rollout in the test plan is a controlled compatibility test;
it does not promise a supported upgrade path for
`TechPreviewNoUpgrade`. At GA promotion, the new fields enter the Default
schemas and the same test covers a normal supported upgrade.

For a planned HyperShift operator rollback to a version without this API,
remove the new fields from HostedCluster and wait for baseline autoscaler
arguments to reconcile before changing the operator binary. An emergency
rollback may happen while these fields are still configured. Rolling back
only the operator binary leaves the installed CRD schemas in place, but the
older operator's typed API does not contain the new fields. Its existing
HostedCluster-to-HostedControlPlane struct copy cannot carry them, and a
full-object update through the old types can remove them from HC or HCP.
If older CRD schemas are also installed, the new fields become unknown to
those schemas and can be pruned on subsequent writes under
[Kubernetes CRD field-pruning rules](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#field-pruning).
Neither form of emergency rollback guarantees that new values remain in the
spec or in the effective autoscaler arguments. The HC metadata annotation
retains revisions for requests that reached
`AutoscalerConfigurationApplied=True` across both rollback forms, provided
the HC object itself and its metadata remain intact; the rollback test must
verify that older typed-controller writes preserve it.

An unchanged hosted control-plane release normally keeps its release-selected
CPO and autoscaler images when the HyperShift operator binary is rolled back.
If the HCP fields disappear, that CPO sees omitted settings and can render
the historical defaults, including `--skip-nodes-with-local-storage=false`
and `--v=4`; an older CPO ignores the new fields. A previous
`AutoscalerConfigurationApplied=True` on HC may be stale while an older
operator runs because it cannot recompute that condition. The emergency
procedure checks the installed CRD schemas, CPO image, raw HC and HCP specs,
HC recovery annotation, and live Deployment arguments. If
`localStorageScaleDown: Skip` cannot be confirmed and scale-down must stop,
set the already-supported `scaling: ScaleUpOnly` policy, remove any
`scaleDown` block as required by validation, and verify
`--scale-down-enabled=false` in the Deployment. After restoring a compatible
operator and CRDs, the administrator validates the record's version and UID,
selects the last desired pre-rollback revision, and explicitly patches its
missing `settings` fields into HC. If the safety switch was used, restore
the selected revision's complete `scaleDown` block and new fields with
`scaling: ScaleUpAndScaleDown` in the same HC update. Beyond the ordinary
20-revision cap, the controller must not also erase the history merely
because the pruned spec looks empty, and must not automatically replay it:
retained revisions may represent intentionally removed settings. A request
without a matching revision that never reached HC `True`, or that has aged
out of the cap, requires another saved copy for
recovery. Control-plane release downgrades remain unsupported by HyperShift's
existing
[upgrade policy](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/upgrades.md).

Reinstalling with the Default feature set while configured fields still
exist is also a schema change: the Default HC and HCP CRDs omit them, and
subsequent writes can prune them. A current CPO running with the gate off
must ignore any surviving fields and use historical arguments; if they
remain visible, it reports `FeatureGateDisabled`. Verify the annotation and
live arguments, then restore a compatible gated or GA installation before
explicitly reapplying lost values from the selected revision. The management
feature-set transition is covered as a compatibility test, not as a supported
Tech Preview upgrade or downgrade procedure.

## Version Skew Strategy

The HyperShift operator, CPO, autoscaler image, and guest cluster can move at
different times. The HostedCluster controller's existing spec copy needs no
new translator, but an older CPO cannot act on new autoscaling fields. The
feature is available initially only when the management-cluster
`HostedClusterAutoscalerConfiguration` gate is enabled and the hosted-control-
plane payload stream contains both the CPO argument translator and an
autoscaler image with the required flags; GPU additionally needs a passing
enforcement test. Support is determined per advertised release stream and
verified by CI, rather than inferred from a newer management-cluster operator
or a guest-cluster feature set alone.

During mixed-version rollout, old CPO behavior remains in force until the new
CPO reconciles. The three historical arguments stay effective either way;
in particular, an older CPO keeps `--v=4` even if `logVerbosity` is stored in
HC and HCP. The `--v` flag already exists on the paired autoscaler image;
version skew for this field is in CPO translation. New CPO emits only flags
known to its paired release image. Documentation and support procedures
direct administrators to check the
`AutoscalerConfigurationApplied` entry in `HostedCluster.status.conditions`
before treating a requested value as effective. On initial rollout, an old
CPO does not set the HCP condition, so the HC reports `Unknown` instead of
implying that a copied spec field has taken effect. `False` means the new CPO
has seen the request but has not confirmed a completed rollout, or the HC
controller could not persist its recovery revision; `True` means the latest
revision matches the requested fields and the arguments are present in the
rolled-out autoscaler Deployment.
The condition is observational: while it is not `True`, an old autoscaler
can still scale down local-storage nodes according to the historical `false`
default.
Administrators should use this setting only with a supported control-plane
payload and wait for `True` before relying on it for local-storage protection.
After rollback to older operator or CPO binaries, a previous `True` may
persist without being recomputed; the rollback procedure checks the live
Deployment arguments directly.
After compatible code returns, a new `True` confirms only the currently
stored HC spec. If an older schema pruned some fields, compare that spec
with the pre-rollback recovery revision before treating it as the intended
configuration.

If a current CPO sees retained new HCP fields while the management gate is
off, it renders historical arguments and reports `False` with
`FeatureGateDisabled`. A Default CRD can instead prune the fields, leaving no
request to report; check the stored HC and HCP specs as well as the feature
set before relying on status.

No new kubelet, Machine, or guest operator API is required.

## Operational Aspects of API Extensions

This extends the existing HostedCluster and HostedControlPlane CRDs. It adds
no new CRD, admission/conversion webhook, finalizer, API server, or network
dependency. When the gate is enabled, invalid configurations fail admission
with field-specific enum, range, list, or duration messages before they can
roll the autoscaler. When it is disabled, the Default CRD variants omit the
fields and Kubernetes may prune unknown keys instead of rejecting a request.
The small spec fields add negligible API overhead. A configured high verbosity
level can increase autoscaler log volume and processing overhead. The HC
metadata recovery record adds one annotation and a metadata write before
new settings are mirrored to HCP. A failed write keeps the request
unconfirmed and prevents it from being applied.

When any new autoscaling field is requested, the
`AutoscalerConfigurationApplied` entry in HCP and HC `status.conditions`
provides a programmatic application signal:

| Observation | HCP condition | HC condition |
| --- | --- | --- |
| Old CPO does not report the condition, or the HCP observation is stale | Absent or stale | `Unknown` |
| Recovery annotation missing or stale; persistent write failure | Any | `Unknown` with `RecoverySnapshotPending`, or `False` with `RecoverySnapshotFailed` |
| Gate disabled while a new request remains in an older gated schema | `False` with `FeatureGateDisabled` from a current CPO | `False` once the HCP observation is current |
| Requested flags are missing, duplicated, or different; rollout is pending; or the autoscaler is unavailable | `False` | `False` once the HCP observation is current |
| Live flags match the request and the updated Deployment rollout is complete | `True` | `True` once the HCP spec and latest recovery revision match the HC request |

CPO rechecks the live Deployment after changes, so a later drift changes a
previous `True` to `False` until repaired. Current HC `True` confirms that the
configuration can be recovered from its metadata revision and that arguments
were applied; a valid resource maximum can still block growth as configured.
After rollback
to an operator or CPO that predates this condition, a retained `True` can be
stale on HC or HCP; verify the HCP spec and Deployment arguments directly.

If an unsupported flag reaches an autoscaler image, its container can fail
to start; `AutoscalerConfigurationApplied=False`, the cluster-autoscaler
Deployment's `Available` condition, and CPO component availability reporting
reveal that failure. Pending pods may then remain unscheduled because
autoscaler-driven NodePool growth stops; existing nodes and running workloads
remain. A valid but overly tight limit leaves the Deployment healthy while
scale-up is blocked. Autoscaler logs, guest pod events, and the
`kube-system/cluster-autoscaler-status`
ConfigMap distinguish this from an unhealthy Deployment. The HyperShift team
owns API/argument translation and CPO availability; the autoscaler/CAPI
provider maintainers own resource accounting failures.

## Support Procedures

1. Run `oc get hostedcluster NAME -n NAMESPACE -o yaml` and read the
   stored `spec.autoscaling` and the `AutoscalerConfigurationApplied` entry in
   `status.conditions` after requesting a new setting. Confirm the HyperShift
   management cluster was installed with `--tech-preview-no-upgrade` while
   this feature is in Tech Preview; the hosted cluster's
   `spec.configuration.featureGate` does not enable it. If the field is
   absent on read-back, check the installed CRD variant because a Default
   schema may have pruned it. `True` at the current HC generation means
   the latest recovery revision matches the currently stored request and its
   arguments reached the
   rolled-out autoscaler; `Unknown`
   means application is unconfirmed, including when an old CPO cannot report
   the condition. `False` can also mean the gate is disabled or the CPO has
   not completed application; read its reason. Do not rely on
   `localStorageScaleDown: Skip` until this condition is `True` under
   a compatible HyperShift operator and CPO.
2. If the condition is not `True`, compare `HostedCluster.spec.autoscaling`
   with the mirrored `HostedControlPlane.spec.autoscaling`, inspect the HCP
   condition and its `observedGeneration`, and inspect the autoscaler
   Deployment's arguments and rollout in the hosted-control-plane namespace.
   An omitted local-storage or balancing field must still yield `false` or
   `true` respectively; omitted `logVerbosity` must yield one `--v=4`.
   Other omitted new fields yield no new flag. If verbosity was changed for
   diagnosis, remove the field to restore `--v=4` and normal log volume.
3. Check the Deployment's `Available` condition and CPO component condition.
   For a failed container, inspect its logs for an unknown flag or parsing
   error and confirm that its image belongs to a supported payload stream.
4. For a healthy autoscaler that does not scale, inspect its logs, guest pod
   events, `kube-system/cluster-autoscaler-status`, NodePool min/max values,
   and aggregate guest-node CPU, memory, and GPU capacity. For GPU limits,
   compare `resourceLimits.gpus[].type` with the GPU NodePool's
   `spec.nodeLabels["cluster-api/accelerator"]` and current guest Node labels.
   Check the effective scale-from-zero accelerator label on the CAPI template
   (`capacity.cluster-autoscaler.kubernetes.io/labels` on AWS/Azure) and GPU
   extended-resource capacity from infrastructure `Status.Capacity` or GPU
   type/count annotations.
5. To restore pre-enhancement behavior, remove the new HostedCluster fields
   and wait for the autoscaler Deployment to reconcile. This changes
   autoscaler decisions prospectively; it does not delete existing nodes or
   override NodePool bounds. The recovery annotation remains as historical
   data; an administrator can remove it after confirming the baseline if
   that history is no longer needed.
6. After an emergency HyperShift operator rollback with new fields still
   configured, inspect the raw HC and HCP specs, the HC
   `hypershift.openshift.io/autoscaler-configuration-recovery` annotation,
   installed CRD schemas, CPO image, and live autoscaler arguments. A retained
   HC condition may be stale.
   If `Skip` is unconfirmed, use `scaling: ScaleUpOnly` to stop scale-down,
   remove any `scaleDown` block, and verify its flag. Restore a compatible
   gated or GA operator and CRDs. Check the record's version and HC UID,
   select the last desired pre-rollback revision (the annotation retains the
   20 most recent revisions, so the pre-rollback revision is present unless
   more than 20 later changes already occurred), then explicitly patch its
   `settings` values into HC without replacing unrelated autoscaling fields.
   If `ScaleUpOnly` was set as a safety measure, restore the saved complete
   `scaleDown` block and new fields with `scaling: ScaleUpAndScaleDown` in the
   same HC update.
   Wait for a current `True` condition and verify live arguments. If the
   record is absent because the request never reached `True`, or because it
   aged out of the 20-revision cap, use an external saved copy if one exists.

The user-facing [HyperShift autoscaling guide](https://github.com/openshift/hypershift/blob/main/docs/content/how-to/autoscaling.md)
will include these steps and examples. Support should start with the
HostedCluster condition and inspect effective Deployment arguments when it
is not `True` during forward mixed-version upgrades. After an operator
rollback, inspect the arguments regardless of the retained condition value.

## Infrastructure Needed 

The existing HyperShift envtest harness and two-NodePool autoscaling test can
host the API, translation, CPU/memory, and node-policy cases. GPU graduation
additionally needs a CI lane with a GPU-capable CAPI NodePool and the target
payload image.
