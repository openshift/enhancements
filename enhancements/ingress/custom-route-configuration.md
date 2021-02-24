---
title: custom-route-configuration
authors:
  - "@deads2k"
reviewers:
  - "@danmace"
  - "@miciah"
  - "@standa"
  - "@spadgett"
  - "@sur"
  - "@lilic"
approvers:
  - "@sttts"
  - "@miciah"
creation-date: 2021-01-08
last-updated: 2021-01-08
status: implementable
see-also:
replaces:
superseded-by:
---

# Custom Route Configuration

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add `CustomizeableRoute{Spec,Status}` to `IngressSpec` in a way that allows multiple operators to provide information about
Routes (or Ingresses, or maybe even something else), that a cluster-admin wants to provide custom names and/or custom
serving certificates for.

## Motivation

Some customers do not allow wildcard serving certificates for the openshift ingress to use, so they have to provide individual
serving certificates to each component.
Those customers need a way to see all the routes they need to customize and a way to configure them.
A stock installation has the following routes which may need customization of both names and serving cert/keys
1. openshift-authentication   oauth-openshift
2. openshift-console          console
3. openshift-console          downloads
4. openshift-monitoring       alertmanager-main
5. openshift-monitoring       grafana
6. openshift-monitoring       prometheus-k8s
7. openshift-monitoring       thanos-querier
8. image-registry (this isn't appearing in my installation, but @Miciah says it's there)

It is just as easy to create a generic solution that provides one stop shopping for cluster-admins as it is to produce
a series of one-off solutions.
This proposes a single point of configuration because we like cluster-admins and we dislike writing multiple pages of
slightly different documentation.

### Goals

1. provide a way for cluster-admins to see all the routes that have flexible names and serving certificates in the cluster.
2. provide a single API that is consistent for every route they need to configure
3. provides a way to have an operator with scope limited permissions read the serving cert/key pairs
4. allow a cluster-admin to specify a different DNS name.

### Non-Goals

1. provide a way to specify SNI serving certificates for operands.
2. dictate how an operator must expose itself, the termination policy on a route, or how the secret is used.

## Proposal

```go
type IngressSpec struct {
    // other fields

    // ComponentRoutes is a list of routes that a cluster-admin wants to customize.  It is logically keyed by
    // .spec.componentRoutes[index].{namespace,name}.
    // To determine the set of possible keys, look at .status.componentRoutes where participating operators place
    // current route status keyed the same way.
    // If a ComponentRoute is created with a namespace,name tuple that does not match status, that piece of config will
    // not have an effect.  If an operator later reads the field, it will eventually (but not necessarily immediately)
    // honor the pre-existing spec values.
    ComponentRoutes []ComponentRouteSpec
}

type IngressStatus struct {
    // other fields

    // ComponentRoutes is where participating operators place the current route status for routes which the cluster-admin
    // can customize hostnames and serving certificates.
    // How the operator uses that serving certificate is up to the individual operator.
    // An operator that creates entries in this slice should clean them up during removal (if it can be removed).
    // An operator must also handle the case of deleted status without churn.
    ComponentRoutes []ComponentRouteStatus
}

type ComponentRouteSpec struct{
    // namespace is the namespace of the route to customize.  It must be a real namespace.  Using an actual namespace
    // ensures that no two components will conflict and the same component can be installed multiple times.
    Namespace string
    // name is the *logical* name of the route to customize.  It does not have to be the actual name of a route resource.
    // Keep in mind that this is your API for users to customize.  You could later rename the route, but you cannot rename
    // this name.
    Name string
    // Hostname is the host name that a cluster-admin wants to specify
    Hostname string
    // ServingCertKeyPairSecret is a reference to a secret in namespace/openshift-config that is a kubernetes tls secret.
    // The serving cert/key pair must match and will be used by the operator to fulfill the intent of serving with this name.
    // That means it could be embedded into the route or used by the operand directly.
    // Operands should take care to ensure that if they use passthrough termination, they properly use SNI to allow service
    // DNS access to continue to function correctly.
    // Operators are not required to inspect SANs in the certificate to set up SNI.
    ServingCertKeyPairSecret SecretNameReference

    // possible future, we could add a set of SNI mappings.  I suspect most operators would not properly handle it today.
}

type ComponentRouteStatus struct{
    // namespace is the namespace of the route to customize.  It must be a real namespace.  Using an actual namespace
    // ensures that no two components will conflict and the same component can be installed multiple times.
    Namespace string
    // name is the *logical* name of the route to customize.  It does not have to be the actual name of a route resource.
    // Keep in mind that this is your API for users to customize.  You could later rename the route, but you cannot rename
    // this name.
    Name string
    // defaultHostname is the normal host name of this route.  It is provided in case cluster-admins find it more recognizeable
    // and having it here makes it possible to answer, "if I remove my configuration, what will the name be".
    DefaultHostname string
    // ConsumingUsers is a slice of users that need to have read permission on the secrets in order to use them.
    // This will usually be an operator service account.
    ConsumingUsers []string
    // currentHostnames is the current name used to by the route.  Routes can have more than one exposed name, even though we
    // only allow one route.spec.host.
    CurrentHostnames []string

    // conditions are degraded and progressing.  This allows consistent reporting back and feedback that is well
    // structured.  These particular conditions have worked very well in ClusterOperators.
    // Degraded == true means that something has gone wrong trying to handle the ComponentRoute.  The CurrentHostnames
    // may or may not be operating successfully.
    // Progressing == true means that the component is taking some action related to the ComponentRoute
    Conditions []ConfigCondition

    // relatedObjects allows listing resources which are useful when debugging or inspecting how this is applied.
    // They may be aggregated into an overall status RelatedObjects to be automatically shown by oc adm inspect
    RelatedObjects []ObjectReference
    
    

    // This API does not include a mechanism to distribute trust, since the ability to write this resource would then 
    // allow interception.  Instead, if we need such a mechanism, we can talk about creating a way to allow narrowly scoped
    // updates to a configmap containing ca-bundle.crt for each ComponentRoute.
    // CurrentCABundle []byte
}
```

Validation rules to be specified in the openshift/api PR, but basic restrictions on strings as you'd expect.


### User Stories

#### Cluster-admin wants to customize a route
To use this, a cluster-admin would
1. Either read docs to find the namespace, name tuples or look at an existing cluster and read ingresses.config.openshift.io.
2. Create the serving cert/key pair secret in ns/openshift-config
3. Create an entry in `ingresses.config.openshift.io.spec.componentRouteSpec` for their customization
4. Wait to see the corresponding change in status.


#### Story 2

### Implementation Details/Notes/Constraints [optional]

#### Control loop to manage secret read permissions in openshift-config
List/watch for individual names has been possible for several releases.
A control loop in either the ingress operator or the cluster config operator will watch for ingress.spec.componentRouteSpec.servingCertKeyPairSecret
and will create a role/rolebinding for the corresponding ingress.status.componentRouteStatus.consumingUsers.
This will allow an operator (or other binary with that user) to get/list/watch on the particular secret.

This means that the power to mutate ingresses.config.openshift.io and ingresses.config.openshfit.io/status
will imply the power to read secrets in openshift-config.
If we decide we do not wish to do this, then it will incumbent upon the cluster-admin to create the role and rolebinding.

#### library-go usage
Because this will be leveraged by the authentication operator (and possibly the console operator), we can provide a config
observer that reads the ingresses.spec, handles the specified names, sets up the specific list/watch, and takes care of
copying the secret so it can be mounted.

It would also be possible (though I'm less sure someone would use it), to provide a route injection option for serving cert/key
pairs.

Library-go is also an ideal spot to provide functionality to inject route hosts.  Something like our StaticResource
controller which automatically handles the API status and the required route changes.

### Risks and Mitigations

Some components have attempted to resolve similar issues in one off ways.
This has produced a disjointed experience across the stack and challenging documentation to follow.
Since these other ways exist, a migration to a unified mechanism may be challenging for those components.
The helpers suggested above can ease that transition, but it still exists.

## Design Details

### Open Questions [optional]

1. Which components should migrate?
2. What timeline should that migration happen?
3. Which components can start from this unified API?
4. Do we allow OLM operators to participate?

### Test Plan

1. an operator test should be written for this
2. QE testing on configuration changes.

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

For components that did not try to solve this in a one-off way, the upgrade is easy, the new feature becomes available.

For components that created a one-off solution, the upgrade will vary depending on the steps they used.

On downgrade, the customizations will be removed by the old versions of the operators.
While this is annoying, it is consistent with our general downgrade story of new features require the new product.

### Version Skew Strategy

Until the ingress operator or cluster config operator is upgraded, the role and rolebinding granting read permission to the
serving secrets won't be running.
This means that the feature will not function until the entire cluster is upgraded.
This is consistent with our general versioning story where new features cannot be used until the cluster is fully upgraded.

## Implementation History

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

### cluster-admin created roles and rolebindings
This avoids the transitive privilege escalation to read secrets based on update rights to ingresses.config.openshift.io
and ingresses.config.openshift.io/status.
However, those permissions are closely held.

### continue creating one-off configuration options
This could be done, though it is likely the authentication operator would choose to create the API described here to do it.
It would also leave customers going through a large list in documentation of different ways to configure exposed routes
and hoping they get them all.
On upgrades, new routes could become available and they would have to hunt those down too.

## Infrastructure Needed [optional]

Nothing new, unless we want to create a "configuration changes" style job.