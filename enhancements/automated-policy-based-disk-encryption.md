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

Starting with OpenShift 4.3, Red Hat Enterprise Linux RHCOS (RHCOS) boot images will be encryption-ready. On first boot either administator provided or automated policy will be used for the root disk encryption. Bootstrapping and node provisioning **MUST** fail when the policy cannot be applied unless the administrator has opted out.

## Motivation

The security of data-at-rest is of chief concern for end-users, organizations and enterprise Information Technology management. Every year, familiar companies have severe data breaches. Some industries require the encryption of data-at-rest, while customers of those industries demand it. Encryption of data is a best practice.

### Goals

This enhancement is to provide policy based application of enterprise-grade encryption to the root filesystem.

When an applicable policy is found:
* The root filesystem will be encrypted using standard AES-256 encryption at the OS-Level on first boot. Only FIPS 140-2 compliant ciphers, hashes and checksum algorithms will be used.
* OS-Level hooks will be included to support this feature
* Automated boot/reboot handled by Clevis
* User intervention MUST not be required to initialize or (re)boot a cluster.
* Users will have the ability to opt-out default policy-based encryption or apply their own.
* Failure to apply policies will prevent node bootstrap.

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

To provide policy-based encryption, RHCOS boot images will:
* Be encryption ready by having root filesystem housed within a bare LUKS container.
* On first-boot, the root file system will be encrypted.
* Automated unlocking of the root file sysystem will be policy based.
* Unless the user opts-out, best-practice policies will be enforced for the encryption of the root filesystem.

On first boot, a Dracut module will encrypt the root-filesystem using a Clevis policy. Should the policy binding operation fail, the node bootstrap will fail also. 

The encryption step will take between 1-3 minutes, depending on the CPU and disk I/O. Afer first-boot, a Linux `systemd` service will provide for updating the policy. If an administrator wishes to disable encryption, they will have to reprovision the node.

Clevis handles the automated unlocking of the root filesystem.

### User Stories

#### Story 1: Requirements

ACME Corp is only able to consider technologies that have encrypted disks, but they want to use Google Cloud. With OpenShift's 4.3 boot images, they can evaluate OpenShift.

#### Story 2: Upgrades

ACME Corp has a 4.1 cluster and is upgrading to 4.3 and wants to use the new encryption. The IT department simply:
* Adds their Clevis machine config set
* [updates the machine set](https://docs.openshift.com/container-platform/4.1/machine_management/creating-machineset.html)
* Scales up new nodes
* Scales down the old nodes

#### Story 3: Edge Clusters

ACME Corp deploys sensitive bare-metal clusters to the edge. They build their clusters with a Clevis profile using TPM2 and Tang. Evil Corp clandestinely steals a few of ACME Corp's nodes. Since ACME Corp's nodes are configured using NBDE encryption, no-data is compromised. ACME Corp avoids being shamed on the evening news and withstands the ire of the Board of Directors.

### Implementation Details/Notes/Constraints

This proposal introduces dependencies on RHCOS and the the Installer only. The vast majority of the work will be done through operating system level hooks. OpenShift itself will entirely unaware that RHCOS is encrypted.
* A new Dracut module will be added. Upstream `cryptsetup` has a in-tree [Dracut Module for disk-reencryption](https://gitlab.com/cryptsetup/cryptsetup/tree/master/misc/dracut_90reencrypt).  The module will need to be extended to support Cleivs configurations.
* RHCOS will need to add Clevis and it's dependencies. Clevis provides TPM2 and Tang support upon installation and provides the backbone for extending to additional key-stores.
* Extend the Cloud CryptAgent to act as a Clevis Pin. This not a requirement for release. 
* A new Linux systemd unit for handling application of new policies. 

## Design Details

### On Disk Changes

The current partitioning for a RHCOS images is:
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

RHCOS today uses `/dev/disk/by-label` for mounting the root-filesystem. Once the LUKS container `crypt-root` is opened, `/dev/disk/by-label/rootfs` will appear.


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
# head -c 256 > new_pass
# echo coreos |
  cryptsetup-reencrypt \
     --cipher="aes-cbc-essiv:sha256" \
     --key-size=256 \
     --progress-frequency=5 \
     --key-file=new_pass
     /dev/nbd0p4
```

In the above step, a new LUKS master key created, and each block is encrypted. After the encryption, an appropriate Clevis configuration will be applied and the `new_key` will be removed; future encryption operations will use existing Clevis configruation. The use of an ephemeral passphrase is needed for initial encryption and LUKS binding of the Clevis policy.

### Policies

The default policy will be selected based on the installation target.
* AWS: AMIs are encrypted.  OS-level encryption not used.
* Alibaba: TBD
* Azure: TBD
* Bare Metal: TPM2 required.
* GCP: vTPM
* IBM Cloud: TBD

Users will be able to define their own policy, or to opt out entirely. 

### Installer Support

Basic Installer support is required. The Installer will select the default policy, or use a custom policy if defined in `install-config.yaml`. Policies will added as an Ignition file-payload using Clevis' JSON format. 

Encryption policy will be defined in `install-config.yaml` under a new `os_encryption` section. For example:
```
fips: <true|false>
os_encryption:
   disable: <true|default=false>
   enforce: <default=true|false> 
   tpm2: <true|false>
   tang:
     - <URL>:<THUMBPRINT>
     - <URL>:<THUMBPRINT>
   user: <base64 Encoded clevis.json>
```

The Installer is not required to support all the possible configruations. The default configruation would be rendered as:
```
os_encryption:
   disable: false
   enforce: true
```
In this state, the Installer will user deliver a default policy based on the installation target. Setting `enforce: false` will allow for policy binding to fail and the nodes may be encrypted. If `disable: true` encryption will not be attempted at all.

**NOTE**: there is no `disks` section today. The new top-level directive will be used in the future for the definition of disk-partitioning. 

The following are examples of policies:
* Tang: `{"url": "http://...", "thp": "<THUMBPRINT>"`}`
* TPM2: `{}` Note: this is correct

If both `tpm2: true` and a Tang stanza is found, rendered:
```
{"t": 2,
   "pins": {
    "tpm2": {},
    "tang": [
        {"url": "<URL>",
         "thp": "<THUMBPRINT>"} ]
   }
}
```

The Installer is neither required to, nor expected to, validate a user-provided configuration.

KMS pins will be provider specific. At this time, more research is required. It is anticipated that KMS pins be delivered using the `user` payload option.

Example user provided payload:
```
disk:
   encryption:
      policy: user
      user: U2VyaW91c2x5IHRoaXMgaXMgYSBkdW1iIGV4YW1wbGUK
```

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

This design does not require any changes to the Installer. Once the CoreOS installer is done, the on-disk CoreOS will be booted and re-encrypted normally.

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
