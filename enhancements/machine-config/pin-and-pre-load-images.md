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
last-updated: 2023-03-15
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

Provide an mechanism to pin and pre-load container images.

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
connection to an image registry server I want to pin and pre-load all the
images required for the upgrade in advance, so that when I decide to actually
perform the upgrade there will be no need to contact that slow and/or
unreliable registry server and the upgrade will successfully complete in a
predictable time.

#### Pre-load and pin application images

As the administrator of a cluster that has a low bandwidth and/or unreliable
connection to an image registry server I want to pin and pre-load the images
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
to request that a set of container images are pinned and pre-loaded:

    ```yaml
    apiVersion: machineconfiguration.openshift.io/v1alpha1
    kind: PinnedImageSet
    metadata:
      name: my-pinned-images
    spec:
      pinnedImages:
      - name: "quay.io/openshift-release-dev/ocp-release@sha256:..."
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

1. A new `Validation WebHook` will be added to ensure the validity of
`PinnedImageSetRefs` passed to `MachineConfigPoolSpec` and provide appropriate
errors before the update is applied.

1. The `MachineConfigDaemon` will ensure adequate storage, manage image prefetch
logic, `CRI-O` configuration updates and status reporting through the
`MachineConfigPool` using the new `pinned_image_manager`.

### API Extensions

A new `PinnedImageSet` custom resource definition will be added to the
`machineconfiguration.openshift.io` API group.

```golang
  type PinnedImageSetSpec struct {
    // [docs]
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=2000
    // +listType=map
    // +listMapKey=name
    PinnedImages []PinnedImageRef `json:"pinnedImages"`
  }

  type PinnedImageRef struct {
    // [docs]
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=447
    // +kubebuilder:validation:XValidation:rule=`self.matches('^([a-zA-Z0-9-]+\\.)+[a-zA-Z0-9-]+(:[0-9]{2,5})?/([a-zA-Z0-9-_]{0,61}/)?[a-zA-Z0-9-_.]*@sha256:[a-f0-9]{64}$')`,message="The OCI Image reference must be in the format host[:port][/namespace]/name@sha256:<digest> with a valid SHA256 digest"
    Name string `json:"name"`
  }
```

`MachineConfigPoolSpec` will add `PinnedImageSets` .

```golang
    // [docs]
	  // +openshift:enable:FeatureGate=PinnedImages
	  // +optional
	  // +listType=map
	  // +listMapKey=name
	  PinnedImageSets []PinnedImageSetRef `json:"pinnedImageSets,omitempty"`
     
    type PinnedImageSetRef struct {
        // [docs]
	      // +kubebuilder:validation:MinLength=1
	      // +kubebuilder:validation:MaxLength=253
	      // +kubebuilder:validation:Pattern=`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`
	      // +kubebuilder:validation:Required	
	      Name string `json:"name"`
    }
```

### Implementation Details/Notes/Constraints

Starting with version 4.14 of OpenShift CRI-O will have the capability to pin
certain images (see [this](https://github.com/cri-o/cri-o/pull/6862) pull
request for details). That capability will be used to pin all the images
required for the upgrade, so that they aren't garbage collected by kubelet and
CRI-O.

In addition when the CRI-O service is upgraded and restarted it removes all the
images. This used to be done by the `crio-wipe` service, but is now done
internally by CRI-O. It can be avoided setting the `version_file_persist`
configuration parameter to "", but that would affect all images, not just the
pinned ones. This behavior needs to be changed in CRI-O so that pinned images
aren't removed, regardless of the value of `version_file_persist`.

The changes to `pinned_images` `CRI-O` configuration observed in the
`PinnedImageSet` will be persisted to a static config file
`/etc/crio/crio.conf.d/50-pinned-images` directly by the `pinned_image_manager`.

```yaml
apiVersion: machineconfiguration.openshift.io/v1alpha1
kind: PinnedImageSet
metadata:
  name: my-pinned-images
spec:
  pinnedImages:
  - name: "quay.io/openshift-release-dev/ocp-release@sha256:..."
  - name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:..."
  - name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:..."
  ...
```

The content of the file will the `pinned_images` parameter containing the list
of images:

```toml
pinned_images=[
  "quay.io/openshift-release-dev/ocp-release@sha256:...",
  "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
  "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
  ...
]
```

The PinnedImageSet is closely linked with the `MachineConfigPool`, and each
Custom Resource (CR) can be associated with a pool at the
`MachineConfigPoolSpec` level. This design is advantageous because it enables
controllers or administrators to assign specific sets of images to different
pools. By default, a cluster has two types of pools: master and worker. However,
users can create [custom pools](https://github.com/openshift/machine-config-operator/blob/master/docs/custom-pools.md)
for more precise control.

This is specially convenient for pinning images for upgrades: there are many
images that are needed only by the control plane nodes and there is no need to
have them consuming disk space in worker nodes.

The _machine-config-daemon_ will grow a new `pinned_image_manager` utilizing
the same general flow as the existing `MAchineConfigDaemon` `certificate_writer`. This approach is not dependent on
defining the configuration as `MachineConfig`. The controller that will watch on
`PinnedImageSet` and `MachineConfigPool` resources. On sync the manager will
perform the following tasks.

The _machine-config-daemon_ will add the following metrics for admins to track/verify image prefetch progress.

- mcd_prefetch_image_pull_success_total: Total number of successful prefetched image pulls.

- mcd_prefetch_image_pull_failure_total: Total number of prefetch image pull failures.

The _machine-config-daemon_ will be enhanced with a new feature called
`pinned_image_manager`, which follows a similar process to the existing
`certificate_writer`. This new feature will operate independently of the
`MachineConfig` for setting configurations. It is advantageous to not use
`MachineConfig` because the configuration and image prefetching can happen
across the nodes in the pool in parallel. The new controller will be watching
both `PinnedImageSet` and `MachineConfigPool` resources. When synchronizing, the
`pinned_image_manager` will:

1. Begin by marking the node with an annotation to indicate the manager is
`Working` utilizing the `nodeWriter`, similar to the `MachineConfigDaemon` `update`
process. This approach helps to avoid the need for additional node annotations.
If an error occurs during this phase, it will be indicated by setting the status
of the `MachineConfigPool` to `Degraded=true`. The `pinned_image_manager` could
later provide more detailed statuses via `MachineConfigNode`.

```sh
NAME     CONFIG                UPDATED   UPDATING   DEGRADED   MACHINECOUNT   READYMACHINECOUNT   UPDATEDMACHINECOUNT   DEGRADEDMACHINECOUNT
master   rendered-master-...   False      True      False      3              3                   2                     0                     
```

1. Verify that there is enough disk space available in the machine to pull the
   images.

1. Use the CRI-O gRPC API to run the equivalent of `crictl pull` for each of the
images.

1. Create, modify or delete the CRI-O pinning configuration file.

1. Reload the CRI-O configuration, with the equivalent of `systemctl reload crio`.

In order to verify that there is enough disk space we will first check which of
the images in the `PinnedImageSet` have not yet been downloaded, using CRI-O
gRPC API. For those that haven't been download we will then fetch from the
registry server the size of the blobs of the layers (only the size, not the
actual data).

Calculating the exact total size of the layers once uncompressed and written to
disk is difficult without deep knowledge of the underplaying containers storage
library. Instead of that we will assume that the disk space required is twice
the size of the blobs of the layers. That is an heuristic that works well for
complete OpenShift releases: blobs take 16 GiB and when they are written to disk
they take 32 GiB.

If the calculate disk space exceeds the available disk space then the
`pinned_image_manager` will report the error as a `Degraded=true` status
for the `MachineConfigPool` with the error in the `message`.

Note that even with this check it will still be possible (but less likely) to
have failures to pull images due to disk space: the heuristic could be wrong,
and there may be other components pulling images or consuming disk space in
some other way. Those failures will be detected and reported in the status of
the `MachineConfigPool` when CRI-O fails to pull the image.

Once all the relevant images are successfully pinned and downloaded to the
matching nodes, the `pinned_image_manager` will signal the completion of the
process by invoking nodeWriter.SetDone(). This action notifies the
`MachineConfigPool` that the task has been completed and once all of the nodes
report via existing Node annotation machineconfiguration.openshift.io/state:
Done the resulting `MachineConfigPool` status will be `Updating=false` `Updated=true`.

Today the Updated status reflects only the rendered worker config. The message
will need to be slightly updated in the case where prefetch is a success.

```
  - lastTransitionTime: "2024-03-14T20:31:32Z"
    message: All nodes are updated with MachineConfig rendered-worker-be6914dc246ca0b222fc6b4e2181b6da and image prefetch complete
    reason: ""
    status: "True"
    type: Updated
```

**Note:** As the API is introduced as `v1alpha1` via `TechPreview` this will not
directly affect customer upgrades. The usage of `Node` annotations will not be a
`v1` goal for status reporting but this accommodation is intended to allow the
maturity of `MachineConfigNode` before it is implemented. The goal is to use
`MachineConfigNode` directly when it has graduated to `v1`.

### Risks and Mitigation

Pre-loading disk images can consume a large amount of disk space. For example,
pre-loading all the images required for an upgrade can consume more 32 GiB.
This is a risk because disk exhaustion can affect other workloads and the
control plane. There is already a mechanism to mitigate that: the kubelet
garbage collection. To ensure disk space issues are reported and that garbage
collection doesn't interfere we will introduced new mechanisms:

1. Disk space will be checked before trying to pre-load images, and if there are
issues they will be reported explicitly via the status of the
`MachineConfigPool` status setting the pool as `Degraded`. A `MachineConfigPool`
in `Degraded` state will block upgrades and this is by design. `PinnedImageSet`
is a dependency of a cluster upgrade.

1. There is a possibility that storage may become fully exhausted by unforeseen
factors while images are being downloaded. Therefore, the configuration for
`pinned_images` in CRI-O and the subsequent reloading of the CRI-O service are
executed as the final steps, and only after all images have been successfully
pulled. This approach ensures that, in an emergency situation where space needs
to be freed up, the kubelet can remove these images to clear storage space.

1. If disk space is exhausted while pulling the images, then the issue will be
reported via the status of the `Node` as well. Typically these issues
are reported as failures to pull images in the status of pods, but in this case
there are no pods pulling the images. CRI-O will still report these issues in
the log (if there is space for that), like for any other image pull.

1. If disk exhaustion happens after the images have been pulled, then we will
ensure that the kubelet garbage collector doesn't select these images. That
will be handled by the image pinning support in CRI-O: even if kubelet asks
CRI-O to delete a pinned image CRI-O will not delete it.

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

The feature will ideally be introduced as `Dev Preview` in OpenShift 4.X,
moved to `Tech Preview` in 4.X+1 and declared `GA` in 4.X+2.

#### Dev Preview -> Tech Preview

- Availability of the CI test.

- Obtain positive feedback from at least one customer.

#### Tech Preview -> GA

- User facing documentation created in
[https://github.com/openshift/openshift-docs](openshift-docs).

#### Removing a deprecated feature

Not applicable, no feature will be removed.

### Upgrade / Downgrade Strategy

Upgrades from versions that don't support the `PinnedImageSet` custom resource
don't require any special handling because the custom resource is optional:
there will be no such custom resource in the upgraded cluster.

Downgrades to versions that don't support the `PinnedImageSet` custom resource
don't require any changes. The existing pinned images will be ignored in the
downgraded cluster, and will eventually be garbage collected.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

#### Failure Modes

Image pulling may fail due to lack of disk space or other reasons. This will be
reported via the conditions in the `PinnedImageSet` custom resource. See the
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
