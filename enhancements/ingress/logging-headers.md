---
title: logging-headers
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
last-updated: 2020-06-10
status: implemented
see-also:
- "enhancements/ingress/logging-api.md"
replaces:
superseded-by:
---

# Ingress Header Logging

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController logging API to allow the user to
configure capture of HTTP request and response headers in access logs.  By
default, when access logging is enabled, the IngressController logs the HTTP
request line and response code but no headers.  Using the new API, users may
specify a list of arbitrary HTTP request headers and a list of response headers
that should be captured and logged when access logging is enabled.

## Motivation

Certain headers may need to be logged for compliance reasons or other reasons.
In OpenShift 3, cluster administrators could use a custom HAProxy configuration
template to customize logging, but using a custom template is not permitted in
OpenShift 4.

### Goals

1. Enable the cluster administrator to specify HTTP request and response headers that should be captured.
2. Enable the cluster administrator to specify a custom HTTP log format string that references the captured headers.

### Non-Goal

1. Providing control on a per-frontend or per-route basis.

## Proposal

The IngressController API is extended by adding an optional `HTTPCaptureHeaders`
field with type `IngressControllerCaptureHTTPHeaders` to `AccessLogging`:

```go
type AccessLogging struct {
	// ...

	// httpCaptureHeaders defines HTTP headers that should be captured in
	// access logs.  If this field is empty, no headers are captured.
	//
	// Note that this option only applies to cleartext HTTP connections
	// and to secure HTTP connections for which the ingress controller
	// terminates encryption (that is, edge-terminated or reencrypt
	// connections).  Headers cannot be captured for TLS passthrough
	// connections.
	//
	// +optional
	HTTPCaptureHeaders IngressControllerCaptureHTTPHeaders `json:"httpCaptureHeaders,omitempty"`
}
```

The `IngressControllerCaptureHTTPHeaders` type has a `Request` field and a
`Response` field, both of type `[]IngressControllerCaptureHTTPHeader`, for
specifying headers to capture:

```go
// IngressControllerCaptureHTTPHeaders specifies which HTTP headers the
// IngressController captures.
type IngressControllerCaptureHTTPHeaders struct {
	// request specifies which HTTP request headers to capture.
	//
	// If this field is empty, no request headers are captured.
	//
	// +nullable
	// +optional
	Request []IngressControllerCaptureHTTPHeader `json:"request,omitempty"`

	// response specifies which HTTP response headers to capture.
	//
	// If this field is empty, no response headers are captured.
	//
	// +nullable
	// +optional
	Response []IngressControllerCaptureHTTPHeader `json:"response,omitempty"`
}
```

The `IngressControllerCaptureHTTPHeader` type has fields for specifying a header
name and maximum length for a captured header value (header values that exceed
the maximum length are truncated to that length):

```go
// IngressControllerCaptureHTTPHeader describes an HTTP header that should be
// captured.
type IngressControllerCaptureHTTPHeader struct {
	// name specifies a header name.  Its value must be a valid HTTP header
	// name as defined in RFC 2616 section 4.2.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^[-!#$%&'*+.0-9A-Z^_`a-z|~]+$"
	// +required
	Name string `json:"name"`

	// maxLength specifies a maximum length for the header value.  If a
	// header value exceeds this length, the value will be truncated in the
	// log message.  Note that the ingress controller may impose a separate
	// bound on the total length of HTTP headers in a request.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +required
	MaxLength int `json:"maxLength"`
}
```

By default, the IngressController does not log any HTTP headers.  The following
example captures the `Host` and `Referer` request headers and the
`Content-length` and `Location` response headers and uses them in a custom log
format string:

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
      httpCaptureHeaders:
        request:
        - name: Host
          maxLength: 90
        - name: Referer
          maxLength: 90
        response:
        - name: Content-length
          maxLength: 9
        - name: Location
          maxLength: 90
      httpLogFormat: '{"time":"%t","clientAddress":"%ci","clientPort":"%cp","frontend":"%f","backend":"%b","server":"%s","request":"%r","response":"%ST","requestLength":"%U","responseLength":"%B","requestHost":"%[capture.req.hdr(0),json(utf8s)]","requestReferer":"%[capture.req.hdr(1),json(utf8s)]","responseContentLength":"%[capture.res.hdr(0),json(utf8s)]","responseLocation":"%[capture.res.hdr(1),json(utf8s)]"}'
```

This format string produces log messages of the following form:

```text
2020-06-10T17:48:38.013139+00:00 router-default-cd9c9d6c7-xrp5l router-default-cd9c9d6c7-xrp5l haproxy[70]: {"time":"10/Jun/2020:17:48:37.874","clientAddress":"174.19.21.82","clientPort":"33862","frontend":"fe_sni","backend":"be_secure:openshift-console:console","server":"pod:console-64ff858c-ghp2k:console:10.128.0.47:8443","request":"GET /auth/login HTTP/1.1","response":"303","requestLength":"845","responseLength":"975","requestHost":"console-openshift-console.apps.ci-ln-8cjiqnb-d5d6b.origin-ci-int-aws.dev.rhcloud.com","requestReferer":"https:\/\/console-openshift-console.apps.ci-ln-8cjiqnb-d5d6b.origin-ci-int-aws.dev.rhcloud.c","responseContentLength":"341","responseLocation":"https:\/\/oauth-openshift.apps.ci-ln-8cjiqnb-d5d6b.origin-ci-int-aws.dev.rhcloud.com\/oauth\/a"}
```

Omitting the `spec.logging.access.httpLogFormat` field to use the default format
string would produce log messages of the following form:

```text
2020-06-10T17:43:59.327608+00:00 router-default-65dccc49cc-2cshx router-default-65dccc49cc-2cshx haproxy[71]: 174.19.21.82:33634 [10/Jun/2020:17:43:59.323] fe_sni~ be_secure:openshift-console:console/pod:console-64ff858c-ghp2k:console:10.128.0.47:8443 0/0/0/3/3 303 975 - - --VN 23/5/0/0/0 0/0 \
{console-openshift-console.apps.ci-ln-8cjiqnb-d5d6b.origin-ci-int-aws.dev.rhcloud.com|https://console-openshift-console.apps.ci-ln-8cjiqnb-d5d6b.origin-ci-int-aws.dev.rhcloud.c} {341|https://oauth-openshift.apps.ci-ln-8cjiqnb-d5d6b.origin-ci-int-aws.dev.rhcloud.com/oauth/a} "GET /auth/login HTTP/1.1"
```

### Validation

Omitting either `spec.logging.access.httpCaptureHeaders.request` or
`spec.logging.access.httpCaptureHeaders.response` is valid and means no request
or response headers, respectively, are captured.  Omitting both fields specifies
the default behavior, which is not to capture any HTTP headers, same as was the
case before the API was introduced.

The API validates the `spec.logging.access.httpCaptureHeaders.request[*].name`
and `spec.logging.access.httpCaptureHeaders.response[*].name` field values as
described by the field type's `+kubebuilder:validation:Pattern` marker.
Similarly, the API validates that the
`spec.logging.access.httpCaptureHeaders.request[*].maxLength` and
`spec.logging.access.httpCaptureHeaders.response[*].maxLength` field values are
positive integers.

### User Stories

#### As a cluster administrator, I need to configure an IngressController to capture certain HTTP request and response headers, in order to satisfy compliance requirements

To satisfy this use-case, the cluster administrator can set the
IngressController's `spec.logging.access.httpCaptureHeaders` field to capture
the required headers..

### Implementation Details

Implementing this enhancement requires changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator
* openshift/router

The router configures HAProxy using a configuration template.  The template uses
environment variables as input parameters.  The enhancement adds two new
environment variables, `ROUTER_CAPTURE_HTTP_REQUEST_HEADERS` and
`ROUTER_CAPTURE_HTTP_RESPONSE_HEADERS`, which specify the headers to capture.
The ingress operator sets this variable based on the IngressController's
`spec.logging.access.httpCaptureHeaders` field value.

The values of `ROUTER_CAPTURE_HTTP_REQUEST_HEADERS` and
`ROUTER_CAPTURE_HTTP_RESPONSE_HEADERS` are comma-delimited lists of values of
the form `name:maxLength`.  (Colons and commas are not allowed in HTTP header
names, so their use unambiguously delimits values in the environment variable.)
The template translates these values into appropriate `capture request header`
and `capture response header` stanzas.

For example, if `ROUTER_CAPTURE_HTTP_REQUEST_HEADERS` is set to
`Host:15,Referer:15` and `ROUTER_CAPTURE_HTTP_RESPONSE_HEADERS` is set to
`Content-length:9,Location:15`, then the template adds the following stanzas to
the appropriate frontends:

```conf
  capture request header Host len 15
  capture request header Referer len 15
  capture response header Content-length len 9
  capture response header Location len 15
```

### Risks and Mitigations

If the underlying IngressController implementation were to change away from
HAProxy to a different implementation, we would need to ensure that the new
implementation supported the same capabilities.

## Design Details

### Test Plan

The controller that manages the IngressController Deployment and related
resources has unit test coverage; for this enhancement, the unit tests are
expanded to cover the additional functionality.

The operator has end-to-end tests; for this enhancement, a test is added that
(1) creates an IngressController that specifies that access logs should be
logged using container logging and that certain request headers and response
headers should be captured, (2) sends an HTTP request to one of the standard
routes (such as the console route), and (3) verifies that the expected headers
are logged.

### Graduation Criteria

N/A.

### Upgrade / Downgrade Strategy

On upgrade, the default configuration captures no HTTP headers, as before the
introduction of the HTTP header capture API.

On downgrade, the HAProxy configuration template ignores unrecognized
environment variables.  If a custom HTTP log format string has references to
captured headers, HAProxy substitutes a hyphen ("-") for missing referents.

### Version Skew Strategy

N/A.

## Implementation History

## Alternatives
