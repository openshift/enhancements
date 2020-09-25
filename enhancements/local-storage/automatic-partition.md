---
title: automatic-partition-of-disks-in-Local-Storage-Operator
authors:
  - "@rohan47"
reviewers:
  - "@hekumar"
  - "@sapillai"
  - "@leseb"
  - "@travisn"
  - "@rohantmp"
  - "@jarrpa"
approvers:
  - "@hekumar"
creation-date: 2020-09-25
last-updated: 2020-09-25
status:
---

# Automatic partition of Disks in the Local-Storage-Operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This is a proposal to enable automated partition of local devices via the `LocalVolumeSet` CR

This functionality will be implemented in the local-storage-operator(LSO)

## Motivation

For performance/usage reasons the user will want to use partitions.
On NVMe devices, some of the devices are capable of delevering very high IOPS per device. Many applications IOPS tend to bottleneck long before it reaches anywhere close to the device IOPS limit.
So for an application pod that uses an NVMe device, there will be a big gap between measured or advertised capabilities of these devices and what you can get with multiple application pods consuming partitoins of the same NVMe device.

### Goals

- Partition the selected devices based on the partition policy specified by the user.
- If a LocalVolumeSet has partitioning policy, it will create partitions on all disks that match the device filter.

### Non-Goals

- If anything changes later such as the partition policy or the device is resized, LSO will not add or modify any partitions.
- Policy for wiping device to clean the partitions
- Creating PVs based on both raw devices and partitions in the same LocalVolumeSet is not supported.

### Risks and Mitigations
- This will be handled by [LocalVolumeSet](https://github.com/openshift/enhancements/blob/master/enhancements/local-storage/automatic-detection.md#risks-and-mitigations)

## Proposal

The `local-storage-operator` is already capable of detecting local disks from the nodes and provisioning PVs out of them, but it can't partition them.
The idea is to introduce the feature of partitioning the local disks and using the partitions to provision the PVs.

The administrator can specify if he wants to partition the disks in the LocalVolumeSet CR and then LSO, after detecting the disk, will partition them first before provisioning PVs on those partitions.

### Workflow for Local PV creation via `LocalVolumeSet CR`
- The admin decides they want to create partitions on some devices in the cluster
- He creates a LocalVolumeSet CR that includes the partitioning policy
- Once the `LocalVolumeSet` is created, LSO reconciles the LocalVolumeSet CR (the same as designed for full device),
- The controller will:
  - configure the local-static-provisioner to make a new StorageClass based on certain directories on the selected nodes.
  - assign diskmaker daemonset to the selected nodes.
  - there will be one daemonset for each `LocalVolumeSet` CR.
- The diskmaker daemon will find devices that match the discovery policy and check if partitioningSpec is present
  - if partitionSpec is present,
    - it will check for any existing configmaps which reference the same LocalVolumeSet and the node name.
    - if a configmap is present, it will check the devices and their partitioning status and partition the unpartitioned devices.
    - else it will create a configmap for its respective node having a list of devices and its partitioning state set to unpartitioned.
    - it will partition the devices.
  - as soon as a device is partitioned
    - the diskmaker will symlink the child partitions of the device into the directory that the local-static-provisioner is watching
    - will update the partitioning status for that device to done in the configmap
    - The local-static-provisioner will provision PVs on those partitions
- The localVolumeset won't accept changes after its created and has a partitioningSpec


## Design Details for Auto partitioning of local devices

Updated `LocalVolumeSet` API scheme for Auto partition:


```go


type LocalVolumeSetSpec struct {
 ...
  // PartitioningSpec is the partitioning policy according to which the selected devices will be partitioned.
  PartitioningSpec PartitoiningSpec `json:"partitioningSpec"`


  // Remaining fields in LocalVolumeSetStatus will be same as the implemented LocalVolumeSetSpec
}

type PartitioningSpec struct {
  // PartSize determines the exact size of each partition.
  PartSize *resource.Quantity `json:"partSize,omitempty"`
  Count    int64              `json:"count,omitempty"`
}

```

## Partitioning policy

* Size: If only size is specified, create partitions for the whole disk all of that size. If there is space remaining, create a PV for the remaining amount.
* Count: If only count is specified, divide the disk up into that many partitions and calculate the size.
* If both count and size are specified, only create that number of partitions of the desired size and leave the remaining disk unpartitioned.

### Example of LocalVolumeSet CR:

```yaml
apiVersion: local.storage.openshift.io/v1alpha1
kind: LocalVolumeSet
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
  maxDeviceCount: 10
  deviceInclusionSpec:
    deviceTypes:
      - RawDisk
    deviceMechanicalProperty:
      - Rotational
      - NonRotational
    minSize: 30G # minSize should be >= partitioningSpec.size
    maxSize: 100G # maxSize should be >= partitioningSpec.size
  partitioningSpec:
    size: 30G
    count: 3
status:
  conditions:
  - lastTransitionTime: "2020-03-17T09:33:43Z"
    status: "True"
    type: Available
  totalProvisionedDeviceCount: 5
  totalProvisionedPartitionCount: 15
```

As per the above example the administrator wants partitions of size 30G and wants 3 partitions of each device.
Lets assume that there are 5 devices of 100G  that are detected and are fit for partitioning, so each device will be partitioned into 3 parts of 30G each and 10G of each device will remain unpartitioned.

### Test Plan

- The integration tests for the LSO already exist. These tests will need to be updated to test this feature.
- The tests must ensure that partitioning of devices and provisioning of PVs is working correctly.
- The tests must ensure that data corruption are not happening during auto detection of devices.

### Graduation Criteria

- Documentation exists for the behaviour of each configuration item.
- Unit and End to End tests coverage is sufficient.

##### Removing a deprecated feature

- None of the features are getting deprecated

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

- Manually partitioning the devices and then using them. This will be a lot of work if the number of devices to partition is big.
- Having a seperate CR for partitoning and leaving the localVolumeset design alone.(This is open for a debate)

