---
title: do-not-block-on-degraded-true-clusteroperators
authors:
  - "@wking"
reviewers:
  - "@PratikMahajan, update team lead"
  - "@sdodson, update staff engineer"
approvers:
  - "@PratikMahajan, update team lead"
api-approvers:
  - None
creation-date: 2024-11-25
last-updated: 2025-01-09
tracking-link:
  - https://issues.redhat.com/browse/OTA-540
---

# Do not block on Degraded=True ClusterOperators

## Summary

The cluster-version operator (CVO) uses an update-mode when transitioning between releases, where the manifest operands are [sorted into a task-node graph](/dev-guide/cluster-version-operator/user/reconciliation.md#manifest-graph), and the CVO walks the graph reconciling.
Since 4.1, the cluster-version operator has blocked during update and reconcile modes (but not during install mode) on `Degraded=True` ClusterOperator.
This enhancement proposes ignoring `Degraded` when deciding whether to block on a ClusterOperator manifest.

## Motivation

The goal of blocking on manifests with sad resources is to avoid further destabilization.
For example, if we have not reconciled a namespace manifest or ServiceAccount RoleBinding, there's no point in trying to update the consuming operator Deployment.
Or if we are unable to update the Kube-API-server operator, we don't want to inject [unsupported kubelet skew][kubelet-skew] by asking the machine-config operator to update nodes.

However, blocking the update on a sad resource has the downside that later manifest-graph task-nodes are not reconciled, while the CVO waits for the sad resource to return to happiness.
We maximize safety by blocking when progress would be risky, while continuing when progress would be safe, and possibly helpful.

Our experience with `Degraded=True` blocks turns up cases like:

* 4.6 `Degraded=True` on an unreachable, user-provided node, with monitoring reporting `UpdatingnodeExporterFailed`, network reporting `RolloutHung`, and machine-config reporting `MachineConfigDaemonFailed`.
  But those ClusterOperator were all still `Available=True`, and in 4.10 and later, monitoring workloads are guarded by PodDisruptionBudgets (PDBs)

### User Stories

* As a cluster administrator, I want the ability to defer recovering `Degraded=True` ClusterOperators without slowing ClusterVersion updates.

### Goals

ClusterVersion updates will no longer block on ClusterOperators solely based on `Degraded=True`.

### Non-Goals

* Adjusting how the cluster-version operator treats `Available` and `versions` in ClusterOperator status.
  The CVO will still block on `Available=False` ClusterOperator, and will also still block on `status.versions` reported in the ClusterOperator's release manifest.

* Adjusting whether `Degraded` ClusterOperator conditions propagated through to the ClusterVersion `Failing` condition.
  As with the current install mode, the sad condition will be propagated through to `Failing=True`, unless outweighed by a more serious condition like `Available=False`.

## Proposal

The cluster-version operator currently has [a mode switch][cvo-degraded-mode-switch] that makes `Degraded` ClusterOperator a non-blocking condition that is still propagated through to `Failing`.
This enhancement proposes making that an unconditional `UpdateEffectReport`, regardless of the CVO's current mode (installing, updating, reconciling, etc.).

### Workflow Description

Cluster administrators will be largely unaware of this feature.
They will no longer have ClusterVersion update progress slowed by `Degraded=True` ClusterOperators, so there will be less admin involvement there.
They will continue to be notified of `Degraded=True` ClusterOperators via [the `warning` `ClusterOperatorDegraded` alert][ClusterOperatorDegraded] and the `Failing=True` ClusterVersion condition.

### API Extensions

No API extensions are needed for this proposal.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift's ClusterOperator context is the same as standalone, so it will receive the same benefits from the same cluster-version operator code change, and does not need special consideration.

#### Standalone Clusters

Yes, the enhancement is expected to improve the update experience on standalone, by decoupling ClusterVersion update completion from recovering `Degraded=True` ClusterOperators, granting the cluster administrator the flexibility to address update speed and operator degradation independently.

#### Single-node Deployments or MicroShift

Single-node's ClusterOperator context is the same as standalone, so it will receive the same benefits from the same cluster-version operator code change, and does not need special consideration.
This change is a minor tweak to existing CVO code, so it is not expected to impact resource consumption.

MicroShift updates are managed via RPMs, without a cluster-version operator, so it is not exposed to the ClusterVersion updates this enhancement is refining, and not affected by the changes proposed in this enhancement.

### Implementation Details/Notes/Constraints

The code change is expected to be a handful of lines, as discussed in [the *Proposal* section](#proposal), so there are no further implementation details needed.

### Risks and Mitigations

The risk would be that there are some ClusterOperators who currently rely on the cluster-version operator blocking during updates on ClusterOperators that are `Available=True`, `Degraded=True`, and which set the release manifest's expected `versions`.
As discussed in [the *Motivation* section](#motivation), we're not currently aware of any such ClusterOperators.
If any turn up, we can mitigate by [declaring conditional update risks](targeted-update-edge-blocking.md) using the existing `cluster_operator_conditions{condition="Degraded"}` PromQL metric, while teaching the relevant operators to set `Available=False` and/or without their `versions` bumps until the issue that needs to block further ClusterVersion update progress has been resolved.

How will security be reviewed and by whom?
Unclear.  Feedback welcome.

How will UX be reviewed and by whom?
Unclear.  Feedback welcome.

### Drawbacks

As discussed in [the *Risks* section](#risks-and-mitigations), the main drawback is changing behavior that we've had in place for many years.
But we do not expect much customer pushback based on "hey, my update completed?!  I expected it to stick on this sad component...".
We do expect it to reduce customer frustration when they want the update to complete, but for reasons like administrative siloes do not have the ability to recover a component from minor degradation themselves.

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Graduation Criteria

There are no API changes proposed by this enhancement, which only affects sad-path handling, so we expect the code change to go straight to the next generally-available release, without feature gating or staged graduation.

### Dev Preview -> Tech Preview

Not applicable.

### Tech Preview -> GA

Not applicable.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

This enhancement only affects the cluster-version operator's internal processing of longstanding ClusterOperator APIs, so there are no skew or compatibility issues.

## Version Skew Strategy

This enhancement only affects the cluster-version operator's internal processing of longstanding ClusterOperator APIs, so there are no skew or compatibility issues.

## Operational Aspects of API Extensions

There are no API changes proposed by this enhancement.

## Support Procedures

This enhancement is a small pivot in how the cluster-version operator processes ClusterOperator manifests during updates.
As discussed in [the *Drawbacks* section](#drawbacks), we do not expect cluster admins to open support cases related to this change.

## Alternatives

We could continue with the current approach, and absorb the occasional friction it causes.

## Infrastructure Needed

No additional infrastructure is needed for this enhancement.

[ClusterOperatorDegraded]: https://github.com/openshift/cluster-version-operator/blob/820b74aa960717aae5431f783212066736806785/install/0000_90_cluster-version-operator_02_servicemonitor.yaml#L106-L124
[cvo-degraded-mode-switch]: https://github.com/openshift/cluster-version-operator/blob/820b74aa960717aae5431f783212066736806785/pkg/cvo/internal/operatorstatus.go#L241-L245
[kubelet-skew]: https://kubernetes.io/releases/version-skew-policy/#kubelet
