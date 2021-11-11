---
title: add-new-CRD-ImageSourceDigestPolicy-and-ImageSourceTagPolicy-to-config.openshift.io
authors:
  - "@QiWang19"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
  - "@sttts"
  - "@oscardoe"
creation-date: 2021-03-10
last-updated: 2021-12-10
status: implementable
---

# Add CRD ImageSourceDigestPolicy and ImageSourceTagPolicy to config.openshift.io/v1

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today, the ImageContentSourcePolicy(ICSP) object sets up mirror with `mirror-by-digest-only` property set to true. Requires images be pulled by digest only. This enhancement introduces new CRDs to config/v1:
- ImageSourceDigestPolicy: holds mirror configurations that are used to pull image from mirrors by digest specification only.
- ImageSourceTagPolicy: holds mirror configurations that allow pulling image from mirrors using tag specification.

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

- Enable the user to pull images using digest only from mirrors configured through config.openshift.io/v1 API ImageSourceDigestPolicy.
- Enable the user to pull images using tags from mirrors configured through config.openshift.io/v1 API ImageSourceTagPolicy.

### Non-Goals

- This proposal does not recommend using by-tag references for OpenShift release images. Those should still be referenced by digest, regardless of whether `ImageSourceTagPolicy` is configured for repositories where the release images are mirrored

## Proposal

The New CRD `ImageSourceDigestPolicy` will be added to config.openshift.io/v1.
The schema of `ImageSourceDigestPolicy` is same as `ImageContentSourcePolicy`.

```go
// ImageSourceDigestPolicy is the specification of the ImageSourceDigestPolicy CRD.
type ImageSourceDigestPolicySpec struct {
	// repositoryDigestMirrors allows images referenced by image digests in pods to be
	// pulled from alternative mirrored repository locations. The image pull specification
	// provided to the pod will be compared to the source locations described in RepositoryDigestMirrors
	// and the image may be pulled down from any of the mirrors in the list instead of the
	// specified repository allowing administrators to choose a potentially faster mirror.
	// To pull image from mirrors using tag specification, should configure 
	// a list of mirrors using "ImageSourceTagPolicy" CRD.
	//
	// If the image pull specification matches the reposirtory of "source" in multiple policies,
	// only the policies have the most specific namespace matching will be used.
	// For example, if there are policies using quay.io/libpod and quay.io/libpod/busybox as
	// the "source", only the policies using quay.io/libpod/busybox are going to apply
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
	// If want to use a deterministic order of mirrors, configure them into one list of mirrors using expected order.
	// +optional
	// +listType=atomic
	RepositoryDigestMirrors []RepositoryDigestMirrors `json:"repositoryDigestMirrors"`
}

// RepositoryDigestMirrors holds cluster-wide information about how to handle mirrors in the registries config.
type RepositoryDigestMirrors struct {
	// source is the repository that users refer to, e.g. in image pull specifications.
	// "source" ueses one of the following formats:
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
	// mirrors is zero or more repositories that may also contain the same images.
	// Images can be pulled from these mirrors only if they are referenced by their digests.
	// If the "mirrors" is not specified, the image will continue to be pulled from the specified
	// repository in the pull spec. No mirror will be configured.
	// The order of mirrors in this list is treated as the user's desired priority, while source
	// is by default considered lower priority than all mirrors. Other cluster configuration,
	// including (but not limited to) other repositoryDigestMirrors objects,
	// may impact the exact order mirrors are contacted in, or some mirrors may be contacted
	// in parallel, so this should be considered a preference rather than a guarantee of ordering.
	// "mirrors" ueses one of the following formats:
	// host[:port]
	// host[:port]/namespace[/namespace…]
	// host[:port]/namespace[/namespace…]/repo
	// for more information about the format, see the document about the location field:
	// https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md#choosing-a-registry-toml-table
	// +optional
	// +listType=set
	Mirrors []Mirror `json:"mirrors,omitempty"`
}

// +kubebuilder:validation:Pattern=`^((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+)?(?::[0-9]+)?)(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?$`
type Mirror string
```

An example ImageSourceDigestPolicy file will look like:
```yaml
apiVersion: config.openshift.io/v1
kind: ImageSourceDigestPolicy
metadata:
  name: ubi8repo
spec:
  repositoryDigestMirrors: 
  - mirrors:
    - example.io/example/ubi-minimal
    source: registry.access.redhat.com/ubi8/ubi-minimal
```

The ImageSourceDigestPolicy object will lead to the creation of a drop-in file at `/etc/containers/registries.conf` by the MCO [Container Runtime Config Controller](https://github.com/openshift/machine-config-operator/tree/master/pkg/controller/container-runtime-config) that
limits the usage of configured mirrors to image pulls by digest only.

The New CRD `ImageSourceTagPolicy` will be added to config.openshift.io/v1.

```go
// ImageSourceTagPolicy is the specification of the ImageSourceTagPolicy CRD.
type ImageSourceTagPolicySpec struct {
	// repositoryTagMirrors allows images referenced by image tags in pods to be
	// pulled from alternative mirrored repository locations. The image pull specification
	// provided to the pod will be compared to the source locations described in RepositoryTagMirrors
	// and the image may be pulled down from any of the mirrors in the list instead of the
	// specified repository allowing administrators to choose a potentially faster mirror.
	// To pull image from mirrors using digest specification only, should configure 
	// a list of mirrors using "ImageSourceDigestPolicy" CRD.
	//
	// If the image pull specification matches the reposirtory of "source" in multiple policies,
	// only the policies have the most specific namespace matching will be used.
	// For example, if there are policies using quay.io/libpod and quay.io/libpod/busybox as
	// the "source", only the policies using quay.io/libpod/busybox are going to apply
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
	// If want to use a deterministic order of mirrors, configure them into one list of mirrors using expected order.
	// +optional
	// +listType=atomic
	RepositoryTagMirrors []RepositoryTagMirrors `json:"repositoryTagMirrors"`
}

// RepositoryTagMirrors holds cluster-wide information about how to handle mirrors in the registries config.
type RepositoryTagMirrors struct {
	// source is the repository that users refer to, e.g. in image pull specifications.
	// "source" ueses one of the following formats:
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
	// mirrors is zero or more mirror repositories that can be used to pull the images that are referenced by their tags. 
	// Pulling images by tag can potentially yield different images, depending on which endpoint we pull from.
	// Configuring a list of mirrors using "ImageSourceDigestPolicy" CRD and forcing digest-pulls for mirrors avoids that issue.
	// If the "mirrors" is not specified, the image will continue to be pulled from the specified
	// repository in the pull spec. No mirror will be configured.
	// The order of mirrors in this list is treated as the user's desired priority, while source
	// is by default considered lower priority than all mirrors. Other cluster configuration,
	// including (but not limited to) other repositoryMirrors objects,
	// may impact the exact order mirrors are contacted in, or some mirrors may be contacted
	// in parallel, so this should be considered a preference rather than a guarantee of ordering.
	// "mirrors" ueses one of the following formats:
	// host[:port]
	// host[:port]/namespace[/namespace…]
	// host[:port]/namespace[/namespace…]/repo
	// for more information about the format, see the document about the location field:
	// https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md#choosing-a-registry-toml-table
	// +optional
	// +listType=set
	Mirrors []Mirrors `json:"mirrors,omitempty"`
}

// +kubebuilder:validation:Pattern=`^((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+)?(?::[0-9]+)?)(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?$`
type Mirror string
```

An example ImageSourceTagPolicy file will look like:
```yaml
apiVersion: config.openshift.io/v1
kind: ImageSourceTagPolicy
metadata:
  name: ubi8repo
spec:
  repositoryTagMirrors: 
  - mirrors:
    - example.io/example/ubi-minimal
    source: registry.access.redhat.com/ubi8/ubi-minimal
```

The ImageSourceTagPolicy object will lead to the creation of a drop-in file at `/etc/containers/registries.conf` by MCO containerruntime controller that there are no
digest only limitations on the specified mirrors, allowing images to be pulled using tags.

### User Stories

#### As a user, I would like to pull images from mirror using tag, without digest references

The user will need to define multiple mirror configurations, used by apps/manifests that don't use digests to pull
the images when working with disconnected environments or pulling from registries that act as transparent pull-through proxy caches.

For users on a cluster that already supports ImageSourceTagPolicy and ImageSourceDigestPolicy, the user can pull images without digests by configuring mirrors using `ImageSourceTagPolicy` CRD.
For users with upgraded clusters that suppport the `ImageSourceTagPolicy` CRD, if they had ICSP objects, the ICSP objects still take
effect. The user will need to configure mirrors using `ImageSourceTagPolicy` CRD on the upgraded cluster to use tags.
The MCO will create the ImageSourceTagPolicy object. Once this is done, the images can be pulled from the mirrors without the digest referenced.

#### As a user using ICSP, I would like to use ICSP pull images using digest by default from mirrors

The user can still define ICSP CR before its deprecation. After the deprecation of ICSP, the user will need to use
`ImageSourceDigestPolicy` for digest pull specification requirement.

### API Extensions
- Adds new CRD ImageSourceDigestPolicy to config.openshift.io/v1.
- Adds new CRD ImageSourceTagPolicy to config.openshift.io/v1
- Impacts on existing resources(The impacts are in the "Operational Aspects
of API Extensions" section)
  - Components currently using ImageContentSourcePolicy should upgrade to use ImageSourceDigestPolicy.

### Implementation Details/Notes/Constraints [optional]

Implementing this enhancement requires changes in:
- [containers/images](https://github.com/containers/image): registries.conf needs change its mirror setting to make digests only configurable for mirrors registry, not for the main registry. This can be achieved by having a boolean for each mirror table, or having separated mirror lists: a list of mirrors that allow for tag specification,
and a list of mirrors require digest specification, or have different boolean value.
- [openshift/api/config/v1](https://github.com/openshift/api/tree/master/config/v1): definition the schema and API of the ImageSourceDigestPolicy and ImageSourceTagPolicy.
- [openshift/client-go](https://github.com/openshift/client-go), [openshift/cluster-config-operator](https://github.com/openshift/cluster-config-operator/pull/220): rebase the openshift/api version in these repositories to apply the new CRD
to the cluster.
- [openshift/runtime-utils/pkg/registries](https://github.com/openshift/runtime-utils/tree/master/pkg/registries): helper functions to edit registries.conf.
- [openshift/machine-config-operator](https://github.com/openshift/machine-config-operator): MCO needs watch for the ImageSourceDigestPolicy and ImageSourceDigestPolicy. The machine-config-operator/pkg/controller/container-runtime-config controller needs to operate the ImageSourceDigestPolicy and ImageSourceDigestPolicy CRDs.
Converts the existing ImageContenSourcePolicy objects to objects of new CRD.
- This [document](https://docs.google.com/document/d/11FJPpIYAQLj5EcYiJtbi_bNkAcJa2hCLV63WvoDsrcQ/edit?usp=sharing) keeps a list of components that use operator.openshift.io/v1alpha1 ImageContentSourcePolicy. Need to change those repositories to upgrade to ImageSourceDigestPolicy.

#### Notes

1. During the upgrade path, MCO can watch for both old CR ImageContentSourcePolicy and new CRs and create objects.

2. The merge order of mirrors for the same source is deterministic by topological sorting:<br/>
Order is preserving the relative order of the mirrors using topological sorting. A graph is formed using each mirror as a
vertex, adding an unidirectional edge between adjacent mirrors from the same mirror list. If the graph is cyclic graph, the topological sort will start
from the first vertex after the alphabetical sorting of the vertces. If multiple verteces is from the same vertex, they will be
visited in alphabetical order.

3. The ImageStream does not support pulling using tags yet, however, work has started on this feature.

### Risks and Mitigations

For users that opt into using this feature, pulling images from mirror registries without the digest specifications could lead to returning a different image version from a different registry if the image tag mapping is out of sync. However, OpenShift core is not exposed to this risk because it requires
images using digests to avoid skew and won't consume this feature at all.

## Design Details

### Test Plan

Update the container runtime config controller unit tests that are currently in the MCO to verify that registries.conf
requires digest specification for mirrors configured through ImageSourceDigestPolicy.

Update the container runtime config controller unit tests that are currently in the MCO to verify that registries.conf does not
require digest specification for mirrors configured through ImageSourceTagPolicy.

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

- A Jira card [OCPNODE-717](https://issues.redhat.com/browse/OCPNODE-717) was created to record the upgrade of ImageSourceDigestPolicy. The [repositories](https://docs.google.com/document/d/11FJPpIYAQLj5EcYiJtbi_bNkAcJa2hCLV63WvoDsrcQ/edit) currently rely on operator/v1alpha1 ImageContentSourcePolicy(ICSP) will be migrated to config/v1 ImageSourceDigestPolicy.

- The ImageContentSourcePolicy(ICSP) will follow the rules from [Deprecating an entire component](https://docs.openshift.com/container-platform/4.8/rest_api/understanding-api-support-tiers.html#deprecating-entire-component_understanding-api-tiers). The duration is
12 months or 3 releases from the announcement of deprecation, whichever is longer.  

### Upgrade / Downgrade Strategy

#### Upgrade Strategy

An existing cluster is required to make an upgrade to 4.11 in order to make use of the allow mirror by tags feature. Before the deprecation of ImageContentSourcePolicy, MCO needs to watch for both ImageContentSourcePolicy, and ImageSourceDigestPolicy and create objects.

Tier 1 component `imagecontentsourcepolicy.operator.openshift.io/v1alpha1` will stay 12 months or 3 releases from the announcement of deprecation.
During the development on the release that one release ahead of deprecating `imagecontentsourcepolicy.operator.openshift.io/v1alpha1`,
MCO will copy existing ImageContentSourcePolicy objects to ImageSourceDigestPolicy and create new objects, and delete the
ImageContentSourcePolicy objects. If any errors appear during the process, MCO should report `Upgradeable=False`.

On the release that the ImageContentSourcePolicy CRD is removed from the API, the MCO will update its clusteroperator object to reflect a degrade state if it still finds objects of
ImageContentSourcePolicy. The MCO should report that the ImageContentSourcePolicy is orphaned and let the user know they should create new objects
using the new ImageSourceDigestPolicy or ImageSourceTagPolicy CRDs.

#### Downgrade Strategy
According to [Deprecating an entire component](https://docs.openshift.com/container-platform/4.8/rest_api/understanding-api-support-tiers.html#deprecating-entire-component_understanding-api-tiers), Tier 1 component
`imagecontentsourcepolicy.operator.openshift.io/v1alpha1` will stay 12 months or 3 releases from the announcement of deprecation, whichever is longer. During the upgrade path,
if an 4.11 upgrade fails mid-way through, or if the 4.11 cluster is
misbehaving, the user can rollback to the version that supports `ImageContentSourcePolicy`.
Their `ImageSourceDigestPolicy` and `ImageSourceTagPolicy` dependent workflows will be clobbered and broken if rollback to a version
that lacks support for these CRDs. They can still configure the `ImageContentSourcePolicy` to use mirrors and keep previous behavior.

### Version Skew Strategy

Upgrade skew will not impact this feature. The MCO does not require skew check. CRI-O with n-2 OpenShift skew will still be able to handle the new property.

### Operational Aspects of API Extensions

Impacts of API extensions:
- Components that must be upgraded to use ImageSourceDigestPolicy:
  - [Installer](https://github.com/openshift/installer/blob/6d778f911e79afad8ba2ff4301eda5b5cf4d8e9e/pkg/asset/manifests/imagesourcepolicy.go#L18)
  - [Hypershift](https://github.com/openshift/hypershift/blob/baceec23098d39af089b06c503425c3bbee554d3/control-plane-operator/controllers/hostedcontrolplane/manifests/imagecontentsource.go)
  - [MCO](https://github.com/openshift/machine-config-operator) The machine-config-operator/pkg/controller/container-runtime-config controller needs to operate the ImageSourceDigestPolicy and ImageSourceDigestPolicy CRDs.
  - [Image-registry](https://github.com/openshift/image-registry/blob/a87e6c50cd973723de8b5471453de7c345403d56/pkg/dockerregistry/server/simplelookupicsp.go#L60)
  - [Cluster-config-operator](https://github.com/openshift/cluster-config-operator)
  - [Openshift-api-server](https://github.com/openshift/openshift-apiserver/blob/98786f917ffc7d3dc3b05893f405970b87a419b9/pkg/image/apiserver/registries/registries.go)
  - [Runtime utils](https://github.com/openshift/runtime-utils/blob/8b8348d80d1d1e7b6cf06fb009d5965e0b55baa2/pkg/registries/registries.go#L50)
  - [Openshift-controller-manager](https://github.com/openshift/openshift-controller-manager/blob/2a11f145ad7fcf3e92460800de1d13ba7fbb90b0/pkg/build/controller/build/build_controller.go#L20943)

The Node team will be making changes to the above impacted components to enable the new CRDs. Planning target CY21Q4
Jira card  [OCPNODE-717](https://issues.redhat.com/browse/OCPNODE-717) will record the upgrade of ImageSourceDigestPolicy.

#### Failure Modes

Failure of applying new ImageSourceDigestPolicy and ImageSourceTagPolicy should not impact the cluster health.
If there are existing ImageSourceDigestPolicy, ImageSourceTagPolicy or ImageContenSourcePolicy objects, they
should continue to work.
OCP Node team is likely to be called upon in case of escalation with one of the failure modes.

#### Support Procedures

If the cluster fails to apply new CRs, the condition of the object will be `Failed`.
The newly added CRD will not be rolled out to the cluster.
The machine-config-controller logs will show errors for debugging issues.
For example, the failure of json format, the corresponding error in the log is "could not encode registries Ignition",

Failure can be caused by getting unexpected images when using tag pull specification when the mirror registry does not synchronize the images. Crio info level logs will show which mirror is used to pull an image to help debug the problem.

## Implementation History

- [openshift/api#874: Create ImageContentPolicy CRD to config/v1 and allowMirrorByTags support](https://github.com/openshift/api/pull/874) PR was merged.
## Drawbacks

See [Risks and Mitigations](###risks-and-mitigations)

## Alternatives

## Infrastructure Needed [optional]
