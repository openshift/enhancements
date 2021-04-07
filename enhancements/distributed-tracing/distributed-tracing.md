---
title: distributed-tracing-with-opentelemetry
authors:
  - "@sallyom"
  - "@husky-parul"
  - "@damemi"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-04-14
last-updated: 2021-10-11
status: informational
---

# Distributed Tracing with OpenTelemetry

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenTelemetry tracing is an experimental feature added in etcd 3.5 and Kubernetes APIServer v1.22.
This enhancement proposal will add the option of enabling tracing features in OpenShift as they are
added upstream. Currently there is a KEP being targeted for Kubernetes 1.24 to add tracing instrumentation
in the kubelet, as well as in the main branch of CRI-O. Tracing can be added as an option in the OpenShift
deployment configurations for these components but will require minor updates to deployment manifests.

In distributed systems tracing gives valuable data that logs and metrics cannot provide. This enhancement
will provide the steps required to configure distributed tracing in an OpenShift deployment along with
the steps to deploy a vendor-agnostic collector capable of exporting any OpenTelemetry data to analysis backends.
Some open-source tracing backends for OpenTelemetry are Jaeger and Zipkin.
Here is a list of [vendors that currently support OpenTelemetry.](https://opentelemetry.io/vendors/)

OpenTelemetry tracing has the ability to quickly detect and diagnose problems as well
as improve performance of distributed systems and microservices. By definition,
distributed tracing tracks and observes service requests as they flow through a system by
collecting data as requests go from one service to another. It's possible, then, to
pinpoint bugs and bottlenecks or other issues that can impact overall system performance.
Tracing provides the story of an end-to-end request that is difficult to get otherwise.

The OpenTelemetry Collector component can ingest data in OTLP format and be configured to forward
and export data to a variety of vendor-specific backends, as well as backends that can ingest data
in OTLP, Jaeger, or Zipkin formats.

## Motivation

As platforms and applications become more distributed and built on microservices or serverless, tracing
provides an overall picture of system performance. This visibility reveals service dependencies and how
one component affects another, things which are difficult to observe otherwise. For example, many OpenShift
bugs or issues are not contained to a single component. Instead, several teams and component owners often
work together to solve issues and make system improvements. Distributed tracing aids this by
tracking events across service boundaries. Furthermore, tracing can shrink the time it takes to diagnose issues,
giving useful information and pinpointing problems without the need for extra code. Upstream, etcd has been
instrumented to export gRPC traces. CRI-O is also adding instrumentation. Kubernetes API server added the option
to enable OpenTelemetry tracing in version 1.22. A KEP is under review and work is underway to instrument kubelet.
A POC has been created with kube-scheduler. With these components instrumented, it will be possible to view traces with
CRI-O <-> Kubelet <-> Kube-Apiserver <-> ETCD. At this point, there is much to gain in instrumenting
other components and extending the OpenTelemetry train to give a complete view of the system.

### Goals

Provide an easy way for OpenShift components to add instrumentation for distributed tracing using OpenTelemetry.
Adding OpenTelemetry tracing spans requires only a few lines of code. The more components that add tracing,
the more complete the picture will be for anyone who is debugging or trying to understand cluster performance.
Also, a vendor-agnostic OpenTelemetry Collector with an operator in the works on OperatorHub can be
temporarily deployed in times of debugging to turn on tracing, and removed when no longer needed. Any component
that adds instrumentation should add a switch to turn tracing on. It should be easy to enable
and disable tracing. If enabled but no backend is detected, there should be no performance hit or trace exporter
connection errors in component logs.

### Non-Goals

The OpenTelemetry Collector operator for OpenShift will not be part of core OpenShift. Instead, the operator is available
on the OperatorHub in the OpenShift console, or, can be deployed manually.

## Proposal

As of Kubernetes 1.22, it is possible to enable OpenTelemetry tracing in etcd and in kube-apiserver. Enabling tracing in these
components in OpenShift will require minor changes. The kube APIServer expects a tracing configuration file as well as the tracing
feature gate enabled. Etcd requires experimental tracing flags in order to generate spans and export trace data. This enhancement
will also serve as a guideline for anyone who wishes to add tracing to their components or applications.

### User Stories

* As a cluster administrator, I want to easily switch on or off OpenTelemetry tracing.
* As a cluster administrator, I want to diagnose performance issues with my component or service.
* As a cluster administrator, I want to inspect the service boundary between core components.
* As a component owner, I want to instrument code to enable OpenTelemetry tracing.
* As a component owner, I want to propagate contexts with OpenTelemetry trace data to other components.

### Implementation Details/Notes/Constraints [optional]

OpenTelemetry Collector and Jaeger operators are currently available on the OperatorHub in the OpenShift Console.
Adding the experimental tracing flags and enabling the tracing feature-gate in etcd and kube-APIServer is only
half of what is required to collect trace data. The other piece is to document the deployment of the
OpenTelemetry Collector as well as best practices when exporting data to a tracing analysis backend.

### Risks and Mitigations

There is a slight performance hit when collecting trace data. Instrumentation wraps HTTP and gRPC calls to generate
and export OTLP data, and this consumes CPU and memory. The cost can be minimized by configuring the collectors, processors,
and exporters. Because of this tracing should not be enabled by default and the performance should be closely monitored in
environments where tracing is switched on to better understand the cost vs. the benefit of collecting traces.

## Design Details

### Etcd

* Document how to configure etcd to export traces and how to switch tracing on & off

### Kube APIServer

* Add the TracingConfiguration file flag to OpenShift
* Allow the Tracing feature-gate to be enabled

### Trace Collection

Provide clear documentation for how to collect and visualize trace data from OpenShift.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Upstream e2e, integration, and unit tests exist for tracing in etcd and apiserver.
- Additional testing is necessary to support any OpenShift modifications required to turn on tracing.

Any added code will have adequate unit and integration tests added to openshift-tests suites.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Upgrade expectations: TODO
Downgrade expectations: TODO

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Kube APIServer Tracing: https://kubernetes.io/docs/concepts/cluster-administration/system-traces/

Etcd Tracing: https://github.com/etcd-io/etcd/pull/12919

CRI-O Tracing: https://github.com/cri-o/cri-o/pull/4883

Kubelet Tracing KEP: https://github.com/kubernetes/enhancements/pull/2832

## Drawbacks

The performance cost of implementing distributed tracing must outweigh the benefits.

## Alternatives

[kspan](https://github.com/weaveworks-experiments/kspan) is an experimental project upstream.
It turns events into OpenTelemetry spans. The spans are joined by causality and grouped together into traces.
