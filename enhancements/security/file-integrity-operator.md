---
title: OpenShift node file integrity monitoring
authors:
  - "@mrogers950"
reviewers:
  - "@cgwalters"
  - "@ashcrow"
  - "@jhrozek"
approvers:
  - "@JAORMX"
creation-date: 2019-10-21
last-updated: 2019-11-04
status: provisional
---

# Cluster Node File Integrity Operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

## Summary

This enhancement describes a new security feature for OpenShift. Many security-conscious customers want to be informed when files on a host's filesystem are modified in a way that is unexpected, as this may indicate an attack or compromise.  It proposes a "file-integrity-operator" that provides file integrity monitoring of select files on the host filesystems of the cluster nodes. It periodically runs a verification check on the watched files and provides logs of any changes.

## Motivation

In addition to the reasons stated in the Summary section, as part of the FedRAMP gap assessment of OpenShift/RHCOS, it has been identified that to fulfill several NIST SP800-53 security controls we need to constantly do integrity checks on configuration files (CM-3 & CM-6), as well as critical system paths and binaries (boot configuration, drivers, firmware, libraries) (SI-7). Besides verifying the files, we need to be able to report which files changed and in what manner, in order for the organization to better determine if the change has been authorized or not. In order to fulfull the controls the file integrity checks need to be done using a state-of-the-practice integrity checking mechanism (e.g., parity checks, cyclical redundancy checks, cryptographic hashes). If using cryptographic hashes for integrity checks, such algorithms need to be FIPS-approved.

## Goals

Provide a way for security-conscious customers to be alerted when changes are made to files on the host operating system in a way that is satisfactory for FedRAMP compliance.

## Proposal

The proposed design and current [Proof-of-concept operator](https://github.com/mrogers950/file-integrity-operator) is as follows:
* Deploying node monitoring pods - The file-integrity-operator deploys daemonSets that run a privileged [AIDE](https://aide.github.io/) pod on each master and worker. This AIDE pod does a hostmount of / to the /hostroot directory in the pod. The privileged access for the AIDE pod is needed for the hostmount, so an SELinux policy is applied that will restrict write access to files other than the AIDE database and log.
  * A worker node may be RHCOS or RHEL (UPI). This means there will potentially be different default AIDE configurations. When deploying daemonSets for the workers, the operator will try to determine the OS type and deploy with the appropriate config.
* Scan database initialization - AIDE works off of a database of file checksums. This database must be initialzed during the first run of the AIDE pods, and at times may need to be re-initialized if the AIDE configuration changes.
* Running scans - The AIDE process runs in a loop in the pod, periodically running integrity checks (customizable with Spec.ScanInterval).
* Log scan results - The AIDE process can write scan results to syslog, files, and standard output. How we handle the logs depends on the approach we want to take:
  * Approach A: Log to syslog only and leave collection up to the clusterlogging components. This is likely the preferred method.
  * Approach B: Log to files on the host, and expose them to the user through configMaps.
* AIDE Configuration - A user-provided AIDE configuration can be provided in order to allow customers to modify the integrity check policy.
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
* "Provider" types - Including a provider field in the API definition leaves us room to later define a different integrity method. Suggested future methods are fs-verity and IMA/EVM. Initially, this always defaults to "aide".

### API Specification

```
apiVersion: file-integrity.openshift.io/v1alpha1
kind: FileIntegrity
metadata:
  name: example-fileintegrity
spec:
  provider: aide
  scanInterval: 5m
  config:
    name:
    namespace:
    key
```

### Test Plan

* Basic functionality
  1. Install file-integrity-operator.
  2. Ensure operator roll-out, check for running daemonSet pods.
  3. Modify a file on the host and verify detection of the change.
* Configuration
  * Unit test aide.conf conversion functions.
  * Verify the aide.conf can be changed and propagated to the daemonSet pods.
* Cluster upgrade testing

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

### Upgrade / Downgrade Strategy

The operator will handle configuration and image versioning for its operand. AIDE and its configuration are mature and not expected to have breaking changes between releases. Because of this stability it is likely that the container image we use for AIDE does not need to always upgrade between OpenShift 4 releases.

### Version Skew Strategy

The operator is intended to be the sole controller of its operand resources (configmaps, daemonSets, AIDE container image versions), so there should not be version skew issues.

## Implementation History

* Initial POC at https://github.com/mrogers950/file-integrity-operator

## Drawbacks

* After a cluster upgrade new versions of the node OS will result in false positives as packages are updated.
  * One possible solution is to save the pre-upgrade database and logs, and re-initialize the AIDE database after upgrade.
* AIDE runs periodically, the longer the interval the higher the chances of missing a file change from an actual attack.
