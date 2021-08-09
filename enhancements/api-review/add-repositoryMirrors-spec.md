---
title: add-repositoryMirrors-spec
authors:
  - "@QiWang19"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-03-10
last-updated: 2021-08-09
status: implementable
---

# Add repositoryMirrors spec to cluster wide ImageContentSourcePolicy

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today, the ImageContentSourcePolicy object sets up mirror with `mirror-by-digest-only` property set to true. Requires images be pulled by digest only. This enhancement plans to add new spec to ImageContentSourcePolicy with configurable field `allowMirrorByTags`, so that `mirror-by-digest-only` can be configured by ImageContentSourcePolicy.

## Motivation

Today, the ImageContentSourcePolicy object sets up mirror configuration with `mirror-by-digest-only` property set to true, which leads to using mirrors only when images are referenced by digests. However, when working with disconnected environments, sometimes multiple ImageContentSourcePolicies are needed, some of them are used by apps/manifests that don't use digests when pulling the images.

Adding the `repositoryMirrors` spec to ImageContentSourcePolicy to configure mirrors will make it possible to pull images from mirror using tags without requiring image digest reference.

### Goals

- Enable the user to pull images through ImageContentSourcePolicy configured mirrors by tags.

### Non-Goals

- This proposal does not recommend using by-tag references for OpenShift release images. Those should still be referenced by digest, regardless of whether `allowMirrorByTags` is enabled for repositories where the release images are mirrored

## Proposal

The new spec `repositoryMirrors` will be added to ImageContentSourcePolicy. The `allowMirrorByTags` property will allow for mirroring by-tag images through configured mirrors.

```go
// RepositoryMirrors holds cluster-wide information about how to handle mirrors in the registries config.
// Note: this is different from the RepositoryDigestMirrors that the mirrors only 
// work when pulling the images that are referenced by their digests.
type RepositoryMirrors struct {
    // source is the repository that users refer to, e.g. in image pull specifications.
    // +required
    Source string `json:"source"`
    // If true, mirrors will only be used for digest pulls. Pulling images by
    // tag can potentially yield different images, depending on which endpoint
    // we pull from.  Forcing digest-pulls for mirrors avoids that issue.
    // +optional
    AllowMirrorByTags bool `json:"allowMirrorByTags"`
    // mirrors is one or more repositories that may also contain the same images.
    // The order of mirrors in this list is treated as the user's desired priority, while source
    // is by default considered lower priority than all mirrors. Other cluster configuration,
    // including (but not limited to) other repositoryMirrors objects,
    // may impact the exact order mirrors are contacted in, or some mirrors may be contacted
    // in parallel, so this should be considered a preference rather than a guarantee of ordering.
    // +optional
    Mirrors []string `json:"mirrors"`
}
```

The ImageContentSourcePolicy CRD has the cluster-wide information about handle the mirrors only when pulling the images that are referenced by their digests.
It will now watch for mirrors configured through spec `repositoryMirrors` to not require digest reference.

An example ImageContentSourcePolicy file will look like:
```yaml
apiVersion: operator.openshift.io/v1alpha2
kind: ImageContentSourcePolicy
metadata:
  name: ubi8repo
spec:
  repositoryMirrors:
  - mirrors:
    - example.io/example/ubi-minimal 
    source: registry.access.redhat.com/ubi8/ubi-minimal
    allowMirrorByTags: false # or equivalently, by leaving allowMirrorByTags unset
```

### User Stories

#### As a user, I would like to pull images from mirror using tag, without digests reference
The user need to define multiple ImageContentSourcePolicies, used by apps/manifests that don't use digests to pull
the images when working with disconnected environments or pulling from registries that act as transparent pull-through proxy cache.
The user can pull image without digest by configuring `repositoryMirrors` spec in the ImageContentSourcePolicy file. For users still use `repositoryDigestMirrors` 
from operator.openshift.io/v1alpha1, the conversion webhook will automatically convert the
ImageContentSourcePolicies to operator.openshift.io/v1alpha2 and enable using
`oc edit imagecontentsourcepolicy.operator.openshift.io` to configure `repositoryMirrors`.
And create the ImageContentSourcePolicy project. Once this is done, the images can be pulled from the mirrors without the digest referenced.

### Implementation Details/Notes/Constraints [optional]

Implementing this enhancement requires changes in:
- openshift/api
- openshift/machine-config-operator

This is an example of the ImageContentSourcePolicy file:

```yaml
apiVersion: operator.openshift.io/v1alpha2
kind: ImageContentSourcePolicy
metadata:
  name: ubi8repo
spec:
  repositoryMirrors:
  - mirrors:
    - example.io/example/ubi-minimal 
    source: registry.access.redhat.com/ubi8/ubi-minimal 
```

The ImageContentSourcePolicy file will create a drop-in file at `/etc/containers/registries.conf` that looks like:

```toml
unqualified-search-registries = ["registry.access.redhat.com", "docker.io"]
[[registry]]
  location = "registry.access.redhat.com/ubi8/"
  insecure = false
  blocked = false
  # No mirror-by-digest-only = true configured
  prefix = ""

  [[registry.mirror]]
    location = "example.io/example/ubi8-minimal"
    insecure = false
```

### Risks and Mitigations

Pulling images from mirror registries without the digest specifications could lead to returning different image version from different registry if the image tag mapping is out of sync. But the OpenShift core required image using digests to avoid different versions won't consume this feature at all, so it is not exposed to the risks that anyone who actually uses the feature will be exposed to.

## Design Details

### Test Plan

Update the tests that are currently in the MCO to verify that `mirror-by-digest-only` not have been set when spec repositoryMirrors of the ImageContentSourcePolicy is created.

### Graduation Criteria

GA in openshift will be looked at to
determine graduation.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
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

- With `repositoryMirrors` getting in, the current `repositoryDigestMirrors` can be deprecated since its functionality will also be fully satisfied by `repositoryMirrors` and we don't have to keep duplicated APIs.
- In operator.openshift.io/v1alpha1, gently announce to the users the `repositoryDigestMirrors` will be deprecated. In operator.openshift.io/v1alpha2, the implementation of `repositoryDigestMirrors` will be removed and replaced by `repositoryMirrors`.
- ImageContentSourcePolicy CRD version conversion strategy: A conversion webhook needs to be created and deployed for deprecating the `repositoryDigestMirrors` and migrating users to new `repositoryMirrors`.

### Upgrade / Downgrade Strategy

Upgrades should not be affect. But the downgrade will be affected.
Users upgraded and configured the `repositoryMirrors` spec will presumably have their CRI-O configurations clobbered and break their tag-mirroring-dependent workflows after they downgrade to a version that lacks support for the `repositoryMirrors` spec.
We will not change current default behavior of `repositoryDigestMirrors`. We are just add new spec so users can configure it through the ImageContentSourcePolicy after the upgrade.

### Version Skew Strategy

Upgrade skew will not impact this feature. The MCO does not require skew check. CRI-O with n-2 OpenShift skew will still be able to handle the new property.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]
