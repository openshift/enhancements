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

To enable automated etcd backups of an Openshift cluster from day one after installation, 
without provided configuration from the user.

## Motivation

The current [automated backup of etcd](https://docs.openshift.com/container-platform/4.15/backup_and_restore/control_plane_backup_and_restore/backing-up-etcd.html#creating-automated-etcd-backups_backup-etcd) of an OpenShift
cluster relies on user provided configurations. 

To improve the customers experience, this work proposes taking etcd backup using default config from day one, without additional configuration from the user.

### Goals

* Backups should be taken without configuration after cluster installation from day 1.
* Backups are saved to a default PersistentVolume, that could be overridden by user.
* Backups are taken according to a default schedule, that could be overridden by user.
* Backups are taken according to a default retention policy, that could be overridden by user.

### Non-Goals

* Save cluster backups to remote cloud storage (e.g. S3 Bucket).
  - This could be a future enhancement or extension to the API.
  - Having backups in an independent storage from OCP cluster is crucial for Disaster Recovery scenarios.
* Automate cluster restoration.
* Provide automated backups for non-self hosted architectures like Hypershift.

## Proposal

To enable automated etcd backups of an Openshift cluster, a default etcd backup option to be introduced. 
Using a `SideCar` container within each etcd pod, backups could be taken with no config from user.

### User Stories

* As a cluster administrator I would like OCP cluster backup to be taken without configuration.
* As a cluster administrator I would like to schedule recurring OCP cluster backups so that I have a recent cluster state to recover from in the event of quorum loss (i.e. losing a majority of control-plane nodes).
* As a cluster administrator I would like to have alerts upon failure to take OCP cluster backup.

### API Extensions

No [API](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/automated-backups.md#api-extensions) changes are required, since this approach work independently with default config.

### Topology Considerations
TBD
#### Hypershift / Hosted Control Planes
TBD
#### Standalone Clusters
TBD
#### Single-node Deployments or MicroShift
TBD
### Workflow Description
TBD

## Design Details

A `SideCar` container within each etcd pod in Openshift cluster is being added in order to take backups without any provided configuration from user's perspective.

In order to alleviate the overhead on etcd server, the side container takes the backup by simply copying the snapshot file.
This approach is being used by `microshift`, and it has minimal overhead as it is a simple file system copy operation.
Since the `SideCar` is being deployed alongside each etcd cluster member, it is possible to keep backups across all master nodes.

On the other hand, the backups may **not** be up-to-date since the snapshot might be lagging behind the `WAL`. Therefore, it is recommended to use this approach alongside the Automated Backup enabled using the `EtcdBackup` CR.
Since this work will be enabled with no configuration, it is possible to use define a default values for the `Scheule`, `Retention` independently.

Since cluster backups must be stored on a persistent storage, and each storage solution has pros and cons, several approaches that have been investigated. See [Alternatives](#Alternatives)

This work utilizes `hostPath` approach, since the `SideCar` container has access to the etcd pod's file system.

### Open Questions

This enhancement adds on previous work [automated backup of etcd](https://docs.openshift.com/container-platform/4.15/backup_and_restore/control_plane_backup_and_restore/backing-up-etcd.html#creating-automated-etcd-backups_backup-etcd).
Therefore, it is vital to use the same feature gate for both approaches. The following issues need to be clarified before implementation

* How to distinguish between `NoConfig` backups and configs that are triggered using `EtcdBackup` CR.
* Upon enabling the `AutomatedBackup` feature gate, which approach should be used and according to what criteria.
* How should the current backup controller distinguish between both approaches.
* How to disable the `NoConfig` behaviour, if a user want to. 
  * re-deploy the etcd deployment without the `backupnoconfig` sidecar upon `automatedbackup` CRD installation.
  * add annotation that disables the sidecar upon `automatedbackup` CRD installation. 

### Implementation Details/Notes/Constraints

Need to agree on a default schedule and retention policy to be used by the NoConfig independently of any `EtcdBackup` CR.

### Risks and Mitigations

When the backups are configured to be saved to a `local` type `PV`, the backups are all saved to a single master node where the `PV` is provisioned on the local disk.

In the event of the node becoming inaccessible or unscheduled, the recurring backups would not be scheduled. The periodic backup config would have to be recreated or updated with a different `PVC` that allows for a new `PV` to be provisioned on a node that is healthy.

If we were to use `localVolume`, we need to find a solution for balancing the backups across the master nodes, some ideas could be

- create a PV for each master node using `localVolume` and the backup controller from CEO should take care of balancing the backups across the `available` and `healthy` volumes.
- the controller should keep the most recent backup on a healthy node available for restoration.
- the controller should skip an unhealthy master node from taking a backup

## Test Plan

An e2e test will be added to practice the scenario as follows 

- Enable AutomatedBackupNoConfig FeatureGate.
- Verify that a `Etcdbackup` CR has been installed.
- Verify that a `PV`, `PVC` has been created.
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

### Drawbacks

## Alternatives

**Ideally**, the storage solution to use would have the following characteristics
- Backups should be taken on a round-robin fashion on each master node to avoid overwhelming a specific etcd member more than others.
- Spreading the backups among all etcd cluster members provide guarantees for disaster recovery in case of losing two members at the same time.
- One fits all storage solution, to be used across all Openshift variants including Bare metal and Single Node Openshift.

Below is each storage solution, with its pros and cons.

### Dynamic provisioning using CSI

As Openshift cluster that are based on cloud infrastructure offers dynamic storage provisioning capabilities based on CSI, allocating persistent storage for the etcd backups is trivial.

Depending on the storage capabilities offered by the cloud provider, the allocated persistent volume could be attached to any of the master nodes and the backups can be used by any etcd member on any of the master nodes for restoration.

* Pros
  - Utilising CSI storage is ideal as it is well tested and allocating storage required defining a `PVC` manifest only.
  - Some storage solutions based on CSI allows a `PV` to be attached to multiple nodes. This is ideal for both taking backup and restoration from various master nodes, in case one of the nodes is unreachable.
  - It allows recovery even on disaster scenarios, where no master node is accessible, since the storage is allocated on the provider side.
* Cons
  - CSI is not an option for OCP `BM` or `SNO`.
  - Some cloud providers allocate storage to be accessible only within the same availability zone. Since Openshift cluster allocate each master node on a different zone for high availability purposes, the backup will not be accessible if the master node where the backup was taken is no longer accessible.

### Local Storage

Relying on local storage works for all Openshift variants (i.e. cloud based, SNO and BM). There are three possible approaches using local storage, each is detailed below.

#### Local Storage Operator

Utilising `local storage operator` is a proper solution for all Openshift variants. However, installing a whole operator is too much overhead.

#### Local volumes

A `localVolume` mounts a path from the node's file system tree or a separate disk as a volume into a Pod.

Utilising a `local StorageClass` with no provisioner, guarantees a behaviour similar to dynamic provisioning.
Upon creating a `PVC` with the `local StorageClass`, a `PV` will be allocated using `localVolume` and bound to the `PVC`.
Since the `PV` is actually a path on the node's file system, the etcd backup will be always available regardless of the Pod's status, as long as the node itself is still accessible.
using `localVolume` and the node affinity within the `PV` manifest forces the backup pod to be scheduled on a specific node, where the volume is attached.
Had the Pod been replaced, the new Pod is guaranteed to be scheduled on the same node, where the backup exists.
This deterministic scheduling behaviour is a property of `localVolume`, which is not the case with `hostPath` volumes.
`localVolume` is handled by the PV controller and the scheduler in different manner, in fact it was created to resolve issues with `hostPath`

* Pros
  - It supports all Openshift variants, including `SNO` and `BM`.
  - The Backup Pod will be scheduled always to this master node where the backup data exists.
  - Using a separate disk for backups (e.g. SSD) is possible, since local volume allows mounting it into a Pod.
  - Having the deterministic Pod scheduling is vital, since the restoration needs the recent backup in place to work.
* Cons
  - If the master node where the `PV` is mounted became unhealthy/unavailable/unreachable, then the backups are no longer accessible, also taking new backups is no longer possible.

#### Host Path volumes

A `hostPath` volume mounts a path from the node's file system as a volume into a Pod.

* Pros
  - It supports all Openshift variants, including `SNO` and `BM`.
* Cons
  - `hostPath` could have security impact as it exposes the node's filesystem.
  - No scheduling guarantees for the pod using `hostpath` as is the case with `localVolume`. The pod could be scheduled on a different node from where the hostPath volume exist.

### Hybrid approach

As shown above, dynamic provisioning is the best storage solution to be used, but it is not viable among all Openshift variants.
However, a hybrid solution is possible, in which dynamic provisioning is being used in cloud based Openshift, while utilising local storage for BM and SNO.
Relying on `infrastructure` resource type, we can create the storage programmatically according to the underlying infrastructure and Openshift variant.

### StatefulSet approach

A `StatefulSet` could be deployed among all master nodes, where each backup pod has its own `PV`. This approach has the pros of spreading the backups among all master nodes.
The complexity come from the fact that the backups are being triggered by a `CronJob` which spawn a `Job` to take the actual backup, by deploying a Pod.
Since StatefulSet manages its own Pods, it is not possible to schedule backups by a `CronJob`. However, it is possible to generate event by the `CronJob` which is being watched by the StatefulSet. Then the backups are being taken and controlled mainly by it.

## Infrastructure Needed
TBD