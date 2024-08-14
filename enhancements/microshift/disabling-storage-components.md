---
title: disabling-storage-components
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
creation-date: 2024-07-11
last-updated: 2024-07-11
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-856
see-also:
  - "enhancements/microshift/microshift-default-csi-plugin.md"
---

# Disabling Storage Components

## Summary

MicroShift is a small form-factor, single-node OpenShift targeting IoT and Edge Computing use cases characterized by
tight resource constraints, unpredictable network connectivity, and single-tenant workloads. See
[kubernetes-for-devices-edge.md](./kubernetes-for-device-edge.md) for more detail.

Out of the box, MicroShift includes the LVMS CSI driver and CSI snapshotter. The LVM Operator is not included with the
platform, in adherence with the project's [guiding principles](./kubernetes-for-device-edge.md#goals). See the
[MicroShift Default CSI Plugin](microshift-default-csi-plugin.md) proposal for a more in-depth explanation of the
storage provider and reasons for its integration into MicroShift. Configuration of the driver is exposed via a config
file at /etc/microshift/lvmd.yaml. The manifests required to deploy and configure the driver and snapshotter are baked
into the MicroShift binary during compilation and are deployed during runtime.

LVMD is the node-side component of LVMS and is configured via a config file. If LVMS is started with no LVMD config, the
process will crash, causing a crashLoopBackoff and requiring user intervention. If the file `/etc/microshift/lvmd.yaml`
does not exist, MicroShift will attempt to generate a config in memory and provide it to LVMD via config map. If it
cannot create the config, it will skip LVMS and CSI components instantiation.

Thus, it is already _technically_ possible to disable the storage components, but only via this esoteric functionality.
Users should be provided a clean UX for this feature and not be forced to learn the inner-workings of MicroShift.

## Motivation

There are variety of reasons a user may not want to deploy LVMS. Firstly, MicroShift is designed to run on hosts with as
little as 2Gb of memory. Users operating on such small form factors are going to be resource conscious and seek to limit
unnecessary consumption as much as possible. At idle, the LVMS and CSI components consume roughly 50Mi of memory. 
Providing a user-facing API to disable LVMS or CSI snapshotting is therefore a must for MicroShift.

### User Stories

- A user is operating MicroShift on a small-form factor machine with cluster workloads that require persistent storage
  but do not require volume snapshotting.
- A user is operating MicroShift on a small-form factor machine, is reducing resource consumption wherever possible, and
does not have a requirement for persistent cluster-workload storage.

### Goals

- Enhance the MicroShift config API to support selective deployment of LVMS and CSI Snapshotting.
- Do not make backwards-incompatible changes to the MicroShift config API.
- Do not endanger or make inaccessible persistent data on upgraded clusters.

### Non-Goals

- Provide a generalized framework to support potential future alternatives to LVMS.
- Generically enable installing CSI components without enabling LVMS.

## Proposal

### Workflow Description

**_Installation with LVMS and Snapshotting (Default)_**

1. User installs MicroShift. A MicroShift and LVMD config are not provided.
2. MicroShift starts and detects no config and falls back to default MicroShift configuration (LVMS and CSI on by default).
3. MicroShift checks to determine if host is compatible with LVMS. If it is not, an error is logged, 
LVMS and CSI installation is skipped and install continues. (This it the current behavior in MicroShift)
4. MicroShift attempts to dynamically create generate the LVMD config. If it cannot, an error is logged, LVMS and CSI
installation is skipped and install continues.
5. If all checks pass, LVMS and CSI are deployed.

**_Installation without LVMS and Snapshotting_**

1. User installs MicroShift.
2. User specifies a MicroShift config with fields to disable LVMS and CSI Snapshots.
3. MicroShift starts and detects the provided config.
4. LVMS and CSI snapshot components are not deployed.

**_Post-Start Installation, LVMS only_**

1. User has already installed and started MicroShift service.
2. User later determines there is a requirement for persistent storage, but not snapshotting.
3. User edits the MicroShift config, enabling LVMS only.
4. User restarts MicroShift service.
5. MicroShift starts and detects the provided config.
6. MicroShift performs LVMS startup checks.
7. If all checks pass, LVMS is deployed. Else if checks fail, an error is logged, LVMS deployment is skipped, 
and startup continues.

**_Complete Uninstallation, with Data removal_**

> NOTE: Installation is not reversible. Deleting LVMS or the CSI snapshotter while there are still storage volumes risks
orphaning volumes. It is the user's responsibility to manually destroy data volumes and/or snapshots before
uninstalling.

1. User has already deployed a cluster with LVMS and CSI Snapshotting installed.
2. User has deployed cluster workloads with persistent storage.
3. User decides that LVMS and snapshotting are no longer needed.
4. User edits `/etc/microshift/config.yaml`, setting `.storage.driver: none`. 
5. User takes steps to back up, and then erase, or otherwise ensure that data cannot be recovered.
6. User stops workloads with mounted storage volumes.
   1. (Optional) If workloads can be run without persistent storage and the user wishes to do so: User 
      recreates the workload manifest(s) and specifies another provider, an emptyDir or hostpath volume, or no 
      storage at all.
7. User deletes VolumeSnapshots and waits for deletion of VolumeSnapshotContent objects to verify. The deletion process
cannot happen after the CSI Snapshotter is deleted.
8. User delete PersistentVolumeClaims and waits for deletion of PersistentVolumes. The deletion process
cannot happen after LVMS is deleted.
9. User deletes the relevant LVMS and Snapshotter cluster API resources:
    ```shell
    $ oc delete -n kube-system deployment.apps/csi-snapshot-controller deployment.apps/csi-snapshot-webhook
    $ oc delete -n openshift-storage daemonset.apps/topolvm-node
    $ oc delete -n openshift-storage deployment.apps/topolvm-controller
    $ oc delete -n openshift-storage configmaps/lvmd
    $ oc delete storageclasses.storage.k8s.io/topolvm-provisioner
    ```
10. Component deletion completes. On restart, MicroShift will not deploy LVMS or the snapshotter.

### API Extensions

- MicroShift config will be extended from the root with a field called `storage`, which will have 2 subfields.
  - `.storage.driver`: **ENUM**, type is preferable to leave room for future supported drivers
    - One of ["none", "lvms"]
    - An empty value or null field defaults to deploying LVMS. This preserves MicroShift's default installation workflow
      as it currently is, which in turn ensures API compatibility for clusters that are upgraded from 4.16. If the
      null/empty values defaulted to disabling LVMS, the effect would not be seen on the cluster until an LVMS or CSI
      component were deleted and the cluster is restarted. This creates a kind of hidden behavior that the user may not
      be aware of or want.
  - `.storage.optionalCsiComponents:` **Array**. Empty array defaults to deploying additional CSI 
  components.
    - Expected values are: ['csi-snapshot-controller', 'csi-snapshot-webhook'] OR ['none']. 'none' is mutually exclusive
    with all other values.
    - Even though it's the most common csi components, the csi-driver should not be part of this list because it is
    required, and usually deployed, by the storage provider.

```yaml
storage:
  driver: ENUM
  optionalCsiComponents: ARRAY
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

- N/A

#### Standalone Clusters

- N/A

#### Single-node Deployments or MicroShift

The changes proposed here only affect MicroShift.

### Implementation Details/Notes/Constraints

- We should not remove MicroShift's logic for dynamically generating LVMD default values in the absence of a
  user-provided config. Thus, these checks will be performed only if LVMS is enabled.

### Risks and Mitigations

- N/A

### Drawbacks

- This design adds a field to a user facing API specific to a non-MicroShift component. If MicroShift were to shift towards
being agnostic of the storage provider, this field would have to continue to be supported for existing users and 
deprecated for a reasonable time before being removed.

## Test Plan

Test scenarios will be written to validate the combinations of states the additional fields correlate to their desired
outcomes. Unit tests will be written as necessary.

## Graduation Criteria

### Dev Preview -> Tech Preview

- N/A

### Tech Preview -> GA

- N/A

### GA

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

MicroShift gracefully handles config fields it does not recognize. Downgrading from a system with these config fields
will not break MicroShift start up.

For clusters that disabled the storage components, a downgrade would result in these components being deployed. This
will not break the user's cluster but will result in additional resource overhead. This situation can't be avoided, as
even if the user scaled down the storage component replica sets, restarting or rebooting microshift would cause the
resources to be re-applied, thus scaling the sets back up.

LVMS installation automatically reversible so upgrades and downgrades do not present a danger to user's data.
Upgrading a cluster with LVMS to a cluster with LVMS set to disabled will not cause the storage components to be deleted.

## Version Skew Strategy

LVMS is versioned separately from MicroShift. However, MicroShift pins the version of LVMS during rebasing. This creates
a loose coupling between the two projects, but does not pose a risk of incompatibility. LVMS runs in cluster workloads
and thus does not directly interact with MicroShift internals.

LVMS does not interact with the MicroShift config API, so it will not be affected by this change.

## Operational Aspects of API Extensions

With LVMS and the CSI components deployed, the cluster overhead is increased by roughly 50Mb and 0.1% CPU. Though
comparatively small by OCP standards, 50Mb of memory is a non-trivial amount on far-edge devices.

## Support Procedures

- N/A

## Alternatives

Do Nothing. Continue to use the MicroShift or LVMD config to determine deployment of LVMS and/or snapshotting. Directing
the user to leverage low-level processes to achieve this goal is an anti-pattern and would provide a poor UX.

Install LVMS and CSI via rpm, as is done with other cluster components. MicroShift is strongly opinionated towards LVMS,
with significant logic for managing some of the LVMS life cycle and default configs. If LVMS manifests and container
images were installed via an RPM, the logic for handling dynamic defaults would need to be implemented elsewhere. Either as a %post-install stage or as a separate system service. Neither of these approaches is suitable for the goal of this EP, 
explodes the complexity of managing LVMS and CSI, as well as testing. Because of this, it should be considered in a
separate EP, if at all.
