---
title: haproxy-log-format
authors:
  - "@rohara"
reviewers:
  - "@Miciah"
  - "@knobunc"
  - "@alebedev87"
approvers:
  - "@Miciah"
  - "@knobunc"
  - "@alebedev87"
api-approvers:
  - "@knobunc"
creation-date: 2025-05-01
last-updated: 2025-08-18
status:
see-also: logging-api
replaces:
superseded-by:
tracking-link:
  - "https://issues.redhat.com/browse/NE-2039"
---

# HAProxy Log Formats

## Summary

This enhancement extends the IngressController API to allow the user
to configure different log formats for HAProxy. Currently, users may
only specify the `HttpLogFormat`, which is only used for HTTP traffic
with no SSL termination or passthrough. This enhancement would
introduce both `HttpsLogFormat` and `TcpLogFormat` such that users
can define the log formats.

## Motivation

Users desire the ability to log extra SSL information when
applicable. This is useful when debugging SSL related issues. As such,
the user may want to define the log format similarly to how the API
allows for HTTP log format.

For completeness, a user may also want to define the log format for
TCP logging. This is useful SSL passthrough.

### User Stories

This enhancement will allow users to customize log formats for
HttpsLogFormat and TcpLogFormat, similarly to how HttpLogFormat can
currently be customized. Additionally, using HttpsLogFormat will allow
users to log useful SSL-related information that may assist in
troubleshooting.

### Goals

1. Provide an option to customize HTTPS log format.
2. Provide an option to customize TCP log format.

### Non-Goals

Since all access logs are collected at a single endpoint, it is beyond
the scope of this proposal to allow different log formats to be
collected in separate logs.

## Proposal

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
        // http://docs.haproxy.org/2.8/configuration.html#8.2.3
        //
        // +optional
        HttpLogFormat string `json:"httpLogFormat,omitempty"`

        // httpsLogFormat specifies the format of the log messages for
        // an HTTPS request
        //
        // If this field is empty, log messages use the implementation's default
        // HTTPS log format.  For HAProxy's default HTTPS log format, see the
        // HAProxy documentation:
        // http://docs.haproxy.org/2.8/configuration.html#8.2.4
        //
        // +optional
        HttpsLogFormat string `json:"httpsLogFormat,omitempty"`

        // tcpLogFormat specifies the format of the log messages for a
        // TCP request.
        //
        // If this field is empty, log messages use the implementation's default
        // TCP log format.  For HAProxy's default TCP log format, see the
        // HAProxy documentation:
        // http://docs.haproxy.org/2.8/configuration.html#8.2.2
        //
        // +optional
        TcpLogFormat string `json:"tcpLogFormat,omitempty"`
}
```

The user may specify `spec.logging.access.httpLogFormat` to customize
the HTTP log format. The user may also specify
`spec.logging.access.httpsLogFormat` to customize the HTTPS log
format. The user may also specify `spec.logging.access.tcpLogFormat`
to customize TCP log format. For example, the following is the
definition for an IngressController that customizes each of the log
formats (HTTP, HTTPS, and TCP):

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
      httpLogFormat: "%ci:%cp [%tr] %ft %b/%s %TR/%Tw/%Tc/%Tr/%Ta %ST %B %CC ..."
      httpsLogFormat: "%ci:%cp [%tr] %ft %b/%s %TR/%Tw/%Tc/%Tr/%Ta %ST %B %CC ..."
      tcpLogFormat: "%ci:%cp [%t] %ft %b/%s %Tw/%Tc/%Tt %B %ts ..."
```

Each of the log format fields contains a free-form format string, so
none of these are validated.

### Workflow Description

**Cluster admin** is a human user responsible for deploying a cluster.

**User** is a human user responsible for developing and deploying an application in a cluster.
1. Cluster admin wants to set custom log format for HTTP, HTTPS, and TCP for all the routes of an Ingress Controller.
   Cluster admin edits/create the CR pf the Ingress Controller having Spec as follows -
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
      httpLogFormat: '%ci:%cp [%tr] %ft %b/%s %TR/%Tw/%Tc/%Tr/%Ta %ST %B %CC  ...'
      httpsLogFormat: '%ci:%cp [%tr] %ft %b/%s %TR/%Tw/%Tc/%Tr/%Ta %ST %B %CC ...'
      tcpLogFormat: '%ci:%cp [%t] %ft %b/%s %Tw/%Tc/%Tt %B %ts ...'
```

### API Extensions

This proposal adds two fields (`HttpsLogFormat` and `TcpLogFormat`) to
the existing `AccessLogging` structure.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

Since MicroShift does not run the cluster-ingress-operator, there is
no impact to MicroShift.

### Implementation Details/Notes/Constraints

HAProxy can forward access logs to a syslog endpoint, and openshift-router
recognizes environment variables to enable this feature and specify
the log formats. Currently the only log format is HttpLogFormat
(`ROUTER_SYSLOG_FORMAT`). Additional environment variables for
HttpsLogFormat (`ROUTER_HTTPS_LOG_FORMAT`) and TcpLogFormat
(`ROUTER_TCP_LOG_FORMAT`) will be recognized by the openshift-router
to configure custom log formats for HTTPS and TCP logs, respectively.

Note that the current environment variable for HttpLogFormat
(`ROUTER_SYSLOG_FORMAT`) is non-specific and should be renamed to
(`ROUTER_HTTP_LOG_FORMAT`). The old environment variable
(`ROUTER_SYSLOG_FORMAT`) should still be honored, but the new
environment variable (`ROUTER_HTTP_LOG_FORMAT`) will take precedence.

### Risks and Mitigations

The meaning of the specified log formats depends on the underlying
implementation.  If we were to change away from HAProxy, the semantics of the
format would likely change.  We can mitigate this risk by documenting
it.

### Drawbacks

N/A

## Alternatives (Not Implemented)

It is possible to configure both `httpslog` and `tcplog` formats by
directly adding those options to the HAProxy configuration
template. While this would be an improvement, it would not allow for
custom log formats as can currently be done for `HttpLogFormat`. In
contrast, this proposal aims to provide a method to define custom log
formats for `HttpsLogFormat` and `TcpLogFormat` that are consistent
with the existing `HttpLogFormat`.

## Open Questions [optional]

N/A

## Test Plan

There are no planned e2e or integration tests for this
enhancement. This is largely due to the fact that log format is a
free-form string.

## Graduation Criteria

N/A

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

An upgrade/downgrade of the IngressOperator will result is the router
image also being upgraded/downgraded. As such, openshift-router will
recognize the new variables for log formats.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

## Support Procedures

If any format string contains an invalid variable, HAProxy will exit
immediately without providing a descriptive log message. For example, if a user includes a unrecognized format alias (eg. %GG), haproxy will emit the following error message:

```
[ALERT]    (168045) : config : Parsing [/etc/haproxy/haproxy.cfg:34]: failed to parse log-format : no such format alias 'GG'. If you wanted to emit the '%' character verbatim, you need to use '%%'.
[ALERT]    (168045) : config : Fatal errors found in configuration.
```

## Infrastructure Needed [optional]
