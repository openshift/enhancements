---
title: cluster-etcd-operator
authors:
  - "@alaypatel07"
reviewers:
  - "@hexfusion"
  - "@deads2k"
  - "@retroflexer"
approvers:
 - TBD
creation-date: 2020-2-21
last-updated: 2020-2-21
status: brainstorming
see-also: 
replaces:
superseded-by:
---

# Disaster Recovery options leveraging Cluster Etcd Operator scaling up
Currently the Disaster Recovery scripts support 3 scenarios:
1. Recovering from total loss of control plane (etcd quorum loss) with ssh access to all master nodes and a backup file
2. Restoring the cluster state from a backup. Here the control plane is available but the etcd state is reset to that
 of the backup. We have quorum here, but all the etcd pods will be moved out leading to loss of control plane, before
 we restore
3. Recovering the loss of 1 master node. This includes recovering from a case where one out of three etcd nodes is 
unhealthy, but we still have etcd quorate and controle plane is up, kube API is responding. 

This document discusses the ideas involved in making the first two scenarios easiest to implement and consumed by the 
user. The ideas is to have a single member etcd cluster running so control plane is up and CEO scales the remaining
etcd members.  


## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

TODO

## Summary

cluster-etcd-operator (CEO) is an operator that handles the scaling of etcd during cluster
bootstrap and regular operation and provisioning etcd dependencies such as TLS certificates.

## Motivation

TODO


### Goals

1. Leverage the scale up mechanism of CEO

### Non-Goals

TODO

## Challenges and possible alternatives

A restore operation is a two step process on each node. 
1. Restoring the data directory with appropriate environment variables with the following operation:
```
        ETCDCTL_API=3 etcdctl snapshot restore ./path/to/backup.db \
                    --name $ETCD_NAME \
                    --initial-cluster="$ETCD_NAME=$ETCD_NODE_PEER_URL" \
                    --initial-cluster-token openshift-etcd-<random-uuid> \
                    --initial-advertise-peer-urls $ETCD_NODE_PEER_UR \
                    --data-dir="/path/to/new/data-dir"
```
2. Starting the etcd pod should be started with the same value of $ETCD_NAME, ETCD_INITIAL_CLUSTER and 
$ETCD_NODE_PEER_URL as in step 1.

This will allow etcd to populate the data directory with a new cluster ID, and new membership data that includes only
1 member.

Restoring a single member from a backup and scaling up has two distinct problem, they are:
1. Get the first etcd member running. 
2. Get the operator to deploy static pods that can decide that they are being restored, so they don't start on the 
existing data directories.

### Static Pod identifying a restore

Assuming the first member is up, the control plane will be up and operator start running. A user can trigger a new 
rollout of the etcd pods by running `oc patch forceredpeloymment=reason-time`. New pods will be rolled out on the 
remaining masters that have to be restored. Before etcd commands on these pods are run, the existing data directory 
needs to be removed, because they contain the previous cluster state information.

During the start up pod queries etcd membership, if the pod is an unstarted member, it can be assumed that the data 
directory present is from previous cluster and moved out.

Other solution is to check the cluster id of the new cluster with the cluster id of the existing data directory, move 
out the data directory only if they are different. This is a little time consuming to implement and test.

### Getting the first etcd member running

This will involve bringing a control plane up with a single node etcd cluster and then scaling it up to three. Before
the restore operation is carried out, it is mandatory for the users to have a backup. If they do not have a backup, the 
will lead fatal, non recoverable loss. The steps are:
  1. move the existing etcd pod yaml out of kubelet manifest directory and move etcd data directory into a different 
  location on all the master nodes. This will stop all the etcds leading to total quorum loss and remove all of the data
   directories, making the backup a single source of truth.
  2. Select a master node to run the restore operation on (recovery node) and make sure etcd backup file in present
   on the recovery node
  3. copy the snapshot file on a temporary location and move the copy to backup directory to 
  /var/lib/etcd-backup/snapshot.db on the recovery node
  4. move the kube-apiserver backed up revision back into kubelet manifest directory and delete other revisions and 
  related resources(certs and configmaps). Because the restoration is happening from a backed up data, the revision of
  kube-apiserver pod yaml needs to match the revision that the operator was on while taking the backup.
  5. copy the restore-etcd-pod.yaml from backed up resources to /etc/kubernetes/manifests/etcd-pod.yaml. This will 
  restore a single member control plane using the backed up snapshot.db and then start the etcd pod bringing the control
  plane up. 
  6. force CEO redeployment via `oc patch forceredpeloymment=reason-time`
  7. When CEO redeployment is forced, the existing nodes will start with new pods, a scale up similar to bootstrap will 
  take place
  8. CEO will redeploy the modified pod on recovery master node to the current revision.

## Design Details

TODO

### Test Plan

TODO

### Graduation Criteria

TODO

##### This feature will GA in 4.3 as long as:

TODO

### Upgrade / Downgrade Strategy

TODO

#### Upgrade from 4.3 to 4.4

TODO

#### Downgrade from 4.4 to 4.3

TODO

### Version Skew Strategy

TODO

## Implementation History

TODO

## Drawbacks

If the restoration steps are performed with an etcd cluster that is quorate i.e. 2 etcd members are in healhty state and
1 etcd member is unhealthy, it will lead to a temporary [loss](#getting-the-first-etcd-member-running)(step 1) of the
control plane and restoration will bring cluster state back in time to where the back up was taken. For this reason, it is 
advisable to follow the workflow of restoring a single lost master to get to a state with all the three etcd members 
being healthy. 

## Alternatives
The alternative for getting the first etcd member running is to use a command to render a bootstrap like pod with 
recovered data directory. The workflow here would be to:
  1. move the existing etcd pod yaml out of kubelet manifest directory and move etcd data directory into a different 
  location.
  2. get the etcd ETCD_NAME and ETCD_PEER_URL values from backed up etcd pod yaml 
  3. use etcdctl to restore the snapshot 
  4. start the etcd cert signer
  5. reuse the CEO binary render command to generate a pod yaml. 
  6. start the pod on a port different from regular etcd server, peer and metrics ports mount it the new restored data 
  directory
  7. update the KAS config to point to new etcd server and restart the kube-apiserver pod
  8. create a new host-etcd endpoint resource to point to the new etcd server
  9. force CEO redeployment via `oc patch forceredpeloymment=reason-time`
  10. wait for scale up to complete
  11. clean up: stop etcd-cert signer, remove the temporary etcd pod and its data directory
    
 Challenges involved: making the render command work, making sure new ports can be opened up on masters to bind the 
 temporary etcd and cert-signer.

## Infrastructure Needed [optional]

TODO
