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

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Current disaster recovery scripts were created before the introduction of `cluster-etcd-operator` (CEO). Cluster-etcd-operator (CEO) is an operator that handles the scaling of etcd during cluster bootstrap and regular operation and provisioning etcd dependencies such as TLS certificates. This enhancement allows modifications to the DR scripts such that disaster recovery makes use of the presence of CEO.


## Motivation

The immediate need for enhancement of disaster recovery scripts is to allow basic backup and restore operations to work, which are currently failing the automated tests (e2edisruptive tests). 

In addition, the disaster recovery should allow the recovery of a single failed node, scaling up and scaling down (node drain).

## Goals

1.  If one master is lost, instructions for how to.
    1. create a new master that joins the cluster
    1. removal of the old master from the cluster
1.  Changes to the etcd-quorum recovery steps
1.  All nodes being shut off at the same time and restarted.
1.  IP address change of a single member
1.  IP address change of all members
1.  debugging and detection when DNS information for all members is lost
1.  debugging and detection when DNS information for one member is lost
1.  Removal of a member from the etcd cluster
1.  Recovery of a member with a bad data-dir.
1.  Addition of a new member when there is significant etcd data.
1.  Upgrade, downgrade, re-upgrade
1.  Restoring to a previous state
1.  Remove responsibility for these files from the MCO

## Non-Goals

*  Recover from expired certificates (not considered for 4.4)

## Currently Supported Functionality 

Currently the scripts available support the following functionality:
### Backup ([usr-local-bin-etcd-snapshot-backup-sh.yaml](https://github.com/openshift/machine-config-operator/blob/3e10747951dd1c3c7a9f131d4c5af7805f19a164/templates/master/00-master/_base/files/usr-local-bin-etcd-snapshot-backup-sh.yaml))
Takes a snapshot of cluster’s etcd data along with static-pod-resources at the time of the backup. 

### Restore ([usr-local-bin-etcd-snapshot-restore-sh.yaml](https://github.com/openshift/machine-config-operator/blob/3e10747951dd1c3c7a9f131d4c5af7805f19a164/templates/master/00-master/_base/files/usr-local-bin-etcd-snapshot-restore-sh.yaml))
Restores the etcd data from a backup snapshot. It also restores the static pod resources while deleting all the newer revisions.

### Remove an etcd member ([usr-local-bin-etcd-member-remove-sh.yaml](https://github.com/openshift/machine-config-operator/blob/3e10747951dd1c3c7a9f131d4c5af7805f19a164/templates/master/00-master/_base/files/usr-local-bin-etcd-member-remove-sh.yaml))
This script removes a member from the etcd cluster membership. This is useful when trying to replace a failed master host.

### Add an etcd member ([usr-local-bin-etcd-member-add-sh.yaml](https://github.com/openshift/machine-config-operator/blob/3e10747951dd1c3c7a9f131d4c5af7805f19a164/templates/master/00-master/_base/files/usr-local-bin-etcd-member-add-sh.yaml))
This script adds a member to the etcd cluster membership. This script assumes the certificates are valid.
 
### Replacing a single failed member 
If some etcd members fail, but you still have a quorum of etcd members, you can use the remaining etcd members and the data that they contain to add more etcd members without etcd or cluster downtime. Just running the remove script, and the add script will readd the member to the cluster.

### Recover an etcd member ([usr-local-bin-etcd-member-recover-sh.yaml](https://github.com/openshift/machine-config-operator/blob/3e10747951dd1c3c7a9f131d4c5af7805f19a164/templates/master/00-master/_base/files/usr-local-bin-etcd-member-recover-sh.yaml))
This script helps recover from a complete loss of a master host. This includes situations where a majority of master hosts have been lost, leading to etcd quorum loss and the cluster going offline. This procedure assumes that you have at least one healthy master host. It restores etcd data from backup to gain etcd quorum on the active master host. The steps to recover include setting up an etcd-signer to obtain certificates for new master hosts and growing them to full membership.

### Recover from expired certificates ([recover-kubeconfig.sh](https://github.com/openshift/machine-config-operator/blob/3e10747951dd1c3c7a9f131d4c5af7805f19a164/templates/master/00-master/_base/files/usr-local-bin-openshift-kubeconfig-gen.yaml))
This procedure enables to recover from a situation where the control plane certificates have expired.


## Proposal

### Backup

1. On all masters create a new backup revision `/etc/kubernetes/static-pod-manifests/backup-N`.
2. On all masters write `/etc/kubernetes/static-pod-manifests/backup-N/backup.env` file containing 3 environmental
   variables `CREATED`, `ETCD_REVISION` and `APISERVER_REVISION`.
2. On all masters copy directory `/etc/kubernetes/static-pod-manifests/etcd-pod-${ETCD_REVISION}` to
  `/etc/kubernetes/static-pod-manifests/backup-N/etcd-pod`.
3. On all masters take an etcd snapshot `etcdctl snapshot save
  /etc/kubernetes/static-pod-manifests/backup-N/etcd-data/backup.db`.
4. On all masters copy directory `/etc/kubernetes/static-pod-manifests/kube-apiserver-pod-${APISERVER_REVISION}`
  to `/etc/kubernetes/static-pod-manifests/backup-N/kube-apiserver-pod`.
5. On all masters replace directory `/etc/kubernetes/static-pod-manifests/backups` with a copy of
  `/etc/kubernetes/static-pod-manifests/backup-N` directory.

## User Stories [optional]

### Security

Your clusters backup data is as secure as your cluster. If someone were to root the system they would have direct
access to all data.

### Availability

Your data is as resilient as your cluster. We make N copies of your data so in the case of failure you dont have to
worry about your last backup location.

### Recovery Automation

If the cluster were to lose quorum and every master is seeded with data required to restore. Automation of recovery
tasks becomes easier.


## Implementation Plan

1. Make changes to the scripts as needed
2. Modify CEO to provide the information needed for the DR scripts to work.

## Implementation Details/Notes/Constraints

*  Fix the version mismatch bug -- **done**.
*  Change the name of the spec from `etcd-member.yaml` to `etcd-pod.yaml` -- **done**.
*  Change the backup scripts to include the static pod resources of etcd -- **done**.
*  Change the restore scripts to extract the static pod resources of etcd -- **done**.
*  Fix the need to extract ETCD_DNS_NAME and  DISCOVERY_DOMAIN from pod spec -- **done**.

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

