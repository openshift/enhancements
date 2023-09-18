---
title: egress-ip-multi-nic
authors:
  - "@martinkennelly"
reviewers:
  - "@trozet"
  - "@kyrtapz"
  - "@flavio-fernandes"
  - "@maiqueb"
approvers:
  - "@trozet"
  - "@knobunc"
api-approvers:
- "@trozet"
creation-date: 2023-05-16
last-updated: 2023-06-07
status: implementable
tracking-link:
- https://issues.redhat.com/browse/SDN-1123
---
# Egress IP Multi-NIC
## Summary
For OpenShift release version 4.13 or less, users may utilize egress IP to allow source NAT when packets leave OVN managed network
by specifying the source NAT IP within an `EgressIP` kubernetes custom resource definition.
This enhancement will add egress IP support for networks that exists on NICs that aren't managed by OVN and hence forth called
non-ovn managed networks.

## Motivation
Users may not wish to route workload traffic over OVN managed network and instead prefer to use a non-ovn managed network.
With this enhancement, we can support segregation of workload traffic over multiple NICs while allowing a configurable source IP address.

### Goals
- Support egress IP for non-ovn managed networks

### Non-Goals
- Support egress IP on non-default networks controllers aka secondary network controllers
- Support cloud egress IP assignment for non-ovn managed interfaces using cloud network config controller
  - This maybe easy to do eventually, but it will require a lot of extra testing, so excluding it for now, and it's
    a stretch goal.
- Support egress IP on secondary bridges managed by OVN aka br-ex1

## Proposal

### Implementation Details/Notes/Constraints
Users may provide not only an ovn managed network but also additional non-ovn managed networks that they want to utilize and support egress IP.
Cluster manager will continue to provision `EgressIP` to nodes. Currently, it only assigns them to nodes which have a
well-known label egress assignable `k8s.ovn.org/egress-assignable`.

This enhancement is proposing enhancing cluster manager if the egress IP is within a subnet of a known non-primary network, then it
will also consider which node hosts this secondary network.

Cluster manager will therefore need to understand all available networks within the cluster and this enhancement proposes
to add all non-ovn managed interfaces addresses and subnet mask to existing annotation `k8s.ovn.org/host-addresses` which is
added to all node custom resource objects.

Egress IP must not equal an address already assigned to an interface, and we can use the aforementioned `k8s.ovn.org/host-addresses`
annotation to detect this scenario. The addresses that can be added into this annotation are filtered by the following criteria:
- Link must be up
- Link must not be loopback, type openvswitch or bridge
- Link must not have a master index (aka bridge)
- Link must not have an associated parent index (aka child devices)
- Address must have scope global

With a prerequisite that subnets must not overlap, we can determine which interface can host an egress IP.
We will first determine if the egress IP can be hosted within the primary OVN managed network and if not, then
use the longest prefix match to determine which interface the egress IP is assigned on.

For example, lets say we have the following 2 nodes with networks:
```shell
# worker 1
3: ens4: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP group default qlen 1000
link/ether 52:54:00:c7:bb:91 brd ff:ff:ff:ff:ff:ff
altname enp0s4
inet 192.168.123.181/24 brd 192.168.123.255 scope global dynamic noprefixroute ens4
valid_lft 3552sec preferred_lft 3552sec
inet6 fe80::a4ae:c1cb:2143:bb63/64 scope link noprefixroute
valid_lft forever preferred_lft forever

# worker 2
3: ens4: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP group default qlen 1000
link/ether 52:54:00:06:d6:58 brd ff:ff:ff:ff:ff:ff
altname enp0s4
inet 192.168.123.94/24 brd 192.168.123.255 scope global dynamic noprefixroute ens4
valid_lft 3494sec preferred_lft 3494sec
inet6 fe80::21df:c25a:dc63:f6eb/64 scope link noprefixroute
valid_lft forever preferred_lft forever
```

Cluster manager could assign the egress IP to either node.

Another example, lets say we have the following 2 nodes with networks:
```shell
# worker 1
3: ens4: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP group default qlen 1000
link/ether 52:54:00:c7:bb:91 brd ff:ff:ff:ff:ff:ff
altname enp0s4
inet 192.168.123.181/16 brd 192.168.123.255 scope global dynamic noprefixroute ens4
valid_lft 3552sec preferred_lft 3552sec
inet6 fe80::a4ae:c1cb:2143:bb63/64 scope link noprefixroute
valid_lft forever preferred_lft forever

# worker 2
3: ens4: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP group default qlen 1000
link/ether 52:54:00:06:d6:58 brd ff:ff:ff:ff:ff:ff
altname enp0s4
inet 192.168.123.94/24 brd 192.168.123.255 scope global dynamic noprefixroute ens4
valid_lft 3494sec preferred_lft 3494sec
inet6 fe80::21df:c25a:dc63:f6eb/64 scope link noprefixroute
valid_lft forever preferred_lft forever
```

Cluster manager would assign the `EgressIP` to worker 2.

To allow packets to be steered into secondary networks, OVN reroute logical policies on `ovn_cluster_router` will redirect
the packet out to the management port of the node which the egress IP is hosted.

We will add rules in linux routing policy database control for each source Pod IP and lookup the correct routing table
which will contain one route. This single route within a new managed routing table will direct the packet to the correct interface.

We will also add a masquerade rule to iptables when a packet leaving a particular interface matches the source pod IP.


See section `Design Details` for full implementation details.

### User Stories
As a user, I want the ability for cluster workload traffic to be routed to a network that is not managed by OVN and to have a configurable source IP.

### Test Plan
E2E tests verifying the correct source IP for deployment modes global IC and multi-zone IC:
- Day 2 Networks CRUD including mask resizing
- Configure new EgressIP both on ovn managed and unmanaged networks
- Migrate existing EgressIP on ovn managed and unmanaged networks
- Delete existing EgressIP on ovn managed and unmanaged networks
- Add node
- Delete node
- Edit node
- Add pod
- Delete pod
- Remove pod
- Delete a valid rule manually 
- Delete a valid IPtables rule manually
- Delete a valid route manually
- Delete a valid egress IP assigned to an interface manually
- Valid egress service and egress IP functionality if both are active on the same k8 objs
Today, if egress service and egress IP both 'match' on the same pod, egress service is preferred
Continue this for this enhancement where egress service and egress IP overlap, prefer egress service
- Egress IP interface is now a child interface and attached to a bond which is now the IP of the EIP interface. Reconcile to move the EIP to the bond.
- Egress IP interface is now a child interface and attached to a bond which is now a different IP of the original EIP interface. Reconcile to move the EIP
to a different network and if none found, clear EIP node status and let cluster manager reconcile and find a new node
- Egress IP balancing of non-ovn EIP assignments is the same as current EIP balancing
- Mixing Egress IPs from ovn and non-ovn managed networks
- GARPs are sent when egress IP moves from a non-ovn managed network
- Link goes down
- EIP removed
- Original network that allowed EIP assignment is removed
- EIP was assigned to ovn-managed network but CM assigned it to another network on the node

### API Extensions
N/A

### Risks and Mitigations
- OVN kube node must recalculate all the matched pods for a given egress IP. This will require refactoring this logic
  from ovnkube-controller and may introduce bugs to egress IP itself.
- Stale EgressIPs assigned to a non-ovn managed interface
  A component will be created to manage all network interface address operations.
  This component will independently ensure the state of all network interfaces is what we expect.
  Egress IP(s) assigned to an interface will contain a well-known label which will allow us to clean up stale egress IP addresses.
  Therefore, we don't need any internal caches for this.
- Stale linux routing rules in the routing policy database control
  A component will be created to manage all rule operations.
  We can ensure stale routes are cleaned because we use a specific priority that only we manage, therefore we don't
  need any internal caches for this.
  This component will independently manage the operations of rules and ensure they match the state we want.
- Stale iptables rules
  A component will be created to manage all iptable operations.
  This component will independently manage the operations of iptables and ensure they match the state we want.
  This component will ensure the rules we expect within a newly created specific chain are what we expect.
  Therefore, we don't need any internal caches for this because we know we manage all rules within this chain.
- Incorrect egress IP network interface assignment
  The host addresses label will determine which network (interface) the egress IP will be assigned to.
  Users may not expect that we will assign this egress IP to whichever interface contains the longest prefix match [1].
  This is not a problem because we will emit the packet to the correct network regardless.
- Unable to determine if an egress IP belongs to an ovn managed network or non ovn manged network
  OVN managed network can be determined from well-known label `k8s.ovn.org/node-primary-ifaddr`
  Therefore anything outside this subnet mask is consider a "non-ovn" network
- What about secondary bridges managed by OVN aka br-ex1
  We ignore them and do not support egress IP.
- Selecting the wrong network types from interfaces
  Only consider addresses with scope `Global`.

[1] https://en.wikipedia.org/wiki/Longest_prefix_match

## Design Details
### Cluster Manager
If egress IP is within a subnet of a non-ovn managed network, it will need to assign this egress IP to a node which hosts this network.

### OVN kube controller (aka ovn kube master, aka network controller manager)
Enhance OVN logical router `ovn_cluster_router` to make the next-hop choice a little smarter. If the egress IP is assigned
to a secondary network, then reroute it the management port of the node that hosts the secondary network.

### OVN kube node
OVN kube node will need to watch for `EgressIP`, `Node`, `Pod` and `Namespace` custom resource changes and determine if rules,
routes or iptables rules need configuring. If the node which this component runs on is not labeled with egress IP assignable label,
then the aforementioned are not needed. However, if the node is labeled egress IP assignable and if an egress IP is assigned to
the node and if the egress IP is assigned to a non-ovn managed network, then we will need to perform the following operations:
- Create a new rule for each pod that matched with an egress IP
```shell
$ ip rule
0:	from all lookup local
10: from ${pod_ip} lookup network-a
32766:	from all lookup main
32767:	from all lookup default
```
Within the route table created, we need to create a default route to the correct interface.
We also need to add manage iptables rules in the NAT table for each pod IP to correctly source NAT (SNAT) to the egress IP
only when leaving our desired interface.

If multiple `EgressIP` overlap and select the same set of pods, then this is
undefined behaviour.

### Tooling changes
None needed. All info is already gathered from sos report and must-gather.

### Metrics / Alerts
- Metric counting the length of time to apply iptables rules. This will allow us to understand if this action is CPU
starved and therefore delaying rollout of egress IP configuration. An alert is then needed to notify the cluster admin
and the action taken should be to reduce load on the node.

### Runbooks
- Runbook is needed to diagnose and mitigate slow iptables operations.
- Runbook needed to debug if the feature is not working, documenting what steps a user must take to diagnose failures
and what possibly corrective actions a user may take.

### Graduation Criteria

#### Dev Preview -> Tech Preview
N/A

#### Tech Preview -> GA
GA: 4.15

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy
N/A

### Version Skew Strategy
N/A

### Operational Aspects of API Extensions
N/A

#### Failure Modes
- Distributed components configuring at different times and pod ip leaking out non-ovn managed interfaces
Need to investigate this and ensure we arent leaking internal IPs externally.

#### Support Procedures
- General debug: sos report will gather all network interface addresses, rules, routes and iptables rules which will allow us to understand
  the erroneous configuration.
- CPU constrained and unable to configure iptables rules within a reasonable amount of time
  iptables may get cpu starved, and we begin to drop packets. Within this enhancement implementation, we will expose, as a metric the time
  taken to configure iptables.

## Implementation History
4.14: Initial implementation

## Alternatives
N/A

### Drawbacks
- CPU usages for ovnkube node. Because we need to list-watch lot of different k8 objs, we will consume a lot of CPU on
worker nodes.

### Workflow Description
