---
title: route-admission-policy
authors:
  - "@larhauga"
  - "@chlunde"
  - "@ironcladlou"
reviewers:
  - "@openshift/api-reviewers"
  - "@openshift/sig-network-edge"
  - "@ironcladlou"

approvers:
  - "@openshift/api-approvers"
creation-date: 2019-09-25
last-updated: 2019-12-16
status: implementable
see-also:
replaces:
superseded-by:
---

# Route Admission Policy

Administrators and application developers would like to be able to run applications in multiple namespaces with the same domain name. This is for organizations where multiple teams develop microservices that are exposed on the same host name.

In OpenShift version 4, it is not possible to configure IngressControllers to accept routes from different namespaces and the same host name.

OpenShift version 3 supports this through the `ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK` environment variable when configuring the OpenShift Router directly. Users of this feature are blocked from upgrading to OpenShift 4 unless they discontinue use of the feature.

Allowing claims across namespaces should only be enabled for clusters with trust between namespaces, otherwise a malicious user could take over a host name. This should therefore be a opt-in feature.

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

1. Define an API that allows the cluster administrator to enable sharing of host names across namespaces.

### Non-Goals

## Proposal

Add a new _RouteAdmissionPolicy_ field to the [IngressController API resource][https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go] which allows administrators to configure how cross-namespace host name claims should be handled by the IngressController.

### User Stories

### Implementation Details/Notes/Constraints

The [Ingress Operator](https://github.com/openshift/cluster-ingress-operator) will use `RouteAdmissionPolicy` to configure the environment variable `ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK` in the [OpenShift Router](https://github.com/openshift/router).

The default behavior should be to disallow inter-namespace claims, which is secure and backwards compatible with existing IngressController resources.

### Risks and Mitigations

Addition of the new field requires no migration, as the default is not changing.

## Design Details

### New API

`RouteAdmissionPolicy` enum definition.

```go
// RouteAdmissionPolicy is an admission policy for allowing new route claims.
type RouteAdmissionPolicy struct {
  // namespaceOwnership describes how host name claims across namespaces should
  // be handled. The default is Strict.
  //
  // Value must be one of:
  //
  // - Strict: do not allow routes to claim the same host name across namespaces.
  // - InterNamespaceAllowed: allow routes to claim different paths of the same host name across namespaces.
  //
  // The default is Strict.
  NamespaceOwnership NamespaceOwnershipCheck
}

// NamespaceOwnershipCheck is a route admission policy component that describes
// how host name claims across namespaces should be handled.
type NamespaceOwnershipCheck string

const (
    // InterNamespaceAllowedOwnershipCheck allows routes to claim different paths of the same host name across namespaces.
    InterNamespaceAllowedOwnershipCheck NamespaceOwnershipCheck = "InterNamespaceAllowed"

    // StrictNamespaceOwnershipCheck does not allow routes to claim the same host name across namespaces.
    StrictNamespaceOwnershipCheck NamespaceOwnershipCheck = "Strict"
)

// (Existing fields omitted, only newly proposed fields are visible)
type IngressControllerSpec struct {
  // routeAdmission defines a policy for handling new route claims (for example,
  // to allow or deny claims across namespaces).
  //
  // The default policy is (in YAML):
  //
  //     namespaceOwnership: Strict
  //
  RouteAdmission *RouteAdmissionPolicy
}

// (Existing fields omitted, only newly proposed fields are visible)
type IngressControllerStatus struct {
  // routeAdmission is the route admission policy that is in effect.
  RouteAdmission *RouteAdmissionPolicy
}
```

### Migration of existing clusters

All clusters with `empty` RouteAdmissionPolicy can continue running with the current default, without setting a value.

#### Rollbacks

In case of rollbacks where AllowInterNamespaceClaims has been used, administrators would have to move routes to a shared namespace.

### Test Plan

- Create two routes in two different namespaces with the same host name, and ensure that they are not admitted by default (`null` or unset).

- Create two routes in two different namespaces with the same host name, and ensure that they are admitted when RouteAdmissionPolicy is configured to allow it.

#### Migration

No migration should be required.

### Graduation Criteria

### Upgrade / Downgrade Strategy

See the [rollback section][#rollbacks]

### Version Skew Strategy

The API defines `empty` equivalent to `Strict`, therefore all the clients will have to keep that check to remain backwards compatible.

## Implementation History

## Drawbacks

This only implements one existing feature in the OpenShift Router that existed in OpenShift 3.x, whereas there are many more configurations.

## Alternatives

- There are many environment variables exposed by the OpenShift Router which are not included in this proposal. An alternative proposal would be to allow setting environment variables directly through the ingress-controller CRD, but this would expose more feature in an uncontrolled manner.
- This proposal implements an enum based [comment](https://github.com/openshift/api/pull/416#issuecomment-523658482), but there are only two values. This could be implemented as a boolean. There is an [existing PR](https://github.com/openshift/api/pull/416) with this implementation.

## Infrastructure Needed

[ingress-controller-api]: https://github.com/openshift/api/blob/release-4.2/operator/v1/types_ingress.go
