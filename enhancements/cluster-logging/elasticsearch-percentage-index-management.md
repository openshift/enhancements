---
title: elasticsearch-percentage-index-management
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
creation-date: 2020-12-11
last-updated: 2021-01-18
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# elasticsearch-percentage-index-management

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Migration plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Currently our index management within OpenShift Elasticsearch is limited to just being time based for rollover and deletion. The goal of this proposal is to introduce and outline extending this to also include amount of storage used. There are a two different ways this can be measured within an ES cluster such as per index and per node -- this proposal will also discuss the differences.

## Motivation

Currently our managed Elasticsearch cluster is limited to only specify rollover and deletion policies based on the age of indices which does not translate easily into disk utilization which can mean unforseen expenses for users trying to fit their usage into a specific time window of supporting index age.
If instead they can tell our index management to start deleting the oldest indices once usage reaches a certain percentage this gives a better user experience.

### Goals
The specific goals of this proposal are:

* Outline the addition to the API to provide a means to configure usage for our policies

* Update the current cronjobs to be able to make decisions based on usage

We will be successful when:

* Users are able to also configure their policies with "usage" instead of just age.

* We also contribute this feature to opendistro so that we stay aligned with what they offer (for future adoptions).

### Non-Goals

* For our ES 6.x cluster we will not be implementing this using Opendistro due to gaps in that version of the plugin

## Proposal

Currently we offer a mechanism for users to configure their index lifecycles based on time only. This can be difficult for customer when trying to anticipate their disk usage, especially if they are charged for the storage they use and need to scale up because they were unable to anticipate a large amount of logs being processed.

This seeks to offer a mechanism by which one can denote the usage percentage (either of each node or of the total cluster) for indicating which indices should be deleted over.

### User Stories

#### As an user of EO, I want to be able to configure my delete policies based on a percent usage

Currently we only offer a mechanism to delete indicies based on the age of the index.

### Implementation Details

#### Assumptions

* This will be going out with our ES6 operator which means it will be a part of our ES 6.x cluster

* It will be possible to configure phases based on both usage and time. We will trigger on whichever comes first.

#### Security

No additional security concerns need to be addressed by this.

### Risks and Mitigations

## Design Details

We will add a field that is used by the delete phase. We will delete on either MinAge or PercentUsage, whichever is evaluated as true first.

### index_management_types.go

We will add a usage field for our phases to use:

```go
// +k8s:openapi-gen=true
type IndexManagementDeletePhaseSpec struct {
	// The minimum age of an index before it should be deleted (e.g. 10d)
	//
	MinAge TimeUnit `json:"minAge, optional"`

  // The percent disk usage to wait for before deleting indices for this alias (e.g. 75)
  //
  PercentUsage int32 `json:"usage, optional"`
}

```

### Test Plan

#### Unit Testing

* We want to ensure that when PercentUsage is specified we correctly update our delete cronjob scripts to use it.

#### Integration and E2E tests

* We want to ensure that when PercentUsage is specified, see that we do not delete indices for that alias until disk utilization reaches the specified value.

* We want to ensure that if both MinAge and PercentUsage are specified, see that we will trigger and delete indices when one of the conditions are true.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

#### GA

* Gather feedback from users rather than just developers
* End user documentation
* Sufficient test coverage

### Version Skew Strategy

Given we are adding an optional field to an existing API there should be minimal concern for upgrading. This would also be happening as we are changing our release process from ART to CPaaS and therefore changing our operator versioning.

## Implementation History

| release|Description|
|---|---|
|5.3| **GA** - Initial release

## Drawbacks

Currently this is not possible in the OpenDistro Index Management plugin and causes us to deviate from what they offer. We would need to ensure that it is added by us prior to us moving to ES7 (and adopting their plugins).

The naming convention "MinAge" when paired with "PercentUsage" could give a false impression -- we may end up deleting indices before they reach the "MinAge" because they are the oldest indices and with "Usage" being reached. This may just need to be framed differently within the docs and field descriptions.

## Alternatives

Using the lightweight-curator (link below)

## Infrastructure Needed

None


## Links

* [Curator deleting indices by space](https://www.elastic.co/guide/en/elasticsearch/client/curator/current/filtertype_space.html)

* [lightweight-curator](https://github.com/Tessg22/lightweight-curator)

* [Original EO pr](https://github.com/openshift/elasticsearch-operator/pull/441)