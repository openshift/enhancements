---
title: channel-rename-generally-available
authors:
  - "@wking"
reviewers:
  - "@2uasimojo, Hive"
  - "@cblecker, service delivery"
  - "@jewzaam, service delivery"
  - "@jhadvig, admin web-console, whether we want console docs links"
  - "@jharrington22, service delivery, ARO"
  - "@jiajliu, updates QE"
  - "@jmguzik, test-platform, ci-operator integration and ci-docs"
  - "@joeg-pro, Advanced Cluster Management"
  - "@joepvd, Automated Release Tooling, mirror handling"
  - "@LalatenduMohanty, updates dev, update service configuration"
  - "@patrickdillon, installer, default channel"
  - "@skopacz1, updates docs"
  - "@vkareh, Openshift Cluster Manager"
approvers:
  - "@LalatenduMohanty"
api-approvers:
  - None
creation-date: 2024-02-06
last-updated: 2024-02-22
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1153
---

# Channel Rename Generally Available

## Summary

Changing from `fast-4.y` and `stable-4.y` to `ga-4.y` and `fleet-approved-4.y`, starting with the 4.16 channels.
The `candidate-4.y` and `eus-4.y` patterns will continue unchanged.

## Motivation

Some users find the `fast-4.y` vs. `stable-4.y` distinction confusing, even with the context of [the doc section comparing the two channels][fast-stable-channel-strategies], especially that:

> The promotion delay before promoting a release to the stable channel represents the only difference between the two channels.

This enhancement renames those channels to make the distinction more clear, for situations where the user does not have pointers to the additional context of that documentation.

### User Stories

* As an OpenShift cluster administrator, I want the name of the channel recommending GA updates to remind me that those updates are generally available, because `fast-4.y` does not, and sometimes that makes me nervous.
* As an OpenShift cluster administrator, I want the name of the channel recommending soaked updates to remind me that those updates have been soaked, because `stable-4.y` does not, and sometimes I assume that "stable" means "bug free" or something.
* As an OpenShift representative, I want channel names and semantics to be as clear as possible, to decrease the chances that cluster administrators ask me to explain distinctions, and to make those explainations as simple as possible when I am asked.

### Goals

* Increase customer clarity around channel semantics after the pivot.
* Mitigate the customer-side and Red-Hat-side impacts of the transitional period.

### Non-Goals

* Achieve a completely painless transitional period.
  While pain-reduction is good, an implementation where limited pain is expected is still acceptable.
* Adjust `candidate-4.y` and `eus-4.y` names or semantics.
  These channels are not relevant to this enhancement.

## Proposal

Beginning with 4.15, the `ga-4.y` and `fleet-approved-4.y` patterns will be initiated, making `ga-4.15` and `fleet-approved-4.15` the first in that line.
`ga-4.y` will have the same semantics as `fast-4.y`, and `fleet-approved-4.y` will have the same semantics as `stable-4.y`.
The `fast-4.y` and `stable-4.y` pattern will remain as public aliases, at least during 4.15, with [4.16 and later handling still under discussion][OTA-1221].

### Workflow Description

#### Updates into 4.15

1. The cluster is running a 4.14.z release in a 4.14-capped channel like [`stable-4.14`][stable-4.14].
1. The update service and cluster version operator [populate `status.desired.clusters`](#historical-context) with the channels compatible with the current 4.15.z, including the `*-4.14`, `candidate-4.15`, and other `*-4.15`.
   New with this enhancement, those will include `ga-4.15` and `fleet-approved-4.15`.
1. The cluster admin decides they want to update to 4.15.
1. The cluster admin uses [4.15 `oc adm upgrade channel ...`][set-channel-4.15-oc] or [the 4.14 web-console][set-channel-4.14-web-console] to view the list of available channels.
1. The cluster admin is surprised by the new `ga-4.15`, etc. and they click through to docs:
    * [Web-consoles need an update][web-console-channel-doc-link].
    * `oc adm upgrade [channel]` does not include a doc link at the moment.  [Should it?](#open-questions)  If so, what kind of version skew did we want this to work for?
1. The cluster admin selects a 4.15-capped channel like `ga-4.15`.
1. The cluster admin initiates an update to a 4.15.z release recommended by their new channel.

#### 4.15 installs

1. The cluster admin decides they want to install 4.15.
1. The cluster admin acquires a 4.15.z installer binary.
    * Signed installer binaries are available on the mirrors, e.g. [amd64-fast-4.14][mirror-fast-4.14].
    * [Docs][install-docs] point at [console.redhat.com][console-create] which in turn [points][console-install-user-provisioned] into the mirrors with `stable` links like [this][mirror-stable-installer].
      This path does not currently seem to point out the existence o [the sibling GPG signature][mirror-stable-signature].
    * Installer binaries (with hashes) are also linked from [the OCP portal page][portal-ocp], with URIs that include the version and hashes, but which do not include any channel names.
1. The cluster admin runs the installer, which generates a ClusterVersion manifest declaring [a default channel that is manually maintained][installer-default-channel].
   For 4.15, the default will remain `stable-4.15`.
   For 4.16, the default will [pivot to `ga-4.16`](#installer).

#### Tools making update service requests

The aliases allow tools to continue using the old naming patterns without any impact.

### API Extensions

No APIs will change as a result of this work.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift takes [`spec.channel`][HostedCluster-spec-channel] and the [`desired` Release][HostedCluster-status-version-desired] all the way up to the HostedCluster level, and [they still don't set a default channel][hypershift-default-channel], so the channel renames should be transparent there.

#### Standalone Clusters

Very relevant to standalone clusters too, since that's the original channeled-update topology.

#### Hive

Hive [reads ClusterVersion's `status.desired.version` to populate labels][hive-reconcile-cluster-version], but does not currently interact with `spec.channel` or `status.desired.channels`, so channel renames should be transparent there.

#### Single-node Deployments or MicroShift

Microshift manages installs via RPMs, without using an update service, so channels are not relevant there.

Channel renames will have no resource-consumption impacts on single-node clusters, which do use channeled updates.

### Implementation Details/Notes/Constraints

#### Historical context

History of OpenShift channel handling:

* OpenShift 4.5 and earlier, the in-cluster web console [manually maintained a list of expected channels][web-console-4.15-channels].
* OpenShift 4.6:
  * The cluster-version operator [started setting `status.desired.channels`][cluster-version-status-channels] based on `io.openshift.upgrades.graph.release.channels` update-service metadata ([enhancement](available-update-metadata.md)).
  * The in-cluster web console [started passing `status.desired.channels` on][web-console-consumes-channels] to cluster admins selecting their cluster's channel.
* OpenShift 4.8, the in-cluster web console [dropped the hard-coded fallback channels][web-console-drops-fallback-channels].
* OpenShift 4.9, `oc adm upgrade` [started passing `status.desired.channels` on][oc-adm-upgrade-channel] to cluster admins, and added a `channel` subcommand to edit the channel.

Since then, channels are largely a negotiation between cluster admins selecting a channel and the update service declaring a set of channel choices, with OCP-level components acting as intermediates passing around opaque channel names.

#### graph-data

The graph-data repository will require [updates to channel-management scripts][graph-data-release-script-update] before 4.15 GA:

* The new `ga-4.15` and `fleet-approved-4.15` channels will be created (`ga-4.y` with the same semantics as `fast-4.y` and `fleet-approved-4.y` with the same semantics as `stable-4.y`).

#### Documentation

The current docs are a bit murkey on updates between minor versions.
For example, 4.15 web-console docs talk about using the 4.15 web-console to update, and thus presumably cover both 4.15-to-4.15 and 4.15-to-4.16.
4.16 command-line docs talk about using the 4.16 `oc`, and thus presumably cover both 4.15-to-4.16 and 4.16-to-4.16.
EUS docs are very coy about which versions they're talking about, using placeholders to avoid having to commit.
Mixing this preexisting complication in with this enhancement's renaming seems like it could be tricky.

Luckily, the channel docs themselves only talk about their particular 4.y (e.g. [4.14's channel docs][channel-docs] only talk about `*-4.14`), so that portion should be straightforward.
[This ticket][OTA-1220] is exploring how OCP 4.15 docs should cover the new channels.

On top of OCP docs, there is also [ARO docs exposure](#aro).
And [some ROSA/OSD references to channel group names have leaked into docs](#managed), and may need updating if [Service Delivery decides to rename channel groups](#rename-managed-channel-groups).

There may also be some KCS that reference the existing channel naming paterns, and their maintainers may not be watching closely enough to notice this pivot and remember to update the KCSs (although this seems like a generic risk to all KCSs, and a reason to prefer managed docs over KCSs for any advice that needs long-term maintainence).

#### Installer

No installer changes are expected for 4.15.

[In 4.16, the installer is expected to bump][OTA-1219] their manually-maintained [default channel][installer-default-channel] to use the `ga-4.y` pattern (alternatively, they could [pivot to `fleet-approved-4.y`](#installer-default-channel)).

#### Web-console

As described [here](#historical-context), the web-console is mostly out of the business of hard-coding channel information.
However, they do have [placeholder and help text in the channel model][web-console-channel-modal] that would need updating.
Both are only presented when the ClusterVersion's `status.desired.channels` is empty, which limits exposure to clusters which have already picked an invalid channel or cleared their channel, or when the update service access is failing.
In the absence of this bump, they could mislead cluster administrators into setting old-pattern channel names, and the admins would have to follow the link to [channel docs][channel-docs] to discover appropriate values for their 4.y release.

#### Cluster-version operator

The cluster-version operator could be taught to alert on deprecated channel names.
Although currently it is not clear if the old naming patterns will be deprecated, or remain GA forever, more in [discontinuing the aliases](#discontinuing-the-aliases).

#### Continuous integration

In [the release repository][release]:

```console
release $ git grep -oh 'channel:.*' ci-operator/config | sort | uniq -c
    128 channel: candidate
    423 channel: fast
      1 channel: fast # candidate, fast, stable, eus
     32 channel: stable
```

References to `fast` and `stable` for 4.16 and later could be updated, although the alias mapping means they do not have to be updates.

Some CI docs could also be updated, including examples like [this][ci-docs-release-channel-example].

Brief searching did not turn up any exposure in [ci-tools][], but it may also have some references that could be updated.

There is also [code in metal's dev-scripts][dev-scripts-stable-clients] that is downstream of [ART mirrors](#art).

#### ART

ART may want to rename their channel-based mirror paths like [`fast-4.14`][mirror-fast-4.14] and [`stable`][mirror-stable].
But alias mapping means their existing tooling will continue to work without changes.

#### Assisted-installer

[Work to support custom releases][assisted-installer-custom-releases] is adding an https://api.openshift.com/api/upgrades_info/graph client with [hard-coded channel-name patterns][assisted-installer-custom-releases-channel-names].

Internal assisted-installer-ops scripting also currently hard-codes the `candidate`, `fast`, and `stable` channel fragments which are assembled into channel names and fed to a https://api.openshift.com/api/upgrades_info/v1/graph client.

#### Labs-graph

https://access.redhat.com/labs/ocpupgradegraph/ will need to update its hard-coded list of known channels.
Alternatively, the set of known channels could [become an update service API][channels-api].

### Advanced Cluster Management

[Red Hat Advanced Cluster Management for Kubernetes][RHACM] and [the multicluster engine for Kubernetes operator][MCE] have [a channel-selection interface][MCE-set-channel].
That interface [consumes ClusterVersion's `status.desired.channels`][MCE-console-availableChannels], and [does not have][MCE-console-channel-text] [the in-cluster web-console's placeholder or help text exposure](#web-console).
Application is via [ClusterCurator's `spec.upgrade.channel`][ClusterCurator-spec-upgrade-channel], and from there down into ClusterVersion's `spec.channel` [here, for the ClusterCurator-to-Hive controller][cluster-curator-controller-hive-set-channel].

[acm-hive-openshift-releases' ClusterImageSet naming][acm-hive-openshift-releases-clusterImageSets] would need updating to the new channel name patterns.
Also [some scripting][acm-hive-openshift-releases-tooling], and [`Makefile` targets][acm-hive-openshift-releases-Makefile] run by [the cron job][acm-hive-openshift-releases-cron].
There are some stale [docs][MCE-old-channel-name-references-1] [references][MCE-old-channel-name-references-2].


#### Managed clusters

The current pipeline populating ClusterImageSet `api.openshift.com/available-upgrades` annotation assumes the current channel-naming pattern will continue, and it could be updated to understand the pivot to the new names.
But the alias mapping will keep the existing tooling working, if Service Delivery wants to keep using the old patterns.

[The managed-upgrade operator's `inferUpgradeChannelFromChannelGroup`][managed-upgrade-operator-inferUpgradeChannelFromChannelGroup] could be updated to select the canonical channel for the target 4.y when converting the channel-group back to a channel name.
There is similar code in the OpenShift Cluster Manager for hosted/HyperShift, which does not use the MUO for channel-group-to-channel mapping.

[The `rosa` command-line currently leaks the channel-group name into docs][rosa-command-line-channel-group], and if the managed folks decide to [rename their channel groups](#rename-managed-channel-groups) as a result of this channel-name pivot, those references would need updating.

#### ARO exposure

[Current ARO docs][aro-docs-channel]:

> Azure Red Hat OpenShift 4 supports stable channels only. For example: stable-4.9.

That and surrounding text could be adjusted to use the new pattern.

### Risks and Mitigations

This enhancement does not add new channel semantics, it only adjusts the names used to access established semantics, which mitigates many risks.

#### Admin accidentally selects a channel whose semantics do not match their goals

The new names are both for supported channels, with limited differences between them.
So even if an admin selects `ga-4.16` while mistakenly thinking "I bet this is the 4.16 version of `stable-4.16`", they'd still only be recieving the recommended updates they want, just without the `stable-4.y` / `fleet-approved-4.y` [delay][fast-stable-channel-strategies].

#### Consumer who had been relying on old names needs to update

We know some channel consumers like [ART](#art), and can reach out to them about the pivot.
We know cluster users will hit this transition, and can prepare to onboard them with doc links from update interfaces.
But there may be other consumers and tooling that we are not aware of, and which ends up needing adjustment to handle this pivot.
This is [mitigated by the alias mapping](#tools-making-update-service-requests).

### Drawbacks

The new names will hopefully make the channel semantics more accessible to new users, and eventually even to users who are currently familiar with the old pattern.
But change is hard, and during the transition period, there will be more going on in this space to potentially confuse folks interacting with channels.
The business folks have weighted that estimated long-term clarity against the estimated transitional disruption, and expect the benefits to outweigh the costs.

## Open Questions

* Teach `oc adm upgrade [channel]` to link to channel-semantics docs?
* Update [labs channel list](#labs-graph) or declare channels via an update service API?
* When to remove the outgoing names from aliases?  4.17?  4.18?  Later?  If 4.17 or later, should we alert on the use of the deprecated channel names before [discontinuing the aliases](#discontinuing-the-aliases)?

## Test Plan

Unfortunately, testing updates via update service channel recommendations is extremely limited, and would need a not-yet-developed triggering mechnism.
Especially for channels where membership requires a shipped errata and generally-available support.
But having a test environment where OCP and ROSA and other products that depend on update services can test against mock update services would be useful.
Updates QE has some tests like this locally, but it may be worth expanding coverage as part of this effort.

This style of testing will be more difficult on HyperShift, which currently [requires the use of the default `upstream`][hypershift-clears-upstream] without [providing a knob for customization][hypershift-configurable-upstream].

## Graduation Criteria

The new channel names go straight to GA in 4.15 with docs.
The old channel names go straight from GA in 4.15 to... not clear what the 4.16 and later plan is yet (with an [open question](#open-questions) about possibly [discontinuing the aliases](#discontinuing-the-aliases).

### Dev Preview -> Tech Preview

Not applicable.

### Tech Preview -> GA

- [Testing](#test-plan).
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

- Announce deprecation and removal of the existing channel names (e.g. in the 4.15 and 4.16 release notes, explaining the 4.15 pivot and alias handling).

## Upgrade / Downgrade Strategy

[The alias mapping](#tools-making-update-service-requests) allows old-pattern clients to continue to use those patterns when interacting with new update service, and we expect to continue that alias support for several 4.y.

## Version Skew Strategy

If tooling expecting an old-pattern name attempts to use that pattern against a new-name update service (or some data downstream from that update service), [alias mapping](#tools-making-update-service-requests) will keep it working.
But even if there are issues, systems that set channels and attempt to retrieve update advice should already have provided a flow for this situation, like the cluster-version operator's [`RetrievedUpdates` condition][cluster-version-operator-RetrievedUpdates] and [`CannotRetrieveUpdates` alert][cluster-version-operator-CannotRetrieveUpdates].
The same recovery flows should cover tooling that expecta a new-pattern name attempting to use that pattern against an old-name update service.

## Operational Aspects of API Extensions

The [API extensions](#api-extensions) allow new channel name patterns when interacting with update services, [without requiring changes to existing clients](#tools-making-update-service-requests), so operational impact is expected to be minimal.
The exception is users running their own local update services, and that aspect is discussed [here](#cincinnati).

## Support Procedures

In clusters which have selected a channel that does not exist, or which does not contain their current cluster version, ClusterVersion's [`RetrievedUpdates` condition][cluster-version-operator-RetrievedUpdates] will be `False` and [the `CannotRetrieveUpdates` alert][cluster-version-operator-CannotRetrieveUpdates] will be firing.

This will prevent the cluster from recieving fresh update advice, possibly extending its exposure to any bugs fixed in later releases.

[Setting an appropriate channel][set-channel-4.15-oc] will gracefully recover the cluster.

## Alternatives

### Discontinuing the aliases

The old patterns could be used to create aliases for all future 4.y, but it's also possible to select a future 4.y in which we stop creating the aliases.
While the aliases are available, consumers are able to migrate to the new canonical names, or continue using the old patterns, as they wish.
But if a future 4.y discontinues the aliases, consumers would have to use the new canonical names or break.
During the deprecation phase, we could [alert on the use of the deprecated names](#cluster-version-operator), and point those admins at docs about how to update their channel to the new, canonical names.
Discontinuing aliases would bring more uniform usage to the channel ecosystem, but that uniformity may not be worth the cost of transitioning all the tools known to use the current patterns.

### Hidden aliases

Instead of the current plan for public alias channels, we could have taught graph-data about a new 1.3.0 [version][graph-data-version], adding a new [channel property][graph-data-channel]:

* `aliases` (1.3.0, optional, array of strings) is the set of recognized aliases for this channel.

Cincinnati could be trained to support [graph-data 1.3.0 and the new `aliases` property](#api-extensions).
A new [OpenShift Update Service][openshift-update-service] could be released so users running their own OSUS will be able to handle the new graph-data content.
Cincinnati is [not yet aware of forward-compatibility with new graph-data features][cincinnati-graph-data-forward-compatibility], so even users who do not need the new alias functionality will need to select from:

* Update to the new OSUS release to support 1.3.0 and `aliases`, or
* Adjust their graph-data image to claim it's only 1.2.0 or other older version and ignore `aliases`, or
* Have their older Cincinnati crash-loop on the unrecognized 1.3.0 `version`.

Compared to the current public alias plan, this would have the upside of docs and channel-management UIs needing to more directly address the presence of synonyms, because users would be less likely to bump into them than with the public alias approach.
Hidden aliases has the downside of not needing [Cincinnati changes or OSUS updates](#cincinnati).

### Opening on a long-end-of life 4.y

Similar to an extended overlapping alias phase (whether [public](#proposal) or [hidden](#hidden-aliases)), we could declare new-pattern channel names for very old 4.y (e.g. 4.8).
This would allow [managed changes](#managed-clusters) and other Service Delivery tooling to be tested against the new patterns before they needed to work.
And it would avoid the need to figure out public docs around the new-pattern names in the long-end-of-life releases that users were unlikely to notice or care about.
But in order to test managed-upgrade-operator behaviour, testers would need to be able to install managed 4.8 (or other end-of-life) releases, and it's not clear that they still have that ability when there's no direct customer value in continuing to allow those old installs.

I'm choosing 4.8 as the example, because the current MUO is [not compatible with 4.7 or earlier][managed-upgrade-operator-compatibility].
It's possible that there are other currently-unknown issues that would block QE from being able to install and test a 4.8 ROSA with a modern MUO.

### The value of post-GA soak time

As quoted in [the motivation section](#motivation), the only difference between the fast/GA and stable/fleet-approved channel is the promotion delay.
But how useful is that delay?
Let's survey the existing issues declared for 4.13 and 4.14:

* [`AROBrokenDNSMasq`][AROBrokenDNSMasq]: updates to 4.13.25 (among other releases) were exposed, and the risk was [declared 2023-12-25][AROBrokenDNSMasq-declared] almost two weeks after [4.13.25 entered `stable-4.13` on 2023-12-12][4.13.25-stable-4.13].
  The bug was [reported 2023-12-14][OCPBUGS-25406], which was after the stable channel promotion, but it was from a fast-channel cluster, so a longer soak might have allowed us to assess and declare the risk before the exposed updates entered stable.
* [`AWSECRLegacyCredProvider`][AWSECRLegacyCredProvider]: updates from 4.13 to 4.14 were exposed, and the risk was [declared 2024-01-05][AWSECRLegacyCredProvider-declared], months after [4.14 GAed 2023-10-31][4.14-ga], and ten days before [4.13-to-4.14 entered `stable-4.14` on 2024-01-15][4.13-to-4.14-stable].
* [`AWSMintModeWithoutCredentials`][AWSMintModeWithoutCredentials]: updates to 4.13.9 were exposed, and the risk was [declared 2023-08-25][https://github.com/openshift/cincinnati-graph-data/pull/3984#event-10099561988], three days after [4.13.9 entered `stable-4.13` on 2023-08-22][4.13.9-stable-4.13].
  But the bug was [reported 2023-08-15][OCPBUGS-17733], [the same day the release GAed][], so a longer soak would have allowed us to assess and declare the risk before the exposed updates entered stable.
* [`AzureDefaultVMType`][AzureDefaultVMType]: updates from 4.13 to 4.14 were exposed, and the risk was [declared 2023-12-20][AzureDefaultVMType-declared], months after [4.14 GAed 2023-10-31][4.14-ga], and almost a month before [4.13-to-4.14 entered `stable-4.14` on 2024-01-15][4.13-to-4.14-stable].
* [`AzureRegistryImagePreservation`][AzureRegistryImagePreservation]: updates from 4.13 to 4.14 were exposed, and the risk was [declared 2024-02-05][AzureDefaultVMType-declared], weeks after [4.13-to-4.14 entered `stable-4.14` on 2024-01-15][4.13-to-4.14-stable].
* [`ConsoleImplicitlyEnabled`][ConsoleImplicitlyEnabled]: updates from 4.13 to 4.14 were exposed, and the risk was [declared 2023-10-30][ConsoleImplicitlyEnabled-declared], one day before [4.14 GAed][4.14-ga].
* [`MultiNetworkAttachmentsWhereaboutsVersion`][MultiNetworkAttachmentsWhereaboutsVersion]: updates to 4.13.0 (among other releases) were exposed, and the risk was [declared 2023-07-03][MultiNetworkAttachmentsWhereaboutsVersion-declared], months a [4.13 GAed 2023-05-17][4.13-ga], and months before [4.12-to-4.13 entered `stable-4.13` on 2023-08-15][4.12-to-4.13-stable].
* [`NetPolicyTimeoutsHostNetworkedPodTraffic`][NetPolicyTimeoutsHostNetworkedPodTraffic]: updates to 4.13.30 (among other releases) were exposed, and the risk was [declared 2024-02-08][NetPolicyTimeoutsHostNetworkedPodTraffic-declared], over a week after [4.13.30 entered `stable-4.13` on 2024-01-30][4.13.30-stable-4.13].
  And the bug was [reported 2024-02-02][OCPBUGS-28920], based on data from a `stable-4.13` cluster, so it's unlikely that longer soak would have helped turn this up before the exposed updates entered stable.
* [`OVNKubeMasterDSPrestop`][OVNKubeMasterDSPrestop]: updates to 4.13.17 (among other releases) were exposed, and the risk was [declared 2023-11-13][OVNKubeMasterDSPrestop-declared], weeks after [4.13.17 entered `stable-4.13` on 2023-10-24][4.13.17-to-stable-4.13].
  But the bug was [reported 2023-10-23][OCPBUGS-22293], based on data from a `fast-4.13` cluster, so a longer soak might have allowed us to assess and declare the risk before the exposed updates entered stable.
* [`PerformanceProfilesCPUQuota`][PerformanceProfilesCPUQuota]: updates from 4.12 to 4.13 were exposed, and the risk was [declared 2023-06-29][PerformanceProfilesCPUQuota-declared], over a month after [4.13 GAed 2023-05-17][4.13-ga], and months before [4.12-to-4.13 entered `stable-4.13` on 2023-08-15][4.12-to-4.13-stable].
* [`PersistentVolumeDiskIDSymlinks`][PersistentVolumeDiskIDSymlinks]: updates from 4.12 to 4.13 were exposed, and the risk was [declared 2023-08-01][PersistentVolumeDiskIDSymlinks-declared], months after [4.13 GAed 2023-05-17][4.13-ga], and weeks before [4.12-to-4.13 entered `stable-4.13` on 2023-08-15][4.12-to-4.13-stable].
* [`SeccompFilterErrno524`][SeccompFilterErrno524]: updates from 4.12 to 4.13 were exposed, and the risk was [declared 2023-09-13][SeccompFilterErrno524-declared], almost a month after [4.12-to-4.13 entered `stable-4.13` on 2023-08-15][4.12-to-4.13-stable].

Aggregating:

* 8 risks around minor-version updates (4.y to 4.(y+1)).
  * 1 caught before the update was generally available.
  * 5 caught before the update was in the stable channel.
  * 2 caught after the update was in the stable channel.
* 4 risks around patch-version updates (4.y.z to 4.y.z'), all caught after the update was in the stable channel.
  * 3 caught by clusters in the fast channel or earlier, but we would have had to soak for almost a month after GA to have been able to turn those reports into pre-stable risk declarations.
  * 1 caught by a cluster in the stable channel, so no amount of soak would have necessarily turned this one up earlier.

The current week-or-two is too short to have outreached risk-declaration for these four patch-version regressions, so currently users who wait on that soak are paying all the cost of deferring bugfixes, while still not waiting long enough to reap the benefits of outwaiting risk declarations.
Unless they are waiting significantly longer after the update is recommended in their chosen channel before updating, which you can do regardless of whether or not your channel has built-in soak.

#### Soak everything for months

Do we want to extend [our current][fast-stable-channel-strategies]:

> Updates to the latest z-streams are generally promoted to the stable channel within a week or two...

to 45 days or something?
How many users would wait that long for patch updates?
Yes, there's a benefit to knowing about those 4 patch regressions before you update.
But a GCP cluster born in 4.11 or later, and many other cluster flavors, would not have been exposed to any of the four patch-version regressions.
And there's a risk to running longer while exposed to the bugs that the patch releases are fixing.

But committing to a generic duration range for post-GA soaking is hard.
Do you commit to a lower bound, and deal with "why so slow?" pressure when that delay is slowing access to an important bugfix?
Do you commit to an upper bound, and deal with "why not wait for more data?" pressure when too few users happen to use the soak window to provide feedback?
Do you weigh each new patch release independently to try and balance the importance of the fixes it is known to bring against the risk of not-yet-identified regressions it might also bring, with docs like "we'll know how long to wait when we look at each new patch, and no earlier"?
Users are likely to want both the per-patch tuning of the custom assessment option _and_ the predicatability of the committed, narrow duration range, and it may not be possible to deliver both.

And again, we have no tools to force, or even trigger, customer updates, and customers can always add as much soak as they like after update recommendations appear in their channels.

#### Only soak minor-version updates

We could drop soak for patch-version updates while retaining the current long soak for minor-version updates.
The risk of delaying minor-version updates is just delayed access to new features, which is more sustainable than delaying access to patch-version bugfixes.
What would a channel like this be called?
`fleet-approved-4.y` doesn't explain the meaning, but might be fine with [increased access to channel semantics](#increase-access-to-channel-semantics).
`i-am-happy-to-wait-months-for-features-4.y`?

#### Drop Red-Hat-side post-GA soaking

We could drop the soak entirely, and leave it up to customers to select their target amount of soak after an update recommendation appears in a generally-available channel.

#### Installer default channel

Unless we [consolidate on a single generally-available channel](#consolidate-on-a-single-generally-available-channel), [the installer](#installer) needs to have an opinion on the default channel used by new installs.
Since 4.1, we have defaulted to a channel from the `stable-4.y` pattern.
With this enhancement's rename, and additional implementation options like [client-side soak](#client-side-soak), we may want to revisit our choice.
Moving to an unsoaked, GA channel would:

* Avoid [the `VersionNotFound` window that comes with using a soaked default channel with server-side soak](#server-side-soak) while soaking patch-version updates (although [we could choose to only soak minor-version updates](#only-soak-minor-version-updates)).
* Reduced "why do I not see updates to...?" uncertainty for users who are unfamiliar with the channel's soak policy.
  Although we could mitigate this with [client-side soak](#client-side-soak) or [increased access to channel semantics](#increase-access-to-channel-semantics).
* Increase the risk that a user initiates an update recommended in the unsoaked, generally-available channel and bumps into a patch-version regression that would have been declared by the time the update was recommended in the soaked channel.
  But [there aren't currently any risks like that](#the-value-of-post-ga-soak-time), and in order to reliably declare more patch-version regressions during the soak period, we would need to [soak those updates for over a month](#soak-everything-for-months).

This proposal currently suggests the installer stick with the familiar `stable-4.y` pattern for 4.15, and move to the `ga-4.y` pattern starting with 4.16.
But obviously other alternatives, like moving to `fleet-approved-4.y` are possible, depending on how folks weight the above tradeoffs.

### Server-side soak

[The various post-GA soak strategies](#the-value-of-post-ga-soak-time) can be implemented server-side in Cincinnati and graph-data as they are today, where soaking updates are not included in the channel at all.
The main benefit is that folks are used to this approach.
The downsides include:

* A lack of per-update transparency about soaking happening, or when soaking is expected to wrap up.
  This can be mitigated by [increased access to channel semantics](#increase-access-to-channel-semantics) to make it easier to discover [channel-level soak policy documentation][fast-stable-channel-strategies].
* Alerting on `VersionNotFound` if users install 4.y.z after it is generally available (and enters that channel) but before it entes the slower channel.
  For example, 4.14.10 entered `fast-4.14` on 2024-01-24 and `stable-4.14` on 2024-01-30.
  Installing it during that window with its default `stable-4.14` channel would have triggered `VersionNotFound` `CannotRetrieveUpdates` alerting.
  This could be mitigted by teaching Cincinnati to immediately add 4.14.10 to both `fast-4.14` and `stable-4.14` when its errata is published, while delaying the presence of the updates into 4.14.10 during the soak phase:

  1. 4.14.10 errata published, 4.14.10 added to  `fast-4.14` and `stable-4.14`.  4.14.9 to 4.14.10 and similar added to `fast-4.14`.
  2. Soak the updates to 4.14.10.
  3. 4.14.9 to 4.14.10 and similar added to `stable-4.14`.

  But that would take Cincinnati changes.
  We could also remove the `VersionNotFound` window by [only soaking minor-version updates](#only-soak-minor-version-updates).

### Client-side soak

Since [4.10's conditional update system](targeted-update-edge-blocking.md), we can push information out to clients for increased transparencey.
[OTA-612][] gives some possibilities for what a `WaitingForFeedback` condition might look like.

1. 4.14.10 errata published, 4.14.10 added to  `fast-4.14` and `stable-4.14`.  4.14.9 to 4.14.10 and similar added to `fast-4.14` and `stable-4.14` with a `WaitingForFeedback` update condition.
2. Soak the updates to 4.14.10.
3. Remove the `WaitingForFeedback` update condition from those updates.

This would increase transparency, for the subset of users who read messages about matching risks, by bundling that context as closely as possible to the updates it is affecting.
[OTA-902][] talks about some possible user interface changes that might increase the visibility of this information.
Benefits:

* Possible decrease in the number of folks who ask us "why don't I see updates to 4.y[.z] yet?" while we wait for it to cook, by bringing that context into the cluster and its update UIs.
* Lets us decrease the time we cook for cluster flavors that give us high volumes of feedback quickly, by adjusting PromQL on the `WaitingForFeedback` risk.

Downsides:

* Attempting to use conditional update risks to deliver context to users who were not aware of the feedback delay would not work for the subset of users who do not dig into matching-risk details.
* Users used to the previous generally-available channel handling might be surprised to have a "risk" for updates that they'd previously been able to accept by chosing the quicker channel.

These are not mutually-exclusive.
We could serve channels with server-side soak alongside separate channels with client-side soak, if we wanted to soft-launch and collect user feedback, at the cost of the increased complexity while both channels were available for users to choose between.

### Consolidate on a single generally-available channel

[Client-side soak](#client-side-soak) or [dropping Red-Hat-side post-GA soaking](#drop-red-hat-side-post-ga-soaking) would remove the current distinction between `fast-4.y` vs. `stable-4.y`, allowing us to collapse to a single channel for all generally-available updates.
This increases simplicity by removing the need for GA users to select between multiple channels.

If we remove an existing channel name, consumers who depend on the old pattern will need to pivot, although we could use [private](#api-extensions) or [public aliases](#public-aliases) to [allow continued use of old channel-name patterns](#tools-making-update-service-requests).

### Increase access to channel semantics

Part of the current user confusion stems from guessing about the meaning of channel names like `fast-4.15` without referring to [that channel's documentation][fast-4.15], and guessing that it means "beta" or something when it actually means "generally available".
One path to limiting that confusion would be [an update service channel-listing API][channels-api], so UIs that render channel names could follow them up with an inline sentence or two explaining the semantics.
However, even when text is available, readers may skim and jump to conclusions based on the early wording, so having that inline context might still not be sufficient.
But increasing access to semantics is not mutually exclusive with the other options, and we can also supplement [better names](#api-extensions) or [client-side soak](#client-side-soak) with a channel-listing API in the future.

### Expanded GA to generally-available

Instead of `ga-4.y`, we could use `generally-available-4.y`.
This might help folks who hadn't yet internalized the GA acronym, or who were slow to recognize its downcased form.
But the majority of folks interacting with channels would likely recognize the `ga-4.y`.
And places dealing with channels could link to docs that unpacked the names into multiple sentences of semantics.
And there may also be folks interacting from command-lines without autocomplete, and `ga-4.y` is easier to type without errors.

### Rename managed channel-groups

It seems like it should be possible to have the existing `stable` channel group unpack to the new channel patterns, as discussed in [the managed exposure section](#managed-clusters).
It would also be possible to rename the channel groups from `fast` and `stable` to `ga` and `fleet-approved`.
Upsides include more likely recognition in anyone newly exposed to channel-groups who happens to already be familiar with the new-pattern channel names (and vice versa).
Downsides includes the cost of locating and adjusting consumers who expect the current channel-group naming, including [the current `rosa` example output](#managed-clusters), but likely extending beyond that to many OCM and other consumers I'm not familiar with.
However, for channel-renaming, it seems like channel-groups provide a convenient, existing API-translation layer, and sticking with the current channel-group names (at least in the short term) allows the channel-group rename question to be addressed separately, if and when Service Delivery decides to discuss it, and not a question that needs to be sorted out before channel-renaming happens.

## Related projects

### Zincati

[Zincati][] is an auto-update agent for Fedora CoreOS hosts, and it uses the Cincinnati protocol.
But [Zincati's version of the protocol uses `stream`][Zincati-protocol-request], not [the OpenShift-specific `channel`][cincinnati-openshift-protocol-request], and [Zincati does not consume Cincinnati code][Zincati-cargo-lock], so it is not affected by OpenShift channel renames.

[4.12-to-4.13-stable]: https://github.com/openshift/cincinnati-graph-data/pull/3983#event-10098856342
[4.13.9-fast-4.13]: https://github.com/openshift/cincinnati-graph-data/pull/3991#event-10103441309
[4.13.9-stable-4.13]: https://github.com/openshift/cincinnati-graph-data/pull/4016#event-10165350898
[4.13.17-to-stable-4.13]: https://github.com/openshift/cincinnati-graph-data/pull/4287#event-10759496158
[4.13-25-fast-4.13]: https://github.com/openshift/cincinnati-graph-data/pull/4477#event-11161604677
[4.13.25-stable-4.13]: https://github.com/openshift/cincinnati-graph-data/pull/4509#event-11230723183
[4.13.30-stable-4.13]: https://github.com/openshift/cincinnati-graph-data/pull/4698#event-11651977759
[4.13-ga]: https://github.com/openshift/cincinnati-graph-data/pull/3622#event-9274843432
[4.13-to-4.14-stable]: https://github.com/openshift/cincinnati-graph-data/pull/4616#event-11490100022
[4.14-ga]: https://github.com/openshift/cincinnati-graph-data/pull/4325#event-10823996200
[AROBrokenDNSMasq]: https://issues.redhat.com/browse/MCO-958
[AROBrokenDNSMasq-declared]: https://github.com/openshift/cincinnati-graph-data/pull/4524#event-11263963954
[AWSECRLegacyCredProvider]: https://issues.redhat.com/browse/OCPCLOUD-2434
[AWSECRLegacyCredProvider-declared]: https://github.com/openshift/cincinnati-graph-data/pull/4575#event-11405253651
[AWSMintModeWithoutCredentials]: https://issues.redhat.com/browse/NE-1376
[AWSMintModeWithoutCredentials-declared]: https://github.com/openshift/cincinnati-graph-data/pull/3984#event-10099561988
[AzureDefaultVMType]: https://issues.redhat.com/browse/OCPCLOUD-2409
[AzureDefaultVMType-declared]: https://github.com/openshift/cincinnati-graph-data/pull/4541#event-11307089067
[AzureRegistryImagePreservation]: https://issues.redhat.com/browse/IR-461
[AzureRegistryImagePreservation-declared]: https://github.com/openshift/cincinnati-graph-data/pull/4725#event-11707604003
[ClusterCurator-spec-upgrade-channel]: https://github.com/stolostron/cluster-curator-controller/blame/51d80683b9dc970d3dbb6dbcd9a11ee6824db3d2/pkg/api/v1beta1/clustercurator_types.go#L108-L112
[ConsoleImplicitlyEnabled]: https://issues.redhat.com/browse/OTA-1031
[ConsoleImplicitlyEnabled-declared]: https://github.com/openshift/cincinnati-graph-data/pull/4234#event-10811233456
[HostedCluster-spec-channel]: https://github.com/openshift/hypershift/blob/8b92350b3c885bb3c0a2e3fa02e3c93b6f062703/api/hypershift/v1beta1/hosted_controlplane.go#L44-L49
[HostedCluster-status-version-desired]: https://github.com/openshift/hypershift/blob/8b92350b3c885bb3c0a2e3fa02e3c93b6f062703/api/hypershift/v1beta1/hostedcluster_types.go#L2102-L2105
[MultiNetworkAttachmentsWhereaboutsVersion]: https://access.redhat.com/solutions/7024726
[MultiNetworkAttachmentsWhereaboutsVersion-declared]: https://github.com/openshift/cincinnati-graph-data/pull/3794#event-9711991321
[NetPolicyTimeoutsHostNetworkedPodTraffic]: https://issues.redhat.com/browse/SDN-4481
[NetPolicyTimeoutsHostNetworkedPodTraffic-declared]: https://github.com/openshift/cincinnati-graph-data/pull/4744#event-11750823610
[OCPBUGS-17733]: https://issues.redhat.com/browse/OCPBUGS-17733
[OCPBUGS-22293]: https://issues.redhat.com/browse/OCPBUGS-22293
[OCPBUGS-25406]: https://issues.redhat.com/browse/OCPBUGS-25406
[OCPBUGS-28920]: https://issues.redhat.com/browse/OCPBUGS-28920
[OTA-612]: https://issues.redhat.com/browse/OTA-612
[OTA-902]: https://issues.redhat.com/browse/OTA-902
[OTA-1219]: https://issues.redhat.com/browse/OTA-1219
[OTA-1220]: https://issues.redhat.com/browse/OTA-1220
[OTA-1221]: https://issues.redhat.com/browse/OTA-1221
[OVNKubeMasterDSPrestop]: https://issues.redhat.com/browse/SDN-4196
[OVNKubeMasterDSPrestop-declared]: https://github.com/openshift/cincinnati-graph-data/pull/4377#event-10944562026
[PerformanceProfilesCPUQuota]: https://issues.redhat.com/browse/OCPNODE-1705
[PerformanceProfilesCPUQuota-declared]: https://github.com/openshift/cincinnati-graph-data/pull/3786#event-9677151808
[PersistentVolumeDiskIDSymlinks]: https://issues.redhat.com/browse/COS-2349
[PersistentVolumeDiskIDSymlinks-declared]: https://github.com/openshift/cincinnati-graph-data/pull/3906#event-9976639160
[SeccompFilterErrno524]: https://issues.redhat.com/browse/COS-2437
[SeccompFilterErrno524-declared]: https://github.com/openshift/cincinnati-graph-data/pull/4121#event-10360936562
[RHACM]: https://access.redhat.com/products/red-hat-advanced-cluster-management-for-kubernetes
[MCE]: https://docs.openshift.com/container-platform/4.14/architecture/mce-overview-ocp.html
[MCE-console-availableChannels]: https://github.com/stolostron/console/pull/609/files#diff-778afcb495518fc937e9792dba789cc8478a703af8621869e72e261399b20711R455
[MCE-console-channel-text]: https://github.com/stolostron/mce/blob/a4fbb3d15b4180973afb41764c1850ee23d82ba4/frontend/public/locales/en/translation.json#L554
[MCE-old-channel-name-references-1]: https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.9/html/clusters/cluster_mce_overview#clusterimageset-fast-channel
[MCE-old-channel-name-references-2]: https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.9/html/clusters/cluster_mce_overview#cluster-image-set
[MCE-set-channel]: https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.9/html/clusters/cluster_mce_overview#selecting-a-channel
[Zincati]: https://coreos.github.io/zincati/
[Zincati-cargo-lock]: https://github.com/coreos/zincati/blob/9fd22936c7ede97ba8f46daa1f00c720dad7b08f/Cargo.lock
[Zincati-protocol-request]: https://github.com/coreos/zincati/blob/9fd22936c7ede97ba8f46daa1f00c720dad7b08f/docs/development/cincinnati/protocol.md#request
[acm-hive-openshift-releases-Makefile]: https://github.com/stolostron/acm-hive-openshift-releases/blob/8b3aad11e3945c605b6f61e34a6a5b8134caec61/Makefile#L24-L77
[acm-hive-openshift-releases-clusterImageSets]: https://github.com/stolostron/acm-hive-openshift-releases/tree/8b3aad11e3945c605b6f61e34a6a5b8134caec61/clusterImageSets
[acm-hive-openshift-releases-cron]: https://github.com/stolostron/acm-hive-openshift-releases/blob/8b3aad11e3945c605b6f61e34a6a5b8134caec61/.github/workflows/cron-sync-imageset.yml
[acm-hive-openshift-releases-tooling]: https://github.com/stolostron/acm-hive-openshift-releases/blob/8b3aad11e3945c605b6f61e34a6a5b8134caec61/tooling/promote-stable-clusterimagesets.py#L16
[aro-docs-channel]: https://learn.microsoft.com/en-us/azure/openshift/support-lifecycle#upgrade-channels
[assisted-installer-custom-releases]: https://github.com/openshift/assisted-service/pull/5916
[assisted-installer-custom-releases-channel-names]: https://github.com/openshift/assisted-service/pull/5916#discussion_r1489119941
[channel-docs]: https://docs.openshift.com/container-platform/4.14/updating/understanding_updates/understanding-update-channels-release.html
[channels-api]: https://issues.redhat.com/browse/OTA-162
[ci-docs-release-channel-example]: https://github.com/openshift/ci-docs/blob/8acb4c27721e8b5ec796bab647d017dea694e642/content/en/docs/architecture/ci-operator.md?plain=1#L477
[ci-tools]: https://github.com/openshift/ci-tools
[cincinnati-graph-data-forward-compatibility]: https://issues.redhat.com/browse/OTA-1045
[cincinnati-openshift-protocol-request]: https://github.com/openshift/cincinnati/blob/ed5a214c60ad05d5227b4d971be7e323dd8a10bd/docs/design/openshift.md#request
[cluster-curator-controller-hive-set-channel]: https://github.com/stolostron/cluster-curator-controller/blame/51d80683b9dc970d3dbb6dbcd9a11ee6824db3d2/pkg/jobs/hive/hive.go#L510
[cluster-version-operator-CannotRetrieveUpdates]: https://github.com/openshift/cluster-version-operator/blob/ce6169c7b9b0d44c2e41342e6414ed9db0a31a63/install/0000_90_cluster-version-operator_02_servicemonitor.yaml#L53
[cluster-version-operator-RetrievedUpdates]: https://github.com/openshift/cluster-version-operator/blob/ce6169c7b9b0d44c2e41342e6414ed9db0a31a63/pkg/cvo/status.go#L385
[cluster-version-status-channels]: https://github.com/openshift/cluster-version-operator/pull/419/files#diff-4229ccef40cdb3dd7a8e5ca230d85fa0e74bbc265511ddd94f53acffbcd19b79R258-R261
[console-create]: https://console.redhat.com/openshift/create
[console-install-user-provisioned]: https://console.redhat.com/openshift/install/platform-agnostic/user-provisioned
[dev-scripts-stable-clients]: https://github.com/openshift-metal3/dev-scripts/blob/c05a3c84314f9b59602a040c23396234efca7309/common.sh#L115
[fast-4.15]: https://docs.openshift.com/container-platform/4.15/updating/understanding_updates/understanding-update-channels-release.html#fast-version-channel_understanding-update-channels-releases
[fast-stable-channel-strategies]: https://docs.openshift.com/container-platform/4.14/updating/understanding_updates/understanding-update-channels-release.html#fast-stable-channel-strategies_understanding-update-channels-releases
[graph-data-channel]: https://github.com/openshift/cincinnati-graph-data/tree/63c43c07bf18de676422a85905aa29dda699debc?tab=readme-ov-file#add-releases-to-channels
[graph-data-release-script-update]: https://issues.redhat.com/browse/OTA-1218
[graph-data-version]: https://github.com/openshift/cincinnati-graph-data/tree/63c43c07bf18de676422a85905aa29dda699debc?tab=readme-ov-file#schema-version
[hive-reconcile-cluster-version]: https://github.com/openshift/hive/blob/8c54fc9cac459e0c5852a55775ac570695e3b465/pkg/controller/clusterversion/clusterversion_controller.go#L84-L202
[hypershift-clears-upstream]: https://github.com/openshift/hypershift/blob/8b92350b3c885bb3c0a2e3fa02e3c93b6f062703/control-plane-operator/hostedclusterconfigoperator/controllers/resources/resources.go#L1059
[hypershift-configurable-upstream]: https://issues.redhat.com/browse/OCPSTRAT-1181
[hypershift-default-channel]: https://issues.redhat.com/browse/HOSTEDCP-906
[install-docs]: https://docs.openshift.com/container-platform/4.14/installing/installing_platform_agnostic/installing-platform-agnostic.html#installation-obtaining-installer_installing-platform-agnostic
[installer-default-channel]: https://github.com/openshift/installer/pull/7867
[managed-upgrade-operator-compatibility]: https://github.com/openshift/managed-cluster-config/blob/0dc4c2c8d5c6215f93000da1a0990237fc77508f/deploy/managed-upgrade-operator-config/config.yaml#L9
[managed-upgrade-operator-inferUpgradeChannelFromChannelGroup]: https://github.com/openshift/managed-upgrade-operator/blob/ceec30b0ad3f1d6d71cb61e6d4345db26196ee43/pkg/ocmprovider/ocmprovider.go#L202-L217
[mirror-fast-4.14]: https://mirror.openshift.com/pub/openshift-v4/amd64/clients/ocp/fast-4.14/
[mirror-stable-installer]: https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/openshift-install-linux.tar.gz
[mirror-stable-signature]: https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/sha256sum.txt.gpg
[mirror-stable]: https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/
[oc-adm-upgrade-channel]: https://github.com/openshift/oc/pull/576
[openshift-update-service]: https://docs.openshift.com/container-platform/4.14/updating/updating_a_cluster/updating_disconnected_cluster/disconnected-update-osus.html
[portal-ocp]: https://access.redhat.com/downloads/content/290/
[release]: https://github.com/openshift/release
[rosa-command-line-channel-group]: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-creating-cluster.html#rosa-creating-cluster_rosa-creating-cluster
[set-channel-4.14-web-console]: https://docs.openshift.com/container-platform/4.14/updating/updating_a_cluster/updating-cluster-web-console.html
[set-channel-4.15-oc]: https://docs.openshift.com/container-platform/4.15/updating/updating_a_cluster/updating-cluster-cli.html
[stable-4.14]: https://docs.openshift.com/container-platform/4.14/updating/understanding_updates/understanding-update-channels-release.html#stable-version-channel_understanding-update-channels-releases
[web-console-4.15-channels]: https://github.com/openshift/console/pull/4981
[web-console-channel-doc-link]: https://issues.redhat.com/browse/OCPBUGS-29121
[web-console-channel-modal]: https://github.com/openshift/console/blob/59f3518444c670d5f8364e0ba7b9b5c70a53de29/frontend/public/components/modals/cluster-channel-modal.tsx#L79-L88
[web-console-consumes-channels]: https://github.com/openshift/console/pull/6283/files#diff-ab468773bf6bdc82888f5abe2318066860f9e030113098584bba00ffbace2463R45
[web-console-drops-fallback-channels]: https://github.com/openshift/console/pull/8392/files#diff-ab468773bf6bdc82888f5abe2318066860f9e030113098584bba00ffbace2463L45-R43
