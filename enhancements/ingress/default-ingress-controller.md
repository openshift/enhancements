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
creation-date: 2020-04-06
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
- [ ] Design details are appropriately documented from clear requirements
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

### Non-Goals


## Proposal

The proposal spans two components: the ingress-operator, and the installer.

The ingress-operator should be modified to stop creating the `default` IngressController resource.

Currently, the installer only renders the `default` IngressController resource when the cluster is configured as "internal". The installer should be changed to _always_ render the `default` IngressController.

### User Stories

As an administrator, I want to change the `default` IngressController from public to private as a day-2 operation.

As an administrator, I want to change the `default` IngressController publishing strategy from LoadBalancerService to HostNetwork.

### Implementation Details/Notes/Constraints [optional]


### Risks and Mitigations


## Design Details

### New API Field


### Migration of existing clusters



#### Rollbacks


### Test Plan


#### Migration

### Graduation Criteria

### Upgrade / Downgrade Strategy


### Version Skew Strategy

## Implementation History

## Drawbacks

Same as [Route Admission Policy](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/openshift-route-admission-policy.md#drawbacks)

## Alternatives

TODO: Package the default IngressController manifest annotated with `release.openshift.io/create-only=true` into the ingress-operator payload.
