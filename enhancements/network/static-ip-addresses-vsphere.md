---
title: static-ip-addresses-vsphere
authors:
  - rvanderp3
reviewers: 
  - JoelSpeed - machine API
  - elmiko - machine API
  - patrickdillon - installer
  - jcpowermac - vSphere
  - cybertron - networking
  - zaneb - networking
approvers:
  - JoelSpeed
  - patrickdillon
  - cybertron
api-approvers: 
  - JoelSpeed
creation-date: 2022-10-21
last-updated: 2022-11-01
tracking-link: 
- https://issues.redhat.com/browse/OCPPLAN-9654
see-also:
  - /enhancements/network/baremetal-ipi-network-configuration.md
  - /enhancements/installer/vsphere-ipi-zonal.md
replaces:
superseded-by:
---

# Static IP Addresses for vSphere IPI

## Summary

Static IP addresses are emerging as a common requirement in environments where
the usage of DHCP violates corporate security guidelines.  Additionally, many 
users which require static IPs also require the use of the IPI installer. 
The proposal described in this enhacement discusses the implementation of
of assiging static IPs at both day 0 and day 2.

## Motivation

Users of OpenShift would like the ability to provision vSphere IPI clusters with static IPs.

- https://issues.redhat.com/browse/OCPPLAN-9654

### User Stories

As an OpenShift administrator, I want to provision nodes with static IP addresses so that I can comply with my organization's security requirements.

As an OpenShift administrator, I want to provision static IP addresses with the IPI installer so that I can reduce the complexity of certifying tools required to provision OpenShift.

As an OpenShift administrator, I want to scale nodes with static IPs so that I can meet capacity demands as well as respond to disaster recovery scenarios.

### Goals

- All nodes created during the installation are configured with static IPs
      Rationale: Many environments, due to security policies, do not allow DHCP.

- The IPI installation method is able to provide static IPs to the nodes
  Rationale: Some users must qualify each tool used in their environment. 
  Leveraging IPI greatly reduces the number of tools required to provision
  a cluster.
- 

### Non-Goals

- OpenShift will not be responsible for managing IP addresses.

## Proposal

### Static IPs Configured at Installation

To faciliate the configuration of static IP address, nmstate definitions are created for each node in the install-config.yaml. This follows a similar pattern set forth in the [Baremetal IPI Network Configuration](https://github.com/openshift/enhancements/blob/master/enhancements/network/baremetal-ipi-network-configuration.md#user-facing-api) enhancement. 

This enhancement expands on the [`hosts` slice](https://github.com/openshift/enhancements/blob/master/enhancements/network/baremetal-ipi-network-configuration.md#user-facing-api) by associating hosts with roles and, optionally, a failure domain.

For the bootstrap and control plane nodes, static IPs are passed to the node via the `guestinfo.afterburn.initrd.network-kargs` extraconfig parameter.  [Afterburn](https://github.com/coreos/afterburn/blob/main/src/providers/vmware/amd64.rs) recognizes this parameter when the node initially boots. 

When static IP configuration is required, Machines can not be created via MachineSets.
The installer must create the initial set of compute Machines manually and an administrator
must implement a `preCreate` hook for the MachineSet to allow the MachineSet to create
Machines during day-2 operations. See [IP Configuration of Machines](#Scaling-new-Nodes-with-`machinesets`) for more information.

As with the installer, the vSphere [machine reconciler](https://github.com/openshift/machine-api-operator/blob/master/pkg/controller/vsphere/reconciler.go#L745-L755) 
will pass the static IP configuration via the `guestinfo.afterburn.initrd.network-kargs` extraconfig parameter.  


### Day 2 Static IP Configuration

Nodes being added to a cluster may be configured via an `nmstate` IP configuration or default to DHCP.  The networking configuration of a node/machine is immutable after creation. The vSphere machine API machine controller will apply the nmstate configuration when the associated VM is cloned.

`machinesets` will be supported through the creation of a user-created custom controller.  This custom controller will leverage machine lifecycle hooks to
provide IP configuration to machines descending from `machinesets` with `machine` annotated with the appropriate lifecycle hook.

#### Changes Required

##### Installer

1. Modify the `install-config.yaml` vSphere platform specification to support nmstate configuration. The nmstate configuration is intended to be structurally compatible with the [nmstate API](https://nmstate.io/) but will omit fields that do not map logically to installation of a cluster.

~~~go
// Hosts defines `Host` configurations to be applied to nodes deployed by the installer
type Hosts struct {
  // Bootstrap IP configuration for the bootstrap node. If omitted, node will default to DHCP.
  // +optional
  Bootstrap *Host `json: "bootstrap,omitempty"`

  // ControlPlane slice of IP configurations for the control plane nodes. If empty, nodes will default to DHCP.
  // +optional
  ControlPlane []Host `json: "controlPlane,omitempty"`

  // Compute slice of IP configurations for the compute nodes. If empty, nodes will default to DHCP.
  // +optional
  Compute []Host `json: "compute,omitempty"`
}

// Host defines the IP configuration to be applied for a node deployed by the installer
type Host struct {  
  // FailureDomain refers to the name of a FailureDomain as described in https://github.com/openshift/enhancements/blob/master/enhancements/installer/vsphere-ipi-zonal.md
  // +optional
  FailureDomain string `json: "failureDomain"`

  // NetworkConfig IP configuration to be applied
  // +kubebuilder:validation:Required
  NetworkConfig machineapiv1beta1.NetworkConfig string `json: "networkConfig"` 
}

// NetworkConfig Config specifies an IP configuration to be applied upon cloning.
type NetworkConfig struct {
  // Interfaces slice of interfaces to be configured.	
  // +kubebuilder:validation:MaxItems=1
  // +kubebuilder:validation:MinItems=1
  // +kubebuilder:validation:Required
  Interfaces []Interface `json:"interfaces"`

  // DnsResolver defines nameservers to be applied to a network interface
  // interfaces.
  // +kubebuilder:validation:Required
  DnsResolver DnsResolver `json:"dnsResolver"`

  // Routes routes to be applied to a network interface
  // interfaces.
  // +kubebuilder:validation:Required
  Routes Routes `json:"routes"`
}

// Interface IP configuration to be applied to a network interface.
type Interface struct {
  // Name interface name
  // +kubebuilder:default=ens192
  // +optional
  Name  string        `json:"name"`

  // IPv4 IP configuration for network interface
  // +kubebuilder:validation:Required
  IPv4  IPv4Addresses `json:"ipv4"`
}

// IPV4Addresses IPV4 addresses to be applied for an `Interface`
type IPv4Addresses struct {
  // Address a slice of `IPV4Address`
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:MaxItems=1
  // +kubebuilder:validation:MinItems=1
  // +kubebuilder:validation:Required
  Address []IPv4Address `json:"address"`
}

// IPV4Address IPV4 address to be applied for an `Interface`
type IPv4Address struct {
  // IP address to be applied
  // +kubebuilder:validation:format=ip
  // +kubebuilder:validation:Required
  IP           string `json:"ip"`  
  // PrefixLength length of the IP address prefix
  // +kubebuilder:validation:Minimum=1
  // +kubebuilder:validation:Maximum=32
  // +kubebuilder:validation:Default=23
  // +kubebuilder:validation:Required
  PrefixLength uint  `json:"prefixLength"`
  

// DnsResolver DNS resolution configuration to be applied to a `Host`
type DnsResolver struct {
  // Config DNS resolver configuration to be applied to a `Host`
  // +kubebuilder:validation:Required
  Config DnsResolverConfig `json:"config"`
}

// Config defines DNS resolver configuration to be applied to a `Host`
type DnsResolverConfig struct {
  // Server a slice of DNS name servers  
  // +kubebuilder:validation:MaxItems=1
  // +kubebuilder:validation:MinItems=1
  // +kubebuilder:validation:Required
  Server []string `json:"server"`
}

// Routes routes to be applied to a `Host`
type Routes struct {
  // Config slice of routes to be applied to a `Host`
  // +kubebuilder:validation:MaxItems=1
  // +kubebuilder:validation:MinItems=1
  // +kubebuilder:validation:Required  
  Config []RouteConfig `json:"config"`
}

// RouteConfig routing configuration to be applied to a `Host`.
type RouteConfig struct {
  // NextHopAddress IP address of router
  // +kubebuilder:validation:format=ip
  // +kubebuilder:validation:Required
  NextHopAddress string `json:"nextHopAddress"`
}

~~~

Example of a platform spec configured to provide static IPs for the bootstrap, control plane, and compute nodes:
~~~yaml
platform:
  vsphere:
    hosts:
      bootstrap:
        networkConfig:
           interfaces:
           - ipv4:
               address:
                 - ip: 192.168.101.240
                   prefixLength: 23
           dnsResolver:
             config:
               server:
                 - 192.168.1.215
           routes:
             config:
               - nextHopAddress: 192.168.100.1
      controlPlane:
        - failureDomain: us-east-1
          networkConfig:
            interfaces:
              - ipv4:
                  address:
                    - ip: 192.168.101.241
                      prefixLength: 23
            dnsResolver:
              config:
                server:
                  - 192.168.1.215
            routes:
              config:
                - nextHopAddress: 192.168.100.1
        - failureDomain: us-east-2
          networkConfig:
            interfaces:
              - ipv4:
                  address:
                    - ip: 192.168.101.242
                      prefixLength: 23
            dnsResolver:
              config:
                server:
                  - 192.168.1.215
            routes:
              config:
                - nextHopAddress: 192.168.100.1
        - failureDomain: us-east-3
          networkConfig:
            interfaces:
              - ipv4:
                  address:
                    - ip: 192.168.101.243
                      prefixLength: 23
            dnsResolver:
              config:
                server:
                  - 192.168.1.215
            routes:
              config:
                - nextHopAddress: 192.168.100.1
      compute:
        - networkConfig:
            interfaces:
              - ipv4:
                  address:
                    - ip: 192.168.101.244
                      prefixLength: 23
            dnsResolver:
              config:
                server:
                  - 192.168.1.215
            routes:
              config:
                - nextHopAddress: 192.168.100.1
        - networkConfig:
            interfaces:
              - ipv4:
                  address:
                    - ip: 192.168.101.245
                      prefixLength: 23
            dnsResolver:
              config:
                server:
                  - 192.168.1.215
            routes:
              config:
                - nextHopAddress: 192.168.100.1
        - networkConfig:
            interfaces:
              - ipv4:
                  address:
                    - ip: 192.168.101.246
                      prefixLength: 23
            dnsResolver:
              config:
                server:
                  - 192.168.1.215
            routes:
              config:
                - nextHopAddress: 192.168.100.1
~~~
2. Add validation for the modified/added fields in the platform specification.
3. For compute nodes, produce machine manifests with associated IP configuration.  

Example of `machine` configured with IP configuration
~~~yaml
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  name: test-compute-1
spec:
  lifecycleHooks: {}
  metadata: {}
  providerSpec:
    value:
      numCoresPerSocket: 2
      diskGiB: 60
      snapshot: ''
      userDataSecret:
        name: worker-user-data
      memoryMiB: 8192
      credentialsSecret:
        name: vsphere-cloud-credentials
      network:
        devices:
          - networkName: lab
            config:
              interfaces:
                - ipv4:
                    address:
                      - ip: 192.168.101.245
                        prefixLength: 23
              dnsResolver:
                config:
                  server:
                    - 192.168.1.215
              routes:
                config:
                  - nextHopAddress: 192.168.100.1
      metadata:
        creationTimestamp: null
      numCPUs: 2      
      kind: VSphereMachineProviderSpec
      workspace:
        datacenter: testdc
        datastore: datastore-1
        folder: /testdc/vm/cluster-folder
        resourcePool: /testdc/host/cluster1/Resources
        server: test.vcenter.net
      template: vm-template-rhcos
      apiVersion: machine.openshift.io/v1beta1
~~~
4. For bootstrap and control plane nodes, modify vSphere terraform to convert nmstate to a VM guestinfo parameter
for each VM to be created.

As the assets are generated for the control plane and compute nodes, the slice of `host`s for each 
node role will be used to populate `nmstate` information.  The number of `host`s must match the number of
replicas defined in the associated machine pool.

Additionally, each defined host may optionally define a failure domain.  This indicates that the associated `networkConfig` will be applied to a machine created in the indicated failure domain.


##### Machine API
- Modify vSphere machine controller to convert nmstate to VM guestinfo parameter
- Introduce a new lifecycle hook called `preCreate`.
- Modify [types_vsphereprovider.go](https://github.com/openshift/api/blob/master/machine/v1beta1/types_vsphereprovider.go) to support nmstate configuration. 


###### IP Configuration of Machines
The machine API `VSphereMachineProviderSpec.Network` will be modified to include a new type called `NetworkConfig`.  

~~~go
// NetworkDeviceSpec defines the network configuration for a virtual machine's
// network device.
type NetworkDeviceSpec struct {
	// NetworkName is the name of the vSphere network to which the device
	// will be connected.
	NetworkName string `json:"networkName"`

	// Config specifies an IP configuration to be applied upon cloning.
	// +optional
	Config *NetworkConfig `json:"config,omitempty"`
}
~~~

See [installer](#installer) for a definition of this type as well as other types that make up the definition of `NetworkConfig`.  `NetworkConfig` and it's dependent types will be defined in [types_vsphereprovider.go](https://github.com/openshift/api/blob/master/machine/v1beta1/types_vsphereprovider.go).

An additional lifecycle hook will be added to the The `machine.machine.openshift.io` and `machinesets.machine.openshift.io` CRDs to enable a controller to augment a machine resource before it is rendered in the backing infrastructure.

~~~go
// PreCreate hooks prevent the machine from being created in the backing infrastructure.
// +listType=map
// +listMapKey=name
// +optional
PreCreate []LifecycleHook `json:"preCreate,omitempty"`
~~~

Creation of the resource will be blocked until the `preCreate` hook is removed from the `machine.machine.openshift.io` instance.

### Workflow Description

#### Installation
1. OpenShift administrator reserves IP addresses for installation.
2. OpenShift administrator constructs `install-config.yaml` to define an nmstate configuration for each node that will receive a static IP address.
3. OpenShift administrator initiates an installation with `openshift-install create cluster`.  
4. The installer will proceed to:
- provision bootstrap and control plane nodes with specified IP configuration
- create machine resources containing specified IP configuration
5. Once the machine API controllers become active, the compute machine resources will be rendered with the specified IP configuration.

#### Scaling new Nodes without `machinesets`
1. OpenShift administrator reserves IP addresses for new nodes to be scaled up.
2. OpenShift administrator constructs machine resource to define an nmstate configuration for each new node that will receive a static IP address.
3. OpenShift administrator initiates the creation of new machines by running `oc create -f machine.yaml`.  
4. The machine API will render the nodes with the specified IP configuration.

#### Scaling new Nodes with `machinesets`
1. OpenShift administrator configures a machineset with a `preCreate` lifecycle hook.

Example of a `machineset` configured to configure the `preCreate` lifecycle hook on machines it creates.
~~~yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  name: static-machineset-worker
  namespace: openshift-machine-api
  labels:
    machine.openshift.io/cluster-api-cluster: cluster
spec:
  replicas: 0
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-cluster: cluster
      machine.openshift.io/cluster-api-machineset: static-machineset-worker
  template:
    metadata:
      labels:
        machine.openshift.io/cluster-api-cluster: cluster
        machine.openshift.io/cluster-api-machine-role: worker
        machine.openshift.io/cluster-api-machine-type: worker
        machine.openshift.io/cluster-api-machineset: static-machineset-worker
    spec:
      lifecycleHooks:
        preCreate:
          - name: ipamController
            owner: network-admin
~~~

2. OpenShift administrator or machine autoscaler scales `n` machines
3. Controller watches machine resources created with the a `preCreate` lifecycle hook which matches
the expected name/owner.
4. Controller updates machine providerSpec with IP configuration
5. Controller sets `preTerminate` lifecycle hook
6. Controller removes `preCreate` lifecycle hook

On scale down, the controller will recognize a machine is being deleted and check for a `preTerminate`
lifecycle hook.  If the hook exists, the controller will retrieve the IP address of the node from
nmstate and release the IP.  It is recommended that if releasing a lease fails that the controller
retries some number of times before giving up.  However, upon giving up, the controller should remove 
the `preTerminate` regardless of if the IP address was successfully released to prevent blocking 
the machine's deletion.

In this workflow, the controller is responsible for managing, claiming, and releasing IP addresses.  

~~~mermaid
sequenceDiagram
    machineset controller->>+machine: creates machine with<br> preCreate hook
    machine controller-->machine controller: waits for preCreate hook<br>to be removed
    IP controller-->>+machine: confirm precense of<br>preCreate hook
    IP controller-->IP controller: allocates IP address
    IP controller->>+machine: sets IP configuration on machine
    IP controller->>+machine: removes preCreate hook and<br>sets preTerminate hook
    machine-->>machine controller: IP configuration read from <br>machine and converted to<br>guestinfo.afterburn.initrd.network-kargs
    machine controller->>vCenter: creates virtual machine
~~~


A sample project [machine-ipam-controller](https://github.com/rvanderp3/machine-ipam-controller) is an example of a controller that implements this workflow.


#### Variation [optional]

### API Extensions

The CRDs `machines.machine.openshift.io` and `machinesets.machine.openshift.io` will be modified to add a new lifecycle hook called `preCreate`.  When defined, the `preCreate` lifecycle hook will block the rendering of a machine in it's backing infrastructure.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

### Drawbacks

- Scaling nodes will become more complex. This will require the OpenShift administrator to integrate IP configuration
  management to enable scaling of machine API machine resources.

- If a `machineset` is configured to specify the `preCreate` lifecycle hook, a controller must remove the hook before
machine creation will continue.

- `install-config.yaml` will grow in complexity.

## Design Details

### Open Questions [optional]

#### `nmstate` API
How should we introduce `nmstate` to the OpenShift API?  While we only need a subset of `nmstate` for this enhancement, `nmstate` may have broader applicability outside of vSphere.

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives

## Infrastructure Needed [optional]
