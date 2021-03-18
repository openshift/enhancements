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

### Non-Goals

* There is another use-case that this design does not solve is - editing of `LocalVolume` and `LocalVolumeSet` objects. If user edits LSO CRs in such a manner that - a node which was previously selected is no longer included via
node-selector or if CR excludes a device which was previously used for local-storage then - created PV is not automatically removed and corresponding symlink on the node is not removed either.

## Proposal

1. We propose that when `LocalVolume` or `LocalVolumeSet` object is created - they both contain a finalizer (`LocalVolume` object already has a finalizer). In addition we propose that - when user deletes these objects then finalizer can only be removed:
   a. If there are no bound PVs that are created by this LSO CR.
   b. None of the existing PVs(that match LSO CR) should have reclaim policy of `Retain.`
   c. If there are unbound PVs with reclaimPolicy of `Delete` then finalizer should not be removed until all such PVs are deleted.

2. If `LocalVolume` or `LocalVolumeSet` object can not be deleted because of #1.a or #1.b - then appropriate event should be logged and user should be informed. LSO is not going to perform any cleanup of bound PVs or PVs with `Retain` reclaim policy.
3. Before we can remove finalizer from LSO managed CRs, we should ensure deletion of created unbound PVs. To do that:
   a. First we should make all unbound PVs `Released` so as they can't be bound to PVCs. This will be done by control-plane of LSO.
   b. After making sure all unbound PVs for a deleted LSO CR are released, control-plane of LSO will wait for all such PVs to be deleted before removing the finalizer.
4. On the node if a `LocalVolume` or `LocalVolumeSet` objects is being deleted (i.e has `deletionTimestamp`) then - control-loop running on the node will stop "resyncing" the PV objects on the node, so as control-plane can mark them as released.
5. On the node if a `LocalVolume` or `LocalVolumeSet` object is being deleted and its PVs are in `Released` status then - node side control-loop should scrub the device, remove the symlink on the node and then delete the PV.
6. Once LSO's control-plane detects that all PVs created by LSO CR is deleted, it would remove the finalizer and related objects.

## Drawbacks

One drawback is - if user wants to keep one or more PVs around (in case they have data on it) and still delete the `LocalVolume` or `LocalVolumeSet` object - this design will not allow such flow. Also this change would result in data being wiped off from previously created PVs, which could surprise some users.
