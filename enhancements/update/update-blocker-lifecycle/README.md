---
title: update-blocker-lifecycle
authors:
  - "@wking"
reviewers:
  - "@LalatenduMohanty"
approvers:
  - "@sdodson"
api-approvers:
  - None
creation-date: 2020-09-11
last-updated: 2025-06-18
status: implementable
---

# Update-blocker Lifecycle

We occasionally have bugs which impact update success or the stability of the target release.
When that happens, we protect users by [removing update recommendations or qualifying recommendations them with conditional risks][graph-data-block].
This enhancement describes the process used to identify these bugs and clarify the resulting update risks.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The lifecycle for recommendation changes looks like:

<div style="text-align:center">
  <img src="flow.svg" width="100%" />
</div>

It is tedious for graph-admins to audit bugs with `UpgradeBlocker` to see where they are in the lifecycle and, when necessary, poke component teams about outstanding impact statement requests.
Having an explicit, machine-readable lifecycle reduces the chances that issues fall through the cracks by clarifying the responsible parties for moving the bug to the next stage, which supports tracking and automated reminders.

*Note:*
* In Jira, all of the labels including `UpgradeBlocker` use the labels field.
* In general when we add a label we remove the old label. For example when we add `ImpactStatementProposed` label we remove the `ImpactStatementRequested` label.

## Ticket queues

With the changes from this enhancement, the queues become:

* [Suspect queue][suspect-queue].
* [Component developer queue][component-dev-queue] (individual component teams probably want to add additional filtering for their components).
* [Graph-admin queue][graph-admin-queue].

## Motivation

### Goals

* Clearly define, in a machine-readable fashion, the currently responsible party for bugs in the update-recommendation lifecycle.

### Non-Goals

This enhancement does not attempt to:

* Cover issues which have not yet arrived in the bug system.
* Cover bugs which do not have the `UpgradeBlocker` label.
    For example, bugs with just the `Upgrades` label are not included in the update-recommendation lifecycle.
* Remove the `UpgradeBlocker` label, because that might disrupt existing consumers.

## Proposal

Add new labels for `ImpactStatementRequested`, `ImpactStatementProposed`, and `UpdateRecommendationsBlocked`.
Write tooling that automatically:

* Removes the new labels if any labels from later in the process are set.
* Adds `Upgrades` and `UpgradeBlocker` to any bugs with any of the new labels.

### Impact statement request

The following statement (or a link to this section) can be pasted into bugs when adding `ImpactStatementRequested`:

We're asking the following questions to evaluate whether or not OCPBUGS-# warrants changing update recommendations from either the previous X.Y or X.Y.Z.
The ultimate goal is to avoid recommending an update which introduces new risk or reduces cluster functionality in any way.
In the absence of a declared update risk (the status quo), there is some risk that the existing fleet updates into the at-risk releases.
Depending on the bug and estimated risk, leaving the update risk undeclared may be acceptable.

Sample answers are provided to give more context and the `ImpactStatementRequested` label has been added to OCPBUGS-#.
When responding, please move this ticket to `Code Review`.
The expectation is that the assignee answers these questions.

Which 4.y.z to 4.y'.z' updates increase vulnerability?
* reasoning: This allows us to populate [`from` and `to` in conditional update recommendations][graph-data-block] for "the `$SOURCE_RELEASE` to `$TARGET_RELEASE` update is exposed.
* example: Customers upgrading from any 4.y (or specific 4.y.z) to 4.(y+1).z'.  Use `oc adm upgrade` to show your current cluster version.

Which types of clusters?
* reasoning: This allows us to populate [`matchingRules` in conditional update recommendations][graph-data-block] for "clusters like `$THIS`".
* example: GCP clusters with thousands of namespaces, approximately 5% of the subscribed fleet.  Check your vulnerability with `oc ...` or the following PromQL `count (...) > 0`. If PromQL is provided and the underlying bug might impact updates out of a [4.19](https://docs.redhat.com/en/documentation/openshift_container_platform/4.19/html-single/release_notes/index#ocp-4-19-monitoring-metrics-collection-profiles-ga) or newer cluster, please list [the metrics collection profiles](https://docs.redhat.com/en/documentation/openshift_container_platform/4.19/html-single/monitoring/index#choosing-a-metrics-collection-profile_configuring-performance-and-scalability) with which the PromQL works.

The two questions above are sufficient to declare an initial update risk, and we would like as much detail as possible on them as quickly as you can get it.
Perfectly crisp responses are nice, but are not required.
For example "it seems like these platforms are involved, because..." in a day 1 draft impact statement is helpful, even if you follow up with "actually, it was these other platforms" on day 3.
In the absence of a response within 7 days, we may or may not declare a conditional update risk based on our current understanding of the issue.

If you can, answers to the following questions will make the conditional risk declaration more actionable for customers.

What is the impact?  Is it serious enough to warrant removing update recommendations?
* reasoning: This allows us to populate [`name` and `message` in conditional update recommendations][graph-data-block] for "...because if you update, `$THESE_CONDITIONS` may cause `$THESE_UNFORTUNATE_SYMPTOMS`".
* example: Around 2 minute disruption in edge routing for 10% of clusters.  Check with `oc ...`.
* example: Up to 90 seconds of API downtime.  Check with `curl ...`.
* example: etcd loses quorum and you have to restore from backup.  Check with `ssh ...`.

How involved is remediation?
* reasoning: This allows administrators who are already vulnerable, or who chose to waive conditional-update risks, to recover their cluster.
  And even moderately serious impacts might be acceptable if they are easy to mitigate.
* example: Issue resolves itself after five minutes.
* example: Admin can run a single: `oc ...`.
* example: Admin must SSH to hosts, restore from backups, or other non standard admin activities.

Is this a regression?
* reasoning: Updating between two vulnerable releases may not increase exposure (unless rebooting during the update increases vulnerability, etc.).
  We only qualify update recommendations if the update increases exposure.
* example: No, it has always been like this we just never noticed.
* example: Yes, from 4.y.z to 4.y+1.z Or 4.y.z to 4.y.z+1.

### User Stories

#### A developer wondering about a serious bug

Before this enhancement, the "is this worth altering update recommendations?" process was less discoverable.
With this enhancement, the concerned developer only needs to add the `UpgradeBlocker` label to initiate the process.
And they also have access to this document to more easily understand the rest of the process, if they need to push the whole decision through before an update monitor is available to help out.

#### A component maintainer assigned to a bug

This enhancement adds labels to make it clear whether the bug assignee is currently responsible for providing an impact statement (`ImpactStatementRequested`), or whether the bug assignee has fulfilled their responsibility and moved the bug to `ImpactStatementProposed`.

#### An update monitor managing multiple bugs

This enhancement formalizes the various steps in the decision process, allowing for some steps to be automated, and giving a clear `ImpactStatementProposed` queue for final graph-data management decisions.

### API Extensions

No API; just internal process.

### Risks and Mitigations

No risks.

## Design Details

It's all up in [the *proposal* section](#proposal).

### Test Plan

No test plan.

### Graduation Criteria

No graduation criteria; this is internal policy and has no backwards-compatibility commitments.

#### Dev Preview -> Tech Preview

No graduation criteria.

#### Tech Preview -> GA

No graduation criteria.

#### Removing a deprecated feature

We might decide to do something completely different tomorrow.
Don't build anything on top of this process that you would be sad about throwing away.

### Upgrade / Downgrade Strategy

This enhancement is only intended to help ongoing graph-data operation.
If we pivot strategies, we will likely abandon any closed bugs without porting them to the new strategy.
Or we may port closed bugs to new strategies, if that makes implementing the next strategy easier.

### Version Skew Strategy

All of the data is in the bug system, and we control all the consumers for this internal workflow.
So if we pivot strategies, we can turn off any robots and port everything that needs porting at once.
There is no need to provision for version skew.

### Operational Aspects of API Extensions

No API; just internal process.

#### Failure Modes

No API; just internal process.

#### Support Procedures

No API; just internal process.

## Implementation History

No implementation.

## Drawbacks

No drawbacks.

## Alternatives (Not Implemented)

### Drop `UpgradeBlocker`

As an alternative to keeping the `UpgradeBlocker` label, we could replace it with `ImpactStatementRequestRequested` or some such if folks need bot assistance to move bugs into the component developer's queue.
This would avoid the need for tooling to add `UpgradeBlocker` when missing, and makes it less likely that folks misconstrue a suspect (`ImpactStatementRequested`) as an actual blocker (`UpdateRecommendationsBlocked`).

However, removing `UpgradeBlocker` would break folks who are expecting the current label.
But it's not clear to me who would want to keep consuming that label after we grow the fine-grained labeling, but we have been trying to train developers to set `UpgradeBlocker` on potential blockers, and retraining developers is hard.

[graph-data-block]: https://github.com/openshift/cincinnati-graph-data/tree/0335e56cde6b17230106f137382cbbd9aa5038ed#block-edges
[suspect-queue]: https://issues.redhat.com/issues/?jql=project%20%3D%20OCPBUGS%20AND%20labels%20in%20(upgradeblocker)%20AND%20labels%20not%20in%20(ImpactStatementRequested%2C%20ImpactStatementProposed%2C%20UpdateRecommendationsBlocked)
[component-dev-queue]: https://issues.redhat.com/issues/?filter=12400182&jql=project%20%3D%20OCPBUGS%20AND%20labels%20in%20(ImpactStatementRequested)
[graph-admin-queue]: https://issues.redhat.com/issues/?jql=project%20%3D%20OCPBUGS%20AND%20labels%20in%20(ImpactStatementProposed)
