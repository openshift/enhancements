---
title: csi-volume-expansion
authors:
  - "@bertinatto"
  - "@gnufied"
reviewers:
  - "@huffmanca"
  - "@jsafrane"
approvers:
  - TBD
creation-date: 2019-09-23
last-updated: 2019-09-23
status: provisional
see-also:
replaces:
superseded-by:

---

# CSI Volume Expansion

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We want CSI volume expansion available as Tech Preview in OpenShift 4.3. In
Kubernetes v1.16 this feature turned Beta, which gives us a good level of
confidence that it has been well tested and is ready for OpenShift users.

## Motivation

Volume expansion is available in in-tree provisioners since OpenShift 3.11. However,
users are not currently able to expand volumes provisioned by CSI drives in OpenShift.

Since Red Hat OpenShift Container Storage (OCS) 4.3 uses the CSI interface for providing storage
fabric for OpenShift, we need to to make sure we support CSI volume expansion accordingly.

### Goals

Provide CSI driver authors, like the OCS team, with a mechanism to expand volumes provioned
by their CSI drivers.

### Non-Goals

## Proposal

- Rebase from upstream csi-resizer external controller.
- Package and ship a downstream image of this controller.

### Risks and Mitigations

## Design Details

### Test Plan

The upstream external-resizer sidecar is tested by running the in-tree expansion
tests against both csi-hostpath and GCE-PD CSI drivers. This is done by this Prow job:

https://prow.k8s.io/job-history/kubernetes-jenkins/pr-logs/directory/pull-kubernetes-csi-external-resizer

We plan to have a job running the same tests against the AWS EBS CSI driver. There is a WIP PR to address this at:

https://github.com/openshift/origin/pull/23560

### Graduation Criteria

#### Examples

##### Tech Preview -> GA

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Kubernetes changes are provided by the rebase, so there is no opt-out for upgrade/downgrade.

Regarding the resizer sidecar, CSI driver authors are responsible for the upgrade/dowgrade
strategy of their drivers along with the sidecars those drivers use.

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

Users could use the upstream sidecar instead of the one proposed here.

## Infrastructure Needed [optional]
