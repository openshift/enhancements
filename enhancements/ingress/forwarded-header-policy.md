---
title: forwarded-header-policy
authors:
  - "@Miciah"
reviewers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@sgreene570"
approvers:
  - "@knobunc"
creation-date: 2020-06-10
last-updated: 2020-11-07
status: implemented
see-also:
- "enhancements/ingress/openshift-route-admission-policy.md"
replaces:
superseded-by:
---

# Ingress Forwarded Header Policy

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [X] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the user to specify
a policy for how an IngressController handles the `Forwarded`,
`X-Forwarded-For`, and related HTTP headers.  By default, the IngressController
appends these headers to an HTTP request, preserving any existing headers,
before forwarding the request to the backend application.  The proposed API
extension provides additional options to replace any existing `Forwarded` or
related headers, to preserve the headers if they are already present or else add
them, or never to set the headers.  The enhancement also defines an annotation
that can be set on Routes to override an IngressController's policy.

Note: The behavior prior to this enhancement was inconsistent: the
IngressController appended the `Forwarded` and `X-Forwarded-For` headers and
replaced the `X-Forwarded-Host`, `X-Forwarded-Port` `X-Forwarded-Proto`, and
`X-Forwarded-Proto-Version` headers.  This inconsistency was unintentional and
can be considered a defect, and the enhancement corrects this defect by making
the behavior for all of these headers consistent.

## Motivation

In OpenShift 3, cluster administrators could use a custom HAProxy configuration
template to customize how and when a router set the `Forwarded` and related
headers, but using a custom template is not permitted in OpenShift 4.

Applications that receive HTTP requests through the IngressController may need
the client's source address for compliance reasons or other reasons.  When a
cluster has an external load-balancer (which is common for bare-metal
environments), and the load balancer injects the `Forwarded` or
`X-Forwarded-For` HTTP headers, the cluster administrator typically wants
applications to use these headers rather than headers that the IngressController
injects.

### Goals

1. Enable the cluster administrator to control how the IngressController sets `Forwarded` and related HTTP headers.
2. Enable application developers to override an IngressController's handling of `Forwarded` and related headers on a per-Route basis.
3. Accommodate future additions of configuration related to HTTP headers.

### Non-Goal

1. Providing control over HTTP headers other than `Forwarded`, `X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Port` `X-Forwarded-Proto`, and `X-Forwarded-Proto-Version`.

## Proposal

The IngressController API is extended by adding an optional `HTTPHeaders` field
with type `*IngressControllerHTTPHeaders` to `IngressControllerSpec`:

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

The `IngressControllerHTTPHeaders` type has an optional `ForwardedHeaderPolicy`
field, which has type `IngressControllerHTTPHeaderPolicy` and may have one of
the values "Append", "Replace", "IfNone", or "Never", for specifying how the
IngressController handles `Forwarded` and related headers:

```go
// IngressControllerHTTPHeaderPolicy is a policy for setting HTTP headers.
//
// +kubebuilder:validation:Enum=Append;Replace;IfNone;Never
type IngressControllerHTTPHeaderPolicy string

const (
	// AppendHTTPHeaderPolicy appends the header, preserving any existing header.
	AppendHTTPHeaderPolicy IngressControllerHTTPHeaderPolicy = "Append"
	// ReplaceHTTPHeaderPolicy sets the header, removing any existing header.
	ReplaceHTTPHeaderPolicy IngressControllerHTTPHeaderPolicy = "Replace"
	// IfNoneHTTPHeaderPolicy sets the header if it is not already set.
	IfNoneHTTPHeaderPolicy IngressControllerHTTPHeaderPolicy = "IfNone"
	// NeverHTTPHeaderPolicy never sets the header, preserving any existing
	// header.
	NeverHTTPHeaderPolicy IngressControllerHTTPHeaderPolicy = "Never"
)

// IngressControllerHTTPHeaders specifies how the IngressController handles
// certain HTTP headers.
type IngressControllerHTTPHeaders struct {
	// forwardedHeaderPolicy specifies when and how the IngressController
	// sets the Forwarded, X-Forwarded-For, X-Forwarded-Host,
	// X-Forwarded-Port, X-Forwarded-Proto, and X-Forwarded-Proto-Version
	// HTTP headers.  The value may be one of the following:
	//
	// * "Append", which specifies that the IngressController appends the
	//   headers, preserving existing headers.
	//
	// * "Replace", which specifies that the IngressController sets the
	//   headers, replacing any existing Forwarded or X-Forwarded-* headers.
	//
	// * "IfNone", which specifies that the IngressController sets the
	//   headers if they are not already set.
	//
	// * "Never", which specifies that the IngressController never sets the
	//   headers, preserving any existing headers.
	//
	// By default, the policy is "Append".
	//
	// +optional
	ForwardedHeaderPolicy IngressControllerHTTPHeaderPolicy `json:"forwardedHeaderPolicy,omitempty"`
}
```

By default, the IngressController appends `Forwarded` and related headers to
whatever headers are already present in the HTTP request.  The
`ForwardedHeaderPolicy` field enables the user to modify this behavior, as
described in the godoc above.  For example, the following IngressController
replaces any `Forwarded` and related headers before forwarding the request:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  httpHeaders:
    forwardedHeaderPolicy: Replace
```

Routes may override this policy using the
`haproxy.router.openshift.io/set-forwarded-headers` annotation, which allows the
values "append", "replace", "if-none", and "never", corresponding to the allowed
values for the IngressController's `spec.httpHeaders.forwardedHeaderPolicy`
field.  For example, an IngressController should never set the `Forwarded`
header or related headers for the following Route, irrespective of the
IngressController's own particular policy:

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  annotations:
    haproxy.router.openshift.io/set-forwarded-headers: never
  name: hello-openshift
  namespace: hello-openshift
spec:
  to:
    kind: Service
    name: hello-openshift
```

#### Validation

Omitting `spec.httpHeaders` or omitting `spec.httpHeaders.forwardedHeaderPolicy`
specifies the default behavior.

The API validates the `spec.httpHeaders.forwardedHeaderPolicy` field value as
described by the field type's `+kubebuilder:validation:Enum` marker.

The HAProxy configuration template validates the
`haproxy.router.openshift.io/set-forwarded-headers` annotation.  If the
annotation specifies an invalid value, then the IngressController uses its
configured policy.

### User Stories

#### As a cluster administrator, I have configured an external proxy that injects the `X-Forwarded-For` header into each request before forwarding it to an IngressController, and I want the IngressController to pass the header through unmodified.

To satisfy this use-case, the cluster administrator can specify the "Never"
policy.  The IngressController then never sets the headers, and applications
receive only the headers that the external proxy provides.

#### As a cluster administrator, I want an IngressController to pass the `X-Forwarded-For` header that my external proxy sets on extra-cluster requests through unmodified, and I want the IngressController to set the `X-Forwarded-For` header on intra-cluster requests, which do not go through the external proxy.

To satisfy this use-case, the cluster administrator can specify the "IfNone"
policy.  If an HTTP request already has the header (presumably set by the
external proxy), the IngressController preserves it.  If the header is absent
because the request did not come through the proxy, then the IngressController
adds the header.

#### As an application developer, I have configured an application-specific external proxy that injects the `X-Forwarded-For` header, and I want an IngressController to pass the header through unmodified for my application's Route, without affecting the policy for other Routes.

To satisfy this use-case, the application developer can add an annotation
`haproxy.router.openshift.io/set-forwarded-headers: if-none` or
`haproxy.router.openshift.io/set-forwarded-headers: never` on the Route for the
application.

### Implementation Details

Implementing this enhancement required changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator
* openshift/router

The router configures HAProxy using a configuration template.  The template uses
environment variables as input parameters.  The enhancement adds a new
environment variable, `ROUTER_SET_FORWARDED_HEADERS`, which specifies the policy
using one of the values "append", "replace", "if-none", or "never".  The ingress
operator sets this variable on the router Deployment based on the
IngressController's `spec.httpHeaders.forwardedHeaderPolicy` field value.

HAProxy has an `option forwardfor` configuration keyword to specify when and how
HAProxy sets the `X-Forwarded-For` header, as well as `http-request add-header`
and `http-request set-header` keywords to append or replace arbitrary headers.
The `option forwardfor` keyword has an associated `if-none` keyword, which can
be used to implement the "IfNone" IngressController policy.  The `http-request
set-header` keyword can be conditionalized using ACLs to implement "IfNone".
These keywords are sufficient to implement all four policies.

The HAProxy configuration template generates a backend configuration block for
each Route based on the value in the Route's annotation or the value of
`ROUTER_SET_FORWARDED_HEADERS`.  To implement `append` for the `X-Forwarded-For`
header, the template specifies `option forwardfor`; for the other HTTP headers,
the `http-request add-header` keyword can be used:

```conf
  option forwardfor
  http-request add-header X-Forwarded-Host %[req.hdr(host)]
  http-request add-header X-Forwarded-Port %[dst_port]
  http-request add-header X-Forwarded-Proto http if !{ ssl_fc }
  http-request add-header X-Forwarded-Proto https if { ssl_fc }
  http-request add-header X-Forwarded-Proto-Version h2 if { ssl_fc_alpn -i h2 }
  http-request add-header Forwarded for=%[src];host=%[req.hdr(host)];proto=%[req.hdr(X-Forwarded-Proto)]
```

To implement the "Replace" policy, the template uses the `http-request
set-header` keyword:

```conf
  http-request set-header X-Forwarded-For %[src]
  http-request set-header X-Forwarded-Host %[req.hdr(host)]
  http-request set-header X-Forwarded-Port %[dst_port]
  http-request set-header X-Forwarded-Proto http if !{ ssl_fc }
  http-request set-header X-Forwarded-Proto https if { ssl_fc }
  http-request set-header X-Forwarded-Proto-Version h2 if { ssl_fc_alpn -i h2 }
  http-request set-header Forwarded for=%[src];host=%[req.hdr(host)];proto=%[req.hdr(X-Forwarded-Proto)]
```

To implement the "IfNone" policy, the template uses `option forwardfor if-none`
and `http-request set-header` with ACLs to conditionalize each stanza:

```conf
  option forwardfor if-none
  http-request set-header X-Forwarded-Host %[req.hdr(host)] if !{ req.hdr(X-Forwarded-Host) -m found }
  http-request set-header X-Forwarded-Port %[dst_port] if !{ req.hdr(X-Forwarded-Port) -m found }
  http-request set-header X-Forwarded-Proto http if !{ ssl_fc } !{ req.hdr(X-Forwarded-Proto) -m found }
  http-request set-header X-Forwarded-Proto https if { ssl_fc } !{ req.hdr(X-Forwarded-Proto) -m found }
  http-request set-header X-Forwarded-Proto-Version h2 if { ssl_fc_alpn -i h2 } !{ req.hdr(X-Forwarded-Proto-Version) -m found }
  http-request set-header Forwarded for=%[src];host=%[req.hdr(host)];proto=%[req.hdr(X-Forwarded-Proto)] if !{ req.hdr(Forwarded) -m found }
```

To implement the "Never" policy, the template omits the `option forwardfor` and
`http-request` stanzas for a backend.

### Risks and Mitigations

If the underlying IngressController implementation were to change away from
HAProxy to a different implementation, we would need to ensure that the new
implementation supported the same capabilities.

## Design Details

### Test Plan

The controller that manages the router Deployment and related resources has unit
test coverage; for this enhancement, the unit tests have been expanded to cover
the additional functionality.

The operator has end-to-end tests; for this enhancement, a test has been added
for each policy, where the test (1) creates an IngressController configured with
policy under test, (2) creates a Pod and Route for an HTTP application that
echoes back requests, (3) sends a series of test requests with various
combinations of `x-forwarded-for` headers, and (4) verifies that the application
echoes back headers modified (or not) as expected for the respective policy.
The test for the "Append" policy additionally verifies that "Append" is the
default policy.

### Graduation Criteria

N/A.

### Upgrade / Downgrade Strategy

On upgrade, the default policy for `Forwarded` and related headers remains
"append".  On downgrade, the HAProxy configuration template ignores unrecognized
environment variables and annotations.

### Version Skew Strategy

N/A.

## Implementation History

## Alternatives

