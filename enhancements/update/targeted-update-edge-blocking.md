---
title: targeted-update-edge-blocking
authors:
  - "@wking"
reviewers:
  - "@dofinn"
  - "@LalatenduMohanty"
  - "@sdodson"
  - "@steveeJ"
  - "@vrutkovs"
approvers:
  - TBD
creation-date: 2020-07-07
last-updated: 2020-08-12
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
In those cases there is currently tension between wanting to protect vulnerable clusters by blocking the edge vs. wanting to avoid inconveniencing clusters which we know are not vulnerable and whose administrators may have been planning on taking the recommended update.
This enhancement aims to reduce that tension.

### Goals

* [Cincinnati graph-data][graph-data] maintainers will have the ability to block edges for the subset of clusters matching a particular Prometheus query.

### Non-Goals

* Blocking edges based on data that is not [uploaded to Telemetry][uploaded-telemetry].
* Blocking edges for subsets of clusters when the update service is unable to reach or authenticate with an aggregating Prometheus server.
    For example, users running a local update service will not be able to access Red Hat's internal Telemetry to determine the requesting cluster's platform, etc.
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
    * `promql` (optional, [object][json-string]), with a [PromQL][] query describing affected clusters.
        This query will be submitted to a configurable Prometheus service and should return a set of matching records with `_id` labels.
        Clusters whose [submitted `id` query parameter][cincinnati-for-openshift-request] is in the set of returned IDs are allowed if the value of the matching record is 1 and blocked if the value of the matching record is 0.
        Clusters whose submitted `id` is not in the result set, or which provide no `id` parameter, are also blocked.
        Blocking too many edges is better than blocking too few, because you can recover from the former, but possibly brick clusters on the latter.

        The result of the query may be cached by the update service, so queries which generate unstable result sets should be avoided.
        Clusters whose submitted `id` is not in the result set may be considered cache misses and, if a cache refresh still fails to include them, may be cached as `blocked`.

[The schema version][graph-data-schema-version] would also be bumped to 1.1.0, because this is a backwards-compatible change.
Consumers who only understand graph-data schema 1.0.0 would ignore the `clusters` property and block the edge for all clusters.
While blocking the edge for all clusters is more aggressive than the graph-data maintainers intended, it is failing on the side of safety.
Blocking these edges for all clusters is also not as aggressive as complaining about an unrecognized 2.0.0 version and failing to serve the entire graph.

### Update service support for the enhanced schema

The following recommendations are geared towards the [openshift/cincinnati][cincinnati].
Maintainers of other update service implementations may or may not be able to apply them to their own implementation.

The graph-builder's graph-data scraper should learn about [the new 1.1.0 schema](#enhanced-graph-data-schema-for-blocking-edges), and record any `clusters` properties in edge metadata.
Blocked edges with `clusters` containing a conditional rule like `promql` are "conditional edges" in the following discussion, because the presence of an edge in the recommended update graph returned to a client depends on whether the rule conditions apply to the client cluster or not.

When a request is received, the submitted [`channel` query parameter][cincinnati-for-openshift-request] limits the set of remaining edges.
If any of the remaining edges have `clusters.promql` entries, a new, targeted-edge-blocking policy engine plugin will exclude the edge if the [`id` query parameter][cincinnati-for-openshift-request] is a member of the query's resulting ID set.
If the request does not set the `id` parameter, the plugin should block all conditional edges, and does not need to check a cache or make PromQL queries.
To perform the PromQL request, the update service will be extended with configuration for the new policy enging plugin.
The new plugin will have a configurable Prometheus connection, including a URI, authentication credentials, and an optional set of trusted X.509 certificate authorities.

To reliably and efficiently return cluster-specific responses, the ID set returned by configured queries may be cached (although see [the *ID flooding* section](#denial-of-service-via-id-flooding), regardless of caching).
There are a few possibilities for caching; although selecting a particular caching implementation is up to the update service's maintainers.
The policy engine should refresh the cache on cache misses, as discussed in [the schema section](#enhanced-graph-data-schema-for-blocking-edges).

For every graph-builder response, the policy engine may aggregate queries for all edges and warm the cache of new queries.
This will make the initial client request faster, at the expense of maintaining a cache that may not be needed before it goes stale.

The policy engine should also be prepared for, and alert on, misconfigured PromQL that results in request failures.
In the event of such a failure, the fallback behavior should be blocking the relevant edge for all clusters until the misconfiguration is fixed.

### User Stories

#### Bugs which impact a subset of clusters

As described in [the *Motivation* section](#motivation), enabling things like "we'd like to block this edge for clusters born in 4.1 with the `None` platform".

#### OpenShift Dedicated

[OpenShift Dedicated][dedicated] (OSD) restricts the recommended update sets for managed clusters more aggressively than the default service.
For example, OSD blocked edges into 4.3.19 temporarily while [rhbz#1838007][rhbz-1838007] was investigated, while the default service did not.
With this enhancement, OSD-specific edge blocking could be accomplished with entries like:

```yaml
to: 4.3.19
from: .*
clusters:
  # show unmanaged clusters the edge, but block it from managed clusters
  prometheus: |
    max by (_id) (0*subscription_labels{managed="true"})
    or
    max by (_id) (subscription_labels{managed="false"})
```

The query returns `{_id="..."}=0` results for `managed="true"` clusters (blocking the edge) and `{_id="..."}=1` results for `managed="false"` clusters (allowing the edge).
Clusters with both `managed="true"` and `managed="false"` records in the current time window, because a cluster transitioned into or out of the managed state, will fall into the safer "block" bucket (the `or` semantics are defined [here][PromQL-or]).

### Risks and Mitigations

#### Divergent Prometheus services

There is a risk that [published graph-data information](distribute-secondary-metadata-as-container-image.md) which assumes access to Red Hat's internal Telemetry will block edges when consumed by update services without access to the internal Telemetry.
The risk is small initially, when we have no such edges.
If targeted blocking becomes widespread, a simple but tedious mitigation would be asking users to maintain their own graph-data (e.g. removing all targeted edge-blocking references) and build their own graph-data image.

A more powerful but complicated fix would be documenting a process for running a local Telemetry aggregator and configuring the local update service to run queries against that aggregator.
This documentation would need to cover mechanisms for adding any expected metrics that Red Hat currently adds in internal Telemetry which come up in targeted edge queries, such as [`subscription_labels`](#openshift-dedicated).

Users who did not want to maintain their own graph-data or a local Telemetry aggregator could also deploy their local update service with the targeted edge blocking plugin disabled.
This will leave all conditional edges *enabled*, with the local administrators taking responsibility for otherwise blocking or avoiding conditional edges to which their client cluster set may be vulnerable.

#### Clusters not reporting data

Clusters which have [opted out of uploading Telemetry][uploaded-telemetry-opt-out] or which run on restricted networks and so are unable to report Telemetry will default to the blocked state (more discussion on unknown ID handling in [the *query coverage* section](#query-coverage)).
Being unable to report Telemetry while still being able to connect to Red Hat's hosted update service seems unlikely; if a cluster can do one (e.g. via a proxy), it should be able to do both.
Clusters which connect to neither Telemetry nor Red Hat's hosted update service may run a local update service, and that is discussed in [the *divergent Prometheus services* section](#divergent-prometheus-services).

Clusters which opt out of Telemetry are likely still able to connect to the update service.
When they do, they will be [blocked as cache-misses](#enhanced-graph-data-schema-for-blocking-edges).
I think that conservative approach is acceptable, because we cannot tell if the non-reporting cluster is vulnerable to the known issues or not.
Local administrators should instead test their updates locally and make their own decisions about update safety, as discussed in [the *exposing blocking reasons* section](#exposing-blocking-reasons).

#### Exposing blocking reasons

This enhancement provides no mechanism for telling clients if or why a recommended update edge has been blocked, because [the Cincinnati graph format][cincinnati-spec] provides no mechanism for sharing metadata about recommended edges, or even the existence of not-recommended edges.
Clients might be tempted to want that information to second-guess a graph-data decision to block the edge, but I am [not convinced the information is actionable](overriding-blocked-edges/README.md).

#### Stranding supported clusters

As described [in the documentation][support-documentation], supported updates are still supported even if incoming edges are blocked, and Red Hat will eventually provide supported update paths from any supported release to the latest supported release in its z-stream.
There is a risk, with the dynamic, per-cluster graph, that targeted edge blocking removes all outgoing update recommendations for some clusters on supported releases.
The risk is highest for [clusters which are not reporting data](#clusters-not-reporting-data), so an easy way to audit would be to request a graph without specifing an `id` parameter (which [blocks all conditional edges](#enhanced-graph-data-schema-for-blocking-edges)) and to look for old, dead-end releases.

#### Denial of service via ID flooding

Malicious clients may flood an update service with graph requests rotating through a series of nominal `id` parameters in an attempt to consume the update service's cache capacity and/or PromQL volume.
Update service maintainers should consider rate-limiting requests from a single source, requiring token-based authentication, and other mechanisms for limiting the denial of service exposure of expensive query requests.

## Design Details

### Test Plan

[The graph-data repository][graph-data] should grow a presubmit test to enforce as much of the new schema as is practical.
Validating any PromQL beyond "it's a string" is probably more trouble than its worth, because consuming update services should [alert on and safely handle](#update-service-support-for-the-enhanced-schema) such misconfiguration.

Unit-testing the behavior in [openshift/cincinnati][cincinnati] with a mock Prometheus should be sufficient for [update service support](#update-service-support-for-the-enhanced-schema).

### Graduation Criteria

This will be released directly to GA.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Dynamic edge status that is dependent on Prometheus queries makes [the graph-data repository][graph-data] a less authoritative view of the graph served to a given client at a given time, as discussed in [the *risks and mititgations* section](#risks-and-mitigations).
This is mitigated by the recommendation to avoid unstable queries, so while we may not be able to reconstruct historical graphs perfectly, we will still be able to make fairly accurate guesses.

## Alternatives

### Additional data sources

The update service could switch on data scraped from Insights tarballs or other sources instead of Prometheus.
And we could extend `clusters` in future work to allow for that.
With this initial enhancement, I focused on Prometheus because it already exposes an API and would thus be the easiest source to integrate.

#### Client-provided identifiers

As a subset of possible additional data sources, clients could provide more identifying features in [their requests][cincinnati-for-openshift-request].
While things like `platform=azure` would be fairly straightforward, we are unlikely to foresee all of the parameters we would need to match issues we discover in the future.
Using Prometheus queries and Telemetry increases the likelihood of being able to narrowly scope edge blocking to the set of vulnerable clusters.
However, as discussed in [the *non-goals* section](#non-goals), scoping doesn't have to be perfect.
And client-provided identifiers removes the need for service-side queries and caching, avoiding [some *denial-of-service concerns*](#denial-of-service-via-id-flooding).
So, like other additional data sources, we may still add support for targeting based on client-provided identifiers in the future.

### Query coverage

[The `promql` proposal](#enhanced-graph-data-schema-for-blocking-edges) specifies a single query that allows the update service to distinguish allowed cluster IDs (matching records with the value 1), blocked cluster IDs (matching records with the value 0), and unrecognized IDs (matching records with other values, or no matching records at all).
This allows the update service to, if it wants, select different cache expiration times for unrecognized IDs.
For example, the update service might say:

> I haven't heard about ID 123...  This is the first time a client has asked about that cluster; maybe it's new and the submitted metrics have not made it through the Telemetry pipeline.  I will check again if they call back after 30 minutes, to see if it has shown up by then.  Falling back to the "block" default for now.

Or:

> Ah, I see ID 123... is explicitly in the block list.  Blocking, and no need to refresh for this cluster for the next day, because queries should not return unstable sets.

This distinction is why `promql` returns both sets, instead of having it only return the blocked IDs, or only the allowed IDs.

You could get a similar distinction with separate queries for allowed and blocked clusters, but you'd need to run at least the blocked query on each cache miss and refresh.
You would also be exposed to confusion about "why is my allowed-query-matching cluster excluded?" for clusters which ended up matching both the allowed and blocked queries, because matching the blocked query would take precedence for the safety reasons discussed in [the proposal](#enhanced-graph-data-schema-for-blocking-edges).

[block-edges]: https://github.com/openshift/cincinnati-graph-data/tree/29e2d0bc2bf1dbdbe07d0d7dd91ee97e11d62f28#block-edges
[blocking-4.5.3]: https://github.com/openshift/cincinnati-graph-data/commit/8e965b65e2974d0628ea775c96694f797cd02b1e#diff-72977867226ea437c178e5a90d5d7ba8
[cincinnati]: https://github.com/openshift/cincinnati
[cincinnati-for-openshift-design]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md
[cincinnati-for-openshift-request]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md#request
[cincinnati-spec]: https://github.com/openshift/cincinnati/blob/master/docs/design/cincinnati.md
[dedicated]: https://www.openshift.com/products/dedicated/
[graph-data]: https://github.com/openshift/cincinnati-graph-data
[graph-data-schema-version]: https://github.com/openshift/cincinnati-graph-data/tree/29e2d0bc2bf1dbdbe07d0d7dd91ee97e11d62f28#schema-version
[json-object]: https://tools.ietf.org/html/rfc8259#section-4
[json-string]: https://tools.ietf.org/html/rfc8259#section-7
[PromQL]: https://prometheus.io/docs/prometheus/latest/querying/basics/
[PromQL-or]: https://prometheus.io/docs/prometheus/latest/querying/operators/#logical-set-binary-operators
[rhbz-1838007]: https://bugzilla.redhat.com/show_bug.cgi?id=1838007
[rhbz-1858026-impact-statement-request]: https://bugzilla.redhat.com/show_bug.cgi?id=1858026#c26
[rhbz-1858026-impact-statement]: https://bugzilla.redhat.com/show_bug.cgi?id=1858026#c28
[support-documentation]: https://docs.openshift.com/container-platform/4.5/updating/updating-cluster-between-minor.html#upgrade-version-paths
[uploaded-telemetry]: https://docs.openshift.com/container-platform/4.5/support/remote_health_monitoring/showing-data-collected-by-remote-health-monitoring.html#showing-data-collected-from-the-cluster_showing-data-collected-by-remote-health-monitoring
[uploaded-telemetry-opt-out]: https://docs.openshift.com/container-platform/4.5/support/remote_health_monitoring/opting-out-of-remote-health-reporting.html
