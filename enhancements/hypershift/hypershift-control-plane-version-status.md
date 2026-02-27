---
title: control-plane-version-status
authors:
  - "@alberto-lamela"
reviewers:
  - "@enxebre"
  - "@csrwng"
  - "@deads2k"
  - "@mmazur"
  - "@cbusse"
approvers:
  - "@enxebre"
api-approvers:
  - "@enxebre"
creation-date: 2026-02-27
last-updated: 2026-02-27
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1751
see-also:
  - https://github.com/openshift/hypershift/pull/6300
---

# Control Plane Version Status: Decoupled Upgrade State for Management-Side Components

## Summary

This enhancement adds a new `controlPlaneVersion` field to
`HostedClusterStatus` and `HostedControlPlaneStatus` that tracks the version
history of management-side control plane components independently from the
existing `version` field. The existing `version` field reflects the combined
state of both management-side and data-plane components (as reported by CVO).
The new `controlPlaneVersion` field reports exclusively on management-side
components, providing a clear signal for:

1. Whether all management-side components are running a specific version (e.g.
   to confirm a CVE fix has been applied).
2. What versions are currently active on the management side during an upgrade
   (to determine allowed NodePool version skew).
3. Whether a control plane upgrade has completed regardless of data-plane
   rollout progress.

This replaces the `ControlPlaneUpToDate` condition introduced in
[PR #6300](https://github.com/openshift/hypershift/pull/6300) with a richer
versioning structure that includes update history.

## Glossary

- **Management-side components**: Control plane pods running in the HCP
  namespace on the management cluster (kube-apiserver, etcd,
  kube-controller-manager, kube-scheduler, openshift-apiserver, etc.). These
  are represented by `ControlPlaneComponent` custom resources.
- **Data-plane components**: Operators and workloads running on the guest
  cluster worker nodes (e.g. OVN daemonsets, ingress controller, image
  registry). Their rollout depends on NodePool compute availability.
- **CVO (ClusterVersion Operator)**: Reports the combined version status of
  both management-side and data-plane components. This is what
  `HostedClusterStatus.Version` currently reflects.
- **HCP**: HostedControlPlane custom resource.
- **HC**: HostedCluster custom resource.

## Motivation

### Current State

Today, `HostedClusterStatus.Version` (type `ClusterVersionStatus`) mirrors the
CVO-reported `ClusterVersion`. This version reaches `Completed` state only when
**all** components -- both management-side and data-plane -- have finished
rolling out to the target version. This creates several problems:

1. **Upgrade state is conflated**: A service provider cannot distinguish between
   "management components are at version X but data-plane rollout is pending"
   and "management components themselves have not finished upgrading."
2. **CVE verification is blocked**: To confirm that management-side components
   are not vulnerable to a CVE fixed in version X, operators must wait for the
   entire cluster (including data plane) to report version X as `Completed`.
3. **No-compute clusters are stuck**: When a HostedCluster has zero NodePools
   or all NodePools are scaled to zero, data-plane operators can never complete
   rollout. The CVO-reported version stays `Partial` indefinitely, providing
   no signal about the management side.
4. **Fleet management decisions are imprecise**: Service providers (ROSA, ARO)
   that manage y-stream end-of-support upgrades and z-stream CVE patches need
   to know when the management side is done, not when the entire cluster is
   done.
5. **NodePool version skew decisions require history**: To determine which
   NodePool versions are allowed, the system must know all versions that are
   currently active on the management side. A simple boolean condition or
   single version field is insufficient when upgrades fail or are in progress
   across multiple versions (e.g. 4.19.6 -> 4.19.19 (failed) -> 4.20.1
   (in progress) requires knowing that 4.19 and 4.20 are both active).

### Why Not a Condition?

[PR #6300](https://github.com/openshift/hypershift/pull/6300) introduced a
`ControlPlaneUpToDate` condition that reports `True`/`False` based on whether
all `ControlPlaneComponent` versions match the desired release. While useful as
a quick boolean signal, a condition cannot express:

- **Version history**: Which versions have been applied, when they started,
  when they completed, and whether they succeeded or failed. This history is
  required to compute allowed NodePool version skew.
- **Current version during rollout**: During an upgrade from 4.19 to 4.20,
  the condition is simply `False`. It does not tell you that some components
  are at 4.19 and others at 4.20.
- **Multiple concurrent versions**: In failure scenarios (e.g. 4.19 -> 4.20
  failed, now upgrading to 4.21), there may be 3 versions active
  simultaneously.

A `ClusterVersionStatus`-style field with update history addresses all of
these needs, which is what this enhancement proposes.

## Proposal

### API Changes

Add a new field `controlPlaneVersion` to both `HostedClusterStatus` and
`HostedControlPlaneStatus`:

```go
type HostedClusterStatus struct {
    // ... existing fields ...

    // version is the status of the release version applied to the
    // HostedCluster. This reflects the combined state of both management-side
    // and data-plane components as reported by CVO.
    // +optional
    Version *ClusterVersionStatus `json:"version,omitempty"`

    // controlPlaneVersion is the status of the release version applied
    // exclusively to management-side control plane components. Unlike
    // version (which reflects CVO state including data-plane components),
    // this field tracks only components running in the HCP namespace on
    // the management cluster.
    // +optional
    ControlPlaneVersion *ControlPlaneVersionStatus `json:"controlPlaneVersion,omitempty"`
}

type HostedControlPlaneStatus struct {
    // ... existing fields ...

    // controlPlaneVersion is the status of the release version applied
    // exclusively to management-side control plane components.
    // +optional
    ControlPlaneVersion *ControlPlaneVersionStatus `json:"controlPlaneVersion,omitempty"`
}
```

The `ControlPlaneVersionStatus` type:

```go
// ControlPlaneVersionStatus reports the version state of management-side
// control plane components. It mirrors the structure of ClusterVersionStatus
// but is computed from ControlPlaneComponent resources rather than CVO.
type ControlPlaneVersionStatus struct {
    // desired is the release version that the control plane is reconciling
    // towards. This is the version from the HostedControlPlane spec.
    // +required
    Desired configv1.Release `json:"desired"`

    // history contains a list of versions applied to management-side control
    // plane components. The newest entry is first in the list. Entries have
    // state Completed when all ControlPlaneComponent resources report the
    // target version with RolloutComplete=True. Entries have state Partial
    // when the rollout is in progress or has failed.
    // +optional
    // +kubebuilder:validation:MaxItems=50
    History []configv1.UpdateHistory `json:"history,omitempty"`

    // observedGeneration reports which generation of the HCP spec is being
    // synced.
    // +required
    ObservedGeneration int64 `json:"observedGeneration"`
}
```

This reuses the existing `configv1.Release` and `configv1.UpdateHistory`
types, which provide:

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

**`ControlPlaneVersionStatus.Desired`**: Set to the release image and version
from `HostedControlPlane.Spec.ReleaseImage`. This is the version the control
plane is reconciling towards.

**`ControlPlaneVersionStatus.History`**: Ordered list (newest first) of
version transitions for management-side components:

| Field | Value |
|-------|-------|
| `State` | `Completed` when **all** `ControlPlaneComponent` resources in the HCP namespace report `Status.Version == target version` AND `RolloutComplete` condition is `True`. `Partial` otherwise. |
| `StartedTime` | Timestamp when the upgrade to this version began (i.e. when the HCP spec was updated to this release image). |
| `CompletionTime` | Timestamp when all management-side components reached the target version. `nil` while in progress. For non-current entries, set to the `StartedTime` of the next entry. |
| `Version` | Semantic version string (e.g. `"4.20.1"`). |
| `Image` | Release image pullspec. |

**Transition rules**:

1. When `HCP.Spec.ReleaseImage` changes, a new `Partial` history entry is
   prepended. The previous entry's `CompletionTime` is set to the new entry's
   `StartedTime` (regardless of whether the previous entry completed).
2. On each reconciliation, the controller checks all `ControlPlaneComponent`
   resources. If all report the desired version with `RolloutComplete=True`,
   the current (first) history entry transitions from `Partial` to `Completed`
   and `CompletionTime` is set.
3. History is capped at 50 entries. Oldest entries are pruned when the cap is
   exceeded.

### Reconciliation Logic

The reconciliation happens in the **HostedControlPlane controller**
(`control-plane-operator`), replacing the existing
`reconcileControlPlaneUpToDateCondition` function:

```
reconcileControlPlaneVersion(ctx, hcp, releaseImage):
  1. List all ControlPlaneComponent resources in hcp.Namespace.
  2. If listing fails, set observedGeneration and return error.
  3. Determine desired version and image from releaseImage.
  4. Initialize controlPlaneVersion if nil.
  5. If desired version differs from current history[0].Version:
     a. Close out history[0] by setting CompletionTime = now.
     b. Prepend new entry: {State: Partial, Version: desired, Image: image,
        StartedTime: now, CompletionTime: nil}.
  6. Check all ControlPlaneComponent resources:
     a. For each component, compare Status.Version to desired version.
     b. For each component, check RolloutComplete condition.
  7. If ALL components match desired version AND have RolloutComplete=True:
     a. Set history[0].State = Completed.
     b. Set history[0].CompletionTime = now (if not already set).
  8. Prune history to 50 entries.
  9. Set observedGeneration = hcp.Generation.
```

The **HostedCluster controller** (`hypershift-operator`) copies
`controlPlaneVersion` from the HCP to the HC status, following the same
pattern used for other HCP-to-HC status propagation (e.g. conditions,
`version`).

### Removing the ControlPlaneUpToDate Condition

The `ControlPlaneUpToDate` condition from PR #6300 is removed. Its semantics
are fully subsumed by `controlPlaneVersion`:

| ControlPlaneUpToDate semantics | controlPlaneVersion equivalent |
|-------------------------------|-------------------------------|
| `True` (all components at desired version) | `history[0].State == Completed` |
| `False` (version mismatch) | `history[0].State == Partial` |
| `Unknown` (no components found) | `controlPlaneVersion == nil` OR `history` is empty |
| Error messages about specific components | Not in scope for the version status field; component-level issues are already reported on individual `ControlPlaneComponent` conditions |

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
- **Minimum NodePool version**: Look at the oldest `Partial` entry (4.19.19)
  or the last `Completed` entry (4.19.6). Since 4.19.19 never completed, some
  components may still be at 4.19.6, so the effective minimum is 4.18 (n-1 of
  4.19).
- **Maximum NodePool version**: The current desired version (4.20.1), so the
  max is 4.20.
- **CVE status**: Until `history[0].State == Completed`, you cannot confirm
  all management-side components are at 4.20.1.

### Consumer Use Cases

**Service providers (ROSA/ARO)**:
- Poll `controlPlaneVersion.history[0]` to determine when a control plane
  upgrade is complete, independently of data-plane rollout.
- Use `history[0].State == Completed` as the signal to proceed with
  subsequent operations (e.g. marking upgrade as done in OCM/ARO RP).
- Scan fleet clusters where `controlPlaneVersion.history[0].Version` is below
  a CVE-patched version to identify clusters needing forced upgrades.

**NodePool version skew computation**:
- Walk `history` to find all versions with `State == Partial` (still active on
  some components). The union of these versions plus the last `Completed`
  version determines the range of active control plane versions.
- Allowed NodePool versions are constrained by the n-1/n-2 skew policy
  relative to the lowest active control plane version (min) and the highest
  active version (max).

**Fleet metrics and dashboards**:
- Expose `controlPlaneVersion.history[0].version` and
  `controlPlaneVersion.history[0].state` as Prometheus metrics for fleet-wide
  version distribution dashboards.
- Alert on clusters where `history[0].State == Partial` for longer than a
  threshold.

### OVN Limitation

Until [OCPSTRAT-1454](https://issues.redhat.com/browse/OCPSTRAT-1454) is
resolved, OVN control plane components run on the data plane. This means
`controlPlaneVersion` may reach `Completed` while OVN components are still at
the previous version. Consumers should be aware that `controlPlaneVersion`
covers components tracked by `ControlPlaneComponent` resources in the HCP
namespace, which excludes OVN until OCPSTRAT-1454 is addressed.

## Risks and Mitigations

**Risk**: The new field adds API surface that consumers may depend on for
upgrade orchestration decisions. If the reconciliation logic has bugs (e.g.
prematurely marking `Completed`), it could cause incorrect NodePool version
skew decisions.

**Mitigation**: The reconciliation logic is straightforward -- it iterates
over `ControlPlaneComponent` resources and compares versions. Comprehensive
unit tests cover all edge cases (version mismatch, partial rollout, no
components, multiple failures). The logic reuses the same `ControlPlaneComponent`
status fields that the existing `ControlPlaneUpToDate` condition already
validates.

**Risk**: Consumers may confuse `controlPlaneVersion` with `version` and make
incorrect assumptions about the overall cluster state.

**Mitigation**: API field documentation clearly distinguishes the two fields.
The naming convention (`controlPlaneVersion` vs. `version`) makes the scope
difference explicit.

## Test Plan

- **Unit tests**: Test the `reconcileControlPlaneVersion` function with:
  - All components at desired version with RolloutComplete=True -> Completed.
  - Version mismatch on one or more components -> Partial.
  - No ControlPlaneComponent resources found -> nil/empty history.
  - Version change (new desired) -> new Partial entry prepended, previous
    entry closed.
  - History pruning at 50 entries.
  - Failed upgrade followed by new upgrade -> correct history ordering.
- **E2E tests**:
  - Verify `controlPlaneVersion` is populated on a steady-state cluster.
  - Verify `controlPlaneVersion.history[0].State` transitions from `Partial`
    to `Completed` during an in-place z-stream upgrade.
  - Verify `controlPlaneVersion` is propagated from HCP to HC status.
  - Verify `controlPlaneVersion` reaches `Completed` even when NodePools are
    scaled to zero (i.e. data-plane cannot roll out).
  - Verify y-stream upgrade produces correct history entries.

## Graduation Criteria

### Dev Preview -> Tech Preview

- `controlPlaneVersion` field populated on all HostedClusters.
- History entries correctly track version transitions.
- Unit test and basic e2e coverage.
- `ControlPlaneUpToDate` condition removed.

### Tech Preview -> GA

- E2E coverage of z-stream and y-stream upgrades with history validation.
- Validation with zero-NodePool clusters.
- Fleet-scale testing with many HostedClusters.
- OCM/ARO RP integration validated (consuming `controlPlaneVersion` for
  upgrade decisions).

## Upgrade / Downgrade Strategy

**Upgrade**: When a HostedCluster is upgraded to a version containing this
feature, the controller initializes `controlPlaneVersion` with a single
history entry reflecting the current state of `ControlPlaneComponent`
resources. No manual action is required.

**Downgrade**: When a HostedCluster is downgraded to a version without this
feature, the `controlPlaneVersion` field is ignored by the older controller
and remains in the status as an unknown field. It does not affect cluster
behavior. The field is cleaned up when the older controller next writes the
status (since it does not set the field, it will be omitted).

## Alternatives

### Keep ControlPlaneUpToDate Condition Only

The condition from PR #6300 provides a simple True/False signal. This is
insufficient for:
- Version history needed for NodePool skew computation.
- Distinguishing which versions are active during multi-step upgrades.
- Providing timestamps for fleet SLA tracking.

### Add Multiple Conditions Instead of a Version Field

One could add conditions like `ControlPlaneUpToDate`,
`ControlPlaneUpgradeInProgress`, `ControlPlaneVersionSkewed`, etc. This
becomes unwieldy and still cannot express ordered history with timestamps.
Conditions are best for boolean state, not versioned history.

### Extend the Existing version Field

Modifying `HostedClusterStatus.Version` to reflect only management-side
components would break existing consumers that rely on it to represent the
CVO-reported combined state. The two concerns (management-only vs. combined)
serve different users and must remain separate.
