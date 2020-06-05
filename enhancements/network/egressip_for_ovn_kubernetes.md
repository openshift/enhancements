---
title: Egress IP for OVN-Kubernetes
authors:
  - "@alexanderConstantinescu"
reviewers:
  - "@danwinship"
approvers:
  - TBD
creation-date: 2020-06-05
last-updated: 2019-06-16
status: provisional

---

# EgressIP for OVN-Kubernetes


## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

EgressIP is currently a feature for openshift-sdn and is planned to be implemented for OVN-Kubernetes and subsequently integrated 
in the 4.6 release of OpenShift. This feature implementation for OVN-Kubernetes requires feature parity with openshift-sdn, but 
also aims for a feature improvement which is detailed in this document. 

### Background

EgressIP, as it's currently implemented for openshift-sdn, allows traffic exiting a cluster namespace to have a fixed IP address. 
This IP is attached to the primary network interface on one of the cluster nodes. The assignment of this IP for openshift-sdn is 
a two step process. The cluster admin first needs to modify the `hostsubnet` object of the nodes he/she would like to act as "egress nodes", 
which they do by specifying `EgressIPs` or `EgressCIDRs` for that `hostsubnet`. Secondly they need to modify the `netnamespace` object 
with the field `EgressIPs`, which finally tells openshift-sdn that those `EgressIPs` for that namespace need to be assigned between 
the nodes the cluster admin modified in step one. Egress IPs must always be assigned manually to namespaces. But in the context of 
assignment to nodes, they can be manually assigned, or left to the plugin to assign. For automatic assignment to be done, the user 
needs to set `EgressCIDRs` or multiple `EgressIPs` on the `hostsubnet` object, else: the assignment is left up to the user's own 
management. Egress IP is managed in a highly available manner if automatic assignment is possible, meaning: if the node currently 
hosting that IP falls off the cluster, the network plugin re-assigns the egress IP to another cluster node as a fallback mechanism.

If we summarize what openshift-sdn concretely achieves by its two-step process, we have the following: 

1.1. It allows the cluster admin to have a direct say on which nodes act as egress nodes.
1.2. The assignment of egress IPs to namespaces is a manual process (defined by the cluster admin). The assignment of egress IPs to 
nodes is performed automatically by the network plugin. In the case of a node failure: the egress IPs hosted on that node are 
re-distributed to another node in the cluster which can host them.

All in all, the feature is quite complex as it introduces a lot of notions which obfuscate these two final functionalities/goals.  

This enhancement proposal focuses on how this feature will work with OVN-Kubernetes.

## Motivation

The primary motivation is making OVN-Kubernetes have feature parity with openshift-sdn. Secondary motivations come partially from 
product management: enhancing the feature by allowing cluster admins the freedom to control cluster egress traffic 
on pod/namespace level, and partially from us in the networking team; simplifying the usability for cluster admins and our 
maintenance of the code base which implements it.   

### Goals & Requirements

2.1. The egress IP functionality for OVN-Kubernetes needs to have feature parity with openshift-sdn, according to 1.1. and 1.2.
2.2. The egress IP functionality for OVN-Kubernetes should allow namespace selection when defining the IP of egress traffic.
2.3. The egress IP functionality for OVN-Kubernetes should allow pod selection within the namespaces selected in 2.2. when defining the IP of egress traffic. 
2.4. The egress IP functionality for OVN-Kubernetes should support single-stack IPv4/IPv6 as well as dual-stack, in line with global 
OpenShift requirements for all networking functionality for release 4.6.

This proposal details how OVN-Kubernetes goes about addressing these goals in the section [Proposal](#proposal)

## Proposal

We propose to make the assignment of egress IPs simpler by defining the following sequence of events for a user wishing to define an egress 
IP for their cluster.

As to satisfy 1.1. we propose that cluster admins tag the nodes they wish to use as egress nodes by labelling them with 
`k8s.ovn.org/egress-assignable: ""`. In case an `EgressIP` object is created and no nodes are tagged as such, the egress IP creation is 
invalidated. To rectify that: a cluster admin would have to label the nodes needed, OVN-Kubernetes will then synchronize the EgressIPs 
that were previously created and didn't have any nodes to be assigned to. This will not apply to any `EgressIP`s that were created while 
there was an assignable cluster node. 

As to satisfy 2.2. we propose adding a `NamespaceSelector.MatchLabels` field (following the convention for such a field for Kubernetes / OpenShift) 
to the `EgressIP.Spec`. This will be a required field for the `EgressIP` object, as omitting this field makes it logically impossible to assign
an egress IP to pods. 

As to satisfy 2.3. we propose adding a `PodSelector.MatchLabels` field (following the convention for such a field for Kubernetes / OpenShift) 
to the `EgressIP.Spec`. This will be an optional field and in case it is specified: will be additive with the `NamespaceSelector`. Not specifying 
this field will result in the egress IP being assigned to all pods in the namespace(s) which matched the `NamespaceSelector`. 

In summary: as to satisfy all goal/requirements we propose the following EgressIP object definition:

```
apiVersion: k8s.ovn.org/v1
kind: EgressIP
metadata:
  name: egressip
spec:
  egressIPs:
  - 192.168.126.11
  - 192.168.126.102
  podSelector:
    matchLabels:
      name: my-pod
  namespaceSelector:
    matchLabels:
      name: my-namespace
status:
  assignments:
  - node: nodeX
    egressIP: 192.168.126.11
  - node: nodeY
    egressIP: 192.168.126.102
```
The described object definition means that the following apply:

* The `EgressIP` object is not namespaced, it is defined "cluster-wide".
* The IPs 192.168.126.11 and 192.168.126.102 will be assigned as egress IPs to all pods which have the label `name=my-pod` in the 
namespace(s) matching the label `name=my-namespace`.

### Implementation Details [optional]

The following will apply for the assignment of egress IP:

4.1. Any egress IP can only be assigned to one node, and one node only, at any given moment in time. The reason for this is because the 
egress IP is attached to the primary interface on the node acting as a egress node. IPs need to be unique across local networks, hence duplicate 
IPs (as would be the case if two nodes have the same IP) cannot be allowed.  
4.2. An `EgressIP` object can not have multiple egress IP assignments on the same node.   
4.3. If multiple `EgressIP` objects match the same pod: the behavior is undefined, meaning; the user cannot be guaranteed which egress IP 
those pods will have, it can be any egress IP matching them. 
4.4. Egress IP assignment will always happen on the cluster node which can host that egress IP according to 4.1. and which has the lowest 
amount of egress IP assignments at the moment the `EgressIP` object is created. The reason for this is to balance the outgoing 
network traffic as much as possible and avoid possible bottlenecks.
4.5. The status will always be set with an EgressIP value and node (should it be assignable), even if an `EgressIP` is created, but matches no 
pods nore namespaces.
4.6. If a cluster node goes down, or the cluster admin decides to remove the `k8s.ovn.org/egress-assignable: ""` label from any cluster node which 
is currently hosting egress IP assignments: then those `EgressIP` object will be completely re-assigned following 4.1., 4.2., 4.3. and 4.4. 

### Notes/Constraints [optional]

The initial analysis of this feature enhancement does not highlight a lot of added complexity for OVN-Kubernetes, and 
simplifies the utilization of such a feature from the user's standpoint. 

Points 4.1. - 4.6. means: that if a user creates `EgressIP` objects **before** tagging nodes, then there is a risk that most
`EgressIP` objects will be assigned to the same egress node, should that node be able to host them. It would be impossible for OVN-Kubernetes 
to rectify that situation, as it cannot know how many egress nodes will be available to it, so it will try to assign everything in its queue to 
the first node it sees. It is thus up to the user to delete and re-create all objects should a lot of them have been created that way.
It is subsequently strongly advised that a user first labels all nodes needed, and then creates `EgressIP` objects.

### Risks and Mitigations

None

## Design Details

### Upgrade / Downgrade Strategy

None.

## Implementation History

- *v4.6* (proposed): egress IP for OVN-Kubernetes is implemented 

## Drawbacks

None seen.

## Alternatives

Sticking to feature parity with openshift-sdn (meaning: having a `EgressIPNode` object and `EgressIP` object and mimicking 
openshift-sdn's behaviour with `EgressCIDRs` and `EgressIPs`), but still supporting single/dual-stack egress IP. 

