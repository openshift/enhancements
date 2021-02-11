---
title: Build OKD-on-Fedora-CoreOS in Prow
authors:
  - "@LorbusChris"
reviewers:
  - "@abhinavdahiya"
  - "@ashcrow"
  - "@cgwalters"
  - "@crawford"
  - "@darkmuggle"
  - "@miabbott"
  - "@runcom"
  - "@sdodson"
  - "@smarterclayton"
  - "@staebler"
  - "@vrutkovs"
approvers:
creation-date: 2019-10-21
last-updated: 2021-02-11
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:  
replaces:
superseded-by:
---

# Build OKD-on-Fedora-CoreOS in Prow

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

OKD is the OpenShift community distribution of Kubernetes.

As part of the version 4 effort, the OKD working group has decided to target Fedora CoreOS (FCOS) as the primary base operating system for OKD control plane and worker nodes.

This document contains a proposal to add missing FCOS support to key components,
create OKD artifacts in OpenShift's Prow instance from the canonical OpenShift master and release branches alongside OCP CI artifacts,
and regularily create OKD release payloads from there.

## Motivation

The OKD Working Group wants to deliver a community variant of OpenShift on top of Fedora CoreOS
that is released off of the latest OpenShift stable release branch in a rolling manner,
and provides the ability to update. An update release payload for OKD encapsulates a new release of the underlying operating system
as well as new releases of the cluster components.

### Goals

The goals are to achieve OKD 4 builds in Prow, based on Fedora CoreOS and the current OpenShift codebase, and promotion of rolling releases from there,
as well as facilitation of community participation, adoption, feedback and knowledge sharing.

### Non-Goals

This enhancement does not concern itself with the releases of optional operators for OKD via OperatorHub.

## Proposal

This is a proposal to:

- Build OKD artifacts in OpenShift's Prow instance from the canonical OpenShift master branch and at least the latest release branch for continuous testing.
- Release OKD payloads built in and promoted from Prow from the latest release branch regularly, i.e. every 2 weeks.
- Support Fedora CoreOS as base OS in key OpenShift componentes (`machine-config-operator` and `installer`) so OKD releases can be made without requiring forked codebases.
- Provide easy-to-access ways for community members to participate:
  - by providing a repository with bug report templates
  - by encouraging and directing community contributors to send patches and bug fixes directly to the upstream OpenShift codebase
  - create documentation and share knowledge about OKD  

### Graduation Criteria

- [x] [OKD repository](https://github.com/openshift/okd/) exists for community issue and bug triage, and development tracking.
- [x] The UBI-based container images for all components in the release payload are shared between OKD and OCP for testing in Prow (with a few exceptions, see below)
- [x] OKD build and release jobs are added to `openshift/release`
- [x] Fedora RPM- and FCOS ostree-based `machine-os-content` container images are built continuously from the [`openshift/okd-machine-os`](https://github.com/openshift/okd-machine-os/) repository, with promotion gating for each minor version release stream.
- [x] Mirror OKD release payloads from the internal registry to quay.io  
- [x] Support and end-to-end CI testing is added for running `machine-config-operator`, and the `machine-config-daemon` on FCOS-based OKD clusters.
- [x] `machine-config-operator` sources do not require a fork to be built for OKD
- [x] Support and end-to-end CI testing is added for building OKD installer binaries
- [ ] `openshift-install` sources do not require a fork to be built for OKD: ([Pull Request](https://github.com/openshift/installer/pull/4453))
- [x] All container images that are part of `cluster-samples` and `marketplace` operators are continuously available as either Fedora, CentOS or UBI-based container images.
- [ ] All container images that are part of `baremetal-operator` are continuously available as either Fedora, CentOS or UBI-based container images. ([okd#197](https://github.com/openshift/okd/issues/197), [ironic-image#123](https://github.com/openshift/ironic-image/pull/123), [ironic-ipa-downloader#56](https://github.com/openshift/ironic-image/pull/123))

## Infrastructure Needed [optional]

- Limited Prow CI compute on all platforms for end-to-end testing
- Storage and/or registries for CI artifacts (RPMs, machine-os-content containers, and operator containers)
