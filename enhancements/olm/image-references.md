---
title: image-references-in-operator-bundles
authors:
  - "@stevekuznetsov"
reviewers:
  - "@jwforres"
  - "@shawn-hurley"
  - "@gallettilance"
  - "@lui"
approvers:
  - "@jwforres"
  - "@shawn-hurley"
creation-date: 2020-05-17
last-updated: 2020-05-17
status: implementable
see-also:
  - "/enhancements/olm/operator-bundle.md"
---

# Image References in Operator Bundle Manifests

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Manifests that make up an Operator Bundle for installation via the Operator
Lifecycle Manager refer to one or more container images by their pull
specifications: these container images define the operator and operands that
the manifests deploy and manage. As with OpenShift release payloads, operator
bundles must refer to images by digest in order to produce reproducible
installations. A shared process to build operator bundles that replaces image
references with fully-resolved pull specifications that pin images by digest
must be built; this process must allow for a number of separate build systems
to direct how these replacements occur in order to support a full-featured
build and test strategy.

## Motivation

### Goals

- there is one, canonical, method for building an operator bundle image layer
  from a directory of manifests
- building an operator bundle image does not require the use of a container
  runtime, elevated privileges or any capacities that are not present for 
  containerized workloads on OpenShift
- it is as simple for a developer to build a bundle referring to test versions
  of operand images as it is for a CI or productized build pipleine to create
  bundle images for publication
- operator manifest authors dictate the set of image references in manfiests
  that must be resolved and pinned
- operator bundle images may be inspected to determine the pull specifications
  that were used in the creation of the bundle

### Non-Goals

- operator manifest authors must not be required to define the registry from
  which any individual build system will resolve image references
- upstream operator manifests must not be required to know how common names or
  referecnes change when built in a downstream pipeline

## Proposal

### User Stories

#### Story 1

As an author of a manifest, I would like to check in manifests to my upstream
repository that are self-consistent, valid and make no assumptions about the
build system that will eventually create a bundle image with them.

#### Story 2

As an author of an operand, I would like to create a bundle locally in order
to test my operator end-to-end on a cluster of my choosing without having to
edit the core configuration for the operator.

#### Story 3

As an author of a build system, I would like to operate with tooling that allows
me to clearly define the source of truth for image digests in order to keep the
build-system-specific configuration to an absolute minimum.

#### Story 4

As an engineer involved in publishing an optional operator, I would like to
configure semantically equivalent image pull specifications once, in order to not
need to configure each build system independently.

### Implementation Details/Notes/Constraints

The core problems that must be solved in the implementation of this proposal have
already been handled in the workflow used in `oc adm release new`. When implementing
improvements to the `operator-sdk bundle create` process we will simply need to
create a shared library for the two tools to use. While the shape of the output
is slightly different and some of the semantics about how the output should be 
formatted are dissmilar, the core image reference rewriting is identical and the
process of building a `FROM scratch` image layer is also identical.

Today, some prior art exists in the OSBS workflow for building operator bundles.
As we improve the Operator SDK tooling to create a straghtforward process for
creating bundle images, we must make sure a seamless migration is possible.

### Risks and Mitigations

It will be critical that the design be vetted by all of the concerned parties, from
operator manifest authors to CI system authors and productized pipeline authors to
ensure that the UX is appropriate in all cases. Furthermore, the largest risk in the
implementation here is not prioritizing a clean migration pathway for all current
users who create bundle images, which would lead to further fragmentation of the
ecosystem, which is directly opposed to the goal of this enhancement.

## Design Details

The definition of a minimally-viable operator bundle image will be changed to
ensure that all iamge references in the contained manifests have been resolved 
to a digest and had the pull specifications rewritten to refer to those digests.

The only acceptable process for creating an operator bundle will be to run the
`operator-sdk bundle create` CLI, providing the manifests, metadata and image
sources as input to the creation process.

### Proposed UX

Operator manifest authors write a manifest that refers to images using some
opaque string [ex](https://github.com/openshift/machine-config-operator/blob/master/install/0000_80_machine-config-operator_04_deployment.yaml#L22),
and provide an `image-references` file alongside their manifests that declares
which strings inside of their manifest are referring to pull specifications of
images and names each occurence [ex](https://github.com/openshift/machine-config-operator/blob/98d9ba6841eb4811ed6f4d4de7016ea83c131c54/install/image-references#L7-L10).

The `image-references` file holds data in the format of an OpenShift `ImageStream`:

```yaml
kind: ImageStream
apiVersion: image.openshift.io/v1
spec:
  tags:
  - name: my-image-name # this is the semantic identifier for this image
    from:
      kind: DockerImage
      name: registry.svc.ci.openshift.org/openshift:image # this is an opaque string which exists in manifests
```

This mapping, therefore, defines what needs to be replced in manifests when
they are bundled and identifies each replacement with a name. When
`operator-sdk bundle create` runs, it will require as input a second mapping
from those names to literal image pull specifications and will run the replacement.
In this manner, the configuration provided by the manifest author remains static
regardless of the eventual replacement that a build system will execute.

The second mapping required at build-time will also take the form of an `ImageStream`
and will be provided either via reading an `ImageStream` from an OpenShift cluster or
reading a serialized `ImageStream` from disk. A build system that does not store the
images used as sources in an `ImageStream` may therefore provide a static file instead.
The mapping to provide the image sources looks like:

```yaml
apiVersion: image.openshift.io/v1
kind: ImageStreamImageam
status:
  tags:
  - tag: my-image-name # this matches the semantic identifier provided by the manifest author
    items:
    - dockerImageReference: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:298d0b496f67a0ec97a37c6e930d76db9ae69ee6838570b0cd0c688f07f53780 # this is the fully-resolved pull specification that will be used in replacements
      image: sha256:298d0b496f67a0ec97a37c6e930d76db9ae69ee6838570b0cd0c688f07f53780
```

`operator-sdk bundle create` will use this chain of mapping to perform replacements
in the manifests before creating a bundle image layer. The layer creation will also
be shared logic with `oc adm release new` in order to allow both processes to build
image layers without requiring the use of a containe runtime, other build system, any
elevated permissions, privleges, capacities or SELinux roles. As the output image
layer in both cases is `FROM scratch` and simply contains manifest data, this build
process is simple and producing the layer by creating the underlying tar bundle does
not come with risks.

### Test Plan

It should be possible to duplicate all current tests for `operator-sdk bundle create`
in order to validate that the new workflow creates identical output bundle images.

It should furthermore be possible to duplicate all tests that exist for the OSBS
workflows to similarly validate output.

### Graduation Criteria

In order to graduate this feature, the UX surface for the `operator-sdk` CLI should
be stable and the output bundle images should be fully verified as comaptible with 
previous versions of the tool as well as those built by other tools.

#### Examples

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- Sufficient time for feedback
- Available by default

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

Not applicable.

## Implementation History
