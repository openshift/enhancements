---
title: upgrade-coreos-deploy-image
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
  - "@cybertron"
  - "@dhellmann"
  - "@sdodson"
  - "@LorbusChris"
approvers:
  - "@hardys"
  - "@sadasu"
creation-date: 2021-08-24
last-updated: 2021-08-24
status: implementable
see-also:
  - "/enhancements/coreos-bootimages.md"
---

# Upgrades of the CoreOS-based deploy image

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

To ensure that ironic-python-agent runs on top of an up-to-date OS, we will
update the CoreOS image URLs in the baremetal Provisioning CR to the latest
specified by the release metadata. For users running disconnected installs, we
will require them to make the latest versions available and block further
upgrades until they do so.

## Motivation

Currently, the deploy disk image (i.e. the image running IPA -
ironic-python-agent) is a RHEL kernel plus initrd that is installed (from an
RPM) into the `ironic-ipa-downloader` container image, which in turn is part of
the OpenShift release payload. When the metal3 Pod starts up, the disk image is
copied from the container to a HostPath volume whence it is available to
Ironic.

The provisioning OS disk image is a separate CoreOS QCOW2 image. The URL for
this is known by the installer. It points to the cloud by default and may be
customised by the user to allow disconnected installs. The URL is stored in the
Provisioning CR at install time and never updated automatically. The image
itself is downloaded once and permanently cached on all of the master nodes.
Never updating the image is tolerable because, upon booting, the CoreOS image
will update itself to the version matching the cluster it is to join. It
remains suboptimal because new Machines will take longer and longer (and more
and more bandwidth) to boot as the cluster ages, and also because support for
particular hardware may theoretically require a particular version of CoreOS.
(The former issue at least exists on all platforms, and this is the subject of
a [long-standing enhancement
proposal](https://github.com/openshift/enhancements/pull/201).)

We are changing the deploy disk image to use CoreOS. This may take the form
of both an ISO (for hosts that can use virtualmedia) and of a kernel + initrd +
rootfs (for hosts that use PXE). Like the provisioning disk image, the URLs for
these are known by the installer, but they point to the cloud by default and
may be customised by the user to allow disconnected installs. IPA itself is
delivered separately, as a container image as part of the OpenShift release
payload. We do not wish to continue maintaining or shipping the
ironic-ipa-downloader as part of the payload as well, since it (a) is huge and
(b) requires maintenance effort. This effectively extends the limitation that
we are not updating the provisioning OS image to include the deploy image as
well, although we will continue to be able to update IPA itself.

Once the CoreOS-based deploy image is in place, we no longer need the QCOW2
image at all for newly-deployed clusters, since we can ‘provision’ by asking
CoreOS to install itself (using custom deploy steps in Ironic, exposed as a
custom deploy method in Metal³). However, to use this method on pre-existing
clusters requires updating any existing MachineSets, which is not planned for
the first release.

A naive approach to rolling out the CoreOS-based deploy image would mean that
upon upgrading from an existing cluster, we would no longer have a guaranteed
way of booting into _either_ deploy image:

* The existing deploy kernel + initrd will still exist on at least one master,
  but may not exist on all of them, and not all that do exist are necessarily
  the most recent version. Even if we found a way to sync them, we would have no
  mechanism to update the image to match the current Ironic version, or fix
  bugs, including security bugs.
* We have no way of knowing the URLs for the new deploy image, because they can
  only be supplied at install time by the installer.

### Goals

* Ensure that no matter which version of OpenShift a cluster was installed
  with, we are able to deliver updates to IPA and the OS it runs on.
* Stop maintaining the non-CoreOS RHEL-based IPA image within 1-2 releases.
* Never break existing clusters, even if they are deployed in disconnected
  environments.

### Non-Goals

* Automatically switch pre-existing MachineSets to deploy with `coreos-install`
  instead of via QCOW2 images.
* Update the CoreOS QCOW2 image in the cluster with each OpenShift release.
* Build the CoreOS images from the release payload.

## Proposal

We will both ship the code to use the CoreOS image for IPA and continue to ship
the current ironic-ipa-downloader container image (which has the RHEL IPA image
built in) in parallel for one release to ensure no immediate loss of
functionality after an upgrade.

The release payload [includes metadata](/enhancements/coreos-bootimages.md)
that points to the CoreOS artifacts corresponding to the current running
release. This includes the QCOW2, ISO, kernel, initrd, and rootfs. The actual
images in use are defined in the Provisioning CR Spec. These are fixed at the
time of the initial installation, and may have been customised by the user for
installation in a disconnected environment. Since OpenShift 4.9 there are
fields for each of the image types/parts, although in clusters installed before
this enhancement is implemented, only the QCOW2 field
(`ProvisioningOSDownloadURL`) is set.

The cluster-baremetal-operator will verify the image URLs as part of
reconciling the Provisioning CR.

If any of the `PreprovisioningOSDownloadURLs` are not set and the
`ProvisioningOSDownloadURL` is set to point to the regular location (i.e. the
QCOW location has not been customised), then the cluster-baremetal-operator
will update the Provisioning Spec to use the latest images in the
`PreprovisioningOSDownloadURLs`. The servers set by previous versions of the
installer are `rhcos-redirector.apps.art.xq1c.p1.openshiftapps.com` and
`releases-art-rhcos.svc.ci.openshift.org`. OKD uses a separate mirror for
(Fedora) CoreOS, and this too should be treated as a non-customised location.

The `ProvisioningOSDownloadURL` (QCOW2 link) will never be modified
automatically, since there may be MachineSets relying on it (indirectly, via
the image cache).

If the `ProvisioningOSDownloadURL` has been customised to point to a
non-standard location and any of the `PreprovisioningOSDownloadURLs` are not
set, the cluster-baremetal-operator will attempt to heuristically infer the
correct URL(s). It will do so by substituting the release version and file
extension with the latest version and appropriate extension (respectively)
wherever those appear in the QCOW path. The default paths include both short
version strings (of the form `/4\.[4-9]\b/`) and longer build versions (of the
form `/4[4-9]\.[0-9]{2}\.[0-9]{12}-[0-9]/`). The cluster-baremetal-operator
will then attempt to verify the existence of these files by performing an HTTP
HEAD request to the generated URLs. If the request succeeds, the
cluster-baremetal-operator will update the Provisioning Spec with the generated
URL. If it fails, the cluster-baremetal-operator will report its status as
Degraded and not Upgradeable. This will prevent upgrading, since the next major
release will *not* continue to ship ironic-ipa-downloader as a backup, until
such time as the user manually makes the required images available.
(Unfortunately this prevents even minor version upgrades, when it is only the
next major version that we really want to block.)

### User Stories

As an operator of a disconnected cluster, I want to upgrade my cluster and have it to continue to work for provisioning baremetal machines.

As an operator of an OpenShift cluster, I want to add to my cluster new
hardware that was not fully supported in RHEL at the time I installed the
cluster.

As an operator of an OpenShift cluster, I want to ensure that the OS running on
hosts prior to them being provisioned as part of the cluster is up to date with
bug and security fixes.

An an OpenShift user, I want to stop downloading a extra massive image that is
separate to the one used for cluster members, and based on a different
distribution of RHEL, as part of the release payload.

### Risks and Mitigations

If the HEAD request succeeds but does not refer to a valid image, we may report
success when in fact we will be unable to boot any hosts with the given image.
The expected checksum is available in the metadata, so the
machine-os-downloader should check it. However, if this fails it will block the
rollout of Metal³ altogether.

## Design Details

### Open Questions

* Do we need to restore a [change to the Machine Config
  Operator](https://github.com/openshift/machine-config-operator/pull/1792) to
  allow working with different versions of Ignition that was [previously
  reverted](https://github.com/openshift/machine-config-operator/pull/2126) but
  should now be viable after [fixes to the
  installer](https://github.com/openshift/installer/pull/4413)?
* Should we try to heuristically infer the URLs at all when they are missing,
  or just require the user to manually set them?

### Test Plan

We will need to test all of the following scenarios:

* Install with release 4.y
* Upgrade from release 4.(y-1) -> 4.y
* Simulated upgrade from release 4.y -> 4.y.z

The simulated upgrade will require modifying the image metadata inside the
release payload.

Furthermore, we will need to test each of these scenarios both with the default
image URLs and with custom URLs for disconnected installs.

In the case of disconnected installs, we should be sure test the scenario where
the user initially fails to make the new image available. This must block
further upgrades without breaking anything else (including the ability to
provision new servers), and recover completely once the image is provided.

In future releases we will need to test that new versions of IPA work with all
of the previous releases of CoreOS.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

See... everything above.

### Version Skew Strategy

Changes will happen once both the new metadata and a version of the
cluster-baremetal-operator that supports this feature are present. The order in
which these appear is irrelevant, and changes will only have any discernable
effect on BaremetalHosts newly added or deprovisioned after the update anyway.

## Implementation History

N/A

## Drawbacks

Users operating disconnected installs will be required to manually make
available the latest CoreOS images either before or after the initial upgrade.

Hosts that are running the ironic agent (i.e. that have not yet been
provisioned as cluster members) will be running out-of-date versions of CoreOS
(though not the agent) that may be missing specific hardware support or contain
bugs including CVEs. (However, this was already the case for the deployed image
in the window where it is updating itself, so the problem of missing hardware
support may not get any worse.) Updating to the latest images requires manual
intervention from the user.

## Alternatives

Use similar heuristics to also update the CoreOS image on each subsequent
release (including minor releases if the image in the metadata changes). In a
disconnected environment, if the image is not available we would mark the
operator as Degraded but leave it as Upgradeable provided that _some_ image is
specified.

Wait for it to be possible to [build images within the cluster using only the
release payload](https://github.com/openshift/enhancements/pull/201).

Instead of upgrading, have CoreOS somehow try to update itself in place before
running IPA. (This is likely to be slow, and it's not clear that it is even
possible since we will be running as a live ISO, not written to disk at this
point.)

Don't try to guess the locations of the images if they are not set, and require
the user to manually specify them.
