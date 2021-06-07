---
title: ingress-operator-operand-logging-level
authors:
  - "@sgreene570"
reviewers:
  - "@alebedev87"
  - "@candita"
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@Miciah"
  - "@miheer"
  - "@rfredette"
approvers:
  - "@frobware"
  - "@knobunc"
  - "@Miciah"
creation-date: 2021-06-07
last-updated: 2021-06-09
status: implementable
---

# Ingress Log Level API

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes the API and code changes necessary to expose
a means to change the Ingress Operator and OpenShift Router's Logging Levels to
cluster administrators.

## Motivation

Supporting a trivial way to raise the verbosity of the Ingress Operator and it's Operands (IngressControllers) would make debugging
cluster Ingress issues easier for cluster administrators and OpenShift developers.

Having the openshift-router binary provide more in-depth logging statements could potentially help cluster administrators
determine what events are triggering Router reloads.

Also, a logging level API for the Ingress Operator would assist OpenShift developers working on the Ingress Operator who may
desire more in-depth logging statements when working on the operator's controllers.

Additionally, a logging level API for HAProxy access logs would assist cluster administrators who wish to have more control
over the output of their IngressController's HAProxy access logs.

### Goals

* Add a user-facing API for controlling the run-time verbosity of the [OpenShift Ingress Operator](https://github.com/openshift/cluster-ingress-operator)
* Add a user-facing API for controlling the run-time verbosity of the [OpenShift Router's](https://github.com/openshift/router) sub components.

### Non-Goals

* Change the default logging verbosity of the Ingress Operator or the OpenShift Router in production OCP clusters.

## Proposal

The OpenShift Router consists of two main components: The openshift-router Go program, and HAProxy processes.
Currently, the openshift-router's verbosity level is hard-coded to `--v=2` by default in the
Router's [Dockerfile](https://github.com/openshift/router/blob/master/images/router/haproxy/Dockerfile.rhel8).
Router pod logs currently display openshift-router Go logs, in addition to some HAProxy alert logs.
HAProxy alert logs are typically visible via a router's pod logs when openshift-router attempts
to reload HAProxy into an invalid HAProxy config file, for example.

The [HAProxy Access Logging Enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/logging-api.md), which
was implemented in OCP 4.5, exposed the option enable detailed HAProxy access logs for an IngressController. However, this prior enhancement did not expose
an API for controlling access log verbosity. Currently, HAProxy access logs for an IngressController are
[set to the `Info` log level](https://github.com/openshift/cluster-ingress-operator/pull/572), which means "TCP connection and HTTP request details and errors"
([source](https://www.haproxy.com/blog/introduction-to-haproxy-logging/)) are logged. If enabled, HAProxy access logs could be raised to the `debug` level when
the IngressController LogLevel is set to `Debug` or `Trace`.

The Ingress Operator currently uses [Zapr](https://github.com/go-logr/zapr) logging library with a default log level of `Info`, which ensures that all `log.Info` and `log.Error` calls
are displayed in the Ingress Operator pod logs. Debug-level logs are currently omitted in the Ingress Operator pod logs, although, the Ingress Operator itself rarely logs at any level
other than `log.Info` and `log.Error`. In general, more lower-level logging calls could be added to the Ingress Operator.

For this enhancement, a new `OperatorLogLevel` field is added to the Ingress Config resource:

```go
type IngressSpec struct {
	// <snip>

	// operatorLogLevel controls the logging level of the Ingress Operator.
	// See LogLevel for more information about each available logging level.
	//
	// +optional
	OperatorLogLevel LogLevel `json:"operatorLogLevel"`
}
```

This new field would allow a cluster administrator to specify the desired logging level specifically for the Ingress Operator.

Additionally, a new `LogLevel` sub-field is added to the existing IngressController Logging API (aka `Spec.Logging`):

```go
// IngressControllerLogging describes what should be logged where.
type IngressControllerLogging struct {
	// access describes how the client requests should be logged.
	//
	// If this field is empty, access logging is disabled.
	//
	// +optional
	Access *AccessLogging `json:"access,omitempty"`

	// logLevel describes the logging verbosity of the IngressController.
	//
	// +optional
	LogLevel LogLevel `json:"logLevel"`
}

```

This new field would allow a clsuter administrator to specify the desired logging level of an IngressController's openshift-router Go program, in addition
to the logging level of an IngressController's HAProxy access logs, if they are enabled.

Both of these new APIs would be accompanied by appropriate `LogLevel` definitions:

```go

// LogLevel describes several available logging verbosity levels.
// +kubebuilder:validation:Enum=Normal;Debug;Trace;TraceAll
type LogLevel string

var (
	// Normal is the default.  Normal, working log information, everything is fine, but helpful notices for auditing or common operations.  In kube, this is probably glog=2.
	Normal LogLevel = "Normal"

	// Debug is used when something went wrong.  Even common operations may be logged, and less helpful but more quantity of notices.  In kube, this is probably glog=4.
	Debug LogLevel = "Debug"

	// Trace is used when something went really badly and even more verbose logs are needed.  Logging every function call as part of a common operation, to tracing execution of a query.  In kube, this is probably glog=6.
	Trace LogLevel = "Trace"

	// TraceAll is used when something is broken at the level of API content/decoding.  It will dump complete body content.  If you turn this on in a production cluster
	// prepare from serious performance issues and massive amounts of logs.  In kube, this is probably glog=8.
	TraceAll LogLevel = "TraceAll"
)
```

### User Stories

* As an OpenShift Cluster Administrator, I want to be able to raise the logging level of the Ingress Operator and IngressControllers so that I can more quickly
track down OpenShift Ingress issues.

* As an OpenShift support engineer, I want to have a means of quickly gathering detailed HAProxy access logs, in addition to detailed openshift-router logs, from customers
who are running into Ingress issues in production.

### Implementation Details/Notes/Constraints [optional]

Setting the logging level for IngressController pods should not cause the IngressController pods to rollout.
This is expected behavior whenever an IngressController's pod template's environment variables are changed.

In place of an additional environment variable, openshift-router could watch for changes to a `ConfigMap`
that holds relevant Logging Level information. This would allow Router pods to modify the logging level of
both the openshift-router binary and HAProxy access logs without rolling out any new pods.

In theory, this would make debugging Ingress issues easier, as the Router logging level can be raised "on the fly" whenever a new issue is observed.
Presumably, in some cases, rolling out new router pods may "reset" the issue and make it harder to observe without a reliable reproducer handy.

It is worth noting the openshift-sdn team has already taken a similar approach for controlling the logging verbosity of some OpenShift networking components.

### Risks and Mitigations

Raising the logging verbosity for any component typically results in larger log files that grow quickly.


## Design Details

### Open Questions [optional]

1. Can we think of any scenarios in the past where the Ingress Operator's logs were not verbose enough?
1. Will raising the openshift-router's logging level help cluster administrators understand what events are triggering router reloads?


### Test Plan

The controller that manages the IngressController Deployment and related resources has unit test coverage;
for this enhancement, these unit tests are expanded to cover any new functionality.

The Ingress Operator has end-to-end tests; for this enhancement, the following e2e tests will be added:

1. A test where the Ingress Operator's logging level is increased via the Ingress Config API.
1. A test where the IngressController's logging level is increased via the IngerssController API.
1. A test where the IngressController's logging level is increased for an IngressController that also enables HAProxy access logs.

As a part of these tests, additional testing logic could determine whether or not the logging level changes actually occurred, and also
ensure that no Router pod rollouts took place.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

On downgrade, any logging options are ignored by the Ingress Operator and OpenShift Router.
A harmless logging level configmap in the `openshift-ingress` namespace may be left behind.


### Version Skew Strategy

N/A

## Implementation History

[HAProxy Access Logging Enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/logging-api.md)

## Drawbacks


## Alternatives

* Don't provide any Ingress logging level APIs for the operator and router (current behavior)
* Raise current verbosity of the Ingress Operator and router (not desirable)

