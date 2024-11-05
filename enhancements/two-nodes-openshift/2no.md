---
title: 2no
authors:
  - "@mshitrit"
  - "@jaypoulz"
reviewers:
  - "@rwsu"
  - "@fabbione"
  - "@carbonin"
  - "@thomasjungblut"
  - "@brandisher"
  - "@DanielFroehlich"
  - "@jerpeter1"
  - "@slintes"
  - "@beekhof"
  - "@eranco74"
  - "@yuqi-zhang"
  - "@gamado"
  - "@frajamomo"
  - "@clobrano"
  - "@cybertron"
approvers:
  - "@thomasjungblut"
  - "@jerpeter1"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@deads2k"
creation-date: 2024-09-05
last-updated: 2024-09-22
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1514
---

# Two Nodes Openshift (2NO) - Control Plane Availability

## Terms

RHEL-HA - a general-purpose clustering stack shipped by Red Hat (and others) primarily consisting of Corosync and Pacemaker.  Known to be in use by airports, financial exchanges, and defense organizations, as well as used on trains, satellites, and expeditions to Mars.

Corosync - a Red Hat led [open-source project](https://corosync.github.io/corosync/) that provides a consistent view of cluster membership, reliable ordered messaging, and flexible quorum capabilities.  

Pacemaker - a Red Hat led [open-source project](https://clusterlabs.org/pacemaker/doc/) that works in conjunction with Corosync to provide general-purpose fault tolerance and automatic failover for critical services and applications.

Fencing - the process of “somehow” isolating or powering off malfunctioning or unresponsive nodes to prevent them from causing further harm, such as data corruption or the creation of divergent datasets.  

Quorum - having the minimum number of members required for decision-making. The most common threshold is 1 plus half the total number of members, though more complicated algorithms predicated on fencing are also possible.
 * C-quorum: quorum as determined by Corosync members and algorithms
 * E-quorum: quorum as determined by etcd members and algorithms

Split-brain - a scenario where a set of peers are separated into groups smaller than the quorum threshold AND peers decide to host services already running in other groups.  Typically results in data loss or corruption.

MCO - Machine Config Operator. This operator manages updates to the node's systemd, cri-o/kubelet, kernel, NetworkManager, etc., and can write custom files to it, configurable by MachineConfig custom resources.

ABI - Agent-Based Installer.

## Summary

Leverage traditional high-availability concepts and technologies to provide a container management solution suitable that has a minimal footprint but remains resilient to single node-level failures suitable for customers with numerous geographically dispersed locations.

## Motivation

Customers with hundreds, or even tens of thousands, of geographically dispersed locations are asking for a container management solution that retains some level of resilience to node-level failures but does not come with a traditional three-node footprint and/or price tag.

The need for some level of fault tolerance prevents the applicability of Single Node OpenShift (SNO), and a converged 3-node cluster is cost prohibitive at the scale of retail and telcos - even when the third node is a "cheap" one that doesn't run workloads.

The benefits of the cloud-native approach to developing and deploying applications are increasingly being adopted in edge computing.
This requires our solution to provide a management experience consistent with "normal" OpenShift deployments and be compatible with the full ecosystem of Red Hat and partner workloads designed for OpenShift.

### User Stories

* As a large enterprise with multiple remote sites, I want a cost-effective OpenShift cluster solution so that I can manage containers without the overhead of a third node.
* As a support engineer, I want a safe and automated method for handling the failure of a single node so that the downtime of the control-plane is minimized.
* As an enterprise running workloads on a minimal OpenShift footprint, I want to minimize time-to-recovery and data loss for my workloads when a node fails.

### Goals

* Provide a two-node control-plane for physical hardware that is resilient to a node-level failure for either node
* Provide a transparent installation experience that starts with exactly 2 blank physical nodes, and ends with a fault-tolerant two-node cluster
* Prevent both data corruption and divergent datasets in etcd
* Minimize recovery-caused unavailability. Eg. by avoiding fencing loops, wherein each node powers cycles its peer after booting, reducing the cluster's availability.
* Recover the API server in less than 120s, as measured by the surviving node's detection of a failure
* Minimize any differences to existing OpenShift topologies
* Avoid any decisions that would prevent future implementation and support for upgrade/downgrade paths between two-node and traditional architectures 
* Provide an OpenShift cluster experience that is similar to that of a 3-node hyperconverged cluster but with 2 nodes

### Non-Goals

* Workload resilience - see related [Pre-DRAFT enhancement](https://docs.google.com/document/d/1TDU_4I4LP6Z9_HugeC-kaQ297YvqVJQhBs06lRIC9m8/edit)
* Resilient storage - see future enhancement
* Support for platforms other than bare metal including automated CI testing
* Support for other topologies (eg. hypershift)
* Adding worker nodes
* Creation of RHEL-HA events and metrics for consumption by the OpenShift monitoring stack (Deferred to post-MVP)
* Supporting upgrade/downgrade paths between two-node and other architectures (for initial release)

## Proposal

Use the RHEL-HA stack (Corosync, and Pacemaker), which has been used to deliver supported 2-node cluster experiences for multiple decades, to manage cri-o, kubelet, and the etcd daemon.
etcd will run as as a voting member on both nodes.
We will take advantage of RHEL-HA's native support for systemd and re-use the standard cri-o and kubelet units, as well as create a new Open Cluster Framework (OCF) script for etcd.
The existing startup order of cri-o, then kubelet, then etcd will be preserved.
The `etcdctl`, `etcd-metrics`, and `etcd-readyz` containers will remain part of the static pod, the contents of which remain under the exclusive control of the Cluster Etcd Operator (CEO).

Use RedFish compatible Baseboard Management Controllers (BMCs) as our primary mechanism to power off (fence) an unreachable peer and ensure that it can do no harm while the remaining node continues.

Upon a peer failure, the RHEL-HA components on the survivor will fence the peer and use the OCF script to restart etcd as a new cluster-of-one.

Upon a network failure, the RHEL-HA components ensure that exactly one node will survive, fence its peer, and use the OCF script to restart etcd as a new cluster-of-one.

In both cases, the control-plane's dependence on etcd will cause it to respond with errors until etcd has been restarted.

Upon rebooting, the RHEL-HA components ensure that a node remains inert (not running cri-o, kubelet, or etcd) until it sees its peer.
If the failed peer is likely to remain offline for an extended period, admin confirmation is required on the remaining node to allow it to start OpenShift.
The functionality exists within RHEL-HA, but a wrapper will be provided to take care of the details.

When starting etcd, the OCF script will use etcd's cluster ID and version counter to determine whether the existing data directory can be reused, or must be erased before joining an active peer.

### Workflow Description

#### Cluster Creation

User creation of a two-node control-plane will be possible via the Assisted Installer.

The procedure follows the standard flow except for the configuration of 2 nodes instead of 3, and the collection of RedFish details (including passwords!) for each node which are needed for the RHEL-HA configuration.
If available via the SaaS offering (not confirmed, ZTP may be the target), the offering will need to ensure passwords are appropriately handled. 

Everything else about cluster creation will be an opaque implementation detail not exposed to the user. 

#### Day 2 Procedures

As per a standard 3-node control-plane, OpenShift upgrades and `MachineConfig` changes can not be applied when the cluster is in a degraded state.
Such operations will only proceed when both peers are online and healthy.

The experience of managing a 2-node control-plane should be largely indistinguishable from that of a 3-node one.
The primary exception is (re)booting one of the peers while the other is offline and expected to remain so.

As in a 3-node control-plane cluster, starting only one node is not expected to result in a functioning cluster.
Should the admin wish for the control-plane to start, the admin will need to execute a supplied confirmation command on the active cluster node. 
This command will grant quorum to the RHEL-HA components, authorizing it to fence its peer and start etcd as a cluster-of-one in read/write mode.
Confirmation can be given at any point and optionally make use of SSH to facilitate initiation by an external script.

### API Extensions

There are two related but ultimately orthogonal capabilities that may require API extensions.

1. Identify the cluster as having a unique topology
2. Tell CEO when it is safe for it to disable certain membership-related functionalities

#### Unique Topology

A mechanism is needed for components of the cluster to understand that this is a 2-node control-plane topology which may require different handling.
We will define a new value for the `TopologyMode` enum: `DualReplica`.
The enum is used for the `controlPlaneTopology` and `infrastructureTopology` fields, and the currently supported values are `HighlyAvailable`, `SingleReplica`, and `External`.

However `TopologyMode` is not available at the point the Agent Based Installer (ABI) performs validation.
We will therefore additionally define a new feature gate `DualReplicaTopology` that can be enabled in `install-config.yaml`, and which ABI can use to validate the proposed cluster - such as the proposed node count.

#### CEO Trigger

Initially, the creation of an etcd cluster will be driven in the same way as other platforms.
Once the cluster has two members, the etcd daemon will be removed from the static pod definition and recreated as a resource controlled by RHEL-HA.
At this point, the Cluster Etcd Operator (CEO) will be made aware of this change so that some membership management functionality that is now handled by RHEL-HA can be disabled.
This will be achieved by having the same entity that drives the configuration of RHEL-HA use the OpenShift API to update a field in the CEO's `ConfigMap` - which can only succeed if the control-plane is healthy.

To enable this flow, we propose the addition of a `managedEtcdKind` field which defaults to `Cluster` but will be set to `External` during installation, and will only be respected if the `Infrastructure` CR's `TopologyMode` is `DualReplicaTopologyMode`.
This will allow the use of a credential scoped to `ConfigMap`s in the `openshift-etcd-operator` namespace, to make the change.

### Topology Considerations

2NO represents a new topology and is not appropriate for use with HyperShift, SNO, or MicroShift

#### Standalone Clusters

Is the change relevant for standalone clusters?
TODO: Exactly what is the definition of a standalone cluster?  Disconnected?  Physical hardware?

### Implementation Details/Notes/Constraints

While the target installation requires exactly 2 nodes, this will be achieved by building support in the core installer for a "bootstrap plus 2 nodes" flow and then using the Assisted Installer's ability to bootstrap-in-place to remove the requirement for a bootstrap node.

The delivery of RHEL-HA components will be opaque to the user and be delivered as an [MCO Extension](../rhcos/extensions.md) in the 4.18 and 4.19 timeframes.
A switch to [MCO Layering](../ocp-coreos-layering/ocp-coreos-layering.md ) will be investigated once it is GA in a shipping version of OpenShift.

Configuration of the RHEL-HA components will be via one or more `MachineConfig`s and will require RedFish details to have been collected by the installer.
Sensible defaults will be chosen where possible, and user customization only where necessary.

The entity (likely a one-shot systemd job as part of a `MachineConfig`) that configures RHEL-HA will also configure a fencing priority.
This is usually done based on the sort order of a piece of shared info (such as IP or node name).
The priority takes the form of a delay, usually in the order of 10s of seconds, and is used to prevent parallel fencing operations during a primary-network outage where each side powers off the other - resulting in a total cluster outage.

RHEL-HA has no real understanding of the resources (IP addresses, file systems, databases, even virtual machines) it manages.
It relies on resource agents to understand how to check the state of a resource, as well as start and stop them to achieve the desired target state.
How a given agent uses these actions, and associated states, to model the resource is opaque to the cluster and depends on the needs of the underlying resource.

Resource agents must conform to one of a variety of standards, including systemd, SYS-V, and OCF.
The latter is the most powerful, adding the concept of promotion, and demotion.
More information on creating OCF agents can be found in the upstream [developer guide](https://github.com/ClusterLabs/resource-agents/blob/main/doc/dev-guides/ra-dev-guide.asc).

Tools for extracting support information (must-gather tarballs) will be updated to gather relevant logs for triaging issues.

#### Failure Scenario Timelines:

1. Cold Boot
   1. One node (Node1) boots
   2. Node1 does have “corosync quorum” (C-quorum)  (requires forming a membership with its peer)
   3. Node1 does not start etcd or kubelet, remains inert waiting for Node2
   4. Peer (Node2) boots
   5. Corosync membership containing both nodes forms
   6. Pacemaker starts etcd on both nodes
      * Detail, this could be a “soft”-start which allows us to determine which node has the most recent dataset.
   7. Pacemaker “promotes” etcd on whichever node has the most recent dataset
   8. Pacemaker “promotes” etcd on the peer once it has caught up
   9. Pacemaker starts kubelet on both nodes
   10. Fully functional cluster
2. Network Failure
   1. Corosync on both nodes detects separation
   2. Etcd loses internal quorum (E-quorum) and goes read-only
   3. Both sides retain C-quorum and initiate fencing of the other side.
      RHEL-HA's fencing priority avoids parallel fencing operations and thus the total shutdown of the system.
   4. One side wins, pre-configured as Node1
   5. Pacemaker on Node1 forces E-quorum (etcd promotion event)
   6. Cluster continues with no redundancy
   7. … time passes …
   8. Node2 boots - persistent network failure
      * Node2 does not have C-quorum (requires forming a membership with its peer)
      * Node2 does not start etcd or kubelet, remains inert waiting for Node1
   9. Network is repaired
   10. Corosync membership containing both nodes forms
   11. Pacemaker “starts” etcd on Node2 as a follower of Node1
   12. Pacemaker “promotes” etcd on Node2 as a full replica of Node1
   13. Pacemaker starts kubelet
   14. Cluster continues with 1+1 redundancy
3. Node Failure
   1. Corosync on the survivor (Node1)
   2. Etcd loses internal quorum (E-quorum) and goes read-only
   3. Node1 retains “corosync quorum” (C-quorum) and initiates fencing of Node2
   4. Pacemaker on Node1 forces E-quorum (etcd promotion event)
   5. Cluster continues with no redundancy
   6. … time passes …
   7. Node2 has a persistent failure that prevents communication with Node1
      * Node2 does not have C-quorum (requires forming a membership with its peer)
      * Node2 does not start etcd or kubelet, remains inert waiting for Node1
   8. Persistent failure on Node2 is repaired
   9. Corosync membership containing both nodes forms
   10. Pacemaker “starts” etcd on Node2 as a follower of Node1
   11. Pacemaker “promotes” etcd on Node2 as a full replica of Node1
   12. Pacemaker starts kubelet
   13. Cluster continues with 1+1 redundancy
4. Two Failures
   1. Node2 failure (1st failure)
   2. Corosync on the survivor (Node1)
   3. Etcd loses internal quorum (E-quorum) and goes read-only
   4. Node1 retains “corosync quorum” (C-quorum) and initiates fencing of Node2
   5. Pacemaker on Node1 forces E-quorum (etcd promotion event)
   6. Cluster continues with no redundancy
   7. Node1 experience a power failure (2nd Failure)
   8. … time passes …
   9. Node1 Power restored
   10. Node1 boots but can not gain quorum before Node2 joins the cluster due to a risk of fencing loop
       * Mitigation (Phase 1): manual intervention (possibly a script)  in case the admin can guarantee Node2 is down, which will grant Node1 quorum and restore cluster limited (none HA) functionality.
       * Mitigation (Phase 2): limited automatic intervention for some use cases: for example, Node1 will gain quorum only if Node2 can be verified to be down by successfully querying its BMC status.
5. Kubelet Failure
   1. Pacemaker’s monitoring detects the failure
   2. Pacemaker restarts kubelet
   3. Stop failure is optionally escalated to a node failure (fencing)
   4. Start failure defaults to leaving the service offline
6. Etcd Failure
   1. Pacemaker’s monitoring detects the failure
   2. Pacemaker either demotes etcd so it can resync, or restarts and promotes etcd
   3. Stop failure is optionally escalated to a node failure (fencing)
   4. Start failure defaults to leaving the service offline

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282

How does it affect any of the components running in the
management cluster? How does it affect any components running split
between the management cluster and guest cluster?

#### Single-node Deployments or MicroShift

How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

### Risks and Mitigations

1. Risk: If etcd were to be made active on both peers during a network split, divergent datasets would be created
   1. Mitigation: RHEL-HA requires fencing of a presumed dead peer before restarting etcd as a cluster-of-one
   1. Mitigation: Peers remain inert (unable to fence peers, or start cri-o, kubelet, or etcd) after rebooting until they can contact their peer

1. Risk: Multiple entities (RHEL-HA, CEO) attempting to manage etcd membership would cause an internal split-brain
   1. Mitigation: The CEO will run in a mode that does manage not etcd membership

1. Risk: Rebooting the surviving peer would require human intervention before the cluster starts, increasing downtime and creating an admin burden at remote sites
   1. Mitigation: Lifecycle events, such as upgrades and applying new `MachineConfig`s, are not permitted in a single-node degraded state
   1. Mitigation: Usage of the MCO Admin Defined Node Disruption [feature](https://github.com/openshift/enhancements/pull/1525) will further reduce the need for reboots.
   1. Mitigation: The node will be reachable via SSH and the confirmation can be scripted
   1. Mitigation: It may be possible to identify scenarios where, for a known hardware topology, it is safe to allow the node to proceed automatically.

1. Risk: “Something changed, let's reboot” is somewhat baked into OCP’s DNA and has the potential to be problematic when nodes are actively watching for their peer to disappear, and have an obligation to promptly act on that disappearance by power cycling them.
   1. Mitigation: Identify causes of reboots, and either avoid them or ensure they are not treated as failures.
   This may require an additional enhancement.

1. Risk: We may not succeed in identifying all the reasons a node will reboot
   1. Mitigation: ... testing? ...

1. Risk: This new platform will have a unique installation flow
   1. Mitigation: ... CI ...




### Drawbacks

The two-node architecture represents yet another distinct install type for users to choose from.

The existence of 1, 2, and 3+ node control-plane sizes will likely generate customer demand to move between them as their needs change.
Satisfying this demand would come with significant technical and support overhead.

## Open Questions [optional]

1. Are there any normal lifecycle events that would be interpreted by a peer as a failure, and where the resulting "recovery" would create unnecessary downtime?
   How can these be avoided?
2. Are there consequences of changing the parentage of processes running cri-o, kubelet, and etcd? (E.g. user process limits)
3. In the test plan, which subset of layered products needs to be evaluated for the initial release (if any)?
4. How are the BMC credentials getting from the install-config and onto the nodes?
5. How does the cluster know it has achieved a ready state?
   From the cluster's perspective, we know that CEO needs to have `managedEtcd: External` set, and all of the operators need to be available. However, there is a time delay between when that configuration is set by CEO and when etcd comes back up as health after it's restarted under the RHEL-HA components. Is a fixed wait enough to determine that we have successfully transitioned to the new topology? If not, how do we detect this?
6. Are there any scenarios that would require signaling from the cluster to the RHEL-HA components to modify or change their behavior?
7. Are there incompatibilities between the existing design and the function of the load balancer deployed through the BareMetalPlatform spec?
8. Which installers will be supported for the initial release?
    The current discussions around the installer point us towards ABI for the initial release. There also seems to be interest in making this available for ZTP for managed clusters.
9. Which platform specs will be available for this topology?
    As discussed, we are currently targeting the BareMetalPlatform spec, but the load-balancing component needs to be evaluated for compatibility.
10. What happens if something fails during the initial setup of the RHEL-HA stack? How will this be communicated back to the user?
    For example, what happens if the setup job fails and etcd is left running? From the perspective of the user and the cluster, this would be identical to etcd being stopped and restarted under the control of the RHEL-HA components. Nothing about the cluster knows about the external entity that owns etcd.


## Test Plan

**Note:** *Section not required until targeted at a release.*

### CI
The initial release of 2NO should aim to build a regression baseline.

| Type  | Name                          | Description                                                                 |
| ----- | ----------------------------- | --------------------------------------------------------------------------- |
| Job   | End-to-End tests (e2e)        | The standard test suite (openshift/conformance/parallel) for establishing a regression baseline between payloads. |
| Job   | Upgrade between z-streams     | The standard test suite for evaluating upgrade behavior between payloads.   |
| Job   | Upgrade between y-streams [^1] | The standard test suite for evaluating upgrade behavior between payloads.  |
| Suite | 2NO Recovery                  | This is a new suite consisting of the tests listed below.                   |
| Test  | Node failure [^2]              | A new 2NO test to detect if the cluster recovers if a node crashes.        |
| Test  | Network failure [^2]           | A new 2NO test to detect if the cluster recovers if the network is disrupted such that a node is unavailable. |
| Test  | Kubelet failure [^2]           | A new 2NO test to detect if the cluster recovers if kubelet fails.         |
| Test  | Etcd failure [^2]              | A new 2NO test to detect if the cluster recovers if etcd fails.            |

[^1]: This will be added after the initial release when more than one minor version of OpenShift is compatible with the
topology.
[^2]: These tests will be designed to make a component on the *other* node fail. This should prevent the test pod from
being restarted mid-test.

### QE
This section outlines test scenarios for 2NO.

| Scenario                      | Description                                                                         |
| ----------------------------- | ----------------------------------------------------------------------------------- |
| Payload install               | A basic evaluation that the cluster installs on supported hardware. Should be run for each supported installation method. |
| Payload upgrade               | A basic evaluation that the cluster can upgrade between releases.                   |
| Performance                   | Performance metrics are gathered and compared to SNO and Compact HA                 |
| Scalability                   | Scalability metrics are gathered and compared to SNO and Compact HA                 |
| Cold Boot                     | Verify that clusters can survive a cold boot event.                                 |
| Both nodes crash              | Verify that clusters can survive an event where both nodes become unavailable.      |

As noted above, there is an open question about how layered products should be treated in the test plan.
Additionally, it would be good to have workload-specific testing once those are defined by the workload proposal.

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

See template for guidelines/instructions.

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

In-place upgrades and downgrades will not be supported for this first iteration, and will be addressed as a separate feature in another enhancement. Upgrades will initially only be achieved by redeploying the machine and its workload.

## Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control-plane and
  the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI,
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

See template for guidelines/instructions.

- For conversion/admission webhooks and aggregated API servers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  N/A

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  [TODO: Expand] Toggling CEO control values with result in etcd being briefly offline.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.

- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance, and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

See template for guidelines/instructions.

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)
- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)
  - What consequences does it have on the cluster health?
  - What consequences does it have on existing, running workloads?
  - What consequences does it have for newly created workloads?
- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

## Alternatives

* MicroShift was considered as an alternative but it was ruled out because it does not support multi-node and has a very different experience than OpenShift which does not match the 2NO initiative which  is on getting the OpenShift experience on two nodes


* 2 SNO + KCP
[KCP](https://github.com/kcp-dev/kcp/) allows you to manage multiple clusters from a single control-plane, reducing the complexity of managing each cluster independently.
With kcp, you can manage the two single-node clusters, each single-node OpenShift cluster can continue to operate independently even if the central kcp management plane becomes unavailable.
The main advantage of this approach is that it doesn’t require inventing a new Openshift flavor and we don’t need to create a new installation flow to accommodate it.
Disadvantages:
* Production readiness
* KCP itself could become a single point of failure (need to configure pacemaker to manage KCP)
* KCP adds additional complexity to the architecture


## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, GitHub details, and/or testing infrastructure.
