---
title: egress-qos
authors:
  - "@oribon"
  - "@pperiyasamy"
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
last-updated: 2024-03-08
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/SDN-2097
  - https://issues.redhat.com/browse/SDN-3152
---

# OVN Pods Egress DSCP QoS

## Summary

Not all traffic has the same priority, and when there is contention for bandwidth, there should be a mechanism for objects outside the cluster to prioritize the traffic.
To enable this, we will use Differentiated Services Code Point (DSCP) which allows us to classify packets by setting a 6-bit field in the IP header, effectively marking the priority of a given packet relative to other packets as "Critical", "High Priority", "Best Effort" and so on.

By introducing a new CRD `QoS`, users could specify a DSCP value for packets originating from pods on a given namespace heading to a specified CIDR, Protocol and Port.
The CRD will be Namespaced, with one resource allowed per namespace.
The resources will be watched by ovn-k, which in turn will configure OVN's [QoS Table](https://man7.org/linux/man-pages/man5/ovn-nb.5.html#QoS_TABLE).
The `QoS` also has `status` field which is populated by ovn-k which helps users to identify whether QoS rules are configured correctly in OVN or not.

## Motivation

Telco customers require support for DSCP marking capability for some of their 5G applications, giving some pods precedence over others.
The QoS markings will be consumed and acted upon by objects outside of the OpenShift cluster to optimize traffic flow throughout their networks.
Provide an ability to support other QoS features like Traffic Policing and Shaping in future.

### Goals

- Provide a mechanism for users to set DSCP on egress traffic coming from specific namespaces.
- Make QoS CRD fields to be more extensible.

### Non-Goals

- Ingress QoS.

- Consolidating with current `kubernetes.io/egress-bandwidth` and `kubernetes.io/ingress-bandwidth` annotations.
Nonetheless, the work done here does not interfere with the current bandwidth QoS mechanism.

- The DSCP marking does not need to be handled or acted upon by OpenShift, just added to selected headers.

- Marking East/West traffic, exposing the DSCP value from the inner packet to the outer geneve packet.

## Proposal

To achieve egress DSCP marking on pods, we introduce a new namespace-scoped CRD `QoS` which allows specifying a set of QoS rules, each has a DSCP value, a destination CIDR, Protcol, Port and a PodSelector.
Traffic coming from pods on the namespace heading to each destination CIDR, Protocol and Port that match the selector will be marked with the corresponding DSCP value.
Not specifying a PodSelector will apply a rule to all pods in the namespace.

### Implementation Details/Notes/Constraints

A new API `QoS` under the `k8s.ovn.org/v1` version will be added to `pkg/crd`.

A new controller in OVN-K will watch `QoS`, `Pod` and `Node` objects, which will create the relevant QoS objects and attach them to all of the node local switches in the cluster in OVN - resulting in the necessary flows to be programmed in OVS.

In order to not create an OVN QoS object per pod in the namespace, the controller will also manage AddressSets. 
For each QoS rule specified in a given `QoS` it'll create an AddressSet, adding only the pods
whose label matches the PodSelector to it, making sure that new/updated/deleted matching pods are also added/updated/deleted accordingly.
Rules that do not have a PodSelector will leverage the namespace's AddressSet.

For example, using LGW mode and assuming there's a single node `node1` and the following `QoS` is created:

```yaml
kind: QoS
apiVersion: k8s.ovn.org/v1
metadata:
  name: default
  namespace: default
spec:
  egress:
  - markingDSCP: 59
    classifier:
      cidr: 1.2.3.4/32
      protocol: "tcp"
      port: 22
      podSelector:
        matchLabels:
          priority: Critical
  - markingDSCP: 46
    classifier:
      cidr: 1.2.3.4/32
      podSelector:
        matchLabels:
          priority: Critical
  - markingDSCP: 30
    classifier:
      cidr: 0.0.0.0/0
```
the equivalent of:
```bash
ovn-nbctl qos-add node1 to-lport 1000 "ip4.src == <default_ns-qos address set> && ip4.dst == 1.2.3.4/32 && tcp && tcp.dst == 22" dscp=59
ovn-nbctl qos-add node1 to-lport 999 "ip4.src == <default_ns-qos address set> && ip4.dst == 1.2.3.4/32" dscp=46
ovn-nbctl qos-add node1 to-lport 998 "ip4.src == <default_ns address set> && ip4.dst == 0.0.0.0/0" dscp=30
```
will be executed.
The podSelector for first and second QoS rules are same. So creating a new Pod that matches this selector in the namespace results in its IPs
being added to corresponding Address Set.

In addition it'll watch nodes to decide if further updates are needed, for example:
when another node `node2` joins the cluster, the controller will attach the existing
`QoS` object to its node local switch.

IPv6 will also be supported, given the following `QoS`:
```yaml
apiVersion: k8s.ovn.org/v1
kind: QoS
metadata:
  name: default
  namespace: default
spec:
  egress:
  - markingDSCP: 48
    classifier:
      cidr: 2001:0db8:85a3:0000:0000:8a2e:0370:7330/124
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
// +kubebuilder:resource:path=qoses
// +kubebuilder::singular=qos
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// QoS is a CRD that allows the user to define a DSCP marking for pods
// egress traffic on its namespace to specified destination CIDRs,
// Protocol and Port.
// Traffic from these pods will be checked against each QoSRule in the
// namespace's QoS, and if there is a match the traffic is marked with
// the relevant DSCP value.
type QoS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Spec      `json:"spec,omitempty"`
	Status QoSStatus `json:"status,omitempty"`
}

// Spec defines the desired state of QoS
type Spec struct {
	// a collection of Egress QoS rule objects
	Egress []Rule `json:"egress"`
}

type Rule struct {
	// MarkingDSCP marking value for matching pods' traffic.
	// +kubebuilder:validation:Maximum:=63
	// +kubebuilder:validation:Minimum:=0
	MarkingDSCP int `json:"markingDSCP"`

	// +optional
	Classifier Classifer `json:"classifier"`
}

// Classifer The classifier on which packets should match
// to apply the QoS Rule.
type Classifer struct {
	// CIDR specifies the destination's CIDR. Only traffic heading
	// to this CIDR must match.
	// This field is optional, and in case it is not set the rule is applied
	// to all traffic regardless of the destination.
	// +optional
	// +kubebuilder:validation:Format="cidr"
	CIDR *string `json:"cidr,omitempty"`

	// protocol (tcp, udp, sctp) that the traffic must match.
	// +kubebuilder:validation:Pattern=^TCP|UDP|SCTP$
	// +optional
	Protocol string `json:"protocol"`

	// port that the traffic must match
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=65535
	// +optional
	Port int32 `json:"port"`

	// PodSelector applies the QoS rule only to the pods in the namespace whose label
	// matches this definition. This field is optional, and in case it is not set
	// results in the rule being applied to all pods in the namespace.
	// +optional
	PodSelector metav1.LabelSelector `json:"podSelector,omitempty"`
}

// QoSStatus defines the observed state of QoS
type QoSStatus struct {
	// A concise indication of whether the QoS resource is applied with success.
	// +optional
	Status string `json:"status,omitempty"`

	// An array of condition objects indicating details about status of QoS object.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}
```

### Test Plan

* Unit tests coverage

* Validate QoS `status` fields are populated correctly.

* IPv4/IPv6 E2E that validates egress traffic from a namespace is marked with the correct DSCP value by creating and deleting `QoS`, setting up src pods and host-networked destination pods.
  * Traffic to the specified CIDR should be marked.
  * Traffic to the specified CIDR, Protocol should be marked.
  * Traffic to the specified CIDR, Protocol and Port should be marked.
  * Traffic to an address not contained in the CIDR, Protocol and Port should not be marked.

### Risks and Mitigations
N/A
## Design Details
N/A
### Graduation Criteria

#### Dev Preview -> Tech Preview
Dev Preview: 4.11
Tech Preview: 4.12
#### Tech Preview -> GA
GA: 4.16
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