---
title: wildcard-admission-policy
authors:
  - "@danehans"
reviewers:
  - "@openshift/api-reviewers"
  - "@openshift/sig-network-edge"
  - "@danehans"

approvers:
  - "@openshift/api-approvers"
creation-date: 2020-03-19
last-updated: 2020-03-27
status: implementable
see-also: [route-admission-policy](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/openshift-route-admission-policy.md)
replaces:
superseded-by:
---

# Wildcard Admission Policy

Application developers require the ability to expose a service outside the cluster by multiple names without specifying
each name individually.

In OpenShift version 4, the use of wildcards in routes is not supported. The [`Route`](https://github.com/openshift/api/blob/master/route/v1/types.go)
resource supports setting a wildcard policy, but the [OpenShift Router](https://github.com/openshift/router) by default
rejects any `Route` that does so, and no API exists to enable this feature on an ingress controller (i.e. router).

OpenShift version 3 supports this feature through the `ROUTER_ALLOW_WILDCARD_ROUTES` environment variable when
configuring the OpenShift Router directly. Users of this feature are blocked from upgrading to OpenShift 4 unless they
discontinue use of the feature.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

See the [Wildcard Admission Policy section](#wildcard-admission-policy)

## Motivation

See the [Wildcard Admission Policy section](#wildcard-admission-policy)

### Goals

1. Define an API field that allows a cluster administrator to configure an ingress controller for admitting routes based
on a route's wildcard policy.
2. Provide similar wildcard route support as OpenShift version 3.

### Non-Goals

1. Redesign how wildcard support is implemented in [OpenShift Router](https://github.com/openshift/router).
2. Introduce a separate API from `RouteAdmissionPolicy` for managing route admission based on a route's wildcard
policy.

## Proposal

Introduce a `WildcardPolicy` field to the [IngressController](https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go)
resource that allows administrators to manage route admission based on the route's wildcard policy configuration.

### User Stories

As a cluster administrator, I need the ability to admit routes based on the route's wildcard policy configuration.

### Implementation Details/Notes/Constraints [optional]

The [Ingress Operator](https://github.com/openshift/cluster-ingress-operator) will use `WildcardPolicy` to configure
the environment variable `ROUTER_ALLOW_WILDCARD_ROUTES` in the [OpenShift Router](https://github.com/openshift/router).

The default behavior of an ingress controller is to admit routes with a wildcard policy of None, which is backwards
compatible with existing `IngressController` resources.

### Risks and Mitigations

Addition of the new field requires no mitigation, as the default is not changing.

## Design Details

### New API Field

`WildcardPolicy` enum definition.

```go
// (Existing fields omitted, only newly proposed fields are visible)
type RouteAdmissionPolicy struct {
	// wildcardPolicy describes how routes with wildcard policies should
	// be handled for the ingress controller. WildcardPolicy controls use
	// of routes [1] exposed by the ingress controller based on the route's
	// wildcard policy.
	//
	// [1] https://github.com/openshift/api/blob/master/route/v1/types.go
	//
	// Note: Updating WildcardPolicy from WildcardsAllowed to WildcardsDisallowed
	// will cause admitted routes with a wildcard policy of Subdomain to stop
	// working. These routes must be updated to a wildcard policy of None to be
	// readmitted by the ingress controller.
	//
	// If empty, defaults to "WildcardsDisallowed".
	//
	WildcardPolicy WildcardPolicy `json:"wildcardPolicy,omitempty"`
}

// WildcardPolicy is a route admission policy component that describes how
// routes with a wildcard policy should be handled.
// +kubebuilder:validation:Enum=WildcardsAllowed;WildcardsDisallowed
type WildcardPolicy string

const (
	// WildcardPolicyAllowed indicates routes with any wildcard policy are
	// admitted by the ingress controller.
	WildcardPolicyAllowed WildcardPolicy = "WildcardsAllowed"

	// WildcardPolicyDisallowed indicates only routes with a wildcard policy
	// of None are admitted by the ingress controller.
	WildcardPolicyDisallowed WildcardPolicy = "WildcardsDisallowed"
)

// (Existing type and field, included for completeness)
type IngressControllerSpec struct {
  // routeAdmission defines a policy for handling new route claims (for example,
  // to allow or deny claims across namespaces).
  //
  // If empty, defaults will be applied. See specific routeAdmission fields
  // for details about their defaults.
  //
  // +optional
  RouteAdmission *RouteAdmissionPolicy `json:"routeAdmission,omitempty"`
}
```

### Migration of existing clusters

All `IngressControllers` with an empty `WildcardPolicy` can continue running with the current default without setting a
value for the field.

#### Rollbacks

Rolling-back an ingress controller from WildcardsAllowed to WildcardsDisallowed will cause previously admitted routes
with `wildcardPolicy: Subdomain` to no longer work even though their status indicates otherwise. These routes can not be
updated since `wildcardPolicy` is immutable. Instead, these routes must be deleted and created with
`wildcardPolicy: None`.

### Test Plan

Test Case 1:
- Create the default ingress controller which contains a nil `wildcardPolicy`. An ingress controller with an undefined
`wildcardPolicy` defaults to `WildcardsDisallowed`.
- Create a route without defining a wildcard policy. Since the default behavior of a route is `wildcardPolicy: None`,
 ensure the route is admitted.
- Create a route with `wildcardPolicy: Subdomain` and ensure that the route is not admitted.

Test Case 2:
- Create an ingress controller with `WildcardPolicy: WildcardsAllowed`
- Create two routes, each with a different wildcard policy and ensure that both are admitted.

#### Migration

- Create an ingress controller with `wildcardRouting: WildcardsAllowed`.
- Create a route that specifies `wildcardPolicy: Subdomain` and ensure that it is admitted.
- Update the ingress controller with `wildcardRouting: WildcardsDisallowed`.
- Verify that the route status still indicates `type: Admitted` and `status: "True"`. Note that this route no longer
works even though the status indicates otherwise.
- Delete the route that specifies `wildcardPolicy: Subdomain` and recreate it with `wildcardPolicy: None`.

### Graduation Criteria

### Upgrade / Downgrade Strategy

See the [rollback section][#rollbacks]

### Version Skew Strategy

The API defines `empty` equivalent to `WildcardsDisallowed`, therefore all the clients will have to keep that check to
remain backwards compatible.

## Implementation History

## Drawbacks

Same as [Route Admission Policy](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/openshift-route-admission-policy.md#drawbacks)

## Alternatives

Same as [Route Admission Policy](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/openshift-route-admission-policy.md#alternatives)
