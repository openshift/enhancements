---
title: header-case-adjustment
authors:
  - "@Miciah"
reviewers:
  - "@candita"
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@sgreene570"
approvers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
creation-date: 2020-11-25
last-updated: 2020-11-25
status: implementable
see-also:
- "enhancements/ingress/forwarded-header-policy.md"
replaces:
superseded-by:
---

# Ingress Header Case Conversion

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the cluster
administrator to specify rules for transforming the case of HTTP header names in
HTTP/1 requests.  By default, an IngressController may down-case HTTP header
names (but not HTTP header values; for example, `X-Foo: Bar` may be transformed
to `x-foo: Bar`).  This transformation is permitted by the HTTP protocol,
according to which header names are case-insensitive.  However, some
non-conformant legacy HTTP clients and servers require headers names to be
capitalized in a specific way.  This new API provides a means to accommodate
such legacy applications until they are fixed to conform to the HTTP protocol.

## Motivation

OpenShift's IngressController implementation is based on HAProxy.  HAProxy
supports both HTTP/1 and HTTP/2.  The latter requires HAProxy's "HTX" feature.
However, in addition to enabling HTTP/2, HTX also causes HAProxy to down-case
HTTP header names.

In preparation for enabling HTTP/2 support, we enabled HTX in OpenShift 4.4.  We
subsequently learned that doing so broke some legacy applications due to the
header case conversion.  Consequently we changed our HAProxy configuration to
turn off HTX unless HTTP/2 was enabled (HTTP/2 is not enabled by default).

With HAProxy 2.2, HTX becomes non-optional.  That is, we can no longer turn off
HTX to prevent HAProxy from down-casing HTTP header names.

OpenShift 4.7 and earlier include HAProxy 2.0 or earlier.  OpenShift 4.8
includes HAProxy 2.2.

To support non-conformant legacy applications that are broken by HTX, HAProxy
provides the "h1-case-adjust" option, using which it is possible to specify a
list of HTTP header names and corresponding "case adjustments".  For example, it
is possible to configure HAProxy to adjust `host: xyz.com` to `Host: xyz.com`
for HTTP/1 clients.  However, this option requires enumerating the HTTP header
names and adjustments.

The purpose of this API is to accommodate legacy applications for which the
cluster administrator knows that specific HTTP header names must have specific
case adjustments performed.  For example, if an application requires that the
`Host` header be so capitalized, the cluster administrator can configure HAProxy
with the appropriate case adjustment and then enable case adjustment for that
application.

### Goals

1. Enable the cluster administrator to specify case adjustments.
2. Enable application developers enable case adjustment for their Routes.

### Non-Goal

1. Preserve the case of HTTP header names.
2. Infer which HTTP header names require adjustment.

## Proposal

The IngressController API currently has an `HTTPHeaders` field with type
`*IngressControllerHTTPHeaders`:

```go
type IngressControllerSpec struct {
	// ...

	// httpHeaders defines policy for HTTP headers.
	//
	// If this field is empty, the default values are used.
	//
	// +optional
	HTTPHeaders *IngressControllerHTTPHeaders `json:"httpHeaders,omitempty"`
}
```

The IngressController API is extended by adding an optional
`HeaderNameCaseAdjustments` field with type
`[]IngressControllerHTTPHeaderNameCaseAdjustment`, where
`IngressControllerHTTPHeaderNameCaseAdjustment` is a string type:

```go
// IngressControllerHTTPHeaderNameCaseAdjustment is the name of an HTTP header
// (for example, "X-Forwarded-For") in the desired capitalization.  The value
// must be a valid HTTP header name as defined in RFC 2616 section 4.2.
//
// +optional
// +kubebuilder:validation:Pattern="^$|^[-!#$%&'*+.0-9A-Z^_`a-z|~]+$"
// +kubebuilder:validation:MinLength=0
// +kubebuilder:validation:MaxLength=1024
type IngressControllerHTTPHeaderNameCaseAdjustment string

// IngressControllerHTTPHeaders specifies how the IngressController handles
// certain HTTP headers.
type IngressControllerHTTPHeaders struct {
	// ...
	
	// headerNameCaseAdjustments specifies case adjustments that can be
	// applied to HTTP header names.  Each adjustment is specified as an
	// HTTP header name with the desired capitalization.  For example,
	// specifying "X-Forwarded-For" indicates that the "x-forwarded-for"
	// HTTP header should be adjusted to have the specified capitalization.
	//
	// These adjustments are only applied to cleartext, edge-terminated, and
	// re-encrypt routes, and only when using HTTP/1.
	//
	// For request headers, these adjustments are applied only for routes
	// that have the haproxy.router.openshift.io/h1-adjust-case=true
	// annotation.  For response headers, these adjustments are applied to
	// all HTTP responses.
	//
	// If this field is empty, no headers are adjusted.
	//
	// +nullable
	// +optional
	HeaderNameCaseAdjustments []IngressControllerHTTPHeaderNameCaseAdjustment `json:"headerNameCaseAdjustments,omitempty"`
}
```

By default, the IngressController converts all HTTP header names to lower-case.
The `HeaderNameCaseAdjustments` field enables the cluster administrator to
specify adjustments.  For example, the following IngressController adjusts the
`host` request header to `Host` for HTTP/1 requests to appropriately annotated
Routes and adjusts the `cache-control` response header to `Cache-Control` and
the `content-length` response header to `Content-Length` for all HTTP/1
responses:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  httpHeaders:
    headerNameCaseAdjustments:
    - Host
    - Cache-Control
    - Content-Length
```

The following Route enables HTTP response header name case adjustments using the
`haproxy.router.openshift.io/h1-adjust-case` annotation:

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  annotations:
    haproxy.router.openshift.io/h1-adjust-case: true
  name: my-application
  namespace: my-application
spec:
  to:
    kind: Service
    name: my-application
```

#### Validation

Omitting `spec.httpHeaders` or omitting
`spec.httpHeaders.headerNameCaseAdjustments` specifies the default behavior.

The API validates that the `spec.httpHeaders.headerNameCaseAdjustments[*]` field
values are valid HTTP header names as described by the field type's
`+kubebuilder:validation:Pattern` marker.

The HAProxy configuration template validates the
`haproxy.router.openshift.io/h1-adjust-case` annotation.  If the annotation
specifies an invalid value, then the IngressController ignores the annotation.

### User Stories

#### As a cluster administrator, I have a legacy server that expects the HTTP `host` header to be capitalized as `Host`.

To satisfy this use-case, the cluster administrator can specify `Host` in
`spec.httpHeaders.headerNameCaseAdjustments`:

```
oc -n openshift-ingress-operator patch ingresscontrollers/default --type=merge --patch='{"spec":{"httpHeaders":{"headerNameCaseAdjustments":["Host"]}}}'
```

The cluster administrator or application developer can annotate the
application's Route:

```
oc annotate routes/my-application haproxy.router.openshift.io/h1-adjust-case=true
```

The IngressController then adjusts the `host` request header as specified.

#### As a cluster administrator, I have a legacy client that expects the HTTP `cache-control` header to be capitalized as `Cache-Control`.

To satisfy this use-case, the cluster administrator can specify `Cache-Control`
in `spec.httpHeaders.headerNameCaseAdjustments`:

```
oc -n openshift-ingress-operator patch ingresscontrollers/default --type=json --patch='[{"op":"add","path":"/spec/httpHeaders/headerNameCaseAdjustments/-","value":"Cache-Control"}]'
```

The IngressController then adjusts the `cache-control` response header as
specified.

### Implementation Details

Implementing this enhancement required changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator
* openshift/router

The router configures HAProxy using a configuration template.  The template uses
environment variables as input parameters.  The enhancement adds a new
environment variable, `ROUTER_H1_CASE_ADJUST`, which specifies adjustments as a
comma-delimited list of header names in the desired capitalizations.  The router
parses the value of `ROUTER_H1_CASE_ADJUST` and translates it into
`h1-case-adjust` settings.  If `ROUTER_H1_CASE_ADJUST` is nonempty, the router
also specifies the `h1-case-adjust-bogus-client` setting.

For example, if `ROUTER_H1_CASE_ADJUST` is set to `Host,X-Forwarded-For`, then
the router specifies the following settings in the HAProxy configuration:

```
global
  # ...
  h1-case-adjust host Host
  h1-case-adjust x-forwarded-for X-Forwarded-For
  # ...

defaults
  # ...
  option h1-case-adjust-bogus-client
  # ...
```

When a Route has the `haproxy.router.openshift.io/h1-adjust-case=true`
annotation, the router specifies the `h1-case-adjust-bogus-server` setting on
the configuration for that Route:

```
backend ...
  option h1-case-adjust-bogus-server
```

The ingress operator translates the value of the new
`spec.httpHeaders.headerNameCaseAdjustments` API field into the format for the
`ROUTER_H1_CASE_ADJUST` environment variable and sets this environment variable
to that value in the router Deployment.

### Risks and Mitigations

OpenShift cannot infer which HTTP header names legacy clients and servers expect
to be capitalized a certain way, so cluster administrators and application
developers need to be aware of the issue and configure IngressControllers and
Routes using this new configuration when updating to OpenShift 4.8.  A release
note may mitigate this concern; for example:

"OpenShift 4.8 will update to HAProxy 2.2, which down-cases HTTP header names by
default (for example, `Host: xyz.com` is transformed to `host: xyz.com`).  If
legacy applications are sensitive to the capitalization of HTTP header names,
see the IngressController's new `spec.httpHeaders.headerNameCaseAdjustments` API
field for a solution to accommodate these legacy applications until they can be
fixed.  Make sure to add the necessary configuration using
`spec.httpHeaders.headerNameCaseAdjustments` before upgrading to OpenShift 4.8."

## Design Details

### Test Plan

The controller that manages the router Deployment and related resources has unit
test coverage; for this enhancement, the unit tests have been expanded to cover
the additional functionality.

The operator has end-to-end tests; for this enhancement, a test has been added
that (1) creates an IngressController configured with case adjustments for the
`X-Forwarded-For` request header and the `Cache-Control` response header, (2)
creates a Pod and Route for an HTTP application that echoes back requests, (3)
sends a request to the Route, and (4) verifies that the `Cache-Control` header
in the response headers and the `X-Forwarded-For` request header that is echoed
back in the response body have the expected capitalization.

### Graduation Criteria

N/A.

### Upgrade / Downgrade Strategy

On upgrade, the default configuration does not perform any case adjustments.  On
downgrade, the operator ignores the `spec.httpHeaders.headerNameCaseAdjustments`
API field, and the HAProxy configuration template ignores unrecognized
environment variables and Route annotations.

### Version Skew Strategy

N/A.

## Implementation History

## Alternatives

As an alternative to adding this API, we can add a release note telling cluster
administrators about the issue and suggesting fixing legacy applications to
follow the HTTP protocol specification.  Note that OpenShift 4.4, 4.5, 4.6, and
4.7 do not exhibit the problem unless HTTP/2 is enabled, which arguably already
gives users ample time to fix broken applications before upgrading to OpenShift
4.8.
