---
title: csi-snapshots
authors:
  - "@chuffman"
  - "@jsafrane"
reviewers:
  - "@gnufied”
  - "@bertinatto"
approvers:
  - "@..."
creation-date: 2019-09-05
last-updated: 2019-12-09
status: provisional
see-also:
replaces:
superseded-by:
---

# CSI Snapshots

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We want to include the upstream CSI Snapshot feature in OpenShift.

## Motivation

* This feature has been requested by both the OCS and CNV teams.
* Snapshotting is an incredibly useful function when it comes to system changes in development areas, as it allows rapid rollback to preview dev versions.
* Container development benefits from filesystems that provide snapshot functionality.

### Goals

* Enable snapshot feature by default and provide the same support as upstream 1.17 release.

* Release and maintain downstream csi-external-snapshotter, csi-external-provisioner sidecars images off of the upstream images based on Kubernetes 1.17 to allow Red Hat CSI drivers (such as OCS) to consume them.

### Non-Goals

## Upstream status
This feature has reached v1beta status upstream and is described in https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/20190709-csi-snapshot.md and https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-snapshot.md.

This feature poses new requirements on Kubernetes distributions such as OpenShift:
* Ship and install snapshot CRDs.
* Ship and run snapshot-controller in a cluster.
  * snapshot-controller is very similar to in-tree PV controller in its function, it watches VolumeSnapshots and provisions VolumeSnapshotContents for them. The snapshot controller is independent on storage backend and only one runs for all CSI drivers installed in a cluster.

Upstream has chosen [addon](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/volumesnapshots) for both. Upstream uses [StatefulSet for the snapshot-controller with single replica](https://github.com/kubernetes/kubernetes/blob/master/cluster/addons/volumesnapshots/volume-snapshot-controller/volume-snapshot-controller-deployment.yaml) for the snapshot-controller.

## Proposal

1. OCP ships a new operator for csi-snapshot-controller, say csi-snapshot-controller-operator.
  * This operator will be installed by CVO in all clusters.
    * We do not know in advance what CSI drivers will a cluster admin install and we want the cluster ready for snapshots.
  * This operator will create / update VolumeSnapshot, VolumeSnapshotContent and VolumeSnapshotClass CRDs.
    * Vanilla copy of [upstream CRDs](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/volumesnapshots/crd) is used.
  * This operator will create Deployment with csi-snapshot-controller
    * Using Deployment with 3 replicas + leader election instead of upstream StatefulSet - leader election is significantly faster to run a new leader when a node with the current leader gets unavailable.
    * Optionally, run the Deployment on masters using proper node selector + tolerations.
  * This operator will report its status and status of the operand (Deployment) via standard `ClusterOperator` object.
    * With RelatedObjects = the Deployment with the controller + `openshift-csi-snapshot-controller` namespace.
  * Everything (the operator + the operand) runs in namespace `openshift-csi-snapshot-controller`.
  * Requires new github repo, openshift/csi-snapshot-controller-operator.
  * Requires new image.

2. OCP ships new image csi-snapshot-controller. Its source code is already available in github.com/openshift/csi-external-snapshotter, we only need an image.
  * The component / image is called kubernetes-csi/snapshot-controller upstream, we add csi- prefix to repository and image names, as we do with other repos / images from github.com/kubernetes-csi

### API
While the operator does not need any special cluster-specific configuration, following API is provided in `github.com/openshift/api/operator/v1` to be a good OpenShift operator. Explicitly, the operator uses `status.observedGeneration` and `status.generations` to track changes in dependent objects.

Package name: `github.com/openshift/api/operator/v1`
API version: `operator.openshift.io/v1`
Kind: `CSISnapshot`

```go
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CSISnapshot provides a means to configure an operator to manage the CSI snapshots. `cluster` is the canonical name.
type CSISnapshot struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    // spec holds user settable values for configuration
    // +kubebuilder:validation:Required
    // +required
    Spec CSISnapshotSpec `json:"spec"`

    // status holds observed values from the cluster. They may not be overridden.
    // +optional
    Status CSISnapshotStatus `json:"status"`
}

// CSISnapshotSpec is the specification of the desired behavior of the CSISnapshot operator.
type CSISnapshotSpec struct {
    OperatorSpec `json:",inline"`
}

// CSISnapshotStatus defines the observed status of the CSISnapshot operator.
type CSISnapshotStatus struct {
    OperatorStatus `json:",inline"`
}
```

The CRD and its `cluster` instance is created by CVO from the csi-snapshot-controller-operator manifest.


### User Stories [optional]

#### Story 1

As OCP cluster admin, I want to deploy a 3rd party CSI driver that supports snapshots and use it right away, without any OCP (re)configuration.

This is possible since csi-snapshot-controller and all snapshot CRDs are created during installation of the cluster.

#### Story 2

As OCS developer, I want to user CSI external-snapshotter sidecar shipped and supported by Red Hat to run my CSI driver.

We (OpenShift storage team) are going to ship the external-snapshotter sidecar in the same way as other CSI sidecars (provisioner, attacher, node-driver-registrar, ...)

### Implementation Details/Notes/Constraints [optional]

This feature has already been implemented upstream. Therefore, this feature requires a Kubernetes rebase of 1.15 or later.

### Risks and Mitigations

* Kubernetes 1.17 rebase.
* Release of upstream external-snapshotter and snapshot-controller for Kubernetes 1.17. This has been done usually within 1 month after appropriate Kubernetes release.

## Design Details

This feature has already been implemented upstream. The upstream design was discussed under https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/20190709-csi-snapshot.md.

### csi-snapshot-controller-operator
The operator is fairly straightforward, it manages just one Deployment that runs csi-snapshot-controller (and related service account and RBAC role bindings).

* Upstream RBAC from [upstream](https://github.com/kubernetes/kubernetes/blob/master/cluster/addons/volumesnapshots/volume-snapshot-controller/rbac-volume-snapshot-controller.yaml)
* Upstream [StatefulSet](https://github.com/kubernetes/kubernetes/blob/master/cluster/addons/volumesnapshots/volume-snapshot-controller/volume-snapshot-controller-deployment.yaml)
  * We will use Deployment with leader election, it recovers faster from leader being on an unavailable node 
  * We may consider DaemonSet on masters, as the controller can read all PVs / PVCs / VolumeSnapshot / VolumeSnapshotContent objects in the cluster and write to most of them (everything except PVC).

### Test Plan

The csi-snapshot-controller-operator will have e2e test to ensure it does not break openshift/origin installation.

We want the e2e tests to run using CSI sidecars shipped as part of OCP. There is ongoing progress in the following links regarding testing OCP CSI sidecars:

* https://github.com/openshift/origin/pull/23560
* https://jira.coreos.com/browse/STOR-223
Tests for this sidecar are currently defined upstream at https://github.com/kubernetes-csi/external-snapshotter/tree/master/pkg/controller .

### Graduation Criteria

There is no dev-preview phase.

##### Tech Preview

* Unit and e2e tests implemented.
* Update snapshot CRDs to v1beta1 and enable VolumeSnapshotDataSource feature gate by default. The feature must also be at least v1beta1 upstream, and have a strong indication that it will be made GA in a reasonable time frame with a compatible API.
* csi-snapshot-controller-operator is installed in all clusters.
* csi-snapshot-controller-operator has e2e test(s).
* csi-external-snapshotter is available to CSI drivers shipped by Red Hat (OCS, Ember, ...).

##### Tech Preview -> GA

* Feature deployed in production and have gone through at least one K8s upgrade.
* Feature must be GA in Kubernetes.

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

Nothing special, just:

* csi-snapshot-controller-operator github repository.
* New csi-snapshot-controller-operator and csi-snapshot-controller images, as described above.
