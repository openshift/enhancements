---
title: coreos-image-in-release
authors:
  - "@zaneb"
reviewers:
  - "@hardys"
  - "@dtantsur"
  - "@elfosardo"
  - "@sadasu"
  - "@kirankt"
  - "@asalkeld"
  - "@cgwalters"
  - "@aravindhp"
  - "@jlebon"
  - "@sosiouxme"
  - "@dhellmann"
  - "@sdodson"
  - "@LorbusChris"
approvers:
  - "@hardys"
  - "@aravindhp"
creation-date: 2021-09-22
last-updated: 2021-09-22
status: implementable
see-also:
  - "/enhancements/coreos-bootimages.md"
---

# Include the CoreOS image in the release for baremetal

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The baremetal platform is switching from the OpenStack QCOW2 CoreOS image to
the live ISO (as also used for UPI). To ensure that existing disconnected
clusters can update to this, the ISO image will be included in the release
payload. This will be balanced out by removing the RHEL image currently shipped
in the release payload.

## Motivation

Currently, the deploy disk image (i.e. the image running IPA -
`ironic-python-agent`) is a RHEL kernel plus initrd that is installed (from an
RPM) into the `ironic-ipa-downloader` container image, which in turn is part of
the OpenShift release payload. When the metal3 Pod starts up, the disk image is
copied from the container to a HostPath volume whence it is available to
Ironic.

The target OS disk image is a separate CoreOS QCOW2 image. The URL for this is
known by the installer. It points to the public Internet by default and may be
customised by the user to allow disconnected installs. The URL is stored in the
Provisioning CR at install time and never updated automatically. The image
itself is downloaded once and permanently cached on all of the master nodes.
Never updating the image is tolerable because, upon booting, the CoreOS image
will update itself to the version matching the cluster it is to join. It
remains suboptimal because new Machines will take longer and longer (and more
and more bandwidth) to join as the cluster ages. This issue exists on all
platforms, and is the subject of a [long-standing enhancement
proposal](https://github.com/openshift/enhancements/pull/201). Other issues
specific to the baremetal platform are that boot times for bare metal servers
can be very long (and therefore the reboot is costly), and that support for
particular hardware may theoretically require a particular version of CoreOS.

We are changing the deploy disk image to use the same CoreOS images used for
UPI deployments. These take the form of both a live ISO (for hosts that can use
virtualmedia) and of a kernel + initrd + rootfs (for hosts that use PXE). When
upgrading an existing disconnected cluster, we currently have no way to acquire
these images without the user manually intervening to mirror them.

Like the QCOW2 provisioning disk image, the URLs for these images are known by
the installer, but they point to the cloud by default and would have to be
customised by the user at install time to allow disconnected installs.
Following a similar approach to that currently used with the QCOW2 also
effectively extends the limitation that we are not updating the provisioning OS
image to include the deploy image as well.

The agent itself (IPA) is delivered separately, in a container image as part of
the OpenShift release payload, so in any event we will continue to be able to
update IPA.

We wish to solve the problems with obtaining an up-to-date CoreOS by including
it in the release payload.

### Goals

* Ensure that no matter which version of OpenShift a cluster was installed
  with, we are able to deliver updates to IPA and the OS it runs on.
* Stop maintaining and shipping the non-CoreOS, RHEL-based IPA PXE files.
* Never break existing clusters, even if they are deployed in disconnected
  environments.

### Non-Goals

* Automatically switch pre-existing MachineSets to deploy with
  `coreos-installer` instead of via QCOW2 images.
* Update the CoreOS QCOW2 image in the cluster with each OpenShift release.
* Provide CoreOS images for platforms other than baremetal.
* Eliminate the extra reboot performed to update CoreOS after initial
  provisioning.

## Proposal

Build a container image containing the latest CoreOS ISO and the
`coreos-installer` binary. This container can be used e.g. as an init container
to make the ISO available where required. The `coreos-installer iso extract
pxe` command can also be used to produce the kernel, initrd, and rootfs from
the ISO for the purposes of PXE booting.

This image could either replace the existing content of the
`ironic-ipa-downloader` repo, be built from the new
`image-customization-controller` repo, or be built from a new repo.

### User Stories

As an operator of a disconnected cluster, I want to upgrade my cluster and have it to continue to work for provisioning baremetal machines.

As an operator of an OpenShift cluster, I want to add to my cluster new
hardware that was not fully supported in RHEL at the time I installed the
cluster.

As an operator of an OpenShift cluster, I want to ensure that the OS running on
hosts prior to them being provisioned as part of the cluster is up to date with
bug and security fixes.

### Implementation Details/Notes/Constraints

We will need to restore a [change to the Machine Config
Operator](https://github.com/openshift/machine-config-operator/pull/1792) to
allow working with different versions of Ignition that was [previously
reverted](https://github.com/openshift/machine-config-operator/pull/2126) but
should now be viable after [fixes to the
installer](https://github.com/openshift/installer/pull/4413).

The correct version of CoreOS to use is available from the [stream
metadata](https://github.com/openshift/installer/blob/master/data/data/rhcos-stream.json)
in the openshift-installer. This could be obtained by using the installer
container image as a build image. Deriving the container image from the
installer container has the convenient side-effect of always triggering a
rebuild when the version changes.

Because OpenShift container images are built in an offline environment, it is
not possible to simply download the image from the public Internet at container
build time. Instead this will be accomplished by *MAGIC*. Uploading the ISOs to
the lookaside cache in Brew is one possibility.

Different processor architectures require different ISOs (though all are
represented in the stream metadata). It may make sense to have a separate
Dockerfile for each architecture. For the foreseeable future, we only need to
be able to pull a container for the local architecture that contains an ISO for
that same architecture. In the future, it is possible that the containers for
each architecture's ISO may themselves need to be multi-architecture, so that
it is possible to run a container image containing an ISO for a *different*
architecture.

The RHCOS release pipeline may need to be adjusted to ensure that the latest
ISO is always available in the necessary location.

OKD uses (rebuilt) Fedora CoreOS images instead of RHEL CoreOS, and this will
need to be taken into account. It may be that the build environment for OKD
allows for a more straightforward solution there (like downloading the image
directly).

### Risks and Mitigations

If the build pipeline does not guarantee the latest RHCOS version gets built
into the container image, then baremetal platform users may miss out on the
latest bug and security fixes.

## Design Details

### Open Questions [optional]

How will we provide up-to-date images to the container build?

Are there similar internet access restrictions on the build environment for
OKD?

### Test Plan

The expected SHA256 hashes of the ISO and PXE files are available metadata in
the cluster, so we should be able to verify at runtime that we have the correct
image.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The container registry will always contain an image with latest ISO. This will
get rolled out to the `image-customization-controller` pod, and future boots of
the deploy image will be based on the new ISO. The restart of ironic during the
upgrade should ensure that any BaremetalHosts currently booted into the deploy
image (i.e. not provisioned) will be rebooted into the new one.

For the initial release, pre-existing clusters will continue to provision with
QCOW2 images (but now via the new CoreOS-based IPA). Since the MachineSets will
not be automatically updated, everything will continue to work after
downgrading again (now via the old RHEL-based IPA).

Newly-installed clusters cannot be downgraded to a version prior to their
initial install version.

### Version Skew Strategy

The Cluster Baremetal Operator should ensure that the
`image-customization-controller` is updated before Ironic, so that reboots of
non-provisioned nodes triggered by the Ironic restart use the new image.

## Implementation History

N/A

## Drawbacks

Users with disconnected installs will end up mirroring the ISO as part of the
release payload, even if they are not installing on the baremetal platform and
thus will make no use of it. The extra data is substantially a duplicate of the
ostree data already stored in the release payload. While this is less than
ideal, the fact that this change allows us to remove the RHEL-based IPA image
already being shipped in the release payload means it is actually a net
improvement.

## Alternatives

We could use the prototype
[coreos-diskimage-rehydrator](https://github.com/cgwalters/coreos-diskimage-rehydrator)
to generate the ISO. This would allow us to generate images for other platforms
without doubling up on storage. However it would be much simpler to wait until
building images for other platforms is actually a requirement before
introducing this complexity. The ISO generating process is an implementation
detail from the user's perspective, so it can be modified when required.

We could attempt to generate an ISO from the existing ostree data in the
Machine Config. However, there is no known way to generate an ISO that is
bit-for-bit identical to the release, so this presents an unacceptably high
risk as the ISO used will not be the one that has been tested.

We could attempt to de-duplicate the data in the reverse direction, by having
the Machine Config extract its ostree from the ISO. This is theoretically
possible, but by no means straightforward. It could always be implemented at a
later date if required.

We could require users installing or upgrading disconnected clusters to
[manually mirror the ISO](https://github.com/openshift/enhancements/pull/879).

## Infrastructure Needed

We will need to have the latest RHCOS images available for download into a
container image in the build environment.

We may need a new repo to host the Dockerfile to build the container image
containing CoreOS.
