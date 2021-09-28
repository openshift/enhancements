---
title: versioning-cincinnati-api
authors:
  - "@pratikmahajan"
reviewers:
  - "@LalatenduMohanty"
  - "@sdodson"
  - "@wking"
  - "@dhellmann"
approvers:
  - "@LalatenduMohanty"
  - "@sdodson"
creation-date: 2021-08-30
last-updated: 2021-08-30
status: implementable
replaces: []
superseded-by: []
---

# Versioning cincinnati api and json schema

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary
As of today, we declare the schema version in Cincinnati Graph URI. We currently, do not embed the version information
in json schema. The enhancement will embed the version information in the json schema. This has the benefit of being
easy to implement and to pass around if we ship Cincinnati JSON over non-HTTP channels. The enhancement will also begin
versioning Cincinnati payloads by media type, for example `application/vnd.redhat.cincinnati.v1+json`.
The move to supplying requested version in media type field will require us to move towards a generic un-versioned Graph
URI, with the move, we'll still support old URI for foreseeable future.

## Motivation

### Goals
1. Embed version information in json schema.
2. Implement media type versioning for clients requesting a specific schema version of Cincinnati
3. Move to a generic un-versioned URI for Cincinnati
4. Cincinnati continues to support older clients after adding support for new clients for foreseeable future.
5. Add metrics to identify hits on URIs and versions.

### Non-Goals
1. This enhancement does not attempt to drop the `v1` URI.


## Proposal

### User Stories

### Implementation

#### Version information in json schema
Embed the version information in json schema. The updated json schema should look like:
```json
{
    "version": 1,
    "nodes": [],
    "edges": []
}
```

#### Media Type Versioning
Currently, the versioning is done by using the versioned URI and does not support media type versioning.
This enhancement allows users to request a specific version of the Cincinnati specification
[proactively][proactive-negotiation] with [the `Accept` header][accept].  
For example, `Accept: application/vnd.redhat.cincinnati.v1+json` says "I only understand v1 Cincinnati graphs;
please give me that format if possible".  
In the event where the media type is not provided, or `application/json` is provided, Cincinnati should return a default
response that is compatible with the oldest supported version so as not to break old clusters from upgrading.

In a similar space, the Open Container Initiative Image Format Specification
[uses versioned media types][image-spec-media-types] to identify versions of configuration and other formats.


#### URI Change
The current URI follows `{URL}/v1/graph`. The new un-versioned URI will follow `{URL}/graph`. The existing URI can be
redirected to the new URI. This can be done through cincinnati. In the case where an old URI is being called, cincinnati
will by default return the graph that is compatible with the oldest supported version.


#### Version metrics
Cincinnati should also implement a way to understand how many clients are consuming the various versions.
This can be done by implementing metrics for old URI vs new URI in Cincinnati. Similarly, metrics should be implemented
to understand the consumption of various media types (versions). This will help us to make decisions on deprecating the
old versions and URI. For example, a single `cincinnati_pe_http_response_total` counter with labels for `uri` and `version`


### Risks and Mitigations
* As there are some customers that have old URI hardcoded while creating the cluster, we will have to support old URI
  for indefinite period of time. We can redirect the old URI to new one to avoid maintaining different URIs, but we'll
  still have to maintain different versions and v1 for indefinite period of time.
* Another disadvantage listed in the HTTP RFC is inefficiency when  only a small percentage of responses have multiple
  representations. But in this case, including the desired/acceptable version(s) in a per-request header is about as
  efficient as hitting a versioned URI that the service supports, and much more efficient than
  [the alternatives](#uri-versioning) for supporting versioned URIs when the service does not support the client's preferred version.


## Design Details

### Test Plan
* cincinnati tests to make sure we are not breaking older versions.
* cincinnati returns correct resource version when the content type is provided.

### Graduation Criteria
GA. When it works, we ship it.


#### Dev Preview -> Tech Preview


#### Tech Preview -> GA


#### Removing a deprecated feature


### Upgrade / Downgrade Strategy


### Version Skew Strategy
* The `v1` URI will be supported for foreseeable future, this applies only to URI.
* Maintenance cut-offs can be set for versions. Also, developers can take decision on deprecating older versions
  depending upon the version metrics gathered. Although `v1` URI will be supported for foreseeable future, the
  individual versions will not.

## Implementation History

## Drawbacks


## Alternatives

### URI Versioning

In addition to adding version information to the JSON Schema for downstream consumers, we can continue with the current
URI based versioning for the Cincinnati API. The benefit to that approach is that we don't need to deprecate the
existing v1 URI.  
A downside is that clients need to either:

* Poll a series of versioned URIs until they find one the service supports.  
  For example, the client could poll the v3 URI, get a 404, poll the v2 URI, get a 404, and then successfully hit the
  v1 URI if the client preferred v3, but the service they were hitting only supported v1.
* Hit a new URI that lists versions supported by the service.
  For example, `/graph/versions`, which would return a list of URIs and versions from which the client selects its favorite.
  The HTTP RFCs call this [reactive negotiation][reactive-negotiation].

While these alternatives would work, it's simpler in the client code to construct an `Accept` header and make a single
request, and one of the cluster-version operator's design goals is to make the client as simple as possible, because
bugs in the cluster-version operator can be hard to recover from.


### Media-type versioning with the existing, versioned URI

We could serve multiple, media-typed versions from the existing add content-type negotiation with the existing
`{URL}/v1/graph` location. The benefit to that approach is that we don't need to deprecate the existing URI.
The drawback is that users might find the dissonance confusing while reading docs talking about v2 content from a v1 URI.

[accept]: https://datatracker.ietf.org/doc/html/rfc7231#section-5.3.2
[image-spec-media-types]: https://github.com/opencontainers/image-spec/blob/v1.0.1/media-types.md
[proactive-negotiation]: https://datatracker.ietf.org/doc/html/rfc7231#section-3.4.1
[reactive-negotiation]: https://datatracker.ietf.org/doc/html/rfc7231#section-3.4.2
