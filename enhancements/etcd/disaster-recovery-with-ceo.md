---
title: disaster-recovery-with-ceo
authors:
  - "@skolicha"
reviewers:
  - "@deads2k"
  - "@hexfusion"
  - "@alpatel07"
approvers:
  - TBD
creation-date: 2020-02-19
last-updated: 2020-02-19
status: provisional
see-also:
  - "https://github.com/kubernetes/enhancements/blob/master/etcd/cluster-etcd-operator.md"  
---

# disaster-recovery-using-cluster-etcd-operator

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [x] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Disaster Recovery (DR) scripts were created prior to OCP 4.4 before the introduction of `cluster-etcd-operator`
(CEO). Cluster-etcd-operator (CEO) is an operator that handles the scaling of etcd and provisioning etcd dependencies
such as TLS certificates. The introduction of CEO obviated the need for some of the disaster recovery scripts such as
scaling and TLS certificate generation. This enhancement describes how a cluster-admin responds to exceptional 
circumstances in his cluster, such as loss of quorum, unhealthy member or need to restore to a previous state.

## Motivation

While the operator handles many aspects of cluster management, it relies on having a running kube control plane to do
so. In cases where no running kube control plane exists and cannot be started due to an etcd failure, it is necessary
to have manual action on the part of a cluster-admin to get back to a running control plane.

## Goals

1. Transfer the ownership of the DR files from the MCO to CEO
1. Remove the deprecated DR scripts
1. Backing up the cluster state
1. Recovering a single unhealthy control-plane
    1. create a new master that joins the cluster
    1. removal of the old master from the cluster
1. Restoring the cluster state from backup 
1. Recovering from a loss of quorum
1. Removal of a member from the etcd cluster
1. Recovery of a member with a bad data-dir.
1. All nodes being shut off at the same time and restarted.
1. Upgrade, downgrade, re-upgrade

## Non-Goals

*  Recover from expired certificates (not considered for 4.4)

## Proposal
The proposal is to transfer the ownership of the DR scripts from MCO, remove all the deprecated DR scripts, and to 
simplify other existing scripts for backing up and restoring the cluster state. 

With the simplification of the scripts, different disaster recovery scenarios are documented properly to utilize the
simplified scripts along with other Openshift utility commands to achieve the recovery of the cluster. 

## Implementation Plan
###  Transfer the ownership of the DR files from the MCO to CEO
All etcd-scripts should be removed from MCO, and be generated along with the static-pod-resources as configmaps. The 
scripts should be copied to /usr/local/bin in the init container for a convenient access to the admin user.
    
###  Remove the deprecated DR scripts
The scripts for adding a new etcd member; removing an etcd member; and recovering an etcd-member are no longer needed,
and should be deleted from the repository.
    
### Backing up the cluster state
The script for backing up the cluster state should back up static-pod-resources for etcd, kube-apiserver, 
kubecontrollermanager and kube-scheduler, along with a copy of the etcd snapshot database. The reason for backing up
the static pod resources is to match the static pods state on disk with what is in etcd,  when we restore. Furthermore,
when etcd data is encrypted, the encryption keys are stored in static pod resources, and restoring the etcd data without
restoring the corresponding key will make the etcd data unusable.
    
### Restoring the cluster state from backup 
This solution handles situations where you want to restore your cluster to a previous state, for example, if something
critical got deleted accidentally. This process is also used for recovering from a loss of quorum.
    
The script for restoring to a previous cluster state using a backup file will restore the etcd database from the backup
along with the static pod resources for the entire control plane (etcd, kube-apiserver, kubecontrollermanager and
kube-scheduler). See the implementation details for more information.
    
### Removal of a member from the etcd cluster
In the past, we used a script to remove a member from the etcd cluster. In this release, the remove operation will be
documented to invoke the `etcdctl member remove` command directly on the newly created `etcdctl` container.
    
### Recovering a single unhealthy member
See the implementation details below.
    
### Recovering from a loss of quorum
Recovering from a loss of quorum requires restoring from a previous backup. See the implementation details for
`Restoring the cluster from backup`.
   
### Recovery of a member with a bad data-dir.
If you have a majority of your masters still available and have an etcd quorum, then the following procedure should
restore the data-dir:
1. Stop etcd static pod by moving the etcd pod yaml out of kubelet manifest directory.
1. Remove the bad data directory.
1. Restart etcd static pod by moving the etcd pod yaml back into the kubelet manifest directory.

If you have lost the majority of your master hosts, leading to etcd quorum loss, then, it requires restoring from a
previous backup. See the section `Recovering from a loss of quorum.`

###  All nodes being shut off at the same time and restarted.
Fix the initialization process to detect lights-out scenario to restart the etcd without requiring any manual
intervention.
    
### Upgrade, downgrade, re-upgrade
Upgrade to 4.4, downgrade to 4.3, and re-upgrading to 4.4 should work, although downgrade is not officially supported.

## Implementation Details/Notes/Constraints

### Restoring the cluster from backup
In the prior releases, we restored the backup on all masters simultaneously for the cluster to come back up properly.
With this release we implement a different approach of bringing a control plane up with a single node etcd cluster and
then scaling it up to three.

This new approach involves two steps:

1. Restore single master control-plane
    1. Establish ssh connectivity to all masters. Part of this process will make the kube-apiserver inaccessible, so you
     will not be able to run `oc debug node` once the restore process has started.
    1. Stop etcd and kubelet static pods on the recovery node by moving the manifests out of kubelet manifest directory,
    and remove the etcd data-dir. This will stop all the etcds leading to total quorum loss, and removing all of the
    data directories makes the backup a single source of truth.
    1. Select a master node to run the restore operation on (recovery node) and make sure that the etcd backup file is
    present on the recovery node
    1. Move a copy of the etcd snapshot from the backup to /var/lib/etcd-backup/snapshot.db on the recovery node
    1. Move the static pod manifests back into kubelet manifest directory and restore the static pod resources from the
    backup. Because the restoration is happening from a backed up data, the revision of the manifests and resources
    need to match the revision that the operator was on while taking the backup.
    1. Copy the restore-etcd-pod.yaml from backed up resources to /etc/kubernetes/manifests/etcd-pod.yaml. This will
    restore a single member control plane using the backed up snapshot.db and then start the etcd pod bringing the
    control plane up. 
    1. These steps are achieved through a script `cluster-restore.sh`. See the "Outline of the scripts" section to learn
    more details of the script.

2. Get the operator to deploy static pods on the other master nodes.
    If the member has been added to the cluster and has never been started before, but a data directory exists, it
    means that we have dirty data which must be removed (or archived).
    1. Fix the initialization process to detect an unstarted member with existing data-dir. 
    1. Force etcd redeployment via `oc patch etcd forceredpeloymment=reason-time`
    1. CEO will redeploy the modified pod resources with the next revision.
    1. The existing nodes will restart the static pods in rolling fashion with new spec.
    
    Note that when we force redeployment, the recovery node will also restart with the newly generated normal spec and
    obtain data-dir from the started members.

### Recovering a single unhealthy member

If the cluster-etcd-operator status shows `Degraded` condition as `true` with the status of a single `unhealthy` member,
then the etcd member could be in one of the three scenarios:

1. The master node in question is down and expected to come back up. 

    In this case, admin doesn't need to do anything.
    
1. The master node is up and generally healthy, but etcd is unhealthy. If the `endpoint status` and `endpoint health`
also indicate that the member is not healthy or unreachable and if the `etcd` logs on the pod indicate that it has
experienced catastrophic failure, or if the pod is crash looping and resolution is not possible. 

    In that event, admin needs to force the member to rejoin by removing and redeploying. Go to the "forcing an existing
    member to rejoin" section.
    
1. The master node is down and not expected to come back.

    In this case, "remove the etcd member that will never return" followed by "adding a new master node"
    
### Forcing an existing member to rejoin

If the node is healthy, but the etcd pod experienced a catastrophic failure, then, we suggest to stop the pod, remove
the member and force redeployment, so that cluster-etcd-operator can regenerate static-pod-resources and restart the
pod. Force an existing member to rejoin by the sequence of commands listed below:
1. oc debug node/affected-node
1. mv /etc/kubernetes/manifests/etcd-pod.yaml backup-location
1. rm -rf /var/lib/etcd/*
1. oc -n openshift-etcd rsh some-etcd-pod 
1. etcdctl member remove the-bad-one
1. oc patch forceredpeloymment=reason-time

### Removing an etcd member that will never return
1. oc -n openshift-etcd rsh some-etcd-pod 
1. etcdctl member remove the-bad-one

## Outline of the scripts
After removing the deprecated scripts, the OCP 4.4 release will only include two scripts for disaster recovery, viz.,
`cluster-backup.sh` and `cluster-restore.sh`. Below sections give a brief outline of the scripts.

### cluster-backup.sh ([link](https://github.com/openshift/cluster-etcd-operator/blob/master/bindata/etcd/cluster-backup.sh))
The cluster backup script takes a snapshot of the etcd data along with the static pod resources. This backup can be
saved and used at a later time if you need to restore etcd.
                                                                                                                       
If etcd encryption is enabled, it is recommended to store the static pod resources separately from the etcd snapshot
for security reasons. However, both will be required in order to restore it to the previous state.

### cluster-restore.sh ([link](https://github.com/openshift/cluster-etcd-operator/blob/master/bindata/etcd/cluster-restore.sh))
The cluster restore script facilitates restoring the backed up data on a single node to bring up the cluster on a
single node control plane. Once the single node control plane is up, forcing the redeployment will enable the operators
to restart static pods on the other master nodes, as described above.

The series of actions performed on the recovery node by `cluster-restore.sh` can be summarized as follows:
1. Verify that the backup directory exists and contains etcd snapshot data and static pod resources.
1. If static pods are currently running on the recovery node, stop them. Wait for all containers to stop.
1. Remove the extant data-dir.
1. Restore the static pod resources from the backup.
1. Copy the snapshot db to /var/lib/etcd-backup directory for the restore-etcd static pod to pick up.
1. Start the restore-etcd static pod. The pod yaml is customized to restore the snapshot and execute etcd.
1. Restart all other static pods such as kube-apiserver, kube-scheduler and kube-controllermanager.

## Risks and Mitigations

## Design Details

## Test Plan

* Need to fix existing e2etests to match the changed syntax.
* Need to add new tests to test the scale up scenario.

## Graduation Criteria

## Upgrade / Downgrade Strategy

## Version Skew Strategy


## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

### New github projects:


### New images built:

