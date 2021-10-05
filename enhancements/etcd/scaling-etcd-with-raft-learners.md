---
title: Scaling etcd with Raft Learners
authors:
- "@hexfusion"
  reviewers:
- "@deads2k"
- "@lilic"
- "@hasbro17"
- "@joelspeed"
- "@jeremyeder"
  approvers:
- "@mfojtik"
  creation-date: 2021-10-04
  last-updated: 2021-10-04
  status: implementable
  see-also: []
  replaces: []
  superseded-by: []
---

# Scaling etcd with Raft Learners

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Over time as clusters live longer and workloads grow the ability to scale the control-plane and replace failed nodes
becomes a critical part of the admins maintenance overhead. Today the `cluster-etcd-operator` manages scaling up of
the etcd cluster. To provide the foundation for initiatives such as scale down and vertical control plane scaling[1].
The `cluster-etcd-operator` must ensure proper safety mechanisms exist to adjust membership of the etcd cluster.

Introduced in etcd v3.4 the raft learner[2] provides mitigations which reduces quorum and
stability issues during scaling. A learner is essentially an etcd member which is non voting thus can not impact 
quorum but like other members will receive log replications from the leader.

This enhancement proposes:
- Replacing the default scale up performed by the cluster-etcd-operator to use raft learners.
- Deprecation and removal of the current `discover-etcd-initial-cluster`[3] command and replacing its
functionality with the existing `etcdenvvar` controller and subcommands of the `cluster-etcd-operator`. By
eliminating this code it also removes the teams largest carry against upstream etcd.
- Ensuring that scale up only happens on control-plane nodes which have a quorum-guard pod monitoring health.
- Adding a flag to etcd which allows configuration of maximum learners in cluster (--max-learners). Today the max is 1.

POC: Functional proof of concept: https://github.com/openshift/cluster-etcd-operator/pull/682

[1] https://issues.redhat.com/browse/OCPPLAN-5712

[2] https://etcd.io/docs/v3.4/learning/design-learner/

[3] https://github.com/openshift/etcd/blob/openshift-4.10/openshift-tools/pkg/discover-etcd-initial-cluster/initial-cluster.go

## Motivation

Great demand exists to provide flexible automated scaling of the control-plane in the same way the worker
nodes do today.

### Goals

- provide safe scale up of etcd cluster using raft learners.
- add additional membership level details such as member ID to the init container validation process for etcd.
- add observability to allow admin to clearly diagnose scaling failure.
- reduce divergence from upstream etcd by removing `discover-etcd-initial-cluster`.

### Non-Goals

- implement scale down logic for the `cluster-etcd-operator`

## Proposal

### User Stories

1. As a cluster-admin I want to scale up etcd without fear of quorum loss.

2. As a cluster-admin I want to be able to replace failed control-place nodes without a deep understanding of etcd.

### Critical Alerts

We will add the following critical alerts:

- alert about learner member which has not been promoted or started > 30s
- alert if the number of etcd members is not equal to the number of quorum-guard pods > 30s

### Monitoring Dashboard

We add new metric figures to the etcd dashboard in the console:

1. include membership status over time `is_leader`, `is_learner` and `has_leader`.

### Target Config Controller

The `Target Config Controller` ensures that the appropriate resources are populated when an observed change takes
place in the cluster resulting in a new static pod revision. The `etcdenvvar controller` provides the etcd runtime
environment variables to the `Target Config Controller` which are then is used to populate the etcd static pod manifest.

`ETCD_INITIAL_CLUSTER` ENV variable is a critical part of etcd scaling process. Before a member has joined the cluster
and received a snapshot from the leader including the cluster membership details from the member bucket the new etcd
member must be able to communicate with its peers. This proposal includes moving population of
`ETCD_INITIAL_CLUSTER` from etcd itself via `discover-etcd-initial-cluster`[1] to the `etcdenvvar controller` in the
`cluster-etcd-operator`.

[1] https://github.com/openshift/etcd/tree/openshift-4.10/openshift-tools/pkg/discover-etcd-initial-cluster

### cluster-etcd-operator verify membership command

In 4.9 the `verify` command was added to `cluster-etcd-operator` as way to ensure that before a backup is taken
there is appropriate disk space available (verify storage). In order for cluster membership to be adjusted safely extra
precautions and logical gates needs to be added. The logic for these extra safety steps will be added as a sub
command `verify membership` and will be added as init container in the etcd static pod.

The `verify membership` command will consume a new ENV `ETCD_MEMBER_IDS` populated by `etcdenvvar controller`. This
ENV will take a similar map format as `ETCD_INITIAL_CLUSTER` <member_id>=<peerurls>. The verify command will ask the
cluster for the current etcd members `MemberListRequest` and compare the member IDs expected in the ENV vs actual. The
expectation is that any change in cluster membership is matched with a static pod revision containing the
appropriate pairing.

#### Reads

1. `MemberListResponse`: The member list provides membership details on the etcd cluster.

- Consumers: `etcdenvvar controller`, `verify membership`

- ref: https://pkg.go.dev/github.com/coreos/etcd/etcdserver/etcdserverpb#MemberListResponse

#### Writes

1. `ETCD_INITIAL_CLUSTER`: This variable will be added to the etcd static pod ENV, allowing new etcd members to join
the cluster.

- Writer: new function added to `envVarFns`[1].

- Consumer: etcd binary during first join of cluster, afterwards it is ignored.

2. `ETCD_MEMBER_IDS`: This variable will be added to the etcd static pod EMV, the mapping of this data enables extra
precautions during scaling.

- Writer: new function added to `envVarFns`[1].

- Consumer: cluster-etcd-operator verify membership command.

New etcd members on an existing node will need to archive the data-dir of the previous member to allow scale up to
occur. `verify membership` can observe this by asking the cluster for membership details and using the conditions of
`Name=""` and the method of `Member` `IsLearner` to conclude this.

[1] https://github.com/openshift/cluster-etcd-operator/blob/release-4.10/pkg/etcdenvvar/etcd_env.go#L43

### Cluster Member Controller

The cluster member controller manages scaling up etcd during member replacement and bootstrap. This proposal makes
changes to the way this controller manages scale up.

`ensureEtcdLearnerPromotion`: This new method will provide the logic necessary to ask the cluster for the list of
members and attempt to promote any learner members which have a log in sync with the leader.

#### Reads

1. openshift-etcd pods/quorum-guard: Today the controller loops not `Ready` etcd pods and will attempt to scale
up if the etcd is not already part of the cluster membership. While this is effective during bootstrap it is not
efficient as scaling no longer needs to be serial. Also by ensuring the quorum guard pod exists on the node before
scale up we can give more power to the PDB in scaling.

2. `MemberListResponse`: The Cluster API `MemberListRequest` RPC provides `[]Member` for the etcd cluster including the
method `IsLearner`.

- Consumers: scale up and promotion of learner members

- ref: https://pkg.go.dev/github.com/coreos/etcd/etcdserver/etcdserverpb#MemberListResponse

3. `MemberPromoteResponse`: The Cluster API `MemberPromoteRequest` asks the leader of the term if the learner's log
is in sync with its own. If no return error.

- Consumers `ensureEtcdLearnerPromotion`

- ref: https://pkg.go.dev/go.etcd.io/etcd/api/v3/etcdserverpb#MemberPromoteResponse

### Risks and Mitigations

The largest risks to scaling is quorum loss, data loss, and split brain scenario.

- **Quorum Loss**: Because new members are added as non voting members and can not be promoted to voting members unless
the etcd process starts and the log has been completely replicated and in sync with leader.


- **Split brain**: The cluster gates starting of the etcd process on a verification process based on the cluster
member id. This ensures that each revision has an explicit expected membership. Because quorum guard replicas are
managed by the operator the cluster topology will remain in a safe configuration (odd number of members).


- **Data loss**: A concern with rolling scaling of etcd with large data files is possible data loss. The MTTR of log
replication is directly tied to the size of the state. If members of the cluster were replaced too quickly in
conjunction of it could be possible although very unlikely that no member has a complete log. Raft learners ensures
this is not possible by waiting for the log to be replicated from the leader before promotion to voting member can
take place.

## Design Details

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding to implement the
design.  For instance, > 1. This requires exposing previously private resources which contain
sensitive information.  Can we do this?

### Test Plan

- e2e with

    1. add dangling etcd learner member which has not been started and not promoted.
        - verify alert fires
        - ensure degraded cluster
    2. scale up and scale down etcd cluster
          - ensure stability of etcd cluster during bootstrap.
          - verify `ETCD_INTIAL_CLUSTER` remains valid through scaling and no split brain occurs (leader x2).
          - inject invalid member into `ETCD_MEMBER_IDS` to ensure `verify membership` blocks rollout.
          - replace member on node and ensure `verify membership` properly archives previous data-dir.
    3. verify DR workflow

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal should keep
this high-level with a focus on what signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc
definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning), or by
redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is
accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**Examples**: These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

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

**For non-optional features moving to GA, the graduation criteria must include end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this is in the test
plan.

Consider the following in developing an upgrade/downgrade strategy for this enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to
  make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to
  make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and workloads during upgrades. Ensure the
  components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a minor release
  stream without being required to pass through intermediate versions - i.e. `x.y.N->x.y.N+2` should
  work without requiring `x.y.N->x.y.N+1` as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade steps. So, for example, it
  is acceptable to require a user running 4.3 to upgrade to 4.5 with a `4.3->4.4` step followed by a
  `4.4->4.5` step.
- While an upgrade is in progress, new component versions should continue to operate correctly in
  concert with older component versions (aka "version skew"). For example, if a node is down, and an
  operator is rolling out a daemonset, the old and new daemonset pods must continue to work
  correctly even while the cluster remains in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is misbehaving, it should be
  possible for the user to rollback to `N`. It is acceptable to require some documented manual steps
  in order to fully restore the downgraded cluster to its previous state. Examples of acceptable
  steps include:
    - Deleting any CVO-managed resources added by the new version. The CVO does not currently delete
      resources that no longer exist in the target version.

### Version Skew Strategy

How will the component handle version skew with other components?  What are the guarantees? Make
sure this is in the test plan.

Consider the following in developing a version skew strategy for this enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and in the kubelet? How
  does an n-2 kubelet without this feature available behave when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI or CNI may require
  updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to highlight and record other
possible approaches to delivering the value proposed by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new subproject, repos
requested, GitHub details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources started right away.
