---
title: optionally-deploy-csi-plugin
authors:
  - "@copejon"
reviewers:
  - "@pacevedom: MicroShift team-lead"
  - "@jerpeter1, Edge Enablement Staff Engineer"
  - "@jakobmoellerdev Edge Enablement LVMS Engineer"
  - "@suleymanakbas91,  Edge Enablement LVMS Engineer"
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

Out of the box, MicroShift includes the LVMS CSI driver and CSI snapshotter. The LVM Operator is not included with the
platform, in adherence with the project's [guiding principles](./kubernetes-for-device-edge.md#goals). See the
[MicroShift Default CSI Plugin](microshift-default-csi-plugin.md) proposal for a more in-depth explanation of the
storage provider and reasons for its integration into MicroShift. Configuration of the driver is exposed via a config
file at /etc/microshift/lvmd.yaml. The manifests required to deploy and configure the driver and snapshotter are baked
into the MicroShift binary during compilation and are deployed during runtime. As it is now, even if a user wanted to
disable the driver and/or snapshotter, MicroShift would deploy them again when the service restarts. Additionally, the
LVMS and CSI images are always packaged together with MicroShift and thus always consume a certain amount of storage
space on the MicroShift host.

## Motivation

Not all users may want dynamic persistent storage or volume snapshotting for their MicroShift deployments and should be
enabled to choose whether to deploy the driver and / or snapshotter. This will afford resource-conscious users the
opportunity to better tune their MicroShift deployment to their hardware requirements.

### User Stories

- A user deploys MicroShift with the LVMS driver and CSI snapshot controller.
- A user deploys MicroShift with the LVMS driver only.
- A user deploys MicroShift with neither the LVMS driver nor the CSI snapshot controller.

### Goals

- Provide the LVMS CSI driver and LVMS CSI snapshot controller as optional components of MicroShift
- Reuse the optional-installation pattern implemented by MicroShift for the [Multus CNI
  Plugin](./multus-cni-for-microshift.md).
- Do not alter the LVMS-on-MicroShift user experience 

### Non-Goals

- Generalizing this design to install any other CSI driver.
- Users should not expect that uninstalling the RPMs will affect running containers
- Support installing CSI snapshotter, but not the CSI driver. The snapshotter depends on the CSI driver; it cannot
  run as a standalone component.

## Proposal

Extract away deployment of the LVMS CSI driver and LVMS CSI snapshot controller from MicroShift logic and provide the
components as separate, optional rpms. The new rpms will be named `microshift-lvms` and `microshift-lvms-snapshotting`.
`microshift-lvms-snapshotting` will have a dependency on `microshift-lvms` because it cannot be run effectively as a
standalone component. Users should be able to install these rpms simultaneously with core MicroShift rpms. It should
also be possible to install LVMS and CSI components to a cluster that was initially deployed without them. This should
not be a reversible process: uninstalling the rpms should not delete CSI components or cluster APIs from the cluster.
Doing so endangers users' data and could lead to orphaning of LVM volumes. Instead, uninstalling the rpms will only
ensure that LVMS and Snapshotting are not deployed on MicroShift startup/restart. It will be the user's responsibility
to ensure there are not LVMS PVs in the cluster before deleting the LVMS and Snapshotting API instances.

### Workflow Description

**_Prerequisites_**

- Host has LVM installed

**_Installation with CSI Driver and Snapshotting_**

1. User determines there is a requirement for persistent storage and volume snapshotting.
2. User specifies an ostree blueprint which includes the following sections:
    1. Packages: microshift, microshift-greenboot, microshift-networking, microshift-selinux, microshift-lvms, 
       microshift-lvms-snapshotting
    2. File (Optional, User Defined): lvmd.yaml
3. User compiles an ostree commit from the blueprint
4. User deploys the ostree commit to host
5. MicroShift host boots
6. MicroShift starts
7. MicroShift deploys:
   1. LVMS Snapshot manifests
   2. LVMS CSI manifests

**_Installation without CSI Driver and Snapshotting_**

1. User determines there is not a requirement for persistent storage and volume snapshotting.
2. User specifies an ostree blueprint which includes the following sections:
    1. Packages: microshift, microshift-greenboot, microshift-networking, microshift-selinux
3. User compiles an ostree commit from the blueprint
4. User deploys the ostree commit to host
5. MicroShift host boots
6. MicroShift starts without storage support

**_Day-1|2 Installation_**

1. User has previous installed MicroShift on a device.
2. User later determines there is a requirement for persistent storage and volume snapshotting.
3. User specifies an ostree blueprint which includes the following sections:
    1. Packages:  microshift-lvms, microshift-lvms-snapshotting
    2. File (Optional, User Defined): lvmd.yaml
4. User compiles an ostree commit from the blueprint
5. User deploys the ostree commit to host
6. MicroShift host reboots
7. MicroShift starts
8. MicroShift deploys:
    1. LVMS Snapshot manifests
    2. LVMS CSI manifests

**_Uninstallation_**

1. User has deployed a cluster with LVMS and CSI Snapshotting installed.
2. User has run pods on the cluster with LVMS PVs.
3. User decides to LVMS and snapshotting are no longer needed.
4. User checks that there are no LVMS PVs, PVCs, or Snapshots in the cluster. If there are, it is the user's 
   responsibility to prevent unintentional data loss or LVM volume orphaning before proceeding.
5. User uninstalls microshift-lvms and microshift-lvms-snapshotting rpms.
6. User deletes the relevant cluster API resources
    ```shell
    $ oc delete -n kube-system deployment.apps/csi-snapshot-controller deployment.apps/csi-snapshot-webhook   
    $ oc delete -n openshift-storage daemonset.apps/topolvm-node
    $ oc delete -n openshift-storage deployment.apps/topolvm-controller
    $ oc delete -n openshift-storage configmaps/lvmd
    $ oc delete oc get storageclasses.storage.k8s.io/topolvm-provisioner
    ```
7. The cluster processes the deletions and the process is complete

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
  - MicroShift source code will no longer manage LVMS or CSI manifest deployment or its configuration. The 
    microshift service manager which handles these components will be removed entirely. The following files will be 
    deleted:
    - `microshift/pkg/components/csi-snapshot-controller.go`
    - `microshift/pkg/components/storage.go`
    - `microshift/pkg/components/render_test.go`, which only tests LVMS parameter rendering
    - `microshift/pkg/assets/storage.go`
  - Additionally, certain functions related to managing LVMS will be deleted:
    - `microshift/pkg/components/render_test.go:startCSIPlugin()`
    - `microshift/pkg/components/render_test.go:startCSISnapshotterController()`
  - MicroShift also contains logic to determine if LVM is installed and attempt a best-guess at which volume group, 
    if any, should be set in the LVMD config. This code will need to be removed and implemented as:
    - Specifying LVM as an RPM dependency of `microshift-lvms`.
    - A `microshift-lvms` rpm post-install script that determines what volume group, if any, should be specified.

- **RPM Spec**
  - New RPM specs will be written for `microshift-lvms` and `microshift-lvms-snapshotting` packages. These have the 
    following characteristics:
    - `microshift-lvms-snapshotting` will depend on `microshift-lvms`.
    - `microshift-lvms` will depend on `lvm2` to ensure host compatibility
    - `microshift-lvms` will encapsulate LVMS and CSI driver container images and manifests. A post-install script 
      will attempt a best-guess at a default volume-group by reimplementing the go log in microshift as bash or 
      python. Not being able to identify a suitable volume-group is not a fatal error, and a warning will be logged 
      during install. The user will be responsible for properly configuring the lvmd file.
    - `microshift-lvms-snapshotting` is effectively a wrapper around CSI snapshotting images and manifests. It does 
      not encapsulate LVMS images or configurations.
  - It is critical that user-defined LVMD configurations be preserved during upgrades, downgrades, and reinstalls. RPM 
    post-install logic will be not stomp on existing lvmd configurations. Overwriting the lvmd config poses a 
    significant threat of orphaning LVM volumes if MicroShift were to change the volume-groups used by lvmd.

- **Dynamic Configuration Defaults**
  - For LVMS to start, it must have a configuration file. Thus, in order to support running out of the box and 
    after upgrades, a default configuration should be provided when the user does not create one. This is handled by 
    MicroShift right now in
[mainline code](https://github.com/openshift/microshift/blob/main/pkg/config/lvmd/lvmd.go). This logic must also ensure
    that user-defined configurations are not overwritten after upgrading, which could break the system and require user
    intervention.
  - The dynamic portion of the default config is the default LVM volume group. Defaulting logic will be executed by a 
    post-install section of the RPM spec. It will decide if a default config is necessary, and if so, which volume 
    group to use:
    1. If a configuration file exists, skip creating a default config file and continue rpm installation.
    2. Else, if there are 0 volume groups, halt installation and report the error to the user.
    3. Else, if there is 1 and only 1 volume group, set it as the default VG in the LVMS `deviceClass`.
    4. Else, if there are more than 1 volume groups, check if one named `microshift` exists.
       1. If it does exist, set it as the default VG in the LVMS `deviceClass`.
       2. Else, halt installation and report the error to the user.

### Risks and Mitigations

- N/A

### Drawbacks

- N/A

## Test Plan

There are already tests for RPM and Ostree install processes. This proposal only slightly alters the installation 
process and thus will require only minimal changes to existing tests.

## Graduation Criteria

### GA

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Upgrade / Downgrade Strategy

LVMS and CSI operate on existing Kubernetes APIs to track the state of cluster storage. It is safe to upgrade or 
downgrade the runtime components without losing accountability or otherwise endangering provisioned storage.

## Version Skew Strategy

Risk of version skew is mitigated because the LVMS and CSI components are built from the same release image as the 
rest of MicroShift's components. It is safe to upgrade and downgrade LVMS and/or CSI within the same major version 
of MicroShift, just as it is on OCP.

## Operational Aspects of API Extensions

<!--
Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.
-->


## Support Procedures

- N/A

## Alternatives

Use the MicroShift or LVMD config to determine deployment of LVMS and/or snapshotting. It does not satisfy the goal of
optionalizing the installation of the components. Instead, this solution would simply enable/disable the components.
Thus, LVMS and CSI components would have to be installed on the host even if they were never to be used.