---
title: automated-backups-of-etcd
authors:
  - "@hasbro17"
reviewers:
  - "@deads2k"
  - "@dusk125"
  - "@Elbehery"
  - "@tjungblu"
  - "@williamcaban"
approvers:
  - "@deads2k"
  - "@williamcaban"
api-approvers:
  - "@deads2k"
creation-date: 2023-03-17
last-updated: 2023-03-17
tracking-link:
  - https://issues.redhat.com/browse/ETCD-81
see-also:
  - "https://issues.redhat.com/browse/OCPBU-252"
  - "https://issues.redhat.com/browse/OCPBU-160"
---


# Automated Backups of etcd

## Summary

Enable the automated backups of etcd snapshots and other metadata necessary to restore an openshift cluster from a quorum loss scenario.

## Motivation

The [current documented procedure](https://docs.openshift.com/container-platform/4.12/backup_and_restore/control_plane_backup_and_restore/backing-up-etcd.html) for performing a backup of an OpenShift
cluster is manually initiated. This reduces the likelihood of a timely backup being available during a disaster recovery scenario.

The procedure also requires gaining a root shell on a control plane
node. Shell access to OpenShift control plane nodes access is generally
discouraged due to the potential for affecting the reliability of the
node.

### Goals

- One-time cluster backup can be triggered without requiring a root shell
- Scheduled backups can be configured after cluster installation
- Backups are saved locally on the host filesystem of control-plane nodes
- This feature is validated with an e2e restore test that ensures the backups saved can be used to recover the cluster from a quorum loss scenario


### Non-Goals
- Have automated backups enabled by default with cluster installation
- Save cluster backups to PersistentVolumes or cloud storage
  - This would be a future enhancement or extension to the API
- Automate cluster restoration


### User Stories

- As a cluster administrator I want to initiate a cluster backup without requiring a root shell on a control plane node so as to minimize the risk involved
- As a cluster administrator I want to schedule recurring cluster backups so that I have a recent cluster state to recover from in the event of quorum loss (i.e. losing a majority of control-plane nodes)
- As a cluster administrator I want to have failure to take cluster backups for more than a configurable period to be reported to me via critical alerts


## Proposal

### Workflow Description

To enable automated backups a new singleton CRD can be used to specify the backup schedule and other configuration as well as monitor the status of the backup operations. 

A new controller in the cluster-etcd-operator would then reconcile this CRD to save backups locally per the specified configuration.

### API Extensions

A new CRD `EtcdBackup` will be introduced to the API group `config.openshift.io` with the version `v1alpha1`. The CRD would be feature-gated until the prerequisite e2e test has an acceptable pass rate. 

The spec and status structures will be as listed below:


```Go
// EtcdBackupSpec represents the configuration of the EtcdBackup CRD.
type EtcdBackupSpec struct {

    // Reason defines the reason for the most recent backup.
    // Setting this to a value different from the most recent reason will trigger a one-time backup
    // The default "" means no backup is triggered
    // +kubebuilder:default:=""
    // +optional
    Reason string `json:"reason"`
    
    // Schedule defines the recurring backup schedule in Cron format
      // every 2 hours: 0 */2 * * *
      // every day at 3am: 0 3 * * *
    // Setting to an empty string "" means disabling scheduled backups
    // Default: ""
    // TODO: Define how we do validation for the format (e.g validation webhooks)
    // and the limits on the frequency to disallow unrealistic schedules (e.g */2 * * * * every 2 mins)
    // TODO: What is the behavior if the backup doesn't complete in the specified interval
        // e.g: every 1hr but the backup takes 2hrs to complete
        // Wait for the current one to complete?
        // And do we generate an alert even if MaxDurationToCompleteBackup is not set, since the scheduled interval has passed?
    Schedule string `json:"schedule"`
    
    // MaxDurationToCompleteBackup sets the max duration after which if
    // an initiated backup has not successfully completed a critical alert will be generated.
    // The format is as accepted by time.ParseDuration(), e.g "1h10m"
    // See https://pkg.go.dev/time#ParseDuration
    // Setting to "0" disables the alert.
    // Default: "1h"
    // TODO: Validation on min and max durations.
        // The max should be smaller than the scheduled interval
        // for the next backup else the alert will never be generated
    MaxDurationToCompleteBackup string `json: "maxDurationWithoutBackup"`
    
    // RetentionCount defines the maximum number of backups to retain.
    // If the number of successful backups matches retentionCount 
    // the oldest backup will be removed before a new backup is initiated.
    // The count here is for the total number of backups
    // TODO: Validation on min and max counts.
    // TODO: Retention could also be based on the total size, or time period
        // e.g retain 500mb of backups, or discard backups older than a month
        // If we want to extend this API in the future we should make retention
        // a type so we can add more optional fields later
    RetentionCount int `json: "retentionCount"`
    
}
```

**TODO:** How do we feature gate the entire type?
For a field within a non-gated type it seems to be the tag [`// +openshift:enable:FeatureSets=TechPreviewNoUpgrade`](https://github.com/openshift/api/blob/master/example/v1/types_stable.go#L33). No such tag visible on the [whole type gated example](https://github.com/openshift/api/blob/1957a8d7445bf2332f027f93a24d7573f77a0dc0/example/v1alpha1/types_notstable.go#L19).


```Go
// EtcdBackupStatus represents the status of the EtcdBackup CRD.
type EtcdBackupStatus struct {

    // Conditions represents the observations of the EtcdBackup's current state.
    // TODO: Identify different condition types/reasons that will be needed.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    
    // ObservedGeneration is the most recent generation observed for this
    // EtcdBackup. It corresponds to the EtcdBackup's .metadata.generation that the condition was set based upon.
    // This lets us know whether the latest condition is stale.
    // +optional
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`

    // TODO: How do we track the state of the current/last backup?
        // Would condition states be enough? e.g Created/In-progress/Complete/Failed/Unknown
    // Track metadata on last successful backup?
        // node name, path, size, timestamp
    // With backup retention do we also track the last backup removed?
}
```


### Implementation Details/Notes/Constraints [optional]

#### Backup controller

A backup controller in the cluster-etcd-operator will reconcile the CRD to execute the backup schedule and report the status of ongoing backups and failures to the `EtcdBackup` status.

There are different options to explore on how we want to execute saving the backup snapshot and any other required metadata. As well as how we enforce the schedule.

#### Execution method

Create a backup pod that runs the backup script on a designated master node to save the backup file on the node's host filesystem:
- The CEO already has an existing [upgradebackupcontroller][upgradebackupcontroller] that deploys a [backup pod][backup pod] that runs the [backup script][backup script].
  - It may be simpler to reuse that but we may need to modify the backup script to save or append additional metadata.
  - Making changes to a bash script without unit tests would make it harder to maintain.

As an alternative we could implement an equivalent Go cmd for the backup pod to execute a variant of the backup script.
- The etcd Go client provides a [`snapshot save()`](https://pkg.go.dev/go.etcd.io/etcd/client/v3@v3.5.7/snapshot) util that can be used for this method.
- We would be maintaining two potentially different backup procedures this way. The backup script won't be replaced by the `EtcdBackup` API since it can still be used in a quorum-loss scenario where the API server is down.

Since it is an implementation detail with no API impact we can start with utilizing the backup script for simplicity and codify it later.

#### Backup schedule

To enforce the specified schedule we can utilize [CronJobs](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) to run backup pods.

**TODO:** If load balancing between control-plane nodes is a requirement then it needs to be determined how we can achieve that with the Job or Pod spec.
- The [`nodeSelector`](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector) field would let us choose the group of control-plane/master nodes but not round-robin between them.
- The [`nodeName`](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodename) field would let us choose the node
  - More brittle as node names can change and would require writing custom scheduling logic to load balance across nodes

See the Open Questions section for load balancing concerns.

#### Saving the backup

The backup pod needs to be able to save the backup data onto the host filesystem. To achieve this the backup pod can be mounted with a [hostPath Volume](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath).

**TODO:** Does anything prevent us from doing this e.g authorization or security issues.

#### Alerting

TBD

### Risks and Mitigations

TBD

## Design Details

### Open Questions

### Load balancing and IO impact
- What would be the IO impact or additional load caused by a recurring execution of the backup pod and taking the snapshot?
  - If significant, should we avoid selecting the leader to reduce the risk of overloading the leader and triggering a leader election?
  - We can find the leader via the [endpoint status](https://etcd.io/docs/v3.5/dev-guide/api_reference_v3/#message-statusresponse-apietcdserverpbrpcproto) but the leader can change during the course of the backup or we may not have the option of filtering the leader out if the backup pod is unschedulable on non-leader nodes (e.g low disk space). 
- Other than the leader IO considerations, when picking where to run and save the backup, do we need to load balance or round-robin across all control-plane nodes?
  - Leaving it to the scheduler to schedule the backup pod (or having the operator save on the node it's running on) is simpler, but we could end up with all/most backups being saved on a single node. In a disaster recovery scenario we could lose that node and the backups saved.
  - Could be a future improvement if we start with the simpler choice of leaving it to the scheduler.

### Test Plan

Prior to merging the backup controller implementation, an e2e test would be added to openshift/origin to test the validity of the backups generated from the automated backups feature. The pass rate on this test will feature gate the API until we pass an acceptable threshold to make it available by default.

This test would effectively run through the backup and restore procedure as follows:
- Start with a cluster that has the `EtcdBackups` API enabled
- Save a backup of the cluster by triggering a one time backup using the `EtcdBackups` API, or create a schedule to do that
- Modify the cluster state post backup (e.g create pods, endpoints etc) so we can later ensure that restoring from backup does not include this additional state
- Induce a disaster recovery scenario of 2/3 nodes lost with etcd quorum loss
- Step through the recovery procedure from the saved backup
- Ensure that the control-plane has recovered and is stable
- Validate that we have restored the cluster to the previous state so that it does not have the post-backup changes

See the [restore test design doc](https://docs.google.com/document/d/1NkdOwo53mkNBCktV5tkUnbM4vi7bG4fO5rwMR0wGSw8/edit?usp=sharing) for a more detailed breakdown on the restore test and the validation requirements.

Along with the e2e restore test, comprehensive unit testing of the backup controller will also be added to ensure the correct reconciliation and updates of the:
- Backup schedule
- Backup retention
- Backup status
- Alerting rules

### Graduation Criteria

**TODO:** Need to clarify how feature gating relates to dev preview -> tech preview -> GA.

#### Dev Preview -> Tech Preview

- TBD

#### Tech Preview -> GA

- TBD

### Upgrade / Downgrade Strategy

- TBD

### Version Skew Strategy

TBD

### Operational Aspects of API Extensions

TBD

**TODO:** Mention where the conditions for the `EtcdBackups` controller being degraded would be set. Most likely on the `etcd.operator.openshift.io`  `Etcd/cluster` CRD where all the other CEO controllers set their degraded status.

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

TBD

## Alternatives

TBD

## Infrastructure Needed

The new API will be introduced in the `openshift/api` repo and the controller will be added to the existing cluster-etcd-operator in the `openshift/cluster-etcd-operator` repo.


[upgradebackupcontroller]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/pkg/operator/upgradebackupcontroller/upgradebackupcontroller.go#L284-L298
[backup pod]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/bindata/etcd/cluster-backup-pod.yaml#L31-L38
[backup script]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/bindata/etcd/cluster-backup.sh#L121-L129