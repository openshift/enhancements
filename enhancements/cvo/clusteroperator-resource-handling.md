---
title: clusteroperator-resource-handling
authors:
  - "@deads2k"
reviewers:
  - "@abhinavdahiya"
  - "@sdodson"
approvers:
  - "@abhinavdahiya"
  - "@derekwaynecarr"
creation-date: 2019-12-11
last-updated: 2019-12-11
status: implementable
see-also:
replaces:
superseded-by:
---

# ClusterOperator Resource Handling

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

`clusteroperators.config.openshift.io` are used to determine success of installation.
They are also used to drive collection of debugging data from tools like `oc adm inspect` and `oc adm must-gather`.
The `clusteroperator` resources in a payload should always be present, even before installation of particular
operators to see which `clusteroperators` need to report in and to allow establishing `.status.relatedResources` before
an operator pod runs.
This is critical for debugging clusters that fail to install or fail to upgrade with new operators present.

## Motivation

Debugging information for clusters that succeed in bootstrapping, but fail during installation is missing most
of the data required to resolve the issue via must-gather.

### Goals

1. allow collection of debugging data for failed installs using normal tools. 

### Non-Goals

1. create a new tool to gather data for failed installs after bootstrapping.

## Proposal

1. `clusteroperator` resources in the payload should be created with the required status conditions (available, progressing,
   degraded) set to `Unknown`.
2. `clusteroperator` creation by the CVO needs to honor or update `.status.relatedResources`.  This requires updating
    status after the creation.
3.  `clusteroperator` resources in the payload should all be created immediately regardless of where in the payload ordering
    they are located.  This ensures that they are always present during collection.
4.  The CVO waiting logic on `clusteroperator` remains the same.

### Risks and Mitigations

1. Existing clusteroperators may treat presence and absence or condition==Unknown as special and fail to reconcile.
   This would be a bug in the operator implementation that needs to be fixed.

## Design Details

### Test Plan

1. When an install in CI fails at some point in the release, we should see must-gather information
2. During an installation, the `clusteroperator` resources should be visible via the API immediately.

### Graduation Criteria

GA. When it works, we ship it.

### Upgrade / Downgrade Strategy

No special handling is needed because the condition meaning remains the same.  The upgrade will simply have new 
`clusteroperators` created at the start of the upgrade.

### Version Skew Strategy

No special consideration.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

