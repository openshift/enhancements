---
title: multiple-disk-setup-installer
authors:
  - Joseph Callen 
reviewers:
  - "@vr4manta"
  - "@JoelSpeed"
  - "@patrickdillon"
  - "@yuqi-zhang"
approvers:
  - "@patrickdillon"
api-approvers: # If new CRDs or API fields are introduced to a shared API (e.g. machine.openshift.io)
  - 
creation-date: 2025-05-16
last-updated: 2025-05-16
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2046 
see-also:"/enhancements/"
  -  
replaces:
  - 
superseded-by:
  - 
---

# Multiple Disk Setup in Installer

This enhancement outlines the design for supporting multiple disk setup within the installer. The initial changes will focus on the installer's capability to generate an ignition config that correctly creates partitions, file systems, and systemd mounts for these disks.

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
* Support for all possible file systems; a common default (e.g., XFS or ext4) will be used, with potential for future extension.
* Changes to how the operating system image itself is deployed to the primary disk.

## Proposal

The core of this proposal is to introduce new structures within the machine pool API that allow users to declare additional disks and their intended use. The installer will then interpret these declarations to generate the appropriate `storage` and `systemd` sections in the ignition config.

### Workflow Description

1.  The **cluster creator** defines an `install-config.yaml` (or equivalent API input).
2.  Within the machine pool definition (e.g., for control plane or worker nodes), the cluster creator specifies an array of `Disk` objects.
3.  Each `Disk` object will specify its `Type` (`Etcd`, `Swap`, or `UserDefined`) and the necessary parameters like `PlatformDiskID` (referring to a disk available on the platform, e.g., `/dev/sdb`, `nvme1n1`) and `MountPath` for `UserDefined` disks.
4.  The installer reads this configuration.
5.  For each specified `Disk`:
    * The installer generates ignition file entries to create a partition on the `PlatformDiskID`. A simple, single full-disk partition will be assumed initially.
    * It generates entries to format the partition with a default filesystem (e.g., XFS). For `Swap` type, it will be formatted as swap.
    * It generates systemd mount unit entries to mount the filesystem at the specified `MountPath` (for `UserDefined` and a default path for `Etcd`, e.g., `/var/lib/etcd`). For `Swap`, it generates the entry to enable the swap space.
6.  The generated ignition config is then used to provision the machines.
7.  Upon boot, ignition processes these configurations, partitioning, formatting, and mounting the additional disks.

### Installer Machine Pool changes

This enhancement introduces the following new types to the machine pool API:

```go
type DiskType string

const (
    Etcd        DiskType = "etcd"
    Swap        DiskType = "swap"
    UserDefined DiskType = "user-defined"
)

type Disk struct {
    Type DiskType `json:"type,omitempty"`

    UserDefined *DiskUserDefined `json:"userDefined,omitempty"`
    Etcd        *DiskEtcd        `json:"etcd,omitempty"`
    Swap        *DiskSwap        `json:"swap,omitempty"`
}

type DiskUserDefined struct {
    PlatformDiskID string `json:"platformDiskID,omitempty"` // e.g., /dev/sdb, by-id/ata-*, etc.
    MountPath      string `json:"mountPath,omitempty"`      // e.g., /var/custom-data
}

type DiskSwap struct {
    PlatformDiskID string `json:"platformDiskID,omitempty"` // e.g., /dev/sdc
}

type DiskEtcd struct {
    PlatformDiskID string `json:"platformDiskID,omitempty"` // e.g., /dev/sdd
    // MountPath for etcd will be implicitly /var/lib/etcd or a standard, configurable default.
}
```


* This enhancement modifies the behavior of the installer to interpret these new API fields and generate corresponding ignition configurations.
* It does not directly modify existing resources owned by other parties but generates configuration that affects node storage layout.


### Installer ignition additions


This currently is an incomplete example, would be replaced with switch/case for each platforms' implementation of additional disks.

```go

	if installConfig.Config.ControlPlane != nil {
		for i, d := range installConfig.Config.ControlPlane.DiskSetup {
			if d.Type == types.Etcd {

				// platform specific...
				if installConfig.Config.ControlPlane.Platform.Azure != nil {
					azurePlatform := installConfig.Config.ControlPlane.Platform.Azure
					if d.Etcd.PlatformDiskID == azurePlatform.DataDisks[i].NameSuffix {
						device := fmt.Sprintf("/dev/disk/azure/scsi1/lun%d", *azurePlatform.DataDisks[i].Lun)
						AddEtcdDisk(a.Config, device)
					}
				}
			}
		}
	}



func AddEtcdDisk(config *igntypes.Config, device string) {
	config.Storage.Disks = append(config.Storage.Disks, igntypes.Disk{
		Device: device,
		Partitions: []igntypes.Partition{{
			Label:    ptr.To("etcddisk"),
			StartMiB: ptr.To(0),
			SizeMiB:  ptr.To(0),
		}},
		WipeTable: ptr.To(true),
	})

	config.Storage.Filesystems = append(config.Storage.Filesystems, igntypes.Filesystem{
		Device:         "/dev/disk/by-partlabel/etcddisk",
		Format:         ptr.To("xfs"),
		Label:          ptr.To("etcdpart"),
		MountOptions:   []igntypes.MountOption{"defaults", "prjquota"},
		Path:           ptr.To("/var/lib/etcd"),
		WipeFilesystem: ptr.To(true),
	})
	config.Systemd.Units = append(config.Systemd.Units, igntypes.Unit{
		Name:    "var-lib-etcd.mount",
		Enabled: ptr.To(true),
		Contents: ptr.To(`
[Unit]
Requires=systemd-fsck@dev-disk-by\x2dpartlabel-var.service
After=systemd-fsck@dev-disk-by\x2dpartlabel-var.service

[Mount]
Where=/var/lib/etcd
What=/dev/disk/by-partlabel/etcddisk
Type=xfs
Options=defaults,prjquota

[Install]
RequiredBy=local-fs.target
`),
	})
}

```

### Topology Considerations

#### Standalone Clusters


### Implementation Details/Notes/Constraints

* **Disk Identification:** `PlatformDiskID` will rely on user-provided identifiers that are stable and predictable by the time ignition runs (e.g., `/dev/disk/by-id/...` is preferred over `/dev/sdx` names). The installer will not perform disk discovery; it will trust the user-provided ID.
* **Partitioning:** Initially, the proposal is to use the entire disk for a single partition. 
* **Filesystem:** A default filesystem XFS will be used for `Etcd` and `UserDefined` disks. 
* **Error Handling:** If a specified `PlatformDiskID` is not found by ignition, the mount will fail. The node may or may not come up healthy depending on the criticality of the mount (e.g., a failed etcd disk mount would be critical for a control plane node). This needs to be clearly documented.
* **Idempotency:** Ignition's handling of filesystems and mounts is generally idempotent.
* **Security:** Standard file permissions will be applied. No special SELinux labeling is planned for the initial implementation beyond what the system defaults for the given mount point.

### Risks and Mitigations

* **Risk:** User provides an incorrect `PlatformDiskID`, leading to formatting the wrong disk or installation failure.
    * **Mitigation:** Clear documentation and examples emphasizing the use of stable disk IDs (e.g., `/dev/disk/by-id/*`). The installer could potentially perform basic validation on the format of the ID but cannot guarantee its existence on the target machine.
* **Risk:** Conflicts with other processes or installer steps trying to use the same disk.
    * **Mitigation:** The disks specified for these additional roles should be distinct from the primary OS disk and any disks managed by other operators (like local storage operator) unless explicitly coordinated.
* **Risk:** Ignition generation logic becomes overly complex.
    * **Mitigation:** Start with simple cases (single partition, default filesystem) and iterate.
* **Security Review:** Will be required, focusing on how disk access and mount options are handled. Input from security teams will be solicited.
* **UX Review:** Simplicity of the API and clarity of documentation are key. UX review will ensure the configuration is intuitive.

### Drawbacks

* Potential for user error in specifying disk IDs, which can have destructive consequences if the wrong disk is targeted (though ignition typically runs before significant data exists on new nodes).

## Alternatives (Not Implemented)

* **Relying on Day-2 Operations:** Users could manually partition and mount disks after the cluster is up, or use operators like the Local Storage Operator. This proposal aims to make these common configurations available at installation time for simplicity and to ensure critical mounts (like etcd) are present from the first boot.

## Open Questions [optional]


## Test Plan

**Note:** *Section not required until targeted at a release.*

* Unit tests for the ignition generation logic based on various `Disk` configurations.
* E2E tests involving deploying clusters with:
    * Dedicated etcd disks.
    * Dedicated swap disks.
    * User-defined disks with specified mount points.
* Tests will verify that disks are correctly partitioned, formatted, and mounted, and that services like etcd utilize their dedicated storage.
* Testing across different cloud platforms and bare metal to ensure `PlatformDiskID` handling is robust.

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

### Dev Preview -> Tech Preview

### Tech Preview -> GA

* Core functionality implemented: partitioning, formatting (default FS), and mounting for Etcd, Swap, UserDefined disk types.
* Ability to specify `PlatformDiskID`.
* Basic e2e tests pass for common scenarios.
* Potentially allow configuration of filesystem type if strong demand exists.

### Removing a deprecated feature

Not applicable for this initial enhancement.

## Upgrade / Downgrade Strategy

* **Upgrade:** This feature primarily affects ignition generation. Upgrading an installer to a version with this feature will allow new clusters or new machine pools (if applicable post-install) to use it. Existing machine pools will not be affected unless re-provisioned with a configuration that uses these new fields. No changes are required for existing clusters to keep previous behavior.

## Version Skew Strategy

* This feature is primarily contained within the installer and the ignition config it produces. The ignition version consumed by the OS (e.g., RHCOS/FCOS) must support the storage (`files`, `filesystems`, `raid`) and `systemd` unit configurations generated. Standard ignition features will be used, minimizing skew risks with the OS.
* No direct interaction with other control plane components that would cause version skew issues beyond the installer's interaction with the machine API (if these fields are added there).

## Operational Aspects of API Extensions


## Support Procedures


## Infrastructure Needed [optional]

* CI jobs will need to be updated or created to test configurations with multiple disks, potentially requiring CI environments that can simulate or provide multiple attachable disk devices.