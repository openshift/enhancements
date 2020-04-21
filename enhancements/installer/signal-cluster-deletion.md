---
title: signal-cluster-deletion
authors:
  - "@abutcher"
reviewers:
  - dgoodwin
  - abhinavdahiya
  - wking
  - eparis
approvers:
  - dgoodwin
  - abhinavdahiya
  - wking
  - eparis
creation-date: 2020-03-30
last-updated: 2020-04-21
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

## Motivation

### Goals

Signal that cluster deletion has begun through deletion of a new CRD `Alive` and wait for that object to be deleted during uninstall before continuing with removing cluster resources such as instances, storage and networking.

### Non-Goals

* Attaching finalizers to operator resources based on the `Alive` object which would facilitate removal of operator resources.
* Removing resources other than the `Alive` object during cluster uninstall.

## Proposal

### User Stories

#### Story 1

#### Story 2

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
  blockTeardown: true  # block teardown of operator resources
```

Add an admission plugin that prevents delete when `blockTeardown` has been set.

We believe it would be best to add the CRD and admission plugin to the cluster version operator. We don't foresee this requiring a controller and doing a separate operator seems like overkill for this object.

#### Install

Create a `Alive` object during installation.

#### Upgrade

Create `Alive` object for existing clusters.

#### Uninstall

Delete a cluster's `Alive` resource during uninstall when requested via flag such as `openshift-install destroy cluster --cluster-alive-delete`. Cluster destroy will wait for a default amount of time and fail if `Alive` deletion was not successful. The default timeout will not be configurable and users will be expected to attempt shutdown multiple times upon failure.

`destroy --cluster-alive-delete` will fail if `blockTeardown` has been set for a cluster's `Alive` object and report that an admin is blocking teardown for this cluster. This will either be checked directly or attempted and rejected by the admission plugin.

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
