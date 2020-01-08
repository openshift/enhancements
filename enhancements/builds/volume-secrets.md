---
title: volume-mounted-secret-and-configmap-injection
authors:
  - "@bparees"
reviewers:
  - @adambkaplan
  - @smarterclayton
approvers:
  - @adambkaplan
creation-date: 2020-01-08
last-updated: 2020-01-08
status: provisional
---

# Volume Mounted Secret+ConfigMap Injections


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

 > 1. Can we make this the default or even only behavior for builds?  Probably not, need to make it opt-in to avoid potentially breaking existing buildconfigs.

## Summary

Today builds support getting source input from configmaps and secrets.  When users utilize this feature, the configmap or secret is volume-mounted into the build pod.  The next steps depend on whether it is an s2i or dockerfile build.

For s2i builds, the generated Dockerfile contains commands to `ADD` the content at a path specified by the user, the assemble script is invoked, and then the injected content is zeroed out prior to committing the image via a `RUN rm` command added to the Dockerfile.

For dockerfile builds, the user is instructed to add appropriate `ADD` and `RUN rm` commands to their dockerfile to inject the content that is available in the build's working directory (along with their application source, where applicable).

There are a few undesirable aspects to this:
1) In the dockerfile case, the content can still be found in lower layers of the image unless a layer squashing option is 
selected.
2) Requires extra work by the user in the Dockerfile, so each Dockerfile must be customized

This enhancement proposes to introduce an option to use buildah's capability to mount a volume at build time.  The content mounted into the build pod would be then mounted into the container processing the Dockerfile, making that content available within the container so Dockerfile commands could reference it.  No explicit `ADD` would be required, and since mounted content is not committed to the resulting image, no `RUN rm` equivalent is required to clean up the injected content.


## Motivation

### Goals

* Simplify how users consume secret + configmap content in builds
* Increase the security of protected content being injected to images
* Simplify use cases that require consuming credentials during the build, but need to ensure those credentials do not end up in the output image.
* Eventually extend this api to allow the mounting of traditional volumes (such as those backed by persistent storage)

### Non-Goals

* This enhancement should not result in a change of behavior for users of the existing secret/configmap injection api.


## Proposal

### User Stories [optional]

The enabled use cases are essentially identical to what can be done with the configmap/secret input api in builds today, but with a better user experience and security as discussed above.  It does not enable a new use case that is not already possible today, except that layer squashing will not be required.

Future extensions to this enhancement could enable the use case of providing build input content from a persistent volume and allowing the build to store/cache content for future builds on such a volume.  Those will be discussed in the future enhancement at that time.

### Implementation Details/Notes/Constraints [optional]

We will need to introduce a new mechanism in the build api which allows the user to indicate that they want to inject "volume" content into the build.  Initially the only allowed volume types will be configmaps and secrets.  The api will otherwise be similar to the existing secret/configmap injection api in which users identify the configmap/secret and the target path for injection.  

This will be done by adding a Volume[] field to the Source and Docker strategy structs.  The Volume[] field will allow 
defining volumes to be mounted to the build pod in the same way that a normal pod allows for this.  Similarly a VolumeMount[]
field will be added, but without the MountPropagation and SubPath fields.  MountPropagation and SubPath can be considered
for support in the future.

These fields will be wired, via the build controller, directly to the build pod that is constructed, modulo some validation logic to constrain the types of Volumes we want to support (initially just configmaps and secrets).  In addition all mounts
into the pod will be done at a path of our choosing, not the VolumeMount path specified, to ensure the user cannot 
overwrite critical function inside the build pod and use it as an escalation pathway.

The logic that invokes buildah will then pass the mounted directories as volume mount arguments.  The mount path provided
to buildah will be determined from the VolumeMount specification.

Proposed api/structs:
Note:  DockerBuildStrategy will be updated in the same way.
```
// SourceBuildStrategy defines input parameters specific to an Source build.
type SourceBuildStrategy struct {
  // From is reference to an DockerImage, ImageStream, ImageStreamTag, or ImageStreamImage from which
  // the docker image should be pulled
  From kapi.ObjectReference

  // PullSecret is the name of a Secret that would be used for setting up
  // the authentication for pulling the Docker images from the private Docker
  // registries
  PullSecret *kapi.LocalObjectReference

  // Env contains additional environment variables you want to pass into a builder container.
  Env []kapi.EnvVar

  // Scripts is the location of Source scripts
  Scripts string

  // Incremental flag forces the Source build to do incremental builds if true.
  Incremental *bool

  // ForcePull describes if the builder should pull the images from registry prior to building.
  ForcePull bool

  // Volumes is a list of volumes that can be mounted by the build
  // More info: https://kubernetes.io/docs/concepts/storage/volumes
  // Only Secret and ConfigMap type volumes are supported currently.
  Volumes []kapi.Volume

  // VolumeMounts are volumes to mount into the build
  VolumeMounts []VolumeMount
}
```


```
// VolumeMount describes a mounting of a Volume within the build environment.
type VolumeMount struct {
  // This must match the Name of a Volume.
  Name string
  // Mounted read-only if true, read-write otherwise (false or unspecified).
  // Defaults to false.
  // +optional
  ReadOnly bool
  // Path within the build environment at which the volume should be mounted.  Must
  // not contain ':'.
  MountPath string
}
```

Example usage (use of the existing secret/configmap injection api is included for comparison, it is not changing)
```
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

### Risks and Mitigations

Since the build pod is privileged, we need to ensure that users cannot abuse this api to trick the build controller
into creating build pods that can exploit those privileges.  This means ensuring that any volume mount specifications
the user provides, which are translated into volumemounts in the build pod, cannot be used to alter the build logic.
To this end, we should explicitly control where the volumes are mounted within the build pod.  (We can mount them 
anywhere the user specifies within the buildah container).  

We also need to ensure that the user can't use this api to inject/mount content that they could not normally mount
into a pod they created themselves.  For this reason we must explicitly disallow `HostPath` volume types, for example.
We will mitigate this by whitelisting the volume types we support, starting with only allowing ConfigMaps and Secrets.
As additional types are whitelisted, we will need to determine it is safe to add them.


## Design Details

### Test Plan

This feature will need new e2e tests that leverage the new api option.  We have existing tests for configmap+secret injection would should be able to be copied+adapted to testing this feature relatively easily.


### Graduation Criteria

This should be introduced directly as a GA feature when it is implemented.

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Additional user complexity in choosing when to enable this behavior.  It is also unfortunate we can't default it because it will be a better choice for most users.

## Alternatives

Updating the existing injection apis to have a "asVolume" field was considered as it would be a simpler implementation (more code reuse) but it was rejected as there is a long term goal to allow builds to mount traditional volumes as well.  The existing injection api can't easily be extended to support such a thing, so the design proposed in this enhancement is a better stepping stone to that goal.  This also puts us on a better path to support using volumes for caching build content between build
invocations which has been a longtime goal of the build api.

