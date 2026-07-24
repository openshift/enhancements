---
title: nutanix-os-disk-storage-container
authors:
- "@chdeshpa"

reviewers:
- "@patrickdillon"
- "@JoelSpeed"
- "@yanhua121"

approvers:
- "@patrickdillon"

api-approvers:
- "@JoelSpeed"

creation-date: 2026-06-03
last-updated: 2026-06-03
tracking-link:
- https://issues.redhat.com/browse/RFE-9321
status: implementable
---

# Nutanix: Configurable Storage Container for Principal OS Disk

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This enhancement exposes an explicit `storageConfig` field on the Nutanix `osDisk` configuration in install-config and Machine API, allowing administrators to select the Nutanix storage container for VM principal OS (system) disks during IPI installation and MachineSet scale-out. Today, OS disk placement is implicit — determined by Nutanix platform behavior based on where the RHCOS image resides — with no first-class API to override it. This creates operational gaps for organizations that require OS disks on dedicated storage tiers.

## Motivation

Nutanix IPI clusters (OCP 4.18+) support configuring storage containers for **data disks** via `dataDisks[].storageConfig.storageContainer`, but the principal OS/system disk only exposes `diskSizeGiB`. When customers need OS disks on a specific storage container (e.g., a high-performance tier vs. the default `SelfServiceContainer`), they rely on uploading the preloaded RHCOS image to the desired container and hoping Nutanix places the system disk there. This behavior is:

1. **Implicit** — not controlled by any OpenShift API field
2. **Undocumented** — not guaranteed by Nutanix across Prism versions
3. **Fragile** — fails silently if image placement changes

### User Stories

* As an OpenShift administrator installing on Nutanix IPI, I want to configure the storage container for control plane and worker OS disks in `install-config.yaml`, so that cluster nodes use a designated storage tier without depending on where a preloaded image was uploaded.

* As a platform engineer, I want MachineSets to specify the OS disk storage container for scaled-out workers, so that new nodes follow the same storage policy as install-time configuration.

* As a support engineer, I want a documented API-backed setting distinct from `preloadedOSImageName`, so that troubleshooting aligns with actual product behavior.

### Goals

- Provide an explicit, validated API to set the storage container for system/OS disks at install time.
- Propagate the setting through Machine API and CAPN for new nodes created by MachineSet.
- Preserve current behavior (platform-default placement) when the field is omitted.
- Reuse the existing `StorageConfig` type pattern from data disks for consistency.
- Support both UUID and name-based storage container references, including failure domain `referenceName`.

### Non-Goals

- Relocating OS disks on existing VMs to another storage container (day-2 migration).
- Mapping Nutanix projects to storage containers (remain separate concepts).
- Bootstrap ignition ISO storage placement.
- Nutanix UPI parity (unless requested in a follow-on).
- Semantic validation of Nutanix storage container capacity or compatibility (deferred to Prism at provisioning time).

## Proposal

### Workflow Description

**cluster administrator** is a user responsible for installing and configuring an OpenShift cluster.

1. The cluster administrator creates or edits their `install-config.yaml`
2. Under any machine pool's Nutanix platform configuration, they set `osDisk.storageConfig.storageContainer` with either a UUID or name reference
3. The cluster administrator runs `openshift-install create cluster`
4. During cluster provisioning, the installer:
   - Validates the storage container exists in Prism Central (PC lookup via v3 API)
   - Resolves failure-domain `referenceName` if used
   - Creates Machine CRs (CAPN or MAPI) with the storage container reference on the system disk spec
   - Nutanix provisions VMs with OS disks on the specified container
5. For day-2 operations, MachineSets with `systemDiskStorageConfig` will direct CAPN to place new node OS disks on the configured container

#### Example Configuration

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: mycluster
platform:
  nutanix:
    prismCentral:
      endpoint:
        address: pc.example.com
        port: 9440
      username: admin
      password: secret
    subnetUUIDs:
      - 12345678-abcd-1234-abcd-123456789abc
    preloadedOSImageName: rhcos-418.94.202505010932-0
controlPlane:
  name: master
  replicas: 3
  platform:
    nutanix:
      cpus: 8
      memoryMiB: 32768
      osDisk:
        diskSizeGiB: 120
        storageConfig:
          storageContainer:
            type: uuid
            uuid: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
compute:
  - name: worker
    replicas: 3
    platform:
      nutanix:
        cpus: 4
        memoryMiB: 16384
        osDisk:
          diskSizeGiB: 120
          storageConfig:
            storageContainer:
              type: name
              name: ocp-high-perf-tier
```

#### Variation: Failure Domain Reference

```yaml
platform:
  nutanix:
    failureDomains:
      - name: fd-az1
        storageContainers:
          - name: ocp-tier1
            uuid: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
compute:
  - name: worker
    platform:
      nutanix:
        osDisk:
          diskSizeGiB: 120
          storageConfig:
            storageContainer:
              type: referenceName
              referenceName: ocp-tier1
```

### API Extensions

#### install-config types (`pkg/types/nutanix/machinepool.go`)

Extend the existing `OSDisk` struct:

```go
// OSDisk defines the system disk for a Nutanix VM.
type OSDisk struct {
    // DiskSizeGiB defines the size of disk in GiB.
    // +optional
    DiskSizeGiB int64 `json:"diskSizeGiB,omitempty"`

    // StorageConfig specifies the storage configuration for the OS disk.
    // When omitted, the platform default storage container is used.
    // +optional
    StorageConfig *StorageConfig `json:"storageConfig,omitempty"`
}
```

This reuses the existing `StorageConfig` type already defined for data disks:

```go
type StorageConfig struct {
    // StorageContainer refers to the storage container used for the disk.
    StorageContainer *StorageResourceReference `json:"storageContainer,omitempty"`
}

type StorageResourceReference struct {
    // Type identifies whether this is a uuid, name, or referenceName.
    Type StorageResourceIdentifierType `json:"type"`
    // UUID of the storage container.
    UUID string `json:"uuid,omitempty"`
    // Name of the storage container.
    Name string `json:"name,omitempty"`
    // ReferenceName maps to a failure domain storageContainers entry.
    ReferenceName string `json:"referenceName,omitempty"`
}
```

#### Machine API types (`openshift/api`)

Add to `NutanixMachineProviderConfig`:

```go
// SystemDiskStorageConfig optionally specifies the storage container for the
// VM system (OS) disk. When omitted, Nutanix platform defaults apply.
// +optional
SystemDiskStorageConfig *NutanixVMStorageConfig `json:"systemDiskStorageConfig,omitempty"`
```

This mirrors the existing `NutanixVMDisk.StorageConfig` pattern used for data disks.

#### CAPN (`CreateSystemDiskSpec`)

In `cluster-api-provider-nutanix`, extend the system disk creation to accept a storage container reference:

```go
systemDisk := &prismclientv3.VMDisk{
    DataSourceReference: &prismclientv3.Reference{
        Kind: utils.StringPtr("image"),
        UUID: utils.StringPtr(imageUUID),
    },
    DiskSizeMib: utils.Int64Ptr(systemDiskSize),
}

// If storage container is configured, set it on the system disk
if machineSpec.SystemDiskStorageConfig != nil &&
    machineSpec.SystemDiskStorageConfig.StorageContainerReference != nil {
    systemDisk.StorageConfig = &prismclientv3.DiskStorageConfig{
        StorageContainerReference: &prismclientv3.Reference{
            Kind: utils.StringPtr("storage_container"),
            UUID: utils.StringPtr(
                machineSpec.SystemDiskStorageConfig.StorageContainerReference.UUID),
        },
    }
}
```

### Interaction with `preloadedOSImageName`

When both `preloadedOSImageName` and `osDisk.storageConfig.storageContainer` are set:

**Behavior:** The explicit `osDisk.storageConfig.storageContainer` overrides implicit placement from the image location. The image itself is still used as the data source (clone source) for the system disk, but the resulting disk is placed on the explicitly configured container.

**Rationale:** This matches how data-disk `storageConfig` works independently of image placement, and gives users explicit control. Nutanix Prism supports creating a disk from an image while placing it on a different storage container.

### Topology Considerations

| Topology | Behavior |
|----------|----------|
| Standalone IPI | Primary target; validated at install time |
| MachineSet scale-out | New VMs honor `systemDiskStorageConfig` in provider spec |
| Failure domains | `referenceName` resolved via `platform.nutanix.failureDomains[].storageContainers` |
| Hypershift / Hosted CP | Out of scope for initial implementation |

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Prism rejects storage container + image combination | Install-time validation queries PC to verify container exists on the target PE |
| Older Prism versions may not support explicit placement | Document minimum AOS/PC version; fail clearly at validation |
| Field confusion with data disk `storageConfig` | Reuse same types and naming for consistency |

## Design Details

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Feature behind `NutanixOSDiskStorageContainer` feature gate (DevPreviewNoUpgrade)
- IPI install verified with explicit storage container (UUID and name)
- Unit tests for validation and asset generation

#### Tech Preview -> GA

- Feature gate promoted to Default feature set
- E2E test: IPI install + MachineSet scale-out with storage container
- Documentation merged
- QE sign-off on Nutanix lab

#### Removing a deprecated feature

N/A — new feature.

### Upgrade / Downgrade Strategy

- Clusters installed without the field continue unchanged on upgrade.
- The field is optional; existing install-configs remain valid.
- On downgrade, the field in MachineSet provider specs is ignored by older controllers (standard Kubernetes extension semantics).

### Version Skew Strategy

- The installer writes the provider spec; controller version does not need to be coordinated separately.
- CAPN must be at the version that understands `SystemDiskStorageConfig` before the field takes effect on new machines.

### Operational Aspects of API Extensions

- **Failure mode:** If a nonexistent storage container UUID is specified, install fails at validation with a clear error message referencing the invalid UUID/name.
- **Monitoring:** No new metrics required; existing machine provisioning metrics apply.

## Implementation History

- 2026-05-21: Investigation started; 4.18 branch verified — gap confirmed
- 2026-06-03: Enhancement proposal drafted

## Alternatives

### Alternative 1: Rely on preloaded image placement

Users upload the RHCOS image to the desired storage container, and Nutanix implicitly places system disks there.

**Rejected because:** This is undocumented behavior, not guaranteed across Prism versions, and provides no MachineSet-level control for day-2 nodes.

### Alternative 2: Validation error when explicit container conflicts with image location

Reject configurations where the storage container differs from where the preloaded image resides.

**Rejected because:** This is unnecessarily restrictive. Nutanix supports cross-container disk creation from an image source, and customers explicitly want to separate image storage from VM disk storage.

### Alternative 3: New top-level field instead of extending `osDisk`

Add `systemDiskStorageContainer` as a sibling to `osDisk` rather than nesting inside it.

**Rejected because:** Nesting inside `osDisk.storageConfig` mirrors the data-disk pattern exactly, reducing cognitive load and code duplication.

## Infrastructure Needed

- Nutanix lab cluster with multiple storage containers for CI testing
- Prism Central v2024.1+ with multiple Prism Elements (for failure domain testing)
