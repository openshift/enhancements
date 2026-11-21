---
title: managing-upstream-feature-gates
authors:
  - @everettraven
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @JoelSpeed # API review background + architect
  - @ScottDodson # architect
  - @DevanGoodwin # architect
  - @benluddy # control-plane group staff engineer for rebase concerns/considerations
  - @albertolamela # HCP considerations + architect
approvers: # This should be a single approver. The role of the approver is to raise important questions, ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval. Team leads and staff engineers often make good approvers.
  - @JoelSpeed
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None". Once your EP is published, ask in #forum-api-review to be assigned an API approver.
  - N/A
creation-date: 2026-08-19
last-updated: 2026-08-19
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced|informational
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
---

# Managing Upstream Feature Gates

## Summary

This enhancement is focused on reaching consensus on OpenShift's policy on
how we track and enable upstream feature gates on OpenShift. This includes, but is
not limited to:
- When upstream features should be enabled and what OpenShift featureset they should be enabled in.
- Testing to be implemented to enforce correct enablement/api-serving requirements.

## Motivation

Historically in OpenShift, we have not closely tracked the state of upstream Kubernetes feature gates and
enforced when they should or should not be enabled within OpenShift. Instead, this has been on a case-by-case
basis in which teams wanting to enable alpha/beta upstream features early would undergo the process of creating an OpenShift equivalent
feature gate to enable in a given OpenShift feature set and plumbing everything through correctly.

More recently, we've identified that not having a policy around upstream feature gate enablement state on OpenShift
results in it being practically impossible for teams building products/integrations (or customers interested in early testing)
to enable a featureset on OpenShift, like TechPreviewNoUpgrade, to test the functionality. This also means that we are not
getting any meaningful signal as to the impact that a feature might have on OpenShift until the feature becomes enabled-by-default
upstream.

The primary motivation of this enhancement is to close this gap.

### User Stories

1. As a member of OpenShift concerned with product quality, I want to enable upstream features that are not enabled-by-default to ensure we are getting CI signals as to the impact a feature may have on the platform.

2. As a member of OpenShift working on a feature/product/etc., I want upstream features that are not enabled-by-default and enabled-by-default but in beta to be enabled in the DevPreviewNoUpgrade and TechPreviewNoUpgrade feature sets so that I can test integrations with that feature before it goes GA in upstream.

3. As a user of OpenShift interested in trying out new upstream Kubernetes features, I want to be able to enable upstream features that are not enabled-by-default and in beta so that I can test new Kubernetes functionality and plan how I may be able to use it in a production environment when it is stable.

### Goals

- Define an explicit policy for when upstream features are enabled in which OpenShift feature sets.
- Define automated payload testing requirements that enforce the agreed upon policy for upstream gate enablement.
- Understand and identify mitigations to CI stability impacts of enabling upstream features that are not enabled by default.

### Non-Goals

- TBD

## Proposal

This EP proposes the following:
- A concrete policy and tooling for how OpenShift feature-gates are created and the featuresets they are included in (defined in openshift/api).
- End-to-end testing for determining which group-version-resources should be served by default and which features should be enabled by default. This testing would be performed on both OpenShift and HyperShift.

### Upstream Feature Gate Enablement Policy

This section covers what OpenShift feature set upstream feature gates will be enabled in based on their level of maturity.

#### DevPreviewNoUpgrade Feature Set

All upstream feature gates are enabled in the DevPreviewNoUpgrade feature set.

#### TechPreviewNoUpgrade Feature Set

All upstream feature gates that are enabled-by-default, regardless of maturity, will be enabled in the TechPreviewNoUpgrade feature set.

All upstream feature gates that are beta, but not enabled-by-default, will be enabled in the TechPreviewNoUpgrade feature set.

#### Default Feature Set

All upstream feature gates that are marked as GA will be enabled in the Default feature set.

### Repository Changes

#### openshift/api

In order to facilitate automating this process as much as possible, openshift/api will have new tooling
introduced to help generate the set of upstream feature gate definitions.

It will utilize upstream libraries to identify feature gate maturity state based on the current k8s library version being imported.
More specifically, it will use `k8s.io/apiserver/pkg/util/feature.DefaultMutableFeatureGate` and a blank import of `k8s.io/pkg/features` to trigger [this `init` function](https://github.com/kubernetes/kubernetes/blob/2220c3853a2402ffc0502995c49b383f84ae8ceb/pkg/features/kube_features.go#L3012-L3025)

Additionally, feature gate definitions will be updated to allow specifying the group-resource(s) associated with a feature-gate.
This will allow openshift/api to maintain a library implementation that, given a set of enabled feature gates, will return the appropriate
API group-version-resource pairings to be enabled on the kube-apiserver via the `--runtime-config` flag.

Proof-of-Concept PR: https://github.com/openshift/api/pull/2994

#### openshift/cluster-kube-apiserver-operator

Instead of maintaining a [static list of feature-gate name -> group-version pairing](https://github.com/openshift/cluster-kube-apiserver-operator/blob/9c413cd4dc8c3876cc40ee85c207bf9b143f106f/pkg/operator/configobservation/apienablement/observe_runtime_config.go#L18-L32), this operator will be updated to utilize the new openshift/api owned library.

#### openshift/hypershift

Instead of [maintaining a hardcoded set of conditionals](https://github.com/openshift/hypershift/blob/458c251a004eaa43c12aa03b163cbd7dcea9646e/control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go#L240-L254), the hostedcontrolplane controller will be updated to utilize the new openshift/api owned library.

#### openshift/origin

In order to ensure that we don't accidentally serve the beta versions of APIs for enabled feature gates, a new test will be added to the openshift conformance suite that uses the openshift/api owned
library to validate that we are only serving the `v1` (or greater) version of the group-resource associated with a gate that has been included in the Default feature set.

### Workflow Description

#### Kube Rebase

1. Update openshift/api tooling k8s package versions.
2. Run `make update` to run generators, including new upstream feature-gate generator.
3. For feature-gates that require APIs to be enabled, add the appropriate group-resource(s) to the gate definition.
4. Create and merge PR with changes.
5. Update openshift/origin dependency on openshift/api
6. Update openshift/cluster-kube-apiserver-operator and openshift/hypershift dependency on openshift/api.

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

TBD

## Alternatives (Not Implemented)

TBD

## Open Questions [optional]

TBD

## Test Plan

N/A

## Graduation Criteria

N/A

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A
