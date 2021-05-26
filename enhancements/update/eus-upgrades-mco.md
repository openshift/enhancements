---
title: eus-upgrades-mco
authors:
  - "@rphillips"
reviewers:
  - @kikisdeliveryservice
  - @yuqi-zhang
approvers:
  - "@derekwaynecarr"
  - "@crawford"
creation-date: 2021-05-05
last-updated: 2021-05-05
status: provisional
see-also:
replaces:
superseded-by:
---

# EUS Upgrades MCO

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- [x] [Enhancement 762](https://github.com/openshift/enhancements/pull/762)


## Summary

This enhancement outlines the Machine Config Operator (MCO) behavior when
propogating Kubernetes Events in EUS upgrade scenarios.  The MCO will emit
events that are user readable and be clear enough that a user can take an
action.

Related to [Enhancement 762](https://github.com/openshift/enhancements/pull/762).

## Motivation

The introduction of EUS creates a subset of clusters which we expect will run
4.6 for a year or more then upgrade rapidly, though serially, from 4.6 to 4.10.
This rapid upgrade introduces the risk that those clusters may upgrade faster
than is safe due to constraints imposed by OpenShift, the upstream components of
OpenShift, or deployed workloads.

This also creates a scenario where admins wish to reduce both the duration and
the disruption to workload associated with the upgrade.

### Goals

- The MCO will look at all the pools within the cluster, if there are paused
pools the MCO will make sure the nodes have a n-2 skew.
- If there is a paused pool with a node skew of greater than n-2, then an
event will be emitted.

### Non-Goals

### User Stories

#### Paused Pools

Modify syncUpgradeableStatus within the MCO to loop through all the pools and
nodes looking for paused pools that are greater than the n-2 skew.

If a node is found, then emit an event with the message:

> The $poolName Machine Config Pool does not conform to the n-2 skew.  Please
> unpause $poolname to continue upgrades.  Unpausing the pool will cause
> disruptions to the workloads.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

N/A

## Design Details

### Open Questions [optional]

### Test Plan

- CI tests are necessary which attempt to upgrade while violating kubelet to API
compatibility, ie: 4.6 to 4.7 upgrade with MachineConfigPools paused, then check
for Upgradeable=False condition to be set by the API Server Operator, assuming
that our rules only allow for N-1 skew.
- CI tests will be necessary to verify the events emitted from the MCO

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

#### Examples

These are generalized examples to consider, in addition to the aforementioned [maturity levels][maturity-levels].

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

Despite the topic area, this work does not actually change Upgrade or Downgrade
strategy.

### Version Skew Strategy

N/A

## Implementation History

[PR](https://github.com/openshift/machine-config-operator/pull/2552) -
@deads2k wrote this as a starting point.  The PR can be used as a starting
point, but we need to rewrite it to conform to this document.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

## Infrastructure Needed [optional]

N/A
