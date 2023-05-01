---
title: csi-proxy
authors:
  - "@sebsoto"
reviewers:
  - "@openshift/openshift-team-windows-containers"
  - "@openshift/openshift-team-storage, for potential pitfalls that could be ran into"
approvers:
  - "@aravindhp"
api-approvers:
  - None
creation-date: 2023-03-29
last-updated: 2023-03-29
tracking-link:
  - https://issues.redhat.com/browse/OCPBU-465
---

# Enable running CSI drivers on Windows Nodes through CSI Proxy

## Release Sign-off Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary

The goal of this enhancement is to enable the use of persistent storage for Windows workloads, without the use of
in-tree storage drivers, [which are in the process of being deprecated and removed.](https://kubernetes.io/blog/2022/09/26/storage-in-tree-to-csi-migration-status-update-1.25/)

This will be done by running [CSI Proxy](https://github.com/kubernetes-csi/csi-proxy) as a Windows service on each
Windows node. csi-proxy is a pre-requisite for CSI node driver pods to interact with the underlying Windows storage
APIs. With csi-proxy running on a cluster's Windows nodes, cluster administrators are free to deploy the CSI storage
drivers of their choice as a Windows DaemonSet, enabling Windows pods on each node to have access to storage through
PersistentVolumeClaims.

## Motivation

Windows nodes on OpenShift have had persistent storage available through the use of the in-tree storage drivers.
Persistent storage is a necessary feature for production workloads on Kubernetes, and it is important that this
functionality continue to work, even with the in-tree drivers being removed.

### User Stories

* As a developer using OpenShift, I want the Windows workloads I create to be able to read and write to storage which
  persists when pods are removed and re-created.

### Goals

* Windows nodes configured by WMCO fulfill all the pre-requisites to run CSI node drivers.

### Non-Goals

* The Windows CSI storage drivers will not be distributed as part of OpenShift.
  * At this time, we are not able to ship Windows containers images as all Windows container images are packaged
    with a Windows kernel. Red Hat has a policy to not ship 3rd party kernels for support reasons.
* Enabling the use of volumes which can be shared between Linux and Windows.

## Proposal

The [windows-machine-config-operator](https://github.com/openshift/windows-machine-config-operator/)(WMCO),
will ensure that csi-proxy is running on the Windows nodes it configures. This will be done through the normal
workflow for adding a service to the expected service list.

The csi-proxy binary will built from source and baked into the WMCO image. WMCO will copy the binary to each node at
configuration time. A new item will be added to the windows-services ConfigMap, instructing the
windows-instance-config-daemon to ensure that csi-proxy is always running as a Windows service on each node.

With the nodes in this state, users are free to follow upstream documentation to deploy the CSI node drivers
of their choice on the Windows nodes in the cluster.

No support from Red Hat will be given for issues occurring within the upstream drivers, as they are not being
distributed as part of OpenShift.

### Workflow Description

1. The cluster administrator will identify what CSI storage drivers the cluster requires for its workloads.
2. Based on the requirements of the workloads, the cluster admin can follow upstream documentation to deploy the CSI
   node drivers on its Windows nodes.

### API Extensions

N/A

### Risks and Mitigations

* If there are any security issues present in the CSI driver images deployed by the user, they will not be fixed
  by upgrading OCP. It is up to the user to stay informed and update their images as necessary.
* A mismatch between the OpenShift Linux CSI drivers and the user deployed Windows CSI drivers could cause undefined
  behavior. This is described in the [version skew](#version-skew-strategy) section.

### Drawbacks

The main drawback to this approach is it puts the onus on the cluster administrator. As they will be required to deploy
and maintain the Windows CSI node driver DaemonSet themselves, they will only be given support for issues with either
the CSI driver controller, or the csi-proxy Windows service. Support will not be given for issues originating within
the upstream CSI node drivers.

This may cause an increased load on both support and developers, as support issues coming in will need to be
properly filtered out based on the root cause of the problem.

## Design Details

### Test Plan

WMCO already has an extensive end to end test suite. To facilitate testing the additions described in this enhancement,
the e2e tests will be augmented to deploy a CSI node driver DaemonSet across Windows nodes, just as a user would. The
e2e tests would then validate that a Windows pod is able to mount a volume backed by the driver. Initially vSphere
storage will be tested, and other platforms can be tested in the future as desired. The purpose of testing is
to ensure CSI proxy is properly interacting with the Windows instance and Windows pods.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

This feature is targeted for WMCO 9.0.0 in OpenShift 4.14.
It will be backported to WMCO v7 and v8.

#### Removing a deprecated feature

No action required.

### Upgrade / Downgrade Strategy

#### Cluster upgrades

[When upgrading cluster versions, in-tree driver Volumes will automatically be updated to use the appropriate CSI node
drivers by the cluster](https://docs.openshift.com/container-platform/4.12/storage/container_storage_interface/persistent-storage-csi-migration.html).
In order to properly make use of this, and ensure a clean transition with minimal workload interruption, the CSI node
drivers must be running on the Windows node before the upgrade commences. This will be up to the cluster administrator
to do. If this has not been done, and if in-tree storage volumes are being used by Windows workloads, WMCO will block
upgrades via its [OperatorCondition](https://olm.operatorframework.io/docs/concepts/crds/operatorcondition/).
WMCO is able to detect that the CSI drivers have been sucessfully deployed by checking that all Windows nodes have a
CSINode object associated with them.

This feature is targeted for WMCO 9.0.0, but will be backported to WMCO v8 and v7. This enables a smooth transition
of volumes when upgrading from OCP 4.12 and 4.13. This specifically effects Azure, which loses in-tree storage
functionality in 4.13, and vSphere which loses it in 4.14. New clusters on other platforms should be created with a
minumum version of OCP 4.12, so they can immediately use CSI driver storage, as there will not be a way to migrate
in-tree volumes for those platforms.

If users are making use of the Linux CSI node drivers on the cluster, and not deploying all parts of storage
themselves, users will have to ensure the following:
Before upgrading, users should check the OCP release notes to determine how the Linux CSI drivers have changed, and
if they need to use a new Windows CSI node driver image. If so, the Windows DaemonSet should be updated once the
cluster upgrade is complete. More on this can be found in the [version skew](#version-skew-strategy) section.

WMCO does not support downgrades.

#### csi-proxy upgrades

The csi-proxy binary will be updated [the same as all other Windows services configured by WMCO/WICD](health-management.md#upgrade--downgrade-strategy).
The service will be stopped, if necessary a newer binary will be copied over, and the service will be started again, with configuration defined in the windows-services ConfigMap.

### Version Skew Strategy

If the CSI driver pods included in OCP are used as the driver controller and Linux node drivers, while the Windows
node drivers use the upstream images, there may be a mismatch. This should be mitigated by the cluster administrator
ensuring that the upstream images they use are as close to a version match as possible with the OCP distributed
drivers. During cluster upgrades, it is expected that CSI Linux node drivers will run for a period of time with an older
version during CSI driver upgrade. Some version skew is expected. For Windows, that skewed period may be longer as there
is a manual process involved here.

To eliminate this issue, the cluster administrator can choose to do the following:
1) When only Windows workloads need access to storage, an OCP cluster with no CSI drivers installed can be
   used. The cluster admin would then follow upstream documentation to deploy the Windows CSI drivers.
2) When both Windows and Linux workloads need access to storage, using separate clusters for Linux and Windows
   workloads is an option. This will allow the user to still receive support from Red Hat for Linux storage issues,
   while removing the potential for version skew. Having separate clusters for Linux and Windows workloads is already
   a recommendation that is made for security reasons.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

As stated in the [drawbacks](#drawbacks) section, the only customer issues Red Hat will help with are those that
are caused by either an error within csi-proxy or an issue with the CSI driver controller. The csi-proxy logs
will be collected as part of must-gather, which should be attached to each customer bug.

## Implementation History

The implementation history can be tracked by following the associated work items in Jira and source code improvements in
the WMCO Github repo.

## Alternatives

The ideal solution for this problem is for the Windows CSI node drivers to be added to each OpenShift CSI operator,
such as the [vmware-vsphere-csi-driver-operator](https://github.com/openshift/vmware-vsphere-csi-driver-operator),
as a DaemonSet for Windows nodes. As previously stated, this is not possible due to the inability for OpenShift to ship
and support Windows images. In the future, we could use [host process container images](https://github.com/microsoft/windows-host-process-containers-base-image),
which may not be subject to the same restrictions, however that would require CI and CPaaS pipelines to support
building Windows containers.

Another potential solution is forking each CSI driver and modifying them to run directly on the node as a Windows
service. This would allow us to ship the Windows node drivers as part of our product remove the requirement for the
user to deploy them themselves. This is not seen as a viable solution. The amount of work required to initially do so,
and then maintain the changes for every driver we support, is too large to undertake.
