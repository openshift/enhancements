---
title: virtual-private-cloud
authors:
  - dave-tucker
  - skitt
  - tssurya
reviewers:
  - danwinship
  - jcaamano
  - knobunc
approvers:
  - knobunc
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None". Once your EP is published, ask in #forum-api-review to be assigned an API approver.
  - TBD
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
status: implementable
tracking-link: https://issues.redhat.com/browse/OCPSTRAT-2845
  - TBD
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Virtual Private Cloud (VPC)

## Summary

Virtual Private Clouds (VPCs) are a well defined concept for network
virtualization within a Cloud Environment. We are starting to see more
and more use-cases where users are trying to build out their own Private
Cloud environments and wish to extend this VPC concept in their Bare Metal
environment. This enhancement proposal aims to define what a VPC is on
OpenShift Bare Metal clusters.

## Motivation

Today, achieving VPC-like isolation on OpenShift requires manually creating and
wiring together multiple low-level networking primitives (C(UDN)s, CNCs,
RouteAdvertisements, EgressIPs, NetworkPolicies), and even then there are gaps
to achieving a full VPC (eg. Route Table). This is error-prone, provides no single
pane of glass for network health, and does not extend across cluster boundaries.
Users deploying distributed applications like CockroachDB need the ability to
extend networks between clusters and have policies, routes, and connectivity
follow wherever the network is present.

### User Stories

* As a Red Hat OpenShift customer, I want to extend my User Defined Networks
  between OpenShift clusters so that I can deploy distributed applications
  (e.g. CockroachDB) that require cross-cluster network connectivity.

* As a cluster administrator, I want to define a VPC with subnets, routing,
  and security policies in a single resource so that I do not have to
  individually create and wire together C(UDN)s, CNCs, RouteAdvertisements,
  EgressIPs, and NetworkPolicies.

* As a cluster administrator, I want the VPC controller to synchronize network
  configuration between clusters so that custom routes, network policies,
  and VPN connections are rendered wherever the network is present.

* As an application developer, I want to deploy workloads into a namespace
  that belongs to a VPC subnet without needing to understand the underlying
  networking primitives.

### Goals

- Define the VPC CRD and the VPC controller that translates VPC
  intent into lower-level OVN-Kubernetes constructs.
- Support heterogeneous subnets (Layer2/Layer3, Geneve/EVPN)
  within a single VPC.
- Provide automatic intra-VPC routing between all subnets.
- Enable VPC peering via extended ClusterNetworkConnect.
- Support multi-cluster VPC extension: synchronize networks, policies, and
  routes across clusters.
- Preserve backward compatibility: existing bare C(UDN) and CNC workflows
  continue to work without a VPC.

### Non-Goals

- Transit Gateway modeling (cloud-provider specific).
- VPN connection management (out of scope for initial delivery).
- Replacing or modifying the existing OVN-Kubernetes C(UDN)/CNC APIs -- the
  VPC controller builds on top of them.

## Introduction

A **Virtual Private Cloud (VPC)** is a logically isolated virtual network within
a cloud environment. It gives the owner complete control over IP address ranges,
subnets, route tables, and network gateways. VPCs are a foundational construct
in both public and private clouds:

- [AWS VPC](https://docs.aws.amazon.com/vpc/latest/userguide/how-it-works.html) --
  Amazon's VPC provides an isolated section of the AWS cloud with user-defined
  IP ranges, subnets across Availability Zones, route tables, internet and NAT
  gateways, security groups, and VPC peering.
- [VMware NSX VPC](https://techdocs.broadcom.com/us/en/vmware-cis/nsx/vmware-nsx/4-2/administration-guide/nsx-multi-tenancy/nsx-virtual-private-clouds.html) --
  NSX VPCs provide a self-service multi-tenant networking model with independent
  routing domains, Layer 2 subnets, Tier-0/VRF gateway connectivity, DHCP, and
  IP address management.

The following subsections define the core constructs that make up a VPC.

### Subnets

A **subnet** is a segment of a VPC's IP address range where workloads are placed.
Subnets provide isolation within the VPC and can be classified by their
connectivity:

- **Public subnet**: Has a route to an internet gateway; resources are externally
  reachable.
- **Private subnet**: No direct internet route; uses NAT for outbound access.
- **Isolated subnet**: No routes to destinations outside the VPC.
- **VPN-only subnet**: Routes traffic through a VPN connection.

See: [AWS Subnets](https://docs.aws.amazon.com/vpc/latest/userguide/configure-subnets.html),
[NSX VPC Subnets](https://techdocs.broadcom.com/us/en/vmware-cis/nsx/vmware-nsx/4-2/administration-guide/nsx-multi-tenancy/nsx-virtual-private-clouds.html)

### Route Tables

A **route table** contains rules that determine where network traffic is
directed. Every subnet is associated with a route table. Routes specify a
destination CIDR and a target (e.g. internet gateway, NAT gateway, peering
connection). The most specific matching route (longest prefix match) wins.

In a VPC, all subnets automatically have a "local" route that enables
intra-VPC communication -- subnets within the same VPC can always reach each
other.

See: [AWS Route Tables](https://docs.aws.amazon.com/vpc/latest/userguide/VPC_Route_Tables.html)

### Internet Gateway / NAT Gateway

An **internet gateway** enables communication between VPC resources and the
public internet (for public subnets). A **NAT gateway** provides outbound
internet access for resources in private subnets without exposing them to
inbound internet traffic.

See: [AWS Internet Gateway](https://docs.aws.amazon.com/vpc/latest/userguide/working-with-igw.html)

### Security Groups

A **security group** is a stateful virtual firewall that controls inbound and
outbound traffic at the instance level. Rules specify allowed protocols, ports,
and source/destination CIDRs. Because they are stateful, return traffic is
automatically allowed.

See: [AWS Security Groups](https://docs.aws.amazon.com/vpc/latest/userguide/vpc-security-groups.html)

### Network ACLs

A **network ACL (NACL)** is a stateless firewall at the subnet level. Unlike
security groups, NACLs evaluate rules in order (by rule number) and require
explicit allow/deny for both inbound and outbound traffic.

See: [AWS Network ACLs](https://docs.aws.amazon.com/vpc/latest/userguide/nacl-basics.html)

### VPC Peering

A **VPC peering connection** is a networking connection between two VPCs that
enables routing traffic between them using private IP addresses. It is not a
gateway or VPN -- there is no single point of failure or bandwidth bottleneck.
Peered VPCs cannot have overlapping CIDR blocks.

See: [AWS VPC Peering](https://docs.aws.amazon.com/vpc/latest/peering/vpc-peering-basics.html)

### Route Server

A **route server** enables dynamic routing within a VPC by exchanging routes
with network appliances via BGP. It automatically updates VPC route tables
when devices fail, providing routing fault tolerance without manual
intervention.

See: [AWS VPC Route Server](https://docs.aws.amazon.com/vpc/latest/userguide/dynamic-routing-route-server.html)

### VPN Connections

A **VPN connection** provides encrypted IPsec connectivity between a VPC and
an on-premises network or another VPC over the public internet. AWS supports
Site-to-Site VPN (two redundant tunnels per connection) and Client VPN
(OpenVPN-based remote access).

See: [AWS VPN Connections](https://docs.aws.amazon.com/vpc/latest/userguide/vpn-connections.html)

### Transit Gateway

A **transit gateway** acts as a central hub for routing traffic between
multiple VPCs, VPN connections, and on-premises networks. It simplifies
network architecture by avoiding the need for full-mesh peering between
every pair of VPCs.

See: [AWS Transit Gateway](https://docs.aws.amazon.com/vpc/latest/tgw/what-is-transit-gateway.html)

## Proposal

We plan to take a bottom-up approach at solving this for a single-cluster
first, and then extrapolating that for multi-cluster scenarios.

### Single Cluster

In a single cluster, a VPC is an isolation boundary that groups one or more
subnets (UDNs/CUDNs) into a logically isolated network with its own address
space, automatic intra-VPC routing, and well-defined points of external
connectivity.

```
                      ┌──────────────────────────────────────────────────────────┐
                      │              CNC (VPC Peering)                           │
                      │    vpcSelector: {peer-group: prod-staging}               │
                      │    connectSubnets: [{cidr: 192.168.0.0/16, /24}]         │
                      │    connectivity: [PodNetwork, ClusterIPServiceNetwork]   │
                      └───────────────────────┬──────────────────────────────────┘
                                              │
                            ┌─────────────────┴──────────────────┐
                            │ selects VPCs by label              │
                            ▼                                    ▼
          ┌──────────────────────────────┐    ┌──────────────────────────────┐
          │  VPC: production             │    │  VPC: staging                │
          │  labels:                     │    │  labels:                     │
          │    peer-group: prod-staging  │    │    peer-group: prod-staging  │
          │  cidrBlocks: [10.0.0.0/16]   │    │  cidrBlocks: [10.1.0.0/16]   │
          │  subnets: [web(Public),      │    │  subnets: [app]              │
          │    app(Private),             │    │                              │
          │    db(Isolated),             │    │                              │
          │    mgmt(VPNOnly),            │    │                              │
          │    vm-network(Private)]      │    │                              │
          │                              │    │                              │
          │  ┌─────────────────────┐     │    │  ┌─────────────────────┐     │
          │  │ Intra-VPC routing:  │     │    │  │ Intra-VPC routing:  │     │
          │  │ automatic "local"   │     │    │  │ automatic "local"   │     │
          │  │ route between all   │     │    │  │ route between all   │     │
          │  │ member subnets      │     │    │  │ member subnets      │     │
          │  └─────────────────────┘     │    │  └─────────────────────┘     │
          └──┬──────┬──────┬──────┬──────┬─────┘    └──────┬───────────────────────┘
             │      │      │      │      │                 │
    creates  │      │      │      │      │ C(UDN)s         │ creates C(UDN)s
             ▼      ▼      ▼      ▼      ▼                 ▼
          ┌──────┐┌──────┐┌──────┐┌──────┐┌──────────┐  ┌─────────┐
          │C(UDN)││C(UDN)││C(UDN)││C(UDN)││ C(UDN)   │  │ C(UDN)  │
          │prod- ││prod- ││prod- ││prod- ││ prod-vm  │  │ staging │
          │web   ││app   ││db    ││mgmt  ││ -network │  │ -app    │
          │      ││      ││      ││      ││          │  │         │
          │Public││Privat││Isolat││VPN   ││ Private  │  │ Private │
          │L3    ││L3    ││L3    ││L3    ││ L2       │  │ L3      │
          │Genev ││Genev ││Genev ││Genev ││ EVPN     │  │ Geneve  │
          │/24   ││/24   ││/24   ││/24   ││ /24      │  │ /24     │
          └──┬───┘└──┬───┘└──┬───┘└──┬───┘└────┬─────┘  └────┬────┘
             │       │       │       │         │              │
             ▼       ▼       ▼       ▼         ▼              ▼
          ns's    ns's    ns's    ns's     ns's          ns's
          (via namespaceSelector)                   (via namespaceSelector)

     ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─
     Resources created by the VPC controller based on connectivity:

     connectivity: Public  ──►  RouteAdvertisements (BGP export)
     connectivity: Private ──►  EgressIP (outbound NAT / SNAT)
     connectivity: Isolated ─►  (none — no external routes)
     connectivity: VPNOnly ──►  IPsec north-south encryption

     Additional CRDs applied per-VPC or per-subnet:

     ┌─────────────────────┐  ┌──────────────────────────────────┐
     │  RouteTable (new)   │  │  NetworkPolicy                   │
     │  Custom routes per  │  │  Security Groups                 │
     │  VPC: programs VRF  │  │  (per-namespace, stateful)       │
     │  (LGW) or GR (SGW)  │  └──────────────────────────────────┘
     └─────────────────────┘  ┌──────────────────────────────────┐
                              │  AdminNetworkPolicy              │
                              │  NACLs equivalent                │
                              │  (cluster-scoped, ordered rules) │
                              └──────────────────────────────────┘
```

The following table maps VPC constructs across AWS, OVN-Kubernetes, and VMware NSX:

| Feature | AWS VPCs | OVN-Kubernetes | VMware NSX |
|---|---|---|---|
| **Tenancy** | AWS Account | Namespace (multiple namespaces for C(UDN)) | NSX Project (Tenant) |
| **Workload Attachment** | Subnet | K8s Namespace | Subnet |
| **Fixed CIDR** | Yes | N/A (same as multiple subnets, see below) | Yes (expanded to a set of IP Blocks) |
| **Multiple Subnets** | Yes | VPC defines subnets; each becomes a C(UDN) | Yes |
| **Public Subnets** | Yes | `connectivity: Public` — RouteAdvertisements (BGP) | Yes |
| **Private Subnets** | Yes | `connectivity: Private` — EgressIP (outbound NAT) | Yes |
| **Isolated Subnets** | Yes | `connectivity: Isolated` — no external routes | Yes |
| **VPN-Only Subnets** | Yes | `connectivity: VPNOnly` — north-south IPsec | N/A (VPN setup/routing on Tier-0 gateway) |
| **Route Tables** | Yes | RouteTable CRD (new) - programs VRF in LGW and GR in SGW | Yes (via Tier-0 gateway) |
| **Route Server** | Yes | No. Fail-over applicable for multi-cluster but low priority | Yes (via Tier-0 gateway) |
| **Internet Gateway** | Yes | EgressIPs | Yes (via Tier-0 gateway) |
| **NAT Gateway** | Yes | EgressIPs + VPC egress configuration | Yes (via Tier-0 gateway) |
| **Security Groups** | Yes | NetworkPolicy | Yes |
| **VPC Peering** | Yes | ClusterNetworkConnect (extended with `vpcSelector`) | Yes (via inter-VRF routing) |
| **VPN Connections** | Yes | No | Yes |
| **Transit Gateway** | Yes | No. Cloud-provider specific, not planned to model | Yes (via Tier-0 gateway) |

### Multiple Clusters

In a multi-cluster deployment, the VPC must span cluster boundaries so that
subnets, policies, routes, and peering relationships are rendered on every
cluster where the VPC is present. The proposed model is **hub-spoke**.

```
     ┌───────────────────────────────────────────────────┐
     │                  Hub Cluster                      │
     │                                                   │
     │   VPC: production                                 │
     │     cidrBlocks: [10.0.0.0/16]                     │
     │     subnets:                                      │
     │     - public:  10.0.1.0/24  L3 Geneve              │
     │     - private: 10.0.10.0/24 L3 Geneve             │
     │     - vm-net:  10.0.100.0/24 L2 EVPN              │
     │                                                   │
     │   NetworkPolicy, AdminNetworkPolicy,              │
     │   RouteTable, RouteAdvertisements                 │
     │                                                   │
     │   ┌─────────────────────────────────────────┐     │
     │   │       VPC Controller (hub)              │     │
     │   │  - Watches VPC + associated resources   │     │
     │   │  - Does IPAM (per-cluster allocations)  │     │
     │   │  - Renders C(UDN)s locally              │     │
     │   │  - NO API access to spokes              │     │
     │   └──────────┬──────────────────┬───────────┘     │
     └──────────────┼──────────────────┼─────────────────┘
                    │                  │
        sync spec   │                  │  sync spec
        + policies  │                  │  + policies
        (transport  │                  │  (transport
         decoupled) │                  │   decoupled)
                    ▼                  ▼
     ┌──────────────────────┐  ┌──────────────────────┐
     │   Spoke Cluster A    │  │   Spoke Cluster B    │
     │                      │  │                      │
     │ VPC Controller (spoke)│  │ VPC Controller (spoke)│
     │  - Reads synced spec │  │  - Reads synced spec │
     │  - Creates C(UDN)s   │  │  - Creates C(UDN)s   │
     │  - Applies policies  │  │  - Applies policies  │
     │  - Self-heals        │  │  - Self-heals        │
     │                      │  │                      │
     │  C(UDN)s:            │  │  C(UDN)s:            │
     │  - prod-public       │  │  - prod-public       │
     │  - prod-private      │  │  - prod-private      │
     │  - prod-vm-net       │  │  - prod-vm-net       │
     │                      │  │                      │
     │  NetworkPolicy, ANP  │  │  NetworkPolicy, ANP  │
     │  RouteTable, RA      │  │  RouteTable, RA      │
     └──────────────────────┘  └──────────────────────┘
```

#### How it works

1. **Single source of truth**: The VPC resource and all associated resources
   (NetworkPolicy, AdminNetworkPolicy, RouteTable, RouteAdvertisements) are
   defined on the **hub cluster**. This is the only place users create or
   modify VPC configuration.

2. **Hub does IPAM**: The VPC controller on the hub allocates non-overlapping
   per-cluster CIDR slices for each subnet and writes the allocations into the
   VPC spec (or an associated status/allocation resource).

3. **Hub renders locally**: On the hub cluster, the VPC controller creates the
   local C(UDN)s, intra-VPC routing, and associated resources exactly as in
   the single-cluster model, using the hub's CIDR allocation.

4. **Spec synced to spokes**: The VPC spec (with per-cluster IPAM allocations)
   and associated policy/route resources are synced to each spoke cluster. The
   sync transport is decoupled from the VPC controller. The hub does not need
   API access to spoke clusters.

   **TODO**: Determine the sync mechanism. Evaluate whether the templating/binding
   model is necessary or whether the VPC controller should sync its own resolved
   spec directly. The following alternatives are under consideration:

   - **VPC controller syncs resolved spec directly**: The hub controller does
     IPAM, writes a fully-resolved per-cluster VPC spec (no placeholders), and
     syncs it to each spoke via a lightweight mechanism (e.g. git, shared config
     store, sync agent). The spoke VPC controller reads the resolved spec and
     renders C(UDN)s/policies locally. Simple, self-contained, no dependency on
     a templating system. Best fit if VPC is the primary multi-cluster use case.

   - **VPC controller generates templates and bindings**: The hub controller
     generates a VPC template (with placeholders like `{{ .clusterCIDR }}`) and
     per-cluster binding objects that provide IPAM allocations. The
     [templating/binding system](https://gist.github.com/skitt/c534dba0292b5533df7495de322dcd25)
     handles sync and instantiation on spokes. Reuses shared infrastructure --
     the same system can sync bare NetworkPolicies and UDNs independently of
     VPCs. Adds indirection (template → binding → instantiated resource) and a
     dependency on the templating CRDs/controllers.

   - **Hybrid: VPC syncs spec, templating handles policies**: The VPC controller
     syncs its own spec directly (for C(UDN) creation on spokes), but uses the
     templating/binding system for resources that genuinely vary per cluster
     (e.g. NetworkPolicies with cluster-specific IP blocks). Avoids building
     two full sync mechanisms while keeping the VPC sync path simple for the
     common case.

5. **Spoke controller renders and self-heals**: Each spoke cluster runs its
   own VPC controller instance. It reads the synced VPC spec, creates the
   corresponding C(UDN)s, NetworkPolicies, AdminNetworkPolicies, RouteTables,
   and RouteAdvertisements locally using its allocated CIDR slice. If any
   resource is accidentally deleted or drifts, the spoke controller recreates
   it from the synced spec. The spoke controller never modifies the VPC spec
   -- it only renders.

6. **Hub going down does not break spokes**: Since each spoke has a local copy
   of the VPC spec and its own controller, existing rendering continues
   uninterrupted. Only new VPC changes are delayed until the hub recovers.

7. **Cross-cluster connectivity**: The C(UDN)s on each cluster are connected
   via the underlying OVN-Kubernetes inter-cluster networking (e.g. EVPN or
   similar cross-cluster fabric). The VPC controller ensures the same subnets
   exist on each cluster so that workloads like CockroachDB can communicate
   across cluster boundaries on the same network.

#### IPAM across clusters

The VPC's `cidrBlocks` define the global address space. When rendering subnets
across multiple clusters, the VPC controller on the hub is responsible for
allocating non-overlapping per-cluster portions of each subnet's CIDR so that
pod IPs are globally unique across all clusters in the VPC.

For example, a subnet with CIDR 10.0.1.0/24 across three clusters:

- Cluster A: 10.0.1.0/26 (64 addresses)
- Cluster B: 10.0.1.64/26 (64 addresses)
- Cluster C: 10.0.1.128/26 (64 addresses)

The per-cluster allocation is recorded on the hub and pushed to each spoke as
part of the VPC spec sync. Each spoke's VPC controller uses its allocated
slice when creating the local C(UDN), ensuring no overlap. Pods communicate
with their real IPs across clusters without NAT -- important for applications
like CockroachDB that require stable, routable pod addresses.

#### What gets synchronized

| Resource | Synchronized? | Notes |
|---|---|---|
| VPC spec (subnets, CIDRs) | Yes | Hub → spokes; spoke controller creates C(UDN)s |
| NetworkPolicy | Yes | Rendered on every cluster where VPC subnets exist |
| AdminNetworkPolicy | Yes | Cluster-scoped policies synced to all spokes |
| RouteTable | Yes | Custom routes rendered on each cluster |
| RouteAdvertisements | Yes | BGP export config for public subnets |
| EgressIP | Cluster-specific | Each cluster may have different egress IPs |
| VPC peering (CNC) | Hub-managed | Peering between VPCs coordinated from hub |

### Approach

We propose introducing a new **VPC** CRD (cluster-scoped) and a new
**VPC controller** implemented in a new repository under the
ovn-kubernetes organization. The VPC controller runs as a separate daemonset,
deployed and managed by the Cluster Network Operator (CNO). It acts as a
translation layer: it watches VPC resources and automatically creates and
reconciles the underlying OVN-Kubernetes CRDs needed to realize the VPC. In
multi-cluster deployments, the operator also synchronizes network
configuration, policies, and routes across clusters.

The VPC controller's lifecycle management will be done through Cluster Network Operator
on OpenShift. [TBD] The vpc controller pods will just run on the control plane
of the cluster (management clustr in case of multiple clusters) and create the relevant
network constructs in OVN-Kubernetes.

When a user creates a VPC, the operator:

- Validates that subnet CIDRs fall within the VPC's address space
  (**CIDR governance**)
- Creates the underlying C(UDN)s for each subnet defined in the VPC spec
- Sets up automatic routing between all subnets in the VPC (the equivalent of
  the implicit "local" route in AWS), so that subnets can communicate without
  the user having to manually create interconnect resources
- Aggregates status from member C(UDN)s into a single VPC status, giving the
  user one place to check the health of their network
- Supports day-2 expansion: secondary CIDR blocks can be appended to the VPC to
  grow the address space, and new subnets can be added within those ranges

The controller maps VPC intent to the following existing and new OVN-Kubernetes
resources, none of which users need to create or manage directly:

| VPC Concept | Underlying OVN-K Resource | Notes |
|---|---|---|
| Subnet | C(UDN) (existing, unchanged) | Each C(UDN) is a subnet; heterogeneous topologies (L2/L3) and transports (Geneve/EVPN) within the same VPC |
| Intra-VPC routing | CNC (existing) | Automatic "local" route between all member C(UDN)s |
| VPC peering | CNC (existing, extended with `vpcSelector`) | Transit fabric between VPCs |
| Custom routes | RouteTable (new) | Programs VRF (LGW) or GR (SGW) |
| Public subnets | RouteAdvertisements (existing) | BGP export |
| NAT / IGW | EgressIP (existing) | Outbound SNAT |
| Security groups | NetworkPolicy (existing) | Stateful pod-level firewall |
| NACLs | AdminNetworkPolicy (existing) | Cluster-scoped ordered rules |

Users can still create bare C(UDN)s and CNCs directly without a VPC -- the
existing OVN-Kubernetes workflows are fully preserved. The VPC layer is
additive. The full API definitions are in the [API Extensions](#api-extensions)
section.

### Workflow Description

Explain how the user will use the feature. Be detailed and explicit.
Describe all of the actors, their roles, and the APIs or interfaces
involved. Define a starting state and then list the steps that the
user would need to go through to trigger the feature described in the
enhancement. Optionally add a
[mermaid](https://github.com/mermaid-js/mermaid#readme) sequence
diagram.

Use sub-sections to explain variations, such as for error handling,
failure recovery, or alternative outcomes.

For example:

**cluster creator** is a human user responsible for deploying a
cluster.

**application administrator** is a human user responsible for
deploying an application in a cluster.

1. The cluster creator sits down at their keyboard...
2. ...
3. The cluster creator sees that their cluster is ready to receive
   applications, and gives the application administrator their
   credentials.

See
https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md#high-level-end-to-end-workflow
and https://github.com/openshift/enhancements/blob/master/enhancements/agent-installer/automated-workflow-for-agent-based-installer.md for more detailed examples.

### API Extensions

This enhancement introduces the following API extensions:

#### VPC CRD (new)

A cluster-scoped CRD that defines an isolated network boundary. Its controller
handles CIDR governance, intra-VPC routing, and status aggregation.

```go
// VPC defines an isolated network boundary that groups one or more
// ClusterUserDefinedNetworks (subnets) into a logically isolated network
// with its own address space and automatic intra-VPC routing.
//
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:path=vpcs,scope=Cluster,shortName=vpc,singular=vpc
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type VPC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	// +required
	Spec VPCSpec `json:"spec"`

	// +optional
	Status VPCStatus `json:"status,omitempty"`
}

// VPCSpec defines the desired state of a VPC.
type VPCSpec struct {
	// cidrBlocks defines the address space for this VPC. Every subnet's
	// CIDR must fall within one of these blocks. The list is mutable:
	// secondary CIDR blocks can be appended after creation to expand
	// the VPC's address space. A CIDR block cannot be removed while
	// any subnet references addresses within it.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +required
	CIDRBlocks []CIDR `json:"cidrBlocks"`

	// subnets defines the subnets within this VPC. The VPC controller
	// creates a ClusterUserDefinedNetwork for each entry. Subnets
	// within the same VPC can have different topologies and transports.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +required
	Subnets []VPCSubnet `json:"subnets"`
}

// VPCSubnet defines a subnet within the VPC. The VPC controller creates
// a UDN or CUDN from this definition depending on the scope.
type VPCSubnet struct {
	// name is the subnet name. The resulting network resource is named
	// "<vpc-name>-<subnet-name>".
	//
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`

	// scope determines whether the controller creates a
	// ClusterUserDefinedNetwork (Cluster) or a UserDefinedNetwork
	// (Namespace). Defaults to Cluster.
	//
	// +optional
	// +kubebuilder:default=Cluster
	// +kubebuilder:validation:Enum=Cluster;Namespace
	Scope NetworkScope `json:"scope,omitempty"`

	// namespace is the target namespace for the UDN when scope is
	// Namespace. Ignored when scope is Cluster.
	//
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// cidr is the pod CIDR for this subnet. Must fall within one of
	// the VPC's cidrBlocks.
	//
	// +kubebuilder:validation:Required
	// +required
	CIDR CIDR `json:"cidr"`

	// topology is the network topology for this subnet.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Layer2;Layer3
	// +required
	Topology NetworkTopology `json:"topology"`

	// hostSubnet is the per-node prefix length (Layer3 only).
	// +optional
	HostSubnet int32 `json:"hostSubnet,omitempty"`

	// transport is the encapsulation mode for this subnet.
	// Defaults to Geneve if not specified.
	//
	// +optional
	// +kubebuilder:validation:Enum=Geneve;EVPN
	Transport *TransportMode `json:"transport,omitempty"`

	// connectivity defines the external connectivity class of this subnet.
	// This determines what the VPC controller provisions beyond the C(UDN):
	//
	// - Public: externally reachable. Controller creates RouteAdvertisements
	//   (BGP) to export subnet routes to the physical network.
	// - Private: no direct external reachability. Controller creates EgressIP
	//   for outbound NAT so pods can reach external destinations.
	// - Isolated: no routes outside the VPC. No RouteAdvertisements, no
	//   EgressIP. Traffic is confined to intra-VPC routing only.
	// - VPNOnly: traffic exits the VPC exclusively through a VPN connection.
	//   No internet-facing routes. Controller configures IPsec or similar
	//   north-south encryption for this subnet's traffic.
	//
	// Defaults to Private if not specified.
	//
	// +optional
	// +kubebuilder:default=Private
	// +kubebuilder:validation:Enum=Public;Private;Isolated;VPNOnly
	Connectivity SubnetConnectivity `json:"connectivity,omitempty"`

	// namespaceSelector determines which namespaces are attached
	// to this subnet. Used when scope is Cluster (CUDN).
	//
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// VPCStatus defines the observed state of a VPC.
type VPCStatus struct {
	// subnets reports the status of each subnet in the VPC.
	// +optional
	Subnets []VPCSubnetStatus `json:"subnets,omitempty"`

	// conditions reports the status of VPC operations.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type VPCSubnetStatus struct {
	// name is the subnet name.
	Name string `json:"name"`

	// networkName is the name of the UDN or CUDN created for this subnet.
	NetworkName string `json:"networkName"`

	// scope is Cluster (CUDN) or Namespace (UDN).
	Scope string `json:"scope"`

	// cidr is the pod CIDR of the subnet.
	CIDR string `json:"cidr"`

	// topology is the subnet's network topology.
	Topology string `json:"topology"`

	// ready indicates whether the underlying UDN/CUDN is ready.
	Ready bool `json:"ready"`
}
```

Example:

```yaml
apiVersion: k8s.ovn.org/v1
kind: VPC
metadata:
  name: production
  labels:
    peer-group: prod-staging
spec:
  cidrBlocks:
  - 10.0.0.0/16
  subnets:
  - name: web
    cidr: 10.0.1.0/24
    topology: Layer3
    hostSubnet: 26
    connectivity: Public
    namespaceSelector:
      matchLabels:
        subnet: prod-web
  - name: app
    cidr: 10.0.10.0/24
    topology: Layer3
    hostSubnet: 26
    connectivity: Private
    namespaceSelector:
      matchLabels:
        subnet: prod-app
  - name: db
    cidr: 10.0.20.0/24
    topology: Layer3
    hostSubnet: 26
    connectivity: Isolated
    namespaceSelector:
      matchLabels:
        subnet: prod-db
  - name: mgmt
    cidr: 10.0.30.0/24
    topology: Layer3
    hostSubnet: 26
    connectivity: VPNOnly
    namespaceSelector:
      matchLabels:
        subnet: prod-mgmt
  - name: vm-network
    cidr: 10.0.100.0/24
    topology: Layer2
    transport: EVPN
    connectivity: Private
    namespaceSelector:
      matchLabels:
        subnet: prod-vm
```

The VPC controller creates five C(UDN)s from this: `production-web`,
`production-app`, `production-db`, `production-mgmt`, and
`production-vm-network`. Each gets the appropriate topology, transport, and
namespaceSelector. The `connectivity` field drives what additional resources
the controller creates:

- **production-web** (Public): RouteAdvertisements for BGP export
- **production-app** (Private): EgressIP for outbound NAT
- **production-db** (Isolated): no external routing at all
- **production-mgmt** (VPNOnly): IPsec north-south encryption
- **production-vm-network** (Private): EgressIP for outbound NAT

Automatic intra-VPC routing is established between all five subnets, and all
CIDRs are validated against 10.0.0.0/16.

#### ClusterNetworkConnect (existing, extended)

CNC is extended with a `vpcSelector` field to enable VPC-to-VPC peering.
The existing `networkSelectors`, `connectSubnets`, and `connectivity` fields
are unchanged.

```yaml
apiVersion: k8s.ovn.org/v1
kind: ClusterNetworkConnect
metadata:
  name: prod-staging-peering
spec:
  # NEW: select VPCs to peer
  vpcSelector:
    matchLabels:
      peer-group: prod-staging
  connectSubnets:
  - cidr: 192.168.0.0/16
    networkPrefix: 24
  connectivity:
  - PodNetwork
  - ClusterIPServiceNetwork
```

The VPCs carry the matching label:

```yaml
apiVersion: k8s.ovn.org/v1
kind: VPC
metadata:
  name: production
  labels:
    peer-group: prod-staging
spec:
  cidrBlocks: ["10.0.0.0/16"]
  networkSelector:
    matchLabels:
      vpc: production
---
apiVersion: k8s.ovn.org/v1
kind: VPC
metadata:
  name: staging
  labels:
    peer-group: prod-staging
spec:
  cidrBlocks: ["10.1.0.0/16"]
  networkSelector:
    matchLabels:
      vpc: staging
```

CNC's `connectSubnets` provides the transit CIDR for inter-VPC routing. This
is the purpose `connectSubnets` was designed for: providing the interconnect
plumbing between otherwise isolated networks.

#### RouteTable CRD (new)

**TBD.** The common default-route patterns (route to IGW, route to NAT GW)
are handled by the `connectivity` field — the VPC controller creates
RouteAdvertisements or EgressIP as needed, with no explicit route entries.
The routing infrastructure (per-C(UDN) VRF in LGW, Gateway Router in SGW)
already exists. What's missing is a user-facing API to inject **custom static
routes** into those routers (e.g. routes to on-prem networks, peered VPC
gateways, traffic engineering overrides). The scope and API of this CRD need
further design work. It isn't clear if we really need this CRD or not.

Fill in the operational impact of these API Extensions in the "Operational Aspects
of API Extensions" section.

### Topology Considerations

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

#### OpenShift Kubernetes Engine

How does this proposal affect OpenShift Kubernetes Engine (OKE)?  Does it depend
on features that are excluded from the OKE product offering?  See [the
comparison of OKE and OCP in the product documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/overview/oke-about#about_oke_similarities_and_differences).

### Implementation Details/Notes/Constraints

This section will break down the implementation of
every single VPC resource/construct:

#### Subnets (C(UDN)s)

In AWS, a subnet is a range of IP addresses within a VPC, pinned to a single
Availability Zone. Key properties of the
[AWS::EC2::Subnet](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ec2-subnet.html)
resource:

1. **VpcId** — the VPC the subnet belongs to.
2. **CidrBlock** — the IPv4 CIDR for the subnet. Must fall within the VPC's
   CIDR. Immutable (change requires replacement).
3. **AvailabilityZone** — the AZ the subnet resides in. Immutable.
4. **MapPublicIpOnLaunch** — whether instances get a public IP automatically.

A subnet is not inherently "public" or "private" — that behaviour comes from
its route table (see Internet Gateway and NAT Gateway sections below). The
subnet itself is just a CIDR range in an AZ.

**OVN-Kubernetes mapping: C(UDN)**

In OVN-Kubernetes, a subnet maps to a ClusterUserDefinedNetwork (CUDN) or
UserDefinedNetwork (UDN), collectively referred to as C(UDN):

| AWS Subnet property | C(UDN) equivalent |
|---|---|
| VpcId (parent VPC) | VPC controller creates the C(UDN) — ownership tracked via `vpc.k8s.ovn.org/name` label |
| CidrBlock | `spec.network.layer3.subnets[].cidr` (L3) or `spec.network.layer2.subnets[]` (L2) |
| AvailabilityZone | See Open Questions — `topology.kubernetes.io/zone` node labels on bare metal |
| MapPublicIpOnLaunch | Not applicable — determined by `connectivity` field on the VPC subnet |
| Immutable CIDR | Same — C(UDN) CIDR is immutable once created |
| Route table association | Determined by `connectivity` field; controller creates ancillary resources |

**Key differences from AWS:**

- **Topology choice.** AWS subnets are always L3. C(UDN)s support both Layer2
  and Layer3 topologies within the same VPC.
- **Transport choice.** C(UDN)s can use Geneve (overlay) or EVPN. AWS manages
  the underlay transparently.
- **Scope.** A C(UDN) can be cluster-scoped (CUDN, spans namespaces via
  `namespaceSelector`) or namespace-scoped (UDN). AWS subnets are always
  VPC-scoped.
- **Connectivity is declarative.** In AWS, a subnet's external connectivity
  is determined by what you put in its route table (IGW route = public, NAT GW
  route = private, nothing = isolated). In the VPC CRD, this is captured by the
  `connectivity` field, and the VPC controller creates the right resources:

| `connectivity` | External reachability | Controller creates |
|---|---|---|
| **Public** | Externally routable — subnet routes advertised via BGP | RouteAdvertisements (see Internet Gateway section) |
| **Private** | Outbound only — pods can reach external destinations | EgressIP (see NAT Gateway section) |
| **Isolated** | None — traffic confined to intra-VPC routing | (nothing) |
| **VPNOnly** | VPN only — traffic exits via encrypted tunnel | IPsec north-south encryption |

All subnets regardless of connectivity class get automatic **intra-VPC routing**
between each other.

Using the `production` VPC from the API Extensions example above, the VPC
controller creates the following resources. All C(UDN)s are named
`<vpc-name>-<subnet-name>` and labeled with `vpc.k8s.ovn.org/name: production`
for ownership tracking.

```yaml
# ── 1. Public subnet: production-web ──────────────────────────
# C(UDN): L3 Geneve, externally routable via RouteAdvertisements
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: production-web
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: web
    vpc.k8s.ovn.org/connectivity: Public
spec:
  namespaceSelector:
    matchLabels:
      subnet: prod-web
  network:
    topology: Layer3
    layer3:
      role: Primary
      subnets:
      - cidr: 10.0.1.0/24
        hostSubnet: 26
---
# RouteAdvertisements: export pod routes to the physical network
apiVersion: k8s.ovn.org/v1
kind: RouteAdvertisements
metadata:
  name: production-web
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: web
spec:
  advertisements:
  - advertisementType: PodNetwork
  networkSelector:
    matchLabels:
      vpc.k8s.ovn.org/name: production
      vpc.k8s.ovn.org/subnet: web
  nodeSelector:
    matchLabels: {}
---
# ── 2. Private subnet: production-app ─────────────────────────
# C(UDN): L3 Geneve (default), outbound NAT via EgressIP
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: production-app
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: app
    vpc.k8s.ovn.org/connectivity: Private
spec:
  namespaceSelector:
    matchLabels:
      subnet: prod-app
  network:
    topology: Layer3
    layer3:
      role: Primary
      subnets:
      - cidr: 10.0.10.0/24
        hostSubnet: 26
---
# EgressIP: outbound SNAT for pods in the private subnet
apiVersion: k8s.ovn.org/v1
kind: EgressIP
metadata:
  name: production-app
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: app
spec:
  egressIPs:
  - 192.168.1.100
  namespaceSelector:
    matchLabels:
      subnet: prod-app
---
# ── 3. Isolated subnet: production-db ─────────────────────────
# C(UDN): L3 Geneve, no external routes whatsoever
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: production-db
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: db
    vpc.k8s.ovn.org/connectivity: Isolated
spec:
  namespaceSelector:
    matchLabels:
      subnet: prod-db
  network:
    topology: Layer3
    layer3:
      role: Primary
      subnets:
      - cidr: 10.0.20.0/24
        hostSubnet: 26
# No RouteAdvertisements, no EgressIP.
# Only intra-VPC routes are programmed.
---
# ── 4. VPNOnly subnet: production-mgmt ────────────────────────
# C(UDN): L3 Geneve, traffic exits exclusively via IPsec tunnel
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: production-mgmt
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: mgmt
    vpc.k8s.ovn.org/connectivity: VPNOnly
spec:
  namespaceSelector:
    matchLabels:
      subnet: prod-mgmt
  network:
    topology: Layer3
    layer3:
      role: Primary
      subnets:
      - cidr: 10.0.30.0/24
        hostSubnet: 26
# VPC controller configures IPsec north-south encryption for
# this subnet. The exact IPsec CRD / configuration is TBD but
# the controller ensures that all egress from this subnet
# traverses an encrypted tunnel to a configured VPN endpoint.
---
# ── 5. Private L2 subnet: production-vm-network ───────────────
# C(UDN): L2 EVPN, outbound NAT via EgressIP
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: production-vm-network
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: vm-network
    vpc.k8s.ovn.org/connectivity: Private
spec:
  namespaceSelector:
    matchLabels:
      subnet: prod-vm
  network:
    topology: Layer2
    layer2:
      role: Primary
      subnets:
      - "10.0.100.0/24"
    transport: EVPN
---
# EgressIP: outbound SNAT for VMs in the L2 subnet
apiVersion: k8s.ovn.org/v1
kind: EgressIP
metadata:
  name: production-vm-network
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: vm-network
spec:
  egressIPs:
  - 192.168.1.101
  namespaceSelector:
    matchLabels:
      subnet: prod-vm
```

In addition to the per-subnet resources above, the VPC controller programs
**intra-VPC routes** between all five subnets so that:
- `10.0.1.0/24` (web) ↔ `10.0.10.0/24` (app) ↔ `10.0.20.0/24` (db) ↔ `10.0.30.0/24` (mgmt) ↔ `10.0.100.0/24` (vm-network)

are all reachable from each other within the VPC, regardless of their
connectivity class or topology. The `connectivity` field only governs external
(north-south) reachability; east-west traffic within the VPC is always permitted.

#### VPC Controller Responsibilities

The VPC controller runs as a separate daemonset deployed by CNO. Its source
lives in a new repository under the ovn-kubernetes organization. It watches
VPC resources and performs:

1. **CIDR governance**: The controller validates that each subnet's CIDR falls
   within the VPC's `cidrBlocks`. If validation fails, the controller reports
   a condition on the VPC status and does not create the C(UDN) for the
   offending subnet.

2. **C(UDN) lifecycle**: For each subnet in the VPC spec, the controller creates
   the corresponding UDN or CUDN (based on the subnet's `scope` field) and
   manages its lifecycle -- creating, updating, and deleting C(UDN)s as subnets
   are added or removed from the VPC.

3. **Connectivity provisioning**: Based on each subnet's `connectivity` class,
   the controller creates the appropriate ancillary resources:
   - `Public`: RouteAdvertisements for BGP export to the physical network.
   - `Private`: EgressIP for outbound SNAT.
   - `Isolated`: no external routes (controller ensures no RouteAdvertisements
     or EgressIP exist for this subnet).
   - `VPNOnly`: IPsec north-south encryption configuration.

4. **Intra-VPC routing**: For all subnets in the VPC, the controller sets up
   bidirectional routes between their OVN logical routers. This is the
   equivalent of the implicit "local" route in AWS VPCs, where all subnets
   in the same VPC can reach each other without any explicit configuration.

5. **Status aggregation**: The controller reports which subnets are in the VPC,
   their C(UDN) names, CIDRs, topologies, and readiness.

6. **Secondary CIDR validation**: When `cidrBlocks` is expanded (secondary CIDRs
   appended), the controller re-evaluates any previously rejected subnets and
   accepts those that now fall within the updated address space. Conversely,
   a CIDR block cannot be removed while any subnet references addresses within it.

#### Internet Gateway (Public Subnets)

In AWS, a public subnet has internet access because of three cooperating
resources:

1. **Internet Gateway (IGW)** — attached to the VPC (one per VPC). Provides
   bidirectional internet connectivity.
2. **Route** — an entry in the *public* subnet's route table:
   `0.0.0.0/0 → igw-id`.
3. **Public IP** — each instance in the public subnet gets a public IP
   (auto-assigned or an Elastic IP) that the IGW maps 1:1 to the private IP.

```
  internet ──► IGW ──► public pod (1:1 NAT to public IP)
  public pod ──► route table (0.0.0.0/0 → IGW) ──► IGW ──► internet
```

The IGW is stateless and bidirectional — external hosts can initiate
connections to the pod's public IP, and pods can initiate connections to
external hosts. There is no SNAT; the pod's public IP is its identity on the
internet.

**OVN-Kubernetes mapping: RouteAdvertisements**

In OVN-Kubernetes there is no separate "gateway" appliance. Instead,
RouteAdvertisements export the subnet's pod routes via BGP to the physical
network, making pods directly routable from outside the cluster:

| AWS Resource | OVN-K equivalent |
|---|---|
| Internet Gateway (bidirectional internet) | RouteAdvertisements — BGP announces pod CIDRs to the physical fabric |
| Public IP (1:1 NAT) | Pod IP is the routable IP — no NAT needed when routes are advertised |
| Route `0.0.0.0/0 → IGW` | Physical network has return routes to pod CIDRs via BGP |
| IGW is VPC-scoped (one per VPC) | RouteAdvertisements is per-subnet (one per public C(UDN)) |

For a VPC public subnet with `connectivity: Public`, the VPC controller
creates:

```yaml
apiVersion: k8s.ovn.org/v1
kind: RouteAdvertisements
metadata:
  name: production-web
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: web
spec:
  advertisements:
  - advertisementType: PodNetwork
  networkSelector:
    matchLabels:
      vpc.k8s.ovn.org/name: production
      vpc.k8s.ovn.org/subnet: web
  nodeSelector:
    matchLabels: {}
```

Once applied, the physical network learns routes to `10.0.1.0/24` (the
public subnet's CIDR) via BGP peering with the cluster nodes. External hosts
can reach pods directly by their pod IPs, and pods can reach external hosts —
bidirectional, just like an AWS IGW.

**Differences from AWS:**

- **No separate gateway resource.** There is no "IGW appliance" to create
  and attach. RouteAdvertisements configures the cluster's BGP speakers to
  announce pod routes — the physical fabric *is* the gateway.
- **No public IP allocation.** In AWS each instance needs a public IP that
  the IGW maps 1:1. With BGP route advertisement the pod IP itself is
  routable on the physical network — no NAT, no public IP allocation.
- **Per-subnet, not per-VPC.** In AWS one IGW covers the whole VPC. In
  OVN-K, RouteAdvertisements is created per public subnet, giving
  fine-grained control over which subnets are externally reachable.
- **Depends on physical network.** BGP peering must be configured between
  the cluster nodes and the physical fabric (ToR switches). This is an
  infrastructure prerequisite that has no AWS equivalent (AWS manages the
  physical network).

#### NAT Gateway (Private Subnets)

In AWS, a private subnet reaches the internet through three cooperating
resources:

1. **Elastic IP (EIP)** — a static public IP address.
2. **NAT Gateway** — placed in a *public* subnet, associated with the EIP.
   Performs SNAT for outbound traffic.
3. **Route** — an entry in the *private* subnet's route table:
   `0.0.0.0/0 → nat-gateway-id`.

```
  private pod ──► private route table ──► NAT GW (in public subnet) ──► IGW ──► internet
                  0.0.0.0/0 → NAT GW     SNAT to EIP
```

The NAT Gateway itself has AZ affinity — it is placed in a specific
Availability Zone's public subnet, and traffic from private instances in that
AZ routes to the local NAT Gateway.

**OVN-Kubernetes mapping: EgressIP**

In OVN-Kubernetes, the EgressIP resource collapses all three AWS resources
into one:

| AWS Resource | EgressIP equivalent |
|---|---|
| Elastic IP (static public IP) | `spec.egressIPs: ["192.168.1.100"]` |
| NAT Gateway (SNAT engine) | OVN performs SNAT on the node hosting the EgressIP |
| NAT Gateway AZ placement | Node labels (`k8s.ovn.org/egress-assignable`) on nodes in a specific rack / failure domain (e.g. `topology.kubernetes.io/zone=rack-a`) |
| Route `0.0.0.0/0 → NAT GW` | Implicit — OVN-K automatically routes egress traffic through the EgressIP node |
| `namespaceSelector` (which private subnet uses this NAT) | `spec.namespaceSelector` matching the private subnet's namespaces |

For a VPC private subnet with `connectivity: Private`, the VPC controller
creates:

```yaml
apiVersion: k8s.ovn.org/v1
kind: EgressIP
metadata:
  name: production-app
  labels:
    vpc.k8s.ovn.org/name: production
    vpc.k8s.ovn.org/subnet: app
spec:
  egressIPs:
  - 192.168.1.100
  namespaceSelector:
    matchLabels:
      subnet: prod-app
```

The EgressIP is assigned to a node labeled `k8s.ovn.org/egress-assignable`.
If the subnet is zone-pinned (see Open Questions), the VPC controller can
constrain the EgressIP to nodes in the same failure domain, mirroring how
AWS places a NAT Gateway in a specific AZ's public subnet.

**Differences from AWS:**

- **No separate gateway resource.** EgressIP is both the EIP and the NAT
  engine. There is no intermediate hop through a "public subnet" — SNAT
  happens directly on the node.
- **No explicit route.** The `0.0.0.0/0` default route is implicit in
  OVN-K's EgressIP implementation rather than being a discrete route table
  entry. If explicit routing is desired, the RouteTable CRD can be used.
- **Simpler day-2.** Changing the egress IP or moving it to a different node
  is a single edit to the EgressIP resource; in AWS you would recreate the
  NAT Gateway and update the route.

#### Route Table (Custom Routes)

In AWS, a [route table](https://docs.aws.amazon.com/vpc/latest/userguide/VPC_Route_Tables.html)
is the central mechanism that determines subnet connectivity. The default route
(`0.0.0.0/0 → igw` or `0.0.0.0/0 → nat-gw`) is what makes a subnet public or
private.

In our model, this is **not necessary**. The `connectivity` field on the VPC
subnet replaces the default-route patterns that dominate AWS route tables:

- `connectivity: Public` → RouteAdvertisements (no route to an IGW — BGP
  advertises pod routes directly)
- `connectivity: Private` → EgressIP (no route to a NAT GW — OVN performs
  SNAT internally)
- `connectivity: Isolated` → nothing (traffic stays in the VPC by default)

The routing infrastructure already exists — every C(UDN) gets an OVN logical
router (Linux VRF in LGW mode, Gateway Router in SGW mode). What doesn't
exist today is a user-facing API to program **custom route entries** into
those routers for advanced use cases:

- Static routes to on-prem networks (e.g. `172.16.0.0/12` via next-hop)
- Routes to peered VPC gateways
- Traffic engineering overrides

**TODO**: Determine the scope and API for a RouteTable CRD. Since the common
default-route patterns are already handled by `connectivity`, the RouteTable
CRD would be narrowly focused on custom/advanced static routes injected into
the existing per-C(UDN) VRF (LGW) or GR (SGW). This needs further design work.

#### Day-2: Expanding the VPC Address Space

Like AWS secondary VPC CIDRs, the VPC's `cidrBlocks` list is mutable and
can be appended to after creation. This enables the VPC to grow without
recreating it.

```yaml
# Day 0: VPC created with initial /24
apiVersion: k8s.ovn.org/v1
kind: VPC
metadata:
  name: production
spec:
  cidrBlocks:
  - 10.0.0.0/24
  networkSelector:
    matchLabels:
      vpc: production
---
# C(UDN) created within that range
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: prod-web
  labels:
    vpc: production
spec:
  namespaceSelector:
    matchLabels:
      subnet: prod-web
  network:
    topology: Layer3
    layer3:
      role: Primary
      subnets:
      - cidr: 10.0.0.0/26
        hostSubnet: 28
```

```yaml
# Day 2: address space exhausted, add secondary CIDR
apiVersion: k8s.ovn.org/v1
kind: VPC
metadata:
  name: production
spec:
  cidrBlocks:
  - 10.0.0.0/24        # original
  - 10.0.1.0/24        # secondary, appended
  networkSelector:
    matchLabels:
      vpc: production
---
# New subnet in the secondary range now passes validation
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: prod-backend
  labels:
    vpc: production
spec:
  namespaceSelector:
    matchLabels:
      subnet: prod-backend
  network:
    topology: Layer3
    layer3:
      role: Primary
      subnets:
      - cidr: 10.0.1.0/26
        hostSubnet: 28
```

Mutability rules:
- **Append**: New CIDR blocks can be added at any time.
- **Remove**: A CIDR block can only be removed if no subnet has pod CIDRs
  within it. The controller enforces this via a validating webhook.
- **Modify**: Existing CIDR entries are immutable. To resize, remove the old
  (if unused) and add a new one.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?

## Alternatives (Not Implemented)

### API Design

#### Alternative A: C(UDN) as VPC (no new CRD)

In this model, a single C(UDN) would represent an entire VPC. The C(UDN) spec
would be extended with a `vpcSubnets` section allowing multiple named subnets
with different CIDR blocks and connectivity properties (public, private,
isolated) within a single C(UDN).

```yaml
kind: ClusterUserDefinedNetwork
metadata:
  name: production-vpc
spec:
  network:
    topology: Layer3
    layer3:
      role: Primary
      subnets:
      - cidr: 10.0.0.0/16
        hostSubnet: 24
  vpcSubnets:
  - name: public
    cidrBlocks: ["10.0.1.0/24"]
    connectivity: Public
  - name: private
    cidrBlocks: ["10.0.10.0/24"]
    connectivity: Private
```

**Why this was not selected:**

1. **C(UDN) has a single topology.** A C(UDN) is either Layer2 or Layer3, not both.
   A VPC frequently needs subnets of different topologies (e.g. a routed L3 subnet
   for application workloads alongside a flat L2 subnet for VM migration). Modeling
   the VPC as a single C(UDN) would make heterogeneous subnet topologies impossible.
   Two C(UDN)s of different topologies in the same VPC would become "VPC-L3 peering
   with VPC-L2" which is semantically wrong -- they are subnets in the same VPC,
   not separate VPCs being peered.

2. **C(UDN) has a single transport mode.** A C(UDN) uses one transport: Geneve
   or EVPN. A VPC may contain subnets where some use Geneve overlay (private,
   internal) and others use EVPN (externally reachable L2 networks). A single
   C(UDN) cannot express this mix.

3. **CNC becomes awkward for VPC peering.** If C(UDN) is the VPC, then CNC
   (ClusterNetworkConnect) would peer VPCs. But CNC was designed to connect
   individual networks, not VPC-level constructs. CNC's `networkSelectors` select
   C(UDN)s, so you would be selecting C(UDN)s-as-VPCs and connecting them with CNC,
   while simultaneously needing CNC to connect C(UDN)s-as-subnets within a VPC.
   CNC would serve double duty with ambiguous semantics.

4. **C(UDN) spec mutability is limited.** The C(UDN) spec is currently immutable
   (`+kubebuilder:validation:XValidation:rule="self == oldSelf"`). There is an
   upstream OKEP to allow appending new CIDRs to an existing C(UDN), which would
   partially address VPC CIDR expansion. However, even with appendable CIDRs,
   the C(UDN) spec would not support the full range of VPC day-2 mutations: adding
   subnets with different topologies or transport modes, changing egress configuration,
   or adding peering relationships. AWS VPCs support extensive post-creation operations
   (`create-subnet`, `attach-internet-gateway`, `create-vpc-peering-connection`,
   `associate-route-table`). With a separate VPC CRD, the VPC spec is fully mutable
   while each underlying C(UDN) (subnet) remains individually immutable (or append-only
   for CIDRs) -- the VPC controller creates new C(UDN)s when subnets are added and
   deletes C(UDN)s when subnets are removed.

#### Alternative B: CNC as VPC (no new CRD)

In this model, ClusterNetworkConnect would serve as the VPC. A CNC already
groups multiple C(UDN)s via `networkSelectors` and auto-routes between them via
`connectSubnets`. The VPC would emerge from creating a CNC that selects all
subnets (C(UDN)s) in the same logical group.

```yaml
kind: ClusterNetworkConnect
metadata:
  name: production-vpc
spec:
  networkSelectors:
  - networkSelectionType: ClusterUserDefinedNetworks
    clusterUserDefinedNetworkSelector:
      networkSelector:
        matchLabels:
          vpc: production
  connectSubnets:
  - cidr: 192.168.0.0/16
    networkPrefix: 24
  connectivity:
  - PodNetwork
  - ClusterIPServiceNetwork
```

**Why this was not selected:**

1. **CNC cannot be both the VPC and VPC peering.** If CNC is the VPC, then what
   peers two VPCs? Another CNC? This creates two classes of CNC with different
   semantics (CNC-as-VPC vs CNC-as-peering) using the same CRD. There is no way
   to distinguish between "this CNC groups subnets into a VPC" and "this CNC
   connects two VPCs" in the API.

2. **`connectSubnets` is transit plumbing, not a VPC CIDR.** CNC's `connectSubnets`
   defines a dedicated transit CIDR for the interconnect fabric between selected
   networks. It is not the VPC's address space. A VPC needs a `cidrBlocks` field
   for CIDR governance (validating that member subnet CIDRs fall within the VPC's
   range). CNC has no such concept and adding one would conflate its purpose.

3. **Intra-VPC routing should be implicit.** In AWS, all subnets in a VPC route to
   each other automatically via the immutable "local" route. CNC provides routing
   via `connectSubnets`, which requires the admin to allocate a dedicated transit
   CIDR, choose a `networkPrefix`, and ensure it doesn't overlap with pod subnets,
   service CIDRs, join subnets, masquerade subnets, and node subnets. This is
   appropriate for explicit peering between distinct networks, but not for the
   implicit "subnets in the same VPC can always talk" semantics.

4. **VPC-level configuration has no home.** VPC-level concerns such as CIDR governance,
   DNS configuration, default egress policy, and status aggregation of member subnets
   don't belong on a resource whose purpose is network-to-network connectivity. Adding
   these to CNC would grow it beyond its design intent.

#### Alternative C: VPC as a field on C(UDN) (no new CRD)

In this model, the VPC is not a separate resource but a stanza on the C(UDN) spec.
C(UDN)s with the same `vpc.name` value are grouped into a VPC. The controller
watches C(UDN)s and creates intra-VPC routing between those sharing the same name.

```yaml
kind: ClusterUserDefinedNetwork
metadata:
  name: prod-public
spec:
  vpc:
    name: production
    cidrBlock: 10.0.0.0/16
  network:
    topology: Layer3
    ...
```

**Why this was not selected:**

1. **Duplicated VPC-level configuration.** Every C(UDN) in the VPC must declare the
   same `vpc.cidrBlock`. If they disagree, the controller must resolve the conflict.
   There is no single source of truth for VPC-level settings.

2. **No resource for VPC-level operations.** VPC peering (via CNC with `vpcSelector`)
   needs a VPC resource to select. With VPC as a field on C(UDN), there is nothing for
   the CNC `vpcSelector` to reference -- you can only reference C(UDN)s. VPC peering
   would degenerate back to C(UDN) peering, losing the abstraction.

3. **Status reporting is scattered.** VPC health, CIDR utilization, and member subnet
   status would be reported across multiple C(UDN) status fields rather than in a single
   place. Debugging and monitoring become harder.

4. **Future extensibility.** If VPC-level features accumulate (DNS configuration,
   default security policies, IPAM pools, quota), they would all need to be crammed
   into the C(UDN)'s `vpc` stanza. A dedicated thin CRD is the natural extraction point
   and avoids bloating the C(UDN) API.

### Approach

### Alternative A: VPC CRD Without a Translation Controller

In this model, the VPC CRD exists as a passive grouping/governance resource,
but there is no dedicated VPC controller that translates VPC intent into the
underlying OVN-Kubernetes resources. Users create the VPC for CIDR validation
and status, but must still individually create every C(UDN), CNC,
RouteAdvertisements, EgressIP, and RouteTable by hand and wire them together
via labels.

```
User creates:
  1. VPC (production)           — cidrBlocks: [10.0.0.0/16], networkSelector
  2. C(UDN) (prod-public)       — label vpc=production, cidr 10.0.1.0/24
  3. C(UDN) (prod-private)      — label vpc=production, cidr 10.0.10.0/24
  4. C(UDN) (prod-vm-network)   — label vpc=production, cidr 10.0.100.0/24
  5. CNC  (production-routing)  — networkSelectors matching vpc=production
  6. RouteAdvertisements         — targeting prod-public
  7. EgressIP                    — for prod-private NAT
  8. RouteTable                  — custom routes for the VPC
```

The VPC validates CIDRs and aggregates status, but the user is responsible for
creating and maintaining every other resource.

**Why this was not selected:**

1. **Poor user experience.** Users should not need to learn the internals of
   C(UDN)s, CNCs, RouteAdvertisements, EgressIPs, and RouteTables just to create
   a VPC. Requiring 6-8 individual resource creations with correct cross-resource
   label wiring for what is conceptually a single operation ("create me a VPC with
   these subnets") is not an acceptable UX.

2. **Error-prone label wiring.** Even with the VPC providing CIDR governance,
   correct operation still depends on labels matching across multiple resources:
   C(UDN) labels must match VPC `networkSelector`, CNC `networkSelectors` must
   match the right C(UDN)s, RouteAdvertisements must target the correct networks.
   A single label typo silently breaks routing with no feedback.

3. **Intra-VPC routing is not automatic.** Without a controller, the user must
   explicitly create a CNC (with `connectSubnets` and transit CIDR allocation)
   just to get subnets within the same VPC to talk to each other. In AWS, this
   is the implicit "local" route -- it exists the moment a subnet joins a VPC.
   Requiring the user to manually provision transit plumbing for what should be
   automatic intra-VPC connectivity defeats the purpose of the VPC abstraction.

4. **Day-2 is a manual checklist.** Adding a subnet requires the user to:
   create a C(UDN) with correct labels and CIDR, update the CNC if needed,
   optionally create RouteAdvertisements or EgressIP for the new subnet, and
   verify everything is wired. There is no controller to detect that a new
   subnet joined the VPC and automatically set up routing, or to flag
   misconfigurations.

5. **The VPC CRD becomes a bookkeeping artifact.** If the VPC only validates
   CIDRs and reports status but doesn't drive any behavior, users have little
   incentive to create one. The CRD exists but provides marginal value over just
   using a naming convention on C(UDN) labels.

The VPC controller (a separate daemonset deployed by CNO, sourced from
a new repository under the ovn-kubernetes organization) closes this gap by watching the VPC resource and automatically
creating and reconciling the underlying OVN-Kubernetes CRDs -- intra-VPC
routing, status aggregation, and integration with CNC for peering -- so that
the user experience mirrors the simplicity of cloud VPC operations. In
multi-cluster deployments, the operator also synchronizes this configuration
across clusters.

### Alternative B: Hub Pushes Rendered Resources Directly (multi-cluster)

In this model, the VPC controller runs **only on the hub**. The hub does IPAM,
renders the C(UDN)s, policies, routes, and pushes the fully rendered resources
directly to each spoke cluster via the spoke's Kubernetes API. Spokes have no
VPC controller -- just standard OVN-Kubernetes reacting to the resources the
hub created.

```
  Hub VPC Controller
    │
    ├─ API access ──► Spoke A (creates C(UDN)s, policies directly)
    └─ API access ──► Spoke B (creates C(UDN)s, policies directly)
```

**Pros:**
- Simpler spoke: no new component beyond existing OVN-Kubernetes.
- Single controller to debug -- all logic is on the hub.

**Why this was not selected:**

1. **Hub needs API access to every spoke.** The hub controller must hold
   kubeconfig/credentials for each spoke cluster. This is a security concern
   (credentials for N clusters stored in one place) and an operational burden
   (credential rotation, RBAC management across clusters).

2. **No self-healing on spokes.** If a C(UDN) or policy is accidentally deleted
   on a spoke, nothing recreates it until the hub's reconciliation loop
   notices and pushes again. If the hub is down, the spoke has no way to
   recover.

3. **Hub is a single point of failure.** If the hub goes down, no spoke can
   receive updates and drift cannot be corrected. In the selected model,
   spokes continue self-healing from their local copy of the VPC spec.

4. **Tight coupling.** The hub must understand spoke API versions, handle
   transient connectivity failures to each spoke, and manage remote resource
   lifecycle (create, update, delete). This makes the hub controller
   significantly more complex than one that only manages local resources and
   syncs a spec.

### Alternative C: Replicated Model (multi-cluster)

In this model, the VPC CRD exists on every cluster independently. Each cluster
has its own copy of the VPC resource, and a synchronization mechanism (e.g.
a distributed controller or GitOps pipeline) keeps the VPC specs in sync
across clusters.

```
  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
  │   Cluster A      │  │   Cluster B      │  │   Cluster C      │
  │                  │  │                  │  │                  │
  │  VPC: production │  │  VPC: production │  │  VPC: production │
  │  (full copy)     │  │  (full copy)     │  │  (full copy)     │
  │                  │  │                  │  │                  │
  │ VPC Controller   │  │ VPC Controller   │  │ VPC Controller   │
  │  (autonomous)    │  │  (autonomous)    │  │  (autonomous)    │
  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘
           │                     │                      │
           └─────────── sync ────┴──────────────────────┘
                    (GitOps / distributed)
```

**Pros:**
- No hub dependency -- every cluster is self-sufficient. If the hub goes down
  in a hub-spoke model, spokes cannot receive updates. In the replicated model,
  each cluster owns its VPC and can operate independently.
- Simpler operator: each instance only manages its local cluster.

**Why this was not selected:**

1. **No single source of truth.** The VPC exists as independent copies on each
   cluster. If two administrators modify the VPC on different clusters
   simultaneously, the specs diverge and the sync mechanism must resolve
   conflicts. In the hub-spoke model, the hub is the single source of truth
   and conflicts are impossible.

2. **IPAM coordination is harder.** Each cluster must allocate non-overlapping
   portions of the subnet CIDRs without a central authority. This requires a
   distributed consensus or external IPAM service, adding complexity. The
   hub-spoke model centralizes IPAM on the hub.

3. **Policy drift.** NetworkPolicies and AdminNetworkPolicies can drift between
   clusters if sync fails or lags. In the hub-spoke model, policies are
   always pushed from a single authoritative source.

4. **User confusion.** Users must decide which cluster to modify the VPC on
   (all of them? one and wait for sync?). The hub-spoke model provides a
   clear answer: always modify on the hub.

### Alternative D: Federated Model (multi-cluster)

In this model, the VPC does not live on any OpenShift cluster. Instead, it is
defined on an external federation control plane (e.g. Open Cluster Management
hub, Kubernetes Federation v2, or a dedicated VPC management API) and a
federation mechanism distributes the VPC to member clusters.

```
  ┌──────────────────────────────────┐
  │   External Federation Plane      │
  │   (OCM Hub / Fed v2 / Custom)    │
  │                                  │
  │   VPC: production                │
  │   FederatedPlacement: [A, B, C]  │
  └───────┬──────────┬───────┬───────┘
          │          │       │
          ▼          ▼       ▼
  ┌────────────┐ ┌────────────┐ ┌────────────┐
  │ Cluster A  │ │ Cluster B  │ │ Cluster C  │
  │ (receives  │ │ (receives  │ │ (receives  │
  │  VPC via   │ │  VPC via   │ │  VPC via   │
  │  federation│ │  federation│ │  federation│
  │  agent)    │ │  agent)    │ │  agent)    │
  └────────────┘ └────────────┘ └────────────┘
```

**Pros:**
- Clean separation of VPC management from cluster operations.
- Leverages existing multi-cluster management tooling (OCM, ArgoCD, etc.).
- The VPC API could be used by non-OpenShift clusters if the federation
  plane is generic.

**Why this was not selected:**

1. **External dependency.** Requires a federation control plane that may not
   exist in all deployments. The hub-spoke model uses a regular OpenShift
   cluster as the hub -- no additional infrastructure is required.

2. **Indirection adds latency and complexity.** VPC changes go through the
   federation plane, then to each cluster's agent, then to the local VPC
   operator. In the hub-spoke model, the hub operator pushes directly to
   spokes -- one fewer hop.

3. **Tight coupling to federation API.** The VPC resource must conform to the
   federation plane's resource distribution model (e.g. OCM ManifestWork,
   Argo ApplicationSet). If the federation tooling changes, the VPC controller
   must adapt. In the hub-spoke model, the VPC controller owns the sync
   mechanism end-to-end.

4. **Not all users have a federation plane.** Many deployments are simply
   "a few OpenShift clusters" without OCM or similar tooling. The hub-spoke
   model works out of the box -- designate any cluster as the hub.

## Open Questions [optional]

1. **Subnet pinning to failure domains / zones**: In AWS, a subnet must reside
   in a single Availability Zone. On bare metal, nodes can be labeled with
   failure domains (e.g. `topology.kubernetes.io/zone=rack-a`). Should the VPC
   subnet spec include a `nodeSelector` or zone field so that the resulting
   C(UDN) is pinned to specific nodes? This would enable AWS-like zone-aware
   subnet placement (e.g. `public-a` on rack-a, `public-b` on rack-b) for
   high availability. **TODO**: Determine whether this should be a VPC-level
   concern or left to the C(UDN) / scheduler.

2. **What does an Availability Zone mean on bare metal?** In AWS, AZs are
   first-class constructs that govern subnet placement, NAT Gateway affinity,
   and fault isolation. On bare metal there is no built-in AZ — the closest
   equivalent is node labels such as `topology.kubernetes.io/zone=rack-a`.
   If we adopt this model:
   - A "zone" is a set of nodes sharing a `topology.kubernetes.io/zone` label.
   - Subnet pinning (question 1) would use this label to bind a C(UDN) to a
     zone.
   - NAT Gateway (EgressIP) affinity would follow the same label — the VPC
     controller creates the EgressIP with node affinity matching the subnet's
     zone, mirroring how AWS places a NAT Gateway in a specific AZ's public
     subnet.
   - The VPC controller could validate that the zone label exists on at least
     one node before accepting a zone-pinned subnet.

   **TODO**: Decide whether to formalize a "VPC zone" concept backed by
   `topology.kubernetes.io/zone` labels, or leave zone mapping as an
   out-of-band convention. This affects subnet placement, EgressIP affinity,
   and future HA patterns (e.g. one NAT Gateway per zone).

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

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

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

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
