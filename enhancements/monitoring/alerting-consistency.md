---
title: alerting-consistency
authors:
  - "@michaelgugino"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-02-03
last-updated: 2021-02-03
status: implementable
---

# Alerting Consistency

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Clear and actionable alerts are a key component of a smooth operational
experience.  Ensuring we have clear and concise guidelines for our alerts
will allow developers to better inform users of problem situations and how to
resolve them.

## Motivation

Improve Signal to noise of alerts.  Align alert levels "Critical, Warning, Info"
with operational expectations to allow easier triaging.  Ensure current and
future alerts have documentation covering problem/investigation/solution steps.

### Goals

* Define clear criteria for designating an alert 'critical'
* Define clear criteria for designating an alert 'warning'
* Define minimum time thresholds before prior to triggering an alert
* Define ownership and responsibilities of existing and new alerts
* Establish clear documentation guidelines
* Outline operational triage of firing alerts

### Non-Goals

* Define specific alert needs
* Implement user-defined alerts

## Proposal

### User Stories

#### Story 1

As an OpenShift developer, I need to ensure that my alerts are appropriately
tuned to allow end-user operational efficiency.

#### Story 2

As an SRE, I need alerts to be informative, actionable and thoroughly
documented.

### Implementation Details/Notes/Constraints [optional]

This enhancement will require some developers and administrators to rethink
their concept of alerts and alert levels.  Many people seem to have an
intuition about how alerts should work that doesn't necessarily match how
alerts actually work or how others believe alerts should work.

Formalizing an alerting specification will allow developers and administrators
to speak a common language and have a common understanding, even if the concepts
seem unintuitive initially.

### Risks and Mitigations

People will make wrong assumptions about alerts and how to handle them.  This
is not unique to alerts, people often believe certain components should behave
in a particular way in which they do not.  This is an artifact of poor
documentation that leaves too much to the imagination.

We can work around this problem with clear and concise documentation.

## Design Details

### Critical Alerts

TL/DR:  For alerting current and impending disaster situations.

Timeline:  ~5 minutes.

Reserve critical level alerts only for reporting conditions that may lead to
loss of data or inability to deliver service for the cluster as a whole.
Failures of most individual components should not trigger critical level alerts,
unless they would result in either of those conditions. Configure critical level
alerts so they fire before the situation becomes irrecoverable. Expect users to
be notified of a critical alert within a short period of time after it fires so
they can respond with corrective action quickly.

Some disaster situations are:
* loss or impending loss of etcd-quorum
* etcd corruption
* inability to route application traffic externally or internally
(data plane disruption)
* inability to start/restart any pod other than capacity.  EG, if the API server
were to restart or need to be rescheduled, would it be able to start again?

In other words, critical alerts are something that require someone to get out
of bed in the middle of the night and fix something **right now** or they will
be faced with a disaster.

An example of something that is NOT a critical alert:
* MCO and/or related components are completely dead/crash looping.

Even though the above is quite significant, there is no risk of immediate loss
of the cluster or running applications.

You might be thinking to yourself: "But my component is **super** important, and
if it's not working, the user can't do X, which user definitely will want."  All
that is probably true, but that doesn't make your alert "critical" it just makes
it worthy of an alert.

The group of critical alerts should be small, very well defined,
highly documented, polished and with a high bar set for entry. This includes a
mandatory review of a proposed critical alert by the Red Hat SRE team.

### Warning Alerts

TL/DR: fix this soon, or some things won't work and upgrades will probably be
blocked.

If your alert does not meet the criteria in "Critical Alerts" above, it belongs
to the warning level or lower.

Use warning level alerts for reporting conditions that may lead to inability to
deliver individual features of the cluster, but not service for the cluster as a
whole. Most alerts are likely to be warnings. Configure warning level alerts so
that they do not fire until components have sufficient time to try to recover
from the interruption automatically. Expect users to be notified of a warning,
but for them not to respond with corrective action immediately.

Timeline:  ~60 minutes

60 minutes?  That seems high!  That's because it is high.  We want to reduce
the noise.  We've done a lot to make clusters and operators auto-heal
themselves.  The whole idea is that if a condition has persisted for more than
60 minutes, it's unlikely to be resolved without intervention any time later.

#### Example 1: All machine-config-daemons are crashlooping

That seems significant
and it probably is, but nobody has to get out of bed to fix this **right now**.
But, what if a machine dies, I won't be able to get a replacement?  Yeah, that
is probably true, but how often does that happen?  Also, that's an entirely
separate set of concerns.

#### Example 2:  I have an operator that needs at least 3 of something to work
* With 2/3 replicas, warning alert after 60M.
* With 1/3 replicas, warning alert after 10M.
* With 0/3 replicas, warning alert after 5M.

Q: How long should we wait until the above conditions rises to a 'critical'
alert?

A: It should never rise to the level of a critical alert, it's not critical.  If
it was, this section would not apply.

#### Example 3:  I have an operator that only needs 1 replica to function

If the cluster can upgrade with only 1 replica, and the the service is
available despite other replicas being unavailable, this can probably
be just an info-level alert.

#### Q: What if a particular condition will block upgrades?

A: It's a warning level alert.


### Alert Ownership

Previously, the bulk of our alerting was handled directly by the monitoring
team.  Initially, this gave use some indication into platform status without
much effort by each component team.  As we mature our alerting system, we should
ensure all teams take ownership of alerts for their individual components.

Going forward, teams will be expected to own alerting rules within their own
repositories.  This will reduce the burden on the monitoring team and better
enable component owners to control their alerts.

Additional responsibilities include:

1. First in line to receive the bug for an alert firing
1. Responsible for describing, "what does it mean" and "what does it do"
1. Responsible for choosing the level of alert
1. Responsible for deciding if the alert even matters


### Documentation Required

1. The name of the alerting rule should clearly identify the component impacted
by the issue (for example etcdInsufficientMembers instead of
InsufficientMembers, MachineConfigDaemonDrainError instead of MCDDrainError).
It should camel case, without whitespace, starting with a capital letter. The
first part of the alert name should be the same for all alerts originating from
the same component.
1. Alerting rules should have a "severity" label whose value is either info,
warning or critical (matching what we have today and staying aside from the
discussion whether we want minor or not).
1. Alerting rules should have a description annotation providing details about
what is happening and how to resolve the issue.
1. Alerting rules should have a summary annotation providing a high-level
description (similar to the first line of a commit message or email subject).
1. If there's a runbook in https://github.com/openshift/runbooks, it should be
linked in the runbook_url annotation.


### Open Questions [optional]

* Explore additional alerting severity (eg, 'minor') for triage purposes

### Test Plan

## Implementation History

## Drawbacks

People might have rules around the existing broken alerts.  They will have to
change some of these rules.

## Alternatives

Document policies, but not make any backwards-incompatible changes to the
existing alerts and only apply the policies to new alerts.

### Graduation Criteria
None

#### Dev Preview -> Tech Preview"
None

#### Tech Preview -> GA"
None

#### Removing a deprecated feature"
None

### Upgrade / Downgrade Strategy"
None

### Version Skew Strategy"
None
