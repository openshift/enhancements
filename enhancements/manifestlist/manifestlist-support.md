---
title: manifest-list-support
authors:
  - "@ricardomaraschini"
reviewers:
  - "@dmage"
approvers:
  - "@dmage"
creation-date: 2021-02-03
last-updated: 2021-02-03
status:
see-also:
replaces:
superseded-by:
---

# Add support to manifest list image imports

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

A manifest list is a group of image manifests, it is used to keep track of references to images
targeting different platforms/architectures in a single entity. OpenShift does not support this
type of manifest, it currently supports only a 1 to 1 mapping (one manifest pointing to one
image). Manifest lists are identified by their media types:

```text
application/vnd.docker.distribution.manifest.list.v2+json
application/vnd.oci.image.index.v1+json
```

Their representation is very simple, it contains (aside from the media type and schema version)
an array of manifests with their respective platform information:

```json
{
  "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
  "schemaVersion": 2,
  "manifests": [
    {
      "digest": "sha256:0b159cd1ee1203dad901967ac55eee18c24da84ba3be384690304be93538bea8",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "amd64",
        "os": "linux"
      },
      "size": 1362
    },
    {
      "digest": "sha256:0492a0bfe6585aee9f80b476b8d432a03968bb0a2b67d00a340cac69956e6c50",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "arm",
        "os": "linux",
        "variant": "v5"
      },
      "size": 1362
    }
  ],
}
```

This enhancement proposal designs a plan to add support for this new type of manifest in
OpenShift.

## Motivation

When running on multi-architecture clusters it makes sense to be able to allow users to work with
images that support different architectures as well. By adding support for manifest lists we are
also adding support for hosting these multi-architecture images in OpenShift's internal registry.

### Goals

- Allow users to import manifest lists using the `oc` client.
- Allow users to execute pull-through of manifest lists.
- Make image pruner aware of manifest lists.
- Allow users to push manifest lists into the integrated registry.
- Improve the `oc describe` command to present manifest list info.

### Non-Goals

- Modify OpenShift's web console to support manifest lists.
- Designs OpenShift multi-architecture builds.
- Provide facilities to build manifest lists.

## Proposal

The proposed approach involves allowing one Image struct to refer to multiple other images by
their hashes. In such a scenario, an Image `A` would represent the manifest list containing among
its properties a slice of other Images `B` and `C`, each of them being a Manifest for an image of
a different platform.

In other words, when seeing from an ImageStream point of view we would have a mapping that looks
like this:

```text
ImageStream X -> Image A -> Image B
                            Image C
                            Image D

```

Here `Image A` would contain references to `B`, `C`, and `D`. As a manifest list is simply a
reference to other manifests they do not have any extra information such as "environment
variables", "entry point" or "open ports" among its fields, this extra information, therefore,
still belongs to the "child images" (`B` and `C` for instance). Image A would serve as a means
to group other Images.

### User Stories

#### Support for manifest list import

As an OpenShift user
I want to be able to import manifest lists
So that I can refer to images supporting multiple platforms

#### Support for manifest list pull-through

As an OpenShift user
I want to be able to pull through manifest lists
So that I can cache multi-architecture images within my cluster

#### Support for pushing manifest lists to OpenShift's image registry

As an OpenShift user
I want to be able to push manifest lists to OpenShift's internal registry
So that I can publish my multi-architecture images

#### Support for manifest lists during prune

As an OpenShift user
I want to be able to automatically clean unused manifest lists
So that I can keep my storage resource usage under control

### Implementation Details/Notes/Constraints

#### On pushing manifests into integrated registry

OpenShift's internal registry will need to accept image pushes by SHA, not only by tag. The
difference here is that the registry is not supposed to create an Image object every time it
sees a push by SHA but only when it sees a push by tag. In the latter scenario the registry may
need to create more than one image (if the pushed manifest is of type manifest list).

### Risks and Mitigations

#### Console usage of ImageStreamImport

OpenShift console uses ImageStreamImport to obtain information about images to present them to
users. When dealing with manifest lists, not all information will be presented back to the console
(the Config portion of an image is absent in a manifest list). Some changes would be needed in
this regard to tackling this, see the "Proposed ImageStreamImport changes" section below for
further details.

#### Image registry authorization

The current Image and ImageStream layouts are leveraged by the image registry when verifying if
one is allowed or not to execute a pull for a certain blob or manifest. API needs to be adjusted
to take into account this extra level of indirection.

These verifications are made using ImageStreamLayers API. This API returns a list of blobs and
manifests that are part of a certain ImageStream. The call made is similar to this:

```go
imageclient.ImageV1().ImageStreams("namespace").Layers(
	ctx, "image-stream-name", metav1.GetOptions{},
)
```

This call returns an ImageStreamLayers object, similar to:

```go
type ImageStreamLayers struct {
	metav1.TypeMeta 
	metav1.ObjectMeta
	Blobs map[string]ImageLayerData
	Images map[string]ImageBlobReferences
}
```

Blobs property in this struct is a list of all blobs that are part of the ImageStream while
Images is a list of all manifests (Image objects) that are part of the same ImageStream. This
API will need to be updated to also include child image details (blobs and images).

## Design Details

### Proposed Image API changes

Images are indexed (named) in OpenShift by the hash of their content. Currently an Image is
represented by the following structure:

```go
type Image struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	DockerImageReference string
	DockerImageMetadata runtime.RawExtension
	DockerImageMetadataVersion string
	DockerImageManifest string
	DockerImageLayers []ImageLayer
	Signatures []ImageSignature
	DockerImageSignatures [][]byte
	DockerImageManifestMediaType string
	DockerImageConfig string
}
```

This document proposes the introduction of an extra field pointing to all "child images" (as all
child images are other manifests we would call them `DockerImageManifests`):

```go
	DockerImageManifests []ImageManifest
```

`ImageManifest` struct would look like the following:

```go
type ImageManifest struct {
	Digest       string
	MediaType    string
	ManifestSize int64
	Architecture string
	OS           string
	Variant      string
}
```

The Image pointed by the `Digest` property contains all regular information (i.e. Config) we can
see nowadays in a regular Image.

Not all fields in an Image are relevant for a manifest list, therefore other change is to add an
"omitempty" flag on `DockerImageLayers` property as well as to make `DockerImageMetadata` a
pointer.

The resulting Image struct would look like:


```go
type Image struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	DockerImageReference string
	DockerImageMetadata *DockerImage
	DockerImageMetadataVersion string
	DockerImageManifest string
	DockerImageLayers []ImageLayer
	Signatures []ImageSignature
	DockerImageSignatures [][]byte
	DockerImageManifestMediaType string
	DockerImageConfig string
	DockerImageManifests []ImageManifest
}
```

### Proposed ImageStreamImport changes

Users may start an image import by creating an `ImageStreamImport` object. When using these
objects the user specifies if it desires to import a full repository or a single tag in a
repository. An `ImageStreamImport` specification looks like:

```go
type ImageStreamImportSpec struct {
	Import bool
	Repository *RepositoryImportSpec
	Images []ImageImportSpec
}
```

Property `Repository` (when set) indicates that the user intends to import a full repository,
i.e. multiple tags, while `Images` is a slice indicating that a user wants to import only a set
of tags from one or more repositories. API reports back the import outcome in the `.status`
property (some types below were embed to make visualization easier):

```go
type ImageStreamImportStatus struct {
	Import *ImageStream
	Images []ImageImportStatus
	Repository struct {
		Status metav1.Status
		Images []ImageImportStatus
		AdditionalTags []string
	}
}
```
In the end, the outcome of a single tag import or a full repository import is represented through
one or multiple `ImageImportStatus` structs, this struct looks like this:

```go
type ImageImportStatus struct {
	Status metav1.Status
	Image *Image
	Tag string
}
```

`ImageImportStatus` struct has a reference for a single image on its `Image` field. This
enhancement proposes the addition of another field where child images (manifests within a
manifes list) would be listed, leaving `ImageImportStatus` struct looking like:


```go
type ImageImportStatus struct {
	Status metav1.Status
	Image *Image
	Tag string
	Manifests []Image
}
```

Here the image that refers to the manifest list would be presented on `Image` property (as it is
today for regular manifests) while all inner manifests within the manifest list would be listed
in the `Manifests` slice.

### Proposed ImageStreamImage changes

ImageStreamImages are a way of referring to a specific Image within an ImageStream by its hash.
It is useful when tagging an ImageStreamTag into another ImageStreamTag. With ImageStreamImage
one can issue a command such as:

```console
$ oc tag imagestream0:tag0 imagestream1:tag1
```

This command would create a tag similar to this one inside "imagestream1":

```yaml
spec:
  tags:
  - name: tag1
    from:
      kind: ImageStreamImage
      name: imagestream0@sha256:<sha256>
```

ImageStreamImage can also be used to "query details" about a specific Image, by using the
following command a user can obtain information about a specific Image inside an ImageStream:

```console
$ oc get isimage <imagestream>@<sha of one of the tags>
```

With the introduction of the manifest list support, there won't be many details about an Image
on its "parent" manifest (extra information belongs to child images). This enhancement proposes
an extension to allow users to also view details for any of the child images.

Both of the commands below would be valid, the first command would not return that much
information but it works as a means of obtaining a list of all child images:

```console
$ oc get isimage <imagestream>@<sha of the parent image>
$ oc get isimage <imagestream>@<sha of any of the child images>
```

The output of the `oc describe isimage` command line will need to be changed to display
information present in the DockerImageManifests property of the parent Image.

### Proposed pruner changes

Pruner gathers information about images in use within a cluster by inspecting the following
objects:

- Pods
- Replication controllers
- Deployment configs
- Replica sets
- Deployments
- Daemon sets
- Builds
- Build configs
- Stateful sets
- Jobs
- Cronjobs

If a given image is in use by any of these entities it is not pruned. This operation will need
to be refactored to take into account the extra level of indirection that the "child images"
layer brings. For example, in a scenario where we have a tag in ImageStream X pointing to a
manifest list A containing images B, C, and D:

```text
ImageStream X -> Image A -> Image B
                            Image C
                            Image D

```

If a reference for, let's say, `Image D` exists in the cluster pruner will maintain `Image A`,
`Image B`, `Image C`, and `Image D` as well. The same logic also applies when deleting tags from
an `ImageStream` during prune: all child images are also deleted whenever a tag ceases to exist.

The extra cost for this indirection analysis will be paid during the initial evaluation of images
in use. During this operation, we already have all Images and ImageStreams available in memory and
during evaluation, we fill in two maps indicating the images in use and who is using them using
a struct like the following:

```text
p.usedImages = map[imageStreamImageReference][]resourceReference{}
```

By considering the above example scenario again (Pod called `pod0` using `Image D`), after
the initial evaluation we would have the following resulting struct (edited for easier reading):

```text
p.usedImages = {
	"Image A": [ "pod0" ],
	"Image B": [ "pod0" ],
	"Image C": [ "pod0" ],
	"Image D": [ "pod0" ]  <- the actual image in use
}
```

This would ensure that we treat the whole manifest list as a single entity at a cost of
maintaining in the cluster images that may not be in use but are part of a manifest list.

### Test Plan

#### Validate we can deploy manifest lists

1. Deploy a temporary image registry.
2. Publish a manifest list containing images for different platforms.
3. Create an ImageStream object to import the published manifest list.
4. Create a Deployment using the ImageStream.
5. Gathers and validates pod statuses.

A second test would be very similar to the one above but at this time leveraging pull-through.
Podman currently supports managing manifest lists and we can use its internals during step 2.

#### Validate we can mirror manifest lists

1. Deploy a temporary image registry.
2. Push an image to it using a manifest list.
3. Issue an `oc image mirror` targetting the internal registry.
4. Validates the resulting ImageStream and Image objects.

### Validate builds based in a manifest list work

1. Push a ManifestList Image into the internal registry.
2. Create a build (Docker build) leveraging the pushed image.
3. Validate the build succeeded by inspecting its output.
4. Check the resulting image has also been pushed to the registry by pulling it.

### Graduation Criteria

#### Tech Preview

Not applicable - this feature is intended to be GA upon release.

#### Generally Available

Requirements for reaching GA (in addition to tech preview criteria):

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

2021-02-03: Initial draft proposal.
2021-03-18: Added sections on ImageStreamImage and ImageStreamImport.
2021-03-23: Added note about ImageStreamLayers.

## Alternatives
