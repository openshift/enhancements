---
title: pao-render
authors:
  - "@marsik"
  - "@titzhak"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-06-04
last-updated: yyyy-mm-dd
status: implementable
see-also:
  - "/enhancements/workload-partitioning/management-workload-partitioning.md"
---

Start by filling out this header template with metadata for this enhancement.

* `reviewers`: This can be anyone that has an interest in this work.

* `approvers`: All enhancements must be approved, but the appropriate people to
  approve a given enhancement depends on its scope.  If an enhancement is
  limited in scope to a given team or component, then a peer or lead on that
  team or pillar is an appropriate approver.  If an enhancement captures
  something more broad in scope, then a member of the OpenShift architects team
  or someone they delegate would be appropriate.  Examples would be something
  that changes the definition of OpenShift in some way, adds a new required
  dependency, or changes the way customers are supported.  Use your best
  judgement to determine the level of approval needed.  If you’re not sure,
  just leave it blank and ask for input during review.

# Performance Addon Operator render mode

This is the title of the enhancement. Keep it simple and descriptive. A good
title can help communicate what the enhancement is and should be considered as
part of any review.

The YAML `title` should be lowercased and spaces/punctuation should be
replaced with `-`.

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

The Performance addon operator is a day 2 optional operator installed via OLM. However the [Workload partitioning](workload-partitioning/management-workload-partitioning.md) feature needs to have some configuration ready at the installation time when PAO is not yet ready.
This feature is about adding a `render` mode to PAO that would allow an admin to pre-compute all the Openshift manifests that are needed in a way that prevents any typing mistakes from happening.
These generated manifests can then be passed to the installer and later taken over by the PAO reconcile loop. 

## Motivation

There are many pieces that must be configured just right for the latency and resource management to work properly. PAO addresses that for day 2 latency tuning, but there was no way to use PAO's knowledge at the install time.

### Goals

1) PAO can execute outside of a cluster via podman/docker, consume user manifests from a directory and generate additional manifests to another (or the same) directory
1) PAO takes all PerformanceProfile yamls from the input directory and generates all the usual manifests as yaml files to the output directory
1) When a command line option `--enable-workload-partitioning` is passed to PAO in render mode, it also generates the necessary configuration for the `management` partition to work properly
1) PAO at day 2 must not make any changes to the existing generated objects it owns
1) PAO sets the ownership references on the generated yamls so the generated files are properly linked to the PerformanceProfiles

### Non-Goals

TBD

## Proposal

### User Stories

- 1) [optional] The cluster administrator will collect PAO must-gather output from a test cluster with no partitioning enabled
  1) [optional] The cluster administrator will execute the Performance profile creator to get well formed PerformanceProfiles
  1) The PerformanceProfiles are placed into a directory
  1) PAO is executed using podman run -v /input/dir:/input -v /output/dir:/output registry/performance-addon-operator render [optional --enable-workload-partitioning]
  1) The files from both input and output directories are passed over to the installer

### Implementation Details/Notes/Constraints [optional]

TBD

### Risks and Mitigations

- PAO needs to be written in such way that the render mode and the live reconcile loop always generate identical content. This should be handled by tests and/or CI.

## Design Details

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

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

This feature is supposed to be GA and supported right from release. It requires an explicit user action to be activated.

The plan is to provide tests, CI and documentation in the release where this will be introduced.

End-to-end tests of the Workload partitioning are covered in the high level feature description.

### Upgrade / Downgrade Strategy

This feature is activated locally by user and follows the lifecycle of PAO itself. There is no upgrade procedure to perform.

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
