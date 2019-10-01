---
title: CSI Snapshots
authors:
  - "@chuffman"
reviewers:
  - "@gnufied”
  - “@jsafrane”
approvers:
  - "@..."
creation-date: 2019-09-05
last-updated: 2019-09-25
status: provisional
see-also:
replaces:
superseded-by:
---

# csi-snapshots

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

We want to include the upstream CSI Snapshot feature in OpenShift.

## Motivation

* This feature has been requested by both the OCS and CNV teams.
* Snapshotting is an incredibly useful function when it comes to system changes in development areas, as it allows rapid rollback to preview dev versions.
* Container development benefits from filesystems that provide snapshot functionality.

### Goals

* Rebase the downstream csi-external-snapshotter and csi-external-provisioner images off of the upstream images based on Kubernetes 1.16.

* Package and ship a downstream image of the this sidecar.

### Non-Goals

## Proposal

This feature has already been implemented upstream, and is described in https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/20190709-csi-snapshot.md . It is currently documented upstream at https://kubernetes.io/docs/concepts/storage/volume-snapshots/ .

We will be releasing the `external-snapshotter` sidecar image to enable CSI Snapshot support in OCP as Developer Preview only. This sidecar can be used by internal teams, such as CNV or OCS, for their drivers.

### User Stories [optional]

#### Story 1

As an OCS developer, I want to release the `csi-external-snapshotter` sidecar image so that users can utilize volume snapshots.

#### Story 2

### Implementation Details/Notes/Constraints [optional]

This feature has already been implemented upstream. Therefore, this feature requires a Kubernetes rebase of 1.15 or later.

### Risks and Mitigations

* Kubernetes 1.15+ rebase

## Design Details

No design is needed, as this feature has already been implemented upstream. The upstream design was discussed under https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/20190709-csi-snapshot.md.

### Test Plan

We’ll want the e2e tests to run using CSI sidecars shipped as part of OCP. There is ongoing progress in the following links regarding testing OCP CSI sidecars:

* https://github.com/openshift/origin/pull/23560
* https://jira.coreos.com/browse/STOR-223
Tests for this sidecar are currently defined upstream at https://github.com/kubernetes-csi/external-snapshotter/tree/master/pkg/controller .

### Graduation Criteria

##### Dev Preview -> Tech Preview

* Unit and e2e tests implemented.
* Update snapshot CRDs to v1beta1 and enable VolumeSnapshotDataSource feature gate by default. The feature must also be at least v1beta1 upstream, and have a strong indication that it will be made GA in a reasonable time frame with a compatible API.

##### Tech Preview -> GA

* Feature deployed in production and have gone through at least one K8s upgrade.
* Feature must be GA in Kubernetes.

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

None. The csi-external-snapshotter repository will be updated to include this feature.
