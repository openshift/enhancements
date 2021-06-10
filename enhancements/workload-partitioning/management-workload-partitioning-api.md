---
title: management-workload-partitioning-api
authors:
  - "@marsik"
  - "@dhellmann"
  - "@mrunalp"
  - "@browsell"
reviewers:
  - "@deads2k"
  - "@staebler"
  - TBD
approvers:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@markmc"
creation-date: 2021-06-04
last-updated: 2021-06-04
status: provisional
see-also:
  - "/enhancements/workload-partitioning/management-workload-partitioning.md"
---

# Management workload partitioning API


To get started with this template:
1. **Pick a domain.** Find the appropriate domain to discuss your enhancement.
1. **Make a copy of this template.** Copy this template into the directory for
   the domain.
1. **Fill out the "overview" sections.** This includes the Summary and
   Motivation sections. These should be easy and explain why the community
   should desire this enhancement.
1. **Create a PR.** Assign it to folks with expertise in that domain to help
   sponsor the process.
1. **Merge at each milestone.** Merge when the design is able to transition to a
   new status (provisional, implementable, implemented, etc.). View anything
   marked as `provisional` as an idea worth exploring in the future, but not
   accepted as ready to execute. Anything marked as `implementable` should
   clearly communicate how an enhancement is coded up and delivered. If an
   enhancement describes a new deployment topology or platform, include a
   logical description for the deployment, and how it handles the unique aspects
   of the platform. Aim for single topic PRs to keep discussions focused. If you
   disagree with what is already in a document, open a new PR with suggested
   changes.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

The `Metadata` section above is intended to support the creation of tooling
around the enhancement process.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The overall feature summary is present in the [Management workload partitioning](workload-partitioning/management-workload-partitioning.md) feature. This document focuses on how the feature enablement API and its interactions will look like.

## Motivation

This feature is about designing a read-only API that will describe the enabled workload partitions (types, classes, etc.). This information is needed for kubelet to start exposing the right resources as well for the admission webhook to know when the pod manipulation is needed.

### Goals

The short term goal is to get enough functionality to create the `management` partition where all OpenShift infrastructure could be placed. This partition is cluster wide and only affects pods with certain pre-agreed-upon labels and placed in specific pre-annotated namespaces.

### Non-Goals

- Long term goal of allowing cluster admins to create their own partitions is not part of this proposal for now
- Day 2 creation of partitions is not part of the proposal
- Partitions with scope less than the entire cluster (such as being limited to Namespaces or MCPs) are not part of the proposal

## Proposal

The proposal is to define a new cluster-wide Custom Resource Definition that would describe the allowed partition names in the status section. That way it hints at being a read only object where no user/admin input or modifications are expected.

```yaml
apiVersion: workload.openshift.io/v1
kind: WorkloadPartitions
metadata:
  # arbitrary name, all objects of this Kind should be processed and merged
  name: management-partition
status:
  # List of strings, defines partition names that will be recognized by the
  # workload partitioning webhook. This list will also inform PAO about partitions
  # that should be configured on the kubelet and CRI-O level.
  clusterPartitionNames:
    - management
```

It is expected this API will be created at the installation process. Either manually or using the Performance Addon Operator render mode.

To allow for future extensibility and possible multiple sources of workload partition names (coming from customers, the installer, or other operators, etc.), we propose that there might be multiple `WorkloadPartitions` objects injected into the cluster. The expected behavior is that all components would just merge all the defined names together.

There is no controller or reconcile loop as part of this proposal. Only the cluster administrator will have the ability to create or manipulate the WorkloadPartitions objects. Anyone will be allowed to read them.

### User Stories

Please check the user stories in the parent [Workload partitioning management](management-workload-partitioning.md) feature.

The gist of this piece is that it is expected to define the workload partitions before the installation of a cluster starts. A cluster with no `WorkloadPartitions` objects present at the installation time will not have workload partitioning enabled.

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

1) An admin creates a WorkloadPartitions object manually after the cluster has been running for some time on a cluster that already has partitioning enabled
1) An admin creates the WorkloadPartitions object manually after the cluster has been running for some time on a cluster with no workload partitioning enabled
1) An admin deletes the WorkloadPartitions object that was created during the install process
1) A random user manages to create a WorkloadPartitions object due to a bug in the defined RBAC rules

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Open Questions [optional]

1) Where should this CRD be defined and who will own the definition?
   - Just as a manifest coming from the installer?
   - Part of some CVO related component?
1) the apiVersion namespace needs to be given some thought

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

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

There is currently no active component managing the WorkloadPartitions custom resource. Since we do not want to allow enabling workload partitioning for existing clusters there does not have to be any upgrade logic. There is an associated risk (discussed in the Risks section) if the admin enables workload partitioning manually on a cluster where it was not enabled before.

In the future, we will have to handle upgrade from older to newer versions of the WorkloadPartitions CRD if we change it in a backwards incompatible way.

### Version Skew Strategy

This CRD is closely tied to features in Kubelet and the new workload partitioning admission webhook. All those pieces are supposed to be shipped and deployed as part of standard OpenShift installation.

If someone creates a WorkloadPartitions CR in a cluster without the admission hook and with an old kubelet, pods would not be mutated because nothing would be looking at the Workloads CR.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

This feature and its implementation might lead cluster administrators to start using it for manual overrides of the core components like cpu and/or topology manager. That would be an anti-pattern and the presentation of this feature should clearly state that.

## Alternatives

There is an alternative style of the CRD that is possible. Instead of a `clusterPartitionNames` we could use just `partitions` and declare the expected scope in `partitions.scope`. See the example below.

```yaml
apiVersion: workload.openshift.io/v1
kind: WorkloadPartitions
metadata:
  name: management-partition
status:
  partitions:
    - name: management
      scope: cluster # optional, cluster scope would be the default
    # No other partition or scope is expected to be supported in this initial version
```

## Infrastructure Needed [optional]

Nothing comes to my mind.
