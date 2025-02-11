---
title: ovn-hypershift-live-migration
authors:
  - "@ellorent"
reviewers:
  - "@martinkennelly"
  - "@trozet"
  - "@dcbw"
  - "@danwinship"
  - "@tssurya"
  - "@numans"
approvers:
  - "@martinkennelly"
  - "@trozet"
api-approvers:
  - "@martinkennelly"
  - "@trozet"
creation-date: 2023-02-06
last-updated: 2023-02-06
tracking-link:
  - "https://issues.redhat.com/browse/CNV-22946"
see-also:
  - "https://hypershift-docs.netlify.app/"
---


# OVN-kubernetes live migration for hypershift kubevirt provider

## Summary

The [Hypershift](https://hypershift-docs.netlify.app/) project creates openshift clusters on demand on top of already provisioned openshift by deploying  (and attaching working nodes to) "Hosted Control Planes" using one of different providers, such as aws, azure, baremetal (agent), and kubevirt.

In KubeVirt's case, the worker nodes will be spun as KubeVirt virtual machines.

There are some network requirements, some of them are imposed by kubevirt 
live migration and others by kubernetes:

- The IP Kubelet uses to register/communicate must follow the node during live migration.
- Guest Cluster Network isolation. Guest clusters shouldn’t be able to talk to other guest clusters directly except through public LBs. Guest clusters shouldn’t be able to talk to any infra components except through public LBs.
- No TCP connection breakage over E/W and N/S during live migration with minimal disruption.
  - Disruption for pre-copy live migration ~1sec
  - Disruption for post-copy live migration (this happend if libvirt activate the target domain before live migration finish) ~8sec
- Expose hosted cluster pods running at workers (VMs) using NodePort/LoadBalancer services at infra cluster.
- Allow workers services access to services like kubedns and apiserver from hosted control plane.

To accomplish the goals listed above the ovn-kubernetes default network has to implement some features triggered for an Hypershift KubeVirt VM.

## Motivation

Hypershift and kubevirt integration requires the ability to have live-migration functionality across the network with OVN-Kubernetes implemented at the pod's default network.

### User Stories

I, a Hypershift user, need to migrate one hosted cluster without affecting different ones and have minimal
disruption.

### Goals

Hypershift kubevirt provider needs the live-migration functionality, missing features at CNI need to be implement at the ovn-kubernetes 
which is the default CNI for openshift. This should work with dual stack.

### Non-Goals

This feature is not implemented at secondary networks, only primary networks will be live migratable.

## Proposal

At kubevirt every VM is executed inside a "virt-launcher" pod; how the "virt-launcher" pod net interface is "bound" to the VM is specified by the network binding at the VM spec.

The main VM network binding mechanisms are:
- masquerade binding: the VM has a stable fake address that is masqueraded (SNAT) at virt-launcher pod. 
- bridge binding: The virt-launcher interface is replaced by a bridge with one leg on the pod interface and the other at the VM tap device. When IPAM is configured on the pod interface, a DHCP server is started to advertise the pod's IP to DHCP aware VMs.

Since Masquerade binding has a fake address at VM guest it cannot be used for hypershift, the kubernetes nodes should see the address they are using to register to the api server.

The bridge binding is able to expose the pod IP to the VM as expected by hypershift but kubevirt does not support live migration with it, for kubevirt to support live migration with bridge binding 
at primary interface the OVN kubernetes CNI and controllers need to do the following:
- Do IP assignemnt with ovn-k but skip the CNI part that configure it at pod's netns veth.
- Do not expect the IP to be on the pod netns.
- Add the ability at ovn-k controllers to migrate pod's IP from node to node.



### Workflow Description

The gist of the design is:

1. Hypershift kubevirt workers attach to ovn-k node switches but CNI does not set IP addresses in virt-launcher pods netns
2. Deliver IP address at VMs configuring logical switch port DHCP options, the kubevirt DHCP server is deactivated.
3. A point to point routing is used so one node's subnet IP can be routed from different node
3. The VM's gateway IP and MAC are independent of the node they are running on using proxy arp
4. OVN missing ARP proxy features, configure MAC and IPv6, will be part of future 23.06 release, this is the [commit](https://github.com/ovn-org/ovn/commit/77846b215f317695384bd4bd27a647f6607413b1)


### API Extensions

n/a

## Design Details

### OVN-Kubernetes 

#### Topology

**Point to point routing:**

When the VM is running at a node that do not "own" it's IP address, like what happend
after a live migration, do point to point routing with a policy for outbound traffic and a static route for
inbound. By doing this, the VM can live migrate to different node and keep previous 
addresses (IP / MAC), thus preserving n/s and e/w communication, and ensuring traffic
goes over the node where the VM is running. The latter reduces inter node communication.

If the VM is going back to the node that "owns" the ip, those static routes and 
policies should be reconciled (deleted).

```text
       ┌───────────────────────────┐
       │     ovn_cluster_router    │
┌───────────────────────────┐   ┌────────────────────────────┐
│ static route              │───│ policy                     │
│ prefix: 10.244.0.8        │   │ match: ip4.src==10.244.0.8 │
│ nexthop: 10.244.0.8       │   │ action: reroute            │
│ output-port: rtos-node1   │   │ nexthop: 10.64.0.2         │
│ policy: dst-ip            │   │                            │  
└───────────────────────────┘   └────────────────────────────┘ 
```
**Nodes logical switch ports:**

The "router" logical switch port per node need to activate "arp_proxy" option
with a fixed MAC and VMs gateway so VM neighbor cache is consistent 
after live migration also VM's subnet need to be added so VM can ping IP's 
from same subnet after live migration.

```text
    ┌────────────────────┐   ┌────────────────────┐
    │logical switch node1│   │logical switch node2│
    └────────────────────┘   └────────────────────┘
┌────────────────────┐  │     │   ┌─────────────────────┐      
│ lsp stor-node1     │──┘     └───│ lsp stor-node2      │      
│ options:           │            │ options:            │    
│  arp_proxy:        │            │   arp_proxy:        │
│   0a:58:0a:f3:00:00│            │    0a:58:0a:f3:00:00│
│   169.254.1.1      │            │    169.254.1.1      │
│   10.244.0.0/24    │            │    10.244.0.0/24    │
└────────────────────┘            └─────────────────────┘
```

**VMs logical switch ports:**

The logical switch port for new VMs will use ovn-k ipam to reserve an IP address,
and when live-migrating the VM, the LSP address will be re-used.

CNI must avoid setting the IP address on the migration destination pod, but ovn-k controllers should 
preserve the IP allocation.

Also the DHCP options will be configured to deliver the address to the VMs

```text
    ┌──────────────────────┐   ┌────────────────────┐
    │logical switch node1  │   │logical switch node2│
    └──────────────────────┘   └────────────────────┘
┌──────────────────────┐  │     │   ┌──────────────────────┐      
│ lsp ns-virt-launcher1│──┘     └───│ lsp ns-virt-launcher2│     
│ dhcpv4_options: 1234 │            │ dhcpv4_options: 1234 │
│ address:             │            │ address:             │    
│  0a:58:0a:f4:00:01   │            │  0a:58:0a:f4:00:01   │
│  10.244.0.8          │            │  10.244.0.8          │
└──────────────────────┘            └──────────────────────┘

┌─────────────────────────────────┐
│ dhcp-options 1234               │
│   lease_time: 3500              │
│   router: 169.254.1.1           │
│   dns_server: [kubedns]         │
│   server_id: 169.254.1.1        │
│   server_mac: c0:ff:ee:00:00:01 │
└─────────────────────────────────┘
```

**Virt-launcher pod address:**
The CNI will not set an address at virt-launcher pod netns, that address is
assigned to the VM with the DHCP options from the LSP, this allows to use 
kubevirt bridge binding with pod networking and still do live migration.

#### IPAM

The point to point routing feature allows an address to be running at a node different from
the one "owning" the subnet the address is coming from. This will happen
after VM live migration.

One scenario for live migration is to shut down the node the VMs were migrated
from, this means that the IPAM node subnet should go back to the pool but since
the migrated VMs contains IPs from it those IPs should reserved in case the 
subnet is assigned to a new node.

On that case before assigning the subnet to the node the VMs ips need to be 
reserved so they don't get assigned to new pods

Another scenario is ovn-kubernetes pods restarting after live migration, on 
that case ovnkube-master should discover to what IP pool the VM belongs 
(using an annotation or searching for subnets) and reserve the address.

#### Detecting hypershift VMs and live migration

This enhancement is implemented as part of ovn-k pod's CNI and controllers so
the feature detection mechanism have to be implemented as part of kubevirt
virt-launcher pods.

To skip CNI IP configuration and assign DHCP options to LSP the following
label and annotation are check to ensure that only kubevirt pods with explicit configuration
are affected:
- `ovn.org/skip-ip-configuration-on-cni`
- `kubevirt.io/vm=vm1`

As part of this enhancement ovn-k should configure a point to point routing after 
live migration to redirect it to the correct node and port, this feature is only
activated if the pod is annotated with `kubevirt.io/allow-pod-bridge-network-live-migration`.

During live migration there is source and target virt-launcher pod's with different names, the ovn-k pod's controller use `CreationTimestamp` and `kubevirt.io/vm=vm1`
to differentiate between them, then it watch the following annotation and label 
at target pod `kubevirt.io/nodeName` and `kubevirt.io/migration-target-start-timestamp`:
- The `kubevirt.io/nodeName` is set after the VM finishes live migrating or when it becomes ready.
- The `kubevirt.io/migration-target-start-timestamp` is set when live migration has not finished but migration-target pod is ready to receive traffic (this happens at post-copy live migration, where migration is taking too long).

The point to point routing cleanup (remove of static route and policy) will be done at two cases:
- VM is deleted.
- VM is live migrated back to the node that owns its IP.

### Risks and Mitigations

#### OVN extended arp_proxy features


The current ovn arp_proxy is missing the following features: enforcing the MAC 
address and ipv6, ideally those get implemented and ovn-kubernetes have 
them for 4.14 release.

In case they don't arrive on time, it will not be able to configure IPv6 hosted
clusters at hypershift and VMs ARP table will need to be refreshed after live
migration. 

### Drawbacks

Doing live migration over pod network with kubevirt and changing its topology may present some risks and need to be 
thoroughly tested and supported.

### Open Questions 

Does this impact non hypershift payloads ? The control-plane will have some more
load from the point to point routing but data-plane is not affected

Does this work with future interconnect topology ? It's not using a distributed 
switch per hosted cluster so that part should be fine, but it may needed 
to replicate point to point routing at all node's ovn_cluster_router.

### Test Plan

Add some network tests to [hypershift conformance](https://github.com/openshift/origin/pull/27456)
that will do live migration with the following scenarios:
- TCP connection between VMs
- TCP connection to VMs from service
- TCP connection from VM to service

The connection has to survive and downtime should be minimal.


### Graduation Criteria

#### Dev Preview -> Tech Preview
n/a

#### Tech Preview -> GA

Current hypershift kubevirt provider tech preview is 4.13, this enhancement
will allow hypershift kubevirt provider one of the GA requirements
(live migration) for 4.14.

#### Removing a deprecated feature
n/a

### Upgrade / Downgrade Strategy

Since hypershift is tech preview before this enhancement Upgrade/Downgrade is 
not supported for hosted clusters, but it should not break openshift 
networking for other payloads on upgrade or downgrade.

### Version Skew Strategy
n/a

### Operational Aspects of API Extensions

#### Failure Modes

Probable failure situations include
- OVN control plane too loaded by point to point routing
  - Too much CPU / Memory
  - Too slow a transaction rate
  - Disk is almost full

- From bug at the feature ovn-k can suffer the following failures:
  - Stale IP allocations can empty node's IP pool if a bug is present at the feature, can be addresses with reconcile cycle.
  - Stale policies and static routes at ovn_cluster_router after live migration or VM's deletion, can be addresses with reconcile cycle.

#### Support Procedures

The CNV network team will be responsible for debugging hypershift kubevirt live 
migration issues, alerts implemented as events related to those routed should be helpful. 

## Implementation History

4.14: Initial implementation

## Alternatives

### Use multi-homing L2 with a subnet and connect it to default pod network

The multi-homing code isn't GA yet, and connecting it to pod network is a bigger risk than adding
requirements directly to current pod networking.

