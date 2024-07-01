---
title: optionally-deploy-csi-plugin
authors:
  - "@copejon"
reviewers:
  - "@pacevedom: MicroShift team-lead"
  - "@jerpeter1, Edge Enablement Staff Engineer"
  - "@jakobmoellerdev Edge Enablement LVMO SME"
  - "@suleymanakbas91,  Edge Enablement LVMO SME"
approvers:
  - "@jerpeter1, Edge Enablement Staff Engineer"
api-approvers:
  - "None"
creation-date: 2024-06-10
last-updated: 2024-06-11
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-856
see-also:
  - "enhancements/microshift/microshift-default-csi-plugin.md"
---

# Optionally Deploy CSI Plugin

## Summary

MicroShift is a small form-factor, single-node OpenShift targeting IoT and Edge Computing use cases characterized by
tight resource constraints, unpredictable network connectivity, and single-tenant workloads. See
[kubernetes-for-devices-edge.md](./kubernetes-for-device-edge.md) for more detail.

Out of the box, MicroShift includes the LVMS CSI driver and CSI snapshotter. The LVM Operator is not itself packaged
with the platform, in adherence with the project's [guiding principles](./kubernetes-for-device-edge.md#goals). See the
[MicroShift Default CSI Plugin](microshift-default-csi-plugin.md) proposal for a more in-depth explanation of the
storage provider and reasons for its integration into MicroShift. Configuration of the driver is exposed via a config
file at /etc/microshift/lvmd.yaml. The manifests required to deploy and configure the driver and snapshotter are baked
into the MicroShift binary during compilation and are deployed during runtime. This means that even if a user wanted to
disable the driver and/or snapshotter, they would be deployed again when the service restarts. Additionally, the LVMS an
CSI containers required to run LVMS are always packaged with the product and thus will always consume a certain amount
of storage space devices where MicroShift is deployed.

## Motivation

Not all users may want a persistent storage provider or volume snapshot capabilities for their MicroShift deployments and
should be enabled to choose whether to deploy the driver and / or snapshotter. This will afford resource-conscious users
the opportunity to better tune their MicroShift deployment to their hardware requirements.

### User Stories

- A user deploys MicroShift with the LVMS driver and CSI snapshot controller.
- A user deploys MicroShift with the LVMS driver only.
- A user deploys MicroShift with neither the LVMS driver nor the CSI snapshot controller.

### Goals

- Provide the LVMS CSI driver and LVMS CSI snapshot controller as optional components of MicroShift
- Reuse the optional-installation pattern implemented by MicroShift for the [Multus CNI
  Plugin](./multus-cni-for-microshift.md).
- Enable installing the LVMS CSI driver and LVMS CSI snapshot controller to a cluster that was previously deployed
  without the components.

### Non-Goals

- Generalizing this design to install any other CSI driver.
- Uninstalling the CSI driver or snapshotter rpms will not uninstall the cluster level components. This is a data safety
  concern that should be handled by the user.
- Support installing CSI snapshotter, but not the CSI driver. The snapshotter depends on the CSI driver; it cannot
  run as a standalone component.

## Proposal

Extract away deployment of the LVMS CSI driver and LVMS CSI snapshot controller from MicroShift logic and provide the
components as separate, optional rpms. Users should be able to install these rpm simultaneously with core MicroShift
rpms. It should be possible to install the CSI components to a cluster that was initially deployed without them.
This should not be a reversible process: uninstalling the rpms should not delete CSI components or cluster APIs from the
cluster. Doing so endangers users' data and could lead to orphaning of LVM volumes.

### Workflow Description

**_Prerequisites_**

- Host has LVM installed

**_Installation with CSI Driver and Snapshotting_**

1. User determines there is a requirement for persistent storage and volume snapshotting.
2. User specifies an ostree blueprint which includes the following sections:
    1. Packages: microshift, microshift-greenboot, microshift-networking, microshift-selinux, microshift-lvms, 
       microshift-lvms-snapshotting
    2. File (Optional): lvmd.yaml
3. User compiles an ostree commit from the blueprint
4. User deploys the ostree commit to host
5. MicroShift host boots
6. MicroShift starts
7. MicroShift deploys:
   1. LVMS CSI manifests
   2. LVMS Snapshot manifests

**_Installation without CSI Driver and Snapshotting_**

1. User determines there is not a requirement for persistent storage and volume snapshotting.
2. User specifies an ostree blueprint which includes the following sections:
    1. Packages: microshift, microshift-greenboot, microshift-networking, microshift-selinux
3. User compiles an ostree commit from the blueprint
4. User deploys the ostree commit to host
5. MicroShift host boots
6. MicroShift starts

**_Day-1|2 Installation_**

1. User has previous installed MicroShift on a device.
2. User later determines there is a requirement for persistent storage and volume snapshotting.
3. User specifies an ostree blueprint which includes the following sections:
    1. Packages:  microshift-lvms, microshift-lvms-snapshotting
    2. File (Optional): lvmd.yaml
4. User compiles an ostree commit from the blueprint
5. User deploys the ostree commit to host
6. MicroShift host reboots
7. MicroShift starts
8. MicroShift deploys:
   1. LVMS CSI manifests
   2. LVMS Snapshot manifests

### API Extensions

- None

### Topology Considerations

#### Single-node Deployments or MicroShift

The changes proposed here only affect MicroShift.

### Implementation Details/Notes/Constraints

- **Rebase Changes:** 
  - LVMS and the snapshotting manifests will be moved from `microshift/assets/components/lvms` and 
  `microshift/assets/components/csi-snapshot-controller` to `microshift/assets/optional/lvms` and 
  `microshift/assets/optional/csi-snapshot-controller`. These paths will be updated in the 
    `microshift/scripts/auto-rebase/lvms_assets.yaml` to reflect the new paths.
  - LVMS image digests will be moved from `microshift/assets/release/release-$ARCH.json` to 
    `microshift/assets/optional/lvms/release-$ARCH.json`.
  - CSI Snapshotter image digests will be moved from `microshift/assets/release/release-$ARCH.json` to 
      `microshift/assets/optional/csi-snapshot-controller/release-$ARCH.json`.
  - During rebasing, the image digests for LVMS and CSI Snapshotter will be parsed from the LVMS bundle (like they 
    are now) and the digests will be written to their respective `release-$ARCH.json` files.

- **Mainline Code Changes:**
  - MicroShift source code will no longer manage LVMS or CSI manifest deployment or its configuration. Therefore the 
    microshift service manager which handles these components will be removed entirely. The following files will be 
    deleted:
    - `microshift/pkg/components/csi-snapshot-controller.go`
    - `microshift/pkg/components/storage.go`
    - `microshift/pkg/components/render_test.go`, which only tests LVMS parameter rendering
    - `microshift/pkg/assets/storage.go`
  - Additionally, certain functions related to managing LVMS will be deleted:
    - `microshift/pkg/components/render_test.go:startCSIPlugin()`
    - `microshift/pkg/components/render_test.go:startCSISnapshotterController()`

### Risks and Mitigations


### Drawbacks

**TBD**

## Test Plan

There are already tests for RPM and Ostree install processes. This proposal only slightly alters the installation 
process and thus will require only minimal changes to existing tests.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- End-to-end tests
- Sufficient time for feedback
- Available by default
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Upgrade / Downgrade Strategy

**TBD**

## Version Skew Strategy

**TBD**

## Operational Aspects of API Extensions

**TBD**

## Support Procedures

**TBD**

## Alternatives

**TBD**