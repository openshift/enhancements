---
title: pre-assigning-nodes-to-machine-config-pools
authors:
  - "@beekhof"
reviewers:
  - "@slintes" 
  - "@markmc"
  - "@JoelSpeed"
  - "@alexcrawford"
  - "@kikisdeliveryservice"
approvers:
  - "@markmc"
  - "@JoelSpeed"
  - "@alexcrawford"
  - "@kikisdeliveryservice"
creation-date: 2021-03-31
last-updated: 2021-03-31
status: provisional
see-also:
  - "/openshift/enhancements/pull/716"
replaces:
  - 
superseded-by:
  - 
---

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In telco bare-metal environments, there is a need to have Nodes be provisioned
with a known Machine Config Pool.

The traditional flow is for Nodes to come up as workers and be moved to the
desired pool after provisioning, however this consumes a significant portion of
the maintenance window in bare-metal environments.

## Motivation

In telco environments, there is a fixed window (typically 4 hours, half of which
is reserved for rollback) during which new hardware needs to be installed and
provisioned.

In the case of remote worker nodes, they will typically need to be a part of a
specific (non-default) MachineConfigPool, and are provisioned with the correct
pool based on the Ingnition file contents.

However, the MCO/MCD normally manages MCP assignment and does so based on Node
Labels.  This creates a race to add the necessary labels before the MCO/MCD
moves the Node back to the default config pool.

On bare metal, the cost of loosing this race is significant, and as a result we
could spend half of the 2 hour window just rebooting.

### Goals

- Ensure that the MCO's understanding of the "correct" config pool for a node is
  consistent with the config pool it was provisioned with via Ignition

- Ensure that nodes can still move between pools if they are relabeled

- Ensure that nodes that have moved pools do not revert to their original pool
  if the Node object is deleted (eg. due to Machine Health Check)

### Non-Goals

## Proposal

_(Caveat, I'm cherry-picking a thread with Clayton to kickstart discussion)_

The pool itself should be able to define labels and have them be part of the
Node CR during initial registration.

The pools implicit and explicit labels should always flow through to the booted
machine.

It should be impossible for a user to configure a kubelet config such that the
mco identifying labels are lost.

It should be possible to add labels directly to the pool such that users
can get them added (add only because we don’t want to deal with conflict
resolution if a user changes them)

### User Stories

1. As an admin, I want Nodes to have a specific set of labels based on their
   https://www.clli.com driven name, so that I can target workloads to machines
   based on hardware profile, location, etc.

1. As an admin, I want Nodes to be created with a set of labels consistent with
   the selection criteria of the MCP that it was provisioned with, so that it does
   not flap between pools, consuming valuable time rebooting.

1. As an admin, I want Nodes that have changed pools to be created with a set of
   labels associated with the new pool, so that they do not revert to their
   original pool after health based remediation.

### Implementation Details/Notes/Constraints [optional]

TBA

### Risks and Mitigations

The MCO is a core component, while the risk of introducing bugs may be no
greater than for any other component, the consequences are more pronounced.

## Design Details

TBA

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

- Initial version: 2021-03-31

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

1. Applying labels from Machines and/or MachineSets happens too late in the
   process and does not close the possibility for race conditions, nor does it
   apply to adding UPI workers.

2. Node Feature Discovery can (as a result of our work) apply labels based on
   Node names, but it also happens too late in the process and does not close
   the possibility for race conditions

3. Modifying kubelet.service as part of the initial Ignition configuration
   works, but is risky if that file ever needs to change, and considered a hack.

4. [enhancement #716](https://github.com/openshift/enhancements/pull/716).

## Infrastructure Needed [optional]

- Bugzilla Component
