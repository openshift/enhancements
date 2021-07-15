---
title: Automation of etcd defragmentation
authors:
  - "@hexfusion"
reviewers:
  - "@marun"
  - "@lilic"
  - "@deads"
approvers:
  - "@smarterclayton"
  - "@deads"
creation-date: 2021-07-14
last-updated: 2021-07-14
status: implementable

see-also:
  - "https://etcd.io/docs/v3.5/op-guide/maintenance/"
---
# Automation of etcd defragmentation

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Enable automated defragmentation of the etcd cluster state.

## Motivation

As defragmentation is an important part of cluster health, managing this operation with a controller aligns with the 
principals and promises of OpenShift 4. 

### Goals

- Provide an automated mechanism which provides deframentation as the result of observations from the cluster.

### Non-Goals

- Provide an API which allows customization of the defragmentation thresholds.

## Proposal

We propose to add a defrag controller to the `cluster-etcd-operator` that will check the endpoints of the etcd 
cluster every 10 minutes for observable signs of fragmentation. 

[openshift/cluster-etcd-operator PR #625](https://github.com/openshift/cluster-etcd-operator/pull/625)

### User Stories

- I want my cluster to manage maintenance of etcd backend without interaction or planning by admin.
- I want to ensure that heavy churn workloads do not negatively impact cluster performance.
- I want to ensure that cluster operands utilize only the necessary resources to provide service.

Large production clusters such as OpenShift CI feel the pain of fragmentation daily. The constant churn of resources 
from creating and deleting many jobs can quickly lead to resource bloat as the result of fragmentation. This growth can 
lead to OOM events and downtime. This controller was prototyped as a result of a CI outage.

### Implementation Details/Notes/Constraints

## etcd Storage Backend
etcd's storage backend is powered by `bbolt` a B+Tree memory mapped key/value store with MVCC support. While MVCC is
necessary to support fully concurrent transactions and the heart of the `Watch` functionality.
MVCC poses a performance challenge for `bbolt` because the db can store many versions of the keys value. These
versions can build up indefinitely and result in memory and disk performance issues. The solution is to regularly
compact the key space.

compaction is essentially a delete action in `bbolt`, where the prevision versions of a keys value are pruned.
`bbolt` organizes data on pages and when a page is no longer used as the result of a db mutation it is called a
free page and an accounting of all free pages are stored in a freelist for later reuse.

Compaction does reduce the memory footprint by removing old keys but free pages can not be reclaimed back to disk
without defragmentation.

## Defrag Controller

During the operator sync the controller will make a  call to the cluster API using the `MemberListRequest` RPC which 
returns the current list of etcd members. We will then loop these members and for each dial the maintenance API using
the `StatusRequest` RPC which will return detailed information on the backend status.

```go
type StatusResponse struct {
    // dbSize is the size of the backend database physically allocated, in bytes, of the responding member.
    DbSize int64 `protobuf:"varint,3,opt,name=dbSize,proto3" json:"dbSize,omitempty"`
    [...] 
    // dbSizeInUse is the size of the backend database logically in use, in bytes, of the responding member.
    DbSizeInUse int64 `protobuf:"varint,9,opt,name=dbSizeInUse,proto3" json:"dbSizeInUse,omitempty"`
}
```

From these two fields we can conclude the total fragmented space on that etcd members backend store. If the
percentage of bytes of the total keyspace are over a defined threshold percentage. The controller will 
utilize maintenance API again and issue a `DefragmentRequest` RPC.

Defragmentation is I/O blocking thus important to only defrag a single etcd at a time. Additionally, to reduce
the possibility of multiple leader elections we defragment the leader last. The amount of time that is required to
defragment the store is directly related to the amount of free pages that exists. For this reason we feel it
makes sense to continuously observe the clusters state and defrag frequently along the cadence of 
compaction resulting in shorter intervals of disruption by keeping the number of pages limited and the size of the 
database smaller.

The criteria for defragmentation is defined as:

- Cluster must be observed as `HighlyAvailableTopologyMode`.
- The etcd cluster must report all members as healthy.
- `minDefragBytes` this is a minimum database size in bytes before defragmentation should be considered.
- `maxFragmentedPercentage` this is a percentage of the store that is fragmented and would be reclaimed after
  defragmentation.
  
TODO show performance graphs.

### Risks and Mitigations

- SNO: As defragmentation is I/O blocking it is unsafe for non HA clusters. Mitigation to this risk includes a check in 
  the controller for control-plane topology and will exit 0 if non HA.
  
- datafile corruption: Recently a bug was fixed in upstream etcd[1] which could lead to corruption of the key space 
  as a result of defragmentation if it happened while etcd was being terminated. Mitigation to this risk includes 
  heavy exercise in CI and telemetry will alert `etcdMemberDown` if one of the etcd instances goes down as the 
  result of defragmentation. This will allow clear signal of an issue allowing time to patch the bug.
  
- etcd disruption as a result of defragmentation: It is possible that some clusters could see leader elections as a 
  result of defragmentation. While we can not mitigate all risk here, by setting a high % fragment threshold the 
  gain from a 50% reduction in state should outweigh a possible leader election. Additional soak time in CI should 
  allow for a very clear picture of this risk. 
  
[1] https://github.com/etcd-io/etcd/pull/11613

## Design Details

### Open Questions

- What would be a reasonable way to allow disabling the controller without introducing a new API?

- Can a single defrag configuration handle all clusters?

### Test Plan

- Comprehensive unit testing of
  - defragmention controller logic
  
- Performance testing various providers and cluster sizes.

- Exercise defragmentation during each e2e run.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

New feature

### Version Skew Strategy

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Q: Alert based on fragmentation threshold, while this would be a CronJob.

A: While we could take the approach of scheduling and managing a time based workload that also incorporates the same 
logic into a subcommand of the operator. The controller model only was chosen for simplicity.

Q: Customer managed CronJob

A: Not every customer understands the importance of defragmentation or the corner cases. The general goal of 
OpenShift is to effectivly manage the cluster with operators.

Q: oc adm etcd defrag with added protections to mitgate corner cases.

A: While this could be effective it would still require scheduling that might not happen.
