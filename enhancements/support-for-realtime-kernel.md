---
title: support-for-real-time-kernels
authors:
  - "@miabbott"
reviewers:
  - "@ashcrow"
  - "@darkmuggle"
  - "@ericavonb"
  - "@mike-nguyen"
  - "@sinnykumari"
approvers:
  - "@ashcrow"
  - "@cgwalters"
  - "@crawford"
  - "@imcleod"
  - "@runcom"
creation-date: 2019-12-19
last-updated: 2020-01-27
status: provisional
see-also:
replaces:
superseded-by:
---

# Support For Real-time Kernels

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Many workloads associated with telcos and other verticals like the financial
service industry (FSI) require a high degree of determinism.  While Linux is
not a real-time operating system, the real-time kernel provides a preemptive
scheduler providing the OS with real-time characteristics. We want to provide
customers the ability to select the real-time kernel for Red Hat Enterprise
Linux CoreOS (RHCOS) nodes in the cluster.

## Motivation

The real-time kernel, along with realtime-friendly hardware and BIOS settings,
is a requirement to meet the low latency needs that network equipment providers
(NEP), are looking to deploy as part of the 5G roll-out.

Unfortunately, the additional overhead from the scheduler makes it a poor fit
for the majority of “general purpose” workloads, therefore, real-time is not
suited to be the default kernel in RHCOS. OpenShift Container Platform (OCP)
clusters and MachineConfigPools will need to be configurable to run either kernel.

### Goals

- Provide the `kernel-rt` packages in the `machine-os-content` image
- Provide the ability to select the real-time kernel for RHCOS nodes in the cluster
- Provide initial tuning via `tuned` profiles of the RHCOS nodes for real-time workloads (if required)

### Non-Goals

- Defaulting to the real-time kernel or offering multiple boot images
- Additional tuning of the RHCOS nodes after the real-time kernel is selected (handled by [Cluster Node Tuning Operator](https://github.com/openshift/cluster-node-tuning-operator))
- Providing real-time kernel support to older versions (pre-4.4) of OCP
- Supporting real-time kernel selection on non-RHCOS nodes

## Proposal

1. Include the `kernel-rt` packages in the `machine-os-content` image
2. Provide tunable in MachineConfig that selects the type of kernel to use
3. Do initial tuning of RHCOS node via `tuned` profiles after real-time kernel is applied

### User Stories

#### Story 1 - Fresh Install (Day 1)

Weyland-Yutani Corp. wants to deploy OCP at their site for processing satellite signals
from potential alien spacecraft.  They want to avoid as much down time as possible when
deploying their cluster with real-time kernel support.  They generate install manifests
via the OpenShift intsaller and include a MachineConfig with `kernelType: realtime` for
their RHCOS nodes.

#### Story 2 - Fresh Install (Day 2)

Cyberdyne Systems wants to deploy OCP at their radio tower to handle the processing
of radio signals.  Processing the signals requires the determinism and guarantees
that come with a real-time kernel.  They deploy OCP succesfully and then create a
MachineConfig to select the real-time kernel on their RHCOS nodes.

#### Story 3 - Upgrade Path

Nakatomi Corp. has an older OCP cluster deployed using all RHCOS nodes.  They plan on
introducing a workload that would benefit from the use of the real-time kernel.
They upgrade their OCP cluster to the latest version with real-time kernel support
and create a MachineConfig that selects the real-time kernel on their RHCOS nodes.

#### Story 4 - Node Replacement

Tyrell Corp. has an existing OCP cluster deployed with RHEL 7 worker nodes using
the real-time kernel.  They want to upgrade to the latest version of OCP and switch
their worker nodes to RHCOS nodes.  They remove the RHEL 7 worker nodes from the
cluster, upgrade the cluster, add in RHCOS nodes, and create a MachineConfig to
select the real-time kernel on their RHCOS nodes.

#### Story 5 - Unsupported Version

Corellian Engineering Corp. has an existing OCP cluster that does not support
real-time kernels.  They learn of support for real-time kernels in the new version
of OCP and try to create a MachineConfig that selects the real-time kernel.  The
MachineConfig is ignored.

### Implementation Details/Notes/Constraints

This proposal only covers providing the real-time kernel to RHCOS nodes and
selecting it via MachineConfig.  It **DOES NOT** cover any changes that may be
required by the container runtime or other components of OCP.

### Risks and Mitigations

#### Performance Impact

Selecting the real-time kernel for RHCOS nodes without properly configuring the
underlying hardware **MAY** incur an undesirable performance impact.  It is
recommended that only users who control their hardware (i.e. not cloud users)
attempt to use the real-time kernel.  If users enable real-time kernel support
without properly configured hardware, they can always remove the offending
MachineConfig to return to the previous state.

#### Lack of Full Support

If there are additional changes required across the OCP product to properly support
real-time kernel (i.e. container runtime, etc) which cannot be delivered at the
same time, the real-time kernel packages can still be included as part of the
`machine-os-content` image.  Additionally, exposing the tunable in the MachineConfig
spec can be removed.  Once support has been fully committed across the product,
we can expose the tunable again.

## Design Details

### Providing the Packages

The proposal is to include the `kernel-rt` packages in the standard `machine-os-content`
image.  They **WILL NOT** be included as part of the ostree commit, but rather
as separate files within the image.  Example:

```console
$ sudo podman pull quay.io/openshift-release-dev/ocp-v4.0-art-dev:realtime
Trying to pull quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:realtime...
Getting image source signatures
Copying blob 3c141bccb4e4 skipped: already exists
Copying config 485b845970 done
Writing manifest to image destination
Storing signatures
485b845970f19098b662f85a9e22bf4db7aabea83fe5d9c155d798dffcd6f2e9

$ ctr=$(sudo podman create quay.io/openshift-release-dev/ocp-v4.0-art-dev:realtime)

$ mnt=$(sudo podman mount $ctr)

$ sudo ls -l $mnt
total 50036
-rw-r--r--. 1 miabbott miabbott 26061020 Dec 16 15:58 kernel-rt-core-4.18.0-147.0.3.rt24.95.el8_1.x86_64.rpm
-rw-r--r--. 1 miabbott miabbott 22869480 Dec 16 15:58 kernel-rt-modules-4.18.0-147.0.3.rt24.95.el8_1.x86_64.rpm
-rw-r--r--. 1 miabbott miabbott  2285240 Dec 16 15:58 kernel-rt-modules-extra-4.18.0-147.0.3.rt24.95.el8_1.x86_64.rpm
-rw-rw-r--. 1 root     root        13764 Dec 16 15:59 pkglist.txt
drwxrwxr-x. 3 root     root           18 Dec 16 15:58 srv

```

### Selecting the Kernel

The proposal is to have the MachineConfig spec changed to support a field named
`kernelType` which will determine the which kernel should be used on the RHCOS
nodes. If `kernelType: realtime` is configured, the `kernel-rt` packages will
be used.  If `kernelType: default` is configured, the default kernel will be used.
In the absence of any `kernelType` field, the default choice is for the default
kernel to be used.

Example MachineConfig:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: "worker"
  name: 99-worker-realtime-kernel
spec:
  kernelType: "realtime"
```

### Installing the Real-time Kernel

When the MCO parses a MachineConfig with `kernelType: realtime`, it shall instruct
`rpm-ostree` on the RHCOS node to override the installed kernel with the `kernel-rt`
package from the `machine-os-content` image.

### Removing the Real-time Kernel

Users wishing to revert back to the default `kernel` packages will need to delete
the MachineConfig which contains `kernelType: realtime`.

### Upgrades

When the OCP cluster is upgraded, the RHCOS nodes that are using the real-time
kernel will also update the `kernel-rt` packages if updates are available in
the `machine-os-content` image.  This treats the `kernel-rt` package like any
other RPM installed on the RHCOS nodes.

### Test Plan

- Test install of `kernel-rt` packages on single RHCOS node
- Test removal of `kernel-rt` packages on single RHCOS node
- Test upgrade of `kernel-rt` packages on single RHCOS node
- Test `kernelType: realtime` on OCP cluster with RHCOS nodes using default kernel
- Test `kernelType: default` on OCP cluster with RHCOS nodes using default kernel
- Test supplying an unsupported value for `kernelType` on an OCP cluster with RHCOS nodes.
- Test deleting MachineConfig that contains `kernelType: realtime` already deployed to RHCOS nodes
- Test `kernelType: realtime` on OCP cluster with RHEL nodes
- Test `kernelType: default` on OCP cluster with RHEL nodes
- Test upgrade of OCP cluster with RHCOS nodes using real-time kernel (no `kernel-rt` updates in `machine-os-content`)
- Test upgrade of OCP cluster with RHCOS nodes using real-time kernel (`kernel-rt` updates available in `machine-os-content`)
- Test `kernelType: realtime` on older OCP cluster with no real-time kernel support

### Graduation Criteria

For this feature to be considered stable:
- `kernel-rt` packages available in `machine-os-content` images
- MachineConfig spec has `kernelType` field
- Cluster with RHCOS nodes can successfully select + use the `kernel-rt` packages
- Cluster with RHCOS nodes using `kernel-rt` can be upgraded successfully

### Upgrade / Downgrade Strategy

- Nodes using `kernel-rt` packages should continue to use `kernel-rt` packages through upgrades
- Nodes using `kernel-rt` packages should continue to use `kernel-rt` packages after a downgrade.
  - **WARNING**: downgrading a cluster to a previous version that does not have real-time kernel support is unsupported.

### Version Skew Strategy

There should be no implications for real-time kernel support if other components in the cluster are at a different version.

## Implementation History

- 2019-12-11: RT kernel included in `machine-os-content` image - https://url.corp.redhat.com/rhcos-rt-kernel
- 2019-12-12: PR for MCO support - https://github.com/openshift/machine-config-operator/pull/1330

## Drawbacks

- If there are additional changes required to cluster components for full
support of the real-time kernel, this proposal has not been scoped for that
and should be pushed to a later release.

- It should be noted that this increases the support matrix for the
OpenShift support folks.  The amount of additional configurations that
this could create (i.e. disk encryption on/off, different platforms,  etc.)
is something that needs to be considered when reviewing the proposal.

- Customers requiring use of custom kernel modules (or [SRO](https://github.com/zvonkok/special-resource-operator))
for things like nVidia GPUs will need to rebuild the kernel modules
to support the real-time kernel.

- Customers requiring FIPS support **SHOULD NOT** use the real-time kernel.
This configuration is not tested or supported.  The MCO will reject any
MachineConfigs that contain `kernelType=realtime` if `fips=true` is already
being used on the RHCOS nodes; an error message will be logged in this case.

## Alternatives

The alternative model for delivering the `kernel-rt` packages would be to
create an additional ostree commit in the `machine-os-content` image which
the RHCOS nodes would rebase to.

Currently there is a single ostree repo with a single ostree commit in the
`machine-os-content` image.  It is possible to create a second commit with
the `kernel-rt` packages which would be used instead of the default kernel.
While this is a cleaner, more pure ostree implementation of delivering a
different set of packages, it was determined that the work required in the
RHCOS build pipeline would be non-trivial.

The intent is to pursue the current proposal and perhaps revisit this
alternative as we gain experience handling different package sets for RHCOS
nodes in the cluster.

See https://hackmd.io/VJRcGjeTSk6k0RCHp-bteQ as reference
