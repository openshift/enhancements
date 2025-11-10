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
last-updated: 2025-11-06
tracking-link:
  - "https://issues.redhat.com/browse/STOR-2682"
see-also:
  - "https://issues.redhat.com/browse/OCPBUGS-61988"
replaces:
superseded-by:
---

# LSO: Recreate Symlinks

## Summary

Local Storage Operator (LSO) creates symlinks under `/mnt/local-storage` pointing to local disks by their `/dev/disk/by-id` path, and then creates a PersistentVolume (PV) pointing to each `/mnt/local-storage` symlink. This design presumed that the `/dev/disk/by-id` links were stable, and while they are more stable across reboots than `/dev/sdb` for example, it is still possible for the underlying by-id symlinks to change based on udev rule changes or even firmware updates from hardware vendors. LSO needs a documented and supported way to recover from these `/dev/disk/by-id` symlink changes on the node.

## Motivation

[RHEL docs](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_storage_devices/persistent-naming-attributes_managing-storage-devices#persistent-attributes-for-identifying-file-systems-and-block-devices_persistent-naming-attributes) state that "Device names managed by udev in /dev/disk/ can change between major releases, requiring link updates." and we have a specific case that became apparent in [OCPBUGS-61988](https://issues.redhat.com/browse/OCPBUGS-61988) that we need to mitigate for RHEL 10 (OCP 4.22).

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

LSO's diskmaker will detect all valid by-id symlinks for each PV and add them as an annotation on the PV. It will also annotate the by-id symlink currently in use. For filesystem volumes, it will annotate the UUID of the filesystem to help find the corresponding disk. This means each PV will have up to three new annotations added by diskmaker:

* `storage.openshift.com/current-link-target`: current by-id symlink in use for the device
* `storage.openshift.com/dev-disk-by-id-list`: list of valid by-id symlinks for the device
* `storage.openshift.com/dev-disk-by-uuid`: by-uuid symlink corresponding to the device (when applicable)

LSO will throw an alert if `current-link-target` does not match the recommended symlink in `dev-disk-by-id-list`. `dev-disk-by-uuid` is not available for raw block volumes, but may still be used for filesystem volumes to ensure detected symlinks match the device with the correct on-disk identifier.

In response to the alert, the administrator can review the annotations on the PV, and then add a `storage.openshift.com/recreate-symlink` annotation to the PV to tell diskmaker to recreate the symlink. This can be done to proactively switch to the recommended symlink, or it can be done reactively when a symlink is no longer valid (assuming there is another known valid by-id symlink). Diskmaker will recreate the symlink pointing to the new by-id symlink and remove the `recreate-symlink` annotation when it is complete.

### API Extensions

No new API changes, just the diskmaker annotations for each PV as described above.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A, not topology specific.

#### Standalone Clusters

N/A, not topology specific.

#### Single-node Deployments or MicroShift

N/A, not topology specific.

### Implementation Details/Notes/Constraints

Diskmaker will use the following selection criteria when choosing the recommended symlink for each PV:

1. The link must be in the sorted by-id list, starting from highest priority based on the [prefix priority list](https://github.com/openshift/local-storage-operator/blob/397dcaa02032f52916aa3ed49e5b9ecd1b96cc01/pkg/internal/diskutil.go#L177-L199).
2. If the current link target still exists, the new by-id target must point to the same device.
3. If a UUID was previously discovered and recorded, the new by-id target must point to the same device.
4. Return the first valid link from the by-id list that meets this criteria. Throw an error if no valid symlink can be found.

Diskmaker already watches for PV changes, but it will also need to watch for udev changes (?) to trigger reconcile to update the new annotations. It will need a reconcile trigger for detecting new UUID symlinks as well.

Diskmaker will keep the link name and only change the link target. For example, if a PV has an existing symlink `/mnt/local-storage/localblock/scsi-0NVME_MODEL_abcde` pointing to `/dev/disk/by-id/scsi-0NVME_MODEL_abcde`, but there is a by-id link `/dev/disk/by-id/scsi-2ace42e0035eabcde`, setting `storage.openshift.com/recreate-symlink` will cause diskmaker to replace `/mnt/local-storage/localblock/scsi-0NVME_MODEL_abcde` with a new symlink pointing to `/dev/disk/by-id/scsi-2ace42e0035eabcde`.

### Risks and Mitigations

What happens if a symlink is changed while a workload is still running? This actually works as long as the new symlink is created first and then moved to the existing `/mnt/local-storage` link name. An existing process can hold a reference to the old (replaced) symlink until the process exits, and a new process will read the new symlink after `mv`.

Why opt-in? There may be other valid ways to resolve the issue and LSO does not have the context to make the right call in all cases. Instead of letting diskmaker change symlinks automatically, we want to call administrator attention to the problem and let them make an informed decision about their environment.

### Drawbacks

The biggest drawback is that without relying on some on-disk metadata LSO still relies on at least one symlink remaining stable across upgrades. This design is of limited help if _all_ of the by-id symlinks change between two releases.

## Alternatives (Not Implemented)

* The `/mnt/local-storage` disk path stored in the PV is immutable, and we want the ability to fix existing PV's, which means we must keep the same `/mnt/local-storage` link name and update only the `/dev/disk/by-id` link target.
* We can't partition a device without breaking consumers or introducing separate API and does not help existing PV's recover.
* If diskmaker added a label to each device, it could easily be wiped by the disk consumer. It could label a device after kubelet creates a filesystem on it and save it in PV annotations but this only helps with filesystem volumes.
* LSO could store udev attributes instead of symlinks but we would need to have some logic to filter useful and useless attributes, not clear if it helps rebuilding symlinks in all cases.

## Open Questions [optional]

We want to annotate the PV with the UUID if an on-disk identifier can be found, for informational purposes at the very least. There are some open questions on how LSO can use the UUID, but these may be addressed in future revisions:

1. In addition to filesystem volumes, it would help to get the UUID of Ceph OSD volumes and annotate them. Ceph OSD volumes don't have by-uuid symlinks, can this be changed?

2. If diskmaker uses UUID to recreate symlinks, we need to figure out how to solve snapshot restore for LocalVolume object -- is it possible to have UUID conflicts that resolve to the wrong disk?

## Test Plan

We'll extend existing LSO unit tests, e2e, extended test suite, and manual testing where needed.

We do not have easy access to many hardware configurations and much of our testing will be limited to simulating these symlink issues.

## Graduation Criteria

This is an optional OLM operator, and since it does not introduce API changes or feature flags, it will be considered GA-ready when the feature is merged.

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

LSO already sets [maxOpenShiftVersion](https://github.com/openshift/local-storage-operator/blob/397dcaa02032f52916aa3ed49e5b9ecd1b96cc01/config/manifests/stable/local-storage-operator.clusterserviceversion.yaml#L118) to N+1, meaning if the cluster is running 4.21 OCP with 4.20 LSO, upgrades to 4.22 OCP will be blocked until LSO is upgraded to 4.21. This is important because we want to deliver these changes to alert of potential symlink issues prior to upgrades to RHEL 10 (OCP 4.22).

## Operational Aspects of API Extensions

N/A

## Support Procedures

TBD

## Infrastructure Needed [optional]

N/A

