---
title: channel-metadata
authors:
  - "@wking"
reviewers:
  - "@LalatenduMohanty"
  - "@spadgett"
approvers:
  - "@crawford"
  - "@LalatenduMohanty"
  - "@sdodson"
creation-date: 2020-11-09
last-updated: 2020-11-09
status: implementable
see-also:
  - "/enhancements/update/available-update-metadata.md"
---

# Channel Metadata

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We have [official documentation for channels and their intended semantics][channel-docs], but not all customers read the docs.
This enhancement proposes an API which update services may use to declare channels and associated metadata, and a path for propogating that data into clusters, where tooling like [`oc adm upgrade [channel]`][oc-adm-upgrade-channel] and [the web console][web-console-channel] can expose them to users as they are considering channel choices or the resulting update recommendations.
This is similar to [available-update metadata](available-update-metadata.md), but for different data.

## Motivation

Exposing channel descriptions to provide more context about the current channel or possible channel choices makes it easier for cluster administrators to select the channel that aligns with their intended update semantics.
It reduces the chance that cluster administrators misinterpret the channel names (e.g. expecting `fast-*` to be unsupported, when in fact, it [is supported][channel-docs-fast]).

Distributing the information via an API allows the update service, which is in charge of populating [the channel-specific update recommendations][cincinnati-channels], to declare the semantics it is using for each channel.
This allows user-facing interfaces like `oc` and the web console to access the channel metadata and display it to the user without hard-coding assumptions about the update service.

### Goals

* Exposing a list of channels and related metadata provided by the upstream update recommendation service.

### Non-Goals

* Requiring all channels to be publically declared.

## Proposal

### Channel-list API

This enhancement adds a new update service API, served from:

```
/api/upgrades_info/channels
```

Clients make [an HTTP GET][http-get] request of the endpoint.
Clients and the update service may negotiate [HTTP authentication][http-authentication] to access non-public channels.
[The `Accept` header][http-accept] should be set to `application/vnd.redhat.cincinnati.channels.v1+json`.

The update service responds with [JSON][json], as required by [the `+json` structured syntax suffix][json-structured-syntax-suffix], with the following schema:

* `channels` (required [object][json-object]), with the set of available channels.
    Keys are channel names, like `stable-4.1`.
    Values are [objects][json-object] with the following properties:

    * `description` (optional [string][json-string]) with a paragraph of unstructured text describing the channel semantics.

### In-cluster Storage

This enhancement adds a new custom resource definition to `config.openshift.io`, based on the following Go types:

```go
// Channel represents an OpenShift update service channel.
type Channel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of the channel.
	// +kubebuilder:validation:Required
	// +required
	Spec ChannelSpec `json:"spec"`

	// status contains information about the channel.
	// +optional
	Status ChannelStatus `json:"status"`
}

// ChannelSpec is the desired state of the channel.  There are no
// properties, because the channel custom resource just reflects data
// collected from outside the cluster.
type ChannelSpec struct {
}

// ChannelStatus represents the status of an OpenShift update service channel.
type ChannelStatus struct {
  // description is a paragraph of unstructured text describing the
	// channel semantics.
  // +optional
  Description string `json:"description,omitempty"`
}
```

The cluster-version operator (CVO) will poll [the channel-list API](#channel-list-api) and populate the in-cluster `Channel` objects, which can then be consumed by other tooling, such as [`oc adm upgrade [channel]`][oc-adm-upgrade-channel] and [the web console][web-console-channel].
This enhancement also adds a `RetrievedChannels` condition, similar to [the existing `RetrievedUpdates` condition][retrieved-updates], to alert adminstrators about failures in retrieving `Channel` objects.
When failing to retrieve channel metadata, the CVO will leave the existing `Channel` objects untouched, under the expectation that the semantics of existing channels will evolve slowly, if at all.

### User Stories

#### Informed Channel Selection

Alice's cluster is on release 4.5.16, and she wants to know if she can update to 4.6.
With interfaces like the web console and `oc adm upgrade ...` exposing channel descriptions channel semantics, she will no longer have to discover [these docs][channel-docs-fast] to understand what `fast-4.6` means.

By exposing these semantics via an API, it's also possible for folks using alternative update services to declare their own semantics for their own channels.
They will not need to match patterns used by Red Hat when managing channels for our hosted update service.

#### Recovering from unknown versions

Users may misconfigure their channel (e.g. `does-not-exist-4.6`) or force an update to a release that does not belong to their current channel (e.g. forcing an update to 4.6.1 while remaining on `stable-4.5`).
In those situations, [the available-update metadata](available-update-metadata.md) tooling will not be able to determine the channels to which the current release belongs, because the release will not appear in requests for the configured channel.
With this enhancement, clients may iterate over available channels, making graph requests, and trying to find the current release.
This allows them to say things like:

> 4.6.1 is not in `stable-4.5`, but we have found it in `stable-4.6`, `fast-4.6`, and `candidate-4.6`.
> Would you like to change the configured channel to one of those options?
> To help you decide, here are their channel descriptions: ...

### Implementation Details/Notes/Constraints

A new update service endpoint will be a new opportunity for transmission to break down.
It's possible that egress proxies and other network configuration allow update service graph requests but block update service channel requests.
Consumers should defend against, and may chose to hard-code default guesses (assuming they know what the configured upstream would want), display any `RetrievedChannels` failure messages, or both.

### Risks and Mitigations

There is no security risk, because this is only exposing information that is already public via [documentation][channel-docs].

## Design Details

### Test Plan

[The update service][cincinnati] and [its operator][cincinnati-operator] will grow e2e tests excercising the new endpoint.
The cluster-version operator will grow unit tests using mock responses.
There will be no openshift/origin e2e test-cases around this functionality.

### Graduation Criteria

The ClusterVersion object is already GA, so there would be nothing to graduate or space to cook a preview implementation.

We could start with a `v1alpha` [`Channel` structure](#in-cluster-storage), but given the simplicity of the structure, it seems safe enough to start with `v1`.

### Upgrade / Downgrade Strategy

[The new API](#channel-list-api) is expected to be stable, but if it evolves, [the `Channel` structure](#in-cluster-storage) may get new versions as well.

### Version Skew Strategy

Consumers of [the `Channel` objects](#in-cluster-storage) should be robust to their absence, even in the absence of version skew, as discussed in [the implementation details](#implementation-detailsnotesconstraints).

The cluster-version operator should be robust to update services which do not support [the new API](#channel-list-api) via the same logic that protects them from network issues while connecting to it.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Drawbacks for the chosen approach are discussed in the individual [*alternatives*](alternatives) subsections.

## Alternatives

### Channel-list API versioning

[The channel-list API](#channel-list-api) is versioned by media type.
Alternative versioning schemes include:

* URI path, e.g. `/api/upgrades_info/v1/channels`.
    But this makes discovering new versions difficult.
    For example, clients would need to poll `/api/upgrades_info/v2/channels` to see if a given update service supports v2.
* Payload property, e.g. `{"version": "1.0.0", "channels": [...]}`.
    But this makes backwards compatibility difficult.
    For example, a client that only understands 1.0.0 which receives a 2.0.0 payload will know that it cannot understand the remainder of the payload.
    It can exit clearly, complaining of the unrecognized version, but it cannot extract the desired channel data.

By using [HTTP content negotiation][http-content-negotiation], the client and update service have the best chance of discovering a version understandable to both parties.

### Channels as a root JSON object

The [channel-list API](#channel-list-api) declares a root `channels` property containing channel entries.
This may seem redundant, but the property allows room for future root properties around pagination or other metadata we find a need for that growing forward.

### Direct access to the update service

Components like the web console could bypass `Channel` and talk to the update service directly, but this would require significant code duplication (e.g. consuming [the `Proxy` configuration][proxy]).

[channel-docs]: https://docs.openshift.com/container-platform/4.6/updating/updating-cluster-between-minor.html#understanding-upgrade-channels_updating-cluster-between-minor
[channel-docs-fast]: https://docs.openshift.com/container-platform/4.6/updating/updating-cluster-between-minor.html#fast-4-6-channel
[cincinnati]: https://github.com/openshift/cincinnati
[cincinnati-channels]: https://github.com/openshift/cincinnati/blob/75baa1d77e5797023b9a44e4a07d731c65855654/docs/design/openshift.md#channels
[cincinnati-operator]: https://github.com/openshift/cincinnati-operator
[http-accept]: https://tools.ietf.org/html/rfc7231#section-5.3.2
[http-authentication]: https://tools.ietf.org/html/rfc7235
[http-content-negotiation]: https://tools.ietf.org/html/rfc7231#section-5.3
[http-get]: https://tools.ietf.org/html/rfc7231#section-4.3.1
[json]: https://tools.ietf.org/html/rfc8259
[json-array]: https://tools.ietf.org/html/rfc8259#section-5
[json-media-type]: https://tools.ietf.org/html/rfc8259#section-11
[json-object]: https://tools.ietf.org/html/rfc8259#section-4
[json-string]: https://tools.ietf.org/html/rfc8259#section-7
[json-structured-syntax-suffix]: https://tools.ietf.org/html/rfc6839#section-3.1
[oc-adm-upgrade-channel]: https://github.com/openshift/oc/pull/576
[proxy]: https://docs.openshift.com/container-platform/4.6/networking/enable-cluster-wide-proxy.html
[retrieved-updates]: https://github.com/openshift/cluster-version-operator/blob/3c62269518686d7db3eaa049918c2ddc937f56a4/docs/user/status.md#retrievedupdates
[semver]: https://semver.org/spec/v2.0.0.html
[web-console-channel]: https://github.com/openshift/console/pull/6283#issue-465685816
