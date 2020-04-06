---
title: signal-cluster-deletion
authors:
  - "@abutcher"
reviewers:
  - dgoodwin
  - abhinavdahiya
approvers:
  - dgoodwin
  - abhinavdahiya
creation-date: 2020-03-30
last-updated: 2020-04-06
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

Signal that cluster deletion has begun through deletion of a new CRD `ClusterAlive` and wait for that object to be deleted during uninstall before continuing with removing cluster resources such as instances, storage and networking.

### Non-Goals

* Attaching finalizers to operator resources based on the `ClusterAlive` object which would facilitate removal of operator resources.
* Removing resources other than the `ClusterAlive` object during cluster uninstall.

## Proposal

### User Stories

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints

#### API

Introduce a new CRD `ClusterAlive` from which operator resources could attach a finalizer initiating cascading delete when the `ClusterAlive` resource is deleted.

```yaml
apiVersion: v1
kind: ClusterAlive
metadata:
  name: cluster
spec:
  blockTeardown: true  # block teardown of operator resources
```

#### Install

Create a `ClusterAlive` object during installation. (Is a manifest the right place to create for new clusters ?)

#### Upgrade

Create `ClusterAlive` object for existing clusters. (Is this the right place to create for existing clusters ?)

#### Uninstall

Delete a cluster's `ClusterAlive` resource during uninstall when requested via flag such as `openshift-install destroy cluster --cluster-alive-delete`. Cluster destroy will wait for a default amount of time and fail if `ClusterAlive` deletion was not successful. The default timeout will not be configurable and users will be expected to attempt shutdown multiple times upon failure.

### Risks and Mitigations

## Design Details

### Test Plan

In OpenShift CI, clusters will try to delete their `ClusterAlive` object and if that fails, all infra resources will be destroyed.

```
destroy cluster --cluster-alive-delete || true
destroy cluster
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

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

Not applicable.

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed

Not applicable.
