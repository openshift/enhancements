---
title: vsphere-multi-disk
authors:
- "@vr4manta"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
- "@JoelSpeed"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
- "@JoelSpeed"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
- "@JoelSpeed"
creation-date: 2024-11-05
last-updated: 2024-11-05
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
- https://issues.redhat.com/browse/SPLAT-1880
see-also:
replaces:
superseded-by:
---

# vSphere Multi Disk

## Summary

This feature enhancement aims to allow admins the ability to configure additional disks that are not present in the vSphere VM templates (OVA) by enhancing the vsphere machine API and adding the capability to the cloning process.

## Motivation

As the use of Kubernetes clusters grows, admins are needing more and more improvements to the VMs themselves to make sure they run as smoothly as possible.  The number of cores and memory continue to increase for each machine and this is causing the amount of workloads to increase on each virtual machine.  This growth is now causing the base VM image to not provide enough storage for OS needs.  In some cases, users just increase the size of the primary disk using the existing configuration options for machines; however, this does not allows for all desired configuration choices.  Admins are now wanting the ability to add additional disks to these VMs for things such as etcd storage, image storage, container runtime and even swap.

### User Stories

* As an OpenShift administrator, I want to be able to add additional disks to any of the vsphere VMs so that my nodes can have additional disks for me to use to assign special case storage such as etcd data, swap, container images, etc.

### Goals

- Enhance vSphere machine API to allow creating and attaching data disks to the VMs during cloning process.
- Enhance OCP installer to supported adding data disks to control plane and compute machines.
- Update OCP Cluster CAPI Operator / Cluster API vSphere to include new upstream API changes to support creating data disks.
- Keep upstream vSphere cluster API (CAPV) in sync with OCP Machine API (MAPI).

### Non-Goals

- Introduce API breaking changes.
- Add ability to advance configure additions disks, such as, define controller type (IDE, scsi, etc) or unit number in the controller.
- Any disk management features such as encryption.

## Proposal

Today the machine API does not allow for vSphere machines to be able to be configured with additional data disks that are not present in the target vSphere OVA template.  This enhancement proposes adding a new field that will allow administrators to configure machines to have additional disks added to the VM that can be configured by OS services to be used for any purpose.

### Workflow Description

**_Installation with data disks_**

1. User create install-config.yaml with machine pools containing data disk configuration
2. User runs the `openshift-install` program to start the creation of new cluster
3. Installer generates configs for CAPI to create the control plane machines
4. Installer generates configs for cluster CPMS, control plane machine, and compute machines sets with the machine pool information applied (including new data disk configs)
5. CAPI / CAPV creates the control plane VMs in vSphere
6. MAPI creates any compute nodes that were configured to be created at install time
7. Cluster creation completes with all desired VMs / nodes operational as well as all Cluster Operators reporting Available with no errors.

**_Machine Set Creation_**

1. User creates new machine set configuration with the vsphere machine provider spec containing data disks
2. User runs `oc create -f <filename>` to create the new machine set
3. User scales up machine set
4. MAPI creates new VM in vSphere and requests new disks be dynamically added to the VM during the cloning process
5. Machine state progresses to `Provisioned`
6. The VM is started after the cloning process completes
7. The new VM provisions successfully and the node is created in OpenShift
8. The Machine state progresses to `Running`
9. The MachineSet shows desired, current, ready, and available all with the correct counts

**_Machine Creation (No MachineSet)_**

1. User creates new machine configuration with the vsphere machine provider spec containing data disks
2. User runs `oc create -f <filename>` to create the new machine
3. MAPI creates new VM in vSphere and requests new disks be dynamically added to the VM during the cloning process
4. The Machine state transitions to `Provisioned`
5. The VM is started
6. The new VM starts successfully and the node is created in OpenShift
7. The Machine state transitions to `Running`

### API Extensions

This enhancement will be enhancing the installer's CRD / type used for the install-config.yaml and will also be enhancing the vsphere machine provider spec type and all dependent CRDs.

#### Installer

The installer's install-config will be enhanced to allow the vSphere machine pools to define data disks.

```go
apiVersion: v1
baseDomain: openshift.example.net
featureSet: TechPreviewNoUpgrade
compute:
- architecture: amd64
  hyperthreading: Enabled
  name: worker
  platform: 
    vsphere:
      zones:      
      - fd-1
      cpus: 4
      coresPerSocket: 2
      memoryMB: 8192
      osDisk:
        diskSizeGB: 60
      dataDisks:
      - diskSizeGiB: 10
        name: "container_images"
      - diskSizeGiB: 20
        name: "log files"
      - diskSizeGiB: 30
        name: "swap"
  replicas: 0
controlPlane:
  architecture: amd64
  hyperthreading: Enabled
  name: master
  platform:
    vsphere: 
      zones:
      - fd-1
      cpus: 8 
      coresPerSocket: 2
      memoryMB: 16384
      osDisk:
        diskSizeGB: 100
      dataDisks:
      - diskSizeGiB: 10
        name: "etcd"
      - diskSizeGiB: 20
        name: "container-images"
  replicas: 3
...
```

In the example above, we have added a new field `dataDisks` that is used to define each of the disks to create for the virtual machine.  The `name` field is used to identify the disk.  This is used primarily for debug and for identifying the disk definition.  The `diskSizeGiB` field specifies how large the disk needs to be.

#### API

The configurations in the installer are also applied to the MAPI machine, machine set, and CPMS definitions.  All three of these are using the updated `VSphereMachineProviderSpec` type.  This type has also been updated to include the new fields similar to the installer.

```go
type VSphereMachineProviderSpec struct {
...
    // disks is a list of non OS disks to be created and attached to the VM.  The max number of disk allowed to be attached is
    // currently 15.  This limitation is being applied to allow no more than 16 disks on the default scsi controller for the VM.
    // The first disk on that SCSI controller will be the OS disk from the template.
    // +openshift:enable:FeatureGate=VSphereMultiDisk
    // +optional
    DataDisks []VSphereDisk `json:"dataDisks,omitempty"`
}

// VSphereDisk describes additional disks for vSphere.
type VSphereDisk struct {
    // name is a name to be used to identify the disk definition. If name is not specified,
    // the disk will still be created.  The name should be unique so that it can be used to clearly
    // identify purpose of the disk, but is not required to be unique.
    // +optional
    Name string `json:"name,omitempty"`
    // sizeGiB is the size of the disk (in GiB).
    // +kubebuilder:validation:Required
    SizeGiB int32 `json:"sizeGiB"`
}
```

Examples of the various CRs using the above type enhancement:

CPMS
```yaml
apiVersion: machine.openshift.io/v1
kind: ControlPlaneMachineSet
metadata:
  creationTimestamp: "2024-11-06T15:30:46Z"
  finalizers:
  - controlplanemachineset.machine.openshift.io
  generation: 1
  labels:
    machine.openshift.io/cluster-api-cluster: ngirard-multi-rtmvw
  name: cluster
  namespace: openshift-machine-api
  resourceVersion: "17330"
  uid: fe7f5e45-40da-4c69-b1b9-5cc415970af4
spec:
  replicas: 3
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-cluster: ngirard-multi-rtmvw
      machine.openshift.io/cluster-api-machine-role: master
      machine.openshift.io/cluster-api-machine-type: master
  state: Active
  strategy:
    type: RollingUpdate
  template:
    machineType: machines_v1beta1_machine_openshift_io
    machines_v1beta1_machine_openshift_io:
      failureDomains:
        platform: VSphere
        vsphere:
        - name: fd-1
      metadata:
        labels:
          machine.openshift.io/cluster-api-cluster: ngirard-multi-rtmvw
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
            dataDisks:
            - diskSizeGiB: 10
              name: "etcd"
            - diskSizeGiB: 20
              name: "container-images"
            diskGiB: 100
            kind: VSphereMachineProviderSpec
            memoryMiB: 16384
            metadata:
              creationTimestamp: null
            network:
              devices: null
            numCPUs: 8
            numCoresPerSocket: 2
            snapshot: ""
            template: ""
            userDataSecret:
              name: master-user-data
            workspace: {}
```

Machine:
```yaml
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  annotations:
    machine.openshift.io/instance-state: poweredOn
  creationTimestamp: "2024-11-06T15:30:43Z"
  finalizers:
  - machine.machine.openshift.io
  generation: 3
  labels:
    machine.openshift.io/cluster-api-cluster: ngirard-multi-rtmvw
    machine.openshift.io/cluster-api-machine-role: master
    machine.openshift.io/cluster-api-machine-type: master
    machine.openshift.io/region: ""
    machine.openshift.io/zone: ""
  name: ngirard-multi-rtmvw-master-0
  namespace: openshift-machine-api
  ownerReferences:
  - apiVersion: machine.openshift.io/v1
    blockOwnerDeletion: true
    controller: true
    kind: ControlPlaneMachineSet
    name: cluster
    uid: fe7f5e45-40da-4c69-b1b9-5cc415970af4
  resourceVersion: "37828"
  uid: bc0f156d-e768-41dc-a4cd-277bc4543c6d
spec:
  lifecycleHooks:
    preDrain:
    - name: EtcdQuorumOperator
      owner: clusteroperator/etcd
  metadata: {}
  providerID: vsphere://42107680-8b89-768c-666e-e4dfd4d9228c
  providerSpec:
    value:
      apiVersion: machine.openshift.io/v1beta1
      credentialsSecret:
        name: vsphere-cloud-credentials
      dataDisks:
      - diskSizeGiB: 10
        name: "etcd"
      - diskSizeGiB: 20
        name: "container-images"
      diskGiB: 100
      kind: VSphereMachineProviderSpec
      memoryMiB: 16384
      metadata:
        creationTimestamp: null
      network:
        devices:
        - networkName: ci-vlan-1240
      numCPUs: 8
      numCoresPerSocket: 2
      snapshot: ""
      template: ngirard-multi-rtmvw-rhcos-us-east-us-east-1a
      userDataSecret:
        name: master-user-data
      workspace:
        datacenter: cidatacenter
        datastore: /cidatacenter/datastore/vsanDatastore
        folder: /cidatacenter/vm/ngirard-multi-rtmvw
        resourcePool: /cidatacenter/host/cicluster/Resources
        server: openshift.example.com
```

MachineSet:
```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  annotations:
    machine.openshift.io/memoryMb: "8192"
    machine.openshift.io/vCPU: "4"
  creationTimestamp: "2024-11-06T15:30:45Z"
  generation: 2
  labels:
    machine.openshift.io/cluster-api-cluster: ngirard-multi-rtmvw
  name: ngirard-multi-rtmvw-worker-0
  namespace: openshift-machine-api
  resourceVersion: "75977"
  uid: ce68fab6-dcbc-4adf-bba8-ea685bb52fd7
spec:
  replicas: 1
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-cluster: ngirard-multi-rtmvw
      machine.openshift.io/cluster-api-machineset: ngirard-multi-rtmvw-worker-0
  template:
    metadata:
      labels:
        machine.openshift.io/cluster-api-cluster: ngirard-multi-rtmvw
        machine.openshift.io/cluster-api-machine-role: worker
        machine.openshift.io/cluster-api-machine-type: worker
        machine.openshift.io/cluster-api-machineset: ngirard-multi-rtmvw-worker-0
    spec:
      lifecycleHooks: {}
      metadata: {}
      providerSpec:
        value:
          apiVersion: machine.openshift.io/v1beta1
          credentialsSecret:
            name: vsphere-cloud-credentials
          dataDisks:
          - diskSizeGiB: 10
            name: "container_images"
          - diskSizeGiB: 20
            name: "log files"
          - diskSizeGiB: 30
            name: "swap"
          diskGiB: 60
          kind: VSphereMachineProviderSpec
          memoryMiB: 8192
          metadata:
            creationTimestamp: null
          network:
            devices:
            - networkName: ci-vlan-1240
          numCPUs: 4
          numCoresPerSocket: 2
          snapshot: ""
          template: ngirard-multi-rtmvw-rhcos-us-east-us-east-1a
          userDataSecret:
            name: worker-user-data
          workspace:
            datacenter: cidatacenter
            datastore: /cidatacenter/datastore/vsanDatastore
            folder: /cidatacenter/vm/ngirard-multi-rtmvw
            resourcePool: /cidatacenter/host/cicluster/Resources
            server: openshift.example.com
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

### Risks and Mitigations

This feature of allowing administrators to add new disks does not really introduce any risks.  The disks will be created and added to the VMs during the cloning process.  Once the VM is configured, the administrator can configure these disks to be used however they wish.  The assignment of these disks is out of scope for this feature.

### Drawbacks

N/A

## Open Questions [optional]

> 1. Are there any other disk configuration options that we must add in order for this feature to be GA ready?  Is keeping it simple w/ just size enough for GA or just TP?

## Test Plan

- We will add an automated test to CI to test installing a cluster with data disks assigned to both control plane and compute nodes.
- New e2e tests will be added to test creating machine sets with data disk configuration and scaling in these VMs.
- Tests will be added to verify CAPV is working with Cluster CAPI Operator

## Graduation Criteria

### Dev Preview -> Tech Preview

- Installer allows configuration of data disks
- CAPV is able to add disks to VMs during master creation
- MAPI is able to add disks to VMs during creation of machines / machine sets
- Cluster API Operator deploys CAPV with support of adding data disks to VMs
- CI jobs for testing installation with data disks configured
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- User facing documentation created in OCP documentation
- E2E tests are added for testing compute nodes with data disks

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The upgrade / downgrade process is not being impacted by this feature.  No changes will need to be made if rolling back during a failed upgrade.

Upgrade expectations:

- No changes need to be made prior to cluster upgrade

Downgrade expectations:

- If upgrade succeeded and new machines were configured to use data disks, these configuration must be undone before downgraded due to CRD incompatibility. 
- Rollback of install will convert the CRDs back to a supported state.  There is no manual need to remove any CRDs since no new CRDs are introduced.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

- Addition of data disks to VMs adds marginal time to creation (clone) of each virtual machine.  The amount of time is negligible compared to the cloning process as a whole.
- New vmdk files will be created for each machine that will be present in the VM's folder in vCenter.  The naming of the new disk files will follow that of the primary disk.  This is normally the VM's name with _# at the end where # is the index the disk is configured in.

## Support Procedures

N/A

## Alternatives

N/A

## Infrastructure Needed [optional]

N/A
