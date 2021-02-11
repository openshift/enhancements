---
title: reduced-reboot-upgrades
authors:
  - "@sdodson"
reviewers:
  - @darkmuggle
  - @rphillips
  - @derekwaynecarr
  - @crawford
  - @dcbw
  - @miabbott
  - @mrunalp
  - @zvonkok
  - @pweil-
  - @wking
  - @vrutkovs
approvers:
  - @derekwaynecarr
  - @crawford
creation-date: 2020-01-21
last-updated: 2020-01-21
status: provisional
see-also:
  - https://github.com/openshift/enhancements/pull/585
  - "/enhancements/eus-mvp.md"

---

# Reduced Reboot Upgrades

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement is intended to reduce host reboots when upgrading across two or
more OpenShift minor versions by enabling an N-2 version skew policy between all
host components and cluster scoped resources.

## Motivation

While OpenShift is designed to minimize workload disruption and risk associated
with rolling reboots there exist a class of customers and workloads where
reboots remain a disruptive and time consuming activity. Additionally, with the
introduction of Extended Update Support (EUS) a new upgrade pattern will emerge
where clusters run 4.6 for a year or more then rapidly upgrade across multiple
minor versions in a short period of time. Those customers wish to complete their
upgrades in a condensed time frame and with as few reboots as possible, they do
not intend to run each minor version for an extended period of time.

### Goals

- Define testing requirements for N-2 host to cluster resource version skew
- Define version skew policies for host and cluster scoped resources
- Reduce reboots in accordance with our new tested policies

### Non-Goals

- Exceeding upstream's documented Kubelet version skew policies

## Proposal

### User Stories

#### Node - Improve Upstream Kubelet Version Skew Testing

Kubernetes defines a version skew policy(https://kubernetes.io/docs/setup/release/version-skew-policy/#kubelet)
which allows for kubelet N-2 to be compatible with kube-apiserver version N. At
this point in time OpenShift is not comfortable with the level of testing upstream
and the intersection of the specific features of OpenShift. We should work to
define and implement upstream testing changes which give us an appropriate level
of confidence that N-2 version skew issues would be identified in the community
whenever possible.

#### Node - Implement Downstream Kubelet Version Skew Testing

In addition to upstream version skew testing is in place we must also implement
downstream version skew testing which includes any additional tests required for
OpenShift specific implementation details.

#### Teams with Host Components - Allow N-2 Host Component Version Skew

All teams which own components that directly interface with or ship host based
components will need to ensure that they're broadening their compatibility to
allow for N-2 version skew between host and cluster scoped resources.

This would include for example the SDN DaemonSets in 4.10 remaining compatible
with OVS and any other host components in 4.10, 4.9, and 4.8. On a case by case
basis teams should decide whether it makes more sense to maintain a broader
compatibility matrix or that N-1 bits and MachineConfig are backported to N-2 and
upgrade graph is amended with these new minimum version requirements.

For instance, if 4.9.12 is the minimum version for 4.9 to 4.10 upgrades we'd
ensure that the next 4.8.z shipping after 4.9.12 has RHCOS bits and MachineConfig
which offer parity with 4.9.12 so that it's not required that we reboot into
4.9.12. If teams choose to pursue this option they will need to continue to ensure
that 4.7 to 4.8.z and 4.8.z-n to 4.8.z upgrades continue to work as well.

Teams which believe this is not achievable or the level of effort is extremely
high should document those findings.

Thus far RHCOS, Node, MCO, SDN, Containers, and PSAP teams are known to fall into
this group of teams which have components coupled to the host.

#### MCO - Widen Node Constraints to allow for N-2

Building upon the EUS-to-EUS upgrade MVP work(https://github.com/openshift/enhancements/blob/master/enhancements/update/eus-upgrades-mvp.md#mco---enforce-openshifts-defined-host-component-version-skew-policies)
to allow MCO to enforce host constraints we will broaden those constraints to
enable host component version skew.

Admins who choose to would then be able to skip a host reboot by following this
pattern:

1. Starting with a 4.8 cluster, pause Worker MachineConfigPool
1. Upgrade to 4.9
1. Upgrade to 4.10
1. Unpause Worker MachineConfigPool

Note that this can be decoupled in a way that when we ship 4.9 the initial MCO
could assert constraints which require 4.9 host components before upgrading to
4.10. Then after we ship 4.10 and have sufficiently tested a 4.8 to 4.10 host
component version skew a later 4.9.z MCO could have its constraints broadened.
This allows us additional time to test broader version skews if we so choose.


### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

This imposes significant risk due to a number of factors:
- We're currently not confident in upstream's testing matrix and our specific
feature sets
- We're never before expected teams to offer broader than N-1 compatibility.
Teams have always assumed at most N-1 and even then, especially early after a
GA release, it's not uncommon to find problems in N-1 compatibility.
- While N-1 is tested in the context of upgrades it's not been tested in long
term use.

We may mitigate some of this by further delaying EUS-to-EUS upgades after normal
minor version upgrades have been promoted to stable and allocating significantly
more time and effort to testing efforts. Ultimately this introduces another
dimension to an already complex testing matrix.

## Design Details

### Open Questions [optional]

1. Should we make these between specific named versions. ie: 4.6-4.8, and 4.8-4.10
or should this be a standard N-2 rule, ie: 4.6-4.8, 4.7-4.9, 4.8-4.10?

### Test Plan

This is actually a major focus of the entire effort, so we'll fill this out
now but expect to bring more clarity in the future once we have a better test
plan.

- We must have upstream N-2 version skew testing, which test suites should
be run at completion? e2e?
- We must have downstream N-2 version skew testing which meets or exceeds our
existing upgrade testing. We need to decide if this is install OCP 4.N and
RHCOS 4.N-2 or if this is install OCP 4.N-2 pause Worker MCP, upgrade twice, test.
The former will be quicker but the latter will be more representative of the
customer use case.
- We must decide how many platforms must be covered, all of them? tier 1?

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

Upgrade expectations:
- **NEW** This is new, the rest is the standard boiler plate which still applies --
  Admins may pause Worker MachineConfig pools at specifically defined product versions
  then apply multipe minor version upgrades before having to un-pause the MCP in
  order to upgrade to the next minor.
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
- When downgrading in conjunction with reboot avoidance as described in this
  enhancement it is assumed that you will rollback at most one minor version, if
  you had upgraded 4.8 to 4.9 to 4.10 then you would only be able to downgrade
  back to 4.9.
- If you had paused MachineConfigPools they should remain paused. If you had
  unpaused MachineConfigPools then those should remain unpaused when rolling back
  so that host bound components similarly downgrade.
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

We will need to extensively test this new host to cluster scoped version skew.
For the time being we will only allow version skew across specific versions,
4.8 to 4.10. This should be enforced via MCO mechanisms defined in a previous
enhancement(https://github.com/openshift/enhancements/blob/master/enhancements/update/eus-upgrades-mvp.md#mco---enforce-openshifts-defined-host-component-version-skew-policies).

Components which ship or interface directly with host bound components must ensure
that they've tested across our defined version skews.

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

This introduces significant additional compatibility testing dimensions to much
of the product. We should strongly consider whether reducing reboots by 50% is
worth it.

## Alternatives

- We make no changes to our host component version skew policies.
- We find other ways to reduce workload disruption without expanding our compatibility testing matrices.

## Infrastructure Needed [optional]

This effort will expand our CI requirements with additional test jobs which must
run at least once a week if not daily. Otherwise there's no net new projects or
repos expected.
