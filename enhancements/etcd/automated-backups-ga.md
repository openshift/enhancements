---
title: automated-backups-ga
authors:
 - "@bhperry"
reviewers:
 - "@dusk125"
 - "@tjungblu"
 - "@atiratree"
 - "@hasbro17"
approvers:
 - "@tjungblu"
 - "@dusk125"
 - "@atiratree"
 - "@JoelSpeed"
api-approvers:
 - "@JoelSpeed"
creation-date: 2026-05-12
last-updated: 2026-05-12
tracking-link:
 - "https://redhat.atlassian.net/browse/CNTRLPLANE-3407"
see-also:
 - "https://redhat.atlassian.net/browse/OCPSTRAT-1937"
replaces:
 - enhancements/etcd/automated-backups.md
---

# Automated Backups of etcd GA

Supersedes [automated-backups](./automated-backups.md)

## Summary

Enable automated backups of etcd snapshots and static pod manifests on an OpenShift self-hosted cluster to recover lost data, rollback changes (e.g. cluster upgrade), or restore from a quorum loss scenario.

## Motivation

The [current documented procedure](https://docs.openshift.com/container-platform/4.12/backup_and_restore/control_plane_backup_and_restore/backing-up-etcd.html) for performing an etcd backup of an OpenShift
cluster is manually initiated. This reduces the likelihood of a timely backup being available during a disaster recovery scenario.

The procedure also requires gaining a root shell on a control plane node. Shell access to OpenShift control plane nodes access is generally
discouraged due to the potential for affecting the reliability of the node.

### Goals

- Initiate on demand cluster backup without a root shell
- Backups are saved to a configurable location
   - PersistentVolume
   - Local node storage
   - Support additional storage backends in future enhancements (e.g. cloud object storage)
- Automated backup schedule can be configured by cluster administrators with k8s API
- Retention rules allow managing the lifecycle of automated backups to prevent running out of disk space
- Automated backups are enabled at install time with a default schedule using the same API mechanism available to users
- This feature is validated with an e2e restore test that ensures the saved backups can be used to recover a cluster from quorum loss

### Non-Goals

- Save cluster backups to cloud storage (e.g. S3)
- Automate cluster restoration
- Provide automated backups for non-self hosted architectures like Hypershift

### User Stories

- As a cluster administrator I want to initiate a one-time backup of a cluster before upgrading it with minimal risk
- As a cluster administrator I want to schedule recurring backups so that I have a recent restore point to recover from if the control plane goes down
- As a cluster administrator I want to be notified when automated cluster backups have failed for a configurable amount of time (e.g. no successful backups for the last 2 days)
- As a cluster administrator I want to be able to delete old backups from the API

## Proposal

### Workflow Description

#### On demand backups

On demand backups may be requested with the new cluster-scoped CRD `operator.openshift.io/v1alpha1` `EtcdBackup`. Each EtcdBackup CR corresponds to an etcd backup job for a single master node with a specified storage location.

A new controller in the cluster-etcd-operator called [`BackupController`](https://github.com/openshift/cluster-etcd-operator/blob/d7d43ee21aff6b178b2104228bba374977777a84/pkg/operator/backupcontroller/backupcontroller.go#L79) reconciles backup requests as follows:

- Watches `EtcdBackup` CRs
- Ensure `EtcdBackup` has `operator.openshift.io/etcd-backup` finalizer
- Create a backup Job with `ttlSecondsAfterFinished` for GC and `operator.openshift.io/etcd-backup` finalizer
   - On completion, report metadata to termination log `{"size": "1GB", "path": "/path/to/backupfile"} > /dev/termination-log`
   - Use a reasonably generous TTL so that admins have an opportunity to inspect the job/pod/logs if needed
   - The Job finalizer is added to ensure that the controller gets a chance to sync the Job’s status and termination-log to the `EtcdBackup` before the GC cleans it up
- Set BackupPending status condition on `EtcdBackup`
- Watch for Job success/failure
   - On success
       - Get backup Pod and parse metadata from termination message
       - Update `EtcdBackup` status with `BackupCompleted` condition, file size, and file path
   - On failure
       - Update `EtcdBackup` status with `BackupFailed` condition
   - Remove `operator.openshift.io/etcd-backup` finalizer from the Job

#### Scheduled backups

Automated backup schedules may be configured with the new cluster-scoped CRD `operator.openshift.io/v1alpha1` `EtcdBackupPolicy`. Each EtcdBackupPolicy CR sets the cron schedule, retention, and storage location for etcd backups on a targeted set of master nodes.

A new controller in the cluster-etcd-operator called [`PeriodicBackupController`](https://github.com/openshift/cluster-etcd-operator/blob/d7d43ee21aff6b178b2104228bba374977777a84/pkg/operator/periodicbackupcontroller/periodicbackupcontroller.go#L69) reconciles backup schedule requests as follows:

- Watches `EtcdBackupPolicy` CRs
- Creates robfig/cron entries internally to manage backup schedules
   - Using an internal cron scheduler reduces the complexity of the kubernetes object model and gives cluster-etcd-operator full control over scheduling
   - If CEO is down, then an external CronJob creating EtcdBackup CRs does not improve availability of backups since the CEO is needed to reconcile them
   - When CEO is restored after an outage the internal cron schedules are rebuilt from EtcdBackupPolicies, and any missed EtcdBackups during the outage can be created as needed
- Manage retention policies (outside of cron func)
   - List completed `EtcdBackups` and associate them with parent `EtcdBackupPolicy`
   - Sort by age and apply configured retention policies, deleting from oldest first
- On cron trigger, the PeriodicBackupController creates a new `EtcdBackup` CR based on the `EtcdBackupPolicy`
   - Backup follows the [On demand backups](#on-demand-backups) flow

#### Garbage Collection

Backup garbage collection is managed by watching for `EtcdBackup` CRs that have been deleted and still have the `operator.openshift.io/etcd-backup` finalizer

- Collect all deleted `EtcdBackups` by storage backend (e.g. with Local storage, collect by `node + hostPath`, with PVC storage collect by `PVC name`)
- Create a Job that mounts the storage backend and deletes the associated files
- Remove the `operator.openshift.io/etcd-backup` finalizer from `EtcdBackups`

### API Extensions

#### EtcdBackup API

The `EtcdBackup` CRD for requesting one-time backups will be introduced to the API group-version `operator.openshift.io/v1alpha1`. The CRD would be feature-gated with the annotation `release.openshift.io/feature-set: TechPreviewNoUpgrade` until the prerequisite e2e test has an acceptable pass rate. See the Test Plan and Graduation Criteria sections for more details.

Finalizer `operator.openshift.io/etcd-backup` is added to `EtcdBackup` before the backup job is started.

```go
// EtcdBackup is a request for a backup snapshot of the etcd cluster on a master node.
type EtcdBackup struct {
   // spec configures the node and storage location of the backup.
   // +kubebuilder:validation:Required
   // +required
   Spec EtcdBackupSpec `json:"spec"`

   // status describes the state of the backup request.
   // +kubebuilder:validation:Optional
   // +optional
   Status EtcdBackupStatus `json:"status"`
}

// +kubebuilder:validation:XValidation:rule="self.nodeName == '' && self.storage.type == 'Local'",message="nodeName is required for Local storage."
type EtcdBackupSpec struct {
   // nodeName specifies the master node where an etcd backup should be taken.
   // If not specified, a random master node will be selected.
   // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="nodeName is immutable once set"
   // +kubebuilder:validation:Optional
   // +optional
   NodeName string `json:"nodeName,omitempty"`

   // storage specifies the location where etcd backup files will be saved.
   // +kubebuilder:validation:Required
   // +required
   Storage EtcdBackupStorage `json:"storage"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'PVC' ? has(self.pvc) : !has(self.pvc)",message="pvc is required when type is PVC, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="self.type == 'Local' ? has(self.local) : !has(self.local)",message="local is required when type is Local, and forbidden otherwise"
// +union
type EtcdBackupStorage struct {
   // +kubebuilder:validation:Enum:=PVC;Local;
   // +kubebuilder:validation:Required
   // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable once set"
   // +required
   // +unionDiscriminator
   Type EtcdBackupStorageType `json:"type"`

   // pvc specifies the PersistentVolumeClaim (PVC) which binds a PersistentVolume where the etcd backup file will be saved.
   // The PVC must always be created in the "openshift-etcd" namespace.
   // This field is required when the storage type is "PVC"
   // +kubebuilder:validation:Optional
   // +optional
   // +unionMember
   PVC *EtcdBackupStoragePvc `json:"pvc,omitempty"`

   // local specifies a host path directory on the master node where the etcd backup file will be saved.
   // This field is required when storage type is "Local"
   // +kubebuilder:validation:Optional
   // +optional
   // +unionMember
   Local *EtcdBackupStorageLocal `json:"local,omitempty"`
}


// EtcdBackupStorageType is an enum of the supported storage backends for backup files
type EtcdBackupStorageType string

const (
   EtcdBackupStorageTypePVC EtcdBackupStorageType = "PVC"
   EtcdBackupStorageTypeLocal EtcdBackupStorageType = "Local"
)

type EtcdBackupStoragePvc struct {
   // name is a reference to a PVC in the "openshift-etcd" namespace where the etcd backup file will be saved.
   // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable once set"
   // +kubebuilder:validation:Required
   // +required
   Name string `json:"name"`

   // path is a directory on the volume where the etcd backup file will be saved.
   // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable once set"
   // +kubebuilder:validation:Optional
   // +optional
   Path string `json:"path"`
}

type EtcdBackupStorageLocal struct {
   // hostPath is a local directory on the master node where the etcd backup file will be saved.
   // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="hostPath is immutable once set"
   // +kubebuilder:validation:Required
   // +required
   HostPath string `json:"hostPath"`
}

type EtcdBackupStatus struct {
   // conditions provide details on the status of the etcd backup job.
   // +kubebuilder:validation:Optional
   // +listType=map
   // +listMapKey=type
   // +optional
   Conditions []metav1.Condition `json:"conditions"`

   // job is a reference to the Job created for the backup.
   // +kubebuilder:validation:Optional
   // +optional
   Job *core.ObjectReference `json:"job,omitempty"`

   // nodeName is the master node where the backup snapshot was taken.
   // +kubebuilder:validation:Optional
   // +optional
   NodeName string `json:"nodeName,omitempty"`

   // filePath is the absolute path to the backup file on the storage backend.
   // +kubebuilder:validation:Optional
   // +optional
   FilePath string `json:"filePath,omitempty"`
}

type BackupConditionReason string

var (
   // BackupPending is added to the EtcdBackupStatus Conditions when the etcd backup is pending.
   BackupPending BackupConditionReason = "BackupPending"

   // BackupCompleted is added to the EtcdBackupStatus Conditions when the etcd backup has completed.
   BackupCompleted BackupConditionReason = "BackupCompleted"

   // BackupFailed is added to the EtcdBackupStatus Conditions when the etcd backup has failed.
   BackupFailed BackupConditionReason = "BackupFailed"
)
```

#### EtcdBackupPolicy API

The `EtcdBackupPolicy` CRD for requesting an automated backup schedule will be introduced to the API group-version `operator.openshift.io/v1alpha1`. The CRD would be feature-gated with the annotation `release.openshift.io/feature-set: TechPreviewNoUpgrade` until the prerequisite e2e test has an acceptable pass rate. See the Test Plan and Graduation Criteria sections for more details.

```go
// EtcdBackupPolicy sets an automated schedule for taking backups of the etcd cluster.
type EtcdBackupPolicy struct {
   // spec configures the schedule, retention, and storage policies for automated backups.
   // +kubebuilder:validation:Required
   // +required
   Spec EtcdBackupPolicySpec `json:"spec"`
  
   // status describes the state of the backup policy.
   // +kubebuilder:validation:Optional
   // +optional
   Status EtcdBackupPolicyStatus `json:"status"`
}

type EtcdBackupPolicySpec struct {
   // schedule sets the backup schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
   // +kubebuilder:validation:Required
   // +required
   Schedule string `json:"schedule"`

   // The time zone name for the given schedule, see https://en.wikipedia.org/wiki/List_of_tz_database_time_zones.
   // If not specified, this will default to the time zone of the cluster-etcd-operator process.
   // +kubebuilder:validation:Optional
   // +optional
   TimeZone string `json:"timeZone,omitempty"`

   // nodeSelector specifies which master node(s) to run backup jobs on.
   // If no selector is specified, the default node-role.kubernetes.io/master label will be used.
   // +kubebuilder:validation:Optional
   // +optional
   NodeSelector map[string]string `json:"nodeSelector,omitempty"`

   // nodeCount sets the maximum number of nodes to run backups on when multiple are selected.
   // If a value greater than zero is set, nodes are chosen in a random order from the set of healthy nodes selected by the nodeSelector.
   // Values less than or equal to zero will select all available nodes.
   NodeCount int `json:"nodeCount,omitempty"`

   // storage specifies the location where etcd backup files will be saved.
   // +kubebuilder:validation:Required
   // +required
   Storage EtcdBackupStorage `json:"storage"`

   // retentionRules defines the policy for retaining and deleting existing backups.
   // Backups are deleted from the oldest first until all rules are satisfied (logical OR).
   // If no rules are specified then no backups will be deleted.
   // +kubebuilder:validation:Optional
   // +optional
   RetentionRules []EtcdBackupPolicyRetentionRule `json:"retentionRules,omitempty"`
}

// +union
type EtcdBackupPolicyRetentionRule struct {
   // type defined which rule field is set
   // +unionDiscriminator
   // +kubebuilder:validation:Enum:="MaxQuantity";"MaxAge";"MaxSize"
   // +kubebuilder:validation:Required
   // +required
   Type EtcdBackupPolicyRetentionRuleType

   // maxQuantity enforces the deletion of backups that exceed the given count.
   // +kubebuilder:validation:Optional
   // +optional
   MaxQuantity *int `json:"maxQuantity,omitempty"`

   // maxAge enforces the deletion of backups older than the given duration.
   // +kubebuilder:validation:Optional
   // +optional
   MaxAge *metav1.Duration `json:"maxAge,omitempty"`

   // maxSize enforces the deletion of backups when the total size exceeds the given amount.
   // +kubebuilder:validation:Optional
   // +optional
   MaxSize *resource.Quantity `json:"maxSize,omitempty"`
}

type EtcdBackupPolicyStatus struct {
   // lastScheduleTime is the time when the last scheduled backup was triggered.
   // This is used by the controller to track when backups have been executed
   // and to prevent duplicate executions on controller restart.
   // +kubebuilder:validation:Optional
   // +optional
   LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

   // lastScheduleNodes is the name of nodes selected during the last scheduled execution.
   // +kubebuilder:validation:Optional
   // +optional
   LastScheduleNodes []string `json:"lastScheduleNodes,omitempty"`
}
```

#### Examples

```yaml
# On demand backup of a single master node
kind: EtcdBackup
apiVersion: operator.openshift.io/v1alpha1
metadata:
   name: backup-2026-05-12
spec:
   nodeName: master-0
   storage:
       type: Local
       local:
           hostPath: /path/to/etcdbackups
```

```yaml
# Store daily backups on each master node, retain 2 per node
kind: EtcdBackupPolicy
apiVersion: operator.openshift.io/v1alpha1
metadata:
   name: daily-local-backups
spec:
   schedule: @daily
   retentionRules:
   - type: MaxQuantity
     maxQuantity: 10
   - type: MaxSize
     maxSize: 20G
   storage:
       type: Local
       local:
           hostPath: /path/to/etcdbackups
---
# Store backups for each master node on a shared NFS PVC, retain up to 2 weeks
kind: EtcdBackupPolicy
apiVersion: operator.openshift.io/v1alpha1
metadata:
   name: weekly-pvc-backups
spec:
   schedule: @weekly
   retentionRules:
   - type: MaxAge
     maxAge: 336h
   storage:
       type: PVC
       pvc:
           name: etcd-nfs-backups
---
# Store hourly backups from a single master node on a block storage PVC
kind: EtcdBackupPolicy
apiVersion: operator.openshift.io/v1alpha1
metadata:
   name: weekly-pvc-backups
spec:
   schedule: @hourly
   nodeSelector:
       node-role.kubernetes.io/master: ""
       my.custom/label: "somevalue"
   nodeCount: 1
   retentionRules:
   - type: MaxQuantity
     maxQuantity: 6
   storage:
       type: PVC
       pvc:
           name: etcd-block-storage-backups
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

#### OpenShift Kubernetes Engine

### Implementation Details/Notes/Constraints [optional]

With the above APIs, the following controllers are required to save/delete backups:
- [BackupController](https://github.com/openshift/cluster-etcd-operator/blob/65a2a91872d9279c4654176f1a7a49c0c74dcce5/pkg/operator/backupcontroller/backupcontroller.go#L50): to reconcile `operator.openshift.io EtcdBackup` CRs for one-time backups
- [PeriodicBackupController](https://github.com/openshift/cluster-etcd-operator/blob/65a2a91872d9279c4654176f1a7a49c0c74dcce5/pkg/operator/periodicbackupcontroller/periodicbackupcontroller.go#L41) to reconcile `operator.openshift.io EtcdBackupPolicy` CRs for scheduled backups.
- BackupGarbageCollectionController (not yet implemented, but will replace [BackupRemoveController](https://github.com/openshift/cluster-etcd-operator/blob/c0614ca08f4f22f9c11684c7e1f05da5f57389d6/pkg/operator/backupcontroller/backup_removal_controller.go))
   - Could potentially be combined with the BackupController. Already watching EtcdBackup and Job there, would just be a different reconciliation path

#### Executing the backup cmd

The BackupController will add the `etcd.openshift.io/backup` Finalizer to `EtcdBackup` and then create a [backup Job](https://github.com/openshift/cluster-etcd-operator/blob/master/bindata/etcd/cluster-backup-job.yaml) that runs the existing [backup script](https://github.com/openshift/cluster-etcd-operator/blob/master/bindata/etcd/cluster-backup.sh) to save the etcd snapshot and static pod manifests to the desired location.
Since the existing bash script is harder to maintain and test, in the future this can be changed to use the etcd Go client to save the snapshot. See [#1099](https://github.com/openshift/cluster-etcd-operator/pull/1099).

On completion the backup Job writes file size and path as JSON to `/dev/termination-log`, allowing the BackupController to fill in metadata on the associated `EtcdBackup`. This approach allows basic communication from the backup job to the controller without providing RBAC privileges to the job itself, and ensuring that only a single process is responsible for updating `EtcdBackup` status.

#### Backup storage

When choosing where to save backups for both scheduled and one-time backups the user can specify different locations for files to be saved. With the `PVC` storage type they can save backups to any CSI compatible remote storage solution (e.g `nfs` type PV). They may also choose to use the `Local` storage type in order to store backups directly on the master node at a given hostPath. In future, additional storage types may be added to support different remote backends like cloud object storage.

PVC storage backed by a block volume with mode ReadWriteOnce may be used with `nodeCount: 1` in order to select one available master node for backup. If no node count is set, or value is greater than 1, no special handling is used to manage the order of backups. Jobs will be spawned for all selected nodes and will complete in a non-deterministic order as they are able to bind the shared volume.

#### Retention Limits

As outlined in the `operator.openshift.io EtcdBackupPolicy` API, the operator will support the following kinds of retention policies to ensure it doesn't exhaust the available space for saving backups:
- Quantity: Specify the maximum number of backups to retain
- Size: Specify the maximum total size of all backups to retain

Backups will be deleted in order from oldest to newest until all limits are satisfied. E.g. with `quantity: 3` `size: 20Gb` there will be at most 3 backups with a combined total size of 20Gb retained at any given time. Backup deletion is managed by the PeriodicBackupController via the k8s API. Deleting an EtcdBackup signals the BackupGarbageCollectionController to delete the associated file from its storage backend.

#### Garbage Collection

The Jobs created for each EtcdBackup will be cleaned up by setting ttlSecondsAfterFinished. Finalizer on the Job prevents it from being deleted before the controller is able to update state on the associated EtcdBackup.

EtcdBackups that are manually created will not be GC'd, but those created by a EtcdBackupPolicy will be deleted based on the configured retention policies.

When an EtcdBackup is deleted, its Finalizer will prevent the object from being removed until the BackupGarbageCollectionController runs. The GC controller will check the status of the EtcdBackup, and if it has a completed backup file then it will create a job to mount storage and delete the file. Once this job is complete the Finalizer can be removed and the EtcdBackup object will finish deleting.

### Risks and Mitigations

#### Local storage vs. local PV

When backups are configured to be saved to a PVC backed by local disk, they will all be saved on a singular master node. By using the `Local` storage type instead backups can be spread across all master nodes with retention managed on a per-node basis. If one or more nodes are lost, backups can be retrieved from any of the remaining master nodes.

#### Remote Storage PVC

To decouple backup storage from the master nodes, any available CSI driver may be used to provision a PVC as the storage backend. In this case retention is managed per-volume. If multiple nodes are backing up to a single volume and a retention rule MaxQuantity of 2 is used, then only the 2 most recent node backups will be stored. Different retention rules may be mixed together to accommodate various needs and storage backends (e.g. maxAge could be used to keep all backups from the last 3 days taken across multiple master nodes).

#### Restore from old backup

When restoring from an older backup, newer `EtcdBackup` CRs will be lost even though their files still exist on their storage backend. This results in orphaned backup files which will no longer be managed by `EtcdBackupPolicy` retention rules.

In future there could be an automated discovery process that finds files on the storage backend of an `EtcdBackupPolicy` and generates an `EtcdBackup` for each. This will be left for a future enhancement, and for now this edge case will be clearly documented.

### Drawbacks

- EtcdBackupPolicy physical file deletion via retention mechanism is disconnected from backup creation since it is managed via the k8s API. A separate job is spawned to mount the storage backend and delete files. In the case of a ReadWriteOnce volume used for backup storage this could cause contention on mounting the volume onto the backup job and the cleanup job.

## Design Details

### Open Questions

- Should retention rules also include MinQuantity that enforces a minimum N backups are retained when combined with other rules (e.g. MaxAge, MaxSize).
   - Would prevent cases where new backups have failed and all the existing backups get aged out

- Creating an EtcdBackup could allow a user to escape their namespace and privileges. This feature requires an auth check to only allow cluster-admins to interact with the backups.

## Test Plan

Prior to merging changes to the backup controller implementation, existing e2e recovery tests will be adapted to test the validity of the backups generated from the automated backup feature. The pass rate on this test will feature gate the API until we pass an acceptable threshold to make it available by default.

This test will run through the following backup and restore procedure:
- Start with a cluster that has the `EtcdBackups` and `EtcdBackupPolicy` APIs enabled
- Save a backup of the cluster on demand by using the `EtcdBackup` API
- Modify the cluster state post backup
- Induce a disaster recovery scenario of 2/3 nodes lost with etcd quorum loss
- Step through the recovery procedure from the saved backup
- Ensure that the control-plane has recovered and is stable
- Validate the we have restored the cluster to the previous state so that it does not have the post-backup changes

See the [restore test design doc](https://docs.google.com/document/d/1NkdOwo53mkNBCktV5tkUnbM4vi7bG4fO5rwMR0wGSw8/edit?usp=sharing) for a more detailed breakdown on the restore test and the validation requirements.

Along with the e2e restore test, comprehensive unit testing of the backup controller will also be added to ensure the correct reconciliation and updates of the:
- Backup scheduling
- Backup retention
- Backup status
- Alerting rules

## Graduation Criteria

This enhancement build on top of the existing feature in tech preview described in [automated backups of etcd](./automated-backups.md). The revised version of the new APIs will be `v1alpha1` which will be introduced behind the `TechPreviewNoUpgrade` feature gate.

### Dev Preview -> Tech Preview

- NA

### Tech Preview -> GA

The pre-requisite for graduating from Tech Preview will be a track record of reliably restoring from backups generated by the automated backups feature.
The pass rate for the e2e restore test outlined in the Test Plan should be at 99% before we graduate the feature to GA.

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives (Not Implemented)

- Backup job patches `EtcdBackup` status directly
   - Requires granting RBAC privileges
   - More fragile
       - If backup succeeds but patch fails, `EtcdBackup` is in a mixed success/failure state
       - Multiple processes updating the same object can lead to conflicts

- Manage backup retention based only on files on the filesystem
   - If deletion of `EtcdBackup` is not tied to file deletion, cluster admins have less control
   - More difficult to associate files to their `EtcdBackupPolicy` without writing additional metadata to the FS or embedding data in file names
   - Multiple `EtcdBackupPolicies` writing to the same FS location could have conflicting retentions if their files aren't properly associated

- Use an existing open source backup solution such as Velero
   - To be discussed further with the team that made the original enhancement

## Infrastructure Needed

The new API will be introduced in the `openshift/api` repo and the controller will be added to the existing cluster-etcd-operator in the `openshift/cluster-etcd-operator` repo.

[upgradebackupcontroller]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/pkg/operator/upgradebackupcontroller/upgradebackupcontroller.go#L284-L298
[backup pod]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/bindata/etcd/cluster-backup-pod.yaml#L31-L38
[backup script]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/bindata/etcd/cluster-backup.sh#L121-L129
