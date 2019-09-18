---
title: Cluster Publishing Strategy
authors:
  - "@abhinavdahiya"
reviewers:
  - "@openshift/api-reviewers"
  - "@openshift/openshift-team-installer"
  - "@ironcladlou"
approvers:
  - "@openshift/api-approvers"
creation-date: 2019-09-18
last-updated: 2019-09-18
status: implementable
see-also:
replaces:
superseded-by:
---

# Cluster Publishing Strategy

Administrators would like to be able to install OpenShift cluster as `internal/private`, which is only accessible from within the network, VPN tunnels and, not visible to the Internet. Therefore all the cluster operators that create endpoints for the cluster would need a place to discover this intent of the administrator and ensure that their resources are only accessible from within the network, VPN tunnels and not visible to the Internet by default.

This should not block administrators from using operator-level configuration to override publishing strategy of individual endpoints.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

See the [section][#cluster-publishing-strategy]

## Motivation

See the [section][#cluster-publishing-strategy]

### Goals

1. Define an API that allows all the cluster operators to discover the default cluster publishing strategy.

### Non-Goals

## Proposal

Modify the existing `config.openshift.io`'s custom resource [`Infrastructure`][infrastructure-api] to include an enum field `Publish` that can be set to `External` or `Internal`.

The field will be added to [`status` section][infrastructure-api-status] because the goal is to provide an API for discovery i.e. the current state, and not to provide an API for customers to specify the desired state.

### User Stories

#### Default Ingress Controller LoadBalancer Scope

The ingress-operator will be able to use this field to define the **default** [scope][ingress-controller-api-load-balancer-scope] for the LoadBalancers for the [ingress controllers][ingress-controller-api].

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

Addition of the new field requires migration of all the existing clusters. Since we have only supported `External` strategy for OpenShift, the migration should be simple enough.

## Design Details

### New API

`PublishingStrategy` enum definition.

```go
// PublishingStrategy is a strategy for how various endpoints for the cluster are exposed.
type PublishingStrategy string

const (
    // ExternalPublishingStrategy exposes endpoints for the cluster to the Internet
    ExternalPublishingStrategy PublishingStrategy = “External”
    // InternalPublishingStrategy exposes the endpoints for the cluster to the private network only.
    InternalPublishingStrategy PublishingStrategy = “Internal”
)
```

Updating the `InfrastructureStatus` object to add `Publish` field.

```go
type InfrastructureStatus struct {
...

// Publish controls how the user facing endpoints of the cluster like the k8s API, OpenShift routes etc. are exposed.
// When no strategy is specified, the strategy is `External`.
// +optional
Publish PublishingStrategy `json:”publish,omitempty”`

...

}
```

### Migration of existing clusters

All clusters with `empty` PublishingStrategy can be updated to `External`, while all non-empty values will be kept untouched.

#### Rollbacks

In case of rollbacks after the migration has been done, the value `External` will be accurate.

### Test Plan

#### Migration

An e2e test for ensuring the migration is correctly performed during upgrades from 4.2 will be required.

### Graduation Criteria

### Upgrade / Downgrade Strategy

See the [migration section][#migration-of-existing-clusters]

### Version Skew Strategy

The API defines `empty` equivalent to `External`, therefore all the clients will have to keep that check to remain backwards compatible.

## Implementation History

## Drawbacks

## Alternatives

- Because the ingres-operator is currently the only user of this API field, the `PublishingStrategy` could also be added to the [`Ingress`][ingress-api] custom resource. But because the strategy affects more than the ingress, like the
Network security filters on clouds, this field is more suited for `Infrastructure` custom resource in the long term.

## Infrastructure Needed [optional]

[infrastructure-api]: https://github.com/openshift/api/blob/release-4.2/config/v1/types_infrastructure.go
[infrastructure-api-status]: https://github.com/openshift/api/blob/release-4.2/config/v1/types_infrastructure.go#L35
[ingress-api]: https://github.com/openshift/api/blob/release-4.2/config/v1/types_ingress.go
[ingress-controller-api]: https://github.com/openshift/api/blob/release-4.2/operator/v1/types_ingress.go
[ingress-controller-api-load-balancer-scope]: https://github.com/openshift/api/blob/release-4.2/operator/v1/types_ingress.go#L174
