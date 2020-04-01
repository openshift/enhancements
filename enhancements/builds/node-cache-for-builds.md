---
title: node-cache-for-builds
authors:
  - "@adambkaplan"
reviewers:
  - "@nalind"
  - "@rhdan"
  - "@TomSweeneyRedHat"
  - "@bparees"
approvers:
  - "@bparees"
creation-date: 2019-02-18
last-updated: 2019-03-25
status: implementable
see-also:
replaces:
superseded-by:
---

# Node Cache for Builds

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Builds in OpenShift 4.x currently require every layer of the base image to be pulled from its source
registry every time the build is run. In OCP 3.x builds could take advantage of image layers that
were stored on the build's node. This proposal provides a means of partially restoring this
node-level cache by allowing build pods to avoid pulling layers that are present on the node. Unlike
3.x, however, layers created during the build process will not be directly cached on the node due to
buildah's isolation.

## Motivation

### Goals

1. Provide a means for builds to take advantage of image layers that are stored on the node.

### Non-Goals

1. Provide a means for builds to write their layers to a cache (node or shared).
2. Provide a means for builds to share an image layer cache within a namespace.
3. Provide a means for the internal registry to pre-populate an image layer cache within a namespace
   or across a cluster.
4. Add persistent volume support for builds.

## Proposal

Build pods will mount the image layer cache that exists on every OpenShift node into the build pod.
Buildah - running within the build pod - will use this layer cache to reduce the number of layers
that need to be pulled when the build commences.

### User Story

As a developer using OpenShift to build container images I want builds to use image layers cached on
the node So that builds don't have to pull all layers in their base images and builds run faster.

### Implementation Details/Notes/Constraints [optional]

1. The build controller will mount the node image cache located at `/var/lib/containers/storage`
   into the build pod as `readOnly`. The host path mount will use the `DirectoryOrCreate` type
   option to ensure that builds will not fail if the path on the host does not exist. The mount
   point should not conflict with buildah's own default storage directory. Ex:
   `/var/lib/shared/containers/storage`.
2. The openshift/builder image is modified to configure additional image stores for buildah (via
   `/etc/containers/storage.conf`).
3. When using the additional image store, buildah (and its supporting libraries) is configured to
   extract any layers found in the cache to its own read-write storage. If this requires new
   configuration to be created, any additional configuration options must be exposed via buildah's
   API as well as CLI (flags). [\[1\]](https://issues.redhat.com/browse/RUN-592)
4. The `forcePull` option will be added to the cluster-wide `BuildDefaults` and `BuildOverrides`.
   When `forcePull` is set on a build, buildah must pull the manfiest for all images from their
   upstream repository prior to checking the cache. However, if `forcePull` is used layers found in
   the cache may be used to improve build performance.

### Risks and Mitigations

#### Image layer pruned by kubelet GC

*Risk:* Kubelet removing image layer via GC process

*Mitigation:* Buildah (and its libraries) will be responsible for copying layers found in the
additional image store into its own working container storage within the running build container.
This ensures that any layers pruned from underneath the container during the build will not break
the running build. Since builds in 4.4 and earlier pull every layer into the build pod, the impact
of copying these layers should be with respect to ephemeral storage.

#### Builds can modify content in the node's container storage

*Risk:* Writing to the node's container storage is a security risk, can lead to the kubelet running
rogue images.

*Mitigation:* Node container storage will be mounted as read-only as additional read-only storage.

#### Unbound image cache growth

*Risk:* Cached images can grow to the point that nodes fail due to disk pressure.

*Mitigation:* Node cache is read-only and managed by the kubelet and CRI-O. Builds take advantage of
the overlay filesystem and write their layers to ephemeral storage within the image build container.
When the build completes, the container's ephemeral storage is recycled.

#### Bypass permission checks to pull an image

*Risk:* Builds can bypass permission checks that prevent a user from pulling an image.

*Mitigation:* Builds can specify `forcePull` via the `BuildConfig` spec. As a part of this
enhancement, we will restore the `forcePull` option for cluster-wide `BuildDefaults` and
`BuildOverrides`. This will allow cluster admins to require every build to pull the manifests for an
image from its upstream repository.

#### Containers/storage upgrades can make builds unusable

*Risk:* On upgrade, version skew between the `containers/storage` library used by cri-o and buildah
can cause builds to be unusable.

*Mitigation:* The image layer format in use by `containers/storage` is designed to be backwards
compatible between major versions. Podman and cri-o already share the same store on the node, and
these have their own cadence for updating their version of `containers/storage`. If in the event
data in the additional store is not usable, buildah should fall back to its default behavior of
pulling image content from the upstream registry.

#### Host path with image cache does not exist

*Risk:* The node where the build runs does not have a `/var/lib/containers/storage` cache.

*Mitigation:* In the unlikely event that the cri-o storage is not configured on the node (or is
placed in an alternate location), the `HostPath` mount will use the `DirectoryOrCreate` type to
ensure that such a directory always exists on the host. Buildah will fall back to its current
behavior if the additional storage directory is empty.

#### Build container does not have permission to read the image cache

*Risk:* Build container does not have permission to read the image cache on the node.

*Mitigation:* Builds already run as a privileged container, and thus are able to obtain more
privileges than the parent kubelet. If for any reason the node does not allow privileged containers
to read contents in the node image cache, buildah should be able to fall back to its current
behavior.

## Design Details

### Test Plan

1. Configure a single-node machine set with a unique label (ex: `build-test: true`)
2. Use `oc new-app` to create a sample NodeJS application.
3. Alter the generated `BuildConfig` and `DeploymentConfig` such that the build and node both run on
   the test node.
4. Wait for the deployment to be rolled out onto the test node.
5. Run `oc start-build` for the `BuildConfig` above. Verify the following:
   1. The build completes.
   2. Image layers are pulled from the node cache. Most, if not all, base layers should be cached.

### Graduation Criteria

This will be released directly to GA.

### Upgrade / Downgrade Strategy

The standard upgrade procedure will ensure that the build controller and builder image are rolled
out. Builds launched after upgrade will be able to take advantage of the cache feature.

On downgrade builds will resume their current behavior and will not have an image cache.

### Version Skew Strategy

The build controller uses leader election to ensure only one controller is creating new builds. Once
a build has been started, its pod configuration cannot change.

## Implementation History

2020-02-18: Initial draft
2020-03-25: Implementable

## Drawbacks

1. Mounting content from the node requires the build pod to continue running as privileged.
2. Cluster admins can only partially opt out by setting `forcePull` on the cluster `BuildOverrides`
   or `BuildDefaults`. When `forcePull` is set the build pod will still have accees to the node
   cache. However, in this case builds will connect to upstream repositories as a means of verifying
   that the build has permission to pull its images.
3. This proposal does not provide a means of writing build layers back to a cache. S2I builds may
   benefit the most if by circumstance the resulting image is deployed on the same node as the
   build. Docker strategy builds which utilize multistage Dockerfiles may benefit the least from
   this feature, since the layers used to assemble an application are often not present in the
   resulting deployed image.

## Alternatives

1. Use persistent volumes to cache layers for builds
   1. General build volume support is being considered as a separate enhancement.
   2. Automatic provisioning of block storage is not guaranteed in all environments.
   3. Write-atomicity and performance can vary significantly depending on the underlying persistent
      storage.
   4. Persistent volumes will need their own mechanism for garbage collection.
2. Mount the node container storage directly
   1. Instead of using additional stores, mount the node cache as buildah's container storage.
   2. This is very insecure, as the volume mount would need to be writeable.
   3. Layers read and written during the build process are at risk of being pruned by the CRI-O GC
      process, since CRI-O does not see these layers as being in use.
3. Use object storage to cache build layers globally
   1. Use RWX object storage, such as Amazon S3, to cache layers globally across a
      namespace/cluster.
   2. As with alternative 1, availability of such storage is not guaranteed in all environments.
   3. Similarly, write-atomicity and performance can vary significantly. S3 in particular only has
      eventual consistnecy guarantees with respect to writes and reads.
   4. Similarly, this shared cache risks unbound growth without its own GC mechanism.
