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

To improve the user's experience, this work proposes taking etcd backup using default config from day one, without additional configuration from the user.

### Goals

* Backups should be taken without configuration after cluster installation from day 1.
* Backups are saved to a default location on each master node's disk using `hostPath` volume.
* Backups are taken according to a default schedule.
* Backups are being maintained according to a default retention policy.

### Non-Goals

* Save cluster backups to remote cloud storage (e.g. S3 Bucket).
  - This could be a future enhancement or extension to the API.
  - Having backups in an independent storage from OCP cluster is crucial for Disaster Recovery scenarios.
* Automate cluster restoration.
* Provide automated backups for non-self hosted architectures like Hypershift.

## Proposal

To enable automated etcd backups of an Openshift cluster from Day 1, without user supplied configuration, this work proposes a default etcd backup option.

The default backup option utilizes a sidecar container within each etcd pod. The container is responsible for maintaining backups within a `hostPath` volume on each master node's disk.

This approach guarantees have backups across all master nodes which is vital in disaster recovery scenarios.

### User Stories

* As a cluster administrator, I would like OCP cluster backup to be taken without configuration from day one.
* As a cluster administrator, I would like to schedule recurring OCP cluster backups so that I have a recent cluster state to recover from in the event of quorum loss (i.e. losing a majority of control-plane nodes).

### API Extensions

No [API](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/automated-backups.md#api-extensions) changes are required.

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

A sidecar container is being deployed within each etcd pod in Openshift cluster in order to take backups according to default configuration using `config.openshift.io/v1alpha1 Backup`.

To distinguish default _no-config_ backups from user configured counterparts, a `config.openshift.io/v1alpha1 Backup` CR's name must be `default`.

Initially, the sidecar container within etcd pods is not running (i.e. disabled). However, creating a `Backup` CR with `name=default` enables the container.

Once enabled, the backups are being taken on each master node according to the provided `schedule` within the CR. Moreover, the backups are being maintained according to the supplied retention policy. 

The default CR's configuration could be overridden by the user according to their preference.

The backups are being stored within a `hostPath` volume. The volume is a directory within each master node's file system.

Before deciding upon this approach, several approaches that have been investigated. See [Alternatives](#Alternatives).

### Open Questions

Currently, the no-config backup sidecar container exits upon error. Since the container is being deployed within the etcd Pod, the whole Pod will never be `Running` as long as the container is failing.
Once the container fails, it will be automatically restarted according to exponential backoff strategy. This scenario leads to `CrashLoopBackOff` and the etcd Pod will never be `Running`. 

As the no-config backup sidecar is being deployed alongside each etcd member, consequently an error within this container leads to unavailable etcd cluster. Hence, failing the container upon error is not a viable option.
Also disabling the container from the etcd pod upon error is not recommended, unless requested by the user. As discussed above, enabling and disabling the sidecar container leads to deploying a new version of the static pods.
This scenario leads to a duration of cluster unavailability. 

As shown above, it is necessary to report the no-config backup sidecar failure without failing the container. Below are possible resolution scenarios. 

- Log the error upon failure
  - A log parser could be used in order to detect the error, without failing the container.
  
- Update Cluster-etcd-Operator StatusConditions
  - Reporting the condition and degrading the operator could be a solution. 

- Use a Prometheus gauge in order to report the container situation. 

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

- Enable AutomatedBackupNoConfig FeatureGate.
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

Before deciding the _no-config_ sidecar container as described above, several alternatives have been investigated. They are being enumerated below.

The storage solution to use would have the following characteristics
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

A hybrid solution is possible, in which dynamic provisioning is being used in cloud based Openshift, while utilising local storage for BM and SNO.
Relying on `infrastructure` resource type, we can create the storage programmatically according to the underlying infrastructure and Openshift variant.

### StatefulSet approach

A `StatefulSet` could be deployed among all master nodes, where each backup pod has its own `PV`. This approach has the pros of spreading the backups among all master nodes.
The complexity come from the fact that the backups are being triggered by a `CronJob` which spawn a `Job` to take the actual backup, by deploying a Pod.
Since StatefulSet manages its own Pods, it is not possible to schedule backups by a `CronJob`. However, it is possible to generate event by the `CronJob` which is being watched by the StatefulSet. Then the backups are being taken and controlled mainly by it.

## Infrastructure Needed
TBD