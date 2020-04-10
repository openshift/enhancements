---
title: Stability: Failure detector for Kube Aggregator
authors:
  - "@p0lyn0mial"
reviewers:
  - "@sttts"
approvers:
  - "@sttts"
creation-date: 2020-04-10
last-updated: 2020-04-10
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - "/enhancements/this-other-neat-thing.md"  
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Stability: Failure detector for Kube Aggregator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance, 
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this? 

## Summary

Kubernetes Aggregator uses a simple proxy implementation to send client requests to individual servers. 
The pool of available servers is maintained by the endpoint controller based on the availability of a node/pod.

In general, a server can be removed from the pool in two cases.
When a process (kubelet) on the same machine sees it as broken or when the whole node is down.
The first case doesn't take into account possibly faulty network connections from the outside.
The second might be slow as the node controller marks a node as unhealthy after 5 minutes of inactivity.

## Motivation

Numerous cases have been reported indicating that the system takes a very long time to recover from a node/pod failure. 
In the best-case scenario, the system can remain unavailable for 5 minutes. This is the time needed to mark an instance as broken by the node controller.
But there are cases in which the system is down for 15 minutes or even longer.
The failure might be a result of faulty hardware, network instability or misconfiguration.

Let’s take into account a [concrete example](https://bugzilla.redhat.com/show_bug.cgi?id=1809031) to realize how fragile the system is.
It suggests that the user was unable to create a project when 1 node out of 3 was down.
In this particular case, the failure was visible after blocking network traffic to one availability zone.

After investigation, it turned out that indeed network traffic was blocked but only inbound. 
The node wasn’t marked as broken because it could report its status back to the system because the outgoing traffic was allowed.

More cases:

- [OpenShift APIs become unavailable for more than 15 minutes](https://bugzilla.redhat.com/show_bug.cgi?id=1809031)
- [Restarting a master disconnects ocp users](https://bugzilla.redhat.com/show_bug.cgi?id=1818083)

### Goals

1. Recovering from a node/pod failure should takes seconds not minutes.
2. The failed request should be retried without impacting the user connection. Requests that don't cause side effects should be sent again to a different replica/endpoint.
3. There is a mechanism that is responsible for failure detection and propagation. Suspicious or faulty endpoints should be taken out of the pool of healthy replicas.
4. Metrics are collected that help administrators detect and fix anomalies. 

### Non-Goals

## Proposal

Improve/Change Kube Aggregator so that it has:

1. a retry mechanism that needs to detect which requests are safe to be sent again. On each try it need to pick up a different endpoint.
2. a reporting component that collects data about every request: success, failure, latency.
3. a failure detector that inspects collected data and marks endpoints as broken/healthy. It needs to have some flow control mechanism (batching), be able to remove data that are no longer needed (removed endpoints) and only store X amount of records.
4. a new service/endpoint resolver that picks up only healthy endpoints based on data from the failure detector.

### User Stories [optional]

Detail the things that people will be able to do if this is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

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
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

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
