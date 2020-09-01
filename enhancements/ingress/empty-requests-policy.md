---
title: empty-requests-policy
authors:
  - "@Miciah"
reviewers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@sgreene570"
approvers:
  - "@knobunc"
creation-date: 2020-08-31
last-updated: 2020-08-31
status: implementable
see-also:
replaces:
superseded-by:
---

# Ingress Empty Requests Policy

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the user to
configure IngressControllers to log or ignore "empty requests", meaning
incoming connections on which the IngressController receives no requests.

## Motivation

Empty requests typically result from load balancers' health probes or Web
browsers' speculative connections ("preconnect").  Administrators might consider
such connections to be noise that clutters logs and wastes resources.

However, empty requests may also be caused by network errors, in which case
logging the requests may be useful for diagnosing the errors.  In addition, the
connections may be caused by port scans, in which case logging the connections
may aid in detecting intrusion attempts.

Whether or not these connections should be logged or ignored depends on the
environment and the preferences of the cluster administrator, and thus an API to
specify the desired configuration is needed.

### Goals

1. The cluster administrator can specify whether an IngressController should log empty requests.
2. The cluster administrator can specify whether an IngressController should respond with HTTP 408 when an empty request times out.
3. The cluster administrator can specify whether an IngressController should count empty requests in metrics.

### Non-Goals

1. Changing the default response or logging behavior for empty requests is out of scope.
2. Enabling configuring behavior for empty requests on a per-frontend or per-route basis is out of scope.

## Proposal

To enable the cluster administrator to configure logging of empty requests, the
IngressController API is extended by adding an optional `LogEmptyRequests` field
with type `LoggingPolicy` to `AccessLogging`:

```go
type AccessLogging struct {
	// ...

	// logEmptyRequests specifies how connections on which no request is
	// received should be logged.  Typically, these empty requests come from
	// load balancers' health probes or Web browsers' speculative
	// connections ("preconnect"), in which case logging these requests may
	// be undesirable.  However, these requests may also be caused by
	// network errors, in which case logging empty requests may be useful
	// for diagnosing the errors.  In addition, these requests may be caused
	// by port scans, in which case logging empty requests may aid in
	// detecting intrusion attempts.  Allowed values for this field are
	// "Log" and "Ignore".  The default value is "Log".
	//
	// +optional
	// +kubebuilder:default:="Log"
	LogEmptyRequests LoggingPolicy `json:"logEmptyRequests,omitempty"`
}
```

The `LoggingPolicy` type accepts either one of two values: "Log" and "Ignore":

```go
// LoggingPolicy indicates how an event should be logged.
// +kubebuilder:validation:Enum=Log;Ignore
type LoggingPolicy string

const (
	// LoggingPolicyLog indicates that an event should be logged.
	LoggingPolicyLog LoggingPolicy = "Log"
	// LoggingPolicyIgnore indicates that an event should not be logged.
	LoggingPolicyIgnore LoggingPolicy = "Ignore"
)
```

To enable the administrator to configure whether the IngressController should
respond to empty HTTP requests and count them in metrics, an optional
`HTTPEmptyRequestsPolicy` field of the eponymous type is added to the
`IngressControllerSpec` type:

```go
// IngressControllerSpec is the specification of the desired behavior of the
// IngressController.
type IngressControllerSpec struct {
	// ...

	// httpEmptyRequestsPolicy describes how HTTP connections should be
	// handled if the connection times out before a request is received.
	// Allowed values for this field are "Respond" and "Ignore".  If the
	// field is set to "Respond", the ingress controller sends an HTTP 400
	// or 408 response, logs the connection (if access logging is enabled),
	// and counts the connection in the appropriate metrics.  If the field
	// is set to "Ignore", the ingress controller closes the connection
	// without sending a response, logging the connection, or incrementing
	// metrics.  The default value is "Respond".
	//
	// Typically, these connections come from load balancers' health probes
	// or Web browsers' speculative connections ("preconnect") and can be
	// safely ignored.  However, these requests may also be caused by
	// network errors, and so setting this field to "Ignore" may impede
	// detection and diagnosis of problems.  In addition, these requests may
	// be caused by port scans, in which case logging empty requests may aid
	// in detecting intrusion attempts.
	//
	// +optional
	HTTPEmptyRequestsPolicy HTTPEmptyRequestsPolicy `json:"httpEmptyRequestsPolicy,omitempty"`
}
```

The `HTTPEmptyRequestsPolicy` type accepts either one of two values: "Respond"
and "Ignore":

```go
// HTTPEmptyRequestsPolicy indicates how HTTP connections for which no request
// is received should be handled.
// +kubebuilder:validation:Enum=Respond;Ignore
type HTTPEmptyRequestsPolicy string

const (
	// HTTPEmptyRequestsPolicyRespond indicates that the ingress controller
	// should respond to empty requests.
	HTTPEmptyRequestsPolicyRespond HTTPEmptyRequestsPolicy = "Respond"
	// HTTPEmptyRequestsPolicyIgnore indicates that the ingress controller
	// should ignore empty requests.
	HTTPEmptyRequestsPolicyIgnore HTTPEmptyRequestsPolicy = "Ignore"
)
```

By default, the IngressController does not log empty requests; empty requests
are counted in metrics; and when an HTTP request times out without the
IngressController's having received a response, the IngressController sends an
HTTP 408 response and closes the connection.  The following example configures
the IngressController not to log empty requests:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  logging:
    access:
      destination:
        type: Container
      logEmptyRequests: Ignore
```

The following example configures the IngressController to close HTTP connections
without sending a response or counting them in metrics when they time out before
a request is received:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  httpEmptyRequestsPolicy: Ignore
```


### Validation

Omitting either `spec.logging.access.logEmptyRequests` or
`spec.httpEmptyRequestsPolicy` is valid and specifies the default behavior.  The
API validates both fields as described by the respective field types'
`+kubebuilder:validation:Enum` markers.

### User Stories

#### As a cluster administrator, I need to configure an IngressController not to log empty requests

To satisfy this use-case, the cluster administrator can set the
IngressController's `spec.logging.access.logEmptyRequests` field to `Ignore`.

#### As a cluster administrator, I need to configure an IngressController not to respond to empty HTTP requests

To satisfy this use-case, the cluster administrator can set the
IngressController's `spec.httpEmptyRequestsPolicy` field to `Ignore`.

### Implementation Details

Implementing this enhancement requires changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator
* openshift/router

OpenShift Router configures HAProxy using a configuration template.  The
template uses environment variables as input parameters.  The enhancement adds
two new environment variables: `ROUTER_DONT_LOG_NULL` and
`ROUTER_HTTP_IGNORE_PROBES`, which accept Boolean values and control the `option
dontlognull` and `option http-ignore-probes` HAProxy settings, respectively.

### Risks and Mitigations

If the underlying IngressController implementation were to change away from
HAProxy to a different implementation, we would need to ensure that the new
implementation supported the same capabilities.

## Design Details

### Test Plan

The controller that manages the IngressController Deployment and related
resources has unit-test coverage; for this enhancement, the unit tests are
expanded to cover the additional functionality.

The operator has end-to-end tests; for this enhancement, the following test can
be added:

1. Create an IngressController that specifies that access logs should be logged using container logging, and that otherwise uses the default settings.
2. Open a connection to one of the standard routes (such as the console route) without sending an HTTP request, and wait for the connection to time out.
3. Verify that the IngressController sends an "HTTP 408" response, and that the empty request is logged.
4. Configure the IngressController not to log of empty requests.
5. Repeat Step 2.
6. Verify that the IngressController sends an "HTTP 408" response, and that the empty request is **not** logged.
7. Configure the IngressController not to respond to empty requests.
8. Repeat Step 2.
9. Verify that the IngressController closes the connection **without** sending a response, and that the empty request is **not** logged.

### Graduation Criteria

N/A.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.  We do not plan to deprecate this feature.

### Upgrade / Downgrade Strategy

On upgrade, the default configuration logs and responds to empty requests the
same as before the introduction of the APIs described in this enhancement.

On downgrade, the HAProxy configuration template ignores unrecognized
environment variables.

### Version Skew Strategy

N/A.

## Implementation History

This enhancement is being implemented in OpenShift 4.9.

## Drawbacks

This enhancement adds further complexity to the IngressController API (i.e., API
sprawl).  Additionally, if the default ingress controller implementation were to
change away from HAProxy to a different implementation, we would need to ensure
that the new implementation supported the same capabilities.

## Alternatives

Filtering out empty requests from logs could be performed on the consumer side,
and an external firewall or proxy could modify the response behavior.  However,
HAProxy supports this functionality natively, and users wish to have this
functionality configurable in the product.
