---
title: azure-data-disk
authors:
- "@jcpowermac"
- "@mfbonfigli"
reviewers:
- "@patrickdillon"
- "@vr4manta"
approvers:
- "@patrickdillon"
api-approvers:
- None
creation-date: 2025-04-22
last-updated: 2026-10-22
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
installation. Partitioning, formatting and mounting those disks is the other half of the same
feature. The disk setup implementation here in Azure is aligned with the mechanisms described in
the companion enhancement [multi-disk-setup](./multi-disk-setup.md); however, it ships within the
same `AzureMultiDisk` gate, since on Azure disk provisioning and setup need to ship together.

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
- Allow those disks to be partitioned, formatted and mounted at install time. On Azure this is a
  required part of the same feature.

### Non-Goals

- Adding, removing or resizing data disks on existing machines (day 2).
- Data disks in `platform.azure.defaultMachinePlatform`. See
  [Alternatives](#alternatives-not-implemented).
- Azure Stack Hub. Data disks are explicitly rejected on `StackCloud`.

## Proposal

### Workflow Description

**cluster administrator** is a user responsible for installing and configuring an OpenShift cluster.

1. The cluster administrator creates or edits their install-config.yaml file
2. Under `controlPlane.platform.azure` and/or `compute[].platform.azure`, they add a `dataDisks`
   section. `dataDisks` is rejected in `platform.azure.defaultMachinePlatform`.
3. For each data disk, they specify:
   - `lun`: Logical Unit Number (0-63) - Required
   - `diskSizeGB`: Size of the disk in GB - Required
   - `nameSuffix`: Suffix of the disk name, `<machine-name>_<nameSuffix>` - Required
   - `cachingType`: Optional caching strategy (None, ReadOnly, ReadWrite), defaulting to None
   - `managedDisk`: Optional managed disk configuration including storage account type and
     encryption settings
4. To have a disk partitioned, formatted and mounted, they add a `diskSetup` entry for it on the
   same machine pool, specifying:
   - `type`: The disk's purpose - `etcd`, `swap` or `user-defined`
   - `platformDiskID`: The data disk the entry acts on, which must be the `nameSuffix` of the
     `dataDisks` entry at the same index
   - `mountPath`: Where to mount the filesystem (`user-defined` only; an `etcd` disk always
     mounts at `/var/lib/etcd`)
5. The cluster administrator runs `openshift-install create cluster`
6. During cluster provisioning, the installer:
   - Validates the data disk and disk setup configuration
   - Creates `AzureMachine` (CAPI) resources for the control plane and `Machine`/`MachineSet` (MAPI)
     resources for compute, each carrying the specified data disks
   - Generates a MachineConfig for every `diskSetup` entry, which the Machine Config Operator
     applies as the nodes boot
   - Azure provisions the VMs with all data disks attached
7. The VMs come up with the data disks attached and, for every disk with a `diskSetup` entry,
   partitioned, formatted and mounted as described in **Disk Setup** below. A disk with no
   `diskSetup` entry is attached raw, and can still be set up with a user-supplied MachineConfig.

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
  diskSetup:
  - type: etcd
    etcd:
      platformDiskID: etcd  # Matches the nameSuffix above
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
  diskSetup:
  - type: user-defined
    userDefined:
      platformDiskID: containers  # Matches the nameSuffix at the same index
      mountPath: /var/lib/containers
```

The resulting Azure managed disks are named `<machine-name>_<nameSuffix>`, for example
`mycluster-abcde-master-0_etcd`, and are addressable inside the guest at
`/dev/disk/azure/data/by-lun/<lun>`. That link is created by the `azure-vm-utils` udev rules for
data disks on both the SCSI and the NVMe controller, so it exists on NVMe-only instance types too,
and it is what the generated disk setup MachineConfigs address the disk through.

In this example the `etcd` disk is partitioned, formatted and mounted at `/var/lib/etcd` on the
control plane nodes and the `containers` disk at `/var/lib/containers` on the compute nodes, while
the `logs` disk has no `diskSetup` entry and stays raw.

#### Disk Setup

`diskSetup` entries live on the machine pool, next to the `dataDisks` they act on: `diskSetup[i]`
configures `dataDisks[i]`, and the entry's `platformDiskID` must equal that disk's `nameSuffix`
(see **Validation Rules**). For each entry the installer generates one MachineConfig, applied
when the nodes first boot. The entry's type is one of `etcd`, `swap` or `user-defined`. Its
partition label names the MachineConfig, the partition and the `/dev/disk/by-partlabel/<label>`
device: `etcd` and `swap` disks are labelled after their type, a `user-defined` disk after its
`platformDiskID` with every non-alphanumeric character stripped. The mount unit is named after
the systemd-escaped form of the mount path, so `/var/lib/etcd` becomes `var-lib-etcd.mount`. The
filesystem is XFS, mounted with the `defaults` and `prjquota` options.

- The device the MachineConfig partitions is the data disk's by-LUN udev link,
  `/dev/disk/azure/data/by-lun/<lun>`, which exists on both the SCSI and the NVMe controller.
- The `platformDiskID` is the paired disk's `nameSuffix`, so it inherits the `nameSuffix` bounds:
  at most 15 characters and only the characters Azure admits in a disk name. A `user-defined`
  entry's ID is additionally capped at 12 characters, because the ID becomes the GPT partition
  label and part of the MachineConfig name, so a `user-defined` disk's `nameSuffix` is
  effectively capped at 12.
- `type: etcd` is permitted only on the control plane and mounts at `/var/lib/etcd`; a `swap`
  entry is permitted only on compute pools. At most one `etcd` and one `swap` entry per pool, and
  any number of `user-defined` entries.
- A `user-defined` entry's `mountPath` must be an absolute, normalized path other than `/`,
  containing only alphanumerics, `/`, `.`, `_` and `-`, and the systemd mount unit name derived
  from it must stay within the 255 characters systemd loads.
- Two entries in one pool must not mount at the same path: they would ship conflicting mount
  units on different devices. The `etcd` entry's implicit `/var/lib/etcd` participates, so a
  `user-defined` entry cannot claim it.

For the `etcd` entry of the example above, the generated MachineConfig is:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: master
  name: 01-disk-setup-etcd-master
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      disks:
      - device: /dev/disk/azure/data/by-lun/0
        partitions:
        - label: etcd
          sizeMiB: 0
          startMiB: 0
        wipeTable: true
      filesystems:
      - device: /dev/disk/by-partlabel/etcd
        format: xfs
        label: etcd
        mountOptions:
        - defaults
        - prjquota
        path: /var/lib/etcd
        wipeFilesystem: true
    systemd:
      units:
      - contents: |

          [Unit]
          Requires=systemd-fsck@dev-disk-by\x2dpartlabel-etcd.service
          After=systemd-fsck@dev-disk-by\x2dpartlabel-etcd.service

          [Mount]
          Where=/var/lib/etcd
          What=/dev/disk/by-partlabel/etcd
          Type=xfs
          Options=defaults,prjquota

          [Install]
          RequiredBy=local-fs.target
        enabled: true
        name: var-lib-etcd.mount
```

The partition spans the whole disk (`sizeMiB: 0`), and the configuration wipes the partition
table of the device and any filesystem already on it, so it must only ever address a freshly
provisioned data disk. A `swap` entry differs in the partition type (the GPT swap type GUID),
the filesystem format and the unit (`dev-disk-by\x2dpartlabel-swap.swap`, wanted by
`swap.target`), and has no mount path.



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
    // The installer requires it, bounds it to 15 characters so the disk name fits
    // Azure's limits, restricts it to the characters Azure admits in a disk name,
    // and requires it to be unique within a pool.
    NameSuffix string `json:"nameSuffix"`

    // DiskSizeGB is the size in GB to assign to the data disk.
    DiskSizeGB int32 `json:"diskSizeGB"`

    // ManagedDisk specifies the Managed Disk parameters for the data disk.
    // +optional
    ManagedDisk *ManagedDiskParameters `json:"managedDisk,omitempty"`

    // Lun specifies the logical unit number of the data disk. The value must be
    // between 0 and 63 and unique for each data disk attached to a VM. The installer requires it.
    // +optional
    Lun *int32 `json:"lun,omitempty"`

    // CachingType specifies the caching requirements.
    // Possible values include: None, ReadOnly, ReadWrite. The installer rejects any other
    // value, and anything but None for an UltraSSD_LRS disk. An omitted value is
    // defaulted to None.
    // +optional
    CachingType string `json:"cachingType,omitempty"`
}

type ManagedDiskParameters struct {
    // StorageAccountType is the storage account type to use.
    // Examples: Standard_LRS, Premium_LRS, StandardSSD_LRS, UltraSSD_LRS. The installer requires it
    // whenever managedDisk is specified, and to be a managed disk SKU the Azure SDK defines.
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
   - Must be a managed disk SKU defined by the Azure compute SDK. See **Storage Account Types**
     below.

3. **ManagedDisk.SecurityProfile**:
   - Rejected on both control plane and compute pools. The field is carried by the CAPZ type the
     install-config embeds, but the Machine API type the installer converts compute disks to has
     no `securityProfile` member, so the setting would be silently dropped there, and Azure does
     not support confidential disk encryption on data disks in the first place. The installer
     rejects it rather than accepting and discarding it.

4. **Feature Gate**:
   - `FeatureGateAzureMultiDisk` must be enabled when `dataDisks` and `diskSetup` are configured
     in Azure
   - Applies to `defaultMachinePlatform`, `controlPlane` and `compute`

5. **NameSuffix**:
   - Required, and must be unique within the `dataDisks` array for that pool. It is appended to
     the machine name to form the managed disk name, and it is what `diskSetup` is matched
     against. Two disks sharing one would otherwise only be rejected during provisioning, by the
     CAPZ and Machine API webhooks.
   - At most 15 characters: Azure caps disk names at 80 characters and VM names at 64, and the
     disk name is `<machineName>_<nameSuffix>`. The bound also covers the `platformDiskID` of a
     paired `diskSetup` entry, which must equal the `nameSuffix`.
   - Restricted to the characters Azure admits in a disk name: alphanumerics, `_`, `.` and `-`,
     ending with an alphanumeric or `_`.

6. **Lun (Logical Unit Number)**:
   - Must not be nil
   - Must be in the range 0-63
   - Must be unique within the `dataDisks` array for that pool

7. **DiskSizeGB**:
   - Must be greater than 0
   - On the control plane, must be within the 4-32767 GB that the CAPZ webhook admits, since
     CAPZ creates the control plane machines at install time

8. **CachingType**:
   - When set, must be one of `None`, `ReadOnly` or `ReadWrite`
   - Left unset, it is defaulted to `None`. See **Disk Provisioning** below for why

9. **UltraSSD_LRS**:
   - `ultraSSDCapability` must be `Enabled`. The effective value is read from the pool, falling
     back to `platform.azure.defaultMachinePlatform` when the pool leaves it unset, so a value set
     on the pool wins over the default.
   - `cachingType` must be `None` or unset, since Azure cannot cache an `UltraSSD_LRS` disk. An
     unset value is defaulted to `None`, so only an explicitly set other value can conflict.

**Enforced whenever `diskSetup` is non-empty:**

10. **Correspondence with `diskSetup`**:
    - `len(diskSetup)` must not exceed `len(dataDisks)`, so `diskSetup` on a pool with no
      `dataDisks` is rejected
    - `diskSetup[i]`'s `platformDiskID` must equal `dataDisks[i]`'s `nameSuffix`. The match is
      **positional**

11. **Pool platform stanza**:
    - A pool with `diskSetup` entries must carry its own `platform.azure` stanza holding the
      `dataDisks` they pair with

#### Implementation Details

There are two consumers of `dataDisks`, and both are exercised by a normal install:

- **CAPI** (`pkg/asset/machines/azure/azuremachines.go`) builds the `AzureMachine` resources used to
  provision the control plane during bootstrap. `capz.DataDisk` is the install-config type, so these
  pass through unconverted.
- **MAPI** (`pkg/asset/machines/azure/machines.go`) builds the `Machine` and `MachineSet` resources
  that the cluster reconciles after bootstrap, including all compute machines. These require a
  conversion to `machineapi.DataDisk`.

The MAPI conversion maps each entry to a `machineapi.DataDisk` with the same `nameSuffix`,
`diskSizeGB`, `lun` and `cachingType`, and the `managedDisk.storageAccountType` and
`diskEncryptionSet`, whose ID must be non-empty.

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
- `pkg/types/azure/defaults/machinepool.go` - Defaults an omitted data disk `cachingType` to `None`
- `pkg/asset/machines/azure/machines.go` - Converts install-config data disks to Machine API format
- `pkg/asset/machines/azure/azuremachines.go` - Applies data disks to CAPI AzureMachine specs
- `pkg/asset/machines/util.go` - Builds the disk setup MachineConfig for a data disk, addressing
  it through its by-LUN udev link
- `pkg/types/azure/validation/featuregates.go` - Implements feature gate checking
- `pkg/types/validation/machinepools.go` - Cross-platform validation hooks

**Feature Gate:**

The `FeatureGateAzureMultiDisk` feature gate controls access to this functionality. The installer will validate that this feature gate is enabled when `dataDisks` or `diskSetup` are configured in the install-config.yaml.

**Disk Provisioning:**

- An omitted `cachingType` is defaulted to `None`. `None` is the only caching mode valid for every
  SKU and matches what the Machine API path effectively applies by default

**Storage Account Types:**

`managedDisk.storageAccountType` is checked at install time against the managed disk SKUs defined
by the Azure compute SDK.

`UltraSSD_LRS` additionally requires `ultraSSDCapability: Enabled`, which is what enables the
capability on the VM itself. That field is independent of `dataDisks` and defaults to `Disabled`.

### Risks and Mitigations

This feature allows administrators to add new disks at install time. The risks lie in
misconfigurations, which are largely prevented by the validations described in this enhancement
proposal.

### Drawbacks

**Install-time Only Configuration:**

Data disks can only be configured during cluster installation. Existing machines cannot have data disks added post-installation.

**Disk Setup Is Specified Separately:**

Provisioning a disk and setting it up are configured as two separate arrays, `dataDisks` and
`diskSetup`, that pair positionally, so users have to understand and configure the two together.
A disk with no `diskSetup` entry stays raw and needs a MachineConfig or another mechanism to be
used.

**Feature Gate Requirement:**

The feature requires enabling a feature gate, which adds an extra configuration step for users and may not be discoverable without reading documentation.

## Open Questions [optional]

None.

## Test Plan

**Unit Tests** (`pkg/types/azure/validation/machinepool_test.go`), implemented:

- `managedDisk` with an empty `storageAccountType` is rejected
- A `storageAccountType` that is not a known managed disk SKU is rejected, and a known one accepted
- `managedDisk.securityProfile` is rejected
- `dataDisks` in `defaultMachinePlatform` is rejected
- `dataDisks` on `AzureStackCloud` is rejected
- Nil LUN, LUN outside 0-63, duplicate LUNs, and `diskSizeGB <= 0` are each rejected, with or
  without `diskSetup`
- A control plane `diskSizeGB` outside the 4-32767 GB range is rejected
- A `nameSuffix` that is empty, longer than 15 characters, or contains characters Azure does not
  admit in a disk name is rejected
- A duplicate `nameSuffix` is rejected
- An unsupported `cachingType` is rejected, and caching is allowed on a non-`UltraSSD_LRS` disk
- An `UltraSSD_LRS` disk is rejected without `ultraSSDCapability` set, or with it `Disabled`, and
  accepted with it `Enabled` on the pool or only on `defaultMachinePlatform`. A pool that sets
  `Disabled` overrides an `Enabled` default.
- An `UltraSSD_LRS` disk with a `cachingType` other than `None` is rejected, and accepted with
  `None` or with it left unset
- With `diskSetup` present: `len(diskSetup) > len(dataDisks)` is rejected, including when there are
  no `dataDisks` at all, and a `platformDiskID` that does not match the `nameSuffix` at the same
  index is rejected
- A pool with `diskSetup` entries but no `platform.azure` stanza is rejected
- Valid configurations pass
- `dataDisks` set on `defaultMachinePlatform`, `controlPlane` or `compute` without
  `FeatureGateAzureMultiDisk` is rejected
- `dataDisks` are converted to `machineapi.DataDisk` with `DeletionPolicy: Delete`
- `dataDisks` are propagated to the `AzureMachine` spec

**Unit Tests** (`pkg/types/azure/defaults/machinepool_test.go`), implemented:

- An omitted `cachingType` is defaulted to `None` in a per-pool copy, after
  `defaultMachinePlatform` inheritance is resolved, so the default does not leak into the
  default machine platform and an explicitly set value is never revisited

**Asset Generation Tests** (`pkg/asset/machines/util_test.go`,
`pkg/asset/machines/azure/machines_test.go`, `pkg/asset/machines/worker_test.go`), implemented:

- The generated disk setup MachineConfig addresses an Azure data disk at
  `/dev/disk/azure/data/by-lun/<lun>`, the udev link that exists on both the SCSI and the NVMe
  controller
- The defaulted `cachingType: None` reaches both the `AzureMachine` spec and the Machine API
  provider spec
- A machine pool with `diskSetup` entries and no platform stanza is rejected with an error
  rather than panicking

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

**CI Jobs:**

The above is exercised by `e2e-azure-ovn-multidisk-techpreview`, defined for both
`openshift/installer` and `openshift/machine-config-operator`.

## Graduation Criteria

### Dev Preview -> Tech Preview

- [x] Installer allows configuration of data disks
- [x] CI job installing with data disks configured
- [x] Relative API stability
- [x] Sufficient unit test coverage

### Tech Preview -> GA

- [x] E2E tests covering control plane **and** compute nodes with data disks
      (job `e2e-azure-ovn-multidisk-techpreview`)
- [x] E2E tests covering the data disks end to end: disk provisioning, and the partitioning and
      formatting of the disks, for both an `etcd` and a `user-defined` data disk
- [x] Sufficient time for feedback. The gate has been in TechPreview since 4.20.
- [ ] Available by default -- requires moving `FeatureGateAzureMultiDisk` to
      `enable(inDefault(), inOKD(), inTechPreviewNoUpgrade(), inDevPreviewNoUpgrade())` in
      `openshift/api`. The gate also protects `diskSetup` on Azure, so promoting it on its own
      delivers Azure multi-disk end to end. `MultiDiskSetup` does not need to move with it.
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
validation. Errors about an individual disk carry its index and the install-config spelling of the
field (`dataDisks[i].nameSuffix`, `dataDisks[i].lun`), so with several disks the message names the
one at fault. The messages emitted today are:

- `"not allowed on default machine pool, use dataDisks compute and controlPlane only"`
- `"data disks are not supported on AzureStackCloud"`
- `"storageAccountType is required when managedDisk is specified"`
- `Unsupported value: "<value>": supported values: "Premium_LRS", ...` on `storageAccountType` -
  not a managed disk SKU the Azure SDK defines
- `Unsupported value: "<value>": supported values: "None", "ReadOnly", "ReadWrite"` on
  `cachingType`
- `"data disk security profiles are not yet supported by the Machine API and would be silently
  ignored"`
- `"nameSuffix is required"`
- `"may only contain alphanumerics, '_', '.' and '-', and must end with an alphanumeric or '_'"`
  on `nameSuffix`
- `Too long: may not be more than 15 bytes` on `nameSuffix`
- `Duplicate value: "<nameSuffix>"` - two disks in the pool share a `nameSuffix`
- `"<nameSuffix>" must have lun id` - LUN omitted
- `"<nameSuffix>" must have lun id between 0 and 63` - LUN out of range
- `Duplicate value: <lun>` on `lun` - duplicate LUN
- `"diskSizeGB must be greater than zero"`
- `"must be between 4 and 32767 for control plane data disks"` on `diskSizeGB`
- `"diskSetup entries must be paired with data disks: set dataDisks on this machine pool's
  platform.azure stanza"`
- `"each diskSetup entry configures the dataDisk at the same index; diskSetup length is N but
  dataDisks length is M, so the extra diskSetup entries would be silently ignored"`
- `does not match etcd|swap|user defined platformDiskID "<id>"` on `nameSuffix` - a `diskSetup`
  entry and the `dataDisks` entry at the same index disagree

If `FeatureGateAzureMultiDisk` is not enabled, validation fails with a feature gate error naming the
`dataDisks` field.

**Verifying Data Disks on Running VMs:**

The most reliable in-guest check is the Azure udev symlink tree, which is keyed by LUN and therefore
maps directly back to the install-config:

```console
$ ls -l /dev/disk/azure/data/by-lun/
lrwxrwxrwx. 1 root root 12 ... 0 -> ../../../../sdc
lrwxrwxrwx. 1 root root 12 ... 1 -> ../../../../sdd
```

The links are created by the `azure-vm-utils` udev rules for data disks on both the SCSI and the
NVMe controller, so the check works on NVMe-only instance types too; there the links resolve to
NVMe namespaces rather than `/dev/sd*` devices.

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

Automatically format and mount data disks during VM provisioning, without the user specifying a layout. This was not implemented because:
- Different use cases require different filesystem types and mount options
- This is better handled by MachineConfig which provides more flexibility
- Separation of concerns: installer provisions infrastructure, MachineConfig configures OS

**Alternative 3: Support in defaultMachinePlatform**

Allow data disk configuration in platform.azure.defaultMachinePlatform. This was not implemented to maintain consistency with how other machine-specific configurations are handled and to avoid complexity in inheritance and override behavior.

## Infrastructure Needed [optional]

None. This feature uses existing Azure infrastructure and does not require new CI resources or testing infrastructure beyond standard E2E test clusters.



