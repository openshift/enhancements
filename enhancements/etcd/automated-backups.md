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

Enable the automated backups of etcd snapshots and other metadata necessary to restore an OpenShift self-hosted cluster from a quorum loss scenario.

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
  - Not targeted for a future enhancement at this time
- Provide automated backups for non-self hosted architectures like Hypershift


### User Stories

- As a cluster administrator I want to initiate a one-time cluster backup without requiring a root shell on a control plane node so as to minimize the risk involved
- As a cluster administrator I want to schedule recurring cluster backups so that I have a recent cluster state to recover from in the event of quorum loss (i.e. losing a majority of control-plane nodes)
- As a cluster administrator I want to have failure to take cluster backups for more than a configurable period to be reported to me via critical alerts


## Proposal

### Workflow Description

#### One time backups

To enable one-time backup requests via an API, a new CRD, `EtcdBackupRequest` (open to better name suggestions), will be used to trigger one-time backup requests.

A new controller in the cluster-etcd-operator, `EtcdBackupRequestController`, will reconcile backup requests as follows:
- Watch for new `EtcdBackupRequest` CRs as created by an admin
- Create a backup Job configured for the backup request spec
- Track the backup progress, failure or success on the `EtcdBackupRequest` status

**TODO:** Decide if `EtcdBackupRequest` needs to be cluster-scoped?

#### Scheduled backups

To enable recurring backups a new cluster-scoped singleton CRD `EtcdBackupSchedule` (open to better name suggestions) will be used to specify the backup schedule, retention policy and other related configuration.

A new controller in the cluster-etcd-operator `EtcdBackupScheduleController` would then reconcile the `EtcdBackupSchedule` CRD with the following workflow:
- Watches the `EtcdBackupSchedule` CR as created by an admin
- Creates a [CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) that would in turn create `EtcdBackupRequest` CRs at the desired schedule
- Updates the CronJob for any changes in the schedule
- Fulfils the specified retention policy by pruning existing backup files before creating a new `EtcdBackupRequest`
- Prunes completed `EtcdBackupRequest` CRs older than a default time period e.g 24hrs
  - **TODO:** Should this period be configurable?


### API Extensions

#### EtcdBackupSchedule API

The `EtcdBackupSchedule` CRD will be introduced to the API group `config.openshift.io` with the version `v1alpha1`. The CRD would be feature-gated with the annotation `release.openshift.io/feature-set: TechPreviewNoUpgrade` until the prerequisite e2e test has an acceptable pass rate. See the Test Plan and Graduation Criteria sections for more details.

The spec will be as listed below, while the status will be empty since the status of individual backups will be tracked on the `EtcdBackupRequest` status block.

**TODO**: While config CRs don't normally seem to have a status block, where do we track the cumulative status e.g total number of backups saved?

```Go
// EtcdBackupScheduleSpec represents the configuration of the EtcdBackupSchedule CRD.
type EtcdBackupScheduleSpec struct {
    
    // Schedule defines the recurring backup schedule in Cron format
      // every 2 hours: 0 */2 * * *
      // every day at 3am: 0 3 * * *
    // Setting to an empty string "" means disabling scheduled backups
    // Default: ""
    // TODO: Define CEL validation for the cron format
    // and the limits on the frequency to disallow unrealistic schedules (e.g */2 * * * * every 2 mins)
    // TODO: Discuss in multiple backups section. What is the behavior if the backup doesn't complete in the specified interval
        // e.g: every 1hr but the backup takes 2hrs to complete
        // Wait for the current one to complete?
    Schedule string `json:"schedule"`
    
    // RetentionPolicy defines the retention policy for retaining and deleting existing backups.
    // +optional
    RetentionPolicy RetentionPolicy `json: "retentionPolicy"`
}

// RetentionPolicy defines the retention policy for retaining and deleting existing backups.
// This struct is a discriminated union that allows users to select the type of retention policy from the supported types.
// +union
type RetentionPolicy struct {
  // RetentionType sets the type of retention policy. The currently supported and valid values are "retentionCount"
	// Currently, the only valid policies are retention by count (RetentionCount) and by size (RetentionSizeGb). More policies or types may be added in the future.
	// +unionDiscriminator
	// +kubebuilder:validation:Required
  // +kubebuilder:validation:Enum:=RetentionCount;RetentionSizeGb
	RetentionType RetentionType `json:"retentionType"`
	
  // RetentionCount defines the maximum number of backups to retain.
  // If the number of successful backups matches retentionCount 
  // the oldest backup will be removed before a new backup is initiated.
  // The count here is for the total number of backups
  // +kubebuilder:validation:Minimum=1
  // +optional
  RetentionCount int `json: "retentionCount,omitempty"`

  // RetentionSizeGb defines the total size in Gb of backups to retain.
  // If the current total size backups exceeds RetentionSizeGb then 
  // the oldest backup will be removed before a new backup is initiated.
  // +kubebuilder:validation:Minimum=0.1
  // +optional
  RetentionSizeGb float64 `json: "retentionSizeGb,omitempty"`

  // TODO: Can the union members be unspecified, and if so what would they default to?
  // Add // +kubebuilder:default=<value>
	
}

// RetentionType is the enumeration of valid retention policy types
// +enum
// +kubebuilder:validation:Enum:="RetentionCount";"RetentionSizeGb"
type RetentionType string

const (
  RetentionTypeCount RetentionType = "RetentionCount"
  RetentionTypeSize RetentionType = "RetentionSizeGb"
)

```

#### EtcdBackupRequest API

The `EtcdBackupRequest` CRD will be introduced to the API group `backup.etcd.openshift.io` with the version `v1alpha1`. The CRD would be feature-gated with the annotation `release.openshift.io/feature-set: TechPreviewNoUpgrade` until the prerequisite e2e test has an acceptable pass rate.

The spec and status will be as listed below:

```Go
// EtcdBackupRequestSpec represents the configuration of the EtcdBackupRequest CRD.
type EtcdBackupRequestSpec struct {

    // Reason defines the reason for the most recent backup.
    // TODO: Do we need to specify a reason? This is more metadata than spec to reconcile.
    // We could just have a spec-less CR as created by the admin or the EtcdBackupSchedule per the schedule.
    // The reason could be tagged into an annotation if needed.
    Reason string `json:"reason"`

}

// EtcdBackupRequestStatus represents the status of the EtcdBackupRequest CRD.
type EtcdBackupRequestStatus struct {

    // Conditions represents the observations of the EtcdBackupRequest's current state.
    // TODO: Identify different condition types/reasons that will be needed.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    

    // TODO: How do we track the state of the current/last backup?
        // Would condition states be enough? e.g Created/In-progress/Complete/Failed/Unknown
    // Track metadata on last successful backup?
        // node name, path, size, timestamp

}
```


### Implementation Details/Notes/Constraints [optional]

There are different options to explore on how we want to execute saving the backup snapshot and any other required metadata. As well as how we enforce the schedule and retention policy.


#### Executing the backup cmd

Create a backup Job that runs the the backup script on a designated master node to save the backup file on the node's host filesystem:
- The CEO already has an existing [upgradebackupcontroller][upgradebackupcontroller] that deploys a [backup pod][backup pod] that runs the [backup script][backup script].
  - It may be simpler to reuse that but we may need to modify the backup script to save or append additional metadata.
  - Making changes to a bash script without unit tests would make it harder to maintain.

As an alternative we could implement an equivalent Go cmd for the backup pod to execute a variant of the backup script.
- The etcd Go client provides a [`snapshot save()`](https://pkg.go.dev/go.etcd.io/etcd/client/v3@v3.5.7/snapshot) util that can be used for this method.
- We would be maintaining two potentially different backup procedures this way. The backup script won't be replaced by the `EtcdBackup` API since it can still be used in a quorum-loss scenario where the API server is down.

Since it is an implementation detail with no API impact we can start with utilizing the backup script for simplicity and codify it later.

#### Saving the backup and retention

When deciding how to save the backup files on the control-plane host machine(s) there are two different approaches with different impacts on the visibility of backups, availability, and maintenance burdens of the retention policies.

### Saving across all nodes

One method is to spread out the saved backups on all available control-plane nodes. This would involve:
- Scanning all control-plane nodes for existing backup files
- Pruning the backup file from the node with the oldest backup
- Saving a new backup on the node that has the oldest most-recent-backup

E.g given the following nodes N(x) with backups B(t) where x is the node suffix and t is the backup timestamp:
- N1[B1, B5], N2[B2, B4], N3[B3, B6]
- We would select N1 to prune since B1 is the oldest of (B1,B2,B3)
- We would select N2 as the node to save the latest backup on since B4 is the oldest of (B4,B5,B6).

This approach has the benefit of spreading out backups across nodes which reduces the likelihood of losing the most recent backup in the event of losing access to a node. However it also has the following drawbacks:

- Gathering the state of the backup files is more complicated:
  - The backup controller could serially schedule a pod on each node to try and gather the backup state. It's unclear how the controller would gather that state (e.g have each pod write to a configmap).
- Serially scheduling on each node to gather the state increases the time to perform a backup
- If a node is unhealthy and is unschedulable then the backup may be stalled as we need to gather the state by scheduling on all nodes
- Providing visibility into the state of the backup files for the admin is non-trivial:
  - We would require a status object that lists all nodes and the ordered list of backup filenames per node.

### Saving to a local PV

Alternatively the backup files can be saved at a single location on cluster on a [local Persistent Volume](https://kubernetes.io/docs/concepts/storage/volumes/#local). Saving to a single volume for local storage has the following pros as compared to distributing backups across the control-plane nodes:

- Simplifies the retention logic for locally stored backups. This reduces the maintenance burden of having different retention strategies for locally saved backups vs off-cluster (e.g nfs or gcePersistentDisk volumes).
- Using a PV also grants the flexibility to do lifecycle management of backups e.g what to do with the old backups if a new schedule is created for a different volume.
- The saved backups state is more visible from a single location by looking at the PV contents vs looking at all nodes to piece the order of saved backups.
- For local backups if the cluster is unhealthy the automated backups won't be stalled:
  - If another node besides the one with the local volume is unavailable we can still save the backup.
  - If that node with the local volume is inaccessible, we can still provision a new volume on another available node and update the spec to save backups to the new location. 
- Can leverage [volume snapshots](https://kubernetes.io/docs/concepts/storage/volume-snapshots/) or volume cloning to improve availability of saved backups.

**TODO:** If we go this route, outline the workflow of statically provisioning a local volume, a storage class, and how to specify the PVC in the EtcdBackup/EtcdSchedule spec to use that location for saving backups.

#### Multiple Backup Requests

**TODO:** In the presence of multiple backup requests `EtcdBackupRequest`, specify how we want to update all but the newest backup request as aborted/won't update.

#### Backup schedule

To enforce the specified schedule we can utilize [CronJobs](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) to run backup pods.

**TODO:** If load balancing between control-plane nodes is a requirement then it needs to be determined how we can achieve that with the Job or Pod spec.
- The [`nodeSelector`](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector) field would let us choose the group of control-plane/master nodes but not round-robin between them.
- The [`nodeName`](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodename) field would let us choose the node
  - More brittle as node names can change and would require writing custom scheduling logic to load balance across nodes

See the Open Questions section for load balancing concerns.



#### Failure to backup and alerting

If a backup job fails to complete after a default timeout interval e.g 5 mins the backup controller should set a `BackupControllerDegraded` condition on the `etcds.operator.openshift.io/v1` `cluster` CR. 
The time to complete a backup should typically be in the range of 30s ( see [upstream FAQ on snapshot timeout](https://etcd.io/docs/v3.5/faq/#what-does-the-etcd-warning-snapshotting-is-taking-more-than-x-seconds-to-finish--mean)). However depending on the size of the data and the hardware this may take longer.

**TODO:** Should we provide an operator flag for overriding the default backup timeout for large or slow clusters?

**TODO:** Specify the custom alerting rule for generating an alert from the `BackupControllerDegraded` condition.

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
  - Ideally want a way to have all clients/operators restart their watches as they may be trying to watch revisions that do not exist post backup. Restarting the individual operators is one solution but it does not scale well for a large number of workloads on the cluster.
  - Another potential idea is to artificially increase the etcd store revisions before brining up the API server to invalidate and refresh the storage cache. Requires more testing and experimentation.

See the [restore test design doc](https://docs.google.com/document/d/1NkdOwo53mkNBCktV5tkUnbM4vi7bG4fO5rwMR0wGSw8/edit?usp=sharing) for a more detailed breakdown on the restore test and the validation requirements.

Along with the e2e restore test, comprehensive unit testing of the backup controller will also be added to ensure the correct reconciliation and updates of the:
- Backup schedule
- Backup retention
- Backup status
- Alerting rules

### Graduation Criteria

The first version of the new API(s) will be `v1alpha1` which will be introduced behind the `TechPreviewNoUpgrade` feature gate.

#### Dev Preview -> Tech Preview

- NA

#### Tech Preview -> GA

The pre-requisite for graduating from Tech Preview will be a track record of reliably restoring from backups generated by the automated backups feature.
The pass rate for the e2e restore test outlined in the Test Plan should be at 99% before we graduate the feature to GA.

Additionally we may also want to iterate on gathering feedback on the user experience to improve the API before we GA.

### Upgrade / Downgrade Strategy

- TBD

### Version Skew Strategy

TBD

### Operational Aspects of API Extensions

TBD

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