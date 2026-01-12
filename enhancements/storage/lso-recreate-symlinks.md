---
title: lso-recreate-symlinks
authors:
  - "@dobsonj"
reviewers:
  - "@openshift/storage"
approvers:
  - "@jsafrane"
  - "@gnufied"
api-approvers:
  - "None"
creation-date: 2025-11-06
last-updated: 2025-12-16
tracking-link:
  - "https://issues.redhat.com/browse/STOR-2682"
see-also:
  - "https://issues.redhat.com/browse/OCPBUGS-61988"
  - "https://issues.redhat.com/browse/OCPBUGS-63310"
  - "https://issues.redhat.com/browse/OCPBUGS-60033"
  - "https://bugzilla.redhat.com/show_bug.cgi?id=2414811"
replaces:
superseded-by:
---

# LSO: Recreate Symlinks

## Summary

Local Storage Operator (LSO) creates symlinks under `/mnt/local-storage` pointing to local disks by their `/dev/disk/by-id` path, and then creates a PersistentVolume (PV) pointing to each `/mnt/local-storage` symlink. This design presumed that the `/dev/disk/by-id` links were stable, and while they are more stable across reboots than `/dev/sdb` for example, it is still possible for the underlying by-id symlinks to change based on udev rule changes or even firmware updates from hardware vendors. LSO needs a documented and supported way to recover from these `/dev/disk/by-id` symlink changes on the node.

## Motivation

[RHEL docs](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_storage_devices/persistent-naming-attributes_managing-storage-devices#persistent-attributes-for-identifying-file-systems-and-block-devices_persistent-naming-attributes) state that "Device names managed by udev in /dev/disk/ can change between major releases, requiring link updates." and we have a specific case that became apparent in [OCPBUGS-61988](https://issues.redhat.com/browse/OCPBUGS-61988) that we need to mitigate for RHEL 10.

sg3_utils 1.48 in RHEL 10 disables a udev rule that creates `/dev/disk/by-id/scsi-0NVME_*` symlinks on RHEL 9.x. This udev rule is already problematic for customers affected by OCPBUGS-61988 but it is also problematic for others who may already be using those symlinks successfully today because upgrading to RHEL 10 will cause those `/dev/disk/by-id` symlinks to disappear. LSO needs some way to recreate symlinks for existing PV's when this happens.

### User Stories

* As an administrator, I want the option to switch to more stable symlinks for LSO PV's prior to upgrading OCP to a version which may cause existing by-id symlinks to change.
* As an administrator, I want a way to recover existing LSO PV's when existing by-id symlinks change unexpectedly.

### Goals

* Preventative opt-in to recreate symlinks for existing LSO PV's based on the by-id symlink recommended by LSO as the most stable in relative terms.
* Recovery mechanism to recreate symlinks for existing LSO PV's after the by-id symlinks have already changed.
* Avoid manual changes on the node or other one-off recovery workarounds.

### Non-Goals

* LSO will not format or partition the device to maintain its own on-disk identifier outside of consumer control because this fundamentally alters the contract LSO offers to consumers.
* LSO will not automatically recreate symlinks without explicit administrator opt-in because there may be other valid ways to resolve the issue and LSO does not have the context to make the right call in all cases.
* LSO will not (at this time) offer a more fine-grained filtering mechanism to allow administrators to influence which symlinks should be preferred by LSO.

## Proposal

### Workflow Description

This enhancement introduces a new LocalVolumeDeviceLink CRD. LSO will create new LocalVolumeDeviceLink objects for each PV created by LSO's diskmaker. Diskmaker will detect all valid by-id symlinks for each PV and update `localVolumeDeviceLink.status`. This status will include the full list of valid symlink targets, the by-id symlink currently used by the PV, the preferred symlink chosen by LSO, and the corresponding UUID of the filesystem if one is found.

These status fields will be updated periodically from diskmaker's existing reconcile loop, which runs on start up and on changes to the PV or the owner object (LocalVolume / LocalVolumeSet). Additionally, we want reconcile to be triggered by udev disk changes as an indication that by-id symlinks may have changed.

#### Alert

LSO will throw an alert if:
1) `localVolumeDeviceLink.spec.policy == None` and `localVolumeDeviceLink.status.currentLinkTarget != localVolumeDeviceLink.status.preferredLinkTarget`. This means the administrator has not yet specified a policy and the current target is different from the preferred target.
2) `localVolumeDeviceLink.spec.policy == None` and no by-id symlink is found at all. This means there is no `/dev/disk/by-id` symlink found for the device.

#### Administrator response

The administrator can review the device link status and make a choice:
1) set `localVolumeDeviceLink.spec.policy` to `CurrentLinkTarget` to keep the current symlink as it is and silence the alert.
2) set `localVolumeDeviceLink.spec.policy` to `PreferredLinkTarget` to tell diskmaker to recreate the symlink using the target from `localVolumeDeviceLink.status.preferredLinkTarget`. This can be done to proactively switch to the recommended symlink, or it can be done reactively when a symlink is no longer valid (assuming there is another known valid by-id symlink).

#### Recreate Symlink

If `localVolumeDeviceLink.spec.policy == PreferredLinkTarget`, diskmaker will recreate the symlink pointing to the preferred link target. If successful, it will update `localVolumeDeviceLink.status`. If there is any error that prevents this, diskmaker will add a failure condition and retry as part of the reconcile loop.

If the value of `localVolumeDeviceLink.status.preferredLinkTarget` changes later and `localVolumeDeviceLink.spec.policy` is still set to `PreferredLinkTarget`, diskmaker will again reconcile the symlink using the new target found in `localVolumeDeviceLink.status.preferredLinkTarget`.

To stop diskmaker from attempting to change the symlink, the administrator can set `localVolumeDeviceLink.spec.policy` to `None` or `CurrentLinkTarget`.

#### Deletion

LocalVolumeDeviceLink will be deleted when its owner object is deleted (LocalVolume / LocalVolumeSet).

### API Extensions

```
type LocalVolumeDeviceLink struct {
	metav1.TypeMeta `json:",inline"`
	// has OwnerRef set to LocalVolume or LocalVolumeSet object
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LocalVolumeDeviceLinkSpec   `json:"spec,omitempty"`
	Status LocalVolumeDeviceLinkStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:Enum=None;CurrentLinkTarget;PreferredLinkTarget
type LocalVolumeDeviceLinkPolicy string

const (
	LocalVolumeDeviceLinkPolicyNone = "None" // default
	LocalVolumeDeviceLinkPolicyCurrentLinkTarget = "CurrentLinkTarget"
	LocalVolumeDeviceLinkPolicyPreferredLinkTarget = "PreferredLinkTarget"
)

type LocalVolumeDeviceLinkSpec struct {
	// PersistentVolumeName is the name of the persistent volume linked to the device
	PersistentVolumeName string `json:"persistentVolumeName"`
	// Policy of the device link
	Policy LocalVolumeDeviceLinkPolicy `json:"policy"`
}

type LocalVolumeDeviceLinkStatus struct {
	// CurrentLinkTarget is the current by-id symlink used for the device
	CurrentLinkTarget string `json:"currentLinkTarget,omitempty"`
	// PreferredLinkTarget is the preferred by-id symlink for the device
	PreferredLinkTarget string `json:"preferredLinkTarget,omitempty"`
	// ValidLinkTargets is the list of valid by-id symlinks for the device
	ValidLinkTargets []string `json:"validLinkTargets,omitempty"`
	// FilesystemUUID is the UUID of the filesystem found on the device (when available)
	// +optional
	FilesystemUUID string `json:"filesystemUUID,omitempty"`
	// Conditions is a list of operator conditions
	Conditions []operatorv1.OperatorCondition `json:"conditions,omitempty"`
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A, not topology specific.

#### Standalone Clusters

N/A, not topology specific.

#### Single-node Deployments or MicroShift

N/A, not topology specific.

### Implementation Details/Notes/Constraints

Diskmaker will use the following selection criteria when choosing the recommended symlink for each PV and return the first valid link from the by-id list that meets this criteria:

1. The link must be in the sorted by-id list, starting from highest priority based on the [prefix priority list](https://github.com/openshift/local-storage-operator/blob/397dcaa02032f52916aa3ed49e5b9ecd1b96cc01/pkg/internal/diskutil.go#L177-L199).
2. If the current link target still exists, the new by-id target must point to the same device.
3. If a by-uuid symlink was previously discovered and recorded, the new by-id target must point to the same device.
4. There is no other symlink in `/mnt/local-storage/<storageclass>` pointing to this by-id target.

Diskmaker will keep the link name and only change the link target. For example, if a PV has an existing symlink `/mnt/local-storage/localblock/scsi-0NVME_MODEL_abcde` pointing to `/dev/disk/by-id/scsi-0NVME_MODEL_abcde`, but there is a by-id link `/dev/disk/by-id/scsi-2ace42e0035eabcde`, setting the device link policy to `PreferredLinkTarget` will cause diskmaker to replace `/mnt/local-storage/localblock/scsi-0NVME_MODEL_abcde` with a new symlink pointing to `/dev/disk/by-id/scsi-2ace42e0035eabcde`.

### Risks and Mitigations

What happens if a symlink is changed while a workload is still running? This actually works as long as the new symlink is created first and then moved to the existing `/mnt/local-storage` link name. An existing process can hold a reference to the old (replaced) symlink until the process exits, and a new process will read the new symlink after `mv`.

Why opt-in? There may be other valid ways to resolve the issue and LSO does not have the context to make the right call in all cases. Instead of letting diskmaker change symlinks automatically, we want to call administrator attention to the problem and let them make an informed decision about their environment.

### Drawbacks

The biggest drawback is that without relying on some on-disk metadata LSO still relies on at least one symlink remaining stable across upgrades. This design is of limited help if _all_ of the by-id symlinks change between two releases.

### Future Work

We have some ideas to improve this in the future if there is a need, but they are out-of-scope for the initial implementation:

* Fine-grained filtering mechanism to influence which by-id symlink is used
* Other policy options for the `localVolumeDeviceLink.spec.policy` field
* Detect ceph bluestore UUID (i.e. `ceph-volume raw list /dev/xyz --format=json`) or by-uuid symlink [Bug 2414811](https://bugzilla.redhat.com/show_bug.cgi?id=2414811).

## Alternatives (Not Implemented)

* The `/mnt/local-storage` disk path stored in the PV is immutable, and we want the ability to fix existing PV's, which means we must keep the same `/mnt/local-storage` link name and update only the `/dev/disk/by-id` link target.
* We can't partition a device without breaking consumers or introducing separate API and does not help existing PV's recover.
* If diskmaker added a label to each device, it could easily be wiped by the disk consumer. It could label a device after kubelet creates a filesystem on it and save it in the device link status but this only helps with filesystem volumes.
* LSO could store udev attributes instead of symlinks but we would need to have some logic to filter useful and useless attributes, not clear if it helps rebuilding symlinks in all cases.
* We considered implementing this using annotations on the PV, but compared to an API it limits our options for maintainability, error reporting, and future enhancements.

## Open Questions [optional]

None

## Test Plan

We'll extend existing LSO unit tests, e2e, extended test suite, and manual testing where needed.

We do not have easy access to many hardware configurations and much of our testing will be limited to simulating these symlink issues.

## Graduation Criteria

This is an optional OLM operator and will be considered GA-ready when the feature is merged.

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

LSO already sets [maxOpenShiftVersion](https://github.com/openshift/local-storage-operator/blob/397dcaa02032f52916aa3ed49e5b9ecd1b96cc01/config/manifests/stable/local-storage-operator.clusterserviceversion.yaml#L118) to N+1, meaning if the cluster is running 4.21 OCP with 4.20 LSO, upgrades to 4.22 OCP will be blocked until LSO is upgraded to 4.21. This is important because we want to deliver these changes to alert of potential symlink issues prior to upgrades to RHEL 10.

## Operational Aspects of API Extensions

N/A

## Support Procedures

TBD

## Infrastructure Needed [optional]

N/A

