---
title: disable-force-detach-option
authors:
  - "@dobsonj"
reviewers:
  - "@ingvagabund"
  - "@gnufied"
approvers:
  - "@jsafrane"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-01-30
last-updated: 2026-09-08
tracking-link:
  - "https://issues.redhat.com/browse/STOR-2789"
see-also:
  - "https://issues.redhat.com/browse/OCPBUGS-61077"
  - "https://issues.redhat.com/browse/RFE-8138"
  - "https://github.com/openshift/api/pull/2668"
replaces:
superseded-by:
---

# Option to disable force detach of volumes

## Summary

There have been issues with certain drivers where the volume is force detached while the volume is still mounted, leading to data corruption. For drivers directly exposing LUNs, force detach bypasses the unstage flow where multipath -f is invoked and goes straight to unpublish which is unmapping the LUN from the per-node igroup. This enhancement introduces an option to disable force detach of volumes in OCP to avoid this problem.

## Motivation

Force detaching a volume can corrupt a volume's data in some clusters. This is not always safe and the cluster admin needs an option to disable it.

### User Stories

As a cluster admin, I need the option to disable force detach on timeout to avoid data loss.

### Goals

* Allow cluster admin to control the [disable-force-detach-on-timeout](https://kubernetes.io/docs/concepts/cluster-administration/node-shutdown/#storage-force-detach-on-timeout) config option for kube-controller-manager (KCM).

### Non-Goals

* Not exposing other parameters.
* Not providing finer-grained control than cluster scope.
* Not allowing anyone other than an admin to control this parameter.
* Not changing the default behavior (force detach enabled).

## Proposal

Create a new config object for kube-controller-manager with a force detach option. `cluster-kube-controller-manager-operator` will read the option from the API and control the `disable-force-detach-on-timeout` config option for KCM.

### Workflow Description

The cluster admin sets `.spec.forceDetachOnTimeout="Disabled"` in the `cluster` ControllerManager object. `cluster-kube-controller-manager-operator` is notified of this change via an informer, and it updates the KCM target config to include the `disable-force-detach-on-timeout` option. KCM restarts and reads the new config option.

### API Extensions

There is already a [KubeControllerManager](https://github.com/openshift/api/blob/master/operator/v1/types_kubecontrollermanager.go) operator type to configure cluster-kube-controller-manager-operator, and this enhancement introduces a `ControllerManager` config type to configure kube-controller-manager.
This is approach is consistent with other workload components like
[KubeAPIServer](https://github.com/openshift/api/blob/master/operator/v1/types_kubeapiserver.go) / [APIServer](https://github.com/openshift/api/blob/master/config/v1/types_apiserver.go) and
[KubeScheduler](https://github.com/openshift/api/blob/master/operator/v1/types_scheduler.go) / [Scheduler](https://github.com/openshift/api/blob/master/config/v1/types_scheduling.go).

Example:
```
apiVersion: config.openshift.io/v1alpha1
kind: ControllerManager
spec:
  forceDetachOnTimeout: Disabled
```

API PR: <https://github.com/openshift/api/pull/2668>

### Topology Considerations

#### Hypershift / Hosted Control Planes

Nothing special for HCP.

#### Standalone Clusters

Nothing special for standalone clusters.

#### Single-node Deployments or MicroShift

Nothing special for SNO.

#### OpenShift Kubernetes Engine

Nothing special for OKE.

### Implementation Details/Notes/Constraints

None

### Risks and Mitigations

None

### Drawbacks

The drawback is adding an extra config option that needs to be tested by us and understood by our users. This is outweighed by the need to avoid data loss caused by force detach.

## Alternatives (Not Implemented)

Add a new field to the Storage CR to control force detach behavior. This was ruled out because the Storage CR does not typically control KCM behavior. Storage is also an optional component that customers can disable but they may still want to disable force detach behavior in KCM.

## Open Questions [optional]

None

## Test Plan

- Unit tests and API tests
- Manual validation of code changes
- e2e: run CSI test suite with force detach disabled

| Test scenario | Type | Where | Test link | Notes |
| :---- | :---- | :---- | :---- | :---- |
| `.spec.forceDetachOnTimeout="Disabled"` should disable force detach on the cluster. | Manual |  |  | |
| `.spec.forceDetachOnTimeout="Enabled"` should enable force detach on the cluster. | Manual |  |  | |
| Default behavior should remain the same (force detach enabled). | Manual |  |  | |
expectations).

## Graduation Criteria

| OCP Version | OCP Status |
|-------------|------------|
| 5.1         | Alpha (DP) |
| 5.2         | Beta (TP)  |
| 5.3         | Beta (TP)  |
| 5.4         | GA         |

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage

### Tech Preview -> GA

- e2e tests implemented
- High severity bugs are fixed.
- Reliable CI signal, minimal test flakes.
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The ForceDetachOnTimeout API field will be optional. If it is unspecified or empty, it will behave the same as "Enabled" (current default). So upgraded clusters will still have this option enabled unless the admin decides to set it explicitly to "Disabled".

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

This enhancement introduces a new ControllerManager API object that needs to be included in the must-gather.

## Support Procedures

TBD

## Infrastructure Needed [optional]

N/A

