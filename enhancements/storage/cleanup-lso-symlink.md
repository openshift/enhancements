---
title: handle-local-volume-deletion
authors:
  - "@gnufied"
reviewers:
  - "@jsafrane”
  - "@fbertina"
  - "@chuffman"
approvers:
  - "@..."
creation-date: 2021-04-17
last-updated: 2021-04-17
status: implementable
see-also: https://github.com/openshift/enhancements/blob/master/enhancements/storage/cleanup-lso-symlink.md
replaces:
superseded-by:
---

# Cleanup and handling of LocalVolume and LocalVolumeSet deletion

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes a mechanism that will allow PVs provisioned by local-storage-operator(LSO) to be deleted and corresponding symlinks to be removed from the node when `LocalVolume` or `LocalVolumeSet` objects are deleted.

## Motivation

Currently when a user deletes `LocalVolume` or `LocalVolumeSet` objects - we do not automatically delete the PVs created from those objects and neither do we remove the symlinks created on the node. This has been documented as a limitation because removal of symlinks and PVs can be sometimes race-prone.

This design fixes that problem and ensures that both PVs and symlinks are removed when `LocalVolume` and `LocalVolumeSet` objects are removed.

### Goals

* Do not allow deletion of `LocalVolumeSet` object if it has bound PVCs.
* Remove unbound PVs and remove symlinks on the host if `LocalVolume` or `LocalVolumeSet` objects are removed.

Note: We already do not permit deletion of `LocalVolume` objects via finalizer if it has bound PVCs. This was difficult to implement correctly for `LocalVolumeSet` objects because local-static-provisoner was being used as a daemon and tracking of individual PVs was not possible. This is discussed in more detail in - https://github.com/openshift/local-storage-operator/pull/150#discussion_r492828334


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

Currently reconciler loop running on the node re-creates PVs as long as a symlink for the given device exists in the LSO's pre-configured directory. When PV is marked as `Released` in step#1, the
static provisioner code will automatically scrub the volume and delete the PV and hence changes required in reconciler loop of node daemon is:

1. If `LocalVolume` or `LocalVolumeSet` object is being deleted (has `deletionTimestamp`), it will not create new symlinks for automatically discovered volumes that match CR's device selection criteria.
2. If `LocalVolume` or `LocalVolume` object is being deleted and symlink being evaluated exists but corresponding PV does not, then rather than creating new PV - it will remove the associated symlink.

This would ensure that diskmaker daemon on the node will remove symlinks and PVs that belong to CR that is being deleted.

#### 3. Remove finalizer from CR if no PV exists for given CR in LSO control-plane

In the control-plane of LSO if a CR is being deleted and has no associated PVs, the finalizer that prevents deletion of these CRs will be removed and hence associated `LocalVolume` and `LocalVolumeSet` objects will be deleted and successfully cleaned up.

## Drawbacks

One drawback is - if user wants to keep one or more PVs around (in case they have data on it) and still delete the `LocalVolume` or `LocalVolumeSet` object - this will require users to set `ReclaimPolicy` of `Retain` on those PVs, before deleting `LocalVolume` or `LocalVolumeSet` objects and then using force delete to delete those objects.
