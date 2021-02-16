---
title: windows-machine-config-operator-upgrades
authors:
  - "@sebsoto"
reviewers:
  - "@sdodson"
  - "@derekwaynecarr"
approvers:
  - "@sdodson"
  - "@derekwaynecarr"
creation-date: 2020-07-10
last-updated: 2020-11-09
status: implementable
---

# Windows Machine Config Operator Upgrades

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement allows a cluster administrator to upgrade a deployed Windows
Machine Config Operator (WMCO), and associated Windows node components, to a
newer version.

## Motivation

The main motivation behind this enhancement is to give the end user
an easy and non-disruptive way to upgrade to a newer release of the WMCO, and
upgrade associated Windows node components with it.
WMCO is not part of the release payload, and is managed with [OLM](https://github.com/operator-framework/operator-lifecycle-manager).

### Goals

As part of this enhancement we plan to do the following:
* Enable upgrading WMCO with as little disruption as possible.
* Enable upgrading on-node Kubernetes components through newer versions of the
  WMCO, including major version upgrades across OpenShift cluster upgrades, and
  bug and CVE fixes during the current version.

### Non-Goals

In this enhancement we do not plan to support:
* Windows operating system updates.
* Upgrading Windows nodes which were not configured by WMCO.

## Proposal

To allow for the WMCO to be upgraded in a safe and non-disruptive way, it is
proposed that each Windows VM configured by previous versions of the WMCO has
its Machine object deleted, resulting in the drain and deletion of the Windows
node, and the termination and recreation of the VM. The upgraded WMCO instance
will then be able to configure VMs that will be created in place of the
terminated ones.

In order to allow for this to occur with minimal service disruption, it is
recommended that users have at least three Windows nodes within the cluster.

All Windows Kubernetes component updates will be tied to WMCO releases.

### Justification

The reason we need to cause the VMs to be fully terminated, instead of updating
them, is that changes made to the VM done in previous versions of the WMCO may
cause issues with the current version. If we chose to do an in-place upgrade,
we would need to actively undo changes made to the node that may cause issues
with the current WMCO version. The logic to keep track and undo these changes
adds an additional layer of complexity that also adds the benefit of a quicker
upgrade time. However, in-place upgrades do not provide any key benefits
for Windows workloads. For example, Windows has [restrictions regarding storage](https://kubernetes.io/docs/setup/production-environment/windows/intro-windows-in-kubernetes/#storage)
preventing the projection of volumes from host storage, because of this we do
not have to worry about preserving node local data.
With all aspects weighed, the risk of introducing additional issues to provide
an in-place upgrade is not worth it at this time, however it should be explored
as the product matures.

The reason we will not handle Windows operating system updates is that the
cluster administrator provides the Windows image to create the VMs with, and
it stays their responsiblity to provide an updated image.

### Design Details

WMCO is published to OperatorHub as a Red Hat operator. Each minor version of
OpenShift has a different Red Hat operators index. By adding the label
`com.redhat.openshift.versions` to the WMCO dist-git Dockerfile, and setting
its value to the appropriate OCP version, we can specify which operator index
WMCO will be released to.

When a cluster is upgraded OLM will switch to using a new Red Hat operators
index. Because WMCO is named the same in both indexes, OLM will upgrade WMCO
from the previous version, up to the latest version available in the new
cluster.

This update will either happen automatically, or require approval, based on
the settings given through OLM when initially installing the operator.

In order to facilitate upgrades, WMCO will add an annotation to the node
indicating the version of the operator which configured the node.
Using a node annotation is the easiest way to track this information in a way
that lasts across WMCO restarts. WMCO is directly responsible for the creation
of Windows nodes, and we should expect that other entities will not clear or
alter the annotations WMCO gives to such nodes.

On reconcile, if the annotated version does not match the current version,
or the annotation is unexpectedly missing, WMCO will delete the Machine
associated with the node. The [Machine API Operator](https://github.com/openshift/machine-api-operator/)
will drain and delete the node before the Machine deletion completes. It will
then create a new Machine to reconcile the MachineSet replica count. WMCO will
configure this new machine and apply the new version annotation. The max amount
of unavailable nodes at a time will be dependent on the maxUnhealthy field defined
internally by the WMCO. This field ensures that we only have maxUnhealthy number of
nodes that are not ready during upgrades. A Windows node is not ready if it
is missing the version annotation set by WMCO. Having limited number of unavailable
nodes avoids the downtime of the workloads running on the Windows nodes. The maxUnhealthy
value defaults to 1 per MachineSet minimizing the number of unavailable nodes and will
be configurable by the users in future releases. WMCO will not render more than the specified
amount of Windows nodes unavailable.

This design requires that all Windows Machines are backed by a MachineSet. This
is already a requirement of WMCO, and the addition of these changes only
increases the importance of this requirement, as WMCO will have the additional
privilege of Windows Machine deletion.

The procedure for an upgrade is as follows:
1) A new WMCO version is released
2) If the current cluster version fufills the minimum Kubernetes version
   requirement, OLM upgrades WMCO. If the cluster version is not high enough,
   the WMCO upgrade will occur once it is.
3) The new WMCO reconciles as usual, ensuring that all unconfigured Windows
   Machines are configured and join the cluster as a node. Each of them are
   given an annotation indicating the WMCO version that configured them.
4) Each Windows node is checked for the WMCO version annotation, if the
   annotated version of a Windows node does not match the WMCO version, and
   the number of unavailable Windows nodes is less than maxUnhealthy value,
   the associated Machine is deleted.
5) When a replacement Machine is created by the Machine API Operator, WMCO will
   reconcile again and configure the VM. This will repeat until all Windows
   nodes have been configured by the upgraded WMCO.

### User Stories

Stories can be found within the [Windows Upgrades epic](https://issues.redhat.com/browse/WINC-404)

### Risks and Mitigations

* If the cluster is upgraded, and the new version introduces a change that
  breaks the previous version of WMCO, Windows workloads will be interrupted
  for a period of time before WMCO is upgraded and rolls out the new nodes.
  This will be known ahead of release and can be messaged out appropriately.
  For unexpected complications, we will add alerts indicating nodes failing
  after upgrades.
* OLM does not allow for easy downgrading. The downgrade process is recommended
  to be done by removing the operator and reinstalling the previous version. If
  a new version of an operator causes breaking issues for a user they will have
  to perform this manual process.
* OLM does not have a way for us to force a WMCO upgrade if WMCO and the
  cluster version are incompatible. It is important that the cluster
  administrator ensures that a WMCO upgrade occurs if WMCO is not set to
  automatically upgrade.

### Test Plan

* It is planned to use the upcoming CI For Optional Operators
  [Container Verification Pipeline](https://issues.redhat.com/browse/DPTP-900)
  for release candidate testing and [PR-Based Testing Workflow](https://issues.redhat.com/browse/DPTP-1023)
  for testing upgrades on a PR basis.
* If the above epics are not completed in time, we will manually test WMCO
  upgrades for each release.
* A new upgrade test suite will be added, deploying a workload on a Windows
  node created by the current release of WMCO, performing an upgrade, and
  testing that the workload continues to function properly post-upgrade. This
  will be in addition to the existing test suites, which test that new
  workloads function properly when run on Windows nodes created by the
  candidate WMCO.

### Graduation Criteria

This enhancement will start as GA.

### Upgrade / Downgrade Strategy

This enhancement describes the upgrade strategy. Downgrades are [not supported](https://github.com/operator-framework/operator-lifecycle-manager/issues/1177)
by OLM.

### Version Skew Strategy

Kubernetes components on the Windows nodes should be brought up to date in a
reasonable timeframe. If WMCO is not set to automatically upgrade, it is the
cluster administrator's duty to upgrade it as soon as possible after a cluster
upgrade.

## Implementation History

[Windows Machine Config Operator](https://github.com/openshift/windows-machine-config-operator/)
