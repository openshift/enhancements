---
title: azure-data-disk
authors:
- "@jcpowermac"
reviewers:
- "@patrickdillon"
- "@vr4manta"
- "@mfbonfigli"
approvers:
- "@patrickdillon"
api-approvers:
- None
creation-date: 2025-04-22
last-updated: 2026-09-28
tracking-link:
- https://issues.redhat.com/browse/SPLAT-2133
- https://issues.redhat.com/browse/OCPSTRAT-2046
see-also:
- "/enhancements/installer/multi-disk-setup.md"
---

# Azure Data Disks

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This enhancement adds support for configuring multiple data disks on Azure virtual machines used as
OpenShift cluster nodes. As cluster workloads grow and require more storage for specialized use cases
like etcd data, container images, and application data, administrators need the ability to provision
additional disks beyond the primary OS disk.

This feature allows users to define data disks in the install-config.yaml for the control plane and
compute machine pools, with each disk being created and attached during the initial cluster
installation. Formatting and mounting those disks is out of scope here and is covered by the
companion enhancement [multi-disk-setup](./multi-disk-setup.md).

## Motivation

As the use of Kubernetes clusters grows, admins are needing more and more improvements to the VMs
themselves to make sure they run as smoothly as possible. The number of cores and memory continue to
increase for each machine and this is causing the amount of workloads to increase on each virtual
machine. This growth is now causing the base VM image to not provide enough storage for OS needs.

In some cases, users just increase the size of the primary disk using the existing configuration
options for machines; however, this does not allow for all desired configuration choices. Admins are
now wanting the ability to add additional disks to these VMs for things such as etcd storage, image
storage and container runtime.

### User Stories

* As an OpenShift administrator, I want to be able to add additional disks to the Azure VMs which are
  acting as nodes, so that nodes have additional disks I can assign to special case storage such as
  etcd data, container images, etc.

### Goals

- Provide the ability on the control plane and compute machine pools to attach additional Azure
  managed disks at install time, which can then be used for etcd or user-defined storage such as
  container images.

### Non-Goals

- Setup of the disk. Partitioning, formatting and mounting are defined in the companion enhancement
  [multi-disk-setup](./multi-disk-setup.md).
- Adding, removing or resizing data disks on existing machines (day 2).
- Data disks in `platform.azure.defaultMachinePlatform`. See
  [Alternatives](#alternatives-not-implemented).
- Azure Stack Hub. Data disks are explicitly rejected on `StackCloud`.
- Confidential-VM security profiles on data disks. `managedDisk.securityProfile` is rejected rather
  than silently ignored, because the Machine API data disk type has no field to carry it. Adding
  real support requires changes outside the installer and is being investigated separately in
  [SPLAT-2897][splat-2897].

[splat-2897]: https://redhat.atlassian.net/browse/SPLAT-2897

## Proposal

### Workflow Description

**cluster administrator** is a user responsible for installing and configuring an OpenShift cluster.

1. The cluster administrator creates or edits their install-config.yaml file
2. Under `controlPlane.platform.azure` and/or `compute[].platform.azure`, they add a `dataDisks`
   section. `dataDisks` is rejected in `platform.azure.defaultMachinePlatform`.
3. For each data disk, they specify:
   - `lun`: Logical Unit Number (0-63) - Required
   - `diskSizeGB`: Size of the disk in GB - Required
   - `nameSuffix`: Optional suffix for the disk name
   - `cachingType`: Optional caching strategy (None, ReadOnly, ReadWrite)
   - `managedDisk`: Optional managed disk configuration including storage account type and
     encryption settings
4. The cluster administrator runs `openshift-install create cluster`
5. During cluster provisioning, the installer:
   - Validates the data disk configuration
   - Creates `AzureMachine` (CAPI) resources for the control plane and `Machine`/`MachineSet` (MAPI)
     resources for compute, each carrying the specified data disks
   - Azure provisions the VMs with all data disks attached
6. The data disks are available in the VM but not partitioned, formatted or mounted. That is handled
   separately, either by the `diskSetup` API described in
   [multi-disk-setup](./multi-disk-setup.md) or by a user-supplied MachineConfig.

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
        nameSuffix: logs
        diskSizeGB: 64
        cachingType: None
```

The resulting Azure managed disks are named `<machine-name>_<nameSuffix>`, for example
`mycluster-abcde-master-0_etcd`, and are addressable inside the guest at
`/dev/disk/azure/scsi1/lun<lun>`.



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

The upstream `capz.DataDisk` type is reused as-is. 
```go
type DataDisk struct {
    // NameSuffix is the suffix to be appended to the machine name to generate
    // the disk name. Each disk name will be in format <machineName>_<nameSuffix>.
    NameSuffix string `json:"nameSuffix"`

    // DiskSizeGB is the size in GB to assign to the data disk.
    DiskSizeGB int32 `json:"diskSizeGB"`

    // ManagedDisk specifies the Managed Disk parameters for the data disk.
    // +optional
    ManagedDisk *ManagedDiskParameters `json:"managedDisk,omitempty"`

    // Lun specifies the logical unit number of the data disk. The value must be
    // between 0 and 63 and unique for each data disk attached to a VM. The installer requires it, but only when diskSetup is
    // also set.
    // +optional
    Lun *int32 `json:"lun,omitempty"`

    // CachingType specifies the caching requirements.
    // Possible values include: None, ReadOnly, ReadWrite.
    // +optional
    CachingType string `json:"cachingType,omitempty"`
}

type ManagedDiskParameters struct {
    // StorageAccountType is the storage account type to use.
    // Examples: Standard_LRS, Premium_LRS, StandardSSD_LRS, UltraSSD_LRS. The installer requires it to be non-empty whenever
    // managedDisk is specified.
    // +optional
    StorageAccountType string `json:"storageAccountType,omitempty"`

    // DiskEncryptionSet is the disk encryption set properties.
    // +optional
    DiskEncryptionSet *DiskEncryptionSetParameters `json:"diskEncryptionSet,omitempty"`

    // SecurityProfile specifies the security profile for the managed disk.
    // +optional
    SecurityProfile *VMDiskSecurityProfile `json:"securityProfile,omitempty"`
}
```

#### Validation Rules

Validation is split across two functions with different trigger conditions. This distinction matters,
so it is spelled out explicitly.

**Always enforced when `dataDisks` is non-empty:**

1. **Placement**:
   - `dataDisks` is rejected in `platform.azure.defaultMachinePlatform`
   - `dataDisks` is rejected when `platform.azure.cloudName` is `AzureStackCloud`

2. **ManagedDisk.StorageAccountType**:
   - Required whenever `managedDisk` is specified, on both control plane and compute pools
   - Cannot be the empty string

3. **ManagedDisk.SecurityProfile**:
   - Rejected on both control plane and compute pools. `capz.ManagedDiskParameters` carries the
     field, but the Machine API `DataDiskManagedDiskParameters` type has no `securityProfile`
     member at all, so the installer rejects it rather than accepting and discarding it.

4. **Feature Gate**:
   - `FeatureGateAzureMultiDisk` must be enabled when `dataDisks` are configured
   - Applies to `defaultMachinePlatform`, `controlPlane` and `compute`

**Enforced only when both `dataDisks` and `diskSetup` are non-empty:**

5. **Lun (Logical Unit Number)**:
   - Must not be nil
   - Must be in the range 0-63
   - Must be unique within the `dataDisks` array for that pool

6. **DiskSizeGB**:
   - Must be greater than 0

7. **Correspondence with `diskSetup`**:
   - `len(diskSetup)` must not exceed `len(dataDisks)`
   - `diskSetup[i]`'s `platformDiskID` must equal `dataDisks[i]`'s `nameSuffix`. The match is
     **positional**, not by name -- see [multi-disk-setup](./multi-disk-setup.md).

#### Implementation Details

There are two consumers of `dataDisks`, and both are exercised by a normal install:

- **CAPI** (`pkg/asset/machines/azure/azuremachines.go`) builds the `AzureMachine` resources used to
  provision the control plane during bootstrap. `capz.DataDisk` is the install-config type, so these
  pass through unconverted.
- **MAPI** (`pkg/asset/machines/azure/machines.go`) builds the `Machine` and `MachineSet` resources
  that the cluster reconciles after bootstrap, including all compute machines. These require a
  conversion to `machineapi.DataDisk`.

The MAPI conversion:

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

This feature does not apply to Hypershift / Hosted Control Planes.

#### Standalone Clusters

This is the target topology. Data disks can be configured on the `controlPlane` and `compute`
machine pools.

#### Single-node Deployments or MicroShift

This feature applies to single-node OpenShift, which is installer-provisioned and has a `master`
pool like any other standalone cluster. Data disks can be attached to the single node by configuring
them on `controlPlane`.

This feature does not apply to MicroShift, which does not use the OpenShift installer.

#### OpenShift Kubernetes Engine

This feature applies to OpenShift Kubernetes Engine.

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
- The MAPI conversion always sets `DeletionPolicy: Delete`, so a data disk is removed when its
  `Machine` is deleted. `capz.DataDisk` has no equivalent field; the CAPI path only provisions the
  control plane during bootstrap, and those machines are represented by MAPI `Machine` objects and
  a `ControlPlaneMachineSet` from then on
- Disks are not formatted or mounted automatically - this must be handled separately (e.g., via MachineConfig)

**Storage Account Types:**

`managedDisk.storageAccountType` is passed through to Azure unchanged. The installer only checks
that it is non-empty, so the accepted values are whatever Azure accepts for the region and
instance type, and a typo is not caught at install-config validation time. The commonly used
values are:

- `Standard_LRS` - Standard locally redundant storage
- `Premium_LRS` - Premium locally redundant storage (SSD)
- `StandardSSD_LRS` - Standard SSD locally redundant storage
- `UltraSSD_LRS` - Ultra SSD locally redundant storage

`UltraSSD_LRS` additionally requires `ultraSSDCapability: Enabled` on the same machine pool, which
is what enables the capability on the VM itself. That field is independent of `dataDisks` and
defaults to `Disabled`.

**Constraints:**

- LUN values must be unique within each machine's `dataDisks` configuration. Uniqueness is only
  enforced when `diskSetup` is also present.
- Maximum of 64 data disks per VM (LUN 0-63). The practical limit is lower and depends on the VM
  size; Azure rejects the VM creation if the instance type does not support the requested count.
- `managedDisk.securityProfile` is rejected on every pool, not just compute. Supporting it is
  tracked separately in [SPLAT-2897][splat-2897].
- Data disks cannot be configured in `defaultMachinePlatform`.
- Data disks are not supported on Azure Stack Hub.
- The same `dataDisks` list applies to every machine in a pool. Per-machine disk layouts are not
  expressible.


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

**Unit Tests** (`pkg/types/azure/validation/machinepool_test.go`), implemented:

- `managedDisk` with an empty `storageAccountType` is rejected
- `managedDisk.securityProfile` is rejected
- `dataDisks` in `defaultMachinePlatform` is rejected
- `dataDisks` on `AzureStackCloud` is rejected
- With `diskSetup` present: nil LUN, LUN outside 0-63, duplicate LUNs, and `diskSizeGB <= 0` are
  each rejected
- With `diskSetup` present: `len(diskSetup) > len(dataDisks)` is rejected, and a `platformDiskID`
  that does not match the `nameSuffix` at the same index is rejected
- Valid configurations pass
- `dataDisks` set on `defaultMachinePlatform`, `controlPlane` or `compute` without
  `FeatureGateAzureMultiDisk` is rejected
- `dataDisks` are converted to `machineapi.DataDisk` with `DeletionPolicy: Delete`
- `dataDisks` are propagated to the `AzureMachine` spec

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

The above is exercised by `e2e-azure-ovn-multidisk-techpreview`, defined for both
`openshift/installer` and `openshift/machine-config-operator`, with
`e2e-azure-ovn-multidisk-techpreview-upgrade` covering the upgrade case. Both jobs are currently
optional and do not run by default.

## Graduation Criteria

### Dev Preview -> Tech Preview

- [x] Installer allows configuration of data disks
- [x] CI job installing with data disks configured
- [x] Relative API stability
- [x] Sufficient unit test coverage

### Tech Preview -> GA

- [x] E2E tests covering control plane **and** compute nodes with data disks
      (job `e2e-azure-ovn-multidisk-techpreview`)
- [x] Sufficient time for feedback. The gate has been in TechPreview since 4.20.
- [ ] Available by default -- requires moving `FeatureGateAzureMultiDisk` to
      `enable(inDefault(), inOKD(), inTechPreviewNoUpgrade(), inDevPreviewNoUpgrade())` in
      `openshift/api`
- [ ] User-facing documentation in the OCP installation docs

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

This feature does not impact the upgrade or downgrade process. Data disks are provisioned at cluster installation time and are not modified during cluster upgrades.

Existing clusters without data disks can continue to operate normally after upgrading to a version that includes this feature. New machine pools created after upgrade can utilize the data disk configuration feature.

During a failed upgrade rollback, no special handling is required for data disks: they are attached when the VM is provisioned and are not touched by upgrades.

## Version Skew Strategy

This feature does not introduce version skew concerns. Data disk configuration is handled entirely during cluster installation by the installer and is translated into Machine API resources. There is no runtime coordination between components of different versions required.

## Operational Aspects of API Extensions

This enhancement does not introduce API extensions in the Kubernetes API sense (no CRDs, admission webhooks, or aggregated API servers). The changes are limited to the installer's install-config.yaml schema.

The installer validates the configuration at install time and generates appropriate Machine API resources. No ongoing API validation or conversion is required post-installation.

## Support Procedures

**Detecting Configuration Issues:**

If the data disk configuration is invalid, `openshift-install` fails during install-config
validation. The messages emitted today are:

- `"not allowed on default machine pool, use dataDisks compute and controlPlane only"`
- `"data disks are not supported on AzureStackCloud"`
- `"storageAccountType is required when managedDisk is specified"`
- `"data disk security profiles are not yet supported by the Machine API and would be silently
  ignored"`
- `"<nameSuffix>" must have lun id` - LUN omitted, **only reported when `diskSetup` is also set**
- `"<nameSuffix>" must have lun id between 0 and 63` - LUN out of range, same condition
- `"dataDisk must have a unique lun number"` - duplicate LUN, same condition
- `"diskSizeGB must be greater than zero"` - same condition

If `FeatureGateAzureMultiDisk` is not enabled, validation fails with a feature gate error naming the
`dataDisks` field.

**Verifying Data Disks on Running VMs:**

The most reliable in-guest check is the Azure udev symlink tree, which is keyed by LUN and therefore
maps directly back to the install-config:

```console
$ ls -l /dev/disk/azure/scsi1/
lrwxrwxrwx. 1 root root 12 ... lun0 -> ../../../sdc
lrwxrwxrwx. 1 root root 12 ... lun1 -> ../../../sdd
```

`lsblk` also lists the devices, but kernel `/dev/sd*` names are assigned in attach order and are not
stable across reboots, so they should not be used to identify a specific configured disk. Disk
size, caching type and storage account type are visible from the management side with
`az vm show -d -g <resource-group> -n <vm-name> --query storageProfile.dataDisks`.

**Common Issues:**

- **Disks not appearing in the guest**: confirm the disks exist in Azure first
  (`az vm show` as above).
- **VM creation rejected by Azure**: the requested disk count exceeds the maximum for the VM size.
  Choose a larger instance type or fewer disks.
- **Performance issues**: verify `storageAccountType` (`Premium_LRS` for production workloads) and
  the caching type.

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



