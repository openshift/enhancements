---
title: available-update-metadata
authors:
  - "@wking"
reviewers:
  - "@abhinavdahiya"
  - "@LalatenduMohanty"
  - "@smarterclayton"
approvers:
  - "@LalatenduMohanty"
  - "@sdodson"
  - "@smarterclayton"
creation-date: 2019-11-19
last-updated: 2020-08-05
status: implemented
see-also:
  - "/enhancements/update/channel-metadata.md"
---

# Available-update Metadata

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Currently the ClusterVersion API has [a single `Update` type][api-update] used for both [the `desiredUpdate` input][api-desiredUpdate] and the [`desired`][api-desired] and [`availableUpdates`][api-availableUpdates] outputs.
But update requests like `desiredUpdate` have different schema requirements than output like `desired` and `availableUpdates`.
For example, [the `force` property][api-force] makes sense in the `desiredUpdate` context, but not in the `availableUpdates` contexts.
And release metadata like related URIs and available channels makes sense in the `desired` and `availableUpdates` contexts, but not in the `desiredUpdate` context.
This enhancement adds a new `Release` type to be used in `status` properties so we can distinguish between input and output schema.

## Motivation

[Cincinnati][] provides [metadata about releases][cincinnati-metadata], including (since [here][cincinnati-channel-metadata]) the channels to which the release currently belongs.
Exposing that information to in-cluster consumers allows them to use live metadata, instead of talking to Cincinnati directly or hard-coding expected values.
For example, the web console currently [hard-codes an expected set of available channels][console-channels] for a given release; with this enhancement they could look up available channels in `status.desired.channels`.

### Goals

* Exposing channel and related URI metadata provided by the upstream update recommendation service for the currently-desired and available-update releases.
* Documenting the well-known metadata keys for channels and related URIs.

### Non-Goals

* Restricting metadata keys to those documented in this enhancement.
    Update recommendation services such as [Cincinnati][] may continue to add and remove keys as they see fit as long as they do not use different semantics for the keys defined by this enhancement.
* Propagating metadata keys beyond those documented in this enhancement.
    As discussed in [the *pass-through metadata maps* section](#pass-through-metadata-maps), capturing additional information from Cincinnati metadata will require additional enhancements and new `Release` properties.
* Teaching Cincinnati to support requests without `channel` query parameters.
    This might be useful for recovering from users specifying [`desiredUpdate`][api-desiredUpdate] that were not in their configured [`channel`][api-channel], but I am fine leaving recovery to the users.
    They should have received sufficient information to adjust their channel from wherever they received the target release they are supplying.

## Proposal

As proposed in [this API pull request][api-pull-request], this enhancement adds a new type:

```go
// Release represents an OpenShift release image and associated metadata.
// +k8s:deepcopy-gen=true
type Release struct {
  // version is a semantic versioning identifying the update version. When this
  // field is part of spec, version is optional if image is specified.
  // +required
  Version string `json:"version"`

  // image is a container image location that contains the update. When this
  // field is part of spec, image is optional if version is specified and the
  // availableUpdates field contains a matching version.
  // +required
  Image string `json:"image"`

  // url contains information about this release. This URL is set by
  // the 'url' metadata property on a release or the metadata returned by
  // the update API and should be displayed as a link in user
  // interfaces. The URL field may not be set for test or nightly
  // releases.
  // +optional
  URL URL `json:"url,omitempty"`

  // channels is the set of Cincinnati channels to which the release
  // currently belongs.
  // +optional
  Channels []string `json:"channels,omitempty"`
}
```

and to use that type instead of the current `Update` for `ClusterVersionStatus` properties.
Specifically for each `ClusterVersionStatus` property:

* [`desired`][api-desired] needs to use `Release` to support [dynamic channel selection](#dynamic-channel-selection), because "which channels does my current version belong to?" is a question about the release we're currently reconciling towards.
* [`availableUpdates`][api-availableUpdates] needs to use `Release` to support [available-update related URIs](#available-update-related-uris), because "what do I gain from updating to `${VERSION}`?" is a question about a particular available update.

Then, in the cluster-version operator (CVO), this enhancement proposes porting existing logic like [available-update translation][cluster-version-operator-update-translation] and [available-update lookup][cluster-version-operator-update-lookup] to preserve the Cincinnati metadata, with information from the release image itself taking precedence over information from Cincinnati.
In some cases (when an administrator requested an update that was not in the available update set), this would require an additional Cincinnati request to retrieve metadata about the requested release.

#### Metadata properties

This enhancement declares the following to be well-known `metadata` properties in Cincinnati node objects:

* `url` (optional), contains information about this release.
* `io.openshift.upgrades.graph.release.channels` (optional), the comma-delimited set of channels to which the release currently belongs.

Cincinnati implementations may, at their discretion, define, add, and remove additional properties as they see fit.

If Cincinnati removes those properties or assigns different semantics to those properties, then we will bump `apiVersion` on the cluster-side custom resource.
That would be painful, so I'm glad there is nothing on the horizon that would lead to Cincinnati making changes like that.

### User Stories

#### Dynamic Channel Selection

Alice's cluster is on release 4.1.23, and she wants to know if she can update to 4.2.
With interfaces like the web console and `oc adm upgrade ...` exposing:

```console
$ curl -sH 'Accept:application/json' 'https://api.stage.openshift.com/api/upgrades_info/v1/graph?channel=stable-4.1&version=4.1.23' | jq -r '.nodes[] | select(.version == "4.1.23").metadata["io.openshift.upgrades.graph.release.channels"] | split(",")[]'
prerelease-4.1
stable-4.1
candidate-4.2
```

she could see that, while upgrading to 4.2 was possible, it was not yet considered stable.
And running the query again later, she could see that it had been added to `stable-4.2`.

The web console and `oc` would use `status.desired.channels`, to provide this information.

#### Available-Update Related URIs

Currently the web console only shows available update version names.
With this enhancement, they could also provide *View release* links, to give users convenient access to errata or other release notes describing the release image.

```console
$ curl -sH 'Accept:application/json' 'https://api.stage.openshift.com/api/upgrades_info/v1/graph?channel=stable-4.1&version=4.1.23' | jq -r '.nodes[] | select(.version == "4.1.23").metadata.url'
https://access.redhat.com/errata/RHBA-2019:3766
```

The web console and `oc` would use `status.availableUpdates[].url` to provide this information.

### Implementation Details/Notes/Constraints

If the cluster administrator sets [`desiredUpdate`][api-desiredUpdate] to an image unknown to Cincinnati and also sets [`force`][api-force] to tell the CVO to apply the update regardless of the target release not appearing in [`availableUpdates`][api-availableUpdates], there will be no Cincinnati metadata to populate the [`desired`][api-desired] status.
`image` and `version` will be copied over from `desiredUpdate`, as they [are][cluster-version-operator-findUpdateFromConfig] [now][cluster-versoin-operator-desired-copy].
The only difference is that [`force`][api-force] will no longer be propagated, as discussed in [the deprecation section](#removing-a-deprecated-feature).

When the [`desiredUpdate`][api-desiredUpdate] is found in [`availableUpdates`][api-availableUpdates] (by matching [`image`][api-update], falling back to matching by [`version`][api-update]), then Cincinnati metadata will be available under `url` and `channels` immediately.

When the [`desiredUpdate`][api-desiredUpdate] is not found in [`availableUpdates`][api-availableUpdates] but is known to Cincinnati, `metadata` and `upstreamMetadata` will initially be empty (as in the *image unknown to Cincinnati* case described above), but following release image retrieval `metadata` will be populated with the release image metadata, and following the next [`upstream`][api-upstream] request `upstreamMetadata` will be populated with the returned metadata.

### Risks and Mitigations

There is no security risk, because this is only exposing through ClusterVersion information that is already public via the release images and direct Cincinnati requests.

## Design Details

### Test Plan

There will be no testing outside of cluster-version operator unit tests, where a test harness already exists with [mock Cincinnati responses][cluster-version-operator-available-updates-handler].

### Graduation Criteria

The ClusterVersion object is already GA, so there would be nothing to graduate or space to cook a preview implementation.

##### Removing a deprecated feature

This change would remove [the `force` property][api-force] from [`desired`][api-desired] and [`availableUpdates`][api-availableUpdates].
But that property was optional before and did not have clear semantics in either location, so the removal should break no consumers.

### Upgrade / Downgrade Strategy

The YAML rendering of the old and updated type are compatible, with the only difference being optional properties, so there is no need for upgrade/downgrade logic.

### Version Skew Strategy

The YAML rendering of the old and updated type are compatible, with the only difference being optional properties, so there is no need for upgrade/downgrade logic.

## Implementation History

* [API pull request][api-pull-request].
* [CVO implementation][cluster-version-operator-pull-request], landed 2020-07-31.
* Will GA with 4.6.

## Drawbacks

Drawbacks for the chosen approach are discussed in the individual [*alternatives*](alternatives) subsections.

## Alternatives

Components like the web console could bypass ClusterVersion and talk to the upstream Cincinnati directly, but this would require significant code duplication.

### Using `homepage` instead of `url`

I prefer `homepage`, but [in this thread][homepage-vs-url], Clayton and Lala pushed back against it.
The pushback seems to revolve around a belief that domains, organizations, projects and such can have homepages, but that releases are too specific to deserve that name.
I think the homepage semantics can apply clearly to any noun, including releases as well as domains, organizations, etc.
I am comfortable with `url` as long as the declared semantics are "the most generic URI still scoped to this release, `url` is an appropriate reference for all information about the release".
I have been unable to get Clayton to agree to wording expressing these semantics so far (more discussion [here][url-docs-a] and [here][url-docs-b]), but the existing wording does not rule out those semantics, so maybe we can update the docs later to clarify this point without it being an API-breaking change.

And then `url` instead of `uri` because Kubernetes APIs have lots of URL precedent, despite RFC 3986's URIs making [the location vs. identifier distinction][rfc-3986-s1.1.3] in 2005.

### Pass-through metadata maps

Instead of explicit `url` and `channels` properties, `Release` could have a generic `Metadata map[string]string` property to pass through information extracted from Cincinnati or the release image's metadata.
That approach has the benefit of providing access to all available metadata without requiring `Release` schema changes or follow-up enhancements.

A risk in exposing metadata is that users would rely on unstable metadata properties not defined in this enhancement, and then break if and when their [`upstream`][api-upstream] removed the property.
The instability of those properties could be clearly explained in this enhancement, but that doesn't mean that users will be happy if they decide to take a risk on an unstable metadata property that is subsequently removed.
Clayton [pushed back][no-pass-through] against the pass-through approach, eventually describing it as "a tire fire".

### Distinguishing based on data source

Initial versions of this enhancement proposal recommended a single `metadata` property.
Intermediate versions split that into `metadata` and `upstreamMetadata`, because there was [concern][upstream-trust] about the relability of data retrieved from the upstream Cincinnati service.
The `upstreamMetadata` property name (and before that, [an `additionalMetadata` property name][additional-metadata-suggestion]) was [suggested][upstream-metadata-suggestion] was an attempt to address that concern by giving the upstream-sourced data a less authoritative name.
And `upstreamMetadata` without a primary `metadata` property seemed to [beg the "in addition to what?" question][additional-metadata-alone].

The benefit to the `upstreamMetadata` demotion is that it gives us space to be explicit about the chain of trust behind the asserted metadata.
The drawback is that we would have to be explicit about chain of trust in the API documentation, and it's unlikely that many consumers care about chain-of-trust distinctions for `url` or `io.openshift.upgrades.graph.release.channels`.
It is possible that future well-known metadata properties are more sensitive than [the ones documented by this enhancement](#metadata-properties).

With [*pass-through metadata maps* rejected](#pass-through-metadata-maps), I've also dropped the source distinction (i.e. I do not distinguish between `url` and `upstreamURL`).
As described in [the *proposal* section](#proposal), metadata from the release image takes precedence over metadata from the [`upstream`][api-upstream].
And because neither [property](#metadata-properties) is particularly sensitive, `url` is always set in Red Hat's release images, and `io.openshift.upgrades.graph.release.channels` is never set in Red Hat's release images (we always add it in Cincinnati), distinguishing based on source seemed like overkill.

### Capturing this information in a separate custom resource

Instead of changing the `ClusterVersionStatus` properties, we could deprecate them and make `Release` a stand-alone custom resource.
`Version` would become `ObjectMeta.Name`, and `Image` and `Metadata` would be `Status` properties.
The benefit of this approach would be a smaller, tighter ClusterVersion.
The drawback is that you need some way to figure out which is the current release and which are the available updates, which is a bit racier if you can no longer retrieve a single, atomic snapshot with a `GET ClusterVersion`.
The following subsections float some options for declaring those associations.

#### Declaring a source version

We'd also add some additional properties:

```go
// Source a the release version for which updates to this release can be launched.
// For example, source on a 4.1.1 release might be "4.1.0".
Source string `json:"source"`
```

In addition, the current release (being reconciled by the cluster-version operator) would have a `release.openshift.io/role` annotation with a `current` value.

#### Referencing from ClusterVersion with a new property

We'd add new `ClusterVersionStatus` properties like:

```
// Current is the release version which is currently being reconciled by the cluster-version operator.
Current string `json:"current,omitempty"`

// AvailableUpdatesV2 holds release versions to which the current version may be updated.
AvailableUpdatesV2 []string `json:"availableUpdatesV2,omitempty`
```

#### Referencing from `ClusterVersionStatus` with an existing property

Leave the current `ClusterVersionStatus` alone and just continue to fill the now-redundant `Update` properties (like `Image`) and use their `Version` properties instead of [*referencing from ClusterVersion with a new property*](#referencing-from-clusterversion-with-a-new-property)'s bare strings.

[additional-metadata-suggestion]: https://github.com/openshift/enhancements/pull/123#discussion_r430733692
[additional-metadata-alone]: https://github.com/openshift/enhancements/pull/123#discussion_r435451269
[api-availableUpdates]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L123-L130
[api-channel]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L61-L66
[api-desired]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L82-L87
[api-desiredUpdate]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L40-L54
[api-force]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L239-L251
[api-pull-request]: https://github.com/openshift/api/pull/521#issuecomment-555649500
[api-update]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L224-L252
[api-upstream]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L56-L60
[cincinnati]: https://github.com/openshift/cincinnati/
[cincinnati-channel-metadata]: https://github.com/openshift/cincinnati/pull/167
[cincinnati-metadata]: https://github.com/openshift/cincinnati/blob/c59f45c7bc09740055c54a28f2b8cac250f8e356/docs/design/cincinnati.md#update-graph
[cluster-version-operator-available-updates-handler]: https://github.com/openshift/cluster-version-operator/blob/751c6d0c872e05f218f01d2a9f20293b4dfcca88/pkg/cvo/cvo_test.go#L2284
[cluster-versoin-operator-desired-copy]: https://github.com/openshift/cluster-version-operator/blob/8240a9b3711fa6938129d06ee8c6957a8f3b6464/pkg/cvo/cvo.go#L393-L421
[cluster-version-operator-findUpdateFromConfig]: https://github.com/openshift/cluster-version-operator/blob/8240a9b3711fa6938129d06ee8c6957a8f3b6464/pkg/cvo/updatepayload.go#L294
[cluster-version-operator-pull-request]: https://github.com/openshift/cluster-version-operator/pull/419
[cluster-version-operator-update-lookup]: https://github.com/openshift/cluster-version-operator/blob/01adf75393b6e11d3d8c98ecfeeebd3feb998a6c/pkg/cvo/updatepayload.go#L297-L309
[cluster-version-operator-update-translation]: https://github.com/openshift/cluster-version-operator/blob/8240a9b3711fa6938129d06ee8c6957a8f3b6464/pkg/cvo/availableupdates.go#L192-L198
[console-channels]: https://github.com/openshift/console/pull/2935
[homepage-vs-url]: https://github.com/openshift/enhancements/pull/123#discussion_r453837883
[no-pass-through]: https://github.com/openshift/enhancements/pull/123#discussion_r436849412
[rfc-3986-s1.1.3]: https://tools.ietf.org/html/rfc3986#section-1.1.3
[upstream-metadata-suggestion]: https://github.com/openshift/enhancements/pull/123#discussion_r436848838
[upstream-trust]: https://github.com/openshift/enhancements/pull/123#discussion_r349853363
[url-docs-a]: https://github.com/openshift/enhancements/pull/408#discussion_r458743166
[url-docs-b]: https://github.com/openshift/enhancements/pull/408#discussion_r460192651
