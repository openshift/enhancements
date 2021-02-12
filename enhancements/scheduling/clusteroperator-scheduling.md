---
title: clusteroperator-scheduling
authors:
  - "@michaelgugino"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-02-12
last-updated: 2021-02-12
status: implementable
---

# ClusterOperator Scheduling


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

To ensure system components, especially ClusterOperators, have the correct
set of tolerations.

## Motivation

The motivation is to create an easy to follow reference for developing and
updating ClusterOperators and similar system components.

### Goals

* List best-practices for tolerations

### Non-Goals

* Non-standard cluster profiles such as single node
* Add all practices here today, we can add them over time.

## Proposal

### User Stories

#### Story 1

As a cluster administrator, I want upgrades to proceed safely and without
intervention.

#### Story 2

As an OpenShift developer, I want to be able to reference a guide to ensuring
my ClusterOperators are properly defined for scheduling purposes.

### Implementation Details/Notes/Constraints [optional]

Not all elements of scheduling are defined here.  There are several, and we
can update this document as needed.

### Risks and Mitigations

Not all ClusterOperators behave identically.  Careful consideration for each
ClusterOperator is paramount.

## Design Details

### NoSchedule Tolerations

#### node.kubernetes.io/unschedulable

There are various `NoSchedule` taints that are commonly applied to nodes.
ClusterOperators should not tolerate NoSchedule taints with key:
`node.kubernetes.io/unschedulable`

This is a special key that is applied when a node is cordoned, such as during
drain.  Since this is a NoSchedule taint, anything without this taint will
not automatically be evicted, it will only prevent future scheduling.

During upgrades, we cordon and drain all nodes.  Sometimes the scheduler will
place pods tolerating NoSchedule onto a node quicker than the loop for
draining a node can complete.  This results in hot-looping between draining
and the scheduler.

Any other NoSchedule toleration is fine.

### Open Questions [optional]



### Test Plan

Mostly everything should work identically in CI.

## Implementation History


## Drawbacks

Adjusting scheduling parameters can cause undesired effects in some cases.

## Alternatives

Without these guidelines, we might face difficult situations for automated
operations such as upgrades.
