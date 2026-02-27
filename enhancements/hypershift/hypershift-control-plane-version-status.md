---
title: hypershift-control-plane-version-status
authors:
  - "@enxebre"
reviewers:
  - "@ahmed"
  - "@csrwng"
  - "@deads2k"
  - "@mmazur"
  - "@cbusse"
approvers:
  - "@csrwng"
api-approvers:
  - "@joelspeed"
creation-date: 2026-02-27
last-updated: 2026-02-27
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1751
see-also:
  - https://github.com/openshift/hypershift/pull/6300
---

# Control Plane Version Status: Decoupled Upgrade State for Management-Side Components

## Summary

This enhancement adds a new `controlPlaneVersion` field to `HostedClusterStatus` and `HostedControlPlaneStatus` that tracks the version history of management-side control plane components independently from the existing `version` field. The existing `version` field reflects the state of components (as reported by CVO). This doesn't cover the CPO itself and the components it manages manamegement side.
The new `controlPlaneVersion` field reports exclusively on management-side components, providing a clear signal for:
This was initially captured via https://issues.redhat.com/browse/OCPSTRAT-1751.

1. Whether all management-side components are running a specific version (e.g. to confirm a CVE fix has been applied).
2. What versions are currently active on the management side during an upgrade (to determine allowed NodePool version skew).
3. Whether a control plane upgrade has completed regardless of data-plane rollout progress.

## Glossary

- **Management-side components**: Control plane pods running in the HCP namespace on the management cluster (kube-apiserver, etcd, kube-controller-manager, kube-scheduler, openshift-apiserver, etc.). These are represented by `ControlPlaneComponent` custom resources.
- **Data-plane components**: Operators and workloads running on the guest cluster worker nodes (e.g. OVN pods, ingress controller, image registry). Their rollout depends on NodePool compute availability.
- **CVO (ClusterVersion Operator)**: Reports the version status of release components running in the data plane. This is what `HostedClusterStatus.Version` currently reflects.
- **CPO (Control Plane Operator)**: The operator running on the management cluster that reconciles the HostedControlPlane and manages control plane component deployments in the HCP namespace.
- **HCP**: HostedControlPlane custom resource.
- **HC**: HostedCluster custom resource.

## Motivation

### User Stories

- As a **service provider (ROSA/ARO)**, I want to know when all management-side control plane components have reached a target version so that I can confirm a CVE patch has been applied without waiting for data-plane rollout.
- As a **service provider**, I want to track control plane upgrade completion independently from data-plane rollout so that I can mark y-stream end-of-support or z-stream forced upgrades as done in Cluster Service.
- As a **service provider**, I want to upgrade a HostedCluster control plane even when no NodePools exist or all are scaled to zero, and get a clear completion signal.
- As a **cluster administrator**, I want to see which control plane versions are currently active (including during failed or multi-step upgrades) so that I can understand the allowed NodePool version skew.
- As an **SRE**, I want fleet-wide visibility into which clusters have completed their management-side upgrade and which are stalled, so that I can prioritize intervention.

### Goals

- Provide a dedicated API field (`controlPlaneVersion`) on `HostedClusterStatus` and `HostedControlPlaneStatus` that reports management-side component version state independently from CVO.
- Include version history with timestamps and completion state, enabling consumers to determine all currently active management-side versions.
- Enable NodePool version skew computation by exposing the full set of in-flight and completed control plane versions.
- Reuse existing clusterVersion semantics and types when possible for consistency with the CVO-based `version` field.

### Non-Goals

- Ensure the control plane version can reach `Completed` even when there are no NodePools or no data-plane compute available. This is orthogonal and subject to each component behaviour. This proposal focuses on signalling that state.
- Replacing or modifying the existing CVO-based `version` field on `HostedClusterStatus`. The two fields serve different consumers and must coexist.
- Tracking data-plane component versions. This field covers only management-side components represented by `ControlPlaneComponent` resources.
- Providing available or conditional update recommendations for the control plane. Update recommendations are already surfaced through the existing `version` field via CVO.

### Current State

Today, `HostedClusterStatus.Version` (type `ClusterVersionStatus`) bubbles up the CVO-reported `ClusterVersion`. This version reaches `Completed` state only when components managed by the CVO running on the dataplane have fnisihed rolling out to the target version. This creates several problems:

1. **Upgrade state is conflated**: A service provider cannot distinguish between "management components are at version X but data-plane rollout is pending" and "management components themselves have not finished upgrading."
2. **CVE verification is blocked**: To confirm that management-side components are not vulnerable to a CVE fixed in version X, operators must wait for the entire cluster (including data plane) to report version X as `Completed`.
3. **No-compute clusters are stuck**: When a HostedCluster has zero NodePools or all NodePools are scaled to zero, data-plane operators can never complete rollout. The CVO-reported version stays `Partial` indefinitely, providing no signal about the management side.
4. **Fleet management decisions are imprecise**: Service providers (ROSA, ARO) that manage y-stream end-of-support upgrades and z-stream CVE patches need to know when the management side is done, not when the entire cluster is done.
5. **NodePool version skew decisions require history**: To determine which NodePool versions are allowed, the system must know all versions that are currently active on the management side. A simple boolean condition or single version field is insufficient when upgrades fail or are in progress across multiple versions (e.g. 4.19.6 -> 4.19.19 (failed) -> 4.20.1 (in progress) requires knowing that 4.19 and 4.20 are both active).

A `ClusterVersionStatus`-style field with update history addresses all of these needs, which is what this enhancement proposes.

### Why Not a Condition?

[PR #6300](https://github.com/openshift/hypershift/pull/6300) introduced a `ControlPlaneUpToDate` condition that reports `True`/`False` based on whether all `ControlPlaneComponent` versions match the desired release. While useful as a quick boolean signal, a condition cannot express:

- **Version history**: Which versions have been applied, when they started, when they completed, and whether they succeeded or failed. This history is required to compute allowed NodePool version skew.
- **Current version during rollout**: During an upgrade from 4.19 to 4.20, the condition is simply `False`. It does not tell you that some components are at 4.19 and others at 4.20.
- **Multiple concurrent versions**: In failure scenarios (e.g. 4.19 -> 4.20 failed, now upgrading to 4.21), there may be 3 versions active simultaneously.


## Proposal

### Workflow Description

This feature is transparent to users — no manual workflow is required. The `controlPlaneVersion` field is automatically populated and updated by the HostedControlPlane controller (CPO) as management-side components roll out.

**Service provider (ROSA/ARO)** consumes the field to make upgrade orchestration decisions:

1. The service provider initiates a HostedCluster upgrade by updating the release image in the HostedCluster spec.
2. The HyperShift operator propagates the desired release to the HostedControlPlane spec.
3. The CPO begins rolling out management-side components (deployments in the HCP namespace).
4. On each reconciliation, the CPO updates `controlPlaneVersion.history[0]` with the current rollout state (`Partial` while in progress).
5. When all `ControlPlaneComponent` resources report `RolloutComplete=True` at the desired version, the CPO sets `history[0].State = Completed`.
6. The HyperShift operator copies `controlPlaneVersion` from HCP status to HC status.
7. The service provider observes `controlPlaneVersion.history[0].State == Completed` and proceeds with post-upgrade actions (e.g. marking the upgrade as done, confirming CVE patches applied).

**Cluster administrator** can inspect `controlPlaneVersion` on the HostedCluster to understand which management-side versions are active, independently from the CVO-reported `version` field.

### API Extensions

This enhancement modifies the `HostedClusterStatus` and `HostedControlPlaneStatus` CRDs (owned by the HyperShift team) by adding a new `controlPlaneVersion` field. No new CRDs, webhooks, aggregated API servers, or finalizers are introduced. The new field is purely informational (status-only) and does not affect the behavior of existing resources.

See the [API Changes](#api-changes) section below for the detailed type definitions.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specific to HyperShift / Hosted Control Planes. The `controlPlaneVersion` field is added to `HostedClusterStatus` and `HostedControlPlaneStatus`, which are HyperShift-specific resources. The reconciliation logic runs in the CPO on the management cluster and inspects `ControlPlaneComponent` resources in the HCP namespace.

#### Standalone Clusters

This change is not relevant for standalone clusters. Standalone clusters use the CVO-reported `ClusterVersion` for version tracking, and management-side / data-plane components are co-located on the same cluster. There is no split between management and data-plane version state.

#### Single-node Deployments or MicroShift

This proposal does not affect single-node deployments or MicroShift. It only applies to HyperShift-managed HostedClusters.

#### OpenShift Kubernetes Engine

This proposal does not depend on features excluded from OKE. The `controlPlaneVersion` field is part of the HyperShift API surface and is relevant wherever HyperShift is used. OKE standalone clusters are not affected.

### API Changes

Add a new field `controlPlaneVersion` to both `HostedClusterStatus` and `HostedControlPlaneStatus`, using a new `ControlPlaneVersionStatus` type. This type is modeled after the existing `ClusterVersionStatus` but omits the `AvailableUpdates` and `ConditionalUpdates` fields, which are not exercised for management-side version tracking for now (available updates are already surfaced through the existing `version` field via CVO):

```go
type HostedClusterStatus struct {
    // ... existing fields ...

    // version is the status of the release version applied to the HostedCluster.
    // This reflects the state of components running in the data plane as reported by CVO.
    // +optional
    Version *ClusterVersionStatus `json:"version,omitempty"`

    // controlPlaneVersion is the status of the release version applied exclusively to management-side control plane
    // components. Unlike version (which reflects CVO state including data-plane components), this field tracks only
    // components running in the HCP namespace on the management cluster.
    // +optional
    ControlPlaneVersion *ControlPlaneVersionStatus `json:"controlPlaneVersion,omitempty"`
}

type HostedControlPlaneStatus struct {
    // ... existing fields ...

    // controlPlaneVersion is the status of the release version applied exclusively to management-side control plane components.
    // +optional
    ControlPlaneVersion *ControlPlaneVersionStatus `json:"controlPlaneVersion,omitempty"`
}
```

The `ControlPlaneVersionStatus` type:

```go
// ControlPlaneVersionStatus reports the version state of management-side control plane components.
// It is modeled after ClusterVersionStatus but omits AvailableUpdates and ConditionalUpdates, which are
// not applicable to management-side version tracking.
type ControlPlaneVersionStatus struct {
    // desired is the release version that the control plane is reconciling towards.
    // This is the version from the HostedControlPlane spec.
    // +required
    Desired configv1.Release `json:"desired"`

    // history contains a list of versions applied to management-side control plane components. The newest entry is
    // first in the list. Entries have state Completed when all ControlPlaneComponent resources report the target
    // version with RolloutComplete=True. Entries have state Partial when the rollout is in progress or has failed.
    // +optional
    // +kubebuilder:validation:MaxItems=100
    History []configv1.UpdateHistory `json:"history,omitempty"`

    // observedGeneration reports which generation of the HCP spec is being synced.
    // +required
    ObservedGeneration int64 `json:"observedGeneration"`
}
```

Each `configv1.UpdateHistory` entry provides:

```go
type UpdateHistory struct {
    State          UpdateState  // "Completed" or "Partial"
    StartedTime    metav1.Time
    CompletionTime *metav1.Time // nil while in progress
    Version        string       // e.g. "4.20.1"
    Image          string       // release image pullspec
}
```

### Semantics

**`ControlPlaneVersion.Desired`**: Set to the release image and version from `HostedControlPlane.Spec.ReleaseImage` or`HostedControlPlane.Spec.ControlPlaneReleaseImage` if set. This is the version the control plane is reconciling towards.

**`ControlPlaneVersion.History`**: Ordered list (newest first) of version transitions for management-side components:

| Field | Value |
|-------|-------|
| `State` | `Completed` when **all** `ControlPlaneComponent` resources in the HCP namespace report `Status.Version == target version` AND `RolloutComplete` condition is `True`. `Partial` otherwise. |
| `StartedTime` | Timestamp when the upgrade to this version began (i.e. when the HCP spec was updated to this release image). |
| `CompletionTime` | Timestamp when all management-side components reached the target version. `nil` while in progress. For non-current entries, set to the `StartedTime` of the next entry. |
| `Version` | Semantic version string (e.g. `"4.20.1"`). |
| `Image` | Release image pullspec. |

**Transition rules**:

1. When the desired release changes (detected by comparing both version string and image against `history[0]`), a new `Partial` history entry is prepended. The previous entry's `CompletionTime` is set to the new entry's `StartedTime` (regardless of whether the previous entry completed). This ensures that image-only changes (e.g. release image rebuilds with the same semver) are not missed, consistent with CVO's `mergeEqualVersions()` semantics.
2. On each reconciliation, the controller checks all `ControlPlaneComponent` resources. If all report the desired version with `RolloutComplete=True`, the current (first) history entry transitions from `Partial` to `Completed` and `CompletionTime` is set.
3. History is capped at 100 entries. Oldest entries are pruned when the cap is exceeded.

### Reconciliation Logic

The reconciliation happens in the **HostedControlPlane controller** (`control-plane-operator`):

The desired release image is inferred via the existing function which considers API input for both Spec.ControlPlaneReleaseImage and Spec.ControlPlaneReleaseImage

```
func HCPControlPlaneReleaseImage(hcp *hyperv1.HostedControlPlane) string {
	if hcp.Spec.ControlPlaneReleaseImage != nil {
		return *hcp.Spec.ControlPlaneReleaseImage
	}
	return hcp.Spec.ReleaseImage
}
```

```
reconcileControlPlaneVersion(ctx, hcp, releaseImage):
  1. List all ControlPlaneComponent resources in hcp.Namespace.
  2. If listing fails, set observedGeneration and return error.
  3. Determine desired version and image from releaseImage.
  4. Initialize controlPlaneVersion if nil.
  5. If desired release differs from current history[0] (comparing both version AND image):
     a. Close out history[0] by setting CompletionTime = now.
     b. Prepend new entry: {State: Partial, Version: desired, Image: image,
        StartedTime: now, CompletionTime: nil}.
  6. Check all ControlPlaneComponent resources:
     a. For each component, compare Status.Version to desired version.
     b. For each component, check RolloutComplete condition.
  7. If ALL components match desired version AND have RolloutComplete=True:
     a. Set history[0].State = Completed.
     b. Set history[0].CompletionTime = now (if not already set).
  8. Prune history to 100 entries.
  9. Set observedGeneration = hcp.Generation.
```

Step 5 compares **both** the version string and the image to determine whether a new history entry should be prepended. This follows the same approach used by CVO in its [`mergeEqualVersions()`](https://github.com/openshift/cluster-version-operator/blob/master/pkg/cvo/status.go) function, which guards against image-only changes (e.g. release image rebuilds with the same semver) being silently ignored. The CVO logic treats two releases as equal only when their image matches (and the version is either empty or also matches), or their version matches (and the image is either empty or also matches). If the image changes but the version string stays the same, CVO correctly treats it as a new update and prepends a new history entry. The CPO reconciliation should follow the same semantics.

The **HostedCluster controller** (`hypershift-operator`) copies `controlPlaneVersion` from the HCP to the HC status, following the same pattern used for other HCP-to-HC status propagation (e.g. conditions, `version`).

### Example Status

**Steady state** (all management-side components at 4.20.1):

```yaml
status:
  controlPlaneVersion:
    desired:
      version: "4.20.1"
      image: "quay.io/openshift-release-dev/ocp-release:4.20.1-x86_64"
    observedGeneration: 3
    history:
    - state: Completed
      version: "4.20.1"
      image: "quay.io/openshift-release-dev/ocp-release:4.20.1-x86_64"
      startedTime: "2026-02-20T10:00:00Z"
      completionTime: "2026-02-20T10:15:00Z"
    - state: Completed
      version: "4.20.0"
      image: "quay.io/openshift-release-dev/ocp-release:4.20.0-x86_64"
      startedTime: "2026-02-10T08:00:00Z"
      completionTime: "2026-02-20T10:00:00Z"
  version:
    desired:
      version: "4.20.1"
      image: "quay.io/openshift-release-dev/ocp-release:4.20.1-x86_64"
    history:
    - state: Partial  # data-plane still rolling out
      version: "4.20.1"
      ...
```

**During upgrade** (management at 4.20.1, data-plane still at 4.20.0):

```yaml
status:
  controlPlaneVersion:
    desired:
      version: "4.20.1"
      image: "quay.io/openshift-release-dev/ocp-release:4.20.1-x86_64"
    history:
    - state: Completed  # management side done
      version: "4.20.1"
      ...
  version:
    desired:
      version: "4.20.1"
    history:
    - state: Partial    # CVO still waiting for data-plane
      version: "4.20.1"
      ...
```

**Failed upgrade with re-upgrade** (4.19.6 -> 4.19.19 failed, now on 4.20.1):

```yaml
status:
  controlPlaneVersion:
    desired:
      version: "4.20.1"
      image: "quay.io/openshift-release-dev/ocp-release:4.20.1-x86_64"
    history:
    - state: Partial
      version: "4.20.1"
      image: "quay.io/openshift-release-dev/ocp-release:4.20.1-x86_64"
      startedTime: "2026-02-25T14:00:00Z"
      completionTime: null
    - state: Partial  # never completed
      version: "4.19.19"
      image: "quay.io/openshift-release-dev/ocp-release:4.19.19-x86_64"
      startedTime: "2026-02-24T10:00:00Z"
      completionTime: "2026-02-25T14:00:00Z"
    - state: Completed
      version: "4.19.6"
      image: "quay.io/openshift-release-dev/ocp-release:4.19.6-x86_64"
      startedTime: "2026-02-01T08:00:00Z"
      completionTime: "2026-02-24T10:00:00Z"
```

In this scenario, a consumer can determine:
- **Minimum NodePool version**: Look at the oldest `Partial` entry (4.19.19) or the last `Completed` entry (4.19.6). Since 4.19.19 never completed, some components may still be at 4.19.6, so the effective minimum is 4.18 (n-1 of 4.19).
- **Maximum NodePool version**: The current desired version (4.20.1), so the max is 4.20.
- **CVE status**: Until `history[0].State == Completed`, you cannot confirm all management-side components are at 4.20.1.

### Consumer Use Cases

**Service providers (ROSA/ARO)**:
- Poll `controlPlaneVersion.history[0]` to determine when a control plane upgrade is complete, independently of data-plane rollout.
- Use `history[0].State == Completed` as the signal to proceed with subsequent operations (e.g. marking upgrade as done in OCM/ARO RP).
- Scan fleet clusters where `controlPlaneVersion.history[0].Version` is below a CVE-patched version to identify clusters needing forced upgrades.

**NodePool version skew computation**:
- Walk `history` to find all versions with `State == Partial` (still active on some components). The union of these versions plus the last `Completed` version determines the range of active control plane versions.
- Allowed NodePool versions are constrained by the n-1/n-2 skew policy relative to the lowest active control plane version (min) and the highest active version (max).

**Fleet metrics and dashboards**:
- Expose `controlPlaneVersion.history[0].version` and `controlPlaneVersion.history[0].state` as Prometheus metrics for fleet-wide version distribution dashboards.
- Alert on clusters where `history[0].State == Partial` for longer than a threshold.

### Implementation Details/Notes/Constraints

The implementation is contained within the existing CPO reconciliation loop. Key constraints:

- The `ControlPlaneComponent` CRD must expose `Status.Version` and a `RolloutComplete` condition. These fields already exist from previous work.
- History comparison uses both version string and image to detect changes, consistent with CVO's `mergeEqualVersions()` semantics.
- History is capped at 100 entries to bound status size.
- The HCP-to-HC status propagation follows the same pattern as existing fields (conditions, version).

### OVN Limitation

Until [OCPSTRAT-1454](https://issues.redhat.com/browse/OCPSTRAT-1454) is resolved, OVN components running on the data plane might block upgrade of OVN components running management side. This holds the controlPlaneVersion to complete if for any reason OVN data plane can not rollout.

### Risks and Mitigations

**Risk**: The new field adds API surface that consumers may depend on for upgrade orchestration decisions. If the reconciliation logic has bugs (e.g. prematurely marking `Completed`), it could cause incorrect NodePool version skew decisions.

**Mitigation**: The reconciliation logic is straightforward -- it iterates over `ControlPlaneComponent` resources and compares versions. Comprehensive unit tests cover all edge cases (version mismatch, partial rollout, no components, multiple failures). ControlPlaneComponent API give us a foundational layer and single point to address bugs.

**Risk**: Consumers may confuse `controlPlaneVersion` with `version` and make incorrect assumptions about the overall cluster state.

**Mitigation**: API field documentation clearly distinguishes the two fields. The naming convention (`controlPlaneVersion` vs. `version`) makes the scope difference explicit.

### Drawbacks

The main drawback is adding another version-related status field to an already complex API surface. Consumers must understand the distinction between `version` (CVO-reported, data-plane inclusive) and `controlPlaneVersion` (management-side only). However, this complexity is inherent to the HyperShift architecture where management and data-plane components are decoupled, and the benefit of independent version tracking outweighs the cost of an additional field.

## Test Plan

- **Unit tests**: Test the `reconcileControlPlaneVersion` function with:
  - All components at desired version with RolloutComplete=True -> Completed.
  - Version mismatch on one or more components -> Partial.
  - No ControlPlaneComponent resources found -> nil/empty history.
  - Version change (new desired) -> new Partial entry prepended, previous entry closed.
  - History pruning at 100 entries.
  - Failed upgrade followed by new upgrade -> correct history ordering.
- **E2E tests**:
  - Verify `controlPlaneVersion` is populated on a steady-state cluster.
  - Verify `controlPlaneVersion.history[0].State` transitions from `Partial` to `Completed` during an in-place z-stream upgrade.
  - Verify `controlPlaneVersion` is propagated from HCP to HC status.
  - Verify `controlPlaneVersion` reaches `Completed` even when NodePools are scaled to zero (i.e. data-plane cannot roll out).
  - Verify y-stream upgrade produces correct history entries.

## Graduation Criteria

### Dev Preview -> Tech Preview

- `controlPlaneVersion` field populated on all HostedClusters.
- History entries correctly track version transitions.
- Unit test and basic e2e coverage.

### Tech Preview -> GA

- E2E coverage of z-stream and y-stream upgrades with history validation.
- Validation with zero-NodePool clusters.
- Fleet-scale testing with many HostedClusters.
- OCM/ARO RP integration validated (consuming `controlPlaneVersion` for upgrade decisions).

### Removing a deprecated feature

Not applicable. This enhancement adds a new field; no existing features are being deprecated or removed.

## Upgrade / Downgrade Strategy

**Upgrade**: When a HostedCluster is upgraded to a version containing this feature, the controller initializes `controlPlaneVersion` with a single history entry reflecting the current state of `ControlPlaneComponent` resources. No manual action is required.

## Version Skew Strategy

The `controlPlaneVersion` field is a status-only addition with no behavioral impact on other components. During an upgrade of the CPO itself:

- An older CPO that does not know about `controlPlaneVersion` will simply not populate the field. Consumers must tolerate a nil `controlPlaneVersion`.
- A newer CPO will begin populating the field immediately. No coordination with other components is required.
- The field reuses existing `configv1.UpdateHistory` and `configv1.Release` types, so there are no new type version skew concerns.

## Operational Aspects of API Extensions

This enhancement adds a new status field (`controlPlaneVersion`) to existing CRDs (`HostedCluster` and `HostedControlPlane`). It does not introduce new CRDs, webhooks, aggregated API servers, or finalizers.

- **No impact on existing SLIs**: The field is populated during the existing CPO reconciliation loop. It adds a small amount of additional work (listing `ControlPlaneComponent` resources and comparing versions), which is negligible compared to the existing reconciliation.
- **No new failure modes**: If the reconciliation logic fails to update `controlPlaneVersion`, the field will be stale or nil. This does not affect cluster health or existing functionality. Consumers should treat a missing field as "unknown."
- **Monitoring**: The `controlPlaneVersion.history[0].state` and `controlPlaneVersion.history[0].version` can be exposed as Prometheus metrics for fleet dashboards and alerting.

## Support Procedures

- **Detecting issues**: If `controlPlaneVersion` is nil or stale on a HostedCluster that has been running for some time, check the CPO logs for errors related to `ControlPlaneComponent` listing or status updates. The CPO logs will contain entries related to `reconcileControlPlaneVersion`.
- **Stale `Partial` state**: If `controlPlaneVersion.history[0].State` remains `Partial` for an extended period, inspect individual `ControlPlaneComponent` resources in the HCP namespace to determine which components have not reached the desired version or do not have `RolloutComplete=True`.
- **Field not populated**: On older CPO versions that predate this feature, the field will be nil. This is expected and not an error.
- **Disabling**: The field cannot be independently disabled. It is part of the CPO reconciliation. If the field is causing issues, the CPO itself would need to be investigated.

## Alternatives (Not Implemented)

### Keep ControlPlaneUpToDate Condition Only

The `ControlPlaneUpToDate` condition introduced in [PR #6300](https://github.com/openshift/hypershift/pull/6300) provides a simple True/False signal indicating whether all management-side components match the desired version. While this is a useful boolean check, a `ClusterVersionStatus`-style field with update history is preferred because it additionally provides:
- Version history prefered for NodePool skew computation.
- Distinguishing which versions are active during multi-step upgrades.
- Providing timestamps for fleet SLA tracking.

### Add Multiple Conditions Instead of a Version Field

One could add conditions like `ControlPlaneUpToDate`, `ControlPlaneUpgradeInProgress`, `ControlPlaneVersionSkewed`, etc. This becomes unwieldy and still cannot express ordered history with timestamps. Conditions are best for boolean state, not versioned history.

### Extend the Existing version Field

Modifying `HostedClusterStatus.Version` to reflect only management-side components would break existing consumers that rely on it to represent the CVO-reported 
 state. The two concerns (management-only vs. combined) serve different users and must remain separate.
