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
2. Scaling up/down of etcd peer membership.
3. Replace failed member assuming quorate cluster.
4. Automation of disaster recovery tasks (human interaction required)

### Non-Goals

1. Automated disaster recovery of a cluster that has lost quorum.

## Proposal

API:

```

type EtcdScale struct {
    Name       string            `json:"name,omitempty"`
    Cluster    EtcdCluster       `json:"cluster,omitempty"`
}

type EtcdCluster struct {
    ID       uint64             `json:"ID,omitempty"`
    Members  []Member           `json:"members,omitempty"`
}

type Member struct {
    ID         uint64            `json:"ID,omitempty"`
    Name       string            `json:"name,omitempty"`
    PeerURLS   []string          `json:"peerURLs,omitempty"`
    ClientURLS []string          `json:"clientURLs,omitempty"`
    Conditions []MemberCondition `json:"conditions,omitempty"`
}

type MemberCondition struct {
    // type describes the current condition
    Type MemberConditionType `json:"type"`
    // status is the status of the condition (True, False, Unknown)
    Status v1.ConditionStatus `json:"status"`
    // timestamp for the last update to this condition
    // +optional
    LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
    // reason is the reason for the condition's last transition.
    // +optional
    Reason string `json:"reason,omitempty"`
    // message is a human-readable explanation containing details about
    // the transition
    // +optional
    Message string `json:"message,omitempty"`
}

type MemberConditionType string

const (
    // Ready indicated the member is part of the cluster and endpoint is Ready
    MemberReady MemberConditionType = "Ready"
    // Unknown indicated the member condition is unknown and requires further observations to verify
    MemberUnknown MemberConditionType = "Unknown"
    // Degraded indicates the member pod is in a degraded state and should be restarted. Currently unused
    MemberDegraded MemberConditionType = "Degraded"
    // Remove indicates the member should be removed from the cluster
    MemberRemove MemberConditionType = "Remove"
    // MemberAdd is a member who is ready to join cluster but currently has not. Currently unused
    MemberAdd MemberConditionType = "Add"
)
```

Etcds: Actions to reconcile cluster membership are based of the observations reported by the
configobservation controller.

The example below shows an observed member failure caused by catastrophic failure. The result of
the containers readiness probe failing the member is taken from the members bucket and placed
into pending bucket `( Degraded = true, Progressing = true )`. After further observation it is
confirmed that the container is actively in `CrashLoopBackOff`. The result of these observations
is a new status of `Remove`. The remove status is then acted upon by the `clustermembercontroller`
which removes the member from the etcd cluster by calling `MemberRemove` against the etcd
`Cluster` API. The `staticpod` controller will also respond by stopping the static Pod, removing
the existing data-dir and then starting the static Pod. The discovery init container then will
wait for status now observed as `Unknown` to change to Add before it continues
. `configobservation` mcontroller observes the Pod and confirms etcd-member container is no
longer CrashLooping and sets status to Add after which the member is added back to the etcd
cluster with appropriate values populated for `ETCD_INITIAL_CLUSTER` allowing it join. Now
readiness probe will succeed and member is added back to the member bucket `( Degraded = false
, Progressing = false )`.
```
apiVersion: v1
items:
- apiVersion: operator.openshift.io/v1
  kind: Etcd
  metadata:
    annotations:
      release.openshift.io/create-only: "true"
    creationTimestamp: "2019-10-02T12:38:41Z"
    generation: 2
    name: cluster
    resourceVersion: "108019"
    selfLink: /apis/operator.openshift.io/v1/etcds/cluster
    uid: 905931e7-e511-11e9-8e06-020bc572eaf2
  spec:
    forceRedeploymentReason: ""
    logLevel: ""
    managementState: Managed
    observedConfig:
      cluster:
        members:
        - name: etcd-member-ip-10-0-154-109.us-east-2.compute.internal
          peerURLs: https://etcd-0.sbatsche.devcluster.openshift.com:2380
          status: Ready
        - name: etcd-member-ip-10-0-164-65.us-east-2.compute.internal
          peerURLs: https://etcd-2.sbatsche.devcluster.openshift.com:2380
          status: Ready
        pending:
        - name: etcd-member-ip-10-0-135-168.us-east-2.compute.internal
          peerURLs: https://etcd-1.sbatsche.devcluster.openshift.com:2380
          status: Remove
```

#### InitContainers

This section will walk through the init containers for the etcd-member static pod and outline how
they work and available paths.

Preface:

Because of the dependcy some of the init containers have on communications to the apiserver we
utilize `InClusterConfig()`[1] to create the client config. `InClusterConfig` requires
the availablity of `KUBERNETES_SERVICE_HOST`  and `KUBERNETES_SERVICE_PORT` as well as the CA and
token for the default service account. But because we are a static pod our containers do not
automatically get mounted with a default service token. To work around this limitation we have a
controller `staticsynccontroller`[2] which is deployed as DaemonSet whos task is to sync these
assets to disk as soon as they become available. These resources are mounted to the init
containers and thus give us the same general functionality of a regular kube Pod.

[1] https://github.com/kubernetes/client-go/blob/69012f50f4b0243bccdb82c24402a10224a91f51/rest/config.go#L447

[2] https://github.com/openshift/cluster-etcd-operator/blob/release-4.4/pkg/cmd/staticsynccontroller/staticsynccontroller.go

wait-for-kube:

This init containers task is to wait[1] for the existance of the service account resources synced
by staticsynccontroller to appear. Once this check completes we check that the ENV dependencies
`KUBERNETES_SERVICE_HOST`  and `KUBERNETES_SERVICE_PORT` are also met for `InClusterConfig()`. A
known limitation of this init is that in certain circumstances such as DR kube will never be
available before etcd. A proposed solution[2] for this is a check for the existance of the etcd
state file. If the db file exists we can assume this etcd has previously started or is currently
being recovered.

[1] https://github.com/openshift/cluster-etcd-operator/blob/6ec52009735c8ccdc38afb7fa03e62f852570ff4/pkg/cmd/waitforkube/waitforkube.go#L50

[2] https://github.com/openshift/cluster-etcd-operator/pull/73

discovery:

The discovery container is part of setup-etcd-environment located in machine-config-operator repo
. This containers purpose is to populate ENV variables that are later consumed by later init
containers and the etcd binary itself. We are currently still dependent on SRV DNS records for
both CEO bootrapping and the 4.1 - 4.3 SRV bootstrapping. The init currently has a few code
paths outlined below.

In all code paths we do a preflight check that the hostIP address of the master node matches with
one of the DNS lookup results made against each A record returned in the SRV query against the
`etcd-server-ssl` service. We do this validation because this IP address is added to the SAN IP of
etcd certs. This IP must match that of the etcd server instance or TLS auth will fail.

discovery code paths:

1.) SRV bootstrap: If the flag `--bootstrap-srv` is set true we will populate
  `ETCD_DISCOVERY_SRV` which is the domain that the etcd server will use to populate the initial
   list of peers to bootstrap the cluster with.

2.) CEO bootstrap: When we are boostrapping using the operator we need to populate a few other
 important ENV variables.

- `ETCD_NAME` 

- `ETCD_INITIAL_CLUSTER` is used to populate the list of active peers that the etcd instance will
 use to join the cluster. Once this variable is consumed it is no longer used again as the member
 state will be persisted to the member bucket of the state file and managed by the etcd server
 Membership API. The values for this variable are populated by the membership controller and
 persisted to a configmap `member-config`. The discovery init container will wait for the
 configmap annotation to contain the name[1] of the local static pod. When this criteria is met
 this static pod will begin the process of scaling by populating `ETCD_INITIAL_CLUSTER` with the
 list of current etcd peers provided by configmap and then appending itself as a new member. 

- `ETCD_INITIAL_CLUSTER_STATE` this is always set to `existing` as we are joining an existing
 cluster

- `ETCD_ENDPOINTS` this is a comma seperated list of existing endpoints. We use this list in the
 membership container as we attempt to validate membership against the Membership API.

example /run/etcd/environment populated for CEO scaling
```
export ETCD_NAME=etcd-member-ip-10-0-131-156.ec2.internal
export ETCD_INITIAL_CLUSTER=etcd-bootstrap=https://10.0.0.106:2380,etcd-member-ip-10-0-153-109.ec2.internal=https://etcd-1.cluster-name.devcluster.openshift.com:2380,etcd-member-ip-10-0-131-156.ec2.internal=https://etcd-0.cluster-name.devcluster.openshift.com:2380
export ETCD_INITIAL_CLUSTER_STATE=existing
export ETCD_ENDPOINTS=https://10.0.0.106:2379,https://10.0.153.109:2379
ETCD_ESCAPED_LOCALHOST_IP=127.0.0.1
ETCD_WILDCARD_DNS_NAME=*.cluster-name.devcluster.openshift.com
ETCD_DNS_NAME=etcd-0.cluster-name.devcluster.openshift.com
ETCD_IPV4_ADDRESS=10.0.131.156
ETCD_ESCAPED_IP_ADDRESS=10.0.131.156
ETCD_ESCAPED_ALL_IPS=0.0.0.0
ETCD_LOCALHOST_IP=127.0.0.1
```
3.) Disaster recovery: In DR we must be able to circumvent any kube dependcies DR scripts
 populate required ENV `ETCD_INITIAL_CLUSTER` and `ETCD_INITIAL_CLUSTER_STATE`. Which the init
 container makes sure are persisted through the process.

[1] https://github.com/openshift/machine-config-operator/blob/release-4.4/cmd/setup-etcd-environment/run.go#L270

certs: The certs init container invokes the `mount` command of `cluster-etcd-operator`. This
command takes two params  `commonname` which is the common name defined in the SAN. This key is
used to search for the proper secret containing the TLS certs for the etcd instance. We will wait
[1] until find this secret and then we perisit it to disk. These certs are then consumed by the
etcd server and proxy.

[1] https://github.com/openshift/cluster-etcd-operator/blob/096795bd444fd6988eca576b1e65d58e9c161018/pkg/cmd/mount/mount.go#L94

membership: This is the last init container and its purpose is to confirm that the local etcd
instance is a member of the cluster. It does this by querying Cluster API for MemberList. Once it
matches its localname to a member on that list we start `etcd-member` and `etcd-metrics
` containers. We run this query every 2 seconds.

### User Stories

#### Story 1

User installs cluster and after a period of time a reboot on a node causes catastrophic failure
resulting in etcd-member container crashlooping. Cluster etcd operator observes this failure and
replaces failed static Pod without human interaction.

#### Story 2

User after install of cluster needs to replace Node because of hardware failure. When Node is
removed operator scales down etcd cluster. When new Node is added to cluster, the operator
performs scale up process adding the etcd member to the cluster.

#### Story 3

User would like to scale to 5 node etcd cluster from 3. User adds nodes to cluster and operator
scales etcd accordingly.

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
to the manifests directory. This static pod has a shortened list of init containers with include
`discovery` and `certs`. Because we are starting before the operator exists we utilize the
standalone etcd cert signer[3] used in 4.1 - 4.3.

[1] https://github.com/openshift/installer/blob/552f107a2d6b062f009c94c65be0f195f2c9168c/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template#L124

[2] https://github.com/openshift/installer/blob/release-4.4/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template#L140

[3] https://github.com/openshift/installer/blob/552f107a2d6b062f009c94c65be0f195f2c9168c/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template#L83

#### Controllers

##### clusterMember



##### staticPod

staticPod controller is deployed as a DaemonSet and takes action on the physical static Pod
itself. We observe the status of the containers in `etcd-member` and conclude from that data if
any of these containers are in `CrashLoopBackOff`. If this is true we then verify if the member
status has been updated to reflect `MemberRemove` by the observation controller. The reason for
this validation is an extra check to avoid removing a member with a transient failure. The
observation controller will wait for the container to have been in `CrashLoopBackOff` for 5mins
before applying `MemberRemove` status. Before we stop the static pod we extract a copy of the
etcd-member.yaml from machine-config. Stopping the Pod involves removing the static pod spec from
the manifests directory. Once the Pod is stopped we wait 10secs then remove the underlying etcd
data-dir. Starting the Pod involves persisting the spec we pulled from machine-config back to disk.

The idea here is to resolve single etcd member failures automatically which in the past would
require a disater recovery step.

##### staticSync

staticSync controller handles the issue of providing static Pods with assets allowing static Pods
to use the default service account as normal Pods do. These assets include 3 files namespace, ca
.crt, and token. We wait for the presence of these files in the container[1] and then we sync to
the hostDir '/etc/kubernetes/static-pod-resources/etcd-member/secrets/'. These files are
later mounted by containers in the etcd-member static Pod to generate our apiserver client
config via `InClusterConfig`[2].

[1] https://github.com/openshift/cluster-etcd-operator/blob/11dcd2b8d1e3d0420bdb6db969721b39a35367ea/pkg/cmd/staticsynccontroller/staticsynccontroller.go#L184

[2] https://github.com/kubernetes/client-go/blob/69012f50f4b0243bccdb82c24402a10224a91f51/rest/config.go#
L447

##### etcdcertsigner

The certsigner controller populates the SAN details (IP, DNS) required for TLS certs to be
generated. It then populates[1] those certs on a per Pod basis including key pairs for etcd peer
, server, metrics. Those are then persisted to Secrets which later are queried by the mount
subcommand in the certs init container and persisted to disk which is then consumed by the etcd
-member and etcd-metrics containers.

[1] https://github.com/openshift/cluster-etcd-operator/blob/096795bd444fd6988eca576b1e65d58e9c161018/pkg/operator/etcdcertsigner/etcdcertsignercontroller.go#L207

##### configobservation

The configObservation controller has the primary job of converting observations from etcd endpoints into appropriate member/pending keys for etcds.

Buckets pending vs member:

The condition for each are based on the existence of records in `Addresses` (members) and `NotReadyAddresses` (pending).

```
apiVersion: v1
kind: Endpoints
[...]
subsets:
- addresses:
  - ip: 10.0.154.109
    nodeName: ip-10-0-154-109.us-east-2.compute.internal
    targetRef:
      kind: Pod
      name: etcd-member-ip-10-0-154-109.us-east-2.compute.internal
      namespace: openshift-etcd
       resourceVersion: "12228"
      uid: c0fb9fa9-e511-11e9-8e06-020bc572eaf2
  notReadyAddresses:
  - ip: 10.0.135.168
    nodeName: ip-10-0-135-168.us-east-2.compute.internal
    targetRef:
      kind: Pod
      name: etcd-member-ip-10-0-135-168.us-east-2.compute.internal
      namespace: openshift-etcd
      resourceVersion: "402469"
      uid: 88b71e3d-e546-11e9-a40a-02cd8303909c
```

status:

- member:

The status of the etcd Pod is based on a few simple principles. First if the endpoint is `Ready
 `and all containers are `Ready` so is the member and status is `MemberReady`. `Ready` as defined
by the endpoint is based on the readiness probe on `etcd-member`. A member who is not part of the
cluster or is not observable by the cluster will have the status `MemberUnknown`. An example of
an etcd instance that can be a member of the cluster but is still `MemberUnknown` is the
bootstrap etcd. Because the kubelet on bootstrap node is in standalone mode. It does not report
the status of the etcd-member static pod containers to the rest of the cluster. For this reason
we can not conclude it is  MemberReady` in the same way we do other members.

- pending:

The default status of a pending member is `MemberUnknown` as we don't really know the reason for
the notReadyAddress state without further investigation. When `etcd-member` Pod starts the init
containers will wait for various observations to continue. Because we are waiting in a not Ready
state we can be simply waiting for direction from the operator. To validate this we ensure that
the Pods containers are not currently in `CrashLoopBackoff`. If we pass the `isPodCrashLoop`[1
] when then check `isPendingReady`[2]. `isPendingReady` makes sure that the init containers have
exited 0, because inits containers including certs are now complete we can begin the process of
scaling etcd via clustermembercontroller and the status is updated to `MemberReady`.

[1] https://github.com/openshift/cluster-etcd-operator/blob/release-4.4/pkg/operator/configobservation/etcd/observe_etcd.go#L192


```
apiVersion: v1
items:
- apiVersion: operator.openshift.io/v1
  kind: Etcd
  metadata:
    annotations:
      release.openshift.io/create-only: "true"
    creationTimestamp: "2019-10-02T12:38:41Z"
    generation: 2
    name: cluster
    resourceVersion: "108019"
    selfLink: /apis/operator.openshift.io/v1/etcds/cluster
    uid: 905931e7-e511-11e9-8e06-020bc572eaf2
  spec:
    forceRedeploymentReason: ""
    logLevel: ""
    managementState: Managed
    observedConfig:
      cluster:
        members:
        - name: etcd-member-ip-10-0-154-109.us-east-2.compute.internal
          peerURLs: https://etcd-0.sbatsche.devcluster.openshift.com:2380
          status: Ready
        - name: etcd-member-ip-10-0-164-65.us-east-2.compute.internal
          peerURLs: https://etcd-2.sbatsche.devcluster.openshift.com:2380
          status: Ready
        pending:
        - name: etcd-member-ip-10-0-135-168.us-east-2.compute.internal
          peerURLs: https://etcd-1.sbatsche.devcluster.openshift.com:2380
          status: Remove
```

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

#### Upgrade from 4.3 to 4.4

quorum-guard: currently quorum-guard is a deployment in the `machine-config-operations` namespace the bash logic used
to validate quorum expects certs to exist in /etc/kubernetes/static-pod-resources/etcd-member/. The target controller
in CEO now installs these certs by revision on disk in a path such as /etc/kubernetes/static-pod-resources/etcd-pod-N/.

quorum-guard was added to MCO's CVO manifests because CEO did not yet exist. Because we gracefully handle the certs in
secrets in the `openshift-etcd` namespace it makes sense to move quorum-guard with the addition of the operator.

1.) add quorum-guard deployment to CEO manifests and deploy into `openshift-etcd` namespace mounting the proper secrets.
2.) remove quorum-guard from CVO controlled manifests in MCO repo.
3.) create a CVO controlled Jobs that removes the Deployment and PDB from MCO namespace.

#### Downgrade from 4.4 to 4.3

quorum-guard: if another PDB and quorum-guard are running this should not cause an issue. We could document the steps to
manually handle removal of quorum-gaurd in the `openshift-etcd` namespace.

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
