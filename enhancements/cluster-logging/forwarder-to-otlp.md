---
title: forwarder-to-otlp
authors:
  - "@jcantrill"
reviewers:
  - "@alanconway"
  - "@cahartma"
  - "@pavolloffay"
  - "@periklis"
  - "@xperimental"
approvers:
  - "@alanconway"
api-approvers: 
  - "@alanconway"
creation-date: 2024-04-30
last-updated: 2024-04-30
tracking-link:
  - "https://issues.redhat.com/browse/LOG-4225"
see-also:
  - "/enhancements//cluster-logging/cluster-logging-v2-apis.md"
replaces: []
superseded-by: []
---

# Log Forwarder to OTLP endpoint

Spec **ClusterLogForwarder.obervability.openshift/io** to forward logs to an **OTLP** endpoint

## Summary

The enhancement defines the modifications to the **ClusterLogForwarder** spec 
and Red Hat log collector to allow administrators to collect and forward logs 
to an OTLP receiver as defined by the [OpenTelemetry Observability framework](https://opentelemetry.io/docs/specs/otlp/)

## Motivation

Customers continue to look for greater insight into the operational aspects of their clusters by using
observability tools and open standards to allow them to avoid vedor lock-in.  OpenTelemetry

> ... is an Observability framework and toolkit designed to create and manage telemetry 
data such as traces, metrics, and logs. Crucially, OpenTelemetry is vendor- and tool-agnostic, meaning 
that it can be used with a broad variety of Observability backends...as well as commercial offerings.

This framework defines the [OTLP Specification](https://opentelemetry.io/docs/specs/otlp/) for vendors
to implement.  This proposal defines the use of that protocol to specifically forward logs collected on
a cluster.  Implementation of this proposal is the first step to embracing a community standard
and to possibly deprecate the data model currently supported by Red Hat logging.

### User Stories

* As an administrator, I want to forward logs to an OTLP enabled, Red Hat managed LokiStack
so that I can aggregate logs in my existing Red Hat managed log storage solution.
**Note:** The realization of this use-case depends on [LOG-5523](https://issues.redhat.com/browse/LOG-5523)

* As an administrator, I want to forward logs to an OTLP receiver
so that I can use my observability tools to evaluate all my signals (e.g. logs, traces, metrics).

### Goals

* Implement OTLP over HTTP using text(i.e. JSON) in the log collector to forward to any receiver that implements OTLP
**Note:** Stretch goal to implement OTLP over HTTP using binary(i.e. protobuf)

* Deprecate the [ViaQ Data Model](https://github.com/openshift/cluster-logging-operator/blob/release-5.9/docs/reference/datamodels/viaq/v1.adoc) in
favor of [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)
* Allow customer to continue to use **ClusterLogForwarder** as-is in cases where they are not ready or not interested in OpenTelementry

### Non-Goals

* Replace the existing log collection agent with the [OTEL Collector](https://opentelemetry.io/docs/collector/)

## Proposal

This section should explain what the proposal actually is. Enumerate
*all* of the proposed changes at a *high level*, including all of the
components that need to be modified and how they will be
different. Include the reason for each choice in the design and
implementation that is proposed here.

To keep this section succinct, document the details like API field
changes, new images, and other implementation details in the
**Implementation Details** section and record the reasons for not
choosing alternatives in the **Alternatives** section at the end of
the document.

This proposal inte

### Workflow Description

**cluster administrator** is a human responsible for administering the **cluster-logging-operator**
and **ClusterLogForwarders**

1. The cluster administrator deployes the cluster-logging-operator if it is already not deployed
1. The cluster administrator edits or creates a **ClusterLogForwarder** and defines an OTLP output
1. The cluster administrator references the OTLP output in a pipeline
1. The cluster-logging-operator reconciles the **ClusterLogForwarder**, generates a new collector configuration,
and updates the collector deployment


### API Extensions

```yaml
apiVersion: "observability.openshift.io/v1"
kind: ClusterLogForwarder
spec:
outputs:
- name:
  type:           # add otlp to the enum
  tls:
    secret:             #the default resource to search for keys
      name:                # the name of resource
      cacert:
        secret:               #enum: secret, configmap 
          name:                # the name of resource
        key:                 #the key in the resource
      cert:
        key:                 #the key in the resource
      key:
        key:                 #the key in the resource
    insecureSkipVerify:
    securityProfile: 
  otlp:
    url:                   #must terminate with '/v1/logs'
    authorization:
      secret:              #the secret to search for keys
        name:
      username:
      password:
      token:
    tuning:
        delivery:
        maxWrite:            # quantity (e.g. 500k)
        compression:         # gzip,zstd,snappy,zlib,deflate
        minRetryDuration:
        maxRetryDuration:
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

There is a separate log collection and forwarding effort to support writing logs directly to the hosted infrastructure (e.g. AWS, Azure, GCP) using
short-lived tokens and native authentication schemes.  This proposal does not address this specifically for HCP and assumes any existing
ClusterLogForwarders will continue to use existing functionality to support HCP.

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

The [log collector](https://github.com/vectordotdev/vector/issues/13622), at the time of this writing, does not directly support OTLP. This proposal,
intends to provide OTLP over HTTP using a combination of the HTTP sink and transforms.  Initial work demonstrates this is feasible by verifying
the log collector can successfully write logs to a configuration of the OTEL collector.  The vector components required to make this possible:

* Remap transform that formats records according to the OTLP specification and OpenTelemetry scemantic conventions
* Reduce transform to batch records by resource type (e.g. container workloads, journal logs, audit logs)
* HTTP sink to forward logs

### Risks and Mitigations

This change is relatively low risk since we have verified its viability and we do not need
to wait for an upstream change in order to release a usable solution.

### Drawbacks

Many customers see the trend and advantage of using OpenTelemetry
and have asked for its support. By not providing a solution, the Red Hat log collection product
is not following industry trends and risks becoming isolated from the larger observability community.

## Open Questions [optional]

## Test Plan

* Verify forwarding logs to the OTEL collector
* Verify forwarding logs to Red Hat managed LokiStack

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

Not Applicable since this adds a new feature

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives

Wait for the upstream collector project to implement OTLP sink

## Infrastructure Needed [optional]

