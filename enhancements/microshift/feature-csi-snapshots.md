---
title: microshift-csi-snapshot-integration
authors:
  - copejon
reviewers:
  - dhellman
  - pmtk
  - eggfoobar
  - pacevedom
approvers:
  - dhellmann
api-approvers:
  - None
creation-date: 2023-05-10
last-updated: 2023-06-02
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-1140
see-also: []
---

# CSI Snapshotting Integration

## Summary

MicroShift is a small form-factor, single-node OpenShift targeting IoT and Edge Computing use cases characterized 
by tight resource constraints, unpredictable network connectivity, and single-tenant workloads. See 
[kubernetes-for-devices-edge.md](./kubernetes-for-device-edge.md) for more detail.

This document proposes the integration of the CSI Snapshot Controller to support backup and restore scenarios for 
cluster workloads.  The snapshot controller, along with the CSI external snapshot sidecar, will provide an API 
driven pattern for managing stateful workload data.

## Motivation

CSI snapshot functionality was originally excluded from the CSI driver integration in MicroShift to support the 
low-resource overhead goals of the project. However, user feedback has made it clear that a supportable, robust 
backup/restore solution is necessary.  While it would be possible to run a workflow out-of-band to manage workload 
data, this would be reinventing the wheel and contribute significantly to technical debt.  CSI Snapshots are already 
integrated into Openshift with ongoing downstream support. 

### User Stories

As an edge device owner running stateful workloads on MicroShift, I want to create in-cluster snapshots of that state 
and to restore workloads to that state utilizing existing Kubernetes patterns.

### Goals

* Enable an in-cluster workflow for snapshotting and restoration of cluster workload data
* Follow the [MicroShift design principles](https://github.com/openshift/microshift/blob/main/docs/design.md)

### Non-Goals

* Enable backup/restore workflows external to MicroShift
* Provide a workflow for exporting data from a MicroShift cluster
* Provide a means of opting out.  This falls under platform composability and is out of scope

## Proposal

Deploy the CSI snapshot controller and CSI plugin sidecar with MicroShift out of the box to provide users with a means of 
snapshotting, cloning, and restoring workload state.

### Workflow Description

#### Deploy MicroShift with Snapshotting Support

_Prerequisites_

* The device owner has created an LVM volume group and a thin-pool on this volume group.
* The `/etc/microshift/lvmd.yaml` config includes a `deviceClass` to represent the thin-pool.  The lvmd.yaml may 
contain a mix of thin and thick `deviceClasses`.

```yaml
device-classes:
- name: ssd-thin
  volume-group: myvg1
  type: thin
  thin-pool:
    name: pool0
    overprovision-ratio: 50.0
- name: default
  volume-group: rhel
  default: true
  spare-gb: 0
```

_Workflow_

1. The user installs the MicroShift RPMs, either via os-rpmtree layer or package manager.
2. The user starts the MicroShift systemd service
3. MicroShift starts the Service Manager loop to begin deploying cluster resources.
4. The Service Manager initiates the `startCSIPlugin()` service.
5. The `startCSIPlugin()` service deploys the CSI VolumeSnapshot CSI Snapshot Controller and Validating Webhook, and a 
   default VolumeSnapshotClass.
   - The `startCSIPlugin()` starts LVMS immediately after. It is not necessary to wait for the CSI Controller to 
         become **Ready** as there are no start-sequence dependencies between the 2 systems.
4. The CSI VolumeSnapshot Controller begins listening for VolumeSnapshot API events.
5. The user observes the CSI Snapshot Controller pod reaches the **Ready** state.
6. The user observes the `topolvm-controller` and `csi-snapshot-controller` pods reach the **Ready** state.
    - The `topolvm-controller` includes the `csi-external-snapshotter` sidecar container, which is why the user must 
      verify its phase.
      
#### Create a Snapshot Dynamically from a PVC

This section describes how to create snapshot from a PVC utilizing a VolumeSnapshotClass to dynamically specify 
provisioning parameters.

_Prerequisites_

* A running MicroShift cluster
* CSI Snapshot Controller is in **Ready** state
* Topolvm-Controller is in **Ready** state
* `volumeSnapshotClass` has been deployed
* `storageClass` has been deployed

_Workflow_
 
1. The user creates a PVC of an arbitrary size, in Gb increments, to be the source volume from which we will 
   create a snapshot.
2. The user creates a Pod which consumes the PVC.  This is required because LVMS only supports 
   `WaitForFirstConsumer` provisioning.
3. LVMS provisions the backend storage volume and creates a `logicalVolume.topolvm.io` CR.
4. The _csi-external-provisioner_, part of the _topolvm-controller_ pod, binds the PVC to the volume's PV.
5. The user's workload starts and mounts the volume.
6. The user stops the workload by deleting the consuming Pod, or if managed by a replication controller, scales the 
   replicas to 0.
7. The user creates a VolumeSnapshot object with the following key-values:
   - `.spec.volumeSnapshotClassName` set to the volume snapshot class's name. If not provided, falls back to default 
     class.
   - `.spec.source.persistentVolumeClaimName` set to the source PVC's name.
8. The _snapshotting validation webhook_ intercepts the API volume snapshot API request.
9. If validation succeeds, the `volumeSnapshot` is persisted in etcd.
   - Else: the VolumeSnapshot is rejected. (see [Failure Modes](#failure-modes)).
10. The _CSI external snapshotter_ sidecar, part of the _topolvm-controller_ deployment, detects the `volumeSnapshot` 
    event. It generates a `volumeSnapshotContent` object and triggers the `CreateSnapshot` CSI gRPC process.
11. LVMS executes the `CreateSnapshot` gRPC process and creates a snapshot of the LVM thin volume.
12. LVMS creates a `logicalVolume.topolvm.io` CR and returns the snapshot volume's metadata to the _CSI external 
   snapshotter_.
13. The _CSI external-snapshotter_ updates the `volumeSnapshotContent`'s `status` field to indicate it is ready.
14. The _CSI snapshot controller_ detects the update to the `volumeSnapshotContent` status and binds the
    `volumeSnapshot` to the `volumeSnapshotContent` instance. It sets the `volumeSnapshot`'s `status.readyToUse`
    field to `true`. 

From here, users will follow the [Restore](#restore) procedure to consume the snapshot volume.

#### Create a Snapshot Manually from a Pre-Provisioned Snapshot

This section describes how to create snapshot directly from a `volumeSnapshotContent` object.

_Prerequisites_

* A running MicroShift cluster
* CSI Snapshot Controller is in **Ready** state
* Topolvm-Controller is in **Ready** state
* Default `storageClass` has been deployed automatically

> NOTE: This is a static provisioning process, meaning the source `volumeSnapshotContent` object is already present. 
> There is not need for dynamically provisioning the `volumeSnapshotContent` and therefore we do not need to specify 
> a `volumeSnapshotClass`.    

1. The user creates a `volumeSnapshot` object with the following key-values:
   - `.spec.source.volumeSnapshotContentName` set to the source `volumeSnapshotContent`'s name. 
2. The _snapshotting validation webhook_ intercepts the API volume snapshot API request.
3. If validation succeeds, the `volumeSnapshot` is persisted in etcd.
   - Else: the VolumeSnapshot is rejected. (see [Failure Modes](#failure-modes)).
4. The _CSI external-snapshotter_ sidecar, part of the _topolvm-controller_ deployment, detects the `volumeSnapshot`
    event. It generates a `volumeSnapshotContent` object and triggers the `CreateSnapshot` CSI gRPC.
5. LVMS executes the `CreateSnapshot` gRPC and creates a snapshot of the LVM thin volume.
6. LVMS creates a `logicalVolume.topolvm.io` CR and returns the snapshot volume's metadata to the _CSI external 
   snapshotter_.
7. The _CSI external-snapshotter_ updates the `volumeSnapshotContent`'s `status` field to indicate it is ready.
8. The _CSI snapshot controller_ detects the update to the `volumeSnapshotContent` status and binds the
    `volumeSnapshot` to the `volumeSnapshotContent` instance. It sets the `volumeSnapshot`'s `status.readyToUse`
    field to `true`. 

From here, users will follow the [Restore](#restore) procedure to consume the snapshot volume.

#### Deleting a Volume Snapshot

> Backing logical volumes will be handled according to the deletion policy of the volumeSnapshotClass. Possible 
> values are: "Retain", "Delete".

_Prerequisites_

* A running MicroShift cluster
* CSI Snapshot Controller is in **Ready** state
* Topolvm-Controller is in **Ready** state
* A `volumeSnapshot` bound to a `volumeSnapshotContent`

_Workflow_

> NOTE: `volumeSnapshot` and `volumeSnapshotContent` instances will have finalizers set on them to control the deletion 
> sequence.

1. The user deletes the `volumeSnapshot` object. Finalizers on this object prevent immediate deletion. 

_If the reclaimPolicy is **Delete**:_

2. The `CSI snapshot controller` detects the deletion event and deletes the bound `volumeSnapshotContent` 
   object. Finalizers on this object prevent immediate deletion.
3. The _CSI external-snapshotter_ detects the `volumeSnapshotContent` deletion event.  It calls the `DeleteSnapshot` CSI 
   gRPC.
4. LVMS executes the `DeleteSnapshot` process.  The LVM snapshot is destroyed and the process returns to the caller.
5. The _CSI external-snapshotter_ removes the finalizer on the `VolumeSnapshotContent`, allowing it to be garbage collected
6. The _CSI snapshot controller_ removes the finalizer on the `VolumeSnapshot`, allowing it to be garbage collected

_If the reclaimPolicy is **Retain**:_

2. The `CSI snapshot controller` detects the deletion event. It removes finalizers on `volumeSnapshot` and 
   `volumeSnapshotContent` objects.
3. The `volumeSnapshot` is garbage collected.
4. The `volumeSnapshotContent` and backing LVM thin volume are retained.  A user can then delete the backing storage 
   by deleting the `volumeSnapshotContent`.  

#### Restore

Restoring is the process of creating a new lvm volume from a snapshot that is populated with data from the snapshot. 
This can be achieved two ways via the `spec.dataSource` PVC field.  

_Prerequisites_

* A `storageClass` is deployed automatically
* A `volumeSnapshotClass` is deployed automatically
* A `volumeSnapshot` is bound to a `volumeSnapshotContent` and `readyToUse` is `true

_Workflow_

1. The user creates a PVC with the following fields:
   - `spec.storageClassName`: must be the same storage class that provisioned the source volume
   - `spec.dataSource.name`: name of the source `volumeSnapshot`
   - `spec.dataSource.kind`: VolumeSnapshot
   - `spec.dataSource.apiGroup`: snapshot.storage.k8s.io
2. The user creates a Pod to consume the PVC. Required because LVMS only supports `WaitForFirstConsumer` provisioning.
3. The _CSI external provisioner_ sidecar, already integrated into the _topolvm-controller_, detects the PVC request,
   and makes a `CreateVolume` gRPC call to LVMS. 
4. _topolvm-controller_ creates a `logicalVolume.topolvm.io` CRD with the same values of the source `logicalVolume.
   topolvm.io` instance. This ensures the new volume is created on the same node as the source.
5. _topolvm-node_ creates a new thin-volume from the volume snapshot.
6. _topolvm-controller_ returns the success status to the `CreateVolume` gRPC caller.
7. The _CSI external provisioner_ creates a `persistentVolume` to track the new thin-volume
8. The _CSI external provisioner_ binds the PVC and PV together
9. The workload attaches the new volume and starts


#### Volume Cloning

Volume cloning uses CSI volume snapshotting interfaces behind the scenes.  It enables the pre-populating of a PVC's 
storage volume with data from an existing PVC.

_Prerequisites_

- A PVC has been created following one of the provisioning workflows above.

_Workflow_

1. The user creates a PVC with the following fields:
   - `spec.storageClassName`: must be the same storage class that provisioned the source volume
   - `spec.dataSource.name`: name of the source `persistentVolumeClaim`
   - `spec.dataSource.kind`: PersistentVolumeClaim
   - `spec.dataSource.apiGroup`: core
2. The user creates a Pod to consume the PVC. Required because LVMS only supports `WaitForFirstConsumer` provisioning.
3. The _CSI external provisioner_ sidecar, already integrated into the _topolvm-controller_, detects the PVC request,
   and makes a `CreateVolume` gRPC call to LVMS. 
4. _topolvm-controller_ creates a `logicalVolume.topolvm.io` CRD with the same values of the source `logicalVolume.
   topolvm.io` instance. This ensures the new volume is created on the same node as the source.
5. _topolvm-node_ creates a new thin-volume from the volume snapshot.
6. _topolvm-controller_ returns the success status to the `CreateVolume` gRPC caller.
7. The _CSI external provisioner_ creates a `persistentVolume` to track the new thin-volume
8. The _CSI external provisioner_ binds the PVC and PV together
9. The workload attaches the new volume and starts


#### Deploying

The CSI Snapshot Controller and LVMS components are deployed by default on MicroShift. The manifests for these 
components are baked into the MicroShift binary and are deployed upon startup. This follows the existing pattern 
for MicroShift’s control-plane elements.

The CSI Snapshotter configuration is managed via the VolumeSnapshotClass API. This API serves a similar purpose as 
StorageClasses and allows admins to specify dynamic snapshotting parameters at runtime.  MicroShift will deploy a 
default VolumeSnapshotClass on startup. This instance will reference the default StorageClass that is already 
deployed by MicroShift to enable snapshotting out of the box.

The MicroShift deployment model utilizes rpm-ostree layers. Following the existing deployment pattern, 
VolumeSnapshotClass manifests can be packaged and deployed onto target devices.
(See [kubernetes-for-device-edge. md#workflow-description.md](./kubernetes-for-device-edge.md#workflow-description)).
Application deployers can predefine VolumeSnapshotClass manifests and use image-builder to package them into a 
rpm-ostree layer.  This layer can be installed to the device, which writes the manifests to 
/etc/microshift/manifests. This pattern is congruent with how a device owner would install custom StorageClasses. It 
is recommended that new custom StorageClasses be packaged with VolumeSnapshotClasses that reference them.

#### Upgrading

The CSI Snapshot component images are packaged and versioned with each OpenShift release and can be extracted from 
the ocp-release image. This allows MicroShift to use existing rebase tooling to upgrade the CSI Snapshot components 
in step with OCP releases.

#### Configuring

Cluster admins will use the VolumeSnapshotClass API to set dynamic configurations for snapshot creation. 
Driver-specific configuration is available through the ‘parameters’ sub-field, which is a string:string map and is 
defined by the particular storage provider.

#### Deploying Applications

### API Extensions

CSI Volume Snapshot APIs are a core component of OpenShift Container Platform and are detailed in
[OCP documentation](https://docs.openshift.com/container-platform/4.13/storage/container_storage_interface/persistent-storage-csi-snapshots.html).

A portion of the OpenShift documentation is provided below for additional API detail.

#### VolumeSnapshotClass

Allows a cluster administrator to specify different attributes belonging to a VolumeSnapshot object.
These attributes may differ among snapshots taken of the same volume on the storage system, in which
case they would not be expressed by using the same storage class of a persistent volume claim.

The VolumeSnapshotClass CRD defines the parameters for the csi-external-snapshotter sidecar to use
when creating a snapshot. This allows the storage back end to know what kind of snapshot to
dynamically create if multiple options are supported.

Dynamically provisioned snapshots use the VolumeSnapshotClass CRD to specify
storage-provider-specific parameters to use when creating a snapshot.

The VolumeSnapshotContentClass CRD is not namespaced and is for use by a cluster administrator to
enable global configuration options for their storage back end.

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: csi-hostpath-snap
driver: hostpath.csi.k8s.io [1] 
deletionPolicy: Delete
```

1. The name of the CSI driver that is used to create snapshots of this **VolumeSnapshotClass** object. The name 
must be the same as the Provisioner field of the storage class that is responsible for the PVC that is being 
snapshotted.

#### VolumeSnapshot

Similar to the PersistentVolumeClaim object, the VolumeSnapshot CRD defines a developer request for a snapshot. The 
CSI snapshot controller handles the binding of a VolumeSnapshot CRD with an appropriate VolumeSnapshotContent CRD. 
The binding is a one-to-one mapping.

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

1. The request for a particular class by the volume snapshot. If the **volumeSnapshotClassName** setting is absent 
and there is a default volume snapshot class, a snapshot is created with the default volume snapshot class name. 
But if the field is absent and no default volume snapshot class exists, then no snapshot is created.
2. The name of the **PersistentVolumeClaim** object bound to a persistent volume. This defines what you want to 
create a snapshot of. Required for dynamically provisioning a snapshot.

#### VolumeSnapshotContent

A snapshot taken of a volume in the cluster that has been provisioned by a cluster administrator.

Similar to the PersistentVolume object, the VolumeSnapshotContent CRD is a cluster resource that
points to a real snapshot in the storage back end.

For manually pre-provisioned snapshots, a cluster administrator creates a number of
VolumeSnapshotContent CRDs. These carry the details of the real volume snapshot in the storage system.

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

- Increased overhead.  The CSI snapshot controller and sidecar increase the on-disk footprint by a
total of 360Mb.  At idle, the components use a negligible amount of CPU (>1% CPU on a 2 core machine)
and roughly 18Mb.

### Drawbacks

## Design Details

### CSI Snapshotter Components

The total additional cluster components are:

1. VolumeSnapshot CRD
2. VolumeSnapshotContent CRD
3. VolumeSnapshotClass CRD

### CSI-Snapshot Controller

The CSI Snapshot Controller is maintained SIG-Storage upstream. Downstream OCP releases of this container 
image will be used by MicroShift.  The controller is a standalone component responsible for watching 
`volumeSnapshot`, `volumeSnapshotContent`, and `volumeSnapshotClass` objects.  When a `volumeSnapshot` is created, 
the controller will trigger a snapshot operation by creating a `volumeSnapshotConent` object, which will be 
detected by the _CSI external-snapshotter sidecar_.

MicroShift will embed the controller's manifests and deploy them during startup.  They are: 

1. CSI VolumeSnapshot Controller Deployment
2. CSI VolumeSnapshot Controller Service Account
3. CSI VolumeSnapshot Controller ClusterRole and ClusterRoleBinding
4. CSI VolumeSnapshot Controller Role and RoleBinding

### CSI-External

The CSI Snapshot Validation Webhook is maintained SIG-Storage upstream. Downstream OCP releases of this container 
image will be used by MicroShift. The container image is specified by LVMS and is included in the _topolvm-controller_ 
deployment. It watches for `volumeSnapshotContent` and `volumeSnapshotClass` events.  When an event is detected, it 
makes the appropriate CSI gRPC call to LVMS via a shared unix socket.

### CSI-Validation-Webhook

The CSI Snapshot Validation Webhook is maintained SIG-Storage upstream. Downstream OCP releases of this container 
image will be used by MicroShift.  The webhook serves as gatekeeper to `volumeSnapshot` CREATE and UPDATE events. 
For the specifics of validation, _see_ 
[Kubernetes KEP](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/1900-volume-snapshot-validation-webhook#validating-scenarios).

1. Validating Webhook Deployment
2. Validating Webhook Service
3. ValidatingWebhookConfiguration
4. Validating Webhook ClusterRole and ClusterRoleBinding

### MicroShift Assets

MicroShift manages the deployment of embedded component assets through an interface called the Service Manager. The 
logic behind this interface deploys components in an order that respects interdependencies, waits for services to 
start, and intelligently stops services on interrupt.  MicroShift's default CSI storage service is already managed 
by the service manager. 

MicroShift's CSI service manager will deploy the CSI Volume Snapshot components.  The files will be stored under the
`microshift/assets/components/csi-external-snapshot-controller` directory.  The following files will be added: 

1. `csi_controller_deployment.yaml` specifies the CSI Snapshot Controller
2. `serviceaccount.yaml` specifies the controller's service account
3. `05_operand_rbac.yaml` provides RBAC configuration to the CSI controller 
4. `webhook_config.yaml`
5. `webhook_deployment.yaml`
6. `volumesnapshotclasses_snapshot.storage.k8s.io.yaml` defines the VolumeSnapshotClass CRD 
7. `volumesnapshotcontents_snapshot.storage.k8s.io.yaml` defines the VolumeSnapshotContents CRD
8. `volumesnapshots_snapshot.storage.k8s.io.yaml` defines the VolumeSnapshot CRD

### Greenboot Changes

MicroShift's greenboot scripts will be changed to include checks for the CSI Snapshot Controller and 
WebhookValidation Pod.

### Deployment

CSI Snapshot controller manifests will be integrated into the CSI service manager. The controller manifests will be 
deployed as part of MicroShift's LVMS startup process.  Because LVMS does not depend on the controller to be 
running prior to its deployment, it is not necessary to wait for the controller to reach a ready state before 
starting LVMS.

### Test Plan

Smoke tests will be written and run as part of pre-submit tasks on MicroShift CI.  At a minimum, the tests should
verify that a VolumeSnapshot created from an existing PVC containing data:

1. results in the snapshotting of the underlying volume
2. is addressable and accessible via a consumer PVC
3. enables rollback of data on the original volume

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

**Images**

The CSI Snapshot controller and sidecar are pieces of the Kubernetes CSI implementation.  OpenShift
maintains downstream versions of these images and tracks them as part of OCP releases.

**Manifests**

MicroShift's rebase automation specifies repos in the OpenShift github organization from which specific manifests 
are obtained. For the CSI Snapshot Controller, we will specify 
https://github.com/openshift/cluster-csi-snapshot-controller-operator/tree/release-$RELEASE/assets 
as the remote source from which to derive the controller manifests and image references.

### Version Skew Strategy

The CSI Snapshot version is tracked by the ocp-release image.  This provides sufficient guardrails
against version skew. The images are versioned in step with the `volumeSnapshot` APIs.  Thus, we should always use 
the image references from the same source as the CRD manifests to ensure compatibility.

### Operational Aspects of API Extensions

#### Failure Modes

- _Not Enough Storage Capacity:_ If the volume group capacity has been reached, snapshotting can fail because the 
  storage driver refuses to create a snapshot.  This will be observable as the VolumeSnapshot being stuck in the 
  **Pending** phase.

- _Valification Failure_: Webhook validation checks for malformed VolumeSnapshots and will post errors to the 
  problem object as Events. 

_Troubleshooting_

To determine the cause of snapshot failures, use `oc describe` to examine VolumeSnapshot and VolumeSnapshotContent 
events. 

- **VolumeSnapshot:**  `$ oc describe volumesnapshot -n $NAMESPACE $NAME`
- **VolumeSnapshotContent:** `$ oc describe volumesnapshotconent $NAME`

Errors related to restoring a snapshot to a PVC will be captured in the PVC's events.

- **PersistentVolumeClaim:** `$ oc describe pvc -n $NAMESPACE $NAME`

If you need to delve deeper, the logs can be examined with the following commands:

- CSI Snapshot Controller: `$ oc logs -n kube-system csi-snapshot-controller-$SUFFIX`
- CSI External Snapshotter: `$ oc logs -n openshift-storage topolvm-controller-$SUFFIX csi-external-snapshotter`
- LVMS Controller: `$ oc logs -n openshift-storage topolvm-controller-$SUFFIX topolvm-controller`
- LVMS Node: `$ oc logs -n openshift-storage topolvm-node-$SUFFIX topolvm-node`
- LVMS LVM Daemon: `oc logs -n openshift-storage topolvm-node-$SUFFIX lvmd`

#### Support Procedures

## Implementation History

## Alternatives

MicroShift design does not support CSI snapshotting.