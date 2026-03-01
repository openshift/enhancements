---
title: kubevirt-static-ip-address-management
authors:
  - Enrique Llorente <ellorent@redhat.com>
reviewers:
  - "@phoracek"
  - "@maiqueb"
  - "@orenc"
  - "@EdDev"
  - "@orelmisan"
  - "@RamLavi"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2025-10-06
last-updated: 2025-10-06
tracking-link:
  - https://issues.redhat.com/browse/CNV-67916
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# HyperShift KubeVirt Static IP Address Management

## Summary

This enhancement proposes adding static IP address management (IPAM) capabilities to HyperShift when using the KubeVirt provider and the KubeVirt nodepool is using a multus layer2 network as its default network. The implementation enables operators to define IP pools and network configurations that are automatically allocated to virtual machines during cluster provisioning. This functionality addresses the need for predictable, static network configuration in environments where DHCP is not available or desirable, particularly in on-premises and edge deployments.

This enhancement will use the short word "capk" to reference the cluster-api-provider-kubevirt component from hyperhift kubevirt provider.

## Motivation

Currently, HyperShift with the KubeVirt provider relies on DHCP for IP address allocation to guest cluster nodes. This approach introduces a DHCP dependency and for some customers, DHCP is not desired for various reasons, like latency, complexity or reliability.

By implementing static IPAM for KubeVirt-based HyperShift clusters, we enable deployment in a broader range of environments and provide operators with greater control over network configuration.

### User Stories

* As a cluster administrator I want to use Ingress Controller(s) on nodes with predefined IPs, so that I can update their upstream DNS and LB beforehand.
* As a cluster administrator, I need to be able to configure static IPs for my tenant workers, since DHCP server is not available in my network.

### Goals

- Enable static IP address assignment for HyperShift guest cluster nodes running on KubeVirt
- Provide a flexible API for defining IP address pools like the MetalLB subnet format with support for: CIDR ranges (192.168.10.0/24), hyphenated ranges (192.168.9.1-192.168.9.5), and single IPs (192.168.1.100/32 for IPv4 or 2001:db8::1/128 for IPv6)
- Support both IPv4 and IPv6 addressing for interface, default gw and DNS resolver.
- Support KubeVirt VMs live migration so the assigned network configuration stick to the VMs after live migrating them.
- Maintain the IPs across node or control plane pods restarts.

### Non-Goals

- Replacing existing network configuration mechanisms in standalone OpenShift clusters
- Implementing a full-featured IPAM solution with conflict detection across multiple management clusters
- Providing network configuration for components outside of HyperShift guest cluster nodes
- Support complex network configurations like bonding of interfaces and others usually configured with ignition
- Add IPAM network configuration to secondary networks not related to bootstrap process
- Supporting dynamic IP pool expansion or runtime IP reallocation

## Proposal

This proposal introduces static IPAM capabilities through modifications to three key components:

1. **Cluster API Provider KubeVirt**: Extend the API to support IP pool definitions and network configuration, implement IP allocation logic from defined pools
2. **HyperShift**: Add network configuration options to the KubeVirt platform specification at nodepool, implement translation from this network configuration to capk ip pool and openstack network config
3. **CoreOS Afterburn**: Enable parsing and application of OpenStack or netplan network config standard data as dracut network kernel args from cloud-init (config drive or nocloud) for KubeVirt VMs, this is similar what is done at [proxmoxve provider](https://github.com/coreos/afterburn/pull/1023).

### Workflow Description

**cluster administrator** is a human user responsible for deploying a HyperShift hosted cluster.

1. Define the `NodePool` kubevirt platfrom configuration with the following:
   - Disable the default pod network and select the network to replace it at the `NodePool` CRD instance:
   ```yaml
   spec:
     plaform:
       kubevirt:
         attachDefaultNetwork: false
         additionalNetworks:
         - name: my-nad-1
   ```

   - The cluster administrator defines an IP pool configuration in the `NodePool` CRD instance using the:
   ```yaml
   spec:
     plaform:
       kubevirt:
         bootstrapNetworkConfig:
            network: my-nad-1-network  # network name (to assign a ip pool) not the nad name
	        addresses: # IP address ranges: CIDR notation, hyphenated ranges, or single IPs with /32 (IPv4) or /128 (IPv6)
            - 192.168.1.10-192.168.1.50
            - 2620:0:860:2::10-2620:0:860:2::50
            - 192.168.1.100/32  # Single IP address
	        excludedAddresses: # Optional: IP addresses or ranges to exclude from allocation
            - 192.168.1.20-192.168.1.25
            - 2620:0:860:2::20/128  # Single IPv6 address to exclude
	        nameservers:
            - 8.8.8.8
	        gateway:
            - 192.168.1.1
            - 2620:0:860:2::1
   ```

3. Create the hosted cluster with the defined `NodePool`

4. HyperShift reconciles the kubevirt `NodePool` CR and create `KubevirtMachineTemplate` CR for the guest cluster nodes:
   ```yaml
   spec:
     template:
       spec:
         virtualMachineTemplate:
           spec:
             template:
               spec:
                 ...
                 domain:
                   devices:
                   - ...
                     acpiIndex: 101
         networkConfig:
           ipPool:
             my-nad-1-network:
               interfaceName: eno101
               subnets:
               - 192.168.1.1
               - 2620:0:860:2::1
            cloudInitNetworkData: |
                {
                  "links": [
                    {
                      "id": "eno101",
                      "type": "phy",
                    }
                  ],
                  "networks": [
                    {
                      "id": "network0",
                      "type": "ipv4",
                      "link": "eno101",
                      "routes": [
                        {
                          "network": "0.0.0.0",
                          "netmask": "0.0.0.0",
                          "gateway": "192.168.1.1"
                        }
                      ],
                      "network_id": "network0"
                    }
                  ],
                  "services": [
                    {
                      "type": "dns",
                      "address": "8.8.8.8"
                    },
                  ]
                }
   ```
  with means:
   - Configure `acpiIndex: 101` on the virtual machine first interface that will have this network so we have interface name `eno101`
   - Configure interface `eno101`, gateways, and nameservers in the OpenStack network config data (without addresses) and store at KubevirtMachineTemplate `networkData` field
   - Pass nodepool's Network and addresses to KubevirtMachineTemplate NeworkName and Subnets
   - Pass interface `eno101` to KubevirtMachineTemplate Interface.
   - Create the generated KubevirtMachineTemplate.

5. The Cluster API Provider KubeVirt controller reconcile the KubevirtMachineTemplate:
   - Allocates an available IP address from the network's pool (there are different pools per network)
   - Records the allocation in the KubevirtMachine status subresource (`allocatedBootstrapAddresses` field), so in memory pools can be re-constructed on restarts
   - Expand the openstack network configuration with the allocated address
   - Injects the expanded network configuration into the VM's cloud-init config drive

6. When the CoreOS VM boots:
   - Afterburn reads the OpenStack network config from the cloud-init config drive
   - Translates the network configuration into dracut kernel arguments
   - Applies the static network configuration to the specified interface(s) at boot before ignition
   - Ignition starts and fetch configuration from hypershift ignition server without issues since basic network configuration is in place.

7. The node comes online with the configured static IP address on the appropriate network (default or custom) and joins the hosted cluster

#### Failure Handling

- **IP Pool Exhaustion**: If all IPs in the pool are allocated, machine creation will fail with a clear error message indicating pool exhaustion at the NodePool CRD status with a condition like `KubevirtIPPoolExhausted=true`
- **Invalid Network Configuration**: The API will validate network configuration format during HostedCluster creation, rejecting invalid configurations, this should be check at api-machinery level using something like CEL or webhook and NetworkValidations at hypershift level.
- **Network Configuration Failure**: If afterburn cannot parse or apply network configuration, the node bootstrap will fail and the issue will be visible in console logs in the form of a systemd unit failing

### API Extensions

This enhancement introduces new API fields to existing Custom Resource Definitions:

**HostedCluster API** (hypershift):
- New struct `KubevirtBootstrapNetworkConfig`:
  ```go
  type KubevirtBootstrapNetworkConfig struct {
	// +optional
	Network string `json:"network"`
    // Addresses specify IP ranges from which addresses will be allocated for this interface.
    // Supporting:
    // - CIDR notation (e.g., 192.168.10.0/24)
    // - Hyphenated ranges (e.g., 192.168.9.1-192.168.9.5)
    // - Single IPs with /32 for IPv4 (e.g., 192.168.1.100/32) or /128 for IPv6 (e.g., 2001:db8::1/128)
    // +optional
    // kubebuilder:validation:MinLength=1
    // kubebuilder:validation:MaxLength=2
    // +kubebuilder:validation:XValidation:rule="self.all(addr, addr.contains('/') ? isCIDR(addr) : addr.contains('-') ? (size(addr.split('-')) == 2 && isIP(addr.split('-')[0]) && isIP(addr.split('-')[1])) : false)", message="each address must be a valid CIDR, IP range (two IPs separated by '-'), or single IP with /32 (IPv4) or /128 (IPv6)"
	Addresses []string `json:"addresses"`

    // ExcludedAddresses specifies IP addresses or ranges that should be excluded from allocation.
    // subnet format (same as Addresses field).
    // +optional
    // +kubebuilder:validation:XValidation:rule="self.all(addr, addr.contains('/') ? isCIDR(addr) : addr.contains('-') ? (size(addr.split('-')) == 2 && isIP(addr.split('-')[0]) && isIP(addr.split('-')[1])) : false)", message="each address must be a valid CIDR, IP range (two IPs separated by '-'), or single IP with /32 (IPv4) or /128 (IPv6)"
	ExcludedAddresses []string `json:"excludedAddresses,omitempty"`
	// nameservers is a list of DNS server IP addresses to use for name resolution.
	// +optional
    // kubebuilder:validation:MinLength=1
    // kubebuilder:validation:MaxLength=2
	Nameservers []string `json:"nameservers"`
	// gateway is a list of gateway IP addresses for routing traffic outside the local network.
	// +optional
    // kubebuilder:validation:MinLength=1
    // kubebuilder:validation:MaxLength=2
	Gateway []string `json:"gateway"`
  }
  ```

- New field in KubevirtPlatformSpec:
  ```go
  type KubevirtPlatformSpec struct {
      // ... existing fields ...
      BootstrapNetworkConfig *KubevirtBootstrapNetworkConfig `json:"bootstrapNetworkConfig"`
  }
  ```

**KubevirtMachineTemplate CRD** (cluster-api-provider-kubevirt):
- New field in KubevirtMachinesSpec:
```go
type KubevirtMachineSpec struct {
      // ... existing fields ...
    BootstrapNetworkConfig *VirtualMachineBootstrapNetworkConfig
}
```
- New struct `VirtualMachineBootstrapNetworkConfig`
 ```go
 type VirtualMachineBootstrapNetworkConfig struct {
     // +optional
     // +kubebuilder:default=eno101
     Interface string `json:"interface,omitempty"`
     // +required
     NetworkName string `json:"networkName"`
     // Subnets specify IP ranges from which addresses will be allocated for this interface.
     // Supporting:
     // - CIDR notation (e.g., 192.168.10.0/24)
     // - Hyphenated ranges (e.g., 192.168.9.1-192.168.9.5)
     // - Single IPs with /32 for IPv4 (e.g., 192.168.1.100/32) or /128 for IPv6 (e.g., 2001:db8::1/128)
     // +required
     Subnets []string `json:"subnets"`
     // ExcludedAddresses specifies IP addresses or ranges that should be excluded from allocation.
     // subnet format (same as Subnets field).
     // +optional
     ExcludedAddresses []string `json:"excludedAddresses,omitempty"`
     // NetworkData contains user-provided cloud-init network data in YAML format.
     // The system will find the interface specified in Interface field and inject
     // allocated IP addresses into this network configuration.
     // +optional
     NetworkData *string `json:"networkData,omitempty"`
 }
 ```

**KubevirtMachine Status** (cluster-api-provider-kubevirt):
- New status subresource in KubevirtMachineStatus:
```go
type KubevirtMachineStatus struct {
    // ... existing fields ...
    // AllocatedBootstrapAddresses tracks allocated IP addresses per network interface.
    // The map key is the interface name and the value is a list of IP addresses (with CIDR notation).
    // This field is used to reconstruct the in-memory IP pool when the cluster-api-provider-kubevirt
    // controller restarts.
    // Example: {"ens123": ["10.10.0.42/24", "fd41:1234::932/64"]}
    // +optional
    AllocatedBootstrapAddresses map[string][]string `json:"allocatedBootstrapAddresses,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement specifically targets HyperShift hosted control planes running on KubeVirt. The changes affect:

**Management Cluster Components**:
- Cluster API Provider KubeVirt controller (KubevirtMachineTemplate controller): Enhanced with IP allocation logic
- HyperShift operator (NodePool controller): Updated to handle network configuration in NodePool spec

**Guest Cluster Components**:
- Node bootstrap process: Modified to apply static network configuration via afterburn
- No changes to running workload network behavior

The enhancement does not affect the control plane's own network configuration, only the data plane (worker nodes) in the guest cluster.

#### Standalone Clusters

This enhancement is not relevant for standalone OpenShift clusters, as they use different provisioning mechanisms (installer, machine-api) that have their own network configuration capabilities.

#### Single-node Deployments or MicroShift

This enhancement does not directly impact single-node OpenShift (SNO) or MicroShift deployments, as they do not use HyperShift or the KubeVirt provider.

### Implementation Details/Notes/Constraints
**Network Interface selection**:
Hypershift need to pass an interface name to cluster api provider kubevirt so it knows the network interface to configure the allocated ip to. To do so hypershift configure [ACPI Index](https://github.com/kubevirt/kubevirt/pull/9116) at the VMI to have predictible interfaces names, for example `acpiIndex: 101`
will get us `eno101`.


**IP Allocation**:
The KubevirtMachineTemplate controller at capk implements a simple sequential allocation algorithm using the subnet format:
1. Parse IP ranges from with format (CIDR notation like 192.168.10.0/24, hyphenated ranges like 192.168.9.1-192.168.9.5, or single IPs like 192.168.1.100/32) into a list of desired addresses
3. Allocate the first available IP from the IP pool cache
4. Record allocation in the machine's status subresource (`allocatedBootstrapAddresses` field)
5. Expand networkData with the `ip_address` field at the interface passed at `interface`

**IP Deallocation**:
The KubevirtMachine controller detects a virtual machine instance removal, then release the ip by
putting it back to the IP pool. Note that IPs will be released only on removal to keep same IP on node restart or temporal shut down.

**IP Pool cache**:
To implement IP allocation and deallocation is desirable to use a cache to reduce CPU consumption and latency by maybe increasing memory.
These are the steps that the operator would need to do without a cache:
1. List all the KubevirtMachines that has an address at the same network
2. Record all those address on some structure (allocator?)
3. Ask the structure for a free IP.
As we can see at the end the cache is kind of reconstructed everytime to look for a free IP on it, we have the worst of both worlds since memory consumption is the same but we have to iterate and re-construct.

**Control plane nodes or capk controller pods restart**:
When the capk controller starts, it will check all the virtual machines instances that belongs to it and will reconstruct
the in memory ip pool, the reconciliation process cannot start until this is finished.

**OpenStack Network Config Format**:
As stated by the [cloud-init reference docs](https://cloudinit.readthedocs.io/en/latest/reference/network-config.html#network-configuration-sources) the implementation should use the OpenStack network config standard format when cloud init is delivered by config drive. The network configuration is generated as JSON and injected into the VM's cloud-init config drive.

Example OpenStack network config format following the [schema](https://docs.openstack.org/nova/latest/_downloads/9119ca7ac90aa2990e762c08baea3a36/network_data.json):
```json
{
  "links": [
    {
      "id": "eno101",
      "type": "phy",
      "ethernet_mac_address": "52:54:00:12:34:56"
    }
  ],
  "networks": [
    {
      "id": "network0",
      "type": "ipv4",
      "link": "eno101",
      "ip_address": "192.168.1.100", <-- this is the part allocated by "cluster api provider kubevirt controller"
      "netmask": "255.255.255.0",
      "routes": [
        {
          "network": "0.0.0.0",
          "netmask": "0.0.0.0",
          "gateway": "192.168.1.1"
        }
      ],
      "network_id": "network0"
    }
  ],
  "services": [
    {
      "type": "dns",
      "address": "8.8.8.8"
    },
    {
      "type": "dns",
      "address": "8.8.4.4"
    }
  ]
}
```

**Afterburn Integration**:
Production-ready PR implementing this:
- https://github.com/coreos/afterburn/pull/1238

Afterburn is modified to:
- Detect when running on KubeVirt (similar to existing ProxmoxVE provider detection)
- Read OpenStack network config from the cloud-init config drive metadata location (a drive with label `config-2`)
- Parse OpenStack network config standard JSON format (the file `network_data.json` at the `config-2` drive)
- Support static IPv4/IPv6 addresses, default gateways, physical ethernet interfaces, DHCP configuration, and DNS nameservers
- Generate dracut kernel arguments for static network configuration
- Expose first interface's IPv4 and IPv6 addresses as metadata attributes
- Preload the virtio_blk module via a dracut module for early boot disk access needed for rhcos + kubevirt (fcos is fine)

**Limitations**:
- No automatic IP conflict detection across multiple management clusters
- No integration with external IPAM systems
- Since this depends on RHCOS fix for afterburn and ignition also we are touching the hosted cluster control plane this API should be exposed only
  to newer hosted clusters, or error if used on older ones.

**Improvements**:
- The merged [PR](https://github.com/coreos/ignition/pull/2134) to use NoCloud with ignition instead of ConfigDrive allow to use cloud init network config v1 or v2 this is simpler format than openstack config, issues that currently hypershift is using config drive instead of nocloud to pass the userdata with the whole MCO over ignition, so changing it to nocloud may have upgrade issues that we have to take into account.

### Risks and Mitigations

**Risk**: IP address conflicts if multiple clusters use overlapping IP pools
**Mitigation**: Document best practices for IP pool planning. Consider future enhancement for IP conflict detection or integration with external IPAM solutions.

**Risk**: Incorrect network configuration could make nodes unreachable
**Mitigation**: Provide comprehensive validation of network configuration in the API. Document testing procedures and troubleshooting steps. Ensure console access is available for debugging boot issues.

**Risk**: Afterburn changes could affect other CoreOS deployments
**Mitigation**: Changes are isolated to KubeVirt provider detection and handling. Extensive testing in CI for both HyperShift and standalone CoreOS deployments.

**Risk**: Race conditions at ip allocation and pool re construction
**Mitigation**: The cluster-api-provider-kubevirt controller use only one gorouting for it, but still the in memory pool need to be protected with read/write locks, also capk will not be ready during pod or node restart.

### Drawbacks

- Adds complexity to the hypershift kubevirt controllers:
  - implementing an ip pool mechanism
  - Needs to parse and understand the openstack (configdrive) or netplan (nocloud) network config format
- Fix depend on a coreos change:
  - It will take some time for the RPM to reach targeted RHCOS, also we will have to plan backports there.

## Alternatives (Not Implemented)

### Putting the network configuration inside the AdditionalNetworks structs

Add `networkConfig` to the AdditionalNetwork struct

```yaml
type AdditionalNetwork struct{
   Name string
   NetworkConfig ... <- this is new 
}
```

Pros:
- We re-use the current addtional network concept without introducing bootstrap one
- Allow to use the ip allocator at secondary networks too

Cons:
- Introduce more complexity, since it opens the door to do network configuration for all the networks
- We don't really know if users may need an hypershift ipam mechanism for secondaries.
- Users can configure common network configuration with ignition

### Store all the ignition config directly at a secret instead of retrieve it using the ignition server

Currently hypershift workers retrieve the ignition config from an ignition server, so they need to have network configured before that, an alternative
would be to directly put the whole ignition config directly at the secret referenced by the VM so accessing ignition server is not needed.

**Why not chosen**: The kubernetes resources has a size limit that has already been reached by hypershift when storing the whole ignition config there [Jira](https://issues.redhat.com//browse/OCPBUGS-60148), also it means changing how a hypershift fundamental part of the bootstrap process works.

### Use the ovn-kubernetes localnet IPAM mechanism

With this solution IPAM configuration will be done at the ovn-kubernetes localnet user defined network instead of the hypershift kubevirt nodepool

Missing features from current localnet implementation:
- There is no official support for localnet + IPAM
- localnet is missing gateway and dns configuration
- There is no out of the box solution for IPv6 RAs (there is no OVN logical router here)
- Subnets field do not support the subnet format (CIDR ranges, hyphenated ranges like 192.168.9.1-192.168.9.5, or single IPs like 192.168.1.100/32)
- Subnets field is only mutable at NAD not at CUDN

Pro:
- A lot of the missing logic is already present for primary UDN layer2 as part of the IP migration from MTV, only missing part is name resolution server.
- This works for VMs and pods, so it would be possible in the future to also put the control plane under same network and IPAM.
- It will be supported for ovn-kubernetes team, not only hypershift kubevirt maintainers.
- There is a center place to allocate IPs so there will be no overlap between hosted clusters
- No need to implement the complexities of ip pool at hypershift

Cons:
- No clear solution for implementing IPv6 RAs since there is no logical router.
  since it's not really aligned with ovn-kubernetes maintainers priorities.
- This make customer depends on ovn-kubernetes localnet topology.

### Use the ovn-kubernetes localnet IPAM mechanism just for addresses (it's already implemented upstream)

With this solution IPAM configuration will be done at the ovn-kubernetes localnet user defined network instead of the hypershift kubevirt nodepool

Missing features from current localnet implementation:
- There is no official support for localnet + IPAM
- Subnets field do not support the subnet format (CIDR ranges, hyphenated ranges like 192.168.9.1-192.168.9.5, or single IPs like 192.168.1.100/32)
- Subnets field is only mutable at NAD not at CUDN

Pro:
- Most of the implementation is in place
- This works for VMs and pods, so it would be possible in the future to also put the control plane under same network and IPAM.
- It will be supported for ovn-kubernetes team, not only hypershift kubevirt maintainers.
- There is a center place to allocate IPs so there will be no overlap between hosted clusters
- No need to implement the complexities of ip pool at hypershift

Cons:
- This make customer depends on ovn-kubernetes localnet topology.
- Network configuration will be split between localnet NAD/CUDN and NodePool NetworkConfig
- The coreos afterburn component for kubevirt provider do not implement configuring hybrid dracut config like:
```
ip=<device>:dhcp rd.route=::/0:192.168.1.1 nameserver=8.8.8.8 nameserver=8.8.4.4
```
- Only CIDRs supported, not the full subnet format (hyphenated ranges like 192.168.3.4-192.168.3.7)
- The localnet Subnet field is only mutable at NADs if they belong to just one network but not at CUDNs
- IPAM on secondaries is not using OVN DHCPOptions, it's using kubevirt DHCP so IPv6 is not supported

### Implement the ip pool at kubevirt ipam controller

Instead of implementing the ip pool at cluster api provider kubevirt, there is one per hosted cluster, implement it at kubevirt ipam controller, there is one for all the hosted clusters,
that would allow us to overcome the ip overlapping issue between thosed clusters, although it's not real clear if it's a really a problem.

Major problem is that there is no a clear way to specify what IPAM configuration to the controller since the VMI itself do not have enough "api" for it, and changing kubevirt API need
clear justifications.

### Adding a DHCP server per network at hosted cluster control plane

Implement a pod per network attaching something like a dnsmasq to that network and serve the configured IPAM.

Main problem is the complexity of implementing proper storage backend for DHCP on a kubernetes environment, conflict with the reasons why the customer refused to use DHCP.

### Use whereabouts with ipamclaims 

The IPAM plugin whereabouts is implementing already an ippool if we add support for IPAMClaims we will be able to make it support live migration.

Pros:
- Out of the box ip pool
- Not very difficult to implement ipamclaims there

Cons:
- It do not work with localnet, so there is no support for MultiNetworkPolicies, although there is some [upstream project](https://github.com/kubevirt-manager/mnp-nft-bridge) to support it.


## Open Questions

1. Do we need to think of a solution for ip overlapping between different hosted clusters ?

## Test Plan

**Unit Tests**:
- IP allocation algorithm correctness (sequential allocation, deallocation, pool exhaustion, pool resize)
- Network configuration generation in OpenStack network config standard format
- API validation (IP range parsing, configuration validity)

**Integration Tests**:
- Multiple machines receiving unique IPs from the pool and deallocating them on scale down
- Resizing the IP pool allow to bypass exhaustion
- Failure scenarios (pool exhaustion, invalid configuration)

**End-to-End Tests**:
- Verify all nodes receive correct static IP addresses
- Test node connectivity and cluster functionality
- Scale cluster up and verify new nodes receive IPs from pool
- Scale cluster down and verify that IPs get released
- Test upgrade scenarios
- Test live migration scenarios
- Test ip overlapping between nodepools with different networks

**Performance Testing**:
- IP allocation performance with large pools (1000+ addresses)
- Scale testing with 100+ node clusters

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to deploy HyperShift clusters on KubeVirt with static IP configuration
- Basic documentation for configuration and troubleshooting
- Sufficient test coverage in CI
- Gather feedback from early adopters
- Verify functionality in representative customer environments

### Tech Preview -> GA

- Comprehensive user documentation in openshift-docs
- Support for both IPv4 and IPv6
- Production-ready error handling and validation
- Feedback incorporated from Tech Preview users

### Removing a deprecated feature

- This section is not applicable as this enhancement introduces a new feature rather than deprecating an existing one.

## Upgrade / Downgrade Strategy

**Upgrades**:
- New clusters can opt into static IP configuration
- API changes are additive and backward compatible
- Afterburn changes are backward compatible with existing cloud-init config drive data

**Downgrades**:
N/A

**Migration Strategy**:
Migrating a hosted cluster with node IP's provided by CNI or DHCP to the configuration where the IPs
are provided by hypershift is not expected, if this is needed the hosted cluter will be re-constructed at least the NodePool with the proper ipam configuration.

## Version Skew Strategy

**Management Cluster vs Guest Cluster**:
- Network configuration is applied at node bootstrap time, so guest cluster version is independent
- Management cluster must be at the version supporting this feature to create clusters with static IPs

**RHCOS Version Compatibility**:
- Afterburn changes require RHCOS version with updated afterburn package
- Since RHCOS kubevirt images are part of the openshift release payload we will have to document
  up to what openshift version we support the feature, depending on how far we manage to backport
  the RHCOS afterburn feature.

**Component Upgrade Ordering**:
1. Afterburn changes must be included in RHCOS images used for node templates
2. The new hypershift operator will also upgrade the new cluster api provider kubevirt since it's
   responsible for upgrading it.

## Operational Aspects of API Extensions

### API Extension Impact

**New CRD Fields**:
- BootstrapNetworkConfig in KubevirtMachineSpec (cluster-api-provider-kubevirt)
- BootstrapNetworkConfig in nodepool's KubevirtPlatformSpec (hypershift)

**SLIs for API Extensions**:

**Impact on Existing SLIs**:
- Minimal impact on API throughput (network config is small JSON payload)
- Slight increase in KubevirtMachine creation time (additional IP allocation step)
- No impact on kube-apiserver or other cluster components

**Failure Modes**:
1. **IP Pool Exhaustion**: Machine creation fails with clear error. Operator must expand pool or delete unused machines.
2. **Invalid Network Configuration**: HostedCluster creation fails validation. Operator must correct configuration.
3. **Network Config Application Failure**: Node fails to boot or is unreachable. Visible in VM console logs. Operator must verify network configuration correctness.
4. **IP Allocation Conflict**: Different nodepools using same network but with overlapping IPs should fail with clear error. Operator should fix the subnets.

## Support Procedures

### Detecting Failures

**IP Pool Exhaustion**:
- Symptom: New KubevirtMachine resources remain in Pending state
- Detection: Check KubevirtMachine events and status conditions
- Log output: "No available IPs in pool for interface X"
- Metric: `kubevirt_ippool_exhausted{cluster="X"}`
- NodePool Status condition "KubevirtIPPoolExhausted=true"

**Network Configuration Application Failure**:
- Symptom: Node does not join cluster
- Detection: Check VM console logs via KubeVirt
- Log output: Afterburn errors in boot logs or dracut network configuration failures (e.g., "failed to parse OpenStack network config")
- Resolution: Verify OpenStack network config format correctness, check gateway/DNS reachability

### Disabling the Feature

To disable static IP configuration for a cluster:
- Remove NetworkConfig from KubevirtMachine template spec
- Nodes will fall back to DHCP

Consequences:
- Existing nodes with static IPs continue to function
- New nodes will use DHCP
- No impact on running workloads

### Recovery Procedures

**IP Pool Exhaustion**:
1. Remove the nodepool or pull back the replicas to original value
2. Create a new nodepool with a new subnet for the same network 

**IP Pool Corruption**:
1. Audit all KubevirtMachine resources for allocated IPs (check `status.allocatedBootstrapAddresses` field)
2. Remove the allocated IP from the status subresource of affected VMs
3. Drain the hosted cluster nodes with colliding IPs
4. Hard restart the VMs
5. Restart cluster-api-provider-kubevirt controller to re-synchronize

**Network Misconfiguration**:
1. Access VM console via KubeVirt to diagnose boot issues
2. Verify network configuration in HostedCluster spec
3. Fix network config
3. Scale down and then up the nodepool

## Infrastructure Needed

- CI infrastructure capable of running HyperShift with KubeVirt (existing)
- Test environment with isolated network for static IP testing

## References

**Proof of Concept Implementations**:
- [Cluster API Provider KubeVirt PoC](https://github.com/qinqon/cluster-api-provider-kubevirt/commit/9fe6fb05bfa21c891a93d0dfcbbfae586ed88c34)
- [HyperShift PoC](https://github.com/qinqon/hypershift/commit/8988b705b665007b42ad3ebd39e6e398f2a2c15e)

**Production PRs waiting reviews**:
- [CoreOS Afterburn PR](https://github.com/coreos/afterburn/pull/1238)

**Related Documentation**:
- [HyperShift Documentation](https://hypershift-docs.netlify.app/)
- [Cloud-init Network Configuration](https://cloudinit.readthedocs.io/en/latest/reference/network-config.html)
- [Openstack network config schema](https://docs.openstack.org/nova/latest/_downloads/9119ca7ac90aa2990e762c08baea3a36/network_data.json)
- [KubeVirt Documentation](https://kubevirt.io/user-guide/)
- [MetalLB IPAddressPool Configuration](https://metallb.universe.tf/configuration/) - The subnet format used in this enhancement is based on MetalLB's address range specification
