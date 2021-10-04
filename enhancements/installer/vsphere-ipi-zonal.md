---
title: vsphere-ipi-zonal 
authors:
  - "@jcpowermac"
reviewers:
  - "@rvanderp3"
  - "@bostrt"
  - "@JoelSpeed"
approvers:
  - "@rvanderp3"
  - "@patrickdillon"
api-approvers:
  - "@JoelSpeed"
  - "@deads2k"
creation-date: 2021-09-21
last-updated: 2022-08-24
status: implementable
see-also:
  - "/enhancements/"
replaces:
  -
superseded-by:
  -
tracking-link:
- https://issues.redhat.com/browse/SPLAT-320

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
vSphere environment with multiple datacenters and clusters.

This will be an opinionated design, the vCenter datacenter will always be a `region`
and a vCenter cluster will always be a `zone`.

## Motivation

Users of OpenShift would like the ability to deploy
within multiple datacenters and clusters to increase
reliability. Customers would also like to take advantage
of the concept of regions and zoning that this type of deployment would
offer.

- https://issues.redhat.com/browse/RFE-845
- https://issues.redhat.com/browse/OCPPLAN-4927

### Goals

- Support regions and zones in vSphere using multiple datacenters (region) and
clusters (zone)
- Support installation into multiple datacenters and multiple clusters

### Non-Goals

- Support multiple subnets
- Support multiple vcenters

Note: The platform spec will be modified to support this at a future date.
Only a single item in `vcenters` will be supported for the initial release.

## Proposal

Modification of the installer to support the provisioning of masters in defined
datacenters and clusters.

### Workflow Description

### api

#### Infrastructure

Using the upstream [vSphere cluster api](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/main/apis/v1beta1/vspherefailuredomain_types.go)
and the upstream [vSphere cloud config manager](https://github.com/kubernetes/cloud-provider-vsphere/blob/master/pkg/common/config/types_yaml.go)
as examples to implement parameters of `VSpherePlatformSpec`.
These parameters include the optional and required information to manage a OpenShift cluster on vSphere.

```golang
// VSpherePlatformFailureDomainSpec holds the region and zone failure domain and
// the vCenter topology of that failure domain.
type VSpherePlatformFailureDomainSpec struct {
	// name defines the arbitrary but unique name
	// of a failure domain.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`

	// region defines the name of a region tag that will
	// be attached to a vCenter datacenter. The tag
	// category in vCenter must be named openshift-region.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// zone defines the name of a zone tag that will
	// be attached to a vCenter cluster. The tag
	// category in vCenter must be named openshift-zone.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	// +kubebuilder:validation:Required
	Zone string `json:"zone"`

	// server is the fully-qualified domain name or the IP address of the vCenter server.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// ---
	// + Validation is applied via a patch, we validate the format as either ipv4, ipv6 or hostname
	Server string `json:"server"`

	// Topology describes a given failure domain using vSphere constructs
	// +kubebuilder:validation:Required
	Topology VSpherePlatformTopology `json:"topology"`
}

// VSpherePlatformTopology holds the required and optional vCenter objects - datacenter,
// computeCluster, networks, datastore and resourcePool - to provision virtual machines.
type VSpherePlatformTopology struct {
	// datacenter is the name of vCenter datacenter in which virtual machines will be located.
	// The maximum length of the datacenter name is 80 characters.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=80
	Datacenter string `json:"datacenter"`

	// computeCluster the absolute path of the vCenter cluster
	// in which virtual machine will be located.
	// The absolute path is of the form /<datacenter>/host/<cluster>.
	// The maximum length of the path is 2048 characters.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^/.*?/host/.*?`
	ComputeCluster string `json:"computeCluster"`

	// networks is the list of port group network names within this failure domain.
	// Currently, we only support a single interface per RHCOS virtual machine.
	// The available networks (port groups) can be listed using
	// `govc ls 'network/*'`
	// The single interface should be the absolute path of the form
	// /<datacenter>/network/<portgroup>.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxItems=1
	// +kubebuilder:validation:MinItems=1
	Networks []string `json:"networks"`

	// datastore is the absolute path of the datastore in which the
	// virtual machine is located.
	// The absolute path is of the form /<datacenter>/datastore/<datastore>
	// The maximum length of the path is 2048 characters.
	// +kubebuilder:validation:Required
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
}

// VSpherePlatformVCenterSpec stores the vCenter connection fields.
// This is used by the vSphere CCM.
type VSpherePlatformVCenterSpec struct {

	// server is the fully-qualified domain name or the IP address of the vCenter server.
	// +kubebuilder:validation:Required
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
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
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
type VSpherePlatformSpec struct {
	// vcenters holds the connection details for services to communicate with vCenter.
	// Currently, only a single vCenter is supported.
	// ---
	// + If VCenters is not defined use the existing cloud-config configmap defined
	// + in openshift-config.
	// +openshift:enable:FeatureSets=TechPreviewNoUpgrade
	// +kubebuilder:validation:MaxItems=1
	// +kubebuilder:validation:MinItems=0
	// +optional
	VCenters []VSpherePlatformVCenterSpec `json:"vcenters,omitempty"`

	// failureDomains contains the definition of region, zone and the vCenter topology.
	// If this is omitted failure domains (regions and zones) will not be used.
	// +openshift:enable:FeatureSets=TechPreviewNoUpgrade
	// +optional
	FailureDomains []VSpherePlatformFailureDomainSpec `json:"failureDomains,omitempty"`

	// nodeNetworking contains the definition of internal and external network constraints for
	// assigning the node's networking.
	// If this field is omitted, networking defaults to the legacy
	// address selection behavior which is to only support a single address and
	// return the first one found.
	// +openshift:enable:FeatureSets=TechPreviewNoUpgrade
	// +optional
	NodeNetworking VSpherePlatformNodeNetworking `json:"nodeNetworking,omitempty"`
}
```

### CCCMO

The CCCMO cloud-config transformation already exists but will 
need to be modified to support the VSpherePlatformSpec.

### installer

#### Platform Spec

The platform spec needs to be modified to support our initial goals of multiple
datacenters (regions) and clusters (zones) and vcenters. This platform spec
design is based on the changes suggested in openshift/api, Cluster API for vSphere
and out-of-tree CCM.

We are adding multiple additional parameters to the Platform struct:

- `VCenters`
- `FailureDomains`

`VCenters` will contain the connections and configuration for each vCenter
that is required for the out-of-tree CCM. Note: Only a single vCenter will
be supported by this effort. While the out-of-tree CCM supports multiple vCenters
the out-of-tree CSI does not.

`FailureDomains` will define the configuration of a region, zone, and topology.
`Topology` defines the vCenter objects that make up a region or zone
including `Datacenter`, `ComputeCluster`, `Hosts`, `Networks` and a `Datastore`.

The existing platform spec vcenter parameters will be deprecated
but _not_ removed or remove support for using those parameters.

This is an extension of the existing platform spec. The parameters below
will be added to the existing platform struct.

```golang
package vsphere

// DiskType is a disk provisioning type for vsphere.
// +kubebuilder:validation:Enum="";thin;thick;eagerZeroedThick
type DiskType string

const (
	// DiskTypeThin uses Thin disk provisioning type for vsphere in the cluster.
	DiskTypeThin DiskType = "thin"

	// DiskTypeThick uses Thick disk provisioning type for vsphere in the cluster.
	DiskTypeThick DiskType = "thick"

	// DiskTypeEagerZeroedThick uses EagerZeroedThick disk provisioning type for vsphere in the cluster.
	DiskTypeEagerZeroedThick DiskType = "eagerZeroedThick"
)

// Platform stores any global configuration used for vsphere platforms
type Platform struct {
	// VCenter is the domain name or IP address of the vCenter.
	VCenter string `json:"vCenter"`
	// Username is the name of the user to use to connect to the vCenter.
	Username string `json:"username"`
	// Password is the password for the user to use to connect to the vCenter.
	Password string `json:"password"`
	// Datacenter is the name of the datacenter to use in the vCenter.
	Datacenter string `json:"datacenter"`
	// DefaultDatastore is the default datastore to use for provisioning volumes.
	DefaultDatastore string `json:"defaultDatastore"`
	// Folder is the absolute path of the folder that will be used and/or created for
	// virtual machines. The absolute path is of the form /<datacenter>/vm/<folder>/<subfolder>.
	Folder string `json:"folder,omitempty"`
	// Cluster is the name of the cluster virtual machines will be cloned into.
	Cluster string `json:"cluster,omitempty"`
	// ResourcePool is the absolute path of the resource pool where virtual machines will be
	// created. The absolute path is of the form /<datacenter>/host/<cluster>/Resources/<resourcepool>.
	ResourcePool string `json:"resourcePool,omitempty"`
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
	Network string `json:"network,omitempty"`
	// DiskType is the name of the disk provisioning type,
	// valid values are thin, thick, and eagerZeroedThick. When not
	// specified, it will be set according to the default storage policy
	// of vsphere.
	DiskType DiskType `json:"diskType,omitempty"`
	// vcenters holds the connection details for services to communicate with vCenter.
	// Currently only a single vCenter is supported.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=1
	// +kubebuilder:validation:MinItems=1
	VCenters []VCenter `json:"vcenters,omitempty"`
	// failureDomains holds the VSpherePlatformFailureDomainSpec which contains
	// the definition of region, zone and the vCenter topology.
	// If this is omitted failure domains (regions and zones) will not be used.
	// +kubebuilder:validation:Optional
	FailureDomains []FailureDomain `json:"failureDomains,omitempty"`
}

// FailureDomain holds the region and zone failure domain and
// the vCenter topology of that failure domain.
type FailureDomain struct {
	// name defines the name of the FailureDomain.
	// This name is abritrary.
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
	// +kubebuilder:validation:Optional
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
	// folder is the name or inventory path of the folder in which the
	// virtual machine is created/located.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	Folder string `json:"folder,omitempty"`
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
	Port uint `json:"port,omitempty"`
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
```

Add [platform validation](https://github.com/openshift/installer/blob/master/pkg/types/vsphere/validation/platform.go)
for the new struct fields that are required.


#### Set infrastructure spec

With the changes to the openshift/api infrastructure config
the [infrastructure manifest asset generation](https://github.com/openshift/installer/blob/master/pkg/asset/manifests/infrastructure.go#L188-L195) will need to be modified to
generate VSpherePlatformSpec and provide the values from the install-config platform spec.

#### MachinePool

The `MachinePool` [struct](https://github.com/openshift/installer/blob/master/pkg/types/vsphere/machinepool.go#L5-L26)
needs a single change to include the zones.
The `Zones` like the other cloud providers will determine the location
where the virtual machine will reside within a vSphere environment.

```golang
type MachinePool struct {
...
	Zones []string `json:"zones,omitempty"`
}
```

Add [machinepool validation](https://github.com/openshift/installer/blob/master/pkg/types/vsphere/validation/machinepool.go)

#### Cloud Config

The out-of-tree CCM is required for this change. The out-of-tree CCM also updates
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

The external CCM will not be installed by default in 4.12. As a result
for 4.12 a vSphere zonal installation will also require TechPreviewNoUpgrade
to be enabled. The installer will properly configure the cloud-config
based on this requirement.

#### Text-based User Interface (TUI)

There are too many options to support this configuration.
Deploying into multiple datacenters/clusters will only be supported via
the `install-config.yaml`

#### Terraform

Terraform will need to change to support cloning the control plane virtual
machines in multiple datacenters and clusters.

With the added information provided in the platform spec from the
`VCenters` and `FailureDomains` includes
all the parameters we will need to create the appropriate tags,
tag categories and vCenter objects to provision RHCOS instances.

##### Terraform variables and TFVarsSources

The terraform `config` struct will need to be modified.
`FailureDomains` provide the vSphere objects that are needed
for importing the OVA. Each `FailureDomain` will have an 
individual template. `NetworksInFailureDomains` contains
the managed object id for each port group name.
The `ControlPlanes` is a list of the Machine Provider Spec
which contains all the required parameters for provisioning 
the control plane RHCOS guests. And finally `DatacentersFolders`
is a map with the key a union of the datacenter and folder name.
`folder` contains `Datacenter` and `Name` of the vCenter folder. 


```golang

type config struct {
	VSphereURL        string          `json:"vsphere_url"`
  ...

	// vcenters can still remain a map for easy lookups
	VCenters       map[string]vtypes.VCenter `json:"vsphere_vcenters"`
	FailureDomains []vtypes.FailureDomain    `json:"vsphere_failure_domains"`
	NetworksInFailureDomains map[string]string `json:"vsphere_networks"`
	ControlPlanes []*machineapi.VSphereMachineProviderSpec `json:"vsphere_control_planes"`
	DatacentersFolders map[string]*folder `json:"vsphere_folders"`
}
```

#### Machine and MachineSet

The control plane
[Machines](https://github.com/openshift/installer/blob/b0b96468893db2240e82ba2aa0935679c8c49201/pkg/asset/machines/vsphere/machines.go#L19-L64)
will need to be modified to create a Machine per `FailureDomain`.

For each `FailureDomain` an additional `MachineSet` for
the compute nodes will need to be created based on the `MachinePool`
`zones` configuration.

##### OVA import

For each `FailureDomain` a ova import will need to occur.
If there is only a single zone then a single import will be required.

### User Stories

- https://issues.redhat.com/browse/RFE-845
- https://issues.redhat.com/browse/OCPPLAN-4927
- https://issues.redhat.com/browse/SPLAT-320

### API Extensions


### Risks and Mitigations

- The out-of-tree CCM is required for this work. It will need to be enabled at
installation time.

## Design Details

The vSphere platform spec and configuration will change to include `vcenters` and `FailureDomains`. The `MachinePool` will also add an
optional `zones` field.

`FailureDomains` will contain a unique name including the following parameters:

- region
- zone
- topology
  - datacenter
  - cluster
  - networks
  - datastore
  - resourcePool
  - folder

The vSphere platform spec will be the default configuration for the cloud-config
adding the additional datacenters from the machinepool.

The master virtual machines will be provisioned with terraform per datacenter
and cluster. In the case of workers multiple machinesets will need to be
configured - one per datacenter/cluster pair.

### install-config

```yaml
apiVersion: v1
baseDomain: example.com
controlPlane:
  name: "master"
  replicas: 3
  platform:
    vsphere:
      zones:
       - "us-east-1"
       - "us-east-2"
       - "us-east-3"
compute:
- name: "worker"
  replicas: 4
  platform:
    vsphere:
      zones:
       - "us-east-1"
       - "us-east-2"
       - "us-east-3"
       - "us-west-1"
platform:
  vsphere:
    apiVIP: "192.168.0.1"
    ingressVIP: "192.168.0.2"
    vCenter: "vcenter"
    username: "username"
    password: "password"
    network: port-group
    datacenter: datacenter
    cluster: vcs-mdcnc-workload-1
    defaultDatastore: workload_share_vcsmdcncworkload_Yfyf6
    vcenters:
    - server: "vcenter"
      user: "vcenter-username"
      password: "vcenter-password"
      datacenters:
      - IBMCloud
      - datacenter-2
    failureDomains:
    - name: us-east-1
      region: us-east
      zone: us-east-1a
      topology:
        computeCluster: /${vsphere_datacenter}/host/vcs-mdcnc-workload-1
        networks:
        - network1 
        datastore: workload_share_vcsmdcncworkload_Yfyf6
    - name: us-east-2
      region: us-east
      zone: us-east-2a
      topology:
        computeCluster: /${vsphere_datacenter}/host/vcs-mdcnc-workload-2
        networks:
        - network1 
        datastore: workload_share_vcsmdcncworkload2_vyC6a
    - name: us-east-3
      region: us-east
      zone: us-east-3a
      topology:
        computeCluster: /${vsphere_datacenter}/host/vcs-mdcnc-workload-3
        networks:
        - network1 
        datastore: workload_share_vcsmdcncworkload3_joYiR
    - name: us-west-1
      region: us-west
      zone: us-west-1a
      topology:
        datacenter: datacenter-2
        computeCluster: /datacenter-2/host/vcs-mdcnc-workload-4
        networks:
        - network1 
        datastore: workload_share_vcsmdcncworkload3_joYiR
```

Each vcenter datacenter defined in either master or worker machinepool will
need to be added to the out-of-tree CCM configuration. This allows the CCM
to find the virtual machines across multiple vcenter datacenters.

### Open Questions

### Test Plan

- Configure only to run IBM, unable to run in VMC without multiple datacenters
or clusters. The existing CI vSphere infrastructure
can be extended to include an additional datacenter
and cluster.

- The additional vSphere-specific jobs should run with this configuration
(csi,etc)

### Graduation Criteria

#### Dev Preview -> Tech Preview

- CI: A new vSphere-specific job will need to be added for the installer
and periodics to support the new configuration
of regions, zones and multiple datacenters and clusters.

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback

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

- Multiple datacenter and multiple cluster vSphere environment for development
and CI.
