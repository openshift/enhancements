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

Operators can create external resources in clouds and other services that have no opportunity to be cleaned up when a cluster is deprovisioned, causing extra work for cluster maintainers to track down and remove these resources. Signalling cluster deletion would allow components running in the cluster to clean up external resources during deprovision and would ensure that total cleanup is an automatic process.

This process will still be best effort as it is always possible users may manually destroy the cluster itself without allowing this mechanism to run to completion.

## Motivation

### Goals

Provide a means to signal that cluster deletion has begun, and give cluster components time to clean up before continuing with removing the clusters instances, storage and networking.

### Non-Goals

* Teardown of the actual cluster itself. (control plane, instances, etc) This process is expected to still be run external to the cluster.
* Attaching finalizers to operator resources based on the `Alive` object which would facilitate removal of operator resources.
* Removing resources other than the `Alive` object during cluster deprovision.

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

#### Provision

Create an `Alive` object during installation.

How this happens remains unknown. We do not wish to create an entire second level operator just to deploy this resource. We do not wish to stuff it into the CVO. Some kind of mechanism to a CRD and CR manifest into the release repo seems most likley.

#### Upgrade

Create an `Alive` object for existing clusters.

If the resource is carried in the release image, this should handle the upgrade case.

#### Deprovision

Best practice for entities that provision/deprovision clusters (openshift-install, Hive, managed offerings from cloud partners) would be to make use of this deletion to allow in-cluster resources to clean up. However we accept that this is a best effort, opt-in solution, and not all implementations will make use of it. (leaving them no worse off than they are today)

The best practice would then become:

  1. Delete the Alive custom resource before destroying the cluster itself.
  1. Wait a period of time to allow the process to complete.
  1. If the Alive resource successfully disappears before the timeout, proceed with normal cluster teardown.
  1. If the Alive resource does not disappear before the time out, each implementation will need to make their own decision on how to proceed. (log remaining finalizers and proceed regardless, or fail and require manual intervention / retries)
    1. We expect that `openshift-install destroy cluster` will add a new command line flag to initiate Alive teardown, and will fail if it does not complete in time requiring the user to run without the flag or manually investigate and correct the problem.

### Risks and Mitigations

We are unsure of the value in implementing this purely in OpenShift. It may make sense to bring to sig-cloud or sig-cluster-lifecycle for consideration, allowing the broader Kubernetes ecosystem to make use of it. In theory, it may also help in Kubernetes itself in cleanup of things like Service load balancers.

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
