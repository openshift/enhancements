---
title: upgrades-with-ovn-ic
authors:
  - "@ricky-rav"
  - "@jtan"
reviewers:
  - "@trozet"
  - "@tsurya"
approvers:
  - "@trozet"
api-approvers:
  - "None"
creation-date: 2023-05-26
last-updated: 2024-02-20
tracking-link:
  - https://issues.redhat.com/browse/SDN-3905
---

# Upgrades from 4.13 to 4.14 with OVN interconnect

## Summary


Allow any upgrade path that proceeds via a 4.13 self-hosted or hypershift-hosted cluster to smoothly upgrade to 4.14, which features OVNK InterConnect (IC) multizone. 

## Motivation

Starting in 4.14 the default ovn-kubernetes cluster will be deployed with IC multizone, where every node is deployed its own zone and features its own local OVN stack (ovnkube-controller, nbdb, northd, sbdb, ovnkube-node, ovn-controller). Self-hosted and hypershift-hosted clusters will need to support upgrading to the new IC multizone control plane with ~zero disruption. 

### User Stories

As an Openshift user, I want to upgrade my self-hosted or hypershift-hosted cluster to run ovn-kubernetes 4.14 with ~zero disruption. 


### Goals

The ability to upgrade ovnkube clusters from 4.13 to 4.14 ~zero disruption and no additional setup. 

### Non-Goals


## Proposal

Implement a multistep upgrade process that will take clusters from 4.13 to 4.14 with IC multizone ovnkube fully configured. 


### Workflow Description
The cluster administrator is expected to follow the normal workflow for upgrading the cluster, no additional steps are needed.

### API Extensions
No API extensions are needed.

### Risks and Mitigations
NA

### Drawbacks
As described in detail further below, the upgrade to multizone IC requires two OVN rollouts. This means that the time to upgrade OVN to multizone IC on a cluster is roughly twice as much as what we had in the centralized legacy architecture.

However, this extra time is only needed when upgrading from 4.13 to 4.14: future upgrades from 4.14 to future versions will only require one roll out, so the total upgrade time will be comparable to what we had before OVNK interconnect.


## Design Details
Throughout this document we will refer to:
- a zone: a set of nodes managed by the ovnkube-controller container (running in the ovnkube-node pod); all nodes that are in the same zone are *local* nodes to that zone ovnkube-controller and nodes that are in different zones are *remote* nodes to that zone ovnkube-controller. 
- IC multizone: our target configuration for OVN interconnect, where every node belongs to its own zone (i.e., one node per zone)
- IC singlezone: a temporary configuration for OVN interconnect, where all nodes belong to the same global zone (i.e. all nodes in one zone); this architecture is equivalent to what we have in Openshift 4.13, except for ovnkube being interconnect aware.

In order to allow ~zero-disruption upgrades from 4.13, which has no interconnect support, to 4.14 IC multizone, we require two phases:
- Phase 1: upgrade from 4.13 to 4.14 single-zone interconnect
- Phase 2: upgrade from 4.14 single-zone interconnect to 4.14 multi-zone interconnect

In **phase 1**, we first upgrade to 4.14 single-zone interconnect, which keeps the architecture equivalent to 4.13, but is interconnect aware (namely, the ovnkube binary is executed with `--enable-interconnect`). The ovnkube components deployed in this phase are, exactly like in 4.13, the following:
- ovnkube-master daemonset: a centralized ovnkube control plane running ovnkube-master, nbdb, northd, sbdb on master nodes;
- ovnkube-node daemonset: ovnkube data plane running ovnkube-node and ovn-controller on all nodes.

The update of ovnkube components follows the logic used until 4.13 in upgrades: we do a rolling update first for ovnkube-node, then for the ovnkube-master.

In **phase 2**, we finally upgrade to 4.14 multi-zone interconnect, which is our target OVN interconnect architecture with a distributed OVN stack.  The ovnkube components deployed in this phase are:
- ovnkube-control-plane deployment: a centralized slimmed-down ovnkube control plane, only running ovnkube-cluster-manager on master nodes; 
- ovnkube-node daemonset: ovnkube data plane running ovnkube-controller, nbdb, northd, sbdb, ovn-controller on all nodes.

The ovnkube images are updated by first doing a rolling update for ovnkube-node, then ovnkube-control-plane is added, after which the old ovnkube-master daemonset is removed.

The rationale behind this 2-phase upgrade is that before we can have interconnect multizone, we need the whole cluster to enable interconnect first, in our case with interconnect single zone, which matches the existing 4.13 architecture.
Let's imagine instead what would happen if we were to upgrade from 4.13 directly to 4.14 multizone. 
As soon as we start replacing the (non-IC) 4.13 ovnkube-node pods with the new multizone IC ovnkube-node pods (featuring the full per-node OVN stack), each node would mark itself as belonging to its own zone and the rest of the cluster, still running non-IC 4.13 images, wouldn't be able to talk to it and would be effectively disconnected.

We address this problem by first upgrading a 4.13 cluster to interconnect single-zone and only then we roll out the new distributed OVN stack on each node: as we replace a single-zone ovnkube-node pod with a multizone ovnkube-node pod on a given node, the node starts to be in its own zone and must be detected as remote by the rest of the cluster, otherwise it will be disconnected from it. 
This can only happen if all nodes in the cluster are already interconnect aware, which is the case at the end of phase 1.

### CNO implementation details
When the user starts the upgrade from 4.13 to 4.14, CNO knows it has to carry out the 2-phase upgrade described above by checking whether the running ovnkube-node and ovnkube-master instances have been started with the `--enable-interconnect` argument. 
At the very beginning of phase 1, CNO pushes to the API server a ConfigMap that will be used to track the status of the whole upgrade and in particular the intermediate step of single-zone IC:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ovn-interconnect-configuration
  namespace: openshift-ovn-kubernetes
data:
  zone-mode: singlezone
  temporary: true
```
The cluster at this point runs 4.13 ovnkube, that is:
- 4.13 ovnkube-master daemonset on master nodes (centralized control plane)
- 4.13 ovnkube-node daemonset on all nodes (dataplane)

CNO will then roll out the YAMLs for single-zone ovnkube, to be found in `bindata/network/ovn-kubernetes/self-hosted/single-zone-interconnect`: first ovnkube-node, then ovnkube-master.
```text
$ ls -l bindata/network/ovn-kubernetes/self-hosted/single-zone-interconnect
-rw-rw-r-- 1    586 Aug 17 15:25 005-service.yaml
-rw-rw-r-- 1  21846 Aug 17 15:25 alert-rules-control-plane.yaml
-rw-rw-r-- 1   1150 Aug 17 15:25 monitor-master.yaml
-rw-rw-r-- 1  41773 Aug 17 15:25 ovnkube-master.yaml
-rw-rw-r-- 1  26274 Aug 17 15:25 ovnkube-node.yaml
```

Once the rollout of interconnect-enabled ovnkube-master has taken place, the cluster now runs 4.14 IC single-zone ovnkube, that is:
- 4.14 ovnkube-master daemonset with IC enabled on master nodes (centralized control plane)
- 4.14 ovnkube-node daemonset with IC enabled on all nodes (dataplane)

CNO detects that phase 1 has ended and updates the `ovn-interconnect-configuration` ConfigMap to mark the start of phase 2:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ovn-interconnect-configuration
  namespace: openshift-ovn-kubernetes
data:
  zone-mode: multizone
  temporary: false
```

At the beginning of phase 2, the YAMLs are to be found in `bindata/network/ovn-kubernetes/self-hosted/multi-zone-interconnect-tmp`. The contents of this folder are symbolic links to:
- the single-zone centralized control plane (`ovnkube-master.yaml -> ../single-zone-interconnect/ovnkube-master.yaml`), which we want to keep running until all nodes have successfully updated to IC multizone, that is until all nodes run multizone ovnkube-node;
- all new multizone components: `ovnkube-node.yaml -> ../multi-zone-interconnect/ovnkube-node.yaml`,  `ovnkube-control-plane.yaml -> ../multi-zone-interconnect/ovnkube-control-plane.yaml`.

The remaining files are for alerts and service monitors.
```text
$ ls -l bindata/network/ovn-kubernetes/self-hosted/multi-zone-interconnect-tmp
-rw-rw-r-- 1  7148 Aug 17 15:25 alert-rules-control-plane.yaml
lrwxrwxrwx 1    53 Aug 17 15:25 monitor-control-plane.yaml -> ../multi-zone-interconnect/monitor-control-plane.yaml
lrwxrwxrwx 1    47 Aug 17 15:25 monitor-master.yaml -> ../single-zone-interconnect/monitor-master.yaml
lrwxrwxrwx 1    53 Aug 17 15:25 ovnkube-control-plane.yaml -> ../multi-zone-interconnect/ovnkube-control-plane.yaml
lrwxrwxrwx 1    47 Aug 17 15:25 ovnkube-master.yaml -> ../single-zone-interconnect/ovnkube-master.yaml
lrwxrwxrwx 1    44 Aug 17 15:25 ovnkube-node.yaml -> ../multi-zone-interconnect/ovnkube-node.yaml
```

CNO starts phase 2 by rolling out multizone ovnkube-node. Each node getting its ovnkube-node image updated will now run in its own zone with its own local ovnkube stack (ovnkube-controller, nbdb, northd, sbdb, ovn-controller) and will consider all other nodes as remote. Reversely, the rest of the cluster will consider this node as remote. 

Once that is done, CNO will deploy the multizone ovnkube-control-plane deployment on master nodes. The ovnkube-control-plane pods only run ovnkube-cluster-manager: the two instances elect a leader, which will allocate IP subnets to nodes in a centralized manner. Beware that up to 4.14.13 there were three instances of ovnkube-control-plane pods.

At this point of phase 2, the cluster runs:
- multizone ovnkube-node (distributed per-node OVN stack)
- multizone ovnkube-control-plane (slimmed-down centralized control plane)
- single-zone ovnkube-master (centralized, but with no dataplane instances any longer listening to it)

We're now in the final bit of phase 2, for which the YAMLs are to be found in `bindata/network/ovn-kubernetes/self-hosted/multi-zone-interconnect`: this is also the folder that is used for any fresh install of 4.14. The only remaining action is to remove the single-zone ovnkube-master pods, which are now of no use.

```text
$ ls -l bindata/network/ovn-kubernetes/self-hosted/multi-zone-interconnect/
-rw-rw-r-- 1   7077 Aug 17 15:25 alert-rules-control-plane.yaml
-rw-rw-r-- 1   1206 Aug 17 15:25 monitor-control-plane.yaml
-rw-rw-r-- 1   5641 Aug 17 15:25 ovnkube-control-plane.yaml
-rw-rw-r-- 1  52271 Sep  1 11:58 ovnkube-node.yaml
```
We have finally completed the whole upgrade from 4.13 to 4.14 and the cluster is now running:
- multizone ovnkube-node
- multizone ovnkube-control-plane

CNO reports 4.14 in its operator status and CVO can carry on with the upgrade of the remaining operators.

#### Notes for hypershift
The upgrade to IC for hypershift follows exactly the same path as outlined above for standalone openshift.

There is however an extra key component that is used until 4.13 in hypershift: a route to sbdb that connects 4.13 nodes to 4.13 masters, specifically to the sbdb in master nodes. This is not relevant anymore in IC multizone, since every node will have its own local instance of sbdb.
In order to prevent disruptions and always have a fully functioning cluster all throughout the upgrade to IC multizone, the route to sbdb needs to remain in place until all nodes have transitioned to IC multizone (end of phase 2). 
This is why `008-route.yaml` is found also in the `multi-zone-interconnect-tmp` folder and is only removed along with ovnkube-master when we finally switch to the `multi-zone-interconnect` folder, when the upgrade ends.

```text
$ ls -l bindata/network/ovn-kubernetes/managed/single-zone-interconnect/
-rw-rw-r-- 1    863 Aug 17 15:25  005-service.yaml
-rw-rw-r-- 1    822 Aug 17 15:25  008-route.yaml
-rw-rw-r-- 1  21941 Aug 17 15:25  alert-rules-control-plane.yaml
-rw-rw-r-- 1   1976 Aug 17 15:25  monitor-master.yaml
-rw-rw-r-- 1  43906 Aug 17 15:25  ovnkube-master.yaml
-rw-rw-r-- 1  30036 Aug 17 15:25  ovnkube-node.yaml
```

```text
$ ls -l bindata/network/ovn-kubernetes/managed/multi-zone-interconnect-tmp/
lrwxrwxrwx 1    44 Aug 17 15:25 005-service.yaml -> ../single-zone-interconnect/005-service.yaml
lrwxrwxrwx 1    42 Aug 17 15:25 008-route.yaml -> ../single-zone-interconnect/008-route.yaml
-rw-rw-r-- 1  7234 Aug 17 15:25 alert-rules-control-plane.yaml
lrwxrwxrwx 1    53 Aug 17 15:25 monitor-control-plane.yaml -> ../multi-zone-interconnect/monitor-control-plane.yaml
lrwxrwxrwx 1    47 Aug 17 15:25 monitor-master.yaml -> ../single-zone-interconnect/monitor-master.yaml
lrwxrwxrwx 1    53 Aug 17 15:25 ovnkube-control-plane.yaml -> ../multi-zone-interconnect/ovnkube-control-plane.yaml
lrwxrwxrwx 1    47 Aug 17 15:25 ovnkube-master.yaml -> ../single-zone-interconnect/ovnkube-master.yaml
lrwxrwxrwx 1    44 Aug 17 15:25 ovnkube-node.yaml -> ../multi-zone-interconnect/ovnkube-node.yaml
```

```text
$ ls -l bindata/network/ovn-kubernetes/managed/multi-zone-interconnect/
-rw-rw-r-- 1   7163 Aug 17 15:25 alert-rules-control-plane.yaml
-rw-rw-r-- 1   1862 Aug 17 15:25 monitor-control-plane.yaml
-rw-rw-r-- 1   9959 Aug 17 15:25 ovnkube-control-plane.yaml
-rw-rw-r-- 1  41563 Aug 28 11:18 ovnkube-node.yaml
```

### Openshift 4.14 fresh install
A fresh install of Openshift 4.14 will start directly with IC multizone. CNO will apply the YAMLs from `bindata/network/ovn-kubernetes/managed/multi-zone-interconnect/`


### Test Plan
CI upgrade jobs from 4.13 to 4.14 on all supported platforms on the CI/CD pipeline, with special attention to disruption tests, will validate the new upgrade path for OVN interconnect.

### Graduation Criteria
~Zero disruption all throughout upgrades from 4.13 to 4.14 on all supported platforms on the CI/CD pipeline.
#### Dev Preview -> Tech Preview
NA
#### Tech Preview -> GA
The feature will go GA in 4.14.
#### Removing a deprecated feature
NA

### Upgrade / Downgrade Strategy
Downgrades to 4.13 will go from OVNK IC directly to (non-IC) 4.13, skipping the two phases we introduced in the upgrade path.

### Version Skew Strategy
NA
### Operational Aspects of API Extensions
NA

#### Failure Modes
NA

#### Support Procedures
NA




## Implementation History
https://github.com/openshift/cluster-network-operator/pull/2154
https://github.com/openshift/cluster-network-operator/pull/1874
## Alternatives
NA
