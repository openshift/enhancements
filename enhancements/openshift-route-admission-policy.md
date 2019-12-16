---
title: Route Admission Policy
authors:
  - "@larhauga"
  - "@chlunde"
reviewers:
  - "@openshift/api-reviewers"
  - "@openshift/sig-network-edge"
  - "@ironcladlou"
approvers:
  - "@openshift/api-approvers"
creation-date: 2019-09-25
last-updated: 2019-09-25
status: implementable
see-also:
replaces:
superseded-by:
---

# Route Admission Policy

Administrators and application developers would like to be able to run applications in multiple namespaces with the same domain name. This is for organizations where multiple teams develop microservices that are exposed on the same hostname.

With the OpenShift ingress operator, it is not possible to configure the router to accept routes from different namespaces, while the openshift-router supports this feature. 

Customers on OpenShift 3 using `ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK` are blocked from upgrading to OpenShift 4 without this RFE or disabling the default router.

This flag should only be enabled for clusters with trust between namespaces, otherwise a malicious user could take over a hostname. This should therefore be a opt-in feature.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

See the [section][#route-admission-policies]

## Motivation

See the [section][#route-admission-policies]

### Goals

1. Define an API that allows the cluster administrator to enable sharing of hostnames across namespaces.

### Non-Goals

## Proposal

Modify the existing `operator.openshift.io`'s custom resource [`IngressController`][ingress-controller-api] to include an enum field `RouteAdmissionPolicy` that can be set to `AllowInterNamespaceClaims` or `Strict`, where `Strict` is default.


### User Stories

#### Default Route Admission Policy

The default behavior is `Strict` which match the current behavior and is a secure default.


### Implementation Details/Notes/Constraints [optional]

The openshift-ingress-operator will be able to use the enum `RouteAdmissionPolicy` to enable the environment variable `ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK` in the openshift-ingress router. This is enabled when `RouteAdmissionPolicy` is set to `AllowInterNamespaceClaims`.

### Risks and Mitigations

Addition of the new field requires no migration, as the default matches the current configuration.

## Design Details

### New API

`RouteAdmissionPolicy` enum definition.

```go
// RouteAdmissionPolicy is an admission policy for allowing new route claims.
type RouteAdmissionPolicy string

const (
    // AllowInterNamespaceClaimsRouteAdmissionPolicy allows routes to claim the same hostname across namespaces.
    AllowInterNamespaceClaimsRouteAdmissionPolicy RouteAdmissionPolicy = "AllowInterNamespaceClaims"

    // StrictRouteAdmissionPolicy does not allow routes to claim the same hostname across namespaces.
    // This is the default behavior 
    StrictRouteAdmissionPolicy RouteAdmissionPolicy = "Strict"
)
```

### Migration of existing clusters

All clusters with `empty` RouteAdmissionPolicy can continue running with the current default, without setting a value.

#### Rollbacks

In case of rollbacks where AllowInterNamespaceClaims has been used, administrators would have to move routes to a shared namespace.

### Test Plan

- Create two routes in two different namespaces with the same hostname, and ensure that they are not admitted by default (`empty` or `unset`) and `Strict`.

- Create two routes in two different namespaces with the same hostname, and ensure that they are admitted when RouteAdmissionPolicy is set to `AllowInterNamespaceClaims`.

#### Migration

No migration should be required.

### Graduation Criteria

### Upgrade / Downgrade Strategy

See the [rollback section][#rollbacks]

### Version Skew Strategy

The API defines `empty` equivalent to `Strict`, therefore all the clients will have to keep that check to remain backwards compatible.

## Implementation History

## Drawbacks

This only implements one existing feature in the OpenShift router that existed in OpenShift 3.x, whereas there are many more configurations.

## Alternatives

- There are many environment variables exposed by the openshift-router which are not included in this proposal. An alternative proposal would be to allow setting environment variables directly through the ingress-controller CRD, but this would expose more feature in an uncontrolled manner. 
- This proposal implements an enum based [comment](https://github.com/openshift/api/pull/416#issuecomment-523658482), but there are only two values. This could be implemented as a boolean. There is an [existing PR](https://github.com/openshift/api/pull/416) with this implementation.

## Infrastructure Needed [optional]

[ingress-controller-api]: https://github.com/openshift/api/blob/release-4.2/operator/v1/types_ingress.go
