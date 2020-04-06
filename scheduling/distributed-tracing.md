---
title: Add distributed tracing to operators and operands
authors:
  - "@damemi"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-04-06
last-updated: 2020-04-06
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/0034-distributed-tracing-kep.md
replaces:
  - n/a
superseded-by:
  - n/a
---

# Distributed Tracing for Operators and Operands

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

This proposes adding distributed tracing functionality to operators and the components they manage.
Recommending the [Open Telemetry](https://github.com/open-telemetry/opentelemetry-go) library, this
enhancement would provide the benefit similar to a normal stack trace, with the added context of
traces across services to give a view of the full lifecycle of affected resources in a single location
with shared contexts.

## Motivation

To improve debuggability for resources that are consumed and modified by multiple different actors, this will
provide a broader view of the changes a resource is undergoing with the efficiency of existing libraries and tools.

### Goals

1. Add tracing to operators
2. Add tracing to the components managed by the operators

### Non-Goals

1. Develop our own tools to provide enhanced tracing beyond what is available already

## Proposal

This is where we get down to the nitty gritty of what the proposal actually is.

### User Stories [optional]

Detail the things that people will be able to do if this is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.

#### Story 1

The kube-scheduler goes through many internal decisions when filtering and scoring nodes
for pods to be placed on. Adding another layer on top of this is the Kube Scheduler Operator,
which manages the scheduler configuration and kube-scheduler pods themselves.

Currently, it is very difficult to get the "big picture" of why a pod was scheduled onto a certain node.
Even with logging, this does not provide a clear insight into the decisions of the scheduler and what
configurations of the cluster affect them.

With distributed tracing, any interactions between components involved in scheduling would share a context
that allows them to add trace steps to the current flow. For example:

1. The kube-scheduler-operator updates the kube-scheduler config to add a new score plugin
2. This configuration change is propogated to the kube-scheduler
3. A new pod is created and sent for scheduling
4. The scheduler analyzes all filter plugins against its config
5. The scheduler analyzes all score plugins
6. The pod is placed on a node

This is an abbreviation but demonstrates the basic flow of how our components could share a context to
trace across distributed actions.

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
