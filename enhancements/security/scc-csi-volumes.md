---
title: scc-csi-volumes
authors:
  - "@adambkaplan"
reviewers:
  - "@bparees"
  - "@jsafrane"
approvers:
  - "@sttts"
  - "@deads2k"
creation-date: 2021-04-27
last-updated: 2021-04-27
status: provisional
see-also: []
replaces: []
superseded-by: []
---

# Security Context Constraints for CSI Volumes

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Ephemeral CSI volumes are a newer type of volume in Kubernetes, which allows content to mounted into pods with underlying ephemeral file systems.
OpenShift Security Context Constraints (SCCs) take an all-or-nothing approach when it comes to these volumes.
A service account/user can either mount a `csi` volume, or it cannot.
This proposal will enhnance SCCs to control which CSI drivers are allowed.

## Motivation

Ephemeral CSI volumes are provided via CSI drivers that can be added to OpenShift.
When a pod mounts a `csi` volume, it must specify the driver that will provide the underlying content.
Cluster admins will need a means to approve which service accounts can use a particular CSI driver.

Ephemeral CSI drivers are expected to be used in the following use cases:

1. Mounting shared simple content access certificates used to install RHEL content.
2. Accessing [sealed secrets](https://secrets-store-csi-driver.sigs.k8s.io/) stored in Vault, Azure Key Vault, or Google Secret Manager.

### Goals

* Allow cluster admins to control which ephemeral CSI drivers can be mounted via SCCs.

### Non-Goals

* Allow `csi` volumes to be mounted in build pods.
* Enable SCCs to limit [generic ephemeral volumes](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#generic-ephemeral-volumes)

## Proposal

### User Stories

As an OpenShift cluster admin
I want to control which CSI drivers can be used to mount csi volumes
So that my workloads only use trusted CSI drivers to access content

As an application developer
I want to use the secret store CSI driver to mount csi volumes
So that I can access sealed secrets in my workloads

### Implementation Details/Notes/Constraints [optional]

The following API extends SCCs to restrict the allowed CSI drivers used for ephemeral CSI volumes, in the same manner that Flexvolumes are restricted:

```go
type SecurityContextConstraints struct {
  // existing API
  ...
  //

  Volumes []FSType

  // AllowedCSIVolumes is an allowlist of permitted ephemeral CSI volumes.
  // Empty or nil indicates that all CSI drivers may be used.
  // This parameter is effective only when the usage of CSI volumes is allowed in the "Volumes" field.
	// +optional
  AllowedCSIVolumes []AllowedCSIVolume
}

// AllowedCSIVolume represents a single CSI volume driver that is allowed to be used.
type AllowedCSIVolume struct {
  // Driver is the name of the CSI driver that supports ephemeral CSI volumes.
  Driver string
}
```

As is the case with Flexvolumes, CSI volumes must be enabled by adding `csi` to the list of allowed volumes in the SCC.
Once added, the SCC can restrict the allowed drivers via the `AllowedCSIVolumes` list.
Empty or nil will inidicate "allow all" for backwards compatibility reasons.
To prevent the mounting of CSI volumes, admins should remove `csi` from the list of allowed volumes in the SCC.

### Risks and Mitigations

**Risk:** Default security context constraints will allow any CSI driver to be used

*Mitigations:*

- Only the `privileged` and `node-exporter` default SCCs allow CSI volumes to be mounted (they allow any volume mount)
- Testing will ensure that new SCCs 

**Risk:** Restrictions on allowed CSI drivers will not be backwards compatible

*Mitigation:*

As with Flexvolumes, SCCs will default to "allow all" CSI drivers if `csi` volume mounts are allowed.
The AllowedCSIVolumes list is used to restrict `csi` volume mounts.

## Design Details

### Open Questions [optional]

TBD

### Test Plan

e2e testing will require us to use a CSI driver that provisions ephemeral CSI volumes.
No such CSI driver is provided by OpenShift today.
However, we do have the [projected resource CSI driver](https://github.com/openshift/csi-driver-projected-resource), which has images on CI and can be deployed via its YAML manifests.
These tests should be included in a conformance test suite.

The test should run as follows:

1. Setup
   1. Deploy the projected resource CSI driver
   2. Create a secret in the openshift-config namespace with test data.
   3. Create the projected resource `Share` object.
   4. Create a ClusterRole that grants read access to the `Share` object, and aggregates it to the cluster "edit" role.
2. Test execution
   1. Create a `Pod` that mounts the share as a `csi` volume and prints the contents.
   2. Verify if the pod was created, and if so verify the contents of the shared secret.
3. Test scenarios
   1. Prohibit all `csi` volumes
   2. Allow `csi` volumes, but only allow a dummy driver (not installed)
   3. Allow `csi` volumes, but only allow the projected resource CSI driver
   4. Allow all `csi` volumes

### Graduation Criteria

This new capability will be promoted directly to GA.

### Upgrade / Downgrade Strategy

SCCs today allow `csi` volumes to be allowed or blocked.
On upgrade, an SCC that allows all `csi` volumes will continue to allow any CSI driver.
Users will then need to edit existing SCCs or create new ones to restrict the allowed ephemeral CSI drivers.
The default SCCs installed by OpenShift will not change.

On downgrade, SCCs will still allow `csi` volumes to be allowed or blocked en masse.
The default SCCs will continue to be deployed and managed by the CVO.

### Version Skew Strategy

TBD

## Implementation History

2021-04-27: Initial draft

## Drawbacks

Testing this capability will be a challenge because OpenShift does not provide any driver that mounts `csi` volumes.
Until OpenShift ships a CSI driver that supports ephemeral `csi` volume mounts, reliable testing may require us to implement our own "dummy" ephemeral CSI driver.
The alternative is to utilize upstream projects like the [secret store CSI driver](https://secrets-store-csi-driver.sigs.k8s.io/getting-started/installation.html).

## Alternatives

### External Admission Webhooks

External admission webhook tools like OPA and Kyverno provide immense flexibility, and can be configured to allow or deny csi volume drivers.
We can keep SCCs as is, and recommend these tools if cluster admins wish to control the allowed CSI volume drivers.

This is problematic for two reasons:

1. OPA and Kyverno must be installed on top of OpenShift.
2. These tools can be overkill if cluster admins are otherwise happy with SCCs to lock down their clusters.

### Pod Security <next>

Upstream Kubernetes is considering a "next" version of Pod Security Policies.
This is currently a drafted [KEP](https://github.com/kubernetes/enhancements/pull/2582) - it has not been approved, let alone added to a Kubernetes milestone.
The draft proposes implementing hard-coded profiles (`privileged`, `baseline`, `restricted`) as recommended by SIG-Security.
Fine grained control of allowed CSI volume drivers does not appear to be in scope for this proposal.

## Infrastructure Needed [optional]

None expected.
