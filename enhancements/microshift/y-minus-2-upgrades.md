---
title: y-minus-2-upgrades
authors:
  - dhellmann
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "DanielFroehlich, PM"
  - "pmtk, upgrades expert"
  - "jogeo, QE lead"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - jerpeter1
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2024-02-08
last-updated: 2024-02-08
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/USHIFT-2246
see-also:
  - "/enhancements/microshift/microshift-updateability-ostree.md"
  - "/enhancements/update/eus-upgrades-mvp.md"
replaces: []
superseded-by: []
---

# Upgrading from 4.Y-2 to 4.Y

## Summary

This enhancement describes how MicroShift will support upgrading
in-place across 2 minor versions at a time.

## Motivation

We are already seeing a tendency for MicroShift users to adopt EUS
versions and stay on them until they can update to the next EUS
release. This makes sense given the deployment scenarios for
MicroShift, which often involve remote locations, limited bandwidth,
or other reasons that make the appetite for frequent updates as low
as, or lower than, it is for OpenShift users.

### User Stories

As an edge device administrator, I want to deploy versions of the
platform software (OS, MicroShift, etc.) with the longest support
life-cycle so I can focus on my own applications and _using_ the
device.

As an edge device administrator, I want to upgrade from one
long-life-cycle version of the platform software directly to another,
without applying the intermediate version.

### Goals

* Support updating single-node deployments of MicroShift in place on
  RPM-based and ostree-based systems from version 4.Y-2 to 4.Y.

### Non-Goals

* Multi-node support for MicroShift has been discussed, but is out of
  scope for this enhancement.
* Upgrading and skipping versions always requires a full host reboot
  to ensure all components are restarted and we have no plans to
  remove that requirement.

## Proposal

Versions 4.12 and 4.13 of MicroShift were preview releases. We did not
intend to support upgrading to 4.14 from either earlier version at
all, but did implement upgrade testing as part of preparing 4.14 for
release. We wanted to limit that testing to 1 version. Therefore, in
4.14 we introduced an explicit version check to determine if the data
version (the contents of `/var/lib/microshift` are more than 1 minor
version older than the software version (the version embedded in the
new binary). If the skew is too great, MicroShift exits with an error.

To implement this enhancement, we will change the check to support a
skew of 2 versions.

We expect this to require minimal work in MicroShift because

* The storage migration controller is already running and can be used
  to update storage versions of any resources.
* There are not currently any changes to the etcd storage format.
* The version skew check in MicroShift itself is straightforward to
  change.

### Workflow Description

1. Edge device administrator deploys a host with MicroShift 4.Y-2
   installed.
2. Software runs, time passes.
3. Edge device administrator updates the host to run MicroShift 4.Y.
  * For ostree-based systems, the host is automatically rebooted as
    part of the update process.
  * For RPM-based systems, the user must reboot the host after the
    software update is completed.
4. Edge device restarts.
5. MicroShift restarts.
6. MicroShift checks the data and binary version difference for
   compatibility.
7. If the check fails, MicroShift exits with an error.
8. If the check passes, MicroShift continues to run, including
   performing any data migration necessary.

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

This enhancement only applies to MicroShift.

### Implementation Details/Notes/Constraints

The details are covered in the rest of this document.

### Risks and Mitigations

There is some risk in supporting multiple version upgrades in a way
OCP does not. OCP upgrades from one EUS version to another while
ensuring minimal disruption by upgrading the control plane to the Y+1
version, then to the Y+2 version, then updating the worker nodes. SNO
OCP does not claim support for upgrades without disruption, so it does
not make a distinction between types of versions and requires stepping
through one release at a time. MicroShift upgrades are significantly
simpler because they are all single-node (so disruption is expected)
and there are no operators for managing the host or cluster
configuration that need to move the system through multiple states as
part of the upgrade.

There is a risk that some underlying data format for etcd will change
between MicroShift versions (kubernetes storage versions, etcd file
format, etc.). If that happens, someone will have to build a tool to
support migrating from 4.Y-2 to 4.Y-1 _anyway_. MicroShift will need
to carry over the use of that tool for an extra release to support the
2 version upgrade capability.

If we extend the supported upgrade skew, we would have to continue to
carry the migration tool for the full length of the allowed upgrade
window after 4.Y-1 (if the allowed skew is 5, we would carry the tool
in 4.Y-1, 4.Y, 4.Y+1, 4.Y+2, and 4.Y+3 to support upgrading 4.Y-1 to
4.Y+3 at one time).

The [kubernetes version skew
policy](https://kubernetes.io/releases/version-skew-policy/) is
written assuming multi-node clusters. Even so, it supports 3
kubernetes version difference between the API server and kubelet and 1
version between the API server instances. This is what allows
OpenShift's EUS upgrade process, in which the control plane is updated
independently of the worker nodes, to work. While the version skew
restriction is enforced, this is done to prevent old nodes from
joining a cluster and there is no code in kubernetes that explicitly
links skew to upgrades in order to prevent updating multiple versions
at a time.  In a single-node MicroShift deployment, the API server and
kubelet are in the same binary and have the same version, so there is
no skew at all.

If, in the future, MicroShift does need to support multi-node
deployments there will be many other aspects of deployment and upgrade
to consider, in addition to the version skew problem. We can envision
implementing a process similar to what OpenShift uses, where the
control plane and workers are updated using separate steps. This would
make the single-node configuration of MicroShift and the multi-node
configuration mirror the trade-offs of being able to upgrade the
entire cluster at one time or offering no downtime that are present in
SNO and HA OCP.

If an upgrade fails, even after a complex data migration, MicroShift's
rollback process is to discard the new database and restore the old
version from a backup before continuing. This ensures that an old
version of the software matches the older database (file format,
schema, and content).

MicroShift does not automatically create `StorageVersionMigration` CRs
to trigger data migration. The core kubernetes APIs are safe because
upstream has committed to not drop any storage versions. CRDs
installed on top could be more of an issue, but they are installed by
the end user so it's up to them to track the need for updates.

### Drawbacks

The main drawback to implementing this enhancement is the increased
test matrix for upgrades. We can automate those tests to minimize the
impact.

## Test Plan

We will add an automated test to CI to deploy 4.Y-2 and update to 4.Y
using the latest published packages of 4.Y-2 and testing the "source"
version (HEAD of the branch or the pull request content) of 4.Y. This
ensures that every package we build can be continuously upgraded to
the latest version of the source.

MicroShift's OS support policy is to allow combining each version of
MicroShift with 1 EUS version of RHEL and the next non-EUS version of
RHEL. We test upgrades from 4.Y-1 to 4.Y with the same underlying OS
and also moving from the EUS version to non-EUS version. The aspects
of testing the OS support during upgrades are orthogonal to the work
for this enhancement, however, and should not require additional
expansion of the test matrix, either in CI or by QE.

The QE team will need to perform similar tests using the 4.Y-2 and 4.Y
packages built by the release team. Their test plan includes:

1. The scope of QE testing will be confined to upgrades of RHEL and
   rpm-ostree hosts deployed with RPMs built by ART (nightly, EC, RC
   and after release z-streams).
1. Both x86_64 and aarch64 architecture will be covered.
   1. Not every combination listed below will be tested on both
      architectures.
   1. A mix of Intel and ARM hosts will be used
1. Rpm-ostree based hosts
   1. Initial cluster bring-up will be a mix of deployments from ISO
      installer and rpm-ostree upgrades from a bare RHEL host
   1. The following upgrade paths will be covered:
      1. RHEL 9.2 / 4.14.latest → RHEL 9.4 / 4.16.Z
      1. RHEL 9.3 / 4.14.latest → RHEL 9.4 / 4.16.Z
      1. RHEL 9.2 / 4.15.latest → RHEL 9.4 / 4.16.Z
      1. RHEL 9.3 / 4.15.latest → RHEL 9.4 / 4.16.Z
   1. Z-stream upgrade testing for 4.16 will also include Z-1
      1. RHEL 9.4 / 4.16.Z-1 → RHEL 9.4 / 4.16.Z
   1. Rollback of each upgrade listed above will be performed
   1. Z-stream upgrade testing for 4.14 and 4.15 will remain at Y-1 and Z-1
   1. The multi-version update testing may be limited to EUS versions
      to manage the test workload.
1. RPM upgrades on RHEL hosts
   1. Given the support statement says MicroShift will support only
      "2 RHEL versions: the most recent EUS at the time of development
      and the next RHEL release after that EUS release", upgrade
      testing of MicroShift versions 4.14 or 4.15 to 4.16 will also
      require an OS upgrade from 9.2 or 9.3 to 9.4.
      1. MicroShift will need to be stopped and upgraded at the same time
         1. MicroShift 4.14 or 4.15 are not supported on RHEL 9.4
         1. MicroShift 4.16 is not supported on RHEL 9.2 or 9.3
   1. The upgrade paths described above will also be covered for RPM
      installed on RHEL
   1. RHEL states that it is unsafe to downgrade to an earlier minor
      release or an earlier EUS repository. Therefore, downgrades of
      MicroShift deployed with RPMs on RHEL is not supported and will
      not be tested

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

- Ability to utilize the enhancement end to end
- End user documentation
- Sufficient test coverage
- Available by default
- Conduct load testing

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The mechanics of upgrade and rollback for MicroShift do not change as
part of this work.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A

## Failure Modes

N/A

## Implementation History

* https://github.com/openshift/microshift/pull/2952

## Alternatives

We could limit the ability to skip versions so that it is possible to
go from an even version (EUS) to the next odd or even version, but not
allow moving from an odd (non-EUS) version to the next odd version
(4.14 to 4.16 would be OK, but 4.15 to 4.17 would not). This would
make the version checking logic more complicated and would introduce
opportunities for that skip-level upgrade process to be broken in a
non-EUS version so that it has to be fixed before the next EUS
release. By allowing skipping 1 of any type of version, we test the
feature continuously and avoid those issues.
