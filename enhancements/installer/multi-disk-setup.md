---
title: multi-disk-setup
authors:
  - "@jcpowermac"
reviewers:
  - "@vr4manta"
  - "@patrickdillon"
  - "@yuqi-zhang"
approvers:
  - "@patrickdillon"
api-approvers:
  - None
creation-date: 2025-05-16
last-updated: 2026-09-28
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2046
see-also:
  - "/enhancements/installer/azure-data-disk.md"
---

# Multi-Disk Setup

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

This enhancement enables the installer to configure multiple disks for specialized purposes (etcd, swap, user-defined storage) by generating MachineConfig resources that create the necessary ignition configurations to partition, format, and mount these disks at cluster installation time.

## Summary

This enhancement proposes to extend the installer to support configurations for multiple disks.
This will allow users to define specific roles for additional disks, such as for `etcd` data, `swap` space, or other `user-defined` mount points.
The installer will generate the necessary ignition configuration to partition, format, and mount these disks as specified by the user during the installation process.

## Motivation

Currently, the installer has limited explicit support for configuring multiple disks with distinct roles during the initial setup.
Users often require dedicated disks for performance, capacity, or specific application needs (e.g., separating etcd I/O, providing swap).
This enhancement aims to provide a streamlined and supported way to declare these multi-disk configurations directly through the installer, simplifying the deployment process for such scenarios.

### User Stories

* As a cluster administrator, I want to specify a dedicated disk for etcd during installation, so that I can ensure etcd has isolated I/O performance and dedicated storage.
* As a cluster administrator, I want to configure a dedicated swap disk during installation, so that I can provide swap space for nodes that require it.
* As a cluster administrator, I want to define custom mount points on additional disks for specific application data or utilities during installation, so that I can prepare nodes with pre-configured storage layouts.
* As an installer developer, I want a clear API for specifying additional disks and their purposes on control plane and compute nodes, so that I can reliably generate the corresponding ignition configurations.

### Goals

* Enable the installer to accept configurations for multiple disks on control plane and compute nodes.
* Define clear types for these additional disks (e.g., Etcd, Swap, UserDefined).
* Generate ignition configurations that create the necessary partitions, file systems (e.g., XFS), and systemd mount units for the specified disks.
* Ensure the initial implementation is isolated to the installer and its ignition generation capabilities.

### Non-Goals

* Dynamic disk management post-installation (this will be handled by storage operators or manual intervention).
* Complex RAID configurations via the installer (simple disk partitioning and formatting is the focus).
* Changes to how the operating system image itself is deployed to the primary disk.
* Making swap available to pods. `type: swap` provisions and activates swap at the node level only. See **Constraints** below.

## Proposal

The core of this proposal is to introduce new structures within the machine pool API that allow users to declare additional disks and their intended use. The installer will then interpret these declarations to generate the appropriate `storage` and `systemd` sections in the ignition config.

### Workflow Description

**cluster administrator** is a user responsible for installing and configuring an OpenShift cluster.

1. The cluster administrator creates or edits the install-config.yaml
2. For platforms that support additional disks (currently Azure and vSphere), the administrator:
   - Configures platform-specific disks (e.g., Azure `dataDisks`)
   - Adds `diskSetup` entries to specify how each disk should be configured
3. For each disk in `diskSetup`, the administrator specifies:
   - `type`: The disk's purpose (etcd, swap, or user-defined)
   - `platformDiskID`: Platform-specific identifier matching a provisioned disk
   - `mountPath`: (user-defined only) Where to mount the filesystem
4. The administrator runs `openshift-install create manifests`
5. The installer:
   - Validates the disk setup configuration
   - Matches `platformDiskID` to actual platform disks (e.g., Azure DataDisk by nameSuffix)
   - Resolves platform-specific device paths
   - Generates MachineConfig resources containing ignition configurations
6. The administrator runs `openshift-install create cluster`
7. During cluster provisioning:
   - Platform provisions VMs with attached disks (e.g., Azure attaches DataDisks)
   - MachineConfig resources are applied via MCO
   - Ignition processes the disk configurations, partitioning, formatting, and mounting disks
8. Nodes boot with disks properly configured for their designated purposes

#### Example Configuration (Azure)

Example install-config.yaml with etcd on dedicated disk:

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
      # First, configure Azure DataDisks
      dataDisks:
      - lun: 0
        nameSuffix: etcd
        diskSizeGB: 512
        cachingType: ReadOnly
        managedDisk:
          storageAccountType: Premium_LRS
  # Then, configure how to use those disks
  diskSetup:
  - type: etcd
    etcd:
      platformDiskID: etcd  # Matches nameSuffix above
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
      - lun: 1
        nameSuffix: swapspace
        diskSizeGB: 64
  diskSetup:
  - type: user-defined
    userDefined:
      platformDiskID: containers  # Matches nameSuffix
      mountPath: /var/lib/containers
  - type: swap
    swap:
      platformDiskID: swapspace  # Matches nameSuffix
```

**Key Points:**
- `dataDisks` provisions the Azure managed disks
- `diskSetup` configures how those disks are partitioned, formatted, and mounted
- `platformDiskID` in `diskSetup` must match `nameSuffix` in `dataDisks`
- The installer resolves Azure LUN to device path `/dev/disk/azure/scsi1/lun{LUN}`

### API Extensions

This enhancement adds a new `diskSetup` field to the installer's MachinePool API, which is **platform-generic** but requires platform-specific implementation for disk identification and provisioning.

#### Install-Config API Changes

The following types are added to support disk setup configuration:

```go
// DiskType defines the purpose/role of an additional disk
type DiskType string

const (
    Etcd        DiskType = "etcd"         // Dedicated etcd storage
    Swap        DiskType = "swap"         // Swap space
    UserDefined DiskType = "user-defined" // Custom mount point
)

// Disk represents a disk setup configuration
type Disk struct {
    // Type specifies the disk's purpose
    // Required field.
    Type DiskType `json:"type"`

    // UserDefined configuration for user-defined disks
    // Required when Type is "user-defined"
    UserDefined *DiskUserDefined `json:"userDefined,omitempty"`

    // Etcd configuration for etcd disks
    // Required when Type is "etcd"
    Etcd *DiskEtcd `json:"etcd,omitempty"`

    // Swap configuration for swap disks
    // Required when Type is "swap"
    Swap *DiskSwap `json:"swap,omitempty"`
}

// DiskUserDefined defines configuration for a user-defined disk
type DiskUserDefined struct {
    // PlatformDiskID identifies the disk on the platform
    // Platform-specific format (e.g., Azure: "etcd", GCP: disk name, AWS: device name)
    // Required field.
    PlatformDiskID string `json:"platformDiskID"`

    // MountPath specifies where to mount the filesystem
    // Required field.
    MountPath string `json:"mountPath"`
}

// DiskSwap defines configuration for a swap disk
type DiskSwap struct {
    // PlatformDiskID identifies the disk on the platform
    // Required field.
    PlatformDiskID string `json:"platformDiskID"`
}

// DiskEtcd defines configuration for an etcd disk
type DiskEtcd struct {
    // PlatformDiskID identifies the disk on the platform
    // Required field.
    PlatformDiskID string `json:"platformDiskID"`
    // Note: MountPath is implicitly /var/lib/etcd
}

// MachinePool enhanced with DiskSetup
type MachinePool struct {
    // ... existing fields ...

    // DiskSetup specifies configurations for additional disks
    // Optional field.
    DiskSetup []Disk `json:"diskSetup,omitempty"`
}
```

#### Validation Rules

The following validation rules are enforced:

1. **Type Field**:
   - Must be one of: `etcd`, `swap`, `user-defined`

2. **Type-specific Configuration**:
   - When `type: etcd`, the `etcd` field is required
   - When `type: swap`, the `swap` field is required
   - When `type: user-defined`, the `userDefined` field is required

3. **PlatformDiskID**:
   - Identifies which platform disk the entry applies to. The format is platform-specific: on
     Azure it is the `dataDisks[].nameSuffix`, on vSphere the `dataDisks[].name`.

4. **Relationship with Platform DataDisks**:
   - **Azure**: `diskSetup[i]` is matched to `dataDisks[i]` **positionally**, and that entry's
     `platformDiskID` must equal the `nameSuffix` of the disk at the same index. The number of
     `diskSetup` entries must not exceed the number of `dataDisks`. Both checks only run when
     `dataDisks` and `diskSetup` are each non-empty.
   - **vSphere**: `platformDiskID` is resolved **by name** against `dataDisks[].name`.
   - Azure Stack Cloud does not support data disks or disk setup

5. **Uniqueness Constraints**:
   - Only one `etcd` type disk per machine pool
   - Only one `swap` type disk per machine pool
   - Multiple `user-defined` disks are allowed

6. **Role Restrictions**:
   - `type: etcd` is only permitted on the `master` pool (`cannot specify etcd on worker machine pools`)
   - `type: swap` is rejected on the `master` pool (`swap is unsupported on control plane nodes`)

7. **MountPath** (user-defined only):
   - Should be an absolute path that does not collide with an existing system mount point.

8. **PlatformDiskID length** (user-defined only):
   - At most 12 characters, because the value becomes the GPT partition label and is reused to
     build the MachineConfig name.


### MachineConfig Generation

The installer generates **MachineConfig** resources that contain the ignition configuration for disk setup. This approach allows the disk setup to be applied via the MachineConfigOperator (MCO) during node bootstrapping.

#### Implementation Architecture

```go
// NodeDiskSetup, in pkg/asset/machines/util.go, derives the mount path, the label and the
// device path, then hands off to ForDiskSetup. dataDisk is the platform disk that master.go
// or worker.go already selected for this entry; see Platform-Specific Disk Mapping below.
func NodeDiskSetup(installConfig *installconfig.InstallConfig, role string,
                   diskSetup types.Disk, dataDisk any) (*mcfgv1.MachineConfig, error) {

    // Mount path and label are fixed per disk type. Only user-defined disks carry a
    // caller-supplied label, taken from the platformDiskID.
    var path string
    label := string(diskSetup.Type)
    switch diskSetup.Type {
    case types.Etcd:
        path = "/var/lib/etcd"
    case types.Swap:
        path = ""
    case types.UserDefined:
        path = diskSetup.UserDefined.MountPath
        label = diskSetup.UserDefined.PlatformDiskID
    }

    // The device path is platform-specific.
    switch installConfig.Config.Platform.Name() {
    case azuretypes.Name:
        if installConfig.Config.Enabled(features.FeatureGateAzureMultiDisk) {
            azureDataDisk := dataDisk.(capzv1beta1.DataDisk)
            device := fmt.Sprintf("/dev/disk/azure/scsi1/lun%d", *azureDataDisk.Lun)
            return machineconfig.ForDiskSetup(role, device, label, path, diskSetup.Type)
        }
        // Gate disabled: no MachineConfig is generated.
        return nil, nil

    case vspheretypes.Name:
        vsphereDataDisk := dataDisk.(vsphere.DiskInfo)
        device := fmt.Sprintf(VsphereScsiByPath, vsphereDataDisk.Index+1)
        return machineconfig.ForDiskSetup(role, device, label, path, diskSetup.Type)
    case awstypes.Name:
        // AWS: map EBS volume to device
        // Implementation TBD
    case gcptypes.Name:
        // GCP: map persistent disk to device
        // Implementation TBD
    default:
        return nil, errors.Errorf("unsupported platform %q",
                                  installConfig.Config.Platform.Name())
    }
}

// ForDiskSetup, in pkg/asset/machines/machineconfig/disks.go, builds the ignition config and
// wraps it in a MachineConfig named 01-disk-setup-<label>-<role>.
func ForDiskSetup(role, device, label, path string,
                  diskType types.DiskType) (*mcfgv1.MachineConfig, error) {

    ignConfig := igntypes.Config{
        Ignition: igntypes.Ignition{
            Version: igntypes.MaxVersion.String(),
        },
    }

    // The label becomes a GPT partition label and part of the MachineConfig name, so any
    // non-alphanumeric characters are stripped out of it.
    label = regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(label, "")

    // Render the systemd unit from one of the two templates, then build the ignition
    // config around it.
    var templateStringToParse string
    switch diskType {
    case types.Etcd, types.UserDefined:
        templateStringToParse = diskMountUnit
    case types.Swap:
        templateStringToParse = swapMountUnit
    }
    // ... template.Execute(&dmu, diskMount{MountPath: path, Label: label}) ...
    units := dmu.String()

    var rawExt runtime.RawExtension
    switch diskType {
    case types.Etcd, types.UserDefined:
        rawExt, err = getDiskIgnition(ignConfig, device, label, path, units)
    case types.Swap:
        rawExt, err = getSwapIgnition(ignConfig, device, label, units)
    }

    return &mcfgv1.MachineConfig{
        ObjectMeta: metav1.ObjectMeta{
            Name: fmt.Sprintf("01-disk-setup-%s-%s", strings.ToLower(label), role),
            Labels: map[string]string{
                "machineconfiguration.openshift.io/role": role,
            },
        },
        Spec: mcfgv1.MachineConfigSpec{Config: rawExt},
    }, nil
}
```

#### Ignition Configuration Details

For **etcd** and **user-defined** disks:

```go
func getDiskIgnition(ignConfig igntypes.Config, device, label, path, units string) (runtime.RawExtension, error) {
    // 1. Create partition
    ignConfig.Storage.Disks = append(ignConfig.Storage.Disks, igntypes.Disk{
        Device: device,
        Partitions: []igntypes.Partition{{
            Label:    ptr.To(label),
            StartMiB: ptr.To(0),  // Start at beginning
            SizeMiB:  ptr.To(0),  // Use entire disk
        }},
        WipeTable: ptr.To(true),
    })

    // 2. Create filesystem
    ignConfig.Storage.Filesystems = append(ignConfig.Storage.Filesystems, igntypes.Filesystem{
        Device:         fmt.Sprintf("/dev/disk/by-partlabel/%s", label),
        Format:         ptr.To("xfs"),
        Label:          ptr.To(label),
        MountOptions:   []igntypes.MountOption{"defaults", "prjquota"},
        Path:           ptr.To(path),
        WipeFilesystem: ptr.To(true),
    })

    // 3. Create systemd mount unit, named after the mount path
    unitName := strings.ReplaceAll(strings.Trim(path, "/"), "/", "-")
    ignConfig.Systemd.Units = append(ignConfig.Systemd.Units, igntypes.Unit{
        Name:     fmt.Sprintf("%s.mount", unitName),
        Enabled:  ptr.To(true),
        Contents: &units,
    })
    return ignition.ConvertToRawExtension(ignConfig)
}
```

The mount unit itself is rendered from a fixed template:

```systemd
[Unit]
Requires=systemd-fsck@dev-disk-by\x2dpartlabel-{{.Label}}.service
After=systemd-fsck@dev-disk-by\x2dpartlabel-{{.Label}}.service

[Mount]
Where={{.MountPath}}
What=/dev/disk/by-partlabel/{{.Label}}
Type=xfs
Options=defaults,prjquota

[Install]
RequiredBy=local-fs.target
```

For **swap** disks:

```go
func getSwapIgnition(ignConfig igntypes.Config, device, label, units string) (runtime.RawExtension, error) {
    unitName := "dev-disk-by\\x2dpartlabel-swap.swap"

    // 1. Create partition, tagged with the GPT swap GUID
    ignConfig.Storage.Disks = append(ignConfig.Storage.Disks, igntypes.Disk{
        Device: device,
        Partitions: []igntypes.Partition{{
            Label:    ptr.To(label),
            StartMiB: ptr.To(0),
            SizeMiB:  ptr.To(0),
            GUID:     ptr.To("0657FD6D-A4AB-43C4-84E5-0933C84B4F4F"),
        }},
        WipeTable: ptr.To(true),
    })

    // 2. Format as swap
    ignConfig.Storage.Filesystems = append(ignConfig.Storage.Filesystems, igntypes.Filesystem{
        Device: fmt.Sprintf("/dev/disk/by-partlabel/%s", label),
        Format: ptr.To("swap"),
        Label:  ptr.To(label),
    })

    // 3. Create systemd swap unit
    ignConfig.Systemd.Units = append(ignConfig.Systemd.Units, igntypes.Unit{
        Name:     unitName,
        Enabled:  ptr.To(true),
        Contents: &units,
    })
    return ignition.ConvertToRawExtension(ignConfig)
}
```

The swap unit is rendered from its own template, which has no `[Unit]` section:

```systemd
[Swap]
What=/dev/disk/by-partlabel/{{.Label}}

[Install]
WantedBy=swap.target
```

#### Platform-Specific Disk Mapping

Mapping happens in two steps. `master.go` and `worker.go` select the platform disk that a
`diskSetup` entry refers to, and `NodeDiskSetup` in `pkg/asset/machines/util.go` turns that disk
into a device path and calls `ForDiskSetup`. How the disk is selected differs per platform.

**Azure:**

The `platformDiskID` references the `nameSuffix` of an Azure DataDisk, but the disk is picked
**positionally**, i.e. `diskSetup[i]` uses `dataDisks[i]`. The name equality is enforced separately,
by the Azure install-config validation described above.

```go
// pkg/asset/machines/master.go -- selection is by index
if i < len(azureControlPlaneMachinePool.DataDisks) {
    dataDisk = azureControlPlaneMachinePool.DataDisks[i]
}

// pkg/asset/machines/util.go -- the LUN gives the device path
device := fmt.Sprintf("/dev/disk/azure/scsi1/lun%d", *azureDataDisk.Lun)
return machineconfig.ForDiskSetup(role, device, label, path, diskSetup.Type)
```

**vSphere:**

The `platformDiskID` is resolved **by name** against `dataDisks[].name`, and the disk's index in
that list gives the SCSI unit number. If no disk matches, `dataDisk` stays nil and the entry is
skipped without an error.

```go
// pkg/asset/machines/master.go -- selection is by name
for index, disk := range vsphereControlPlaneMachinePool.DataDisks {
    if disk.Name == diskName {
        dataDisk = vsphere.DiskInfo{Index: index, Disk: disk}
        break
    }
}

// pkg/asset/machines/util.go -- the index gives the device path
device := fmt.Sprintf("/dev/disk/by-path/pci-0000:03:00.0-scsi-0:0:%d:0", vsphereDataDisk.Index+1)
return machineconfig.ForDiskSetup(role, device, label, path, diskSetup.Type)
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This feature does not apply to Hypershift / Hosted Control Planes.

#### Standalone Clusters

This feature applies to standalone OpenShift clusters. Disk setup can be configured for both control plane and compute nodes. Control plane nodes commonly use dedicated etcd disks for performance isolation. Compute nodes can use swap or user-defined disks for application workloads.

#### Single-node Deployments or MicroShift

This feature applies to single-node OpenShift deployments. The single node belongs to the `master` pool, so it can be configured with dedicated disks for etcd or user-defined purposes.

#### OpenShift Kubernetes Engine

This feature applies to OpenShift Kubernetes Engine.


### Implementation Details/Notes/Constraints

**Modified Files:**

The implementation spans several installer components:
- `pkg/types/machinepools.go` - Adds the `DiskSetup` field and the disk setup types to MachinePool
- `pkg/types/validation/machinepools.go` - Platform-agnostic disk setup validation
- `pkg/types/validation/featuregates.go` - Gates `diskSetup` behind `MultiDiskSetup`
- `pkg/types/azure/validation/machinepool.go` - Azure-specific validation
- `pkg/asset/machines/master.go`, `pkg/asset/machines/worker.go` - Select the platform disk for
  each `diskSetup` entry and add the resulting MachineConfig to the role's manifests
- `pkg/asset/machines/util.go` - Per-platform device path resolution (`NodeDiskSetup`)
- `pkg/asset/machines/machineconfig/disks.go` - MachineConfig and ignition generation
- `data/data/install.openshift.io_installconfigs.yaml` - CRD schema updates

**Platform Support:**

Currently implemented for:
- **Azure**: Full support with DataDisk integration
- **vSphere**: Full support with DataDisk integration

Planned for future implementation:
- **AWS**: EBS volume mapping
- **GCP**: Persistent disk mapping
- **Bare Metal**: Direct device path specification

**Disk Identification:**

- `PlatformDiskID` is **platform-specific**:
  - **Azure**: Must match the `nameSuffix` of the `dataDisks` entry at the same index
  - **vSphere**: Must match the `name` of one of the `dataDisks` entries
  - **Other platforms**: TBD based on platform disk naming conventions
- On Azure the installer validates that the reference exists; on vSphere it does not
- Device paths are resolved at MachineConfig generation time

**Partitioning:**

- Uses the entire disk for a single partition
- Partition starts at 0 MiB and uses full disk capacity (SizeMiB: 0)
- Partition table is wiped before creating new partition

**Filesystem:**

- **Etcd and user-defined disks**: XFS with mount options `defaults,prjquota`. Both share the same
  code path, so the filesystem type and the mount options are the same for either type and are not
  configurable.
- **Swap disks**: Formatted as swap space
- Filesystem is wiped before formatting (WipeFilesystem: true)

**MachineConfig Integration:**

- Each disk setup generates a separate MachineConfig resource
- MachineConfigs are labeled with role-specific labels for MCO targeting
- Ignition version 3.2.0 is used
- MachineConfigs are rendered during `create manifests` phase

**Error Handling:**

- On Azure, a `platformDiskID` that does not match the data disk at the same index fails
  install-config validation.
- If the platform disk is not attached at boot time, the mount will fail
- Failed mounts for critical disks (etcd) will prevent the node from becoming Ready
- Failed mounts for non-critical disks (user-defined, swap) may allow the node to become Ready but with degraded functionality

**Re-provisioning:**

- Ignition runs on first boot only, so disk setup is applied once per node
- The generated config sets `WipeTable: true`, and `WipeFilesystem: true` for etcd and
  user-defined disks. Any pre-existing content on the target disk is destroyed when the node is
  provisioned, which matters if the platform disk is reused rather than created fresh.

**Security:**

- Standard file permissions are applied by ignition
- SELinux contexts are set based on the mount point path
- For etcd disks at `/var/lib/etcd`, SELinux context is automatically correct
- For user-defined paths, administrators should verify appropriate SELinux policies

**Constraints:**

- Disk setup requires corresponding platform disks to be configured (e.g., Azure DataDisks)
- Number of disk setups cannot exceed number of platform disks
- Only one etcd disk and one swap disk per machine pool
- Azure Stack Cloud does not support disk setup
- Disks must be available at first boot (cannot be added post-installation via this mechanism)
- The generated MachineConfigs carry the literal role `master` or `worker`, and bind to the
  matching MachineConfigPool through the usual role label.
- Arbiter pools are not covered by either generation path, so no disk setup MachineConfigs are
  produced for them.
- `type: swap` is rejected on the control plane pool. On compute pools it provisions and activates
  swap at the node level, but the kubelet does not make that swap available to pods: the MCO
  kubelet templates set `memorySwap.swapBehavior: NoSwap` for every role, and neither
  `swapBehavior` nor `failSwapOn` can be overridden through a `KubeletConfig`, as the MCO rejects
  both. Control plane and arbiter nodes additionally run `failSwapOn: true`, which prevents the
  kubelet from starting at all if a swap device is active -- hence the outright rejection there.

### Risks and Mitigations

**Risk: Incorrect PlatformDiskID Configuration**

User provides an incorrect `platformDiskID` that doesn't match any configured platform disk.

*Mitigation:*
- On Azure the installer compares each `platformDiskID` against the `nameSuffix` of the data disk
  at the same index and fails install-config validation on a mismatch
- Clear error messages guide users to correct configuration
- Documentation provides platform-specific examples

**Risk: Disk Not Available at Boot**

The platform disk is configured but not successfully attached to the VM at boot time.

*Mitigation:*
- Platform provisioning errors will cause cluster installation to fail
- For critical mounts (etcd), node will fail health checks and not join cluster

**Risk: Conflicts with Existing Mounts**

User-defined mount paths conflict with system or other application mount points.

*Mitigation:*
- Documentation warns against using system paths
- Validation could reject common system paths (/etc, /usr, /bin, etc.)
- Encourage use of /var subdirectories or /mnt paths

**Risk: Platform-Specific Implementation Gaps**

Initial implementation covers Azure and vSphere; other platforms need separate implementations.

*Mitigation:*
- Design is platform-generic at the API level
- Clear platform support documentation
- Validation rejects unsupported platforms with helpful error messages

**Risk: Security and SELinux Issues**

Custom mount paths may have incorrect SELinux contexts or permissions.

*Mitigation:*
- Ignition applies contexts based on mount path patterns
- Documentation guides users on SELinux considerations
- Recommend using standard paths under /var where possible
- Security review of generated ignition configurations

**Risk: etcd Performance**

Misconfigured etcd disks (wrong storage type, caching) could degrade performance.

*Mitigation:*
- Documentation provides best practices (Premium_LRS, ReadOnly caching for Azure)
- Example configurations demonstrate optimal settings
- Performance team review of recommendations

### Drawbacks

**Install-Time Only Configuration:**

Disk setup can only be configured during cluster installation.

**Platform-Specific Mapping Complexity:**

Each platform requires custom logic to map `platformDiskID` to actual device paths. This increases implementation and testing burden for each platform.

**Limited Flexibility:**

- Only supports single partition per disk
- Fixed filesystem type (XFS for data, swap for swap)
- Cannot specify custom partition layouts or multiple partitions
- Cannot configure RAID

These limitations keep the initial implementation simple but may require future enhancements.

**Dependency on Platform Disk Configuration:**

Disk setup is not standalone - it requires platform-specific disk provisioning (e.g., Azure DataDisks). Users must understand both concepts and configure them correctly together.

**No Day 2 Management:**

Once configured, disk setup cannot be modified or removed without node reprovisioning. This is due to MCO rejecting day-2 changes to `storage.disks` and
`storage.filesystems` as irreconcilable.

## Alternatives (Not Implemented)

**Alternative 1: Day-2 Manual Configuration**

Users could manually partition and mount disks after the cluster is up using ssh access and manual disk management.

*Not implemented because:*
- Requires manual intervention on each node
- Error-prone and not scalable
- Critical mounts like etcd need to be present from first boot
- Inconsistent with declarative infrastructure-as-code principles


**Alternative 2: Extend MachineConfig Directly**

Users could create custom MachineConfigs with ignition disk configurations.

*Not implemented as the only option because:*
- Requires deep knowledge of ignition and MachineConfig
- Platform-specific device paths are not user-friendly
- No validation of platform disk availability
- Installer-provided abstraction is more user-friendly

However, this alternative *complements* this enhancement - advanced users can still create custom MachineConfigs for complex scenarios not covered by the diskSetup API.

**Alternative 3: Ignition Config Templates**

Provide ignition config templates that users can customize for their disk setup needs.

*Not implemented because:*
- Requires users to understand ignition format
- Platform-specific device mapping is complex
- No automated validation
- Less integrated with install-config.yaml workflow


## Open Questions [optional]

None. The implementation has been completed and deployed.


## Test Plan

**Unit Tests:**

- Validation logic for disk setup configuration:
  - Test type validation (etcd, swap, user-defined)
  - Test platformDiskID presence and matching
  - Test uniqueness constraints (only one etcd, one swap)
  - Test mountPath validation for user-defined disks
  - Test platform-specific validation (Azure DataDisk matching)

- MachineConfig generation:
  - Test ignition config generation for etcd disks
  - Test ignition config generation for swap disks
  - Test ignition config generation for user-defined disks
  - Test partition configuration
  - Test filesystem configuration
  - Test systemd mount unit generation
  - Test device path resolution for different platforms

**Integration Tests:**

- Install-config validation:
  - Create install-config.yaml with various disk setup configurations
  - Verify validation catches all error conditions
  - Verify valid configurations are accepted
  - Test Azure-specific DataDisk matching validation

- Manifest generation:
  - Run `create manifests` with disk setup configured
  - Verify MachineConfig resources are generated
  - Verify ignition configurations are correct
  - Verify role-specific labeling

**End-to-End Tests (Azure):**

- Cluster installation with etcd on dedicated disk:
  - Configure control plane with etcd disk setup
  - Verify cluster installs successfully
  - Verify `/var/lib/etcd` is mounted from dedicated disk
  - Verify etcd is using the dedicated disk
  - Verify partition and filesystem are correct (XFS, correct mount options)

- Cluster installation with swap disk:
  - Configure compute nodes with swap disk setup
  - Verify swap space is enabled
  - Verify swap is using the dedicated disk
  - Verify `swapon -s` shows the swap partition

- Cluster installation with user-defined disks:
  - Configure custom mount point (e.g., `/var/lib/containers`)
  - Verify mount is present and accessible
  - Verify partition and filesystem are correct

- Multi-disk configuration:
  - Configure multiple disk types on same machine pool
  - Verify all disks are correctly configured
  - Verify no conflicts or ordering issues

- Error cases:
  - Test with mismatched platformDiskID (validation should fail)
  - Test with missing DataDisk configuration (validation should fail)
  - Test with unsupported platform (validation should fail)

**Platform-Specific Tests:**

- Azure:
  - Test with different Azure storage account types
  - Test with different LUN assignments
  - Test Azure Stack Cloud rejection
  - Test with disk encryption

- Future platforms (AWS, GCP, Bare Metal):
  - Platform-specific device mapping tests
  - Platform-specific validation tests

**CI Jobs:**

- `e2e-azure-ovn-multidisk-techpreview` - optional presubmit on `openshift/installer` and
  `openshift/machine-config-operator`, installing with `dataDisks` and `diskSetup` on both pools
- `e2e-vsphere-ovn-disk-setup-techpreview` - the vSphere equivalent, on the same two repositories
- `e2e-azure-ovn-multidisk-techpreview-upgrade` - weekly periodic in `openshift/release`

The error cases are covered by unit tests.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Core functionality implemented for Azure platform:
  - Partitioning, formatting (XFS/swap), and mounting for etcd, swap, and user-defined disk types
  - PlatformDiskID matching with Azure DataDisks
  - MachineConfig generation with ignition configurations

- Installer validation:
  - Type validation
  - PlatformDiskID matching
  - Uniqueness constraints

- Basic testing:
  - Unit tests for validation and MachineConfig generation
  - E2E test for etcd on dedicated disk
  - E2E test for user-defined disk

- Documentation:
  - API reference documentation
  - Azure-specific examples
  - Installation guide with disk setup

### Tech Preview -> GA

- [x] Sufficient time for feedback from Tech Preview users. The gate has been in
      `TechPreviewNoUpgrade` since 4.20.
- [x] E2E coverage on Azure for both control plane and compute
      (`e2e-azure-ovn-multidisk-techpreview`)
- [x] E2E coverage on vSphere (`e2e-vsphere-ovn-disk-setup-techpreview`)
- [x] Upgrade coverage (`e2e-azure-ovn-multidisk-techpreview-upgrade`)
- [ ] Available by default, which requires moving `FeatureGateMultiDiskSetup` to
      `enable(inDefault(), inOKD(), inTechPreviewNoUpgrade(), inDevPreviewNoUpgrade())` in
      `openshift/api`. On Azure this must land together with the same change to
      `FeatureGateAzureMultiDisk`: the two gates cover disjoint fields -- `MultiDiskSetup` gates
      `diskSetup`, `AzureMultiDisk` gates the Azure `dataDisks` -- and `NodeDiskSetup` skips Azure
      disk setup entirely unless `AzureMultiDisk` is enabled. Graduating either gate on its own
      therefore leaves the feature unusable on Azure. vSphere `dataDisks` are not behind a gate,
      so vSphere needs only `MultiDiskSetup`.
- [ ] User-facing documentation in the OCP installation docs, including the `WipeTable` warning and
      the mount-path guidance

**For non-optional features moving to GA, the graduation criteria must include end to end tests.**

### Removing a deprecated feature

Not applicable for this initial enhancement.

## Upgrade / Downgrade Strategy

This feature does not impact the upgrade or downgrade process for existing clusters. Disk setup is configured at cluster installation time and becomes part of the MachineConfig for the role.

**Upgrade Scenarios:**

- Existing clusters without disk setup can continue to operate normally after upgrading to a version that includes this feature
- Disk setup is configured at install time only. The generated MachineConfigs target a role, so nodes added to an existing pool afterwards do pick them up at first boot, as long as their MachineSet attaches a matching data disk. Giving a pool a *different* layout after installation is not possible: the MCO rejects day-2 changes to the Ignition `storage.disks` and `storage.filesystems` sections
- Existing nodes with disk setup configurations remain unchanged during cluster upgrades

**Downgrade Scenarios:**

- Downgrading the installer to a version without disk setup support will not affect already-provisioned nodes
- New installations with the downgraded installer will not support disk setup
- Existing MachineConfigs with disk setup remain on the cluster but new nodes cannot be provisioned with disk setup using the downgraded installer

**Important Considerations:**

- Disk setup is an installation-time feature, not a runtime feature
- Once nodes are provisioned with disk setup, the configuration persists regardless of installer version
- No migration or cleanup is required during upgrades or downgrades

## Version Skew Strategy

This feature does not introduce version skew concerns between cluster components.

**Installer and OS:**

- Uses Ignition v3.2.0, which is supported by RHCOS/FCOS
- Standard ignition disk, filesystem, and systemd unit configurations
- No custom or experimental ignition features required

**MachineConfig and MCO:**

- MachineConfigs are standard resources compatible with existing MCO versions
- No changes to MCO required
- MCO processes ignition configurations as normal

**No Runtime Dependencies:**

- Disk setup occurs during node bootstrapping via ignition
- No ongoing coordination between components required
- No version-dependent APIs or protocols

## Operational Aspects of API Extensions

This enhancement does not introduce API extensions in the Kubernetes API sense (no CRDs, admission webhooks, or aggregated API servers). The changes are limited to the installer's install-config.yaml schema.

The installer generates standard MachineConfig resources that contain ignition configurations. These MachineConfigs are processed by the existing MachineConfigOperator (MCO) without any modifications to the MCO.

**Operational Impact:**

- MachineConfigs are created during `create manifests` phase
- No ongoing reconciliation or operators required
- No new metrics, alerts, or monitoring needed specifically for disk setup
- Standard MachineConfig troubleshooting procedures apply

## Support Procedures

**Detecting Configuration Issues:**

**At Installation Time:**

If the disk setup configuration is invalid, `openshift-install` fails during install-config
validation. The messages emitted today are:

- `etcd configuration must be created`, `swap configuration must be created` and
  `userDefined configuration must be created` - the `type` was set but the matching block is absent
- `cannot be longer than 12 characters` - `userDefined.platformDiskID` over the limit
- `cannot specify etcd on worker machine pools` - `type: etcd` outside the control plane pool
- `swap is unsupported on control plane nodes` - `type: swap` on the control plane pool
- `Too many: 2: must have at most 1 item` - more than one `etcd` or more than one `swap` entry
- `does not match etcd PlatformDiskID "<id>"`, and the equivalent `swap` and `user defined`
  variants - Azure, the `platformDiskID` is not the `nameSuffix` of the data disk at the same index
- `Too long: may not be more than <n> bytes` - Azure, more `diskSetup` entries than `dataDisks`.
  The "bytes" wording comes from the generic `field.TooLong` helper; the number is a disk count.
- `data disks are not supported on AzureStackCloud` - Azure Stack Cloud limitation

**At Runtime:**

If a disk fails to mount during node bootstrapping:

1. Check node journal logs: `journalctl -u var-lib-etcd.mount` (or appropriate unit name)
2. Check ignition logs: `journalctl -u ignition-*`
3. Verify disk is attached: `lsblk`
4. Check partition labels: `ls -l /dev/disk/by-partlabel/`

**Verifying Disk Setup on Running Nodes:**

To verify disk setup is correctly applied:

```bash
# List all mounts
mount | grep -E '(etcd|swap|containers)'

# Check swap status
swapon -s

# Verify partition configuration
lsblk -f

# Check filesystem type and options
findmnt /var/lib/etcd

# View MachineConfig on the cluster
oc get machineconfig | grep disk

# Check MCO application status
oc get mcp
```

**Common Issues:**

**Issue: Disk Not Mounted**

*Symptoms:* Mount point doesn't exist, filesystem not available

*Diagnosis:*
1. Check if platform disk was attached: `lsblk`
2. Check systemd mount unit status: `systemctl status var-lib-etcd.mount`
3. Check ignition logs for partition/filesystem creation

*Resolution:*
- If disk wasn't attached: Check platform configuration (e.g., Azure DataDisk)
- If mount unit failed: Check journalctl for specific error
- If partition missing: Node may need to be reprovisioned

**Issue: Wrong PlatformDiskID**

*Symptoms:* Validation fails with platformDiskID mismatch error

*Diagnosis:* Review install-config.yaml, verify platformDiskID matches platform disk identifier

*Resolution:*
- For Azure: Ensure platformDiskID matches DataDisk nameSuffix
- Correct the install-config.yaml
- Re-run installer validation

**Issue: etcd Performance Problems**

*Symptoms:* High etcd latency, slow cluster operations

*Diagnosis:*
1. Verify etcd is using dedicated disk: `findmnt /var/lib/etcd`
2. Check disk I/O: `iostat -x 1`
3. Review Azure storage type: Should be Premium_LRS

*Resolution:*
- Ensure correct storage account type (Premium_LRS for production)
- Verify caching type (ReadOnly recommended)
- Check for other I/O contention on the disk

**Support Escalation:**

- Installer team: Installation and validation issues
- MCO team: MachineConfig application issues
- Etcd team: Etcd-specific performance or functionality issues
- Platform team (Azure/AWS/GCP): Platform disk provisioning issues


## Infrastructure Needed [optional]

**CI Infrastructure:**

- Azure CI environments must support provisioning VMs with multiple data disks
- CI jobs need permissions to create and attach managed disks
- Test clusters should include configurations with:
  - Control plane nodes with etcd disks
  - Worker nodes with user-defined disks
  - Mixed configurations

**Test Resources:**

- Additional Azure resources for data disks in CI subscriptions
- Cost considerations for Premium_LRS disks in testing
- Cleanup automation to remove test disks after job completion

**Documentation Infrastructure:**

- Examples repository with sample install-config.yaml files
- Documentation for each supported platform (currently Azure and vSphere)
- Troubleshooting guides and runbooks

**No Special Infrastructure Required:**

- No new clusters or dedicated test environments needed beyond standard CI
- Uses existing Azure CI infrastructure with additional disk provisioning
- No new monitoring or observability infrastructure required