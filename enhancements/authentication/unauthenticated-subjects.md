---
title: hardening-openshift-apiserver-default-policy-resource
authors:
  - "@vareti"
reviewers:
  - "@marun"
  - "@sttts"
  - "@deads2k"
approvers:
  - "@sttts"
creation-date: 2020-05-07
last-updated: 2020-05-13
status: implementable
see-also:
replaces:
superseded-by:
---

# Hardening OpenShift-apiserver Default Policy Resources on Upgrade Clusters

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

* In earlier releases of OpenShift, the authorization module would allow anonymous users to access discovery endpoints.
* This enhancement proposes a way to remove unauthenticated subjects from cluster role bindings after an upgrade from this earlier release.
* It is possible that a customer is relying on anonymous user behavior in an implementation.
* This enhancement also proposes a way to opt-out of this reconciliation mechanism. Thus helping cases where changes are not desirable to customer implementations.
* Later releases of OpenShift [removed](https://github.com/openshift/origin/pull/22953/file) this access for anonymous users. So, this enhancement would not affect the customers who install a fresh cluster using these later releases.

## Motivation

  * In OpenShift release 4.1, an anonymous user could access discovery endpoints. Later releases revoked this access by removing unauthenticated subjects from cluster role bindings. Because of the way default policy resources are reconciled, unauthenticated access is preserved in upgrade clusters.
  * Exposing unauthenticated endpoints like discovery API to any user by default cluster role bindings is a privacy concern. This is undesirable and needs prevention if possible. Revoking this access would aslo help in limiting the possibility of attacks similar to [CVE-2018-1002105](https://github.com/kubernetes/kubernetes/issues/71411).

### Goals

> 1. Upgrading an OpenShift cluster remove unauthenticated subjects from cluster role bindings that give access to discovery API.
> 2. Providing user a way to opt-out of this reconciliation before the upgrade.

### Non-Goals

## Proposal

* Some background on current logic before going into proposal.
  * A user who does not provide authentication information like certificates or bearer token to kube-apiserver is considered anonymous and will be part of "system:unauthenticated" group.
  * Both kube-apiserver and openshift-apiserver create a set of cluster roles and cluster role bindings during bootstrap.
    * [Source code](https://github.com/kubernetes/kubernetes/blob/v1.18.2/plugin/pkg/auth/authorizer/rbac/bootstrappolicy/policy.go) for Kubenetes default cluster roles and cluster role bindings.
    * [Source code](https://github.com/openshift/openshift-apiserver/blob/release-4.5/pkg/bootstrappolicy/policy.go) for OpenShift default cluster roles and cluster role bindings.
  * `rbac.authorization.kubernetes.io/autoupdate=true` annotation is also added to default policy resources.
  * On start-up both openshift-apiserver and kube-apiserver reconcile their default policy resources as follows
    * if the annotation `rbac.authorization.kubernetes.io/autoupdate` is set to `false`
	  * skips reconciliation of resource.
    * if the annotation `rbac.authorization.kubernetes.io/autoupdate` is set to any value other than `false`
	  * add missing permissions to cluster roles.
	  * add missing subjects to cluster role bindings.
    * if the annotation is deleted
	  * add annotation `rbac.authorization.kubernetes.io/autoupdate=true` to the RBAC resource.
	  * add missing permissions to cluster roles.
	  * add missing subjects to cluster role bindings.
  * reconciliation does not delete any existing permissions or subjects.
  * An admin to opt-out of reconciliation for a specific resource by setting annotation `rbac.authorization.kubernetes.io/autoupdate=false`.
  * Detailed behavior can be found in this [link](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#auto-reconciliation).

* Enhancement proposes a new controller implemented by openshift-apiserver operator to the reconcile following cluster role bindings created by openshift-apiserver.
  * cluster-status-binding
  * discovery
  * system:openshift:discovery
  * basic-users
* If the annotation `rbac.authorization.kubernetes.io/autoupdate` is present and set to `true`, the controller removes the unauthenticated subject from the list of subjects and updates the resource in the API server.
* Customers can opt-out of this reconciliation for a cluster role binding by setting the annotation `rbac.authorization.kubernetes.io/autoupdate` to `false`.

### User Stories [optional]

N/A

### Implementation Details/Notes/Constraints [optional]

N/A

### Risks and Mitigations

* Because of OpenShift 4.1 allowing access to discovery endpoint, some customer implementations might be expecting this privileged access to anonymous users. Removing unauthenticated subject on an upgrade might break some of these implementations.
* Informing customers beforehand of this change would help the customer to decide better. Customers can either opt-out of reconciliation or change their implementation to match the new requirements. A customer can opt-out of reconciliation by setting the annotation `rbac.authorization.kubernetes.io/autoupdate` to `false` before the upgrade.
* Depending on the case, a customer might need to annotate one or more of the following cluster role bindings to skip reconciliation
  * cluster-status-binding
  * discovery
  * system:openshift:discovery
  * basic-users
* An example command to annotate the cluster role binding is given below
```
oc annotate --overwrite clusterrolebinding system:openshift:discovery "rbac.authorization.kubernetes.io/autoupdate=false"
```

## Design Details

### Test Plan

Existing e2e test can be re-used to validate the change.

### Graduation Criteria

As this enhancement fixes a bug, graduation criteria is not applicable.

##### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

An alternate implementation is to add an alert to warn of potentially insecure bootstrap policy with instructions to fix. This would not break existing clusters and ensures that they could fix the problem. But asking customers to directly manipulate the subject fields of default policies could be harmful to cluster. A wrong edit can result in non-functional clusters.

## Infrastructure Needed [optional]
