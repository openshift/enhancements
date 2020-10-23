---
title: support-for-coreos-extensions
authors:
  - "@cgwalters"
reviewers:
  - "@ashcrow"
approvers:
  - "@ashcrow"
  - "@crawford"
  - "@imcleod"
  - "@runcom"
creation-date: 2020-04-21
last-updated: 2020-04-21
status: provisional
see-also:
replaces:
superseded-by:
---

# Support For CoreOS extensions

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes a MachineConfig fragment like:
```
extensions:
  - usbguard
```

This is the OpenShift version of the [Fedora CoreOS extension system tracker](https://github.com/coreos/fedora-coreos-tracker/issues/401).

That will add additional software onto the host - but this software will still be versioned with the host (included as part of the OpenShift release payload) and upgraded with the cluster.

## Motivation

We have requests to ship things like `usbguard` that are needed to meet compliance in some scenarios (and `usbguard` is very useful on bare metal), but `usbguard` also makes no sense to ship in an OS for the public cloud.

The `rpm-ostree` project has had as its goal from the start to re-cast RPMs as "operating system extensions" - much like how e.g. Firefox and its extensions work.  By default it operates as a pure image system, but packages can be layered on (and overridden) client side.

OpenShift is already making use of this today for the [realtime kernel](https://github.com/openshift/enhancements/blob/master/enhancements/support-for-realtime-kernel.md).

We propose continuing the trail blazed with the RT kernel by adding additional RPMs to `machine-os-content` that aren't part of the base OS "image", but can be installed later.  This is how `kernel-rt` works today; we added a `kernel-rt.rpm` that is committed to the container.  In the future though, we may instead ship a separate `machine-os-extensions` container, or something else.  The only API stable piece here is the `MachineConfig` field (same as for the `kernelType: realtime`).

An important way to think about this is that we can still ascribe a *single version number* to this set of content - all content is still versioned with and tested with the OS (and hence the main OpenShift 4 release image).

### Goals

- Add support for cluster operators (notably the MCO) to deploy non-containerized software that is tested and versioned along with the OS, but not installed by default
- Avoid forcing the authors of software like `usbguard` to maintain two ways to ship their software (as RPM and as a container)

### Non-Goals

- Direct support for installing RPMs *not* from the release image
- Support for traditional RHEL systems (see below)

## Proposal

1.  RHCOS build system is updated to inject `usbguard` (and other software) into `machine-os-content` as an RPM
2.  MachineConfig gains support for `extensions` that are layered on in the same way it applies OS updates and manages `kernel-rt`

### Implementation Details/Notes/Constraints

Note this will also likely enable us to move `open-vm-tools` out of the host and have the MCO install it on VSphere environments for example.

#### Adding Extensions

When adding a new extension the following criteria generally should be met:

- containerizing the technology is not an option
- the technology and content is shippable by Red Hat
- the technology is specific to a particular platform/cloud _or_ use case
- the technology is not necessary at first boot

### Risks and Mitigations

#### 3rd party RPMs

This will blaze a trail that will make it easier to install 3rd party RPMs, which is much more of a risk in terms of compatibility and for upgrades.

#### Non-Kubernetes native management

How one *ships* software has a huge impact on how one *manages* the software.  Shipping software as an RPM implies nowadays it usually runs as a `systemd` unit, parses config files in `/etc`, logs to the journal etc.

When one ships software as a container for Kubernetes, one can use `oc/kubectl` to inspect its status, it might offer custom resource definitions, etc.  And we have made investment in ensuring a "Kubernetes native" interface to important components.

Writing containers should be preferred - but, we cannot do that all at once, for all software that is relevant for host execution.

### Upgrades

The MCO will update the OS and extensions as one unit.  All shipped extensions will have been tested with the same version of the host.

## Alternatives

Multiple OS builds: Becomes a combinatorial nightmare.

Do nothing: RHCOS may continue to grow with every new case that is expected to be supported. This would mean packages, such as `usbguard`, special cloud agents, debug or development tools, and other packages would increase the size of the base image while only providing features to a subset of customers

Force `usbguard` authors to containerize: Not a technical problem exactly but...it is *really* hard to have two ways to ship software; see above.
