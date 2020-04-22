---
title: improved-static-networking-configuration
authors:
  - "@miabbott"
reviewers:
  - "@ashcrow"
  - "@dustymabe"
  - "@dhellmann"
  - "@cgwalters"
approvers:
  - "@ashcrow"
  - "@cgwalters"
  - "@crawford"
  - "@imcleod"
  - "@runcom"
creation-date: 2020-04-22
last-updated: 2020-06-18
status: implementable
see-also: https://github.com/openshift/enhancements/pull/210
---

# Improved Static Networking Configuration

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary
This enhancement describes improvements to how static networking is configured on Fedora CoreOS (FCOS) and RHEL CoreOS (RHCOS).  It covers improving the configuration of static networking for bare metal and VMWare hosts.

## Motivation
Many customer environments require static networking and do not allow any type of DHCP server. This is most prominent in VMware environments where they struggle with the OVA images and are resisting the bare metal installer (via ISO) approach. This is also a challenge in bare metal environments due to an unfriendly and time sensitive interactive flow (i.e. catching the `grub` prompt). Due to the intersections of Ignition, Platform ID, and active networking in the `initrd`, many of the current RHCOS images append `ip=dhcp,dhcp6` and catching the bootloader on the console is not a pleasant experience. 

This is basic networking configuration and users assume basic configurations like this are possible. As RHCOS requires a functional networking config early in the boot process, this eliminates many of the existing mechanisms customers use to handle the assigning of static networking configs. Some of the OpenShift personas we target are not aware of `dracut` arguments and it’s not a great experience to expect them to understand very low-level details of how the OS works.

### Goals
  - Provide a guided configuration of networking and Ignition source information
  - Provide tooling to create customized ISOs that include unique Ignition configurations
  - Provide support for a VMWare "backchannel" to provide network configuration and Ignition source information

### Non-Goals
  - Providing a fully automated solution for providing static network configuration to a fleet of machines

## Proposal
For the bare metal use case, we are proposing the use of the new [live ISO](https://github.com/openshift/enhancements/pull/210) to provide an interactive install environment where users can configure their networking parameters and have them persisted into the installed system.  In this environment, users will be able to invoke a TUI (`nmtui`) that will allow them to configure networking.  We believe this functionality will initially be most useful for users deploying Openshift Container Platform (OCP) onto bare metal using the user provisioned infrastructure (UPI) workflow.

For users that do not wish to use the interactive method for configuring networking, the installer has the ability to embed the networking configuration via an Ignition config, resulting in a custom ISO that can be used to install to a host in an automated fashion.  This would be done via the new `coreos-installer embed iso` functionality.

Additionally, the installer has the ability to copy the network configuration from the live ISO environment into the initramfs of the installed system.  This allows users to configure the network of the installed host **before** Ignition needs to fetch any configs.  This is implemented as a flag to the intsaller:  `coreos-installer --copy-network ...`

For the VMware use case, the proposal is to the existing `guestinfo` data object to provide the static networking configuration.  See the [cloud-init datasource for VMware guestinfo](https://github.com/vmware/cloud-init-vmware-guestinfo) for an example of how the [guestinfo data object](https://pubs.vmware.com/vi3/sdk/ReferenceGuide/vim.vm.GuestInfo.html) can be extended and used in this way.

### User Stories

#### Story 1
A user wants to spin up a single FCOS node on bare metal using a static networking configuration.  They boot the live ISO and are dropped into an interactive FCOS environment.  They are able to use a tool to guide them to configure static networking for the host and provide a URL to the Ignition configuration.  The user runs `coreos-installer` after the configuration is complete and the host boots into the newly installed OS with static networking configured and Ignition config applied.

### Story 2
A customer has a fleet of bare metal machines they want to provision with RHCOS and static networking, that will later be used for the installation of OpenShift Container Platform (OCP).  They use some of their own automation to generate unique Ignition configs that include the static networking configuration.  Using similar automation, they use `coreos-installer iso embed` to embed the Ignition configs into unique ISOs per machine.  These ISOs can then be attached to the baseboard machine controller (BMC) or lights out management (LOM) of the host, at which point the hosts can be booted and RHCOS will be automatically installed with the static networking configured and Ignition config applied.  (See [this blog post](https://dustymabe.com/2020/04/04/automating-a-custom-install-of-fedora-coreos/) for an example of this use use case.)

### Story 3
A customer has a fleet of bare metal machines already deployed that they want to reuse for installing RHCOS and OCP.  They want to configure networking in the initramfs of these bare metal systems **before** the system fetches any remote Ignition configuration.  They would use some of their own automation to leverage `coreos-install iso embed` to create a custom ISO image that would include the "base" Ignition config of the target install, any NetworkManager keyfiles for configuring complex networking (VLANs, bonded interfaces, static IP configuration, etc) and a `systemd` service that runs `coreos-installer install ...` with the necessary arguments.  The ISO could then be attached to the BMC/LOM of the bare metal systems, where it would boot up into the ISO environment, run `coreos-installer` via the `systemd` unit, and land the necessary configuration and RHCOS disk image onto the underlying hardware.  After the install completes, the host would reboot into RHCOS with the initramfs configured per the NetworkManager keyfiles and proceed to fetch any additional Ignition configuration needed.

### Story 4
A customer wants to deploy OCP onto VMware vSphere and use static networking for their hosts.  For their specific environment, it is impractical for them to use the bare metal ISO install path.  During the definition of the VM using the OVA image, the customer provides data to a `guestinfo` property that specifies the static networking configuration in the form of `dracut` network kernel arguments.  When the VM is booted, the values in the `guestinfo` property are applied before networking is brought online and the VM continues to boot normally.

### Implementation Details/Notes/Constraints
This enhancement is being delivered across multiple projects and requires coordination among all of them.  This requires changes to at least:
  - `coreos-installer`
  - `ignition-dracut`
  - `coreos-assembler`
  - `afterburn`

### Risks and Mitigations
Landing the changes first in FCOS and then delivering them in RHCOS has been the model we've tried to ascribe to as much as possible.  However, there are often challenges in enabling the changes in RHCOS due to differences in things like software versions and kernel support.

If these changes are unable to be made to RHCOS, we may need to consider shipping newer versions of certain pieces of software in RHCOS that is not available in RHEL.

The biggest risk to delivering the enhancement to RHCOS is the ability to generate and deliver the live ISO (see [#210](https://github.com/openshift/enhancements/pull/210)).

At the very worst, we would continue to ship the legacy `coreos-installer` and artifacts for RHCOS, which would not contain any of the enhancements outlined here.

### Upgrades
These enhancements should only affect a subset of initial FCOS/RHCOS installs.  The upgrade path should not be affected.

Any hosts that have been installed with static networking configured should be able to upgrade successfully and maintain their networking configuration.

### Test Plan
  - Boot FCOS/RHCOS live ISO and confirm user is dropped into a live environment
    - Confirm the same experience on a network with DHCP and without DHCP
    - Confirm there is a tool (TUI perhaps) that can be used to configure networking
    - Confirm there is a message (motd?) that informs users about tool
    - Confirm the OS can be installed onto the underlying disk
    - Confirm the host can be rebooted and networking config persists into the installed system
  - Use `coreos-installer iso embed` to create unique ISO with provided Ignition config
    - Confirm the unique ISO can be booted after creation
    - Confirm the OS can be installed using the unique ISO without intervention
    - Confirm that DHCP or static networking configuration can be provided via embedded Ignition config
    - Confirm the host can be rebooted and networking config persists into the installed system
  - Create FCOS/RHCOS VM in VMware and provide `guestinfo` with networking parameters
    - Confirm static networking configuration can be provided to VM
    - Confirm DHCP networking configuration can be provided to VM
    - Confirm VM boots successfully using provided network config
    - Confirm VM reboots successfully and networking persists into the installed system

## Alternatives
- [RHCOS Ignition Fail to Live](https://github.com/openshift/enhancements/pull/256)
