---
title: aws-elb-idle-timeout
authors:
  - "@Miciah"
reviewers:
  - "@gcs278"
approvers:
  - "@frobware"
api-approvers:
creation-date: 2020-08-31
last-updated: 2022-03-28
tracking-link:
  - "https://issues.redhat.com/browse/NE-357"
see-also:
 - "./request-timeout-configuration.md"
replaces:
superseded-by:
---

# Ingress AWS ELB Idle Connection Timeout

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the user to
configure the timeout period for idle connections to an IngressController that
is published using an AWS Classic Elastic Load Balancer.

## Motivation

OpenShift router's default client timeout period for Routes is 30 seconds.  It
is possible to set a custom timeout period for a specific Route, or for an
IngressController (using [the request-timeout-configuration
enhancement](./request-timeout-configuration.md)).  However, the external
load-balancer may impose a timeout period of its own.  In particular, AWS
Classic ELBs by default have a timeout period of 60 seconds.  If the LB's
timeout period is shorter than the Route's or IngressController's, then the LB
can time out and terminate the connection, which is counter to the user's
specified intention and may cause connection failures where the cause is not
obvious to the user.  Thus, in order to enable Routes and IngressControllers to
have higher timeout periods, it is necessary to enable the external
load-balancer's timeout period to be increased as well.

### Goals

1. Enable the cluster administrator to specify the connection timeout period for AWS Classic ELBs.

### Non-Goals

1. Introduce a platform-agnostic API (which would be impossible to implement on most platforms).
1. Introduce an equivalent API for other platforms or LB types besides AWS Classic ELBs.

## Proposal

To enable the cluster administrator to configure the connection timeout period
for AWS Classic ELBs, the IngressController API is extended by adding an
optional `ConnectionIdleTimeout` field with type `*metav1.Duration` to
`AWSClassicLoadBalancerParameters`:

```go
// AWSClassicLoadBalancerParameters holds configuration parameters for an
// AWS Classic load balancer.
type AWSClassicLoadBalancerParameters struct {
	// connectionIdleTimeout specifies the maximum time period that a
	// connection may be idle before the load balancer closes the
	// connection.  The value must be parseable as a time duration value;
	// see <https://pkg.go.dev/time#ParseDuration>.  The default value for
	// this field is 60s.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=duration
	// +optional
	ConnectionIdleTimeout *metav1.Duration `json:"connectionIdleTimeout"`
}
```

The following example configures a 2-minute timeout period:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  endpointPublishingStrategy:
    type: LoadBalancerService
    loadBalancer:
      providerParameters:
        type: AWS
        aws:
          type: Classic
          classicLoadBalancer:
            connectionIdleTimeout: 2m
```

### Validation

Omitting `spec.endpointPublishingStrategy`,
`spec.endpointPublishingStrategy.loadBalancer`,
`spec.endpointPublishingStrategy.loadBalancer.providerParameters`,
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws`,
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer`,
or
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.connectionIdleTimeout`
is valid and specifies the default behavior.  The API validates that any
provided value is a time duration value.

### User Stories

#### As a cluster administrator, I need my IngressController's ELB to have a 2-minute timeout

To satisfy this use-case, the cluster administrator can set the
IngressController's
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.connectionIdleTimeout`
field to `2m`.

#### As an project administrator, I need my application to have a 5-minute timeout

To satisfy this use-case, the cluster administrator must set the
IngressController's
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.connectionIdleTimeout`
field to `5m` or longer, and the project administrator must similarly set a
Route annotation for the application's Route:

```shell
oc annotate -n my-project routes/my-route haproxy.router.openshift.io/timeout=5m
```

### API Extensions

This enhancement extends the IngressController API by adding a new field:
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.connectionIdleTimeout`.

This enhancement modifies the ingress operator's behavior to set the
`service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout`
annotation on LoadBalancer-type Services that the operator manages
when this new field is set.

### Implementation Details

Implementing this enhancement requires changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator

When the endpoint publishing strategy type is "LoadBalancerService", the ingress
operator creates the appropriate Service.  Additionally, with this enhancement,
if the
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.connectionIdleTimeout`
field is specified, the operator adds the
`service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout`
annotation to this Service, with the specified value.

### Risks and Mitigations

Adding platform-specific parameters poses a risk of API sprawl.  Mitigating
factors are that the API already has a field for Classic ELB-specific
parameters.  Moreover, the connection timeout period cannot be configured on
other platforms; that is, it is inherently platform-specific, and so trying to
make the API appear more generic would be misleading.

It is possible that a user may have set the
`service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout`
annotation on an operator-managed Service; see the "Upgrade / Downgrade
Strategy" section for further discussion of this issue.

## Design Details

### Test Plan

The controller that manages the router Deployment and related resources for an
IngressController has unit test coverage; for this enhancement, the unit tests
are expanded to cover the additional functionality.

The operator has end-to-end tests; for this enhancement, the following test can
be added:

1. Create an IngressController that specifies a short ELB idle timeout (for example, 10 seconds).
2. Create a pod with an HTTP application that sends responses with a delay (for example, 40 seconds).
3. Create a route for this application.
4. Open a connection to this route and send a request.
5. Verify that the connection times out after 10 seconds.
6. Configure the IngressController with a longer ELB idle timeout (for example, 120 seconds).
7. Configure the route with a 90-second timeout.
8. Open a connection to this route and send a request.
9. Verify that a response is received.

### Graduation Criteria

N/A.  This feature will go directly to GA.

#### Dev Preview -> Tech Preview

N/A.

#### Tech Preview -> GA

N/A.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

It is possible that a user may have set the
`service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout`
annotation on a LoadBalancer-type Service that the ingress operator manages.
Users are generally not supposed to modify operator-managed resources, and such
a modification would be unsupported.  However, the ingress operator did not
previously prevent the user from making such a modification.  Thus when a
cluster is upgraded from a version of OpenShift without this enhancement to a
version with it, the operator could overwrite a user-specified annotation.
After upgrade, the user would then need to update the
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.connectionIdleTimeout`
field of the corresponding IngressController to the desired value to restore the
configuration.  Otherwise, the default ELB timeout period remains in effect
after an upgrade.  Because modifying operator-managed resources is generally
unsupported, it should suffice to address this issue through a release note.
If necessary, a change could be added in an earlier version of OpenShift to set
the `Upgradeable=False` ClusterOperator status condition if a user-specified
annotation were detected.

Before this enhancement was added, the ingress operator did not update the
Service's annotations when they changed.  Thus downgrading the operator may
leave any configured timeout period in effect until the IngressController is
deleted.

### Version Skew Strategy

N/A.

### Operational Aspects of API Extensions

This enhancement enables users to change the connection timeout period for ELBs
associated with IngressControllers.  In particular, the user can change this
setting for an ELB that is associated with the default IngressController, which
usually handles core platform Routes, such as those for OpenShift console,
OAuth, Prometheus, and Grafana.

#### Failure Modes

If the user sets a very short timeout period, this may manifest as empty
responses from the ELB:

```console
% curl -v 'http://idling-echo-idling-echo.apps.ci-ln-t6t488t-76ef8.origin-ci-int-aws.dev.rhcloud.com/shell?cmd=sleep+90'
*   Trying 100.24.138.152...
* TCP_NODELAY set
* Connected to idling-echo-idling-echo.apps.ci-ln-t6t488t-76ef8.origin-ci-int-aws.dev.rhcloud.com (100.24.138.152) port 80 (#0)
> GET /shell?cmd=sleep+90 HTTP/1.1
> Host: idling-echo-idling-echo.apps.ci-ln-t6t488t-76ef8.origin-ci-int-aws.dev.rhcloud.com
> User-Agent: curl/7.61.1
> Accept: */*
>
* Empty reply from server
* Connection #0 to host idling-echo-idling-echo.apps.ci-ln-t6t488t-76ef8.origin-ci-int-aws.dev.rhcloud.com left intact
curl: (52) Empty reply from server
zsh: exit 52    curl -v
```

In contrast, timeouts from OpenShift router manifest as HTTP 504 Gateway Timeout
responses from HAProxy:

```console
% curl -v 'http://idling-echo-idling-echo.apps.ci-ln-t6t488t-76ef8.origin-ci-int-aws.dev.rhcloud.com/shell?cmd=sleep+90'
*   Trying 3.208.187.178...
* TCP_NODELAY set
* Connected to idling-echo-idling-echo.apps.ci-ln-t6t488t-76ef8.origin-ci-int-aws.dev.rhcloud.com (3.208.187.178) port 80 (#0)
> GET /shell?cmd=sleep+90 HTTP/1.1
> Host: idling-echo-idling-echo.apps.ci-ln-t6t488t-76ef8.origin-ci-int-aws.dev.rhcloud.com
> User-Agent: curl/7.61.1
> Accept: */*
>
< HTTP/1.1 504 Gateway Time-out
< content-length: 92
< cache-control: no-cache
< content-type: text/html
<
<html><body><h1>504 Gateway Time-out</h1>
The server didn't respond in time.
</body></html>
* Connection #0 to host idling-echo-idling-echo.apps.ci-ln-t6t488t-76ef8.origin-ci-int-aws.dev.rhcloud.com left intact
```

#### Support Procedures

Timeout failures can be diagnosed as described under the "Failure Modes"
section.  If requests to OpenShift router result in empty responses, the ELB
timeout period may need to be increased.  If requests result in HTTP 504
responses as shown above, then the timeout period may need to be increased on
the Route or IngressController for which requests are timing out.

## Implementation History

* An API PR was posted in
  [openshift/api#730](https://github.com/openshift/api/pull/730) on 2020-08-31.
* A work-in-progress, proof-of-concept implementation was posted in
 [openshift/cluster-ingress-operator#451](https://github.com/openshift/cluster-ingress-operator/pull/451)
 on 2020-09-01.

## Drawbacks

This enhancement expands the IngressController API surface with a cloud-specific
parameter that a user could misconfigure by setting a short timeout value,
causing empty responses when the network has high latency, OpenShift router
is overloaded, or a backend pod is slow to respond.

## Alternatives

One alternative is to support users' directly setting the
`service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout`
annotation on operator-managed Services.  This would require less work to
implement.  However, allowing users to set the annotation would set a precedent
of allowing users to modify operator-managed resources, which we want to avoid.
Additionally, an explicit IngressController API field can be validated, and the
IngressController API provides a centralized location (under the existing
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws` field)
for configuring the AWS load balancer.
