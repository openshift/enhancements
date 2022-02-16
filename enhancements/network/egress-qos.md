---
title: egress-qos
authors:
  - "@oribon"
reviewers:
  - "@trozet"
  - "@danwinship"
  - "@tssurya"
  - "@abhat"
approvers:
  - "@danwinship"
  - "@trozet"
  - "@knobunc"
api-approvers:
  - "@trozet"
creation-date: 2022-02-16
last-updated: 2022-04-14
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/SDN-2097
---

# OVN Pods Egress DSCP QoS

## Summary

Not all traffic has the same priority, and when there is contention for bandwidth, there should be a mechanism for objects outside the cluster to prioritize the traffic.
To enable this, we will use Differentiated Services Code Point (DSCP) which allows us to classify packets by setting a 6-bit field in the IP header, effectively marking the priority of a given packet relative to other packets as "Critical", "High Priority", "Best Effort" and so on.

By introducing a new CRD `EgressQoS`, users could specify a DSCP value for packets originating from pods on a given namespace heading to a specified CIDR.
The CRD will be Namespaced, with one resource allowed per namespace.
The resources will be watched by ovn-k, which in turn will configure OVN's [QoS Table](https://man7.org/linux/man-pages/man5/ovn-nb.5.html#QoS_TABLE). 

## Motivation

Telco customers require support for DSCP marking capability for some of their 5G applications, giving some pods precedence over others.
The QoS markings will be consumed and acted upon by objects outside of the OpenShift cluster to optimize traffic flow throughout their networks.

### Goals

- Provide a mechanism for users to set DSCP on egress traffic coming from specific namespaces.

### Non-Goals

- Ingress QoS.

- Consolidating with current `kubernetes.io/egress-bandwidth` and `kubernetes.io/ingress-bandwidth` annotations.
Nonetheless, the work done here does not interfere with the current bandwidth QoS mechanism.

- The DSCP marking does not need to be handled or acted upon by OpenShift, just added to selected headers.

- Marking East/West traffic, exposing the DSCP value from the inner packet to the outer geneve packet.

## Proposal

To achieve egress DSCP marking on pods, we introduce a new namespace-scoped CRD `EgressQoS` which allows specifying a set of QoS rules, each has a DSCP value, a destination CIDR and a PodSelector.
Traffic coming from pods on the namespace heading to each destination CIDR that match the selector will be marked with the corresponding DSCP value.
Not specifying a PodSelector will apply a rule to all pods in the namespace.

### Implementation Details/Notes/Constraints

A new API `EgressQoS` under the `k8s.ovn.org/v1` version will be added to `pkg/crd`.

A new controller in OVN-K will watch `EgressQoS`, `Pod` and `Node` objects, which will create the relevant QoS objects and attach them to all of the node local switches in the cluster in OVN - resulting in the necessary flows to be programmed in OVS.

In order to not create an OVN QoS object per pod in the namespace, the controller will also manage AddressSets. 
For each QoS rule specified in a given `EgressQoS` it'll create an AddressSet, adding only the pods
whose label matches the PodSelector to it, making sure that new/updated/deleted matching pods are also added/updated/deleted accordingly.
Rules that do not have a PodSelector will leverage the namespace's AddressSet.

For example, using LGW mode and assuming there's a single node `node1` and the following `EgressQoS` is created:

```yaml
kind: EgressQoS
apiVersion: k8s.ovn.org/v1
metadata:
  name: default
  namespace: default
spec:
  egress:
  - dscp: 46
    dstCIDR: 1.2.3.4/32
    podSelector:
      matchLabels:
        priority: Critical
  - dscp: 30
    dstCIDR: 0.0.0.0/0
```
the equivalent of:
```bash
ovn-nbctl qos-add node1 to-lport 1000 "ip4.src == <default_ns-egress-qos-1000 address set> && ip4.dst == 1.2.3.4/32" dscp=46
ovn-nbctl qos-add node1 to-lport 999 "ip4.src == <default_ns address set> && ip4.dst == 0.0.0.0/0" dscp=30
```
will be executed.
Creating a new Pod that matches the first rule's podSelector in the namespace results in its IPs being added to that rule's Address Set.

In addition it'll watch nodes to decide if further updates are needed, for example:
when another node `node2` joins the cluster, the controller will attach the existing
`QoS` object to its node local switch.

IPv6 will also be supported, given the following `EgressQoS`:
```yaml
apiVersion: k8s.ovn.org/v1
kind: EgressQoS
metadata:
  name: default
  namespace: default
spec:
  egress:
  - dscp: 48
    dstCIDR: 2001:0db8:85a3:0000:0000:8a2e:0370:7330/124
```
and a single pod with the IP `fd00:10:244:2::3` in the namespace, the controller will create the relevant QoS object that will result in a similar flow to this on the pod's node:
```bash
 cookie=0x6d99cb18, duration=63.310s, table=18, n_packets=0, n_bytes=0, idle_age=63, priority=555,ipv6,metadata=0x4,ipv6_src=fd00:10:244:2::3,ipv6_dst=2001:db8:85a3::8a2e:370:7330/124 actions=mod_nw_tos:192,resubmit(,19)
```

### User Stories
#### Story 1

As a user of OpenShift, I should be able to mark egress traffic coming from a specific namespace with a valid DSCP value.

### API Extensions

A new namespace-scoped CRD is introduced:

```go
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:path=egressqoses
// +kubebuilder::singular=egressqos
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// EgressQoS is a CRD that allows the user to define a DSCP value
// for pods egress traffic on its namespace to specified CIDRs.
// Traffic from these pods will be checked against each EgressQoSRule in
// the namespace's EgressQoS, and if there is a match the traffic is marked
// with the relevant DSCP value.
type EgressQoS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EgressQoSSpec   `json:"spec,omitempty"`
	Status EgressQoSStatus `json:"status,omitempty"`
}

// EgressQoSSpec defines the desired state of EgressQoS
type EgressQoSSpec struct {
	// a collection of Egress QoS rule objects
	Egress []EgressQoSRule `json:"egress"`
}

type EgressQoSRule struct {
	// DSCP marking value for matching pods' traffic.
	// +kubebuilder:validation:Maximum:=63
	// +kubebuilder:validation:Minimum:=0
	DSCP int `json:"dscp"`

	// DstCIDR specifies the destination's CIDR. Only traffic heading
	// to this CIDR will be marked with the DSCP value.
	// This field is optional, and in case it is not set the rule is applied
	// to all egress traffic regardless of the destination.
	// +optional
	DstCIDR *string `json:"dstCIDR,omitempty"`

	// PodSelector applies the QoS rule only to the pods in the namespace whose label
	// matches this definition. This field is optional, and in case it is not set
	// results in the rule being applied to all pods in the namespace.
	// +optional
	PodSelector metav1.LabelSelector `json:"podSelector,omitempty"`
}
```

### Test Plan

* Unit tests coverage

* IPv4/IPv6 E2E that validates egress traffic from a namespace is marked with the correct DSCP value by creating and deleting `EgressQoS`, setting up src pods and host-networked destination pods.
  * Traffic to the specified CIDR should be marked.
  * Traffic to an address not contained in the CIDR should not be marked.

### Risks and Mitigations
N/A
## Design Details
N/A
### Graduation Criteria

#### Dev Preview -> Tech Preview
Dev Preview: 4.11
Tech Preview: 4.12
#### Tech Preview -> GA
GA: 4.13
#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy
N/A
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

## Drawbacks
N/A
## Alternatives
N/A