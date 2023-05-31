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
last-updated: 2023-05-30
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

* A device owner has created an LVM volume group and a thin-pool on this volume group.
* The `/etc/microshift/lvmd.yaml` config includes a `deviceClass` to represent the thin-pool.  The lvmd.yaml may 
contain a mix of thin and thick `deviceClasses`.

```
device-classes:
- name: ssd-thin
  volume-group: myvg1
  type: thin
  thin-pool:
    name: pool0
    overprovision-ratio: 50.0
```

_Workflow_

1. Install MicroShift RPMs
2. Start the MicroShift systemd service
3. Observe the CSI Snapshot controller pod reaches the Ready state
4. Observe the topolvm-controller and csi-snapshot-controller pods reach the Ready state

#### Create a Snapshot Dynamically

This section describes how to create snapshot directly from a PersistentVolumeClaim object.

_Prerequisites_

* A running MicroShift cluster
* A PVC created using a CSI driver that supports VolumeSnapshot objects.
* A storage class to provision the storage back end.
* No pods are using the persistent volume claim (PVC) that you want to take a snapshot of.

> Do not create a volume snapshot of a PVC if a pod is using it. Doing so might cause data corruption because the PVC 
> is not quiesced (paused). Be sure to first tear down a running pod to ensure consistent snapshots.

_Workflow_

1. Create a VolumeSnapshotClass object.  **NOTE:** MicroShift deploys a default VolumeSnapshotClass to enable LVMS 
   snapshotting out of the box.  This step is only necessary for creating novel VolumeSnapshotClasses. 

    ```yaml
    apiVersion: snapshot.storage.k8s.io/v1
    kind: VolumeSnapshotClass
    metadata:
      name: topolvm-snap
    driver: topolvm.io 
    deletionPolicy: Delete
    ```

   
2. Create the object you saved in the previous step by entering the following command:
    
    `$ oc create -f volumesnapshotclass.yaml`


3. Create a VolumeSnapshot object:
   
   **_volumesnapshot-dynamic.yaml_**
   ```yaml
   apiVersion: snapshot.storage.k8s.io/v1
   kind: VolumeSnapshot
   metadata:
     name: mysnap
   spec:
     volumeSnapshotClassName: topolvm-snap
     source:
       persistentVolumeClaimName: myclaim
   ```

   - **volumeSnapshotClassName:** The request for a particular class by the volume snapshot. If the 
     volumeSnapshotClassName setting is absent and there is a default volume snapshot class, a snapshot is created with 
     the default volume snapshot class name. But if the field is absent and no default volume snapshot class exists, 
     then no snapshot is created.

   - **persistentVolumeClaimName:** The name of the PersistentVolumeClaim object bound to a persistent volume. This 
     defines what you want to create a snapshot of. Required for dynamically provisioning a snapshot.


2. Create the object you saved in the previous step by entering the following command:

   `$ oc create -f volumesnapshot-dynamic.yaml`

#### Create a Volume Snapshot Manually

This section describes how to create snapshot directly from a VolumeSnapshotContent object.

1. Provide a value for the **volumeSnapshotContentName** parameter as the source for the snapshot, in addition to 
   defining volume snapshot class as shown above.

   **_volumesnapshot-manual.yaml_**
   ```yaml
   apiVersion: snapshot.storage.k8s.io/v1
   kind: VolumeSnapshot
   metadata:
     name: mysnap
   spec:
     source:
       volumeSnapshotContentName: mycontent 
   ```

   **volumeSnapshotContentName:** The volumeSnapshotContentName parameter is required for pre-provisioned snapshots.

2. Create the object you saved in the previous step by entering the following command:

    `$ oc create -f volumesnapshot-manual.yaml`

#### Verify a Snapshot was Created

After the snapshot has been created in the cluster, additional details about the snapshot are available.

1. To display details about the volume snapshot that was created, enter the following command:

   `$ oc describe volumesnapshot mysnap` 

    _Example Output:_
    ```yaml
    apiVersion: snapshot.storage.k8s.io/v1
    kind: VolumeSnapshot
    metadata:
      name: mysnap
    spec:
      source:
        persistentVolumeClaimName: myclaim
      volumeSnapshotClassName: csi-hostpath-snap
    status:
      boundVolumeSnapshotContentName: snapcontent-1af4989e-a365-4286-96f8-d5dcd65d78d6 
      creationTime: "2020-01-29T12:24:30Z" 
      readyToUse: true 
      restoreSize: 1Gi
      ```

#### Deleting a Volume Snapshot

You can configure how OpenShift Container Platform deletes volume snapshots.

_Workflow_

1. Specify the deletion policy that you require in the VolumeSnapshotClass object, as shown in the following example:

    ```yaml
    apiVersion: snapshot.storage.k8s.io/v1
    kind: VolumeSnapshotClass
    metadata:
      name: 
    driver: topolvm.io
    deletionPolicy: Delete 
    ```
   **deletionPolicy:** When deleting the volume snapshot, if the Delete value is set, the underlying snapshot is 
   deleted along with the VolumeSnapshotContent object. If the Retain value is set, both the underlying snapshot and 
   VolumeSnapshotContent object remain.
   If the Retain value is set and the VolumeSnapshot object is deleted without deleting the corresponding 
   VolumeSnapshotContent object, the content remains. The snapshot itself is also retained in the storage back end.


2. Delete the volume snapshot by entering the following command:

    `$ oc delete volumesnapshot <volumesnapshot_name>`
    
    _Example Output:_

    `volumesnapshot.snapshot.storage.k8s.io "mysnapshot" deleted`


3. If the deletion policy is set to Retain, delete the volume snapshot content by entering the following command:

    `$ oc delete volumesnapshotcontent <volumesnapshotcontent_name>`


4. _Optional:_ If the VolumeSnapshot object is not successfully deleted, enter the following command to remove any finalizers for the leftover resource so that the delete operation can continue:
    
    `$ oc patch -n $PROJECT volumesnapshot/$NAME --type=merge -p '{"metadata": {"finalizers":null}}'`
    
    _Example Output:_

    `volumesnapshotclass.snapshot.storage.k8s.io "csi-ocs-rbd-snapclass" deleted`


#### Restore

The VolumeSnapshot CRD content can be used to restore the existing volume to a previous state.

After your VolumeSnapshot CRD is bound and the readyToUse value is set to true, you can use that resource to 
provision a new volume that is pre-populated with data from the snapshot.

_Prerequisites_

* Logged in to a running OpenShift Container Platform cluster. 
* A persistent volume claim (PVC) created using a Container Storage Interface (CSI) driver that supports volume 
  snapshots. 
* A storage class to provision the storage back end. 
* A volumesnapshot has been created and is ready to use.

_Workflow_

1. Specify a VolumeSnapshot data source on a PVC as shown in the following:

    _pvc-restore.yaml_
    ```yaml
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: myclaim-restore
    spec:
      storageClassName: topolvm.io
      dataSource:
        name: mysnap 
        kind: VolumeSnapshot 
        apiGroup: snapshot.storage.k8s.io 
      accessModes:
        - ReadWriteOnce
      resources:
        requests:
          storage: 1Gi
    ```

   - **name**:  Name of the VolumeSnapshot object representing the snapshot to use as source.
   - **kind**:  Must be set to the VolumeSnapshot value.
   - **apiGroup**: Must be set to the snapshot.storage.k8s.io value.


2. Create a PVC by entering the following command:

    `$ oc create -f pvc-restore.yaml`


3. Verify that the restored PVC has been created by entering the following command:
    
    `$ oc get pvc`

   A new PVC such as myclaim-restore is displayed.


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

As mentioned above, CSI Volume Snapshot logic is divided between two runtimes: the CSI Volume Snapshot Controller 
and the CSI Volume Snapshot sidecar.  These runtimes are pre-packaged in container images and available through the
OpenShift container registry.  Additionally, a validating webhook verifies the correctness of CRD instances.

The total additional cluster components are:

1. CSI VolumeSnapshot Controller Deployment
2. CSI VolumeSnapshot Validating Webhook Deployment, responsible for validating CRD instances
3. CSI VolumeSnapshot Validating Webhook Service, to enable communication with kube-apiserver
4. CSI VolumeSnapshot Validating Webhook API, to registery webhook with kube-apiserver
5. CSI VolumeSnapshot Validating Webhook ClusterRole and ClusterRoleBinding
6. CSI VolumeSnapshot Controller Service Account
7. CSI VolumeSnapshot Controller ClusterRole and ClusterRoleBinding
8. CSI VolumeSnapshot Controller Role and RoleBinding
9. VolumeSnapshot CRD
10. VolumeSnapshotContent CRD
11. VolumeSnapshotClass CRD
12. CSI Volume Snapshot Sidecar container, included in the LVMS Controller deployment spec.

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

The controller will be deployed prior to LVMS.  Because LVMS does not depend on the controller to be running prior 
to its deployment, it is not necessary to wait for the controller to reach a ready state before starting LVMS.  
Starting these components concurrently will also shorten overall startup time.

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
as the remote source from which to derive the controller manifests.

LVMS does not provide its manifests as plain yaml files.  Instead, these are encoded into the logic of the operator, 
which is not deployed on MicroShift.  This makes retrieving them automatically difficult.  For this reason, the LVMS 
manifests are derived once from a running instance of the controller and stored under `microshift/assets/components/lvms/`.

### Version Skew Strategy

The CSI Snapshot version is tracked by the ocp-release image.  This provides sufficient guardrails
against version skew.

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives

There are no alternatives to deploying the Kubernetes CSI implementation of the snapshotting specification.