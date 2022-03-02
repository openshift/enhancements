---
title: global-healthcheck-interval-in-haproxy
authors:
  - "@cholman"
reviewers:
  - "@frobware"
  - "@knobunc"  
  - "@Miciah"
  - "@miheer"
  - "@rfredette"
  - "@deads2k"
approvers:
  - "@frobware"
  - "@Miciah"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - "@deads2k"
creation-date: 2021-11-05
last-updated: 2022-03-01
status: implementable
see-also:
replaces:
superseded-by:
---

# Global Healthcheck Interval in HAProxy

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Expose and make configurable the `ROUTER_BACKEND_CHECK_INTERVAL` environment variable in HAProxy's template so that
administrators may customize the length of time between consecutive healthchecks of backend services.

This is already configurable via a route annotation called `router.openshift.io/haproxy.health.check.interval`, but
exposing the healthcheck interval at a global scope is desired for efficient administration of routes.  HAProxy allows
setting the healthcheck globally as well as per-route, and both options will be addressed as a part of this proposal.

## Motivation

Too frequent healthcheck probes have been identified as a cause of concern for customers with highly loaded clusters. The
default healthcheck interval is set to 5 seconds.  For high-load customers, the ability to reduce the frequency of the
backend healthcheck interval, without having to configure each route separately, is an important factor.

### Goals

Enable the configuration of a backend healthcheck interval via the `IngressControllerSpec`, specifically via the
`IngressControllerSpecTuningOptions`, with a new parameter `HealthCheckInterval`, which represents a duration of time.
`HealthCheckInterval` exposes OpenShift router's internal environment variable `ROUTER_BACKEND_CHECK_INTERVAL` as an
API that the cluster administrator can set.

### Non-Goals

Although the frequency of the healthcheck interval is a cause of concern for some customers, this has not been
identified as a cause for concern for all customers, chiefly because we set it to 5 seconds.  The HAProxy default
is 2 seconds, and an interval of 10 seconds is documented in
[HAProxy configuration manual](http://cbonte.github.io/haproxy-dconv/2.2/configuration.html#3.2-max-spread-checks)
as a **large** check interval.

Performance validation is not in scope for this proposal.

Other parameters may come into play for adjusting the healthcheck interval, but will not be covered in this proposal.
It is worth checking in the future, for the impact of `spread-checks`, `max-spread-checks`, `fastinter`, `downinter`,
and the use of passive health checks rather than active health checks.

## Proposal

HAProxy configures health check intervals using the
[`check` parameter](http://cbonte.github.io/haproxy-dconv/2.2/configuration.html#5.2-check).
- `check inter <duration>`: sets the interval between two consecutive health checks to the time specified
in `duration`.

For plain HTTP backends, or backends with TLS termination set to edge, re-encrypt, or passthrough,
the `check inter` duration value may come from (evaluated in order):
- the `router.openshift.io/haproxy.health.check.interval` annotation on the route
- a (currently hidden) `ROUTER_BACKEND_CHECK_INTERVAL` environment variable
- 5 seconds as a default if the previous two values are unset

Note that this `check inter` value also serves as a timeout for health checks sent to servers
if `timeout check` is not set, but `timeout check` is currently hard-coded to 5 seconds in the OpenShift HAProxy
configuration.  The minimum value for `check inter` will be 5 seconds.
(See the Risks and Mitigations section for further discussion.)

`check inter` is set per server for the same backends as `timeout check`, with some
additional constraints.  For plain HTTP backends, or backends with TLS termination set to edge, re-encrypt, or
passthrough, it is set only if the endpoint is not idled, and the route has more than one active endpoint.

This proposal will add a new optional parameter, `HealthCheckInterval` to the `IngressControllerTuningOptions` struct,
to set the duration used in `check inter` at the backend scope for every backend server that currently sets a healthcheck
interval.

### User Stories

#### As a cluster administrator, I need to reduce the frequency of healthchecks to 20 seconds on all backends

Edit the `IngressController` specification to add `healthCheckInterval` to `tuningOptions`.  The duration of the
`healthCheckInterval` must be set as a string representing time values.  The time value format is an integer optionally
followed by a time unit suffix (e.g. "ms", "s", "m", etc.). If no unit is specified, the value is measured in
milliseconds. More information on the time format can be found in the description of
[duration string](https://pkg.go.dev/time#ParseDuration).

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  tuningOptions:
    healthCheckInterval: "20s"
...
```

#### As a cluster administrator, I need to reduce the frequency of healthchecks to 20 seconds on all backends except one

Edit the `IngressController` specification to add `healthCheckInterval` to `tuningOptions` as shown above.  Then set
the route annotation on the exception route to the default 5 seconds:

```yaml
apiVersion: v1
kind: Route
metadata:
  annotations:
    router.openshift.io/haproxy.health.check.interval: "5s"
...
```

#### As a cluster administrator, I need to set the frequency of healthchecks to the default

Edit the `IngressController` specification to remove `healthCheckInterval` from `tuningOptions`.  The default is restored.

### API Extensions

This proposal will modify the `IngressController` API by adding a new variable called `HealthCheckInterval` to the
`IngressControllerTuningOptions` struct
type. This will modify the behavior of the HAProxy running in the ingress operator router by changing the healthcheck
interval on all backends in a cluster.

```go
type IngressControllerTuningOptions struct {
...
	// healthCheckInterval defines how long the router waits between two consecutive
	// health checks on its configured backends.  This value is applied globally as
	// a default for all routes, but may be overridden per-route by the route annotation
	// "router.openshift.io/haproxy.health.check.interval".
	//
	// If unset, the default healthCheckInterval is 5s.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=duration
	// +optional
	HealthCheckInterval *metav1.Duration `json:”healthCheckInterval,omitempty”`
}
```
### Implementation Details/Notes/Constraints [optional]

To expose the `healthCheckInterval` to HAProxy, the environment variable `ROUTER_BACKEND_CHECK_TIMEOUT` will be added
to the environment in
[desiredRouterDeployment](https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/operator/controller/ingress/deployment.go#L206):
```go
// desiredRouterDeployment returns the desired router deployment.
func desiredRouterDeployment(ci *operatorv1.IngressController, ingressControllerImage string, ingressConfig *configv1.Ingress, apiConfig *configv1.APIServer, networkConfig *configv1.Network, proxyNeeded bool, haveClientCAConfigmap bool, clientCAConfigmap *corev1.ConfigMap) (*appsv1.Deployment, error) {
...
if ci.Spec.TuningOptions.HealthCheckInterval != nil && ci.Spec.TuningOptions.HealthCheckInterval > 5*time.Second {
    env = append(env, corev1.EnvVar{Name: "ROUTER_BACKEND_CHECK_TIMEOUT", 
          Value: durationToHAProxyTimespec(ci.Spec.TuningOptions.HealthCheckInterval)})
...
}
```
The HAProxy template will not be modified.

### Risks and Mitigations

One risk of this proposal is that customers may inadvertently cause different problems with a long healthcheck
interval.  For example, backend services that would normally be marked as DOWN would be seen as UP for a longer time
period, causing more errors to be seen by their application users.  To mitigate this issue the healthcheck can be
adjusted to a shorter interval, or set back to the default.

Furthermore, a change to the health check `inter` duration can have an impact on the `timeout connect` for healthchecks,
which is the maximum time to wait for a connection attempt to succeed.
We globally set the `timeout connect` to be 5 seconds by default. (The default can be overridden as environment variable
`ROUTER_DEFAULT_CONNECT_TIMEOUT` but only with a custom router.)  We also hard-code `timeout check` to 5 seconds and
apply to the same backend scope as `check inter`.  As
[documented in the HAProxy configuration manual](http://cbonte.github.io/haproxy-dconv/2.2/configuration.html#4.2-timeout%20check),
when `timeout check` is set, HAProxy:
>uses min(`timeout connect`, `inter`) as a connect timeout for check and `timeout check` as an additional read timeout

This is likely to be a problem only if the `inter` duration was accidentally set to lower than 5 seconds, in which case
the healthchecks could time out before the expected 5 seconds.  To mitigate this, the validation on the healthcheck
interval value (`healthCheckInterval`) will have a minimum of 5 seconds.

## Design Details

### Open Questions [optional]

Is there ever a reason to allow users to set the healthcheck interval lower than 5 seconds?

### Test Plan

Unit testing can validate that `desiredRouterDeployment` sets the `ROUTER_BACKEND_CHECK_TIMEOUT` environment variable
correctly.

E2E testing can verify that the annotation `router.openshift.io/haproxy.health.check.interval` takes precedence
over the `healthCheckInterval` setting in a runtime environment.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

No new environment variables are added, but a new one is exposed.  Upgrades to this enhancement should not impact any
existing installations until a `healthCheckInterval` setting is configured.  Existing route annotations for the
healthcheck interval will still override the global healthcheck interval.

Downgrades from this enhancement will revert to the previous behavior of using either the existing route annotation, or
the default of 5 seconds for healthcheck interval.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

The purpose of backend healthchecks in HAProxy are to verify that the destinations to which it intends to deliver the
traffic
are still alive and reachable.  HAProxy will stop delivering traffic to faulty destinations.  The healthcheck interval
must be small enough to ensure the unreachable destination is not used for very long after it fails.
HAProxy healthchecks are implemented as `TCP SYN` probes.  HAProxy sends the probe, and then closes the connection with
a `RST` as soon as it gets a `SYN,ACK` response from the destination.

The default healthcheck interval is 2 seconds, but OpenShift has increased it to 5 seconds, which represents a decrease
in the number of healthchecks.  As mentioned earlier, 10 seconds is considered a very long (perhaps overly long)
healthcheck interval.

Use of the new `healthCheckInterval` in the `tuningOptions` will change the frequency of healthchecks
that HAProxy performs on its backends.  There are scenarios where this could either improve or compromise the
performance of HAProxy.  Increasing the healthcheck interval too much can result in increased latency,
due to backend servers that are no longer available, but haven't yet been detected as such.  Decreasing the healthcheck
interval too much can cause extra traffic, which shows up as `SYN` packet storms.
In summary, tuning the healthcheck interval to the perfect value may take a few trials.

#### Failure Modes

N/A

#### Support Procedures

If the frequency of healthchecks compromises the performance of HAProxy, and the revert to default
values doesn't fix it, that is evidence of another issue.  

## Implementation History

## Drawbacks

This enhancement is customer-driven and is not proven to have the effects that the customer desires.

## Alternatives

- This customer request is similar to the HSTS customer request, in that users can set the value already
via route annotation but administrators want to make it mandatory on all routes, or to be global.  We
could use the same approach we used for HSTS, by adding an admission controller that can be configured
by administrators to enforce a minimum or maximum health check interval configuration on each backend.
However, the healthcheck interval is probably easier for administrators to understand as a tuning option,
and is not a security-critical change to routes like HSTS was.
  

- We could enable the automatic addition of route annotations for healthcheck interval, based
on a configuration in the ingress controller spec.  This would cause mutating routes, which is not acceptable.

## Infrastructure Needed [optional]
N/A
