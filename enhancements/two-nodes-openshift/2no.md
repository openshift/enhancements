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

# Two Nodes Openshift (2NO)

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

ZTP - Zero-Touch Provisioning.


## Summary

The Two Nodes OpenShift (2NO) initiative aims to provide a container management solution with a minimal footprint suitable for customers with numerous geographically dispersed locations. 
Traditional three-node setups represent significant infrastructure costs, making them cost-prohibitive at retail and telco scale. This proposal outlines how we can implement a two-node OpenShift cluster while retaining the ability to survive a node failure.

## Motivation

Customers with tens-of-thousands of geographically dispersed locations seek a container management solution that retains some level of resilience but does not come with a traditional three-node footprint. Even "cheap" third nodes represent a significant cost at this scale.
The benefits of the cloud-native approach to developing and deploying applications are increasingly being adopted in edge computing. As the distance between a site and the central management hub grows, the number of servers at the site tends to shrink. The most distant sites often have physical space for only one or two servers.
We are seeing an emerging pattern where some infrastructure providers and application owners desire a consistent deployment approach for their workloads across these disparate environments. They also require that the edge sites operate independently from the central management hub. Users who have adopted Kubernetes at their central management sites wish to extend this independence to remote sites through the deployment of independent Kubernetes clusters.
For example, in the telecommunications industry, particularly within 5G Radio Access Networks (RAN), there is a growing trend toward cloud-native implementations of the 5G Distributed Unit (DU) component. This component, due to latency constraints, must be deployed close to the radio antenna, sometimes on a single server at remote locations like the base of a cell tower or in a datacenter-like environment serving multiple base stations.
A hypothetical DU might require 20 dedicated cores, 24 GiB of RAM consumed as huge pages, multiple SR-IOV NICs carrying several Gbps of traffic each, and specialized accelerator devices. The node hosting this workload must run a real-time kernel, be carefully tuned to meet low-latency requirements, and support features like Precision Timing Protocol (PTP). Crucially, the "cloud" hosting this workload must be autonomous, capable of continuing to operate with its existing configuration and running workloads even when centralized management functionality is unavailable.
Given these factors, a two-node deployment of OpenShift offers a consistent, reliable solution that meets the needs of customers across all their sites, from central management hubs to the most remote edge locations.


### User Stories

* As a large enterprise with multiple remote sites, I want a cost-effective OpenShift cluster solution so that I can manage containers without the overhead of a third node.
* As a support engineer, I want an automated method for handling the failure of a single node so that I can quickly restore service and maintain system integrity.
* As an infrastructure administrator, I want to ensure seamless failover for virtual machines (VMs) so that in the event of a node failure, the VMs are automatically migrated to a healthy node with minimal downtime and no data loss.
* As a network operator, I want my Cloud-Native Network Functions (CNFs) to be orchestrated consistently using OpenShift, regardless of whether they are in datacenters, or at the far edge where physical space is limited.

### Goals

* Implement a highly available two-node OpenShift cluster.
* Ensure cluster stability and operational efficiency.
* Provide clear methods for node failure management and recovery.
* Identify and integrate with a technology or partner that can provide storage in a two-node environment.

### Non-Goals

* Reliance on traditional third-node or SNO setups.
* Make sure we don't prevent upgrade/downgrade paths between two-node and traditional architectures
* Adding worker nodes
* Support for platforms other than bare metal including automated ci testing
* Support for other topologies (eg. hypershift)
* Failover time: if the leading node goes down, the remaining nodes takes over and gains operational state (writable) in less than 60s
* support full recovery of the workload when the node comes back online after restoration - total time under 15 minutes


## Proposal


To achieve a two-node OpenShift cluster, we are leveraging traditional high-availability concepts and technologies. The proposed solution includes:

1. Leverage of the Full RHEL-HA Stack:
   * Run the RHEL-HA stack “under to kubelet” (directly on the hardware, not as an OpenShift workload)
   * Corosync for super fast failure detection, membership calculations, which in turn will trigger Pacemaker to apply Fencing based on Corosync quorum/membership information.
   * Pacemaker for integrating membership and quorum information, driving fencing, and managing if/when kubelet and etcd can be started
   * Pacemaker models kubelet and cri-o as a clone (much like a ReplicaSet) and etcd as a “promotable clone” (think a construct designed for leader/follower style services).  Together with fencing and quorum, this ensures that an isolated node that reboots is inert and can do no harm.
   * Pacemaker is [configured](//TODO mshitrit add link) to manage etcd/cri-o/kubelet, it will start/stop/restart those services using a script or an executable.
   * Pacemaker does not understand what it is managing, and expects an executable or script that knows how to start/stop/monitor (and optionally promote/demote) the service.
     1. Likely we would need to create one for etcd
     2. For kubelet and cri-o we can likely use the existing systemd unit file
2. Failure Scenarios:
   * Implement detailed handling procedures for cold boots, network failures, node failures, kubelet failures, and etcd failures using the RHEL-HA stack.
    [see examples](#failure-handling)
3. Fencing Methods:
   * We plan to use Baseboard Management Controller (BMC) as our primary fencing method, the premise of using BMC for fencing is that a node that is powered off, or was previously powered off and configured to be inert until quorum forms, is not in a position to cause corruption or diverging datasets.  Sending power-off (or reboot) commands to the peer’s BMC achieves this goal.


### Workflow Description

#### Cluster Creator Role:
* The Cluster Creator will automatically install the 2NO (by using an installer), installation process will include the following steps:  
  * Deploys a two-node OpenShift cluster
  * Configures cluster membership and quorum using Corosync.
  * Sets up Pacemaker for resource management and fencing.

#### Application Administrator Role:
* Receives cluster credentials.
* Deploys applications within the two-node cluster environment.

#### Failure Handling:

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


### API Extensions

API Extensions are CRDs, admission and conversion webhooks, aggregated API servers,
and finalizers, i.e. those mechanisms that change the OCP API surface and behaviour.

- Name the API extensions this enhancement adds or modifies.
- Does this enhancement modify the behaviour of existing resources, especially those owned
  by other parties than the authoring team (including upstream resources), and, if yes, how?
  Please add those other parties as reviewers to the enhancement.

  Examples:
  - Adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
  - Restricts the label format for objects to X.
  - Defaults field Y on object kind Z.

Fill in the operational impact of these API Extensions in the "Operational Aspects
of API Extensions" section.

### Topology Considerations

2NO represents a new topology, and is not appropriate for use with HyperShift, SNO, or MicroShift

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282

How does it affect any of the components running in the
management cluster? How does it affect any components running split
between the management cluster and guest cluster?

#### Standalone Clusters

Is the change relevant for standalone clusters?

#### Single-node Deployments or MicroShift

How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

How does this proposal affect MicroShift? For example, if the proposal
adds configuration options through API resources, should any of those
behaviors also be exposed to MicroShift admins through the
configuration file for MicroShift?

### Implementation Details/Notes/Constraints

#### Installation flow
1. We’ll set up Pacemaker and Corosync on RHCOS using MCO layering.
   * [TBD extend more]
2. Install an “SNO like” first node using a second bootstrapped node.
   * This is somewhat similar what is done in SNO CI in AWS (up until the part the bootstrapped node is removed) and is possible because CEO can distinguish the bootstrapped node as a special use case, thus enabling its removal without breaking the etcd quorum for the remaining node.
   We should be safe after [MGMT-13586](https://issues.redhat.com/browse/MGMT-13586) which makes the installer wait for the bootstrap etcd member to be removed first before shutting it down.
3. After the bootstrapped node is removed add it to the cluster as a “regular” node.
4. Switch CEO to “2NO” mode (where it does not manage etcd) and remove the etcd static pods
   * [TBD localized/global switch]
   * This is done because we want to allow simpler maintenance and keeping  some of CEO functionality (defragmentation, cert rotation ect…)
5. Configure Pacemaker/Corosync to manage etcd/kubelet/cri-o
6. [TBD storage]

#### Fencing / quorum management
Fencing quorum is managed by corosync and etcd  will be managed by Pacemaker which will force the etcd quorum.

Here is a node failure example demonstrating that:
1. Corosync on the survivor (Node1)
2. Etcd loses internal quorum (E-quorum) and goes read-only
3. Node1 retains “corosync quorum” (C-quorum) and initiates fencing of Node2
4. Once fencing is successful Pacemaker will use a fence/resource agent (TBD) which will reschedule the workload from the fenced node
5. Pacemaker on Node1 forces E-quorum (etcd promotion event)
6. Cluster continues with no redundancy
7. … time passes …
8. Node2 has a persistent failure that prevents communication with Node1
   * Node2 does not have C-quorum (requires forming a membership with it’s peer)
   * Node2 does not start etcd or kubelet, remains inert waiting for Node1
9. Persistent failure on Node2 is repaired
10. Corosync membership containing both nodes forms
11. Pacemaker “starts” etcd on Node2 as a follower of Node1
12. Pacemaker “promotes” etcd on Node2 as full replica of Node1
13. Pacemaker starts kubelet
14. Cluster continues with 1+1 redundancy

[Here](#failure-handling) is a  more extensive list of failure scenarios.

#### CEO Enhancement
1. Requires a new infrastructure type in OpenShift APIs
2. Make sure that even though CEO will not manage etcd, it will still retain other relevant capabilities (defragmentation, certificate rotation, backup/restore etc...).
3. Some functionality to “know” when to switch to “2NO mode”

### Risks and Mitigations

#### Risks:

1. In the event of a node failure, Pacemaker on the survivor will fence the other node and cause etcd to recover quorum. However this will not automatically recover affected workloads. [mitigation](#scheduling-workload-on-fenced-nodes)
2. We plan to configure Pacemaker to manage etcd and give it quorum (for example after the remaining node fence its peer in a failed node use case)  [mitigation](#pacemaker-controlling-key-elements)
   1. How do we plan Pacemaker to give the etcd quorum and which consideration should be taken ?
   2. How does Pacemaker giving etcd quorum affect other etcd stakeholders (etcd pod, etcd operator, etc…)  ?
3. Having etcd/kubelet/cri-o managed by Pacemaker is a major change, it should be particularly considered in the installation process. Having a different process that manages those key services may cause timing issues, race conditions and potentially break some assumptions relevant to cluster installations. How does bootstrapping Pacemaker to manage etcd/kubelet/cri-o affects different installers processes (i.e assistant installer , agent base installer, etc) ? [mitigation](#unique-bootstrapping-affecting-installation-process)
   1. **CEO (Cluster Etcd Operator)/Pacemaker Conflict:**
      Since we plan to use Pacemaker to manage etcd, we need to make sure we prevent the current management done by the CEO.
   2. **Bootstrap Problem:** when only 2 nodes are used for the installation process  one of them serves as a bootstrap node so once this node isn’t part of the cluster etcd will lose quorum.
   3. **Setting 2NO resources:** how do we plan to get specific 2NO resources (pacemaker, corosync, etc…) on the node ?
4. Some Lifecycle events may reboot the node as part of the normal process (applying a disk image, updating ssh auth keys, configuration changes etc…). In a 2NO setup each node expects its peer to be up and will try to power fence it in case it isn’t  because of that, reboot events may trigger unnecessary fencing with unexpected consequences. [mitigation](#non-failure-node-reboots)


#### Mitigations:
 
##### Scheduling workload on fenced nodes
   1. **[Preferred Mitigation]** Pacemaker will utilize **resource/fence agents** to do the following:
      1. Before Pacemaker starts fencing of the faulty node it would place a “No Execute” taint on that node. This taint will prevent any new workload from running on the fenced node.
      2. After fencing is successful, Pacemaker will place an “Out Of Service” taint on the faulty node, which will trigger the removal of that workload and rescheduling on the it’s healthy peer node.
      3. Once the unhealthy node regains health and joins the cluster Pacemaker will remove both of these taints.
   
      <br>**Other alternatives**
   2. After Pacemaker has successfully fenced the faulty node it can mark the fenced node thus allowing a different operator to manage the rescheduling of the workload.
   3. Integrate NHC & a remediation agent ? (if so, NHC needs to be coordinated with Pacemaker in order to make sure we don’t needlessly fence the node multiple times )

##### Pacemaker controlling key elements
   1. Consult with relevant area experts (etcd, cri-o , kubelet etc…)
   2. Verify solution with extensive testing

##### Unique bootstrapping affecting installation process

   1. **CEO (Cluster Etcd Operator)/Pacemaker Conflict:**
   
      1. **[Preferred Mitigation]** Add a “disable” or a “2NO” feature to CEO
         1. Requires a new infrastructure type in OpenShift APIs
         2. 2NO installation needs to work with CEO up to the point where corosync wants to take over
         3. We need a signal to CEO when it should relinquish its control to corosync - new field in the cluster/etcd CRD?
         4. How can we replicate the functionality of CEO that is tied to static pods? e.g. Certificate rotation, backup/restore, apiserver<>etcd endpoint controller
         5. Do we want this as a localized switch (i.e for example as a flag in etcd CRD) or as a global option that might serve other 2NO stakeholders as well ?
      
         **Note**: CEO alternatively could also remove ONLY the etcd container from its static pod definition
        
          <br>**Other alternatives**    
      2. Scale down CEO Deployment replica after bootstrapping. The downside is that we need to figure out how to get etcd upgrades, as they will be blocked.
      3. Add a “disable CEO” feature to CVO (Downside is that other CEO functionalities will be needed to be managed)

   2. **Bootstrap Problem:** Potential approaches to solve this:
   
      1. **[Preferred Mitigation]** Install an “SNO like” first node using a second bootstrapped node. <br>This is somewhat similar what is done in SNO CI in AWS (up until the part the bootstrapped node is removed) and is possible because CEO can distinguish the bootstrapped node as a special use case, thus enabling its removal without breaking the etcd quorum for the remaining node.
    We should be safe after [MGMT-13586](https://issues.redhat.com/browse/MGMT-13586) which makes the installer wait for the bootstrap etcd member to be removed first before shutting it down.
        
         <br>**Other alternatives**
      2. As part of the installation process configure corosync/pacemaker to manage etcd so that we can make sure having the bootstrap node does not cause etcd to lose quorum (or least that etcd can still regain it with only one node)
      3. It’s also worth mentioning that we’ve discussed a more simple option of using 3 nodes and taking one down, however this option is rejected because we can’t assume that a customer that wants a 2NO would have a third available node.
      
   3. **Setting 2NO resources:** Potential approaches to solve this:
      
      1. **[Preferred Mitigation]** Using MCO (Machine Config Operator) to layer RHCOS with 2NO resources, however this is done out of the scope of the installer so we need to verify that there aren’t any issues with that.

         <br>**Other alternatives**
      2. It is also worth noting that we’ve considered modifying the RHCOS to contain 2NO resources (currently there is [another initiative](https://issues.redhat.com/browse/OCPSTRAT-1628) to do so) - At the moment this option is less preferable because it would couple 2NO to RHCOS frequent release cycle as well as add the 2NO resources in other OCP components which do not require it.
      3. RHEL extensions
##### Non failure node reboots
   1. Apply MCO Admin Defined Node Disruption [feature](https://github.com/openshift/enhancements/pull/1525) which allows os updates without node reboot.
   2. Potentially it’s a graceful reboot in which case Pacemaker will get a notification and can handle the reboot.
   3. Some delay mechanism ?
   4. Handle those specific use cases for a different behavior for a 2NO cluster ?
   5. Other alternatives ?

General mitigation which apply to most of the risks are
* Early feedback from relevant experts
* Thorough testing of failure scenarios.
* Clear documentation and support procedures.


#### Appendix - Disabling CEO:
Features that the CEO currently takes care of:
* Static pod management during bootstrap, installation and runtime
* etcd Member addition/removal on lifecycle events of the node/machine (“vertical scaling”)
* Defragmentation
* Certificate creation and rotation
* Active etcd endpoint export for apiserver (etcd-endpoints configmap in openshift-config namespace)
* Installation of the Backup/Restore scripts

Source as of 4.15: [CEO <> CEE](https://docs.google.com/presentation/d/1U_IyNGHCAZFAZXyzAs5XybR8qT91QaQ2wr3W9w9pSaw/edit#slide=id.g184d8fd7fc3_1_99)





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
