---
title: Subprovisioner CSI Driver Integration into LVMS
authors:
  - "@jakobmoellerdev"
reviewers:
  - "CNV Team"
  - "LVMS Team"
approvers:
  - "@DanielFroehlich"
  - "@jerpeter1"
  - "@suleymanakbas91"
api-approvers:
  - "@DanielFroehlich"
  - "@jerpeter1"
  - "@suleymanakbas91"
creation-date: 2024-05-02
last-updated: 2024-05-02
status: discovery
---

# Subprovisioner CSI Driver Integration into LVMS

[Subprovisioner](https://gitlab.com/subprovisioner/subprovisioner) 
is a CSI plugin for Kubernetes that enables you to provision Block volumes 
backed by a single, cluster-wide, shared block device (e.g., a single big LUN on a SAN).

Logical Volume Manager Storage (LVMS) uses the TopoLVM CSI driver to dynamically provision local storage on the OpenShift Container Platform clusters.

This proposal is about integrating the Subprovisioner CSI driver into the LVMS operator to enable the provisioning of 
shared block devices on the OpenShift Container Platform clusters. 

This enhancement will significantly increase scope of LVMS, but allows LVMS to gain the unique value proposition
of serving as a valid layered operator that offers LUN synchronization and provisioning capabilities.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This is a proposal to
- Create an enhancement to the "LVMCluster" CRD that is able to differentiate a deviceClass into a new
  type of shared storage that can be provisioned side-by-side or in alternative to regular LVMS device-classes managed by TopoLVM.
- Create a productization for a LUN-backed CSI driver alternative to TopoLVM that allows for shared vg usage, especially in the context of virtualization.


## Motivation

TopoLVM as our existing in-tree driver of LVMS is a great solution for local storage provisioning, but it lacks the ability to provision shared storage.
This is a significant limitation for virtualization workloads that require shared storage for their VMs that can dynamically be provisioned and deprovisioned 
on multiple nodes. Since OCP 4.15, LVMS support Multi-Node Deployments as a Topology, but without Replication or inbuilt resiliency behavior.

The Subprovisioner CSI driver is a great solution for shared storage provisioning, but it is currently not productized as part of OpenShift Container Platform.

### Goals

- Extension of the LVMCluster CRD to support a new deviceClass policy field that can be used to provision shared storage via Subprovisioner.
- Find a way to productize the Subprovisioner CSI driver as part of OpenShift Container Platform and increasing the Value Proposition of LVMS.
- Allow provisioning of regular TopoLVM deviceClasses and shared storage deviceClasses side-by-side in the same cluster.

### Non-Goals

- Compatibility with other CSI drivers than Subprovisioner. 
- Switching the default CSI driver for LVMS from TopoLVM to Subprovisioner or the other way around.
- Implementing a new CSI driver from scratch.
- Integrating the Subprovisioner CSI driver into TopoLVM.

### Risks and Mitigations
- There is a risk of increased maintenance burden by integrating a new CSI driver into LVMS without gaining traction
  - tested separately in the Subprovisioner project as pure CSI Driver similar to TopoLVM and within LVMS with help of QE
    - we will not GA the solution until we have a clear understanding of the maintenance burden. The solution will stay in TechPreview until then.
- There is a risk that Subprovisioner is so different from TopoLVM that behavior changes can not be accomodated in the current CRD
  - we will scrap this effort for integration and look for alternative solutions if the integration is not possible with reasonable effort.
- There is a risk that Subprovisioner is gonna break easily as its a really young project
  - we will not GA the solution until we have a clear understanding of the stability of the Subprovisioner project. The solution will stay in TechPreview until then.

## Proposal

The proposal is to extend the LVMCluster CRD with a new deviceClass policy field that can be used to provision shared storage via Subprovisioner.
We will use this field as a hook in lvm-operator, our orchestrating operator, to provision shared storage via Subprovisioner instead of TopoLVM.
Whenever LVMCluster discovers a new deviceClass with the Subprovisioner associated policy, it will create a new CSI driver deployment for Subprovisioner and configure it to use the shared storage deviceClass.
As such, it will handover the provisioning of shared storage to the Subprovisioner CSI driver. Also internal engineering such as sanlock orchestration will be managed by the driver.


### Workflow of Subprovisioner instantiation via LVMCluster

1. The user is informed of the intended use case of Subprovisioner, and decides to use it for its multi-node capabilities before provisioning Storage
2. The user configures LVMCluster with non-default values for the Volume Group and the deviceClass policy field
3. The lvm-operator detects the new deviceClass policy field and creates a new CSI driver deployment for Subprovisioner.
4. The Subprovisioner CSI driver is configured to use the shared storage deviceClass, initializes the global lock space, and starts provisioning shared storage.
5. The user can now provision shared storage via Subprovisioner on the OpenShift Container Platform cluster.
6. The user can also provision regular TopoLVM deviceClasses side-by-side with shared storage deviceClasses in the same cluster. Then, TopoLVM gets provisioned side-by-side.

## Design Details for `LVMCluster CR extension`

API scheme for `LVMCluster` CR:

```go

  // The DeviceAccessPolicy type defines the accessibility of the create lvm2 volume group backing the deviceClass. 
  type DeviceAccessPolicy string
  
  const (
    DeviceAccessPolicyShared DeviceAccessPolicy = "shared"
    DeviceAccessPolicyNodeLocal  DeviceAccessPolicy = "nodeLocal"
  )

  // LVMClusterSpec defines the desired state of LVMCluster
  type LVMClusterSpec struct {
    // Important: Run "make" to regenerate code after modifying this file
    
    // Tolerations to apply to nodes to act on
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
    // Storage describes the deviceClass configuration for local storage devices
    // +Optional
    Storage Storage `json:"storage,omitempty"`
  }
  type Storage struct {
    // DeviceClasses are a rules that assign local storage devices to volumegroups that are used for creating lvm based PVs
    // +Optional
    DeviceClasses []DeviceClass `json:"deviceClasses,omitempty"`
  }
  
  type DeviceClass struct {
    // Name of the class, the VG and possibly the storageclass.
    // Validations to confirm that this field can be used as metadata.name field in storageclass
    // ref: https://github.com/kubernetes/apimachinery/blob/de7147/pkg/util/validation/validation.go#L209
    // +kubebuilder:validation:MaxLength=245
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
    Name string `json:"name,omitempty"`
    
    // DeviceSelector is a set of rules that should match for a device to be included in the LVMCluster
    // +optional
    DeviceSelector *DeviceSelector `json:"deviceSelector,omitempty"`
    
    // NodeSelector chooses nodes on which to create the deviceclass
    // +optional
    NodeSelector *corev1.NodeSelector `json:"nodeSelector,omitempty"`
    
    // ThinPoolConfig contains configurations for the thin-pool
    // +optional
    ThinPoolConfig *ThinPoolConfig `json:"thinPoolConfig,omitempty"`
    
    // Default is a flag to indicate whether the device-class is the default.
    // This will mark the storageClass as default.
    // +optional
    Default bool `json:"default,omitempty"`
    
    // FilesystemType sets the filesystem the device should use.
    // For shared deviceClasses, this field must be set to "" or none.
    // +kubebuilder:validation:Enum=xfs;ext4;none;""
    // +kubebuilder:default=xfs
    // +optional
    FilesystemType DeviceFilesystemType `json:"fstype,omitempty"`
    
    // Policy defines the policy for the deviceClass.
    // TECH PREVIEW: shared will allow accessing the deviceClass from multiple nodes.
    // The deviceClass will then be configured via shared volume group.
    // +optional	  
    // +kubebuilder:validation:Enum=shared;local
+   DeviceAccessPolicy DeviceAccessPolicy `json:"deviceAccessPolicy,omitempty"`
  }

  type ThinPoolConfig struct {
    // Name of the thin pool to be created. Will only be used for node-local storage, 
    // since shared volume groups will create a thin pool with the same name as the volume group.
    // +kubebuilder:validation:Required
    // +required
    Name string `json:"name"`
    
    // SizePercent represents percentage of space in the volume group that should be used
    // for creating the thin pool.
    // +kubebuilder:default=90
    // +kubebuilder:validation:Minimum=10
    // +kubebuilder:validation:Maximum=90
    SizePercent int `json:"sizePercent,omitempty"`
    
    // OverProvisionRatio is the factor by which additional storage can be provisioned compared to
    // the available storage in the thin pool. Only applicable for node-local storage.
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Required
    // +required
    OverprovisionRatio int `json:"overprovisionRatio"`
  }

  // DeviceSelector specifies the list of criteria that have to match before a device is assigned
  type DeviceSelector struct {
    // A list of device paths which would be chosen for creating Volume Group.
    // For example "/dev/disk/by-path/pci-0000:04:00.0-nvme-1"
    // We discourage using the device names as they can change over node restarts.
+   // For multiple nodes, all paths MUST be present on all nodes.
    // +optional
    Paths []string `json:"paths,omitempty"`
  
    // A list of device paths which could be chosen for creating Volume Group.
    // For example "/dev/disk/by-path/pci-0000:04:00.0-nvme-1"
    // We discourage using the device names as they can change over node restarts.
+	// For multiple nodes, all paths SHOULD be present on all nodes.
    // +optional
    OptionalPaths []string `json:"optionalPaths,omitempty"`
  
    // ForceWipeDevicesAndDestroyAllData runs wipefs to wipe the devices.
    // This can lead to data lose. Enable this only when you know that the disk
    // does not contain any important data.
    // +optional
    ForceWipeDevicesAndDestroyAllData *bool `json:"forceWipeDevicesAndDestroyAllData,omitempty"`
  }
```

## Design Details on volume group orchestration and management via vgmanager

TBD

## Design Details for Status Reporting

TBD

### Test Plan

- The integration tests for the LVMS already exist. These tests will need to be updated to test this feature.
- The tests must ensure that detection of devices are working/updating correctly.

### Graduation Criteria

TBD

#### Removing a deprecated feature

- None of the features are getting deprecated

### Upgrade / Downgrade Strategy

TBD

### Version Skew Strategy

TBD

## Implementation History

TBD

## Drawbacks

TBD

## Alternatives

TBD