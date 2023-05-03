---
title: periodic-etcd-defragmentation-in-microshift
authors:
  - dusk125
reviewers:
  - "@hasbro17, etcd Team"
  - "@tjungblu, etcd Team"
  - "@Elbehery, etcd Team"
  - "@fzdarsky"
  - "@deads2k"
  - "@derekwaynecarr"
  - "@mangelajo"
  - "@pmtk"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2023-02-15
last-updated: 2023-03-03
tracking-link:
  - https://issues.redhat.com/browse/ETCD-391
---

# Periodic etcd Defragmentation in MicroShift

## Summary

This enhancement proposes adding a control loop to automatically and periodically run the etcd defragment commands.

## Motivation

Similar to a spinning-platter harddrive, etcd, through subsequent object creates and deletes, will have its internal database become fragmented. This causes etcd to use more space in memory and on disk than it actually has "in-use".
Periodic defragmentation (along with compacation invoked by the API Server) is the way to ensure that etcd does not run out of space (hit the maximum-allowed database size of 8GB).

Etcd running in the Openshift Container Platform (OCP) is automatically and periodically defragemented by the Cluster Etcd Operator (CEO). Because operators in Microshift are to be avoided, an alternative control loop for etcd defragmentation needs to be selected.

### User Stories

1. "As a Microshift Device Administrator, I want etcd to automatically defragment itself so that it doesn't run out of space, or use more space than it needs."

### Goals

- Etcd is automatically and periodically defragmented by a control loop that fits into the Microshift paradigm.
- Allow the user to configure the conditions for defragmenting.

### Non-Goals

- Replicate other aspects/control loops of the OCP CEO.
- Include etcdctl to the deployment of Microshift.

## Proposal

All the following solutions will periodically calculate the "fragmented percentage" (db size on disk vs db in use). The frequency of this calculation and the threshold which the defragmentation occurs could be configurable (with defaults taken from OCP) by a Microshift Administrator so they can control when the defragmentation occurs.
This configuration should also have the option to turn defragmentation completely off so that if an Administrator wants to opt-out of the potential for short disruptions, they have the ability to do so with the knowledge that etcd could run out of space during execution.
For short-lived, light workload clusters, turning it off completely is likely not to cause problems.

There is also the option of running defragmentation at Microshift startup (after etcd is alive and well); however, we should still have the inflight threshold detection as that could help us avoid an out of space issue while the cluster is running.

### Defragmentation from a goroutine in Microshift
This solution would launch a goroutine, inside the Microshift binary, that would wait for the condition(s) to run defragmentation.
This would allow for the logs/status of the defragmentation to appear in the same log stream as the other Microshift logs so and Administrator could see its status and if the defragmentation failed in the same place as they would be monitoring for other Microshift issues/status.

This approach would easily afford the aforementioned periodic/threshold dual condition for defragmentation. It checks the conditions for defragmentation on a configurable frequency, and if both conditions - maximum fragmented percentage and minimum database size - meet or exceed their thresholds, a defragmentation will be launched.

### Workflow Description
For the end user, any change we make should be completely transparent. Defragmentation is only a problem when it isn't set up (or it fails frequently) and etcd runs out of resources, causing a disruption.
Also, the end user should be able to control how often, if at all, defragmentation runs. They can do this by updating the Microshift configuration to change the fragmentation percentage threshold, minimum database size threholds, and condition check frequency.

For a Microshift Administrator, they should be able to easily know when a defragmentation is occuring/occured and if it has failed.

#### Variation [optional]
TODO

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

TODO

### Risks and Mitigations

Some methods - cron-job and systemd unit - could potentially fail silently (or at least force monitoring of a non-Microshift location: cron logs/journalctl) and lead the defragmentation to not execute, which could allow etcd to grow and hit its quota which would cause further writes to fail.
The gorountine might be superior in this case as it could send logs to the same stream as normal Microshift, which would be more likely noticed by an Administrator.
To mitigate the above issue, we could have the systemd unit's logs streamed into the Microshift log stream; it would essentially be a subprocess of Microshift.

### Drawbacks

Regardless of which method is chosen, during the duration of the defragmentation, etcd will be unavailable for writes to the database; clients may still read data, but writes will be denied until the defragmentation is finished. Given the size of Microshift, this hold should not be very long, likely at most single seconds; this would be a good thing to test to ensure the disruption duration is acceptable.


## Design Details

### Open Questions [optional]

- Do we even need/want automatic defrag?
  - Since these are small clusters, is the short hold on writes (while the defrag takes place) acceptable for this application.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Since this isn't really a functional change to Microshift or etcd themselves, the only additional test would be ensuring that the duration for defragmentation is within an acceptable limit for Microshift.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TODO

#### Dev Preview -> Tech Preview

TODO

#### Tech Preview -> GA

TODO

#### Removing a deprecated feature

TODO

### Upgrade / Downgrade Strategy

TODO

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

TODO

#### Support Procedures

TODO

## Implementation History

TODO

## Alternatives

### Defragmentation from cron
This approach would have the defragmentation condition(s) checked via a cron-job. When Microshift is installed, the cron-job(s) could be added to the tab and the system would handle the timing of the pure periodic and/or the threshold calculation.

There are a few issues with this approach:
1. It relies on cron being installed on the system and running.
2. If Microshift is not running, the cronjob might still fire and would fail because it would be able to reach etcd which would cause a lot of false-positive failures to appear in the log.
3. If we wanted to cover both approaches (pure periodic and threshold), then we would likely need two different cronjobs added to the tab during install.

### Defragmentation from transient systemd unit
This would be similar to the work to launch etcd itself as a transient systemd unit. A new sub-command could be added to Microshift (or microshift-etcd) that would do much the same as the goroutine approach above: two timeouts in a loop (assuming doing both pure periodic and threshold) that will launch the defragmentation.
Its runtime would be attached to that of Microshift and, like the goroutine, it would be easy to detect when/if a defragmentation was running or when/if it failed.

This approach would be the most like an operator in OCP.

## Infrastructure Needed [optional]

TODO
