---
title: detached-ovn-gateway-bridge
authors:
- "@pliurh"
reviewers:
- "@jcaamano, OpenShift Networking"
- "@pacevedom, MicroShift"
- "@pmtk, MicroShift"
- "@trozet, OpenShift Networking"
- "@zshi-redhat, MicroShift"
approvers:
- "@dhellmann"
api-approvers:
- None
creation-date: 2023-04-26
last-updated: 2023-05-22
tracking-link:
- https://issues.redhat.com/browse/NP-654
see-also:
- None
replaces:
- None
superseded-by:
- None
---

# Use Detached External Gateway Bridge of OVN-Kubernetes for MicroShift

## Summary

This enhancement proposes a new external gateway configuration of OVN-Kubernetes
for MicroShift. With the new configuration, the `br-ex` will be created without
binding any host physical interface.

## Motivation

MicroShift is designed to run on IoT and edge devices, it can be deployed on
various kinds of devices with different network environment. 

Today configure-ovs.sh (microshift-ovs-init.service) sets up OVN-Kubernetes
gateway bridge `br-ex` for OVN-Kubernetes.  There have been several types of
issues associated with it mainly due to:

1. transient network interruption when it moves the node IP from uplink port to
   gateway bridge

2. certain type of network devices are not supported (e.g. wifi, tap/tun)

3. it relies on (default) route info

### User Stories

* As an MicroShift cluster administrator, I want the installation experience of
MicroShift is similar as a normal application. I don't want to have any
networking interrupt during the installation.

* As an MicroShift cluster administrator, I want to use a specific network
  devices (such as wifi, tap/tun) as the node default interface.

* As an MicroShift cluster administrator, I want to install MicroShift on an
  isolated host that has no default gateway available.

### Goals

* Decouple the br-ex bridge from the host physical interfaces.
* Enhance OVN-Kubernetes local gateway mode to support external gateway bridge
  without physical NIC.

### Non-Goals

* Creating a new gateway mode in OVN-Kubernetes for single node cluster.

## Proposal

Instead of creating the br-ex bridge with the node's default interface, we
create the br-ex without adding any host interface as its uplink port. Today, in
order to work around the certificates issue, MicroShift adds a static kubernetes
API server `advertise-address` (default 10.44.0.0/32) to the host `lo`
interface. To avoid using an extra IP for the dummy gateway interface `br-ex`,
we will use this advertiseAddress: 

1. if `br-ex` doesn't exist, which means configure-ovs.sh was not invoked, the
   CNI is not ovn-k, add advertiseAddress to `lo` interface.
2. Otherwise, add advertiseAddress to `br-ex` interface.

Currently, the local gateway mode and shared gateway mode both use the default
gateway of the host as the default gateway of the node gateway router. It is
required by certain egress features like egress IP. However MicroShift doesn't
need those features . The pod egress traffic will leave OVN-Kubernetes and enter
host kernel via `mp0` instead of the gateway route. In the context of
MicroShift, only the host to service traffic will be processed by the gateway
router. In the gateway router we only need to set the default route with the
ovn-k internal masquerade next-hop (169.254.169.4) as next-hop. So that the
internal to localhost traffic can be forwarded to host kernel via bridge
`br-ex`.

The OVN gateway router will have a routing table that looks like:

```none
IPv4 Routes
Route Table <main>:
                0.0.0.0/0             169.254.169.4 dst-ip rtoe-GR_ovn-control-plane
            10.244.0.0/16                100.64.0.1 dst-ip
```

And the new logical topology will be like this:

```none

      ┌─────────────┐
      │ eth0        │
  ┌───┤ 10.89.0.3/24├────────────────────────────────────────────┐
  │   └───┬─────────┘                        MicroShift Node     │
  │       │                                                      │
  │    ┌──┴──────┐                                               │
  │    │routes + │     ┌───────-───────┐                         │
  │    │iptables ├─────┤     br-ex     │                         │
  │    └─────┬───┘     │  10.44.0.0/32 │                         │
  │          │         └───────┬───────┘                         │
  │          │                 │                                 │
  │    ┌─────┼─────────────────┼────────────────────────────┐    │
  │    │     │                 │       OVN Virtual Topology │    │
  │    │     │        ┌────────┴────────┐                   │    │
  │    │     │        │ External Switch │                   │    │
  │    │     │        └────────┬────────┘                   │    │
  │    │     │                 │                            │    │
  │    │     │     ┌───────────┴──────────┐                 │    │
  │    │     │     │     Gateway Router   │                 │    │
  │    │     │     └───────────┬──────────┘                 │    │
  │    │     │                 │                            │    │
  │    │     │         ┌───────┴────────┐                   │    │
  │    │     │         │  Join Switch   │                   │    │
  │    │     │         └───────┬────────┘                   │    │
  │    │     │                 │                            │    │
  │    │     │   ┌─────────────┴─────────────┐              │    │
  │    │     │   │    OVN Cluster Router     │              │    │
  │    │     │   │        10.244.0.1/24      │              │    │
  │    │     │   └─────────────┬─────────────┘              │    │
  │    │     │                 │                            │    │
  │    │     │        ┌────────┴─────────┐                  │    │
  │    │     │        │    Node Switch   │                  │    │
  │    │     │        └────────┬─────────┘                  │    │
  │    │     │                 │                            │    │
  │    │  ┌──┴────────────┐    │      ┌──────────────┐      │    │
  │    │  │ mp0           ├────┴──────┤Pod           │      │    │
  │    │  │ 10.244.0.2/24 │           │10.244.0.50/24│      │    │
  │    │  └───────────────┘           └──────────────┘      │    │
  │    │                                                    │    │
  │    └────────────────────────────────────────────────────┘    │
  │                                                              │
  └──────────────────────────────────────────────────────────────┘
```

There will be no change to the overlay path taken by the east-west traffic
between the pods.

The following north-south traffic paths will not be affected either:

* Overlay pod to external
  * This traffic path is the same as normal local gw mode since the egress
    traffic will leave OVN-K virtual topology via mp0. Then it will be
    masqueraded by the host iptables rule and then be sent out from eth0.
* Host to ClusterIP services backed by overlay pods
* Host to NodePort services backed by overlay pods

Since we will use a static IP address (10.44.0.0) on br-ex other than the NodeIP
(10.89.0.3) which is on eth0. OVN-K will treat the NodeIP as an external IP. The
endpoint IP of service backed by hostNetwork pod will still use NodeIP. Also the
ingress traffic to NodePort Service will still use the NodeIP as the dest_ip.
Therefore the following traffic cases will be affected:

* Host to ClusterIP services backed by host pods
* Host to NodePort services backed by host pods
* External to NodePort services
* External to LoadBalancer services

### Host to ClusterIP Services Backed by Host Pods

The service and endpoint are:

```bash
[root@rhel92 microshift]# oc get svc nginx-host
NAME         TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)        AGE
nginx-host   NodePort    10.43.117.48   <none>        80:30847/TCP   5d21h
[root@rhel92 microshift]# oc get ep nginx-host
NAME         ENDPOINTS            AGE
nginx-host   10.89.0.3:8080       5d21h
```

The general flow is:
1. As OVN-K adds the following static route in the host routing table for the
   Kubernetes service IP range via br-ex interface. Kernel will pick the IP of
   br-ex as the source IP. The packet is sent to the OVN gateway router via
   br-ex with src_ip (10.44.0.0), dest_ip(10.43.117.48).

   ```none
   10.43.0.0/16 via 169.254.169.4 dev br-ex mtu 1400
   ```

2. In br-ex,the packet will be SNATed to 169.254.169.2, then sent to the OVN
   Gateway Router

   ```none
   cookie=0xdeff105, duration=17970.101s, table=0, n_packets=4222, n_bytes=388436, priority=500,ip,in_port=LOCAL,nw_dst=10.43.0.0/16 actions=ct(commit,table=2,zone=64001,nat(src=169.254.169.2))
   cookie=0xdeff105, duration=5.359s, table=2, n_packets=4237, n_bytes=394001, actions=mod_dl_dst:d2:a9:ab:af:55:37,output:"patch-br-ex_rhe"
   ```

3. The loadbalancer in the GR DNAT the packet endpoint 10.89.0.3:8080. The
   packet be SNATed to the GR interface IP (10.44.0.0).

   ```none
    1. lr_in_dnat (northd.c:10553): ct.new && !ct.rel && ip4 && reg0 == 10.43.117.48 && tcp && reg9[16..31] == 80, priority 120, uuid fb1a2cf8
       flags.force_snat_for_lb = 1;
       ct_lb_mark(backends=10.89.0.3.129:8080; force_snat);
   ```

4. According to the default route of the GR, the packet is send back to br-ex.
   
   ```none
   sh-5.2# ovn-nbctl lr-route-list GR_ovn-control-plane
   IPv4 Routes
   Route Table <main>:
                   0.0.0.0/0             169.254.169.4 dst-ip rtoe-GR_ovn-control-plane
               10.244.0.0/16                100.64.0.1 dst-ip
   ```

5. In br-ex, the packet will be SNAT to 169.254.169.1, and send to local host.
   The packet will hit the following openflow rules:
   
   ```none
   cookie=0xdeff105, duration=48397.416s, table=0, n_packets=6, n_bytes=928, priority=500,ip,in_port="patch-br-ex_rhe",nw_src=10.44.0.0,nw_dst=10.89.0.3 actions=ct(commit,table=4,zone=64001)
   cookie=0xdeff105, duration=48397.416s, table=4, n_packets=6, n_bytes=928, ip actions=ct(commit,table=3,zone=64002,nat(src=169.254.169.1))
   cookie=0xdeff105, duration=10.170s, table=3, n_packets=12647, n_bytes=1517745, actions=move:NXM_OF_ETH_DST[]->NXM_OF_ETH_SRC[],mod_dl_dst:9e:0c:05:73:23:54,LOCAL
   ```

6. The packet enters the host kernel, then be processed by the hostNetwork pod.

#### Reply

1. the hostNetwork pod sends the reply packet with src_ip(10.89.0.3) and
   dest_ip(169.254.169.1).

2. In the host routing table there's a static route that forward the traffic to
   169.254.169.1 via br-ex
   
   ```none
   169.254.169.1 dev br-ex src 10.89.0.3
   ```

3. In br-ex, the packet will be unDNAT to 10.44.0.0 then forwarded to OVN
   gateway router
   ```none
   cookie=0xdeff105, duration=48397.416s, table=0, n_packets=5, n_bytes=1974, priority=500,ip,in_port=LOCAL,nw_dst=169.254.169.1 actions=ct(table=5,zone=64002,nat)
   cookie=0xdeff105, duration=48397.416s, table=5, n_packets=5, n_bytes=1974, ip actions=ct(commit,table=2,zone=64001,nat)
   cookie=0xdeff105, duration=10.170s, table=2, n_packets=16588, n_bytes=1428836, actions=mod_dl_dst:9e:0c:05:73:23:54,output:"patch-br-ex_rhe"
   ```
4. The loadbalancer in OVN GR will then handle unSNAT, unDNAT to src_ip
   (10.43.117.48), dest_ip(169.254.169.2) and send the packet back to br-ex.

5. br-ex will unDNAT and send the packet to localhost.
   
   ```none
   cookie=0xdeff105, duration=13742.995s, table=0, n_packets=2198, n_bytes=306654, priority=500,ip,in_port="patch-br-ex_rhe",nw_src=10.43.0.0/16,nw_dst=169.254.169.2 actions=ct(table=3,zone=64001,nat)
   cookie=0xdeff105, duration=14.962s, table=3, n_packets=2198, n_bytes=306654, actions=move:NXM_OF_ETH_DST[]->NXM_OF_ETH_SRC[],mod_dl_dst:16:38:a3:9b:fd:6b,LOCAL
   ```
   
### Host to NodePort Services Backed by Host Pods

This traffic case is almost the same as the previous one. The only difference is
that before the packet is sent to br-ex, it will be DNATed to the service's
clusterIP by host iptables rule.

```none
-A OVN-KUBE-NODEPORT -p tcp -m addrtype --dst-type LOCAL -m tcp --dport 30847 -j DNAT --to-destination 10.43.117.48:80
```

### External to NodePort Services / LoadBalancer services

The general flow is:

1. The packet enters host via eth0.
2. In the host, the packet is DNATed to the service's clusterIP by the following iptables rule.

To nodePort services:

```none
-A OVN-KUBE-NODEPORT -p tcp -m addrtype --dst-type LOCAL -m tcp --dport 30847 -j DNAT --to-destination 10.43.117.48:80
```

To loadBalancer services:

```none
-A OVN-KUBE-EXTERNALIP -d 10.89.0.3/32 -p tcp -m tcp --dport 8081 -j DNAT --to-destination 10.43.117.48:80
```

3. The packet is forwarded to br-ex bridge.

The rest of the steps is the same as the normal local gateway mode.

### Workflow Description

#### Deploying

There will be no change to the MicroShift deployment procedure.

#### Upgrading

The microshift-ovs-init.service will be responsible for the OVS bridge change.
As we will not bind interface to br-ex bridge, the `gatewayInterface` flag
in `/etc/microshift/ovn.yaml` will be deprecated. Also, as the configuration of
OVS will not affect host physical interfaces, there is no need to allow user to
config the OVS bridges. The flag `disableOVSInit` can also be deprecated.

#### Configuring

There is no need to introduce any new configuration flag for this change. Unless
we want to allow user to specify the IP address used by the br-ex interface.

#### Deploying Applications

Applications are deployed as usual.

#### Variation [optional]

N/A

### API Extensions

N/A

### Implementation Details/Notes/Constraints

On the OVN-Kubernetes side, we need to change the local gateway mode:

1. Allow using an external gateway bridge without uplink port
2. If there is no gateway available via br-ex, configure default route via
   169.254.169.4.

   ```none
   0.0.0.0/0             169.254.169.4 dst-ip rtoe-GR_ovn-control-plane
   ```

On the MicroShift side, we will need to make the following change:

1. We need to update the microshift-ovs-init.service to 
   * Create br-ex bridge, and assign a static IP to it.
   * Since we don't need NetworkManager to manage the DHCP client. We can remove
   those logic from the configure-ovs.sh.
   * For upgrading, we also need to remove the NetworkManager connections
   created by configure-ovs.sh in previous version.

### Risks and Mitigations

The OVN-Kubernetes configuration described in this document is not used by any
other known platforms. Therefore there is no CI with this setup in
OVN-Kubernetes upstream. In order to minimize the risk of regression, we need to
improve the OVN-Kubernetes CI to cover this setup.

### Drawbacks

The egress features that rely on sending packets to wire without passing kernel
will not work. As there is no uplink port to forward those packets to. So
egressGW and egressIP will not work. A possible solution is to forward those
traffic to the host kernel, then adding iptables rules to steer the traffic to
wire. We will update the egress IP status with an error message until we fix it.

With this change, the ovn node annotations such
`k8s.ovn.org/node-primary-ifaddr` and `k8s.ovn.org/l3-gateway-config` will use
the local IP of br-ex as the node IP, instead of the IP of host's primary
interface. Therefore the following features that need to read these node
annotation will be affected. The egress rules of Egress firewall contains the
node IP. And the traffic case of services with `internalTrafficPolicy=Local`
backed by hostNetwork pods will break. For services with
`internalTrafficPolicy=Local`, ovn-k will only put local host endpoints to the
node switch loadbalancer. If the pod is hostNetwork, the local endpoint means
the pod with the NodeIP. Normally, the ovn-k will use the nodeIP as the ovn-k
external IP in node annotation `k8s.ovn.org/l3-gateway-config`. However, in our
case, as we don't bind host interface to br-ex, we have different IPs for nodeIP
and ovn-k external IP. So ovn-k cannot identify the right local endpoint in such
a case. It then put no endpoint to the load-balancer.This problem is not so
critical to MicroShift. Since we only get one node in the cluster. Creating a
service with `internalTrafficPolicy=Local` makes no sense in such a case. We can
fix this problem later until we find a proper solution.

However, the aforementioned features/traffic-case are not required in
MicroShift. So we want to make the work first and leave advanced features till
later.

## Design Details

### Open Questions [optional]

1. We'll need to allocate a IP address to br-ex bridge. This IP address will not
   be exposed externally. Shall we use a fixed IP address, or we allow user to
   be able to specify it?

   A: We decide to reuse the `advertise-address` of kubernetes API server by add
   this address to br-ex. Users can specify the address with the config flag
   `apiServer.advertiseAddress` in MicroShift. The default value is `10.44.0.0`.

### Test Plan

The current tests we have are sufficient for testing this networking change.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

When doing upgrade, the microshift-ovs-init.service will be responsible for
deleting the br-ex bridge and NetworkManager config files that is created by
previous version with NetworkManager.

Downgrade cannot be done automatically, users will need to manually remove the
br-ex bridge created by microshift-ovs-init.service.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

N/A

## Alternatives

There're 2 other alternatives that can also work.

### Modify the disabled gateway mode of OVN-Kubernetes

OVN-Kubernetes used to support a disabled gateway mode. In such mode the gateway
router and external switch will not be created for the node, therefore br-ex
bridge is unnecessary. Nodes in such mode will relay on other gateway nodes for
north-south traffic.

We can take advantage of this disabled gateway mode and make necessary change to
allow all north-south traffic to go through mp0 interface.

The pros of this solution are:
1. We will have a simpler OVN topology
2. The north-south traffic will take less hops in OVN, thus gets better
   performance.

The cons of this are:
1. It will introduce a new gateway mode to OVN-Kubernetes, which will increase
   the overall maintenance overhead.
2. The egress features like egressIP, egress gateway may not work.
3. We'll need to spend more time and resource to make the code change in
   OVN-Kubernetes.

### Create br-ex bridge with a dummy interface

The overall traffic paths will be same as the proposed solution. We don't need
to change the ovn-kubernetes to make it work. However, the dummy interface plays
no role in all the traffic paths. It will only act as the placeholder. So we'd
better change the local gateway mode to how it should work, instead of taking
a workaround to avoid code change.

## Infrastructure Needed [optional]

N/A