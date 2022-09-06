---
title: microshift-default-cni
authors:
  - majopela
  - zshi-redhat
reviewers:
  - "@copejon, MicroShift contributor"
  - "@fzdarsky, MicroShift architect"
  - "@ggiguash, MicroShift contributor"
  - "@sallyom, MicroShift contributor"
  - "@oglok, MicroShift contributor"
  - "@dcbw, OpenShift Networking Manager"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2022-07-18
last-updated: 2022-08-22
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-40
see-also:
  - https://github.com/openshift/microshift/blob/main/docs/design.md
  - https://github.com/openshift/enhancements/pull/1187
  - https://docs.google.com/document/d/1Adc2zgsmS83ooXXy-NR8-8HnrcIv4or1J4lSxWJB52A
  - https://docs.google.com/document/d/1zvPucfb9prDp1sv5pAa9IbiQCnm61BSNyyQUpWA56R0
---

# Default CNI 

## Summary

This enhancement proposes the adoption of a default MicroShift CNI.

MicroShift addresses customer use cases with low-resource,
field-deployed edge devices (SBCs, SoCs) requiring a minimal K8s
container orchestration layer, please see PR#1187 for more details.

We are proposing OVNKubernetes to align with all the other OpenShift form factors,
and provide the ability to use NetworkPolicies which some customers demand.

In this enhancement we describe the changes made to the OVNKubernetes configuration
as well as the work that needs to be done, and possible improvements.

## Motivation

MicroShift used flannel during the proof of concept phase, which later on
was simplified to bridge-cni, being the most minimalistic plugin which would
provide Pod to Pod communication in a single node environment and nothing else.

A very important characteristic necessary for any CNI plugin used in MicroShift
is a low-enough RAM, Disk (which translates to network bandwidth consumption on
updates), CPU footprint and boot times.

The target platforms for MicroShift are small SBCs or SoCs (Single Board Computers
or Systems on Chip) where most of the computing capability must remain available
for the final applications, otherwise MicroShift stops being useful in its target
environment.

An optimized OVNKubernetes is chosen for the reasons explained in the summary, and
we believe this optimization is enough for most customers.

### User Stories

An application developer builds applications where:
1. Pods need to talk to each other via TCP or UDP connectivity.
2. Pods need to talk to services on the LAN or Internet via TCP or UDP.
3. Some pods need be exposed as NodePort types of services on the LAN.
4. Some pods need to be exposed to other pods as ClusterIP services types.

An application developer builds applications where some pods must
have limited connectivity to other pods, or to the external network (i.e.
plugin services from vendors which must only contact specific internal APIs),
this is implemented by NetworkPolicies.

The edge device with MicroShift and the final application is deployed on an
IPv6, or DualStack IPV4/IPV6 network.

As a device owner I can protect the edge device with a host-level firewall
(firewalld, iptables, nftables), and that type of configuration is compatible
with the CNI.

The edge device should be capable of deploying on a network with dynamic address
provisioning like DHCP, SLAAC, or DHCPv6 and remain functional when the IP address
changes.

### Goals
* Choose a default CNI that will meet most MicroShift users' needs while
  being supportable by the OpenShift networking team.

* Implement a default CNI that satisfies the described user stories while
  minimizing the impact on footprint.

* The CNI plugin should run without the ClusterNetworkOperator, providing
  the `microshift-networking` elements any basic system configuration elements,
  tools or dependencies to make the CNI plugin work.

* Follow the [MicroShift design
  principles](https://github.com/openshift/microshift/blob/main/docs/design.md)

### Non-Goals

* We do not plan to support multi-node clusters.
* We are not trying to address high availability within the CNI.
* While some of the optimizations applied to OVNKuberneters in MicroShift could
  be applicable to single-node OpenShift that is out of scope for this enhancement
  and should be handled separately.

## Proposal

To meet the requirements described in the user stories, as well footprint
requirements of the edge environment, an optimized version of OVNKubernetes
is proposed.

This option aligns better with Red Hat's strategy on networking and avoids
the need to maintain bridge-cni which is deprecated in podman 4.

Using OVN-kubernetes is only possible thanks to the following optimizations:
* Single node operation
* Communication via unix sockets, avoiding the need of TLS & Certificate creation
  at boot time
* Removed CRDs for egressip, egressfirewall and egressqos. These CRDs are supported
  by OVNKubernetes, but not required for microshift.
* No leader election for the ovn databases, as there is no high availability or
  multi-node operation.
* OpenvSwitch and ovsdb-server RAM optimization
  [CPUAffinity=0 and --no-mlockall](https://bugzilla.redhat.com/show_bug.cgi?id=2106570)
* [Workload partitioning](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md),
   to limit the cpusets where specific ovn-kubernetes workloads
  can run.
* New ovn-kubernetes image with reduced footprint, oriented to single node operation.
* ovn-dbchecker and kube-rbac-proxy containers are removed, ovn-dbchecker will run on the
  ovn-master container.

The optimizations reduce the total footprint impact of OVN-Kubernetes + OpenvSwitch
down to:
* 100MB of RAM (from +600MB)
* 366MB of Disk (from 1GB)
* 0.011CPU cores (based on an i7-7600 platform or a Xeon 4210 VM with 1 VCPU)

Remaining work items in ovn-kubernetes:
* Handling node IP changes
* Improve cache allocation in libovsdb (this accounts for 16MB on northbound
southbound database clients)

Ideas for future improvements:
* Ovnk code efficiency: Using index, not list all. I.e.  getInterface(index)
  vs getInterfaces(), avoid allocation of large buffers when not necessary,
  use less informers or event broadcasters where possible.

* Single node operation: Disable ingress ip rotation, Remove ovn-master leader
  election, further simplification of the ovn topology.

* Go optimizations: cgo enable/disable comparison, libovsdb, netlink libraries,
  analyzing heap memory usage via pprof.

* Debugging improvements: “-s -w” build flags to remove debug info, enable pprof only
  on debug builds.

### Workflow Description

#### Deploying

MicroShift should setup the CNI during startup, and any specific configurations
to CRI-O or the system should be handled by the RPM install of `microshift-networking`.

#### Upgrading

Upgrades to the OVN database should be handled by the ovnkube-master during
the ovn startup process.

#### Configuring

MicroShift reads its configuration file once on startup and feeds any
necessary network details (like Pod or Service Cluster IP CIDRs down
to the CNI)

#### Deploying Applications

Applications are deployed as usual.

### API Extensions

The MicroShift default CNI does not add any APIs to OpenShift.

### Risks and Mitigations

Implementing OVNKubernetes without the ClusterNetworkOperator and under
an optimized topology requires a different set of deployment manifests,
those manifests are embedded into MicroShift and the combination of
MicroShift plus CNI should be subject to the same amount of testing
OVN Kubernetes is, excluding not supported features like multi-node.

Footprint optimizations applied today could stop being effective in the
long term. To mitigate this risk continous performance/footprint analysis
must be implemented in MicroShift.

### Drawbacks

The drawback of using OVN Kubernetes instead of something simple like
bridge-cni is the increase of disk, RAM and CPU footprint.

## Design Details

### Open Questions [optional]

1. 
### Test Plan

We will run the network Kubernetes and OpenShift compliance tests in
CI.

We will have end-to-end CI tests for many deployment scenarios.

We will have manual and automated QE for other scenarios.

We will have automated QE scenarios measuring RAM/Disk footprint.

We will submit Kubernetes conformance results for MicroShift to the
CNCF.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to use the networking elements of the CNI in MicroShift,
  including NetworkPolicies
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

The first release will have "limited availability" to a few partners
and customers.

- CNI stable
- Upgrade path tested
- Downgrade path tested (green boot rollback)

#### Full GA

- Available by default

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The ovn-master should maintain the database schema in sync with the
specific OVN version.

### Version Skew Strategy

There is no version skew for most MicroShift components because they
are built in or running from images for which the references are built
in.

### Operational Aspects of API Extensions

No API extensions.

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

* [openshift/microshift](https://github.com/openshift/microshift)
* [Design guidelines](https://github.com/openshift/microshift/blob/main/docs/design.md)

## Alternatives

### bridge-cni

Bridge CNI is part of the CRI-O project and provides minimal connectivity
between pods on a single node deployment. While bridge-cni is very lightweight,
because nothing needs to be deployed into the cluster, and it's made of a single
cni binary installed along with CRI-O, it does not provide support for
NetworkPolicies.

### openshift-sdn

While openshift-sdn is simpler than OVN-Kubernetes, it's deprecated, and does
not provide complete support for neither NetworkPolicies or IPv6.

### flannel & canal

Neither flannel or canal are directly supported by Red Hat today, and the total
footprint is bigger than the optimized version of OVN Kubernetes.
