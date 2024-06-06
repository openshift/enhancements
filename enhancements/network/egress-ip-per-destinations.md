---
title: egress-ip-per-destinations
authors:
- "@martinkennelly"
reviewers:
- "@trozet"
- "@kyrtapz"
approvers:
- "@trozet"
api-approvers:
- "@joelspeed"
creation-date: 2024-03-12
last-updated: 2024-03-12
tracking-link:
- "https://issues.redhat.com/browse/SDN-4454"
see-also:
- "NA"
replaces:
- "NA"
superseded-by:
- "NA"
---

# Egress IP per destination
## Summary
Today, we can use an `EgressIP` to describe the source IP for a pod if it selected by an `EgressIP` custom resource (CR) selectors. This includes namespace
and pod selectors. If multiple `EgressIP` CRs select the same set of pods, the behavior is undefined. This is because
we cannot reliably choose which source IP to use. One of the EgressIPs will be active and the others on stand-by.

This enhancement proposes adding a new selector for when we want to apply the `EgressIP`. This new selector will only apply
the `EgressIP` as the source IP to a set of pods communicating with destination IP if that destination IP is within predefined network CIDRs.

If the new destination traffic selector is specified for all `EgressIP` CRs that selected a set of pods and the destination networks selected do not overlap, then we
can allow a set of pods to have multiple source IPs depending on the destination address.

## Motivation
### User Stories

As a cluster tenant, I want my application traffic to egress a particular interface depending on the traffic destination address and must
have a distinct but consistent source IP for each interface.

As a cluster administrator, I want to easily configure which destination networks should have an `EgressIP` because the destination
networks may change, and I do not wish to have to update individual `EgressIP` custom resources for every update.

### Goals
- Allow a set of pods to have a consistent source IP depending on the destination
- Support shared and local gateway (aka `routingViaHost=true`) modes in-order to reduce asymmetry between the modes
- Backward compatibility with existing `EgressIP` API
- If the new API isn't defined, preserve existing behaviour

### Non-Goals
- Change existing `EgressIP` behavior when the new field is not defined

## Proposal

A new `TrafficSelector` field is added to `EgressIP` spec which uses Kubernetes label selector semantics
to select a new CRD called `EgressIPTraffic` which contains a list of destination networks. This list of networks defines when an `EgressIP` should
be used as source IP when an application attempts to talk to a destination address, and it is within one of the predefined networks
defined within a `EgressIPTraffic` CR.

The reason for not embedding the destination networks within each `EgressIP` CR is that it places a burden
on the cluster administrator to update all `EgressIP` CRs if the destination networks change.

With the Kubernetes label selector semantics, cluster administrators can define a set of destination networks that multiple `EgressIP`
CRs can consume.

### Semantics
#### No `TrafficSelector` selector defined
Existing `EgressIP` behaviour is conserved - any pod selected must have the Egress IP defined as source IP when a packet is egress-ing the node
regardless of the destination IP.

#### `TrafficSelector` is defined but selects zero `EgressIPTraffic` or `EgressIPTraffic` CRs with zero destination networks defined
Any selected pod must have the node IP as source IP when egress-ing the node.

#### Single EgressIP CR selects multiple `EgressIPTraffic` CRs
If greater than one `EgressIPTraffic` CRs are selected, the destination networks are added together, and if a destination IP is within one
of the defined destination networks, then the pods source IP will be the Egress IP.

#### Multiple `EgressIP` CRs selecting a set of pods where one of the `EgressIP` CR defines no `TrafficSelector` selector and one `EgressIP` does
Whichever is the first `EgressIP` CR to become active is going to control the source IP for the pods when traffic is egress-ing the node.
The other EgressIP CRs are in stand-by mode. The order of which `EgressIP` will become active first is undefined.

#### Multiple `EgressIP` CRs selecting a set of pods, where multiple `EgressIP` CRs select one or more EgressIPTraffic and the set of destination networks do not overlap
The source IP of the packet leaving an egress node will be the defined Egress IP if the destination IP is within one
of the selected `EgressIPTraffic` CR destination networks. If it is not within one of the destination networks, a packet
will egress the node the pod resides on with a source IP equal to the nodes IP.

#### Multiple EgressIP CRs select one or more EgressIPTraffic and the set of destination networks overlap partially or fully
The semantics will remain the same as today if multiple `EgressIP` CRs select overlapping set of pods. One of the `EgressIP` CRs will
be active, and the others will be on standby.

### Workflow Description

**cluster admin** creates a set of namespaces for tenants to use.
Tenant has requested that its applications need a consistent source IP
for 2 different traffic paths - one health signal traffic and one for work load traffic.
The cluster admin updates a dedicated egress node by attaching two new networks on separate interfaces.
The cluster admin labels this node egress-able.
The cluster admin creates two `EgressIPTraffic` CRs, specifies a destination network in each and labels each CR differently.
The cluster admin creates two `EgressIP` CRs each containing an IP that falls within the previously added networks range.
The two `EgressIP`s selects the same set of pods but different `TrafficSelector`s to select the previously created `EgressIPTraffic` CRs.
The `EgressIP`s are automatically assigned to the secondary host interfaces on the egress node.

The cluster admin gives permissions to the **application admin** to the namespaces.

The **application admin** deploys its application and the application has a consistent but different source IP for the
health signal traffic and the work load traffic.

### API Extensions

Add `TrafficSelector` field to `EgressIPSpec`:

```golang
// EgressIPSpec is a desired state description of EgressIP.
type EgressIPSpec struct {
	// EgressIPs is the list of egress IP addresses requested. Can be IPv4 and/or IPv6.
	// This field is mandatory.
	EgressIPs []string `json:"egressIPs"`
	// TrafficSelector applies the egress IP only to the network traffic defined within the selected EgressIPTraffic(s).
	// If not set, all egress traffic is selected.
	// +optional
	// +openshift:enable:FeatureGate=EgressIPPerDestination
	TrafficSelector metav1.LabelSelector `json:"trafficSelector"`
	// NamespaceSelector applies the egress IP only to the namespace(s) whose label
	// matches this definition. This field is mandatory.
	NamespaceSelector metav1.LabelSelector `json:"namespaceSelector"`
	// PodSelector applies the egress IP only to the pods whose label
	// matches this definition. This field is optional, and in case it is not set:
	// results in the egress IP being applied to all pods in the namespace(s)
	// matched by the NamespaceSelector. In case it is set: is intersected with
	// the NamespaceSelector, thus applying the egress IP to the pods
	// (in the namespace(s) already matched by the NamespaceSelector) which
	// match this pod selector.
	// +optional
	PodSelector metav1.LabelSelector `json:"podSelector,omitempty"`
}
```

New CRD:

```golang
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:nostatus
// +kubebuilder:resource:shortName=eipt,scope=Cluster
// +kubebuilder:printcolumn:name="DestinationNetworks",type=string,JSONPath=".spec.destinationNetworks"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +openshift:enable:FeatureGate=EgressIPPerDestination
// EgressIPTraffic defines a set of networks outside the cluster network.
type EgressIPTraffic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec EgressIPTrafficSpec `json:"spec"`
}

// EgressIPTrafficSpec defines the desired state description of EgressIPTraffic.
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type EgressIPTrafficSpec struct {
	// DestinationNetworks is the list of Network CIDRs that defines external destinations
	// for IPv4 and/or IPv6.
	DestinationNetworks []CIDR `json:"destinationNetworks,omitempty" validate:"omitempty"`
}

// CIDR is a network CIDR. IPv4 or IPv6.
// +kubebuilder:validation:XValidation:rule="isCIDR(self)",message="CIDR must be in valid IPV4 or IPV6 CIDR format"
type CIDR string

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// EgressIPTrafficList contains a list of EgressIPTraffic
type EgressIPTrafficList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []EgressIPTraffic `json:"items"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes
Nothing new that's specific for HCP.

#### Standalone Clusters
Nothing specific to standalone.

#### Single-node Deployments or MicroShift
Nothing specific for these deployments.

### Implementation Details/Notes/Constraints

#### Cluster manager
No changes to `EgressIP` node assignment or health checks therefore no changes.

#### OVNKube-controller (OVNKube-node)
If a user defines an `EgressIP` CR and doesn't populate `TrafficSelector` then `EgressIP` behaviour is the same as today (without this proposed feature).
All traffic egress-ing the cluster will be SNAT'd.

If a user defines an `EgressIP` CR and populates the `TrafficSelector` field with a selector, and it matches or does not match an `EgressIPTraffic`
CR but regardless, there are no defined networks among the selected `EgressIPTraffic` CRs, then no cluster external traffic is SNAT'd to the Egress IP. This
matches the behaviour of other "selectors" in `EgressIP`.

##### Common amongst gateway "modes"
If a user defines an `EgressIP` CR and the `TrafficSelector` is populated:-
1. Create an address set managed and consumed specifically by said `EgressIP` CR. This address set will contain all the destination networks
   that maybe defined in one or more `EgressIPTraffic` CRs
   This implementation detail is necessary because we need to limit the number of address sets to one for any selected set of pod because OVN NAT field `allowed_ext_ips`
   allows setting one address set. We cannot set more than one address set to this field.
   Also, it limits the inclusion to one extra OVN LRP match condition for the selected pods for the OVN Logical Router `ovn_cluster_router`.
   i.e. `ip4.src == $pod_ip && ip4.dst == $address_set_name ..`
2. For each of the selected `EgressIPTraffic` CRs, add, if any, the networks defined to the address set mention in set 1
3. For each selected pod, create an OVN Logical Route Policy entry for OVN Logical Router `ovn_cluster_router` at the
   same priority as existing egress IP LRPs, with a match criteria of source IP equalling a selected pod IP and
   destination IP equal to the name of the address set defined in step 1 and an action of reroute. The OVN action reroute value
   doesn't change with proposal

##### Egress IP assigned to the primary interface
###### Shared gateway / local gateway (aka routingViaHost=true)
Note: Currently, for OCP 4.15, traffic paths for Egress IPs do not alter when an Egress IP is assigned to the primary interface
for shared and local gateway modes - traffic for selected pods always egress via a nodes OVN gateway Logical Router and is not sent to the host
networking stack in local gateway mode.

4. On the egress node, create a new OVN NAT entry and populate the `allowed_external_ips` with the address set name created in step 1
5. On the egress node, add the newly created NAT entry to the egress nodes gateway OVN Logical Router NATs
6. If any selected `EgressIPTraffic` CRs gets created, updated or deleted, then we find any `EgressIP` CRs that select it and ensure
that set of `EgressIP` CRs reconciles

A sync function is required for the aforementioned "destination networks" OVN address sets for the case of any events missed.

##### Egress IP assigned to a host secondary interface
###### Shared gateway
4. On the egress node, for each destination network, create an iproute2 IP rule including `from` pod IP and `to` equaling the network with a lookup
for the custom routing table
5. IPTables configuration remains unchanged

### Risks and Mitigations
Managing a new CRD introduces some risk and this risk would be reduced if the information in that new CRD
is including in the `EgressIP` CRD. i.e. embed the destination networks within the `EgressIP` CRD.
This design choice is taken for user experience and future extensibility.

### Drawbacks
As the routing becomes more complex in the future, with *possibly* more traffic selectors, we probably should use pkt marks and keep the
routing logic exclusively in OVN and only use iproute2 ip rules to act on the marks instead of this proposal which is checking
src and dst. This approach with iproute2 ip rules doesn't make it easy to add more traffic selectors.

Possible premature optimisation by creating a new CRD that's scoped for traffic filtering instead of just for destination address filtering.

## Test Plan
- Unit tests and e2es. E2Es current test framework provide one "external" endpoint. In-order to fully test this feature,
we will need multiple external endpoints
- QE test plan and regression tests

## Graduation Criteria
| OpenShift | Maturity     |
|-----------|--------------|
| 4.17      | Tech Preview |
| 4.18      | GA           |

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA
4.18

### Removing a deprecated feature
- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy
Upgrade expectations:
No disruption to existing `EgressIP` functionality or performance during update

Downgrade expectations:
New field, and we do not expect the user to utilize this feature during an upgrade, therefore, no downgrade issues as the
feature will not be active.

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions
- No status is planned to be included in the new CRD `EgressIPTraffic`
- Expected to very slightly increase CPU/memory usage on every node watch a new CRD and process its changes
- No impact on scalability
- Health can be determined via k8 events / logs / metrics (retry logic failure when configuring LRPs)

## Support Procedures
If you do not see the SNAT to the defined egress IP, then check the network operator status and ensure there are no issues.
Following this, find the node you expect the egress IP to be assigned to by checking the `EgressIP` status.
Then, lookup the OVN Kubernetes pod running on that node and for container `ovnkube-controller`, grep for the `EgressIP`
CR name. If no obvious error, lets confirm OVN Kubernetes created the necessary artifacts in OVN for a
pod selected by the `EgressIP`.

Use `oc` to `rsh` into the OVN Kubernetes pod on that node and ensure there is an OVN Logical Route Policy in Logical Router `ovn_cluster_router`:
  ```shell
  sh-5.2# ovn-nbctl lr-policy-list ovn_cluster_router
  ...
  100 ip{4|6}.src == $POD_IP && ip{4|6}.dst == $ADDRESS_SET_NAME          reroute $GR_PORT_IP (primary inf) or $MGNT_PORT_IP (host secondary inf)
  ...
  ```
  Confirm the network(s) you defined in `EgressIPTraffic` CR(s) and selected by your `EgressIP` traffic selector, are present in the address
  set seen previously:
  ```shell
  sh-5.2# ovn-nbctl find address_set name=$ADDRESS_SET_NAME
  _uuid               : 46b985b9-8869-474a-b17a-2ed36db45cc8
  addresses           : [$NETWORK_CIDR_1,$NETWORK_CIDR_2,...]
  external_ids        : {...}
  name                : $ADDRESS_SET_NAME
  ```
- If `EgressIP` is assigned to primary interface:
  Confirm the Egress IP assigned node OVN gateway Logical Router contains a NAT where the `logical_ip` is the selected $POD_IP:
  ```shell
  sh-5.2# ovn-nbctl lr-nat-list GR_$NODE_NAME
  TYPE             GATEWAY_PORT          EXTERNAL_IP        EXTERNAL_PORT    LOGICAL_IP          EXTERNAL_MAC         LOGICAL_PORT
  ...
  snat                                   $EGRESS_IP                           $POD_IP
  ...
  ```
  Inspect the created NAT to ensure it contains the address set found previously, and it is set to the field `allowed_ext_ips`:
  ```shell
  sh-5.2# ovn-nbctl find NAT logical_ip=$POD_IP
  ...
  allowed_ext_ips     : 46b985b9-8869-474a-b17a-2ed36db45cc8
  ...
  type                : snat
  ```
- If `EgressIP` is assigned to a host secondary interface:
  Ensure the correct iproute2 IP rules are present for a selected pod. If `EgressIP` used a traffic selector that selects one or
  more `EgressIPTraffic` CRs that contain a set of network CIDRs A-Z, then expect a list of IP rules for each network CIDR
  ```shell
  sh-52# ip (-6) rule
  ...
  60000:	from $pod_ip to $NETWORK_CIDR_A lookup 4567
  ...
  60000:	from $pod_ip to $NETWORK_CIDR_A lookup 4567
  ...
  ```

## Alternatives
https://docs.google.com/document/d/1_mhEzjVEEkbMl6k0OKP4rQPxd0eVe7OVuxVmHu29qEA

Drawbacks: Not backward compatible. Change in existing behaviour for routing.

## Infrastructure Needed [optional]
N/A
