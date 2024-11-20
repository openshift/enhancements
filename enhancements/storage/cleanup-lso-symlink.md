---
title: handle-local-volume-deletion
authors:
  - "@gnufied"
  - "@dobsonj"
reviewers:
  - "@jsafrane"
  - "@fbertina"
  - "@chuffman"
approvers:
  - "@..."
creation-date: 2021-04-17
last-updated: 2024-11-20
status: implemented
see-also: https://github.com/openshift/enhancements/blob/master/enhancements/storage/cleanup-lso-symlink.md
replaces:
superseded-by:
---

# Cleanup and handling of LocalVolume and LocalVolumeSet deletion

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [x] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes a mechanism that will allow PVs provisioned by local-storage-operator(LSO) to be deleted and corresponding symlinks to be removed from the node when `LocalVolume` or `LocalVolumeSet` objects are deleted.

## Motivation

Previously, when a user deleted `LocalVolume` or `LocalVolumeSet` objects - LSO did not automatically delete the PVs created from those objects and neither do we remove the symlinks created on the node. This has been documented as a limitation because removal of symlinks and PVs can be sometimes race-prone.

This design fixes that problem and ensures that both PVs and symlinks are removed when `LocalVolume` and `LocalVolumeSet` objects are removed.

### Goals

* Do not allow deletion of `LocalVolumeSet` object if it has bound PVCs.
* Remove unbound PVs and remove symlinks on the host if `LocalVolume` or `LocalVolumeSet` objects are removed.

### Non-Goals

* There is another use-case that this design does not solve is - editing of `LocalVolume` and `LocalVolumeSet` objects. If user edits LSO CRs in such a manner that - a node which was previously selected is no longer included via
node-selector or if CR excludes a device which was previously used for local-storage then - created PV is not automatically removed and corresponding symlink on the node is not removed either.

## Proposal

The reason we simply can not remove PVs and delete symlinks on the hosts is - a naive procedure is race prone. A PV that is being cleaned up can be claimed by a PVC and hence can result in data loss.

### Prevent deletion of `LocalVolume` and `LocalVolumeSet` objects with PVs

As a first step - we propose that both `LocalVolume` and `LocalVolumeSet` objects contain a finalizer which will prevent deletion of these objects and LSO will only remove the finalizer when all
PVs created by these objects are removed.  This serves two purpose:

1. It will prevent accidental deletion of `LocalVolume` and `LocalVolumeSet` objects.
2. It will give LSO opportunity to cleanup the provisioned volumes before `LocalVolume` and `LocalVolumeSet` objects can be deleted.

In addition we propose that bound PVs or Released PVs with `Retain` policy will actively prevent deletion of `LocalVolume` and `LocalVolumeSet` objects.
No automatic cleanup will be possible in that case and events should be added to LSO CRs that inform the user about PVs that are blocking deletion of `LocalVolume` and `LocalVolumeSet` objects.

* For Bound PVs, user should delete the corresponding PVC to get the PV Released.
* For Released PVs with reclaim policy "Retain", user should back up any important data from the volume and delete the PV.


### Automatic cleanup of available/pending PVs and symlinks by LSO

In this proposal we only seek removal of available/pending PVs that were provisioned by LSO, when corresponding `LocalVolume` and `LocalVolumeSet` object is deleted. To accomplish this in a manner that
would be free from potential race conditions, following steps needs to be performed by LSO:

#### 1. Mark available PVs as released in LSO control-plane

If `LocalVolume` or `LocalVolumeSet` has `deletionTimestamp` then control-plane of LSO should reconcile all PVs created by these objects and ensure that available PVs are marked as `Released`.
This would prevent binding of these PVs to any incoming PVCs.

#### 2. Cleanup of volumes, symlinks and PVs on the node

In this proposal, we will _always_ remove the symlink after a PV is cleaned and deleted. The expected lifecycle of a PV from diskmaker's perspective is:

1. Create symlink
2. Create PV
3. PV eventually gets Released
4. Clean PV
5. Delete PV
6. Remove symlink

The symlink must not be removed before the PV is deleted, otherwise it interferes with the deletion process. Therefore, the changes required in reconciler loop of the diskmaker node daemonset are:

1. Add a new finalizer `storage.openshift.com/lso-symlink-deleter` when creating the PV. This finalizer should not be removed until after the corresponding symlink has been deleted.
2. After the PV has been cleaned, the reconciler sends a Delete request for the PV and adds a deletionTimestamp, but the finalizer still exists at this point. We should not attempt to clean the PV again once the deletionTimestamp exists.
3. After the PV is cleaned and deleted, diskmaker should remove the symlink and then remove the finalizer, allowing the PV object to be removed.
4. If the `LocalVolume` or `LocalVolumeSet` object is being deleted (has `deletionTimestamp`), diskmaker should not create new symlinks or PVs for automatically discovered volumes that match CR's device selection criteria. It will only try to clean up any remaining PV's that are released.

#### 3. Remove finalizer from CR if no PV exists for given CR in LSO control-plane

When diskmaker removes the `storage.openshift.com/lso-symlink-deleter` finalizer from the last PV, then the LSO control-plane will see the `LocalVolume` or `LocalVolumeSet` object is being deleted and has no associated PVs, and it will remove the `storage.openshift.com/local-volume-protection` finalizer that prevents deletion of these CRs.

When the last `LocalVolume` or `LocalVolumeSet` object is deleted, at this point all the PV's associated with them are already gone, and the diskmaker daemonset is deleted.

## Drawbacks

One drawback is - if user wants to keep one or more PVs around (in case they have data on it) and still delete the `LocalVolume` or `LocalVolumeSet` object - this will require users to set `ReclaimPolicy` of `Retain` on those PVs, before deleting `LocalVolume` or `LocalVolumeSet` objects and then using force delete to delete those objects.

## Test Plan

There is an e2e test to ensure when a `LocalVolume` or `LocalVolumeSet` object is deleted, the associated PV's and symlinks are also deleted.
