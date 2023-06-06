---
title: microshift-csi-snapshot-integration
authors:
  - copejon
reviewers:
  - dhellman
  - pmtk
  - eggfoobar
  - pacevedom
  - "jsafrane, storage team"
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
integrated into OpenShift with ongoing downstream support. 

### User Stories

As an edge device owner running stateful workloads on MicroShift, I want to create in-cluster snapshots of that state 
and to restore workloads to that state utilizing existing Kubernetes patterns.

### Goals

* Enable an in-cluster workflow for snapshotting and restoration of cluster workload data
* Follow the [MicroShift design principles](https://github.com/openshift/microshift/blob/main/docs/design.md)

### Non-Goals

* Enable backup/restore workflows external to MicroShift
* Provide a workflow for exporting data from a MicroShift cluster
* Enabling MicroShift deployment without CSI Snapshotter.  This falls under platform composability and is out of scope
* Describe MicroShift installation and configuration processes in detail
* Support snapshots of the root disk / partition. In the context of this document, only PersistentVolumeClaims and 
  PersistentVolumes can be snapshot.

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

1. The user installs the MicroShift RPMs and starts the MicroShift systemd service.
2. The Service-Manager initiates the `startCSISnapshotter` service.
3. The `startCSISnapshotter` service deploys the CSI VolumeSnapshot CSI Snapshot Controller and Validating Webhook, and a 
   default VolumeSnapshotClass.
   - The snapshot controller is not a startup dependency of LVMS; both can be started concurrently.
4. The CSI VolumeSnapshot Controller begins listening for VolumeSnapshot API events.
5. The user observes the CSI Snapshot Controller pod reaches the **Ready** state.
   - `$ oc get pod -n kube-system csi-snapshot-controller-$SUFFIX`
6. The user observes the `topolvm-controller` and `csi-snapshot-controller` pods reach the **Ready** state.
    - `$ oc get pod -n openshift-storage topolvm-controller-$SUFFIX`
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
* The user has deployed a Pod with an attached PVC backed by a LVM thin volume.

_Workflow_
 
1. The user stops the workload by deleting the consuming Pod, or if managed by a replication controller, scales the 
   replicas to 0.
2. The user creates a `volumeSnapshot` object with the following key-values:
   - `.spec.volumeSnapshotClassName` set to the `volumeSnapshotClass`'s name. If not provided, falls back to default 
     class.
   - `.spec.source.persistentVolumeClaimName` set to the source PVC's name.
3. The _snapshotting validation webhook_ intercepts the API `volumeSnapshot` API request.
4. If validation succeeds, the `volumeSnapshot` is persisted in etcd.
   - Else: the `volumeSnapshot` is rejected. (see [Failure Modes](#failure-modes)).
5. The _CSI external snapshotter_ sidecar, part of the _topolvm-controller_ deployment, detects the `volumeSnapshot` 
    "create" event. It generates a `volumeSnapshotContent` object and triggers the `CreateSnapshot` CSI gRPC process.
6. LVMS executes the `CreateSnapshot` gRPC process and creates a snapshot of the LVM thin volume.
7. LVMS creates a `logicalVolume.topolvm.io` CR and returns the snapshot volume's metadata to the _CSI external 
   snapshotter_.
8. The _CSI external-snapshotter_ updates the `volumeSnapshotContent`'s `status` field to indicate it is ready.
9. The _CSI snapshot controller_ detects the update to the `volumeSnapshotContent` status and binds the
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
Application deployers can predefine VolumeSnapshotClass manifests.  These can be written the manifests to 
/etc/microshift/manifests. This pattern is congruent with how a device owner would install custom StorageClasses. It 
is recommended that new custom StorageClasses be packaged with VolumeSnapshotClasses that reference them.

#### Upgrading

CSI components are upgraded by MicroShift-internal automation.  These scripts pull the CSI image digests and 
related manifests from the OpenShift LVMS Operator Bundle.  This helps to keep MicroShift's version of 
CSI compatible with the default storage provisioner (LVMS). MicroShift deploys CSI `v1`, with micro-versions being 
guaranteed backwards compatible within each major version.  Therefore, micro-version upgrades will not require user 
intervention to protect snapshots from deletion or orphaning.

Upgrading the CSI containers will introduce a small interrupt window to provisioning operations.  Therefore, 
to mitigate any risk or orphaning resources, it is recommended that snapshotting be allowed to complete before 
restarting MicroShift and initiating the upgrade.

#### Configuring

Cluster admins will use the VolumeSnapshotClass API to set dynamic configurations for snapshot creation. 
Driver-specific configuration is available through the ‘parameters’ sub-field, which is a string:string map and is 
defined by the particular storage provider.

#### Deploying Applications

### API Extensions

The APIs to be added are defined under the `snapshot.storage.k8s.io/v1` API group. They are:

1. VolumeSnapshot
2. VolumeSnapshotClass
3. VolumeSnapshotContent

For a detailed description of these APIs, see 
[OpenShift documentation](https://docs.openshift.com/container-platform/4.13/storage/container_storage_interface/persistent-storage-csi-snapshots.html#volume-snapshot-crds)

### Risks and Mitigations

- Increased overhead.  The CSI snapshot controller and sidecar increase the on-disk footprint by a
total of 360MB.  At idle, the components use a negligible amount of CPU (>1% CPU on a 2 core machine)
and roughly 18Mb.

### Drawbacks

## Design Details

### CSI Snapshot Controller

The CSI Snapshot Controller is maintained SIG-Storage upstream. Downstream OCP releases of this container 
image will be used by MicroShift.  The controller is a standalone component responsible for watching 
`volumeSnapshot`, `volumeSnapshotContent`, and `volumeSnapshotClass` objects.  When a `volumeSnapshot` is created, 
the controller will trigger a snapshot operation by creating a `volumeSnapshotConent` object, which will be 
detected by the _CSI external-snapshotter sidecar_.

### CSI External Snapshotter

The CSI External Snapshotter is maintained SIG-Storage upstream. Downstream OCP releases of this container 
image will be used by MicroShift. The container is deployed as a sidecar of the _topolvm-controller_. It 
watches for `volumeSnapshotContent` and `volumeSnapshotClass` events.  When an event is detected, it 
makes the appropriate CSI gRPC call to LVMS via a shared unix socket.

### CSI Validation Webhook

The CSI Snapshot Validation Webhook is maintained SIG-Storage upstream. Downstream OCP releases of this container 
image will be used by MicroShift.  The webhook serves as gatekeeper to `volumeSnapshot` CREATE and UPDATE events. 
For the specifics of validation, _see_ 
[Kubernetes KEP](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/1900-volume-snapshot-validation-webhook#validating-scenarios).

> For a deeper explanation of CSI snapshot controller and sidecar, refer to 
[OpenShift documentation](https://docs.openshift.com/container-platform/4.13/storage/container_storage_interface/persistent-storage-csi-snapshots.html#persistent-storage-csi-snapshots-controller-sidecar_persistent-storage-csi-snapshots).  
> Snapshot webhook validation is described in detail [here](https://github.com/kubernetes-csi/external-snapshotter#validating-webhook).

### Asset Management

MicroShift manages the deployment of embedded component assets through the [service-manager](https://github.com/openshift/microshift/blob/3e08567344fa040a10fb30e013182fa62248f403/pkg/servicemanager/type.go#L1)
interface. The logic behind this interface deploys components in an order that respects interdependencies, waits for
services to start, and intelligently stops services on interrupt.

Infrastructure controllers (service-ca, DNS, routes, CSI and CNI plugins) are managed by the 
[infrastructure service](https://github.com/openshift/microshift/blob/878630eac00ab3884072c8ca23c429d5fce4bb6d/pkg/components/controllers.go).
The CSI Snapshotter will be appended to the list of controllers that the infrastructure service manages.

The manifests will be stored under the `microshift/assets/components/csi-external-snapshot-controller` directory.  
The following files will be added: 

1. `csi_controller_deployment.yaml` specifies the CSI Snapshot Controller
2. `serviceaccount.yaml` specifies the controller's service account
3. `05_operand_rbac.yaml` provides RBAC configuration to the CSI controller 
4. `webhook_config.yaml`
5. `webhook_deployment.yaml`
6. `volumesnapshotclasses_snapshot.storage.k8s.io.yaml` defines the VolumeSnapshotClass CRD 
7. `volumesnapshotcontents_snapshot.storage.k8s.io.yaml` defines the VolumeSnapshotContents CRD
8. `volumesnapshots_snapshot.storage.k8s.io.yaml` defines the VolumeSnapshot CRD

### Vendored Packages

None of the snapshotter logic will run from the MicroShift binary, meaning there will be no additional vendored dependencies.

### Greenboot Changes

MicroShift's greenboot scripts will be changed to include checks for the CSI Snapshot Controller and 
WebhookValidation Pod.  Greenboot checks already exist to check the _topolvm-controller_, which includes the snapshot sidecar.

This will be implemented as the addition a the `kube-system` namespace to the list of checked namespaces in the
[microshift-running-checks.sh](https://github.com/openshift/microshift/blob/809c1b9182aa04a9d40ac101ed60cb0d7b6a9d09/packaging/greenboot/microshift-running-check.sh#L6). 

### Deployment

CSI Snapshot controller manifests will be integrated into the CSI service-manager. The controller manifests will be 
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

#### Support Procedures

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


## Implementation History

## Alternatives

MicroShift design does not support CSI snapshotting.