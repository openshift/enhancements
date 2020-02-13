---
title: RHEL CoreOS Live ISO/installer
authors:
  - "@cgwalters"
reviewers:
  - "@coreos-team"
approvers:
  - "@coreos-team"
creation-date: 2020-02-07
last-updated: 2020-02-07
status: implementable
---

# Rebase RHEL CoreOS 4.5 on Fedora CoreOS $latest

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

RHEL CoreOS is built from RHEL components, but uses configuration and tooling from Fedora CoreOS.  They share many things such as the installer code, etc.

This enhancement tracks rebasing RHCOS 4.5 on the latest FCOS code, in particular the installer.

### New "Live" installer

Currently, RHCOS 4.4 and below offer a kernel/initramfs and ISO that can run a shell script installation program from the initramfs.

In FCOS $latest, there is a Live ISO/PXE media that runs the full OS (for example, it includes `podman`), and a completely revamped [coreos-installer](https://github.com/coreos/coreos-installer/).

## Motivation

This allows administrators to run arbitrary code before and after the installation process by providing Ignition that wraps the installer.  For example, higher level tooling like Foreman and Ironic can easily `curl` a URL to report provisioning success, or even pull down and launch a container before/after.

An additional goal is to allow administrators to more easily run RHCOS as a "live" system for *interactive* experiementation and testing.  For example, an administrator could run the system from a USB stick to validate the network interface names, etc.

### Goals

A live ISO and new `coreos-installer` binary are shipped, tested and work in all scenarios supported by the existing CoreOS installer, and contain the new functionality.  The legacy installer continues to be provided (for now).

### Non-Goals

Support for Live/diskless nodes joining a cluster.

## Proposal

Add the Live ISO to the  RHCOS build process.

The legacy installer continues to be built and tested.

Add a basic (virtualized) test case for the CoreOS installer.

Ship the new `coreos-install` binary at https://mirror.openshift.com and as an RPM in the OpenShift channel.

Switch the `e2e-metal` job in Packet to use the new CoreOS installer.

Update the documentation.

### Risks and Mitigations

It might be confusing to have two installers, and it will be an ongoing maintenance burden for the RHCOS team.

## Design Details

### Test Plan

- A basic virtualized kola test
- Convert e2e-metal Packet job

### Graduation Criteria


#### Examples

### Upgrade / Downgrade Strategy

It may be problematic for some customers to adjust their PXE or other automation setup to handle both new and old cases.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

If we have to carry forward the old CoreOS installer for a significant amount of time, maintenance will be a bit of a burden.

## Alternatives

Keep the existing CoreOS installer and add tweaks to it to report provisioning success, etc.  This would quickly approach the general case though that the new Live CoreOS installer handles.
