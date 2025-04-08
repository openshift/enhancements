---
title: cluster-logging-otel-support
authors:
  - "@jcantrill"
reviewers:
  - "@alanconway, Red Hat Logging Architect"
  - "@periklis, Red Hat Logging team lead"
  - "@xperimental, Red hat Log Storage team member"
  - "@JoaoBraveCoding, Red hat Log Storage team member"
approvers:
  - "@alanconway, Red Hat Logging Architect"
api-approvers:
  - "@alanconway"
  - "@periklis"
creation-date: 2024-09-25
last-updated: 2024-10-08
tracking-link:
  - https://issues.redhat.com/browse/LOG-5637
see-also:
  - "/enhancements/cluster-logging/forwarder-to-otlp.md"
  - "/enhancements/cluster-logging/logs-observability-openshift-io-apis.md"
replaces: []
superseded-by: []
---

# In-Cluster End-to-End Support for OpenTelemetry

## Summary

Red Hat Observability is adopting the standards defined by the [OpenTelemetry Observability framework](https://opentelemetry.io)
which includes collection and storage of log streams.  The in-cluster Red Hat logging solution
requires modifications in order for logs to be normalized to [OTEL semantic conventions](https://opentelemetry.io/docs/specs/semconv/),
written to storage using the [OpenTelemetry protocol](https://opentelemetry.io/docs/specs/otlp/), and searchable in an
OpenShift cluster's web Console.

## Motivation

### User Stories

* As an administrator, I want to deploy an in-cluster logging solution that makes use of the OpenTelementry Observability framework
so I can make correlations with other observability signals (i.e. metrics, tracing) in an industry, standardized way
* As a developer, I want users to be able to query log records using OTEL or [ViaQ](https://github.com/openshift/cluster-logging-operator/blob/release-5.9/docs/reference/datamodels/viaq/v1.adoc)
attributes: log type, namespace, pod name, container name to facilitate LokiStack's transition from using ViaQ to OTEL
* As a developer, I want Red Hat LokiStack to provide a native OTLP ingestion point
so that other Red Hat products can forward observability signals using OTEL and rely upon OpenShift Logging tenancy and RBAC integration

The administrator role is any user who has permissions to deploy the operator and the cluster-wide resources required to deploy the logging components.
The developer role is any member of the Red Hat OpenShift Logging team.

### Goals

* Provide an in-cluster log search and alerting experience based on [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)
* Provide an OTLP-native log ingestion for trusted and verified collectors.
* Provide native zero-effort OpenShift Logging Tenancy and RBAC integration to verified collectors
* Provide a transition period for users to continue to use the ViaQ model until they must modify their logQL queries to use OTEL

### Non-Goals

* Entirely replace the [ViaQ](https://github.com/openshift/cluster-logging-operator/blob/release-5.9/docs/reference/datamodels/viaq/v1.adoc) data model with OpenTelemetry Semantic Conventions.
* In-cluster log storage for public, non-verified log ingestion.

## Proposal

In order to realize the goals of this enhancement:

* Modify **cluster-logging-operator** to allow writing to **LokiStack** over ViaQ(the default) or OTLP based upon a **ClusterLogForwarder** annotation
* Modify the vector log collector deployment to write to **LokiStack** over OTLP and additionally include select (MVP) ViaQ attributes
* Update the **loki-operator** to [provide an API](https://issues.redhat.com/browse/LOG-5673) to configure Loki to ingest logs through an OTLP endpoint
* Update the **loki-operator** to provide sane defaults to the new LokiStack OTEL API for server-side indexing of OTEL resource attributes to improve query performance
* Modify the **Logging UIPlugin** to use OTEL attributes that correspond to ViaQ MVP attributes

### Workflow Description

The following workflow describes deployment of a full logging stack to collect and forward logs to a Red Hat managed log store, **LokiStack**, using OTLP.

**cluster administrator** is a human responsible for:

* Managing and deploying day 2 operators
* Managing and deploying an in-cluster LokiStack
* Managing and deploying a cluster-wide log forwarder
* Managing and deploying a OpenShift console UI plugin

**cluster-observability-operator** is an operator responsible for:

* managing and deploying observability operands and console plugins (e.g Logging UIPlugin)

**loki-operator** is:

* an operator responsible for managing a LokiStack instance.

**cluster-logging-operator** is:

* an operator responsible for managing log collection and forwarding.

The cluster administrator does the following:

1. Deploys the Red Hat **cluster-observability-operator**
1. Deploys the Red Hat **loki-operator** which supports server-side indexing and OTLP ingestion endoint
1. Deploys an instance of **LokiStack** into the `openshift-logging` namespace
1. Deploys the Red Hat **cluster-logging-operator**
1. Creates a **ClusterLogForwarder** custom resource for the **LokiStack**
    1. **NOTE:** In Tech Preview, this resources includes an annotation to instruct the operator to use the OTEL datamodel in lieu of ViaQ

The **cluster-observability-operator**:

1. Deploys the console **Logging UIPlugin** for reading logs in the OpenShift console

The **loki-operator**

1. Deploys the **LokiStack** for storing logs in-cluster

The **cluster-logging-operator**:

1. Deploys the log collector to forward logs to log storage in the `openshift-logging` namespace


### API Extensions

During the time this feature is considered Tech Preview, instances of **ClusterLogForwarder** will be [annotated](./forwarder-to-otlp.md) to enable
forwarding to OTLP receivers. Additionally, a field will be added to the **LokiStack** output type where administrators can configure the desired
data model of forwarded logs.

Example of configuring multiple outputs:

```yaml
apiVersion: observability.openshift.io/v1
kind: ClusterLogForwarder
metadata:
  annotations:
    observability.openshift.io/tech-preview-otlp-output: enabled
spec:
  outputs:
  - name: my-loki-otel
    type: lokiStack
    lokiStack:
      dataModel: otel
  - name: my-other-loki-otel
    type: lokiStack
    lokiStack:
      dataModel: otel
  - name: my-legacy-loki                  #dataModel: viaq
    type: lokiStack
  pipelines:
  - name: in-to-out
    outputRefs: [my-loki-otel,my-other-loki-otel,my-legacy-loki]
    inputRefs: [application]
```

This example configures a collector to forwards logs to multiple LokiStacks (e.g. my-loki-otel,my-other-loki-otel) using OTEL semantic attributes while
also forwarding logs to a separate LokiStack (e.g. my-legacy-loki) using ViaQ.

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

#### Log Storage

* The **loki-operator** will be updated so deployments of Lokistack rely upon the OTLP API instead of the Push API. This will allow
trusted and verified Red Hat products, like Red Hat Build of the OpenTelemetry Collector, to forward logs to the same LokiStack as the Red Hat OpenShift Logging.  Additionally, this new version of the **loki-operator**
will support server-side indexing to improve query performance and simplify configuration on the client side. Sane defaults will be provided for users using the `openshift-logging` tenancy.
* The **loki-operator** will configure Loki deployments with server-side indexing of OTEL and Minimal ViaQ Product Labels (MVP).  The Red Hat Observability Logging 
[Data Model](https://github.com/rhobs/observability-data-model/pull/6)
provides additional details.

#### Log Visualization

The **Logging UIPlugin** will be modified to replace its UI widgets to rely upon OTEL attributes in lieu of ViaQ.  These changes will be
released to production one release after the other components achieve **GA**.  Once this milestone is realized, the **Logging UIPlugin** will
no longer rely upon ViaQ attributes which may break existing log queries.

#### Log Collection and Forwarding

* The field `dataModel` of type enum with values: `""`,`viaq`,`otel` will be added to the **LokiStack** output type
* The **cluster-logging-operator** will be updated to evaluate the **LokiStack** `dataModel` field:
    * Values `""` or `viaq` implies forwarding to **LokiStacks** using ViaQ data model
    * Value of `otel`:
        * will internally initialize OTLP outputs for the enabled **LokiStacks**, in lieu of Loki outputs

Once this enhancement achieves **GA**, OTEL will be the default data model when forwarding logs to **LokiStack**.  Cluster administrators will
be able to support the **Tech Preview** behavior by setting the data madel field to `viaq`:


* The **cluster-logging-operator** will be updated to evaluate the **LokiStack** `dataModel` field:
    * Values `""` or `otel` implies forwarding to **LokiStacks** using OTEL data model
    * Value of `viaq`:
        * will internally initialize Loki outputs for the enabled **LokiStacks**, in lieu of OTLP outputs

### Risks and Mitigations

### Drawbacks

This design offers a transitional plan to allow users to change the enrichment model of their log storage while continuing to use the console web UI plugin and
MVP attributes.  Advanced queries using [logQL](https://grafana.com/docs/loki/latest/query/) that rely upon attributes other than MVP may no longer function.


## Open Questions [optional]

1. Do we need to forward the complimentry UI OTEL attributes when configured for ViaQ?  
    A: *YES*, all OTEL receivers will receive the same set of attributes as defined in the [Data Model](https://github.com/rhobs/observability-data-model/pull/6).

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to conditionally configure the datamodel(ViaQ or OTEL) when forwarding to in-cluster LokiStack
- Ability to retrieve OTEL formatted log entries from in-cluster LokiStack using MVP attributes
- Ability to use existing log based alerts after logging upgrade without changes
- Announce deprecation of log forwarding to LokiStack using ViaQ data model, to be removed **two** logging releases after GA

### Tech Preview -> GA

- Console UI plugin enhanced to replace MVP attribute dependencies with OTEL equivalents
- Forwarding to in-cluster LokiStack with OTEL datamodel by default
- Gather feedback from consumers of Red Hat OpenShift Logging
- Official, Red Hat user facing documentation

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives (Not Implemented)


As an alternative to the above proposed solution (support of OpenTelemetry Semantic Conventions formatted log records via OTLP in addition to ViaQ) 
we could provide a mapping facility from OpenTelemetry Semantic Conventions to ViaQ. Considering the fact that the Log Console is written with ViaQ
this alternative would make changes in the UI obsolete. However with the growing amendments in the OpenTelemetry Semantic Conventions we would need to keep up with that pace.

## Infrastructure Needed [optional]

