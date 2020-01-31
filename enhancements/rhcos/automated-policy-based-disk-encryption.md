---
title: automated-policy-based-disk-encryption-on-boot-images

authors:
  - "Ben Howard @behoward"

reviewers:
  - "Colin Walters @cgwalters"
  - "Steve Milner @ashcrow"
  - "Ian McLeod @imcleod"
  - "Micah Abbott @miabbott"

approvers:
  - "Colin Walters @cgwalters"
  - "Steve Milner @ashcrow"
  - "Ian McLeod @imcleod"
  - "Micah Abbott @miabbott"
 
creation-date: 2019-09-20
last-updated: 2019-09-20
status: provisional

---

# Automated Policy-based Disk Encryption on Boot Images

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Policy Definition
- [ ] Feature implemented and functional
- [ ] Test plan is defined
- [ ] Test plan implemented 
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- [ ] Transition Plan to Ignition controlled policy application

## Summary

Starting with OpenShift 4.3, Red Hat Enterprise Linux RHCOS (RHCOS) boot images will be "encryption ready".  Not encrypted by default, but operating system level encryption will be supported and configurable via Ignition.

## Motivation

The security of data-at-rest is of chief concern for end-users, organizations and enterprise Information Technology management. Every year, familiar companies have severe data breaches. Some industries require the encryption of data-at-rest, while customers of those industries demand it. Encryption of data is a best practice.

### Goals

This enhancement describes encryption of the root filesystem for Red Hat Enterprise Linux CoreOS.  Encryption is implemented with LUKS and Clevis.

### Non-Goals

The following are considered out of scope:
* Encryption of installations using an incompatible boot image.
* Changing Ignition.
* Storing of keys in-Cluster.
* Arbitrary LUKS configurations or disk-layouts (this means providing a separate `/var` partition via Ignition will not be encrypted)
* Re-formatting disk devices or partitions.
* Re-encryption of previously encrypted volumes.
* Encryption of previously provisioned nodes that were not encrypted.

## Terms
* LUKS/LUKS2: the standard Linux encryption method. Encrypted filesystems stored in a container.
* Master Key: the _actual_ encryption key used. The master key encrypted using a passphrase and is embedded in the LUKS meta-data on disk encrypted. This encryption key is unknown to users.
* LUKS. Passphrase: any string or file that can be used to unlock the master key. Each passphrase is bound to a "key-slot."
* LUKS Token: arbitrary JSON stored in the LUKS meta-data.
* NBDE: [Network Bound-Disk Encryption](https://access.redhat.com/documentation/en-US/Red_Hat_Enterprise_Linux/7/html/Security_Guide/sec-Using_Network-Bound_Disk_Encryption.html). A method of providing context-based disk encryption.
* [Clevis](https://github.com/latchset/clevis): Linux CLI tools for policy-based encryption and decryption. Clevis has a Dracut module for automatically opening LUKS containers in an NBDE context.
* Clevis pin: a program called by Clevis to encrypt and decryption of secrets.
* Tang: A server implementation that provides cryptographic binding services without the need for an escrow. Secrets are derived from what the client and the Tang server both have, reducing the attack surface.
* KMS: Key Management System, such as Amazon KMS.
* [TPM2](https://en.wikipedia.org/wiki/Trusted_Platform_Module): A secure cryptoprocessor for storing keys in hardware.
* Dracut Module: program(s) that run early in boot before the on-disk Linux is booted.

If you are unfamiliar with some of these terms [please see an excellent Youtube video on Clevis/Tang](https://youtu.be/2uLKvB8Z5D0).

## Proposal

To provide encryption, RHCOS boot images will:
* Be encryption ready by having root filesystem housed within a bare LUKS container configured with a `null` cipher and an empty passphrase.
* On first-boot, if an encryption configuration is provided, the root filesystem will be encrypted.
* Automated unlocking of the root file system.
* Unless the user opts-out, best-practice policies will be enforced for the encryption of the root filesystem.

On first boot, a Dracut module will encrypt the root-filesystem using a Clevis policy.  Like all Ignition stages, if this operation fails, the system will not complete booting.

The encryption step can take several minutes, depending on the CPU and disk I/O.

Clevis handles the automated unlocking of the root filesystem.

### User Stories

#### Story 1: Requirements

ACME Corp policy requires operating system level encryption to be used for their bare metal servers to preserve data confidentiality after hard drives are de-commissioned.  Their servers have TPM2 devices, and by configuring a binding of the root device to the TPM, the data on the hard drives in inaccessible after they are removed from the servers.  However, this is transparent to the system administrators and does not require them to manually enter a passphrase or key during boot.

#### Story 2: Upgrades

Clusters upgraded from earlier versions will have older boot images and hence not support encryption.  Addressing this is deferred until we have a plan for [updating bootimages](https://github.com/openshift/os/issues/381).

### Implementation Details/Notes/Constraints

This proposal introduces dependencies on RHCOS and the the Installer only. The vast majority of the work will be done through operating system level hooks.
* A new Dracut module will be added. Upstream `cryptsetup` has a in-tree [Dracut Module for disk-reencryption](https://gitlab.com/cryptsetup/cryptsetup/tree/master/misc/dracut_90reencrypt).  The module will need to be extended to support Cleivs configurations.
* RHCOS will need to add Clevis and its dependencies. Clevis provides TPM2 and Tang support upon installation and provides the backbone for extending to additional key-stores.
* Extend the Cloud CryptAgent to act as a Clevis Pin. This not a requirement for release. 
* The initramfs will have a `coreos-encrypt.service` systemd unit implementing this.

## Design Details

### On Disk Changes

RHCOS bootimages shipped with OpenShift 4.1 and 4.2 mostly follow the specification for [Fedora CoreOS filesystem layout](https://github.com/coreos/fedora-coreos-tracker/issues/18).  As of the latest 4.3 development, this is even closer because the `metal` image has unified BIOS/UEFI.

This proposal calls for moving the root filesystem into a LUKS container by default, with a `null` cipher.  This will allow us to easily re-encrypt with a strong key, without implementing full support for [reconfiguring the root filesystem storage](https://github.com/coreos/fedora-coreos-tracker/issues/94).

As of the time of this writing, OpenShift 4.3 development has implemented this.

On first boot, when a Clevis pin is provided, `cryptsetup-reencrypt` will be invoked.

### Policies

In order to aid backward compatibility, for OpenShift 4.3, operating-system level encryption will not be enabled by default.  Note that many IaaS (cloud) providers encrypt disks at the infrastructure level; for example, on AWS the OpenShift installer creates encrypted per-cluster AMIs.  On Google Compute Engine, disk images are automatically encrypted.

However, this proposal aims to support operating system level encryption for bare metal, as well as cloud deployments that require encryption at the operating system level.

### Installer Support

At the time of this writing, encryption is configured by providing custom `MachineConfig` objects [to the installer](https://github.com/openshift/installer/blob/master/docs/user/customization.md#install-time-customization-for-machine-configuration).

### Dracut Module

The upstream Crytpsetup project already has a Dracut module for [re-encrypting a root LUKS volume](https://gitlab.com/cryptsetup/cryptsetup/tree/master/misc/dracut_90reencrypt). This module will need to be extended to support using a NBDE policy. Its anticipated the extension work will be submitted upstream to the Clevis community. 

On first boot, the Dracut module will:
* Create a random passphrase and encrypt it using the policy.
* Encrypt the root LUKS volume.
* Bind the policy to the LUKS volume for Clevis.
* Destroy the random passphrase's key-slot.

If any of these steps fail, the node MUST panic with a message akin to:
```
FAILED TO APPLY ENCRYPTION POLICY FOR ROOT DISK!
ERROR finding TPM2 device.
```

### Clevis Pins

This idea hinges around Clevis, which provides an interface around different secret backend and includes a Dracut module for unattended boot. Clevis uses [JSON Object Signing and Encryption, (JOSE)](https://jose.readthedocs.io/en/latest/).

When Clevis binds a LUKS volume, a new JOSE object is written in the LUKS meta-data as a token. Clevis reads the JOSE object and then calls the appropriate pin. Clevis was added to Fedora 24 and RHEL 7, and it supported and recommended for unattended-encrypted boot. By adding new pins, administrators will have the flexibility to apply policy-based unattended boots.

[Container Linux did a proof-of-concept with LUKS v1 for NBDE unlocks bound to a Cloud KMS](https://github.com/coreos/coreos-cryptagent). Extending the CryptAgent to be a Clevis Pin should be straight forward and will allow for incremental approaches to adding and supporting different Cloud KMS.


### Policy Config Service

Updates for the policy binding will be provided by a "service" to monitor for Clevis configurations changes. As a _very_ simple example of the service as a SystemD Unit::
```
[Unit]
Description=Clevis Configuration Monitor
# clevis.json can be written by either Ignition or the Machine Config
ConditionPathChanged=/etc/clevis.json

[Service]
Type=oneshot
ExecStart=/bin/bash -c 'clevis-bind-luks -k /etc/clevis.key -d /dev/mapper/crypto-root'

[Install]
WantedBy=multi-user.target
```

In order to update a policy, the existing policy must be valid and accessable. This requirement stems from the need to use the existing policy to read the passphrase and then apply the the new policy. Once the new policy is appplied, the prior policy will be revoked.

### On-line Re-encryption

Starting with CryptSetup 2.20, and in the forth-coming RHEL 8.1, online re-encryption is provided. Online re-encryption is not possible in a bare, unencrypted LUKS volume. The other problem with online re-encryption is that it has a considerable cost in terms of speed *and* IO performance. If the disk is a network-attached volume, the blocking I/O on the device and CPU and network will effectively render the node unusable.

```
# cryptsetup reencrypt /dev/nvme0n1p3
Enter passphrase for key slot 0:
Auto-detected active dm device 'luks-a3fa8863-289a-4fdc-bb4d-f2848e66aa3e' for data device /dev/nvme0n1p3.
Progress:   5.1%, ETA 81:44, 12381 MiB written, speed  47.0 MiB/s
```

### Applying LUKS Configuration to existing volumes

`crypsetup` supports converting a filesystem and then doing a block-by-block re-write into a new LUKS container using the same physical space. This solution was rejected due to the speed costs: it is nearly 4.5x-5x slower.

### CoreOS Installer

The current [CoreOS Installer](https://github.com/coreos/coreos-installer/blob/master/coreos-installer) is a Dracut module. At a high-level it:
* parses the boot options
* downloads the image
* writes it disk
* saves parameters into /boot
* reboots

This design does not require any changes to the CoreOS installer. Once the CoreOS installer is done, the on-disk CoreOS will be booted and re-encrypted normally.

### Risks and Mitigations

#### Caveat Emptor

With LUKS, [a passphrase is *NOT* the master key used for encryption](https://gitlab.com/cryptsetup/cryptsetup/blob/master/FAQ#L56-63); the master key is encrypted in the LUKS header on disk. Passphrases are used to unlock the master key. Each passphrase is stored in a "key-slot," which can be removed (`cryptsetup luksKillSlot...`). [An attacker could use an old snapshot or block-level backup to defeat encryption](https://gitlab.com/cryptsetup/cryptsetup/blob/master/FAQ#L56-63). If a user cycles a Clevis configuration, then existing block-level backups and snapshots should be secured unless a re-encryption operation is performed.

##### LUKS overhead: null cipher

If a user opts out of the policy, then the root filesystem will be unencrypted in a bare LUKS container using the null cipher. When using the null cipher (no encryption), the overhead is negligible. The "overhead" really becomes cosmetic and introduces more steps to open a null-encrypted container.

To illustrate the point, the current disk format mount process is simply: `mount /dev/disk/by-label/rootfs /mnt`, while the new process would be:
```
# echo "" | cryptsetup luksOpen /dev/vda4 coreos_crytpfs
# mount /dev/mapper/coreos_cryptfs /mnt
```

Note: the above examples assumes a Linux rescue terminal with the dependencies.

##### Disaster Recovery

Once a policy is applied, by necessity, any disaster recovery requires access to the key-escrow service. This means:
* When bound to TPM2, the chasis is required. In the case of virtual TPM, the _exact same instance must be used_. 
* If bound to a network service (Tang or KMS), then the service must be accessible.

When Clevis binds the policy to a LUKS volume, it embeds a JSON document in a token named `clevis` and be extracted by:
```
# cryptsetup luksDump /dev/nbd0p3
<REDACTED>
Tokens:
  0: clevis
        Keyslot:  1
```
The token can then be dumped via `cryptsetup token export <device> --token-id <id>.` 

In the simple case of a TPM2 binding:
```
# cryptsetup token export  /dev/nbd0p3 --token-id 1 \
    | jq -r '.jwe.protected'  \
    | base64 -d \
    | jq '.'
{
  "alg": "dir",
  "clevis": {
    "pin": "tpm2",
    "tpm2": {
      "hash": "sha256",
      "jwk_priv": "<REDACTED>",
      "jwk_pub": "<REDACTED>",
      "key": "ecc"
    }
  },
  "enc": "A256GCM"
}
```

Assuming that the `clevis-luks-unlock` command is not available, a dump of the token can be read on another computer via:
```
# cat token.data \
    | jose fmt -j- -Og jwe -o- \
    | jose jwe fmt -i- -c \
    | tr -d '\n' \
    | clevis decrypt
```

The tools needed for manual extraction on a Red Hat based Linux system:
- `clevis`
- `clevis-luks`
- `jose`
- `cryptsetup`


### Test Plan

TBD


### Graduation Criteria

In order to be considered stable, RHCOS must:
* boot completely
* Support TPM2, Tang, and plaintext Clevis configurations
* Optionally, support KMS Clevis configuration
* Encryption on first boot
* React to a new Clevis Configuration
* Reboot completely unattended WITHOUT requiring user-intervention.

### Upgrade / Downgrade Strategy

This feature is an OS-level implementation and will not be downloadable for the `machine-os-content.`

In order to upgrade, new nodes will need to be provisioned with the supported boot image. Existing nodes will need to be decommissioned.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Speed: Encrypted data carries a CPU burden. If a user does not opt-out of encryption, then:
* The cost to re-encrypting the LUKS container will slow first boot down by at least 1 to 3 minutes.
* Disk throughput will be affected, as writes are CPU bound.
* Workloads that are both CPU and disk-bound will be impacted.

The cost to force encryption is best shown using a non-scientific test on a laptop:
```
root@:/tmp/test # cryptsetup benchmark --cipher=null
# Tests are approximate using memory only (no storage IO).
#     Algorithm |       Key |      Encryption |      Decryption
cipher_null-ecb        256b      7233.6 MiB/s      7503.9 MiB/s

root@:/tmp/test # cryptsetup benchmark --cipher=aes
# Tests are approximate using memory only (no storage IO).
# Algorithm |       Key |      Encryption |      Decryption
    aes-cbc        256b       864.5 MiB/s      2789.2 MiB/s
```

Note: Impact on will be workload dependent. See:  The [comparision of Linux encryption](https://www.phoronix.com/scan.php?page=article&item=ext4-crypto-418&num=1).

The default mode may create a false sense of security, see `Risks, and Migitations.`

## Alternatives

Previous ideas have explored various models, including:
* Embedded OSTree in the Initramfs and using Ignition to setup encryption at boot time. This idea was rejected as it would create a ridiculously "fat" Initramfs and require significant changes to first boot. Another variation was to have an `ignition-ostree-copy` module that would copy the on-disk image into the initramfs, re-format the root file system (e.g., LUKS) and then continue boot. This would require changing to Ignition v3.
* Teach Ignition about Clevis. Ignition is Linux-aware, but distribution agnostic and runs only at first boot. Clevis and the required pins are very much specific to the Linux distribution. Given that OpenShift is using Ignition v2 and Fedora CoreOS is using Ignition v3, it is not feasible yet.
* Encrypt the data, but not the operating system. Linux has, for some time, supported filesystem overlays for encryption via `ecryptfs.` LUKS-based block-level encryption is generally preferred over the file-based encryption of `ecryptfs.` The [performance of LUKS over ecryptfs is significant](https://www.phoronix.com/scan.php?page=article&item=ext4-crypto-418&num=1).

## Graphz
For a graphic image:
```
# http://www.graphviz.org/content/cluster
digraph initramfs {
    compound=true
    newrank=true
    size=8
    ranksep=0.1

    node [shape=box]

    clevis [label="clevis-luks-askpass"]

    subgraph cluster_normal {
        center=true
        basic [label="basic.target"]

        rootdev [label="initrd-root-device.target"]
        sysroot [label="sysroot.mount"]
        oproot [label="ostree-prepare-root.service"]
        rootfs [label="initrd-root-fs.target"]
        initrd [label="initrd.target"]

        basic -> clevis -> rootdev -> sysroot -> oproot -> rootfs -> initrd
    }

    subgraph cluster_firstboot {
        label="pulled in on first boot"

        reencrypt [label="clevis-reencrypt.service"]
        gpt [label="coreos-gpt-setup.service"]
        ignuser [label="ignition-setup-user.service"]
        ignbase [label="ignition-setup-base.service"]
        network [label="network.target"]
        igndisks [label="ignition-disks.service"]

        mountvar [label="coreos-mount-var.service"]
        ignmount [label="ignition-mount.service"]
        popvar [label="coreos-populate-var.service"]
        ignfiles [label="ignition-files.service"]
        igncomplete [label="ignition-complete.target"]

        gpt -> ignuser
        ignbase -> igndisks
        ignuser -> igndisks
        network -> igndisks

        mountvar -> ignmount -> popvar -> ignfiles -> igncomplete
    }

    basic -> gpt
    basic -> ignbase
    igndisks -> reencrypt -> clevis
    rootfs -> mountvar
    igncomplete -> initrd
}```
