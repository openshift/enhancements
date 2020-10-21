---
title: default-apps-domain
authors:
  - "@dustman9000"
reviewers:
  - "@Miciah"
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@sgreene570"
approvers:
  - "@knobunc"
  - "@Miciah"
creation-date: 2020-08-12
last-updated: 2020-09-04
status: implementable
---

# Default Apps Domain

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement allows for the configuration of a default apps domain, which
can be used to specify an alternative default domain for user-created routes.
This alternative domain, if specified, overrides the default cluster domain for the
purpose of determining the default host for a newly created route.

## Motivation

Many customers have requested "Custom Domains" in which they would like an already
installed Openshift cluster to support additional domains for their apps by default.
This domain is owned by a customer in which they would point a wildcard CNAME record
to an existing well known CNAME record.

### Goals

1. Enable the cluster administor to specify the default apps domain separate
from the installed cluster domain.

## Proposal

The Ingress Config API is extended by adding an optional `AppsDomain` field with type
`string`.

```go
type IngressSpec struct {
    ...
    // appsDomain is an optional domain to use instead of the one specified
    // in the domain field when a Route is created without specifying an explicit
    // host.  If appsDomain is nonempty, this value is used to generate default
    // host values for Route.  Unlike domain, appsDomain may be modified after
    // installation.
    // This assumes a new ingresscontroller has been setup with a wildcard
    // certificate.
    // +optional
    AppsDomain string `json:"appsDomain,omitempty"`
}
```

The following example configures apps.acme.io as the default apps domain.

```yaml
apiVersion: config.openshift.io/v1
kind: Ingress
metadata:
  name: cluster
spec:
  domain: apps.drow-dev01.w9h5.s1.openshiftapps.com
  appsDomain: apps.acme.io
```

### Implementation Details

Implementation of this enhancement requires changes in the following repositories:

* openshift/api
* openshift/cluster-openshift-apiserver-operator

In [config/v1/types_ingress.go](https://github.com/openshift/api/blob/master/config/v1/types_ingress.go) of [openshift/api](https://github.com/openshift/api), add new field: `appsDomain`.
In [observe_ingresses.go](https://github.com/openshift/cluster-openshift-apiserver-operator/blob/master/pkg/operator/configobservation/ingresses/observe_ingresses.go) of [openshift/cluster-openshift-apiserver-operator](https://github.com/openshift/cluster-openshift-apiserver-operator), add logic to check if `appsDomain` is set. If that field is set, use it instead of `spec.Domain` for the value of `routingDomain`.

### Risks and Mitigations

Certain routes are created by various operators at install time. These routes
could be potentially be deleted and re-added post-install time. Implementation
must ensure these routes use the original domain when re-added.

## Design Details

### Test Plan

In the cluster-ingress-operator e2e test, add the following to ensure the following steps are successful:

1. Create an IngressController with wildcard domain (e.g. `*.apps.acme.io`) and wildcard
certificate.
2. Create or modify the `cluster` Ingress, setting the test domain to the `appsDomain`
field.
3. Create a pod with a simple HTTP application that sends a static response (e.g. hello-openshift).
4. Create route for this application (without specifing --host).
5. Ensure new route HOST/PORT uses the appsDomain set in step 2, rather than the cluster domain.
6. Send request to this new route.
7. Verify there is a valid response with no TLS errors.

### Upgrade / Downgrade Strategy

On upgrade, the value of appsDomain will be empty, so the route host defaulting behavior will remain unchanged.

On downgrade, the value of appsDomain will no longer be used for route host defaulting, but existing routes will remain unchanged (nothing automatically modifies spec.host on the route once the route is created).

### Version Skew Strategy

N/A.

## Implementation History

## Alternatives
