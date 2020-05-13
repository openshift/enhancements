---
title: mount-image-filesystem
authors:
  - "@jwforres"
reviewers:
  - TBD
  - "@alicedoe"
approvers:
  - TBD
  - "@oscardoe"
creation-date: 2020-05-11
last-updated: 2020-05-11
status: provisional|implementable(?)
see-also:
replaces:
superseded-by:
---

# Mount Image Filesystem

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

1. Should it be possible to swap out an image mount while a container is running like we do for ConfigMaps and Secrets when their data changes? Example, my large file changed and now I have a new image available and I want to hot swap that file. Unlike a ConfigMap or Secret who's reference doesn't change on the container spec, to make this possible for image mounts the container would have to now reference a new image pullspec.

## Summary

A running container in a Pod on OpenShift will be able to directly mount a read-only filesystem from another image.

## Motivation

There are many situations where it is beneficial to ship the main runtime image separately from a large binary file that will be used by the application at runtime. Putting this large binary inside another image makes it easy to use existing image pull/push semantics to move content around. This pattern is used frequently, but in order to make the content available to the runtime image it must be copied from an initContainer into the shared filesystem of the Pod. For very large files this creates a significant startup cost while copying. It also requires needlessly running the image containing the binary content for the sole purpose of moving the data.

This enhancement proposes allowing the image's filesystem to be directly mounted to the container of the main runtime image. This eliminates the need for file copying within the Pod and gives the runtime container's processes access to the data immediately.

### Goals

The goal of this proposal is to work through the details of a functional prototype within containers/storage, containers/libpod, and/or a CSI driver that can back the ephemeral CSI volume.

### Non-Goals

---

## Proposal

### User Stories [optional]

#### Story 1
As a user deploying an application on OpenShift I can specify the pullspec of an image as a read-only Volume mount for my Pod.

#### Story 2
As a user deploying an application on OpenShift I can specify the pullspec of an image as a read-only Volume mount for my Pod and restrict the mount to a path within the image's filesystem.

### Implementation Details/Notes/Constraints [optional]

For the CSI driver there is some previous work in this space that it may be possible to build on: https://github.com/kubernetes-csi/csi-driver-image-populator

The CSI driver must not pull images into the same image filesystem as the one the kubelet uses, otherwise the image will be garbage collected by the kubelet even though its filesytem is in use by a container.

### Risks and Mitigations

The current proposal depends on CSI inline volumes being enabled in OpenShift, which requires integration with SCCs before it can be enabled safely.

## Design Details

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

### Version Skew Strategy

TODO - any issues w.r.t CSI skew?

## Implementation History

---

## Drawbacks

The proposal requires a separate image filesystem to safely store the image content, otherwise the images may be garbage collected by the kubelet.

## Alternatives

1. Add a native `image` Volume type to the container spec. This is unlikely to be accepted in the storage SIG for Kubernetes since the general direction of storage is to move as much as possible out of tree and rely on CSI for new container storage types. However, the benefit to this alternative is having a natively defined part of the container spec that the kubelet could watch for, allowing the kubelet to be responsible for pulling the images to the node and to manage their garbage collection.

2. 