---
title: add-new-CRD-ImageDigestMirrorSet-and-ImageTagMirrorSet-to-config.openshift.io
authors:
  - "@QiWang19"
reviewers:
  - "@mtrmac"
  - "@kikisdeliveryservice"
  - "@sttts"
approvers:
  - TBD
api-approvers:
  - TBD
  - "@sttts"
  - "@oscardoe"
creation-date: 2021-03-10
last-updated: 2022-06-03
status: implementable
---

# Add CRD ImageDigestMirrorSet and ImageTagMirrorSet to config.openshift.io/v1

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today, the ImageContentSourcePolicy(ICSP) object sets up mirror with `mirror-by-digest-only` property set to true, i.e. it only supports using mirrors for image references by digest. This enhancement introduces new CRDs to config/v1:
- ImageDigestMirrorSet: holds mirror configurations that are used to pull image from mirrors by digest specification only.
- ImageTagMirrorSet: holds mirror configurations that are used to pull image from mirrors using tag specification only.

## Motivation

### Motivation to add support allow mirror by tags

- Today, the ICSP object sets up mirror configuration with `mirror-by-digest-only` property set to `true`, which leads to using mirrors only when images are referenced by digests. However, when working with disconnected environments, sometimes multiple ImageContentSourcePolicies are needed, some of them are used by apps/manifests that don't use digests when pulling the images.

### Motivation to add new CRDs to config/v1

- Current ImageContentSourcePolicy(ICSP) of operator.openshift.io/v1alpha1 was added to operator group after operator.openshift.io/v1. Adding v1alpha1 after v1 was a mistake. We should not continue development on ImageContentSourcePolicy(ICSP) of operator.openshift.io/v1alpha1.
- ImageContentSourcePolicy(ICSP) holds cluster-wide information about how to handle registry mirror rules. It is not about the configuration of an operator. The CRD should be moved to config.openshift.io/v1.
- Current ImageContentSourcePolicy(ICSP) has no kubebuilder annotations to validate the fields of it. We need to
add validation for the CRD. Even if we continue with operator.openshift.io, add validation to the field and add
ImageContentSourcePolicy(ICSP) to operator.openshift.io/v1 CRDs don't have ratcheting validation yet to cope with
that. So a new CRD should be created, and ImageContentSourcePolicy(ICSP) under operator.openshift.io/v1alpha1 will be removed.

### Goals

- Enable the user to pull images using digest only from mirrors configured through config.openshift.io/v1 API ImageDigestMirrorSet.
- Enable the user to pull images using tags from mirrors configured through config.openshift.io/v1 API ImageTagMirrorSet.

### Non-Goals

- This proposal does not recommend using by-tag references for OpenShift release images. Those should still be referenced by digest, regardless of whether `ImageTagMirrorSet` is configured for repositories where the release images are mirrored

## Proposal

The New CRD `ImageDigestMirrorSet` will be added to config.openshift.io/v1.

```go
// ImageDigestMirrorSetSpec is the specification of the ImageDigestMirrorSet CRD.
type ImageDigestMirrorSetSpec struct {
	// imageDigestMirrors allows images referenced by image digests in pods to be
	// pulled from alternative mirrored repository locations. The image pull specification
	// provided to the pod will be compared to the source locations described in imageDigestMirrors
	// and the image may be pulled down from any of the mirrors in the list instead of the
	// specified repository allowing administrators to choose a potentially faster mirror.
	// To use mirrors to pull images using tag specification, users should configure
	// a list of mirrors using "ImageTagMirrorSet" CRD.
	//
	// If the image pull specification matches the repository of "source" in multiple imagedigestmirrorset objects,
	// only the objects which define the most specific namespace match will be used.
	// For example, if there are objects using quay.io/libpod and quay.io/libpod/busybox as
	// the "source", only the objects using quay.io/libpod/busybox are going to apply
	// for pull specification quay.io/libpod/busybox.
	// Each “source” repository is treated independently; configurations for different “source”
	// repositories don’t interact.
	//
	// If the "mirrors" is not specified, the image will continue to be pulled from the specified
	// repository in the pull spec.
	//
	// When multiple policies are defined for the same “source” repository, the sets of defined
	// mirrors will be merged together, preserving the relative order of the mirrors, if possible.
	// For example, if policy A has mirrors `a, b, c` and policy B has mirrors `c, d, e`, the
	// mirrors will be used in the order `a, b, c, d, e`.  If the orders of mirror entries conflict
	// (e.g. `a, b` vs. `b, a`) the configuration is not rejected but the resulting order is unspecified.
	// Users who want to use a specific order of mirrors, should configure them into one list of mirrors using the expected order.
	// +optional
	// +listType=atomic
	ImageDigestMirrors []ImageDigestMirrors `json:"imageDigestMirrors"`
}

// ImageDigestMirrors holds cluster-wide information about how to handle mirrors in the registries config.
type ImageDigestMirrors struct {
	// source matches the repository that users refer to, e.g. in image pull specifications. Setting source to a registry hostname
	// e.g. docker.io. quay.io, or registry.redhat.io, will match the image pull specification of corressponding registry.
	// "source" uses one of the following formats:
	// host[:port]
	// host[:port]/namespace[/namespace…]
	// host[:port]/namespace[/namespace…]/repo
	// [*.]host
	// for more information about the format, see the document about the location field:
	// https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md#choosing-a-registry-toml-table
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\*(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+$|^((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+)?(?::[0-9]+)?)(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?$`
	Source string `json:"source"`
	// mirrors is zero or more locations that may also contain the same images. No mirror will be configured if not specified.
	// Images can be pulled from these mirrors only if they are referenced by their digests.
	// The mirrored location is obtained by replacing the part of the input reference that
	// matches source by the mirrors entry, e.g. for registry.redhat.io/product/repo reference,
	// a (source, mirror) pair *.redhat.io, mirror.local/redhat causes a mirror.local/redhat/product/repo
	// repository to be used.
	// The order of mirrors in this list is treated as the user's desired priority, while source
	// is by default considered lower priority than all mirrors.
	// If no mirror is specified or all image pulls from the mirror list fail, the image will continue to be
	// pulled from the repository in the pull spec unless explicitly prohibited by "mirrorSourcePolicy"
	// Other cluster configuration, including (but not limited to) other imageDigestMirrors objects,
	// may impact the exact order mirrors are contacted in, or some mirrors may be contacted
	// in parallel, so this should be considered a preference rather than a guarantee of ordering.
	// "mirrors" uses one of the following formats:
	// host[:port]
	// host[:port]/namespace[/namespace…]
	// host[:port]/namespace[/namespace…]/repo
	// for more information about the format, see the document about the location field:
	// https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md#choosing-a-registry-toml-table
	// +optional
	// +listType=set
	Mirrors []ImageMirror `json:"mirrors,omitempty"`
	// mirrorSourcePolicy defines the fallback policy if fails to pull image from the mirrors.
	// If unset, the image will continue to be pulled from the the repository in the pull spec.
	// sourcePolicy is valid configuration only when one or more mirrors are in the mirror list.
	// +optional
	MirrorSourcePolicy MirrorSourcePolicy `json:"mirrorSourcePolicy,omitempty"`
}

// +kubebuilder:validation:Pattern=`^((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+)?(?::[0-9]+)?)(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?$`
type ImageMirror string

// MirrorSourcePolicy defines the fallback policy if fails to pull image from the mirrors.
// +kubebuilder:validation:Enum=NeverContactSource;AllowContactingSource
type MirrorSourcePolicy string

const (
	// neverContactSource prevents image pull from the specified repository in the pull spec if the image pull from the mirror list fails.
	NeverContactSource MirrorSourcePolicy = "NeverContactSource"

	// allowContactingSource allow falling back to the specified repository in the pull spec if the image pull from the mirror list fails.
	AllowContactingSource MirrorSourcePolicy = "AllowContactingSource"
)
```

An example ImageDigestMirrorSet file will look like:
```yaml
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: ubi8repo
spec:
  repositoryDigestMirrors: 
  - mirrors:
    - example.io/example/ubi-minimal
    source: registry.access.redhat.com/ubi8/ubi-minimal
    mirrorSourcePolicy: AllowContactingSource
```

The ImageDigestMirrorSet object will lead to the creation of a drop-in file at `/etc/containers/registries.conf` by the MCO [Container Runtime Config Controller](https://github.com/openshift/machine-config-operator/tree/master/pkg/controller/container-runtime-config) that
limits the usage of configured mirrors to image pulls by digest only.

The New CRD `ImageTagMirrorSet` will be added to config.openshift.io/v1.

```go
// ImageTagMirrorSetSpec is the specification of the ImageTagMirrorSet CRD.
type ImageTagMirrorSetSpec struct {
	// imageTagMirrors allows images referenced by image tags in pods to be
	// pulled from alternative mirrored repository locations. The image pull specification
	// provided to the pod will be compared to the source locations described in imageTagMirrors
	// and the image may be pulled down from any of the mirrors in the list instead of the
	// specified repository allowing administrators to choose a potentially faster mirror.
	// To use mirrors to pull images using digest specification only, users should configure
	// a list of mirrors using "ImageDigestMirrorSet" CRD.
	//
	// If the image pull specification matches the repository of "source" in multiple imagetagmirrorset objects,
	// only the objects which define the most specific namespace match will be used.
	// For example, if there are objects using quay.io/libpod and quay.io/libpod/busybox as
	// the "source", only the objects using quay.io/libpod/busybox are going to apply
	// for pull specification quay.io/libpod/busybox.
	// Each “source” repository is treated independently; configurations for different “source”
	// repositories don’t interact.
	//
	// If the "mirrors" is not specified, the image will continue to be pulled from the specified
	// repository in the pull spec.
	//
	// When multiple policies are defined for the same “source” repository, the sets of defined
	// mirrors will be merged together, preserving the relative order of the mirrors, if possible.
	// For example, if policy A has mirrors `a, b, c` and policy B has mirrors `c, d, e`, the
	// mirrors will be used in the order `a, b, c, d, e`.  If the orders of mirror entries conflict
	// (e.g. `a, b` vs. `b, a`) the configuration is not rejected but the resulting order is unspecified.
	// Users who want to use a deterministic order of mirrors, should configure them into one list of mirrors using the expected order.
	// +optional
	// +listType=atomic
	ImageTagMirrors []ImageTagMirrors `json:"imageTagMirrors"`
}

// ImageTagMirrors holds cluster-wide information about how to handle mirrors in the registries config.
type ImageTagMirrors struct {
	// source matches the repository that users refer to, e.g. in image pull specifications. Setting source to a registry hostname
	// e.g. docker.io. quay.io, or registry.redhat.io, will match the image pull specification of corressponding registry.
	// "source" uses one of the following formats:
	// host[:port]
	// host[:port]/namespace[/namespace…]
	// host[:port]/namespace[/namespace…]/repo
	// [*.]host
	// for more information about the format, see the document about the location field:
	// https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md#choosing-a-registry-toml-table
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\*(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+$|^((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+)?(?::[0-9]+)?)(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?$`
	Source string `json:"source"`
	// mirrors is zero or more locations that may also contain the same images. No mirror will be configured if not specified.
	// Images can be pulled from these mirrors only if they are referenced by their tags.
	// The mirrored location is obtained by replacing the part of the input reference that
	// matches source by the mirrors entry, e.g. for registry.redhat.io/product/repo reference,
	// a (source, mirror) pair *.redhat.io, mirror.local/redhat causes a mirror.local/redhat/product/repo
	// repository to be used.
	// Pulling images by tag can potentially yield different images, depending on which endpoint we pull from.
	// Configuring a list of mirrors using "ImageDigestMirrorSet" CRD and forcing digest-pulls for mirrors avoids that issue.
	// The order of mirrors in this list is treated as the user's desired priority, while source
	// is by default considered lower priority than all mirrors.
	// If no mirror is specified or all image pulls from the mirror list fail, the image will continue to be
	// pulled from the repository in the pull spec unless explicitly prohibited by "mirrorSourcePolicy".
	// Other cluster configuration, including (but not limited to) other imageTagMirrors objects,
	// may impact the exact order mirrors are contacted in, or some mirrors may be contacted
	// in parallel, so this should be considered a preference rather than a guarantee of ordering.
	// "mirrors" uses one of the following formats:
	// host[:port]
	// host[:port]/namespace[/namespace…]
	// host[:port]/namespace[/namespace…]/repo
	// for more information about the format, see the document about the location field:
	// https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md#choosing-a-registry-toml-table
	// +optional
	// +listType=set
	Mirrors []ImageMirror `json:"mirrors,omitempty"`
	// mirrorSourcePolicy defines the fallback policy if fails to pull image from the mirrors.
	// If unset, the image will continue to be pulled from the repository in the pull spec.
	// sourcePolicy is valid configuration only when one or more mirrors are in the mirror list.
	// +optional
	MirrorSourcePolicy MirrorSourcePolicy `json:"mirrorSourcePolicy,omitempty"`
}
```

An example ImageTagMirrorSet file will look like:
```yaml
apiVersion: config.openshift.io/v1
kind: ImageTagMirrorSet
metadata:
  name: ubi8repo
spec:
  ImageTagMirrors: 
  - mirrors:
    - example.io/example/ubi-minimal
    source: registry.access.redhat.com/ubi8/ubi-minimal
	mirrorSourcePolicy: AllowContactingSource
```

The ImageTagMirrorSet object will lead to the creation of a drop-in file at `/etc/containers/registries.conf` by MCO containerruntime controller that there are no
digest only limitations on the specified mirrors, allowing images to be pulled using tags.

### User Stories

#### As a user, I would like to pull images from mirror using tag, without digest references

The user will need to define multiple mirror configurations, used by apps/manifests that don't use digests to pull
the images when working with disconnected environments or pulling from registries that act as transparent pull-through proxy caches.

For users on a cluster that already supports ImageTagMirrorSet and ImageDigestMirrorSet, the user can pull images without digests by configuring mirrors using `ImageTagMirrorSet` CRD.
For users with upgraded clusters that support the `ImageTagMirrorSet` CRD, if they had ICSP objects, the ICSP objects still take
effect. The user will need to configure mirrors using `ImageTagMirrorSet` CRD on the upgraded cluster to use tags.
The MCO will consume the ImageTagMirrorSet object. Once this is done, the images can be pulled from the mirrors without the digest referenced.

#### As a user， I would like to pull image from mirrors and block the repository in the pull spec

The user can set mirrorSourcePolicy: NeverContactSource in ImageTagMirrorSet or ImageDigestMirrorSet, depending on which CR the user uses to configure the mirror. The image will still use mirrors to pull image, but the pull will not be redirected to the pull spec if the
mirrors fail.

#### As a user using ICSP, I would like to use ICSP pull images using digest by default from mirrors

The user can still define ICSP CR before its deprecation. After the deprecation of ICSP, the user will need to use
`ImageDigestMirrorSet` for digest pull specification requirement.

### API Extensions
- Adds new CRD ImageDigestMirrorSet to config.openshift.io/v1.
- Adds new CRD ImageTagMirrorSet to config.openshift.io/v1
- Impacts on existing resources(The impacts are in the "Operational Aspects
of API Extensions" section)
  - Components currently using ImageContentSourcePolicy should upgrade to use ImageDigestMirrorSet.

### Implementation Details/Notes/Constraints [optional]

Implementing this enhancement requires changes in following components (should use underlying code from c/image/pkg/sysregistriesv2 or similarly-centralized code):
- [containers/image](https://github.com/containers/image): registries.conf needs change its mirror setting to make digest-only/tag-only configurable for individual mirrors, not just the the main registry. This can be achieved by having a boolean for each mirror table, or having separated mirror lists: a list of mirrors that allow for tag specification,
and a list of mirrors require digest specification, or have different boolean value.
- [openshift/api/config/v1](https://github.com/openshift/api/tree/master/config/v1): definition the schema and API of the ImageDigestMirrorSet and ImageTagMirrorSet.
- [openshift/client-go](https://github.com/openshift/client-go), [openshift/cluster-config-operator](https://github.com/openshift/cluster-config-operator/pull/220): rebase the openshift/api version in these repositories to apply the new CRD
to the cluster.
- [openshift/runtime-utils/pkg/registries](https://github.com/openshift/runtime-utils/tree/master/pkg/registries): helper functions to edit registries.conf.
- [openshift/machine-config-operator](https://github.com/openshift/machine-config-operator): the container runtime config controller that currently watches ICSP will
also need to watch for the ImageDigestMirrorSet and ImageTagMirrorSet. The machine-config-operator/pkg/controller/container-runtime-config controller needs to operate the ImageDigestMirrorSet and ImageTagMirrorSet CRDs.
- A new controller will be implemented to convert the existing ImageContentSourcePolicy objects to objects of new ImageDigestMirrorSet.
- [openshift/kubernetes/openshift-kube-apiserver/admission](https://github.com/openshift/kubernetes/tree/master/openshift-kube-apiserver/admission): add a webhook to convert from ImageConentSourcePolicy API to ImageDigestMirrorSet API
- This [document](https://docs.google.com/document/d/11FJPpIYAQLj5EcYiJtbi_bNkAcJa2hCLV63WvoDsrcQ/edit?usp=sharing) keeps a list of components that use operator.openshift.io/v1alpha1 ImageContentSourcePolicy. Need to change those repositories to upgrade to ImageDigestMirrorSet.

Phase 1： 
- Add ImageDigestMirrorSet, ImageTagMirrorSet to openshift/api
- Finish underlying implementation to support the per-mirror level tag/digest pull configuration.

Phase 2: 
- implement the migration path: 
  - implement a controller that can copy existing ImageContentSourcePolicy objects to ImageDigestMirrorSet objects. 
  - implement the webhook to do the conversion from operator.openshift.io/v1alpha1/imagecontentsourcepolicies to config.openshift.io/v1/imagedigestmirrorsets
- components listed in the above [document](https://docs.google.com/document/d/11FJPpIYAQLj5EcYiJtbi_bNkAcJa2hCLV63WvoDsrcQ/edit?usp=sharing) migrate to new APIs ImageDigestMirrorSet, ImageTagMirrorSet
- register and expose the CRDs.

#### Implementation updates
Actual implementation for migration path of Phase 2 was different from the original design:

The webhook is not implementable. It cannot convert between different kind of resources. 

If copy existing ImageContentSourcePolicy content to ImageDigestMirrorSet and allow both kind of objecting existing, the final `registries.conf` will lead to the customer implying only one of the object is honored. For example,

ICSP `repositoryDigestMirrors` has:
```yaml
source: foo
mirrors: 
- a
- b
- c
```

IDMS `ImageDigestMirrors` has:
```yaml
source: foo
mirrors:
- c
- b
- a
```
The registries.conf will be:
```toml
[[registry]]
  location = "foo"
  [[registry.mirror]]
    location = "a"
	pull-from-mirror = "digest-only"
  [[registry.mirror]]
    location = "b"
	pull-from-mirror = "digest-only"
  [[registry.mirror]]
    location = "c"
	pull-from-mirror = "digest-only"
```
The order of [[registry.mirror]] is [a, b, c] and will imply the ICSP is honored.

For the above reason the actual implementation for migration path:
- implement the migration path:
  - [openshift/kubernetes#1310](https://github.com/openshift/kubernetes/pull/1310) prohibit coexistence of ICSP and IDMS objects, or ICSP and ITMS objects.
  - [openshift/oc#1238](https://github.com/openshift/oc/pull/1238) implemented oc command that can convert ImageContentSourcePolicy yaml to ImageDigestMirrorSet yaml. The command can be used by customer when they want to migrate from ICSP to IDMS resources.

#### Update the implementation for migration path
The ImageContentSourcePolicy(ICSP) follows the rules from [Understanding API tiers](https://docs.openshift.com/container-platform/4.13/rest_api/understanding-api-support-tiers.html). Tier 1 component
`imagecontentsourcepolicy.operator.openshift.io/v1alpha1` should be stable within OCP 4.x. We will support it during all of 4.x and mark it deprecated and encourage users to move to IDMS while supporting both in the cluster, but will not remove ICSP in OCP 4.x.

Migration plan:
1. oc command can generate IDMS yaml files.
2. creates IDMS objects without removing ICSP. The customer can remove ICSP objects after IDMS objects get rolled out. This will result in same `registry.conf` and hence not result in a reboot.

Propose the following updates: 
- revert the [openshift/kubernetes#1310](https://github.com/openshift/kubernetes/pull/1310) to allow both ICSP and IDMS objects exist on the cluster.
- both the source and mirror mappings from ICSP and IDMS objects will be written to `/etc/containers/registries.conf`. The mirrors of the same source will be merged by topological sorting as below [Notes(2)](#notes).
- [OCPNODE-1771](https://issues.redhat.com/browse/OCPNODE-1771) epic keeps track of the tickets needed for the above migration plan.

#### Notes

1. During the upgrade path, the container runtime config controller of MCO can watch for both old CR ImageContentSourcePolicy and new CRs and create objects.

2. The merge order of mirrors for the same source is deterministic by topological sorting:<br/>
Order is preserving the relative order of the mirrors using topological sorting. A graph is formed using each mirror as a
vertex, adding an unidirectional edge between adjacent mirrors from the same mirror list. If the graph is cyclic graph, the topological sort will start
from the first vertex after the alphabetical sorting of the vertices. If multiple vertices is from the same vertex, they will be
visited in alphabetical order.

3. The ImageStream does not support pulling using tags yet, however, work has started on this feature.

### Risks and Mitigations

For users that opt into using this feature, pulling images from mirror registries without the digest specifications could lead to returning a different image version from a different registry if the image tag mapping is out of sync. However, OpenShift core is not exposed to this risk because it requires
images using digests to avoid skew and won't consume this feature at all.

## Design Details

### Test Plan

Update the container runtime config controller unit tests that are currently in the MCO to verify that registries.conf
requires digest specification for mirrors configured through ImageDigestMirrorSet.

Update the container runtime config controller unit tests that are currently in the MCO to verify that registries.conf does not
require digest specification for mirrors configured through ImageTagMirrorSet.

Update the container runtime config controller unit tests that are currently in the MCO to verify that registries.conf blocks
the primary registry of the mirrors.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end and verify that there are no regressions
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

#### Removing a deprecated feature

- A Jira card [OCPNODE-717](https://issues.redhat.com/browse/OCPNODE-717) was created to record the upgrade of ImageDigestMirrorSet, and ImageTagMirrorSet. The [repositories](https://docs.google.com/document/d/11FJPpIYAQLj5EcYiJtbi_bNkAcJa2hCLV63WvoDsrcQ/edit?usp=sharing) currently rely on operator/v1alpha1 ImageContentSourcePolicy(ICSP) will be migrated to config/v1 ImageDigestMirrorSet.

- The ImageContentSourcePolicy(ICSP) will follow the rules from [Deprecating an entire component](https://docs.openshift.com/container-platform/4.13/rest_api/understanding-api-support-tiers.html). Tier 1 component
`imagecontentsourcepolicy.operator.openshift.io/v1alpha1` is stable within a major release. We will mark it deprecated and encourage users to move to IDMS while supporting both in the cluster, but they will not be removed in OCP 4.x.  

### Upgrade / Downgrade Strategy

#### Upgrade Strategy

An existing cluster is required to make an upgrade to 4.13 in order to make use of the allow mirror by tags feature.

Migration path: 
- a controller will copy existing ImageContentSourcePolicy objects to create new ImageDigestMirrorSet objects. 
- a webhook will do the conversion from operator.openshift.io/v1alpha1/imagecontentsourcepolicies to config.openshift.io/v1/imagedigestmirrorsets.

#### Downgrade Strategy
According to [Deprecating an entire component](https://docs.openshift.com/container-platform/4.13/rest_api/understanding-api-support-tiers.html#deprecating-entire-component_understanding-api-tiers), Tier 1 component
`imagecontentsourcepolicy.operator.openshift.io/v1alpha1` is stable within a major release. it will not be removed in OCP 4.x. During the upgrade path,
if an 4.13 upgrade fails mid-way through, or if the 4.13 cluster is
misbehaving, the user can rollback to the version that supports `ImageContentSourcePolicy`.
Their `ImageDigestMirrorSet` and `ImageTagMirrorSet` dependent workflows will be clobbered and broken if rollback to a version
that lacks support for these CRDs. They can still configure the `ImageContentSourcePolicy` to use mirrors and keep previous behavior.

### Version Skew Strategy

Upgrade skew will not impact this feature. The MCO does not require skew check.

### Operational Aspects of API Extensions

Impacts of API extensions:

This [document](https://docs.google.com/document/d/11FJPpIYAQLj5EcYiJtbi_bNkAcJa2hCLV63WvoDsrcQ/edit?usp=sharing) keeps a list of components that use operator.openshift.io/v1alpha1 ImageContentSourcePolicy.
Components must be upgraded to use ImageDigestMirrorSet before the exposure of ImageDigestMirrorSet, ImageTagMirrorSet CRDs.
Jira card  [OCPNODE-717](https://issues.redhat.com/browse/OCPNODE-717) will record the upgrade of ImageDigestMirrorSet.
- [openshift/machine-config-operator](https://github.com/openshift/machine-config-operator/search?q=imagecontentsourcepolicy)
- [openshift/runtime-utils](https://github.com/openshift/runtime-utils/search?q=imagecontentsourcepolicy)
- [openshift/installer](https://github.com/openshift/installer/search?q=imagecontentsourcepolicy)
- [openshift/hypershift](https://github.com/openshift/hypershift/search?q=imagecontentsourcepolicy)
- [openshift/image-registry](https://github.com/openshift/image-registry/search?q=imagecontentsourcepolicy)
- [openshift/cluster-config-operator](https://github.com/openshift/cluster-config-operator/search?q=imagecontentsourcepolicy)
- [openshift/openshift-apiserver](https://github.com/openshift/openshift-apiserver/search?q=imagecontentsourcepolicy)
- [openshift/openshift-controller-manager](https://github.com/openshift/openshift-controller-manager/search?q=imagecontentsourcepolicy)
- [openshift/oc](https://github.com/openshift/oc/search?q=imagecontentsourcepolicy)
- [openshift/oc-mirror](https://github.com/openshift/oc-mirror/search?q=imagecontentsourcepolicy)
- [openshift/verification-tests](https://github.com/openshift/verification-tests/search?q=imagecontentsourcepolicy)
- [openshift/release](https://github.com/openshift/release/search?q=imagecontentsourcepolicy)
- [openshift/assisted-service](https://github.com/openshift/assisted-service/search?q=imagecontentsourcepolicy)
- [openshift/cincinnati-operator](https://github.com/openshift/cincinnati-operator/search?q=imagecontentsourcepolicy)
- [openshift/openshift-tests-private](https://github.com/openshift/openshift-tests-private/search?q=imagecontentsourcepolicy)
- [openshift/console](https://github.com/openshift/console/search?q=imagecontentsourcepolicy)
- [openshift/assisted-service](https://github.com/openshift/assisted-service/search?q=imagecontentsourcepolicy)




#### Failure Modes

Failure of applying new ImageDigestMirrorSet and ImageTagMirrorSet should not impact the cluster health.
If there are existing ImageDigestMirrorSet, ImageTagMirrorSet or ImageContentSourcePolicy objects, they
should continue to work.
OCP Node team is likely to be called upon in case of escalation with one of the failure modes.

#### Support Procedures

If the cluster fails to apply new CRs, the newly added CRD will not be rolled out to the cluster.
The machine-config-controller logs will show errors for debugging issues.
For example, the failure of json format, the corresponding error in the log is "could not encode registries Ignition",

Failure can be caused by getting unexpected images when using tag pull specification when the mirror registry does not synchronize the images. CRI-O info level logs will show which mirror is used to pull an image to help debugging the problem.

## Implementation History

- [openshift/api#874: Create ImageContentPolicy CRD to config/v1 and allowMirrorByTags support](https://github.com/openshift/api/pull/874)
- [openshift/api#1126: Add CRD ImageDigestMirrorSet and ImageTagMirrorSet](https://github.com/openshift/api/pull/1126)
- [openshift/api#1164: Do not register new ICSP CRDs, yet](https://github.com/openshift/api/pull/1164)
- [containers/image#1411: Add pull-from-mirror for adding per-mirror level restrictions](https://github.com/containers/image/pull/1411)
- [OCPNODE-1771: allow both CRD for migration ICSP resources to IDMS](https://issues.redhat.com/browse/OCPNODE-1771) Updates the design of migration path

## Drawbacks

See [Risks and Mitigations](###risks-and-mitigations)

## Alternatives

## Infrastructure Needed [optional]
