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
This proposal will enhance SCCs to control which CSI drivers are allowed.

## Motivation

Inline CSI volumes are provided via CSI drivers that can be added to OpenShift.
When a pod mounts an inline `csi` volume, it must specify the driver that will provide the underlying content.
Cluster admins will need a means to identify which drivers are considered safe for use, including in restricted namespaces.

Ephemeral CSI drivers are expected to be used in the following use cases:

1. Mounting shared simple content access certificates used to install RHEL content.
2. Accessing [sealed secrets](https://secrets-store-csi-driver.sigs.k8s.io/) stored in Vault, Azure Key Vault, or Google Secret Manager.

### Goals

* Allow cluster admins to control which ephemeral CSI drivers can be used to mount `csi` volumes.

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

#### Safe for Consumption Annotation

CSI drivers that provide inline CSI volumes must mark themselves as safe for consumption.
To mark a CSI driver safe, admins or an operator will need to add the `security.openshift.io/allow-csi-volumes-restricted` annotation to the driver's `CSIDriver` object:

```yaml
apiVersion: storage.k8s.io/v1
kind: CSIDriver
annotations:
  security.openshift.io/allow-csi-volumes-restricted: "true"
  ...
metadata:
  name: mycsi.dev
  ...
```

#### Pod admission

Pod that mount `csi` volumes cannot be admitted unless the chosen SCC allows `csi` volumes.
In addition, the driver for the volume mount must have the `security.openshift.io/allow-csi-volumes-restricted` annotation.

#### Allow CSI volumes in restricted namespaces

Ephemeral CSI drivers will be promoted to the list of "safe" volume mounts, and be considered equivalent to a Secret or ConfigMap from a safety/restriction perspective.
CSI volumes will be added to the list of allowed volume types in the `restricted` SCC.
All default SCCs installed by OpenShift with less restrictions will likewise allow `csi` volumes.
When SCCs are ordered, the `csi` volume type will count as a "trivial" volume mount.

### Risks and Mitigations

**Risk:**
Default security context constraints will allow any CSI driver to be used.

*Mitigation:*
Only the CSI drivers that are marked safe by an operator or admin will be allowed to mount `csi` volumes.

**Risk:**
Checking the annotation on the CSI driver will not be backwards compatible.

*Mitigation:*

Documentation for storage and SCCs will need to communicate that inline CSI volume drivers will need to add the `security.openshift.io/allow-csi-volumes-restricted` annotation to work on OpenShift.
A CSI driver that mounts inline CSI volumes should be safe for any user to use on OpenShift, or it should not be allowed at all.

**Risk:**
During upgrade, any pod can use any CSI driver due to version skew between the default SCCs and the SCC admission logic in openshift-apiserver.

*Mitigation:*
The upgrade test suites must ensure that pods cannot be created with `csi` volumes in a restricted namespace.

## Design Details

### Open Questions [optional]

1. Should we allow the privileged SCC to mount any CSI volume (via an `AllowAllCSIVolumeDrivers` flag)?
   This has implications for OpenShift builds, which currently uses the `privleged` SCC.

### Test Plan

e2e testing will require us to use a CSI driver that provisions ephemeral CSI volumes.
No such CSI driver is provided by OpenShift today.
However, we do have the [projected resource CSI driver](https://github.com/openshift/csi-driver-projected-resource), which has images on CI and can be deployed via its YAML manifests.
These tests should be included in a conformance test suite.

The test should run as follows:

1. Setup
   1. Deploy the projected resource CSI driver with the `security.openshift.io/allow-csi-volumes-restricted` annotation.
   2. Create a secret in the openshift-config namespace with test data.
   3. Create the projected resource `Share` object.
   4. Create a ClusterRole that grants read access to the `Share` object, and aggregates it to the cluster "edit" role.
2. Test execution
   1. Create a `Pod` that mounts the share as a `csi` volume and prints the contents.
   2. Verify if the pod was created, and if so verify the contents of the shared secret.
3. Test scenarios
   1. Pod uses the projected resource CSI driver.
   2. Pod uses a dummy CSI driver (not installed).

The upgrade test suite should also ensure that pods with `csi` volumes cannot be created in a restricted namespace as long as the are no CSI drivers with the `security.openshift.io/allow-csi-volumes-restricted` annotation.

### Graduation Criteria

This new capability will be promoted directly to GA.

### Upgrade / Downgrade Strategy

No API changes are proposed, so there are no API compatibility concerns with respect to upgrades or downgrades.
With respect to behavior, all SCCs will lose the ability to mount `csi` volumes out of the box.
This capability becomes enabled when a CSI driver is added to the cluster and is annotated with `security.openshift.io/allow-csi-volumes-restricted`.

### Version Skew Strategy

TBD

## Implementation History

2021-04-27: Initial draft

## Drawbacks

Testing this capability will be a challenge because OpenShift does not provide any driver that mounts `csi` volumes.
Until OpenShift ships a CSI driver that supports ephemeral `csi` volume mounts, reliable testing may require us to implement our own "dummy" ephemeral CSI driver.
The alternative is to utilize upstream projects like our projected resource CSI driver.

This approach also disables a capability - `csi` volumes are disabled for all accounts until a CSI driver is marked safe for consumption.
Even the privileged SCC cannot be used to mount a `csi` volume until a safe CSI driver is added to the cluster.
Once such a driver is marked safe, `csi` volumes are available to all authenticated service accounts with those specific drivers.

## Alternatives

### Allowlist inline CSI drivers

Instead of using annotations, we could use an allowlist to gate which drivers are allowed per SCC.

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

This approach can make inline CSI volumes appear unsafe to an administrator, and makes it harder to configure a restricted namespace.
The relative safety of an inline CSI volume depends on the driver that provisions the volume.

### External Admission Webhooks

External admission webhook tools like OPA and Kyverno provide immense flexibility, and can be configured to allow or deny csi volume drivers.
We can keep SCCs as is, and recommend these tools if cluster admins wish to control the allowed CSI volume drivers.

This is problematic for two reasons:

1. OPA and Kyverno must be installed on top of OpenShift.
2. These tools can be overkill if cluster admins are otherwise happy with SCCs to lock down their clusters.

### Pod Security Policy v2

Upstream Kubernetes is considering a "next" version of Pod Security Policies.
This is currently a drafted [KEP](https://github.com/kubernetes/enhancements/pull/2582) - it has not been approved, let alone added to a Kubernetes milestone.
The draft proposes implementing hard-coded profiles (`privileged`, `baseline`, `restricted`) as recommended by SIG-Security.
Fine grained control of allowed CSI volume drivers does not appear to be in scope for this proposal.

## Infrastructure Needed [optional]

None expected.
