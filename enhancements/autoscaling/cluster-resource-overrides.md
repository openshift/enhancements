---
title: Cluster-Resource-Overrides-Enablement
authors:
  - "@deads2k"
reviewers:
  - "@sttts"
  - "@derekwaynecarr"
approvers:
  - "@derekwaynecarr"
creation-date: 2019-09-11
last-updated: 2019-09-11
status: provisional
see-also:
replaces:
superseded-by:
---

# ClusterResourceOverrides Enablement

The `autoscaling.openshift.io/ClusterResourceOverride` cannot be enabled in 4.x.  The plugin already exists, this design 
is about how we make it possible for a customer to enable the feature.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

The `autoscaling.openshift.io/ClusterResourceOverride` admission plugin is an uncommonly used admission plugin with configuration
values.  Because it is uncommonly used, it doesn't fit well with our targeted configuration which aims to avoid adding
lots of intricately documented knobs.  Instead of wiring the admission plugin via a kube-apiserver operator, we can create
a mutating admission webhook based on the [generic-admission-server](https://github.com/openshift/generic-admission-server)
and install it via OLM.

## Motivation

The `autoscaling.openshift.io/ClusterResourceOverride` admission plugin is used for over-commit, let's stipulate that it is
important enough to enable.  The kube-apiserver is designed to be extended using mutating admission webhooks, we have the
technology to easily build one, and we have the ability to create a simple operator to manage it.  We want to enable the 
feature using a pattern that we can extend to other admission plugins that can scale beyond the small team that maintains
the kube-apiserver.

### Goals

1. Enable the `autoscaling.openshift.io/ClusterResourceOverride` admission plugin that is used for overcommit.
2. Use existing extension points, libraries, and installation mechanisms in the manner we would recommend to 
 external teams.
3. Have a fairly straightforward way to install and enable this admission plugin.
4. Rebootstrapping must be possible.

### Non-Goals

1. Revisit how `autoscaling.openshift.io/ClusterResourceOverride` works.  We're lifting it as-is.
2. Couple a slow moving admission plugin to a fast moving kube-apiserver.

### Open Questions

1. Do we need to protect openshift resources from being overcommitted?  Perhaps the cluster-admin's intent is exactly that.
2. We cannot uniformly apply protection just to our payload resources, how do we position this?
 External teams may be surprised that their resource requirements are not respect, but ultimately the cluster-admin is in
 control of his cluster.  This is what running self-hosted means.
3. How are OLM operators tested against OpenShift levels?
4. How do we build and distribute this OLM operator using OpenShift CI?
5. How do we describe version skew limitations to OLM so our operator gets uninstalled *before* an illegal downgrade or upgrade?
 This is a concrete case of the API we want to use isn't available before 1.16 and after 1.18, the previous API could be gone.

## Proposal

1. Create a mutating admission webhook server that provides `autoscaling.openshift.io/ClusterResourceOverride`.
2. Create an operator that can install, maintain, and configure this mutating admission webhook.
3. Ensure that we consistently label all prereq namespaces (we attempted runlevel before so this may work), to be sure
 that we re-bootstrap.
4. Expose the new operator via OLM and integrate our docs that way.

### User Stories

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints

We *must* be able to re-bootstrap the cluster.  This means that a cluster with this admission plugin created must be able
to be completely shut down and subsequently restarted.

### Risks and Mitigations

External teams may be surprised that their resource requirements are not respect, but ultimately the cluster-admin is in
control of his cluster.  This is what running self-hosted means.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

TBD, see open questions.

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

See open questions.  

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

## Infrastructure Needed

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
