---
title: eus-upgrades-mvp
authors:
  - "@sdodson"
reviewers:
  - @darkmuggle
  - @soltysh
  - @sttts
  - @deads2k
  - @ecordell
  - @dgoodwin
approvers:
  - "@eparis"
  - "@derekwaynecarr"
  - "@crawford"
  - "@pweil-"
creation-date: 2020-01-12
last-updated: 2020-01-12
status: provisional
see-also:
replaces:
superseded-by:
---

# EUS Upgrades MVP

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement outlines a set of cross platform improvements meant to ensure
the safety of multiple back-to-back minor version upgrades associated with EUS
to EUS upgrades. Additionally, we outline improvements applicable to all
upgrades meant to reduce the duration of upgrades and workload disruption.

All of the work detailed herein is prerequisite to enabling upgrades which skip
reboots. However where non-overlapping resources exist we can pursue reboot
removal in parallel such as the Node team validating upstream's stated Kubelet
version skew policies.

## Motivation

The introduction of EUS creates a subset of clusters which we expect will run
4.6 for a year or more then upgrade rapidly, though serially, from 4.6 to 4.10.
This rapid upgrade introduces the risk that those clusters may upgrade faster
than is safe due to constraints imposed by OpenShift, the upstream components of
OpenShift, or deployed workloads.

This also creates a scenario where admins wish to reduce both the duration and
the disruption to workload associated with the upgrade.

### Goals

- We will inform admins of incompatibilities between their current usage and the
final upgrade target
- We will provide disconnected admins tools that help them plan their entire
upgrade path from 4.6 to 4.10 so that they may mirror all content necessary
- We will provide guidance on expected upgrade duration scaled by node count exclusive
of the variability associated with workload rescheduling (ie: everything except
Worker MachineConfigPool rollout)
- We will inhibit upgrades to the next minor version whenever necessary to preserve
cluster stability and supportability, ie:
  - Respecting version skew policies between OS managed components (kubelet, crio, RHCOS)
  and operator managed components (Kubernetes API, OpenShift API, etc)
  - Respecting version skew policies of OLM managed Operators
  - Ensuring APIs removed by components in the release payload are no longer in
  use
- We will ensure that CVO managed operators upgrade as quickly as is safe to do so
- We will reduce workload disruption associated with pod rescheduling during rolling
reboots


### Non-Goals

- This enhancement does not attempt to remove *any* steps along the serial
  upgrade path from 4.6 to 4.7 to 4.8 to 4.9 to 4.10 *including* reboots. A
  follow-up enhancement will address those concerns.

## Proposal

Before we dive into user stories it's useful that you're familiar with CVO and
Console features which are relevant to how upgrades and channel options are
presented to the admin. The CVO and Console as of 4.6 are now aware of other
channels containing the cluster's current version. This means that if 4.6.50 is
a non EUS version and 4.6.51 is an EUS version both the CVO and Console can
display a different set of channel options. This means that 4.6.50 would see
candidate-4.6, fast-4.6, stable-4.6, eus-4.6, candidate-4.7, fast-4.7, and
stable-4.7 channels.

Where as 4.6.51 would see only eus-4.6 and eus-4.10 assuming we have enabled EUS
4.6 to 4.10 upgrades. After switching to eus-4.10 the cluster would offer
upgrades to 4.7 versions present in the eus-4.10 channel, when that completes
the cluster would offer 4.8 versions present in the eus-4.10 channel, repeat
until the cluster has completed its upgrade to 4.10.

### User Stories

#### APIServer - Notify cluster admins of API use which will be removed between 4.7 and 4.10

We have pre-existing mechanisms to alert admins to their use of APIs which are
slated for removal in a future release. Currently those mechanisms do not look
forward far enough to span 4.6 to 4.10, we will need to add the ability to
backport that API metadata to 4.6 once we have a firm understanding of all
removals between 4.6 and 4.10.

In addition to Info level alerts core API providers will also need to set
Upgradeable=False whenever the next minor version removes an API which is in
use.

This allows us to *inform* the admin for removals that are more than one minor
version away and *block* upgrades for removals which are imminent.

### MCO - Enforce OpenShift's defined host component version skew policies

The MCO, will set Upgradeable=False whenever any MachineConfigPool has one more
more nodes present which fall outside of a defined list of constraints. For
instance, if OpenShift has a defined Kubelet Version Skew of N-1, the node
constraints enforced by the MCO defined in OCP 4.7 (Kube 1.20) would be as follows:

```yaml
node.status.nodeInfo.kubeletVersion:
- v1.20
```

If the policy were to change allowing for a version skew of N-2, v1.19 would be
added to the list of acceptable matches. As a result a cluster which had been
upgraded from 4.6 to 4.7 would allow a subsequent upgrade to 4.8 as long as all
kubelets were either v1.19 or v1.20. The 4.8 MCO would then evaluate the Upgradeable
condition based on its constraints, if v1.19 weren't allowed it would then
inhibit upgrades to 4.9. This means the MCO must set Upgradeable=False until it
has confirmed constraints have been met.

```yaml
node.status.nodeInfo.kubeletVersion:
- v1.20
- v1.19
```

The MCO is not responsible for defining these constraints and constraints are
only widened whenever we have CI testing proves them to be safe.

These changes will need to be backported to 4.7 prior to 4.7 EOL.

#### OLM - Allow Operators to define inclusive compatible version ranges

We should allow maintainers to define an inclusive version range for their Operator.

#### OLM - Enforce defined OLM managed Operator compatibility

If a managed Operator defines a maxKubeVersion or maxOCPVersion which would be
violated by upgrading to the next minor OLM must set Upgradeable=False and
enumerate the constraint in the condition message.

Note, this is up for debate as to what the behavior should be in absence of a
defined maximum version.

#### OTA - Inhibit minor version upgrades when an upgrade is in progress

We should inhibit minor version upgrades via Upgradeable=False whenever an existing
upgrade is in progress. This prevents retargetting of upgrades before we've reached
a safe point.

Imagine:

1. Be running 4.6.z.
1. Request an update to 4.7.z'.
1. CVO begins updating to 4.7.z'.
1. CVO requests recommended updates from 4.7.z', and hears about 4.8.z".
1. User accepts recommended update to 4.8.z" before the 4.7.z' OLM operator had come out to check its children's max versions against 4.8 and set Upgradeable=False.
1. Cluster core hits 4.8.z" and some OLM operators fail on compat violations.

This should not inhibit further z-stream upgrades, but we should be sure that
we catch the case of 4.6.z to 4.7.z to 4.7.z+n to 4.8.z whenever 4.7.z was not
marked as Complete.

#### OTA - Optimize & Model Minor Version Upgrade Duration

With support of the PerfScale team the Over The Air team will measure and
analyze upgrade timings using simulated workloads at a defined capacity and
load. This analysis will be used to identify optimizations and set expectations
on a per minor version basis as to the duration of the upgrade.

Due to the variability in workload deployment these measurements will be
*exclusive of Worker MCP rollout*. The simulated workload is intended only to
ensure that our measurements more closely mimic real world usage, not upgrades
of clusters with zero workload.

##### SDN - Canary and Optimize SDN DaemonSet rollout

In 4.6 and 4.7 we identified a number of DaemonSets including dns, mcd, node-tuned,
node-exporter which were not critical to workload availability which could rollout
in a more parallel manner. Those operators were changed to utilize maxUnavailable
of 10% rather than absolute value of 1, on a cluster of 250 nodes that generally
reduced rollout time from 80 minutes to 10 minutes. The multus, sdn, ovs DaemonSets
have not been updated but likely could be now that workload critical processes
have been moved to the host. Before doing this given the risk we should devise
a method to canary updates to these DaemonSets. Any other options to speed up
updates to these DaemonSets which are deployed to all Linux nodes should also be
considered.

Once these new deployment patterns have been vetted in 4.8 or 4.9 these changes
should be backported through to 4.6.

#### Workloads - Optimize scheduling for rolling upgrades

Currently the MCO cycles nodes in a deterministic but unintentional manner. The
workload on that host which can be relocated scatters to other hosts according
to scheduling rules. This has been measured and shown that approximately 70% of
all reschedulable pods are rescheduled more than necessary if they were
rescheduled in an optimal manner. Optimal rescheduling should be closer to (1 /
node_count_per_failure_domain) percentage of pods being rescheduled more than
once during a rolling restart of a given failure domain.

We should study this more deeply to understand what improvements could be made
to reduce pod rescheduling during the rolling reboot of a cluster associated with
upgrades. This may require coordination between MCO and the scheduler such that
the scheduler can bias slightly towards hosts which were more recently updated.

We should consider whether or not we're only concerned with rolling reboots due
to machineConfig updates or if we're attempting to optimize all roling reboots.

We should consider enabling the descheduler as well to ensure that when the
upgrade completes we rebalance so that the last node ends up with proportionate
workload.

#### ??? - Provide a hosted graph viewer which will assist disconnected admins in mirroring all content between 4.6 and 4.10

Assume that 4.6.eus-z -> 4.7.z -> 4.8.z -> 4.9.z -> 4.10.z are all in the eus-4.10
channel providing a chain of upgrades that eventually lead to 4.10. We should build
a graph viewer / solver somewhere which allows an admin of a 4.6 cluster hoping
to upgrade to 4.10 to discover all content which must be mirrored between their
current version and their eventual 4.10 target.

In general the graph viewer should provide a complete path from the cluster's
current version to the latest .z in the selected channel assuming such a path exists.
If someone were to provide cluster version 4.4.3 but select the stable-4.6 channel
no such path exists and it should indicate that, if they were to select stable-4.5
it would provide the upgrade path of 4.4.3 to 4.4.29 to 4.5.24 which is the latest 4.5.z.

### Implementation Details/Notes/Constraints [optional]

The main caveat of what's outlined here is that it does not reduce reboots
which, for many reasons, is highly desired by some admins. The rationale for not
addressing that in this enhancement is that this enhancement targets
improvements which benefit all upgrades and are requisite in both scenarios.

The epic / user story to backport API removal notifications to 4.6 assumes that
we will require anyone upgrading along an EUS specific upgrade path to update to
a 4.6 version which is current as of the time that 4.9 goes GA. This date is chosen
because it's the point in time where we must know with certainty every API which
is removed in 4.10. EUS customers may assume that EUS means "I only need to upgrade
every 18 months" which will be at odds with our expectation that they upgrade
into a late 4.6.z before starting their upgrade to 4.10.

### Risks and Mitigations

The improvements here do not seem to introduce risk unto themselves. However
failure to implement some of these such as the version skew constraints would
impose significant risk to supportability in clusters which may upgrade more
rapidly than is safe to do so.

Additionally, the timeframe for final delivery extends three releases into the
future. This allows for design iteration during the 4.8 release cycle with
implementation focused on the 4.9 and 4.10 release cycles.

Upstream generally pays little attention to concerns which are present in EUS
clusters where they stall for a year or more and then expect to rapidly upgrade
across 3 or more minor versions to the next EUS version. Therefore decisions are
made upstream which may hinder our ability to deliver this pattern to EUS
customers.

## Design Details

### Open Questions [optional]

- As this is a bit of a meta-enhancement calling out epic level work it should be
no surprise that there will be implementation details unaccounted for today. We
will need to decide on an epic by epic level which require enhancements of their
own.

- While I believe all of this work to be prerequisite to taking on work to skip
upgrade steps that may not be widely agreed upon.

- Do we need to consider CCO for similar treatment that we're providing for API
compatibility? This seems like a nice to have that we could consider for followup.
If we remova an API workloads break, if credential requests are not capable of
being met upgrades to a subsequent release will not be possible but the workload
should remain unaffected.

- Should the graph solver attempt to assist with mirroring of catalog content or
only core platform content?

- Should we externalize the API removal bits? Perhaps via a marketplace operator
that's broadly scoped as "EUS 4.6 to EUS 4.10 Validator"?

### Test Plan

- CI tests are necessary which attempt to upgrade while violating kubelet to API
compatibility, ie: 4.6 to 4.7 upgrade with MachineConfigPools paused, then check
for Upgradeable=False condition to be set by MCO assuming that our rules only allow
for N-1 skew.
- CI tests are necessary which install an OLM Operator which expresses a maxKubeVersion
or maxOCPVersion equal to the current cluster version and checks for Upgradeable=False
on OLM
- CI tests are necessary which check for Info level alerts when to be removed
APIs are in use.
- Once we've determined the target upgrade duration we should tighten the existing
`Upgrades should be fast` test to those values so that we catch regressions on
our upgrade duration targets. We should also add infrequent test jobs which
perfom the same upgrade duration tests at cluster sizes and workloads that more
closely represent our deployed fleet.

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
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Despite the topic area, this work does not actually change Upgrade or Downgrade
strategy.

### Version Skew Strategy

N/A

## Implementation History

See the noted CVO and Console features related to channel presentation outlined
above. Otherwise nothing specifically tied to this effort.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Rather than having MCO enforce version skew policies between OS managed
components and operator managed components it could simply set Upgradeable=False
whenever a rollout is in progress. This would preclude minor version upgrades in
situations where a z-stream rollout stalls for some reason or another. We also
know of some customers who have maintenance windows such that they may be in a
perpetual state of MCP rollout.

We could also consider simply waiting for all MachineConfigPool rollouts to have
completed before the MCO considers itself upgraded. This has the downside of
shifting highly variable workload dependent operation into the scope of the
upgrade which is something we track very closely both in terms of success and
duration.


## Infrastructure Needed [optional]

N/A
