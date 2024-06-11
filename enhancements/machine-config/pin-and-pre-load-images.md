---
title: pin-and-pre-load-images
authors:
- "@jhernand"
reviewers:
- "@avishayt"   # To ensure that this will be usable with the appliance.
- "@danielerez" # To ensure that this will be usable with the appliance.
- "@mrunalp"    # To ensure that this can be implemented with CRI-O and MCO.
- "@nmagnezi"   # To ensure that this will be usable with the appliance.
- "@oourfali"   # To ensure that this will be usable with the appliance.
approvers:
- "@sinnykumari"
- "@mrunalp"
api-approvers:
- "@sdodson"
- "@zaneb"
- "@deads2k"
- "@JoelSpeed"
creation-date: 2023-09-21
last-updated: 2023-09-21
tracking-link:
- https://issues.redhat.com/browse/RFE-4482
see-also:
- https://github.com/openshift/enhancements/pull/1432
- https://github.com/openshift/api/pull/1609
replaces: []
superseded-by: []
---

# Pin and pre-load images

## Summary

Provide a mechanism to pin and pre-load container images.

## Motivation

Slow and/or unreliable connections to the image registry servers interfere with
operations that require pulling images. For example, an upgrade may require
pulling more than one hundred images. Failures to pull those images cause
retries that interfere with the upgrade process and may eventually make it
fail. One way to improve that is to pull the images in advance, before they are
actually needed, and ensure that they aren't removed. Doing that provides a
more consistent upgrade time in those environments. That is important when
scheduling upgrades into maintenance windows, even if the upgrade might not
otherwise fail.

### User Stories

#### Pre-load and pin upgrade images

As the administrator of a cluster that has a low bandwidth and/or unreliable
connection to an image registry server, I want to pin and pre-load all the
images required for the upgrade in advance so that when I decide to actually
perform the upgrade there will be no need to contact that slow and/or
unreliable registry server and the upgrade will complete in a
predictable time.

#### Pre-load and pin application images

As the administrator of a cluster that has a low bandwidth and/or unreliable
connection to an image registry server, I want to pin and pre-load the images
required by my application in advance, so that when I decide to actually deploy
it there will be no need to contact that slow and/or unreliable registry server
and my application will successfully deploy in a predictable time.

### Goals

Provide a mechanism that cluster administrators can use to pin and pre-load
container images.

### Non-Goals

We wish to use the mechanism described in this enhancement to orchestrate
upgrades without a registry server. But that orchestration is not a goal
of this enhancement; it will be part of a separate enhancement based on
parts of https://github.com/openshift/enhancements/pull/1432.

## Proposal

### Workflow Description

1. The administrator of a cluster uses the new `PinnedImageSet` custom resource
to request that a set of container images be pinned and pre-loaded to a defined
_machine-config-pool_:

    ```yaml
    apiVersion: machineconfiguration.openshift.io/v1alpha1
    kind: PinnedImageSet
    metadata:
      name: my-pinned-images
    labels:
      machineconfiguration.openshift.io/role: "worker"
    spec:
      nodeSelector:
        matchLabels:
          node-role.kubernetes.io/control-plane: ""
      pinnedImages:
      - name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:..."
      - name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:..."
      ...
    ```

1. `MachineConfigPoolSpec` will add an optional field taking a reference to a `PinnedImageSet`.
  
  ```yaml
  apiVersion: machineconfiguration.openshift.io/v1
  kind: MachineConfigPool
  spec:
    pinnedImageSets:
    - name: "my-pinned-images"
  ```

1. The _machine-config-daemon_ will add a new controller `PinnedImageSetManager` to
ensure storage, manage image prefetch logic, `CRI-O` configuration updates and
status reporting through the _machine-config-pool_ and `MachineConfigNode`.

1. The _machine-config-controller_ will add a new controller
`PinnedImageSetController` which will watch for changes on the
_machine-config-pool_ and `PinnedImageSet` resources. On change based on the
labels defined the controller will update the
`MachineConfigPoolSpec.PinnedImageSets` field for the corresponding pool.

### API Extensions

A new `PinnedImageSet` custom resource definition will be added to the
`machineconfiguration.openshift.io` API group.

```go
  type PinnedImageSetSpec struct {
          // [docs]
          // +kubebuilder:validation:Required
          // +kubebuilder:validation:MinItems=1
          // +kubebuilder:validation:MaxItems=500
          // +listType=map
          // +listMapKey=name
	        PinnedImages []PinnedImageRef `json:"pinnedImages"`
  }

 type PinnedImageRef struct {
          // [docs]
          // +kubebuilder:validation:Required
          // +kubebuilder:validation:MinLength=1
          // +kubebuilder:validation:MaxLength=447
          // +kubebuilder:validation:XValidation:rule=`self.split('@').size() == 2 && self.split('@')[1].matches('^sha256:[a-f0-9]{64}$')`,message="the OCI Image reference must end with a valid '@sha256:<digest>' suffix, where '<digest>' is 64 characters long"
          // +kubebuilder:validation:XValidation:rule=`self.split('@')[0].matches('^([a-zA-Z0-9-]+\\.)+[a-zA-Z0-9-]+(:[0-9]{2,5})?/([a-zA-Z0-9-_]{0,61}/)?[a-zA-Z0-9-_.]*?$')`,message="the OCI Image name should follow the host[:port][/namespace]/name format, resembling a valid URL without the scheme"
          Name string `json:"name"`
}
```

`MachineConfigPoolSpec` will add a list of `PinnedImageSets` .

```go
        // [docs]
        // +openshift:enable:FeatureGate=PinnedImages
        // +optional
        // +listType=map
        // +listMapKey=name
        // +kubebuilder:validation:MaxItems=100
        PinnedImageSets []PinnedImageSetRef `json:"pinnedImageSets,omitempty"`
     
    type PinnedImageSetRef struct {
        // [docs]
        // +openshift:enable:FeatureGate=PinnedImages
        // +kubebuilder:validation:MinLength=1
        // +kubebuilder:validation:MaxLength=253
        // +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`
        // +kubebuilder:validation:Required
	      Name string `json:"name"`
    }
```

`MachineConfigPoolStatus` will add `PoolSynchronizersStatus` which provides the
_machine-config-pool_ a mechanism to aggregate node-level configurations deployed
at the pool-level which are not `MachineConfig` based.

```go
        // [docs] 
        // +openshift:enable:FeatureGate=PinnedImages
        // +listType=map
        // +listMapKey=poolSynchronizerType
        // +optional
	      PoolSynchronizersStatus []PoolSynchronizerStatus `json:"poolSynchronizersStatus,omitempty"`

        // +kubebuilder:validation:XValidation:rule="self.machineCount >= self.updatedMachineCount", message="machineCount must be greater than or equal to updatedMachineCount"
        // +kubebuilder:validation:XValidation:rule="self.machineCount >= self.availableMachineCount", message="machineCount must be greater than or equal to availableMachineCount"
        // +kubebuilder:validation:XValidation:rule="self.machineCount >= self.unavailableMachineCount", message="machineCount must be greater than or equal to unavailableMachineCount"
        // +kubebuilder:validation:XValidation:rule="self.machineCount >= self.readyMachineCount", message="machineCount must be greater than or equal to readyMachineCount"
        // +kubebuilder:validation:XValidation:rule="self.availableMachineCount >= self.readyMachineCount", message="availableMachineCount must be greater than or equal to readyMachineCount"
        type PoolSynchronizerStatus struct {
                // poolSynchronizerType describes the type of the pool synchronizer.]
                // +kubebuilder:validation:Required
                PoolSynchronizerType PoolSynchronizerType `json:"poolSynchronizerType"`
                // machineCount is the number of machines that are managed by the node synchronizer.
                // +kubebuilder:validation:Required
                // +kubebuilder:validation:Minimum=0
                MachineCount int64 `json:"machineCount"`
                // updatedMachineCount is the number of machines that have been updated by the node synchronizer.
                // +kubebuilder:validation:Required
                // +kubebuilder:validation:Minimum=0
                UpdatedMachineCount int64 `json:"updatedMachineCount"`
                // readyMachineCount is the number of machines managed by the node synchronizer that are in a ready state.
                // +kubebuilder:validation:Required
                // +kubebuilder:validation:Minimum=0
                ReadyMachineCount int64 `json:"readyMachineCount"`
                // availableMachineCount is the number of machines managed by the node synchronizer which are available.
                // +kubebuilder:validation:Required
                // +kubebuilder:validation:Minimum=0
                AvailableMachineCount int64 `json:"availableMachineCount"`
                // unavailableMachineCount is the number of machines managed by the node synchronizer but are unavailable.
                // +kubebuilder:validation:Required
                // +kubebuilder:validation:Minimum=0
                UnavailableMachineCount int64 `json:"unavailableMachineCount"`
                // +kubebuilder:validation:XValidation:rule="self >= oldSelf || (self == 0 && oldSelf > 0)", message="observedGeneration must not move backwards except to zero"
                // observedGeneration is the last generation change that has been applied.
                // +kubebuilder:validation:Minimum=0
                // +optional
                ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
```

`MachineConfigNodeStatus` will add  `PinnedImageSets` allowing for detailed
reporting on the node-level status of each `PinnedImageSet` being reconciled for
this `Node`.

```go 
      type MachineConfigNodeStatus struct {
        [...]
        	// +listType=map
	        // +listMapKey=name
	        // +kubebuilder:validation:MaxItems=100
         	// +optional
	        PinnedImageSets []MachineConfigNodeStatusPinnedImageSet `json:"pinnedImageSets,omitempty"`
      }

      // +kubebuilder:validation:XValidation:rule="has(self.desiredGeneration) && has(self.currentGeneration) ? self.desiredGeneration >= self.currentGeneration : true",message="desired generation must be greater than or equal to the current generation"
      // +kubebuilder:validation:XValidation:rule="has(self.lastFailedGeneration) && has(self.desiredGeneration) ? self.desiredGeneration >= self.lastFailedGeneration : true",message="desired generation must be greater than last failed generation"
      // +kubebuilder:validation:XValidation:rule="has(self.lastFailedGeneration) ? has(self.desiredGeneration): true",message="desired generation must be defined if last failed generation is defined"
      type MachineConfigNodeStatusPinnedImageSet struct {
          // name is the name of the pinned image set.
          // Must be a lowercase RFC-1123 hostname (https://tools.ietf.org/html/rfc1123)
          // It may consist of only alphanumeric characters, hyphens (-) and periods (.)
          // and must be at most 253 characters in length.
          // +kubebuilder:validation:MaxLength:=253
          // +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`
          // +kubebuilder:validation:Required
          Name string `json:"name"`
          // currentGeneration is the generation of the pinned image set that has most recently been successfully pulled and pinned on this node.
          // +optional
          CurrentGeneration int32 `json:"currentGeneration,omitempty"`
          // desiredGeneration version is the generation of the pinned image set that is targeted to be pulled and pinned on this node.
          // +kubebuilder:validation:Minimum=0
          // +optional
          DesiredGeneration int32 `json:"desiredGeneration,omitempty"`
          // lastFailedGeneration is the generation of the most recent pinned image set that failed to be pulled and pinned on this node.
          // +kubebuilder:validation:Minimum=0
          // +optional
          LastFailedGeneration int32 `json:"lastFailedGeneration,omitempty"`
          // lastFailedGenerationErrors is a list of errors why the lastFailed generation failed to be pulled and pinned.
          // +kubebuilder:validation:MaxItems=10
          // +optional
          LastFailedGenerationErrors []string `json:"lastFailedGenerationErrors,omitempty"`
      }
```

The `MachineConfigNode` enum `StateProgress` will add two enumerators.

```go
const (
      // MachineConfigNodePinnedImageSetsProgressing describes a machine currently progressing to the desired pinned image sets
      MachineConfigNodePinnedImageSetsProgressing StateProgress = "PinnedImageSetsProgressing"
      // MachineConfigNodePinnedImageSetsDegraded describes a machine that has failed to progress to the desired pinned image sets
      MachineConfigNodePinnedImageSetsDegraded StateProgress = "PinnedImageSetsDegraded"
)
```


### Implementation Details/Notes/Constraints

Starting with version 4.16 of OpenShift `CRI-O` will have the capability to pin
certain images (see [this](https://github.com/cri-o/cri-o/pull/6862) pull
request for details). That capability will be used to pin all the images
required for the upgrade, so that they aren't garbage collected by kubelet and
CRI-O.

The changes to `pinned_images` `CRI-O` configuration observed in the
`PinnedImageSet` will be persisted to a static config file
`/etc/crio/crio.conf.d/50-pinned-images` by the `PinnedImageSetManager`.

The content of the `CRI-O` config file will define the `pinned_images` key containing the list
of images to pin:

```toml
pinned_images=[
  "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
  "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
  ...
]
```

The `PinnedImageSet` resource is closely linked with the _machine-config-pool_, and each
Custom Resource (CR) is automatically associated with a pool at the
`MachineConfigPoolSpec` level. This design is advantageous because it enables
controllers or administrators to assign specific sets of images to different
pools. By default, a cluster has two types of pools: master and worker. However,
users can create [custom pools](https://github.com/openshift/machine-config-operator/blob/master/docs/custom-pools.md)
for more precise control.

Having this pool-level granularity is convenient for upgrades as there are
images that are needed only by the worker nodes and which would not be present
on a master node and vice-versa.

The _machine-config-daemon_ will be enhanced with a new feature called
`PinnedImageSetManager`, which follows a similar logical flow to the existing
`certificate_writer`. This new feature will obtain its configuration directly
from the PinnedImageSet custom resource independent of `MachineConfig`. It is
advantageous to not use `MachineConfig` in this use case because the
configuration and image prefetching can happen across all the nodes in the pool
in parallel instead of a rolling update. The new controller will be watching
both `PinnedImageSet` and _machine-config-pool_ and `Node` resources. When
synchronizing, the `PinnedImageSetManager` will:

1. Create a `Context` with a timeout of 2 minutes. This ensures that the
controller provides timely feedback on progress status. The controller will
perform the below tasks on each sync and after the 2-minute timeout, it will
drain the `PinnedImageSet` worker pool and requeue. Caching plays an important part
in this flow to ensure that work done is not repeated.

1. Calculate a list of all the `MachineConfigPools` that the `Node` is a member of and loop
through each. Then on the pool-level create a list of `PinnedImageSets`
defined in the `MachineConfigPoolSpec` and begin to loop through each
`PinnedImageSet` in serial.

1. Ensure that the `Node` is `Ready=true` and `NodeDiskPressure=false`.

1. Check that there is enough disk space available on the `Node` to pull the
images. This is done by calculating the size of the image blobs using the
`podman manifest inspect` command and caching the results.

1. Obtain auth for each image in the set and cache it.

1. Schedule the images in the `PinnedImageSet` to the worker pool which handles pulling each image.

1. The worker will use the `CRI` client to ensure the image does not exist in
the local container storage. If this is true we attempt to pull the image. If
any image in the set is not able to be pulled an error will be returned. These
errors will be logged and added to the message of  `PinnedImageSetsDegraded`
condition of `MachineConfigNodeStatus`. It is the job of the admin to review the
_machine-config-daemon_ logs to understand the reason the image can not be
pulled and resolve the failure. Reasons can include but are not limited to
invalid auth, networking, image no longer exists in the registry, and typos.

1. Once all images are pulled we create a unique list of all images in all the sets.

1. Ensure that all the images still exist and nothing was removed during the
reconciliation process and the `Node` is `Ready=true` and
`NodeDiskPressure=false`. 

1. Write the `CRI-O` config to disk if no images exist remove the config.

1. Reload the `CRI-O` configuration, with the equivalent of `systemctl reload crio`.

1. Update `MachineConfigPoolStatus` and `MachineConfigNodeStatus` to reflect success.

#### Notes

Calculating the exact total size of the layers once uncompressed and written to
disk is difficult without deep knowledge of the underlying container storage
library. Instead of that we will assume that the disk space required is twice
the size of the blobs of the layers. That is an heuristic that works well for
complete OpenShift releases: blobs take 16 GiB and when they are written to disk
they take 32 GiB.

If at anytime in the image pull process, the image pull workers return an error
equivalent to syscall.ENOSPC or "no space on device" the error will result in
`Degraded=true` status condition for the _machine-config-pool_ with the error in
the `Message`. This is the only time that this controller will set the
`MachineConfigPoolStatus` Conditions to `Degraded=true`.

Note that even with this check it will still be possible (but less likely) to
have failures due to disk space: the heuristic could be wrong,
and there may be other components pulling images or consuming disk space in
some other way. Those failures will be detected and reported in the status of
the _machine-config-pool_ when CRI-O fails to pull the image.

##### Status Reporting Details

A critical component for `PinnedImageSets` is to provide node-level reporting on
the progress of the `PinnedImageSetManager` reconciling `PinnedImageSets`. To
achieve this the implementation relies on reporting node-level status to
`MachineConfigNode`. When a `PinnedImageSet` is synced we will update
`MachineConfigNodePinnedImageSetsProgressing=true`. Any error returned during
reconciliation will result in `MachineConfigNodePinnedImageSetsDegraded=true`
and reflect accordingly in the counts for
`MachineConfigNodeStatusPinnedImageSet` on the node-level and
`PoolSynchronizersStatus` on the `MachineConfigPool` level. When a
PinnedImageSet is reconciled `MachineConfigNodePinnedImageSetsProgressing=false`
is set and `PoolSynchronizerStatus.MachineCount` will =
`PoolSynchronizerStatus.UpdatedMachineCount` and the `Generation` of the
`PinnedImageSet` should show in the MachineConfigPoolStatus as
`MachineConfigNodeStatusPinnedImageSet.CurrentGeneration` =
`MachineConfigNodeStatusPinnedImageSet.DesiredGeneration`.

```yaml
status:
  pinnedImages:
  - quay.io/openshift-release-dev/ocp-release@sha256:...
  - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  conditions:
  - type: Ready
    status: "False"
  - type: Failed
    status: "True"
    message: Node 'node12' failed to pull image `quay.io/...` because ...
```

We will consider using the new _machine-config-operator_ state reporting
mechanism introduced [https://github.com/openshift/api/pull/1596](here) to
report additional details about the progress or issues of each node.

Additional information may be needed inside the `status` field of the
`PinnedImageSet` custom resources in order to implement the mechanisms described
above. For example, it may be necessary to have conditions per node, something
like this:

```yaml
status:
  node0:
  - type: Ready
    status: "False"
  - type: Failed
    status: "True"
  node1:
  - type: Ready
    status: "True"
  - type: Failed
    status: "True"
  ...
```

Those details are explicitly left out of this document, because they are mostly
implementation details, and not relevant for the user of the API.

### Risks and Mitigations

Pre-loading disk images can consume a large amount of disk space. For example,
pre-loading all the images required for an upgrade can consume more 32 GiB.
This is a risk because disk exhaustion can affect other workloads and the
control plane. There is already a mechanism to mitigate that: the kubelet
garbage collection. To ensure disk space issues are reported and that garbage
collection doesn't interfere we will introduce new mechanisms:

1. There is a possibility that storage may become fully exhausted by unforeseen
factors while images are being downloaded. Therefore, the configuration for
`pinned_images` in `CRI-O` and the subsequent reloading of the `CRI-O` service are
executed as the final steps, and only after all images have been successfully
pulled. This approach ensures that, in an emergency situation where space needs
to be freed up, the kubelet can remove these images to clear storage space.

1. If disk space is exhausted while pulling the images, then the issue will be
reported via the `PinnedImageSetManager` by setting the _machine-config-pool_ to
`Degraded=true`. Typically these issues are reported as failures to pull images
in the status of pods, but in this case there are no pods pulling the images.
`CRI-O` will still report these issues in the log (if there is space for that),
like for any other image pull.

1. If disk exhaustion happens after the images have been pulled, then we will
ensure that the kubelet garbage collector doesn't select these images. That
will be handled by the image pinning support in `CRI-O`: even if kubelet asks
`CRI-O` to delete a pinned image `CRI-O` will not delete it.

1. The recovery steps in the documentation will be amended to ensure that these
images aren't deleted to recover disk space.

### Drawbacks

This approach requires non trivial changes to the machine config operator.

## Design Details

### Open Questions

None.

### Test Plan

We add a CI test that verifies that images are correctly pinned and pre-loaded.

### Graduation Criteria

The feature will ideally be introduced as `Tech Preview` in OpenShift 4.16 and declared `GA` in TBD.


#### Tech Preview -> GA

- User facing documentation created in
[https://github.com/openshift/openshift-docs](openshift-docs).

- MachineConfigNode is promoted to v1.

- PinnedImageSet is promoted to v1.

#### Removing a deprecated feature

Not applicable, no feature will be removed.

### Upgrade / Downgrade Strategy

Upgrades from versions that don't support the `PinnedImageSet` custom resource
don't require any special handling because the custom resource is optional:
there will be no such custom resource in the upgraded cluster.

Downgrades to versions that don't support the `PinnedImageSet` custom resource
don't require any changes. The existing pinned images will need to be potentially
manually unpinned.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

#### Failure Modes

Image pulling may fail due to lack of disk space or other reasons. This will be
reported via the conditions in the `MachineConfigNode` custom resource. See the
risks and mitigation section for details.

#### Support Procedures

Nothing.

## Implementation History

There is an initial prototype exploring some of the implementation details
described here in this [https://github.com/jhernand/upgrade-tool](repository).

## Alternatives

The alternative to this is to manually pull the images in all the nodes of the
cluster, manually create the `/etc/crio/crio.conf.d/pin.conf` file and manually
reload the CRI-O service.

## Infrastructure Needed

Infrastructure will be needed to run the CI test described in the test plan
above.
