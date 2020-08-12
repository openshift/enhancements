---
title: bgp-overview
authors:
  - "@russellb"
reviewers:
  - TBD
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

### Dynamic Routing for Egress Traffic

Some networks may have more than one router accessible from Nodes for
availability reasons.  In this case an administrator may want to use BGP down
to the Nodes themselves so they are aware of the multiple routing options and
potentially use ECMP (Equal Cost Multi Path) to distribute traffic
across each of the routers.

This capability is not provided by OpenShift today.  It may be possible as a
custom configuration, but not with all network providers.  With OpenShift-SDN,
it should be possible to run your own BGP routing daemon on the host via a
`DaemonSet`.

OVN-Kuberenetes uses a different host network configuration where egress
traffic is routed by Open vSwitch flows programmed by OVN.  This mode is
referred to as "shared gateway mode".  It means that a dynamic routing daemon
must also feed those routes into OVN configuration, so it's not as straight
forward as just running an existing daemon on the host.

A future enhancement is needed to describe how we would provide this
functionality natively with OpenShift.

### Pod Network Control Plane

Many people become aware of BGP in the Kubernetes community via its use in
[Calico][1].  The primary way BGP is used in Calico is as a control plane for
the Pod network.  In other words, Calico uses BGP internally for Nodes to share
the locations of Pod IP addresses in the cluster.

You may read Calico's own documentation to understand more details about how
Calico uses BGP.

There are other ways this control plane functionality can be implemented, such
as using the Kubernetes API or by using some other custom control plane
technology.  Both OpenShift-SDN and OVN-Kubernetes use alternative methods to
implement their control planes and it is not feasible to change them to use
BGP for this purpose.

### External Service Load Balancing

Kubernetes Services with `type=LoadBalancer` are typically used as an
abstraction in front of a cloud's load balancer service.  Many on premise
cluster environments do not have a load balancer service available, so these
Services may not be used.

It is possible to implement `LoadBalancer` Services for these on premise
clusters other ways.  One way involves using a BGP speaker on Nodes to publish
routes to Nodes where `LoadBalancer` IP addresses may be reached.  [MetalLB][2]
is one project that implements this technique.  [OpenShift Enhancement #356][3]
discusses the use of MetalLB with OpenShift in more detail.

### Exposing Pods or Services Directly

Some users have expressed an interest in publishing routes for the Pod and
Service networks via BGP.  This bypasses all of the usual interfaces used for
getting traffic to applications in a cluster, so it's a bit of an awkward
capability to offer from an architectural perspective.

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

### Virtual Network Interconnect

The Pod network is by default an isolated virtual network.  There is sometimes
a desire to interconnect multiple virtual networks together.  This could be
between multiple Kubernetes clusters, but it doesn't have to be the case.

BGP EVPN (Ethernet VPN, or Virtual Private Network) is a BGP technology that is
discussed in the context of this use case.  BGP EVPN can be used to advertise
MAC addresses and IP-MAC address bindings.  BGP EVPN is not a full solution to
this use case by itself, as it is a technology that can help implement part of
the control plane.  There are more details that must be explored, including
what the data plane looks like between the two clusters.

This is a significantly non-trivial effort.  Future enhancements are needed to
discuss this in much more detail.

## Common Design Considerations

This section summarizes the common design considerations for OpenShift in
support of BGP related features.

### Avoid L2 (Layer 2) Domain Assumptions

We must avoid the assumption of Nodes residing on the same L2 domain as much as
possible.  Every OpenShift Node should only assume L3 connectivity to every
other Node.

One example that should be avoided is any use of keepalived, as all Nodes
running keepalived to manage a VIP (Virtual IP address) must reside on the same
L2 segment since ARP (IPv4) or NDP (IPv6) is used to announce the new location
of the VIP when it moves to a new Node.

## BGP Resources

* https://blog.cdemi.io/beginners-guide-to-understanding-bgp/
* https://www.ciscopress.com/articles/article.asp?p=2756480
* https://en.wikipedia.org/wiki/Border_Gateway_Protocol

[1]: https://projectcalico.org
[2]: https://metallb.universe.tf/
[3]: https://github.com/openshift/enhancements/pull/356
