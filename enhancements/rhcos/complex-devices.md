---
title: complex-devices
authors:
  - "@arithx"
reviewers:
  - "@ashcrow"
  - "@miabbott"
  - "@darkmuggle"
  - "@imcleod"
  - "@cgwalters"
approvers:
  - "@ashcrow"
  - "@miabbott"
  - "@darkmuggle"
  - "@imcleod"
  - "@cgwalters"
creation-date: 2020-01-23
last-updated: 2020-02-11
status: provisional
---

# Complex Devices

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement is targeted at allowing the creation of complex device types
via Ignition. This enhancement would also support allowing these devices to be
the root device.

## Motivation

This proposal aims to give Ignition (spec v3) the ability to create LUKS devices
as well as custom root filesystems. This functionality will allow us to no longer
need to invoke `cryptsetup-reencrypt` on first boot for LUKS volumes, as well as
having RAID or any other filesystem format that Ignition supports as the root
filesystem.

### Goals

- LUKS volumes can be configured via Ignition
- RAID (0, 1, 5, 10) volumes can be configured via Ignition
- Custom root devices can be configured via Ignition

### Non-Goals

- Re-encryption of previously encrypted volumes.
- Non-first boot operations (e.x. encryption of previously provisioned nodes
  that were not encrypted)
- Nested configurations

## Proposal

There are multiple different areas that will require changes to support this
proposal.
- FCCT gains sugar that writes a file to `/boot` specifying how to mount root
  devices
- Ignition gains support for LUKS devices (RAID already supported)
- ostree work to allow re-laying the ostree on new root devices from inside the
  initrd
- New initrd module that runs very early in the boot process which can process
  a file in `/boot` and determine how to mount the root device
  * Could be as simple as passing through to Clevis for LUKS devices
  * Or, use kernel arguments
- New initrd module that runs in between the Ignition Disks & Files stages that
  lays down the ostree on new root devices (`ostree-redeploy-rootfs.service`)

### Implementation Details/Notes/Constraints

This implementation relies on a file written by Ignition in /boot to determine
how to mount the root device on subsequent boots. If this file is incorrect the
machine will fail to boot on subsequent boots.

The easy way for users to create an Ignition configuration that specifies a new
root device would be the use of FCCT which is outside of the OpenShift
installer. While it's still possible to specify the file by hand it could be
error-prone, leading to frustration as systems fail to mount the root device on
subsequent boots.

This feature will require Ignition spec v3 which marks it as a pre-requisite that
RHCOS has moved to it before this can land.

----

Boot Order (first boot):
 1. Ignition disks
     - Handles formatting disks / creating partitions, opening encrypted volumes (to allow
       for the creation of filesystems on encrypted volumes), closing encrypted volumes
 2. Dracut module runs parsing /boot file & unlocks sysroot
     - Needs to parse Ignition file for /boot file contents
 3. mount sysroot
 4. ostree-redeploy-rootfs
 5. Ignition mount
     - Opens & mounts non-root volumes
 6. Ignition files
 7. Ignition umount
     - Closes & umounts non-root volumes

----

Boot Order (subsequent boots):
 1. Dracut module runs parsing /boot file & unlocks sysroot
 2. mount sysroot

----

Other than the root filesystem everything else is opened / mounted by the booted
system (e.x. `/etc/crypttab`). The distribution owns mounting & unlocking the
root filesystem.

----

LUKS RHCOS currently has `rhcos.rootfs=luks`, use this to detect the upgrade case.

#### Examples

All examples are in FCC format (https://github.com/coreos/fcct).

How to specify a new root device:
```yaml
variant: rhcos
version: 1.2.0
storage:
  disks:
    - device: /dev/sdb
      wipe_table: true
      partitions:
        - number: 1
          label: root
  filesystems:
    - path: /
      device: /dev/disk/by-partlabel/root
      format: xfs
      wipe_filesystem: true
      contains_root: true
      label: root
```

Creating a LUKS (KeyFile) device:
```yaml
variant: rhcos
version: 1.2.0
storage:
  disks:
    - device: /dev/sdb
      wipe_table: true
      partitions:
        - number: 1
          label: example
  luks:
    - device: /dev/disk/by-partlabel/example
      label: example-luks
      key_file: "data/url/whatever"
  filesystems:
    - path: /example
      device: /dev/mapper/example-luks
      format: ext4
      wipe_filesystem: true
      label: example
```

Creating a Clevis (TPM2 + Tang) device:
```yaml
variant: rhcos
version: 1.2.0
storage:
  disks:
    - device: /dev/sdb
      wipe_table: true
      partitions:
        - number: 1
          label: example
  luks:
    - device: /dev/disk/by-partlabel/example
      label: example-luks
      uuid: 56470e32-9b67-435e-b0de-302f5f39941e
      clevis:
        - tpm2
        - tang:
          - url: http://tang.example
            thumbprint: <thumbprint>
  filesystems:
    - path: /example
      device: /dev/mapper/example-luks
      format: ext4
      wipe_filesystem: true
      label: example
```

----

LUKS section options:
- luks (list of objects): the list of LUKS volumes to be configured. Every LUKS volume must have a unique device.
  - device (string): the devices for the volume.
  - name (string): the name to use for the resulting luks volume.
  - label (string): the label to use for the resulting luks volume.
  - uuid (string): the uuid to use for the resulting luks volume.
  - cipher (string): the cipher to use for the resulting luks volume.
  - key_file (string): the path to the key file for the resulting luks volume.
  - hash (string): the hash to use for the resulting luks volume.
  - clevis (list of objects):
    - tpm2 (flag): whether to use tpm2
    - tang (list of objects):
      - url (string): url to the tang device
      - thumbprint (string): the thumbprint of the tang device
  - options (list of strings): any additional options to be passed to luksFormat.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

TBD

High level:
- New E2E jobs:
  - Non-complex, non-standard root device (XFS -> EXT4)
  - RAID root device
  - LUKS root device w/ Clevis TPM2 + Tang bindings
- Upgrade test:
  - Old LUKS `rhcos.rootfs=luks`
- New kola tests:
  - Creating a LUKS device
  - Creating a RAID device
  - Mock Tang
  - Upgrade test
  - Seperate LUKS for root and /var/lib/container
    - Clevis LUKS root and TPM /var/lib/container
    - Clevis LUKS root and KEY file /var/lib/container
  - Creating non-standard root filesystems

### Graduation Criteria

In order to be considered stable, RHCOS must:

- Boot & reboot completely
- Support custom Ignition defined root devices
  - Setup the disks/partitioning, RAID/LUKS and file systems
  - Support OSTree, user-data and then /var/lib/containers on different blocks
- Support configuring LUKS & RAID volumes via Ignition
- Not require user-intervention
- Full KOLA and E2E testing

### Upgrade / Downgrade Strategy

There is no downgrade.

Upgrade will require new boot image since this is Ignition bounded.

## Drawbacks

- Pushes more users to use custom Ignition configurations
- Greater customizability of the underlying OS exponentially expands the amount
  of potential test cases

## Alternatives

1. Store how to find/mount root device in kernel command-line arguments
    * Pros:
        * Relatively Simple
        * Transparent, matches with more traditional methods of specifying root devices.
    * Cons:
        * Kernel command-line max length varies on different architectures (only 896 on s390x)
        * Difficult to represent if complex setups are used
2. Regenerate initrd to include configuration to start complex devices
    * Pros:
        * Requires less new code in initramfs
    * Cons:
        * Initramfs content can now be difficult; makes debugging harder
        * Harder to use signed initramfs
        * Requires new bootloader entries
