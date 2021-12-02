---
title: allow-mtu-changes
authors:
  - "@juanluisvaladas"
  - "@jcaamano"
reviewers:
  - "@danwinship"
  - "@dcbw"
  - "@knobunc"
  - "@msherif1234"
  - "@trozet"
  - "@yuqi-zhang"
approvers:
  - TBD
creation-date: 2021-10-07
last-updated: 2021-12-02
status: provisional
---

# Allow MTU changes

This covers adding the capability to the cluster network operator of changing
the MTU post installation.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Customers may need to change the MTU post-installation. However these changes
aren't trivial and may cause downtime, hence Cluster Network Operator currently
forbids them.

This enhancement proposes an automated procedure launched on demand and
coordinated by the Cluster Network Operator. This procedure is based on draining
and rebooting the nodes sequentially after performing the required configuration
changes to minimize service disruption and to ensure that the  
nodes have a healthy configuration and that workloads preserve connectivity
during and after the procedure.

## Motivation

While cluster administrators usually set the MTU correctly during the
installation, sometimes they need to change it afterwards for reasons such as
changes in the underlay or because they were set incorrectly at install time.

### Goals

* Allow to change MTU post install on OVN-Kubernetes or OpenshiftSDN.

### Non-Goals

* Other safe or unsafe configuration changes.

## Proposal

Cluster Network Operator (CNO from now on) will drive the MTU change when it
detects the change on the operator network configuration. The procedure is based
on two rolling reboot sequences that will be coordinated through Machine Config
Operator (MCO from now on). On the first reboot, routes MTU (`routable-mtu` from
now on) will be used to pin the effective MTU in use to the lower value allowing
changes to the MTU set on interfaces (`mtu` from now on) without disruption. On
a second reboot, once all the interfaces are set to the final MTU value, route
MTU is unset to make that interface MTU the one in effect. In more detail:

1. Set `mtu` to the higher MTU value and `routable-mtu` to the lower MTU value
   of the current and target MTU values.
2. Do a rolling reboot. As a node restarts, `routable-mtu` is set on routes
   while interfaces have `mtu` configured. The node will effectively use the
   lower `routable-mtu` for outgoing traffic, but be able to handle incoming
   traffic up to the higher `mtu`.
4. Set `mtu` to the target MTU value and unset `routable-mtu`.
5. Do a rolling reboot. As a node restarts, the target MTU value is set on the
   interfaces and the routes are reset to default MTU values. Since the MTU
   effectively used in yet to be restarted nodes of the cluster is the lower one
   but able to handle the higher one, this has no impact on traffic.

Due to the reboots it is adequate to consider this an MTU migration rather than
just simply an MTU change. Given the impact that the reboots have on the
cluster, this operation will be safeguarded by a specific migration annotation
or field.

The procedure will include handling the MTU of the cluster pod network and the
MTU of the host itself as is critical to do so in a coordinated manner to avoid
service disruptions and more reboots than the minimum necessary.

The procedure will optionally leverage Node Feature Discovery operator (NFD from
now on) through witch information about MTU capabilities of the nodes as well as
information of MTU migration status will be annotated on the nodes. This
information can be used to validate and track progress of the procedure.

Changes will be required on openshift-api, OpenshiftSDN, OVN-Kubernetes, CNO and
MCO.

### OVN-Kubernetes & OpenshiftSDN: new `routable-mtu` configuration parameter

A new `routable-mtu` setting for the CNI, additional to the already existing
`mtu` one, is introduced to be set as MTU in the following routes:
* default route in pods, covering traffic from a pod to any destination that is
  not a pod on the same host.
* non link scoped host routes of interfaces managed by the CNI covering traffic
  from host network to pods hosted on different nodes
* host routes of interfaces managed by the CNI covering traffic to the service
  network.

On startup, CNI should reset the MTU value on interfaces and routes to the
current setting of `mtu` and `routable-mtu` respectively even if such routes
already exist. If `routable-mtu` is unset, the CNI should unset the MTU on the
routes.

The CNI should annotate the `mtu` and `routable-mtu` values used on each node
that will allow to track the mtu-migration procedure.

### MCO: ability to configure host MTU migration

A configuration file will be deployed through MCO with the MTU parameters that
need to be set in the nodes during MTU migration:
* `TARGET_MTU` should be persistently set in the network configuration of the
  appropriate host interfaces. This setting should persist even if `TARGET_MTU`
  is unset in a subsequent reboot.
* `MTU` should be dynamically set in the appropriate host interfaces. If `MTU`
  is not set but `TARGET_MTU` is set, use the highest of `TARGET_MTU` or
  current interface MTU as `MTU`.
* `ROUTABLE_MTU` should be dynamically set for link scoped routes of the
  appropriate host interfaces. If `ROUTABLE_MTU` is not set but `TARGET_MTU` is
  set, use the lowest of `TARGET_MTU` and current interface MTU
  as`ROUTABLE_MTU`.

Host tooling that configures host networking for the CNI should source this
configuration file and act accordingly.

Host tooling should write a NFD source hook to setup annotations for current,
minimum and maximum supported MTU for the physical interface and route MTU if
any, on each node.

### openshift-api: add MTU migration specific fields

New `MTU.Network.From`, `MTU.Network.To` and `MTU.Machine.To` fields will be
added in the existing `NetworkMigration` type which is used in CNO's cluster and
operator network configurations and will contain the CNI and host MTU values to
migrate from and to respectively.

### MCO: automating MTU migration rolling reboots

MCO monitors `Network.Status.Migration` from the cluster network configuration.
This `Network.Status.Migration` is being already updated by CNO with the
validated contents of `Network.Spec.Migration` from the operator network
configuration. MCO should react to changes in `Migration.MTU` and if
`MTU.Machine.To` is set render the appropriate MachineConfig containing the MTU
configuration file with `TARGET_MTU` set to `MTU.Machine.To` and a dummy
parameter (that will be ignored other than causing a reboot) for
`MTU.Network.To`.

This will result in the required rolling reboots as `MTU.Machine.To` or
`MTU.Network.To` are set and then unset during the MTU migration procedure.

### CNO: allowing MTU migration

The CNO monitors changes on the operator network configuration. When it detects
a change in `Network.Spec.Migration`:
1. Check that `MTU.Network.From`, `MTU.Network.To` and `MTU.Machine.To` are not
   independently set. Otherwise report an unsupported or unsafe configuration.
2. Check that `MTU.Network.From` equals `Network.Status.ClusterNetworkMTU`.
   Otherwise report an unsupported or unsafe configuration.
3. Check that `MTU.Network.To`, if set or the currently configured CNI `mtu`
   otherwise, is valid MTU value with respect to `MTU.Machine.To`. If not,
   report an unsupported or unsafe configuration.
4. Render & apply the CNI configuration using highest and lowest MTU values from
   `MTU.Network.From` and `MTU.Network.To` as the new CNI `mtu` and
   `routable-mtu` respectively, if different.
5. Render & apply the applied network configuration using `MTU.Network.To` as
   the `mtu`.

This allows a second step that requires no further changes where the
administrator can set the configured CNI `mtu` to `MTU.Network.To` at the same
time it removes the `Migration` information without such a change being declared
as unsafe, and completing the migration procedure.

#### MTU decrease example

Current MTU: 9000 (host 9100)
Target MTU: 1400 (host 1500)

* Administrator updates the operator network configuration with
  `MTU.Network.To=1400`, `MTU.Network.From=9000` and `MTU.Machine.To=1500`.
* CNO sets in the applied cluster configuration `mtu=1400`.
* MCO applies a MachineConfig with `TARGET_MTU=1500`.
* Administrator waits for the corresponding rolling reboot to be performed.
* Administrator checks through node annotations or directly on the nodes for
  expected MTU values.
  * On unexpected MTU values, a rollback could be attempted setting
    `MTU.Network.To=9000`, `MTU.Network.From=9000` and `MTU.Machine.To=9100`.
* Administrator updates the operator network configuration with `mtu=1400` and
  removing the `Migration.MTU` field.
* MCO removes `TARGET_MTU=1500` from MachineConfig.
* Administrator waits for the corresponding rolling reboot to be performed.
* Administrator checks through node annotations or directly on the nodes for
  expected MTU values.
  * On unexpected MTU values, a rollback could be achieved attempting the
    opposite MTU migration.

#### MTU increase example

Current MTU: 1400 (host 1500)
Target MTU: 9000 (host 9100)

* Administrator updates the operator network configuration with
  `MTU.Network.To=9000`, `MTU.Network.From=1400` and `MTU.Machine.To=9100`.
* CNO sets in the applied cluster configuration `mtu=9000`.
* MCO applies a MachineConfig with `TARGET_MTU=9100`.
* Administrator waits for the corresponding rolling reboot to be performed.
* Administrator checks through node annotations or directly on the nodes for
  expected MTU values.
  * On unexpected MTU values, a rollback could be attempted setting
    `MTU.Network.To=1400`, `MTU.Network.From=1400` and `MTU.Machine.To=1500`.
* Administrator updates the operator network configuration with `mtu=9000` and
  removing the `Migration.MTU` field.
* MCO removes `TARGET_MTU=9100` from MachineConfig.
* Administrator waits for the corresponding rolling reboot to be performed.
* Administrator checks through node annotations or directly on the nodes for
  expected MTU values.
  * On unexpected MTU values, a rollback could be achieved attempting the
    opposite MTU migration.

### User Stories

#### As an administrator, I want to change the cluster MTU after deployment

### API Extensions

New `MTU.Network.From`, `MTU.Network.To` and `MTU.Machine.To` fields in
`Network.Status.Migration` from `config.openshift.io/v1` and
`Network.Spec.Migration` from `operator.openshift.io/v1`.

### Implementation Details/Notes/Constraints

## Design Details

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

### Open Questions

* We might not be aware at any given time of host link scoped routes if these
  are dynamically assigned through IPv6 RA. NetworkManager dispatcher does not
  seem to trigger an event for route modifications resulting from IPv6 RA. We
  might have to require the administrator to setup the static routes before
  applying the procedure that may lead to additional reboots.

### Test Plan

Two new CI tests, one for IPv4 and another for IPv6, that will run upstream
network tests and a specific e2e test to verify that the MTU set is the one
effectively used in the different network paths, both after increasing and
decreasing the MTU values. The tests will be triggered by demand.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Risks and Mitigations

* A cluster wide MTU change procedure cannot be carried out in an instant and
  incurs the risk of having different MTUs in use across the cluster at a given
  time. While the described procedure in this enhancement prevents this with the
  use of route MTUs, some consequences from using different MTUs in the cluster
  are analyzed in the following section for context.

#### Running the cluster with different MTUs

On the process of a `live` change of the MTU, there is going to be traffic
endpoints temporarily using different MTU values. In general, if the path MTU to
an endpoint is known, fragmentation will occur or the application will be
informed that it is trying to send larger packets than possible so that it can
adjust. Additionally, connection oriented protocols, such as TCP, usually
negotiate their segment size based on the lower MTU of the endpoints on
connection.

So generally, different MTUs on endpoints affect ongoing connection-oriented
traffic or connection-less traffic, when the known destination MTU is not the
actual destination MTU. In this case, the most likely scenario is that traffic
is dropped on the receiving end by OVS if larger than the destination MTU.

There are circumstances that prevent an endpoint from being aware of the actual
MTU to a destination, which depends on Path MTU discovery and specific ICMP
`FRAG_NEEDED` messages:
* A firewall is blocking these ICMP messages or the ICMP messages are not being
  relayed to the endpoint.
* There is no router between the endpoints generating these ICMP messages.

Let's look at different scenarios.

##### Node to node

On the receiving end, a nic driver might size the buffers in relation to the
configured MTU and drop larger packets before they are handed off to the system.

Past that, OVN-K sets up flows in OVS br-ex for packets that are larger than pod
MTU and sends them off to the network stack to generate ICMP `FRAG_NEEDED`
messages. If these packets exceed the MTU of br-ex, they will be dropped by OVS
and never reach the network stack. Otherwise they will reach the network stack
but not generate ICMP `FRAG_NEEDED` messages as network stack only does so for
traffic being forwarded and not for traffic with that node as final destination.

As there is generally no router in between two cluster nodes, more than likely a
node would not be aware of the path MTU to another node.

##### Node to pod

As explained before, network stack receives larger packets between host MTU and
pod MTU and might cause ICMP `FRAG_NEEDED` messages to be sent to the
originating node such that a node might be aware of the proper path MTU when
reaching out to pod. Otherwise larger than pod MTU traffic will dropped by OVS.

##### Pod to Node

On this datapath, OVS at the destination node will drop the larger packets
without generating ICMP `FRAG_NEEDED` messages as the node is the final
destination of the traffic. The originating pod is never aware of the actual
path MTU.

##### Pod to Pod

This traffic is encapsulated with Geneve. The Geneve driver might drop it and
generate ICMP `FRAG_NEEDED` messages back to the originating pod if it is trying
to send packets that would not fit in the originating node MTU once
encapsulated. But OVN is not prepared to relay back these ICMP messages to the
originating pod so it would not be aware of an appropriate MTU to use.

On the receiving end, OVS would drop the packet silently if larger than the
destination MTU of the veth interface. Even if this would not be the case, the
veth driver itself would drop the packet silently if over the MTU of the pod's
end veth interface.

## Implementation History

## Drawbacks

## Alternatives

* An alternative was considered to perform the MTU change "live", rolling out
  the ovnkube-node daemon set with a new MTU value and changing all the running
  pods MTU. This approach, while quicker and more convenient, was discarded as
  it was considered less safe due to possible edge cases where the odd chance of
  leaving pods behind with wrong MTU values was difficult to avoid, as well as
  offering less guarantees of minimizing service disruption.
