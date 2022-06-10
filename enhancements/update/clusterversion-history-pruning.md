---
title: clusterversion-history-pruning
authors:
  - "@jottofar"
reviewers:
  - "@LalatenduMohanty"
  - "@wking"
approvers:
  - "@sdodson"
api-approvers:
  - N/A
creation-date: 2022-06-10
last-updated: 2022-06-10
tracking-link:
  - https://issues.redhat.com/browse/OTA-664
---

# ClusterVersion History Pruning

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] Operational readiness criteria is defined
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

CVO currently maintains a 50 entry queue of installed versions.
Once the queue is full CVO simply removes the first added entry.
The intent of the pruner as modified by this [commit][commit] was to remove the first added partial update entry, if one exists, otherwise to simply remove the first entry.
Even once [rhbz#2097067][rhbz-2097067] is fixed this can result in the removal of useful cluster version information such as the initial installed version.
Because some things such as install-time infrastructure depend on the initial version, we want to preserve that entry.
There are additional history entries which should be given preference when determining which entry to remove.
This enhancement defines a more intelligent history pruning algorithm with the goal of keeping the more informative history entries, as defined below.
In addition, the history queue will be expanded to 100 entries.

## Motivation

ClusterVersion history can be helpful in debugging and correcting cluster issues as well as simply showing a cluster's version history.
But certain history entries are more helpful than others such as the cluster's initial version, first minor update, and completed rather than partial version updates.
Long lived clusters can have a history that contain very little of the aforementioned helpful entries.

### User Stories

* As a OCP product team member, I want a cluster's ClusterVersion history to retain entries deemed to be the most helpful in identifying and resolving cluster issues.

* As a cluster administrator, I want a cluster's ClusterVersion history to retain entries deemed to be the most helpful in identifying and resolving cluster issues.

* As a cluster administrator, I want a cluster's ClusterVersion history to cover the cluster's entire lifespan.

### Goals

* In generally descending priority, history pruning algorithm will prefer:
  * initial versions and final version
  * complete over partially installed versions
  * first and last minor versions
  * more recent entries
* Preserve useful version history rather than simply preserving more version history.
* Structure pruning algorithm to allow easy tuning/tweaking.
* Implementation will require no API changes.

### Non-Goals

* ClusterVersion history will contain no explicit indication that it has been pruned however pruning details will be logged.

## Proposal

### Workflow Description

When a new entry is to be added to the history queue and the queue contains 100 entries, the new entry and each existing history entry will be ranked:
```go
rank =
  1000 * (isTheInitialEntry or isAFinalEntry or isTheMostRecentCompletedEntry)
  + 30 * (isTheFirstCompletedInAMinor or isTheLastCompletedInAMinor)
  + 20 * (isPartialPortionOfMinorTransition)
  - 20 * (isPartialWithinAZStream)
  - 1.01 * sliceIndex
```
The lowest ranked entry will be removed, or, in the case of the new entry not added, maintaining the queue size of 100 entries.
The removal including details of why an entry was chosen for removal will be logged.

The weights used to rank a given entry are based on the following criteria:
* `1000` - considered most important to keep
* `30` - interesting but not critical  
* `20` - partial stops during a minor transition are interesting, but not as interesting as the completed versions that bookend the minor transition
* `-20` - partials between completed releases within the same minor, not very useful
* `-1.01` - prefer more recent entries, avoid ties

`isAFinalEntry` will be true for the 5 most recent history entries.

Prototyping against ClusterVersion histories from a variety of actual customer clusters will help determine the correct criteria and corresponding weights that result in the most useful history.

Existing `godocs` already explain that ClusterVersion history may be missing entries (has been pruned).

### API Extensions

N/A - no api extensions are being introduced in this proposal

### Risks and Mitigations

* Useful ClusterVersion history could be removed.
This is also a risk if we do nothing.
By defining a pruning algorithm we at least have some control over what is removed.

### Drawbacks

* It is possible that the current pruning method, which simply retains the most recent 50 ClusterVersion history entries, would result in an overall history that is more useful for identifying and resolving a given issue.
However in the vast majority of instances this is not expected.

## Design Details

### Test Plan

* CVO unit tests will be expanded and/or created to test the new logic.

### Graduation Criteria

GA. When it works, we ship it.

#### Dev Preview -> Tech Preview

N/A. This is not expected to be released as Dev Preview.

#### Tech Preview -> GA

N/A. This is not expected to be released as Tech Preview.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

No special consideration.

### Version Skew Strategy

No special consideration.

### Operational Aspects of API Extensions

N/A - no api extensions are being introduced in this proposal

#### Failure Modes

No special consideration.

#### Support Procedures

No special consideration.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Alternatives

### Significantly enlarge ClusterVersion history queue

The ClusterVersion history queue could be enlarged to a point where it is hoped that for a majority of clusters, all, or a significant amount, of the cluster's history can be retained.

* No in-depth analysis has been done to determine the required queue size.
* This would increase memory usage.
* There are limits on resource size in both `etcd` and `kubernetes`.
* For most of the long-lived clusters, which are typically the ones where the history could be the most useful, a very large history could be unwieldy.

[commit]: https://github.com/openshift/cluster-version-operator/commit/6971c2bce79327fd51045fe6726c5d9c4524aaed
[rhbz-2097067]: https://bugzilla.redhat.com/show_bug.cgi?id=2097067
