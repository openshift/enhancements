---
title: azure-data-disk-iops-throughput
authors:
- "@chdeshpa"
reviewers:
- "@jcpowermac"
- "@vr4manta"
approvers:
- "@patrickdillon"
api-approvers:
- TBD
creation-date: 2026-05-16
last-updated: 2026-05-20
tracking-link:
- https://issues.redhat.com/browse/RFE-7972
status: implementable
---

# Azure Data Disk IOPS/Throughput Configuration

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This enhancement extends the existing Azure data disk support (SPLAT-2133) to allow configuration of provisioned IOPS (`diskIOPSReadWrite`) and throughput (`diskMBpsReadWrite`) for Azure Premium SSD v2 (`PremiumV2_LRS`) and Ultra SSD (`UltraSSD_LRS`) data disks. This enables fine-grained performance tuning and cost optimization during OpenShift cluster installation and day-2 machine scaling, as Azure Premium SSD v2 bills IOPS and throughput independently of disk capacity.

## Motivation

Azure Premium SSD v2 (`PremiumV2_LRS`) offers significant cost savings over Premium SSD (`Premium_LRS`) by decoupling IOPS and throughput provisioning from disk size. A customer needing a 64 GiB etcd data disk with 3000 IOPS currently must over-provision a P15 (256 GiB) Premium_LRS disk to reach the IOPS target, paying for unused capacity. With PremiumV2_LRS, the same 64 GiB disk can be provisioned with exactly 3000 IOPS and 125 MBps throughput, reducing costs by approximately 40%.

However, the current OpenShift installer and Machine API do not support specifying these performance parameters. This enhancement adds that capability.

### User Stories

* As an OpenShift administrator deploying on Azure, I want to configure provisioned IOPS and throughput for PremiumV2_LRS data disks so that I can optimize performance and cost for workloads like etcd.
* As an OpenShift administrator, I want to use PremiumV2_LRS data disks with custom IOPS for day-2 machine scaling via MachineSets, so that new worker nodes get the same performance characteristics as install-time nodes.

### Goals

- Allow `diskIOPSReadWrite` and `diskMBpsReadWrite` configuration on data disks using `PremiumV2_LRS` or `UltraSSD_LRS` storage account types.
- Propagate these settings through the full stack: install-config, CAPZ, Machine API, and MAPO (day-2).
- Validate that IOPS/throughput settings are only used with compatible storage account types.

### Non-Goals

- Semantic validation of IOPS/throughput values against Azure capacity limits (deferred to Azure ARM API at provisioning time).
- Automatic filesystem formatting or mounting of data disks.
- Support for `PremiumV2_LRS` on Azure Stack Hub.

## Proposal

### Workflow Description

**cluster administrator** is a user responsible for installing and configuring an OpenShift cluster.

1. The cluster administrator creates or edits their install-config.yaml file
2. For any machine pool (control plane or compute), they add a `dataDisks` section under the Azure platform configuration with `PremiumV2_LRS` storage account type and optional IOPS/throughput values
3. The cluster administrator runs `openshift-install create cluster`
4. During cluster provisioning, the installer:
   - Validates that `diskIOPSReadWrite`/`diskMBpsReadWrite` are only set with `PremiumV2_LRS` or `UltraSSD_LRS`
   - Validates that `cachingType` is `None` or omitted for `PremiumV2_LRS` data disks
   - Creates AzureMachine CRs (for CAPZ) and Machine CRs (for Machine API) with the IOPS/throughput settings
   - Azure provisions the VMs with data disks at the specified performance levels
5. For day-2 operations, MachineSets with PremiumV2_LRS data disks and IOPS/throughput settings can be created, and MAPO will pass the values to the Azure ARM API

#### Example Configuration

Example install-config.yaml with PremiumV2_LRS data disks and custom IOPS/throughput:

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: mycluster
platform:
  azure:
    region: eastus
controlPlane:
  name: master
  replicas: 3
  platform:
    azure:
      type: Standard_D8s_v5
      dataDisks:
      - lun: 0
        nameSuffix: etcd
        diskSizeGB: 64
        cachingType: None
        managedDisk:
          storageAccountType: PremiumV2_LRS
          diskIOPSReadWrite: 3000
          diskMBpsReadWrite: 125
compute:
- name: worker
  replicas: 3
  platform:
    azure:
      type: Standard_D4s_v5
      dataDisks:
      - lun: 0
        nameSuffix: data
        diskSizeGB: 256
        cachingType: None
        managedDisk:
          storageAccountType: PremiumV2_LRS
          diskIOPSReadWrite: 5000
          diskMBpsReadWrite: 200
```

### API Extensions

This enhancement modifies types across multiple OpenShift repositories.

#### openshift/api Changes

The `DataDiskManagedDiskParameters` struct in `machine/v1beta1/types_azureprovider.go` is extended:

```go
type DataDiskManagedDiskParameters struct {
    // storageAccountType is the storage account type to use.
    // Possible values include "Standard_LRS", "Premium_LRS", "UltraSSD_LRS" and "PremiumV2_LRS".
    // +kubebuilder:validation:Enum=Standard_LRS;Premium_LRS;UltraSSD_LRS;PremiumV2_LRS
    StorageAccountType StorageAccountType `json:"storageAccountType"`

    // diskEncryptionSet is the disk encryption set properties.
    // +optional
    DiskEncryptionSet *DiskEncryptionSetParameters `json:"diskEncryptionSet,omitempty"`

    // diskIOPSReadWrite specifies the provisioned IOPS for the managed disk.
    // Only applicable when storageAccountType is PremiumV2_LRS or UltraSSD_LRS.
    // +kubebuilder:validation:Minimum=1
    // +optional
    DiskIOPSReadWrite *int64 `json:"diskIOPSReadWrite,omitempty"`

    // diskMBpsReadWrite specifies the provisioned throughput in MBps for the managed disk.
    // Only applicable when storageAccountType is PremiumV2_LRS or UltraSSD_LRS.
    // +kubebuilder:validation:Minimum=1
    // +optional
    DiskMBpsReadWrite *int64 `json:"diskMBpsReadWrite,omitempty"`
}
```

A new constant is added to the existing `StorageAccountType` enum:

```go
// "StorageAccountPremiumV2LRS" means the PremiumV2_LRS storage type.
StorageAccountPremiumV2LRS StorageAccountType = "PremiumV2_LRS"
```

The kubebuilder validation enum on `DataDiskManagedDiskParameters.StorageAccountType` is updated to include `PremiumV2_LRS`.

#### Installer Changes

The CAPZ `ManagedDiskParameters` type (vendored) is extended with the same `DiskIOPSReadWrite`/`DiskMBpsReadWrite` fields. The CAPZ `generateStorageProfile` function sets these on the ARM `armcompute.DataDisk` struct (not on `armcompute.ManagedDiskParameters`, which does not have these fields in the Azure SDK).

The installer's `provider()` function in `pkg/asset/machines/azure/machines.go` maps the fields from CAPZ DataDisk to Machine API DataDisk.

#### Validation Rules

| Rule | Where enforced |
|------|---------------|
| `diskIOPSReadWrite`/`diskMBpsReadWrite` only with `UltraSSD_LRS` or `PremiumV2_LRS` | MAO webhook (`validateAzureDataDisks`) |
| Caching must be `None` or empty for `PremiumV2_LRS` data disks | MAO webhook + MAPO (`generateDataDisks`) |
| IOPS/MBps values must be >= 1 | openshift/api kubebuilder validation tags |
| Azure-level constraints (IOPS ratio, region support) | Azure ARM API (runtime rejection) |

### Topology Considerations

#### Hypershift / Hosted Control Planes

Azure self-managed Hosted Control Planes is available as a Developer Preview feature in OpenShift 4.21. As of 4.22, Azure HCP is not yet GA via RHACM. If Azure HCP reaches GA in a future release, this feature could apply to worker node data disks configured via NodePool specifications. No additional work is required in this enhancement to support that topology — the Machine API types extended here are shared across standalone and hosted architectures.

#### Standalone Clusters

This feature applies to standalone OpenShift clusters. Data disks with IOPS/throughput can be configured for both control plane and compute nodes.

#### Single-node Deployments or MicroShift

This feature applies to single-node OpenShift deployments. Data disks can be configured on the single control plane node. This feature does not apply to MicroShift as it does not use the OpenShift installer.

### Implementation Details/Notes/Constraints

**Data Flow:**

```
install-config.yaml
    -> Installer parses MachinePool.DataDisks[].ManagedDisk.DiskIOPSReadWrite/DiskMBpsReadWrite
    -> CAPZ AzureMachine CR (install time provisioning via CAPZ controller)
    -> Machine API Machine CR (day-2 via MAO webhook validation -> MAPO provisioning)
    -> Azure ARM API: DataDisk.DiskIOPSReadWrite / DataDisk.DiskMBpsReadWrite
```

**Modified Repositories and Files:**

1. **openshift/api**: `machine/v1beta1/types_azureprovider.go`, `zz_generated.deepcopy.go`
2. **openshift/installer**:
   - `cluster-api/providers/azure/vendor/.../types.go` (CAPZ types)
   - `cluster-api/providers/azure/vendor/.../spec.go` (CAPZ ARM wiring)
   - `data/data/cluster-api/azure-infrastructure-components.yaml` (CRD schema)
   - `data/data/install.openshift.io_installconfigs.yaml` (install-config CRD schema)
   - `pkg/asset/machines/azure/machines.go` (Machine API mapping)
3. **machine-api-provider-azure**: `pkg/cloud/azure/services/virtualmachines/virtualmachines.go` (ARM DataDisk wiring)
4. **machine-api-operator**: `pkg/webhooks/machine_webhook.go` (webhook validation)

**Azure ARM SDK Note:**

In the Azure compute SDK, `DiskIOPSReadWrite` and `DiskMBpsReadWrite` are properties of `armcompute.DataDisk`, not of `armcompute.ManagedDiskParameters`. The CAPZ and MAPO wiring must set these on the DataDisk struct directly.

**Constraints:**

- Maximum of 64 data disks per VM (Azure limit, varies by VM size)
- LUN values must be between 0 and 63
- PremiumV2_LRS requires the VM to be in a region and zone that supports it
- Caching must be disabled (set to `None`) for PremiumV2_LRS and UltraSSD_LRS data disks
- Azure imposes per-disk and per-VM IOPS and throughput limits based on VM size and disk size

### Risks and Mitigations

**Risk**: Azure does not validate IOPS/throughput values at the API schema level. Invalid combinations (e.g., IOPS exceeding VM limits) are only rejected at provisioning time.

**Mitigation**: Clear error messages from Azure ARM API will surface during cluster installation. Documentation will include guidance on Azure limits. The kubebuilder `Minimum=1` validation prevents zero/negative values.

## Design Details

### Open Questions

1. Should this feature be gated behind `TechPreviewNoUpgrade` for initial release, or ship as GA-ready?
2. Should the installer perform any semantic validation of IOPS/throughput values, or defer entirely to Azure?

### Test Plan

**Unit Tests:**

- Install-config validation:
  - `PremiumV2_LRS` accepted for data disk `storageAccountType`
  - IOPS/MBps fields parsed from YAML into correct Go types

- Machine API mapping:
  - CAPZ DataDisk with IOPS/MBps correctly mapped to Machine API DataDisk
  - End-to-end flow: YAML -> CAPZ types -> Machine API types preserves values

- MAPO ARM wiring:
  - PremiumV2_LRS data disk with IOPS/MBps produces correct ARM DataDisk struct
  - PremiumV2_LRS data disk without IOPS/MBps produces ARM DataDisk with nil IOPS/MBps
  - PremiumV2_LRS data disk with invalid cachingType rejected

- MAO webhook:
  - PremiumV2_LRS + IOPS/MBps accepted without warning
  - PremiumV2_LRS + cachingType ReadOnly rejected
  - Standard_LRS + IOPS/MBps rejected
  - UltraSSD_LRS + IOPS/MBps accepted

**End-to-End Tests:**

- Cluster installation with PremiumV2_LRS data disks and custom IOPS/throughput on control plane
- Verify data disks are attached to VMs with correct IOPS/throughput via Azure CLI
- MachineSet scaling with PremiumV2_LRS data disks and custom IOPS/throughput
- Cluster destruction removes all data disks

**CI Jobs:**

CI jobs will be created to test installation with PremiumV2_LRS data disk configurations.

**Prototype Validation (completed):**

A full-stack prototype has been implemented and validated on a live Azure environment (OpenShift 4.22-rc.3). Results:

- Cluster installed successfully with PremiumV2_LRS data disks (64 GiB, 3000 IOPS, 125 MBps) on control plane nodes
- Azure CLI confirmed provisioned disks have correct IOPS/throughput values (`az disk show --query '{iops: diskIOPSReadWrite, mbps: diskMBpsReadWrite}'`)
- CPMS template retains IOPS/MBps fields from install time
- Day-2 MachineSet scale-up with custom MAPO produces worker nodes with correct data disk performance settings
- MAO webhook correctly rejects invalid combinations (Standard_LRS + IOPS, PremiumV2 + ReadWrite caching)
- Unit tests pass across installer, MAPO, and MAO repositories

## Graduation Criteria

### Dev Preview -> Tech Preview

- Installer allows configuration of IOPS/throughput on PremiumV2_LRS data disks
- CI jobs for testing installation with custom IOPS/throughput
- End user documentation, relative API stability
- Sufficient test coverage

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- User facing documentation created in OCP documentation
- E2E tests verify IOPS/throughput values on provisioned Azure disks

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

This feature does not impact the upgrade or downgrade process. IOPS/throughput settings are provisioned at cluster installation time or day-2 machine creation and are not modified during cluster upgrades.

Existing clusters without IOPS/throughput settings continue to operate normally. New machines created after upgrade can utilize the IOPS/throughput configuration.

During a failed upgrade rollback, no special handling is required for IOPS/throughput settings.

## Version Skew Strategy

The `diskIOPSReadWrite` and `diskMBpsReadWrite` fields are optional (`+optional`) with `omitempty` JSON tags. Components that do not recognize these fields will simply ignore them:

- Older MAPO versions will not pass IOPS/throughput to Azure (data disks get Azure defaults)
- Older MAO versions will issue a warning but still accept the Machine object (the fields are in `providerSpec`, which is a `RawExtension`)
- Older installer versions will not include the fields in generated manifests

No version skew coordination is required.

## Operational Aspects of API Extensions

This enhancement does not introduce new CRDs, admission webhooks, or aggregated API servers. The changes extend existing fields in the installer's install-config.yaml schema and the Machine API's `AzureMachineProviderSpec`.

The MAO webhook (which already exists) is extended with additional validation logic for the new fields.

## Support Procedures

**Detecting Configuration Issues:**

- If IOPS/throughput are set on an incompatible storage account type, the MAO webhook returns a clear validation error
- If Azure rejects IOPS/throughput values at provisioning time, the error appears in CAPZ or MAPO controller logs

**Verifying IOPS/Throughput on Running Disks:**

```bash
# Via Azure CLI
az disk show --resource-group <rg> --name <disk-name> \
  --query '{iops: diskIOPSReadWrite, mbps: diskMBpsReadWrite, sku: sku.name}'

# Via Machine API
oc get machine <name> -n openshift-machine-api \
  -o jsonpath='{.spec.providerSpec.value.dataDisks[0].managedDisk}'
```

**Common Issues:**

- **IOPS/MBps ignored on Standard_LRS/Premium_LRS**: These storage types do not support custom IOPS. The webhook rejects this combination with a clear error.
- **Caching error with PremiumV2_LRS**: PremiumV2_LRS requires `cachingType: None`. Both the webhook and MAPO enforce this.
- **Region/zone not supported**: PremiumV2_LRS availability varies by region. Check Azure documentation for supported regions.

## Alternatives (Not Implemented)

**Alternative 1: Installer-Side IOPS/Throughput Validation**

Instead of deferring IOPS/throughput limit validation to Azure, the installer could validate values against Azure's published limits. This was not implemented because:
- Azure limits vary by disk size, VM size, and region
- Limits change over time as Azure updates offerings
- The Azure ARM API provides clear error messages for invalid combinations
- Adding static limits to the installer would require frequent updates

## Future Enhancements (Optional)

### Hive/RHACM Deployment Support

**CLI (Hive `ClusterDeployment`):** This feature works automatically with Hive. Users include `diskIOPSReadWrite`/`diskMBpsReadWrite` in their `install-config.yaml` Secret referenced by the `ClusterDeployment` CR. No changes to Hive are required — Hive passes the install-config directly to the installer binary.

**UI (RHACM Console):** The RHACM console cluster creation wizard does not currently expose data disk IOPS/throughput fields in its form-based UI. Users can work around this by switching to the YAML editor during cluster creation and adding the fields manually. Full UI integration (dedicated form fields for IOPS/throughput when `PremiumV2_LRS` or `UltraSSD_LRS` is selected) would require a separate effort in the `stolostron/console` repository and is out of scope for this enhancement.

## Infrastructure Needed [optional]

None. This feature uses existing Azure infrastructure. CI testing requires Azure subscriptions with access to regions supporting PremiumV2_LRS.
