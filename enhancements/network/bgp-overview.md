---
title: bgp-overview
authors:
  - "@russellb"
reviewers:
  - "@danwinship"
  - "@squeed"
approvers:
  - TBD
creation-date: 2020-08-11
last-updated: 2020-08-11
status: informational
see-also:
  - https://github.com/openshift/enhancements/pull/356
  - https://github.com/openshift/enhancements/pull/394
---

# OpenShift and BGP

BGP (Border Gateway Protocol) is a dynamic routing protocol used to power a
number of important networking use cases, including routing in the global
internet.  For some helpful resources on learning more about BGP, see the
[BGP Resources](#bgp-resources) section of this document.

The purpose of this document is to serve as a high level overview of how BGP
relates to OpenShift.  We will review a number of use cases, discuss how they
could be supported, and link to existing related work.  Any future enhancements
related to BGP should be able to rely on this one to provide some broader
context and should continue to update this document as necessary.

## Use Cases

### L3 (Layer 3) Network Fabric

The use case here does not require any changes to OpenShift.  It is still
useful to call out as it is one use of BGP that is relevant to OpenShift,
particularly with on premise clusters.

It is possible to use BGP between the routers used to build the L3 network that
OpenShift Nodes are connected to.

### L3 Redundancy for Nodes

*First, an aside on terminology: This is referring to IP Routing, and
not the OpenShift feature called Routing that works at the HTTP layer.  There
are also OpenShift features that use the word "egress", but in this context, we
only mean to refer to traffic that is leaving the cluster and not related to
any other features that use that name.*

Some networks may have more than one router accessible from Nodes for
availability reasons.  In this case an administrator may want to use BGP down
to the Nodes themselves so they are aware of the multiple routing options and
potentially use ECMP (Equal Cost Multi Path) to distribute traffic
across each of the routers.

Another motivator for this configuration is to provide L3 redundancy instead of
using bonding to provide L2 redundancy.  A node would publish a route to its IP
to two different routers accessible by two different network interfaces. In
this way

This capability is not provided by OpenShift today.  It may be possible as a
custom configuration, but not with all network providers.  With OpenShift-SDN,
it should be possible to run your own BGP routing daemon on the host via a
`DaemonSet`.

OVN-Kubernetes uses a different host network configuration where egress
traffic is routed by Open vSwitch flows programmed by OVN.  It means that a
dynamic routing daemon must also feed those routes into OVN configuration, so
it's not as straight forward as just running an existing daemon on the host.

A future enhancement is needed to describe how we would provide this
functionality natively with OpenShift.

### Pod Network Control Plane

One function that must be implemented for the Pod network is the control plane
which keeps track of the physical location of each Pod IP address.  In other
words, "Pod A is on Node X".  BGP is one technology which can be used to
distribute this information among Nodes in a cluster.
[Calico](#calico-and-bgp) is one example solution that uses BGP for this
purpose.

There are other ways this control plane functionality can be implemented, such
as using the Kubernetes API (OpenShift-SDN) or by using some other custom
control plane technology (OVN-Kubernetes).

Using BGP for this function is *not* a prerequisite for using BGP to
satisfy other use cases.

### Pod Network Traffic Routing and Avoiding Encapsulation

Another way BGP could be used for the Pod network is to actually route traffic.
Take the example of Pod A talking to Pod B.  Based on the discussion in [Pod
Network Control Plane](#pod-network-control-plane), we may know that Pod A is
on Node X and that Pod B is on Node Y.  BGP can also help get that traffic from
Node X to Node Y.

If the cluster nodes peer with routers in the network infrastructure supporting
the OpenShift cluster, then Pod traffic can be routed through the network
without this use of any tunnel encapsulation.  This can provide performance
improvements to network throughput by avoiding the cost of tunnel
encapsulation.  Hardware offload can help with tunnel encapsulation overhead,
but the cost is still non-zero.  Hardware offload is also not available in all
environments, such as when running on a virtual machine based public cloud.

It's possible to partially avoid tunnel encapsulation without the use of BGP or
another routing protocol.  If a network solution understands the underlying
physical topology, it can skip tunnel encapsulation between nodes on the same
layer 2 segment and only do encapsulation when L3 routing is required.  For
example, [Calico can do
this](https://docs.projectcalico.org/networking/vxlan-ipip) when not able to
peer with the underlying network infrastructure via BGP.

### External Service Load Balancing

Kubernetes Services with `type=LoadBalancer` are typically used as an
abstraction in front of a cloud's load balancer service.  Many on premise
cluster environments do not have a load balancer service available, so these
Services may not be used.

One way to expose a Service is to set the `Service.spec.externalIPs` field.
Once this field has been set, if the IP address(es) are routed to one or more
Nodes in the cluster, traffic to the Service port(s) will be forwarded to the
appropriate Service Endpoints.  The major catch with using this interface is
that routing of these external IP addresses to the Nodes is left up as an
exercise to the cluster administrator.

Another way to implement `LoadBalancer` Services for these on premise
clusters is to use BGP speaker on Nodes to publish routes to Nodes where
`LoadBalancer` IP addresses may be reached.  [MetalLB][2] is one project that
implements this technique.  [OpenShift Enhancement #356][3] discusses the use
of MetalLB with OpenShift in more detail.

### Exposing Pods or Services Directly

Some users have expressed an interest in publishing routes for the Pod and
Service networks via BGP.

If we wanted to offer this, it would not be very difficult.  At least with
OpenShift-SDN and OVN-Kubernetes, Nodes are already set up to handle traffic
destined for Pod or Service cluster IP addresses if traffic were to arrive
destined for those addresses.

To enable this using BGP, a BGP speaker must run on each Node where we would
like to serve as a router into the cluster network.  Each Node can publish a
route for the Pod and Service network and upstream routers may use ECMP to
distribute traffic among the Nodes.

A future enhancement needs to be filed to pursue this integration in more
detail.

### IP Anycast

In both the [External Service Load Balancing](#external-service-load-balancing)
and the [Exposing Pods or Services
Directly](#exposing-pods-or-services-directly) sections, we discuss advertising
some IP addresses via BGP to make them accessible from outside of the cluster.
An extension would be to allow advertising the same IP address from more than
one cluster (IP Anycast).

This is particularly applicable to edge use cases where a client would be
routed to the instance of a particular service IP with the fewest hops
possible. This allows for regional affinity as the client is routed to the
logically closest Node. This also allows for horizontal scalability, as
additional clusters and Nodes could be created to spread client load across
multiple service instances. Mobile clients may be expected to roam between
endpoints as the number of hops changes with location, so this is suited to
either fully stateless services over UDP such as DNS or video streaming, or
semi-stateless services over TCP such as RESTful API endpoints. This technique
could be used to load balance pods or services directly, or to expose multiple
LoadBalancer instances.

### Virtual Network Interconnect and Hybrid Cloud

The Pod network is by default an isolated virtual network.  There is sometimes
a desire to interconnect multiple virtual networks together.  This could be
between multiple Kubernetes clusters, but it doesn't have to be the case.

BGP, specifically MP-BGP, is commonly used as a signaling protocol, together
with other data planes technologies like MPLS and VXLAN to interconnect virtual
network at L2 or L3.

BGP EVPN (Ethernet VPN, or Virtual Private Network) is a BGP technology that is
discussed in the context of this use case.  BGP EVPN can be used to advertise
MAC addresses and IP-MAC address bindings.  BGP EVPN is not a full solution to
this use case by itself, as it is a technology that can help implement part of
the control plane.  There are more details that must be explored, including
what the data plane looks like between the two clusters.

BGP and MPLS is heavily used in the Telco area, and by the Cloud providers to
interconnect their facilities to the Cloud, i.e. [AWS Direct
connect](https://d1.awsstatic.com/whitepapers/Networking/integrating-aws-with-multiprotocol-label-switching.pdf).

This is a significantly non-trivial effort.  Future enhancements are needed to
discuss this in much more detail.

## Common Design Considerations

This section summarizes the common design considerations for OpenShift in
support of BGP related features.
### Administrator Experience

Establishing BGP sessions is not a simple task, and requires a number of unavoidable manual steps and site-specific parameters. It also requires coordination between (what is likely) multiple departments or groups within a typical organization. Politically, the ability to speak BGP to a network grants significant privileges, since (by default) BGP sessions are trusted completely. A misbehaving BGP participant can easily cause massive outages -- even to networks far outside the OpenShift environment. Any design must take this in to account.

Technically, any solution must make it easy for administrators to configure the same BGP peering parameters as they would expect on a router. In addition to the required settings (e.g. AS, peer, password), administrators expect additional knobs that make BGP usable (e.g. communities, timing parameters, multihop, AS prepends). The current status of all peerings must be exposed, since administrators rely on this to debug their networks and trust that OpenShift is not misbehaving.

Documentation and training should reflect the complexity of BGP-based operations, and potential users should be made aware of any prerequisites and technical challenges. Everyone should be aware of the buy-in required for enabling BGP. 
### Avoid L2 (Layer 2) Domain Assumptions

We must avoid the assumption of Nodes residing on the same L2 domain as much as
possible.  Every OpenShift Node should only assume L3 connectivity to every
other Node.

One example that should be avoided is any use of keepalived, as all Nodes
running keepalived to manage a VIP (Virtual IP address) must reside on the same
L2 segment since ARP (IPv4) or NDP (IPv6) is used to announce the new location
of the VIP when it moves to a new Node.

### Avoid any Hard Requirements on BGP

Any use of BGP in OpenShift should remain optional.  We have some customers
that do not want to use BGP at all, either for some technical or policy reason.
We may explore using it to add optional features, but it should not be required
for any OpenShift clusters.

For example, we are considering [adding MetalLB][3] to OpenShift to support
Services of `type=LoadBalancer`.  While MetalLB can use BGP, it also has an
alternative `layer2` mode that can provide the same functionality, though with
a different set of limitations.  It's also possible to set up static routing
for the IP addresses you desire to be reachable on your cluster.

## BGP Resources

### General BGP Information

* https://blog.cdemi.io/beginners-guide-to-understanding-bgp/
* https://www.ciscopress.com/articles/article.asp?p=2756480
* https://en.wikipedia.org/wiki/Border_Gateway_Protocol

### Calico and BGP

Calico is network provider for Kubernetes (and OpenShift) that uses BGP.  If
you read about [its
architecture](https://docs.projectcalico.org/reference/architecture/overview),
you will see that some BGP components are central to the architecture.
Calico's page on [Why BGP](https://www.projectcalico.org/why-bgp/) also
provides some of their reasoning for using it.  This document doesn't intend to
explain Calico fully, but since it's so common for Calico to come up when
people think about Kubernetes and BGP that it's useful to talk about how it
maps to the use cases discussed here.

Calico uses BGP for the [Pod Network Control
Plane](#pod-network-control-plane).  In the Calico docs, they refer to this as
endpoint advertisement using BGP.  As discussed earlier in this doc, using BGP
for this use case is not necessarily a prerequisite for using it for other
network integration use cases.

Calico can also use BGP to avoid tunnel encapsulation completely, but is also
able to [selectively apply overlay
networking](https://docs.projectcalico.org/networking/vxlan-ipip) if peering
with the underlying network is not possible.

[2]: https://metallb.universe.tf/
[3]: https://github.com/openshift/enhancements/pull/356
