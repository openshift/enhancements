---
title: proxy-protocol
authors:
  - "@Miciah"
reviewers:
  - "@candita"
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@miheer"
  - "@rfredette"
  - "@sgreene570"
approvers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
creation-date: 2021-02-23
last-updated: 2021-03-26
status: implementable
see-also:
replaces:
superseded-by:
---

# Ingress PROXY Protocol

This enhancement enables cluster administrators to enable [PROXY
protocol](http://www.haproxy.org/download/2.2/doc/proxy-protocol.txt) on
IngressControllers.

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow cluster
administrators to configure IngressControllers that use the "HostNetwork"
and "NodePortService" endpoint publishing strategies to use [PROXY
protocol](http://www.haproxy.org/download/2.2/doc/proxy-protocol.txt).

## Motivation

Usually an IngressController is configured with some sort of load balancer in
front of it.  Typically in such configurations, the connections that the
IngressController receives have the load balancer's address (rather than the
original client addresses) as the source address.  However, it is useful to have
the original source addresses for connections (as received by the load balancer)
for logging, filtering, and injecting HTTP headers.

Using [PROXY
protocol](http://www.haproxy.org/download/2.2/doc/proxy-protocol.txt) enables a
load balancer to preserve the original source addresses and include them when
forwarding connections to an IngressController, which can then use the original
source addresses for logging etc.

When OpenShift runs in a cloud platform and an IngressController specifies that
a service load-balancer should be used, the ingress operator configures the
service load-balancer and is able to infer based on the specific platform
whether or not PROXY protocol is needed for preserving source addresses.  For
example, when an IngressController specifies that it should use an AWS ELB,
PROXY protocol is required to preserve source addresses, and so the ingress
operator automatically enables PROXY protocol on the ELB and on the
IngressController.  Other supported cloud platforms (including Azure and GCP)
preserve source addresses without the use of PROXY protocol, and so the operator
does not enable PROXY protocol for service load balancers on these platforms.

On non-cloud platforms, an IngressController is usually configured with an
external load-balancer, and OpenShift cannot interrogate or infer whether this
load balancer preserves source addresses or uses PROXY protocol.  Thus cluster
administrators need to be able to configure whether or not the load balancer and
IngressController use PROXY protocol.  To this end, this enhancement provides a
new API that enables cluster administrators to configure PROXY protocol on
IngressControllers.

This new API is specifically likely to be needed when the IngressController
specifies the "HostNetwork" or "NodePortService" endpoint publishing strategies,
which are intended for integrating with external load-balancers.  The API is not
needed for the "LoadBalancerService" strategy where OpenShift can infer whether
PROXY protocol is needed, nor is the API needed for the "Private" strategy where
OpenShift does not publish an endpoint that an external load-balancer could use.

### Goal

1. Enable cluster administrators to enable PROXY protocol on IngressControllers that use the "HostNetwork" and "NodePortService" endpoint publishing strategies.

### Non-Goals

1. Enabling PROXY protocol to be configured on individual Routes.
1. Enable cluster administrators to enable PROXY protocol on IngressControllers that use the "LoadBalancerService" or "Private" endpoint publishing strategies.

## Proposal

To enable cluster administrators to configure IngressControllers to use PROXY
protocol, the IngressController API is extended by adding an optional `Protocol`
field with type `IngressControllerProtocol` to the `HostNetworkStrategy` and
`NodePortStrategy` structs:

```go
// HostNetworkStrategy holds parameters for the HostNetwork endpoint publishing
// strategy.
type HostNetworkStrategy struct {
	// protocol specifies whether the IngressController expects incoming
	// connections to use plain TCP or whether the IngressController expects
	// PROXY protocol.
	//
	// PROXY protocol can be used with load balancers that support it to
	// communicate the source addresses of client connections when
	// forwarding those connections to the IngressController.  Using PROXY
	// protocol enables the IngressController to report those source
	// addresses instead of reporting the load balancer's address in HTTP
	// headers and logs.  Note that enabling PROXY protocol on the
	// IngressController will cause connections to fail if you are not using
	// a load balancer that uses PROXY protocol to forward connections to
	// the IngressController.  See
	// http://www.haproxy.org/download/2.2/doc/proxy-protocol.txt for
	// information about PROXY protocol.
	//
	// The following values are valid for this field:
	//
	// * The empty string.
	// * "TCP".
	// * "PROXY".
	//
	// The empty string specifies the default, which is TCP without PROXY
	// protocol.  Note that the default is subject to change.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Protocol IngressControllerProtocol `json:"protocol,omitempty"`
}

// NodePortStrategy holds parameters for the NodePortService endpoint publishing strategy.
type NodePortStrategy struct {
	// ...
	Protocol IngressControllerProtocol `json:"protocol,omitempty"`
}
```

`IngressControllerProtocol` is a string type with three allowed values:

```go
// IngressControllerProtocol specifies whether PROXY protocol is enabled or not.
// +kubebuilder:validation:Enum="";TCP;PROXY
type IngressControllerProtocol string

const (
	DefaultProtocol IngressControllerProtocol = ""
	TCPProtocol     IngressControllerProtocol = "TCP"
	ProxyProtocol   IngressControllerProtocol = "PROXY"
)
```

The following example configures an IngressController that uses a NodePort
service that communicates using PROXY protocol:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  endpointPublishingStrategy:
    type: NodePortService
    nodePort:
      protocol: PROXY
```

### Validation

Specifying `spec.endpointPublishingStrategy.type: HostNetwork` and omitting
`spec.endpointPublishingStrategy.hostNetwork` or
`spec.endpointPublishingStrategy.hostNetwork.protocol` is valid and specifies
the default behavior, which is not to use PROXY protocol.  Similarly, specifying
`spec.endpointPublishingStrategy.type: NodePortService` and omitting
`spec.endpointPublishingStrategy.nodePort` or
`spec.endpointPublishingStrategy.nodePort.protocol` is valid and specifies the
default behavior, which again is not to use PROXY protocol.  The API validates
that any value provided for
`spec.endpointPublishingStrategy.hostNetwork.protocol` or
`spec.endpointPublishingStrategy.nodePort.protocol` is one of the allowed
values: the empty string, "TCP", or "PROXY".

### User Stories

#### As a cluster administrator, I need my IngressController to use PROXY protocol to communicate with my external load-balancer

To satisfy this use-case, the cluster administrator can set the
IngressController's `spec.endpointPublishingStrategy.nodePort.protocol` or
`spec.endpointPublishingStrategy.hostNetwork.protocol` field to `PROXY`.

### Implementation Details

Implementing this enhancement requires changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator

When the endpoint publishing strategy type is "HostNetwork" or
"NodePortService", the ingress operator creates the appropriate service.  If
`spec.endpointPublishingStrategy.type` is `HostNetwork` and the
`spec.endpointPublishingStrategy.hostNetwork.protocol` field has the value
`PROXY`, or if `spec.endpointPublishingStrategy.type` is `NodePortService` and
the `spec.endpointPublishingStrategy.nodePort.protocol` field is specified with
the value `PROXY`, the operator sets the `ROUTER_USE_PROXY_PROTOCOL` environment
variable on the router Deployment to configure HAProxy to use PROXY protocol.

### Risks and Mitigations

Enabling PROXY protocol when the external load-balancer does not use PROXY
protocol could render the IngressController inaccessible.  This would
particularly be a problem if a cluster administrator enabled PROXY protocol on
the default IngressController and then needed to connect to OAuth (which would
potentially be unavailable) to get a new token.

To mitigate this risk, the `Protocol` API fields' godoc explains the use-case
for specifying `PROXY` and has a warning that specifying `PROXY` could break
ingress if traffic that does not use PROXY protocol is sent to the
IngressController.

## Design Details

### Test Plan

The controller that manages the IngressController Deployment and related
resources has unit test coverage; for this enhancement, the unit tests are
expanded to cover the additional functionality.

The operator has end-to-end tests; for this enhancement, the following test are
added:

1. Create an IngressController that enables the `NodePortService` endpoint publishing strategy type without specifying `spec.endpointPublishingStrategy.nodePort.protocol`.
2. Verify that the IngressController does not configure `ROUTER_USE_PROXY_PROTOCOL=true` on the router deployment.
3. Update the IngressController to specify `spec.endpointPublishingStrategy.nodePort.protocol: PROXY`.
4. Verify that the IngressController updates the router deployment to specify `ROUTER_USE_PROXY_PROTOCOL=true`.

### Graduation Criteria

N/A.

### Upgrade / Downgrade Strategy

On upgrade, the default configuration remains in effect.

If a cluster administrator upgraded to 4.8, then enabled PROXY protocol, and
then downgraded to 4.7, the downgrade would turn PROXY protocol off.  The
administrator would be responsible for reconfiguring any external load-balancers
not to use PROXY protocol when downgrading to OpenShift 4.7.

### Version Skew Strategy

N/A.

## Implementation History

2016-12-21, in OpenShift Enterprise 3.5 (Origin 1.5), [openshift/origin#12271 HAProxy Router: Add option to use PROXY protocol by Miciah](https://github.com/openshift/origin/pull/12271) added the `ROUTER_USE_PROXY_PROTOCOL` environment variable to the HAProxy configuration template.

