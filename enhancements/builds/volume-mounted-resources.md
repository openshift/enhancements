---
title: volume-mounted-resources
authors:
  - @bparees
  - @adambkaplan
reviewers:
  - @derekwaynecarr
  - @smarterclayton
  - @deads2k
approvers:
  - @bparees
  - @deads2k
  - @sttts
creation-date: 2020-01-08
last-updated: 2021-05-05
status: implementable
---

# Volume Mounted Resources

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today builds support getting source input from configmaps and secrets, hereafter referred to as "resources".
When users utilize this feature, the resource is volume-mounted into the build pod and placed in the "build context" within the build's execution environment, alongside other build sources like git source code.
The next steps depend on whether it is an s2i or dockerfile build.

For s2i builds, the generated Dockerfile contains commands to `ADD` the content at a path specified by the user, the assemble script is invoked, and then the injected content is zeroed out prior to committing the image via a `RUN rm` command added to the Dockerfile.

For dockerfile builds, the user is instructed to add appropriate `ADD` and `RUN rm` commands to their dockerfile to inject the content that is available in the build's working directory (along with their application source, where applicable).

There are a few undesirable aspects to this:

1. In the dockerfile case, the content can still be found in lower layers of the image unless a layer squashing option is selected.
2. Requires extra work by the user in the Dockerfile, so each Dockerfile must be customized

This enhancement proposes to introduce an option to use buildah's capability to mount a volume at build time.
The content mounted into the build pod would be then mounted into the container processing the Dockerfile, making that content available within the container so Dockerfile commands could reference it.
No explicit `ADD` would be required, and since mounted content is not committed to the resulting image, no `RUN rm` equivalent is required to clean up the injected content.

To avoid security and lifecycle concerns, the following volume types will be supported initially:

1. Secrets
2. ConfigMaps

## Motivation

### Goals

* Simplify how users consume secret + configmap content in builds
* Increase the security of protected content being injected to images
* Simplify use cases that require consuming credentials during the build, but need to ensure those credentials do not end up in the output image.
* Eventually extend this api to allow the mounting of other volumes (such as those backed by persistent storage)

### Non-Goals

* This enhancement should not result in a change of behavior for users of the existing secret/configmap injection api.
* Provide immediate support for persistent volume claims in builds.
  This is a long term goal that will be addressed in a future enhancement proposal.
* Provide support to mount Secrets and ConfigMaps shared by the [projected resource CSI driver](/enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md).
  This is a long term goal that will be addressed in a future enhancement proposal.

## Proposal

### User Stories [optional]

The enabled use cases are essentially identical to what can be done with the configmap/secret input api in builds today, but with a better user experience and security as discussed above.
It does not enable a new use case that is not already possible today, except that layer squashing will not be required.

Future extensions to this enhancement could enable additional use cases, such as:

- Persistent volumes that cache content
- Shared Secrets and ConfigMaps, such a Simple Content Access certificate used to download RHEL content.

These will be discussed in respective future enhancements.

### Definitions

The folllowing terms will be used in the remainder of the document to clarify the behavior of volume mounts in builds:

- *Volume content*: contents used within a build that are not indended to be present in the resulting container image.
- *Input Volume*: a Kubernetes Volume that is added to the build Pod, with the intent of being used as a transient bind mount within a buildah build.
  See buildah's `--volume` [option](https://github.com/containers/buildah/blob/master/docs/buildah-bud.md#options).
- *Buildah volume mount*: a directory that is added to Buildah's runtime environment as a transient bind mount.
- *Container volume mount*: a volume that is mounted into a container within a Kubernetes pod.
- *Buildah runtime environment*: the process that runs the buildah build within the build pod's main container.
  In OCP 4.6 and higher, buildah uses OCI isolation and effectively runs as a sub-container within the build pod's main container.

### Implementation Details/Notes/Constraints [optional]

We will need to introduce a new mechanism in the build api which allows the user to indicate that they want to inject *volume content* into the build.
Unlike source content, *volume content* is not intended to be included in the container image produced by an OpenShift build.
Initially the only allowed volume types will be ConfigMaps and Secrets.
The API will otherwise be similar to the existing secret/configmap injection api in which users identify the configmap/secret and the target path for injection.

#### Build/BuildConfig API

Volume content can be declared via a new `[]BuildVolume` field that is added to the Source and Docker strategy structs.
These will define the *input volumes* that are appended to other volumes in the build pod.
Items in the `[]BuildVolume` array will be translated directly into a pod `Volume`, using Kubernetes mechanisms to set up the volume for consumption by the build pod.

Unlike Kubernetes, the types of volume sources that can be defined will be restricted to those explictly supported by Builds.
Build pods are created with the privileged security context constraint, which allows a pod to mount any volume type with no SELinux restrictions.
This SCC is inherited from the build controller.
Restricting supported volume sources in the API ensures that builds only allow safe volume types to be mounted.

The subset of supported volume sources will be defined by the `BuildVolumeSource` object.
This will behave like the Kubernetes `VolumeSource` object, with the addition of a type discriminator that is set on Build/BuildConfig admission (if not specified).
The OpenAPI schema must ensure that one of the volume sources are set.
Initially only `Secret` and `ConfigMap` will be supported as valid Volume sources.
The volume sources themselves will use Kubernetes volume source types, as the values in these volume sources will be passed directly to the build pod.

The destination *buildah volume mount* will be controlled by the new `[]BuildVolumeMount` field, which is a subset of the Kubernetes `VolumeMount` API.
The options in `BuildVolumeMount` will be constrained to those allowed in the *buildah volume mount*:

- `MountPath` - the destination directory to place the *buildah volume mount* within buildah's runtime environment.
- `ReadOnly` - whether or not the volume mount within buildah's runtime environment is read-only.

Other options like `SubPath` and `MountPropagation` may be considered for support in the future.

When the build pod is constructed, all *input volumes* will be mounted in a subdirectory of a fixed location in the build pod's main container - `/var/run/openshift.io/volumes`.
The OpenShift builder process will infer the destination directory of these volumes inside the *buildah runtime environment* from the `Build` API object (already available via an environment variable, serialized as JSON).
The logic that invokes buildah will then pass the buildah volume mounts as transient bind mount arguments.

#### API Example

```go
// SourceBuildStrategy defines input parameters specific to an Source build.
type SourceBuildStrategy struct {
  
  // existing API
  ...
  // Note that these fields will also be added to the DockerBuildStrategy API

  // Volumes is a list of input volumes that can be mounted into buildah's runtime environment.
  // Only a subset of Kubernetes Volume sources are supported by builds.
  // More info: https://kubernetes.io/docs/concepts/storage/volumes
  Volumes []BuildVolume

  // VolumeMounts are volumes to mount into buildah's runtime environment at the specified mount path.
  VolumeMounts []BuildVolumeMount
}
```

```go
// BuildVolume describes a volume that is made available to build pods, such that it can be mounted into buildah's runtime environment.
// Only a subset of Kubernetes Volume sources are supported by builds.
type BuildVolume struct {

  // Volume's name.
	// Must be a DNS_LABEL, unique within the pod, and cannot collide with volumes that are always added by the build controller.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string 

	// BuildVolumeSource represents the location and type of the mounted volume.
	BuildVolumeSource `json:",inline" protobuf:"bytes,2,opt,name=volumeSource"`
}

type BuildVolumeSourceType string

const (
  BuildVolumeSourceTypeSecret = "Secret"
  BuildVolumeSourceTypeConfigMap = "ConfigMap"
)

// Represents the source of a volume to mount.
// Only one of its members may be specified.
// This is a subset of the core Kubernetes VolumeSource.
type BuildVolumeSource struct {

  // Type is the type for the volume source.
  // Type must match the populated volume source, and if not specified will be inferred on admission.
  // Only one type of volume source can be specified.
  Type BuildVolumeSourceType

  // Secret represents a secret that should populate this volume.
  // More info: https://kubernetes.io/docs/concepts/storage/volumes#secret
  // +optional
  Secret *kapi.SecretVolumeSource

  // ConfigMap represents a configMap that should populate this volume
  // +optional
  ConfigMap *kapi.ConfigMapVolumeSource
}
```

```go
// BuildVolumeMount describes a mounting of a Volume within buildah's runtime environment.
type BuildVolumeMount struct {
  // This must match the Name of a BuildVolume, and cannot collide with volumes that are always added to the build pod by the build controller.
  Name string
  // Mounted read-only if true, read-write otherwise (false or unspecified).
  // Defaults to false.
  // +optional
  ReadOnly bool
  // Path within the build runtime environment at which the volume should be mounted.
  // Must not contain ':', and cannot collide with a destination path generated by the builder process.
  MountPath string
}
```

Example usage (use of the existing secret/configmap injection api is included for comparison, it is not changing):

```yaml
apiVersion: v1
items:
- apiVersion: build.openshift.io/v1
  kind: BuildConfig
  metadata:
    name: mybuild
    namespace: p1
  spec:
    strategy:
      sourceStrategy:
        from:
          kind: ImageStreamTag
          name: nodejs:10-SCL
          namespace: openshift
        volumes:
        - name: secret
          secret:
            secretName: somesecret
        - name: config
          configMap:
            name: someconfigmap
            items:
            - key: somekey
              path: volume/path/value.txt
        volumeMounts:
        - name: config
          mountPath: /tmp/config
        - name: secret
          mountPath: /tmp/secret
      type: Source
    source:
      secrets:
      - secret: 
          name: myOtherSecret
        destinationDir: /tmp/othersecret
      configMaps:
      - configMap:
          name: myOtherConfigMap
        destinationDir: /tmp/otherconfig
```

#### Volume Name and Destination Collisions

Builds today have several container volume mounts that are provided by the build controller, including:

- Node pull secrets (host)
- System configuration (ConfigMap)
- Certificate authorities for the cluster proxy and internal registry (ConfigMap)
- Blob metadata cache (host)
- RHEL entitlements (host via cri-o configuration)

In addition, the builder process generates the following buildah volume mounts:

- RHEL entitlements (destination is `/run/secrets/etc-pki-entitlement`, `/run/secrets/redhat.repo`, and `/run/secrets/rhsm`)
- Custom PKI trust bundle - destination is `/etc/pki/ca-trust`, added via the Build/BuildConfig's `mountTrustedCA` option.

If an input volume's name collides with a volume created by the build controller, or implied by OpenShift's cri-o configuration, the build should fail to run.
Automatically avoiding volume name collisions - for example, by adding randomized suffixes - can be addressed in a future enhancement.

If a volume mount path collides with a buildah volume mount generated by the builder process, the build should fail to run.
This is unavoidable - we cannot have two mount points share the same destination in the build.

#### Failure behavior

A BuildConfig object can reference a Secret or ConfigMap in a volume that does not exist in the BuildConfig's namespace (yet).
If a Build is generated from a BuildConfig and a referenced Secret or ConfigMap does not exist, the build controller should report a failure status and not create a build pod.
This behavior should also apply to Secrets or ConfigMaps referenced in the build source array.

### Risks and Mitigations

**Risk:** Build mounts can alter the behavior of the build itself (a security risk).

*Mitigations:*

- Supported volume types are gated by the API.
  For example, `HostPath` volume mounts are not supported.
- Input volumes are mounted in a fixed location within the build pod's container.
  The container volume is created by the build controller and cannot be changed by an end user.
- Builds fail to run if a volume name collides with a volume generated by the build controller.
- Builds fail if the a volume mount path collides with a destination generated by the builder process.

**Risk:** Supporting arbitrary volume sources can lead to privilege escalations.
This is a particular concern for builds since build pods run privileged.

*Mitigations:*

- Only volume sources that are known to provide storage isolated from the host file system will be supported (Secrets and ConfigMaps).
- Future supported volume types must ensure isolation from the host.
  Gating must happen at the API or build controller config level, since the build controller uses the privileged SCC to create pods.

**Risk:** Builds can mount content that the user does not have permission to access

*Mitigation:*

Volume mounts for Secrets and ConfigMaps use local object references within the same namespace as the Build.
The assumption is that a Secret in a namespace is accessible to any service account within the namespace.
Furthermore, the existing functionality for source Secrets and ConfigMaps allow any valid object of those types in the namespace to be included in the build.

## Design Details

## Open Questions [optional]

1. Can we make this the default or even only behavior for builds?
   No, need to make it opt-in to avoid potentially breaking existing buildconfigs.
2. What happens if a user overrides the default volume mounts used by Builds today?
   We prevent the build from running.
   Overriding default volume mounts can alter build behavior, which can be a security risk.
3. Can cluster admins alter the default `mounts.conf` configuration for cri-o?
   No - `mounts.conf` cannot be configured via the cluster ContainerRuntimeConfig custom resource.
   Altering `mounts.conf` via MachineConfig is unsupported.

### Test Plan

This feature will need new e2e tests that leverage the new api option.
We have existing tests for configmap+secret injection would should be able to be copied+adapted to testing this feature relatively easily.

### Graduation Criteria

This should be introduced directly as a GA feature when it is implemented.

Future enhancements can add additional volume source types.
When adding new source types, the following questions need to be answered:

- Can the build controller support the lifecycle of this volume source?
- What should happen if the build is not able to mount a volume source? How will the status of the volume mount in the build pod be reflected in the build's status?
- Does this volume source type introduce security risks?
  If so, how will this risk be mitigated?
- How will this volume source be tested?
- How will this volume source be made available to users and cluster admins?
  - Will cluster admins want to disable this volume source?
    Or conversely, will cluster admins need to explicitly enable this volume source as a cluster feature?
  - Will cluster admins want fine control over how this volume source can be mounted?

#### Dev Preview -> Tech Preview

N/A - this will be introduced as a GA feature.

#### Tech Preview -> GA

N/A - this will be introduced as a GA feature.

#### Removing a deprecated feature

N/A - this is a new feature.

### Upgrade / Downgrade Strategy

This will be added to the Build and BuildConfig APIs, which are provided by the aggregated OpenShift apiserver.
On downgrade, the volume fields will not be returned but will remain in etcd storage until a subsequent write is made to the Build or BuildConfig object.
Any Build which is invoked from a BuildConfig after a downgrade will risk losing the volume mount information (ex - via `oc start-build`, an image change trigger, etc.)

### Version Skew Strategy

The `BuildVolumeSource` API will include a type discriminator.
As new volume source types are added to the API, the discriminator will ensure that the build controller will only mount volumes that it is aware of.

## Implementation History

2020-01-08: Initial proposal by @bparees
2021-04-21: Revised proposal with edits by @adambkaplan
2021-05-05: Implementable version

## Drawbacks

This API will overlap with our existing support for injected Secrets and ConfigMaps in build sources.
The nuance of injecting a Secret or ConfigMap as a *buildah volume mount* can be difficult for end users to comprehend.
The following critical question can guide users to what is best for their needs:

*Do you want this content in the resulting image?*
If yes, use build sources.
If no, use build volumes.

Documentation of this feature should encourage users to mount Secrets using the new Volume API, instead of the current "source" API.
Users will need to opt-in to this feature - BuildConfigs that inject secrets today will need to be updated to take advantage of this feature.

## Alternatives

Updating the existing injection apis to have a "asVolume" field was considered as it would be a simpler implementation (more code reuse) but it was rejected as there is a long term goal to allow builds to mount traditional volumes as well.
The existing injection api can't easily be extended to support such a thing, so the design proposed in this enhancement is a better stepping stone to that goal.
This also puts us on a better path to support the following critical use cases:

- Using volumes for caching build content between build invocations.
- Using "cluster-scoped" credentials in builds - in particular simple content access certificates used to download RHEL subscription content.
