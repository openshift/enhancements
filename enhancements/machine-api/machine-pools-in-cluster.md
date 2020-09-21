---
title: az-balanced-machinepools-in-cluster
authors:
  - "@dgoodwin"
reviewers:
  - "@abhinavdahiya"
  - "@crawford"
  - "@JoelSpeed"
  - "@enxebre"
  - "@sdodson"
  - "@staebler"
approvers:
  - "@abhinavdahiya"
  - "@crawford"
  - "@JoelSpeed"
  - "@enxebre"
  - "@sdodson"
creation-date: 2020-09-21
last-updated: 2020-09-21
status: provisional
---

# AZ Balanced MachinePools In-Cluster

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today we expose a useful construct in the Installer's InstallConfig known as MachinePool. When used the installer will query the available AZs for the account, and generate the relevant MachineSets to spread the requested number of machine replicas out over the AZs.

This enhancement proposes bringing this functionality into the cluster itself for use by customers on an on-going basis.


## Motivation

The Installer MachinePool is a useful construct that is only available at install time. Post-install customers must manage this spread themselves.

[OpenShift Hive](https://github.com/openshift/hive) also supports day 2 MachinePools using the same code from the Installer. However because Hive must install and maintain MachineSets for many versions of OCP, this code re-use between the Installer has generated a number of difficult and messy bugs when things change in the Installer. Bringing this feature in-cluster would also give Hive a stable API to interact with instead of fighting with one version of vendored Go code to be used across many OCP versions and a significant number of logic forks based on OCP version.

### Goals

List the specific goals of the proposal. How will we know that this has succeeded?

  * Provide customers with an API to automatically spread machines out over the AZs in their account.
  * Provide OpenShift Hive with a stable API to manage MachineSets spread over AZs day 2.

### Non-Goals

  * No linkup with the MachineDeployment concept for rolling updates of Machines.

## Proposal

### User Stories [optional]

#### Story 1

As an OCP user, I would like to be able to specify min/max machine replicas and
have those machines automatically spread out amongst all Availability Zones
that are available in my account.

#### Story 2

As an OpenShift Hive developer, I would like a stable API for spreading
machines across Availability Zones. This would allow me to sync an API
in-cluster rather than use one version of vendored code from the installer to
generate each MachineSet to be reconciled.

### Implementation Details/Notes/Constraints [optional]

Implementation would require a new CRD and controller aligned hopefully with a pre-existing operator in OCP.

The installer MachinePool definition can be seen [here](https://github.com/openshift/installer/blob/master/pkg/types/machinepools.go).

The code for spreading MachineSets across AZs [already exists today](https://github.com/openshift/installer/blob/master/pkg/asset/machines/aws/machinesets.go) and could be removed from the installer, which would then just pass through an instance of the new API.

The [Hive MachinePool API](https://github.com/openshift/hive/blob/master/pkg/apis/hive/v1/machinepool_types.go) also includes support for enabling auto-scaling on the resulting MachineSets, and this functionality should be carried over to the new API even if not initially supported by the Installer.

### Risks and Mitigations

  * Clusters being upgraded would not be making use of this new API. They would continue to have any "orphaned" MachineSets the installer generated for them day 0 but this should work fine.

## Design Details

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. Upstream cluster-api project has begun using the MachinePool naming for an API that is backed by native cloud provider auto-scaling groups. (ASGs, MIGs) We however are already using this term in both the Installer and Hive. Is it acceptable to push this naming in cluster even if it may one day overlap with a cluster-api type in a different apigroup?
 > 1. How does this feature related to MachineDeployments?
 > 1. Do we have an existing operator which would be a good home for this controller?
 > 1. Would this feature play as nicely with auto-scaling as day 0 Installer MachinePools?

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

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Any exception to this should be
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
