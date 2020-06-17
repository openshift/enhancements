---
title: generic-delete-protection
authors:
  - "@deads2k"
reviewers:
  - "@sttts"
approvers:
  - "@derekwaynecarr"
creation-date: 2020-06-17
last-updated: 2020-06-17
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - https://github.com/open-cluster-management/backlog/issues/2648#issuecomment-645170280  
  - https://github.com/open-cluster-management/backlog/issues/2348#issuecomment-642231925  
replaces:
superseded-by:
---

# Generic Delete Protection

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions 

1. At this moment, adoption is racy because deployment.apps/name is insufficient to distinguish based on UID, but to allow
   freedom of creation ordering, the CriticalService cannot include a UID.
   We may be able to create a status field that tracks a UID, but the intent isn't fully clear.
   If adoption doesn't work, we can end up in a state where ManifestWork cannot be finalized because the CriticalService
   isn't actually eligible for deletion.
   The simplest solution seem to be to state an intent of "best-effort-deletion" or "orphan-resources".
   This could be added as a spec field which could be set post-deletion, pre-finalization.

## Summary

By running the platform on the cluster, there are complications that arise regarding deletion of certain critical services.
These services include webhook admission plugins and finalizers.
This design is about trying to find a way to prevent permanently "stuck" resources without trying to coordinate resource deletion
in clients.

## Motivation

If pods for admission-webhook-a, which protect resource-a, are deleted, then it becomes impossible to create, update, or
delete any resource-a until the webhookconfiguration is deleted or the pods are restored.
If pods for finalizer-a, which protect resource-a, are deleted, then it becomes impossible to delete any resource-a until
the pods are restored.
Resolving resource deletion ordering in clients is impractical.

### Goals

1. Eliminate the need for ordered client deletion.

### Non-Goals

1. Prevent permanently stuck resources in cases where the resource that is stuck and the the pod providing the critical service
   are in the same namespace.  Don't do that. 

## Proposal

We will create a validating admission webhook that intercepts DELETEs of namespaces, criticalservices, and any resource listed as a .spec.provider.
Realistically, we will hard code deployments.apps and have that as the only valid resource in .spec.provider to start.
When a DELETE arrives for this resource we will..

1. Check all CriticalServices to see if this particular instance is protected, if not ALLOW the delete.
2. Check the .spec.criteria for each matching CriticalService.
   If all .spec.criteria are satisfied, ALLOW the delete.
   If not all .spec.criteria are satisfied, do something sane in spec and DENY the delete.
3. As a special case, a CriticalService cannot be deleted until its .spec.provider is no longer present.

```go
package api

type CriticalService struct{
    Spec CriticalServiceSpec
}

type CriticalServiceSpec struct{
    Provider CriticalServiceProvider
    Criteria []CriticalServiceCriteria
}

type GroupResource struct{
    Group string
    Resource string
}

type CriticalServiceProvider struct{
    // only allow deployments.apps to start
    GroupResource
    Namespace string
    Name string
}

type CriticalServiceCriteriaType string
var(
    FinalizerType CriticalServiceCriteriaType = "Finalizer"
    SpecificResourceType CriticalServiceCriteriaType = "SpecificResource"
)

type CriticalServiceCriteria struct{
    Type CriticalServiceCriteriaType
    Finalizer *FinalizerCriticalServiceCriteria
    SpecificResource *SpecificResourceCriticalServiceCriteria
}

type FinalizerCriticalServiceCriteria struct{
    GroupResource
    FinalizerName string
}

type SpecificResourceCriticalServiceCriteria struct{
    GroupResource
    Namespace string
    Name string
}
```

### User Stories 

#### Deployment provides finalizer processing for CRD
```yaml
kind: CriticalService
spec:
  provider:
    group: apps
    resource: deployments
    namespace: finalizer-namespace
    name: finalizer-deployment
  criteria:
  - type: Finalizer
    finalizer:
      group: my.crd.group
      resource: coolresources
      finalizerName: my.crd.group/super-important
  - type: SpecificResource
    specificResource:
      group: apiextensions.k8s.io
      resource: CustomResourceDefinition
      name: coolresources.my.crd.group
```


This would allow three separate ManifestWorks to all be deleted at the same time and avoid conflicting with each other
as the deletes are finalized on individual resources.
```yaml
kind: ManifestWork
metadata:
  name: finalizer
spec:
  criticalservices/for-finalizer-deployment
  namespace/finalizer-namespace
  deployment.apps/finalizer-deployment
---
kind: ManifestWork
metadata:
  name: crd
spec:
  crd/coolresources.my.crd.group
---
kind: ManifestWork
metadata:
  name: cr
spec:
  coolresources.my.crd.group/some-instance
```

It would also allow them to be combined into a single ManifestWork.
```yaml
kind: ManifestWork
metadata:
  name: all
spec:
  criticalservices/for-finalizer-deployment
  namespace/finalizer-namespace
  deployment.apps/finalizer-deployment
  crd/coolresources.my.crd.group
  coolresources.my.crd.group/some-instance
```

This construct also means that namespaces in a management cluster can be be deleted when managed clusters are removed
because deletion can happen in any order. 


When a bulk delete happens, the effective order will be
1. crd/coolresources.my.crd.group is deleted, but waits to be finalized
2. coolresources.my.crd.group/some-instance is deleted, but waits to be finalized
3. coolresources.my.crd.group/some-instance is finalized
4. crd/coolresources.my.crd.group is finalized
5. deployment.apps/finalizer-deployment is deleted, but waits to be finalized
6. namespace/finalizer-namespace is deleted, but waits to be finalized
7. deployment.apps/finalizer-deployment is finalized
8. namespace/finalizer-namespace is finalized
9. criticalservices/for-finalizer-deployment is deleted

This is because
1. CRDs are not finalized until all the CR instances are removed
2. deployment.apps/finalizer-deployment and namespace/finalizer-namespace cannot be deleted until the finalizer is
   removed from all coolresources.my.crd.group and the crd/coolresources.my.crd.group is finalized.
3. criticalservices/for-finalizer-deployment cannot be deleted until deployment.apps/finalizer-deployment is finalized

This is enforced without client deletion coordination.

#### Story 2

### Implementation Details/Notes/Constraints [optional]


### Risks and Mitigations


## Design Details

### Test Plan

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

