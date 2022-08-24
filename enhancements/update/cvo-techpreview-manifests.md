---
title: CVO TechPreview Manifests
authors:
  - "@deads2k"
reviewers:
  - "@wking"
approvers:
  - "@sdodson"
  - "@wking"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - @sttts
creation-date: 2021-11-15
last-updated: 2021-11-15
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - slack thread somewhere
see-also:
replaces:
superseded-by:
---

# CVO TechPreview Manifests

## Summary

[TechPreviewNoUpgrade](https://github.com/openshift/api/blob/be1be0e89115702f8b508d351c4f5c9a16e5ae95/config/v1/types_feature.go#L32-L34)
is the canonical way to enable tech preview features.
It is the setting that all operators use to enable Tech Preview features.
The CVO will honor this setting on a per-manifest basis, so that tech preview manifests are never present on
non-tech preview clusters.

## Motivation

When introducing tech preview operators, the current state of the art (before this enhancement) is to have the
operator build an inert mode that detects if tech preview is not enable and then does not take action.
This approach results in having additional resources, including the clusteroperator/tech-preview itself,
installed on every cluster, including non-tech-preview upgrades.
This also means that currently, if the tech preview does not progress to stable, the component has to keep "dead" manifests
around and marked for deletion by the CVO on upgraded clusters.
By having the CVO honor tech preview per manifest, we can avoid fanning out inert logic to every tech preview operator,
we can avoid exposing tech preview operators to customers via clusteroperators, and we can avoid creating unnecessary
resources like namespaces, roles, rolebindings, serviceacounts, and deployments.

### Goals

1. CVO honors an "only create, reconcile, and update this manifest in clusters with tech preview enabled".
2. Allowing manifests conditional on other FeatureSets

### Non-Goals

1. Bootstrap rendering for tech preview operators.
2. 4.y upgrades.  TechPreviewNoUpgrade explicitly disallows upgrades from 4.y to 4.y+1.
3. Cleanup of manifests created as TechPreviewNoUpgrade.  TechPreviewNoUpgrade explicitly disallows being unset and this
    is enforced using validation on the apiserver.
4. Allow manifests to be removed or un-reconciled if TechPreviewNoUpgrade is *not* set.  There are a few case (mostly
    around CRDs), where this capability is useful, but it is not as common as needing to create something additional.

## Proposal

Add an annotation that can be set on CVO manifests called `release.openshift.io/feature-set` that can be set to a comma
delimited list that can contain any value from 
[FeatureSet](https://pkg.go.dev/github.com/openshift/api/config/v1#FeatureSet) and "Default" in place of "".
If the value is not known, the manifest is never created.
Since most people add manifests in order to see them applied, they will notice if their manifest is never created and
the choice of annotation allows for potential future usage in the CVO consistent with feature gate handling used by
other operators.

If a manifest sets `.metadata.annotations["release.openshift.io/feature-set"]="TechPreviewNoUpgrade"`, the manifest will
not be created unless `featuregates.config.openshift.io|.spec.featureSet="TechPreviewNoUpgrade"`.
If `featuregates.config.openshift.io|.spec.featureSet="TechPreviewNoUpgrade"`, then the manifest is reconciled as normal
for whatever stage the CVO is in.
If a manifest sets `.metadata.annotations["release.openshift.io/feature-set"]="Default"`, the manifest will
not be created unless `featuregates.config.openshift.io|.spec.featureSet=""` (empty string).
If a manifest sets `.metadata.annotations["release.openshift.io/feature-set"]="Default,LatencySensitive"`, the manifest will
not be created unless `featuregates.config.openshift.io|.spec.featureSet` is `""` or `"LatencySensitive"`.
During bootstrapping, the CVO will assume no feature sets are enabled until it can successfully retrieve
`featuregates.config.openshift.io` from the Kubernetes API server.
If bootstrapping changes are required, it will be responsibility of the contributing team to manage the proper inclusion
in the installer and other bootstrapping components.

If a special manifest is required for a FeatureSet that allows changes (LatencySensitive to Default for instance),
the person adding the manifest will have to add the appropriate delete-annotated manifest in Default.
This is expected to be a rare occurrence of a rare occurrence.

### User Stories

### API Extensions

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

## Design Details

### Open Questions [optional]

### Test Plan

1. The TechPreview CI jobs (already present), should run after this feature is implemented.
2. In 4.10, the cluster-api clusteroperator should not be present in our "normal" CI flows.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

FeatureSets do not change often.
In fact, all functional changes we have today cannot be undone at all (you cannot move out of TechPreviewNoUpgrade or CustomNoUpgrade).
When upgrading or downgrading, the selection criteria will remain consistent.
If the selection criteria does change and if a manifest should no longer exist, then it is responsibility of the
component contributing the manifest to add the appropriate delete-annotated manifest for whichever FeatureSet requires it.

### Version Skew Strategy

It is the responsibility of the component to ensure that manifests selected for particular FeatureSets are properly
managed across versions.
David is so confident that leaks are unlikely, that he doesn't think it's worth the time to attempt to build audit
tooling to help detect them.
Trevor promises to make David aware of every time this fails.

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

### Inert Operators
It is possible for every tech preview operator to
1. create an inert run mode
2. self-bootstrap resources "normally" created by the CVO, like CRDS.
3. if the operator doesn't graduate, it must continue to be included in the payload and list its CVO managed resources
    to remove them in the next release.
5. create a clusteroperator that every customer has ignore

This fans the problem out from a single point (CVO), to every tech preview operator.
It also increases the chance that a cluster admin would misinterpret an inert-mode operator as tainting their cluster
with unsupported, tech-preview bits.

### Allowing Other FeatureSets
The `featuregates.config.openshift.io|.spec.featureSet` takes other values, but TechPreviewNoUpgrade has qualities that
make it easier to start with.
1. 4.y+1 upgrades are not allowed
2. it cannot be unset

The combination of these two things make it easier to quickly deliver an MVP.
The API is formed in an extensible way, but only supporting TechPreviewNoUpgrade makes the changes tractable for a single release.

## Infrastructure Needed [optional]
