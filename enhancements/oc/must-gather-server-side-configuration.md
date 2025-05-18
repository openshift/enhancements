---
title: must-gather-server-side-configuration
authors:
  - MarSik
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@cli and support gurus - this will affect data collection for debuggability purposes"
  - "@insights - this might influence how insights data gathering can be done"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2025-02-17
last-updated: 2025-02-17
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/RFE-6505 # A generic RFE for now, this enhancement is a small step in bigger scheme
see-also:
  - "/enhancements/oc/must-gather.md"
---

To get started with this template:
1. ~~**Pick a domain.** Find the appropriate domain to discuss your enhancement.~~
1. ~~**Make a copy of this template.** Copy this template into the directory for
   the domain.~~
1. ~~**Fill out the metadata at the top.** The embedded YAML document is
   checked by the linter.~~
1. ~~**Fill out the "overview" sections.** This includes the Summary and
   Motivation sections. These should be easy and explain why the community
   should desire this enhancement.~~
1. **Create a PR.** Assign it to folks with expertise in that domain to help
   sponsor the process.
1. **Merge after reaching consensus.** Merge when there is consensus
   that the design is complete and all reviewer questions have been
   answered so that work can begin.  Come back and update the document
   if important details (API field names, workflow, etc.) change
   during code review.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

See ../README.md for background behind these instructions.

Start by filling out the header with the metadata for this enhancement.

# Cluster side configuration for must-gather

## Summary

The idea is to allow configuration of must gather via an object deployed to
a cluster. This allows better match to specific data collection needs based
on the purpose of the cluster and avoids human mistakes when running must-gather
manually.

## Motivation

Many customer cases and bug reports I have been exposed to lack the proper
data needed for debugging the cluster at hand.

1. Sometimes it is caused by humans not following the right procedure
   (such as adding an extra must-gather image to the command line),
1. in other cases it was caused by special needs that must gather by default
   cannot satisfy for scale reasons (eg. low level kernel details needed to
   debug Telco RAN latency on single node clusters).

### User Stories

* As a cluster administrator (owner) I want to be able to force all must-gather
  executions to collect the necessary data for debugging of this specific
  cluster.
* As a cluster operator I want to use simple `oc adm must-gather` and collect
  all data necessary without having to re-read multiple documentation pages.
  In other words, I want the command to be as simple as possible with no
  complicated command line arguments needed.
* As an application and cluster owner I want a place to declare application
  specific data collection needs so the probability of mistake when collecting
  the debug snapshot is minimized.
* As a cluster operator I want to collect only what is necessary to
  minimize the collection time and the cluster load caused by the
  collection tooling.
* As a system add-on developer I want to be able to add extra configuration
  for must-gather related to my add-on to collect the proper data. And I want
  this even for add-ons that are NOT operators and have no active controllers
  running.
* As a system add-on developer I want to be able to tweak the behavior
  of my must-gather image per-cluster.
* As a restricted network cluster operator I want all images provided by
  this extra configuration to be automatically mirrored by the tooling
  provided by the platform.

And extra story that may be omitted and is an open question:

* As a support engineer I want to know what Insights rules should be enabled
  and/or disabled by default when analysing the data dump from this cluster.

### Goals

* A mechanism exists that exposes cluster-side configuration to the
  `oc adm must-gather` tool on execution
* This mechanism must allow composability of configuration snippets
  coming from multiple sources (both manual and operator provided).
* The configuration should allow passing arguments to the scripts
  executed by the must gather collection process.
* All extra images needed to satisfy the configuration should be
  available in "offline" registries created for restricted networking
  clusters.

### Non-Goals

* Operator and OLM API updates - the solution should be doable using
  the existing mechanisms for injecting objects into the cluster

## Proposal

The `oc` tool should query a specific namespace `openshift-debug (?)`
and collect all ConfigMap objects with a specific must-gather label. This will be enabled by default, because otherwise admins will forget. However there must be a way to disable this feature.

Each ConfigMap would contain specific well named pieces to configure
"command line arguments" to be added to the currently ongoing `oc adm must-gather`
run.

The ConfigMap will also contain command line arguments or environment
variables per image that will be used when executing the image for
data collection.

The ConfigMaps would also be collected by default, to allow the
`insights` tool to act upon the extra configuration and tweak
the analysis to the "purpose of the cluster".

### Workflow Description

**owner** is a human user or a team responsible for giving the
order for deploying the cluster.

**cluster creator** is a human user responsible for deploying a
cluster.

**cluster administrator** is a human user responsible for
maintaining the cluster and interacting with the control plane.

**support engineer** is a human user responsible for analysing
a misbehaving cluster. He/she can be a customer or a Red Hat
employee depending on the nature of the emergency.

1. The owner decides he needs a new cluster for a purpose P and gives
   the cluster creator an order to deploy and configure it.
1. The cluster creator prepares the install manifests and includes
   the extra configuration mandated by a blueprint for purpose P as 
   well as blueprints for the expected applications.
1. The cluster creator then deploys the cluster and transfers the
   responsibility to the cluster administrator.
1. The cluster administrator deploys all the applications and watches
   after the cluster for the months to come.
1. But behold, a monitoring system reports a failure of some kind.
   A degraded component, lower performance or a misbehaving application.
1. The cluster administrator opens his laptop and runs `oc adm must-gather`
1. Once the command finishes the cluster administrator consults a
   support engineer and provides the collected data to prove his case.
1. The support engineer executes an analysis tool (`insights <must-gather-directory>`)
   and gets a list of potential issues and their explanations.
   The list already contains extra results based on the purpose and
   deployment blueprints the cluster was installed with.

A failure of the must gather story is as follows:

6. The cluster administrator opens his laptop and runs `oc adm must-gather`
1. The command fails due to a broken image or a missing command line option
1. The cluster administrator runs `oc adm must-gather` again with an argument that disables the default configuration and inspects the must gather configuration namespace to find or fix the configuration issue

### API Extensions

- adds a special namespace within the protected `openshift-` space
- adds a ConfigMap definition that `oc` and `insights` can parse and use to tweak their behavior

Fill in the operational impact of these API Extensions in the "Operational Aspects
of API Extensions" section.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The ConfigMaps should be hosted cluster specific and the `oc adm must-gather`
must be able to retrieve the proper configuration and images.

Must-gather should be able to collect data both from the management and hosted
clusters as the configured data collection images might want to collect both cluster
configuration data and the user application data. However this is out of scope
for the proposal, the must gather tool should handle this anyway.

#### Standalone Clusters

> Is the change relevant for standalone clusters?

This allows to control extra collection as needed. Typical IT grade clusters
with many nodes can only collect the basic data.

Specific data collection images can collect more data in targeted fashion - just on specific nodes or MCPs where necessary.

#### Single-node Deployments or MicroShift

> How does this proposal affect the resource consumption of a
> single-node OpenShift deployment (SNO), CPU and memory?

> How does this proposal affect MicroShift? For example, if the proposal
> adds configuration options through API resources, should any of those
> behaviors also be exposed to MicroShift admins through the
> configuration file for MicroShift?

This can make data collection fit exactly the specific deployment type.
Collecting exactly the data necessary, not more and not less by configuring
must gather.

### Implementation Details/Notes/Constraints

> What are some important details that didn't come across above in the
> **Proposal**? Go in to as much detail as necessary here. This might be
> a good place to talk about core concepts and how they relate. While it is useful
> to go into the details of the code changes required, it is not necessary to show
> how the code will be rewritten in the enhancement.

### Risks and Mitigations

> What are the risks of this proposal and how do we mitigate. Think broadly. For
> example, consider both security and how this will impact the larger OKD
> ecosystem.

A bad actor with access to the proper namespace could inject a malicious must-gather image.
The same is true for a bad system extension developer that the cluster administrator installs.

However, with such access, the actor can already do all of what he wants directly.

> How will security be reviewed and by whom?

TBD

> How will UX be reviewed and by whom?

Support engineers?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

> The idea is to find the best form of an argument why this enhancement should
> _not_ be implemented.

People might not pay enough attention to notice extra arguments are being used without them entering them.

> What trade-offs (technical/efficiency cost, user experience, flexibility,
> supportability, etc) must be made in order to implement this? What are the reasons
> we might not want to undertake this proposal, and how do we overcome them?

The proposal defines a ConfigMap, not a CRD. This might prevent efficient validation.

> Does this proposal implement a behavior that's new/unique/novel? Is it poorly
> aligned with existing user expectations?  Will it be a significant maintenance
> burden?  Is it likely to be superceded by something else in the near future?

## Open Questions [optional]

* How will extra arguments to the oc tool interact with the default arguments coming from the cluster?

## Test Plan

**Note:** *Section not required until targeted at a release.*

> Consider the following in developing a test plan for this enhancement:
> - Will there be e2e and integration tests, in addition to unit tests?
> - How will it be tested in isolation vs with other components?
> - What additional testing is necessary to support managed OpenShift service-based offerings?

This can probably only be e2e tested as a whole. It is possible the `oc` cli unit tests can mock the cluster resources, but I haven't checked that yet.

> No need to outline all of the test cases, just the general strategy. Anything
> that would count as tricky in the implementation and anything particularly
> challenging to test should be called out.

> All code is expected to have adequate tests (eventually with coverage
> expectations).

## Graduation Criteria

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

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

> If applicable, how will the component be upgraded and downgraded? Make sure this
> is in the test plan.

This functionality is going to be provided by the `oc` tool. Which means it will support any cluster version as long as the cli tool itself contains the feature.

> Consider the following in developing an upgrade/downgrade strategy for this
> enhancement:
> - What changes (in invocations, configurations, API use, etc.) is an existing
>   cluster required to make on upgrade in order to keep previous behavior?
> - What changes (in invocations, configurations, API use, etc.) is an existing
>   cluster required to make on upgrade in order to make use of the enhancement?

> Upgrade expectations:

There is no impact to the cluster components. Just some extra ConfigMaps that do not influence the runtime.

> Downgrade expectations:

There is no impact to the cluster components. Just some extra ConfigMaps that do not influence the runtime.

## Version Skew Strategy

> How will the component handle version skew with other components?
> What are the guarantees? Make sure this is in the test plan.

This functionality is going to be provided by the `oc` tool. Which means it will support any cluster version as long as the cli tool itself contains the feature.

There is no runtime impact except of few (kilo-)bytes of memory consumed by the ConfigMaps.

## Operational Aspects of API Extensions

There are no runtime aspects to the proposal.

## Support Procedures

> Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

A broken image or a wrong command line argument injected by the cluster configuration will cause `must-gather` data collection to fail. **A method to ignore the cluster level arguments is needed.**

## Alternatives

> Similar to the `Drawbacks` section the `Alternatives` section is used
> to highlight and record other possible approaches to delivering the
> value proposed by an enhancement, including especially information
> about why the alternative was not selected.

The only two alternatives are to:

1. keep the current practices where each cluster operator is responsible for the proper `must-gather` arguments
2. distribute the must gather configuration to the cluster operators' machines as files

## Infrastructure Needed [optional]

> Use this section if you need things from the project. Examples include a new
> subproject, repos requested, github details, and/or testing infrastructure.

None
