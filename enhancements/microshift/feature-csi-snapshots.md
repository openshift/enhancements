---
title: microshift-csi-snapshot-integration
authors:
  - @copejon
reviewers:
  - @dhellman
  - @pmtk
  - @eggfoobar
  - @pacevedom
approvers:
  - @dhellmann
api-approvers:
  - None
creation-date: 2023-05-10
last-updated: 2023-05-25
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-1140
see-also: []
---

# CSI Snapshotting Integration

## Summary

MicroShift is a small form-factor, single-node OpenShift targeting IoT and Edge Computing use cases characterized by tight resource constraints, unpredictable network connectivity, and single-tenant workloads. See [kubernetes-for-devices-edge.md](./kubernetes-for-device-edge.md) for more detail.

This document proposes the integration of the CSI Snapshot Controller to support backup and restore scenarios for cluster workloads.  The snapshot controller, along with the CSI external snapshot sidecar, will provide an API driven pattern for managing stateful workload data.


## Motivation

CSI snapshot functionality was originally excluded from the CSI driver integration in MicroShift to support the low-resource overhead goals of the project. However, user feedback has made it clear that a supportable, robust backup/restore solution is necessary.  While it would be possible to run a workflow out-of-band to manage workload data, this would be reinventing the wheel and contribute significantly to technical debt.  CSI Snapshots are already integrated into Openshift with ongoing downstream support. 

### User Stories

As an edge device owner running stateful workloads on MicroShift, I want to create in-cluster snapshots of that state and to restore workloads to that state utilizing existing Kubernetes patterns.

### Goals

* Enable an in-cluster workflow for snapshotting and restoration of cluster workload data
* Follow the [MicroShift design principles](https://github.com/openshift/microshift/blob/main/docs/design.md)

### Non-Goals

* Enable backup/restore workflows outside of MicroShift
* Provide a workflow for exporting data outside of a MicroShift cluster

## Proposal

Deploy the CSI snapshot controller and CSI plugin sidecar with MicroShift out of the box to provide users with a means of snapshotting, cloning, and restoring workload state.  Resource conscious users should be able to opt out of deploying these components.

### Workflow Description

#### Deploy MicroShift with Snapshotting

_Assuming_
* A running MicroShift cluster
* A running workload with an attached volume, backed by an LVM thin volume

_Workflow_
1. Install MicroShift RPMs
2. Start the MicroShift systemd service
3. Observe the CSI Snapshot controller pod reaches the Ready state
4. Observe the topolvm-controller pod reaches the Ready state

#### Snapshot, Dynamic

_Assuming_

* A running MicroShift cluster
* A running workload with an attached volume, backed by an LVM thin volume

_Workflow_

1. A user creates a VolumeSnapshot, specifying the source PVC and VolumeSnapshotClass
2. The CSI Snapshot Controller creates a VolumeSnapshotContent instance
3. The snapshot driver creates a new backing volume and clones the data from the source PVC’s volume
4. The CSI Snapshot Controller binds the VolumeSnapshotContent and the VolumeSnapshot together, signaling completion
5. The VolumeSnapshot may now be referenced by PVCs as a data source

#### Snapshot, Static
_Assuming_
* A running MicroShift cluster
* A pre-provisioned VolumeSnapshotContent API obj, representing a backend volume containing pre-populated data
* The CSI Snapshot controller is not deployed

_Workflow_

1. The user creates a VolumeSnapshot, specifying the VolumeSnapshotContent name as the source
2. The user creates a PVC, specifying the VolumeSnapshot as the dataSource.
3. The storage driver creates a new backing volume and clones the data from the dataSource to the new volume
4. The VolumeSnapshot will is now available as a PVC data source.

#### Restore

_Assuming_
* A running MicroShift cluster
* A bound VolumeSnapshot

_Workflow_

1.  A user creates a Pod and PVC, specifying the VolumeSnapshot as the data source
2. The CSI storage driver (topolvm), creates a new backing thin volume and a PV to represent the volume
3. The CSI storage driver creates a PV to expose the backend storage at the cluster level
4. The CSI storage driver clones the snapshot volume data to the new volume
5. The in-tree storage controller binds the PV to the PVC
6. The volume is mounted to the Pod’s filesystem and the Pod is started

#### Deploying

The CSI Snapshot Controller and TopoLVM components are deployed by default on MicroShift. The manifests for these components are baked into the MicroShift binary and are deployed upon first-boot.  This follows the existing pattern for MicroShift’s control-plane elements.

The CSI Snapshotter configuration is managed via the VolumeSnapshotClass API. This API serves a similar purpose as StorageClasses and allows admins to specify dynamic snapshotting parameters at runtime.  MicroShift will deploy a default VolumeSnapshotClass on first-boot. This instance will reference the default StorageClass that is already deployed by MicroShift to enable snapshotting out of the box.

The MicroShift deployment model utilizes rpm-ostree layers. Following the existing deployment pattern,  VolumeSnapshotClass manifests can be packaged and deployed onto target devices.  (See [kubernetes-for-device-edge.md#workflow-description.md](./kubernetes-for-device-edge.md#workflow-description)). Application deployers can predefine VolumeSnapshotClass manifests and use image-builder to package them into a rpm-ostree layer.  This layer can be installed to the device, which writes the manifests to /etc/microshift/manifests.  This pattern is congruent with how a device owner would install custom StorageClasses.  It is recommended that new custom StorageClasses be packaged with VolumeSnapshotClasses that reference them.

<!-- TODO DEFAULT SNAPSHOT CLASS -->

#### Upgrading

The CSI Snapshot component images are packaged and versioned with each OpenShift release and can be extracted from the ocp-release image. This allows MicroShift to use existing rebase tooling to upgrade the CSI Snapshot components in step with OCP releases.

#### Configuring

Cluster admins will use the VolumeSnapshotClass API to set dynamic configurations for snapshot creation. Driver-specific configuration is available through the ‘parameters’ sub-field, which is a string:string map and is defined by the particular storage provider.

#### Deploying Applications

### API Extensions

CSI Volume Snapshot APIs are a core component of OpenShift Container Platform and are detailed in [OCP documentation](https://docs.openshift.com/container-platform/4.13/storage/container_storage_interface/persistent-storage-csi-snapshots.html).

A portion of the OpenShift documentation is provided below for additional API detail.

#### VolumeSnapshotClass

Allows a cluster administrator to specify different attributes belonging to a VolumeSnapshot object. These attributes may differ among snapshots taken of the same volume on the storage system, in which case they would not be expressed by using the same storage class of a persistent volume claim.

The VolumeSnapshotClass CRD defines the parameters for the csi-external-snapshotter sidecar to use when creating a snapshot. This allows the storage back end to know what kind of snapshot to dynamically create if multiple options are supported.

Dynamically provisioned snapshots use the VolumeSnapshotClass CRD to specify storage-provider-specific parameters to use when creating a snapshot.

The VolumeSnapshotContentClass CRD is not namespaced and is for use by a cluster administrator to enable global configuration options for their storage back end.

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: csi-hostpath-snap
driver: hostpath.csi.k8s.io [1] 
deletionPolicy: Delete
```

1. The name of the CSI driver that is used to create snapshots of this **VolumeSnapshotClass** object. The name must be the same as the Provisioner field of the storage class that is responsible for the PVC that is being snapshotted.

#### VolumeSnapshot

Similar to the PersistentVolumeClaim object, the VolumeSnapshot CRD defines a developer request for a snapshot. The CSI Snapshot Controller Operator runs the CSI snapshot controller, which handles the binding of a VolumeSnapshot CRD with an appropriate VolumeSnapshotContent CRD. The binding is a one-to-one mapping.

The VolumeSnapshot CRD is namespaced. A developer uses the CRD as a distinct request for a snapshot.

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: mysnap
spec:
  volumeSnapshotClassName: csi-hostpath-snap [1] 
  source:
    persistentVolumeClaimName: myclaim [2]
```

1. The request for a particular class by the volume snapshot. If the **volumeSnapshotClassName** setting is absent and there is a default volume snapshot class, a snapshot is created with the default volume snapshot class name. But if the field is absent and no default volume snapshot class exists, then no snapshot is created.
2. The name of the **PersistentVolumeClaim** object bound to a persistent volume. This defines what you want to create a snapshot of. Required for dynamically provisioning a snapshot.

#### VolumeSnapshotContent

A snapshot taken of a volume in the cluster that has been provisioned by a cluster administrator.

Similar to the PersistentVolume object, the VolumeSnapshotContent CRD is a cluster resource that points to a real snapshot in the storage back end.

For manually pre-provisioned snapshots, a cluster administrator creates a number of VolumeSnapshotContent CRDs. These carry the details of the real volume snapshot in the storage system.

The VolumeSnapshotContent CRD is not namespaced and is for use by a cluster administrator.

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotContent
metadata:
  name: new-snapshot-content-test
  annotations:
    - snapshot.storage.kubernetes.io/allow-volume-mode-change: "true"
spec:
  deletionPolicy: Delete
  driver: hostpath.csi.k8s.io
  source:
    snapshotHandle: 7bdd0de3-aaeb-11e8-9aae-0242ac110002
  sourceVolumeMode: Filesystem
  volumeSnapshotRef:
    name: new-snapshot-test
    namespace: default
```

### Risks and Mitigations

- Increased overhead.  The CSI snapshot controller and sidecar increase the on-disk footprint by a total of 360Mb.  At idle, the components use a negligible amount of CPU (>1% CPU on a 2 core machine) and roughly 18Mb.  Under load, the 

### Drawbacks

## Design Details

### Open Questions [optional]

### Test Plan

Snapshotting functionality will be tested under the Openshift conformance test suite.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Full GA

- Available by default

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The CSI Snapshot controller and sidecar are pieces of the Kubernetes CSI implementation.  OpenShift maintains downstream versions of these images and tracks them as part of OCP releases.

### Version Skew Strategy

The CSI Snapshot version is tracked by the ocp-release image.  This provides sufficient guardrails against version skew.

### Operational Aspects of API Extensions

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

## Alternatives

There are no alternatives to deploying the Kubernetes CSI implementation of the snapshotting specification.