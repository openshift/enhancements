---
title: elasticsearch-hot-warm-architecture
authors:
  - "@ewolinetz"
reviewers:
  - "@jcantrill"
  - "@bparees"
  - "@alanconway"
approvers:
  - "@jcantrill"
  - "@bparees"
  - "@alanconway"
creation-date: 2020-10-13
last-updated: 2020-11-24
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# elasticsearch-hot-warm-architecture

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Migration plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The purpose of this is to increase the configuration for an ES cluster
to better optimize indices and cluster priorities and maintain that
they can still be queried. This has a potential to improve retention
of our logs (but is not our focus). Currently our logs are either
maintained as a high priority index or they are deleted. We seek to
emulate the back-end process that Elasticsearch makes available in the
form of [x-pack
API](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/index-lifecycle-management-api.html)
end points.

## Motivation

Currently our managed Elasticsearch cluster is restricted in the amount of days of retention we can provide in part due to the amount of metadata ES keeps in memory related to its indices. If we are taking advantage of the different index management APIs provided by Elasticsearch we can seek to better tune clusters to increase retention and overall performance.

### Goals
The specific goals of this proposal are:

* Outline the addition to the API to provide a means to configure WARM policies

* Replace|repurpose the current rollover cronjob to also perform hot to warm transition actions

We will be successful when:

* Users are able to configure WARM policies so that we can optimize their cluster usage

* We are able to transition a user's index from one policy to the next based on their configuration

### Non-Goals

* We will not be marking different nodes for the different policies and reallocating shards based on that

* We will not be implementing this using the Opendistro plugin

* This does not seek to increase possible retention, it may be a by-product but it is not a metric for measuring success here.

## Proposal

Currently we offer a mechanism for users to configure their index lifecycles based on two states: hot and delete. By default indices are created and stay in the hot state, where they can be rolled over based on a configuration, however they stay in a "hot" state until they are eventually deleted based on the delete policy.

Elasticsearch offers mechanisms to move indices between hot, warm,
cold, and delete and discusses approaches for this [in a blog
article](https://www.elastic.co/blog/implementing-hot-warm-cold-in-elasticsearch-with-index-lifecycle-management). However,
this functionality is a x-pack feature and we do not support
these. [Opendistro](https://opendistro.github.io/for-elasticsearch-docs/docs/ism/)
offers a way to do this as well, however their plugins require ES7. We
achieve our current functionality using cronjobs -- one for delete and
one for rollover.

Within the context of Elasticsearch ILM:
1) Moving the index into a Warm state means: the index is force merged (to reduce the number of segments in its shards); its replica count is reduced; the index priority is reduced.
The reason for doing this is: the indices are not being written to; they may not be queried against as much; they do not need to be as distributed; they can be a lower priority when it comes to index recovery (meaning they would be recovered after high priority indices are).
These optimizations also help to reduce the heap usage for those indices in the cluster.

2) Moving into a Cold state further reduces its index priority to be lower than Warm (but still higher than delete) and, if one was using specific nodes to house indices of certain state it would move from Warm to Cold nodes. This is different than Closing an index because the index can still be queried (if one were to Close an index it would need to be Opened first before it can be searched).

### User Stories

#### As an EO admin, I want to be able to configure a warm policy for my indices so that I can keep searchable data around longer and take up less resources

Currently we keep indices at their original primary and replica shard count which comes at a cost in ES. Reducing these can help us increase our retention policy.

### Implementation Details

#### Assumptions

* This will be going out with our ES6 operator which means it will be a part of our ES 6.x cluster

* Warm and Rollover phases will be synonymous if not specified to be separate

#### Security

No additional security concerns need to be addressed by this.

### Risks and Mitigations

## Design Details

Currently the API reads differently for Hot and Delete. Hot uses the MaxAge of an index and Delete the MinAge. For the sake of this proposal Warm will follow the same convention as Delete, a later iteration can seek to align all phases to use the same terminology (non-goal).

Given that having three cronjobs for a single index (with all three policies defined) may be noisy, as part of this we should also consolidate the index management cronjobs into a single one.

### index_management_types.go

We will create another phase spec as a placeholder for Warm:

```go
type IndexManagementPhasesSpec struct {
  // +nullable
  Hot *IndexManagementHotPhaseSpec `json:"hot,omitempty"`
  // +nullable
  Warm *IndexManagementWarmPhaseSpec `json:"warm,omitempty"`
  // +nullable
  Delete *IndexManagementDeletePhaseSpec `json:"delete,omitempty"`
}

type IndexManagementWarmPhaseSpec struct {
  // The minimum age of an index before it should be moved to warm once rolled over (e.g. 1d)
  MinAge TimeUnit `json:"minAge"`
}
```

### Test Plan

#### Unit Testing

* In the case a Warm Policy is defined we correctly create the config such that we would move indices into a warm state based on what is provided.

* If no Warm Policy is defined, we do not create a config for it.

#### Integration and E2E tests

* Verify that we see indices correctly get force merged and their replicas decreased. (Logic in EO will likely need to be updated as it currently will update the replica count of all indices currently)

### Graduation Criteria

#### GA

* Gather feedback from users rather than just developers
* End user documentation
* Sufficient test coverage

### Version Skew Strategy

Given we are adding an optional spec/field to an existing API there should be minimal concern for upgrading. This would also be happening as we are changing our release process from ART to CPaaS and therefore changing our operator versioning.

## Implementation History

| release|Description|
|---|---|
|6.0| **GA** - Initial release

## Drawbacks

Currently we are unable to leverage using Opendistro plugins, so we must continue forward with using cronjobs.

## Alternatives

None

## Infrastructure Needed

None
