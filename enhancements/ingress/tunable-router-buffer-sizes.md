---
title: tunable-router-buffer-sizes
authors:
  - "@sgreene570"
reviewers:
  - "@Miciah"
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@miheer"
  - "@candita"
  - "@rfredette"
approvers:
  - "@knobunc"
  - "@miciah"
  - "@frobware"
  - "@danehans"
creation-date: 2020-08-18
last-updated: 2021-03-30
status: implemented
see-also:
replaces:
superseded-by:
---
# Tunable Router Header Buffer Sizes

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the user to configure
the size of the in-memory header buffers for an IngressController. By default, these values are
set at fixed values to limit memory use for typical IngressController usage.

## Motivation

OpenShift users have applications that require large HTTP headers. HTTP requests with
header lengths that exceed the size of the IngressController's maximum header buffer size are
dropped by the IngressController. In some cases, this limitation blocks OCP users from upgrading from OCP 3.x
to 4.x, since the router's header buffer sizes were previously customizable in OCP 3.11 via custom
router templates.

### Goals

1. Enable a cluster administrator to easily modify the HAProxy `tune.bufsize` & `tune.maxrewrite` values for an IngressController
running on an OCP 4.x cluster.

### Non-Goals

1. Determine platform-specific default values for `tune.maxrewrite` and `tune.bufsize`. This is cumbersome, and not critical if cluster administrators
can adjust the values themselves.

## Proposal

The [HAProxy tune.bufsize](https://cbonte.github.io/haproxy-dconv/2.0/configuration.html#tune.bufsize) setting specifies
the size of per-session HAProxy HTTP request buffers. The [OpenShift Router](https://github.com/openshift/router)
sets this value to `32768` bytes by default.

The [HAProxy tune.maxrewrite](https://cbonte.github.io/haproxy-dconv/2.0/configuration.html#tune.maxrewrite) setting specifies
how much memory HAProxy will reserve from the buffer (with length `tune.bufsize`) for HTTP header rewriting and appending.
The [OpenShift Router](https://github.com/openshift/router) sets this value to `8192` bytes by default.

The IngressController API is extended by adding an optional `HttpHeaderBuffer` field with
type `*IngressControllerHeaderBuffer` to `IngressControllerSpec`:

```go
type IngressControllerSpec struct {
	// <snip>

	// httpHeaderBuffer defines parameters for buffer size values. If this
	// field is empty, the default values set within the IngressController
	// will be used. Caution should be used when setting these values as
	// improper HTTP header buffer values could cause the IngressController
	// to reject connections.
	//
	// +optional
	HttpHeaderBuffer *IngressControllerHttpHeaderBuffer `json:"httpHeaderBuffer,omitempty"`
}
```

The `IngressControllerHttpHeaderBuffer` type has 2 `int` fields, one for
`HeaderBufferSize` and one for `HeaderBufferMaxRewrite`.

```go
type IngressControllerHttpHeaderBuffer struct {
	// headerBufferSize describes how much memory should be reserved
	// (in bytes) for IngressController connection sessions.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:minimum=16384
	// +optional
	HeaderBufferSize int32 `json:"headerBufferSize,omitempty"`

	// headerBufferMaxRewrite describes how much memory should be reserved
	// (in bytes) from headerBufferSize for HTTP header rewriting
	// and appending. Note that incoming HTTP requests will be limited to
	// (headerBufferSize - headerBufferMaxRewrite) bytes.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:minimum=4096
	// +optional
	HeaderBufferMaxRewrite int32 `json:"headerBufferMaxRewrite,omitempty"`
}
```

Proper environment variables will be added to the Router deployment to allow for the `HeaderBufferSize` and `HeaderBufferMaxRewrite` values to be
populated in the HAProxy configuration template.

The [Ingress Operator](https://github.com/openshift/cluster-ingress-operator) will need to be modified to handle both the API changes
and setting the new Router environment variables.

Integration tests will also need to be written to ensure that the new IngressController API fields work as intended (and that the router
respects buffer limits without degrading).

### User Stories

#### As a Cluster Administrator, I want to edit the HAProxy configuration by setting "tune.maxrewrite" and "tune.bufsize" so that the Router can accommodate requests with large header sizes

The necessary Router and API changes will be made to allow cluster administrators to tweak these settings on a per-IngressController level.

### Implementation Details/Notes/Constraints

Header buffer size values should be validated by the API server to make sure they are reasonable values. An Ingress Controller should have at least `16384` bytes for
`tune.bufsize` and `4096` bytes for `tune.maxrewrite`. This is to ensure that HTTP/2 will work for an ingress controller (see https://tools.ietf.org/html/rfc7540).
These bare minimum values will also ensure that HAProxy has ample resources to handle conections to critical cluster workloads, such as oauth and the console.

In addition to API validation, the Ingress Operator should further validate user-specified router header buffer sizes.

In particular, `(headerBufferSize - headerBufferMaxRewrite)` must be greater than `0`. The ingress operator should compute the difference between
`headerBufferSize` and `headerBufferMaxRewrite`, and should set an appropriate status condition on an IngressController that specifies improper values.

### Risks and Mitigations

Cluster administrators will have the ability to set IngressController buffer sizes that break cluster ingress.
It is the responsibility of the administrator to modify the HAProxy buffer values responsibly and appropriately.
Ideally, the buffer size values would only be modified if absolutely needed.

## Design Details

### Open Questions

How often will cluster administrators take advantage of this enhancement?
Or, in other words, how sufficient are the current HAProxy buffer default values?

### Test Plan

The ingress operator has end-to-end tests; for this enhancement, a test is added that
(1) creates an IngressController that specifies HttpHeaderBuffer values (preferable with values that exceed
the Ingress Controller defaults), (2) sends HTTP requests
with valid and invalid header length sizes, and (3) verifies that the Ingress Controller only
drops the requests that have header lengths that exceed the specified HttpHeaderBuffer values.

To accomplish this, the IngressController [unique-id header API](https://github.com/openshift/api/pull/689) could be utilized.

### Graduation Criteria

N/A

### Upgrade / Downgrade Strategy

On upgrade, the default HAProxy buffer size values are used, unless the IngressController `HttpHeaderBuffer` field is set
with different values to use instead.

On downgrade, the HAProxy configuration template ignores unrecognized environment variables. In other words, if the cluster
is downgraded to an OCP version that does not support this enhancement, the HAProxy default buffer values will be used.

### Version Skew Strategy

HAProxy will ignore environment variables if they are not used in the configuration template.

The Ingress operator will ignore unused fields in the IngressController spec.

## Implementation History

Previously, in OCP 3.11, the HAProxy `tune.bufsize` value could be manually modified by a cluster administrator.
See the [OCP 3.11 Router Optimizations documentation](https://docs.openshift.com/container-platform/3.11/scaling_performance/routing_optimization.html).
