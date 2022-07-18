---
title: microshift-default-cni
authors:
  - majopela
  - zshi-redhat
  - dhellmann
reviewers:
  - "@copejon, MicroShift contributor"
  - "@fzdarsky, MicroShift architect"
  - "@ggiguash, MicroShift contributor"
  - "@sallyom, MicroShift contributor"
  - "@oglok, MicroShift contributor"
  - "@dcbw, OpenShift Networking Manager"
approvers:
  - "@derekwaynecarr"
api-approvers:
  - None
creation-date: 2022-07-18
last-updated: 2022-07-18
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-40
see-also:
  - https://github.com/openshift/microshift/blob/main/docs/design.md
  - https://github.com/openshift/enhancements/pull/1187
---

# Default CNI 

## Summary

This enhancement proposes the adoption of a default MicroShift CNI.

MicroShift addresses customer use cases with low-resource,
field-deployed edge devices (SBCs, SoCs) requiring a minimal K8s
container orchestration layer.

MicroShift is targeting a class of devices like Xilinx ZYNQ
UltraScale+, fitlet2, NVIDIA Jetson TX2, or Intel NUCs that are
pervasive in edge deployments. These cost a few hundred USD, have all
necessary accelerators and I/O on-board, but are typically not
extensible and are highly constrained on every resource, e.g.:

* CPU: ARM Cortex-A35-class or Intel Atom class CPU, 2-4 cores, 1.5GHz clock
* memory: 2-16GB RAM
* storage: e.g. SATA3 @ 6Gb/s, 10kIOPS, 1ms (vs. NVMe @ 32Gb/s,
  500kIOPS, 10µs)
* network: Less bandwidth and less reliable than data center-based
  servers, including being completely disconnected for extended
  periods. Likely 1Gb/s NICs, potentially even LTE/4G connections at
  5-10Mbps, instead of 10/25/40Gb/s NICs


## Motivation

MicroShift used flannel during the proof of concept phase, which later on
was simplified to bridge-cni, being that pluggin the most minimalistic
plugin which would provide Pod to Pod communication in a single-node environment
and nothing else.

A very important characteristic necessary for any CNI plugin used in MicroShift
is a low-enough RAM, Disk, CPU footprint and boot times, as explained in the summary,
the target platforms for MicroShift are small SBCs or SoCs where most of
the computing capability must remain available for the final applications,
otherwise MicroShift stops being useful in it's target environment.

### User Stories

An application developer builds applications where:
1. Pods need to talk to each other via TCP o UDP connectivity.
2. Pods need to talk to services on the LAN or Internet via TCP or UDP.
3. Some pods need be exposed as NodePort types of services on the LAN.
4. Some pods need to be exposed to other pods as ClusterIP services types.

An application developer builds applications where some pods must
have limited connectivity to other pods, or to the external network (i.e.
plugin services from vendors which must only contact specific internal APIs)

The edge device with MicroShift and the final application is deployed on an
IPv6, or DualStack IPV4/IPV6 network.

As a device owner I can protect the edge device with a host-level firewall
(firewalld, iptables, nftables), and that type of configuration is compatible
with the CNI.

The edge device should be capable of deploying on a network with dynamic address
provisioning like DHCP, SLAAC, or DHCPv6 and remain functional when the IP address
changes.

### Goals
 
* Implement a default CNI that satisfies the described user histories while
  minimizing the impact on footprint.

* The CNI plugin should run without the ClusterNetworkOperator, providing
  the `microshift-networking` elements any basic system configuration elements,
  tools or dependencies to make the CNI plugin work.

* Follow the [MicroShift design
  principles](https://github.com/openshift/microshift/blob/main/docs/design.md)

### Non-Goals

* We do not plan to support multi-node clusters.
* We are not trying to address high availability within the CNI, 
  high availability should be acomplished by having multiple edge devices working
  as single node MicroShifts.

* LoadBalancer type of Services, as this is out of the scope of the CNI.

## Proposal

To meet the requirements described in the user histories, as well footprint
requirements of the edge environment, an optimized version of OVN Kubernetes
is proposed.

This option aligns better with Red Hat's strategy on networking and avoids
the need to maintain bridge-cni which is deprecated in podman 4.

Using OVN-kubernetes is only possible thanks to the following optimizations:
* Single node operation
* Communication via unix sockets, avoiding the need of TLS & Certificate creation
  at boot time
* No leader election, as there is no high availability or multi-node operation.
* OpenvSwitch and ovsdb-server RAM optimization (CPUAffinity=0 and --no-mlockall)
* Workload partitioning, to limit the cpusets where specific ovn-kubernetes workloads
  can run.
* New ovn-kubernetes with reduced footprint, oriented to single node operation.

The optimizations reduce the total footprint impact of OVN-Kubernetes + OpenvSwitch
down to:
* 100MB of RAM (from +600MB)
* 366MB of Disk (from 1GB)
* 0.011CPU cores (based on an i7-7600 platform or a Xeon 4210 VM with 1 VCPU)

Remaining work items in ovn-kubernetes:
* Handling node IP changes
* Improve cache allocation in libovsdb (this accounts for 16MB on northd and southdb
  database clients)

### Workflow Description

#### Deploying

MicroShift should setup the CNI during startup, and any specific configurations
to cri-o or the system should be handled by the RPM install of `microshift-networking`.

#### Upgrading

Upgrades to the OVN database should be handled by the ovn-dbchecker during
the ovn startup process.

#### Configuring

MicroShift reads its configuration file once on startup and feeds any
necessary network details (like Pod or Service Cluster IP CIDRs down
to the CNI)

#### Deploying Applications

Applications are deployed on MicroShift using standard Kubernetes
resources such as Deployments, Services or NetworkPolicies. 
They can be deployed at runtime via API calls, or embedded in the system
image with the manifests loaded from the filesystem by MicroShift on startup.
By using the `IfNotPresent` pull policy and adding images to the CRI-O cache
in the system image, it is possible to build a device that can boot and
launch the application without network access.

### API Extensions

The MicroShift default CNI does not add any APIs to OpenShift.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

Implementing OVNKubernetes without the ClusterNetworkOperator and under
an optimized topology requires a different set of deployment manifests,
those manifests are embedded into MicroShift and the combination of
MicroShift plus CNI should be subject to the same amount of testing
OVN Kubernetes is, excluding not supported features like multi-node.

Footprint optimizations applied today could stop being effective in the
long term. To mitigate this risk continous performance/footprint analysis
must be implemented in MicroShift.

Because MicroShift will run as only a single node, for application
availability we will recommend running two single-node instances that
deploy a common application in active/active or active/passive mode
and then using existing tools to support failover between those states
when either host is unable to provide availability.  This
configuration is more reliable than a single node control plane with a
worker, because if the worker loses access to the control plane
(through power or network loss), the worker has no way to restore or
recover its state, and all workloads could be affected.

### Drawbacks

The drawback of using OVN Kubernetes instead of something simple like
bridge-cni is the increase of disk, RAM and CPU footprint.

## Design Details

### Open Questions [optional]

1. 
### Test Plan

We will run the relevant Kubernetes and OpenShift compliance tests in
CI.

We will have end-to-end CI tests for many deployment scenarios.

We will have manual and automated QE for other scenarios.

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

The ovn-dbchecker should maintain the database schema in sync with the
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

Bridge CNI is part of the cri-o project and provides minimal connectivity
between pods on a single node deployment. While bridge-cni is very lightweight,
because nothing needs to be deployed into the cluster, and it's made of a single
cni binary installed along with cri-o, it does not provide support for
NetworkPolicies.

### openshift-sdn

While openshift-sdn is simpler than OVN-Kubernetes, it's deprecated, and does
not provide complete support for neither NetworkPolicies or IPv6.

### flannel & canal

Neither flannel or canal are directly supported by Red Hat today, and the total
footprint is bigger than the optimized version of OVN Kubernetes.