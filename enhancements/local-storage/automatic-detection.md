---
title: automatic-detection-of-disks-in-Local-Storage-Operator
authors:
  - "@aranjan"
  - "@rohantmp"
reviewers:
  - "@jrivera"
  - "@jsafrane"
  - "@hekumar"
  - "@chuffman"
  - "@rojoseph"
  - "@sapillai"
  - "@leseb"
  - "@travisn"
approvers:
  - "@jrivera"
  - "@jsafrane"
  - "@hekumar"
  - "@chuffman"
creation-date: 2020-01-21
last-updated: 2020-02-24
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

When working with bare-metal OpenShift clusters, due to the absence of storage provisioners like those in the cloud, there is a lot of manual work to be done for consuming local storage from nodes.
The idea is to automate the manual steps that are required for consuming local storage in an OpenShift cluster by extending the Local Storage Operator (LSO).

## Motivation

To automatically detect local devices and provision local volumes out of them.

### Goals

- Automatic detection of available disks from nodes which can be used as PVs for OpenShift-cluster.
- Respond to the attach/detach events of the disks/devices.
- Have options for filtering particular kind of disks based on properties such as name, size, manufacturer, etc.

### Non-Goals

- Inherently protect against confilict with provisioners that own local devices on the same nodes automatic detection is configured to run.

## Proposal

The `local-storage-operator` is already capable of consuming local disks from the nodes and provisioning PVs out of them,
but the disk/device paths needs to explicitly specified in the `LocalVolume` CR.
The idea is to introduce a new Custom Resource, `AutoDetectVolumes`, that enables automated discovery and provisioning of local volume based PVs on a set of nodes addressed by one or more labelSelectors.

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

## Design Details

API scheme for `AutoDetectVolumes`:

```go

type AutoDetectVolumeList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata"`
    Items           []AutoDetectVolume `json:"items"`
}

type AutoDetectVolume struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata"`
    Spec              AutoDetectVolumeSpec   `json:"spec"`
    Status            AutoDetectVolumeStatus `json:"status,omitempty"`
}

type AutoDetectVolumeSpec struct {
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

type AutoDetectVolumeStatus struct {
    Status string `json:"status"`
}

// DeviceMechanicalProperty holds the device's mechanical spec. It can be rotational, nonRotational or
// RotationalAndNonRotational
type DeviceMechanicalProperty string

const (
    // The mechanical properties of the devices
    // Rotational refers to magnetic disks
    Rotational DeviceMechanicalProperty = "Rotational"
    // NonRotational refers to ssds
    NonRotational DeviceMechanicalProperty = "NonRotational"
    // RotationalAndNonRotational refers to both magnetic and ssds
    RotationalAndNonRotational DeviceMechanicalProperty = ""
)

type DeviceDiscoveryPolicyType string

const (
    // The DeviceDiscoveryPolicies that will be supported by the LSO.
    // These Discovery policies will based on lsblk's type output
    // Disk represents a device-type of disk
    Disk DeviceDiscoveryPolicyType = "disk"
    // Part represents a device-type of partion
    Part DeviceDiscoveryPolicyType = "part"
)

type DeviceInclusionSpec struct {
    // DeviceTypes that should be used for automatic detection. This would be one of the types supported
    // by the local-storage operator. Currently the supported types are: disk,part
    // If the list is empty the default value will be `[disk]`.
    DeviceTypes []DeviceDiscoveryPolicyType `json:"deviceType"`

    // DeviceMechanicalProperty denotes whether Rotational or NonRotational disks should be used.
    // by default, RotationalAndNonRotational which matches all disks.
    // +optional
    DeviceMechanicalProperty DeviceMechanicalProperty `json:"deviceMechanicalProperty"`

    // MinSize is the minimum size of the device which needs to be included
    // +optional
    MinSize resource.Quantity `json:"minSize"`

    // MaxSize is the maximum size of the device which needs to be included
    // +optional
    MaxSize resource.Quantity `json:"maxSize"`

    // Models is a list of device models. If not empty, the device's model as outputted by lsblk needs
    // to contain at least one of these strings.
    // +optional
    Models []string `json:"models"`

    // Vendors is a list of device vendors. If not empty, the device's model as outputted by lsblk needs
    // to contain at least one of these strings.
    // +optional
    Vendors []string `json:"vendors"`
}
```

The existing LSO daemon will create PVs that match the `AutoDetectVolume` criteria.

### Test Plan

- The integration tests for the LSO already exist. These tests will need to be updated to test this feature.
- The tests must ensure that detection of devices are working/updating correctly.
- The tests must ensure that data corruption are not happening during auto detection of devices.

### Graduation Criteria

- Documentation exists for the behaviour of each configuration item.
- Unit and End to End tests coverage is sufficient.

#### Examples

```yaml
apiVersion: local.storage.openshift.io/v1alpha1
kind: AutoDetectVolume
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
```

```yaml
apiVersion: local.storage.openshift.io/v1alpha1
kind: AutoDetectVolume
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
```

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