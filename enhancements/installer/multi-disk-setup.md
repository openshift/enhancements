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
last-updated: 2025-05-16
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2046
see-also:
  - "/enhancements/installer/azure-data-disk.md"
---

# Multi-Disk Setup

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

This enhancement enables the installer to configure multiple disks for specialized purposes (etcd, swap, user-defined storage) by generating MachineConfig resources that create the necessary ignition configurations to partition, format, and mount these disks at cluster installation time.

## Summary

This enhancement proposes to extend the installer to support configurations for multiple disks. This will allow users to define specific roles for additional disks, such as for `etcd` data, `swap` space, or other `user-defined` mount points. The installer will generate the necessary ignition configuration to partition, format, and mount these disks as specified by the user during the installation process.

## Motivation

Currently, the installer has limited explicit support for configuring multiple disks with distinct roles during the initial setup. Users often require dedicated disks for performance, capacity, or specific application needs (e.g., separating etcd I/O, providing swap). This enhancement aims to provide a streamlined and supported way to declare these multi-disk configurations directly through the installer, simplifying the deployment process for such scenarios.

### User Stories

* As a cluster administrator, I want to specify a dedicated disk for etcd during installation, so that I can ensure etcd has isolated I/O performance and dedicated storage.
* As a cluster administrator, I want to configure a dedicated swap disk during installation, so that I can provide swap space for nodes that require it.
* As a cluster administrator, I want to define custom mount points on additional disks for specific application data or utilities during installation, so that I can prepare nodes with pre-configured storage layouts.
* As an installer developer, I want a clear API within the machine pool definition to specify additional disks and their purposes, so that I can reliably generate the corresponding ignition configurations.

### Goals

* Enable the installer to accept configurations for multiple disks per machine pool.
* Define clear types for these additional disks (e.g., Etcd, Swap, UserDefined).
* Generate ignition configurations that create the necessary partitions, file systems (e.g., XFS, ext4), and systemd mount units for the specified disks.
* Ensure the initial implementation is isolated to the installer and its ignition generation capabilities.

### Non-Goals

* Dynamic disk management post-installation (this will be handled by storage operators or manual intervention).
* Complex RAID configurations via the installer (simple disk partitioning and formatting is the focus).
* Changes to how the operating system image itself is deployed to the primary disk.

## Proposal

The core of this proposal is to introduce new structures within the machine pool API that allow users to declare additional disks and their intended use. The installer will then interpret these declarations to generate the appropriate `storage` and `systemd` sections in the ignition config.

### Workflow Description

**cluster administrator** is a user responsible for installing and configuring an OpenShift cluster.

1. The cluster administrator creates or edits the install-config.yaml
2. For platforms that support additional disks (currently Azure), the administrator:
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
   - Required, must be one of: `etcd`, `swap`, `user-defined`

2. **Type-specific Configuration**:
   - When `type: etcd`, the `etcd` field is required
   - When `type: swap`, the `swap` field is required
   - When `type: user-defined`, the `userDefined` field is required

3. **PlatformDiskID**:
   - Required for all disk types
   - Format is platform-specific
   - Must reference an available disk on the platform

4. **Relationship with Platform DataDisks** (Azure example):
   - The number of `diskSetup` entries must not exceed the number of `dataDisks` configured
   - Each `platformDiskID` must match a corresponding platform disk identifier (e.g., Azure DataDisk `nameSuffix`)
   - Azure Stack Cloud does not support data disks or disk setup

5. **Uniqueness Constraints**:
   - Only one `etcd` type disk per machine pool
   - Only one `swap` type disk per machine pool
   - Multiple `user-defined` disks are allowed

6. **MountPath** (user-defined only):
   - Required for user-defined disks
   - Must be a valid absolute path
   - Should not conflict with system mount points


### MachineConfig Generation

The installer generates **MachineConfig** resources that contain the ignition configuration for disk setup. This approach allows the disk setup to be applied via the MachineConfigOperator (MCO) during node bootstrapping.

#### Implementation Architecture

```go
// NodeDiskSetup generates a MachineConfig for a disk setup configuration
func NodeDiskSetup(installConfig types.InstallConfig, role string,
                   diskSetup types.Disk, platformDisk interface{}) (*mcfgv1.MachineConfig, error) {

    // Platform-specific disk device mapping
    device := ""
    switch installConfig.Platform.Name() {
    case azuretypes.Name:
        // Azure: map DataDisk LUN to device path
        dataDisk := platformDisk.(azure.DataDisk)
        device = fmt.Sprintf("/dev/disk/azure/scsi1/lun%d", *dataDisk.Lun)

    case awstypes.Name:
        // AWS: map EBS volume to device
        // Implementation TBD

    case gcptypes.Name:
        // GCP: map persistent disk to device
        // Implementation TBD

    default:
        return nil, errors.Errorf("platform %s does not support disk setup",
                                  installConfig.Platform.Name())
    }

    // Generate MachineConfig based on disk type
    return ForDiskSetup(role, device, diskSetup.Type, diskSetup)
}

// ForDiskSetup creates a MachineConfig with ignition configuration
func ForDiskSetup(role, device string, diskType types.DiskType,
                 disk types.Disk) (*mcfgv1.MachineConfig, error) {

    ignitionConfig := igntypes.Config{
        Ignition: igntypes.Ignition{
            Version: "3.2.0",
        },
    }

    switch diskType {
    case types.Etcd:
        label := "etcddisk"
        mountPath := "/var/lib/etcd"
        addFilesystemSetup(&ignitionConfig, device, label, mountPath, "xfs",
                          []string{"defaults", "prjquota"})

    case types.Swap:
        addSwapSetup(&ignitionConfig, device)

    case types.UserDefined:
        label := sanitizeLabel(disk.UserDefined.MountPath)
        addFilesystemSetup(&ignitionConfig, device, label,
                          disk.UserDefined.MountPath, "xfs",
                          []string{"defaults"})
    }

    // Convert ignition config to MachineConfig
    return machineConfigFromIgnition(role, diskType, ignitionConfig)
}
```

#### Ignition Configuration Details

For **etcd** and **user-defined** disks:

```go
func addFilesystemSetup(config *igntypes.Config, device, label, mountPath, fsType string,
                       mountOptions []string) {
    // 1. Create partition
    config.Storage.Disks = append(config.Storage.Disks, igntypes.Disk{
        Device: device,
        Partitions: []igntypes.Partition{{
            Label:    ptr.To(label),
            StartMiB: ptr.To(0),  // Start at beginning
            SizeMiB:  ptr.To(0),  // Use entire disk
        }},
        WipeTable: ptr.To(true),
    })

    // 2. Create filesystem
    config.Storage.Filesystems = append(config.Storage.Filesystems, igntypes.Filesystem{
        Device:         fmt.Sprintf("/dev/disk/by-partlabel/%s", label),
        Format:         ptr.To(fsType),
        Label:          ptr.To(label + "part"),
        MountOptions:   mountOptions,
        Path:           ptr.To(mountPath),
        WipeFilesystem: ptr.To(true),
    })

    // 3. Create systemd mount unit
    unitName := pathToUnitName(mountPath) + ".mount"
    config.Systemd.Units = append(config.Systemd.Units, igntypes.Unit{
        Name:    unitName,
        Enabled: ptr.To(true),
        Contents: ptr.To(generateMountUnit(mountPath, label, fsType, mountOptions)),
    })
}
```

For **swap** disks:

```go
func addSwapSetup(config *igntypes.Config, device string) {
    label := "swapdisk"

    // 1. Create partition
    config.Storage.Disks = append(config.Storage.Disks, igntypes.Disk{
        Device: device,
        Partitions: []igntypes.Partition{{
            Label:    ptr.To(label),
            StartMiB: ptr.To(0),
            SizeMiB:  ptr.To(0),
        }},
        WipeTable: ptr.To(true),
    })

    // 2. Format as swap
    config.Storage.Filesystems = append(config.Storage.Filesystems, igntypes.Filesystem{
        Device:         fmt.Sprintf("/dev/disk/by-partlabel/%s", label),
        Format:         ptr.To("swap"),
        WipeFilesystem: ptr.To(true),
    })

    // 3. Create systemd swap unit
    config.Systemd.Units = append(config.Systemd.Units, igntypes.Unit{
        Name:    "dev-disk-by\\x2dpartlabel-swapdisk.swap",
        Enabled: ptr.To(true),
        Contents: ptr.To(`
[Unit]
Description=Swap on dedicated disk

[Swap]
What=/dev/disk/by-partlabel/swapdisk

[Install]
WantedBy=swap.target
`),
    })
}
```

#### Platform-Specific Disk Mapping

**Azure Example:**

The `platformDiskID` in the `diskSetup` configuration references the `nameSuffix` of an Azure DataDisk. The installer:

1. Matches `diskSetup[i].etcd.platformDiskID` with `dataDisks[i].nameSuffix`
2. Retrieves the LUN from the corresponding DataDisk
3. Maps to device path: `/dev/disk/azure/scsi1/lun{LUN}`

```go
// Azure-specific mapping
for i, diskSetup := range controlPlane.DiskSetup {
    if diskSetup.Type == types.Etcd {
        for _, dataDisk := range controlPlane.Platform.Azure.DataDisks {
            if diskSetup.Etcd.PlatformDiskID == dataDisk.NameSuffix {
                device := fmt.Sprintf("/dev/disk/azure/scsi1/lun%d", *dataDisk.Lun)
                mc, err := ForDiskSetup("master", device, types.Etcd, diskSetup)
                // Add MachineConfig to manifests
            }
        }
    }
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This feature can be applied to Hypershift / Hosted Control Planes for worker nodes. The hosted control plane itself runs as pods and does not use this disk setup mechanism. Worker nodes in hosted clusters can be configured with disk setup for swap or user-defined storage needs.

#### Standalone Clusters

This feature applies to standalone OpenShift clusters. Disk setup can be configured for both control plane and compute nodes. Control plane nodes commonly use dedicated etcd disks for performance isolation. Compute nodes can use swap or user-defined disks for application workloads.

#### Single-node Deployments or MicroShift

This feature applies to single-node OpenShift deployments. The single control plane node can be configured with dedicated disks for etcd, swap, or user-defined purposes. This feature does not apply to MicroShift as it does not use the OpenShift installer.


### Implementation Details/Notes/Constraints

**Modified Files:**

The implementation spans several installer components:
- `pkg/types/types.go` - Adds `DiskSetup` field to MachinePool type
- `pkg/types/validation/machinepools.go` - Generic disk setup validation
- `pkg/types/azure/validation/machinepool.go` - Azure-specific validation
- `pkg/asset/machines/machineconfig/disks.go` - MachineConfig generation for disk setup
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
  - **Azure**: Must match `nameSuffix` from `dataDisks` configuration
  - **Other platforms**: TBD based on platform disk naming conventions
- The installer performs validation to ensure `platformDiskID` references exist
- Device paths are resolved at MachineConfig generation time

**Partitioning:**

- Uses the entire disk for a single partition
- Partition starts at 0 MiB and uses full disk capacity (SizeMiB: 0)
- Partition table is wiped before creating new partition

**Filesystem:**

- **Etcd disks**: XFS with mount options `defaults,prjquota`
- **User-defined disks**: XFS with mount options `defaults`
- **Swap disks**: Formatted as swap space
- Filesystem is wiped before formatting (WipeFilesystem: true)

**MachineConfig Integration:**

- Each disk setup generates a separate MachineConfig resource
- MachineConfigs are labeled with role-specific labels for MCO targeting
- Ignition version 3.2.0 is used
- MachineConfigs are rendered during `create manifests` phase

**Error Handling:**

- If `platformDiskID` doesn't match any platform disk, validation fails at install-config validation
- If the platform disk is not attached at boot time, the mount will fail
- Failed mounts for critical disks (etcd) will prevent the node from becoming Ready
- Failed mounts for non-critical disks (user-defined, swap) may allow the node to become Ready but with degraded functionality

**Idempotency:**

- Ignition's disk/filesystem handling is idempotent
- Re-running ignition with the same configuration is safe
- Partition labels prevent creating duplicate partitions

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

### Risks and Mitigations

**Risk: Incorrect PlatformDiskID Configuration**

User provides an incorrect `platformDiskID` that doesn't match any configured platform disk.

*Mitigation:*
- Installer validates `platformDiskID` against configured platform disks (e.g., Azure DataDisks)
- Validation fails at install-config time if no match is found
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

Initial implementation is Azure-specific; other platforms need separate implementations.

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

Disk setup can only be configured during cluster installation. Nodes provisioned after installation cannot add disk setup without recreating the machine pool or manual intervention.

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

Once configured, disk setup cannot be modified or removed without node reprovisioning. This is consistent with MachineConfig immutability but limits operational flexibility.

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

- Create periodic CI jobs for Azure with disk setup configurations
- Monitor for disk setup-specific failures
- Test upgrades from clusters without disk setup to ensure compatibility

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
  - E2E test for swap disk
  - E2E test for user-defined disk

- Documentation:
  - API reference documentation
  - Azure-specific examples
  - Installation guide with disk setup

### Tech Preview -> GA

- Enhanced reliability:
  - More comprehensive testing (upgrade, downgrade, scale)
  - Sufficient time for feedback from Tech Preview users
  - Bug fixes based on user feedback

- Expanded testing:
  - E2E tests for all supported scenarios
  - Platform-specific test coverage
  - Performance testing for etcd on dedicated disks
  - Stress testing with multiple disk configurations

- Documentation improvements:
  - Best practices guide
  - Troubleshooting guide
  - Performance tuning recommendations
  - User-facing documentation in openshift-docs

- Operational maturity:
  - Clear support procedures
  - Monitoring and alerting guidance
  - Recovery procedures for disk failures

- Optional enhancements based on feedback:
  - Support for additional platforms (AWS, GCP)
  - Configurable filesystem types
  - Additional mount options

**For non-optional features moving to GA, the graduation criteria must include end to end tests.**

### Removing a deprecated feature

Not applicable for this initial enhancement.

## Upgrade / Downgrade Strategy

This feature does not impact the upgrade or downgrade process for existing clusters. Disk setup is configured at cluster installation time and becomes part of the MachineConfig for the role.

**Upgrade Scenarios:**

- Existing clusters without disk setup can continue to operate normally after upgrading to a version that includes this feature
- New machine pools created after upgrade can utilize the disk setup feature (if platform supports dynamic machine pool creation)
- Existing nodes with disk setup configurations remain unchanged during cluster upgrades
- MachineConfigs generated for disk setup are immutable and persist through upgrades

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

If disk setup configuration is invalid, the installer will fail during validation with clear error messages:

- "disk type must be one of: etcd, swap, user-defined" - Invalid disk type
- "platformDiskID is required" - Missing platform disk identifier
- "platformDiskID 'X' does not match any configured data disk" - No matching platform disk (Azure)
- "only one etcd disk per machine pool is allowed" - Multiple etcd disks configured
- "only one swap disk per machine pool is allowed" - Multiple swap disks configured
- "mountPath is required for user-defined disks" - Missing mount path
- "data disk support is not currently available on StackCloud" - Azure Stack Cloud limitation

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
  - Worker nodes with swap disks
  - Worker nodes with user-defined disks
  - Mixed configurations

**Test Resources:**

- Additional Azure resources for data disks in CI subscriptions
- Cost considerations for Premium_LRS disks in testing
- Cleanup automation to remove test disks after job completion

**Documentation Infrastructure:**

- Examples repository with sample install-config.yaml files
- Documentation for each supported platform (currently Azure)
- Troubleshooting guides and runbooks

**No Special Infrastructure Required:**

- No new clusters or dedicated test environments needed beyond standard CI
- Uses existing Azure CI infrastructure with additional disk provisioning
- No new monitoring or observability infrastructure required