---
title: static-ip-addresses-vsphere
authors:
  - rvanderp3
  - vr4manta
reviewers: 
  - JoelSpeed - machine API
  - elmiko - machine API
  - patrickdillon - installer
  - jcpowermac - vSphere
  - zaneb - networking
  - 2uasimojo - hive
approvers:
  - JoelSpeed
  - patrickdillon
  - 2uasimojo
api-approvers: 
  - JoelSpeed
creation-date: 2022-10-21
last-updated: 2024-01-02
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
The proposal described in this enhancement discusses the implementation of
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

To faciliate the configuration of static IP address, network device configuration definitions are created for each node in the install-config.yaml. A `hosts` slice will be introduced to the installer platform specification to allow network device configurations to be specified for a nodes. 
If a `hosts` slice is defined, enough `hosts` must be defined to cover the bootstrap, control plane, and compute nodes to be provisioned during the installation process.

The bootstrap and control plane nodes will have their static IP addresses provisioned by CAPV.  Compute nodes will rely on the machine API to provision their IP addresses.

Example of platform specification with IP addresses defined for bootstrap, control plane, and compute nodes:
~~~yaml
platform:
  vsphere:
    hosts:
    - role: bootstrap
      networkDevice:
        ipAddrs:
        - 192.168.101.240/24
        gateway: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: control-plane
      failureDomain: us-east-1a
      networkDevice:
        ipAddrs:
        - 192.168.101.241/24
        gateway: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: control-plane
      failureDomain: us-east-1b
      networkDevice:
        ipAddrs:
        - 192.168.101.242/24
        gateway: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: control-plane
      failureDomain: us-east-1c
      networkDevice:
        ipAddrs:
        - 192.168.101.243/24
        gateway: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: compute
      networkDevice:
        ipAddrs:
        - 192.168.101.244/24
        gateway: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: compute
      networkDevice:
        ipAddrs:
        - 192.168.101.245/24
        gateway: 192.168.101.1
        nameservers:
        - 192.168.101.2
    - role: compute
      networkDevice:
        ipAddrs:
        - 192.168.101.246/24
        gateway: 192.168.101.1
        nameservers:
        - 192.168.101.2
~~~

The install will be consuming the above config and will be creating custom resources following the CAPI static ip address process.  We will be generating an IPAddress and IPAddressClaim for each master and worker (bootstrap is not included but will have static IP directly applied to the VM based on the config).  These files will be a part of the openshift directory that is generated from `openshift-install create manifests`.  A sample of what is generated based on above config:

```shell
[ngirard@fedora openshift]$ ls
99_cloud-creds-secret.yaml
99_feature-gate.yaml
99_kubeadmin-password-secret.yaml
99_openshift-cluster-api_master-machines-0.yaml
99_openshift-cluster-api_master-machines-1.yaml
99_openshift-cluster-api_master-machines-2.yaml
99_openshift-cluster-api_master-user-data-secret.yaml
99_openshift-cluster-api_worker-machines-0.yaml
99_openshift-cluster-api_worker-machines-1.yaml
99_openshift-cluster-api_worker-machines-2.yaml
99_openshift-cluster-api_worker-machineset-0.yaml
99_openshift-cluster-api_worker-machineset-1.yaml
99_openshift-cluster-api_worker-machineset-2.yaml
99_openshift-cluster-api_worker-machineset-3.yaml
99_openshift-cluster-api_worker-user-data-secret.yaml
99_openshift-machine-api_address-ngirard-dev-lbfxg-master-0-claim-0-0.yaml
99_openshift-machine-api_address-ngirard-dev-lbfxg-master-1-claim-0-0.yaml
99_openshift-machine-api_address-ngirard-dev-lbfxg-master-2-claim-0-0.yaml
99_openshift-machine-api_address-ngirard-dev-lbfxg-worker-0-claim-0-0.yaml
99_openshift-machine-api_address-ngirard-dev-lbfxg-worker-1-claim-0-0.yaml
99_openshift-machine-api_address-ngirard-dev-lbfxg-worker-2-claim-0-0.yaml
99_openshift-machine-api_claim-ngirard-dev-lbfxg-master-0-claim-0-0.yaml
99_openshift-machine-api_claim-ngirard-dev-lbfxg-master-1-claim-0-0.yaml
99_openshift-machine-api_claim-ngirard-dev-lbfxg-master-2-claim-0-0.yaml
99_openshift-machine-api_claim-ngirard-dev-lbfxg-worker-0-claim-0-0.yaml
99_openshift-machine-api_claim-ngirard-dev-lbfxg-worker-1-claim-0-0.yaml
99_openshift-machine-api_claim-ngirard-dev-lbfxg-worker-2-claim-0-0.yaml
99_openshift-machine-api_master-control-plane-machine-set.yaml
99_openshift-machineconfig_99-master-ssh.yaml
99_openshift-machineconfig_99-worker-ssh.yaml
99_role-cloud-creds-secret-reader.yaml
openshift-install-manifests.yaml
```

Using master-0 as the target example, we can see how the machine, IPAddressClaim and IPAddress are configured as follows:

Machine:
```yaml
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  creationTimestamp: null
  labels:
    machine.openshift.io/cluster-api-cluster: ngirard-dev-fhshn
    machine.openshift.io/cluster-api-machine-role: master
    machine.openshift.io/cluster-api-machine-type: master
  name: ngirard-dev-fhshn-master-0
  namespace: openshift-machine-api
spec:
  lifecycleHooks: {}
  metadata: {}
  providerSpec:
    value:
      apiVersion: machine.openshift.io/v1beta1
      credentialsSecret:
        name: vsphere-cloud-credentials
      diskGiB: 100
      kind: VSphereMachineProviderSpec
      memoryMiB: 16384
      metadata:
        creationTimestamp: null
      network:
        devices:
          - addressesFromPools:
              - group: installer.openshift.io
                name: default-0
                resource: IPPool
            nameservers:
              - 8.8.8.8
            networkName: ocp-ci-seg-13
      numCPUs: 4
      numCoresPerSocket: 2
      snapshot: ""
      template: ngirard-dev-fhshn-rhcos-us-east-us-east-1a
      userDataSecret:
        name: master-user-data
      workspace:
        datacenter: IBMCloud
        datastore: /IBMCloud/datastore/mdcnc-ds-1
        folder: /IBMCloud/vm/ngirard-dev-fhshn
        resourcePool: /IBMCloud/host/vcs-mdcnc-workload-1//Resources
        server: ibmvcenter.vmc-ci.devcluster.openshift.com
status: {}
```

IPAddressClaim:
```yaml
apiVersion: ipam.cluster.x-k8s.io/v1beta1
kind: IPAddressClaim
metadata:
  creationTimestamp: null
  finalizers:
    - machine.openshift.io/ip-claim-protection
  name: ngirard-dev-fhshn-master-0-claim-0-0
  namespace: openshift-machine-api
spec:
  poolRef:
    apiGroup: installer.openshift.io
    kind: IPPool
    name: default-0
status:
  addressRef:
    name: ngirard-dev-fhshn-master-0-claim-0-0
```

IPAddress:
```yaml
apiVersion: ipam.cluster.x-k8s.io/v1beta1
kind: IPAddress
metadata:
  creationTimestamp: null
  name: ngirard-dev-fhshn-master-0-claim-0-0
  namespace: openshift-machine-api
spec:
  address: 192.168.13.241
  claimRef:
    name: ngirard-dev-fhshn-master-0-claim-0-0
  gateway: 192.168.13.1
  poolRef:
    apiGroup: installer.openshift.io
    kind: IPPool
    name: default-0
  prefix: 23
```

If the cluster admin wishes to change anything about the ipaddress or ipaddress claim information, they may do so at this time.  Once all changes are made, it is now find to continue the rest of the cluster create process.

### Scaling in Additional Machines

Additional machines requiring static IP addresses may be scaled in by:
- An OpenShift administrator prepares a machine yaml with the static IP address included in the provider specification
- An external controller assigns an IP address in response to an `IPAddressClaim` being created by the machine controller

#### Machine yaml with embedded static IP address

An administrator may create a bespoke machine configuration yaml which embeds static IP configuration. This allows a new machine to be provisioned without the need of an external controller.

Example:
~~~yaml
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  name: test-compute-1
spec:
  metadata: {}
  providerSpec:
    value:
      ...
      network:
        devices:
          - networkName: lab
            ipAddrs:
            - 192.168.101.244/24
            gateway4: 192.168.101.1
            nameservers:
            - 192.168.101.2            
      ...
~~~

#### External controller assigns address

An external controller may be used to automatically assign an IP address to a node during the provisioning process. This is accomplished by the definition of an IP address pool in the machineset/machine provider spec.

Example:
~~~yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  name: static-machineset-worker
  namespace: openshift-machine-api
spec:
  replicas: 2
  ...
  template:
    ...
    spec:          
      ...
      network:
        devices:
        - networkName: port-group-vlan-101          
          addressesFromPools:                   
          - group: machine.openshift.io
            resource: IPPool
            name: example-pool
~~~

The IP address pool specifies the CRD which defines the address pool configuration. This CRD will vary based on the external controller used to provision IP addresses.  The machine controller will create an `IPAddressClaim` which the external controller will fulfill with an `IPAddress`.

#### Control Plane Machine Sets

Control Plane Machinesets (CPMS) works similar to the way compute machinesets work.  When the installer generates the CPMS for the cluster, it will inject the AddessFromPools information to be used for future node scaling.  One difference is that the networkName will not be populated.  This is due to how each FailureDomain can define a different networkName.  This information will be populated when the machine object gets created dynamically after evaluating which failure domain the machine will be placed into.

Example:
~~~
apiVersion: machine.openshift.io/v1
kind: ControlPlaneMachineSet
metadata:
  creationTimestamp: null
  labels:
    machine.openshift.io/cluster-api-cluster: ngirard-dev-bwnz9
  name: cluster
  namespace: openshift-machine-api
spec:
  replicas: 3
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-cluster: ngirard-dev-bwnz9
      machine.openshift.io/cluster-api-machine-role: master
      machine.openshift.io/cluster-api-machine-type: master
  state: Active
  strategy: {}
  template:
    machineType: machines_v1beta1_machine_openshift_io
    machines_v1beta1_machine_openshift_io:
      failureDomains:
        platform: VSphere
        vsphere:
        - name: fd-2
        - name: fd-3
        - name: fd-4
      metadata:
        labels:
          machine.openshift.io/cluster-api-cluster: ngirard-dev-bwnz9
          machine.openshift.io/cluster-api-machine-role: master
          machine.openshift.io/cluster-api-machine-type: master
      spec:
        lifecycleHooks: {}
        metadata: {}
        providerSpec:
          value:
            apiVersion: machine.openshift.io/v1beta1
            credentialsSecret:
              name: vsphere-cloud-credentials
            diskGiB: 100
            kind: VSphereMachineProviderSpec
            memoryMiB: 16384
            metadata:
              creationTimestamp: null
            network:
              devices:
              - addressesFromPools:
                - group: installer.openshift.io
                  name: default-0
                  resource: IPPool
                nameservers:
                - 8.8.8.8
            numCPUs: 4
            numCoresPerSocket: 2
            snapshot: ""
            template: ""
            userDataSecret:
              name: master-user-data
            workspace: {}
status: {}
~~~

#### Changes Required

##### Installer

1. Modify the `install-config.yaml` vSphere platform specification to support the definition of the 
~~~go
// Hosts defines `Host` configurations to be applied to nodes deployed by the installer
type Hosts []Host

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
  // +kubebuilder:example=192.168.1.100/24
  // +kubebuilder:example=2001:DB8:0000:0000:244:17FF:FEB6:D37D/64
  // +kubebuilder:validation:Required
  IPAddrs []string `json:"ipAddrs"`
  
  // nameservers is a list of IPv4 and/or IPv6 addresses used as DNS nameservers, for example,
  // 8.8.8.8. a nameserver is not provided by a fulfilled IPAddressClaim. If DHCP is not the
  // source of IP addresses for this network device, nameservers should include a valid nameserver.
  // +kubebuilder:validation:Format=ipv4
  // +kubebuilder:validation:Format=ipv6
  // +kubebuilder:example=8.8.8.8
  Nameservers []string `json:"nameservers,omitempty"`
}

~~~

2. Add validation for the modified/added fields in the platform specification.
3. For compute nodes, produce machine manifests with associated network device configuration.  
4. For bootstrap and control plane nodes, provide network device configuration to a VM guestinfo parameter in the capv machine spec for each VM to be created.

As the assets are generated for the control plane and compute nodes, the slice of `host`s for each node role will be used to populate network device configuration.  The number of `host`s must match the number of replicas defined in the associated machine pool.

Additionally, each defined host may optionally define a failure domain.  This indicates that the associated `networkDevice` will be applied to a machine created in the indicated failure domain.


##### Machine API
- Modify vSphere machine controller to convert IP configuration to VM guestinfo parameter
- Introduce new types to facilitate IP allocation by a controller.
- Modify [types_vsphereprovider.go](https://github.com/openshift/api/blob/master/machine/v1beta1/types_vsphereprovider.go) to support network device configuration. 


###### network device configuration of Machines

IP configuration for a given network device may be derived from three configuration mechanisms:
1. DHCP
2. An external IPAM IP Pool
3. Static IP configuration defined in the provider spec

The machine API `VSphereMachineProviderSpec.Network` will be extended to include a subset of additional properties as defined in https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/main/apis/v1beta1/types.go.  See [openshift/api#1338](https://github.com/openshift/api/pull/1338) for further details on the API extension to the provider specification.  


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
2. OpenShift administrator constructs machine resource to define a network device configuration for each new node that will receive a static IP address.
3. OpenShift administrator initiates the creation of new machines by running `oc create -f machine.yaml`.  
4. The machine API will render the nodes with the specified network device configuration.

#### Scaling new Nodes with `machinesets`
1. OpenShift administrator configures a machineset with `addressesFromPools` defined in the platform specification.
2. OpenShift administrator or machine autoscaler scales machines
3. If `addressesFromPools` contains a `AddressesFromPool` definition the machine controller will create an `IPAddressClaim`. The machine controller will:
- Set a finalizer on the `IPAddressClaim` called `machines.openshift.io/ip-claim-protection`
- Set an owner reference on the `IPAddressClaim` to the associated `Machine`.
- Set the condition `IPAddressClaimed` of the associated `Machine` to indicate that it it awaiting IPAddressClaims to be bound
- Block the creation of the underlying machine in the infrastructure until all associated `IPAddressClaim`s are bound
4. An external controller will watch for `IPAddressClaim` instances that reference a address pool type known to the external controller.
5. The external controller will create an `IPAddress` and bind it to its associated `IPAddressClaim`.  
6. The machine controller will update the condition `IPAddressClaimed` of the associated `Machine` to indicate its `IPAddressClaim`(s) is bound.  If a `Machine` 
has multiple associated `IPAddressClaims`, a single `IPAddressClaimed` condition will report the number of outstanding claims.
7. The machine controller will then create the virtual machine with the network configuration in the network device spec and the `IPAddress`.

~~~mermaid
sequenceDiagram
    machineset controller->>+machine: creates machine with<br> IPPool
    machine controller-->machine controller: create IPAddressClaim<br>and wait for claim<br>to be bound
    machine controller-->machine controller: IPAddressClaim ownerReference<br>refers to the machine
    IP controller-->IP controller: processes claim and<br>allocates IP address    
    IP controller-->IP controller: create IPAddress and bind IPAddressClaim
    machine controller-->machine controller: build guestinfo.afterburn.initrd.network-kargs<br>and clone VM
~~~

On scale down:
1. The machine controller will remove the finalizer on `IPAddressClaim` associated with a given `Machine` after the underlying virtual machine has been deleted.
2. The kubernetes API will garbage collect the `IPAddressClaim` and `IPAddress` formerly associated with the `Machine`.
3. Once the `IPAddress` associated with the machine is deleted, the external controller can reuse the address.

In this workflow, the controller is responsible for managing, claiming, and releasing IP addresses.  

A sample project [machine-ipam-controller](https://github.com/rvanderp3/machine-ipam-controller) is an example of a controller that implements this workflow.


#### Variation [optional]

### API Extensions

2 new CRDs will be included in the Machine API Operator.

- `ipaddressclaims.ipam.cluster.x-k8s.io` - IP address claim request which is created by the machine reconciler and fulfilled by an external controller
- `ipaddresses.ipam.cluster.x-k8s.io` - IP address fulfilled by an external controller

These two CRDs are part of CAPI and will be imported into this operator from the Cluster CAPI Operator.

The CRDs `machines.machine.openshift.io` and `machinesets.machine.openshift.io` will be modified to allow the definition of `addressesFromPool` in the provider specification.

See https://github.com/openshift/api/pull/1338 for details and discussion related to the API.

### Implementation Details/Notes/Constraints 

#### Eventual Migration to CAPI/CAPV

The inclusion of the CRDs above is intended to follow a similar pattern followed by [CAPV](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/pull/1210/files).  Migration would not require any migration due to the `ipaddressclaim.ipam.cluster.x-k8s.io` and `ipaddress.ipam.cluster.x-k8s.io` CRDs already being a part of CAPI/CAPV.

### Risks and Mitigations

### Drawbacks

- Scaling nodes will become more complex. This will require the OpenShift administrator to integrate network device configuration
  management to enable scaling of machine API machine resources.

- If a `machineset` is configured to specify an IPPool resource.  An external controller is responsible for fulfilling the resultant `IPAddressClaim` that is created during machine rendering.

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
end-to-end tests.**

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives

Lifecycle hook 

## Infrastructure Needed [optional]
