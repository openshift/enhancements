---
title: automated-backups-no-config
authors:
  - "@elbehery"
reviewers:
  - "@soltysh"
  - "@dusk125"
  - "@hasbro17"
  - "@tjungblu"
  - "@williamcaban"
approvers:
  - "@soltysh"
  - "@dusk125"
  - "@hasbro17"
  - "@tjungblu"
  - "@williamcaban"
api-approvers:
  - "@soltysh"
creation-date: 2024-06-17
last-updated: 2024-06-17
tracking-link:
  - https://issues.redhat.com/browse/ETCD-609
see-also:
  - "https://issues.redhat.com/browse/OCPSTRAT-1411"
  - "https://issues.redhat.com/browse/OCPSTRAT-529"
---


# Automated Backups of etcd with No Config

## Summary

Enable the automated backups of etcd from day 1 without provided config from the user.

## Motivation
The [current automated backup of etcd](https://docs.openshift.com/container-platform/4.15/backup_and_restore/control_plane_backup_and_restore/backing-up-etcd.html#creating-automated-etcd-backups_backup-etcd) of an OpenShift
cluster relies on user provided configurations. 

To improve the customers experience, this work proposes taking etcd backup using default config from day one, without additional configuration from the user.


### Goals
- Backups should be taken without configuration after cluster installation from day 1.
- Backups are saved to a default PersistentVolume, that could be overridden by user.
- Backups are taken according to a default schedule, that could be overridden by user.
- Backups are taken according to a default retention policy, that could be overridden by user.


### Non-Goals
- Save cluster backups to cloud storage e.g S3.
  - This could be a future enhancement or extension to the API.
- Automate cluster restoration.
- Provide automated backups for non-self hosted architectures like Hypershift.

### User Stories
- As a cluster administrator I want cluster backup to be taken without configuration.
- As a cluster administrator I want to schedule recurring cluster backups so that I have a recent cluster state to recover from in the event of quorum loss (i.e. losing a majority of control-plane nodes).
- As a cluster administrator I want to have failure to take cluster backups for more than a configurable period to be reported to me via critical alerts.


## Proposal
- Provide a default etcd backup configuration as a `EtcdBackup` CR.
- Provide a default reliable storage mechanism to save the backup data on.
- Provide a separate feature gate for the automated backup with default config (under discussion).


### Workflow Description
- The user will enable the AutomatedBackupNoConfig feature gate (under discussion).
- A `EtcdBackup` CR is being installed on the cluster.
- In a default config scenario, the use should not add any config.
- A PVC and PV are being created to save the backup data reliably.


### API Extensions
- No API changes are required. 
- See https://github.com/openshift/enhancements/blob/master/enhancements/etcd/automated-backups.md#api-extensions

### Topology Considerations
TBD
#### Hypershift / Hosted Control Planes
TBD
#### Standalone Clusters
TBD
#### Single-node Deployments or MicroShift
TBD

### Implementation Details/Notes/Constraints [optional]
- Need to agree on a default schedule.
  - We can add default value annotation to current `Etcdbackup Spec`.
- Need to agree on a default retention policy.
  - We can add default value annotation to current `Etcdbackup Spec`.

Example of default config

```Go
type EtcdBackupSpec struct {

        // +kubebuilder:default:=0 */2 * * *
	Schedule string `json:"schedule"`

        // +kubebuilder:default:=daily
	TimeZone string `json:"timeZone"`

        // +kubebuilder:default:=RetentionNumber
	RetentionPolicy RetentionPolicy `json:"retentionPolicy"`

	// This field will be populated programmatically according to the infrastructure type.
	// see discussion below
	// +optional
	PVCName string `json:"pvcName"`
}
```

- For `PVCName`, we can populate it programmatically according to the underlying infra. See [Design Details](#design-details).

### Risks and Mitigations

When the backups are configured to be saved to a `local` type PV, the backups are all saved to a single master node where the PV is provisioned on the local disk.

In the event of the node becoming inaccessible or unscheduled, the recurring backups would not be scheduled. The periodic backup config would have to be recreated or updated with a different PVC that allows for a new PV to be provisioned on a node that is healthy.

If we were to use `localVolume`, we need to find a solution for balancing the backups across the master nodes, some ideas could be

- create a PV for each master node using `localVolume` and the backup controller from CEO should take care of balancing the backups across the `available` and `healthy` volumes.
- the controller should keep the most recent backup on a healthy node available for restoration.
- the controller should skip an unhealthy master node from taking a backup


### Drawbacks

## Design Details

- Add default values to current `Etcdbackup Spec` using annotation.
- Maintain the reconciliation logic with Cluster-etcd-operator.

### Initial Storage Design proposal
- Several options exist for the default `PVCName`.
  - Relying on `dynamic provisioning` is sufficient, however not an option for `SNO` or `BM` clusters.
  - Utilising `local storage operator` is a proper solution, however installing a whole operator is too much overhead.
  - The most viable solution to cover all OCP variants is to use `local volume`.
    Please find below this solution's Pros & Cons.
    - Pros
      - The PV will be bound to a specific master node.
      - The Backup Pod will be scheduled always to this master node since it uses a PVC bound to this PV.
      - Using an SSD is possible, since local volume allows mounting it into a Pod.
      - Having the Backup Pod scheduling deterministic is vital, since the retention policy needs the recent backup in place to work.
    - Cons
      - If the master node where the PV is mounted became unhealthy/unavailable/unreachable etc.
      - The backups are no longer accessible, also taking new backups is no longer possible.
      - Ideally, the backups should be taken on a round-robin fashion on each master node to avoid overwhelming a specific etcd member more than others.
      - Also spreading the backups among all etcd cluster members provide guarantees for disaster recovery in case of losing two members at the same time.
      - However, using the `local volume` option over all the master nodes will be complicated and error prune.
      

### Final Storage Design proposal
Best `Storage` solution is using `hybrid` approach, based on the OCP variant infrastructure.

Based on the `infrastructure` resource type, we can create the storage programmatically according to the underlying infra.

#### Cloud based OCP

    Pros
    - Utilising `Dynamic provisioning` is best option on cloud based OCP.
    - Using the default `StorageClass` on the `PVC` manifest will provision a `PV`.
    - The backups on the `PV` will be always available even if the master node is unavailable.
    
    Cons
    -  There is no possibility to spread the backups over all the master nodes, since the `PV` is accessible within one and only one availability zone. 


#### Single-node and Bare metal OCP
    Only possible option is to use `local volume`. 

### Open Questions
How to use remote storage for backups on `BM` and `SNO`. 

## Test Plan

An e2e test will be added to practice the scenario as follows 

- Enable AutomatedBackupNoConfig FeatureGate.
- Verify that a `Etcdbackup` CR has been installed.
- Verify that a PV, PVC has been created.
- Verify that a backup has been taken.
- Verify that the backups are valid according to the retention policy.

## Graduation Criteria
TBD

### Dev Preview -> Tech Preview
TBD

### Tech Preview -> GA
TBD

### Removing a deprecated feature
TBD

## Upgrade / Downgrade Strategy
TBD

## Version Skew Strategy
TBD

## Operational Aspects of API Extensions
TBD

#### Failure Modes
TBD

## Support Procedures
TBD

## Implementation History
TBD

## Alternatives
### hostPath

Utilising `hostPath` volumes as a storage solution for the backups.

`hostPath` mounts a path from the node's file system as a volume into the pod.

It supports all OCP variants, including `SNO` and `BM`.  

However, I am strongly against using it for the following reasons

      - `hostPath` could have security impact as it exposes the node's filesystem.
      - No scheduling guarantees for the pod using `hostpath` as is the case with `localVolume`. The pod could be scheduled on a different node from where the hostPath volume exist.
      - On the other hand, using `localVolume` and the node affinity within the PV manifest forces the backup pod to be scheduled on a specific node, where the volume is attached.
      - `localVolume` allows using a separate disk as PV, unlike `hostPath` which mounts a folder from the node's FS.
      - `localVolume` is handled by the PV controller and the scheduler in different manner, in fact it was created to resolve issues with `hostPath`


### Side Car container
 - Another approach is to use a sidecar container within each etcd pod. This sidecar container can create the backup and save it to a backup.

 - This approach although trivial, is easier to maintain, specially to push backups to remote storage.


## Infrastructure Needed
TBD