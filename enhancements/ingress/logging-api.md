---
title: logging-api
authors:
  - "@Miciah"
reviewers:
  - "@alanconway"
  - "@danehans"
  - "@frobware"
  - "@ironcladlou"
  - "@jcantrill"
  - "@knobunc"
approvers:
  - "@ironcladlou"
  - "@knobunc"
creation-date: 2020-03-19
last-updated: 2020-03-19
status: implemented
see-also: cluster-logging/cluster-logging-log-forwarding
replaces:
superseded-by:
---

# Ingress Logging API

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the user to
configure access logs for an IngressController.  By default, access logging is
disabled.  The proposed API extension provides options to log to a sidecar or to
forward logs to a custom syslog endpoint using a specified facility, as well as
an option to specify the format for access logs.  The API is designed to
accommodate future additions of logging types (besides access logs), logging
destinations, and logging parameters.

## Motivation

Users desire the ability to log connections to IngressControllers.  For clusters
that do not receive much traffic, it may suffice to log to standard output and
let the logging stack collect logs.  In this scenario, the user may desire that
access logs go to a sidecar.

For high-traffic clusters, the quantity of log output may exceed the capacity of
the logging stack.  It also may be the case that the user desires to integrate
with logging infrastructure that exists outside of OpenShift.  In such
scenarios, the user may desire that logs go to a custom syslog endpoint.

### Goals

1. Enable the user to configure access logging for an IngressController.
2. Provide options to send logs to either a sidecar or a custom syslog endpoint.
3. Provide an option to configure the log format.
4. Accommodate future additions of possible logging destinations.
5. Accommodate future additions of logging types besides access logs.
6. Accommodate future additions of logging parameters.
7. Avoid redundancy with [the cluster logging log forwarding API](../cluster-logging/cluster-logging-log-forwarding.md).

### Non-Goal

Configuring log collection or aggregation is out of scope.

## Proposal

The IngressController API is extended by adding an optional `Logging` field with
type `*IngressControllerLogging` to `IngressControllerSpec`:

```go
type IngressControllerSpec struct {
	// ...

	// logging defines parameters for what should be logged where.  If this
	// field is empty, operational logs are enabled but access logs are
	// disabled.
	//
	// +optional
	Logging *IngressControllerLogging `json:"logging,omitempty"`
}
```

The `IngressControllerLogging` type has an optional `Access` field of type
`*AccessLogging` for configuring access logging; additional fields for other
types of logging may be added in the future:

```go
// IngressControllerLogging describes what should be logged where.
type IngressControllerLogging struct {
	// access describes how the client requests should be logged.
	//
	// If this field is empty, access logging is disabled.
	//
	// +optional
	Access *AccessLogging `json:"access,omitempty"`
}
```

The `AccessLogging` type allows specifying a destination using the `Destination`
field, which has type `LoggingDestination`, and the log format using the
`HttpLogFormat` field, which has type `string`:

```go
// AccessLogging describes how client requests should be logged.
type AccessLogging struct {
	// destination is where access logs go.
	//
	// +kubebuilder:validation:Required
	// +required
	Destination LoggingDestination `json:"destination"`

	// httpLogFormat specifies the format of the log message for an HTTP
	// request.
	//
	// If this field is empty, log messages use the implementation's default
	// HTTP log format.  For HAProxy's default HTTP log format, see the
	// HAProxy documentation:
	// http://cbonte.github.io/haproxy-dconv/2.0/configuration.html#8.2.3
	//
	// +optional
	HttpLogFormat string `json:"httpLogFormat,omitempty"`
}
```

The `LoggingDestination` type is a union type:

```go
// LoggingDestination describes a destination for log messages.
// +union
type LoggingDestination struct {
	// type is the type of destination for logs.  It must be one of the
	// following:
	//
	// * Container
	//
	// The ingress operator configures the sidecar container named "logs" on
	// the ingress controller pod and configures the ingress controller to
	// write logs to the sidecar.  The logs are then available as container
	// logs.  The expectation is that the administrator configures a custom
	// logging solution that reads logs from this sidecar.  Note that using
	// container logs means that logs may be dropped if the rate of logs
	// exceeds the container runtime's or the custom logging solution's
	// capacity.
	//
	// * Syslog
	//
	// Logs are sent to a syslog endpoint.  The administrator must specify
	// an endpoint that can receive syslog messages.  The expectation is
	// that the administrator has configured a custom syslog instance.
	//
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	// +required
	Type LoggingDestinationType `json:"type"`

	// syslog holds parameters for a syslog endpoint.  Present only if
	// type is Syslog.
	//
	// +optional
	Syslog *SyslogLoggingDestinationParameters `json:"syslog,omitempty"`

	// container holds parameters for the Container logging destination.
	// Present only if type is Container.
	//
	// +optional
	Container *ContainerLoggingDestinationParameters `json:"container,omitempty"`
}
```

The union discriminator can have either the value `Container` or the value
`Syslog`:

```go
// LoggingDestinationType is a type of destination to which to send log
// messages.
//
// +kubebuilder:validation:Enum=Container;Syslog
type LoggingDestinationType string

const (
	// Container sends log messages to a sidecar container.
	ContainerLoggingDestinationType LoggingDestinationType = "Container"

	// Syslog sends log messages to a syslog endpoint.
	SyslogLoggingDestinationType LoggingDestinationType = "Syslog"

	// ContainerLoggingSidecarContainerName is the name of the container
	// with the log output in an ingress controller pod when container
	// logging is used.
	ContainerLoggingSidecarContainerName = "logs"
)
```

For `Syslog`, an endpoint must be specified, and a facility may be specified (if
unspecified the default is "local1"); `Container` has no parameters:

```go
// SyslogLoggingDestinationParameters describes parameters for the Syslog
// logging destination type.
type SyslogLoggingDestinationParameters struct {
	// address is the IP address of the syslog endpoint that receives log
	// messages.
	//
	// +kubebuilder:validation:Required
	// +required
	Address string `json:"address"`

	// port is the UDP port number of the syslog endpoint that receives log
	// messages.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +required
	Port uint32 `json:"port"`

	// facility specifies the syslog facility of log messages.
	//
	// If this field is empty, the facility is "local1".
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=kern;user;mail;daemon;auth;syslog;lpr;news;uucp;cron;auth2;ftp;ntp;audit;alert;cron2;local0;local1;local2;local3;local4;local5;local6;local7
	// +optional
	Facility string `json:"facility,omitempty"`
}

// ContainerLoggingDestinationParameters describes parameters for the Container
// logging destination type.
type ContainerLoggingDestinationParameters struct {
}
```

To disable access logging, the user leaves `spec.logging` or
`spec.logging.access` unspecified.  Otherwise the user must specify a
destination using `spec.logging.access.destination`.  To specify a destination,
the user must specify either `Container` or `Syslog` for
`spec.logging.access.destination.type`.  If the destination type is `Syslog`,
the user must specify a destination endpoint using
`spec.logging.access.destination.syslog.address` and
`spec.logging.access.destination.syslog.port` and may specify a facility using
`spec.logging.access.destination.syslog.facility`.  The user may specify
`spec.logging.access.httpLogFormat` to customize the log format.  For example,
the following is the definition of an IngressController that logs to a syslog
endpoint with IP address 1.2.3.4 and port 10514:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  replicas: 2
  endpointPublishingStrategy:
    type: NodePortService
  logging:
    access:
      destination:
        type: Syslog
        syslog:
          address: 1.2.3.4
          port: 10514
```

### Validation

Omitting `spec.logging` and omitting `spec.logging.access` have well defined
semantics.

The `spec.logging.access.httpLogFormat` field value is a free-form format
string, so it is not validated.

The API validates the `spec.logging.access.destination.type` field value as
described by the field type's `+kubebuilder:validation:Enum` marker.

The API validates the `spec.logging.access.destination.syslog.facility` field
value as described by the field's `+kubebuilder:validation:Enum` marker.

If the ingress controller specifies a syslog destination, the API validates that
the `spec.logging.access.destination.syslog.address` field value is an IPv4 or
IPv6 address and that the `spec.logging.access.destination.syslog.port` field
value is a valid port number.

### User Stories

#### As a cluster administrator, I do not want access logs

Access logs are disabled by default, so no action is needed.

#### As a cluster administrator, I want to enable access logs using OpenShift's logging stack

The user enables access logs and configures them to go to a sidecar.  The
logging stack collects and aggregate logs from the sidecar as for any other
container.

#### As a cluster administrator, I want to enable access logs using my own logging infrastructure

The user enables access logs and configures them to go to a custom syslog
endpoint.

### Implementation Details

HAProxy can forward access logs to a syslog endpoint, and openshift-router
recognizes environment variables to enable this feature and specify the syslog
endpoint (`ROUTER_SYSLOG_ADDRESS`) and facility (`ROUTER_LOG_FACILITY`) and the
log format (`ROUTER_SYSLOG_FORMAT`).  The ingress operator uses these
environment variables to configure logging to a custom syslog endpoint.

To log to a sidecar, the ingress operator configures the Deployment for the
IngressController with an additional `EmptyDir` volume and a sidecar that runs
rsyslog configured to read logs from a Unix domain socket on this volume and
write logs to standard output:

```conf
$ModLoad imuxsock
$SystemLogSocketName /var/lib/rsyslog/rsyslog.sock
$ModLoad omstdout.so
*.* :omstdout:
```

The ingress operator then uses the aforementioned environment variables to
configure HAProxy to log to the Unix domain socket.

### Risks and Mitigations

If the underlying IngressController implementation were to change away from
HAProxy to a different implementation, we would need to ensure that the new
implementation supported the same capabilities.

The meaning of the specified log format depends on the underlying
implementation.  If we were to change away from HAProxy, the semantics of the
format would likely change.  We can mitigate this risk by documenting it.

The [cluster logging log forwarding
API](../cluster-logging/cluster-logging-log-forwarding.md) may eventually
supersede elements of the Ingress logging API, so it is important to design the
Ingress logging API with the log forwarding API in mind in order to avoid
redundancy or other issues.

## Design Details

### Test Plan

The controller that manages the IngressController Deployment and related
resources has unit test coverage; for this enhancement, the unit tests are
expanded to cover the additional functionality.  The operator has end-to-end
tests; for this enhancement, a test is added that (1) configures a pod with
rsyslog, (2) creates an IngressController configured to send access logs to this
pod, and (3) verifies that the rsyslog pod receives access logs.

### Graduation Criteria

N/A.

### Upgrade / Downgrade Strategy

On upgrade, access logging is disabled by default, which is consistent with the
feature's absence in older versions.  On downgrade, the IngressController
Deployment is updated, and any sidecar and `EmptyDir` volume are automatically
removed from the Deployment.  A harmless ConfigMap with rsyslog configuration
may remain (with an owner reference on the Deployment).

### Version Skew Strategy

N/A.

## Implementation History

In OpenShift 1.3/3.3, the `ROUTER_SYSLOG_ADDRESS` environment variable [was
added](https://github.com/openshift/origin/pull/8332/commits/311abc322d69d63c5dd03ec3dfbef7f88d039546),
and in OpenShift 1.5/3.5, the `ROUTER_LOG_FACILITY` environment variable [was
added](https://github.com/openshift/origin/pull/12795/commits/c01fab1b76388e213f595ccc99dcbfdc5d1f1401);
these environment variables are used to implement the logging API.

In OpenShift 3.11, the `oc adm router --extended-logging` flag [was
added](https://github.com/openshift/origin/pull/20260/commits/9d6a4c2cfc6a03137dd07d645a6c7db73b0e15aa),
and in OpenShift 4.2, the `ingress.operator.openshift.io/unsupported-logging`
annotation for IngressController and Ingress config [was
added](https://github.com/openshift/cluster-ingress-operator/pull/293/commits/5a82b658c4952e8af5d4658d13e8802b93ded870);
some of this logic from `oc adm router` is re-used in the ingress operator, and
the logging API in part formalizes the unsupported annotation.

Finally,
[openshift/cluster-ingress-operator#374](https://github.com/openshift/cluster-ingress-operator/pull/374) implements this enhancement.

## Alternatives

### Cluster Logging Log Forwarding API

Instead of expanding the IngressController API, the [the cluster logging log
forwarding API](../cluster-logging/cluster-logging-log-forwarding.md) could be
expanded to encompass the necessary configuration.

#### Drawbacks

The current cluster logging log forwarding API does not support writing straight
to a syslog endpoint.

Configuring logging for individual components is outside the scope of the
cluster logging log forwarding API.

The user may want to configure ingress access logging without deploying the
logging stack.
