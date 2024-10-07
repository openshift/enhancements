---
title: 2no
authors:
  - "@mshitrit"
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
  - "@razo7"
  - "@frajamomo"
  - "@clobrano"

approvers:
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
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@jerpeter1"
creation-date: 2024-09-05
last-updated: 2024-09-22
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1514
---

# Two Nodes Openshift (2NO) - Control Plane Availability

## Terms

RHEL-HA - a general purpose clustering stack shipped by Red Hat (and others) primarily consisting of Corosync and Pacemaker.  Known to be in use by airports, financial exchanges, and defense organizations, as well as used on trains, satellites, and expeditions to Mars.

Corosync - a Red Hat led [open-source project](https://corosync.github.io/corosync/) that provides a consistent view of cluster membership, reliable ordered messaging, and flexible quorum capabilities.  

Pacemaker - a Red Hat led [open-source project](https://clusterlabs.org/pacemaker/doc/) that works in conjunction with Corosync to provide general purpose fault tolerance and automatic failover for critical services and applications.

Resource Agent - A resource agent is an executable that manages a cluster resource. No formal definition of a cluster resource exists, other than "anything a cluster manages is a resource." Cluster resources can be as diverse as IP addresses, file systems, database services, and entire virtual machines - to name just a few examples.
<br>[more context here](https://github.com/ClusterLabs/resource-agents/blob/main/doc/dev-guides/ra-dev-guide.asc)

Fencing - the process of “somehow” isolating or powering off malfunctioning or unresponsive nodes to prevent them from causing further harm or interference with the rest of the cluster.

Fence Agent - Fence agents were developed as device "drivers" which are able to prevent computers from destroying data on shared storage. Their aim is to isolate a corrupted computer, using one of three methods:
* Power - A computer that is switched off cannot corrupt data, but it is important to not do a "soft-reboot" as we won't know if this is possible. This also works for virtual machines when the fence device is a hypervisor.
* Network - Switches can prevent routing to a given computer, so even if a computer is powered on it won't be able to harm the data.
* Configuration - Fibre-channel switches or SCSI devices allow us to limit who can write to managed disks.
<br>[more context here](https://github.com/ClusterLabs/fence-agents/)

Quorum - having the minimum number of members required for decision-making. The most common threshold is 1 plus half the total number of members, though more complicated algorithms predicated on fencing are also possible.
C-quorum: quorum as determined by Corosync members and algorithms
E-quorum: quorum as determined by etcd members and algorithms

Split-brain - a scenario where a set of peers are separated into groups smaller than the quorum threshold AND peers decide to host services already running by other groups.  Typically results in data loss or corruption unless state is stored outside of the cluster.   

MCO - Machine Config Operator. This operator manages updates to systemd, cri-o/kubelet, kernel, NetworkManager, etc. It also offers a new MachineConfig CRD that can write configuration files onto the host.

ABI - Agent-Based Installer.

## Summary

Leverage traditional high-availability concepts and technologies to provide a container management solution suitable for customers with numerous geographically dispersed locations that has a minimal footprint but remains resilient to single node-level failures.

## Motivation

Customers with hundreds, or even tens-of-thousands, of geographically dispersed locations are asking for a container management solution that retains some level of resilience to node level failures, but does not come with a traditional three-node footprint and/or price tag.

The need for some level of fault tolerance prevents the applicability of Single Node OpenShift (SNO), and a converged 3-node cluster is cost prohibitive at the scale of retail and telcos - even when the third node is a "cheap" one that doesn't run workloads.

The benefits of the cloud-native approach to developing and deploying applications are increasingly being adopted in edge computing.
This requires our solution provide a management experience consistent with "normal" OpenShift deployments, and be compatible with the full ecosystem of Red Hat and partner workloads designed for OpenShift.

### User Stories

* As a large enterprise with multiple remote sites, I want a cost-effective OpenShift cluster solution so that I can manage containers without the overhead of a third node.
* As a support engineer, I want a safe and automated method for handling the failure of a single node so that the downtime of workloads is minimized.

### Goals

* Provide a two-node control plane for physical hardware that is resilient to a node-level failure for either node
* Provide a transparent installation experience that starts with exactly 2 blank physical nodes, and ends with a fault-tolerant two node cluster
* Prevent both data corruption and divergent datasets in etcd
* Maintain the existing level of availability. Eg. by avoiding fencing loops, wherein each node powers cycles it's peer after booting, reducing the cluster's availability.
* Recover the API server in less than 120s, as measured from the surviving node's detection of a failure
* Minimize any differences to the primary OpenShift platforms
* Avoid any decisions that would prevent upgrade/downgrade paths between two-node and traditional architectures
* Provide an OpenShift cluster experience that is identical to that of a 3-node hyperconverged cluster, but with 2 nodes

### Non-Goals

* Workload resilience - see related enhancement [link] 
* Resilient storage - see follow-up enhancement
* Support for platforms other than bare metal including automated ci testing
* Support for other topologies (eg. hypershift)
* Adding worker nodes

## Proposal

Use the RHEL-HA stack (Corosync, and Pacemaker), which has been used to delivered supported 2-node cluster experiences for multiple decades, to manage cri-o, kubelet, and the etcd daemon.
We will take advantage of RHEL-HA's native support for systemd and re-use the standard cri-o and kublet units, as well as create a new Open Cluster Framework (OCF) script for etcd.

Use RedFish compatible Baseboard Management Controllers (BMCs) as our primary mechanism to power off (fence) unreachable peers and ensure that they can do no harm while the remaining node continues.

The delivery of RHEL-HA components will either be:

* as an MCO Layer (targeting GA in 4.19),
* as an extension (supported today), or
* included, but inactive, in the base image

Configuration of the RHEL-HA components will be via one or more MachineConfigs, and will require RedFish details from the installer.

Upon a peer failure, the RHEL-HA components on the surivor will fence the peer and use the OCF script to restart etcd as a new cluster of one.

Upon a network failure, the RHEL-HA components ensure that exactly one node will survive, fence it's peer, and use the OCF script to restart etcd as a new cluster of one.

Upon rebooting, the RHEL-HA components ensure that a node remains inert (not running cri-o, kubelet, or etcd) until it sees it's peer.
If the peer is likely to remain offline for an extended period of time, admin confirmation is required to allow the node to start OpenShift.

When starting etcd, the OCF script will use the cluster ID and version counter to determine whether the existing data directory can be reused, or must be erased before joining an active peer.

OpenShift upgrades are not supported in a degraded state, and will only proceed when both peers are online.

MachineConfig updates are not applied in a degraded state, and will only proceed when both peers are online.


### Workflow Description

#### Cluster Creator Role:
* The Cluster Creator will automatically install the 2NO (by using an installer), installation process will include the following steps:  
  * Deploys a two-node OpenShift cluster
  * Configures cluster membership and quorum using Corosync.
  * Sets up Pacemaker for resource management and fencing.

#### Application Administrator Role:
* Receives cluster credentials.
* Deploys applications within the two-node cluster environment.


### API Extensions

No new CRDs, or changes to existing CRDs, are expected at this time.

### Topology Considerations

2NO represents a new topology, and is not appropriate for use with HyperShift, SNO, or MicroShift

#### Standalone Clusters

Is the change relevant for standalone clusters?
TODO: Exactly what is the definition of a standalone cluster?  Disconnected?  Physical hardware?


### Implementation Details/Notes/Constraints

While the target installation requires exactly 2 nodes, this will be achieved by building support in the core installer for a "bootstrap plus 2 nodes" flow, and then using Assisted Installer's ability to bootstrap-in-place to remove the requirement for a bootstrap node.

Initially the creation of an etcd cluster will be driven in the same way as other platforms.
Once the cluster has two members, the etcd daemon will be removed from the static pod and become controlled by RHEL-HA.
At this point, the Cluster Etcd Operator (CEO) will be made aware of this change so that some membership management functionality that is now handled by RHEL-HA can be disabled.
The exact mechanism for this communication has yet to be determined.


#### Failure Scenario Timelines:

1. Cold Boot
   1. One node (Node1) boots
   2. Node1 does have “corosync quorum” (C-quorum)  (requires forming a membership with it’s peer)
   3. Node1 does not start etcd or kubelet, remains inert waiting for Node2
   4. Peer (Node2) boots
   5. Corosync membership containing both nodes forms
   6. Pacemaker “starts” etcd on both nodes
      * Detail, this could be a “soft”-start which allows us to determine which node has the most recent dataset.
   7. Pacemaker “promotes” etcd on whichever node has the most recent dataset
   8. Pacemaker “promotes” etcd on the peer once it has caught up
   9. Pacemaker starts kubelet on both nodes
   10. Fully functional cluster
2. Network Failure
   1. Corosync on both nodes detects separation
   2. Etcd loses internal quorum (E-quorum) and goes read-only
   3. Both sides retain C-quorum and initiate fencing of the other side. There is a different delay between the two nodes for executing the fencing operation to avoid both fencing operations to succeed in parallel and thus shutting down the system completely.
   4. One side wins, pre-configured as Node1
   5. Pacemaker on Node1 forces E-quorum (etcd promotion event)
   6. Cluster continues with no redundancy
   7. … time passes …
   8. Node2 boots - persistent network failure
      * Node2 does not have C-quorum (requires forming a membership with it’s peer)
      * Node2 does not start etcd or kubelet, remains inert waiting for Node1
   9. Network is repaired
   10. Corosync membership containing both nodes forms
   11. Pacemaker “starts” etcd on Node2 as a follower of Node1
   12. Pacemaker “promotes” etcd on Node2 as full replica of Node1
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
      * Node2 does not have C-quorum (requires forming a membership with it’s peer)
      * Node2 does not start etcd or kubelet, remains inert waiting for Node1
   8. Persistent failure on Node2 is repaired
   9. Corosync membership containing both nodes forms
   10. Pacemaker “starts” etcd on Node2 as a follower of Node1
   11. Pacemaker “promotes” etcd on Node2 as full replica of Node1
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
       * Mitigation (Phase 1): manual intervention (possibly a script)  in case admin can guarantee Node2 is down, which will grant Node1 quorum and restore cluster limited (none HA) functionality.
       * Mitigation (Phase 2): limited automatic intervention for some use cases: for example Node1 will gain quorum only if Node2 can be verified to be down by successfully querying its BMC status.
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


### Risks and Mitigations

Risk: If etcd were to be made active on both peers during a network split, divergent datasets would be created
Mitigation: RHEL-HA requires fencing of a presumed dead peer before restarting etcd as a cluster of one
Mitigation: Peers remain inert (unable to fence peers, or start cri-o, kubelet, or etcd) after rebooting until they can contact their peer

Risk: Multiple entities (RHEL-HA, CEO) attempting to manage etcd membership would cause an internal split-brain
Mitigation: The CEO will run in a mode that does manage not etcd membership

Risk: Rebooting the surviving peer would require human intervention before the cluster starts, increasing downtime and creating an admin burden at remote sites
Mitigation: Lifecycle events, such as upgrades and applying new MachineConfigs, are not permitted in a single-node degraded state
Mitigation: Usage of the MCO Admin Defined Node Disruption [feature](https://github.com/openshift/enhancements/pull/1525) will futher reduce the need for reboots.
Mitigation: The node will be reachable via SSH and the confirmation can be scripted
Mitigation: It may be possible to identify scenarios where, for a known hardware topology, it is safe to allow the node to proceed automatically.

Risk: We may not succeed in identifying all the reasons a node will reboot
Mitigation: ... testing? ...

Risk: This new platform will have a unique installation flow
Mitigation: ... CI ...




### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

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
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Alternatives

* MicroShift was considered as an alternative but it was ruled out because it does not support multi node has a very different experience then OpenShift which does not match the 2NO initiative which  is on getting the OpenShift experience on two nodes


* 2 SNO + KCP
[KCP](https://github.com/kcp-dev/kcp/) allows you to manage multiple clusters from a single control plane, reducing the complexity of managing each cluster independently.
With kcp, you can manage the two single-node clusters, each single-node OpenShift cluster can continue to operate independently even if the central kcp management plane becomes unavailable.
The main advantage of this approach is that it doesn’t require inventing a new Openshift flavor and we don’t need to create a new installation flow to accommodate it.
Disadvantages:
* Production readiness
* KCP itself could become a single point of failure (need to configure pacemaker to manage KCP)
* KCP adds an additional layer of complexity to the architecture


## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
