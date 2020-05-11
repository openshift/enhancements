---
title: signal-cluster-deletion
authors:
  - "@abutcher"
  - "@dgoodwin"
reviewers:
  - dgoodwin
  - abhinavdahiya
  - wking
  - eparis
  - ecordell

approvers:
  - dgoodwin
  - abhinavdahiya
  - wking
  - eparis
creation-date: 2020-03-30
last-updated: 2020-05-07
status: provisional
---

# Signal Cluster Deletion

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Operators can create external resources in clouds and other services that have no opportunity to be cleaned up when a cluster is uninstalled, causing extra work for cluster maintainers to track down and remove these resources. Signalling cluster deletion would allow operators to clean up external resources during uninstallation and would ensure that total cleanup is an automatic process.

This process will still be best effort as it is always possible users may manually cleanup resources without allowing this mechanism to run to completion.

## Motivation

### Goals

Signal that cluster deletion has begun and give operators or other cluster components time to teardown before continuing with removing cluster resources such as instances, storage and networking.

### Non-Goals

* Teardown of the actual cluster itself. (control plane, instances, etc) This process is expected to still be run external to the cluster.
* Attaching finalizers to operator resources based on the `Alive` object which would facilitate removal of operator resources.
* Removing resources other than the `Alive` object during cluster uninstall.

## Proposal

### User Stories

#### Story 1
As an OpenShift administrator, I would like for any external resources created in service of the cluster be removed when when I delete the cluster, so that:
* I do not have to track down those resources and remove them manually
* I do not have to pay for resources that are not being used.

#### Story 2
As an OpenShift (software, big-O) Operator, I would like to have a way to know that the cluster is being shut down, so that I can perform any necessary cleanup steps and remove external resources.

#### Story 3
As an OpenShift administrator, I would like to have a way to indicate that certain external resources that would normally be cleaned up on cluster deletion be preserved on cluster delete, so that I don't lose important resources stored externally.

### Implementation Details/Notes/Constraints

#### API

Introduce a new CRD `Alive` from which operator resources could attach a finalizer initiating cascading delete when the `Alive` resource is deleted.

```yaml
apiVersion: v1alpha1
kind: Alive
metadata:
  name: cluster
  namespace: openshift-config
spec:
```

*Note*: We proposed adding the CRD and admission plugin (which pevents delete) to the cluster version operator and it was decided that they would be better placed within OLM. We would prefer not to create a separate operator for a CRD and admission plugin as that would require a repository and release engineering. We're requesting feedback on where to put these resources.

#### Install

Create an `Alive` object during installation.

#### Upgrade

Create an `Alive` object for existing clusters.

#### Uninstall

Projects that initiate teardown of OpenShift clusters externally should make use of this CR if they would like to avoid issues with dangling resources.

  1. Delete the Alive custom resource.
  1. Wait a period of time to allow operators to complete their teardown.
  1. If the cluster is not reachable, proceed with normal cluster teardown regardless of the potential dangling resources.
  1. If timeout is hit and the Alive resource still exists, proceed with teardown regardless of the potential dangling resources.

These steps would need to be taken in openshift-install and Hive, ARO, ROKS, etc.

An openshift-install implementation would be exposed by a new command line flag, and fail hard if the timeout is hit. User would need to re-run to keep trying, or omit the flag to proceed anyhow.

We accept that some clusters will be provisioned with tooling that does not respect this and possibly orphan resources. This is a best effort opt-in solution.

### Risks and Mitigations

## Design Details

### Test Plan

In OpenShift CI, clusters will try to delete their `Alive` object and if that fails, all infra resources will be destroyed.

```
retval = openshift-install destroy cluster --cluster-alive-delete
if retval != success
openshift-install destroy cluster
return retval
```

### Graduation Criteria

This enhancement will follow standard graduation criteria.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- API (v1alpha1) will not be moved to stable until it has been tested for a considerable period of time internally.

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

Not applicable.

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

Not applicable.

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed

Not applicable.
