---
title: automated-cluster-backups

authors:
  - "@skolicha"
  - "@dmace"
  - "@hexfusion"
reviewers:
  - "@deads2k"
  - "@hexfusion"
approvers:
  - TBD
creation-date: 2020-07-29
last-updated: 2020-07-29
status: provisional
see-also:
  - "https://github.com/kubernetes/enhancements/blob/master/etcd/disaster-recovery-with-ceo.md" 
---

# backup config controller

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary
Backups are an important part of cluster management. Our customers
expect a safe and easy backup procedure.

## Motivation
The current cluster backup procedure requires envolking scripts on cluster to
perform backup operations. Because we rely upon on disk resources for the backup
process. Putting the logic 

## Goals

1.  Eliminate on cluster scripts as a requirement
1.  Utilize the cluster-etcd-operator to manage the backup process.

## Non-Goals

*  Automated backup is not considered for this enhancement.

## Currently Supported Functionality 

Currently the scripts available support the following functionality:
### Cluster Backup 
Takes a snapshot of cluster’s etcd data along with static-pod-resources at the time of the backup. 

### Cluster Restore 
Restores the etcd data from a backup snapshot. It also restores the static pod resources while deleting all the newer
revisions.

## Proposal

The following changes are proposed.

- library-go changes: [PR #855](https://github.com/openshift/library-go/pull/855)
- etcd-operator changes: [PR #894](https://github.com/openshift/cluster-etcd-operator/pull/406)
- API changes: TBD

### BackupConfigController

Phase 1 of automated backups is the `BackupConfigController` this controller is tasked with
populating the spec for a cluster-backup pod. The cluster-backup pod persists the TLS
certificates and static-pod spec defined in the revisioned configmaps and secrets of
the operands namespace. By backing up a specific revision N of these resources
we essentially create a checkpoint in time where we can restore the static pods.

The controller on sync observes the `LatestAvailableRevision` for each of the
following operands.

1.  kube-apiserver
1.  kube-controller-manager
1.  kube-scheduler
1.  etcd

It then compares those revisions with the `PreviousBackupRevisions` which we
store in a configmap backup-status. If those values do not match a new
cluster-backup pod spec must be generated. This current design does not propose
public facing API changes.

#### backup-status
```
 apiVersion: v1
  kind: ConfigMap
  metadata:
    namespace: openshift-etcd-operator
    name: backup-status
  data:
    status: "Complete"
    revisions:
      - etcd: "3"
      - kubeControllerManager: "7"
      - kubeApiserver: "7"
      - kubeScheduler: "7"
```

The `BackupConfigController` embeds the new backup-pod spec into a configmap
`cluster-backup-pod`. The reason for this is the backup is not automatically
created by this controller. A user could create an actual backup with the
following command. 

#### create cluster-backup pod
```
$ oc get cm cluster-backup-pod -n openshift-etcd -o "jsonpath={.data['backup-pod\.yaml']}" | oc create -f -
```

#### copy backup to local disk
```bash
$ oc cp -n openshift-etcd cluster-backup:/backup ./backup`

```

#### cluster-backup-pod configmap
```
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: openshift-etcd-operator
  name: cluster-backup-pod
data:
  pod.yaml:
```

#### cluster-backup pod
Cluster backup pod generates the backup resources for the static pod operators
and takes a snapshot from etcd. It then packages the files in a format that is
backwards compatible with the current backups.

initContainers:
1.  backup-init: this container generates a snapshot of etcd state. This
    container is first so that we can attempt to capture etcd state matching backup
    revisions.
1.  backup-etcd-resources: persists the backup revision resources for
    etcd.
1.  backup-kas-resources: persists the backup revision resources for
    kube-apiserver.
1.  backup-kcm-resources: persists the backup revision resources for
    kube controller manager.
1.  backup-ks-resources: persists the backup revision resources for kube
    scheduler.
1.  backup-create: tars the backup resources in the container and places them in
    the /backup directory.    


#### Reads
1.  `oc get cm -n openshift-etcd backup-revisions`
1.  `oc get cm -n openshift-etcd cluster-backup-pod-cm`
1.  `oc get etcds`
1.  `oc get kubeapiservers`
1.  `oc get kubecontrollermanagers`
1.  `oc get kubeschedulers`
1.  `oc get cm -n openshift-etcd revision-status-N`
1.  `oc get cm -n openshift-kube-apiserver revision-status-N`
1.  `oc get cm -n openshift-kube-controller-manager revision-status-N`
1.  `oc get cm -n openshift-kube-scheduler revision-status-N`
-  

#### Writes 
1.  `oc get cm -n openshift-etcd cluster-backup-pod`
1.  `oc get cm -n openshift-etcd backup-revisions`

Because these configmaps and secrets live in etcd statefile we must also take a
snapshot to put alongside the static pod resources. This pairs the on disk
revisioned resources with what is stored in etcds state.

#### Consumers
`oc get cm cluster-backup-pod -n openshift-etcd -o "jsonpath={.data['backup-pod\.yaml']}" | oc create -f -`
can be used to build backup pods.

## User Stories [optional]

### Security

By backing up the static pod configs and secret to the container instead of the
host. We eliminate the need for a privileged with a host mount of
static-pod-resources.

## Implementation Plan

1. Create a subcommands to cluster-etcd-operator backup-init and backup-create.
1. Create cluster backup controller that will facilitate populating the nessisary assets.
1. Review adding customer facing API.

## Risks and Mitigations

* Performance impact

  We would want to limit the frequency of backups to one hour.

* Disks running out of space

  If they backups are not pruned properly, they could overfill the disk.

## Design Details

## Test Plan

## Graduation Criteria

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

### New github projects:


### New images built:
