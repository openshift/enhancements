---
title: Alerting as a feature
authors:
  - "@dofinn"
reviewers:
  - "@smarterclayton"
  - "@jeremyeder"
  - "@michaelgugino"
  - "@s-urbaniak"
  - "@lilic"
  - "@RiRa12621"
  - "@wking"
  - "@cblecker"
  - "@jharrington22"
  - "@mwoodson"
approvers:
  - TBD
creation-date: 2021-03-08
last-updated: 2021-03-08
status: implementable
consumes:
  - [Add: Alerting Standards](https://github.com/openshift/enhancements/pull/637)
---

# Alerting as a Feature

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [x] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Definitions

Caused Based Alert: Describes the direct cause of an issue. Example: etcdMemoryUtilization @ 100%

Symptom Based Alert = Describes a symptom who's source is a cause based alert. Example: etcd connection latency > 200ms. This may be caused by etcdMemoryUtilization (or not?)

## Summary

Alerting as a feature is a holistic composition of [alerting standards](https://github.com/openshift/enhancements/pull/637) coupled with built-in alerting methodologies (none, symptom based and caused based) with an option to deliver SLOs for a single and/or fleet of clusters. 

## Motivation

OCP as a product does not have built-in [alerting standards](https://github.com/openshift/enhancements/pull/637), nor built-in SLOs driven by symptom or caused based alerting methodologies. This may be acceptable when managing a small number of clusters but at scale (+100) it breaks down. As an SRE of a cluster, you are given alerts and documentation on how to [manage them](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.6/html/monitoring/managing-alerts). This should come "out of the box" to support a natively support fleets.

There are two current efforts in the space at the moment:

* [alerting standards](https://github.com/openshift/enhancements/pull/637) -> this will have component teams own their own components alerts as they are they subject matter experts (caused based alerts). 
* SLO driven [symptom based alerting](https://docs.google.com/document/d/199PqyG3UsyXlwieHaqbGiWVa8eMWi8zzAn0YfcApr8Q/edit#heading=h.1upja8jlnnwp) is being explored by OSD/SRE. -> this will enable meaningful alerting from a customer perspective in an SLA environment.  

This raises two problems that belong to different domains.

1. Alerting standards provide teams the guidelines to instrument alerts for their components. But this does not guarantee improvement for an SRE.
2. Symptom Based alerts provide SRE's with user perspective alerting at scale(worth getting out of bed for). But this perspective is not easily illustrated to component engineering teams. 

So how can can these efforts work together in a cohesive life-cycle? 

### Goals

#### Provide two life cycles that deliver Alerting as a feature. 

* Alerting standards are:
  * defined by engineering, monitoring team and SREs (SRE assist on critical severity only). 
  * consumed by SREs + customers
  * Life-cycled by component teams, monitoring teams and SRE. 

* SLOs coupled with symptom based alerts are:
  * defined by monitoring team and SREs.
  * Implemented by SRE
  * Lifecycled by monitoring team and SRE. 
  * Developed, tested and validated by SRE and eventually committed back to the product.

#### Provide options for alerting methods (these could be called Alerting Profiles. They should just be alertmanager configurations). 

1. None -> I want to configure my own alerting completely. 
2. Cause Based -> Send all critical alerts to a defined receiver. 
3. SLO Symptom Based Alerting -> OCP has the ability accept SLOs for primary components like API, Ingress/routes. SLO's are paired with symptom based alerts. Symptom based alerts can also exist in isolation without an SLO (Unsure how meaningful the alert becomes in this situation). 

### Non-Goals

* 

## Proposal


### User Stories

#### Story 1 - SRE - Alerting Profile = none.

> As an SRE running OCP, I want to configure my alerting completely myself. 

This SRE can use the already available caused based alerts that will continuously be improved by the component teams. This SRE does not care for SLOs or symptom based alerts. 

#### Story 2 - SRE - Alerting Profile = caused based.

> As an SRE, I want all critical alerts to be sent to a pager. 

This SRE can use the already available critical severity caused based alerts that will continuously be improved by the component teams. This SRE can provide a receiver (pagerduty, ops genie, etc...) and have all critical alerts be sent to it. 

#### Story 3 - SRE - Alerting Profile = symptom based.

> As an SRE, I want to define SLOs for core critical components of OpenShift and have predefined symptom based alerts to help me maintain my SLOs. 

For each core component, this SRE is able to input a desired SLO. The symptom based alert consumes the SLO and alerts accordingly. Symptom based alerts are standardized and an alertmanager config is provided so only symptom based alerts are routed to a specific receiver. This can be balanced with caused based alerts until adequate coverage of the cluster is reached with symptoms. 

Caused based alerts still play a role in this Story. When an SRE gets a symptom based alert, the potential causes will be reviewed and one will be identified as the source of the symptom. This SRE can provide feedback for cause alerts, symptom alerts and SLOs life-cycles. 

### Implementation Details/Notes/Constraints [optional]

#### Alerting Standards

Each component team will own alerts for their own component. The severity of these can be set to either info, warning or critical. An SRE can configure alertmanager to their liking (alerting profile = none)to route severities as they wish. 

Component teams will autonomously design and develop these alerts and have non-blocking SRE input on critical severities. Remembering SREs will use these 'cause' alerts to ascertain the source of symptoms so the lifecycle of these alerts will be inclusive of:

* component team
* monitoring team
* SRE

#### SLO and Symptom Based Alerts

SRE will need to define SLOs for core components of OpenShift. In the case of OSD, the apiserver is backed by an SLA so an internal SLO is pivotal to meet this agreement. SRE will develop symptom based alerts to measure and page appropriately when an SLO could be in danger of violation. 

Alerting Profile = symptom based

The need to define SLOs will help:

* drive the alerting standards lifecycle
* realize any missing component metrics required to describe a symptom based alert. 
* standardize SLO definition and management across OpenShift infrastructure. 
* drive SLOs and symptom based alerts back into the OpenShift product. 

### Risks and Mitigations

Alerting as a feature requires a receiver be configured for Alertmanager. This is unless we dynamically re-write the severities of alerts based on the alerting profile so the feature can be contained to alert manager. 

The alerts triggered natively in alertmanager help describe the current state of the cluster. These alerts are reviewed only when a symptom is triggered and sent via a receiver. 

Example flow:

1. Symptom based alert received via pagerduty
2. SRE logs into cluster.
3. SRE reviews firing alerts from in-cluster.
4. SRE is able to ascertain the cause of the symptom based on in-cluster alerts that did not reach the pager. 
5. SRE attends to the cause.
6. SRE/Engineering remediate (Bugzilla) cause in an attempt to mitigate it being a future cause for another symptom alert. 

## Design Details

### Open Questions [optional]

1. Does this make OCP too opinionated? 

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

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

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to this should be
  identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

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

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
