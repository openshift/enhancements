---
title: vsphere-ipi-zonal 
authors:
  - "@jcpowermac"
reviewers:
  - "@rvanderp3"
  - "@vr4manta"
  - "@JoelSpeed"
approvers:
  - "@rvanderp3"
  - "@patrickdillon"
api-approvers:
  - "@JoelSpeed"
  - "@deads2k"
creation-date: 2021-09-21
last-updated: 2024-09-26
status: implementable
see-also:
  - "/enhancements/"
replaces:
  -
superseded-by:
  -
tracking-link:
- https://issues.redhat.com/browse/SPLAT-320
- https://issues.redhat.com/browse/SPLAT-1728

---

# Support Zonal vSphere IPI installation 

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The goal of this enhancement is to provide the ability to install in a
vSphere environment with multiple types of failure domains.

The failure domain types include:
- Multiple vCenters, Datacenters and Clusters
- Host and VM Groups - where Clusters are a region and ESXi nodes (Host Group) are the zones.
This is important in vCenter clusters that are stretched over physical datacenters.

## Motivation

Users of OpenShift would like the ability to deploy
within multiple physical and virtual datacenters and clusters to increase
reliability. Customers would also like to take advantage
of the concept of regions and zoning that this type of deployment would
offer.

- https://issues.redhat.com/browse/RFE-845
- https://issues.redhat.com/browse/RFE-4540
- https://issues.redhat.com/browse/RFE-4803
- https://issues.redhat.com/browse/RFE-5527
- https://issues.redhat.com/browse/OCPPLAN-4927
- https://issues.redhat.com/browse/OCPSTRAT-1577

### Goals

To be able to install OpenShift on vSphere with a set topology. This includes:
  - using multiple vCenters, datacenters and clusters
  - using cluster and host groups (including vm-host groups and rules)

### Non-Goals

## Existing and Proposal

Modification of the installer to support:
- the provisioning of control plane nodes in defined datacenters and clusters.
- provisioning in a stretched vSphere cluster using a cluster as a region and hosts as zones

### Workflow Description

### api

#### Infrastructure spec

Using the upstream [vSphere cluster api](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/main/apis/v1beta1/vspherefailuredomain_types.go)
and the upstream [vSphere cloud config manager](https://github.com/kubernetes/cloud-provider-vsphere/blob/master/pkg/common/config/types_yaml.go)
as examples to implement parameters of `VSpherePlatformSpec`.
These parameters include the optional and required information to manage a OpenShift cluster on vSphere.

Current platform spec before additions for vm-host based zonal
```golang
// VSpherePlatformFailureDomainSpec holds the region and zone failure domain and the vCenter topology of that failure domain.
type VSpherePlatformFailureDomainSpec struct {
	// name defines the arbitrary but unique name
	// of a failure domain.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`

	// region defines the name of a region tag that will
	// be attached to a vCenter datacenter. The tag
	// category in vCenter must be named openshift-region.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	// +required
	Region string `json:"region"`

	// zone defines the name of a zone tag that will
	// be attached to a vCenter cluster. The tag
	// category in vCenter must be named openshift-zone.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	// +required
	Zone string `json:"zone"`

	// server is the fully-qualified domain name or the IP address of the vCenter server.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// ---
	// + Validation is applied via a patch, we validate the format as either ipv4, ipv6 or hostname
	Server string `json:"server"`

	// topology describes a given failure domain using vSphere constructs
	// +required
	Topology VSpherePlatformTopology `json:"topology"`
}

// VSpherePlatformTopology holds the required and optional vCenter objects - datacenter,
// computeCluster, networks, datastore and resourcePool - to provision virtual machines.
type VSpherePlatformTopology struct {
	// datacenter is the name of vCenter datacenter in which virtual machines will be located.
	// The maximum length of the datacenter name is 80 characters.
	// +required
	// +kubebuilder:validation:MaxLength=80
	Datacenter string `json:"datacenter"`

	// computeCluster the absolute path of the vCenter cluster
	// in which virtual machine will be located.
	// The absolute path is of the form /<datacenter>/host/<cluster>.
	// The maximum length of the path is 2048 characters.
	// +required
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/host/.*?`
	ComputeCluster string `json:"computeCluster"`

	// networks is the list of port group network names within this failure domain.
	// If feature gate VSphereMultiNetworks is enabled, up to 10 network adapters may be defined.
	// 10 is the maximum number of virtual network devices which may be attached to a VM as defined by:
	// https://configmax.esp.vmware.com/guest?vmwareproduct=vSphere&release=vSphere%208.0&categories=1-0
	// The available networks (port groups) can be listed using
	// `govc ls 'network/*'`
	// Networks should be in the form of an absolute path:
	// /<datacenter>/network/<portgroup>.
	// +required
	// +openshift:validation:FeatureGateAwareMaxItems:featureGate="",maxItems=1
	// +openshift:validation:FeatureGateAwareMaxItems:featureGate=VSphereMultiNetworks,maxItems=10
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	Networks []string `json:"networks"`

	// datastore is the absolute path of the datastore in which the
	// virtual machine is located.
	// The absolute path is of the form /<datacenter>/datastore/<datastore>
	// The maximum length of the path is 2048 characters.
	// +required
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/datastore/.*?`
	Datastore string `json:"datastore"`

	// resourcePool is the absolute path of the resource pool where virtual machines will be
	// created. The absolute path is of the form /<datacenter>/host/<cluster>/Resources/<resourcepool>.
	// The maximum length of the path is 2048 characters.
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/host/.*?/Resources.*`
	// +optional
	ResourcePool string `json:"resourcePool,omitempty"`

	// folder is the absolute path of the folder where
	// virtual machines are located. The absolute path
	// is of the form /<datacenter>/vm/<folder>.
	// The maximum length of the path is 2048 characters.
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/vm/.*?`
	// +optional
	Folder string `json:"folder,omitempty"`

	// template is the full inventory path of the virtual machine or template
	// that will be cloned when creating new machines in this failure domain.
	// The maximum length of the path is 2048 characters.
	//
	// When omitted, the template will be calculated by the control plane
	// machineset operator based on the region and zone defined in
	// VSpherePlatformFailureDomainSpec.
	// For example, for zone=zonea, region=region1, and infrastructure name=test,
	// the template path would be calculated as /<datacenter>/vm/test-rhcos-region1-zonea.
	// +openshift:enable:FeatureGate=VSphereControlPlaneMachineSet
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/vm/.*?`
	// +optional
	Template string `json:"template,omitempty"`
}

// VSpherePlatformVCenterSpec stores the vCenter connection fields.
// This is used by the vSphere CCM.
type VSpherePlatformVCenterSpec struct {

	// server is the fully-qualified domain name or the IP address of the vCenter server.
	// +required
	// +kubebuilder:validation:MaxLength=255
	// ---
	// + Validation is applied via a patch, we validate the format as either ipv4, ipv6 or hostname
	Server string `json:"server"`

	// port is the TCP port that will be used to communicate to
	// the vCenter endpoint.
	// When omitted, this means the user has no opinion and
	// it is up to the platform to choose a sensible default,
	// which is subject to change over time.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32767
	// +optional
	Port int32 `json:"port,omitempty"`

	// The vCenter Datacenters in which the RHCOS
	// vm guests are located. This field will
	// be used by the Cloud Controller Manager.
	// Each datacenter listed here should be used within
	// a topology.
	// +required
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	Datacenters []string `json:"datacenters"`
}

// VSpherePlatformNodeNetworkingSpec holds the network CIDR(s) and port group name for
// including and excluding IP ranges in the cloud provider.
// This would be used for example when multiple network adapters are attached to
// a guest to help determine which IP address the cloud config manager should use
// for the external and internal node networking.
type VSpherePlatformNodeNetworkingSpec struct {
	// networkSubnetCidr IP address on VirtualMachine's network interfaces included in the fields' CIDRs
	// that will be used in respective status.addresses fields.
	// ---
	// + Validation is applied via a patch, we validate the format as cidr
	// +listType=set
	// +optional
	NetworkSubnetCIDR []string `json:"networkSubnetCidr,omitempty"`

	// network VirtualMachine's VM Network names that will be used to when searching
	// for status.addresses fields. Note that if internal.networkSubnetCIDR and
	// external.networkSubnetCIDR are not set, then the vNIC associated to this network must
	// only have a single IP address assigned to it.
	// The available networks (port groups) can be listed using
	// `govc ls 'network/*'`
	// +optional
	Network string `json:"network,omitempty"`

	// excludeNetworkSubnetCidr IP addresses in subnet ranges will be excluded when selecting
	// the IP address from the VirtualMachine's VM for use in the status.addresses fields.
	// ---
	// + Validation is applied via a patch, we validate the format as cidr
	// +listType=atomic
	// +optional
	ExcludeNetworkSubnetCIDR []string `json:"excludeNetworkSubnetCidr,omitempty"`
}

// VSpherePlatformNodeNetworking holds the external and internal node networking spec.
type VSpherePlatformNodeNetworking struct {
	// external represents the network configuration of the node that is externally routable.
	// +optional
	External VSpherePlatformNodeNetworkingSpec `json:"external"`
	// internal represents the network configuration of the node that is routable only within the cluster.
	// +optional
	Internal VSpherePlatformNodeNetworkingSpec `json:"internal"`
}

// VSpherePlatformSpec holds the desired state of the vSphere infrastructure provider.
// In the future the cloud provider operator, storage operator and machine operator will
// use these fields for configuration.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.apiServerInternalIPs) || has(self.apiServerInternalIPs)",message="apiServerInternalIPs list is required once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.ingressIPs) || has(self.ingressIPs)",message="ingressIPs list is required once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.vcenters) && has(self.vcenters) ? size(self.vcenters) < 2 : true",message="vcenters can have at most 1 item when configured post-install"
type VSpherePlatformSpec struct {
	// vcenters holds the connection details for services to communicate with vCenter.
	// Currently, only a single vCenter is supported, but in tech preview 3 vCenters are supported.
	// Once the cluster has been installed, you are unable to change the current number of defined
	// vCenters except in the case where the cluster has been upgraded from a version of OpenShift
	// where the vsphere platform spec was not present.  You may make modifications to the existing
	// vCenters that are defined in the vcenters list in order to match with any added or modified
	// failure domains.
	// ---
	// + If VCenters is not defined use the existing cloud-config configmap defined
	// + in openshift-config.
	// +kubebuilder:validation:MinItems=0
	// +openshift:validation:FeatureGateAwareMaxItems:featureGate="",maxItems=1
	// +openshift:validation:FeatureGateAwareMaxItems:featureGate=VSphereMultiVCenters,maxItems=3
	// +kubebuilder:validation:XValidation:rule="size(self) != size(oldSelf) ? size(oldSelf) == 0 && size(self) < 2 : true",message="vcenters cannot be added or removed once set"
	// +listType=atomic
	// +optional
	VCenters []VSpherePlatformVCenterSpec `json:"vcenters,omitempty"`

	// failureDomains contains the definition of region, zone and the vCenter topology.
	// If this is omitted failure domains (regions and zones) will not be used.
	// +listType=map
	// +listMapKey=name
	// +optional
	FailureDomains []VSpherePlatformFailureDomainSpec `json:"failureDomains,omitempty"`

	// nodeNetworking contains the definition of internal and external network constraints for
	// assigning the node's networking.
	// If this field is omitted, networking defaults to the legacy
	// address selection behavior which is to only support a single address and
	// return the first one found.
	// +optional
	NodeNetworking VSpherePlatformNodeNetworking `json:"nodeNetworking,omitempty"`

	// apiServerInternalIPs are the IP addresses to contact the Kubernetes API
	// server that can be used by components inside the cluster, like kubelets
	// using the infrastructure rather than Kubernetes networking. These are the
	// IPs for a self-hosted load balancer in front of the API servers.
	// In dual stack clusters this list contains two IP addresses, one from IPv4
	// family and one from IPv6.
	// In single stack clusters a single IP address is expected.
	// When omitted, values from the status.apiServerInternalIPs will be used.
	// Once set, the list cannot be completely removed (but its second entry can).
	//
	// +kubebuilder:validation:MaxItems=2
	// +kubebuilder:validation:XValidation:rule="size(self) == 2 && isIP(self[0]) && isIP(self[1]) ? ip(self[0]).family() != ip(self[1]).family() : true",message="apiServerInternalIPs must contain at most one IPv4 address and at most one IPv6 address"
	// +listType=atomic
	// +optional
	APIServerInternalIPs []IP `json:"apiServerInternalIPs"`

	// ingressIPs are the external IPs which route to the default ingress
	// controller. The IPs are suitable targets of a wildcard DNS record used to
	// resolve default route host names.
	// In dual stack clusters this list contains two IP addresses, one from IPv4
	// family and one from IPv6.
	// In single stack clusters a single IP address is expected.
	// When omitted, values from the status.ingressIPs will be used.
	// Once set, the list cannot be completely removed (but its second entry can).
	//
	// +kubebuilder:validation:MaxItems=2
	// +kubebuilder:validation:XValidation:rule="size(self) == 2 && isIP(self[0]) && isIP(self[1]) ? ip(self[0]).family() != ip(self[1]).family() : true",message="ingressIPs must contain at most one IPv4 address and at most one IPv6 address"
	// +listType=atomic
	// +optional
	IngressIPs []IP `json:"ingressIPs"`

	// machineNetworks are IP networks used to connect all the OpenShift cluster
	// nodes. Each network is provided in the CIDR format and should be IPv4 or IPv6,
	// for example "10.0.0.0/8" or "fd00::/8".
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x == y))"
	// +optional
	MachineNetworks []CIDR `json:"machineNetworks"`
}
```

vm-host zonal changes required to vsphere infrastructure objects
Changes to existing `VSpherePlatformFailureDomainSpec`
```golang

// VSpherePlatformFailureDomainSpec holds the region and zone failure domain and the vCenter topology of that failure domain.
// +openshift:validation:FeatureGateAwareXValidation:featureGate=VSphereHostVMGroupZonal,rule="has(self.zoneAffinity) && self.zoneAffinity.type == 'HostGroup' ?  has(self.regionAffinity) && self.regionAffinity.type == 'ComputeCluster' : true",message="when zoneAffinity type is HostGroup, regionAffinity type must be ComputeCluster"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=VSphereHostVMGroupZonal,rule="has(self.zoneAffinity) && self.zoneAffinity.type == 'ComputeCluster' ?  has(self.regionAffinity) && self.regionAffinity.type == 'Datacenter' : true",message="when zoneAffinity type is ComputeCluster, regionAffinity type must be Datacenter"
type VSpherePlatformFailureDomainSpec struct {

    //...
    
	// regionAffinity holds the type of region, Datacenter or ComputeCluster.
	// When set to Datacenter, this means the region is a vCenter Datacenter as defined in topology.
	// When set to ComputeCluster, this means the region is a vCenter Cluster as defined in topology.
	// +openshift:validation:featureGate=VSphereHostVMGroupZonal
	// +optional
	RegionAffinity *VSphereFailureDomainRegionAffinity `json:"regionAffinity,omitempty"`

	// zoneAffinity holds the type of the zone and the hostGroup which
	// vmGroup and the hostGroup names in vCenter corresponds to
	// a vm-host group of type Virtual Machine and Host respectively. Is also
	// contains the vmHostRule which is an affinity vm-host rule in vCenter.
	// +openshift:validation:featureGate=VSphereHostVMGroupZonal
	// +optional
	ZoneAffinity *VSphereFailureDomainZoneAffinity `json:"zoneAffinity,omitempty"`

}
```

Additional consts, types and structs for vm-host zonal, which are associated with `VSpherePlatformFailureDomainSpec` changes.
```golang
// The VSphereFailureDomainZoneType is a string representation of a failure domain
// zone type. There are two supportable types HostGroup and ComputeCluster
// +enum
type VSphereFailureDomainZoneType string

// The VSphereFailureDomainRegionType is a string representation of a failure domain
// region type. There are two supportable types ComputeCluster and Datacenter
// +enum
type VSphereFailureDomainRegionType string

const (
	// HostGroupFailureDomainZone is a failure domain zone for a vCenter vm-host group.
	HostGroupFailureDomainZone VSphereFailureDomainZoneType = "HostGroup"
	// ComputeClusterFailureDomainZone is a failure domain zone for a vCenter compute cluster.
	ComputeClusterFailureDomainZone VSphereFailureDomainZoneType = "ComputeCluster"
	// DatacenterFailureDomainRegion is a failure domain region for a vCenter datacenter.
	DatacenterFailureDomainRegion VSphereFailureDomainRegionType = "Datacenter"
	// ComputeClusterFailureDomainRegion is a failure domain region for a vCenter compute cluster.
	ComputeClusterFailureDomainRegion VSphereFailureDomainRegionType = "ComputeCluster"
)
// VSphereFailureDomainZoneAffinity contains the vCenter cluster vm-host group (virtual machine and host types)
// and the vm-host affinity rule that together creates an affinity configuration for vm-host based zonal.
// This configuration within vCenter creates the required association between a failure domain, virtual machines
// and ESXi hosts to create a vm-host based zone.
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'HostGroup' ?  has(self.hostGroup) : !has(self.hostGroup)",message="hostGroup is required when type is HostGroup, and forbidden otherwise"
// +union
type VSphereFailureDomainZoneAffinity struct {
	// type determines the vSphere object type for a zone within this failure domain.
	// Available types are ComputeCluster and HostGroup.
	// When set to ComputeCluster, this means the vCenter cluster defined is the zone.
	// When set to HostGroup, hostGroup must be configured with hostGroup, vmGroup and vmHostRule and
	// this means the zone is defined by the grouping of those fields.
	// +kubebuilder:validation:Enum:=HostGroup;ComputeCluster
	// +required
	// +unionDiscriminator
	Type VSphereFailureDomainZoneType `json:"type"`

	// hostGroup holds the vmGroup and the hostGroup names in vCenter
	// corresponds to a vm-host group of type Virtual Machine and Host respectively. Is also
	// contains the vmHostRule which is an affinity vm-host rule in vCenter.
	// +unionMember
	// +optional
	HostGroup *VSphereFailureDomainHostGroup `json:"hostGroup,omitempty"`
}

// VSphereFailureDomainRegionAffinity contains the region type which is the string representation of the
// VSphereFailureDomainRegionType with available options of Datacenter and ComputeCluster.
// +union
type VSphereFailureDomainRegionAffinity struct {
	// type determines the vSphere object type for a region within this failure domain.
	// Available types are Datacenter and ComputeCluster.
	// When set to Datacenter, this means the vCenter Datacenter defined is the region.
	// When set to ComputeCluster, this means the vCenter cluster defined is the region.
	// +kubebuilder:validation:Enum:=ComputeCluster;Datacenter
	// +required
	// +unionDiscriminator
	Type VSphereFailureDomainRegionType `json:"type"`
}

// VSphereFailureDomainHostGroup holds the vmGroup and the hostGroup names in vCenter
// corresponds to a vm-host group of type Virtual Machine and Host respectively. Is also
// contains the vmHostRule which is an affinity vm-host rule in vCenter.
type VSphereFailureDomainHostGroup struct {
	// vmGroup is the name of the vm-host group of type virtual machine within vCenter for this failure domain.
	// vmGroup is limited to 80 characters.
	// This field is required when the VSphereFailureDomain ZoneType is HostGroup
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	// +required
	VMGroup string `json:"vmGroup"`

	// hostGroup is the name of the vm-host group of type host within vCenter for this failure domain.
	// hostGroup is limited to 80 characters.
	// This field is required when the VSphereFailureDomain ZoneType is HostGroup
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	// +required
	HostGroup string `json:"hostGroup"`

	// vmHostRule is the name of the affinity vm-host rule within vCenter for this failure domain.
	// vmHostRule is limited to 80 characters.
	// This field is required when the VSphereFailureDomain ZoneType is HostGroup
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	// +required
	VMHostRule string `json:"vmHostRule"`
}
```

###  Cluster Cloud Controller Manager Operator (CCCMO or 3cmo)

The 3cmo translates existing legacy in-tree cloud provider config to the external CCM config.
If the Infrastructure vSphere spec is provided the 3cmo will include those fields in the configuration: 
internal and external network, and vCenter configuration.
This allows the day two configuration of vSphere zonal if the failure domains length is greater than
one the CCM config label section is defined.

### Cluster API and CAPV provider (near future) 

The introduction of CAPI vSphere-based machines will require additional manifests to be created and applied
to the cluster being installed (meaning to not only the cluster-api instance but the cluster being built).
These new manifests will need to include the following objects:

- VSphereCluster
- VSphereDeploymentZone
- VSphereFailureDomain
- VSphereMachine

`VSphereCluster` already exists for the installer to utilize CAPV. The two new objects 
`VSphereDeploymentZone` and `VSphereFailureDomain` were added to the installer to support
vm-host group zonal. CAPV makes it significantly easier to deploy into vm-host group zonal
with just manifest creation.

### installer
#### Platform Spec

The platform spec needs support vSphere topology and zonal deployment. 
This platform spec design is based on the changes api, Cluster API for vSphere
and out-of-tree CCM.

`VCenters` will contain the connections and configuration for each vCenter
that is required for the out-of-tree CCM. 

`FailureDomains` will define the configuration of a region, zone, and topology.
`Topology` defines the vCenter objects that make up a region or zone
including `Datacenter`, `ComputeCluster`, `Hosts`, `Networks`, `Datastore` and 
optionally `ResourcePool`, `Folder`, `Template`, and `TagIDs`.

The vm-host group zonal will add a field of `hostGroup` which needs to pre-exist prior to installation.

The existing platform spec vcenter parameters are deprecated
but not removed or remove support for using those parameters. The deprecated
platform spec though will not gain the new features that failure domains provides.

```golang
package vsphere

import (
	configv1 "github.com/openshift/api/config/v1"
)

// DiskType is a disk provisioning type for vsphere.
// +kubebuilder:validation:Enum="";thin;thick;eagerZeroedThick
type DiskType string

// FailureDomainType is the string representation name of the failure domain type.
// There are two defined failure domains currently, Datacenter and ComputeCluster.
// Each represents a vCenter object type within a vSphere environment.
// +kubebuilder:validation:Enum=HostGroup;Datacenter;ComputeCluster
type FailureDomainType string

const (
	// DiskTypeThin uses Thin disk provisioning type for vsphere in the cluster.
	DiskTypeThin DiskType = "thin"

	// DiskTypeThick uses Thick disk provisioning type for vsphere in the cluster.
	DiskTypeThick DiskType = "thick"

	// DiskTypeEagerZeroedThick uses EagerZeroedThick disk provisioning type for vsphere in the cluster.
	DiskTypeEagerZeroedThick DiskType = "eagerZeroedThick"

	// TagCategoryRegion the tag category associated with regions.
	TagCategoryRegion = "openshift-region"

	// TagCategoryZone the tag category associated with zones.
	TagCategoryZone = "openshift-zone"
)

const (
	// ControlPlaneRole represents control-plane nodes.
	ControlPlaneRole = "control-plane"
	// ComputeRole represents worker nodes.
	ComputeRole = "compute"
	// BootstrapRole represents bootstrap nodes.
	BootstrapRole = "bootstrap"
)

const (
	// HostGroupFailureDomain is a failure domain for a vCenter vm-host group.
	HostGroupFailureDomain FailureDomainType = "HostGroup"
	// ComputeClusterFailureDomain is a failure domain for a vCenter compute cluster.
	ComputeClusterFailureDomain FailureDomainType = "ComputeCluster"
	// DatacenterFailureDomain is a failure domain for a vCenter datacenter.
	DatacenterFailureDomain FailureDomainType = "Datacenter"
)

// Platform stores any global configuration used for vsphere platforms.
type Platform struct {
	// VCenter is the domain name or IP address of the vCenter.
	// Deprecated: Use VCenters.Server
	DeprecatedVCenter string `json:"vCenter,omitempty"`
	// Username is the name of the user to use to connect to the vCenter.
	// Deprecated: Use VCenters.Username
	DeprecatedUsername string `json:"username,omitempty"`
	// Password is the password for the user to use to connect to the vCenter.
	// Deprecated: Use VCenters.Password
	DeprecatedPassword string `json:"password,omitempty"`
	// Datacenter is the name of the datacenter to use in the vCenter.
	// Deprecated: Use FailureDomains.Topology.Datacenter
	DeprecatedDatacenter string `json:"datacenter,omitempty"`
	// DefaultDatastore is the default datastore to use for provisioning volumes.
	// Deprecated: Use FailureDomains.Topology.Datastore
	DeprecatedDefaultDatastore string `json:"defaultDatastore,omitempty"`
	// Folder is the absolute path of the folder that will be used and/or created for
	// virtual machines. The absolute path is of the form /<datacenter>/vm/<folder>/<subfolder>.
	// +kubebuilder:validation:Pattern=`^/.*?/vm/.*?`
	// +optional
	// Deprecated: Use FailureDomains.Topology.Folder
	DeprecatedFolder string `json:"folder,omitempty"`
	// Cluster is the name of the cluster virtual machines will be cloned into.
	// Deprecated: Use FailureDomains.Topology.Cluster
	DeprecatedCluster string `json:"cluster,omitempty"`
	// ResourcePool is the absolute path of the resource pool where virtual machines will be
	// created. The absolute path is of the form /<datacenter>/host/<cluster>/Resources/<resourcepool>.
	// Deprecated: Use FailureDomains.Topology.ResourcePool
	DeprecatedResourcePool string `json:"resourcePool,omitempty"`
	// ClusterOSImage overrides the url provided in rhcos.json to download the RHCOS OVA
	ClusterOSImage string `json:"clusterOSImage,omitempty"`

	// DeprecatedAPIVIP is the virtual IP address for the api endpoint
	// Deprecated: Use APIVIPs
	//
	// +kubebuilder:validation:format=ip
	// +optional
	DeprecatedAPIVIP string `json:"apiVIP,omitempty"`

	// APIVIPs contains the VIP(s) for the api endpoint. In dual stack clusters
	// it contains an IPv4 and IPv6 address, otherwise only one VIP
	//
	// +kubebuilder:validation:MaxItems=2
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:Format=ip
	// +optional
	APIVIPs []string `json:"apiVIPs,omitempty"`

	// DeprecatedIngressVIP is the virtual IP address for ingress
	// Deprecated: Use IngressVIPs
	//
	// +kubebuilder:validation:format=ip
	// +optional
	DeprecatedIngressVIP string `json:"ingressVIP,omitempty"`

	// IngressVIPs contains the VIP(s) for ingress. In dual stack clusters it
	// contains an IPv4 and IPv6 address, otherwise only one VIP
	//
	// +kubebuilder:validation:MaxItems=2
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:Format=ip
	// +optional
	IngressVIPs []string `json:"ingressVIPs,omitempty"`

	// DefaultMachinePlatform is the default configuration used when
	// installing on VSphere for machine pools which do not define their own
	// platform configuration.
	// +optional
	DefaultMachinePlatform *MachinePool `json:"defaultMachinePlatform,omitempty"`
	// Network specifies the name of the network to be used by the cluster.
	// Deprecated: Use FailureDomains.Topology.Network
	DeprecatedNetwork string `json:"network,omitempty"`
	// DiskType is the name of the disk provisioning type,
	// valid values are thin, thick, and eagerZeroedThick. When not
	// specified, it will be set according to the default storage policy
	// of vsphere.
	DiskType DiskType `json:"diskType,omitempty"`
	// VCenters holds the connection details for services to communicate with vCenter.
	// Currently only a single vCenter is supported.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=3
	// +kubebuilder:validation:MinItems=1
	VCenters []VCenter `json:"vcenters,omitempty"`
	// FailureDomains holds the VSpherePlatformFailureDomainSpec which contains
	// the definition of region, zone and the vCenter topology.
	// If this is omitted failure domains (regions and zones) will not be used.
	// +kubebuilder:validation:Optional
	FailureDomains []FailureDomain `json:"failureDomains,omitempty"`

	// nodeNetworking contains the definition of internal and external network constraints for
	// assigning the node's networking.
	// If this field is omitted, networking defaults to the legacy
	// address selection behavior which is to only support a single address and
	// return the first one found.
	// +optional
	NodeNetworking *configv1.VSpherePlatformNodeNetworking `json:"nodeNetworking,omitempty"`

	// LoadBalancer defines how the load balancer used by the cluster is configured.
	// LoadBalancer is available in TechPreview.
	// +optional
	LoadBalancer *configv1.VSpherePlatformLoadBalancer `json:"loadBalancer,omitempty"`
	// Hosts defines network configurations to be applied by the installer. Hosts is available in TechPreview.
	Hosts []*Host `json:"hosts,omitempty"`
}

// FailureDomain holds the region and zone failure domain and
// the vCenter topology of that failure domain.
type FailureDomain struct {
	// name defines the name of the FailureDomain
	// This name is arbitrary but will be used
	// in VSpherePlatformDeploymentZone for association.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`
	// region defines a FailureDomainCoordinate which
	// includes the name of the vCenter tag, the failure domain type
	// and the name of the vCenter tag category.
	// +kubebuilder:validation:Required
	Region string `json:"region"`
	// zone defines a VSpherePlatformFailureDomain which
	// includes the name of the vCenter tag, the failure domain type
	// and the name of the vCenter tag category.
	// +kubebuilder:validation:Required
	Zone string `json:"zone"`
	// server is the fully-qualified domain name or the IP address of the vCenter server.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Server string `json:"server"`
	// Topology describes a given failure domain using vSphere constructs
	// +kubebuilder:validation:Required
	Topology Topology `json:"topology"`

	// Type is the type of failure domain, the current values are "Datacenter", "ComputeCluster" and "HostGroup"
	// +kubebuilder:validation:Enum=Datacenter;ComputeCluster
	// +optional
	RegionType FailureDomainType `json:"regionType,omitempty"`
	// Type is the type of failure domain, the current values are "Datacenter", "ComputeCluster" and "HostGroup"
	// +kubebuilder:validation:Enum=ComputeCluster;HostGroup
	// +optional
	ZoneType FailureDomainType `json:"zoneType,omitempty"`
}

// Topology holds the required and optional vCenter objects - datacenter,
// computeCluster, networks, datastore and resourcePool - to provision virtual machines.
type Topology struct {
	// datacenter is the vCenter datacenter in which virtual machines will be located
	// and defined as the failure domain.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	Datacenter string `json:"datacenter"`
	// computeCluster as the failure domain
	// This is required to be a path
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	ComputeCluster string `json:"computeCluster"`
	// networks is the list of networks within this failure domain
	Networks []string `json:"networks,omitempty"`
	// datastore is the name or inventory path of the datastore in which the
	// virtual machine is created/located.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	Datastore string `json:"datastore"`
	// resourcePool is the absolute path of the resource pool where virtual machines will be
	// created. The absolute path is of the form /<datacenter>/host/<cluster>/Resources/<resourcepool>.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/host/.*?/Resources.*`
	// +optional
	ResourcePool string `json:"resourcePool,omitempty"`
	// folder is the inventory path of the folder in which the
	// virtual machine is created/located.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/vm/.*?`
	// +optional
	Folder string `json:"folder,omitempty"`
	// template is the inventory path of the virtual machine or template
	// that will be used for cloning.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/vm/.*?`
	// +optional
	Template string `json:"template,omitempty"`
	// tagIDs is an optional set of tags to add to an instance. Specified tagIDs
	// must use URN-notation instead of display names. A maximum of 10 tag IDs may be specified.
	// +kubebuilder:example=`urn:vmomi:InventoryServiceTag:5736bf56-49f5-4667-b38c-b97e09dc9578:GLOBAL`
	// +optional
	TagIDs []string `json:"tagIDs,omitempty"`

	// hostGroup is the name of the vm-host group of type host within vCenter for this failure domain.
	// This field is required when the FailureDomain zoneType is HostGroup
	// +kubebuilder:validation:MaxLength=80
	// +optional
	HostGroup string `json:"hostGroup,omitempty"`
}

// VCenter stores the vCenter connection fields
// https://github.com/kubernetes/cloud-provider-vsphere/blob/master/pkg/common/config/types_yaml.go
type VCenter struct {
	// server is the fully-qualified domain name or the IP address of the vCenter server.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=255
	Server string `json:"server"`
	// port is the TCP port that will be used to communicate to
	// the vCenter endpoint. This is typically unchanged from
	// the default of HTTPS TCP/443.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32767
	// +kubebuilder:default=443
	Port int32 `json:"port,omitempty"`
	// Username is the username that will be used to connect to vCenter
	// +kubebuilder:validation:Required
	Username string `json:"user"`
	// Password is the password for the user to use to connect to the vCenter.
	// +kubebuilder:validation:Required
	Password string `json:"password"`
	// Datacenter in which VMs are located.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Datacenters []string `json:"datacenters"`
}

// Host defines host VMs to generate as part of the installation.
type Host struct {
	// FailureDomain refers to the name of a FailureDomain as described in https://github.com/openshift/enhancements/blob/master/enhancements/installer/vsphere-ipi-zonal.md
	// +optional
	FailureDomain string `json:"failureDomain"`
	// NetworkDeviceSpec to be applied to the host
	// +kubebuilder:validation:Required
	NetworkDevice *NetworkDeviceSpec `json:"networkDevice"`
	// Role defines the role of the node
	// +kubebuilder:validation:Enum="";bootstrap;control-plane;compute
	// +kubebuilder:validation:Required
	Role string `json:"role"`
}

// NetworkDeviceSpec defines network config for static IP assignment.
type NetworkDeviceSpec struct {
	// gateway is an IPv4 or IPv6 address which represents the subnet gateway,
	// for example, 192.168.1.1.
	// +kubebuilder:validation:Format=ipv4
	// +kubebuilder:validation:Format=ipv6
	Gateway string `json:"gateway,omitempty"`

	// ipAddrs is a list of one or more IPv4 and/or IPv6 addresses and CIDR to assign to
	// this device, for example, 192.168.1.100/24. IP addresses provided via ipAddrs are
	// intended to allow explicit assignment of a machine's IP address.
	// +kubebuilder:validation:Format=ipv4
	// +kubebuilder:validation:Format=ipv6
	// +kubebuilder:example=`192.168.1.100/24`
	// +kubebuilder:example=`2001:DB8:0000:0000:244:17FF:FEB6:D37D/64`
	// +kubebuilder:validation:Required
	IPAddrs []string `json:"ipAddrs"`

	// nameservers is a list of IPv4 and/or IPv6 addresses used as DNS nameservers, for example,
	// 8.8.8.8. a nameserver is not provided by a fulfilled IPAddressClaim. If DHCP is not the
	// source of IP addresses for this network device, nameservers should include a valid nameserver.
	// +kubebuilder:validation:Format=ipv4
	// +kubebuilder:validation:Format=ipv6
	// +kubebuilder:example=`8.8.8.8`
	Nameservers []string `json:"nameservers,omitempty"`
}

// IsControlPlane checks if the current host is a master.
func (h *Host) IsControlPlane() bool {
	return h.Role == ControlPlaneRole
}

// IsCompute checks if the current host is a worker.
func (h *Host) IsCompute() bool {
	return h.Role == ComputeRole
}

// IsBootstrap checks if the current host is a bootstrap.
func (h *Host) IsBootstrap() bool {
	return h.Role == BootstrapRole
}
```

#### Set infrastructure spec

With the changes to the openshift/api infrastructure config
the [infrastructure manifest asset generation](https://github.com/openshift/installer/blob/master/pkg/asset/manifests/infrastructure.go#L188-L195) will need to be modified to
generate VSpherePlatformSpec and provide the values from the install-config platform spec.

#### MachinePool

The `Zones` like the other cloud providers will determine the location
where the virtual machine will reside within a vSphere environment.

```golang
type MachinePool struct {
...
	Zones []string `json:"zones,omitempty"`
}
```

##### Creating CAPV `VSphereFailureDomains` and `VSphereDeploymentZones` for host zonal

To implement host zonal in vSphere the virtual machines need to be added to a vm group.
CAPV gives this capability by creating `VSphereFailureDomains` and `VSphereDeploymentZones`

```golang
for _, failureDomain := range installConfig.Config.VSphere.FailureDomains {
	if failureDomain.ZoneType == vsphere.HostGroupFailureDomain {
		dz := &capv.VSphereDeploymentZone{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: failureDomain.Name,
			},
			Spec: capv.VSphereDeploymentZoneSpec{
				Server:        fmt.Sprintf("https://%s", failureDomain.Server),
				FailureDomain: failureDomain.Name,
				ControlPlane:  ptr.To(true),
				PlacementConstraint: capv.PlacementConstraint{
					ResourcePool: failureDomain.Topology.ResourcePool,
					Folder:       failureDomain.Topology.Folder,
				},
			},
		}

		dz.SetGroupVersionKind(capv.GroupVersion.WithKind("VSphereDeploymentZone"))

		fd := &capv.VSphereFailureDomain{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: failureDomain.Name,
			},
			Spec: capv.VSphereFailureDomainSpec{
				Region: capv.FailureDomain{
					Name:        failureDomain.Region,
					Type:        capv.FailureDomainType(failureDomain.RegionType),
					TagCategory: "openshift-region",
				},
				Zone: capv.FailureDomain{
					Name:        failureDomain.Zone,
					Type:        capv.FailureDomainType(failureDomain.ZoneType),
					TagCategory: "openshift-zone",
				},
				Topology: capv.Topology{
					Datacenter:     failureDomain.Topology.Datacenter,
					ComputeCluster: &failureDomain.Topology.ComputeCluster,
					Hosts: &capv.FailureDomainHosts{
						VMGroupName:   fmt.Sprintf("%s-%s", clusterID.InfraID, failureDomain.Name),
						HostGroupName: failureDomain.Topology.HostGroup,
					},
					Networks:  failureDomain.Topology.Networks,
					Datastore: failureDomain.Topology.Datastore,
				},
			},
		}
		fd.SetGroupVersionKind(capv.GroupVersion.WithKind("VSphereFailureDomain"))

		manifests = append(manifests, &asset.RuntimeFile{
			Object: fd,
			File:   asset.File{Filename: fmt.Sprintf("01_vsphere-failuredomain-%s.yaml", failureDomain.Name)},
		})

		manifests = append(manifests, &asset.RuntimeFile{
			Object: dz,
			File:   asset.File{Filename: fmt.Sprintf("01_vsphere-deploymentzone-%s.yaml", failureDomain.Name)},
		})
	}
}

```

#### Cloud Config

The external CCM is required for this change. The out-of-tree CCM also updates
the cloud-config ini configuration. Below is an example of what the
cloud-config will change to. A `VirtualCenter` section will be added per
vcenter in `vcenters`. `datacenters` is a comma-delimited string that will contain
all the datacenters per region.

```ini
[Global]
  insecure-flag = "1"
  secret-name = "vsphere-creds"
  secret-namespace = "kube-system"

[VirtualCenter "10.0.0.1"]
  datacenters = "SDDC-Datacenter"

[Labels]
  region = openshift-region
  zone = openshift-zone
```

In a future release the installer will change from ini to yaml once all operators support it.

#### Text-based User Interface (TUI)

There are too many options to support this configuration.
Deploying a topology with failure domains will only be supported via
the `install-config.yaml`

#### The use of Cluster API and the vSphere provider in the Installer

The installer uses CAPI and the CAPV provider to provision the bootstrap and control plane nodes for vSphere.
Terraform is no longer used for vSphere installation.

#### Machine and MachineSet

The control plane machines are now created with CAPI/CAPV and a capv machine is used for deployment.
No modification is required to the machine object to support vm-host zonal.

The compute workspace will add a single field `vmGroup` to indicate that 
the guest needs to be added to this vm-host group of type virtual machine.

##### OVA import

For each `FailureDomain` an ova import will need to occur.
If there is only a single zone then a single import will be required.


##### vm-host zonal specifics

The vCenter vm-host group of type host is required for each zone prior to installation. Each `FailureDomain` has
a `hostGroup` field that is required when `zoneType` is `HostGroup`. The vCenter vm-host group will contain
the list of ESXi hosts that are associated to that zone. 

Tags will continue to also be required prior to installation. The tag category openshift-region
will be associated with a tag created and applied to the vCenter cluster object. The tag category openshift-zone
will be associated with a tag create and applied to each ESXi host in the zone, which is also defined by the vm-host group (type host).

The installer will create a vm-host group of type virtual machine per failure domain. 
It will also create a vm-host group rule per failure domain.

### User Stories

- https://issues.redhat.com/browse/RFE-845
- https://issues.redhat.com/browse/OCPPLAN-4927
- https://issues.redhat.com/browse/SPLAT-320

### API Extensions

### Risks and Mitigations

## Design Details

### Scenario #1 - Datacenter-based region, cluster-based zone

```yaml
platform:                                                                                                                                                                          
  vsphere:
    apiVIP: 10.38.201.130
    ingressVIP: 10.38.201.131
    vcenters:
    - server: vcenter.ci.ibmc.devcluster.openshift.com
      user: ''
      password: ''
      datacenters:
      - cidatacenter
    failureDomains:
    - name: us-east-1
      region: us-east
      zone: us-east-1a
      server: vcenter.ci.ibmc.devcluster.openshift.com
      topology:
        datacenter: cidatacenter
        computeCluster: /cidatacenter/host/cicluster
        networks:
        - ci-vlan-1287
        datastore: /cidatacenter/datastore/vsanDatastore
    - name: us-east-2
      region: us-east
      zone: us-east-2a
      server: vcenter.ci.ibmc.devcluster.openshift.com
      topology:
        datacenter: cidatacenter
        computeCluster: /cidatacenter/host/cicluster2
        networks:
        - ci-vlan-1287
        datastore: /cidatacenter/datastore/vsanDatastore
```

### Scenario #2 - Cluster-based region, Host-based zone

```yaml
platform:                                                                                                                                                                          
  vsphere:        
    apiVIP: 10.93.60.130              
    ingressVIP: 10.93.60.131
    vcenters:                     
    - server: 10.93.60.138                                                                                                                                                         
      user: administrator@vsphere.local
      password: ''
      datacenters:
      - nested8-datacenter
    failureDomains:                                                                                                                                                                
    - name: us-east-1                                                                                                                                                              
      region: us-east                                                                                                                                                              
      regionType: ComputeCluster                                                                                                                                                   
      zone: us-east-1a
      zoneType: HostGroup                                                                
      server: 10.93.60.138
      topology:
        datacenter: nested8-datacenter
        computeCluster: /nested8-datacenter/host/nested-cluster
        networks:
        - VM Network
        datastore:  /nested8-datacenter/datastore/fs-cicluster-nfs
        hostGroup: us-east-1a 
    - name: us-east-2
      region: us-east
      regionType: ComputeCluster
      zone: us-east-2a
      zoneType: HostGroup
      server: 10.93.60.138
      topology:
        datacenter: nested8-datacenter
        computeCluster: /nested8-datacenter/host/nested-cluster
        networks:
        - VM Network
        datastore:  /nested8-datacenter/datastore/fs-cicluster-nfs
        hostGroup: us-east-2a 
```

### Open Questions

### Test Plan


### Graduation Criteria

#### Tech Preview -> GA


#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

- If installer-based configuration there is no upgrade path to support this
configuration - maybe documentation.

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

### Drawbacks

- Extensive modifications to installer

## Alternatives

- None

## Infrastructure Needed

Using existing infrastructure but nested provisioning is required to test host-based zonal which
has been implemented with: 
- https://github.com/openshift/release/pull/61257
- https://github.com/openshift/release/pull/57681
