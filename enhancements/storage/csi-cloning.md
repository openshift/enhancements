---
title: csi-cloning
authors:
  - "@chuffman"
reviewers:
  - "@gnufied”
  - “@jsafrane”
approvers:
  - TBD
  - "@..."
creation-date: 2019-09-05
last-updated: 2019-09-25
status: provisional
see-also:
replaces:
superseded-by:
---

# CSI Cloning

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs] 

## Summary

We want to include the upstream CSI Cloning feature in OpenShift. 

## Motivation

* This has been requested by both the OCS and CNV teams
* This feature is a core functionality offered by storage vendors that we need to enable for our customers

### Goals

* Rebase the downstream external-provisioner to the upstream external-provisioner, based on Kubernetes 1.16.
* Package and ship a downstream image of a sidecar including these features.

### Non-Goals

## Proposal

This feature has already been implemented upstream, and is described in https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/20181111-extend-datasource-field.md . It is currently documented at https://kubernetes.io/docs/concepts/storage/volume-pvc-datasource/ .

We will be releasing the `csi-external-provisioner` sidecar image with cloning support (already enabled by default) as Technology Preview. This sidecar will be provided to internal teams, such as CNV and OCS, for their drivers. No other consumer is supported, although it is eligible for use by any CSI driver.

### User Stories [optional]

#### Story 1
As an OCS developer, I want to release the CSI Driver with cloning support so that drivers which include this feature, such as OCS, can use the API to easily Clone a volume.

### Implementation Details/Notes/Constraints [optional]

This feature has already been implemented upstream. Therefore, this feature requires the Kubernetes 1.16 rebase, and the `csi-external-provisioner` to be released with cloning support.
We must also include e2e tests for this feature.

This feature is described upstream at:

* https://kubernetes.io/docs/concepts/storage/volume-pvc-datasource/
* https://kubernetes.io/docs/concepts/storage/persistent-volumes/#volume-cloning
* https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/20181111-extend-datasource-field.md

### Risks and Mitigations

This enhancement requires both of the following:

* Kubernetes 1.16 rebase
* Upstream release of `csi-external-provisioner`. 

The external provisioner is not yet released, and current estimates place it near the beginning of October.

## Design Details

### Test Plan

We’ll want the e2e tests to run using CSI sidecars shipped as part of OCP. There is ongoing progress in the following links regarding testing OCP CSI sidecars:

* https://github.com/openshift/origin/pull/23560
* https://jira.coreos.com/browse/STOR-223

Additional references:

* https://github.com/kubernetes/kubernetes/tree/master/test/e2e/storage/external - defines how CSI drivers can be tested
* https://github.com/kubernetes/kubernetes/blob/master/test/e2e/storage/testsuites/provisioning.go - contains the tests for cloning CSI devices.

### Graduation Criteria

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

None. The csi-external-provisioner repository will be updated to include this feature.

