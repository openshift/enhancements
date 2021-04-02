---
title: power-of-two-random-choices
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
last-updated: 2021-03-29
status: implementable
see-also:
replaces:
superseded-by:
---

# Enabling the "Power of Two Random Choices" Balancing Algorithm

This enhancement changes the balancing algorithm for IngressControllers to the
"[Power of Two Random
Choices](https://www.haproxy.com/blog/power-of-two-load-balancing/)" balancing
algorithm for more even balancing after router reloads.

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement changes the default balancing algorithm to HAProxy's [Power of
Two Random Choices](https://www.haproxy.com/blog/power-of-two-load-balancing/)
algorithm.  In OpenShift 4.7, IngressControllers use the "Least Connections"
balancing algorithm by default.  The balancing algorithm can be configured on
individual Routes using the `haproxy.router.openshift.io/balance` annotation.
In OpenShift 4.8, the default is changed so that IngressControllers use the
"Power of Two Random Choices" balancing algorithm by default.  Using the Route
annotation continues to be supported to configure individual routes.  As an
added safety measure, an `UnsupportedConfigOverrides` API field is added to the
IngressController API, using which it is possible to revert an IngressController
back to using "Least Connections".

## Motivation

OpenShift's IngressController implementation is based on HAProxy.  For a given
IngressController, OpenShift deploys one or more Pods, each running an HAProxy
instance, which forwards connections for a given Route to the appropriate
backend servers (i.e., the Pods associated with the Route).  OpenShift
configures each HAProxy instance to use HAProxy's Least Connections balancing
algorithm for most Route types.  This algorithm tries to ensure fairness by
considering all the Route's backend servers and dispatching each incoming
connection to the backend server that has the fewest active connections at the
time when HAProxy receives the connection.  This algorithm generally works well
to ensure fairness but is disadvantaged in two ways related to the way that
OpenShift deploys HAProxy.

The first disadvantage is that because OpenShift deploys multiple instances of
HAProxy, and these instances do not communicate with each other, there is no
centralized knowledge of how many connections each backend server has.  This
lack of centralized knowledge to inform scheduling decisions can cause
imbalances.  For example, multiple router Pods might each forward the first
incoming connection that the Pod's HAProxy instance receives to the same server.

Second, OpenShift reloads HAProxy whenever a Route or the endpoints of the
backend servers that are associated with a Route change, and when HAProxy is
reloaded, it loses track of how many connections each server has.

Combined, these disadvantages can cause dramatic imbalances when the HAProxy
instances that constitute an IngressController receive a surge in traffic.

The Power of Two Random Choices algorithm mitigates these disadvantages by
randomly picking two of a Route's backend servers and then choosing the one of
the two with fewer connections.  As described in the HAProxy blog's [Power of
Two Random Choices](https://www.haproxy.com/blog/power-of-two-load-balancing/)
article, this algorithm tends to approach the performance of Least Connections
in the general case and can perform better in case of traffic surges in the
absence of centralized knowledge.

### Goals

1. Change the default balancing algorithm to Power of Two Random Choices.
2. Continue to allow application developers to configure HAProxy the balancing algorithm on a per-Route basis.

### Non-Goals

1. Provide further control of balancing algorithm parameters.
2. Provide an API to configure the `ROUTER_BACKEND_PROCESS_ENDPOINTS` router environment variable.

## Proposal

OpenShift Router is modified to allow HAProxy's "random" balancing algorithm to
be specified.  This algorithm with its default parameters behaves as Power of
Two Random Choices.

The ingress operator is modified to configure OpenShift Router to use the
"random" balancing algorithm.

An API field is added to the IngressController API:

```go
type IngressControllerSpec struct {
	// ...
	
	// unsupportedConfigOverrides allows specifying unsupported	configuration
	// options.  Its use is unsupported.
	//
	// +optional
	// +nullable
	// +kubebuilder:pruning:PreserveUnknownFields
	UnsupportedConfigOverrides runtime.RawExtension `json:"unsupportedConfigOverrides"`
}
```

Cluster administrators can use the new `UnsupportedConfigOverrides` API field to
configure an IngressController to default to the "leastconn" (Least Connections)
balancing algorithm:

```console
$ oc -n openshift-ingress patch ingresses/default --type=merge --patch='{"spec":{"unsupportedConfigOverrides":"set-default-balancing-algorithm-to-leastconn"}}'
```

This API field would not be documented or broadly communicated to users except
as an option to revert to the behavior in OpenShift 4.7 should using "Power of
Two Random Choices" cause problems.  Upon observing this API field, the operator
would configure OpenShift Router to use the "leastconn" algorithm ad in
OpenShift 4.7.

### Validation

The `UnsupportedConfigOverrides` API field is not validated as its use is
unsupported and cluster administrators generally should not use it.

### User Stories

#### As a cluster administrator, I have an IngressController, it has Routes that update frequently, and I want traffic to be balanced after a reload

Using the Power of Two Random Choices may improve balancing in this scenario.

#### As a cluster administrator, I have a large number of IngressController Pod replicas and a Route with occasional traffic spikes, and I want to ensure that traffic spikes are balanced across the Route's backend servers

Using the Power of Two Random Choices satisfies this use-case.

### Implementation Details

Implementing this enhancement requires changes in the following repositories:

* openshift/api
* openshift/router
* openshift/cluster-ingress-operator

The router configures HAProxy using a configuration template.  The template uses
environment variables as input parameters.  In particular, the template reads
the `ROUTER_LOAD_BALANCE_ALGORITHM` environment variable to determine the
default balancing algorithm.  The enhancement modifies the template to allow the
value "random" to be specified for the `ROUTER_LOAD_BALANCE_ALGORITHM`
environment variable or the `haproxy.router.openshift.io/balance` annotation to
specify the Power of Two Random Choices algorithm.

If `spec.unsupportedConfigOverrides` is not configured, the ingress operator
configures the router Deployment by specifying "random" for
`ROUTER_LOAD_BALANCE_ALGORITHM`, thus setting the algorithm to Power of Two
Random Choices.

If `spec.unsupportedConfigOverrides` is configured and includes the substring
"set-default-balancing-algorithm-to-leastconn", the operator configures the
Deployment by specifying "leastconn" for `ROUTER_LOAD_BALANCE_ALGORITHM`,
reverting the default algorithm to Least Connections as in OpenShift 4.7.

### Risks and Mitigations

Power of Two Random Choices approximates Least Connections, but the latter may
be about 4% to 7% more efficient per [HAProxy's blog
post](https://www.haproxy.com/blog/power-of-two-load-balancing/).  Depending on
traffic patterns and performance margins, the cluster administrator may find it
necessary to revert to using Least Connections.

## Design Details

### Test Plan

The controller that manages the router Deployment and related resources has unit
test coverage; for this enhancement, the unit tests are expanded to cover the
additional functionality.

The operator has end-to-end tests; for this enhancement, a test is added that
patches the `spec.unsupportedConfigOverrides` field and verifies that the
ingress operator updates the default router Deployment to use the Least
Connections balancing algorithm.

### Graduation Criteria

N/A.

### Upgrade / Downgrade Strategy

On upgrade, the default configuration changes to Power of Two Random Choices.
If the cluster administrator sets `spec.unsupportedConfigOverrides`, the cluster
will not be upgradeable or downgradable until `spec.unsupportedConfigOverrides`
is unset.

On downgrade, the operator reverts router Deployments back to the old default
Least Connections algorithm.

### Version Skew Strategy

N/A.

## Implementation History

## Alternatives

The HAProxy configuration template has a `ROUTER_BACKEND_PROCESS_ENDPOINTS`
environment variable that can be configured so that the router shuffles the
backend servers for a Route when writing out the HAProxy configuration file.
However using this option has two disadvantages.

The first disadvantage of using `ROUTER_BACKEND_PROCESS_ENDPOINTS` to shuffle
backend servers is that it makes the generated configuration non-deterministic.

The second disadvantage is that it may cause problems for passthrough Routes.
Other Route types use cookies for session affinity/stickiness, but using cookies
is not possible for passthrough TLS connections (which are encrypted and may not
even encapsulate HTTP), and so passthrough Routes use the `source` balancing
algorithm to provide some measure of session affinity.  Shuffling backend
servers could break session affinity for passthrough Routes.
