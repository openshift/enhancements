---
title: phased-update-rollouts
authors:
  - "@wking"
reviewers:
  - "@jottofar"
  - "@LalatenduMohanty"
  - "@sdodson"
  - "@steveeJ"
  - "@vrutkovs"
approvers:
  - TBD
creation-date: 2020-07-20
last-updated: 2020-09-22
status: implementable
---

# Phased Update Rollouts

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement request proposes phased update rollouts, allowing release administrators to declare a time window over which update recommendations are phased into per-cluster requests for stable channel [Cincinnati graphs][cincinnati].

## Motivation

We [currently][channel-semantics] has `fast-4.y` and `stable-4.y` channels for each 4.y series.
That allows users who want to hear about releases and related update edges immediately after we declare them supported to do so via `fast-4.y`, while users who want to wait and see if we still recommend the edge after its seen more volume can do so via `stable-4.y`, as discussed [here][fast-vs-stable].
However, clusters default to `stable-4.y` and users need to explicitly configure `fast-4.y` to populate that higher-risk cluster pool.
This leaves the additional safety of `stable-4.y` ambiguous; when a cluster sees an available update recommended in `stable-4.y`, it's not clear now much traffic it has received.
Conservative users are left to wait an arbitrary amount of additional time and guess about the number of other clusters which may be feeling out the new edge, or rely entirely on their own local testing.

By smearing the update recommendation out over a time window, we address several issues:

1. Reduced likelihood of a thundering herd of updating clusters swamping canonical image registries.
    We currently lack built-in support for [automatic updates][automatic-updates], but some users have built analogous functionality as a higher-level driver for ClusterVersion.
    Phased rollouts make it less likely to have many clusters simultaneously initiate a given update by removing the synchronized update recommendation.
2. Reduced risk for stable promotion.
    Instead of throwing a switch for all stable clusters simultaneously, release administrators would have the ability to augment meager `fast-4.y` data by trickling `stable-4.y` clusters over a new update edge.
    Choices about which clusters in the stable pool get the recommended stable edge first can be made intelligently, and the riskier early slots can be rotated through the pool of stable clusters to share out the risk.
    This means that by the time a given cluster receives an update recommendation, it's more likely that several other Red Hat customers have already been recommended and taken the given edge.

### Goals

* [The graph-data repository][graph-data] should grow a schema for declaring phased-rollout time windows.

### Non-Goals

* Deciding how to sort clusters within a given rollout window.
    For an initial implementaiton, the "phased" rollout can be: _all_ stable clusters receive the edges in question at the end of the configured window.
    That's effectively what we're doing today.
    Future internal work can make more intelligent decisions about sorting and smearing clusters within the configured window.

## Proposal

Currently fast and stable promotion are two separate, manual steps (e.g. [here][4.5.2-fast] and [here][4.5.2-stable]).
With this enhancement, that will become a single supported-release promotion.
The release _nodes_ will appear in both the fast and stable channels immediately at the start of the configured time window, to avoid `VersionNotFound` errors for users in `stable-4.y` running the release (e.g. because they installed it immediately after it was announced as GA, avoiding contention like [this][4.4.3-version-not-found]).
Update-recommendation edges associated with nodes in `fast-4.y` will also appear immediately at the start of the configured time window.
Update-recommendation edges associated with nodes in `stable-4.y` (now the same set as nodes in `fast-4.y`) will be phased in over the configured time window, with the mechanics of the smearing left undefined as described in [_non-goals_](#non-goals).

Because we could have multiple [OpenShift Update Service][cincinnati] pods serving the graph to clients, and because clients may not always be routed to the same pod, we want a way for all of those pods to agree on whether the client cluster should have rolled out or not.
Because there is currently no provision for sharing state between sibling Update Service pods, the easiest way to form that consensus is to have rollouts declare:

* A starting timestamp.
* A rollout duration.

and have further logic be deterministically calculated based on shared pod knowledge of:

* The current time.
* Immutable (or at least slowly-changing) aspects of the cluster, such as its Telemetry ID and support level.
    This logic is left undefined as described in [_non-goals_](#non-goals).

With this enhancement, we will define minor/patch durations at the channel level, and add start timestamps on the node entries:

```yaml
channels:
- name: fast-4.4
- name: stable-4.4
  phasedRollouts:
  - fromVersion: patch
    duration: P2D
  - fromVersion: minor
    duration: P14D
versions:
...
- name: 4.4.3
  start: 2020-05-12T00:00Z
```

The schema becomes (with the [RFC 2119][rfc-2119] MUST and other keywords):

* `channels` (required [array][json-array]) of channels being managed by this file.
    Each entry contains the following properties:
    * `name` (required [string][json-string]) with the channel name.
        Each channel name MUST NOT appear more than once once in the `channels` array or in multiple managing files.
    * `phasedRollouts` (optional [array][json-array]) of rules for calculating rollout duration.
        When `phasedRollouts` is empty or does not contain any matching rules, the default duration is `P0S` (i.e. edges appear immediately at the `start` time).
        When there are multiple matching rules, consumers MUST apply only the `duration` set by the highest-precedence matching rule.
        Each matching criterion MUST NOT appear more than once in the `phasedRollouts` array; for example, it is invalid to have:

        ```yaml
        channels:
        - name: invalid-example
          phasedRollouts:
          - fromVersion: patch
            duration: P1D
          - fromVersion: patch
            duration: P2D
        ```

        Each entry contains the following properties:
        * `duration` (required [string][json-string], an [RFC 3339 `duration`][rfc-3339-p13]) sets the duration of the phased rollout.

            There MAY be a single entry with no other properies besides `duration`.
            When such an entry exists, consumers MUST treat it as the new default duration.
            In other words, it matches all edges, but at the lowest precedence.
        * `fromVersion` (optional [string][json-string]) describing the edges that match this rule, with the following possiblities:

            * `minor`, for edges where the source and target differ by [minor version][semver-minor].
            * `patch`, for edges where the source and target differ by [patch versions][semver-patch].

            Other values may be possible in future versions, and consumers MUST treat unrecognized `fromVersion` values as acceptable, but non-matching rules.

* `versions` (optional [array][json-array]) of releases which belong to the channel.
    Each entry contains the following properties:
    * `name` (required [string][json-string]) with the release name.
    * `start` (required [string][json-string], an [RFC 3339 `iso-date-time`][rfc-3339-p13]) with the rollout start time.
        Consumers MUST promote the named release to the configured `channels` immediately at `start`, but MUST NOT promote the named release to the configured `channels` before `start`.
        Consumers MUST promote edges between promoted releases at or after `start` and at or before `start` + `duration`, but MAY choose any time they wish during that interval.

[The schema version][graph-data-schema-version] would also be bumped to 2.0.0, because this is a backwards-incompatible change.

This is the smallest pivot from the current schema, but provides no mechanism for more detailed control.
Choosing this approach and later pivoting to [an alternative](#alternatives) later would be possible, but increases the development cost and possible support commitments.
However, this approach does allow for additional matching rules to be added later under `phasedRollout` without breaking backwards compatibility.

### Implementation Details/Notes/Constraints

#### Mutating time windows for active rollouts

Adjusting rollout windows mid-rollout (or post-rollout in a way that re-enters rollout for some clusters) could cause edges to flicker for some clusters or appear for significant numbers of clusters immediately without the expected smearing.
Care should be taken to avoid this, especially with coarse configuration mechanisms such as [per-channel durations](#proposal).
If this becomes troublesome, we could grow the graph-data schema to include a `locked` or similar boolean to fix specific nodes/edges as rolled-out or not.

#### Feedback latency

[The reduced-risk motivation](#motivation) depends on the rollout duration being long enough that feedback about potential issues arrives from early updaters before significant numbers of additional clusters have attempted the same update.
Breaking down some latency contributions for a given cluster:

1. The update recommendation service rolls an update recommendation out for a cluster.
2. The cluster's cluster-version operator polls the update recommendation service and notices the available update (minutes).
3. The cluster initiates the update (could be minutes with [automatic updates][automatic-updates], or could be hours/days/never with manual updates).
4. The cluster begins to hit update-triggered issues (sometime between 0 and 3 hours, with a cap of 3 hours for "update is taking surprisingly long, counting as a "failure", and contributing to inspection).
5. The cluster pushes Telemetry up describing its issue (minutes).
6. Alerting trips over the concerning Telemetry and pings for review, while also automatically updating graph-data to [remove the edge][graph-data-block] for safety during triage (minutes).
7. CPaaS is informed of the graph-data change, [builds the image](distribute-secondary-metadata-as-container-image.md), pushes to Quay, and publishes a signature (tens of minutes?).
8. The OpenShift Update Service notices the new image, verifies the signature, pull the image down, and starts serving it (tens of minutes).
9. The rest of the fleet polls the the update recommendation service and notices that the recommendation is gone (minutes).

And of course, even with a 3 hour cap on slow updates, there may be issues which present post-update at arbitrary durations after the update completes, so you may want additional padding for that.
But lumping the minute-scale delays into another hour and saying that the fleet notices a revoked recommendation within four hours after we recommend an update to the impacted cluster, an update service that uniformly distributed a rollout across 24 hours would recommend the update to a sixth of the fleet before the revocation hit.

#### Metrics

Update recommendation services consuming this information may wish to serve a metric like:

    phased_update_rollout{from="4.6.7",to="4.6.8"} 0.6

where the value ramped linearly from zero to one as the rollout progressed.

### Risks and Mitigations

Dev time wasted on false starts, as long as both [graph-data][] and [the OpenShift Update Service][update-recommendation-service] are internal tools.
Once the OpenShift Update Service is supported for external users, some long-term maintenance risk for any implemented false starts.

## Design Details

### Test Plan

Presubmit testing should forbid [mutating time windows for active rollouts](#mutating-time-windows-for-active-rollouts) unless overridden by admins (or `locked` to avoid the mutation).

### Upgrade / Downgrade Strategy

All in

### Version Skew Strategy

[The OpenShift Update Service][update-recommendation-service] should continue to support the existing [graph-data][] schema, growing support for the new schema version in parallel.
Eventually, support for the outgoing version may be deprecated and dropped.
The update service should also clearly reject any unrecognized graph-data schema versions.

## Implementation History

No implementation yet.

## Drawbacks

No drawbacks.

## Alternatives

There are a few alternative declaration schemas discussed below, with different levels of granularity.

### Per-edge time windows

My original graph-data proposal provided for [per-edge windows][per-edge-windows]:

```json
{
  "channels": [
    "stable-4.4"
  ],
  "from": "4.3.18",
  "to": "4.4.3",
  "metadata": {
    "start": "2020-05-12T00:00Z",
    "duration": "P2D"
  }
}
```

using [RFC 3339 timestamps and durations][rfc-3339-p13] (in this case "two days").

This gives the ultimate flexibility, but would be the largest departure from the current graph-data schema, which leaves edge promotion completely implicit.
Now that release admins no longer care about edge injection (or even [about edge removal][no-special-admins-for-edge-removal]), maybe we can pivot to explicit edges (seeded by tooling that scrapes defaults out of the release images, like I had in [graph-data#1][graph-data-1])?

### Per-node/channel time windows

If we land a schema that does not include [per-edge granularity](#per-edge-time-windows), we might consider a schema that includes per-node/channel granularity (based on Scott's desire to phase in 4.4->4.4 stable edges faster than 4.3->4.4 stable edges).
Something like:

```json
{
  "node": "4.4.3",
  "channels": [
    "stable-4.4"
  ],
  "start": "2020-05-12T00:00Z",
  "minorDuration": "P14D",
  "patchDuration": "P2D",
}
```

In that case, edges would enter the configured channel(s) when both the source and target nodes had phased in as edge sources.

### Pausing phased updates rollouts

Graph adminstrators might be [tempted][pause-proposal] to pause rollouts while feedback is gathered on potential instability.
However, pausing a rollout would only remove the update recommendation for some clusters.
Clusters in the fast channel and those in the stable channel who had already received the recommendation but not yet acted on it would still be vulnerable to the potential issue.
Instead, graph administrators are encouraged to proactively [block the edge][graph-data-block] upon receiving a sufficiently serious report, protecting all clusters while the issue is triaged.
The rollout may continue without effect as long as the block is in place.
If the issue turns out to be acceptably minor, a new rollout window may be scheduled once the block is removed, keeping in mind [the limitations around mutating time windows](#mutating-time-windows-for-active-rollouts).

### Timing the fast promotion

[The current proposal](#proposal) recommends that fast nodes, stable nodes, and fast edges are all promoted at the beginning of the configured time window.
The proposal also discusses the lack of Update Service pod state sharing, and the desire for a consistent response regardless of which Update Service pod ends up handling the request.
Graph-admins who set the start time slightly in the future can give all Update Service pods sufficient time to ingest the new configuration before the window begins, to ensure the consistent responses.

However, delaying the fast promotion by pushing back the start time introduces some divergence between the errata going public and the fast promotion, both of which are declarations of Red Hat support for the given release.
There is some concern that users who have selected [the fast channel][fast-semantics] to get:

> ... new ... versions as soon as Red Hat declares the given version as a general availability release.

will be confused by even hour-scale delays between the errata going public and the appearance of update recommendations in their fast-channel clusters.

We could instead promote to fast immediately upon receiving graph-data changes.
If you assume that graph-data changes propagate to Update Service pods quickly compared to cluster-version operator polling, the chance of divergent graph reponses is small.
The [current][cluster-version-operator-polling] cluster-version operator update polling period is between two and four minutes.
And the [current][graph-builder-polling] Update Service graph builders pause for five minutes between scrapes with a five minute timeout on each scrape.
That means that the cluster-version operator is likely to poll once or twice while the graph builder is paused between scrapes, even if the scrape itself is instantaneous.
Sticky routing will keep a single cluster pinned to a single Update Service pod, but it will not mitigate situations where the previous Update Service pod has been removed (for example, because a configuration change is being rolled out).
And of course it's possible that a stuck pod (broken node routing?  Dying graph-data image host?) could delay some Update Service pods from retrieving graph-data for even longer.

In the rare cases when the cluster-version operator receives inconsistent graph responses as a graph-data change propagates, the impact should be small.
Clusters with [automatic updates][automatic-updates] enabled should initiate an update before the next upstream poll.
Clusters without automatic updates may have [the `UpdateAvailable` alert][cluster-version-operator-update-available] fire, resolve, and fire again.

As another alternative, we could set start times slightly in the past, while keeping the fast promotion pinned to the configured start time.
That would minimize the divergence between the errata going public and the appearance of update recommendations in their fast-channel clusters.
But it would still introduce the same Update Service pod consistency issues.

[4.4.3-version-not-found]: https://github.com/openshift/cincinnati-graph-data/pull/240
[4.5.2-fast]: https://github.com/openshift/cincinnati-graph-data/pull/335
[4.5.2-stable]: https://github.com/openshift/cincinnati-graph-data/pull/336
[automatic-updates]: https://github.com/openshift/enhancements/pull/124
[channel-semantics]: https://docs.openshift.com/container-platform/4.5/updating/updating-cluster-between-minor.html#understanding-upgrade-channels_updating-cluster-between-minor
[update-recommendation-service]: https://github.com/openshift/cincinnati
[cincinnati]: https://github.com/openshift/cincinnati/blob/a8abb826ef00cf91fd0f8a84912d4e0c23b1335d/docs/design/openshift.md
[cluster-version-operator-polling]: https://github.com/openshift/cincinnati/pull/264#issue-396640328
[cluster-version-operator-update-available]: https://github.com/openshift/cluster-version-operator/blob/22696615350d73b0c848ef32ae255ba57db3649d/install/0000_90_cluster-version-operator_02_servicemonitor.yaml#L54-L60
[fast-vs-stable]: https://docs.openshift.com/container-platform/4.5/updating/updating-cluster-between-minor.html#fast-and-stable-channel-use-and-strategies
[fast-semantics]: https://docs.openshift.com/container-platform/4.5/updating/updating-cluster-between-minor.html#fast-4-5-channel
[graph-builder-polling]: https://github.com/openshift/cincinnati/blob/8aa16f0a6877a379aac17623fca4ceaf7818126c/dist/openshift/cincinnati.yaml#L291-L295
[graph-data]: https://github.com/openshift/cincinnati-graph-data/
[graph-data-1]: https://github.com/openshift/cincinnati-graph-data/pull/1
[graph-data-schema-version]: https://github.com/openshift/cincinnati-graph-data/tree/39730aff14ca3f5b34b615dc9f3a011df78201cc#schema-version
[graph-data-block]: https://github.com/openshift/cincinnati-graph-data/tree/39730aff14ca3f5b34b615dc9f3a011df78201cc#block-edges
[no-special-admins-for-edge-removal]: https://github.com/openshift/cincinnati-graph-data/pull/330
[json-array]: https://tools.ietf.org/html/rfc8259#section-5
[json-string]: https://tools.ietf.org/html/rfc8259#section-7
[pause-proposal]: https://github.com/openshift/enhancements/pull/427#discussion_r475778970
[per-edge-windows]: https://github.com/openshift/cincinnati-graph-data/pull/1#discussion_r321911263
[rfc-2119]: https://tools.ietf.org/html/rfc2119
[rfc-3339-p13]: https://tools.ietf.org/html/rfc3339#page-13
[semver-minor]: https://semver.org/spec/v2.0.0.html#spec-item-7
[semver-patch]: https://semver.org/spec/v2.0.0.html#spec-item-6
