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
last-updated: 2026-01-20
tracking-link:
  - "https://issues.redhat.com/browse/STOR-2682"
see-also:
  - "https://issues.redhat.com/browse/OCPBUGS-61988"
  - "https://issues.redhat.com/browse/OCPBUGS-63310"
  - "https://bugzilla.redhat.com/show_bug.cgi?id=2414811"
replaces:
superseded-by:
---

# LSO: Recreate Symlinks

## Summary

Local Storage Operator (LSO) creates symlinks under `/mnt/local-storage` pointing to local disks by their `/dev/disk/by-id` path, and then creates a PersistentVolume (PV) pointing to each `/mnt/local-storage` symlink. This design presumed that the `/dev/disk/by-id` links were stable, and while they are more stable across reboots than `/dev/sdb` for example, it is still possible for the underlying by-id symlinks to change based on udev rule changes or even firmware updates from hardware vendors. Refer to the OCPBUGS links in the `see-also` section for examples of known cases where this can happen. LSO needs a documented and supported way to recover from these `/dev/disk/by-id` symlink changes on the node.

## Motivation

[RHEL docs](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_storage_devices/persistent-naming-attributes_managing-storage-devices#persistent-attributes-for-identifying-file-systems-and-block-devices_persistent-naming-attributes) state that "Device names managed by udev in /dev/disk/ can change between major releases, requiring link updates." and we have specific cases where this can happen that we need to mitigate for OCP when upgrading the nodes to RHEL 10. LSO needs some way to recreate symlinks for existing PVs in these scenarios.

* [OCPBUGS-61988](https://issues.redhat.com/browse/OCPBUGS-61988): sg3_utils 1.48 in RHEL 10 disables a udev rule that creates `/dev/disk/by-id/scsi-0NVME_*` symlinks on RHEL 9.x. This udev rule is already problematic for customers affected by OCPBUGS-61988 but it is also problematic for others who may already be using those symlinks successfully today because upgrading to RHEL 10 will cause those `/dev/disk/by-id` symlinks to disappear.
* [OCPBUGS-63310](https://issues.redhat.com/browse/OCPBUGS-63310): A vendor firmware change caused device symlinks to change, and the kernel added a quirk for the affected drives to correct the symlinks. However, if either change is applied without the other (firmware or kernel) then the symlinks used by existing PVs can change.

### User Stories

* As an administrator, I want the option to switch to more stable symlinks for LSO PVs prior to upgrading OCP to a version which may cause existing by-id symlinks to change. LSO must alert when manual intervention is needed prior to upgrade.
* As an administrator, I want a way to recover existing LSO PVs when existing by-id symlinks change unexpectedly. For example, a firmware upgrade may cause a symlink change outside of the usual OCP upgrade path.

### Goals

* Provide an option to switch to more stable symlinks for existing LSO PVs prior to upgrading OCP to a version which may cause existing by-id symlinks to change.
* Provide a recovery mechanism to recreate symlinks for existing LSO PVs after the by-id symlinks have already changed unexpectedly, either by firmware upgrade or some other event that LSO can not prevent.
* Avoid one-off recovery workarounds, like using `oc debug` to log into the node and manually change symlinks.

### Non-Goals

* LSO will not format or partition the device to maintain its own on-disk identifier outside of consumer control because this fundamentally alters the contract LSO offers to consumers.
* LSO will not automatically recreate symlinks without explicit administrator opt-in because there may be other valid ways to resolve the issue and LSO does not have the context to make the right call in all cases.
* LSO will not (at this time) offer a more fine-grained filtering mechanism to allow administrators to influence which symlinks should be preferred by LSO.

## Proposal

### Workflow Description

This enhancement introduces a new LocalVolumeDeviceLink CRD. LSO will create new LocalVolumeDeviceLink objects for each PV created by LSO's diskmaker. Diskmaker will detect all valid by-id symlinks for each PV and update `localVolumeDeviceLink.status`. This status will include:

* `localVolumeDeviceLink.status.validLinkTargets`: The full list of valid symlink targets, since there may be multiple by-id symlinks pointing to the same physical device.
* `localVolumeDeviceLink.status.currentLinkTarget`: The by-id symlink currently used by the PV.
* `localVolumeDeviceLink.status.preferredLinkTarget`: The preferred symlink chosen by diskmaker.
* `localVolumeDeviceLink.status.filesystemUUID`: The corresponding UUID of the filesystem if one is found.

These status fields will be updated periodically from diskmaker's existing reconcile loop, which runs on start up and on changes to the PV or the owner object (LocalVolume / LocalVolumeSet). Much of the PV spec is immutable, but changes to the PV status for example will trigger reconcile.

Additionally, we want reconcile to be triggered by udev disk changes as an indication that by-id symlinks may have changed. The implementation of this enhancement will include some logic to rate-limit the number of reconcile calls per second in cases where the rate of udev events is unpredictably high.

#### Alert

LSO will throw an alert if:
1) `localVolumeDeviceLink.spec.policy == None` and `localVolumeDeviceLink.status.currentLinkTarget != localVolumeDeviceLink.status.preferredLinkTarget`. This means the administrator has not yet specified a policy and the current target is different from the preferred target.
2) `localVolumeDeviceLink.spec.policy == None` and no by-id symlink is found at all. For example, the PV does exist and uses `/dev/sdb` directly but there is no `/dev/disk/by-id` symlink found for the device.

#### Administrator response

The administrator can review the device link status and make a choice:
1) set `localVolumeDeviceLink.spec.policy` to `CurrentLinkTarget` to keep the current symlink as it is and silence the alert.
2) set `localVolumeDeviceLink.spec.policy` to `PreferredLinkTarget` to tell diskmaker to recreate the symlink using the target from `localVolumeDeviceLink.status.preferredLinkTarget`. This can be done to proactively switch to the preferred symlink, or it can be done reactively when a symlink is no longer valid (assuming there is another known valid by-id symlink).

Only the administrator will update `localVolumeDeviceLink.spec.policy`. Diskmaker and other LSO components will not try to change it.

#### Recreate Symlink

If `localVolumeDeviceLink.spec.policy == PreferredLinkTarget`, diskmaker will recreate the symlink pointing to the preferred link target. If successful, it will update `localVolumeDeviceLink.status`. If there is any error that prevents this, diskmaker will add a failure condition and retry as part of the reconcile loop.

If diskmaker updates the value of `localVolumeDeviceLink.status.preferredLinkTarget` while `localVolumeDeviceLink.spec.policy` is still set to `PreferredLinkTarget`, diskmaker will again reconcile the symlink using the new target found in `localVolumeDeviceLink.status.preferredLinkTarget`.

To stop diskmaker from attempting to change the symlink, the administrator can set `localVolumeDeviceLink.spec.policy` to `None` or `CurrentLinkTarget`.

#### Lifecycle

LocalVolumeDeviceLink will be deleted when its owner object is deleted (LocalVolume / LocalVolumeSet).
The PV that the LocalVolumeDeviceLink refers to may be deleted and recreated multiple times, but the chosen policy in LocalVolumeDeviceLink must persist once it is set.

We do not expect accumulating an infinite number of LocalVolumeDeviceLink objects because of the deterministic nature of PV creation.
Diskmaker creates PVs with a name based on the basename of the symlink under /mnt/local-storage, the node name, and the storageclass (see [GeneratePVName](https://github.com/openshift/local-storage-operator/blob/c930ea412cde390f45acaa9643da69f12fbeb57e/pkg/common/provisioner_utils.go#L236-L246)).
Since these parameters do not change for existing devices, the PV name stays exactly the same in the create-delete-create cycle.

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

Additionally, `LocalVolume` and `LocalVolumeSet` will both have a new field to optionally set a default policy for the LocalVolumeDeviceLink objects it creates.

```
type LocalVolumeSpec struct {
    ...
    DefaultDeviceLinkPolicy LocalVolumeDeviceLinkPolicy `json:"defaultDeviceLinkPolicy"`
}

type LocalVolumeSetSpec struct {
    ...
    DefaultDeviceLinkPolicy LocalVolumeDeviceLinkPolicy `json:"defaultDeviceLinkPolicy"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A, not topology specific.

#### Standalone Clusters

N/A, not topology specific.

#### Single-node Deployments or MicroShift

N/A, not topology specific.

#### OpenShift Kubernetes Engine

N/A, not topology specific.

### Implementation Details/Notes/Constraints

Diskmaker will use the following selection criteria when choosing the preferred symlink for each PV and return the first valid link from the by-id list that meets this criteria:

1. The link must be in the sorted by-id list, starting from highest priority based on the [prefix priority list](https://github.com/openshift/local-storage-operator/blob/397dcaa02032f52916aa3ed49e5b9ecd1b96cc01/pkg/internal/diskutil.go#L177-L199).
2. If the current link target still exists, the new by-id target must point to the same device.
3. If a by-uuid symlink was previously discovered and recorded, the new by-id target must point to the same device.
4. There is no other symlink in `/mnt/local-storage/<storageclass>` pointing to this by-id target.

Diskmaker will keep the link name and only change the link target. For example, if a PV has an existing symlink `/mnt/local-storage/localblock/scsi-0NVME_MODEL_abcde` pointing to `/dev/disk/by-id/scsi-0NVME_MODEL_abcde`, but there is a by-id link `/dev/disk/by-id/scsi-2ace42e0035eabcde`, setting the device link policy to `PreferredLinkTarget` will cause diskmaker to replace `/mnt/local-storage/localblock/scsi-0NVME_MODEL_abcde` with a new symlink pointing to `/dev/disk/by-id/scsi-2ace42e0035eabcde`.

Ceph Bluestore (and therefore ODF) does not automatically create by-uuid symlinks on the node. See [Bug 2414811](https://bugzilla.redhat.com/show_bug.cgi?id=2414811). However, diskmaker will still record the UUID from `ceph-volume raw list /dev/xyz --format=json` in `localVolumeDeviceLink.status.filesystemUUID` to help with support procedures.

### Risks and Mitigations

What happens if a symlink is changed while a workload is still running? This actually works as long as the new symlink is created first and then moved to the existing `/mnt/local-storage` link name. An existing process can hold a reference to the old (replaced) symlink until the process exits, and a new process will read the new symlink after `mv`.

Why opt-in? There may be other valid ways to resolve the issue and LSO does not have the context to make the right call in all cases. Instead of letting diskmaker change symlinks automatically, we want to call administrator attention to the problem and let them make an informed decision about their environment.

### Drawbacks

The biggest drawback is that without relying on some on-disk metadata LSO still relies on at least one symlink remaining stable across upgrades. This design is of limited help if _all_ of the by-id symlinks change between two releases.

We mitigate this drawback by recording the UUID of the filesystem in `localVolumeDeviceLink.status.filesystemUUID`, as this should not change in a node update and may help in some recovery scenarios.

### Future Work

We have some ideas to improve this in the future if there is a need, but they are out-of-scope for the initial implementation:

* Fine-grained filtering mechanism to influence which by-id symlink is used
* Other policy options for the `localVolumeDeviceLink.spec.policy` field

## Alternatives (Not Implemented)

* The `/mnt/local-storage` disk path stored in the PV is immutable, and we want the ability to fix existing PVs, which means we must keep the same `/mnt/local-storage` link name and update only the `/dev/disk/by-id` link target.
* We can't partition a device without breaking consumers or introducing separate API and does not help existing PVs recover.
* If diskmaker added a label to each device, it could easily be wiped by the disk consumer. It could label a device after kubelet creates a filesystem on it and save it in the device link status but this only helps with filesystem volumes.
* LSO could store udev attributes instead of symlinks but we would need to have some logic to filter useful and useless attributes, not clear if it helps rebuilding symlinks in all cases.
* We considered implementing this using annotations on the PV, but compared to an API it limits our options for maintainability, error reporting, and future enhancements.

## Open Questions [optional]

None

## Test Plan

We'll extend existing LSO unit tests, e2e, extended test suite, and manual testing where needed.

There will be a new e2e test with a simulated symlink change testing:
- The alert when `localVolumeDeviceLink.spec.policy == None`
- Symlinks are fixed automatically when `localVolumeDeviceLink.spec.policy == PreferredLinkTarget`
- Symlinks stay the same and no alert is thrown when `localVolumeDeviceLink.spec.policy == CurrentLinkTarget`

We do not have easy access to many hardware configurations and much of our testing will be limited to simulating these symlink issues.

## Graduation Criteria

This is an optional OLM operator and will be considered GA-ready when the feature is merged.

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

LSO already sets [maxOpenShiftVersion](https://github.com/openshift/local-storage-operator/blob/397dcaa02032f52916aa3ed49e5b9ecd1b96cc01/config/manifests/stable/local-storage-operator.clusterserviceversion.yaml#L118) to N+1. This means if the cluster is running 4.22 OCP with 4.21 LSO, then upgrades to 4.23 OCP will be blocked until LSO is upgraded to 4.22.

This is important because this enhancement is mostly concerned with OCP upgrade scenarios where the nodes are upgraded from RHEL 9.x (4.22 and earlier) to RHEL 10 (4.23 and later). We expect this enhancement to be implemented in LSO before 4.22.0 is released, which will allow it to create LocalVolumeDeviceLink objects and alert of potential problems prior to upgrading the nodes to RHEL 10.

## Operational Aspects of API Extensions

N/A

## Support Procedures

TBD

## Infrastructure Needed [optional]

N/A

