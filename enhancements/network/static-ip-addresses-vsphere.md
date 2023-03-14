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

- All nodes created during the installation are configured with static IPs.  
  Rationale: Many environments, due to security policies, do not allow DHCP.

- The IPI installation method is able to provide static IPs to the nodes
  Rationale: Some users must qualify each tool used in their environment. 
  Leveraging IPI greatly reduces the number of tools required to provision
  a cluster.

### Non-Goals

- OpenShift will not be responsible for managing IP addresses.

## Proposal

### Static IPs Configured at Installation

To faciliate the configuration of static IP address, network device configuration definitions are created for each node in the install-config.yaml. A `hosts`
slice will be introduced to the installer platform specification to allow network device configurations to be specified for a nodes.

For the bootstrap and control plane nodes, static IP configuration is passed to the node on the [kernel command line](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/networking_guide/sec-configuring_ip_networking_from_the_kernel_command_line) via the `guestinfo.afterburn.initrd.network-kargs` extraconfig parameter.  [Afterburn](https://github.com/coreos/afterburn/blob/main/src/providers/vmware/amd64.rs) recognizes this parameter when the node initially boots. 

When static network device configuration is required, Machines can not be created via MachineSets.
The installer must create the initial set of compute Machines manually and an administrator
must implement a `preProvision` hook for the MachineSet to allow the MachineSet to create
Machines during day-2 operations. See [network device configuration of Machines](#Scaling-new-Nodes-with-`machinesets`) for more information.

As with the installer, the vSphere [machine reconciler](https://github.com/openshift/machine-api-operator/blob/master/pkg/controller/vsphere/reconciler.go#L745-L755) 
will pass the static network device configuration via the `guestinfo.afterburn.initrd.network-kargs` extraconfig parameter.  


### Day 2 Static network device configuration

Nodes being added to a cluster may be configured with an network device configuration or default to DHCP.  The networking configuration of a node/machine is immutable after creation. The vSphere machine API machine controller will apply the network device configuration when the associated VM is cloned.

`machinesets` will be supported through the creation of a user-created custom controller([sample controller](https://github.com/rvanderp3/machine-ipam-controller)).  This custom controller will leverage machine lifecycle hooks to
provide network device configuration to machines descending from `machinesets` with `machine` annotated with the appropriate lifecycle hook.

#### Changes Required

##### Installer

1. Modify the `install-config.yaml` vSphere platform specification to support the definition of the 
~~~go
// Hosts defines `Host` configurations to be applied to nodes deployed by the installer
type Hosts []Host

// Host defines the network device configuration to be applied for a node deployed by the installer
type Host struct {  
  // FailureDomain refers to the name of a FailureDomain as described in https://github.com/openshift/enhancements/blob/master/enhancements/installer/vsphere-ipi-zonal.md
  // +optional
  FailureDomain string `json: "failureDomain"`

  // Slice of NetworkDeviceSpecs to be applied
  // +kubebuilder:validation:Required
  NetworkDevice []NetworkDeviceSpec `json: "networkDevice"` 

  // Role defines the role of the node
  // +kubebuilder:validation:Enum="";bootstrap;control-plane;compute
  // +kubebuilder:validation:Required
  Role string `json: "role"`
}

type NetworkDeviceSpec struct {
	// gateway4 is the IPv4 gateway used by this device.
	// Required when DHCP4 is false.
	// +optional
	// +kubebuilder:validation:Format=ipv4
	Gateway4 string `json:"gateway4,omitempty"`

	// gateway4 is the IPv4 gateway used by this device.
	// Required when DHCP6 is false.
	// +kubebuilder:validation:Format=ipv6
	// +optional
	Gateway6 string `json:"gateway6,omitempty"`

	// ipaddrs is a list of one or more IPv4 and/or IPv6 addresses to assign
	// to this device.
	// Required when DHCP4 and DHCP6 are both Disabled.
	// + Validation is applied via a patch, we validate the format as either ipv4 or ipv6
	// +optional
	IPAddrs []string `json:"ipAddrs,omitempty"`

	// nameservers is a list of IPv4 and/or IPv6 addresses used as DNS
	// nameservers.
	// Please note that Linux allows only three nameservers (https://linux.die.net/man/5/resolv.conf).
	// +optional
	Nameservers []string `json:"nameservers,omitempty"`
}

~~~

Example of a platform spec configured to provide static IPs for the bootstrap, control plane, and compute nodes:
~~~yaml
platform:
  vsphere:
    hosts:
    - role: bootstrap
      networkDevice:
        ipaddrs:
        - 192.168.101.240/24
        gateway4: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: control-plane
      failureDomain: us-east-1a
      networkDevice:
        ipaddrs:
        - 192.168.101.241/24
        gateway4: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: control-plane
      failureDomain: us-east-1b
      networkDevice:
        ipaddrs:
        - 192.168.101.242/24
        gateway4: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: control-plane
      failureDomain: us-east-1c
      networkDevice:
        ipaddrs:
        - 192.168.101.243/24
        gateway4: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: compute
      networkDevice:
        ipaddrs:
        - 192.168.101.244/24
        gateway4: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: compute
      networkDevice:
        ipaddrs:
        - 192.168.101.245/24
        gateway4: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: compute
      networkDevice:
        ipaddrs:
        - 192.168.101.246/24
        gateway4: 192.168.101.1
        nameservers:
        - 192.168.101.2
~~~
2. Add validation for the modified/added fields in the platform specification.
3. For compute nodes, produce machine manifests with associated network device configuration.  

Example of `machine` configured with network device configuration
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
            ipaddrs:
            - 192.168.101.244/24
            gateway4: 192.168.101.1
            nameservers:
            - 192.168.101.2            
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
4. For bootstrap and control plane nodes, modify vSphere terraform to convert network device configuration to a VM guestinfo parameter
for each VM to be created.

As the assets are generated for the control plane and compute nodes, the slice of `host`s for each 
node role will be used to populate network device configuration.  The number of `host`s must match the number of
replicas defined in the associated machine pool.

Additionally, each defined host may optionally define a failure domain.  This indicates that the associated `networkDevice` will be applied to a machine created in the indicated failure domain.


##### Machine API
- Modify vSphere machine controller to convert IP configuration to VM guestinfo parameter
- Introduce a new lifecycle hook called `preProvision`.
- Modify [types_vsphereprovider.go](https://github.com/openshift/api/blob/master/machine/v1beta1/types_vsphereprovider.go) to support network device configuration. 


###### network device configuration of Machines
The machine API `VSphereMachineProviderSpec.Network` will be extended to include a subset of additional properties as defined in https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/main/apis/v1beta1/types.go.  See [openshift/api#1338](https://github.com/openshift/api/pull/1338) for further details on the API extension to the provider specification.  

An additional lifecycle hook will be added to the The `machine.machine.openshift.io` and `machinesets.machine.openshift.io` CRDs to enable a controller to augment a machine resource before it is rendered in the backing infrastructure.

~~~go
// preProvision hooks prevent the machine from being created in the backing infrastructure.
// +listType=map
// +listMapKey=name
// +optional
preProvision []LifecycleHook `json:"preProvision,omitempty"`
~~~

Creation of the resource will be blocked until the `preProvision` hook is removed from the `machine.machine.openshift.io` instance.

### Workflow Description

#### Installation
1. OpenShift administrator reserves IP addresses for installation.
2. OpenShift administrator constructs `install-config.yaml` to define an network device configuration for each node that will receive a static IP address.
3. OpenShift administrator initiates an installation with `openshift-install create cluster`.  
4. The installer will proceed to:
- provision bootstrap and control plane nodes with the specified network device configuration
- create machine resources containing specified network device configuration
5. Once the machine API controllers become active, the compute machine resources will be rendered with the specified network device configuration.

#### Scaling new Nodes without `machinesets`
1. OpenShift administrator reserves IP addresses for new nodes to be scaled up.
2. OpenShift administrator constructs machine resource to define an network device configuration for each new node that will receive a static IP address.
3. OpenShift administrator initiates the creation of new machines by running `oc create -f machine.yaml`.  
4. The machine API will render the nodes with the specified network device configuration.

#### Scaling new Nodes with `machinesets`
1. OpenShift administrator configures a machineset with a `preProvision` lifecycle hook.

Example of a `machineset` configured to configure the `preProvision` lifecycle hook on machines it creates.
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
        preProvision:
          - name: ipamController
            owner: network-admin
~~~

2. OpenShift administrator or machine autoscaler scales `n` machines
3. Controller watches machine resources created with the a `preProvision` lifecycle hook which matches
the expected name/owner.
4. Controller updates machine providerSpec with network device configuration
5. Controller sets `preTerminate` lifecycle hook
6. Controller removes `preProvision` lifecycle hook

On scale down, the controller will recognize a machine is being deleted and check for a `preTerminate`
lifecycle hook.  If the hook exists, the controller will retrieve the IP address of the machine and 
release the IP.  It is recommended that if releasing a lease fails that the controller
retries some number of times before giving up.  However, upon giving up, the controller should remove 
the `preTerminate` regardless of if the IP address was successfully released to prevent blocking 
the machine's deletion.

In this workflow, the controller is responsible for managing, claiming, and releasing IP addresses.  

~~~mermaid
sequenceDiagram
    machineset controller->>+machine: creates machine with<br> preProvision hook
    machine controller-->machine controller: waits for preProvision hook<br>to be removed
    IP controller-->>+machine: confirm precense of<br>preProvision hook
    IP controller-->IP controller: allocates IP address
    IP controller->>+machine: sets network device configuration on machine
    IP controller->>+machine: removes preProvision hook and<br>sets preTerminate hook
    machine-->>machine controller: network device configuration read from <br>machine and converted to<br>guestinfo.afterburn.initrd.network-kargs
    machine controller->>vCenter: creates virtual machine
~~~


A sample project [machine-ipam-controller](https://github.com/rvanderp3/machine-ipam-controller) is an example of a controller that implements this workflow.


#### Variation [optional]

### API Extensions

The CRDs `machines.machine.openshift.io` and `machinesets.machine.openshift.io` will be modified to add a new lifecycle hook called `preProvision`.  When defined, the `preProvision` lifecycle hook will block the rendering of a machine in it's backing infrastructure.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

### Drawbacks

- Scaling nodes will become more complex. This will require the OpenShift administrator to integrate network device configuration
  management to enable scaling of machine API machine resources.

- If a `machineset` is configured to specify the `preProvision` lifecycle hook, a controller must remove the hook before
machine creation will continue.

- `install-config.yaml` will grow in complexity.

## Design Details

### Open Questions

#### `nmstate` API
Q: How should we introduce `nmstate` to the OpenShift API?  While we only need a subset of `nmstate` for this enhancement, `nmstate` may have broader applicability outside of vSphere.

A: In the November 10, 2022 cluster lifecycle arch call, it was decided to move to an [API consistent with CAPV](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/main/apis/v1beta1/types.go).

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
