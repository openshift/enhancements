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

The [current documented procedure](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/backup_and_restore/control-plane-backup-and-restore) for performing an etcd backup of an OpenShift
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
   - Support additional retention rule types in future enhancement (e.g. MaxAge)
- Automated backups are enabled at install time with a default schedule using the same API mechanism available to users
- This feature is validated with an e2e restore test that ensures the saved backups can be used to recover a cluster from quorum loss
- Collect prometheus metrics that can be used to monitor and alert on successful/failed backups

### Non-Goals

- Save cluster backups to cloud storage (e.g. S3)
   - This can be accomplished already using the appropriate CSI driver and PVC
- Automate cluster restoration
- Provide automated backups for non-self hosted architectures like Hypershift
- Backup file discovery will be discussed in a future enhancement, if needed
   - Recreate EtcdBackups found in EtcdBackupPolicy storage locations after a cluster is restored to a previous point in time
   - Delete EtcdBackups if their files are manually deleted (would need to be very careful about this)

### User Stories

- As a cluster administrator I want to initiate a one-time backup of a cluster before upgrading it with minimal risk
- As a cluster administrator I want to schedule recurring backups so that I have a recent restore point to recover from if the control plane goes down
- As a cluster administrator I want to be notified when automated cluster backups have failed for a configurable amount of time (e.g. no successful backups for the last 2 days)
- As a cluster administrator I want to be able to delete old backups from the API

## Proposal

[POC Implementation](https://github.com/openshift/cluster-etcd-operator/pull/1657)

### Workflow Description

#### On demand backups

On demand backups may be requested with the new cluster-scoped CRD `operator.openshift.io/v1alpha1` `EtcdBackup`. Each EtcdBackup CR corresponds to an etcd backup job for a single control plane node with a specified storage location.

##### BackupController

- Watches Pending/Running `EtcdBackup` CRs and Jobs with `app=cluster-backup-job` label
- Sync `EtcdBackups`
   - Set `operator.openshift.io/etcd-backup` finalizer if missing
   - Create a backup Job
      - Job name is set to a deterministic name computed from the name of `EtcdBackup` with a hash suffix
         - Name used as a lock to prevent duplicate job creation in the case of stale informer cache
      - Set `operator.openshift.io/etcd-backup` finalizer
         - Ensures the controller gets a chance to sync the Job’s status and termination-log to the `EtcdBackup` before the GC cleans it up
      - Set `ttlSecondsAfterFinished` to automatically cleanup old jobs once they have been finalized
      - Set OwnerReference to `EtcdBackup`
         - Kubernetes allows namespace-scoped -> cluster-scoped owner ref for automatic cleanup
      - Assign node from etcdbackup.status.nodeName if set (Local backups) else set control plane node label selector
      - On completion, report metadata to termination log `{"files": [{"size": "1Gi", "path": "/backups/snapshot_<datetime>.db"}, {"size": "10Mi", "path": "/backups/static_kuberesources_<datetime>.tar.gz"}]} > /dev/termination-log`
   - When job finishes
      - On success
         - Get backup Pod and parse metadata from termination message
         - Update `EtcdBackup` status with `BackupCompleted` condition and backup files
      - On failure
         - Update `EtcdBackup` status with `BackupFailed` condition
      - Remove `operator.openshift.io/etcd-backup` finalizer from the Job

##### BackupQueueController

- Watches newly created `EtcdBackup` CRs that haven't been processed yet
- Sync `EtcdBackups`
   - Active backup names/nodes/pvcs are kept in memory in the queue controller to avoid making bad decisions based on stale informers. Each sync the expected state is reconciled against observed state to ensure the queue controller has an accurate view of active backups, with live API calls made in edge cases where it may have stale information.
   - Iterate queued backups in creation order (FIFO) and determine if they are eligible to start
      - Local backups use their assigned nodeName as the concurrency key. One Local backup per control plane node.
      - PVC backups use their PVC name as the concurrency key. One PVC backup per PVC.
   - When a backup can start, it is marked with the Pending condition, which tells the BackupController that it is ready to start.
      - Local backups that aren't already bound to a node are assigned to an available node, marking it as active in the internal cache
      - PVC backups are _not_ assigned to a node, they only mark their PVC as active. This allows the kube-scheduler to handle volume topology scheduling.

##### BackupGarbageCollectionController

- Watches deleted `EtcdBackup` CRs that have `operator.openshift.io/etcd-backup` finalizer
- Sync `EtcdBackups`
   - List GC jobs, sort into finished GC and active GC groups
   - List deleted `EtcdBackups`
      - Any with completed GC jobs can be updated to remove the `operator.openshift.io/etcd-backup` finalizer
      - Any that do not have active GC jobs are collected by storage backend
         - local/<nodename>
         - pvc/<pvcname>
   - Finalize completed GC jobs
      - EtcdBackups that have been GC'd are already finalized, jobs can be safely removed
   - Retry failed GC jobs
      - Requeue with exponential backoff using num retries from `operator.openshift.io/etcd-backup-gc-retry` annotation on the job
   - Create new GC jobs for storage backend groups collected earlier
      - A single GC job is created for each backend to minimize the number of pods per node for Local storage and reduce volume mount contention for PVC storage
      - Set `operator.openshift.io/etcd-backup` finalizer to ensure the controller can detect success/failure
      - Set `ttlSecondsAfterFinished` to automatically cleanup old jobs once they have been finalized
      - Set OwnerReferences on the job pointing to the set of `EtcdBackups` that it is garbage collecting
         - Limit max number of backups to GC at one time to 100. Extras will be GC'd after the current job completes
         - Owner references are used on job sync to determine which `EtcdBackups` were cleaned up by UID
      - Job name is generated deterministically using a hash of the storage backend type and nodeName/pvcName
         - Name is used as a lock to deduplicate GC jobs in the case of stale informer cache
      - Mount Local host paths or PVC paths for the set of backups being GC'd, and pass a list of the files to be deleted

#### Scheduled backups

Automated backup schedules may be configured with the new cluster-scoped CRD `operator.openshift.io/v1alpha1` `EtcdBackupPolicy`. Each EtcdBackupPolicy CR sets the cron schedule, retention, and storage location for etcd backups on a targeted set of control plane nodes.

##### BackupPolicyController

- Watches `EtcdBackupPolicy` and finished `EtcdBackup` CRs
- Sync `EtcdBackupPolicy` by name queue key
   - Check for active backups, sync with etcdbackuppolicy.status.active
      - If still active, skip current schedule
   - Parse schedule using robfig/cron and find the next schedule execution time
   - If schedule time is after now, create a new `EtcdBackup` CR based on the `EtcdBackupPolicy`
      - Label with `operator.openshift.io/etcd-backup-policy` to identify parent policy
      - Do NOT attach OwnerReference pointing to `EtcdBackupPolicy` from `EtcdBackup`
         - Backups should be retained when their schedule is deleted
         - Preferable to accidentally retain some extra backups rather than accidentally delete them
         - Cluster admin can delete by `operator.openshift.io/etcd-backup-policy` label if desired
      - `EtcdBackup` name is generated deterministically from `EtcdBackupPolicy` name, assigned node UID, and a minute hash (unixtime/60)
         - Name is used as a lock to prevent duplicate backup execution in the case of stale informer cache
         - Minute hash prevents backups from being created more often than once per minute
   - Backup handling passed off to the [backup controller](#backupcontroller) flow

The choice to use internal cron scheduling over a CronJob reduces the complexity of the kubernetes object model and gives the cluster-etcd-operator full control over scheduling, retries and failure. If CEO is down, then an external CronJob creating EtcdBackup CRs does not improve the availability of backups since the CEO still needs to reconcile them. When CEO is restored after an outage scheduling resumes immediately upon reconciling EtcdBackupPolicies, and any missed EtcdBackups are recreated without duplicates. Multiple missed scheduling times does not create multiple backups.

##### BackupPolicyRetentionController

- Watches `EtcdBackupPolicy` and finished `EtcdBackup` CRs
- Sync `EtcdBackupPolicy` by name queue key
   - List completed and failed `EtcdBackups` by `operator.openshift.io/etcd-backup-policy` label
   - Completed backups are pruned by the `EtcdBackupPolicy` retention rules
      - Rules:
         - MaxQuantity: Retain N etcdbackups per storage backend
         - MaxSize: Retain etcdbackups where sum of file size is less than the configured amount
      - Backups are ordered by age and pruned from the oldest first
   - Failed backups are pruned by the `EtcdBackupPolicy` failedBackupsHistoryLimit
      - Ordered by age, prune oldest first when failed count exceeds the history limit
   - Any `EtcdBackups` selected for pruning are deleted
      - Completed backups will still need to be finalized to remove files from the storage backend. Handled by the [backup garbage collection controller](#backupgarbagecollectioncontroller)

### API Extensions

#### EtcdBackup API

The `EtcdBackup` CRD for requesting one-time backups will be introduced to the API group-version `operator.openshift.io/v1alpha1`. The CRD would be feature-gated with the annotation `release.openshift.io/feature-set: TechPreviewNoUpgrade` until the prerequisite e2e test has an acceptable pass rate. See the Test Plan and Graduation Criteria sections for more details.

Finalizer `operator.openshift.io/etcd-backup` is added to `EtcdBackup` before the backup job is started.

```go
// EtcdBackup provides configuration options and status for a one-time backup attempt of the etcd cluster.
// When an EtcdBackup is deleted the files created by it will be deleted as well, as long as the storage backend is still accessible.
type EtcdBackup struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +required
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable"
	// +required
	Spec EtcdBackupSpec `json:"spec,omitzero"`
	// status holds observed values from the cluster. They may not be overridden.
	// +optional
	Status EtcdBackupStatus `json:"status,omitzero"`
}

type EtcdBackupSpec struct {
	// nodeSelector specifies which control plane nodes to select from for running the backup job. Only one backup job is run per EtcdBackup.
	// The default node-role.kubernetes.io/control-plane label will always be selected for in addition to any labels set here.
	// If no nodes are matched, then the backup will be marked failed.
	// For Local storage type, this may be used to target a specific node to take and store the backup.
	// For PVC storage type, this may be used to control where the backup is taken from.
	// When specified, must contain between 1 and 10 entries.
	// This field is optional.
	// +kubebuilder:validation:MinProperties=1
	// +kubebuilder:validation:MaxProperties=10
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// storage specifies the location where etcd backup files will be saved.
	// This field is required.
	// +required
	Storage EtcdBackupStorage `json:"storage,omitzero"`

	// --- TOMBSTONE ---
	// pvcName was the name of a PersistentVolumeClaim used to store the backup.
	// The field has been replaced by the storage field, which supports multiple types of storage backends including PVC.
	// The field name is reserved to prevent reuse.
	//
	// +optional
	// PVCName string `json:"pvcName"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'PVC' ? has(self.pvc) : !has(self.pvc)",message="pvc is required when type is PVC, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="self.type == 'Local' ? has(self.local) : !has(self.local)",message="local is required when type is Local, and forbidden otherwise"
// +union
type EtcdBackupStorage struct {
	// type of storage backend to use for storing the etcd backup.
	// Allowed values are Local and PVC.
	// When set to Local, the backup will be saved to a host path directory on the control plane node it was taken on.
	// When set to PVC, the backup will be saved to a volume bound by a PersistentVolumeClaim.
	// This field is required.
	// +kubebuilder:validation:Enum=PVC;Local
	// +required
	// +unionDiscriminator
	Type EtcdBackupStorageType `json:"type,omitempty"`

	// pvc specifies the PersistentVolumeClaim (PVC) which binds a PersistentVolume where the etcd backup file will be saved.
	// The PVC must always be created in the "openshift-etcd" namespace.
	// This field is required when the storage type is "PVC", and forbidden otherwise.
	// +optional
	// +unionMember
	PVC EtcdBackupStoragePvc `json:"pvc,omitzero"`

	// local specifies a host path directory on the control plane node where the etcd backup file will be saved.
	// This field is required when storage type is "Local", and forbidden otherwise.
	// +optional
	// +unionMember
	Local EtcdBackupStorageLocal `json:"local,omitzero"`
}

// EtcdBackupStorageType is an enum of the supported storage backends for backup files
type EtcdBackupStorageType string

const (
	EtcdBackupStorageTypePVC   EtcdBackupStorageType = "PVC"
	EtcdBackupStorageTypeLocal EtcdBackupStorageType = "Local"
)

type EtcdBackupStoragePvc struct {
	// name is a reference to a PVC in the "openshift-etcd" namespace where the etcd backup file will be saved.
	// name must be between 1 and 253 characters and conform to RFC 1123 subdomain format:
	// lowercase alphanumeric characters, '-' or '.', starting and ending with alphanumeric characters.
	// This field is required.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="!format.dns1123Subdomain().validate(self).hasValue()",message="name must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"
	// +required
	Name string `json:"name,omitempty"`

	// path is a directory on the volume where the etcd backup file will be saved.
	// When present, path must be an absolute filepath between 1 and 2048 characters containing only alphanumeric characters, '/', '.', '_', or '-', starting with a '/'.
	// This field is optional.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:XValidation:rule=`self.matches('^/[a-zA-Z0-9/._-]+$')`,message="path must be a valid absolute filepath containing only alphanumeric characters, '/', '.', '_', or '-', starting with a '/'"
	// +optional
	Path string `json:"path,omitempty"`
}

type EtcdBackupStorageLocal struct {
	// hostPath is a local directory on the control plane node where the etcd backup file will be saved.
	// hostPath must be an absolute filepath between 1 and 2048 characters containing only alphanumeric characters, '/', '.', '_', or '-', starting with a '/'.
	// This field is required.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:XValidation:rule=`self.matches('^/[a-zA-Z0-9/._-]+$')`,message="hostPath must be a valid absolute filepath containing only alphanumeric characters, '/', '.', '_', or '-', starting with a '/'"
	// +required
	HostPath string `json:"hostPath,omitempty"`
}

// +kubebuilder:validation:MinProperties=1
type EtcdBackupStatus struct {
	// conditions provide details on the status of the etcd backup job.
	// When specified, must contain between 1 and 5 entries, with no more than 1 of each type.
	//
	// Supported conditions include Pending, Running, Completed, Failed, and GarbageCollectionRequired.
	// If Pending is true, the backup is ready to be started.
	// If Running is true, the backup job is currently active.
	// If Completed is true, the backup job succeeded and has saved backup files to its storage location.
	// If Failed is true, the backup job failed and did not create valid backup files.
	// If GarbageCollectionRequired is true or unset, then files may exist which require cleanup when the EtcdBackup is deleted.
	// This field is optional.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// jobName is a reference to the Job created for the backup. It always runs in the openshift-etcd namespace.
	// Must be between 1 and 253 characters and conform to RFC 1123 subdomain format:
	// lowercase alphanumeric characters, '-' or '.', starting and ending with alphanumeric characters.
	// This field is optional.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="!format.dns1123Subdomain().validate(self).hasValue()",message="jobName must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"
	// +optional
	JobName string `json:"jobName,omitempty"`

	// nodeName is used for Local backups to bind the control plane node where the snapshot will be taken.
	// Must be between 1 and 63 characters and conform to RFC 1123 subdomain format:
	// lowercase alphanumeric characters, '-' or '.', starting and ending with alphanumeric characters.
	// This field is optional.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="!format.dns1123Subdomain().validate(self).hasValue()",message="nodeName must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// files tracks the path and size of files generated by the etcd backup.
	// Includes both etcd snapshots and static manifests.
	// When specified, must contain between 1 and 10 entries.
	// This field is optional.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +listType=map
	// +listMapKey=path
	// +optional
	Files []EtcdBackupFile `json:"files,omitempty"`
}

type EtcdBackupFile struct {
	// path to the backup file on the storage backend.
	// Must be an absolute filepath between 1 and 4096 characters containing only alphanumeric characters, '/', '.', '_', or '-', starting with a '/'.
	// This field is required.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:XValidation:rule=`self.matches('^/[a-zA-Z0-9/._-]+$')`,message="path must be a valid absolute filepath"
	// +required
	Path string `json:"path,omitempty"`

	// sizeBytes is the size of the backup file on the storage backend in bytes.
	// If omitted, then the file specified by path is an empty file.
	// When specified, must be between 1 and MaxInt64.
	// This field is optional.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=9223372036854775807
	// +optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// BackupConditionType enumerates the Condition types added to EtcdBackupStatus at different points in its lifecycle
type BackupConditionType string

var (
	// BackupPending means the backup is ready to start.
	BackupPending BackupConditionType = "Pending"
	// BackupPending means the backup job has started.
	BackupRunning BackupConditionType = "Running"
	// BackupCompleted means the backup completed successfully.
	BackupCompleted BackupConditionType = "Completed"
	// BackupFailed means the backup failed.
	BackupFailed BackupConditionType = "Failed"
	// BackupGarbageCollectionRequired indicates whether or not garbage collection is required
	// on a failed backup to cleanup partially created files.
	BackupGarbageCollectionRequired BackupConditionType = "GarbageCollectionRequired"
)

// BackupConditionReason enumerates the Condition reasons associated with BackupConditionTypes
type BackupConditionReason string

var (
	// BackupReasonReadyToStart means the backup has been queued to start.
	BackupReasonReadyToStart BackupConditionReason = "ReadyToStart"

	// BackupReasonJobStarted means the backup job is currently running.
	BackupReasonJobStarted BackupConditionReason = "JobStarted"

	// BackupReasonJobCompleted means the backup job completed successfully.
	BackupReasonJobCompleted BackupConditionReason = "JobCompleted"

	// BackupReasonPVCNotFound means the backup failed due to a missing PVC.
	BackupReasonPVCNotFound BackupConditionReason = "PVCNotFound"
	// BackupReasonNotNotFound means the backup failed due to a missing node.
	BackupReasonNodeNotFound BackupConditionReason = "NodeNotFound"
	// BackupReasonJobFailed means the backup job failed.
	BackupReasonJobFailed BackupConditionReason = "JobFailed"
	// BackupReasonDeleted means the backup was deleted while in progress.
	BackupReasonDeleted BackupConditionReason = "Deleted"

	// BackupReasonFilesPartiallyCreated means the backup job failed after partially creating some files.
	BackupReasonFilesPartiallyCreated BackupConditionReason = "FilesPartiallyCreated"
	// BackupReasonFilesNotCreated means the backup job failed without creating any files.
	BackupReasonFilesNotCreated BackupConditionReason = "FilesNotCreated"
	// BackupReasonFileStateUnknown means the backup job failed without indicating if it had created any files.
	BackupReasonFileStateUnknown BackupConditionReason = "FileStateUnknown"
)
```

#### EtcdBackupPolicy API

The `EtcdBackupPolicy` CRD for requesting an automated backup schedule will be introduced to the API group-version `operator.openshift.io/v1alpha1`. The CRD would be feature-gated with the annotation `release.openshift.io/feature-set: TechPreviewNoUpgrade` until the prerequisite e2e test has an acceptable pass rate. See the Test Plan and Graduation Criteria sections for more details.

```go
// EtcdBackupPolicy sets an automated schedule for taking backups of the etcd cluster.
type EtcdBackupPolicy struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +required
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +required
	Spec EtcdBackupPolicySpec `json:"spec,omitzero"`
	// status holds observed values from the cluster. They may not be overridden.
	// +optional
	Status EtcdBackupPolicyStatus `json:"status,omitzero"`
}

type EtcdBackupPolicySpec struct {
	// schedule sets the backup schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	// The value must be one of the following forms:
	// a predefined macro "@yearly", "@annually", "@monthly", "@weekly", "@daily" or "@hourly";
	// an "@every <duration>" expression where duration is a sequence of decimal numbers each with
	// a unit of "ns", "us", "ms", "s", "m" or "h" (for example "@every 2h" or "@every 1h30m");
	// or a standard cron expression of five space separated fields (minute, hour, day-of-month,
	// month and day-of-week), where each field may only contain digits, the names used for months
	// and days of the week, and the cron special characters "*", "/", "-" and ",".
	// This validation only checks the overall shape of the value; the individual field values are
	// validated when the schedule is parsed by the controller, which reports parsing failures via
	// the schedule's status condition.
	// Must be between 1 and 1024 characters long.
	// This field is required.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	// +kubebuilder:XValidation:rule="self.matches('^(@(annually|yearly|monthly|weekly|daily|hourly)|@every (\d+(\.\d+)?(s|m|h))+|(([-0-9*/,A-Za-z]+ ){4}[-0-9*/,A-Za-z]+))$')",message="must be a valid cron expression, or macro"
	// +required
	Schedule string `json:"schedule,omitempty"`

	// nodeSelector specifies which control plane nodes to select from for running backup jobs.
	// The default node-role.kubernetes.io/control-plane label will always be required in addition to any labels set here.
	// If no nodes are matched, then no EtcdBackups will be created.
	// For Local storage type, an EtcdBackup will be created for each selected control plane node every time the schedule is triggered. This is a special case to provide some resiliancy in the event of control plane node loss.
	// For PVC storage type, a single EtcdBackup will be created with the given nodeSelector every time the schedule is triggered.
	// When specified, must contain between 1 and 10 entries.
	// This field is optional.
	// +kubebuilder:validation:MinProperties=1
	// +kubebuilder:validation:MaxProperties=10
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// storage specifies the location where etcd backup files will be saved.
	// This field is required.
	// +required
	Storage EtcdBackupStorage `json:"storage,omitzero"`

	// retentionRules defines the policy for retaining and deleting existing backups.
	// Backups are deleted from the oldest first until all rules are satisfied.
	// If no rules are specified then backups created by this policy will not be automatically deleted.
	// When an EtcdBackup is deleted the files created by it will be deleted as well, as long as the storage backend is still accessible.
	// When specified, must contain between 1 and 2 entries.
	// Must not have more than 1 entry of each rule type.
	// This field is optional.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=2
	// +listType=map
	// +listMapKey=type
	// +optional
	RetentionRules []EtcdBackupPolicyRetentionRule `json:"retentionRules,omitempty"`

	// failedBackupsHistoryLimit defined the number of failed etcdbackups to retain. Failed backups are deleted from the oldest first.
	// Must be non-negative integer.
	// If set to to 0, then failed backups will be deleted immediately.
	// If unset, defaults to 1.
	// Must be a positive integer no greater than 1000.
	// This field is optional.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	FailedBackupsHistoryLimit *int32 `json:"failedBackupsHistoryLimit,omitempty"`
}

// +union
// +kubebuilder:validation:XValidation:rule="(self.type == 'MaxQuantity') ? has(self.maxQuantity) : !has(self.maxQuantity)",message="maxQuantity is required when type is MaxQuantity, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="(self.type == 'MaxTotalSizeGb') ? has(self.maxTotalSizeGb) : !has(self.maxTotalSizeGb)",message="maxTotalSizeGb is required when type is MaxTotalSizeGb, and forbidden otherwise"
type EtcdBackupPolicyRetentionRule struct {
	// type defined which rule field is set
	// Allowed values are MaxQuantity and MaxTotalSizeGb.
	// When set to MaxQuantity, this rule deletes old EtcdBackups when the quantity is exceeded.
	// When set to MaxTotalSizeGb, this rule deletes old EtcdBackups when the total aggregate size of backup files exceeds the value.
	// This field is required.
	// +unionDiscriminator
	// +kubebuilder:validation:Enum:=MaxQuantity;MaxTotalSizeGb
	// +required
	Type EtcdBackupPolicyRetentionRuleType `json:"type,omitempty"`

	// maxQuantity enforces the deletion of backups that exceed the given count.
	// Must be between 1 and 1000.
	// This field is required when the rule type is "MaxQuantity", and forbidden otherwise.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +optional
	MaxQuantity int32 `json:"maxQuantity,omitempty"`

	// maxTotalSizeGb enforces the deletion of backups by the total size of backups on the storage backend (in GB).
	// This is a soft threshold. The total size of backups may temporarily exceed the limit when new backups are created.
	// Must be between 1 and 10000.
	// This field is required when the rule type is "MaxTotalSizeGb", and forbidden otherwise.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10000
	// +optional
	MaxTotalSizeGb int32 `json:"maxTotalSizeGb,omitempty"`
}

type EtcdBackupPolicyRetentionRuleType string

const (
	EtcdBackupPolicyRetentionRuleMaxQuantity    EtcdBackupPolicyRetentionRuleType = "MaxQuantity"
	EtcdBackupPolicyRetentionRuleMaxTotalSizeGb EtcdBackupPolicyRetentionRuleType = "MaxTotalSizeGb"
)

// +kubebuilder:validation:MinProperties=1
type EtcdBackupPolicyStatus struct {
	// conditions provide details on the status of the etcd backup policy.
	// When specified, must contain between 1 and 3 entries, with no more than 1 of each type.
	//
	// Supported conditions include Error, LastScheduled, and LastSuccessfulTime.
	// If Error is true, the EtcdBackupPolicy is invalid and the condition reason and message will include details on the error.
	// If LastScheduled is true, the most recently scheduled EtcdBackup was created at the condition's lastTransitionTime.
	// If LastSuccessfulTime is true, the most recently completed EtcdBackup finished at the condition's lastTransitionTime.
	// This field is optional.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=3
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// active is a list of references to in progress backups controlled by this policy
	// When specified, must contain between 1 and 20 entries.
	// This field is optional.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=20
	// +listType=map
	// +listMapKey=name
	// +optional
	Active []EtcdBackupReference `json:"active,omitempty"`
}

type EtcdBackupReference struct {
	// name of the backup
	// Must be between 1 and 253 characters and conform to RFC 1123 subdomain format:
	// lowercase alphanumeric characters, '-' or '.', starting and ending with alphanumeric characters.
	// This field is required.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="!format.dns1123Subdomain().validate(self).hasValue()",message="name must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"
	// +required
	Name string `json:"name,omitempty"`
	// uid of the backup.
	// Must be a valid UUID formatted string with 36 characters.
	// This field is required.
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	// +kubebuilder:validation:Format=uuid
	// +required
	UID string `json:"uid,omitempty"`
}

type EtcdBackupPolicyConditionType string

const (
	// BackupPolicyError indicates that something is invalid on the EtcdBackupPolicy.
	BackupPolicyError EtcdBackupPolicyConditionType = "Error"
	// BackupPolicyLastScheduled is set true with the timestamp that the last scheduled backup was triggered.
	BackupPolicyLastScheduled EtcdBackupPolicyConditionType = "LastScheduled"
	// BackupPolicyLastSuccessfulTime is set true with the timestmap that the last scheduled backup completed successfully.
	BackupPolicyLastSuccessfulTime EtcdBackupPolicyConditionType = "LastSuccessfulTime"
)

type EtcdBackupPolicyConditionReason string

const (
	// BackupPolicyInvalidSchedule is set when parsing the schedule fails.
	BackupPolicyInvalidSchedule EtcdBackupPolicyConditionReason = "InvalidSchedule"
	// BackupPolicyInvalidSelector is set when no control plane nodes are selected by the nodeSelector.
	BackupPolicyInvalidSelector EtcdBackupPolicyConditionReason = "InvalidNodeSelector"
)
```

#### Examples

```yaml
# On demand backup of a single control plane node
kind: EtcdBackup
apiVersion: operator.openshift.io/v1alpha1
metadata:
   name: backup-2026-05-12
spec:
   nodeSelector:
      kubernetes.io/hostname: control-plane-1
   storage:
       type: Local
       local:
           hostPath: /path/to/etcdbackups
```

```yaml
# Store daily backups on each control plane node, retain 2 per node
kind: EtcdBackupPolicy
apiVersion: operator.openshift.io/v1alpha1
metadata:
   name: daily-local-backups
spec:
   schedule: "@daily"
   retentionRules:
   - type: MaxQuantity
     maxQuantity: 2
   storage:
       type: Local
       local:
           hostPath: /path/to/etcdbackups
---
# Store hourly backups from labeled control plane node on a PVC
kind: EtcdBackupPolicy
apiVersion: operator.openshift.io/v1alpha1
metadata:
   name: hourly-pvc-backups
spec:
   schedule: "@hourly"
   nodeSelector:
       node-role.kubernetes.io/control-plane: ""
       my.custom/label: "somevalue"
   retentionRules:
   - type: MaxQuantity
     maxQuantity: 6
   storage:
       type: PVC
       pvc:
           name: etcd-backups
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This API is designed for stanalone OCP only, it will not be used for Hypershift.

#### Standalone Clusters

This API is designed for full-cluster backup of standalone clusters.

#### Single-node Deployments or MicroShift

#### OpenShift Kubernetes Engine

### Implementation Details/Notes/Constraints [optional]

With the above APIs, the following controllers are required to save/delete backups:
- BackupController: to reconcile `operator.openshift.io EtcdBackup` CRs for one-time backups
- BackupGarbageCollectionController to run cleanup jobs on the storage backend when an EtcdBackup is deleted
- BackupPolicyController to reconcile `operator.openshift.io EtcdBackupPolicy` CRs for scheduled backups.
- BackupPolicyRetentionController to delete EtcdBackups that are managed by an EtcdBackupPolicy

#### Executing the backup cmd

The BackupController will add the `operator.openshift.io/etcd-backup` Finalizer to `EtcdBackup` and then create a [backup Job](https://github.com/openshift/cluster-etcd-operator/blob/master/bindata/etcd/cluster-backup-job.yaml) that runs the existing [backup script](https://github.com/openshift/cluster-etcd-operator/blob/master/bindata/etcd/cluster-backup.sh) to save the etcd snapshot and static pod manifests to the desired location.
Since the existing bash script is harder to maintain and test, in the future this can be changed to use the etcd Go client to save the snapshot. See [#1099](https://github.com/openshift/cluster-etcd-operator/pull/1099).

On completion the backup Job writes file size and path as JSON to `/dev/termination-log`, allowing the BackupController to fill in metadata on the associated `EtcdBackup`. This approach allows basic communication from the backup job to the controller without providing RBAC privileges to the job itself, and ensuring that only a single process is responsible for updating `EtcdBackup` status.

#### Backup storage

When choosing where to save backups for both scheduled and one-time backups the user can specify different locations for files to be saved. With the `PVC` storage type they can save backups to any CSI compatible remote storage solution (e.g `nfs` type PV). They may also choose to use the `Local` storage type in order to store backups directly on the control plane node at a given hostPath. In future, additional storage types may be added to support different remote backends like cloud object storage, however PVC natively enables any storage locaion supported by a CSI driver so special handling is not needed for the first pass.

In EtcdBackupPolicy:
- `Local` storage will automatically create backups for each selected control plane node each time the schedule is triggered. This allows it to provide a reasonably resiliant backup solution in the event that one or more control plane nodes is permanently lost. As long as one control plane node remains the cluster can be restored.
- `PVC` storage will select a single healthy control plane node to create a backup on each time the schedule is triggered. This provides a more resiliant backup location, all control plane nodes could be lost and the cluster can still be restored as long as the backing volume still exists.
   - There is no special handling for HostPath PVCs to run backups on every control plane node. `Local` storage should be used to handle this case.

#### Retention Limits

As outlined in the `operator.openshift.io EtcdBackupPolicy` API, the operator will support the following kinds of retention rules to ensure it doesn't exhaust the available space for saving backups:
- Quantity: Specify the maximum number of backups to retain
- Size: Specify the maximum size on disk of backups to retain

Additionally, there is a spec.failedBackupsHistoryLimit parameter that controls how many failed EtcdBackups to retain, similar to the failedJobsHistoryLimit on CronJobs, to prevent too many failures from piling up. This field will default to 1.

Backups will be deleted in order from oldest to newest until all limits are satisfied. Backup deletion is managed by the BackupPolicyRetentionController via the k8s API. Deleting an EtcdBackup signals the BackupGarbageCollectionController to delete the associated file from its storage backend.

In future, additional retention rules types can be added such as MaxAge.

#### Garbage Collection

The Jobs created for each EtcdBackup will be cleaned up by setting ttlSecondsAfterFinished. Finalizer on the Job prevents it from being deleted before the controller is able to update state on the associated EtcdBackup.

EtcdBackups that are manually created will not be GC'd, but those created by a EtcdBackupPolicy will be deleted based on the configured retention rules.

When an EtcdBackup is deleted, its Finalizer will prevent the object from being removed until the BackupGarbageCollectionController runs. The GC controller will check the status of the EtcdBackup, and if it has a completed backup file then it will create a job to mount storage and delete the file. Once this job is complete the Finalizer can be removed and the EtcdBackup object will finish deleting.

### Metrics

The following metrics will be added for etcd backup observability

#### EtcdBackup metrics

| Metric Name | Type | Description | Labels
| :--- | :--- | :--- | :--- |
| `etcd_backup_info` | Gauge | Information about etcd backup | `etcd_backup`=\<etcd-backup-name\> <br> `uid`=\<etcd-backup-uid\> <br> `node`=\<node-name\> <br> `storage_type`=Local\|PVC <br> `storage_location`=\<pvc-name / path\> <br> `created_by_policy`=\<etcd-backup-policy-name\> |
| `etcd_backup_labels` | Gauge | Kubernetes labels converted to Prometheus labels | `etcd_backup`=\<etcd-backup-name\> <br> `uid`=\<etcd-backup-uid\> <br> `label_ETCD_BACKUP_LABEL`=\<label-value\> <br> |
| `etcd_backup_status` | Gauge | The current status of the backup. Value of 1 indicates the labeled `status` is active. | `etcd_backup`=\<etcd-backup-name\> <br> `uid`=\<etcd-backup-uid\> <br> `status`=Pending\|Completed\|Failed <br> |
| `etcd_backup_completion_time` | Gauge | Unix timestamp when the backup completed. | `etcd_backup`=\<etcd-backup-name\> <br> `uid`=\<etcd-backup-uid\> <br> |
| `etcd_backup_start_time` | Gauge | Unix timestamp when the backup started. | `etcd_backup`=\<etcd-backup-name\> <br> `uid`=\<etcd-backup-uid\> <br> |
| `etcd_backup_size_bytes` | Gauge | The file size of the backup snapshot. | `etcd_backup`=\<etcd-backup-name\> <br> `uid`=\<etcd-backup-uid\> <br> |

#### EtcdBackupPolicy metrics

| Metric Name | Type | Description | Labels
| :--- | :--- | :--- | :--- |
| `etcd_backup_policy_info` | Gauge | Information about etcd backup policy | `etcd_backup_policy`=\<etcd-backup-policy-name\> <br> `uid`=\<etcd-backup-policy-uid\> <br> `schedule`=\<schedule\> <br> `timezone`=\<timezone\> <br> `storage_type`=Local\|PVC <br> `storage_location`=\<pvc-name / path\> <br> |
| `etcd_backup_policy_labels` | Gauge | Kubernetes labels converted to Prometheus labels | `etcd_backup_policy`=\<etcd-backup-policy-name\> <br> `uid`=\<etcd-backup-policy-uid\> <br> `label_ETCD_BACKUP_POLICY_LABEL`=\<label-value\> <br> |

### Risks and Mitigations

#### Local storage vs. local PV

When backups are configured to be saved to a PVC backed by local disk, they will all be saved on a singular control plane node. By using the `Local` storage type instead backups can be spread across all control plane nodes with retention managed on a per-node basis. If one or more nodes are lost, backups can be retrieved from any of the remaining control plane nodes.

#### Remote Storage PVC

To decouple backup storage from the control plane nodes, any available CSI driver may be used to provision a PVC as the storage backend. In this case retention is managed per-volume. If multiple nodes are backing up to a single volume and a retention rule MaxQuantity of 2 is used, then only the 2 most recent node backups will be stored.

#### Restore from old backup

When restoring from an older backup, newer `EtcdBackup` CRs will be lost even though their files still exist on their storage backend. This results in orphaned backup files which will no longer be managed by `EtcdBackupPolicy` retention rules.

In future there could be an automated discovery process that finds files on the storage backend of an `EtcdBackupPolicy` and generates an `EtcdBackup` for each. This will be left for a future enhancement, and for now this edge case will be clearly documented.

### Drawbacks

- EtcdBackupPolicy physical file deletion via retention mechanism is disconnected from backup creation since it is managed via the k8s API. A separate job is spawned to mount the storage backend and delete files. In the case of a ReadWriteOnce volume used for backup storage this could cause contention on mounting the volume onto the backup job and the cleanup job.
   - Backup GC should be a relatively fast operation, so contention should be minimal. However we may need a concurrency limiting queue to manage backup execution anyway (prevent spawning 100 backups on a single node at once, for instance), so this should also be considered.
   - Velero uses an in-memory queue and promotes backups to "ReadyToStart" one at a time. A similar approach may be used here.

## Design Details

### Open Questions

- Creating an EtcdBackup could allow a user to escape their namespace and privileges by creating an EtcdBackup pointing to a PVC that they control.
   - This would require RBAC access to EtcdBackup and PVC in the openshift-etcd namespace, we don't allow storing to arbitrary PVCs in the cluster
   - Still effectively a privelege escalation route, turn 2 RBAC privileges into GET *
   - Docs will explicitly mention that interacting with this API is a privileged operation and should not be delegated to non-cluster-admins.

- The [manual backup provisioning docs](https://docs.redhat.com/en/documentation/openshift_container_platform/4.4/html/backup_and_restore/backup-etcd) mention that the `static_kuberesources_<datetimestamp>.tar.gz` file contains the encryption keys for the etcd snapshot when etcd encryption is enabled
   - "it is recommended to store this second file separately from the etcd snapshot for security reasons"
   - In the current implementation, both files are stored side-by-side. Should we handle this differently?

## Test Plan

Prior to merging changes to the backup controller implementation, existing e2e recovery tests will be adapted to test the validity of the backups generated from the automated backup feature. The pass rate on this test will feature gate the API until we pass an acceptable threshold to make it available by default.

Backup and restore procedure test:
- Start with a cluster that has the `EtcdBackups` and `EtcdBackupPolicy` APIs enabled
- Save a backup of the cluster on demand by using the `EtcdBackup` API
- Modify the cluster state post backup
- Step through the recovery procedure from the saved backup
- Ensure that the control-plane has recovered and is stable
- Validate the we have restored the cluster to the previous state so that it does not have the post-backup changes

Scheduled backup test:
- Start with a cluster that has the `EtcdBackups` and `EtcdBackupPolicy` APIs enabled
- Create an `EtcdBackupPolicy`
   - Schedule: "@every 1m"
   - Storage: Local
   - Retention: MaxQuantity 2
- Watch EtcdBackup changes
   - Expect to see 2 backups created and reach completed status
   - Expect to see older backups get garbage collected after new ones are completed

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

- Manage backup retention based only on files on the filesystem
   - If deletion of `EtcdBackup` is not tied to file deletion, cluster admins have less control
   - More difficult to associate files to their `EtcdBackupPolicy` without writing additional metadata to the FS or embedding data in file names
   - Multiple `EtcdBackupPolicies` writing to the same FS location could have conflicting retentions if their files aren't properly associated

- Use an existing open source backup solution such as OADP/Velero
   - The team decided to use a custom implementation to avoid requiring installation of an additional operator in the cluster. The primary goal of this enhancement is for backups to be usable and scheduled immediately at install time.
   - The CRDs in this enhancement provide basic full-cluster backup functionality. If additional features are needed then OADP/Veleroa can be installed by cluster admins.

- Cloud storage backends
   - For simplicity, and to avoid adding additional dependencies to CEO, support for cloud storage is handled indirectly through the PVC interface. Any CSI compatible storage driver may be used to create a PVC in the openshift-etcd namespace and then referenced by an EtcdBackup.
   - Future enhancements may discuess adding new storage types to EtcdBackup.

## Infrastructure Needed

The new API will be introduced in the `openshift/api` repo and the controller will be added to the existing cluster-etcd-operator in the `openshift/cluster-etcd-operator` repo.

[upgradebackupcontroller]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/pkg/operator/upgradebackupcontroller/upgradebackupcontroller.go#L284-L298
[backup pod]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/bindata/etcd/cluster-backup-pod.yaml#L31-L38
[backup script]: https://github.com/openshift/cluster-etcd-operator/blob/0584b0d1c8868535baf889d8c199f605aef4a3ae/bindata/etcd/cluster-backup.sh#L121-L129
