---
title: coreos-bootimage-streams
authors:
  - "@cgwalters"
reviewers:
  - "@coreos-team"
approvers:
  - "@coreos-team"
creation-date: 2021-03-04
last-updated: 2021-03-04
status: provisional
---

# Standardized CoreOS bootimage metadata

This is a preparatory subset of the larger enhancement for [in-cluster CoreOS bootimage creation](https://github.com/openshift/enhancements/pull/201).

This enhancement calls for a standardized JSON format for (RHEL) CoreOS bootimage metadata to be available via 3 distinct mechanisms:

- In cluster as a ConfigMap: `oc -n openshift-machine-config-operator get configmap/coreos-bootimages`
- Via `openshift-install coreos print-stream-json`
- At https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos-4.8.json

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Since the initial release of OpenShift 4, we have "pinned" RHCOS bootimage metadata inside [openshift/installer](https://github.com/openshift/installer).
In combination with the binding between the installer and release image, this means that everything needed to install OpenShift (including the operating system "bootimages" such as e.g. AMIs and OpenStack `.qcow2` files) are all captured behind the release image which we can test and ship as an atomic unit.

We have a mechanism to do [in place updates](https://github.com/openshift/machine-config-operator/blob/master/docs/OSUpgrades.md), but there is no automated mechanism to update "bootimages" past a cluster installation.

This enhancement does not describe an automated mechanism to do this: the initial goal is to include this metadata in a standardized format in the cluster and at mirror.openshift.com so that UPI installations can do this manually, and we can start work on an IPI mechanism.

### Stream metadata format

As part of unifying Fedora CoreOS and RHEL CoreOS, we have standardized on the "stream metadata" format used by FCOS.  More in [FCOS docs](https://docs.fedoraproject.org/en-US/fedora-coreos/getting-started/) and [this RHCOS issue](https://github.com/openshift/os/issues/477).

There is a new [stream-metadata-go](https://github.com/coreos/stream-metadata-go) library to consume this data.

### Add stream metadata to openshift/installer

We will continue to have the openshift/installer git repository be the source of truth for pinned RHCOS boot images.

However, we will convert the data there to stream metadata JSON, and port the IPI installer flow to use that.

### Add openshift-install coreos print-stream-json

A new command to simply dump this JSON can be used by UPI installs.

### Update the `installer` container image to inject a configmap

In order to work on automated in-place bootimage updates, the data needs to be lifecycled
with the cluster release image.  There is already an `installer` image as part of the
release payload that just contains the installer binary today.  This enhancement
calls for adding `manifests/` directory to that and having the CVO treat it as a minimal
"operator" that just updates the configmap.

This enhancement calls for installing this configmap into the `openshift-machine-config-operator` namespace;
logically the functionality is split between machineAPI and MCO, but it doesn't ultimately matter
which namespace has the configmap.


### Data available at https://mirror.openshift.com

The way we deliver bootimages at http://mirror.openshift.com/pub/openshift-v4/x86_64/dependencies/rhcos/4.7/latest/ is not friendly to automation.  By placing the stream metadata JSON there, we gain a standardized machine-readable format.

The ART team will synchronize the stream metadata with the data embedded in the latest release image for a particular OpenShift minor.


## Motivation

### Goal 1: Including metadata in the cluster so that we can later write automated IPI/machineAPI updates

Lay the groundwork so that one or both of the MCO or machineAPI operators can act on this data, and e.g. in an AWS cluster update the machineset to use a new base AMI.

### Goal 2: Provide a standardized JSON file UPI installs

See above - this JSON file will be available in multiple ways for UPI installations.

### Goal 3: Provide a standardized JSON file for bare metal IPI and baremetal devscripts

Bare metal IPI and [baremetal devscripts](https://github.com/openshift-metal3/dev-scripts/blob/7e4800462fa7e71aaa9e4a7f4eb10166a6b1789c/rhcos.sh#L14) also
parse the existing `rhcos.json`; it will be significantly better for them to use the
`openshift-install coreos print-stream-json` command.
### Non-Goals

#### Replacing the default in-place update path

In-place updates as [managed by the MCO](https://github.com/openshift/machine-config-operator/blob/master/docs/OSUpgrades.md) today works fairly seamlessly.
We can't require that everyone fully reprovision a machine in order to do in-place updates - that makes updates *much* more expensive, particularly on bare metal environments.
It implies re-downloading all container images, etc.

Today in OpenShift 4, the control plane is also somewhat of a "pet" - we don't have streamlined support for reprovisioning control plane nodes even in IaaS/cloud and hence must continue to do in-place updates.

At some point, along with the [larger in-cluster bootimage generation enhancement](https://github.com/openshift/enhancements/pull/201) we hope to streamline bootimage updates sufficiently that at some point we could *require*
newly scaled up workers to use them.  But this enhancement will not add any such requirement.

### User Stories

#### Story 1

An OpenShift core developer can write a PR which reads the configmap from the cluster and acts on it to update the machinesets to e.g. use a new AMI.

We can start on other nuanced problems like ensuring we only update the machinesets once a controlplane update is complete, or potentially even offering an option in IPI/machineAPI installs to drain and replace workers instead of doing in-place updates.

#### Story 2

ACME Corp runs OpenShift 4 on vSphere in an on-premise environment not connected to the public Internet.  They have (traditional) RHEL 7 already imported into the environment and already pre-configured and managed by the operations team.

The administrator boots an instance there, logs in via ssh, downloads an `oc` binary.  They proceed to follow the instructions for preparing a [mirror registry](https://docs.openshift.com/container-platform/4.7/installing/install_config/installing-restricted-networks-preparations.html).

The administrator also uses `openshift-install coreos print-stream-json` and writes a script to parse the JSON to find the vSphere OVA and download it.  Then the administrator uploads it to the vSphere instance.

From that point, the operations team can use `openshift-install` in UPI mode, referencing that already uploaded bootimage and the internally mirrored OpenShift release image content.

### Risks and Mitigations

We may discover even more things depend on `rhcos.json` inside the installer.  We may have to fall back to continuing to maintain a copy in the installer git (in the old format) for a cycle.

## Design Details

### Test Plan

This will be well covered by existing CI flows for IPI.  For UPI, it will become trickier because the jobs will need to become more OpenShift version dependent.

### Graduation Criteria

TBD

### Version Skew Strategy

We already have to deal with problems of skew in UPI installs in particular - things like administrators trying to use a 4.2 vSphere OVA to install 4.7, etc.  This standardizes an API for the future around discovering and maintaining these images.

## Implementation History

## Drawbacks

## Alternatives

None, we need to do something here.
