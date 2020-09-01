---
title: on-prem-service-load-balancers
authors:
  - "@russellb"
reviewers:
  - @markmc
  - @smarterclayton
  - @derekwaynecarr
  - @squeed
  - @aojea
  - @celebdor
  - @abhinavdahiya
  - @yboaron
  - @cybertron
approvers:
  - @knobunc
  - @danwinship
  - @danehans
creation-date: 2020-05-14
last-updated: 2020-05-14
status: proposed
---

# Service Load Balancers for On Premise Infrastructure

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We do not currently support full automation for [Services of
type=LoadBalancer](https://kubernetes.io/docs/concepts/services-networking/#loadbalancer)
(Service Load Balancers, or SLBs) for OpenShift in bare metal environments.
While bare metal clusters are of primary interest, we hope to find a solution
that would apply to other on-premise environments that don’t have native load
balancer capabilities available (a cluster on VMware, RHV, or OpenStack without
Octavia, for example).

## Motivation

Service Load Balancers are a common way to expose applications on a cluster. We
highly value clusters using on premise infrastructure, but do not support this
feature in that context.  We aim to fill this gap with an optional range of
capabilities.

We do have a related feature, `AutoAssignCIDRs`, where OpenShift will
automatically assign an ExternalIP for SLBs.  However, routing traffic to these
IPs is still left up as an exercise to the administrator.  This enhancement
offers an improvement where we can automate making these IP addresses
reachable.  This method would remain an option for any clusters where the
administrator would like to manage routing for external IPs in a completely
custom manner.  For more information on the existing `IngressIPs` feature:

* https://github.com/openshift/api/blob/master/config/v1/types_network.go#L99
* https://github.com/openshift/openshift-docs/pull/21388

### Goals

Some more context is helpful before specifying the goals of this enhancement.
When a Service has an external IP address, the OpenShift network plugin in use
must already prepare networking on Nodes to be able to receive traffic with
that IP address as a destination.  The network plugin does not know or care
about how that traffic reaches the Node because the mechanism differs depending
on which platform the cluster is running on.  Once that traffic reaches a Node,
the existing Service proxy functionality handles forwarding that traffic to a
Service backend, including some degree of load balancing.

With this context in mind, the goal of this enhancement is less about load
balancing itself, and more about providing mechanisms of routing traffic to
Nodes for external IP addresses used for Service Load Balancers.

A SLB solution must provide these high level features:

* Management of one or more pools of IP addresses to be allocated for SLBs
* High Availability (HA) management of these IP addresses once allocated.  We
  must be able to fail over addresses in less than 5 seconds for an unplanned
  Node outage.  We must be able to perform graceful failover without downtime
  for a planned outage, such as during upgrades.
* Solution must provide automation for making IP addresses available on the
  correct Node(s).  Solution must support a scalable L3 method for doing this
  (likely BGP), but should also be usable in smaller, simpler environments
  using L2 protocols.  Tradeoffs include:
    * Layer 2 (gratuitous ARP for IPv4, NDP for IPv6) - good for wide range of
      environment compatibility, but limiting for larger clusters.  All traffic
      for a single Service Load Balancer IP address must go through one node.
    * Layer 3 (BGP) - good for integration with networks for larger clusters
      and opens up the possibility for a greater degree of load balancing using
      ECMP to send traffic to multiple Nodes for a single Service Load Balancer
* Suitable for large scale clusters (target up to 2000 nodes).
* Must be compatible with at least the following cluster network types:
  [OpenShift-SDN](https://github.com/openshift/sdn) and
  [OVN-Kubernetes](https://github.com/ovn-org/ovn-kubernetes)

### Non-Goals

* We can also support this functionality through the use of partner add-ons,
  but discussion of those solutions is out of scope for this document.

## Proposal

Adopt [MetalLB](https://metallb.universe.tf/) as an out-of-the-box solution for
most on-premise SLB use cases.

MetalLB is commonly referenced when people discuss service load balancers for
bare metal.  The [concepts page](https://metallb.universe.tf/concepts/) gives a
pretty good overview of how it works.  It manages pools of IP addresses to
allocate for SLBs.  Once an IP is allocated to a SLB, it’s assigned to a Node
and the location of that IP address must be announced externally.  It has two
modes to announce IPs: [layer 2](https://metallb.universe.tf/concepts/layer2/)
(ARP for IPv4, NDP for IPv6) or
[BGP](https://metallb.universe.tf/concepts/bgp/).  The layer 2 mode is
sufficient for smaller scale clusters, while the BGP mode can work at much
larger scale.

While the BGP option is attractive for scaling reasons, it’s also more
complicated and will not work in all environments.  It won’t work in an
environment that won’t allow BGP advertisements from the cluster Nodes.  If a
cluster uses a Network addon that also makes use of BGP, MetalLB integration
will be more challenging.  For example see the [MetalLB page about Calico
support](https://metallb.universe.tf/configuration/calico/).

The layer 2 mode has the advantage of working in more environments.  We could
also consider a MetalLB enhancement that makes it understand different L2
domains and manage different IP address pools for each domain where SLBs may
reside.

### How Load Balancing Works with MetalLB

As mentioned in the Goals section, MetalLB does not have to implement load
balancing itself.  It only implements ensuring load balancer IP addresses are
reachable on appropriate Nodes.  The way a cluster uses MetalLB does have an
impact on how load balancing works, though.

When the Layer 2 mode is in use, all traffic for a single external IP address
must go through a single Node in the cluster.  MetalLB is responsible for
choosing which Node this should be.  From that Node, the Service proxy will
distribute load across the Endpoints for that Service.  This provides a degree
of load balancing, as long as the Service traffic does not exceed what can go
through a single Node.

The BGP mode of MetalLB offers some improved capabilities.  It is possible for
the router(s) for the cluster to send traffic for a single external IP address
to multiple Nodes.  This removes the single Node bottleneck.  The number of
Nodes which can be used as targets for the traffic depends on the configuration
of a given Service.  There is a field on Services called
[`externalTrafficPolicy` that can be `cluster` or
`local`](https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer/#preserving-the-client-source-ip).

* `local` -- In this mode, the pod expects to receive traffic with the original
  source IP address still intact. To achieve this, the traffic must go directly
  to a Node where one of the Endpoints for that Service is running. Only those
  Nodes advertise the Service IP in this case.

* `cluster` -- In this mode, traffic may arrive on any Node and will be
  redirected to another Node if necessary to reach a Service Endpoint. The
  source IP address will be changed to the Node's IP to ensure traffic returns
  via the same path it arrived on. The Service IP is advertised for all Nodes
  for these Services.

### User Stories

#### Match Cloud Load Balancer Functionality Where Possible

For each story below which discusses an environment we aim to support, we aim
to provide functionality that matches what you would see with a cluster on a
cloud.  This includes allowing Service load balancers accessible inside and
outside of the cluster, verifiable using existing e2e tests that make use of
service load balancers.

#### Story 1 - Easy Use with a Small Cluster

As an administrator of a small cluster that resides entirely on a single layer
2 domain, I would like to configure one or more ranges of IP addresses from my
network that the cluster is free to use for Service Load Balancers.  I do not
want to do any extra configuration in my network infrastructure beyond just
ensuring that the configured ranges of addresses are not used elsewhere.

MetalLB can do this today.

#### Story 2 - BGP Integration

As an administrator of a larger cluster with Nodes that reside on multiple L2
network segments, I would like to configure one or more ranges of IP addresses
from my network that the cluster is free to use for Service Load Balancers.  I
would like my cluster to peer with my BGP infrastructure to advertise the
current location of IP addresses allocated to Service Load Balancers.

MetalLB can do this today.

#### Story 3 - Larger Clusters without BGP

As an administrator of a larger cluster with Nodes that reside on multiple L2
network segments, I would like to configure one or more ranges of IP addresses
from my network that the cluster is free to use for Service Load Balancers.  I
do not have BGP infrastructure available or I'm not willing to have my cluster
peer with my BGP infrastructure.  I would like to configure awareness of which
subsets of my Nodes can use which pools of IP addresses since not every Node
has physical connectivity to the same L2 networks.

Note that MetalLB does not offer this today.  There has been some discussion
about related functionality:
* https://github.com/metallb/metallb/issues/605
* https://github.com/metallb/metallb/pull/502

### Implementation Details/Notes/Constraints

#### Upstream Engagement

The first step of implementation is to invest in the upstream project.  We
should have one or more engineers engage with the project to handle issues,
review pull requests, and contribute bug fixes or enhancements.  As part of
this process we should continue our technical due diligence with testing and
reviewing code to increase our confidence in choosing this solution.

One area we could contribute immediately is with setting up upstream CI.  The
project does not appear to run any CI today.

[kind](https://github.com/kubernetes-sigs/kind/) is a good basis for CI of
community kubernetes-ecosystem projects. It is usable with the built-in free
github CI support rather than needing someone to be paying for test
infrastructure elsewhere. It would even allow testing both L2 and L3 mode. It
doesn't matter what protocols the actual underlying network supports if you're
doing all of your testing in a virtual network built on top of it.

#### Operator

This is the first of two alternatives for how we might integrate MetalLB in
OpenShift.

We must also create an operator for MetalLB.  We should develop an operator
that is generally useful to the MetalLB community.  We should also have an
OpenShift version of this operator for our use.

It is assumed that the MetalLB operator would be managed by OLM as an optional
additional component to be installed on on-premise clusters.  However, in the
[ROADMAP.md
document](https://github.com/openshift/enhancements/blob/master/ROADMAP.md),
there is an item to "Front the API servers and other master services with
service load balancers".  If this functionality is required at install time,
the details on management of this operator may be revisited.

There is a start of a
[metallb-operator](https://github.com/cybertron/metallb-operator) available and
a [video demo](https://www.youtube.com/watch?v=WgOZno0D7nw).

#### Alternative Integration: Cloud Controller Manager

An alternative integration approach would be via a cloud controller manager
(CCM). An example of this is the [packet.net
CCM](https://github.com/packethost/packet-ccm), which ensures MetalLB is
deployed and also configures it properly to work in packet.net’s BGP
environment.

These integration options must be explored in more detail as part of a more
detailed integration proposal.

### Risks and Mitigations

#### Maturity and API Stability

While MetalLB appears to be [used in production by
some](https://github.com/metallb/metallb/issues/5), the project
itself claims it is in [beta](https://metallb.universe.tf/concepts/maturity/)
and that its users are early adopters.  We will mitigate this risk through our
own technical due diligence: reviewing and contributing to the code and via
extensive testing.

Given the pre-1.0 beta state of the project, we must pay particular close
attention to any interfaces that need to be stabilized before we can ship
MetalLB.  We want to get ahead of potential future upgrade challenges as soon
as possible.

#### Size of the Test Matrix

MetalLB includes two major modes of operation: layer 2 and BGP.  Both have
strengths and weaknesses.  Multiple modes also means an increase in our test
matrix.  If this proves to be a challenge, we should consider a phased roll-out
where we start with only the layer 2 mode (simpler, works in more environments)
and roll out BGP support as a later stage.

#### Security

Like any network facing application, MetalLB should be reviewed for any
security concerns.  This must be part of our ongoing technical due diligence.
So far, the following areas should receive a close look:
* The [memberlist](https://github.com/hashicorp/memberlist) protocol and
  implementation, used by MetalLB's layer 2 mode for cluster membership and
  fast Node failure detection.
* MetalLB's [custom implementation of
  BGP](https://github.com/metallb/metallb/tree/main/internal/bgp)
* MetalLB's
  [implementation](https://github.com/metallb/metallb/tree/main/internal/layer2)
  of [ARP](https://github.com/mdlayher/arp) and
  [NDP](https://github.com/mdlayher/ndp) for its layer 2 mode.

#### Logging, Debugging, Visibility

MetalLB has fairly limited debugging capabilities at this stage.  Events are
created for Services which provide some information.  Otherwise, you must read
the logs of the running components and hope to find some hints about what may
be going on.

Debugging is often a big challenge for networking components.  We should invest
early in enhancements to make understanding and debugging the behavior of
MetalLB as easy as possible.  This can be mitigated with a combination of good
documentation and improved tooling.

## Design Details

### Test Plan

Separate test plans are required for the layer 2 and BGP modes of MetalLB.

Testing MetalLB's layer 2 mode will work with our existing `e2e-metal-ipi` job.
In that CI job, we have full control of the networks used by the installed
cluster, so we can allocate a range of IP addresses for use by MetalLB.  More
investigation is needed, but it's likely that we can not test this on our
`e2e-metal` UPI based jobs because we rely on the network provided by
packet.net between the cluster hosts.

Testing of the BGP mode is more complicated.  It will require setting up a BGP
network environment for the cluster nodes to peer with.  We don't have anything
like this today, so it will take some work.  The upstream MetalLB project needs
this, as well.  It currently lacks any automated testing of the BGP
integration.

### Upgrade / Downgrade Strategy

The operator will include any required logic to handle upgrades or downgrades
and changes between versions of MetalLB.

MetalLB is currently configured via a `ConfigMap` that is not a stable API.  We
will start by building an operator that provides a stable API for configuration
and the `ConfigMap` will become an internal implementation detail fully owned
by the operator.

We must also mitigate these risks through engagement and contributions to the
upstream community to help make sure that changes made to the software and its
configuration interfaces can be managed through an upgrade or downgrade
process.

### Version Skew Strategy

MetalLB has two major components: a single `controller` pod and a `speaker` pod
that runs as a `DaemonSet`.

In layer 2 mode, the `speaker` needs to run on every `Node`.

In BGP mode, there's more flexibility. The behavior depends on the
[ExternalTrafficPolicy](https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer/#preserving-the-client-source-ip)
type on the service load balancers.

* If the type is `cluster`, then `speaker` can run on any subset of Nodes that
  you want peering with BGP routers. All of those Nodes will advertise that
  they can be used to reach all of of the Service load balancer IPs.
* If the type is `local`, then `speaker` must run on every node where an Endpoint
  may exist locally. Each node will only advertise that load balancer IPs are
  reachable that can map to a local Endpoint. Put another way, speaker must run
  on every node where workloads behind a service load balancer with an
  `ExternalTrafficPolicy` of `local` may run.

The primary version skew concern would be when there are `speaker` instances
running from different versions of MetalLB.  For example, in the layer 2 mode,
the `speaker` implementation includes an algorithm for it to independently
determine whether it should be the leader, or announcer, for a given `Service`.
A change to this algorithm in a new version could cause more than one `speaker`
to think it owns a `Service`.

These risks to version skew must be mitigated through upstream community
engagement and contributions, as well as informed management of upgrades in the
MetalLB operator.

## Implementation History

* (May, 2020) - Technical due diligence and upstream engagement beginning

## Drawbacks

TBD

## Alternatives

### Custom Solution using Keepalived

[Bare Metal IPI Networking
Infrastructure](https://github.com/openshift/installer/blob/master/docs/design/baremetal/networking-infrastructure.md)
is a document in the OpenShift Installer repository that discusses some of the
networking integration done for the bare metal IPI platform.

Bare Metal IPI clusters include keepalived + haproxy running on OpenShift
masters to manage a Virtual IP (VIP) for the API and to load balance API
requests.  This has been reused for other on-premise environments (VMware,
OpenStack, RHV).  It is implemented by having the machine-config-operator (MCO)
lay down static pod manifests.  See the document linked above for more details.

One keepalived-based option would be to build on and extend this existing
integration.  Configuration and management of a pool of IP addresses for SLBs
would be new code.

Another keepalived based starting point is the
[keepalived-operator](https://github.com/redhat-cop/keepalived-operator) which
is discussed in this [blog
post](https://www.openshift.com/blog/self-hosted-load-balancer-for-openshift-an-operator-based-approach).

Keepalived only supports L2 advertisement of IP address location (ARP / NDP).
To support larger scale clusters, we must do one of the following:

* Require all SLBs to be hosted on Nodes within a single L2 domain within the
  cluster.
* Make our SLB controller smart enough to understand different IP address
  pools, their associated L2 domains, and which Nodes are on which L2 domain.
* Extend keepalived (either directly or via some integration) to support an L3
  based address location advertisement (likely BGP).

Something based on keepalived is probably our simplest solution.  However,
downsides include:

* This would be entirely built by us.  It’s possible we could build some
  community usage around a simple solution like this, but that would take time.
  Given more featureful alternatives that already exist, I wouldn't expect much
  traction.
* There’s not a lot of opportunity for future functionality growth here unless
  we start swapping out the pieces (keepalived and/or haproxy), which would
  also sacrifice some of the simplicity.
* Keepalived uses the VRRP protocol and it would be nice to move away from
  this.  VRRP IDs are just 0-255 and the Keepalived + haproxy integration for
  bare metal IPI generates VRRP IDs based on the cluster name.  Even with
  different names, it’s possible to have a collision, and that can cause
  problems in lab environments with a lot of test clusters on shared networks.
  VRRP uses multicast by default which is not allowed in all environments,
  though it's also possible to configure keepalived to use unicast, instead.

### kube-vip

* [Web site](https://kube-vip.io/)
* [Kube-vip docs for SLBs](https://kube-vip.io/kubernetes/)
* [Code](https://github.com/plunder-app/kube-vip)

I came across `kube-vip` when the author shared it in the
`#cluster-api-provider` channel on the Kubernetes slack.  It’s new and likely
not mature, but some of the implementation is clever.

Instead of using VRRP to provide IP address HA, it uses RAFT (from
[https://github.com/hashicorp/raft]).  That would help avoid potential VRRP ID
collisions between multiple clusters.

Kube-vip implements its own custom load balancer, which is concerning from a
security, feature, and performance perspective.

`kube-vip` uses a couple of other supporting components:
* [starboard](https://github.com/plunder-app/starboard) - daemonset, manages
  iptables rules based on current IP address location
* [plndr-cloud-provider](https://github.com/plunder-app/plndr-cloud-provider) -
  kubernetes cloud provider

Kube-vip only supports layer 2 based address advertisement, and it doesn’t look
like it supports IPv6 yet.

Despite the project's young age, someone has already [integrated it with
OpenShift 3.11](https://github.com/megian/openshift-kube-vip-ansible).

Some of the key downsides to this option:
* Depends on yet-another-raft-implementation --
  https://github.com/hashicorp/raft
* New, developed as a hobby project by one person, likely PoC level maturity
* Lacking any layer 3 based address advertisement options

### OVN-Kubernetes Native Solution

A primary downside of this approach is being specific to OVN-Kubernetes, where
ideally we’d utilize something a bit more reusable.  Since OVN has much of the
required functionality built-in, it’s at least worth considering.

OVN has a native load balancing implementation which OVN-Kubernetes uses to
implement Services within a cluster.  OVN also includes L3 HA support, where
the IP address for a SLB would automatically fail over to another Node if one
Node fails.  OVN-Kubernetes could be expanded to support SLBs using these
features.

OVN only supports L2 based (ARP / NDP) address location advertisement.  To
address larger scale clusters, we would have to do one of:

* Require all SLBs to be hosted on Nodes within a single L2 domain within the
  cluster.
    * This is very limiting for scale, so it’s either only applicable to
      smaller clusters, or only a subset of the cluster can host SLB IP
      addresses.
* Make our SLB controller smart enough to understand different IP address
  pools, their associated L2 domains, and which Nodes are on which L2 domain.
    * This helps scale, but increases the complexity of our implementation.
* Extend OVN / OVN-Kubernetes (either directly or via some integration) to
  support an L3 based address location advertisement (likely BGP).
    * This doesn’t work for all environments, but the use of BGP is common and
      understood in the Kubernetes ecosystem.

## Infrastructure Needed

As noted in the `Test Plan` section of this document, the existing
`e2e-metal-ipi` job is a sufficient environment to run e2e tests with MetalLB
enabled with its layer 2 mode.  More work is needed to design a test
environment to test the BGP mode.  That work has not been done and may present
new requirements for test infrastructure.

## References

* [MetalLB web page](https://metallb.universe.tf/)
* [MetalLB on GitHub](https://github.com/metallb/metallb/)

Upstream issues that have come up in enhancement discussion:

* [metallb/metallb#168](https://github.com/metallb/metallb/issues/168) -
  Discussing using metallb to front the Kubernetes API server
* [metallb/metallb#621](https://github.com/metallb/metallb/issues/621) -
  Discussing a graceful no downtime failover method for layer 2 mode
