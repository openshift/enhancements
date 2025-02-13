---
title: user-defined-network-segmentation
authors:
  - "@ormergi"
reviewers:
  - "@maiqueb"
  - "@npinaeva"
approvers:
  - ""
api-approvers:
  - ""
creation-date: 2025-02-04
last-updated: 2025-02-04
tracking-link:
  - https://issues.redhat.com/browse/SDN-5519
---

# OVN-Kubernetes Localnet API

## Summary

Extend OVN-Kubernetes ClusterUserDefinedNetwork (CUDN) API to support localnet topology.

## Motivation

As of today one can create a user-defined network over localnet topology using NetworkAttachmentDefinition (NAD).
Using NAD for localnet has some pitfalls due to the fact it is not managed and not validated on creation.
Misconfigurations are detected too late causing bad UX and frustration for users.

Configuring localnet topology networks require changes to cluster nodes network stack and involve some risk and 
knowledge that require cluster-admin intervention.
Such as configuring the OVS switch to which the localnet network connects to. 

Managing localnet topology networks using a well-formed API using the CUDN CRD could improve UX as it is managed by a 
controller, perform validations and reflect the state via status.

### User Stories

#### Definition of personas:
Admin - is the cluster admin.
User - non cluster-admin user, project manager.
Workloads - pod or VMs (KubeVirt).

- As an admin I want to create a user-defined network over localnet topology using CUDN CRD. 
  - In case the network configuration is bad I want to get an informative message saying what went wrong.
- As an admin I want to enable users to connect workloads in project/namespaces they have permission to, to the localnet network I created for them.
- As a user I want to be able to connect my workloads (pod/VMs) to the localnet the admin created in my namespace.
- As a user I want my workloads to be able to communicate with each other over the localnet network.
- As a user I want my connected VMs to the localnet network to be able to migrate from one node to another, having its localnet network interface IP address unchanged.

### Goals

- Enable creating user-defined-networks over localnet topology using OVN-K CUDN CRD.
- Streamline localnet UX: detect misconfigurations early, communicate issues via informative status conditions and or events.

### Non-Goals

- This proposal does not cover changes for network configurations on day2 (mutable network configuration).

## Proposal

### Summary
Extend the CUDN CRD to enable creating user-defined networks over localnet topology.
Since the CUDN CRD is targeted for cluster-admin users, it enables preventing non-admin users performing changes that 
could disrupt the cluster or impact the physical network to which the workloads would connect to.

### Localnet using NetworkAttachmentDefinition
As of today OVN-K enables multi-homing including localnet topology networks using NADs
The following NAD YAML describe localnet topology configuration and options:

```yaml
---
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: tenantblue
  namespace: blue
spec:
    config: > '{
        "cniVersion": "0.3.1",
        "type": "ovn-k8s-cni-overlay"
        "netAttachDefName": "blue/tenantblue",
        "topology": "localnet",
        "name": "tenantblue",#1
        "physicalNetworkName": "mylocalnet1", #2
        "subnets": "10.100.0.0/24", #3
        "excludeSubnets": "10.100.50.0/32", #4
        "vlanID": "200", #5
        "mtu": "1500", #6
        "allowPersistentIPs": true #7
    }'
```
1. `name` 
   The underlying network name.
   - Should match the node OVS bridge-mapping network-name.
   - In case Kubernetes-nmstate is used, should match the `NodeNetworkConfigurationPolicy` (NNCP) `spec.desiredState.ovn.bridge-mappings` value.
2. `physicalNetworkName`
   Points to the node OVS bridge-mapping network-name - the network-name mapped to the node OVS bridge that provides access to that network.
   (Can be defined using Kubernetes-nmstate NNCP - `spec.desiredState.ovn.bridge-mappings`)
   - Overrides the `name` attribute.
   - Allows multiple localnet topology NADs to refer to the same bridge-mapping (thus simplifying the admin’s life - 
     fewer manifests to provision, and keep synced).
3. `subnets`
   Subnets to use for the network across the cluster.
4. `excludeSubnets`
   IP addresses range to exclude from the assignable IP address pool specified by subnets field.
5. `vlanID` - VLAN tag assigned to traffic.
6. `mtu` - maximum transmission unit for a network
7. `allowPersistentIPs`
    persist the OVN Kubernetes assigned IP addresses in a `ipamclaims.k8s.cni.cncf.io` object. This IP addresses will be 
    reused by other pods if requested. Useful for KubeVirt VMs.

### Extend ClusterUserDefinedNework CRD
Given the CUDN CRD is targeted for cluster-admin users, it is a good fit for operations that require cluster-admin intervention, 
such as localnet topology.

The suggested solution is to extend the CUDN CRD to enable creating localnet topology networks.

#### Underlying network name
Given the CUDN API doesn’t expose the underlying network name, represented by the NAD `spec.config.name` (net-conf-network-name), 
by design.
Localnet topology requires the net-conf-network-name to match the OVN bridge-mapping network-name on the node.
In case Kubernetes-nmstate is used, the NAD `spec.config.name` has to match the `NodeNetworkConfigurationPolicy` 
`spec.desiredState.ovn.bridge-mappings` name:
```yaml
spec:
  desiredState:
      ovn:
        bridge-mappings:
        - localnet: physnet  <---- has to match the NAD config.spec.name. OR 
                                   the NAD config.spec.physicalNetworkName
          bridge: br-ex <--------- OVS switch
```
* To overcome this, and avoid exposing the net-conf-network-name in the CUDN CRD spec a new field should be introduced.
The new field should allow users pointing to the bridge-mapping network-name they defined in the node.
The field should be translated to the CNI physicalNetworkName field.

#### MTU
By default OVN-K sets the pod interface with MTU 1400, unless otherwise specified.

For localnet topology this is not optimal because the host physical network-interface usually has default MTU 1500.
In case no MTU is specified the pod interface connected to the host localnet is configured with MTU 1400 resulting 
in bad performance and bad UX requiring troubleshooting the MTU alignment across the stack.

As long OVN-K does not set default MTU (1500) for localnet topology network, 
the controller should specify MTU to be 1500 explicitly in the NAD, if no MTU was specified in the CUDN spec.

### Workflow Description

The CUDN CRD controller should be changes accordingly to support localnet topology.
It should validate localnet topology configuration and generate corresponding NADs for localnet as other topologies (Layer2 & Layer3)
in the selected namespaces.

As of today the CUDN CRD controller does not allow mutating `spec.network`.
In order to comply with NAD spec mutation support, the CUDN CRD controller should allow spec.network mutation for 
localnet topologies, specifically: `MTU`, `VLAN`, `excludeSubnets` and `physicalNetworkName`.

In any order:
- The user configures localnet bridge mapping on the nodes, e.g.: using NNCP.
- Create CUDN, specifying the bridge-mapping network name in the spec.

#### Generating the NAD
##### OVS bridge-mapping’s network-name
Introduce attribute to point ovs-bridge bridge-mapping network-name.
This attribute name should be translated to the CNI “physicalNetworkName” attribute.

Proposal the CUDN spec field name:  “physicalNetworkName”.

Alternatively, the field name could be `name`.
In context of the localnet topology configuration it makes sense. For example: 
```yaml
spec:
  network:
    topology: Localnet
    localnet:
      name: "tenantblue"
```

##### MTU
Should be translated to the CNI “mtu” attribute.
If not specified the controller should specify default MTU (1500) in the NAD spec.config.

##### VLAN
Should be translated to the CNI “vlanID” attribute.
If not specified it should not be present in NAD spec.config.

##### Subnets
The subnets and exclude-subnets should be in CIDR form, similar to Layer2 topology subnets.

##### Persistent IPs
In a scenario of VMs, migrated VMs should have a persistent IP address to prevent disruption to the workloads it runs.
Localnet topology should allow using persistent IP allowing setting the CNI allowPersistentIPs.

As of today, the Layer2 topology configuration API consist of the following stanza allowing using persistent IPs, 
the localnet topology spec should  expose the same options:
```yaml
ipam:
  lifecycle: Persistent
```
### API Extensions

The API extension will be adding localnet topology support to the ClusterUserDefinedNetwork CRD.

####  CUDN spec

The CUDN `spec.network` follows the  [discriminated-union](https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md#discriminated-unions),
convention. 
The `spec.network.topology` serves as the union discriminator, it should accept `Localnet` option.

The API should have validation that ensures `spec.network.topology` match the topology configuration, similar to 
existing validation for other topologies.

#### Localnet topology spec 

| Field name          | Description                                                                                                                                                                                                                                                                                           | optional |
|---------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|
| Role                | Select the network role in the pod.                                                                                                                                                                                                                                                                   | No       |
| PhysicalNetworkName | The OVN bridge mapping network name is configured on the node.                                                                                                                                                                                                                                        | No       |
| MTU                 | The maximum transmission unit (MTU).                                                                                                                                                                                                                                                                  | Yes      |
| VLAN                | The network VLAN ID.                                                                                                                                                                                                                                                                                  | Yes      |
| Subnets             | List of CIDRs used for the pod network across the cluster.Dual-stack clusters may set 2 subnets (one for each IP family), otherwise only 1 subnet is allowed. The format should match standard CIDR notation (for example, "10.128.0.0/16"). This field must be omitted if `ipam.mode` is `Disabled`. | Yes      |
| ExcludeSubnets      | List of CIDRs removed from the specified CIDRs in `subnets`.The format should match standard CIDR notation (for example, "10.128.0.0/16"). This field must be omitted if `subnetes` is unset or `ipam.mode` is `Disabled`.                                                                            | Yes      |
| IPAM                | Contains IPAM-related configuration for the network. When ipam.lifecycle=Persistent enable workloads have persistent IP addresses. For example: Virtual Machines will have the same IP addresses along their lifecycle (stop, start migration, reboots).                                              | Yes      |

#### Suggested API validations 
- `Role`:
  - Required.
  - Can be Secondary only when topology=Localnet.
    - Having Role explicitly makes the API predictable and consistent with other topologies. In addition it enables extending localnet for Primary networks.
- `PhysicalNetworkName`: Required.
- `MTU`:
  - Minimum 576 (minimal for IPv4). Maximum: 65536.
  - When Subnets consist of IPv6 CIDR, minimum MTU should be 1280.
- `VLAN`: <br/>
  According to [dot1q (IEEE 802.1Q)](https://ieeexplore.ieee.org/document/10004498), 
  VID (VLAN ID) is 12-bits field, providing 4096 values; 0 - 4095. <br/>
  The VLAN IDs `0`, `1` and `4095` are reserved. <br/> 
  Suggested validations:
  - Minimum: 2, Maximum: 4094.
- `Subnets`:
  - Minimum items 1, Maximum items 2.
  - Items are valid CIDR (e.g.: "10.128.0.0/16")
  - When 2 items are specified they must be of different IP families.
- `ExcludeSubnets`:
  - Minimum items 1, Maximum items - 25.
  - Items are valid CIDR (e.g.: "10.128.0.0/16")
  - Cannot be set when Subnet is unset or `ipam.mode=Disabled`.

#### YAML examples
Assuming we have defined NNCP with the following bridge mapping
```yaml
desiredState:
    ovn:
      bridge-mappings:
      - localnet: tenantblue 
        bridge: br-ex
```
Example 1:
```yaml
---
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: test-net
spec:
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values: ["red", "blue"]
  network:
    topology: Localnet
    localnet:
      role: Secondary
      physicalNetworkName: tenantblue
      subnets: ["192.168.100.0/24", "2001:dbb::/64"]
```
The above spec will generate the following NAD, in namespace `blue`:
```yaml
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: test-net
  namespace: blue
finalizers:
 - k8s.ovn.org/user-defined-network-protection
labels:
  k8s.ovn.org/user-defined-network: ""
ownerReferences:
- apiVersion: k8s.ovn.org/v1
  blockOwnerDeletion: true
  controller: true
  kind: ClusterUserDefinedNetwork
  name: test-net
  uid: 293098c2-0b7e-4216-a3c6-7f8362c7aa61
spec:
    config: > '{
        "cniVersion": "1.0.0",
        "type": "ovn-k8s-cni-overlay"
        "netAttachDefName": "blue/test-net",
        "role": "secondary",
        "topology": "localnet",
        "name": "cluster.udn.test-net",
        "physicalNetworkName: "tenantblue",
        "mtu": 1500,
        "subnets": "192.168.100.0/24,2001:dbb::/64"
    }'
```
Example 2 (custom MTU, VLAN and sticky IPs):
```yaml
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: test-net
spec:
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values: ["red", "blue"]
  network:
    topology: Localnet
    localnet:
      role: Secondary
      physicalNetworkName: tenantblue
      subnets: ["192.168.0.0/16", "2001:dbb::/64"]
      excludeSubnets: ["192.168.50.0/24"]
      mtu: 9000
      vlan: 200
      ipam:
        lifecycle: Persistent
```
The above spec will generate the following NAD, in namespace `red`:
```yaml
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: test-net
  namespace: red
finalizers:
 - k8s.ovn.org/user-defined-network-protection
labels:
  k8s.ovn.org/user-defined-network: ""
ownerReferences:
- apiVersion: k8s.ovn.org/v1
  blockOwnerDeletion: true
  controller: true
  kind: ClusterUserDefinedNetwork
  name: test-net
spec:
    config: > '{
        "cniVersion": "1.0.0",
        "type": "ovn-k8s-cni-overlay"
        "netAttachDefName": "blue/test-net",
        "role": "secondary",
        "topology": "localnet",
        "name": "cluster.udn.test-net",
        "physicalNetworkName: "tenantblue",
        "subnets": "192.168.0.0/16,2001:dbb::/64",
        "allowPersistentIPs": true,
        "excludeSubents: "10.100.50.0/24",
        "mtu": 9000,
        "vlanID": 200
    }'
```

#### Go types
The CUDN `spec.network.topology` field should be extended to accept `Localnet` string.
And the `spec.network` struct `NetworkSpec` should be to have additional field for the localnet topology  configuration:
```go
const NetworkTopologyLocalnet NetworkTopology = "Localnet"

...

// NetworkSpec defines the desired state of UserDefinedNetworkSpec.
// +union
type NetworkSpec struct {
    // Topology describes network configuration.
    //
    // Allowed values are "Layer3", "Layer2" and "Localnet".
    // Layer3 topology creates a layer 2 segment per node, each with a different subnet. Layer 3 routing is used to interconnect node subnets.
    // Layer2 topology creates one logical switch shared by all nodes.
    // Localnet topology attach to the overlay local network. Enables egress to the provider's physical network. 
    //
    // +kubebuilder:validation:Required
    // +required
    // +unionDiscriminator
    Topology NetworkTopology `json:"topology"`
    
    ...
    
    // Localnet is the Localnet topology configuration.
    // +optional
    Localnet *LocalnetConfig `json:"localnet,omitempty"`
}
```

The CUDN spec should have additional validation rule for `spec.network.topology` field:
```go
// ClusterUserDefinedNetworkSpec defines the desired state of ClusterUserDefinedNetwork.
type ClusterUserDefinedNetworkSpec struct {
    ...
    // +required
    Network NetworkSpec `json:"network"`
}
```

As of today the CUDN has validation to ensure spec.network is immutable:
```go
// +kubebuilder:validation:XValidation:rule="self == oldSelf", message="Network spec is immutable"
```
This validation should be changed to allow mutation localnet topology configurations, at least MTU and VLAN.

Localnet topology configuration type 
```go
type LocalnetConfig struct {
    // Role describes the network role in the pod.
    //
    // Allowed value is "Secondary".
    // Secondary network is only assigned to pods that use `k8s.v1.cni.cncf.io/networks` annotation to select given network.
    //
    // +kubebuilder:validation:Required
    // +required
    Role NetworkRole `json:"role"`
    
    // PhysicalNetworkName the OVS bridge-mapping's network-name configured in the nodes.
    //
    // +kubebuilder:validation:Required
    // +required
    PhysicalNetworkName string `json:"name"`
    
    // Subnets are used for the pod network across the cluster.
    // Dual-stack clusters may set 2 subnets (one for each IP family), otherwise only 1 subnet is allowed.
    //
    // The format should match standard CIDR notation (for example, "10.128.0.0/16").
    // This field must be omitted if `ipam.mode` is `Disabled`.
    //
    // +optional
    Subnets DualStackCIDRs `json:"subnets,omitempty"`
    
    // ExcludeSubnets List of CIDRs removed from the specified CIDRs in `subnets`.
    //
    // The format should match standard CIDR notation (for example, "10.128.0.0/16").
    // This field must be omitted if `subnetes` is unset or `ipam.mode` is `Disabled`.
    //
    // +optional
    ExcludeSubnets []CIDR `json:"excludeSubnets,omitempty"`
    
    // IPAM section contains IPAM-related configuration for the network.
    //
    // +optional
    IPAM *IPAMConfig `json:"ipam,omitempty"`
    
    // MTU is the maximum transmission unit for a network.
    // MTU is optional, if not provided, the default MTU (1500) is used for the network.
    //
    // +kubebuilder:validation:Minimum=576
    // +kubebuilder:validation:Maximum=65536
    // +optional
    MTU int32 `json:"mtu,omitempty"`
    
    // VLAN is the VLAN ID.
    //
    // +kubebuilder:validation:Minimum=2
    // +kubebuilder:validation:Maximum=4094
    // +optional
    VLAN int32 `json:"vlan,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

### Risks and Mitigations

### Drawbacks

## Open Questions [optional]

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

No upgrade impact is expected.

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives
