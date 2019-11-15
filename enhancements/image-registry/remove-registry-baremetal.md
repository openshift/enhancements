---
title: remove-registry-baremetal
authors:
  - "@adambkaplan"
reviewers:
  - "@dmage"
  - "@bparees"
  - "@sdodson"
  - "@rgolangh" # RHV
  - "@russellb" # k8s-native infra
approvers:
  - "@bparees"
  - "@smarterclayton"
  - "@derekwaynecarr"
creation-date: 2019-09-25
last-updated: 2019-10-22
status: implementable
see-also: 
replaces:
superseded-by:
---

# Remove Registry on BareMetal, oVirt, and VSphere

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

On platforms which do not provide shareable object storage, the OpenShift image
registry operator will bootstrap itself as `Removed`. This will allow
`openshift-installer` to complete installations on these platform types. A
separate process will be responsible for configuring storage and updating the
image registry operator's state to `Managed`.

## Motivation

IPI installation options for baremetal platforms and Red Hat Virtualization
(RHV) have been declared a key initiative for OpenShift 4.3. These will
configure themselves as the `baremetal` and`oVirt` platform types, respectively.
In 4.2 the registry bootstraps itself to use `EmptyDir` storage on `baremetal`
to enable pre-GA development - see
([PR #332](https://github.com/openshift/cluster-image-registry-operator/pull/332)).
For RHV no storage is configured and and IPI installation for the `oVirt`
platform type fails to complete.

IPI installation for `vSphere` is not supported at present, but is desired in
the future. Current UPI installs on `vSphere` often result in the registry not
being configured and reporting itself `Degraded=true`.

`EmptyDir` is not suitable for GA components as there is a serious risk of data
loss if a permanent form of storage is not configured.

### Goals

- `openshift-installer` completes if the platform is set to `baremetal` or
  `oVirt`.
- UPI installs for `vSphere` bring the registry up in the `Removed` state.
- Image registry is able to bring itself up after the management state is
  switched from `Removed` to `Managed`.

### Non-Goals

- Provision storage for the registry for RHV and k8s-native infrastructre.
- Provide 1st class support for additional storage types/providers for the
  registry (ex: MCG).
- Harden OpenShift features (ex: Builds, ImageStreams) such that they error
  cleanly if the registry is not installed.

## Proposal

This is where we get down to the nitty gritty of what the proposal actually is.

### User Story

As an OpenShift cluster admin/installer
I want the registry to be `Removed` on RHV and k8s-native infra installations
So that I can provision other storage providers
And later enable the image registry with the desired storage

### Implementation Details/Notes/Constraints

- When the image registry operator bootstraps itself, it will inspect the
  cluster infrastructure platform status.
- The registry will bootstrap itself as `Removed` on the following platform types:
  - `BareMetal` (i.e. baremetal platforms that support IPI install)
  - `oVirt` (RHV)
  - `VSphere`
  - `None` (true UPI baremetal installs)
- The registry will bootstrap itself as `Managed` with `EmptyDir` storage on
  the following platform types:
  - `Libvirt`
  - `Unknown` platforms
- The registry will continue to bootstrap itself as `Managed` with specific
  storage on the following platforms:
  - `AWS` (Amazon S3)
  - `Azure` (Azure blob storage)
  - `GCP` (Google Cloud Storage)
  - `OpenStack` (Swift)
- Any platform that wishes to set up the registry must perform a day 2 action
  that does the following:
  - Provisions suitable storage, typically a PV that supports the `RWX` access
    mode.
  - Updates the image registry operator's management state to `Managed`.
- Telemeter should be aware if a cluster's registry has been removed:
  - Reason codes in the ClusterOperator conditions (`Available` in particular).
  - Prometheus alerts warning the registry has been removed.

### Risks and Mitigations

#### Image registry is never installed

Several components will likely report errors if the registry is `Removed` and
an admin does nothing to set up the registry. Potential candidates that will
break:

1. Builds which push to an imagestream tag will error at runtime.
2. Samples operator imagestreams which import from `registry.redhat.io` will
   require a pull secret to be usable. Failure to provide one will result in an
   `ImagePullBackoff` deployment failure.
3. Imagestreams imported with `--reference-policy=local` will not reference the
   internal registry as expected. `BuildConfig` and `DeploymentConfig` objects
   which do their own resolution of `ImageStreamTag` may encounter
   `ImagePullBackoff` errors if the external image requires a pull secret.

Mitigations:

1. Existing error messages from affected components.
2. Image registry's ClusterOperator will provide `Reason="Removed"` if the
   registry remains removed.
3. If removed, the image registry operator should fire an alert warning that:
   1. The registry has been removed.
   2. The following components may not work as expected:
      1. `ImageStreamTags`
      2. `BuildConfigs` which reference `ImageStreamTags`
      3. `DeploymentConfigs` which reference `ImageStreamTags`
   3. Recommends admins configure storage and update the operator config to
   the `Managed` state.

## Design Details

### Test Plan

- Existing e2e-operator suite will need to verify that the registry behaves as
  expected when `Removed`:
  - No registry components are installed.
  - Reporting "green"/does not block upgrade.
  - Fire an alert.
  - Registry should behave as expected when state is switched to `Managed`
- UPI vSphere test suites will need to update the registry's state to `Managed`
  in addition to configuring storage.
- RHV tests which use IPI installation will need to update the registry's state
  to `Managed` in addition to configuring storage to pass most conformance
  suites.
- Any other CI template which uses `BareMetal` or `None` to install the cluster
  must be updated to have the image registry operator set to `Managed` with
  some form of storage. `EmptyDir` should be sufficient for most CI use cases.

### Graduation Criteria

#### Tech Preview

Not applicable.

#### Generally Available

1. Image registry is not installed on production platforms which do not have
   native object storage (`BareMetal`, `oVirt`, `VSphere`, `None`).
2. Documentation is added instructing customers on how to enable or disable the
   image registry.
3. Release notes document the installer behavior change for `VSphere`.

### Upgrade / Downgrade Strategy

- Upgrades should work if registry is `Removed`
- Downgrade to 4.2.z should work if registry is `Removed` (may require bugs to
  be backported and upgrade graph to go through a z-stream first)

### Version Skew Strategy

## Implementation History

2019-09-25: Initial draft
2019-10-22: Implementable design

## Drawbacks

The internal registry has always been a core part of OpenShift.
This enhancement furthers the case that it may be OK to remove the registry.

## Alternatives

Continue the current path of defauling storage to `EmptyDir` on baremetal, and
bring the registry up as `Managed`. This was discussed in
[openshift/cluster-image-registry-operator#332](https://github.com/openshift/cluster-image-registry-operator/pull/332)
and considered a greater risk - customers could lose data if the registry
relied on ephemeral storage.

## Infrastructure Needed

- RHV will need to create an external process to configure the registry as an
  automated day-2 task
- Image registry components should have optional e2e suites for the following:
  - RHV (`oVirt`)
  - An opinionated baremetal install (`BareMetal`)
  - UPI `VSphere`
