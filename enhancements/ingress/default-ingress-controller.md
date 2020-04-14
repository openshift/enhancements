---
title: default-ingress-controller
authors:
  - "@ironcladlou"
reviewers:
  - "@openshift/api-reviewers"
  - "@openshift/sig-network-edge"
  - "@ironcladlou"

approvers:
  - "@openshift/api-approvers"
creation-date: 2020-04-06
last-updated: 2020-04-14
status: implementable
see-also: [user-defined-default-ingress-controller](user-defined-default-ingress-controller.md)
replaces:
superseded-by:
---

# Default Ingress Controller Changes

Today, unless the user explicitly [provides one at installation time](user-defined-default-ingress-controller.md), OpenShift's `default` IngressController resource is created by the ingress-operator. Importantly, ingress-operator will periodically check for the existence of the `default` instance, creating the resource if it does not exist. While this is useful to ensure some baseline ingress functionality within the cluster even in the event of the default resource being deleted, the current behavior is at odds with the common administrative practice of replacing the `default` instance as a day 2 operation. For example, to change the publishing strategy of the IngressController after installation, the resource must be replaced, causing a race between the administrator and the operator. This contention frustrates automation and can even cost resources if the operator wins (by creating unnecessary cloud resources like load balancers, public IPs, and DNS records.)

This proposal eliminates the contention between the operator and administrators.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This proposal eliminates the operator/administrator contention by building on the [User Defined IngressControllers](user-defined-default-ingress-controller.md) capability. Specifically, by:

1. Changing the ingress-operator to stop creating ingresscontrollers altogether
2. Changing the installer to always render a `default` IngressController

These changes make explicit the notion that the ingress-operator is responsible only for managing IngressControllers created by end users, _not_ for creating IngressControllers.

End users can now assume that the _only_ way IngressControllers are created are via the installer or through their own API calls.

## Motivation

The operator today has a confusing relationship to the `default` IngressController resource. Not only does the operator confound automation in its competition with users for ownership, but the operator is not even guaranteed to have the context necessary to restore any IngressController that was deleted, including the `default` instance. The operator should have a clear responsibility to manage resources brought by the user — the installer being indistinguishable from a user in this context. The installer should have a clear responsibility to always bring the `default` IngressController resource.

### Goals

1. Eliminate contention with users/automation trying to replace the default IngressController post-installation.
2. Clarify the role of the ingress-operator with respect to IngressController resource creation
3. Clarify the role of the installer with respect to the default IngressController resource
4. Provide a well-known file for end-users to modify the default IngressController prior to installation

### Non-Goals

1. This proposal does not introduce any decision-making into the installer with regards to the shape of the default public IngressController resource. The ingress-operator is still responsible for defaulting the spec field.

## Proposal

The proposal spans two components: the ingress-operator, and the installer.


### Ingress Operator

The ingress-operator should be modified to stop creating the `default` IngressController resource.

Because the ingress-operator will no longer ensure a default IngressController resource, status reporting will be enhanced to help alert administrators when the `default` IngressController is missing. The following new ingress operator conditions are proposed:

    MissingDefaultIngressController=[True, False]: True when the `ingresscontrollers/default` resource in the `openshift-ingress-operator` namespace is not found.

When `MissingDefaultIngressController=True`, the operator status should report `Degraded=True` and a `Reason` which cites the `MissingDefaultIngressController` condition.

### Installer

Currently, the installer only renders the `default` IngressController resource when the cluster is configured as "internal". The installer should be changed to _always_ render the `default` IngressController.

The default public IngressController rendered by the installer is the minimum possible resource to make the cluster work. The ingress-operator will handle defaulting to make ingress work out of the box on the platform. The resource the installer will create should look like this:

```
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec: {}
```

### User Stories

As an administrator, I want to change the `default` IngressController from public to private as a day-2 operation.

As an administrator, I want to change the `default` IngressController publishing strategy from LoadBalancerService to HostNetwork.

### Implementation Details/Notes/Constraints

N/A

### Risks and Mitigations


## Design Details

### New API Field

No new API fields are proposed.

### Migration of existing clusters



#### Rollbacks


### Test Plan


#### Migration

### Graduation Criteria

### Upgrade / Downgrade Strategy


### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

* Package the default IngressController manifest annotated with `release.openshift.io/create-only=true` into the ingress-operator payload.

This approach cedes control of the default resource to the cluster-version-operator (CVO). If the user deletes the default instance, the [CVO will try re-creating it](https://github.com/openshift/cluster-version-operator/blob/master/pkg/cvo/internal/generic.go#L44-L47). This reintroduces a race between the user and the CVO and thus does not solve the core problem.
