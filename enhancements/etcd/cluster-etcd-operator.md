---
title: cluster-etcd-operator
authors:
  - "@hexfusion"
reviewers:
  - "@abhinavdahiya"
  - "@alaypatel07"
  - "@crawford"
  - "@deads2k"
  - "@sttts"
approvers:
  - "@derekwaynecarr"
  - "@smarterclayton"
  - "@sttts"
creation-date: 2019-10-1
last-updated: 2020-2-5
status: implementable
see-also: 
replaces:
superseded-by:
---

# Cluster etcd Operator

cluster-etcd-operator (CEO) is an operator that handles the scaling of etcd during cluster bootstrap and regular operation and provisioning etcd dependencies such as TLS certificates.

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

Having an operator manage the etcd cluster members requires the ability to scale up and scale
down the etcd membership ona Pod level independent of the Node itself. Meaning if you were to
scale down from 5 to 4 that does not necessarily mean that you must remove the node from the
cluster to achieve this. A good example would be replacing a failed member. In this case we need
to remove the failed member (scale down to 4) and re-add (scale back to 5). This action would not
require a new Node, only management of the static Pod on that Node. Scaling etcd requires a
balance of proper configuration and timing to minimize stress to cluster.

The most dangerous scaling is from a single-member cluster (bootstrap) to a two-member cluster
. This is dangerous because as soon as the etcd API is notified of the new member the cluster
loses quorum until that new member starts and joins the cluster. To account for these dangers we
utilize the init containers to provide structured gates. These gates allow for actionable
observations between the static pod containers and the operators controllers. To eliminate a race
condition with new members attempting to join the cluster we anchor the scale process on a key
written to a ConfigMap. This ConfigMap also give an anchor point in the sync loop in the case of
failure.

### Goals

1. Deploy etcd instance on the bootstrap node to fast track CVO deployment.
2. Scaling up of etcd peer membership.

### Non-Goals

1. Automated disaster recovery of a cluster that has lost quorum.
2. Automation of disaster recovery tasks (human interaction required)
3. Replace failed member assuming quorate cluster.
4. Scaling down of etcd peer membership.

## Proposal

### HostEndpointsController
This controller maintains the list of endpoints in `oc -n openshift-etcd get endpoints/host-etcd`.
It always places the IP addresses and DNS names for every master node, even those without pods.
It never creates the instance.  In the future, it should honor the `oc -n kube-system get configmap/bootstrap`.

#### Reads
 1. `oc get nodes -l node-role.kubernetes.io/master:`
 2. `oc -n openshift-etcd get endpoints/host-etcd` - to find the bootstrap host IP
 3. DNS to convert node `.status.addresses[internalIP]` into DNS names

#### Writes
`oc -n openshift-etcd get endpoints/host-etcd`
 1. `.annotations["alpha.installer.openshift.io/dns-suffix"]` is set to infrastructure.config.openshift.io|.status.etcdDiscoveryDomain
 2. .spec.address.hostname is set to the part of the DNS name minus the etcdDiscoveryDomain
 3. .spec.address.ip is set to the first internal IP of the node.

#### Consumers
`oc -n openshift-etcd get endpoints/host-etcd` can be used to build etcd clients.
It is used in this operator and in the KAS-o and OAS-o.  

### EtcdMembersController
This controller directly contacts etcd to determine the status of individual members.
It is a read-only controller that ensures visibility into the members in etcd.
It is currently time-driven (simplest possible thing that could work), in the future, it should establish a watch against etcd.

#### Reads
Membership directly from etcd.

#### Writes
.status.conditions
 1. EtcdMembersDegraded - true if there any unhealthy or unknown members in the list.
 2. EtcdMembersProgressing - true if there are any members that at not-started.
 3. EtcdMembersAvailable - false if there are not enough healthy members for quorum.

### ResourceSyncController
This controller copies certs, keys, and CA bundles from sources to destinations where they are consumed.
Things like the etcd-serving-ca from the `ns/openshift-config` to `ns/openshift-etcd-operator` as a for instance. 

### ConfigObserver
Dead for now, but a place holder if we ever had a means to configure or tune etcd.

### StatusController
The standard status controller that unions *Degraded, *Available, *Progressing conditions from etcds.operator.openshift.io for
summarization to a single Degraded, Available, Progressing condition in clusteroperators.config.openshift.io/etcd.

### BootstrapTeardownController
Removes the etcd-bootstrap member from the etcd cluster after enough etcd members have joined.

#### Reads
 1. etcd membership -
    1. to determine if etcd-bootstrap is present and whether enough other members are present.
    2. to ensure that no etcd member is unhealthy
 2. kubeapiserver.operator.openshift.io and `oc -n openshift-kube-apiserver get configmap` -
    to determine if the kube-apiservers have observed the replacement etcd members.  If they haven't
    removing the etcd-bootstrap will result in an outage.
 3. `oc -n kube-system get configmap/bootstrap` to determine if bootstrapping is complete.
    If it is present, then we can definitely remove etcd-bootstrap.
 4. `oc -n kube-system get event bootstrap-finished` to determine if cluster-bootstrap is complete.
    This or the bootstrap configmap is enough to allow removing the member to proceed.
    If the member is removed too early, it can cause the bootstrap-kube-apiserver to fail, which causes
    bootstrapping to fail.
    
#### Writes
 1. directly modifies etcd membership by removing the etcd-bootstrap member.
 
### StaticResourceController
The standard controller for creating fixed (non-changing) resources for an operator.
This is things like our namespaces, serviceaccounts, rolebindings, etc.

### ClusterMemberController
This controller adds members to etcd (equivalent of `etcd add-member`).
It can only add one member at time.

#### Reads
 1. etcd membership - to determine if every member in the cluster is healthy.
    If they are not healthy, then no new member can be added.
 2. `oc -n openshift-etcd get pods`, filtered to etcd-pod-* - to determine if there is an unready pod
    which is a target for adding to the cluster.
    1. pod must be unready
    2. pod must not already be in the etcd cluster
    3. pod must have a DNS name
 3. DNS to determine the etcdDiscoveryDomain based name of master nodes.
 4. master nodes, to determine their first internal IP address.

#### Writes
 1. etcd membership - adds a node's etcd DNS name to the member list
 
### StaticPodController
The standard controller for managing static pods, like the KAS-o, KCM-o, KS-o.

### TaregetConfigController
This controller shapes the static pod manifest itself.  It has some unique features of its output.

#### Static Pod Shape
 1. Every static pod has environment variables, configmaps, and secrets for every etcd member.
    Put another way, the static pod can become the static pod for any member.
    There are env vars like: `NODE_node_name_ETCD_PEER_URL_HOST` for each node_name.
 2. During static-pod-installation, the installer-pod substitutes NODE_NAME and NODE_ENV_VAR_NAME directly into the bytes
    laid into /etc/kubernetes/manifests/etcd-pod.yaml.
    This allows a particular node to have a static pod with the right parameters by selecting the env var.
    For example, that allow nodes to have arguments like `--peer-listen-url=$NODE_NODE_ENV_VAR_NAME_ETCD_PEER_URL_HOST`,
    that will select the "correct" env var for their node. 
 3. The static pod contacts the existing etcd to determine membership.
    It waits until it is able to know that it is in the member list before trying to start.
    When it does this, it is able to determine the member list to use to launch.

### Render Command
Populates the static pod manifest used on the bootstrap node and other resource dependencies during the cluster bootstrap.

#### File Reads
1.) cluster-network-file - to determine the `ClusterCIDR` and `ServiceCIDR`. These values are used to conclude if the cluster is
    single stack.
2.) cluster-config-file - checks the declared `MachineCIDR` of the cluster from the install-config. Render uses the `MachineCIDR`
    of the cluster to validate which IP address on the bootstrap interfaces is the BootstrapIP by making sure it is on the same
    network as the `MachineCIDR`.

#### File Writes
1.) etcd-member-pod.yaml - The static pod manifest for the bootstrap etcd instance.
2.) manifests/00_etcd-host-service.yaml - host-etcd-2 service
3.) manifests/00_openshift-etcd-ns.yaml - openshift-etcd namespace

### Implementation Details/Notes/Constraints

#### Installer

During install we provision a single member etcd cluster on the bootstrap node. This etcd
instance allows for the new master node apiservers to connect the bootstrap etcd endpoint early
and allow CVO to be deployed much faster than the current implementation. Instead of waiting for
quorum of a 3 member etcd cluster to bootstrap we have a single member etcd available very
quickly. As etcd is now managed by the operator the cluster-etcd-operator will reconcile state
and scale up to an eventual 4 node etcd cluster (bootstrap plus 3 masters).  At that point, the
operator will remove the bootstrap etcd instance completing the bootstrap process scaling the
cluster down to 3.

bootkube.sh:

cluster-etcd-operator render[1] command generates the static pod manifest for the etcd deployed
on the master node. After this manifest is persisted to disk on the bootstrap node we copy[2] it
to the manifests directory. This static pod has a single init container `certs`. Because we are
starting before the operator exists we utilize the standalone etcd cert signer[3] used in 4.1 - 4.3.

[1] https://github.com/openshift/installer/blob/552f107a2d6b062f009c94c65be0f195f2c9168c/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template#L124

[2] https://github.com/openshift/installer/blob/release-4.4/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template#L140

[3] https://github.com/openshift/installer/blob/552f107a2d6b062f009c94c65be0f195f2c9168c/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template#L83

## Design Details


### Test Plan

1. Unit tests for all controllers
1. New e2e suite to exercise scaling operations and failure modes.
1. QE will be asked to have cluster-etcd-operator enabled in all test clusters, especially long-lived clusters
1. The OpenShift Online environments will always run with cluster-etcd-operator enabled


### Graduation Criteria

##### This feature will GA in 4.3 as long as:

1. Thorough end to end tests, especially around scaling and failure cases
1. Thorough unit tests around core functionality
1. Docs detailing functionality and updates to disaster recovery

### Upgrade / Downgrade Strategy

Assume that I update the above to indicate that we're using a static pod operator like the kube-apiserver.
It rolls out one node at a time, not coordinated with the MCO in any way, without regard for PDBs.
It does prefer updating it's own crashlooping or unready pods to bringing down working members (we already did this).

#### Upgrade from 4.3 to 4.4

 1. The 4.4-etcd-staticpod moves the /etcd/kubernetes/manifests/etcd-member.yaml to a backup location before trying to start etcd.
 2. This causes the 4.3-machineconfigpool to go degraded because the file that it tries to maintain is gone.
    1. We discovered that the MCO does not upgrade past a degraded condition.
    2. This appears to be flaw in the MCO that prevents upgrading past bugs, but for the short term, we will simply
    skip evaluating the file in question.
 3. If the master node restarts using a 4.3-machineconfigpool, the old /etcd/kubernetes/manifests/etcd-member.yaml will come back.
    This is ok because the 4.4-etcd-staticpod will remove it again and try to claim the same port.
 4. The 4.4-mco will not have an etcd-member.yaml file.  When the 4.4-mco restarts master nodes, they will start back up
    and not have a /etcd/kubernetes/manifests/etcd-member.yaml.  This means the 4.4-machineconfigpool will be healthy again.

#### Downgrade from 4.4 to 4.3

The cluster can function without intervention, but to fully restore 4.3, manual intervention is required.

 1. The 4.4-etcd-pod exists on every master.  Recall that it moves /etcd/kubernetes/manifests/etcd-member.yaml to a backup location before trying to start etcd.
 2. The 4.4-etcd-pod are still maintained by the 4.4 etcd operator because the CVO doesn't know how to remove any resources.
 3. If left, this will leave a 4.3 cluster with a 4.4 style etcd and degraded machineconfigpools.
    The cluster can run in this state for a very long time.
 4. To clean up, upgrade again.  Or....
 5. Delete the openshift-etcd-operator namespace and wait for it to be removed.
 6. **One master at at time**... 
    1. move the 4.4-etcd-pod to a backup location
    2. restore the etcd-member.yaml from its backup location
    3. wait for the etcd-member to rejoin
    4. move to the next master.

### Version Skew Strategy

TODO

## Implementation History

TODO

## Drawbacks

TODO

## Alternatives

TODO

## Infrastructure Needed [optional]

TODO
