---
title: neat-enhancement-idea
authors:
  - "@wking"
reviewers:
  - "@abhinavdahiya"
  - "@crawford"
  - "@smarterclayton"
approvers:
  - TBD
creation-date: 2019-11-19
last-updated: 2019-11-21
status: implementable
---

# Automatic Updates

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Operator-managed updates are [one of the key benefits of OpenShift 4][architecture].
Cluster update suggestions are distributed via [Cincinnati][cincinnati-openshift], so clusters only receive update recommendations that are appropriate for their cluster.
While manually approving updates at a per-cluster level is possible, it should not be required.
This enhancement adds a new `automaticUpdates` property to [the ClusterVersion `spec`][api-spec] so cluster administrators can opt in to or out of automatic updates.

## Motivation

Manually approving updates is tedious, doesn't scale well for administrators managing many clusters, and delays the application of potential security fixes.
Administrators may wish to enable automatic updates for any of those or other reasons.
Currently, administrators must implement custom polling logic to check for and apply any available updates.
This enhancement would provide those administrators with a convenient property instead.

### Goals

* Make it easy to opt in to and out of automatic updates.

### Non-Goals

* Support calendar gating or other logic about when updates may be attempted.
* Choose whether the installer defaults to enabling automatic updates or not for new clusters.

## Proposal

As proposed in [this API pull request][api-pull-request], to add a new property:

```go
// automaticUpdates enables automatic updates.
// +optional
AutomaticUpdates bool `json:"automaticUpdates,omitempty"`
```

to [`ClusterVersionSpec`][api-spec] to record the administrator's preference.

### User Stories

#### Leading Clusters

Alice has clusters she wants to update as soon as an updated release is available in her configured channel.
Before this enhancement she would need to construct a poller to run `oc adm upgrade --to-latest` or similar.
With this enhancement, she can set `automaticUpdates` and does not need to write an external poller.

### Risks and Mitigations

Cluster-managed updates are a relatively new thing, and while we have dozens of OpenShift 4 releases out so far, we have had a few updates result in stuck clusters (like [this][machine-config-operator-drain-wedge].
Allowing automatic updates would make it more likely that update failures happen when there is no administrator actively watching to notice and recover from the failure.
This is mitigated by:

* Alerting.
    The cluster should fire alerts (FIXME: which ones?) when an update gets into trouble.
    Administrators can configure the cluster to push those alerts out to the on-call administrator to recover the cluster.
* Stability testing.
    We are continually refining our CI suite and processing Telemetry from live clusters in order to assess the stability of each update.
    We will not place updates in production channels unless they have proven themselves stable in earlier testing, and we will remove update recommendations from production channels if a live cluster trips over a corner case that we do not yet cover in pretesting.

There are also potential future mitigations:

* Phased rollouts, where Cincinnati spreads an update suggestion out over a configurable time window.
    With hard cutovers and automatic updates, many clusters in a given channel could attempt a new update simultaneously.
    If that update proves unstable, many of those updates would already be in progress by the time the first Telemetry comes back with failure messages.
    A phased rollout would limit the number of simultaneously updating clusters to give Telemetry time to come back so we could stop recommending updates that proved unstable on live clusters not yet covered in pretesting.

There is also a security risk where a compromised upstream Cincinnati could recommend cluster updates that were not in the cluster's best interest (e.g. 4.2.4 -> 4.1.0).
This is mitigated by:

* [The configurable `upstream` property][api-upstream].
    Administrators who do not trust the default `upstream` to be sufficiently reliable may point their clusters at an upstream that they control which they can secure to their satisfaction.
    The drawback to this approach would be that now they need tooling to approve graph updates as Red Hat makes changes, which will introduce some delays to security-fix rollouts.
    But that approval would scale per-custom-Cincinnati instead of scaling per-cluster.

There are also potential future mitigations:

* The cluster-version operator could be taught to limit automatic updates to those which appear in the [signed metadata][cluster-version-operator-release-metadata] of either the source or target release.
    The drawback to this approach is that updates would be limited to those expected when the releases were created and signed.

## Design Details

### Test Plan

In addition to unit tests, this feature will be tested in a fleet of canary clusters which will run with automatic updates enabled.
Having the canaries successfully and automatically update in to and out of a candidate release will be part of release and update stabilization criteria.
The "out of" testing is important, because we cannot strand production clusters on dead-end releases.
When testing a new candidate release B, the full loop for a short-lived test could be:

1. Launch a cluster using a previously-accepted release A, with automatic updates enabled and an [`upstream`][api-upstream] pointing at a per-test service.
2. Add the candidate release B to the per-test upstream, with a recommended update edge from A to B.
3. Watch the cluster successfully update from A to B.
4. Replace the A to B update edge with a B to A update edge in the per-test upstream.
5. Watch the cluster successfully update from B to A.

We might have update edges that are not reversible, so we don't want that A->B->A test to be blocking, but when we decide to stabilize a release candidate where B->A is unstable, we'd want to perform additional verification to convince ourselves that we weren't creating a dead end.

### Graduation Criteria

The ClusterVersion object is already GA, so there would be nothing to graduate or space to cool a preview implementation.

### Upgrade / Downgrade Strategy

The YAML rendering of the old and updated type are compatible, with the only difference being downgrades to earlier releases (where the property was not available) will clear any stored value.
Upon updating back to a version where the property is available, administrators would need to manually set it to restore automatic update behavior.

### Version Skew Strategy

As a boolean property, this would be [`false` by default][go-zero-values] on updates to the updated ClusterVersion Custom Resource Definition.
This preserves semantics for existing clusters, where manual updates are the only option.

The installer or administrator could set `automaticUpdates` `true` at cluster-creation time if they wished to have it enabled out of the box on new clusters.

## Implementation History

* [API pull request][api-pull-request].

## Drawbacks

The drawbacks are increased exposure to [automatic-update risks](#risks-and-mitigations).

## Alternatives

A [schedule structure][schedule-proposal] like:

```yaml
automaticUpdates:
  schedule:
  - Mon..Fri *-2..10-* 10:00,14:00
  - Tues..Thurs *-11..1-* 11:00
```

The schedule structure was designed to support maintenance windows, allowing for updates on certain days or during certain times.
But we could enforce maintenance windows on a cluster independently of configuring automatic updates.
For example, a `schedule` property directly on [the ClusterVersion `spec`][api-spec] would protect administrators from accidentally using the web console or `oc adm upgrade ...` to trigger an update outside of the configured window.
Instead, administrators would have to adjust the `schedule` configuration or set an override option to trigger out-of-window updates.
Customization like this can also be addressed by intermediate [policy-engine][cincinnati-policy-engine], without involving the ClusterVersion configuration at all.
Teaching the cluster-version operator about a local `schedule` filter is effectively like a local policy engine.
I'm not against that, but it seems orthogonal to an automatic update property.

## Infrastructure Needed

There are already canary clusters will polling automatic updates, so we can use those for testing and will not need to provision more long-running clusters to excercise this enhancement.
We can provision additional short-lived clusters in existing CI accounts if we want to provide additional end-to-end testing.

[api-pull-request]: https://github.com/openshift/api/pull/326
[api-spec]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L28-L73
[api-upstream]: https://github.com/openshift/api/blob/082f8e2a947ea8b4ed15c9c0f7b190d1fd35e6bc/config/v1/types_cluster_version.go#L56-L60
[architecture]: https://docs.openshift.com/container-platform/4.1/architecture/architecture.html#architecture-platform-management_architecture
[cincinnati-openshift]: https://github.com/openshift/cincinnati/blob/c59f45c7bc09740055c54a28f2b8cac250f8e356/docs/design/openshift.md
[cincinnati-policy-engine]: https://github.com/openshift/cincinnati/blob/c59f45c7bc09740055c54a28f2b8cac250f8e356/docs/design/cincinnati.md#policy-engine
[cluster-version-operator-release-metadata]: https://github.com/openshift/cluster-version-operator/blob/0386842157d4db5d27ab5935db3cb69c52687d9d/pkg/payload/payload.go#L86
[go-zero-values]: https://golang.org/ref/spec#The_zero_value
[machine-config-operator-drain-wedge]: https://bugzilla.redhat.com/show_bug.cgi?id=1761557
[schedule-proposal]: https://github.com/openshift/api/pull/326#issuecomment-500081307
