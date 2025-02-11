---
title: etcd-supportability
authors:
  - "@dhellmann"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@dusk125, etcd team"
  - "@pmtk, MicroShift team"
  - "@vwalek, Support team"
approvers:
  - "@pacevedom"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2023-03-02
last-updated: 2023-03-02
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com//browse/ETCD-356
see-also:
  - "/enhancements/microshift/etcd-as-a-transient-systemd-unit.md"
  - "/enhancements/microshift/periodic-etcd-defragmentation-in-microshift.md"
  - "/enhancements/microshift/update-and-rollback.md"
replaces:
  - "https://github.com/openshift/enhancements/pull/1356"
---

# Supporting etcd as used in MicroShift

## Summary

This enhancement captures some decisions about how we will provide
support procedures and tools for etcd as it is configured and used in
MicroShift deployements.

## Motivation

We need to decide what approach we will take for support procedures
and tools for the etcd database used by MicroShift. We need to
consider how closely those procedures should be like the procedures
for OCP, and whether it makes sense to try to reuse the same tools. We
have generally taken the stance that we want to align, but not at the
expense of the user experience or the ability to meet the use cases
for which MicroShift is being developed.

### User Stories

* As a MicroShift administrator, I want to back up the database used
  by MicroShift so that I can restore it if something goes wrong with
  the host.
* As a MicroShift administrator, I want to restore the MicroShift
  database when something goes wrong to reset the host to a known good
  state.
* As a MicroShift administrator, I want to fix the certificates used
  to communicate with the database in case something goes wrong.

### Goals

* Describe the relationship between MicroShift and etcd and how it
  differs from OpenShift.
* Outline the support procedures for the user stories listed above.

### Non-Goals

* This document will not contain complete support procedures. More
  complete documentation will be written separately.
* MicroShift does not own the workloads running on it. Backing up and
  restoring the state of applications running on MicroShift is out of
  scope.

## Proposal

### Design Considerations

MicroShift is designed to provide a unified user experience. Many
components, including the kubernetes API server, kubelet, and
controllers, are compiled into a single binary. Users interact with
MicroShift, rather than the individual components.

MicroShift is not a “full stack” solution. It does not manage the OS
on which it runs, and is meant to integrate with, rather than subsume,
tools and processes for managing the host. “MicroShift is an
application running on RHEL.”

MicroShift is meant to run on edge devices, where manual operations
are not always possible. It should therefore be as resilient as
possible and take automated action when possible.

There is a practical benefit of designing the workflows, processes,
and tools for MicroShift so that an administrator’s experience with
OCP can translate to experience with MicroShift. While we do not want
MicroShift to diverge just for the sake of being different, there are
already enough differences in operation due to the implementation
details of MicroShift and the Device Edge product that some divergence
from OCP may be inevitable.

We do not want to force alignment at the expense of other design
principles. For example, after recent issues compiling etcd into the
same binary with the other aspects of MicroShift, it is now being
delivered as a separate binary. However, to retain the design
principle that the user should not have to orchestrate multiple
services to use MicroShift, the MicroShift process manages the etcd
process for the user. Starting and stopping MicroShift starts and
stops etcd transparently. This is similar to the experience of using
k3s or other single-binary distributions of kubernetes, even though
the details are different.

We need to keep the 4 footprints in mind (RAM, CPU, storage,
bandwidth). Tools should be small, and included in the OS image along
with MicroShift rather than delivered via container images at runtime.

We have an aggressive schedule for bringing MicroShift to GA
status. We therefore want to choose simple solutions, even if that
means we need to do additional work to improve tools in later
releases.

### Workflow Description

#### Database backup and restore

For backup and restore we need to consider both the user-driven backup
and the Greenboot backup scenarios.

For the short term, we will start with copying the raw database
files. This allows for a quick operation during Greenboot, including
using copy-on-write backups, as well as user-driven backups using
simple filesystem tools. For the longer term, we will wait for process
improvements for etcd in OCP and potentially bring those into
MicroShift later.

#### Database defragmentation

Because defragmentation is important for keeping the database size
below the quota, we want to emphasize the use of automatic
defragmentation rather than requiring administrators to trigger it
explicitly.

We will start with the implementation in
https://github.com/openshift/microshift/pull/1395/files#diff-a3d824da3c42420cd5cbb0a4a2c0e7b5bfddd819652788a0596d195dc6e31fa5R241-R247,
including the configuration parameters and their defaults:

* Min size threshold: 100 MB
* Max % fragmentation: 45%
* Check period: 5 min
* Startup defragmentation: bool, defaulting to true
* DB size quota: 2GB

#### Delivering etcdctl features

QE and support teams have asked for `etcdctl` for working with the
etcd database. `sos` also supports it.

We consider the features of `etcdctl` as nice-to-have for 4.13 and a
requirement for GA delivery of MicroShift. We will consider different
approaches for delivering it, either as a standalone tool or embedded
in the `microshift` binary, as part of the implementation work.

#### Certificate management

MicroShift regenerates certificates on startup. If something goes
wrong with the certificates, we will have users delete them and
restart the service to regenerate them.

### API Extensions

N/A

### Risks and Mitigations

Starting with basic file copy operations for backup and restore means
that the backups may be much larger than if we had etcd snapshots. We
consider this acceptable for the short term, and will actively
investigate other options for the future (possibly after GA).

Automatic defragmentation consumes resources. We consider the
self-healing benefits to outweigh the minimal overhead.

### Drawbacks

Relying on new processes means we need to write and test more
documentation.

## Design Details

### Open Questions

1. How do we deliver `etcdctl`.

### Test Plan

As support procedures are written we will work with QE to test them.

We will create a CI job to test backup and restore using Greenboot.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Deliver `etcdctl`
- End user documentation for basic procedures

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A (this section is about API extension support)

## Implementation History

- [Upgrade and rollback enhancement](https://github.com/openshift/enhancements/pull/1312)
- Obsolete [enhancement describing data directory changes](https://github.com/openshift/enhancements/pull/1356)
- [Defragmentation enhancement](https://github.com/openshift/enhancements/pull/1350)

## Alternatives

We considered using snapshots for backups immediately, but decided
that because we need something that works when the database is offline
for the Greenboot scenarios we would start there and add more features
in the future.

We considered organizing the files used by `microshift-etcd` in the
same way as etcd in OpenShift, but decided that we prefer to view
MicroShift as "a unit" and keep all of its data files, certificates,
etc. in one location. This is consistent with having MicroShift manage
the etcd process lifecycle.

## Infrastructure Needed [optional]

None
