---
title: cluster-node-file-integrity-operator
authors:
  - "@mrogers950"
reviewers:
  - "@cgwalters"
  - "@ashcrow"
  - "@jhrozek"
approvers:
  - "@JAORMX"
creation-date: 2019-10-21
last-updated: 2020-02-07
status: provisional
---

# Cluster Node File Integrity Operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

This enhancement describes an optional security and compliance feature for OpenShift. Many security-conscious customers want to be informed when files on a host's filesystem are modified in a way that is unexpected, as this may indicate an attack or compromise.  It proposes a "file-integrity-operator" that provides file integrity monitoring of select files on the host filesystems of the cluster nodes. It periodically runs a verification check on the watched files and provides logs of any changes.

## Motivation

In addition to the reasons stated in the Summary section, as part of the FedRAMP gap assessment of OpenShift/RHCOS, it has been identified that to fulfill several NIST SP800-53 security controls we need to constantly do integrity checks on configuration files (CM-3 & CM-6), as well as critical system paths and binaries (boot configuration, drivers, firmware, libraries) (SI-7). Besides verifying the files, we need to be able to report which files changed and in what manner, in order for the organization to better determine if the change has been authorized or not. In order to fulfull the controls the file integrity checks need to be done using a state-of-the-practice integrity checking mechanism (e.g., parity checks, cyclical redundancy checks, cryptographic hashes). If using cryptographic hashes for integrity checks, such algorithms need to be FIPS-approved.

## Goals

Provide a way for security-conscious customers to be alerted when changes are made to files on the host operating system in a way that is satisfactory for FedRAMP and FIPS compliance.

## Proposal

The proposed design and current [Proof-of-concept operator](https://github.com/openshift/file-integrity-operator) is as follows:
* Deploying node monitoring pods - The file-integrity-operator deploys daemonSets that run a privileged [AIDE](https://aide.github.io/) pod on each master and worker. This AIDE pod does a hostmount of / to the /hostroot directory in the pod. The privileged access for the AIDE pod is needed for the hostmount.
* Scan database initialization - AIDE works off of a database of file checksums. Initialization of the database involves AIDE computing and storing the hashes for any file that falls under the policy defined in the AIDE configuration. An init container runs the AIDE initialization command as needed.
* Running scans - With the AIDE database created, the AIDE process runs in a loop in the pod, periodically running integrity checks and writing the results to a log file.
* Viewing scan results - The AIDE log files are exposed to the admin via configMap.
  * A "logcollector" container is included in the DaemonSet pods. This process watches for the status of the AIDE checks, placing the AIDE log into a temporary configMap. The logs are compressed if they run over the 1MB limit. (See Drawbacks section for more regarding this limit).
  * The file-integrity-operator reconciles on the new configMap and does some validation of the data (checks for a logcollector error vs. a scan result), and adds a condition entry to status.nodeStatus. If the AIDE check failed, a new configMap is created with the log, and the "Failed" condition includes the name and namespace of the configMap. The temporary configMap created by the logcollector process is deleted.
* AIDE Configuration - Optionally, a user-provided AIDE configuration can be provided in order to allow customers to modify the integrity check policy.
    * The admin first creates a configMap containing their aide.conf.
    * A FileIntegrity CR is posted that defines the Spec.Config items.
        * Spec.Config.Name: The name of a configMap that contains the admin-provided aide.conf
        * Spec.Config.Namespace: The namespace of the configMap
        * Spec.Config.Key: The data key in the configMap that holds the aide.conf
    * In most cases the provided aide.conf will be specific to standalone host and not tailored for the operator. On reconcile the operator reads the configuration from the configMap and applies a few conversions allowing it to work with the pod configuration.
      * Prefix /hostroot/ to each file selection line.
      * Change database and log path parameters.
      * Change allowed checksum types to FIPS-approved variants.
    * After conversion the operator's AIDE configuration configMap is updated with the new config, and the daemonSet's pods are restarted.

### API Specification

```
apiVersion: file-integrity.openshift.io/v1alpha1
kind: FileIntegrity
metadata:
  name: example-fileintegrity
spec:
  config:
    name: user-conf
    namespace: openshift-file-integrity
    key: conf
status:
  nodeStatus:
  - condition: Succeeded
    lastProbeTime: "2020-02-05T21:08:38Z"
    nodeName: ip-10-0-166-163.ec2.internal
  - condition: Succeeded
    lastProbeTime: "2020-02-05T21:08:39Z"
    nodeName: ip-10-0-143-91.ec2.internal
  - condition: Failed
    lastProbeTime: "2020-02-06T16:53:08Z"
    nodeName: ip-10-0-143-91.ec2.internal
    resultConfigMapName: aide-ds-ip-10-0-143-91.ec2.internal-failed
    resultConfigMapNamespace: openshift-file-integrity
...
```

### Test Plan

* Basic functionality
  1. Install file-integrity-operator.
  2. Ensure operator roll-out, check for running daemonSet pods.
  3. Modify a file on the host and verify detection of the change.
* Configuration
  1. Unit test aide.conf conversion functions.
  2. Verify the aide.conf can be changed and propagated to the daemonSet pods.
* Result reporting
  1. Modify a file on the host, verify that the scan reports a failure.
  2. Verify that upon a scan failure, a status entry is created for the node containing a reference to the log configMap.
  3. Verify the configMap contains an AIDE log with an entry for the modified file.
* Cluster upgrade testing

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

### Upgrade / Downgrade Strategy

The operator will handle configuration and image versioning for its operand. AIDE and its configuration are mature and not expected to have breaking changes between releases. Because of this stability it is likely that the container image we use for AIDE does not need to always upgrade between OpenShift 4 releases.

### Version Skew Strategy

The operator is intended to be the sole controller of its operand resources (configmaps, daemonSets, AIDE container image versions), so there should not be version skew issues.

## Implementation History

* Current implementation: https://github.com/openshift/file-integrity-operator

## Roadmap

* Deploy daemonSets and configurations based on labels, in order to target specific MachineSets. Customers may have UPI RHEL workers on which they manage AIDE out-of-band, and only wish to target the RHCOS masters. MachineSets may require differing AIDE configurations.
* Further secure AIDE process containers with SELinux to only allow writing to the DB and log files.
* Enable the use of object storage for exposing the AIDE logs instead of configMaps. See the Drawbacks section for more regarding AIDE log storage.
* Add a "Provider" field to the API and enable other integrity checking mechanisms.
  * Enable non-userspace and strongly attested integrity checking mechanism(s). Some suggested candidates include fs-verity and IMA. Note: Both IMA/fs-verity methods provide more integrity assurance over AIDE _only_ when backed by a TPM.
    * fs-verity: According to https://www.kernel.org/doc/html/latest/filesystems/fsverity.html, fs-verity works only on read-only files. Requires ext4 formatting options and block size to be == PAGE_SIZE. Files need to be ioctl'ed as verity files and have signature verification performed in userspace. Unless fs-verity is backed by attested dm-verity volumes (not feasible for nodes) or tied into IMA somehow, this provider seems insufficient for our use.
    * IMA-measurement; requires setup of kernel params, filesystem options, TPM keys, and possibly require some userspace tools for setup. The file-integrity-operator would handle this setup by creating MachineConfigs, etc. As a bonus, IMA can log file hashes through auditd. At a glance, this would be a preferred method over fs-verity.
  * Note: Enabling stronger integrity checking mechanisms does not obsolete the use of the file-integrity-operator. The FedRAMP Moderate baseline is still satisfied by the weak assurance of AIDE;  It is important that customers can choose to meet this baseline without requiring a TPM, so the AIDE provider will not go away.

## Drawbacks

* The AIDE provider does not give strong assurance of file integrity, at best, the scan logs can assist in a post-mortem investigation.
* AIDE runs periodically, the longer the interval the higher the chances of missing a file change from an actual attack. We plan to balance the scan and log collection intervals.
* AIDE scanning can be expensive performance-wise.
* configMap data limit and log storage: Even though we compress the AIDE logs if they reach this limit, in theory, a large amount of files can show as new (or changed) in the AIDE log and push the compressed data over this limit.
* After a cluster upgrade new versions of the node OS could result in false positives as packages are updated. For now, we plan on re-initializing the AIDE database after upgrade.
