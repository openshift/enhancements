---
title: CoreOS Encrypted Disks By Default
authors:
  - "Ben Howard @behoward"
reviewers:
  - "Colin Walters @cgwalters"
  - "Steve Milner @ashcrow"
  - "Ian McLeod @imcleod"
  - "Micah Abbott @miabbott"
approvers:
  - TBD
creation-date: 2019-09-20
last-updated: 2019-09-20
status: provisional
---

# CoreOS-Encrypted-by-Default

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]
- [ ] Transition Plan defined for Ignition handling of LUKS root volumes

## Summary

Openshift clusters will be encrypted at the operating system level by default. CoreOS boot images starting with 4.3 will be encrypted, and at first boot will be re-encrypted. Openshift nodes will (re)boot without administrator intervention.

## Motivation

The security of data-at-rest is of chief concern for end-users, organizations and enterprise Information Technology management. Every year, familiar companies have severe data breaches. Some industries require the encryption of data-at-rest, while customers of those industries demand it. Encryption of data is a best practice.

### Goals

This enhancement is to provide enterprise-grade encryption by default at the operating system level and to provide flexibility regarding the configuration.

This means:
* On first-boot, boot images based on CoreOS will be encrypted using standard AES-256 encryption.
* OS-Level hooks will be included to support this feature
* Automated boot/reboot handled by Clevis using "pins" for
    * TPM2
    * Tang
    * Cloud KMS build around the CoreOS CryptAgent
    * plaintext to support pass-through
* Use of FIPS 140-2 compliant ciphers, hashes and checksum algorithms.
* User intervention MUST not be required to initialize or (re)boot a cluster.
* Users will have the ability to opt-out of encryption by default.

### Non-Goals

The following are considered out of scope:
* Encryption of installations not based on a compatible boot image.
* Changing Ignition.
* Installer support.
* Storing of keys in-Cluster.
* Arbitrary LUKS configurations or disk-layouts.
* Re-formatting disk devices or partitions.

### Fedora CoreOS (FCOS) and Red Hat CoreOS (RHCOS)

It is worth stating that this proposal is not mutually exclusive for any plans that FCOS may have towards root disk layout. FCOS and RHCOS would like to reach alignment and feature parity.

With the exclusion of the on-disk format, this work should lay the groundwork for an unattended encrypted boot that can be used by both FCOS and RHCOS.

Ignition is the first boot agent for FCOS and RHCOS. FCOS discussed the topic of partition layouts, with the idea of having Ignition lay-down arbitrary root filesystem layouts. At this time, Openshift requires Ignition V2, while FCOS uses Ignition v3; these versions are incompatible. When Ignition supports the required depenendencies, a reviewion to this proposal will be forthcoming.

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

To provide install-time _and_ rolling re-encryption, CoreOS boot images will:
* Have the root filesystem housed in a LUKS container.
* Use the 'null-cipher' for the initial LUKS container "encryption". This allows _any_ passphrase to unlocked the LUKS container.
* The passphrase will be stored in a LUKS token via the Clevis plaintext pin.

Note: the default boot image is _effectively *NOT* encrypted_.

On first boot, a Dracut module will re-encrypt the LUKS container unless the kernel CLI parameter ('rd.coreos.no_encryption=1') is used.  The module will likely run after Ignition has completed. It will:
  * Prepare a random passphrase,
  * Randomize the LUKS container's UUID,
  * Re-encrypt the LUKS container using AES-256-CBC (FIPS 140-2 compliant). This process generates a new random master key.
  * If a Clevis configration is found, apply it in the initramfs. Otherwise:
      * if a TPM2 device is found, bind the LUKS container to TPM2.
      * if no TPM2 device, use the plaintext Clevis pin


The Dracut modules will also allow both first-boot and reboot re-encryption using kernel commandline arguments. The re-encryption step is necessary to ensure that the LUKS master key is unique to each instance of CoreOS. In testing, the re-encryption step takes between 1 and 3 minutes, depending on the CPU and the backing disk-type.

Afer first-boot, a Linux systemd service will provide for re-applying a Clevis configuration. If the service finds a configuration, it will:
  * apply the Clevis configuration
  * write a new random passphrase in /root
  * revoke the prior Clevis configuration

The service will be activated when `/etc/clevis.json` is found, alloing for a Clevis profile provided installation time, or through a Machine Config. Once a Clevis configuration has is applied, the LUKS container will be automatically opened at boot if the conditions of the configuration allow so.  Otherwise, to facilitate unattended boot, then a plaintext Clevis configuration will be used.

### User Stories

#### Story 1: Requirements

ACME Corp is only able to consider technologies that have encrypted disks, but they want to use Google Cloud. With Openshift's 4.3 boot images, they can evaluate Openshift.

#### Story 2: Compliance

ACME Corp deploys a new Openshift cluster to Microsoft Azure. During an audit, the compliance officer notes that data should be encrypted at rest with a key-escrow in Cloud KMS. During lunch, the IT department deploys a Machine Config with a Clevis configuration, which CoreOS actions. After lunch, the IT department reports that the cluster is compliant with the policy.

#### Story 3: Upgrades

ACME Corp has a 4.1 cluster and is upgrading to 4.3 and wants to use the new encryption. The IT department simply:
* Adds their Clevis machine config set
* [updates the machine set](https://docs.openshift.com/container-platform/4.1/machine_management/creating-machineset.html)
* Scales up new nodes
* Scales down the old nodes

#### Story 4: Edge Clusters

ACME Corp deploys sensitive bare-metal clusters to the edge. They build their clusters with a Clevis profile using TPM2 and Tang. Evil Corp clandestinely steals a few of ACME Corp's nodes. Since ACME Corp's nodes are configured using NBDE encryption, no-data is compromised. ACME Corp avoids being shamed on the evening news and withstands the ire of the Board of Directors.

### Implementation Details/Notes/Constraints [optional]

This proposal introduces dependencies on CoreOS only; it is an OS-level implementation. Openshift itself will entirely unaware that CoreOS is encrypted.
* A new Dracut module will be added. Upstream `cryptsetup` has a in-tree [Dracut Module for disk-reencryption](https://gitlab.com/cryptsetup/cryptsetup/tree/master/misc/dracut_90reencrypt).  The module will need to be extended to support Cleivs configurations.
* CoreOS will need to add Clevis and it's dependencies. Clevis provides TPM2 and Tang support upon installation and provides the backbone for extending to additional key-stores. Two new Clevis pins will be written:
    * A Cloud KMS pin for talking to popular cloud KMS.
    * A plaintext pin for encoding plaintext secrets.
* A new Linux systemd unit for handling application of clevis configuration

## Design Details

### On Disk Changes

The current partitioning for a CoreOS images is:
```
Model: Unknown (unknown)
Disk /dev/nbd0: 17.2GB
Sector size (logical/physical): 512B/512B
Partition Table: gpt
Disk Flags:

Number  Start   End     Size    File system  Name        Flags
 1      1049kB  404MB   403MB   ext4         boot
 2      404MB   537MB   133MB   fat16        EFI-SYSTEM  boot, esp
 3      537MB   538MB   1049kB               BIOS-BOOT   bios_grub
 4      538MB   17.2GB  16.6GB  xfs          rootfs
 ```

In this idea, partition 4, will move the root XFS filesystem into a LUKS container:
```
Model: Unknown (unknown)
Disk /dev/nbd0: 17.2GB
Sector size (logical/physical): 512B/512B
Partition Table: gpt
Disk Flags:

Number  Start   End     Size    File system  Name        Flags
 1      1049kB  404MB   403MB   ext4         boot
 2      404MB   537MB   133MB   fat16        EFI-SYSTEM  boot, esp
 3      537MB   538MB   1049kB               BIOS-BOOT   bios_grub
 4      538MB   17.2GB  16.6GB               crypt-root

# blkid /dev/nbd0p4
/dev/nbd0p4: UUID="1a7086bd-6514-457c-bc06-1215944f026d" TYPE="crypto_LUKS" PARTLABEL="root" PARTUUID="01317d04-4b83-463e-8762-5a07ec2499e6"
```

CoreOS today uses `/dev/disk/by-label` for mounting the root-filesystem. Once the LUKS container `crypt-root` is opened, `/dev/disk/by-label/rootfs` will appear.


As an example, partition 4 could be prepared by creating a well-known plain-text password (e.g. 'coreos') using the `cipher_null`. When `cipher_null` is used, any passphrase is acceptable.
```
# echo "coreos" > /tmp/disk.key
# cryptsetup luksFormat \
    -q \
    --label="coreos_rootfs-default" \
    --cipher=cipher_null \
    --key-file=/tmp/disk.key \
    /dev/nbd0p4
```

Perform the LUKS binding (example of future command)
```
# clevis-luks-bind \
       -d /dev/nbd0p4 \
      plaintext  '{"passphrase": "doesNotMatter"}'
```

During the first boot, a Dracut module will create a new random-key and then re-encrypt the device. For example:
```
# head -c 256 > new_key
# echo coreos |
  cryptsetup-reencrypt \
     --cipher="aes-cbc-essiv:sha256" \
     --key-size=256 \
     --progress-frequency=5 \
     --key-file=new_key
     /dev/nbd0p4
```

In the above step, a new LUKS master key created, and each block is re-encrypted. After the encryption, an appropriate Clevis configuration will be applied and the `new_key` will be removed; future encryption operations will use existing Clevis configruation.

### Clevis Pins

This idea hinges around Clevis, which provides an interface around different secret backend and includes a Dracut module which provides for unattended boot. Clevis uses [JSON Object Signing and Encryption, (JOSE)](https://jose.readthedocs.io/en/latest/).

When Clevis binds a LUKS volume, a new JOSE object is written in the LUKS meta-data as a token. Clevis reads the JOSE object and then calls the appropriate pin. Clevis was added to Fedora 24 and RHEL 7, and it supported and recommended for unattended-encrypted boot. By adding new pins, administrators will have the flexibility to apply policy-based unattended boots.

The first pin deals with the null case, where there is neither a TPM2 device or a Clevis binding for NBDE boot. This pin would be used to store an encoded plaintext password. In the event of the plaintext pin is used, the actual passphrase is obfuscated (here be dragons: the axiom that "security by obscurity is not security" applies). The encryption function of pin would encode the passphrase in a JOSE object using base64 encoding, while the decrypt functionality would do the inverse.

The second pin would deal with the Cloud KMS. [Container Linux did a proof-of-concept with LUKS v1 for NBDE unlocks bound to a Cloud KMS](https://github.com/coreos/coreos-cryptagent). Extending the CryptAgent to be a Clevis should be straight forward and will allow for incremental approaches to adding and supporting different Cloud KMS.

Adding Clevis is relatively straight forward. Each pin provides a `clevis-{encrypt,decrypt}-<NAME>`, when ``<NAME>`` is the name of the pin (e.g. `clevis-encrypt-plaintext`). Pins may be written in any language; most are written in bash. This gives the flexibility to include new pins as the circumstances require.

### Clevis Config Service

The proposed design for the "service" to monitor for Clevis configurations. As a _very_ simple example of the service could be:
```
[Unit]
Description=Clevis Configuration Monitor
# clevis.json can be written by either Ignition or the Machine Config
ConditionPathChanged=/etc/clevis.json
# clevis.key contains the passphrase so LUKS key-slots can be added
ConditionPathExists=/root/clevis.key

[Service]
Type=oneshot
ExecStart=/bin/bash -c 'clevis-bind-luks -k /etc/clevis.key -d /dev/mapper/crypto-root'

[Install]
WantedBy=multi-user.target
```

### On-line Re-encryption

Starting with CryptSetup 2.20, and in the forth-coming RHEL 8.1, online re-encryption is provided. Online re-encryption is problematic when using the 'cipher_null' algorithm: the key size is zero. Online re-encryption re-uses the passphrase when the target "encrypted," and thus will fail the one cipher has a key size of 0, while the other requires a key size greater than 0. The other problem with online re-encryption is that it has a considerable cost in terms of speed *and* IO performance. If the disk is a network-attached volume, the blocking I/O on the device and CPU and network will effectively render the node unusable.

```
# cryptsetup reencrypt /dev/nvme0n1p3
Enter passphrase for key slot 0:
Auto-detected active dm device 'luks-a3fa8863-289a-4fdc-bb4d-f2848e66aa3e' for data device /dev/nvme0n1p3.
Progress:   5.1%, ETA 81:44, 12381 MiB written, speed  47.0 MiB/s
```

The process is rather straight forward:
* apply new Clevis Luks binding
* remove existing bindings and residual key-slots
* start re-encryption
* watch for failure(s)

One interesting feature of online re-encryption that with the use of a "detached-header" for LUKS, nodes that have upgraded from unsupported versions. This would allow for an un-encrypted filesystem to be re-encrypted in place, with the LUKS meta-data externally (for example in /boot/rootfs.luks). While it is conceivable that such an option might be viable, at this time, it out-of-scope due to the complexities involved.


### Dracut Module

The upstream [Cryptsetup](https://testgrid.k8s.io/redhat-openshift-release-informing) project already has a Dracut module for [re-encrypting a root LUKS volume](https://gitlab.com/cryptsetup/cryptsetup/tree/master/misc/dracut_90reencrypt). As part of the implementation, this Dracut module can be modified to support unattended and first-boot encryption.

Rather than provide a new-new module, the upstream module would be modified to support Clevis configurations and submitted to the Clevis upstream.

### CoreOS Installer

The current [CoreOS Installer](https://github.com/coreos/coreos-installer/blob/master/coreos-installer) is a Dracut module. At a high-level it:
* parses the boot options
* downloads the image
* writes it disk
* saves parameters into /boot
* reboots

This design does not require any changes to the Installer. Once the CoreOS installer is done, the on-disk CoreOS will be booted and re-encrypted normally.

### Risks and Mitigations

#### Caveat Emptor
The clearest risk to the design is that while the data will be encrypted by default, the installation will determine **if a passphrase is in plain text** (this makes it trivial for an attacker to decrypt the data).  There are only two cases where the keys are held secure:
* the user provides a Clevis configuration that uses a secure key escrow (Tang or KMS)
* the hardware or Cloud supports TPM2

This risk cannot be understated and should be clearly and explicitly called out in the user documentation. There is an axiom in encryption that no encryption is better than bad encryption as it can create a false sense of security.

With LUKS, [a passphrase is *NOT* the master key used for encryption](https://gitlab.com/cryptsetup/cryptsetup/blob/master/FAQ#L56-63); the master key is encrypted in the LUKS header on disk. Passphrases are used to unlock the master key. Each passphrase is stored in a "key-slot," which can be removed (`cryptsetup luksKillSlot...`). [An attacker could use an old snapshot or block-level backup to defeat encryption](https://gitlab.com/cryptsetup/cryptsetup/blob/master/FAQ#L56-63). If a user cycles a Clevis configuration, then existing block-level backups and snapshots should be secured unless a re-encryption operation is performed.

##### LUKS overhead: null cipher

When using the null cipher (no encryption), the overhead is negligible. The "overhead" really becomes cosmetic and introduces more steps to open a null-encrypted container.

To illistrate the point, the current disk format mount process is simply: `mount /dev/disk/by-label/rootfs /mnt`, while the new process would be:
```
# clevis-luks-unlock \
      -d /dev/disk/by-label/coreos_cryptfs \
      coreos_cryptfs

# mount /dev/mapper/coreos_cryptfs /mnt
```
Note: the above examples assume a Linux rescue terminal with the dependencies.

#### Drift between FCOS and RHCOS

There is a considerable question as to the desirability of this feature in Fedora CoreOS. Both Red Hat CoreOS and Fedora CoreOS have different goals: the former is targeted at Openshift clusters, while the later is targeted at running container workloads (including clusters). There is some concern that the Fedora CoreOS community will not find this solution desirable and thus Red Hat will use a LUKS container while Fedora will not.

### Test Plan

TBD


### Graduation Criteria

In order to be considered stable, CoreOS must:
* boot completely
* Support TPM2, Tang, and plaintext Clevis configurations
* Optionally, support KMS Clevis configuration
* Re-encrypt on first boot
* React to a new Clevis Configuration
* Reboot completely unattended WITHOUT requiring user-intervention.

### Upgrade / Downgrade Strategy

This feature is an OS-level implementation and will not be downloadable for the `machine-os-context.`

In order to upgrade, new nodes will need to be provisioned with the support boot image. Existing nodes will need to be decommissioned.

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
* embedded an OSTree in the Initramfs and using Ignition to setup encryption at boot time. This idea was rejected as it would create a ridiculously "fat" Initramfs and require significant changes to first boot. Another variation was to have an `ignition-ostree-copy` module that would copy the on-disk image into the initramfs, re-format the root file system (e.g., LUKS) and then continue boot. This would require changing to Ignition v3.
* Teach Ignition about Clevis. Ignition is Linux-aware, but distribution agnostic and runs only at first boot. Clevis and the required pins are very much specific to the Linux distribution. Given that Openshift is using Ignition v2 and Fedora Cores is using Ignition v3, it is not feasible yet.
* encrypt the data, but not the operating system. Linux has, for some time supported filesystem overlays for encryption via `ecryptfs.` LUKS-based block-level encryption is generally preferred over the file-based encryption of `ecryptfs.` The [performance of LUKS over ecryptfs is significant](https://www.phoronix.com/scan.php?page=article&item=ext4-crypto-418&num=1).

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
