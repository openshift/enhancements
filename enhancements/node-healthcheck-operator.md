Node Health Check Enhancement 

---
title: node-healthcheck-controller
authors:
  - "@rgolangh"
reviewers:
  - "@abeekof"
  - "@n1r1"
  - "@mshitrit"
  - "@slintes"
  - "@dhellmann"
approvers:
  - "@abeekof"
  - "@n1r1"
---

# Node health checking

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs


## Summary


Enable opt in for automated node health checking and create objects
for unhealthy nodes, to be handled later by external controllers
that comply with the existing  [api].

## Motivation

Enable clusters that do not have a functioning Machine API to detect and respond to node-level failures in order to recover workloads , and/or restore cluster capacity.

The majority of OpenShift clusters are UPI based and do not have Machine objects
and/or a functioning Machine API - which is a prerequisite for using the Machine 
HealthCheck controller.

There are a variety of reasons, even on cloud, that customers might want or need
to pursue a UPI strategy *for deployment*, but that doesn't need to prevent them
from being able to use a standard solution for protecting their workloads, or imply 
that they would prefer to create a bespoke solution.

There are a number of barriers to retrofitting a Machine API to an existing UPI 
cluster, but the most common raised are: complexity, technical limitations, and 
customer policies which prevent BMCs or platform credentials from being stored 
within the cluster).

While there remains a need for UPI, there is also a need to allow UPI workloads to 
be made highly available, and customers look to Red Hat to provide a solution.
The proposed Node Healthcheck Controller (NHC) provides a generic solution that fits all installation types, together with an external remediation API that allows a pluggable remediation system to be installed and remediate unhealthy nodes.

### Goals

- Create an installation-type agnostic node health check solution

- Feature parity with Machine Health Check controller

- Providing a default strategy combined with [poison-pill].

- Endorse usage of external [remediation api]

- Allow defining different unhealthy criteria by node selectors, and have a sane strategy for handling nodes that match multiple NHC CRs.

- Provide defaults health checks and default remedy custom resource creation.

- Long term - Make Machine Health Check controller only implement the external remediation api and drop the default Machine API based remediation mechanism


### Non-Goals

- Coordination of remedy operation - operators/controllers implementers
expected to track the [node maintenance lease proposal]

- Recording long-term stable history of all health-check failures or remediation
  actions.

- Detection of Machines that never become healthy Nodes is not part of the initial implementation.  Machine API based cluster can/should continue to use MHC at this time.

## Proposal
The main use-case we target is UPI, user provisioned infrastructure, which do not have
the Machine-API controlling them. This enables any kind of recovery mechanism to be built and handle remediation.

#### Stories
- As an admin of a cluster I want the cluster to detect Node failures and
 initiate recovery, so that workloads and volumes with at-most-one semantics
  can be run/mounted elsewhere.

- As an admin of a cluster I want the cluster to detect Node failures and
 initiate recovery, so that we can attempt to restore cluster capacity.

- As an OCP engineer, I want the NHC to consume the same logic for detecting
 failures, to avoid code duplication and provide a consistent experience
  compared to MHC.

- As an OCP engineer, I want the NHC to use the same interface for initiating
 recovery, so that we can reuse existing mechanisms (like [poison pill] and metal3) and
  avoid code duplication.

To fulfil the above we create a [Node Health Check custom resource (NCR)](anchor todo) that specifies:
1. health criteria: list of criterias and selector
2. template: a template of an [external custom resource (remediation CR)](link todo)
 to create in response to a match.

### Unhealthy criteria
A node target is unhealthy when the node meets the unhealthy node conditions criteria defined.
If any of those criterias are met for longer than given timeouts, the controller
creates a remediation CR according to the template in the NHC CRD.
Timeouts will be defined as part of the NHC CRD .

### Implementation Details

#### NodeHealthCheck CRD
- Enable watching a node based on a label selector. 
- Enable defining an unhealthy node criteria (based on a list of node conditions).
- Enable setting a threshold of unhealthy nodes. If the current number is at or above this threshold no further action will take place. This can be expressed as an int or as a percentage of the total targets in the pool.

E.g:
- Create a remedy CR when node is `ready=false` or `ready=Unknown` condition for more than 5m.
- I want to avoid triggering remediation if 40% or more of the targets of this pool are unhealthy at the same time.


```yaml
apiVersion: remediation.medik8s.io/v1alpha1
kind: NodeHealthCheck
metadata:
  name: example
  # this CRD is cluster-scope, namespace is not needed  
spec:
  maxUnhealthy: "49%"
  selector:
    matchExpressions:
      key: node-role.kubernetes.io/worker
      operator: exists
  unhealthyConditions:
    - type: "Ready"
      status:  "Unknown"
      duration: "300s"
   - type: "Ready"
     status:  "False"
     duration: "300s"
  remediationTemplate:
    kind: PoisonPillRemediationTemplate
    apiVersion: poison-pill.medik8s.io/v1alphaX
    name: M3_REMEDIATION_GROUP
status:
  healthyNodes: 5
  obeservedNodes: 5
```

#### NodeHealthCheck controller
Watch:
- Watch NodeHealthCheck resources
- Watch nodes with an event handler e.g controller runtime `EnqueueRequestsFromMapFunc` which returns NodeHealthCheck resources.

Reconcile:
- Periodically enqueue a reconcile request
- Fetch all nodes targets. E.g:


- Calculate the number of unhealthy targets.
- Compare current number against `maxUnhealthy` threshold and temporary short
 circuits if the threshold is met.
- Create remedy objects for unhealthy targets according to the template in NHC object

### Notes/Constraints

The functionality of this controller have a lot in common with the Machine Health
Check controller except that its creating objects and not
performing any actions per se. When deploying this controller make
sure 1. you have a controller implementing the API, e.g [poison pill]
2. the machine health check controller is not running, or in case of a mixed
 the node health checks selector avoid targeting nodes with machines.


### Risks and Mitigations

#### Remediation Interaction with ClusterAutoScalar

Misconfiguration of nodes provisioning can cause nodes to fail to get operational.
Remediation is blind to such problems and may interfere with diagnosing the problem.
The backoff mechanism mentioned in MHC may be suited for Machines, because they
have the MachineSet name that can be recorded and tracked. Without the machine API
our knowledge of the nodes could not be handled in such a generic way. There is
simply no generic way to record a node configuration origin in reliable way without
putting provider specific annotations.
Potentially the NHC resource could contain label selectors to track
repeating remediation and mark them as [incrementally back-off](https://issues.redhat.com/browse/OCPCLOUD-800). e.g

Suggested backoff specification under the NHC resource may look like this:
```yaml
kind: NodeHealthCheck
...
spec:
  selector:
     ...
  # optional
  backoff:
    type: exponential
    limit: 10m
```

Caveat of this approach is that we need to take care of conflicting or overlapping backoff definitions.
Setting global catchall backoff strategy is impossible or just coarse without specifying a selector (how do we know what to backoff from?)

Currently backoff is not implemented.

#### Multiple remediation provider deployed

Multiple providers could act on remediation objects. On non heterogeneous cluster
this may even make sense. It is not the goal of this controller to coordinate
those, but providers will need disparate set of nodes to handle based on labels,
or operator on a node if they can hold a [maintenance lease][node maintenance lease proposal].


## Design Details

### Logical Conditions Matching

The resource will be given a list of conditions to track, per a selection of
nodes, and will evaluate each condition in the list. It's a logical evaluation i.e
it is enough to evaluate a single condition to true per node to mark as unhealthy.

```
unhealthyConditions:
   - type:    "Ready"
     status:  "Unknown"
     duration: "300s"
   - type:    "Ready"
     status:  "False"
     duration: "300s"
```
In the above example only 1 condition needs to match to mark a node non-healthy.

### Max Unhealthy count

To create remediation object (i.e act for remediation) the overall nodes in the
selection groups must not exceed `maxUnhealthy` threshold. The value can be
an int or a percentage, similar to v1/apps `maxUnavailable`:
```
spec:
 maxUnhealthy: 30%
```

### Default spec values:
selector: {matchingExpression: { Key: node-role.kubernetes.io/worker, Operator: Exists } }
maxUnhealthy: 49%
unhealthyConditions: [ {type: NotReady, status: Unknown, duration: 300s},{ type: Ready, status: False, 300s} ]
remediationTemplate: n/a

### Mandatory spec fields:
remediationTemplate

### Optional spec fields:
  - selector
  - maxUnhealthy
  - unhealthyConditions


### Status sub-resource fields
observedNodes - specifies how much nodes are observed by this object selector
heathyNodes     - specify the number of healthy nodes
inFlightRemediations - a mapping of a remediaiton object name (== the node name) and the time it was created. Removed
when the remediation object deleted.


### Remediation Object creation
Add the target Node to ownerReferences. We want that object to be garbage
collected with the referenced Node


### Controller details
leader election: true
Sync period: 60s - determines how frequent reconciles happen regardless of resource changes.
watches NHC objects
watches Node objects - this will make the reconcile more efficient. It will allow a sync period with longer intervals

# [rgolan] the rest is WIP #

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests? - yes, e2e alrady exists for the
  upstream medik8s/node-healthcheck-operator and are running with openshift-ci.
  The same will apply for the openshift fork.
- How will it be tested in isolation vs with other components?
  TBD 
- What additional testing is necessary to support managed OpenShift service-based offerings?
  TBD

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

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

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to this should be
  identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.


[poison pill]: https://github.com/poison-pill/poison-pill
[node maintenance lease proposal]: https://github.com/kubernetes/enhancements/pull/1411/
[remediation API]:


