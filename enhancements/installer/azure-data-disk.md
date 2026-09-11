---
title: azure-data-disks
authors:
- "@jcpowermac"
reviewers:
- "@patrickdillon"
approvers:
- "@patrickdillon"
creation-date: 2025-04-22
last-updated: 2025-10-24
tracking-link:
- https://issues.redhat.com/browse/SPLAT-2133
---

# Azure Data Disks

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This enhancement adds support for configuring multiple data disks on Azure virtual machines used as OpenShift cluster nodes. As cluster workloads grow and require more storage for specialized use cases like etcd data, container images, swap space, and application data, administrators need the ability to provision additional disks beyond the primary OS disk. This feature allows users to define data disks in the install-config.yaml for any machine pool, with each disk being created and attached during the initial cluster installation.

## Motivation

As the use of Kubernetes clusters grows, admins are needing more and more improvements to the VMs themselves to make sure they run as smoothly as possible.  The number of cores and memory continue to increase for each machine and this is causing the amount of workloads to increase on each virtual machine.  This growth is now causing the base VM image to not provide enough storage for OS needs.  In some cases, users just increase the size of the primary disk using the existing configuration options for machines; however, this does not allows for all desired configuration choices.  Admins are now wanting the ability to add additional disks to these VMs for things such as etcd storage, image storage, container runtime and even swap.

### User Stories

* As an OpenShift administrator, I want to be able to add additional disks to any of the azure VMs which are acting as a node so nodes can have additional disks for me to use to assign special case storage such as etcd data, swap, container images, etc.

### Goals

- Provide the ability on the machinepool to add additional disks that could be used for etcd, swap or user defined storage like containers.

### Non-Goals

- Setup of the disk. That will be defined in an additional enhancement.

## Proposal

### Workflow Description

**cluster administrator** is a user responsible for installing and configuring an OpenShift cluster.

1. The cluster administrator creates or edits their install-config.yaml file
2. For any machine pool (control plane, compute, or custom), they add a `dataDisks` section under the Azure platform configuration
3. For each data disk, they specify:
   - `lun`: Logical Unit Number (0-63) - Required
   - `diskSizeGB`: Size of the disk in GB - Required
   - `nameSuffix`: Optional suffix for the disk name
   - `cachingType`: Optional caching strategy (None, ReadOnly, ReadWrite)
   - `managedDisk`: Optional managed disk configuration including storage account type and encryption settings
4. The cluster administrator runs `openshift-install create cluster`
5. During cluster provisioning, the installer:
   - Validates the data disk configuration (LUN uniqueness, size > 0, LUN range 0-63)
   - Creates AzureMachine resources with the specified data disks
   - Azure provisions the VMs with all data disks attached
6. The data disks are available in the VM but not formatted or mounted (this is handled separately via MachineConfig or other mechanisms)

#### Example Configuration

Example install-config.yaml with data disks on control plane nodes:

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: mycluster
platform:
  azure:
    region: eastus
    resourceGroupName: my-resource-group
controlPlane:
  name: master
  replicas: 3
  platform:
    azure:
      type: Standard_D8s_v3
      dataDisks:
      - lun: 0
        nameSuffix: etcd
        diskSizeGB: 512
        cachingType: ReadOnly
        managedDisk:
          storageAccountType: Premium_LRS
compute:
- name: worker
  replicas: 3
  platform:
    azure:
      type: Standard_D4s_v3
      dataDisks:
      - lun: 0
        nameSuffix: containers
        diskSizeGB: 256
        cachingType: ReadWrite
      - lun: 1
        nameSuffix: swap
        diskSizeGB: 64
        cachingType: None
```



### API Extensions

This enhancement modifies the installer's install-config.yaml API by adding a new `dataDisks` field to Azure machine pools.

#### Installer API Changes

The installer's Azure MachinePool type is enhanced to support data disk configuration:

```go
type MachinePool struct {
    // DataDisk specifies the parameters that are used to add one or more data disks to the machine.
    // +optional
    DataDisks []capz.DataDisk `json:"dataDisks,omitempty"`
}
```

The `capz.DataDisk` type provides the following fields:

```go
type DataDisk struct {
    // Lun is the Logical Unit Number (0-63) for the data disk.
    // Required field.
    Lun *int32 `json:"lun"`

    // NameSuffix is the suffix to be appended to the disk name.
    // Optional field.
    NameSuffix string `json:"nameSuffix,omitempty"`

    // DiskSizeGB is the size in GB to assign to the data disk.
    // Required field, must be greater than 0.
    DiskSizeGB int32 `json:"diskSizeGB"`

    // CachingType specifies the caching requirements.
    // Possible values include: None, ReadOnly, ReadWrite.
    // Optional field.
    CachingType string `json:"cachingType,omitempty"`

    // ManagedDisk specifies the managed disk parameters.
    // Optional field.
    ManagedDisk *ManagedDiskParameters `json:"managedDisk,omitempty"`
}

type ManagedDiskParameters struct {
    // StorageAccountType is the storage account type to use.
    // Examples: Standard_LRS, Premium_LRS, StandardSSD_LRS, UltraSSD_LRS
    // Required when ManagedDisk is specified for control plane nodes.
    StorageAccountType string `json:"storageAccountType"`

    // DiskEncryptionSet is the disk encryption set properties.
    // Optional field.
    DiskEncryptionSet *DiskEncryptionSetParameters `json:"diskEncryptionSet,omitempty"`

    // SecurityProfile specifies the security profile for the managed disk.
    // Note: Not supported for worker/compute nodes.
    // Optional field.
    SecurityProfile *VMDiskSecurityProfile `json:"securityProfile,omitempty"`
}
```

#### Validation Rules

The following validation rules are enforced during install-config validation:

1. **Lun (Logical Unit Number)**:
   - Required field, cannot be nil
   - Must be in the range 0-63
   - Must be unique within the dataDisks array for each machine

2. **DiskSizeGB**:
   - Required field
   - Must be greater than 0

3. **ManagedDisk.StorageAccountType**:
   - Required when `managedDisk` is specified for control plane nodes
   - Cannot be empty string

4. **ManagedDisk.SecurityProfile**:
   - Not supported for worker/compute machines
   - Will result in validation error if specified on worker nodes

5. **Feature Gate**:
   - The feature gate `FeatureGateAzureMultiDisk` must be enabled when dataDisks are configured
   - Applies to machine pool configurations (control plane and compute)

#### Implementation Details

The installer converts the install-config dataDisks to Machine API format:

```go
dataDisks := make([]machineapi.DataDisk, 0, len(mpool.DataDisks))

for _, disk := range mpool.DataDisks {
    dataDisk := machineapi.DataDisk{
        NameSuffix:     disk.NameSuffix,
        DiskSizeGB:     disk.DiskSizeGB,
        CachingType:    machineapi.CachingTypeOption(disk.CachingType),
        DeletionPolicy: machineapi.DiskDeletionPolicyTypeDelete,
    }

    if disk.Lun != nil {
        dataDisk.Lun = *disk.Lun
    }

    if disk.ManagedDisk != nil {
        dataDisk.ManagedDisk = machineapi.DataDiskManagedDiskParameters{
            StorageAccountType: machineapi.StorageAccountType(disk.ManagedDisk.StorageAccountType),
        }

        if disk.ManagedDisk.DiskEncryptionSet != nil {
            dataDisk.ManagedDisk.DiskEncryptionSet = (*machineapi.DiskEncryptionSetParameters)(disk.ManagedDisk.DiskEncryptionSet)
        }
    }

    dataDisks = append(dataDisks, dataDisk)
}
```

The dataDisks are then applied to the CAPI AzureMachine specification:

```go
azureMachine := &capz.AzureMachine{
    ObjectMeta: metav1.ObjectMeta{
        Name: fmt.Sprintf("%s-%s-%d", clusterID, in.Pool.Name, idx),
        Labels: map[string]string{
            "cluster.x-k8s.io/control-plane": "",
            "cluster.x-k8s.io/cluster-name":  clusterID,
        },
    },
    Spec: capz.AzureMachineSpec{
        // ... other fields ...
        DataDisks: mpool.DataDisks,
    },
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This feature applies to Hypershift / Hosted Control Planes. Data disks can be configured for worker nodes in hosted clusters.

#### Standalone Clusters

This feature applies to standalone OpenShift clusters. Data disks can be configured for both control plane and compute nodes.

#### Single-node Deployments or MicroShift

This feature applies to single-node OpenShift deployments. Data disks can be configured on the single control plane node. This feature does not apply to MicroShift as it does not use the OpenShift installer.

### Implementation Details/Notes/Constraints

**Modified Files:**

The implementation spans several key installer components:

- `pkg/types/azure/machinepool.go` - Adds the `DataDisks` field to the Azure MachinePool type
- `pkg/types/azure/validation/machinepool.go` - Implements validation logic for data disk configuration
- `pkg/types/azure/validation/machinepool_test.go` - Unit tests for validation
- `pkg/asset/machines/azure/machines.go` - Converts install-config data disks to Machine API format
- `pkg/asset/machines/azure/azuremachines.go` - Applies data disks to CAPI AzureMachine specs
- `pkg/types/azure/validation/featuregates.go` - Implements feature gate checking
- `pkg/types/validation/machinepools.go` - Cross-platform validation hooks

**Feature Gate:**

The `FeatureGateAzureMultiDisk` feature gate controls access to this functionality. The installer will validate that this feature gate is enabled when dataDisks are configured in the install-config.yaml.

**Disk Provisioning:**

- Data disks are created and attached during VM provisioning by Azure
- Disks are created as managed disks in the same resource group as the cluster
- The `DeletionPolicy` is always set to `Delete`, meaning disks are removed when the VM is deleted
- Disks are not formatted or mounted automatically - this must be handled separately (e.g., via MachineConfig)

**Storage Account Types:**

The following Azure storage account types are supported:
- `Standard_LRS` - Standard locally redundant storage
- `Premium_LRS` - Premium locally redundant storage (SSD)
- `StandardSSD_LRS` - Standard SSD locally redundant storage
- `UltraSSD_LRS` - Ultra SSD locally redundant storage

**Constraints:**

- LUN values must be unique within each machine's dataDisks configuration
- Maximum of 64 data disks per VM (LUN 0-63)
- SecurityProfile on data disks is only supported for control plane nodes
- Data disks cannot be configured in the defaultMachinePlatform


### Risks and Mitigations

This feature of allowing administrators to add new disks does not really introduce any risks.  The disks will be created and added to the VMs during the provisioning.  Once the VM is configured, the administrator can configure these disks to be used however they wish.  The assignment of these disks is out of scope for this feature.

### Drawbacks

**Install-time Only Configuration:**

Data disks can only be configured during cluster installation. Existing machines cannot have data disks added post-installation. Users who later need additional storage must create new MachineSets with the desired disk configuration and migrate workloads, or use alternative storage solutions like PersistentVolumes.

**Requires Separate Disk Setup:**

This enhancement only provisions raw, unformatted disks. Users must use additional mechanisms (MachineConfig, DaemonSets, etc.) to format, partition, and mount the disks. This adds complexity to the overall setup process, though it provides maximum flexibility for different use cases.

**Feature Gate Requirement:**

The feature requires enabling a feature gate, which adds an extra configuration step for users and may not be discoverable without reading documentation.

## Open Questions [optional]

None.

## Test Plan

**Unit Tests:**

- Validation logic for data disk configuration (`pkg/types/azure/validation/machinepool_test.go`):
  - Test missing LUN field returns validation error
  - Test LUN value out of range (< 0 or > 63) returns validation error
  - Test DiskSizeGB of 0 returns validation error
  - Test empty StorageAccountType for control plane with managedDisk returns validation error
  - Test SecurityProfile on worker nodes returns validation error
  - Test valid data disk configurations pass validation

- Feature gate validation:
  - Test that dataDisks configuration triggers FeatureGateAzureMultiDisk
  - Test that feature gate is properly validated

**Integration Tests:**

- Install-config validation:
  - Create install-config.yaml with various data disk configurations
  - Verify validation catches all error conditions
  - Verify valid configurations are accepted

- Machine generation:
  - Verify that install-config dataDisks are properly converted to Machine API format
  - Verify that dataDisks are correctly applied to AzureMachine specs
  - Verify DeletionPolicy is set to Delete

**End-to-End Tests:**

- Cluster installation with data disks on control plane:
  - Create cluster with data disks configured on master machine pool
  - Verify cluster installs successfully
  - Verify data disks are attached to control plane VMs
  - Verify disk properties (size, LUN, caching type, storage account type)

- Cluster installation with data disks on compute nodes:
  - Create cluster with data disks configured on worker machine pool
  - Verify cluster installs successfully
  - Verify data disks are attached to worker VMs

- Multiple data disks:
  - Configure multiple data disks with different LUNs
  - Verify all disks are created and attached correctly

- Cluster destruction:
  - Delete cluster with data disks configured
  - Verify all data disks are removed (DeletionPolicy: Delete)

**CI Jobs:**

CI jobs will be created to test installation with various data disk configurations to ensure ongoing stability of this feature.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Installer allows configuration of data disks
- CI jobs for testing installation with data disks configured
- End user documentation, relative API stability
- Sufficient test coverage

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

This feature does not impact the upgrade or downgrade process. Data disks are provisioned at cluster installation time and are not modified during cluster upgrades.

Existing clusters without data disks can continue to operate normally after upgrading to a version that includes this feature. New machine pools created after upgrade can utilize the data disk configuration feature.

During a failed upgrade rollback, no special handling is required for data disks as they are part of the immutable machine configuration.

## Version Skew Strategy

This feature does not introduce version skew concerns. Data disk configuration is handled entirely during cluster installation by the installer and is translated into Machine API resources. There is no runtime coordination between components of different versions required.

## Operational Aspects of API Extensions

This enhancement does not introduce API extensions in the Kubernetes API sense (no CRDs, admission webhooks, or aggregated API servers). The changes are limited to the installer's install-config.yaml schema.

The installer validates the configuration at install time and generates appropriate Machine API resources. No ongoing API validation or conversion is required post-installation.

## Support Procedures

**Detecting Configuration Issues:**

- If data disk configuration is invalid, the installer will fail during install-config validation with clear error messages:
  - "must have lun id" - LUN field is missing
  - "must have lun id between 0 and 63" - LUN value out of range
  - "must be greater than zero" - DiskSizeGB is 0 or negative
  - "storageAccount type must not be empty" - StorageAccountType required for control plane
  - "data disk security profiles are not supported for worker machines" - SecurityProfile used on worker

- If the feature gate is not enabled, validation will fail with a feature gate error

**Verifying Data Disks on Running VMs:**

To verify data disks are attached to a node:

1. SSH to the node
2. Run `lsblk` to list all block devices
3. Data disks will appear as `/dev/sd*` devices (e.g., `/dev/sdc`, `/dev/sdd`)
4. The LUN can be verified via Azure Portal or CLI

**Common Issues:**

- **Disks not appearing**: Verify the install-config.yaml was correctly configured and cluster installation completed successfully
- **Wrong disk size**: Check the diskSizeGB value in the install-config.yaml
- **Performance issues**: Verify the correct StorageAccountType is selected (Premium_LRS for production workloads)

## Alternatives (Not Implemented)

**Alternative 1: Day 2 Disk Addition**

Instead of requiring data disks to be configured at install time, allow adding disks to existing machines post-installation. This was not implemented because:
- It adds significant complexity to machine management
- Existing machines are immutable in the Machine API model
- Users can create new MachineSets with data disks if needed

**Alternative 2: Automatic Disk Formatting and Mounting**

Automatically format and mount data disks during VM provisioning. This was not implemented because:
- Different use cases require different filesystem types and mount options
- This is better handled by MachineConfig which provides more flexibility
- Separation of concerns: installer provisions infrastructure, MachineConfig configures OS

**Alternative 3: Support in defaultMachinePlatform**

Allow data disk configuration in platform.azure.defaultMachinePlatform. This was not implemented to maintain consistency with how other machine-specific configurations are handled and to avoid complexity in inheritance and override behavior.

## Infrastructure Needed [optional]

None. This feature uses existing Azure infrastructure and does not require new CI resources or testing infrastructure beyond standard E2E test clusters.



