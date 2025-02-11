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
  - "@tjungblu"
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

The [current documented procedure](https://docs.openshift.com/container-platform/4.12/backup_and_restore/control_plane_backup_and_restore/backing-up-etcd.html) for performing an etcd backup of an OpenShift
cluster is manually initiated. This reduces the likelihood of a timely backup being available during a disaster recovery scenario.

The procedure also requires gaining a root shell on a control plane
node. Shell access to OpenShift control plane nodes access is generally
discouraged due to the potential for affecting the reliability of the node.

### Goals

- One-time cluster backup can be triggered without requiring a root shell
- Scheduled backups can be configured after cluster installation
- Backups are saved to a configurable PersistentVolume, local or remote storage
- This feature is validated with an e2e restore test that ensures the backups saved can be used to recover the cluster from a quorum loss scenario


### Non-Goals
- Backups are saved locally on the host filesystem of control-plane nodes
  - A no-configuration/default local backups option is a future goal that will be addressed in a subsequent enhancement
- Have automated backups enabled by default with cluster installation
- Save cluster backups to cloud storage e.g S3
  - This could be a future enhancement or extension to the API
- Automate cluster restoration
- Provide automated backups for non-self hosted architectures like Hypershift


### User Stories

- As a cluster administrator I want to initiate a one-time cluster backup without requiring a root shell on a control plane node so as to minimize the risk involved
- As a cluster administrator I want to schedule recurring cluster backups so that I have a recent cluster state to recover from in the event of quorum loss (i.e. losing a majority of control-plane nodes)
- As a cluster administrator I want to have failure to take cluster backups for more than a configurable period to be reported to me via critical alerts


## Proposal

### Workflow Description

#### One time backups

To enable one-time backup requests via an API, a new cluster-scoped CRD, `operator.openshift.io/v1alpha1` `EtcdBackup` , will be used to trigger one-time backup requests.

A new controller in the cluster-etcd-operator, [`BackupController`](https://github.com/openshift/cluster-etcd-operator/blob/d7d43ee21aff6b178b2104228bba374977777a84/pkg/operator/backupcontroller/backupcontroller.go#L79), will reconcile backup requests as follows:

- Watch for new `EtcdBackup` CRs as created by an admin
- Create a backup Job configured for the `EtcdBackup` spec
- Track the backup progress, failure or success on the `EtcdBackup` status

#### Scheduled backups

To enable recurring backups a new cluster-scoped singleton CRD `config.openshift.io/v1alpha1` `Backup` will be used to specify the periodic backup configuration such as schedule, timezone, retention policy and other related configuration.

A new controller in the cluster-etcd-operator [`PeriodicBackupController`](https://github.com/openshift/cluster-etcd-operator/blob/d7d43ee21aff6b178b2104228bba374977777a84/pkg/operator/periodicbackupcontroller/periodicbackupcontroller.go#L69) would then reconcile the `Backup` config CR with the following workflow:

- Watches the `Backup` CR as created by an admin
- Creates a [CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) which in turn creates `EtcdBackup` CRs at the desired schedule
- Updates the CronJob for any changes in the schedule
- The CronJob is configured so that each scheduled Job run prunes the existing backups (per the retention policy) before requesting a new backup.
- Setting the CronJob's job history limits allows us to avoid accumulating completed Job runs and `EtcdBackup` CRs. To preserve the history of past runs [the failed and successful run limits](https://github.com/openshift/cluster-etcd-operator/blob/d7d43ee21aff6b178b2104228bba374977777a84/bindata/etcd/cluster-backup-cronjob.yaml#L12-L13) are set to a reasonable default.
- Concurrent executions of scheduled backups are forbidden via setting the [CronJob's concurrency policy](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/#concurrency-policy).

### API Extensions

#### EtcdBackup API

The `EtcdBackup` CRD for requesting one-time backups will be introduced to the API group-version `operator.openshift.io/v1alpha1`. The CRD would be feature-gated with the annotation `release.openshift.io/feature-set: TechPreviewNoUpgrade` until the prerequisite e2e test has an acceptable pass rate. See the Test Plan and Graduation Criteria sections for more details.

The spec and status will be as listed below:

```Go
type EtcdBackupSpec struct {
	// PVCName specifies the name of the PersistentVolumeClaim (PVC) which binds a PersistentVolume where the
	// etcd backup file would be saved
	// The PVC itself must always be created in the "openshift-etcd" namespace
	// If the PVC is left unspecified "" then the platform will choose a reasonable default location to save the backup.
	// In the future this would be backups saved across the control-plane master nodes.
	// +kubebuilder:validation:Optional
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="pvcName is immutable once set"
	PVCName string `json:"pvcName"`
}

// +kubebuilder:validation:Optional
type EtcdBackupStatus struct {
	// conditions provide details on the status of the etcd backup job.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions" patchStrategy:"merge" patchMergeKey:"type"`

	// backupJob is the reference to the Job that executes the backup.
	// Optional
	// +kubebuilder:validation:Optional
	BackupJob *BackupJobReference `json:"backupJob"`
}
```

#### Periodic Backup API

The `Backup` CRD will be introduced to the API group-version `config.openshift.io/v1alpha1`. The CRD would be feature-gated with the annotation `release.openshift.io/feature-set: TechPreviewNoUpgrade` until the prerequisite e2e test has an acceptable pass rate.

The spec will be as listed below, while the status will be empty since the status of individual backups will be tracked on the `EtcdBackup` CR's status and through scheduled runs of the CronJob.

```Go
type BackupSpec struct {
	// etcd specifies the configuration for periodic backups of the etcd cluster
	// +kubebuilder:validation:Required
	EtcdBackupSpec EtcdBackupSpec `json:"etcd"`
}

type BackupStatus struct {
}

// EtcdBackupSpec provides configuration for automated etcd backups to the cluster-etcd-operator
type EtcdBackupSpec struct {

	// Schedule defines the recurring backup schedule in Cron format
	// every 2 hours: 0 */2 * * *
	// every day at 3am: 0 3 * * *
	// Empty string means no opinion and the platform is left to choose a reasonable default which is subject to change without notice.
	// The current default is "no backups", but will change in the future.
	// +kubebuilder:validation:Optional
	// +optional
	// +kubebuilder:validation:Pattern:=`^(@(annually|yearly|monthly|weekly|daily|hourly))|(\*|(?:\*|(?:[0-9]|(?:[1-5][0-9])))\/(?:[0-9]|(?:[1-5][0-9]))|(?:[0-9]|(?:[1-5][0-9]))(?:(?:\-[0-9]|\-(?:[1-5][0-9]))?|(?:\,(?:[0-9]|(?:[1-5][0-9])))*)) (\*|(?:\*|(?:\*|(?:[0-9]|1[0-9]|2[0-3])))\/(?:[0-9]|1[0-9]|2[0-3])|(?:[0-9]|1[0-9]|2[0-3])(?:(?:\-(?:[0-9]|1[0-9]|2[0-3]))?|(?:\,(?:[0-9]|1[0-9]|2[0-3]))*)) (\*|(?:[1-9]|(?:[12][0-9])|3[01])(?:(?:\-(?:[1-9]|(?:[12][0-9])|3[01]))?|(?:\,(?:[1-9]|(?:[12][0-9])|3[01]))*)) (\*|(?:[1-9]|1[012]|JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC)(?:(?:\-(?:[1-9]|1[012]|JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC))?|(?:\,(?:[1-9]|1[012]|JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC))*)) (\*|(?:[0-6]|SUN|MON|TUE|WED|THU|FRI|SAT)(?:(?:\-(?:[0-6]|SUN|MON|TUE|WED|THU|FRI|SAT))?|(?:\,(?:[0-6]|SUN|MON|TUE|WED|THU|FRI|SAT))*))$`
	Schedule string `json:"schedule"`

	// Cron Regex breakdown:
	// Allow macros: (@(annually|yearly|monthly|weekly|daily|hourly))
	// OR
	// Minute:
	//   (\*|(?:\*|(?:[0-9]|(?:[1-5][0-9])))\/(?:[0-9]|(?:[1-5][0-9]))|(?:[0-9]|(?:[1-5][0-9]))(?:(?:\-[0-9]|\-(?:[1-5][0-9]))?|(?:\,(?:[0-9]|(?:[1-5][0-9])))*))
	// Hour:
	//   (\*|(?:\*|(?:\*|(?:[0-9]|1[0-9]|2[0-3])))\/(?:[0-9]|1[0-9]|2[0-3])|(?:[0-9]|1[0-9]|2[0-3])(?:(?:\-(?:[0-9]|1[0-9]|2[0-3]))?|(?:\,(?:[0-9]|1[0-9]|2[0-3]))*))
	// Day of the Month:
	//   (\*|(?:[1-9]|(?:[12][0-9])|3[01])(?:(?:\-(?:[1-9]|(?:[12][0-9])|3[01]))?|(?:\,(?:[1-9]|(?:[12][0-9])|3[01]))*))
	// Month:
	//   (\*|(?:[1-9]|1[012]|JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC)(?:(?:\-(?:[1-9]|1[012]|JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC))?|(?:\,(?:[1-9]|1[012]|JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC))*))
	// Day of Week:
	//   (\*|(?:[0-6]|SUN|MON|TUE|WED|THU|FRI|SAT)(?:(?:\-(?:[0-6]|SUN|MON|TUE|WED|THU|FRI|SAT))?|(?:\,(?:[0-6]|SUN|MON|TUE|WED|THU|FRI|SAT))*))
	//

	// The time zone name for the given schedule, see https://en.wikipedia.org/wiki/List_of_tz_database_time_zones.
	// If not specified, this will default to the time zone of the kube-controller-manager process.
	// See https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/#time-zones
	// +kubebuilder:validation:Optional
	// +optional
	// +kubebuilder:validation:Pattern:=`^([A-Za-z_]+([+-]*0)*|[A-Za-z_]+(\/[A-Za-z_]+){1,2})(\/GMT[+-]\d{1,2})?$`
	TimeZone string `json:"timeZone"`

	// Timezone regex breakdown:
	// ([A-Za-z_]+([+-]*0)*|[A-Za-z_]+(/[A-Za-z_]+){1,2}) - Matches either:
	//   [A-Za-z_]+([+-]*0)* - One or more alphabetical characters (uppercase or lowercase) or underscores, followed by a +0 or -0 to account for GMT+0 or GMT-0 (for the first part of the timezone identifier).
	//   [A-Za-z_]+(/[A-Za-z_]+){1,2} - One or more alphabetical characters (uppercase or lowercase) or underscores, followed by one or two occurrences of a forward slash followed by one or more alphabetical characters or underscores. This allows for matching timezone identifiers with 2 or 3 parts, e.g America/Argentina/Buenos_Aires
	// (/GMT[+-]\d{1,2})? - Makes the GMT offset suffix optional. It matches "/GMT" followed by either a plus ("+") or minus ("-") sign and one or two digits (the GMT offset)

	// RetentionPolicy defines the retention policy for retaining and deleting existing backups.
	// +kubebuilder:validation:Optional
	// +optional
	RetentionPolicy RetentionPolicy `json:"retentionPolicy"`

	// PVCName specifies the name of the PersistentVolumeClaim (PVC) which binds a PersistentVolume where the
	// etcd backup files would be saved
	// The PVC itself must always be created in the "openshift-etcd" namespace
	// If the PVC is left unspecified "" then the platform will choose a reasonable default location to save the backup.
	// In the future this would be backups saved across the control-plane master nodes.
	// +kubebuilder:validation:Optional
	// +optional
	PVCName string `json:"pvcName"`
}

// RetentionType is the enumeration of valid retention policy types
// +enum
// +kubebuilder:validation:Enum:="RetentionNumber";"RetentionSize"
type RetentionType string

const (
	// RetentionTypeNumber sets the retention policy based on the number of backup files saved
	RetentionTypeNumber RetentionType = "RetentionNumber"
	// RetentionTypeSize sets the retention policy based on the total size of the backup files saved
	RetentionTypeSize RetentionType = "RetentionSize"
)

// RetentionPolicy defines the retention policy for retaining and deleting existing backups.
// This struct is a discriminated union that allows users to select the type of retention policy from the supported types.
// +union
type RetentionPolicy struct {
	// RetentionType sets the type of retention policy.
	// Currently, the only valid policies are retention by number of backups (RetentionNumber), by the size of backups (RetentionSize). More policies or types may be added in the future.
	// Empty string means no opinion and the platform is left to choose a reasonable default which is subject to change without notice.
	// The current default is RetentionNumber with 15 backups kept.
	// +unionDiscriminator
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:="";"RetentionNumber";"RetentionSize"
	RetentionType RetentionType `json:"retentionType"`

	// RetentionNumber configures the retention policy based on the number of backups
	// +kubebuilder:validation:Optional
	// +optional
	RetentionNumber *RetentionNumberConfig `json:"retentionNumber,omitempty"`

	// RetentionSize configures the retention policy based on the size of backups
	// +kubebuilder:validation:Optional
	// +optional
	RetentionSize *RetentionSizeConfig `json:"retentionSize,omitempty"`
}

// RetentionNumberConfig specifies the configuration of the retention policy on the number of backups
type RetentionNumberConfig struct {
	// MaxNumberOfBackups defines the maximum number of backups to retain.
	// If the existing number of backups saved is equal to MaxNumberOfBackups then
	// the oldest backup will be removed before a new backup is initiated.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Required
	// +required
	MaxNumberOfBackups int `json:"maxNumberOfBackups,omitempty"`
}

// RetentionSizeConfig specifies the configuration of the retention policy on the total size of backups
type RetentionSizeConfig struct {
	// MaxSizeOfBackupsGb defines the total size in GB of backups to retain.
	// If the current total size backups exceeds MaxSizeOfBackupsGb then
	// the oldest backup will be removed before a new backup is initiated.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Required
	// +required
	MaxSizeOfBackupsGb int `json:"maxSizeOfBackupsGb,omitempty"`
}
```


### Implementation Details/Notes/Constraints [optional]

With the above APIs, the following controllers are required to save backups:
- [BackupController](https://github.com/openshift/cluster-etcd-operator/blob/65a2a91872d9279c4654176f1a7a49c0c74dcce5/pkg/operator/backupcontroller/backupcontroller.go#L50): to reconcile `operator.openshift.io EtcdBackup` CRs for one-time backups
- [PeriodicBackupController](https://github.com/openshift/cluster-etcd-operator/blob/65a2a91872d9279c4654176f1a7a49c0c74dcce5/pkg/operator/periodicbackupcontroller/periodicbackupcontroller.go#L41) to reconcile `config.openshift.io Backup` CRs for scheduled or periodic backups.


#### Executing the backup cmd

The one-time BackupController will create a [backup Job](https://github.com/openshift/cluster-etcd-operator/blob/master/bindata/etcd/cluster-backup-job.yaml) that runs the existing [backup script](https://github.com/openshift/cluster-etcd-operator/blob/master/bindata/etcd/cluster-backup.sh) to save the etcd snapshot and static pod manifests to the desired location. 
Since the existing bash script is harder to maintain and test, in the future this can be changed to use the etcd Go client to save the snapshot. See [#1099](https://github.com/openshift/cluster-etcd-operator/pull/1099).


#### Saving the backup to a PV

When choosing where to save the backup, the user can specify a PVC via the `pvcName` field for both periodic or one-time backups. This allows them to save the backups to either a remote storage solution (e.g `nfs` type PV) or locally via the [`local` PV type](https://kubernetes.io/docs/concepts/storage/volumes/#local). 


#### Retention policy

As outlined in the `config.openshift.io Backup` API, the operator will support the following kinds of retention policies to ensure it doesn't exhaust the available space for saving backups:
- `RetentionNumber` to specify the maximum number of backups to retain
- `RetentionSize` to specify the maximum total size of all backups retained 

#### Multiple Backup Requests

To avoid running multiple backups in the presence of multiple backup requests i.e `EtcdBackup` CRs, the operator will pick one request in lexicographic order and mark all the other requests as skipped. 

#### Garbage collection

When scheduled backups are enabled, the resulting CronJob that is created will have its `spec.failedJobsHistoryLimit` and `spec. successfulJobsHistoryLimit` set to a sane default to prevent the pile up of completed or failed Job runs.
The [defaults](https://github.com/openshift/cluster-etcd-operator/blob/65a2a91872d9279c4654176f1a7a49c0c74dcce5/bindata/etcd/cluster-backup-cronjob.yaml#L12-L13) at the time of writing are 10 failed jobs and 5 successful jobs.

The chain of ownership set via `OwnerReferences` for the objects created for periodic backups is in the following order:
- `config.openshift.io Backup` CR
- `CronJob` created for the `Backup` CR
- `Job/Pod` for a scheduled CronJob run
- `operator.openshift.io EtcdBackup` CR created for that run
- `Job/Pod` to execute the etcd backup for the `EtcdBackup` CR

Given that the CronJob is namespaced and the `operator.openshift.io EtcdBackup` CR is cluster-scoped, the default kubernetes garbage collection [will not be enforced](https://kubernetes.io/docs/concepts/architecture/garbage-collection/#owners-dependents) for cluster-scoped dependents.
To remedy this and cleanup `EtcdBackup` CRs from scheduled runs a [BackupRemovalController](https://github.com/openshift/cluster-etcd-operator/blob/65a2a91872d9279c4654176f1a7a49c0c74dcce5/pkg/operator/backupcontroller/backup_removal_controller.go#L25-L34) will be introduced which will cleanup `EtcdBackup` CRs without any OwnerReferences.



### Risks and Mitigations

When the backups are configured to be saved to a `local`` type PV, the backups are all saved to a singular master node where the PV is provisioned on the local disk.

In the event of a node becoming inaccessible or unschedulable, the recurring backups would not be scheduled. The periodic backup config would have to be recreated or updated with a different PVC that allows for a new PV to be provisioned on a node that is healthy.

#### Spreading backups across nodes

Losing the node where the backups are saved is a significant risk. To mitigate this issue, the backups can be spread across all master nodes using a combination of node-selectors and hostPath type volumes. This would also require the operator to ensure that the retention policy is applied across all nodes by gauging the existing spread which require looking at the history of completed backups.
This would be targeted as a future extension to the feature before it is ready for GA.

TODO: Outline how periodic backups can be spread locally across the control-plane nodes and how retention would be done. 

## Design Details

### Open Questions

TBD

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