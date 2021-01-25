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
approvers:
  - TBD
creation-date: 2021-10-07
last-updated: 2021-10-14
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
aren't trivial and may cause downtime, hence the CNO currently forbids them.

We propose a procedure that will be launched on demand. This procedure will
run pods on every node of the cluster and make the necessary changes in an
ordered and coordinated manner with a service disruption within the least
possible time, which if under a reasonable time of 10 minutes, should be well
under the typical TCP timeout interval.

## Motivation

While cluster administrators usually set the MTU correctly during the
installation, sometimes they need to change it afterwards for reasons such as
changes in the underlay or because they were set incorrectly at install time.

### Goals

* Allow to change MTU post install on OVN Kubernetes.

### Non goals

* Change the MTU without service disruption.

* Other safe or unsafe configuration changes.

## Proposal

The CNO monitors changes on the operator configuration. When it detects a MTU
change:
1. Set the `clusteroperator/network` conditions:
   - Progressing: true
   - Upgradeable: false
2. Check that the MTU value is valid, within threoretically min/max values.
3. Check that all the nodes are on Ready state.
4. Deploy pods on every node with `restartPolicy: Never` which are responsible
   for validating the preconditions. If the preconditions are met the pod will
   exit with code 0. Some of the preconditions that we will check are:
   - The underlay network supports the intended MTU value.
5. Once all the previous pods finish successfully, deploy other set of pods with
   `restartPolicy: Never` on every node that will handle the actual change of
   the MTU (explained in more detail below). Wait for them to be ready and
   running.
6. Ensure that the configmap ovnkube-config is synchronized with the new MTU
   value.
7. If any previous steps (1-6) was unsuccesful, the CNO will set the
   `clusteroperator/network` conditions to:
   - Progressing: false
   - Degraded: true
   Update the operator configuration status with a description of the problem.
   At this point the process is interrupted and we require manual intervention.
8. Force a rollout of the ovnkube-node daemonset. This will ensure
   ovn-kubernetes uses the new MTU value for new pods as well as set the new MTU
   on managed node interfaces like ovn-k8s-mp0, ovn-k8s-gw0 (local gateway mode)
   and related routes.
9. Set the new MTU value to the applied-cluster config map AND wait for pods of
   step 3 to complete successfully.
10. If any of the previous steps (8,9) failed, reboot the node, wait for the
    kubelet to be reporting as Ready again.
    If this step fails, set conditions to:
    - Progressing: false
    - Degraded: true
    Update the operator configuration status with a description of the problem.
11. Upon completion, set conditions to:
    - Progressing: false
    - Degraded: false

The steps to change the MTU performed by pods of previous step 3 are:
1. So that we don't have nodes doing things at different times and we have
   everything synchronized, the pods will wait until the MTU value on the
   applied-cluster configmap changes.
2. Enter every network namespace. If an interface `eth0` exists in that
   namespace with an ip address within the pod subnet, change the MTU of the
   the veth pair.
3. If any of these steps failed (1-3), the pod will exit with code 1, if all
   were successful it will exit with code 0.

An administrator should be able to deploy a machine-config object to change
the node MTU as well. If increasing the MTU, it will do so at the beginning
of the procedure. If decreasing the MTU, it will do so at the end of the
procedure.

### User Stories

#### As an administrator, I want to change the node MTU

An administrator should be able to deploy a machine-object config object
that configures the node MTU permanently. Ideally this would be achieved
through the ability to run configure-ovs with an MTU parameter.
configure-ovs should change the MTU of br-ex and ovs-if-phys0 with the
least impact on the existing configuration to avoid any unnecessary
disruption. This change should persist across reboots.

#### As an administrator, I want to change the cluster network MTU

An administrator should be able to change the cluster network MTU through
CNO configuration change. This would encompass the following tasks:

##### Implement a pod that changes the actual MTU on running pods

Implement a pod that changes the actual MTU for both ends of the veth
pair for pods hosted in the node where the pod runs as described in the
proposal, and in the least possible time.

##### Add support in ovnkube-node to reset MTU on start

Make sure that upon restart, ovnkube-node resets the MTU on all the relevant
interfaces, like ovn-k8s-mp0, ovn-k8s-gw0, br-int as well as related routes
that currently have a MTU set.

##### Add support in CNO for MTU change coordination

Add support in CNO to allow and coordinate the MTU change for OVN-Kubernetes
as described in the proposal.

### Implementation Details/Notes/Constraints

## Design Details

### Open Questions

* If changing the MTU on a node fails, do we have guarantee that we can still
  reboot the node?

### Test Plan
We will create the following tests:
1. An HTTPS server with a very large certificate, and multiple clients
   in different nodes doing a single HTTPS request. The acceptance criteria
   is TLS negotiation suceeds and HTTPS request returns 200 after every MTU
   change.

Packet loss, TCP retransmissions, increased latency, and reduced bandwidth and
connectivity loss considered acceptable while the change is happening.

While previous test is running, we will decrease the MTU, and
once it's finished we'll increase it to it's previous value.

This test will be two new jobs in CI, one for IPv4 and another for IPv6, that
will be launched on demand.

### Risks and Mitigations

* If unexpected problems ocurr this procedure, the mitigation is an automated
  node reboot. The worst possible outcome is a full unplanned reboot
  of the cluster. Documentation should advertise of these possible
  consequences. An alternate implementation with planned reboots is described
  in the Alternatives section.
* Even though the procedure takes place under the absolute TCP timeout interval,
  applications might have their own timeout implementation. Service disruption
  and how applications handle it is a risk that might need to be considered on
  per application basis but that can not be reasonably scoped in this
  enhancement.
* During the procedure, different MTUs will be used throughout the cluster. Next
  section analyzes the consequences in detail.

#### Running the cluster with different MTUs

On the process of a `live` change of the MTU, there is going to be traffic
endpoints temporarily using different MTU values. In general, if the path MTU
to an endpoint is known, fragmentation will occur or the application will be
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
messages. If these packets exceed  the MTU of br-ex, they will be dropped by OVS
and never reach the network stack. Otherwise they will reach the network stack
but not generate ICMP `FRAG_NEEDED` messages as network stack only does so for
traffic being forwarded and not for traffic with that node as final destination.

As there is generally no router in between two cluster nodes, more than likely a
node would not be aware of the path MTU to another node.

##### Node to pod

As explained before, network stack receives larger packets betwewen host MTU and
pod MTU and might cause ICMP `FRAG_NEEDED` messages to be sent to the originating
node such that a node might be aware of the proper path MTU when reaching out to
pod. Otherwise larger than pod MTU traffic will dropped by OVS.

##### Pod to Node

On this datapath, OVS at the destination node will drop the larger packets without
generating ICMP `FRAG_NEEDED` messages as the node is the final destination of the
traffic. The originating pod is never aware of the actual path MTU.

##### Pod to Pod

This traffic is encapsulated with geneve. The geneve driver might drop it and
generate ICMP `FRAG_NEEDED` messages back to the originating pod if it is trying
to send packets that would not fit in the originating node MTU once encapsulated.
But OVN is not prepared to relay back these ICMP messages to the originating pod
so it would not be aware of an appropiate MTU to use.

On the receiving end, OVS would drop the packet silently if larger than the
destination MTU of the veth interface. Even if this would not be the case, the
veth driver itself would drop the packet silently if over the MTU of the pod's
end veth interface.

## Alternatives

### New ovn-k setting: `routable-mtu`

OVN-Kube Node, upon start, sets that `routable-mtu` on all the host routes and
on all created pods routes. This will make all node-wide traffic effectively
use that MTU value even though the interfaces might be configured with a higher
MTU. Then, with a double rolling reboot procedure, it should be possible to
change the MTU with no service disruption.

Decrease example:
* Set in ovn-config a `routable-mtu` setting lower than the `mtu` setting.
* Do rolling reboot, as nodes restart they will effectively use lower MTU, but
  since the actual interfaces MTU did not change they will not drop traffic
  coming from other nodes.
* Set in ovn-config a `mtu` equal to `routable-mtu` or replace `mtu` with the
  `routing-mtu` value and remove the latter.
* Do rolling reboot, as nodes restart they will do so with interfaces configured
  the expected MTU. As other nodes are effectively using this MTU setting, no
  traffic drop is expected.

Increase example:
* Set in ovn-config the actual `mtu` as `routable-mtu` and a new `mtu` setting
  higher than `routable-mtu`.
* Do rolling reboot, nodes will restart with the higer MTU setting configured
  on their interfaces but still be effectively using the lower MTU.
* Set in ovn-config a `mtu` equal to `routable-mtu` or replace `mtu` with the
  value of `routing-mtu` value and remove the latter.
* Do rolling reboot, as nodes restart they will use the higher MTU. As other
  nodes already have this MTU set on their interfaces no drops are expected.

These procedure should be coordinated with changing the MTU setting on br-ex
and its physical port.
