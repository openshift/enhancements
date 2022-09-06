---
title: service-egress-traffic-steering
authors:
  - "@oribon"
reviewers:
  - "@trozet"
  - "@danwinship"
  - "@SchSeba"
  - "@fedepaol"
approvers:
  - "@danwinship"
  - "@trozet"
api-approvers:
  - "@trozet"
creation-date: 2022-07-28
last-updated: 2022-07-28
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/SDN-2682
---

# OVN service egress traffic steering

## Summary

Some external systems that communicate with applications running on the Kubernetes cluster through a LoadBalancer service expect that the source IP of egress traffic originating from the pods backing the service is identical to the destination IP they use to reach them - i.e the LoadBalancer's ingress IP.
This behavior requires that service access from outside the cluster will only work to a designated node. This is contradictory to how default services work where a service may be accessed via any node.

By annotating a LoadBalancer service, users could request that the source IP of egress packets originating from all of the pods that are endpoints of it would be its ingress IP.

## Motivation

Telco customers rely on applications that may initiate traffic bidirectionally over a connection, and thus expect the IP address of the client to be the same in both cases. A common use case is Telco applications that exist outside of the cluster and need to communicate with one or more pods inside the cluster using a service.
Therefore they require that when one of these pod applications initiate traffic towards the external application that it also uses the same IP address.

### Goals

- Provide a mechanism for users running OVN-Kubernetes to request that packets originating from pods backing a specified LoadBalancer service will use the service's ingress IP as their source IP.

### Non-Goals

- Support host-networked pods.

- Announcing the service externally (for service ingress traffic) with OVN-Kubernetes - this part should be handled by a LoadBalancer provider (like MetalLB) as explained later.

- Using the service's ingress IP for pod to pod traffic.

## Proposal

Only SNATing a pod's IP to the LoadBalancer service ingress IP that it is backing is problematic, as usually the ingress IP is exposed via multiple nodes by the LoadBalancer provider. This means we can't just add an SNAT to the regular traffic flow (in LGW mode) of a pod which is:
`pod -> ovn_cluster_router -> node's mgmt port -> host routing table -> host iptables -> exit through an interface` because we don't have a guarantee that the reply will come back to the pod's node (where the traffic originated).
An external host usually has multiple paths to reach the LoadBalancer ingress IP and could reply to a node that is not the pod's node - in that case, the other node does not have the proper CONNTRACK entries to send the reply back to the pod and the traffic is lost.
For that reason, we need to make sure that all traffic for the service's pods (Ingress/Egress) is handled by a single node so the right CONNTRACK entries are always matched and the traffic is not lost.

The egress part will be handled by OVN-Kubernetes, which will choose a node that acts as the point of ingress/egress, and steer the relevant pods' egress traffic to its mgmt port, by using logical router policies on the ovn_cluster_router.
When that traffic reaches the node's mgmt port it will use its routing table and iptables.
Because of that, it will also take care of adding the necessary iptables rules on the selected node to SNAT traffic exiting from these pods to the service's ingress IP. The node will also be labeled with `egress-service.k8s.ovn.org/<svc-namespace>-<svc-name>: ""`, which can be consumed by a LoadBalancer provider to handle the ingress part.

The ingress part will be handled by a LoadBalancer provider, such as MetalLB, that would need to select the right node (and only it) for announcing the LoadBalancer service (ingress traffic) according to the `egress-service.k8s.ovn.org/<svc-namespace>-<svc-name>: ""` label set by OVN-Kubernetes.
Taking MetalLB as an example for a LoadBalancer provider, the user will need to create their `L2Advertisement` and/or `BGPAdvertisements` with the `nodeSelector` field pointing to that label. That way only the node holding the label will be used for announcing the LoadBalancer service ingress IP.
It is worth noting that in MetalLB's case, a given LoadBalancer service can be announced by multiple L2 and BGP advertisements, possibly being (even accidently) announced from multiple nodes. For our use-case the user MUST take care of configuring their MetalLB resources in a way that the service is announced only by the node holding the label - a full example is detailed in [Usage Example](#Usage-Example).

To achieve these goals, we introduce a new annotation for users to set on LoadBalancer services: `k8s.ovn.org/egress-service`, which can be either empty or contain a `nodeSelector` field: `'{"nodeSelector":{"matchLabels":{"color": "green"}}}'` that allows limiting the nodes that can be selected to handle the service's traffic.
By specifying the `nodeSelector` field, only a node whose labels match the specified selectors can be selected for handling the service's traffic as explained earlier.
By not specifying the `nodeSelector` field any node in the cluster can be chosen to manage the service's traffic.

### Implementation Details/Notes/Constraints

A new annotation `k8s.ovn.org/egress-service` is supported for LoadBalancer services.
When `ovnkube-master` detects that a LoadBalancer service has this annotation it will elect a node to act as the point for all of the traffic of that service (ingress/egress). If the annotation contains valid LabelSelectors in its `nodeSelector` field only a node whose labels match the selectors can be elected.
The specified selectors have to match at least one of the nodes in the cluster, otherwise we don't configure anything.
If the `nodeSelector` field is not specified any node can be elected.

After choosing a node, it will create a logical router policy on the ovn_cluster_router for all of the endpoints of the service to steer their egress traffic to that node's mgmt port.
We should take extra care with these policies to not break east-west traffic, using the same allow policies as EgressIP with a higher priority.

For example when 10.244.0.3 and 10.244.1.6 are the endpoints of the service, the elected node's mgmt port is 10.244.0.2 and the cluster nodes are 172.18.0.3 and 172.18.0.4 we expect policies like these to be created:
```none
$ ovn-nbctl lr-policy-list ovn_cluster_router
       ...
       102      ip4.src == 10.244.0.0/16 && ip4.dst == 10.244.0.0/16           allow
       102      ip4.src == 10.244.0.0/16 && ip4.dst == 100.64.0.0/16           allow
       102      ip4.src == 10.244.0.0/16 && ip4.dst == 172.18.0.3/32           allow
       102      ip4.src == 10.244.0.0/16 && ip4.dst == 172.18.0.4/32           allow
       101      ip4.src == 10.244.0.3/32                                       reroute      10.244.0.2
       101      ip4.src == 10.244.1.6/32                                       reroute      10.244.0.2
       ...
```
After that the service will be annotated with `k8s.ovn.org/egress-service-host=<node_name>` and the node labeled with `egress-service.k8s.ovn.org/<svc-namespace>-<svc-name>: ""`.

When `ovnkube-node` detects that a LoadBalancer service has the `k8s.ovn.org/egress-service` annotation and it is running in the node specified in the service's `k8s.ovn.org/egress-service-host` annotation, it will add the relevant SNATs to the host's iptables for each of the service's endpoints.

For example when `ovn-worker` node matches the LabelSelectors specified in the `nodeSelector` field, 10.244.0.3 and 10.244.1.6 are the endpoints of the annotated LoadBalancer service "some-service" in the default namespace whose ingress IP is 172.19.0.100, we expect to see iptables rules like these in `ovn-worker`:
```none
$ iptables-save
*nat
-A POSTROUTING -j OVN-KUBE-EGRESS-SVC
-A OVN-KUBE-EGRESS-SVC -s 10.244.0.3/32 -m comment --comment default/some-service -j SNAT --to-source 172.19.0.100
-A OVN-KUBE-EGRESS-SVC -s 10.244.1.6/32 -m comment --comment default/some-service -j SNAT --to-source 172.19.0.100
```

After this, for a given service with these final annotations:
```none
$ kubectl describe svc some-service
Name:                     some-service
Namespace:                default
Annotations:              k8s.ovn.org/egress-service: {"nodeSelector":{"matchLabels":{"color": "green"}}} (set by user)
                          k8s.ovn.org/egress-service-host: "ovn-worker"                                      (set by ovn-k)
Type:                     LoadBalancer
LoadBalancer Ingress:     172.19.0.100
Endpoints:                10.244.0.3:8080,10.244.1.6:8080
```
the egress traffic flow for the pod `10.244.1.6` on `ovn-worker2` towards an external destination (172.19.0.5) will look like:
```none
                     ┌────────────────────┐
                     │                    │
                     │external destination│
                     │    172.19.0.5      │
                     │                    │
                     └───▲────────────────┘
                         │
     5. packet reaches   │                      2. router policy rereoutes it
        the external     │                         to ovn-worker's mgmt port
        destination with │                      ┌──────────────────┐
        src ip:          │                  ┌───┤ovn cluster router│
        172.19.0.100     │                  │   └───────────▲──────┘
                         │                  │               │
                         │                  │               │1. packet to 172.19.0.5
                      ┌──┴───┐        ┌─────▼┐              │   heads to the cluster router
                   ┌──┘ eth1 └──┐  ┌──┘ mgmt └──┐           │   as usual
                   │ 172.19.0.2 │  │ 10.244.0.2 │           │
                   ├─────▲──────┴──┴─────┬──────┤           │   ┌────────────────┐
4. an iptables rule│     │   ovn-worker  │3.    │           │   │  ovn-worker2   │
   that SNATs to   │     │               │      │           │   │                │
   the service's ip│     │               │      │           │   │                │
   is hit          │     │  ┌────────┐   │      │           │   │ ┌────────────┐ │
                   │     │4.│routes +│   │      │           └───┼─┤    pod     │ │
                   │     └──┤iptables◄───┘      │               │ │ 10.244.1.6 │ │
                   │        └────────┘          │               │ └────────────┘ │
                   │                            │               │                │
                   └────────────────────────────┘               └────────────────┘
                3. from the mgmt port it hits ovn-worker's
                   routes and iptables rules
```
As mentioned earlier, for the opposite direction (ingress/external client initiates) to work properly the LoadBalancer provider needs to announce the service only from `ovn-worker`.

### Node Selection
Selecting a node will work similarly to how EgressIP selects one.
When a LoadBalancer service is annotated with a valid `k8s.ovn.org/egress-service` annotation, a node is selected to be in charge of all of its traffic.
If the `nodeSelector` field is specified, only a node whose labels match the specified selectors can be selected.
A cache of the nodes and their number of allocations is kept in order to spread the allocations between all of the nodes available for a given service - selecting the node with the least amount of allocations every time.

Once a node is selected, it is checked for readiness to serve traffic every x seconds the same way EgressIP does for its nodes (TCP/gRPC).
If a node fails the health check, its allocations move to another node by removing the `egress-service.k8s.ovn.org/<svc-namespace>-<svc-name>: ""` label from it, removing the logical router policies from the cluster router and resetting the `k8s.ovn.org/egress-service-host=<node_name>` annotation on the service which causes it to be reconciled - causing a new node to be selected for the service.
If the node becomes not ready or its labels no longer match the service's selectors the same re-election process happens.

### Usage Example

With all of the above implemented, a user can follow these steps to create a functioning LoadBalancer service whose endpoints exit the cluster with its IP using MetalLB.

1. Create the IPAddressPool with the desired IP for the service. It makes sense to set `autoAssign: false` so it is not taken by another service - our service will request that pool explicitly. 
```yaml
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: example-pool
  namespace: metallb-system
spec:
  addresses:
  - 172.19.0.100/32
  autoAssign: false
```

2. Create the LoadBalancer service. We create it with 2 annotations:
- `metallb.universe.tf/address-pool` - to explicitly request the IP to be from the `example-pool`.
- `k8s.ovn.org/egress-service` - to request that all of the endpoints of the service exit the cluster with the service's ingress IP. We also provide `nodeSelector` so that the traffic exits from a node that matches these selectors.
```yaml
apiVersion: v1
kind: Service
metadata:
  name: example-service
  namespace: some-namespace
  annotations:
    metallb.universe.tf/address-pool: example-pool
    k8s.ovn.org/egress-service: '{"nodeSelector":{"matchLabels":{"color": "green"}}}'
spec:
  selector:
    app: example
  ports:
    - name: http
      protocol: TCP
      port: 8080
      targetPort: 8080
  type: LoadBalancer
```

3. Advertise the service from the node in charge of the service's traffic. So far the service is "broken" - it is not reachable from outside the cluster and if the pods try to send traffic outside it would probably not come back as it is SNATed to an IP which is unknown.
We create the advertisements targeting only the node that is in charge of the service's traffic using the `nodeSelector` field, relying on ovn-k to label the node properly.
```yaml
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: example-l2-adv
  namespace: metallb-system
spec:
  ipAddressPools:
  - example-pool
  nodeSelector:
  - matchLabels:
      egress-service.k8s.ovn.org/some-namespace-example-service: ""
---
apiVersion: metallb.io/v1beta1
kind: BGPAdvertisement
metadata:
  name: example-l2-adv
  namespace: metallb-system
spec:
  ipAddressPools:
  - example-pool
  nodeSelector:
  - matchLabels:
      egress-service.k8s.ovn.org/some-namespace-example-service: ""
```
While possible to create more advertisements resources for the `example-pool`, it is the user's responsibility to make sure that the pool is advertised only by advertisements targeting the node holding the `egress-service.k8s.ovn.org/<svc-namespace>-<svc-name>: ""` label - otherwise the traffic of the service will be broken.
### User Stories
As a user of OpenShift, I should be able to have functioning LoadBalancer services whose backing pods exit the cluster with the service's ingress IP.
#### Story 1

As a Telco customer who uses SNMP in OpenShift, I want to access pods that I'm managing using a LoadBalancer service. In order to do so, I need these pods to send traps using the same IP as I use for polling them.

### API Extensions
N/A

### Test Plan

- Unit tests coverage.
- E2E coverage by creating a LoadBalancer service with the proper annotations and validating that:
  - pod to pod traffic works.
  - external client to service works.
  - pod to external client works and is SNATed properly.


### Risks and Mitigations

- The solution might be a bit fragile as it relies on the user to configure the external advertisement of the service manually, with certain limitations.

- Using a single node to handle all of the traffic of a given service might be a bottleneck, and we will also need to try electing nodes in a way that spreads handling these kind of services between them. Failover must also be taken care of in case a node handling a service falls.

- As generally a Service's purpose is to serve ingress traffic, we might be missing some corner cases when using it also to shape egress traffic behavior.

- If a pod is an endpoint of multiple LoadBalancer services that request this functionality the behavior of the SNATs is undefined.

By stating all of the limitations to the user and with enough test coverage we can be confident that the feature is behaving properly for the main use-case.

## Alternatives

EgressIP already does some of the stuff described here, such as steering the traffic of multiple pods through a single node and SNATing their traffic to a single IP. However tying it to a service's ingress IP would require some degree of coordination between the service resource and the EgressIP resource (via a controller).

Also, in its current form EgressIP in baremetal clusters supports only IPs that belong to the primary interface's network (br-ex) and does not respect "Local Gateway Mode" in the sense that it doesn't use the host's routes and iptables.
To make it work for our use-case we'd have to refactor a lot of its functionalities for this feature alone, which might break/complicate the way users currently use it.

Having said that, the solution proposed here will probably share/reuse some of the code of EgressIP as they have some sort of similarity.
## Design Details
Since both EgressIP and this feature create logical router policies on the cluster router, the policies created here will use a higher priority than the EgressIP ones.
This means that if a pod is an endpoint of both an EgressIP and a LoadBalancer service annotated with a valid `k8s.ovn.org/egress-service` annotation the service's ingress IP will be used for the SNAT.

### Graduation Criteria


#### Dev Preview -> Tech Preview


#### Tech Preview -> GA


#### Removing a deprecated feature


### Upgrade / Downgrade Strategy


### Version Skew Strategy


### Operational Aspects of API Extensions


#### Failure Modes


#### Support Procedures


## Implementation History


### Drawbacks

### Workflow Description
