---
title: automated-backups-no-config
authors:
  - "@elbehery"
reviewers:
  - "@JoelSpeed"
  - "@dusk125"
  - "@hasbro17"
  - "@tjungblu"
approvers:
  - "@hasbro17"
api-approvers:
  - "@JoelSpeed"
creation-date: 2024-06-17
last-updated: 2024-10-21
tracking-link:
  - https://issues.redhat.com/browse/ETCD-609
see-also:
  - "https://issues.redhat.com/browse/OCPSTRAT-1411"
  - "https://issues.redhat.com/browse/OCPSTRAT-529"
---


# Automated Backups of etcd with No Config

## Summary

To enable automated backups of Etcd database and cluster resources of an Openshift cluster from day one after installation, 
without provided configuration from the user.

This document outlines the possible approaches that were investigated, discussed and tested, in addition to the final implemented approach.  

## Motivation

The current method of **Backup** and **Restore** of an Openshift cluster is outlined in  [Automated Backups of etcd](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/automated-backups.md). 
This approach relies on user supplied configuration, in the form of `config.openshift.io/v1alpha1 Backup` CR.

To improve the user's experience, this work proposes taking etcd backup using default configurations from day one, without user interference.

Moreover, for recovery in disaster scenarios, where more than one control-plane node is lost from an Openshift cluster, this proposal attempts to store backups on all control-plane nodes.
Having backups of an Openshift cluster on all control-plane nodes, improves the recovery probability in disaster scenarios.

### User Stories

* As a cluster administrator, I would like an OCP cluster backups to be taken without user's supplied configuration from day one, after installation.
* As a cluster administrator, I would like to an OCP cluster backups available on all master nodes, so that I have a recent cluster state to recover from in the event of losing control-plane nodes.

### Goals

* Backups should be taken without configuration after cluster installation from day 1.
* Backups are saved to a default location on each master node's file system.
* Backups are taken according to a default schedule.
* Backups are being maintained according to a default retention policy.

### Non-Goals

* Save cluster backups to remote cloud storage (e.g. S3 Bucket).
  - This could be a future enhancement or extension to the API.
  - Having backups in an independent storage from OCP cluster is crucial for Disaster Recovery scenarios.
* Automate cluster restoration.
* Provide automated backups for non-self hosted architectures like Hypershift.

## Proposal

This work utilizes the [API](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/automated-backups.md#api-extensions) from current `CronJob` based implementation.
Moreover, it uses the same [PeriodicBackupController](https://github.com/openshift/cluster-etcd-operator/blob/master/pkg/operator/periodicbackupcontroller/periodicbackupcontroller.go#L81) as this proposal builds upon it, and also under the same feature gate.
The default configuration is created through `config.openshift.io/v1alpha1 Backup` CR, with name `default`, to distinguish it from other user supplied `Backup` CRs.
Using a CR enable users to override the default backup configuration.

By design, the _**No-Config**_ backup approach works _orthogonal_ to the [Cron](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/automated-backups) based method, and not a replacement.
However, this method is designed to work on all variants of Openshift (e.g. Single-node and Bare-metal), as well as having backups on all control-plane nodes, for recovering from disaster scenarios.

### API Extensions

This proposal uses the same [API](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/automated-backups.md#api-extensions) from the Cron based method.
No additional API changes are required.

### Topology Considerations
TBD
#### Hypershift / Hosted Control Planes
This proposal does not affect HyperShift.
#### Standalone Clusters
TBD
#### Single-node Deployments or MicroShift
TBD
### Workflow Description
TBD

## Design Details

To back up the Etcd datastore and cluster resources of an Openshift cluster for the purpose of restoration, a persistent storage must be utilised.
Several main point have been considered during designing the implementation of this proposal, however the storage solution to be used is the most influential, particularly to be used by all Openshift variants.  

### Storage

Ideally, the storage solution to use should have the following characteristics :-

- Backups should be taken on each master node to avoid overwhelming a specific etcd member more than others.
- Spreading the backups among all etcd cluster members provide guarantees for disaster recovery in case of losing two members at the same time.
- One fits all storage solution, to be used across all Openshift variants including Bare metal and Single Node Openshift.

According to these requirement, only `hostPath` and `localVolume` storage could be utilized. 
However, for `localVolume` storage, if the master node where the `PV` is mounted became unhealthy/unavailable/unreachable, then the backups are no longer accessible, also taking new backups is no longer possible.
Hence, `hostPath` has been used as it is the only viable option that could be utilized on all Openshift variants. 

### Workload

In order to take backups for Etcd data store and cluster resources, a workload must run on the cluster while having access to Etcd and cluster resources.
Since Etcd data store of an Openshift cluster runs on `Pod` similar to other workload, several constructs such as `Deployment`, `DaemonSet`, `StatefulSet` and `SideCar` could be used.

This section discusses two approaches (i.e. `SideCar` and `DaemonSet`), which are implemented and tested, explaining the Pros and Cons of each method.  
Before deciding upon these two methods, several other approaches have been investigated. See [Alternatives](#Alternatives).

#### SideCar container within all Etcd-static-pod

A sidecar container is being deployed within each etcd pod in Openshift cluster in order to take backups.

Initially, the sidecar container within etcd pods is not running (i.e. disabled). However, creating a `Backup` CR with `name=default` enables the container.

Once enabled, the backups are being taken on each master node according to the provided `schedule` within the CR. Moreover, the backups are being maintained according to the supplied retention policy.

The backups are being stored within a `hostPath` volume. The volume is a directory within each master node's file system.

The approach has been implemented and tested. Below are the Pros and Cons found.

_**Pros**_

As the container runs on the same pod as etcd, it requests backup through `localhost`, since the containers runs on the same pod. Backups are being stored on the `hostPath` volume, with is a Directory on the node's file system. This guarantees no network delays.

_**Cons**_

Etcd pod is a static-pod, and the availability of the etcd cluster is vital for cluster stability. As the Etcd cluster is being managed by the `Cluster-etcd-operator`, and a `Deployment` resource manages the _mirrored_ etcd pods, any changes in the pod manifest results into rolling out a new revision, which leads to a duration of `etcd-cluster` unavailability, till the new revision is rolled out for all etcd pods. Consequently, the `API-Server` can not process any write requests, and read requests are being handled; if any, from the API-Server cache.   

The backup sidecar container is being `disabled` by default. Upon creating a `Backup` CR on an Openshift cluster, the `PeriodicBackupController` reacts by enabling the container. This leads to a change in etcd static pod manifest, and a new revision is being rolled out. As explained above, such action causes etcd-cluster and API-Server unavailability.

Additionally, any error within the backup sidecar, the container goes into `CrashLoopBackOff`, which leads to the whole `Pod` reports `NotReady` status. Consequently, the whole `Etcd-cluster` can not server requests, nor the `API-Server`.


#### DaemonSet manages an Etcd-backup pod within every control-plane node.

This approach separates the `etcd-backup-server` deployment from the etcd static pods, to avoid affecting etcd-cluster and API-Server availability upon any error within the etcd-backup-server.

The advantage of using `DaemonSet` over Deployment is to guarantee scheduling etcd-backup-server pods on all `Control-plane` nodes, regardless of the number of nodes (e.g. 1, 3, 5) using `nodeSelector` capability.
By setting the `nodeSelector` to the master node label (i.e. `node-role.kubernetes.io/master`), etcd-backup-server's pods are guaranteed to scheduled on every control-plane node.

This method has been implemented and tested. The same container used in the Sidecar approach has been utilised. However, the scheduling is being managed by a DaemonSet.
Upon creating a `Backup` CR on an Openshift cluster, the `PeriodicBackupController` reacts by creating a DaemonSet resource, with the `etcd-backup-server` container and the configurations applied in the CR.
During [testing](https://github.com/openshift/cluster-etcd-operator/pull/1354#issuecomment-2408661261), this method has shown a solid and stable deployment, where the Backup pods shown only `1` restart over a duration of `45` minutes. Moreover, the cause of the restart on all the Backup pods were due to `node` issue, not the backup container.

_**Pros**_

As the Backup pods are being scheduled independently of the etcd pods, any error within the Backup pods does not influence cluster Openshift cluster availability.

Moreover, as the Backup pods are being scheduled within every master nodes, the backups are being taken from every local etcd member on the same node. This avoids overwhelming a specific member with backup requests.

Since a Backup pod exist on every master node, and a `hostPath` volume is being used to store the backups, it is guaranteed to have available backups on all master nodes, which is required for disaster recovery scenarios.

_**Cons**_

No issue has been encountered during testing this approach.

### Open Questions

Need to agree upon the default configurations to be used on the default `Backup` CR.

### Implementation Details/Notes/Constraints

The _no-config_ backups builds atop of the existing [automated backup of etcd](https://docs.openshift.com/container-platform/4.15/backup_and_restore/control_plane_backup_and_restore/backing-up-etcd.html#creating-automated-etcd-backups_backup-etcd).

The `PeriodicBackupController` within **cluster-etcd-operator** checks for `Backup` CR with `name=default` within every sync. Upon applying a `Backup` CR with `name=default`, the _no-config_ backup sidecar container is being enabled within each etcd pod in Openshift cluster.
The `Backup` configurations such as `scheudle`, `timezone` and `retention policy`, are being supplied to the _no-config_ container as flags. 
According to user's preference, the default configuration could be modified by updating the CR.

The backups are being stored on the local disk of each master node independently, using `hostPath` volume. This replication is required for disaster recovery scenarios where more than one of the master nodes has been lost.

A `Backup` CR with default configuration below.

```yaml
apiVersion: config.openshift.io/v1alpha1
kind: Backup
metadata:
  name: default
spec:
  etcd:
    schedule: "*/5 * * * *"
    timeZone: "UTC"
    retentionPolicy:
      retentionType: RetentionNumber
      retentionNumber:
        maxNumberOfBackups: 3
```

### Risks and Mitigations

## Test Plan

An e2e test will be added to practice the scenario as follows 

- Enable `AutomatedEtcdBackup` FeatureGate.
- Verify that a `Backup` CR has been installed.
- Verify that backups have been taken successfully.
- Verify that backups are being maintained according to the retention policy.

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

Before deciding the _DaemonSet_ approach as described above, several alternatives have been investigated. They are being enumerated below.

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

A hybrid solution is possible, in which dynamic provisioning is being used in cloud based Openshift, while utilising local storage for BM and SNO.
Relying on `infrastructure` resource type, we can create the storage programmatically according to the underlying infrastructure and Openshift variant.

### StatefulSet approach

A `StatefulSet` could be deployed among all master nodes, where each backup pod has its own `PV`. This approach has the pros of spreading the backups among all master nodes.
The complexity come from the fact that the backups are being triggered by a `CronJob` which spawn a `Job` to take the actual backup, by deploying a Pod.
Since StatefulSet manages its own Pods, it is not possible to schedule backups by a `CronJob`. However, it is possible to generate event by the `CronJob` which is being watched by the StatefulSet. Then the backups are being taken and controlled mainly by it.

## Infrastructure Needed
TBD