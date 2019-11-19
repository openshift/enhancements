---
title: Azure: Allow Customer-Provisioned Virtual Network & Subnets
authors:
  - "@jhixson"
reviewers:
  - "@abhinavdahiya"
  - "@sdodson"
approvers:
  - "@abhinavdahiya"
  - "@sdodson"
creation-date: 2019-08-12
last-updated: 2019-09-24
status:
    implemented

---

# azure-allow-customer-provisioned-virtual-network-and-subnets

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]


## Summary

In many large organizations, IT groups are separated into teams. A common
separation of duties lies between networking/WAN and application or enabling
technology deployments. One group will provision or assign AWS accounts with
core management technology. A second group will be responsible for provisioning
vnet networks complete with subnet & zone selection, routing, VPN connections,
and ingress/egress rules from the company. Lastly, an applications team (or
individual business units) are permitted into the account to provision
application-specific items (instances, buckets, databases, pipelines, load
balancers, etc.)

To accomplish this, our infrastructure needs and Azure account access can be
separated into "infrastructure" versus "application". Infrastructure includes
elements related to the vnet and the networking core components within the VNet
(VNet, subnets, routing tables, internet gateways, NAT, VPN). Application items
include things directly touching nodes within the cluster (LBs, security groups,
storage, nodes). This is a typical separation of responsibilities among IT
groups and deployment/provisioning pipelines within large companies.

## Motivation

### Goals

The ability to install OpenShift clusters into existing VNets

### Non-Goals

Modifying the existing network configuration

## Proposal

- Installer should be able to install cluster in an pre-existing VNet. This
  requires providing subnets too as there is no sane way installer can carve out
network ranges for the cluster.
- There is an expectation that since the VNet and subnets are being shared, the
  installer cannot modify the networking setup. Therefore the installer can only
validate the assumptions about the networking setup.
- Resources that are specific to the cluster and resources owned by the cluster
  can be created by the installer. So resources like load balancers, service
accounts, etc stay under the management of the cluster. All resources created by
the installer should not be modified and will be removed when the cluster is
destroyed in the same fashion as installer owned infrastructure. 
- Any changes required to the shared resources like Tags that do not affect the
  behavior for other tenants can be made.
- Resources owned by the cluster and resources must be clearly identifiable.
  There will be an existing network resource group that will be used that we
can’t touch, and there will be the installer owned resource group that we can
create/update/delete.
- Destroy cluster must make sure that no resources are deleted that didn't
  belong to the cluster. Leaking resources is preferred than any possibility of
deleting non-cluster resource.

### Implementation Details/Notes/Constraints


#### Resources provided to the installer

- NetworkResourceGroup: The network resource group used by the VNet/subnet(s). 
- VirtualNetwork: The VNet the cluster will be installed in. This has to be part
  of and will be retrieved from the provided NetworkResourceGroup. 
- Control-plane subnet: The subnet where the control plane should be deployed.
  Any static IP addresses here need to be replaced with Azure assigned DHCP IP
addresses.
- Compute subnet: The subnet where the compute should be deployed. This can be
  the same as the control plane subnet. Any static IP addresses here need to be
replaced with Azure assigned DHCP IP addresses.

```yaml
apiVersion: v1
baseDomain: example.azure.devcluster.openshift.com
compute:
- hyperthreading: Enabled
  name: worker
  platform:
    azure:
      osDisk:
        diskSizeGB: 128
      type: Standard_D4s_v3
controlPlane:
  hyperthreading: Enabled
  name: master
  platform: {}
  replicas: 3
metadata:
  creationTimestamp: null
  name: testbyovpc
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineCIDR: 10.0.0.0/16
  networkType: OpenShiftSDN
  serviceNetwork:
  - 172.30.0.0/16
platform:
  azure:
    baseDomainResourceGroupName: os4-common
    region: centralus
    networkResourceGroupName: example_network_resource_group
    virtualNetwork: example_virtual_network
    controlPlaneSubnet: example_master_subnet
    computeSubnet: example_worker_subnet
```

#### Assumptions about the resources as provided to the installer

#### Subnets

- Subnets must all belong to the same VNet in provided NetworkResourceGroup.
  The install-config wizard will ensure this by prompting for the
NetworkResourceGroup first and then providing vnet and subnets as a choice
widget (not free-form).  But generic install-config validation needs to cover
user-provided install-config.yaml as well, so it should look up the subnets and
ensure that they all belong to the same VNet.
- Unvalidated: Subnets must provide networking (network security groups, route
  tables, etc.).  At least for the initial implementation, we will not validate
this assumption, will attempt to install the cluster regardless, and will fail
after having created cluster-owned resources if the assumption is violated.
Future work can iterate on pre-create validation for the networking assumptions,
if they turn out to be a common tripping point.

#### VNet

No known assumptions.

#### Private DNS

- Private DNS as it currently exists can’t be used since the way it currently
  works does not allow us to use it in an existing VNet.
- New clusters will use the new Azure Preview Refresh API for private DNS. This
  will allow the installer to use private DNS zones in an existing virtual
network and under and installer owned resource group. 
- Private DNS can be auto registered or created manually. Currently, records are
  created manually. More information can be found here. 
- It will be necessary to update to the new Azure Preview Refresh SDK to use the
  new API.
- The Azure terraform resource provider will need to be updated.
- Upgrading from 4.2 to 4.3, the clusters will continue to use the legacy API
  until or if a migration strategy is considered.

#### Resources created by the installer

#### The installer will no longer create (vs. the fully-IPI flow):
- Subnets (azurerm_subnet)
- Route tables (azurerm_route_table)
- VNets (azurerm_virtual_network)
- Network Security Groups (azurerm_network_security_group) #### The installer
  will continue to create (vs. the fully-IPI flow):
- Virtual Machines (azurerm_virtual_machine, azurerm_image)
- Storage (azurerm_storage_blob, azurerm_storage_container)
- Load balancers (azurerm_lb)
- Private DNS Zones (azurerm_dns_zone) - This uses the current Azure private DNS
  zone. We’ll have to look at updating this to use new API. 
- Network security groups (azurerm_network_security_group)
- Resource Groups (azurerm_resource_group)

#### API

#### The install-config YAML will grow four properties on the Azure platform
structure
- NetworkResourceGroup
- VirtualNetwork
- ControlPlaneSubnet
- ComputeSubnet #### The install-config wizard will add three prompts: 
- A Select prompt:
  - Message: VNet
  - Help: The VNet to be used for installation.
  - Default: new
  - Options: [“new”] + results of a Virtual Networks - List All call.
- A Select prompt:
  - Message: Control-plane subnet
  - Help: The subnet to be used for the control plane instances.
  - Options: results of a Subnets - List call filtered by the selected VNet.
  - Only when: VNet is not “new”
- A Select prompt:
  - Message: Compute subnet
  - Help: The subnet to be used for the compute instances.
  - Options: results of a Subnets - List call filtered by the selected VNet.
  - Only when: VNet is not “new”

#### Isolation between clusters

Since we can’t create or modify network security groups on an existing subnet,
there is no real way to isolate between clusters. 

#### Known limitations

- We can’t attach or manipulate network security groups to an existing subnet
  since it’s not in our control. This means we can’t isolate between clusters
also.
- We can’t attach DNS records to the existing VNet, so we will need to use the
  new Azure private DNS zones. 

#### Destroy

There should be no work required. For Azure, we filter on resource group. As a
result, destroy should only delete things that are in the resource group.

#### CI

CI jobs should set up a VNet, subnets, routing, and NAT and then run the
installer using this configuration. 

#### Work dependent on other teams

CI infrastructure - A new CI job will need to be configured that sets up an
Azure virtual network. It would be a good idea for it to persist if possible so
that we can make use of it without having to create and tear down for every job. 

#### Alternate solutions considered

Network security groups can be attached to a subnet or a network interface. Only
a single network security  can attach to a network. Since the goal is to install
into an existing virtual network, we can’t create or modify a network security
group and keep it under our control. An alternative solution to this problem is
to attach a network security group to each network interface. This would work
somewhat similar to how it would work being attached to a subnet, only the rules
would be duplicated for every network interface. 
