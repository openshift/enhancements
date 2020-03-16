---
title: automatic-detection-of-disks-in-Local-Storage-Operator
authors:
  - "@ashishranjan738"
  - "@rohantmp"
  - "@jarrpa"
reviewers:
  - "@jsafrane"
  - "@hekumar"
  - "@chuffman"
  - "@sapillai"
  - "@leseb"
  - "@travisn"
  - "@rohantmp"
  - "@jarrpa"
approvers:
  - "@jsafrane"
  - "@hekumar"
  - "@chuffman"
creation-date: 2020-01-21
last-updated: 2020-03-16
status: implementable
---

# Automatic detection of Disks in Local-Storage-Operator 

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This is a proposal to enable 
- automated provisioning of LocalPVs sorting them into StorageClasses based on their characteristics via the `LocalVolumeGroup` CRD. 

This CRD will be implemented in the LocalStorageOperator(LSO)

## Motivation

The existing method of manually managing disks on nodes does not scale, and cannot be consumed by another operator.
Choosing devices to be used should be handled on the platform so that multiple consumers of local storage don't conflict.
To enable layering, we need information about potential devices, so that we have information about availability,
and we can use that information to sort devices into storage classes based on their capabilities.

### Goals

- Discovery of potentially usable local disks on chosen nodes.
- Automatic sorting of available disks into StorageClasses based on their characteristics.
- Detect newly attached disks

### Non-Goals

- Inherently protect against confilict with provisioners that own local devices on the same nodes automatic detection is configured to run.

### Risks and Mitigations

- LSO will detect disks that contain data and/or are in-use via ensuring that the device:
  - can be openened exclusively.
  - is not read-only.
  - is not removable.
  - has no child partitions.
  - has no FS signature.
  - state (as outputted by `lsblk`) is not `suspended`
- Ensuring disks aren't re-detected as new or otherwise destroyed if their device path changes.
  - This is already ensured by the current LSO approach of consuming disks by their `UUID`
- This will match all newly attached AWS EBS PVs, just before kubelet formats them.
- This will match all local disks that already have PVs, but these PVs are not bound / used yet.`


## Proposal

The `local-storage-operator` is already capable of consuming local disks from the nodes and provisioning PVs out of them,
but the disk/device paths needs to explicitly specified in the `LocalVolume` CR.
The idea is to introduce a new feature in LSO
  - Auto provisioning of localDevices
    - This will involve introduction of a new CR called `LocalVolumeGroup`.
    - The pupose of this will be to auto discover and provision PVs on devices which match the inclusion filter present in the CR.
    - This will involve a continous process of discovery of devices via the diskmaker daemons. Any discovered devices which matches the inclusion filter will be considered for provisioning of PVs.

## Design Details for `Auto provisioning of local devices`
API scheme for `LocalVolumeGroups`:

```go
type LocalVolumeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []LocalVolumeGroup `json:"items"`
}

type LocalVolumeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              LocalVolumeGroupSpec   `json:"spec"`
	Status            LocalVolumeGroupStatus `json:"status,omitempty"`
}

type LocalVolumeGroupSpec struct {
	// Nodes on which the automatic detection policies must run.
	// +optional
	NodeSelector *corev1.NodeSelector `json:"nodeSelector,omitempty"`
	// StorageClassName to use for set of matched devices
	StorageClassName string `json:"storageClassName"`
	// MinDeviceCount is the minumum number of devices that needs to be detected per node.
	// If the match devices are less than the minCount specified then no PVs will be created.
	MinDeviceCount int `json:"minDeviceCount"`
	// +optional
	// Maximum number of Devices that needs to be detected per node.
	MaxDeviceCount int `json:"maxDeviceCount"`
	// VolumeMode determines whether the PV created is Block or Filesystem. By default it will
	// be block
	// + optional
	VolumeMode PersistentVolumeMode `json:"volumeMode,omitempty"`
	// FSType type to create when volumeMode is Filesystem
	// +optional
	FSType string `json:"fsType,omitempty"`
	// If specified, a list of tolerations to pass to the discovery daemons.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// DeviceInclusionSpec is the filtration rule for including a device in the device discovery
	// +optional
	DeviceInclusionSpec *DeviceInclusionSpec `json:"deviceInclusionSpec"`
}

// DeviceMechanicalProperty holds the device's mechanical spec. It can be rotational or nonRotational
type DeviceMechanicalProperty string

const (
	// The mechanical properties of the devices
	// Rotational refers to magnetic disks
	Rotational DeviceMechanicalProperty = "Rotational"
	// NonRotational refers to ssds
	NonRotational DeviceMechanicalProperty = "NonRotational"
)

type DeviceType string

const (
	// The DeviceTypes that will be supported by the LSO.
	// These Discovery policies will based on lsblk's type output
	// Disk represents a device-type of disk
	Disk DeviceType = "Disk"
	// Part represents a device-type of partion
	Partition DeviceType = "Part"
)

type DeviceTypeList struct {
	// Devices is the list of devices that should be used for automatic detection.
	// This would be one of the types supported by the local-storage operator. Currently,
	// the supported types are: disk, part. If the list is empty the default value will be `[disk]`.
	Devices []DeviceType `json:"devices"`
}

type DeviceInclusionSpec struct {
	// DeviceTypeList holds the list of devices that should be used for automatic detection.
	// If Nil the default detection will be of disks.
	DeviceTypeList *DeviceTypeList `json:"deviceTypeList"`

	// DeviceMechanicalProperty denotes whether Rotational or NonRotational disks should be used.
	// by default, it selects both
	// +optional
	DeviceMechanicalProperty DeviceMechanicalProperty `json:"deviceMechanicalProperty"`

	// MinSize is the minimum size of the device which needs to be included
	// +optional
	MinSize *resource.Quantity `json:"minSize"`

	// MaxSize is the maximum size of the device which needs to be included
	// +optional
	MaxSize *resource.Quantity `json:"maxSize"`

	// Models is a list of device models. If not empty, the device's model as outputted by lsblk needs
	// to contain at least one of these strings.
	// +optional
	Models []string `json:"models"`

	// Vendors is a list of device vendors. If not empty, the device's model as outputted by lsblk needs
	// to contain at least one of these strings.
	// +optional
	Vendors []string `json:"vendors"`
}

type LocalVolumeGroupPhase string

const (
	DiscoveringPhase LocalVolumeGroupPhase = "Discovering"
	FailedPhase      LocalVolumeGroupPhase = "Failed"
	DiscoveredPhase  Phase                 = "Discovered"
	ProvisionedPhase LocalVolumeGroupPhase = "Provisioned"
)

type LocalVolumeGroupStatus struct {
	// Phase describes the state of the LocalVolumeGroup
	Phase LocalVolumeGroupPhase `json:"phase,omitempty"`

	// A human-readable message indicating details about why the LocalVolumeGroup is in this state.
	// +optional
	Message string `json:"message,omitempty"`

	// Reason is a brief CamelCase string that describes any failure and is meant
	// for machine parsing and tidy display in the CLI.
	// +optional
	Reason string `json:"reason,omitempty"`

	// TotalDiscoveredDeviceCount is the count of the total devices which matched the inclusion filter
	TotalDiscoveredDeviceCount *int32 `json:"totalDiscoveredDeviceCount,omitempty"`

	// TotalProvisionedDeviceCount is the count of the total devices over which the PVs has been provisioned
	TotalProvisionedDeviceCount *int32 `json:"totalProvisionedDeviceCount,omitempty"`

  // LastProvisionedTimeStamp is the timeStamp value for lastProvisionedTimeStamp
  // +optional
	LastProvisionedTimeStamp `json:"lastProvisionedTimeStamp,omitempty"`

	// observedGeneration is the last generation change the operator has dealt with
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
```
### Workflow of LocalPV creation via LocalVolumeGroup CR
- Once the `LocalVolumeGroup` is created, the controller will:
  - configure the local-static-provisioner to make a new StorageClass based on certain directories on the selected nodes.
  - assign diskmaker daemons to the selected nodes.
- The diskmaker daemon will find devices that match the disovery policy and symlink them into the directory that the local-static-provisioner is watching.
- Possible `status.phase` values for `LocalVolumeGroup`
  - `Discovering`
  - `Failed`
  - `Discovered`
  - `Provisioned`
  
Example of `LocalVolumeGroup` CR:

```yaml
apiVersion: local.storage.openshift.io/v1alpha1
kind: LocalVolumeGroup
metadata:
  name: example-autodetect
spec:
  nodeSelector:
    nodeSelectorTerms:
      - matchExpressions:
          - key: kubernetes.io/hostname
            operator: In
            values:
              - worker-0
              - worker-1
  storageClassName: example-storageclass
  volumeMode: Block
  minDeviceCount: 5
  maxDeviceCount: 10
  deviceInclusionSpec:
    deviceTypes:
      - disk
    deviceMechanicalProperty: Rotational
    minSize: 10G
    maxSize: 100G
status:
  phase: Provisioned
  totalDiscoveredDeviceCount: 4
  totalProvisionedDeviceCount: 4
  timeStamp: '2020-03-09T08:37:19Z'
```

```yaml
apiVersion: local.storage.openshift.io/v1alpha1
kind: LocalVolumeGroup
metadata:
  name: example-autodetect
spec:
  nodeSelector:
    nodeSelectorTerms:
      - matchExpressions:
          - key: kubernetes.io/hostname
            operator: In
            values:
              - worker-0
              - worker-1
  storageClassName: example-storageclass
  volumeMode: filesystem
  fstype: ext4
  minDeviceCount: 5
  maxDeviceCount: 10
  deviceInclusionSpec:
    deviceTypes:
      - disk
      - nvme
    deviceMechanicalProperty: NonRotational
    minSize: 10G
    maxSize: 100G
    models:
      - SAMSUNG
      - Crucial_CT525MX3
    vendors:
      - ATA
      - ST2000LM
status:
  phase: Provisioned
  totalDiscoveredDeviceCount: 4
  totalProvisionedDeviceCount: 4
  timeStamp: '2020-03-09T08:37:19Z'
```

### Test Plan

- The integration tests for the LSO already exist. These tests will need to be updated to test this feature.
- The tests must ensure that detection of devices are working/updating correctly.
- The tests must ensure that data corruption are not happening during auto detection of devices.

### Graduation Criteria

- Documentation exists for the behaviour of each configuration item.
- Unit and End to End tests coverage is sufficient.

##### Removing a deprecated feature

- None of the features are getting deprecated

### Upgrade / Downgrade Strategy

Since this requires a new implementation no new upgrade strategy will be required.

### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

- Existing manual creation of LocalVolume CRs. With the node selector on the LocalVolume, a single CR can apply to an entire class of nodes (i.e., a machineset or a physical rack of homogeneous hardware). When a machineset is defined, a corresponding LocalVolume can also be created.
- Directly enhancing the LocalVolume CR to allow for auto discovery

The first approach requires some manual work and knowledge of underlying nodes, this makes it inefficient for large clusters. The second approach can introduce breaking change to the existing GA API.
Therefore this approach makes sense.