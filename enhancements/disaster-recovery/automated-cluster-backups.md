---
title: automated-cluster-backups
authors:
  - "@marun"
reviewers:
  - "@hexfusion"
  - "@lilic"
  - "@deads2k"
  - "@smarterclayton"
approvers:
  - "@hexfusion"
  - "@lilic"
  - "@deads2k"
  - "@smarterclayton"
creation-date: 2021-07-08
last-updated: 2021-07-08
status: implementable
see-also:
  - "https://docs.google.com/document/d/1LyZQMgLvgd81iQndcDcjAVN25ZEhCxVGMttoiRkSU6g/edit#"
  - "https://docs.openshift.com/container-platform/4.7/backup_and_restore/backing-up-etcd.html"
  - "https://rancher.com/docs/rancher/v2.x/en/backups/v2.5/configuration/backup-config/"
  - "https://www.openshift.com/blog/ocp-disaster-recovery-part-1-how-to-create-automated-etcd-backup-in-openshift-4.x"
---

# Automated Cluster Backups

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Enable the configuration of automated cluster backups.

## Motivation

The current documented procedure for performing a backup of an OpenShift
cluster is manually initiated. This reduces the likelyhood of a timely backup
being available in the event of catastrophic cluster failure.

The procedure also requires gaining a root shell on a control plane
node. Shell access to OpenShift control plane nodes access is generally
discouraged due to the potential for affecting the reliability of the
node. Indeed, at least one customer reports having had operator error result
in a control plane node being taken offline in the process of attempting to
follow the documented backup procedure.

Finally, OpenShift 4.9 is intended to ship with etcd 3.5, and this will mean
an upgrade for existing clusters from etcd 3.4. Since etcd will not support
downgrade from 3.5 to 3.4, downgrade of a OpenShift cluster from 4.9 to 4.8
will only be possible by restoring from backup. Ensuring support for
automated backups in the 4.8 edge required to upgrade to 4.9 will increase
the likelyhood that customers will have cluster backups available in case a
downgrade is required.


### Goals

1. One-time cluster backup can be initiated without requiring a root shell.
1. Automated backups can be configured as a day 2 operation.
1. Cluster backups are written to the host filesystem of control plane nodes.

### Non-Goals

1. Backup to cluster (i.e. PVC) or cloud storage (e.g. S3).
1. Support day 1 configuration of automated backups.
1. Automate cluster restoration.
1. Rewrite the existing cluster backup script.

## Proposal

- Add a new API singleton supporting backup configuration
  - Suggested resource name `backups.config.openshift.io`
    - Separate backup configuration from the etcd operator in case
      implementation migrates to a dedicated backup operator.
  - Fields:
    - spec.reason `string`
      - Set to a value different from the most recent reason, will prompt a
        one-time backup.
      - Default: `""`
    - spec.schedule `string`
      - [Standard cron expression](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/#cron-schedule-syntax)
      - Results in backups scheduled repeatedly to successive control plane
        nodes.
      - Default: `""`
    - spec.maxDurationWithoutBackup `duration`
      - If a backup has not successfully completed in the configured
        interval, a critical alert will be generated for this and every
        subsequent interval. The alert will either indicate that backups are
        not configured, or supply the most recent reason a backup failed.
      - Setting to `0` will disable alerting.
      - Default: `1h`
    - spec.retentionCount `int`
      - The maximum number of backups to retain.
      - If the number of succesful backups (as determined by recorded
        status) matches `retentionCount`, the oldest backup will removed
        before a new backup is initiated.
      - Default: 15

- TODO Need a way to track backups once initiated
  - New API type?
  - Metadata to store
    - openshift version
    - node the backup was performed on
      - Would also be where the backup was stored
    - path the backup was written to
    - time
    - size
    - success/failure
      - failure reason
  - Stored as spec or status?
  - Remove backup api resources as part of removing backups


- Implement a new controller that responds to the backup configuration
  - The new controller will initially be shipped with the etcd operator for simplicity
  - If `maxDurationWithoutBackup` is non-zero and is exceeded without a
    successfull backup, create a critical alert. This is intended to ensure
    that cluster admins are encouraged to configure automated backups if only
    to disable the alerting.
  - The cost of taking automated backups should be spread over all the
    control plane nodes to minimize the IO impact on any one node.
    - Spreading backups across available nodes has the added benefit of
      maximizing the chances of retaining a viable backup should one or more
      control plane nodes be lost.

- If a backup is configured to occur due to `backupReason` or `schedule`:
  - Given the set of control plane nodes
  - Given the set of the successful backups recorded in status
  - Only a single backup pod should be running at a given time. If a backup
    pod is still running:
    - If the pod has been running longer than a maximum period (TODO How
      long?), terminate it and record the backup as having failed.
    - If the pod has been running for less than the maximum period, defer the
      scheduled backup for (TODO How long)
  - If the number of successful backups matches `retentionCount`
    - Ensure removal of the oldest backup from the node that it was written to
    - Remove record of removed backup from the api?
    - Log removal of the backup
  - Find the set of candidate nodes that have fewer backups than other nodes
    - Could be 1 node (e.g. 2 nodes have 1 backup and 1 node has zero)
    - Could be a subset of nodes (e.g. 1 node has 1 backup and 2 nodes have
      zero)
    - Could be all nodes (i.e. no successful backups have been taken)
  - Schedule a backup pod to a random member of the set of candidate nodes
    - A backup pod should invoke the `cluster-backup.sh` script on the host
      via chroot and write the backup data to the host.
    - A backup pod should first check that the available disk space on the
      node it is running on is a multiple of the size of the node's
      /var/lib/etcd path. A node needs a minimum amount of available storage
      to operate reliably and automated backup must make every effort not to
      exhaust a node's available storage.
  - When a backup pod terminates, record the succcess or failure to a backup
    resource

### User Stories

- I want to be alerted after installation or upgrade to the availability of
  an automated backup capability and be given an indication of the importance
  of configuring it in a timely manner.

- I want to initiate a cluster backup without requiring a root shell on a
  control plane node so as to minimize the risk involved.

- I want to schedule recurring cluster backups so that I have recent cluster
  state to recover with in the event of disaster (i.e. losing 2 control plane
  nodes).

- I want to have failure to take cluster backups for more than a configurable
  period to be reported to me via critical alerts.

- I want to minimize the operational impact of taking regular backups.

### Risks and Mitigations

Spreading the spread the cost of automated backups over all control plane
nodes may not be sufficient to avoid negatively impacting cluster performance
on IO-constrained hosts. Ideally it would be possible to limit the IO
overhead of taking a backup via cgroup limits, but it's not clear that this
would be achievable without the granularity provided by cgroups v2 (not
slated for GA until RHEL9).

Automated backups relying on host storage could fill up the disks of control
plane nodes, rendering them inoperable. This is intended to be mitigated by a
check for available disk space that ensures backups are only initiated on
nodes that have a multiple of the space required to backup the node's
/var/lib/etcd path.

Relying on node storage may hamper recovery if all control plane nodes are
lost. It might make sense to document the requirement to periodically ship
backup data off the cluster. Given that the current restore procedure
requires at least one control plane node, though, a cluster is only
recoverable from backup if at least one of the backup-containing control
plane nodes is available.

## Design Details

### Open Questions

- Under what circumstances should backup pods that do not terminate
  successfully or do not terminate within a reasonable period of time be
  recreated?

- If the node targeted for backup does not have sufficient space, should
  there be an attempt to take a backup on a different node? This question is
  complicated by spreading backups across available nodes since backing up to
  a different node may require removing a backup that is not the oldest.

### Test Plan

- Comprehensive unit testing of
  - Changes in backup configuration
  - Scheduling logic
  - Backup retention
  - Pre-backup check for disk space available

- Update DR e2e testing to initiate one-time backups via the controller

- Add new e2e golden-path testing of scheduled backups

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

Given that this feature is strictly additive and is not dependent or depended
upon by other components, no special consideration need be given to handling
upgrades and downgrades.

### Version Skew Strategy

Given that this feature is strictly additive and is not dependent or depended
upon by other components, no special consideration need be given to handling
version skew.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Q: The built-in
  [CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/)
  feature would appear to be duplicated by this feature. Why not just
  implement on top of `CronJob`?

A: It is desirable to spread backups across control plane nodes to minimize
   the io impact on any one node and to maximize the potential for a given
   control plane node to have a recent backup to restore from. It does not
   appear to be possible to layer the scheduling logic required to achieve
   these capabilities on top of `CronJob` scheduling.

Q: Why not schedule backups to nodes with the built-in
   [Job](https://kubernetes.io/docs/concepts/workloads/controllers/job/)
   feature instead of creating pods directly?

A: `Job` has the ability to reschedule a pod to another node if that node
   becomes unavailable (e.g. due to failure or reboot). This is not desirable
   in the context of an attempt to backup a cluster since nodes are not
   fungeable. A node targeted for backup is chosen to spread both IO impact
   and backup availability across available nodes, and `Job` has no way of
   taking these considerations into account.
