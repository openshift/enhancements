---
title: targeted-update-edge-blocking
authors:
  - "@wking"
reviewers:
  - "@bparees"
  - "@deads2k"
  - "@dhellmann"
  - "@jan-f"
  - "@jottofar"
  - "@LalatenduMohanty"
  - "@sdodson"
  - "@vrutkovs"
approvers:
  - "@sdodson"
creation-date: 2020-07-07
last-updated: 2021-09-23
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
And, since [docs#32091][openshift-docs-32091], [that documentation][support-documentation] also points out that updates initiated after the update recommendation has been removed are still supported.

Incoming bugs are evaluated to determine an impact statement based on [a generic template][rhbz-1858026-impact-statement-request].
Some bugs only impact specific platforms, or clusters with other specific features.
For example, rhbz#1858026 [only impacted][rhbz-1858026-impact-statement] clusters with the `None` platform which were created as 4.1 clusters and subsequently updated via 4.2, 4.3, and 4.4 to reach 4.5.
And rhbz#1957584 [only impacted][rhbz-1957584-impact-statement] clusters updating from 4.6 to 4.7 with Routes whose `spec.host` contains no dots or was otherwise an invalid domain name.

In those cases there is currently tension between wanting to protect vulnerable clusters by blocking the edge vs. wanting to avoid inconveniencing clusters which we know are not vulnerable and whose administrators may have been planning on taking the recommended update.
This enhancement aims to reduce that tension.

### Goals

* [Cincinnati graph-data][graph-data] maintainers will have the ability to block edges for a vulnerable subset of clusters.
* Cluster administrators will have convenient access to the issues graph-data maintainers considered when conditionally or unconditionally blocking edges (a portion of which is currently filled by text-only errata like [this][text-only-errata-example]).
* Avoiding excessive load evaluating in-cluster conditionals.

### Non-Goals

* Exactly scoping the set of blocked clusters to those which would have been impacted by the issue.
    For example, some issues may be races where the impacted cluster set is a random subset of the vulnerable cluster set.
    Any targeting of the blocked edges will reduce the number of blocked clusters which would have not been impacted, and thus reduce the contention between protecting vulnerable clusters and inconveniencing invulnerable clusters, so even overly broad scoping is better than no scoping at all.
* Specifying a particular update service implementation.
    This enhancement floats some ideas, but the details of the chosen approach are up to each update service's maintainers.

## Proposal

### Enhanced graph-data schema for blocking edges

[The existing blocked-edges schema][block-edges] will be extended with the following new properties:

* `url` (optional, [string][json-string]), with a URI documenting the blocking reason.
    For example, this could link to a bug's impact statement or knowledge-base article.
* `name` (optional, [string][json-string]), with a CamelCase reason suitable for [a `ClusterOperatorStatusCondition` `reason` property][api-reason].
* `message` (optional, [string][json-string]), with a human-oriented message describing the blocking reason, suitable for [a `ClusterOperatorStatusCondition` `message` property][api-message].
* `matchingRules` (optional, [array][json-array]), defining conditions for deciding which clusters have the update recommended and which do not.
  The array is ordered by decreasing precedence.
  Consumers should walk the array in order.
  For a given entry, if a condition type is unrecognized, or fails to evaluate, consumers should proceed to the next entry.
  If a condition successfully evaluates (either as a match or as an explicit does-not-match), that result is used, and no further entries should be attempted.
  If no condition can be successfully evaluated, the update should not be recommended.
  Each entry must be an [object][json-object] with at least the following property:
  * `type` (required, [string][json-string]), defining the type in [the condition type registry](#cluster-condition-type-registry).
    For example, `type: Always` identifies the condition as [the `Always` type](#always).
  Additional, type-specific properties for each entry are defined in [the cluster-condition type registry](#cluster-condition-type-registry).

[The schema version][graph-data-schema-version] would also be bumped to 1.1.0, because this is a backwards-compatible change.
Consumers who only understand graph-data schema 1.0.0 would ignore the `matchingRules` property and block the edge for all clusters.
The alternative of failing open is discussed [here](#failing-open).

### Cluster-condition type registry

This registry contains the set of known cluster-condition types, and will be extended if and when future enhancements propose new types.
Cluster condition evaluation has three possible outcomes:

* The cluster matches.
* The cluster does not match.
* Evaluation failed.

They are represented as [objects][json-object] with at least the following property:

* `type` (required, [string][json-string]), defining the type in the condition type registry.
  For example, `type: Always` identifies the condition as [the `Always` type](#always).

Additional, type-specific properties for each entry are defined in the following subsections.

#### Always

The `type: Always` condition entry is an [object][json-object] with no additional properties.
This condition always matches.

#### PromQL

The `type: PromQL` condition entry is an [object][json-object] with the following additional property:

* `PromQL` (required, [object][json-object]), with the following properties:
  * `PromQL` (required, [string][json-string]), with a [PromQL][] query classifying clusters.
    This query will be evaluated on the local cluster, so it has access to data beyond the subset that is [uploaded to Telemetry][uploaded-telemetry].
    The query should return a 1 in the match case (risk matches, update should not be recommended) and a 0 in the does-not-match case (risk does not match, update should be recommended).
    Queries which return no time series, or which return values besides 0 or 1, are evaluation failures, as discussed in [the query coverage section](#query-coverage).

### Enhanced Cincinnati JSON representation

[The Cincinnati graph API][cincinnati-api] will be extended with a new top-level `conditionalEdges` property, with an array of conditional edge [objects][json-object] using the following schema:

* `edges` (required, [array][json-array]), with the update edges covered by this entry.
  Each entry is an [object][json-object] with the following schema:
  * `from` (required, [string][json-string]), with the `version` of the starting node.
  * `to` (required, [string][json-string]), with the `version` of the ending node.
* `risks` (required, [array][json-array], with conditional risks around the recommendation.
  Consumers should evaluate all entries, and only recommend the update if there is at least one entry and all entries recommend the update.
  Each entry is an [object][json-object] with the following schema:
  * `url` (required, [string][json-string]), with a URI documenting the issue, as described in [the blocked-edges section](#enhanced-graph-data-schema-for-blocking-edges).
  * `name` (required, [string][json-string]), with a CamelCase reason, as described in [the blocked-edges section](#enhanced-graph-data-schema-for-blocking-edges).
  * `message` (required, [string][json-string]), with a human-oriented message describing the blocking reason, as described in [the blocked-edges section](#enhanced-graph-data-schema-for-blocking-edges).
  * `matchingRules` (required with at least one entry, [array][json-array]), defining conditions for deciding which clusters have the update recommended and which do not.
    The array is ordered by decreasing precedence.
    Consumers should walk the array in order.
    For a given entry, if a condition type is unrecognized, or fails to evaluate, consumers should proceed to the next entry.
    If a condition successfully evaluates (either as a match or as an explicit does-not-match), that result is used, and no further entries should be attempted.
    If no condition can be successfully evaluated, the update should not be recommended.
    Each entry must be an [object][json-object] with at least the following property:
    * `type` (required, [string][json-string]), defining the type in [the condition type registry](#cluster-condition-type-registry).
      For example, `type: Always` identifies the condition as [the `Always` type](#always).
    Additional, type-specific properties are defined in [the cluster-condition type registry](#cluster-condition-type-registry).

### Enhanced ClusterVersion representation

[The ClusterVersion `status`][api-cluster-version-status] will be extended with a new `conditionalUpdates` property:

```go
// conditionalUpdates contains the list of updates that may be
// recommended for this cluster if it meets specific required
// conditions. Consumers interested in the set of updates that are
// actually recommended for this cluster should use
// availableUpdates. This list may be empty if no updates are
// recommended, if the update service is unavailable, or if an empty
// or invalid channel has been specified.
// +listType=atomic
// +optional
ConditionalUpdates []ConditionalUpdate `json:"conditionalUpdates,omitempty"`
```

The `availableUpdates` documentation will be adjusted to read:

```go
// availableUpdates contains updates recommended for this
// cluster. Updates which appear in conditionalUpdates but not in
// availableUpdates may expose this cluster to known issues. This list
// may be empty if no updates are recommended, if the update service
// is unavailable, or if an invalid channel has been specified.
```

The new ConditionalUpdate type will have the following schema:

```go
// ConditionalUpdate represents an update which is recommended to some
// clusters on the version the current cluster is reconciling, but which
// may not be recommended for the current cluster.
type ConditionalUpdate struct {
	// release is the target of the update.
	// +kubebuilder:validation:Required
	// +required
	Release Release `json:"release"`

	// risks represents the range of issues associated with
	// updating to the target release. The cluster-version
	// operator will evaluate all entries, and only recommend the
	// update if there is at least one entry and all entries
	// recommend the update.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +patchMergeKey=name
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=name
	// +required
	Risks []ConditionalUpdateRisk `json:"risks"`

	// conditions represents the observations of the conditional update's
	// current status. Known types are:
	// * Recommended, for whether the update is recommended for the current cluster.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}
```

The new ConditionalUpdateRisk type will have the following schema:

```go
// ConditionalUpdateRisk represents a reason and cluster-state
// for not recommending a conditional update.
// +k8s:deepcopy-gen=true
type ConditionalUpdateRisk struct {
	// url contains information about this risk.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:MinLength=1
	// +required
	URL string `json:"url"`

	// name is the CamelCase reason for not recommending a
	// conditional update, in the event that matchingRules match the
	// cluster state.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// message provides additional information about the risk of
	// updating, in the event that matchingRules match the cluster
	// state. This is only to be consumed by humans. It may
	// contain Line Feed characters (U+000A), which should be
	// rendered as new lines.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +required
	Message string `json:"message"`

	// matchingRules is a slice of conditions for deciding which
	// clusters match the risk and which do not. The slice is
	// ordered by decreasing precedence. The cluster-version
	// operator will walk the slice in order, and stop after the
	// first it can successfully evaluate. If no condition can be
	// successfully evaluated, the update will not be recommended.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	// +required
	MatchingRules []ClusterCondition `json:"matchingRules"`
}
```

The new ClusterCondition type will have the following schema:

```go
// ClusterCondition is a union of typed cluster conditions.  The 'type'
// property determines which of the type-specific properties are relevant.
// When evaluated on a cluster, the condition may match, not match, or
// fail to evaluate.
// +k8s:deepcopy-gen=true
type ClusterCondition struct {
	// type represents the cluster-condition type. This defines
	// the members and semantics of any additional properties.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum={"Always","PromQL"}
	// +required
	Type string `json:"type"`

	// PromQL represents a cluster condition based on PromQL.
	// +optional
	PromQL *PromQLClusterCondition `json:"PromQL,omitempty"`
}

// PromQLClusterCondition represents a cluster condition based on PromQL.
type PromQLClusterCondition struct {
	// PromQL is a PromQL query classifying clusters. This query
	// query should return a 1 in the match case and a 0 in the
	// does-not-match case case. Queries which return no time
	// series, or which return values besides 0 or 1, are
	// evaluation failures.
	// +kubebuilder:validation:Required
	// +required
	PromQL string `json:"PromQL"`
}
```

[ClusterVersion's `status.history` entries][api-history] will be extended with the following property:

```go
// acceptedRisks records risks which were accepted to initiate the update.
// For example, it may menition an Upgradeable=False or missing signature
// that was overriden via desiredUpdate.force, or an update that was
// initiated despite not being in the availableUpdates set of recommended
// update targets.
// +optional
AcceptedRisks string `json:"acceptedRisks,omitempty"`
```

### Update service support for the enhanced schema

The following recommendations are geared towards the [openshift/cincinnati][cincinnati].
Maintainers of other update service implementations may or may not be able to apply them to their own implementation.

The graph-builder's graph-data scraper should learn about [the new 1.1.0 schema](#enhanced-graph-data-schema-for-blocking-edges), and include the new properties in its blocker cache.
For each edge declared by a release image (primary metadata), the graph-builder will check the blocker cache for matching blocks.
Edges with no matching blocks are unconditionally recommended, and will be included in `edges`.
Edges with matching blocks are conditionally recommended, and will be included in `conditionalEdges`.
Including edges which are recommended for no clusters under `conditionalEdges` gives consumers access to `url`, `name`, and `message` metadata explaining why the edge is not recommended.

### Cluster-version operator support for the enhanced schema

The cluster-version operator will learn to parse [`conditionalEdges`](#enhanced-cincinnati-json-representation) into [`conditionalUpdates`](#enhanced-clusterversion-representation).
The `Recommended` condition can be immediately set to `Unknown` until the `matchingRules` for all the update's risks have been evaluated.
The operator will log an error if the same target is included in both `edges` and `conditionalEdges`, but will prefer the `conditionalEdges` entry in that case.

Additionally, the operator will continually re-evaluate the blocking conditionals in `conditionalUpdates` and update `conditionalUpdates[].conditions` accordingly.
The timing of the evaluation and freshness are largely internal details, but to avoid [consuming excessive monitoring resources](#malicious-conditions) and because [the rules should be based on slowly-changing state](#clusters-moving-into-the-vulnerable-state-after-updating), the operator will handle polling with the following restrictions:

* The cluster-version operator will cache polling results for each query, so a single query which is used in evaluating multiple risks over multiple conditional update targets will only be evaluated once per round.
* After evaluating a PromQL query, the cluster-version operator will wait at least 1 second before evaluating any PromQL.
    This delay will not be persisted between operator restarts, so a crash-looping CVO may result in higher PromQL load.
    But a crash-looping CVO will also cause the `KubePodCrashLooping` alert to fire, which will summon the cluster administrator.
* After evaluating a PromQL query, the cluster-version operator will wait at least an hour before evaluating that PromQL query again.

To perform the PromQL request, the operator will FIXME: details about connecting to the local Thanos/Prometheus.

If there are issues evaluating a conditional update, the operator will set the `Unknown` status on the `Recommended` condition.
The operator will grow a new `warning`-level `CannotEvaluateConditionalUpdates` alert that fires if `lastTransitionTime` for any `Recommended=Unknown` condition is over an hour old.

Any `conditionalUpdates` with `Recommended=True` will have its release inserted into `availableUpdates`.

Both `availableUpdates` and `conditionalUpdates` should be sorted in decreasing SemVer order for stability, to avoid unnecessary status churn.

The cluster-version operator does not currently gate update acceptance on whether the requested target release is a recommended update.
Update targets that are not currently recommended, or are not supported at all, will be allowed without any ClusterVersion overrides ([`force`][api-force] or a similar, new property).
But update targets that are not currently recommended will result in entries in [the `acceptedRisks` history entries](#enhanced-clusterversion-representation).
The `acceptedRisks` history entries will also include descriptions of any other complaints, like forced guards, that the cluster-version operator would like to record.

### Update client support for the enhanced schema

[The web-console][web-console] and [`oc`][oc] both consume ClusterVersion to present clients with a list of available updates.
When listing recommended updates, clients will continue their current behavior, listing the contents of `availableUpdates`.

With this enhancement, they may both be extended to consume [`conditionalUpdates`](#enhanced-clusterversion-representation).
When listing all supported updates (which will be hidden behind an opt-in guard like `--include-not-recommended`), clients will additionally include entries from `conditionalUpdates` with `Recommended!=True`, and include the `reason` and `message` from the `Recommended` condition alongside the supported-but-not-recommended updates.
Clients may optionally provision for reporting additional condition types, in case new types are added in the future.
When `Recommended!=True` entries exist but the `--include-not-recommended` opt-in guard is not listed, clients may inform users about the existence of the guard.

Updating to a conditional edge that the cluster does not qualify for will require `--allow-not-recommended` or similar client-side gate.

### User Stories

The following user stories will walk users through an example flow around the [authentication operator's leaked connections][rhbz-1941840-impact-statement].

#### Graph-data administrators

The graph-data administrators would create a new file `blocked-edges/4.7.4-auth-connection-leak.yaml` like:

```yaml
to: 4.7.4
from: 4\.6\..*
url: https://bugzilla.redhat.com/show_bug.cgi?id=1941840#c33
name: AuthOAuthProxyLeakedConnections
message: On clusters with a Proxy configured, the authentication operator may keep many oauth-server connections open, resulting in high memory consumption by the authentication operator and router pods.
matchingRules:
- type: PromQL
  promql:
    promql: max(cluster_proxy_enabled{type=~"https?"})
```

This would join existing entries like `blocked-edges/4.7.4-vsphere-hostnames-changing.yaml`:

```yaml
to: 4.7.4
from: .*
url: https://bugzilla.redhat.com/show_bug.cgi?id=1942207#c3
name: VSphereNodeNameChanges
message: vSphere clusters leveraging the vSphere cloud provider may lose node names which can have serious impacts on the stability of the control plane and workloads.
matchingRules:
- type: PromQL
  promql:
    promql: |
      cluster_infrastructure_provider{type=~"VSphere|None"}
      or
      0 * cluster_infrastructure_provider
```

and `blocked-edges/4.7.4-vsphere-hw-17-cross-node-networking.yaml`:

```yaml
to: 4.7.4
from: 4\.6\..*
url: https://access.redhat.com/solutions/5896081
name: VSphereHW14CrossNodeNetworkingError
message: Clusters on vSphere Virtual Hardware Version 14 and later may experience cross-node networking issues.
matchingRules:
- type: PromQL
  promql:
    promql: |
      cluster_infrastructure_provider{type=~"VSphere|None"}
      or
      0 * cluster_infrastructure_provider
```

##### Comparison with tombstones

Graph-data administrators may also [tombstone releases][tombstone] when issues are discovered before errata are published.
[Docs#32091][openshift-docs-32091] [documented][support-documentation] support for updates initiated after update recommendations had been removed, but edges to or from tombstoned releases were never supported.
Similarly, edges which were blocked before the release was supported were never supported, although there may be some ambiguity here if the blocks are conditional, as clusters may move in and out of the vulnerable set depending on their current configuration.

We also lack a clear mechanism for warning users about install-time or post-install issues with a particular release.
The graph-data flow only informs update decisions.

Because of both of these reasons, graph-data administrators should prefer tombstones to protect vulnerable users, instead of proceeding with an official release and relying on targeted edge blocking for protection.
Publishing errata after blocking edges should be an exception for situations where we need to ship a fix for one serious issue but do not yet have a fix in place for a second serious issue.

#### Cincinnati JSON

Update services would consume the above graph-data, and serve graphs with:

```json
{
  "conditionalEdges": [
    ...
    {
      "edges": [
        ...
        {"from": "4.6.23", "to": "4.7.4"},
        ...
      ],
      "risks": [
        {
          "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1941840#c33",
          "name": "AuthOAuthProxyLeakedConnections",
          "message": "On clusters with a Proxy configured, the authentication operator may keep many oauth-server connections open, resulting in high memory consumption by the authentication operator and router pods.",
          "matchingRules": [
            {
              "type": "PromQL",
              "promql": {
                "promql": "max(cluster_proxy_enabled{type=~\"https?\"})"
              }
            }
          ]
        },
        {
          "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1942207#c3",
          "name": "VSphereNodeNameChanges",
          "message": "vSphere clusters leveraging the vSphere cloud provider may lose node names which can have serious impacts on the stability of the control plane and workloads.",
          "matchingRules": [
            {
              "type": "PromQL",
              "promql": {
                "promql": "cluster_infrastructure_provider{type=~\"VSphere|None\"}\nor\n0 * cluster_infrastructure_provider"
              }
            }
          ]
        },
        {
          "url": "https://access.redhat.com/solutions/5896081",
          "name": "VSphereHW14CrossNodeNetworkingError",
          "message": "Clusters on vSphere Virtual Hardware Version 14 and later may experience cross-node networking issues.",
          "matchingRules": [
            {
              "type": "PromQL",
              "promql": {
                "promql": "cluster_infrastructure_provider{type=~\"VSphere|None\"}\nor\n0 * cluster_infrastructure_provider"
              }
            }
          ]
        }
      ]
    },
    ...
  ],
  "edges": [...],
  "nodes": [...],
}
```

#### ClusterVersion representation

The CVO on a vSphere HW 14 cluster with a proxy configured would consume the above Cincinnati JSON, and populate ClusterVersion like:

```yaml
...
status:
  availableUpdates:
  - version: 4.6.43
    image: quay.io/openshift-release-dev/ocp-release@sha256:2b8efb25c1c9d7a713ae74b8918457280f9cc0c66d475e78d3676810d568b534
    url: https://access.redhat.com/errata/RHBA-2021:3197
    channels: ...
  - version: 4.6.42
    image: quay.io/openshift-release-dev/ocp-release@sha256:59e2e85f5d1bcb4440765c310b6261387ffc3f16ed55ca0a79012367e15b558b
    url: https://access.redhat.com/errata/RHBA-2021:3008
    channels: ...
  conditionalUpdates:
  ...
  - release:
    - version: 4.7.4
      image: quay.io/openshift-release-dev/ocp-release@sha256:999a6a4bd731075e389ae601b373194c6cb2c7b4dadd1ad06ef607e86476b129
      url: https://access.redhat.com/errata/RHBA-2021:3008
      channels: ...
    risks:
    - url: https://bugzilla.redhat.com/show_bug.cgi?id=1941840#c33
      name: AuthOAuthProxyLeakedConnections
      message: On clusters with a Proxy configured, the authentication operator may keep many oauth-server connections open, resulting in high memory consumption by the authentication operator and router pods.
      matchingRules:
      - type: PromQL
        promql:
          promql: "max(cluster_proxy_enabled{type=~\"https?\"})"
    - url: https://bugzilla.redhat.com/show_bug.cgi?id=1942207#c3
      name: VSphereNodeNameChanges
      message: vSphere clusters leveraging the vSphere cloud provider may lose node names which can have serious impacts on the stability of the control plane and workloads.
      matchingRules:
      - type: PromQL
        promql:
          promql: |
            cluster_infrastructure_provider{type=~"VSphere|None"}
            or
            0 * cluster_infrastructure_provider
    - url: https://access.redhat.com/solutions/5896081",
      name: VSphereHW14CrossNodeNetworkingError",
      message: Clusters on vSphere Virtual Hardware Version 14 and later may experience cross-node networking issues.
      matchingRules:
      - type: PromQL
        promql:
          promql: |
            cluster_infrastructure_provider{type=~"VSphere|None"}
            or
            0 * cluster_infrastructure_provider
    conditions:
    - lastTransitionTime: 2021-08-28T01:05:00Z
      type: Recommended
      status: False
      reason: MultipleReasons
      message: |
        Clusters on vSphere Virtual Hardware Version 14 and later may experience cross-node networking issues. https://access.redhat.com/solutions/5896081

        vSphere clusters leveraging the vSphere cloud provider may lose node names which can have serious impacts on the stability of the control plane and workloads. https://bugzilla.redhat.com/show_bug.cgi?id=1942207#c3

        On clusters with a Proxy configured, the authentication operator may keep many oauth-server connections open, resulting in high memory consumption by the authentication operator and router pods. https://bugzilla.redhat.com/show_bug.cgi?id=1941840#c33
  ...
```

#### Cluster administrator

The cluster administrator using a client to inspect available updates would see output like:

```console
$ oc adm upgrade
Cluster version is 4.6.23

Upstream: https://api.openshift.com/api/upgrades_info/graph
Channel: stable-4.6 (available channels: candidate-4.6, candidate-4.7, eus-4.6, fast-4.6, fast-4.7, stable-4.6, stable-4.7)

Recommended updates:

  VERSION	IMAGE
  4.6.43	quay.io/openshift-release-dev/ocp-release@sha256:2b8efb25c1c9d7a713ae74b8918457280f9cc0c66d475e78d3676810d568b534
  4.6.42	quay.io/openshift-release-dev/ocp-release@sha256:59e2e85f5d1bcb4440765c310b6261387ffc3f16ed55ca0a79012367e15b558b
  ...other unconditional or conditional for this cluster targets, in decreasing SemVer order...

Additional updates which are not recommended based on your cluster configuration are available, to view those re-run the command with --include-not-recommended.
```

And then, if they wanted to see the `Recommended!=True` entries:

```console
$ oc adm upgrade --include-not-recommended
Cluster version is 4.6.23

Upstream: https://api.openshift.com/api/upgrades_info/graph
Channel: stable-4.6 (available channels: candidate-4.6, candidate-4.7, eus-4.6, fast-4.6, fast-4.7, stable-4.6, stable-4.7)

Recommended updates:

  VERSION	IMAGE
  4.6.43	quay.io/openshift-release-dev/ocp-release@sha256:2b8efb25c1c9d7a713ae74b8918457280f9cc0c66d475e78d3676810d568b534
  4.6.42	quay.io/openshift-release-dev/ocp-release@sha256:59e2e85f5d1bcb4440765c310b6261387ffc3f16ed55ca0a79012367e15b558b
  ...other unconditional or conditional for this cluster targets, in decreasing SemVer order...

Supported but not recommended updates:

  Version: 4.7.4
  Image: quay.io/openshift-release-dev/ocp-release@sha256:999a6a4bd731075e389ae601b373194c6cb2c7b4dadd1ad06ef607e86476b129
  Recommended: False
  Reason: MultipleReasons
  Message:
    Clusters on vSphere Virtual Hardware Version 14 and later may experience cross-node networking issues. https://access.redhat.com/solutions/5896081

    vSphere clusters leveraging the vSphere cloud provider may lose node names which can have serious impacts on the stability of the control plane and workloads. https://bugzilla.redhat.com/show_bug.cgi?id=1942207#c3

    On clusters with a Proxy configured, the authentication operator may keep many oauth-server connections open, resulting in high memory consumption by the authentication operator and router pods. https://bugzilla.redhat.com/show_bug.cgi?id=1941840#c33

  Version: 4.6.99-example
  Image: quay.io/openshift-release-dev/ocp-release@...
  Recommended: Unknown
  Reason: PromQLError
  Message: Unable to evaluate PromQL to determine if the cluster is impacted by ExampleReason. https://example.com/ExampleReason

  Version: 4.6.30
  Image: quay.io/openshift-release-dev/ocp-release@sha256:476588ee99a28f39410372175925672e9a37f0cd1272e17ed2454d7f5cafff90
  Recommended: False
  Reason: ThanosDNSUnmarshalError
  Message: The monitoring operator goes Degraded=True when the user monitoring workflow is enabled due to DNS changes. https://access.redhat.com/solutions/6092191

  ...other conditional-and-not-allowed-for-this-cluster and conditional-but-could-not-evaluate..
```

They could update to a recommended release easily:

```console
$ oc adm upgrade --to 4.6.43
```

Or, after opting in with `--allow-not-recommended`, along a supported but not recommended path:

```console
$ oc adm upgrade --allow-not-recommended --to 4.7.4
```

#### ClusterVersion history

After updating along a supported but not recommended path, the history entry would contain an `acceptedRisks` entry:

```yaml
status:
  ...
  history:
  ...
  - startedTime: 2021-08-28T02:00:00Z
    completionTime": 2021-08-28T03:00:00Z
    state: Completed
    version: 4.7.4
    image: quay.io/openshift-release-dev/ocp-release@sha256:999a6a4bd731075e389ae601b373194c6cb2c7b4dadd1ad06ef607e86476b129
    verified: true
    acceptedRisks: |
      Updating from 4.6.23 to 4.7.4 is supported, but not recommended for this cluster.

      Reason: MultipleReasons

      Clusters on vSphere Virtual Hardware Version 14 and later may experience cross-node networking issues. https://access.redhat.com/solutions/5896081

      vSphere clusters leveraging the vSphere cloud provider may lose node names which can have serious impacts on the stability of the control plane and workloads. https://bugzilla.redhat.com/show_bug.cgi?id=1942207#c3

      On clusters with a Proxy configured, the authentication operator may keep many oauth-server connections open, resulting in high memory consumption by the authentication operator and router pods. https://bugzilla.redhat.com/show_bug.cgi?id=1941840#c33
  ...
```

### Risks and Mitigations

#### Clusters moving into the vulnerable state after updating

This enhancement proposes update-acceptance preconditions to keep vulnerable clusters from updating along an edge or to a release based on the cluster's current configuration.
For some criteria, like "is the cluster on the vSphere or `None` platform?", that configuration is static; an AWS cluster is not going to become a vSphere cluster post-update.
But some criteria, like "is an HTTP or HTTPS proxy configured?" or "are there vSphere hosts with HW 14 or unknown HW version?", are more mutable.
A cluster could have no proxy configured, update from 4.6 to 4.7.4, enable a proxy, and then have trouble with [rhbz#1941840][rhbz-1941840-impact-statement].
The current proposal has no provision for warning cluster administrators about configuration changes which might prove dangerous on their current release.

It might be possible to extend the current proposal to annotate `nodes` entries in the Cincinnati JSON response with arrays of known, vulnerable transitions.
But we'd want to distinguish between the configurations which the administrator could change (proxy configuration, vSphere HW version, etc.) and avoid warning about those which could not change (infrastructure platform).
If we could declare these vulnerabilities in `nodes`, it's possible that we would want to restrict `conditionalEdges` warnings to issues which only impacted the update itself.
In that case, the cluster-version operator would populate `conditionalUpdates[].risks` to be the union of update-time issues from Cincinnati's `conditionalEdges` and target-release issues from Cincinnati's `nodes`.

While we could extend `nodes` in future enhancements to include release vulnerabilities, leaving them off this enhancement means that we would need to continue to declare those same vulnerabilities in `conditionalEdges`, at least until we created [explicit versioning for the Cincinnati graph payloads][cincinnati-graph-api-versioning].

#### Stranding supported clusters

As described [in the documentation][support-documentation], supported updates are still supported even if incoming edges are blocked, and Red Hat will eventually provide supported update paths from any supported release to the latest supported release in its z-stream.
There is a risk, with the dynamic, per-cluster graph, that targeted edge blocking removes all outgoing update recommendations for some clusters on supported releases.
This risk can be mitigated in at least two ways:

* For the fraction of customer clusters that do not [opt-out of submitting Insights/Telemetry][uploaded-telemetry-opt-out], we can monitor [the existing `cluster_version_available_updates`][uploaded-telemetry-cluster_version_available_updates] to check for clusters running older versions which are still reporting no available, recommended updates.

* We can process the graph with tooling that removes all `conditionalEdges` and look for any supported versions without exit paths.

#### Malicious conditions

An attacker who compromises a cluster's [`upstream`][api-upstream] update service can already do some fairly disruptive things, like recommend updates from 4.1.0 straight to 4.8.0.
But at the moment, the cluster administrator (or whoever is writing to [ClusterVersion's `spec.desiredUpdate`][api-desiredUpdate]) is still in the loop deciding whether or not to accept the recommendation.

With this enhancement, the cluster-version operator will begin performing more in-cluster actions automatically, such as evaluating PromQL recommended by the upstream update service.
If the Prometheus implementation is not sufficiently hardened, malicious PromQL might expose the cluster to the attacker, with the simplest attacks being computationally intensive queries which cost CPU and memory resources that the administrator would rather be spending on more important tasks.
[Future monitoring improvements][mon-1772] might reduce the risk of expensive queries.
And it's also possible to teach the cluster-version operator to reject PromQL that does not match expected patterns.

Attacks can also be mitigated by pointing ClusterVersion `spec.upstream` at an uncompromised update service, or by clearing `channel` to ask the CVO to not retrieve update recommendations at all.

#### Clusters without local Prometheus stacks

The Prometheus and monitoring stacks are fairly resource-intensive.
There are [open proposals][mon-1569] to reduce their resource requirements.
It is possible that some future clusters decide they need to drop the Prometheus stack entirely, which would leave the CVO unable to evaluate conditions based on PromQL.
A future mitigation would be extending to support [non-PromQL filters](#non-PromQL-filters).

For clusters whose Prometheus stack is present but troubled, [the query coverage subsection](#query-coverage) explains how this enhancement identifies queries where the PromQL is successfully evaluated, but due to local cluster state (e.g. missing metrics, failed scrapes, etc.) the cluster cannot be clearly assigned to either the "vulnerable" or "immune" categories.

## Design Details

### Test Plan

[The graph-data repository][graph-data] should grow a presubmit test to enforce as much of the new schema as is practical.
Validating PromQL will require Go tooling to access [the Go parser][PromQL-go-parser].
The presubmit should require `url`, `name`, and `message` to be populated for new blocks.

Extending existing mocks and stage testing with data using the new schema should be sufficient for [update service support](#update-service-support-for-the-enhanced-schema).

Adding unit tests with data from a mock Cincinnati update service should be sufficient for [cluster-version operator support](#cluster-version-operator-support-for-the-enhanced-schema).

Ad-hoc testing when landing new features should be sufficient for `oc` and the web-console, although if they have existing frameworks for comparing output with mock cluster resources, that would be great too.

### Graduation Criteria

This will be released directly to GA.

#### Dev Preview -> Tech Preview

This will be released directly to GA.

#### Tech Preview -> GA

This will be released directly to GA.

#### Removing a deprecated feature

This enhancement does not remove or deprecate any features.

### Upgrade / Downgrade Strategy

The graph-data schema is already versioned.

We have [an open RFE][ota-123] to version the Cincinnati API, but even without that, adding new optional properties (`conditionalEdges`) for new features (edges which would have previously been completely blocked) is a backwards-compatible change.

### Version Skew Strategy

Newer update services consuming older graph-data will know that they can use their 1.1.0 parser on 1.0.0 graph-data without needing to make changes.

Older update services consuming newer graph-data will know that they are missing some features unique to 1.1.0, but that they will still get something reasonable out of the data by using their 1.0.0 parser (they'll just consider all conditional edges to be complete blockers).

Newer clients talking to older update services will not receive any `conditionalEdges`, but they will understand all the data that the update service sends to them.
Older clients talking to newer update services will not notice `conditionalEdges`, so those edges will continue to be unconditionally blocked for those clients.

Newer clients consuming older ClusterVersion will not receive any `conditionalUpdates`, but they will understand all the data included in the ClusterVersion object (e.g. `availableUpdates`).
Older clients consuming newer ClusterVersion will not notice `conditionalUpdates`, so those edges will continue to be unconditionally blocked for those clients.

## Implementation History

* 2021-09-23: `openshift/api` changes [implemented][implemented-API] for OpenShift 4.10.

## Drawbacks

Dynamic edge status that is dependent on cluster state makes [the graph-data repository][graph-data] a less authoritative view of the graph served to a given client at a given time, as discussed in [the *risks and mititgations* section](#risks-and-mitigations).
This is mitigated by ClusterVersion's [`status.history[].acceptedRisks`](#enhanced-clusterversion-representation), which records any cluster-version operator objections which the cluster administrator chose to override.
It is possible that cluster administrators would chose to clear that data, but it seems unlikely that they would invest significant effort in trying to cover their tracks when [the edges are supported regardless of whether they were recommended][openshift-docs-32091].

## Alternatives

### A positive edges schema in graph-data

This enhancement [extends](#enhanced-graph-data-schema-for-blocking-edges) graph-data's [existing blocked-edges schema][block-edges].
Graph-data does not include a positive `edges` schema.
I tried to sell folks on making edges an explicit, first-class graph-data concept in [graph-data#1][graph-data-pull-1], but lost out to loading edges dynamically from release-image metadata.
Benefit of positive edge definitions include:

* Update services would not need to load release images from a local repository in order to figure out which update sources had been baked inside.
* Accidentally adding or forgetting to block edges becomes harder to overlook when reviewing graph-data pull requests, because more data is local vs. being stored in external release images.

Drawbacks of positive edge definitions include:

* When adding new releases to candidate channels, the ART robots or parallel tooling would need to add new edges to graph-data.

### Update-service-side filtering

Instead of filtering cluster-side in the cluster-version operator, we could filter edges on the update-service side by querying [uploaded Telemetry][uploaded-telemetry] or [client-provided query parameters][cincinnati-for-openshift-request].
However, there is more data available in-cluster beyond what is uploaded as Telemetry.
And because we are [supporting edges which we do not recommend][openshift-docs-32091], we'd need to pass the reasons for not recommending those edges out to clusters anyway.
Passing enough information to make the decision completely on the cluster side is not that much more work.

Also in this space is clusters which are on restricted networks.
Those clusters could be reconfigured to ship their Telemetry or Insights uploads to local aggregators, or could have their Telemetry and Insights sneakernetted to Red Hat's central aggregators.
But client side filtering will work in restricted-network clusters without the need for any of that, especially now that [the OpenShift Update Service][osus] is making it easier to get Cincinnati responses to clusters in restricted networks.

### Failing open

Whether a conditional edge should be recommended for a given cluster depends on intent flowing from the graph-data maintainers, through update services, to the cluster-version operator, and then being evaluated in-cluster.
That flow can break down at any point; for example, the update service may only understand graph-data schema 1.0.0, and not understand 1.1.0.
Or the cluster-version operator may have trouble connecting to the in-cluster Thanos/Prometheues.
In those situations, this enhancement proposal recommends blocking the edge.

An alternative approach would have been failing open, where "failed to evaluate the graph-data maintainer intentions" would result in recommending edges.
That would reduce the risk of leaving a cluster stuck without any recommended updates.
But evalution failures should trigger alerts, so the appropriate administrators can resolve the issue, and delaying an update until we can make a clear determination is safer than updating while we are unable to make a clear determination.

As a final safety valve for situations where recovering evaluation capability would take too long, confident cluster administrators can force through the update guard.

### Query coverage

It might be tempting to define the PromQL values to return a series for matching clusters.
For example:

```yaml
promql: cluster_infrastructure_provider{type=~"VSphere|None"}
```

However, while that would clearly distinguish the "yes, this cluster is vulnerable" case, it would not distinguish between "this cluster is not vulnerable" and "for some reason, this cluster doesn't have `cluster_infrastructure_provider` data at the moment" or other issues with PromQL execution.

[The `PromQL` proposal](#enhanced-graph-data-schema-for-blocking-edges) specifies a single query that allows the update service to distinguish clusters in the matching state (query returns 1, risk matches, update is not recommended) from clusters that do not match (the query returns 0, risk does not match, update is recommended).
This allows the cluster-version operator to distinquish between the three states of `Recommended=True` (0), `Recommended=False` (1), or `Recommended=Unknown` (no result, for example because the query asked for metrics which the local Prometheus was failing to scrape).

### PromQL validation

[The graph-data repository][graph-data] currently includes Python and Rust code.
But the only supported PromQL parser is [written in Go][PromQL-go-parser].
We could write a parallel PromQL parser for validation in a non-Go language.
Benefits include avoiding the trouble of vendoring and compiling Go in graph-data presubmits.
Drawbacks include diverging from the official parser, increasing the risk that PromQL sneaks through the presubmit's parser and subsequently fails vs. the cluster's Go parser (although if this happened, we'd hear about it via Insights).

### Metadata to include when blocking a conditional request

A URI seems like the floor for metadata.
[The current proposal](#proposal) also includes a `name` and `message`.
The benefit is giving users some context about what they'd see if the clicked through to the detail URI.
The downside is that we need to boil the issue down to a slug and sentence or two, and that might lead to some tension about how much detail to include.

The properties are required, although personnally I would have preferred them to be optional.
A benefit of making the properties required is simpler enforcement and post-enforcement consumer code, due to the decreased flexibility.
A drawback of making the properties required is that an update service rejecting graph-data input, or a cluster-version operator rejecting Cincinnati JSON, etc. on the lack of one of these properies is unlikely to improve the user experience.
A conditional update risk that is populated except for a `name` (but which has a `url`, `message`, and `matchingRules`) is probably almost as understandable to a cluster administrator as one which does include a `name`.

[Presubmit guards on graph-data](#test-plan) will enforce a local policy to ensure these properties are populated.
So the likelihood that and cluster retrieved incomplete data from the `upstream` update service is low, just "graph-data presubmit tests had a hole that graph-data admins fell through" and "external folks running their own policy engines".

But if that ever happened, in the optional-property world, the CVO would execute the partial data to the best of its ability and pass the partial metadata on to the cluster admin (who could complain to their graph-data admins if the partial data wasn't actionable).

In the required-property alternative, the cluster-version operator (which needs to continue to successfully write ClusterVersion status) would self-censor, removing any unacceptable-to-the-Kube-API-server risks or conditional targets until the result would be acceptable.
That means that risks and conditional edges which would have been partially represented in the optional-property alternative ClusterVersion object are missing entirely from the required-property alternative.
This could potentially lead to "hey, where did that edge go?  It was there yesterday..." cluster admin confusion, of the sort that currently lead us to publish text-only errata like [this][text-only-errata-example].

Mitigating that "where did that edge go?" confusion in the required-property alternative, David expects graph-admins to be keeping watch over enough ClusterVersion content that they notice quickly if the CVO is not publishing some data that they expected to get through.
So that in cases where partial data does slip into graph-data, the graph-data admins are likely to notice and fix the data before graph admins have time to get too worked up.

David [was unconvinced][required-by-david] by my arguments about cluster admins being able to act on incomplete data (at least more than if the relevant conditional updates were missing entirely), so this enhancement uses required properties.

Also, while I personally prefer `reason` for the slug, [David prefers `name`][name-over-reason], so that's what this enhancement uses.

### Non-PromQL filters

Teaching the CVO to query the local Thanos/Prometheus shouldn't be too bad (`openshift/origin` already does this).
But we could have used `platforms` or something with less granularity to simplify the initial implementation (the CVO would pull the Infrastructure resource and compare with the configured `platforms` to decide if the edge was recommended).
For now, PromQL seems like the best balance of coverage vs. complexity, because it is an already-defined format where a single string can access a large slice of cluster state.

For example, the:

```yaml
matchingRules:
- type: PromQL
  promql:
    promql: |
      cluster_infrastructure_provider{type=~"VSphere|None"}
      or
      0 * cluster_infrastructure_provider
```

examples from [the 4.7.4 user story](#graph-data-administrators) could be replaced by a blocker entry with:

```yaml
matchingRules:
- type: platform
  platform:
    platforms:
    - VSphere
    - None
```

If we decide to add additional [cluster-condition types](#cluster-condition-type-registry) in future enhancements, the current approach allows for:

```yaml
matchingRules:
- type: PromQL
  promql:
    promql: |
      cluster_infrastructure_provider{type=~"VSphere|None"}
      or
      0 * cluster_infrastructure_provider
- type: platform
  platform:
    platforms:
    - VSphere
    - None
```

In which case, the cluster-version operator would:

* Attempt to evaluate the `PromQL` condition.
  * If the PromQL returns "match", the risk matches, and the update is not recommended.
    No further checks needed for this rule set.
  * If the PromQL returns "does not match", the risk does not match, and the update is recommended.
    No further checks needed for this rule set.
  * If the PromQL fails to evaluate, attempt to evauluate the `platform` condition.
    * If the `platforms` comparison matches, the risk matches, and the update is not recommended.
      No further checks needed for this rule set.
    * If the `platforms` comparison does not match, the risk does not match, and the update is recommended.
      No further checks needed for this rule set.
    * If the `platforms` comparison fails to evaulate, the update recommendation status is `Unknown`.

This will gracefully handle the addition of new cluster-condition types, as consumers can treat unrecognized types as evaluation failures.
As long as `matchingRules` contains at least one recognized, functioning type, the cluster-version operator will be able to distinguish recommended from not recommended.

#### Implicit always condition

[The `matchingRules` semantics](enhanced-graph-data-schema-for-blocking-edges) include:

> If no condition can be successfully evaluated, the update should not be recommended.

That means that an empty or unset array would mark the risk as a match for all clusters.
However, David and Scott [requested][always-type] an explicit type for "always matches", so we have [`type: Always`](#always).

#### Lowercase type names

I'd initially gone with `type: promql` and `type: always`, but David said:

> Always and PromQL.  We use CamelCase for enumerated values

So that's what we went with.

### Discriminating union implementation

[Interfaces][go-interface] are great.
It's a very flexible pattern to say "I don't care how you do it, but I want to call you with `$SYNTAX` and get results with `$SEMANTICS`".
However, flexibly storing the data to configure generic interfaces is more complicated.
With [Go's lack of union support][go-union], there are a few possible approachs:

* A central struct with a type identifier and a `RawMessage` sibling.
  This is also a common pattern in C, with structures like [`search.h`'s `ENTRY` having `void *data`][c-search-entry].
  Go's docs have [a `RawMessage` example][go-json-RawMessage-Unmarshal] example:

  ```go
  type Color struct {
    Space string
    Point json.RawMessage // delay parsing until we know the color space
  }
  ```

  to handle JSON like:

  ```json
  {
    "Space": "RGB",
    "Point": {"R": 98, "G": 218, "B": 255}
  }
  ```

  Benefits include easy, distributed extensibility: external consumers adding a new `Space` can just parse their new format out of `Point`'s `RawMessage`.
  Drawbacks include the slight awkwardness of parsing their data out of `Point`, and the useless nesting of `Point` in the JSON serialization.

* A central struct capable of holding all the data.
  [OpenShift's `BuildSource`][api-buildsource] uses this pattern.
  Applied to the above color example, it would be:

  ```go
  type Color struct {
    Space string
    RGB   *RGB
    YCbCr *YCbCr
  }
  ```

  to handle JSON like:

  ```json
  {
    "Space": "RGB",
    "RGB": {"R": 98, "G": 218, "B": 255}
  }
  ```

  This handles similar JSON to the `RawMessage` approach, so it has the same "useless `Point`/`RGB` in the JSON" drawback.
  Drawbacks also include the lack of external extensibility; if an external consumer wants to add a new `Space`, they need to fork the `struct` and insert their new pointer.
  Benefits include extremely convenient Go handling, with no need for nested (un)marshal steps.

* Using a generic `Extra` map:

  ```go
  type Color struct {
    Type  string
    Extra map[string]interface{}
  }
  ```

  This allows for flattened JSON:

  ```json
  {
    "Space": "RGB",
    "R": 98,
    "G": 218,
    "B": 255
  }
  ```

  Benefits include the lack of `Point` nesting in the JSON, and the easy extensibility of the `RawMessage` approach.
  Drawbacks include the tedious `MarshalJSON` and `UnmarshalJSON` hoop-jumping I describe in [the enhanced ClusterVersion section](#enhanced-clusterversion-representation).

I'd initially gone with the `Extra` map, because there are only a handful of developers working with the JSON (un)marshal gymnastics, while there are many more cluster adminsitrators poking into JSON or YAML renderings of ClusterVersion.
But Ben and David convinced me that it's ok blocking external extensibility and requiring folks to go through openshift/api or fork to add new cluster-condition types.
And the central-struct approach collects all the documentation in one place, making it easier to discover (turning the lack of extensibility from a draback into a benefit).

David went a step further and required [an enum for `type`][enum-by-david].
That means that, not only can the discriminating union hold type-specific configuration for an unrecognized type, it cannot hold the fact that an unrecognized type was even requested.
So if we add additional [non-PromQL filters](#non-promql-filters) in future enhancements, the CVO will need to self-censor to keep the Kube-API-server from rejecting the not-in-the-enum `type`, and you get the "where did that edge go?" exposure for cluster-admins discussed in[the section about metadata alternatives](#metadata-to-include-when-blocking-a-conditional-request).

#### Flattening or nesting per-type properties in discriminating unions

As a sub-choice of the central-struct approach to discriminating unions, I had to decide between:

```go
type ClusterCondition struct {
	Type   string
	PromQL string `json:"PromQL,omitempty"`
}
```

with JSON like:

```json
{
  "type": "PromQL",
  "PromQL": "max(cluster_proxy_enabled{type=~\"https?\"})"
}
```

and:

```go
type ClusterCondition struct {
	Type   string
	PromQL *PromQLClusterCondition `json:"PromQL,omitempty"`
}

type PromQLClusterCondition struct {
	PromQL string `json:"PromQL"`
}
```

with JSON like:

```json
{
  "type": "PromQL",
  "PromQL": {
    "PromQL": "max(cluster_proxy_enabled{type=~\"https?\"})"
  }
}
```

The former avoids the useless `PromQL` nesting layer, as long as [the `PromQL` type](#promql) does not require additional configuration beyond the single query string.
The latter allows for extension if we decide the `PromQL` type requires additional configuration.

In the `PromQLClusterCondition` approach, the query does not need to be a pointer, because once you have decided to use PromQL it is no longer optional and will always be required for backwards compatibility.

### Reporting recommendation freshness

Currently [`availableUpdates`][api-availableUpdates] does not have a way to declare the freshness of its contents (e.g. "based on data retrieved from the upstream update service at `$TIMESTAMP`").
We do set the `RetrievedUpdates` condition and eventually alert if there are problems retrieving updates, and the expectation is that if we aren't complaining about being stale, we're fresh enough.
We could take the same approach with `conditionalUpdates`, but now that we also have "based on evaluations of the in-cluster state at `$TIMESTAMP`" in the mix, we may want to proactively declare the time.
On the other hand, continually bumping something that's similar to the node's `lastHeartbeatTime` is a bunch of busywork for both the cluster-version operator and the API-server.
For the moment, we have decided that the additional transparency is not with it.

### Blocking CVO gates on update acceptance

[The update-client support section](#update-client-support-for-the-enhanced-schema) suggests a client-side `--allow-not-recommended` gate on choosing a supported-but-not-recommended target.
[The cluster-version operator support section](#cluster-version-operator-support-for-the-enhanced-schema) currently calls for informative [`history[].acceptedRisks`](enhanced-clusterversion-representation) complaints.

But the CVO could have a blocking gate, and require [`force`][api-force] or similar to override those guards.
A benefit would be that the [`upstream`][api-upstream] recommendation service would be much harder to casually ignore.
A drawback would be that blocking gates would be a large departure from the current lack of any gates `upstream`-based at all.
Skipping CVO-side gates entirely would make it more difficult to reconstruct the frequency of this behavior, compared to scraping it out of ClusterVersion's `history` in Insights tarballs.
For now, non-blocking CVO-side `acceptedRisks` complaints feel like a happy middle ground.

### Structured override history

The [enhanced ClusterVersion representation](#enhanced-clusterversion-representation) adds an `string` `acceptedRisks` history entry.
That entry could instead be structured, with slugs for each overridden condition and messages explaining the why the CVO at the time felt the condition was unhappy.
A structured entry would allow for convenient, automated analysis of frequently-overriden conditions.
But there is a risk that we would make a poor guess at structure, and need follow-up, schema-migrating changes to iterate towards better structures.
With the single string, automated consumers are restricted to a boolean "were there any accepted risks?", although in exceptional cases they might want to search the message for particular substrings.
And accepted risks themselves should be exceptional cases, so using a single string to hold a consolidated message seems like a sufficient level of engineering for the scale of this issue.
We can revisit the structure (in an awkward, schema-migrating change) if analysis of the single strings shows that actually, accepted risks are not as execeptional as we'd thought, if we decide we'd need additional structure to get a handle on the now-larger issue.

[always-type]: https://github.com/openshift/enhancements/pull/821#discussion_r709177386
[api-availableUpdates]: https://github.com/openshift/api/blob/67c28690af52a69e0b8fa565916fe1b9b7f52f10/config/v1/types_cluster_version.go#L126-L133
[api-buildsource]: https://github.com/openshift/api/blob/85977bee07221f012896dcc53b77f44da0be0c4e/build/v1/types.go#L412-L437
[api-cluster-version-status]: https://github.com/openshift/api/blob/67c28690af52a69e0b8fa565916fe1b9b7f52f10/config/v1/types_cluster_version.go#L78-L134
[api-desiredUpdate]: https://github.com/openshift/api/blob/67c28690af52a69e0b8fa565916fe1b9b7f52f10/config/v1/types_cluster_version.go#L43-L57
[api-force]: https://github.com/openshift/api/blob/67c28690af52a69e0b8fa565916fe1b9b7f52f10/config/v1/types_cluster_version.go#L248-L256
[api-history]: https://github.com/openshift/api/blob/67c28690af52a69e0b8fa565916fe1b9b7f52f10/config/v1/types_cluster_version.go#L149-L193
[api-message]: https://github.com/openshift/api/blob/67c28690af52a69e0b8fa565916fe1b9b7f52f10/config/v1/types_cluster_operator.go#L135-L139
[api-reason]: https://github.com/openshift/api/blob/67c28690af52a69e0b8fa565916fe1b9b7f52f10/config/v1/types_cluster_operator.go#L131-L133
[api-upstream]: https://github.com/openshift/api/blob/67c28690af52a69e0b8fa565916fe1b9b7f52f10/config/v1/types_cluster_version.go#L59-L63
[block-edges]: https://github.com/openshift/cincinnati-graph-data/tree/f7528c3120d818c3365361b281b6079b6a858397#block-edges
[blocking-4.5.3]: https://github.com/openshift/cincinnati-graph-data/commit/8e965b65e2974d0628ea775c96694f797cd02b1e#diff-72977867226ea437c178e5a90d5d7ba8
[c-search-entry]: https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/search.h.html#tag_13_39_03
[cincinnati]: https://github.com/openshift/cincinnati
[cincinnati-api]: https://github.com/openshift/cincinnati/blob/master/docs/design/cincinnati.md
[cincinnati-for-openshift-design]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md
[cincinnati-for-openshift-request]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md#request
[cincinnati-graph-api]: https://github.com/openshift/cincinnati/blob/master/docs/design/cincinnati.md#graph-api
[cincinnati-graph-api-versioning]: https://github.com/openshift/enhancements/pull/870
[cincinnati-spec]: https://github.com/openshift/cincinnati/blob/master/docs/design/cincinnati.md
[enum-by-david]: https://github.com/openshift/api/pull/1011#discussion_r712428578
[go-interface]: https://golang.org/ref/spec#Interface_types
[go-json-RawMessage-Unmarshal]: https://pkg.go.dev/encoding/json#example-RawMessage-Unmarshal
[go-union]: https://github.com/golang/go/issues/6213
[graph-data]: https://github.com/openshift/cincinnati-graph-data
[graph-data-pull-1]: http://github.com/openshift/cincinnati-graph-data/pull/1
[graph-data-schema-version]: https://github.com/openshift/cincinnati-graph-data/tree/f7528c3120d818c3365361b281b6079b6a858397#schema-version
[implemented-API]: https://github.com/openshift/api/pull/1011
[json-array]: https://datatracker.ietf.org/doc/html/rfc8259#section-5
[json-object]: https://datatracker.ietf.org/doc/html/rfc8259#section-4
[json-string]: https://datatracker.ietf.org/doc/html/rfc8259#section-7
[mon-1569]: https://issues.redhat.com/browse/MON-1569
[mon-1772]: https://issues.redhat.com/browse/MON-1772
[name-over-reason]: https://github.com/openshift/enhancements/pull/821#discussion_r705538121
[oc]: https://github.com/openshift/oc
[openshift-docs-32091]: https://github.com/openshift/openshift-docs/pull/32091
[osus]: https://docs.openshift.com/container-platform/4.8/updating/understanding-the-update-service.html
[ota-123]: https://issues.redhat.com/browse/OTA-123
[PromQL]: https://prometheus.io/docs/prometheus/latest/querying/basics/
[PromQL-or]: https://prometheus.io/docs/prometheus/latest/querying/operators/#logical-set-binary-operators
[PromQL-go-parser]: https://github.com/openshift/prometheus/tree/989765ceb07f61a85c65777dba1ff8fb7651d647/promql/parser
[required-by-david]: https://github.com/openshift/api/pull/1011#discussion_r712426388
[rhbz-1838007]: https://bugzilla.redhat.com/show_bug.cgi?id=1838007
[rhbz-1858026-impact-statement]: https://bugzilla.redhat.com/show_bug.cgi?id=1858026#c28
[rhbz-1858026-impact-statement-request]: https://bugzilla.redhat.com/show_bug.cgi?id=1858026#c26
[rhbz-1941840-impact-statement]: https://bugzilla.redhat.com/show_bug.cgi?id=1941840#c33
[rhbz-1957584-impact-statement]: https://bugzilla.redhat.com/show_bug.cgi?id=1957584#c19
[support-documentation]: https://docs.openshift.com/container-platform/4.7/updating/updating-cluster-between-minor.html#upgrade-version-paths
[text-only-errata-example]: https://access.redhat.com/errata/RHBA-2021:3415
[tombstone]: https://github.com/openshift/cincinnati-graph-data/tree/f7528c3120d818c3365361b281b6079b6a858397#tombstones
[uploaded-telemetry]: https://docs.openshift.com/container-platform/4.7/support/remote_health_monitoring/showing-data-collected-by-remote-health-monitoring.html#showing-data-collected-from-the-cluster_showing-data-collected-by-remote-health-monitoring
[uploaded-telemetry-cluster_version_available_updates]: https://github.com/openshift/cluster-monitoring-operator/blame/e104fcc9a5c2274646ee3ac50db2cfb7905004e4/Documentation/data-collection.md#L43-L47
[uploaded-telemetry-opt-out]: https://docs.openshift.com/container-platform/4.7/support/remote_health_monitoring/opting-out-of-remote-health-reporting.html
[web-console]: https://github.com/openshift/console
