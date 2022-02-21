---
title: OVN on Hypershift
authors:
  - "@russellb
  - "@squeed"
reviewers:
  - "@dcbw"
  - @danwinship
  - @tssurya
  - @numans
approvers:
  - TBD
creation-date: 2022-02-01
last-updated: 2022-02-08
tracking-link: 
  - "https://issues.redhat.com/browse/SDN-2589"
see-also:
  - "https://hypershift-docs.netlify.app/"
---


# OVN on Hypershift hosted

## Summary

[Hypershift](https://hypershift-docs.netlify.app/) is a new deployment model for OpenShift which removes the requirement that clusters be "self-hosted". Rather, a given hosted cluster's control plane actually resides as pods on another "management" cluster.

Adapting the OVN networking plugin to the Hypershift model requires some careful design. Specifically, OVN provides its own control plane and datastore, all of which will be hosted external to the hosted cluster.

Thus, the OpenShift OVN deployment model must be updated to support split control planes. Likewise, the Network Operator must be able to manage components on both the control plane as well as the hosted cluster.

## Motivation

We will elide the full motivation for hosted control planes (i.e. the Hypershift model). However, hosting the OVN control plane off-cluster has the same advantage as hosting the Kubernetes control plane:
- potentially higher reliability
- no special master nodes.
- no chicken-egg problem while bootstrapping (i.e. cannot use most cluster features until networking comes up)
- More available features (e.g. volumes, stateful sets)
- etc.

The most significant motivation is the availability of StatefulSets for the OVN databases. In OpenShift,
we use the ovsdb raft replication mode to provide high availability on master nodes. Managing this
manually is error prone, especially in the case of lost masters. However, self-hosted OVN cannot use
StatefulSets, since they require a functioning network. OVN on Hypershift does not suffer this
chicken-and-egg problem.

### Goals

Hypershift clusters are easily deployed using OVN, with the entire control plane residing off-cluster. OVN is the default networking provider for Hypershift.

### Non-Goals

- Removing the OVN south-bound DB (sbdb) as the primary integration point (for nodes)
- Full network connectivity between the management and hosted clusters
## Proposal

The gist of the design is:

1. All OVN control-plane components (ovnkube-master, NB/SB databases, ovn-northd), as well as the CNO, reside on the management cluster
2. The CNO manages components in both clusters
3. Communication between the nodes and the OVN control plane is via the southbound OVN database, exposed from the management cluster via a Route
4. No changes to OVN are required


### User Stories

I, a Hypershift user, find running Kubernetes control planes terribly boorish. I would like all the fancy features
available to me through ovn-kubernetes (EgressIP, EgressFirewall, ICNI, et cetera) without having to
run that pesky control plane. My cluster networking Just Works without causing me any problems.

### API Extensions

n/a

## Design Details

### Cluster Network Operator (CNO) design

The majority of this proposal's implementation effort will be in CNO. CNO is a complicated operator, with 10 controllers operating in varying levels of the stack.
The full list of controllers within the CNO can be found [here](https://github.com/openshift/cluster-network-operator/blob/master/docs/operands.md).

Most controllers within the CNO will not need to be aware of the management cluster, and will thus remain unchanged. All of their operands are entirely
on the hosted cluster and Hypershift will be entirely transparent.

Work needed to CNO includes:
- Make specific controllers aware of multiple clusters
- Refactor MTU detection
- Managing ovsdb PKI across both clusters
- Un-hard-coding namespaces. The CNO currently assumes certain namespaces.

It is assumed the CNO's configuration objects (CRDs) reside in the hosted cluster. This is to avoid any potential CRD version conflicts while multiple versions of the CNO are running on the management cluster.

### OVN-Kubernetes 

OVN and OVN-Kubernetes have a control plane and node components. By and large, OVN and OVN-kubernetes will not require any code changes. Rather, how they are deployed must be rethought. This will primarily be implemented as changes in the CNO.

```text
                          │
  Management Cluster      │       Hosted Cluster
                          │
┌──────────────────┐      │      ┌───node─────────┐
│  Route (public)  │◄─────┼──────┤ ovnkube-node   │
└────────┬─────────┘      │      │                │
         │                │      │ ovn-controller │
┌────────▼───┐            │      │                │
│ ovs-db-N.. │            │      │ openvswitch    │
└┬───────────┴───┐        │      │                │
 │  nbdb | sbdb  │        │      └────────────────┘
 └───────▲───────┘        │
         │                │
         │                │
         │                │
┌────────┴───────┐        │
│ ovnkube-master │        │
│   ovn-northd   │        │
└────────────────┘        │
```

**Control-plane:**
- ovn-kube master: Watches apiserver, creates objects in nbdb
- ovn-northd: Translates nbdb objects to sbdb flows
- nbdb, sbdb: Raft-based highly available configuration databases (northbound and southbound)

**Node:**
- ovn-kube node: Does some node-level configuration
- ovn-controller: Programs flows from sbdb to Open vSwitch
- Open vSwitch (OVS): actual packet handling

The control plane components will be deployed as pods on the management cluster. The node components remain on the hosted cluster. The CNO will be made aware of both of these clusters so it can deploy the right compoments in the right place.

The ovnkube-master pod currently runs using host networking. This must be changed to use the cluster network, instead.

The CNO creates a headless service called `ovnkube-db` for the OVN databases. This needs to be updated to have a ClusterIP, since in this updated architecture, we need to point a `Route` at this `Service`. The CNO will also be responsible for creating this `Route` and configuring `ovn-controller` in the hosted cluster to connect to the associated hostname for southbound DB access.

Anywhere the CNO assumes that `openshift-ovn-kubernetes` is the target namespace for OVN components must be updated. That will be the namespace for Node components in the hosted cluster, but not for the components that run in the hosted control plane.

#### OVN Control plane

Deploying the OVN control plane will require some general refactoring, as they no longer will run as HostNetwork, privileged pods. Rather

**ovsdb-server (nbdb, sbdb)**: *should* not require any changes. This depends on Raft working when nodes are identified by DNS (and IPs change).

**OVN-Kubernetes master:** *may* need some changes to perform leader election via the management cluster. Otherwise, it doesn't need to be aware of the management cluster.

#### OVN DBs

The OVN databases (nbdb, sbdb) will continue to use the internal raft-based replication. However, they will be deployed as a StatefulSet instead. Rather than using HostPath volumes, they will use whatever persistent storage volumes are available in the management cluster.
In the case of AWS, there are some important details to recognize: EBS volumes cannot be moved between AZs. Thus, where they are first provisioned is critical.

The databases need a N/2+1 quorum to maintain availability. We need to protect against disruption
- zone-based anti-affinities, to ensure every DB is in a different AZ.
- Suitable PodDisruptionBudget, to ensure we never take a DB down by maintenance.

There has some been desire to support "swinging" control planes between clusters. As long as the PVs are persisted, this should be possible.
We will need to test how the raft implementation handles name changes.


#### OVN Node components
**ovn-controller**: itself will not require any changes as it appears to already have support for the new functionality required. It must support configuration of the SB (southbound) database connection as a URL instead of an IP address. Further, when using TLS, it must set SNI (Server Name Indication) so that the management cluster is able to route the TLS connection to the correct database instance.

**OVN-Kubernetes node:** should not require any changes.

#### Node-DB connectivity
By (ab)using the TLS Passthrough feature of OpenShift Routes, we can expose the southbound database directly to the target cluster.

### Monitoring

The control plane components, as pods in the management cluster, will need to be monitored by the management monitoring infrastructure. We will need to create a ServiceMonitor for each control plane in question.

One complication is that we should create AlertManager alerts on a per-customer basis. That means, inevitably, that we will need some kind of management layer for these alerts. What form this takes is not yet defined.


#### MTU detection
Right now, the CNO assumes every node has the same MTU as the node on which it runs. This is, obviously, not correct anymore. Thus, we
will need to add a MTU "prober" Job that executes, once, to determine MTU in the target cluster. It will write the result in a ConfigMap.
The CNO will then consume this as needed.

Setting MTU correctly is critical -- too large, and we suffer hard-to-diagnose connectivity issues. Too small, and performance suffers.
Given that AWS supports jumbo frames, and GCP nodes have a <1500 MTU, this is not a theoretical problem.

### Risks and Mitigations

**OVSBD Raft and DNS** - the ovsdb team does not currently test Raft with dynamic member IPs. While the team believes this should work, it must be added to their test plan. Also, we have missed the window for any new changes to OVS for the 4.11 release. If any changes to ovsdb are needed, we either need exceptional processes or workarounds.

### Open Questions 


What storage / volume types will be available for the OVN DBs to use? Can we assume a functioning CSI environment? Will this storage have sufficient performance? Can we generate a reasonable size prediction? To answer this, we will need to utilize the existing Perf&Scale processes to characterize performance. Given that etcd on Hypershift also uses a PV-backed volume, this should not be an issue.

Can we minimize cross-AZ network traffic? We only need to talk to the leader of each raft cluster, but there's no guarantee that nbdb
and sbdb will elect the same leader at any point in time. Can we add a mode to ovn-northd that makes this possible? This may need to be
a follow-up improvement.

What bandwidth will the external sbdb traffic consume? Will the added latency cause any issues? Should we enable ovn-controller conditional monitoring? At the time this document was being drafted, the OVN team is re-examining the performance impacts of ovn-controller's conditional monitor feature.

If the bandwidth or performance is not acceptable, will we have to deploy some kind of ovsdb proxy on the infra nodes? Assuming we cannot *require* infra nodes, will we nevertheless need to make that option available to customers? At present, the plan is to defer this unless
experience shows this is necessary.

How do we report control-plane status -- which references "invisible" pods -- to the end-user? Does the end-user need to know that ovsdb is somehow degraded? What do we show in the `network` ClusterOperator object?

How do we report control-plane status to the SRE teams? They will need to react to losses of quorum, etc. Can we deploy alerts? How do we handle cross-version alert definitions?

### Test Plan

The general e2e test suites will be acceptable.  We expect OVN to be the default for all hypershift based OpenShift clusters once OVN support is ready, so anywhere we are testing hypershift will be exercising this integration.

Additionally, we will need to write some disruption tests that ensure ovsdb raft plays well with `StatefulSet` rotations.

We will work closely with the Performance & Scale test team to test out the performance of this architecture.

### Graduation Criteria

Once this functionality is complete, we expect to make OVN the default for hypershift managed clusters. At that point, the graduation of this functionality will be tied to the graduation of hypershift more broadly.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature
n/a
### Upgrade / Downgrade Strategy

This architecture will only apply to new clusters deployed via hypershift. Thus, the generalized Hypershift upgrade model must take OVN in to account.

OVN has somewhat complicated upgrade ordering. On upgrades, nodes must be upgraded before the control plane. For downgrades, the control plane must be rolled out first. While this logic is unchanged in Hypershift, splitting the components across clusters makes this more complicated w.r.t. potential failure cases.
Where there is an ordering defined, we also need to wait for any Deployments / Daemonsets / StatefuSets to roll out -- it's not sufficient just to wait for the object to be updated. Accordingly, the code that enforces the ordering needs to be multi-cluster aware.


### Version Skew Strategy

There should not be any further version skew questions introduced by the hypershift model. Neither OVN-K nor CNO have a tight coupling with the API server or kubernetes control plane. Nevertheless, we (as well as the wider Hypershift team) need to consider the implication of very old control plane components running on an up-to-date control plane. At present, the CNO creates several `v1beta1` objects.

In practice, client-go + kube-api support wide version skews, even if this is not officially supported. This is exploited in the EUS-EUS upgrade strategy.

There is a coupling between OVN-Kubernetes, the machine-configuration-operator, and rollouts of files on the individual nodes ([specifically](https://github.com/openshift/machine-config-operator/blob/master/templates/common/_base/files/configure-ovs-network.yaml)). If end-users delay rollouts to their nodes, then we may trigger bugs not detected by CI or QE (since we do not test delayed rollouts).

### Operational Aspects of API Extensions

#### Failure Modes

Probable failure situations include
- OVN raft is degraded / broken
  - Close to losing quorum for too long
  - Quorum is failed
- OVN DBs are too slow / overloaded
  - Too much CPU / Memory
  - Too slow a transaction rate
  - Disk is almost full


#### Support Procedures

The SD / SRE team will be responsible for operating the control plane. As such, we will need to develop runbooks and alerting.

The most likely problem is slow propagation of flows to the individual nodes. We have alerting to detect
this, but we should define the procedures and alerts carefully.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

It's not entirely clear, ultimately, if OVN sbdb is the right integration point. However, this is ultimately a question as to the design of OVN itself, rather than Hypershift per se. The stretched nature of Hypershift may expose limitations in the ovsdb protocol.

The OVN control plane is "heavy-weight" - that is to say, it has high resource draw and is a complicated piece of software. Taking responsibility for its availability is certainly an operational burden. For end-users, however, it is a distinct advantage.

## Alternatives

### Run two copies of the CNO

An alternative is to run two copies of the CNO for each hosted cluster. One would be in the hosted control plane and would only manage the parts of OVN that run there. The CNO running in the hosted cluster on worker nodes would manage only the parts of OVN running on the worker nodes.

There are disadvantages to this approach and a lack of clear benefits compared to the direction proposed. Separating management of OVN makes upgrade coordination more difficult. We would also like to keep these management workloads off of the worker nodes wherever possible.

### Integrate more directly with hypershift

Instead of reusing the CNO, we briefly considered whether code should be shared with hypershift and have the existing hypershift components manage OVN. This seemed unnecessarily complex and without clear benefit.

Hypershift does include its own implementation of managing some components, but it's focused on things that work in a drastically different way than they do on a standalone cluster. The changes are not as drastic. There's a different in the locations for where pieces run, but the architecture remains fundamentally the same.