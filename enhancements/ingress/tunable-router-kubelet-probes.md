---
title: tunable-router-kubelet-probes
authors:
  - "@Miciah"
reviewers:
  - "@rfredette"
approvers:
  - "@frobware"
api-approvers:
  - "@Miciah"
creation-date: 2022-02-21
last-updated: 2022-02-21
tracking-link:
  - "https://issues.redhat.com/browse/NE-683"
see-also:
replaces:
superseded-by:
---

# Tunable Router Kubelet Probes


## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement allows cluster administrators to modify the liveness and
readiness probe timeout values of OpenShift router deployments.

## Motivation

OpenShift router's liveness and readiness probes use the default timeout value
of 1 second, which is too short for the kubelet's probes to succeed in some
scenarios.  Even with sharding, a router on a large cluster with many Routes may
be unable to respond to probes before a timeout occurs.  Probe timeouts can
cause unwanted router restarts that interrupt application connections.  For
these scenarios, users desire the ability to set larger timeout values to reduce
the risk of unnecessary and unwanted restarts.  OpenShift&nbsp;3 allows users to
configure these timeout values, and the absence of an option to configure probe
timeouts in OpenShift&nbsp;4 is a blocker for some users to migrate from
OpenShift&nbsp;3.

### Goals

* A cluster administrator can update the liveness and readiness probe timeout
  values for the router container of router deployments that are managed by the
  ingress operator, and the operator will not revert the administrator's
  changes.

### Non-Goals

* This enhancement does not add any new API fields.

* This enhancement does not allow the user to update any other probe parameters
  besides timeout values.

## Proposal

With this enhancement, the ingress operator will ignore the value of the
`livenessProbe.timeoutSeconds` and `readinessProbe.timeoutSeconds` fields for
the router container in the pod template spec of router deployments.

### Validation

The Kubernetes API server already validates the liveness and readiness probe
parameters.  This proposal is not adding any new API fields that need to be
validated.

### User Stories

#### As a cluster administrator, I want to set a 5-second timeout for the default router deployment's liveness and readiness probes

With this enhancement, the cluster administrator can directly patch the default router Deployment:

```shell
oc -n openshift-ingress patch deploy/router-default --type=strategic --patch='{"spec":{"template":{"spec":{"containers":[{"name":"router","livenessProbe":{"timeoutSeconds":5},"readinessProbe":{"timeoutSeconds":5}}]}}}}'
```

The ingress operator will not revert the change.  The administrator can verify
that the update took effect and has not been reverted using `oc describe`:

```console
$ oc -n openshift-ingress describe deploy/router-default | grep -e Liveness: -e Readiness:
    Liveness:   http-get http://:1936/healthz delay=0s timeout=5s period=10s #success=1 #failure=3
    Readiness:  http-get http://:1936/healthz/ready delay=0s timeout=5s period=10s #success=1 #failure=3
$ 
```

### API Extensions

This enhancement uses the existing API fields of the Deployment pod template
spec.  The enhancement does not add any APIs or otherwise extend any existing
APIs.

### Risks and Mitigations

In general, users should never modify operator-managed resources; this
enhancement adds an exception to that rule and risks setting a precedent that
users may try to apply with respect to other resources.  To mitigate this risk,
we should add a note in the product documentation that this **is** an exception
and that otherwise modifying operator-managed resources without explicit
instructions is unsupported.

It is unclear why the kubelet's probes fail in some scenarios.  Some reports
link the probe failures to HAProxy reloads.  However, the kubelet probes are
handled by the openshift-router process, and HAProxy reloads are supposed to be
seamless, so probe failures may indicate a deeper issue that has not been
diagnosed.  There is a risk that this enhancement would enable users to ignore
symptoms rather than addressing the underlying issue before it became more
severe.  To mitigate this risk, we should add a node in the product
documentation that modifying the probe timeout values is an advanced tuning
technique that can be used to work around issues, but these issues should
eventually be diagnosed and possibly a support case or Bugzilla report opened
for any issues that causes probes to time out.

## Design Details

### Open Questions

Why are kubelet probes timing out, and is modifying timeout values merely
masking some deeper issue?  Whatever issue causes these timeouts is most likely
an unsupported configuration (for example, including too many routes in a shard
or using underspecced hardware) or an issue that should be reported in Bugzilla.

How should this feature be documented?  Users should not be encouraged to use
this option outside of exceptional circumstances, and users should not be
encouraged generally to modify operator-managed resources.  One option would be
to put the documentation in the "Scalability and Performance" section of the
product documentation with a clear note that this option should be used only in
exceptional circumstances, that any circumstances that require its use most
likely are unsupported or warrant a Bugzilla report, and that modifying the
router Deployment is otherwise unsupported.

### Test Plan

This enhancement involves a minor change to how the ingress operator reconciles
router Deployments; this change can be verified using unit tests and an
end-to-end test, such as the following:

1. Create an IngressController with `spec.endpointPublishingStrategy.type: Private`.
2. Update the liveness and readiness probe timeouts on the router Deployment for the new IngressController.
3. Poll for 1 minute to verify that the operator does not revert the update.

### Graduation Criteria

This enhancement does not require graduation milestones.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.  We do not plan to deprecate this feature.

### Upgrade / Downgrade Strategy

On clusters running older OpenShift releases, the ingress operator reconciles
the probe timeout values.  On upgrade, the default values will remain in place
until the user updates them.  On downgrade, if the user has updated the probe
timeout values, the downgraded operator will revert those changes, restoring the
default settings.

### Version Skew Strategy

N/A.

### Operational Aspects of API Extensions

The cluster administrator could set a high timeout values for the probes, which
could prevent the kubelet from detecting actual router degradation or failures.

#### Failure Modes

Liveness probes tell the kubelet whether it needs to restart the router; if 3
liveness probes fail in succession, the kubelet restarts the container.
Readiness probes tell the kubelet whether the pod is read to receive traffic; if
3 readiness probes fail in succession, the kubelet marks the pod as not-ready,
which has the consequence that kube-proxy marks the endpoint as not-ready; if a
service load-balancer or an external load-balancer is configured for the router
deployment, the load balancer will then stop forwarding traffic to the node for
that not-ready router pod.

The effect of increasing the probe timeout values is to prevent or delay the
restart of a failing router pod and to prevent or delay the removal of a
not-ready router pod from the load balancer's pool.  Thus, increasing these
timeout values could exacerbate traffic disruption in case of a degraded or
failing router pod replica.  The increase to disruption could be as large as the
increase in the timeout values.  For example, setting a 1-minute timeout for the
readiness probe could increase the duration of disruption in the case of a
failing router pod replica by 1 minute.

The kubelet would still detect certain types of failures.  For example, if an
entire node went down, then the node lifecycle controller would detect the
outage and remove the endpoints of any pods on the node (include any router pod
replicas), thus eventually removing them from any associated load balancer's
pool.  Additionally, if the router returned a failure response, the kubelet
would still detect the failure.  During a graceful shutdown, OpenShift router
returns an explicit negative response to readiness probes; this means that
modifying the timeout value would not interfere with graceful shutdown during a
rolling update or scale-down event.

The Network Edge team would be responsible for helping to diagnose and resolve
issues stemming from this enhancement.

#### Support Procedures

To determine whether the user has modified the probe timeout values, a support
engineer can check the router Deployment:

```shell
$ oc -n openshift-ingress describe deploy/router-default | grep -e Liveness: -e Readiness:
    Liveness:   http-get http://:1936/healthz delay=0s timeout=5s period=10s #success=1 #failure=3
    Readiness:  http-get http://:1936/healthz/ready delay=0s timeout=5s period=10s #success=1 #failure=3
$ 
```

The example above shows that the liveness and readiness probe timeout values are
each set to 5 seconds.  The same information would be available in a must-gather
archive:

```console
$ grep -e readinessProbe: -e livenessProbe: -A9 -- namespaces/openshift-ingress/core/pods.yaml
      livenessProbe:
        failureThreshold: 3
        httpGet:
          host: localhost
          path: /healthz
          port: 1936
          scheme: HTTP
        periodSeconds: 10
        successThreshold: 1
        timeoutSeconds: 5
--
      readinessProbe:
        failureThreshold: 3
        httpGet:
          host: localhost
          path: /healthz/ready
          port: 1936
          scheme: HTTP
        periodSeconds: 10
        successThreshold: 1
        timeoutSeconds: 5
$ 
```

## Implementation History

TBD.

## Drawbacks

Implementing this enhancement gives users an option that is easy to misuse or
use to mask deeper issues that may worsen over time.

## Alternatives

### Provide explicit API fields

Instead of allowing the user to modify the Deployment directly, we could add new
API fields for probe timeout values to the IngressController API.  Explicit API
fields would have the advantage of making non-default values more visible to the
user and more obvious in must-gather archives.  However, it would have the
disadvantages of expanding the IngressController API and increasing the
visibility of the option to modify probe timeout values, and this option should
be reserved for the most exceptional circumstances; therefore it is desirable
not to add API fields that would raise the visibility of this option.
