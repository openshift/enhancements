---
title: targeted-update-edge-blocking
authors:
  - "@wking"
reviewers:
  - "@dofinn"
  - "@jottofar"
  - "@LalatenduMohanty"
  - "@PratikMahajan"
  - "@sdodson"
  - "@vrutkovs"
approvers:
  - TBD
creation-date: 2020-07-07
last-updated: 2021-04-16
status: implementable
---

# Targeted Update Edge Blocking

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes a mechanism for blocking edges for the subset of clusters considered vulnerable to known issues with a particular update or target release.

## Motivation

When managing the [Cincinnati][cincinnati-spec] update graph [for OpenShift][cincinnati-for-openshift-design], we sometimes discover issues with particular release images or updates between them.
Once an issue is discovered, we [block edges][block-edges] so we no longer recommend risky updates, or updates to risky releases.

Note: as described [in the documentation][support-documentation], supported updates are still supported even if incoming edges are blocked, and Red Hat will eventually provide supported update paths from any supported release to the latest supported release in its z-stream.

Incoming bugs are evaluated to determine an impact statement based on [a generic template][rhbz-1858026-impact-statement-request].
Some bugs only impact specific platforms, or clusters with other specific features.
For example, rhbz#1858026 [only impacted][rhbz-1858026-impact-statement] clusters with the `None` platform which were created as 4.1 clusters and subsequently updated via 4.2, 4.3, and 4.4 to reach 4.5.
In those cases there is currently tension between wanting to protect vulnerable clusters by [blocking the edge][blocking-4.5.3] vs. wanting to avoid inconveniencing clusters which we know are not vulnerable and whose administrators may have been planning on taking the recommended update.
This enhancement aims to reduce that tension.

### Goals

* [Cincinnati graph-data][graph-data] maintainers will have the ability to block edges for the subset of clusters declaring a particular [infrastructure platform][infrastructure-platform].

### Non-Goals

* Blocking edges based on data that is not [the infrastructure platform][infrastructure-platform].
* Blocking edges based on data that is not submitted by the requesting client.
* Exactly scoping the set of blocked clusters to those which would have been impacted by the issue.
  For example, some issues may be races where the impacted cluster set is a random subset of the vulnerable cluster set.
  Any targeting of the blocked edges will reduce the number of blocked clusters which would have not been impacted, and thus reduce the contention between protecting vulnerable clusters and inconveniencing invulnerable clusters.
* Specifying a particular update service implementation.
  This enhancement floats some ideas, but the details of the chosen approach are up to each update service's maintainers.

## Proposal

### Enhanced graph-data schema for blocking edges

[The blocked-edges schema][block-edges] will be extended with the following new properties:

* `clusters` (optional, [object][json-object]), defining the subset of affected clusters.
  If any `clusters` property matches a given cluster, the edge should be blocked for that cluster.
  * `platforms` (optional, [array][json-array] of [strings][json-string]), listing affected platforms.
    This value will be compared with the requesting cluster.
    Clusters whose platform is in the array will be blocked.
    Clusters whose platform is known to not be in the array will not be blocked.

    Clusters whose platform is unknown are also blocked.
    Blocking too many edges is better than blocking too few, because you can recover from the former, but possibly brick clusters on the latter.

[The schema version][graph-data-schema-version] would also be bumped to 1.1.0, because this is a backwards-compatible change.
Consumers who only understand graph-data schema 1.0.0 would ignore the `clusters` property and block the edge for all clusters.
While blocking the edge for all clusters is more aggressive than the graph-data maintainers intended, it is failing on the side of safety.
Blocking these edges for all clusters is also not as aggressive as complaining about an unrecognized 2.0.0 version and failing to serve the entire graph.

### Update Cincinnati clients

Cincinnati clients should be extended to submit a `platform` query parameter with their [infrastructure platform][infrastructure-platform] with their [Cincinnati request][cincinnati-for-openshift-request].

### Update service support for the enhanced schema

The following recommendations are geared towards [openshift/cincinnati][cincinnati].
Maintainers of other update service implementations may or may not be able to apply them to their own implementation.

The graph-builder's graph-data scraper should learn about [the new 1.1.0 schema](#enhanced-graph-data-schema-for-blocking-edges), and record any `clusters` properties in edge metadata.
The graph-builder should also be prepared for, and alert on, invalid graph-data configuration.
Blocked edges with `clusters` containing a conditional rule like `platforms` are "conditional edges" in the following discussion, because the presence of an edge in the recommended update graph returned to a client depends on whether the rule conditions apply to the client cluster or not.

When a request is received, the submitted [`channel` query parameter][cincinnati-for-openshift-request] limits the set of remaining edges.
If any of the remaining edges have `clusters.platforms` entries, a new, targeted-edge-blocking policy engine plugin will exclude the edge if the `platform` [query parameter][cincinnati-for-openshift-request] a member the array.
If the request does not set the `platform` parameter, the plugin should block all conditional edges.

#### Update service cluster property lookup

To support clients too old to set the `platform` parameter, a strongly-motivated client may configure a mechanism to look up a default platform based on a request's [`id` parameter][cincinnati-for-openshift-request].
This is unlikely to be necessary, but could be used to experiment with [Prometheus support][prometheus-targeted-edge-blocking] or other future strategies.

### User Stories

#### Bugs which impact a subset of clusters

As described in [the *motivation* section](#motivation), enabling things like "we'd like to block this edge for clusters with the `None` platform", although it cannot represent the "born in 4.1" condition.

### Risks and Mitigations

#### Clients not reporting data

Some Cincinnati clients, for example, older cluster-version operators and external web renderers, may not submit `platform` query parameters.
These clients will have conditional edges blocked, unless their update service happens to implement [cluster property lookup](#update-service-cluster-property-lookup).
I think that conservative approach is acceptable, because we cannot tell if the non-reporting client is vulnerable to the known issues or not.
Local administrators should instead test their updates locally and make their own decisions about update safety, as discussed in [the *exposing blocking reasons* section](#exposing-blocking-reasons).

#### Exposing blocking reasons

This enhancement provides no mechanism for telling clients if or why a recommended update edge has been blocked, because [the Cincinnati graph format][cincinnati-spec] provides no mechanism for sharing metadata about recommended edges, or even the existence of not-recommended edges.
Clients might be tempted to want that information to second-guess a graph-data decision to block the edge, but I am [not convinced the information is actionable](overriding-blocked-edges/README.md).

#### Stranding supported clusters

As described [in the documentation][support-documentation], supported updates are still supported even if incoming edges are blocked, and Red Hat will eventually provide supported update paths from any supported release to the latest supported release in its z-stream.
There is a risk, with the dynamic, per-cluster graph, that targeted edge blocking removes all outgoing update recommendations for some clusters on supported releases.
The risk is highest for [clusters which are not reporting data](#clients-not-reporting-data), so an easy way to audit would be to request a graph without specifing `id` or `platform` parameters (which [blocks all conditional edges](#enhanced-graph-data-schema-for-blocking-edges)) and to look for old, dead-end releases.

## Design Details

### Test Plan

[The graph-data repository][graph-data] should grow a presubmit test to enforce as much of the new schema as is practical.

Unit-testing the behavior in [openshift/cincinnati][cincinnati] with test clients should be sufficient for [update service support](#update-service-support-for-the-enhanced-schema).

### Graduation Criteria

This will be released directly to GA.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Blocking only by platform does not address some bugs, for example, those which [only impact the OVN-Kubernetes provider][rhbz-1940498-impact-statement].
If we end up adding [Prometheus support][prometheus-targeted-edge-blocking], the presence of the `platforms` key will create multiple mechanisms for accomplishing the same work, which will moderately raise the development and maintenance burden.

## Alternatives

### Additional data sources

The update service could switch on data from [Promtheus][prometheus-targeted-edge-blocking] or scraped from Insights tarballs or other sources instead of a `platform` query parameter.
And we could extend `clusters` in future work to allow for that.

In favor of switching on Telemetry or Insights data, we are unlikely to foresee all of the parameters we would need to match issues we discover in the future.
Using service-side data makes it more likely that we can tightly scope blockers to vulnerable clusters.

However, client-provided identifiers avoid the need for sevice-side queries and caching, avoiding [some denial-of-service concerns][prometheus-targeted-edge-blocking].
And as discussed in [the *non-goals* section](#non-goals), scoping doesn't have to be perfect.

[block-edges]: https://github.com/openshift/cincinnati-graph-data/tree/29e2d0bc2bf1dbdbe07d0d7dd91ee97e11d62f28#block-edges
[blocking-4.5.3]: https://github.com/openshift/cincinnati-graph-data/commit/8e965b65e2974d0628ea775c96694f797cd02b1e#diff-72977867226ea437c178e5a90d5d7ba8
[cincinnati]: https://github.com/openshift/cincinnati
[cincinnati-for-openshift-design]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md
[cincinnati-for-openshift-request]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md#request
[cincinnati-spec]: https://github.com/openshift/cincinnati/blob/master/docs/design/cincinnati.md
[graph-data]: https://github.com/openshift/cincinnati-graph-data
[graph-data-schema-version]: https://github.com/openshift/cincinnati-graph-data/tree/29e2d0bc2bf1dbdbe07d0d7dd91ee97e11d62f28#schema-version
[infrastructure-platform]: https://github.com/openshift/api/blob/86964261530c2f4e72da15b6d34b0eb69b8a1eb1/config/v1/types_infrastructure.go#L224-L235
[json-array]: https://tools.ietf.org/html/rfc8259#section-5
[json-object]: https://tools.ietf.org/html/rfc8259#section-4
[json-string]: https://tools.ietf.org/html/rfc8259#section-7
[prometheus-targeted-edge-blocking]: https://github.com/openshift/enhancements/pull/426
[rhbz-1858026-impact-statement-request]: https://bugzilla.redhat.com/show_bug.cgi?id=1858026#c26
[rhbz-1858026-impact-statement]: https://bugzilla.redhat.com/show_bug.cgi?id=1858026#c28
[rhbz-1940498-impact-statement]: https://bugzilla.redhat.com/show_bug.cgi?id=1940498#c12
[support-documentation]: https://docs.openshift.com/container-platform/4.5/updating/updating-cluster-between-minor.html#upgrade-version-paths
