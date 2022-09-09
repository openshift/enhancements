---
title: installer-feature-sets
authors:
  - @patrickdillon
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @wking, installer, upgrades
  - @bparees, feature sets
approvers:
  - @sdodson
  - @zaneb
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - @deads2k
creation-date: 2022-09-07
last-updated: 2022-09-07
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CORS-2253
---

# Installer Feature Sets

## Summary

This enhancement proposes introducing gated features to `openshift-install` through the use of feature sets.
Users would be able to enable non-GA features, particularly fields in the install config API, by opting in
to feature sets. The proposed feature sets are `TechPreviewNoUpgrade` and `ExperimentalNoUpgrade`.
`TechPreviewNoUpgrade` would handle the standard tech preview use case, while `ExperimentalNoUpgrade` would
allow for earlier stage experimental features and active development.

## Motivation

Feature gates have been standard in OpenShift since at least 4.1, but have been absent from the Installer.
There are many observable benefits of the practice, but the primary motivations are:

* Align the Installer with OpenShift release practices and graduation criteria, i.e. enable QE & CI testing
of gated features
* Improve Installer developer experience by removing concerns about exposing non-GA features during active development
* Allow a mechanism for pre-release user testing

We have an immediate use case for this as we are targeted to release a tech preview feature in 4.12.

### User Stories

As a member of OpenShift concerned with the release process (TRT, dev, staff engineer, maybe even PM), I want to opt in to
pre-release features so that I can run periodic testing in CI and obtain a signal of feature quality.  

As quality engineering, I want to opt in to pre-release features so that I can test nightly and other release images 
both in CI & locally.

As a customer, I want to opt in to pre-release features so that I can test and verify proof of concepts. 

As an Installer developer, I want to protect new API fields with a feature gate so that I can build features without concern
that an incomplete feature would be exposed to customers.

### Goals

The goal of this proposal is to provide an API that will allow users to enable feature sets in the Installer. In addition,
the Installer should adhere to best practices for implementing feature sets.  

### Non-Goals

Allowing the Installer to enable feature sets in cluster operators or other components is out of scope.

## Proposal

### Workflow Description

Example workflow demonstrating feature set validation and use:

**Developer** contributes code to the openshift-install codebase.

**Admin** is the user performing an install.

1. Developer adds feature `foo` to openshift-install. Feature `foo`
includes field `bar` in the install config; feature `foo` is part of the Tech Preview Feature Set.
2. Admin specifies `bar: baz` in install-config.yaml (but `featureSet` is empty).
3. `openshift-install` returns an error: `the TechPreviewNoUpgrade feature set must be enabled to use this field`
4. Admin adds `featureSet: TechPreviewNoUpgrade` to install-config.yaml
5. Cluster installs successfully as a non-upgradeable Tech Preview cluster


### API Extensions

Feature Sets would become part of the install config API:

```go
// FeatureSet enables features that are not part of the default set of features.
// +kubebuilder:validation:Enum="";ExperimentalNoUpgrade;TechPreviewNoUpgrade
type FeatureSet string

const (
	// ExperimentalNoUpgrade enables features that are experimental or in active development.
	ExperimentalNoUpgrade FeatureSet = "ExperimentalNoUpgrade"

	// TechPreviewNoUpgrade enables features that are in tech preview, i.e. ready for use but not GA & not fully supported.
	TechPreviewNoUpgrade FeatureSet = "TechPreviewNoUpgrade"
)
```

### Implementation Details/Notes/Constraints [optional]

#### Install Config

The basic implementation through the install config is relatively straight forward. Gated features are protected
by validation:

```go

	if c.FeatureSet == nil || *c.FeatureSet != types.TechPreviewNoUpgrade {
		errMsg := "the TechPreviewNoUpgrade feature set must be enabled to use this field"

    if installConfig.Foo != nil && len(installConfig.Foo.Bar) > 0 {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("foo", "bar"), errMsg))
		}
	}
```

Package `types` defines the install config and represents the API offered by the Installer. Most notably this
API is consumed/used by hive. For this reason, it is important that feature sets be part of the install config.

#### CLI Flag (future work)

On the other hand, the install config is an asset and gated features could potentially be exercised
before the install config is validated (or perhaps the feature is completely independent from the install config). For this reason, it would
be a good idea to provide a flag:

```shell
$ openshift-install create cluster --feature-set TechPreviewNoUpgrade
```

This flag, if provided, would also populate the value in the install config; and conflicts between the flag and install config
would throw an error.

While there are clear use cases for a flag, there are no immediate use cases and the implementation of the Installer (with
its separation between command-line flags and assets) does not lead to a clear-cut design. Implementing this functionality
is certainly possible, but I propose we defer implementation for this until it is determined to be necessary.

#### openshift-install explain

`explain` provides a command-line reference for the install config:

```shell
$ openshift-install explain installconfig | tail -n 6
    pullSecret <string> -required-
      PullSecret is the secret to use when pulling images.

    sshKey <string>
      SSHKey is the public Secure Shell (SSH) key to provide access to instances.
```

`explain` should handle feature gated fields appropriately, either by skipping them (not printing them) or clearly marking
them as gated features.

### Risks and Mitigations

I am not aware of any significant risks to introducing this feature. 

### Drawbacks

I am not aware of any significant drawbacks to introducing this feature.

## Design Details

### Open Questions [optional]

1. For `*NoUpgrade` feature sets, it is a requirement that the cluster be non-upgradeable. When feature sets
are implemented by operators, the operator can simply mark itself as non upgradeable. It is an open question
about how to best enforce non-upgradeable functionality from the Installer. Currently I am experimenting with
enforcing this through cvo overrides, but suggestions are welcome.
2. Are the `ExperimentalNoUpgrade` & `TechPreviewNoUpgrade` feature sets appropriate? Sufficient?
3. The [Tech Preview Guidelines](https://github.com/openshift/enhancements/blob/master/guidelines/techpreview.md)
state that Tech Preview features, and perhaps all gated features, need to be listed in the [OpenShift API](https://github.com/openshift/api/blob/bace76a807222b30bb9bfd4926826348156fb522/config/v1/types_feature.go#L117). Is that necessary for the Installer? If so, for all gated features, or only Tech Preview?

### Test Plan

Unit testing the new tech preview functionality and observing existing e2e tests should be sufficient.

### Graduation Criteria

n/a

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature


### Upgrade / Downgrade Strategy

Upgrades will be blocked.

### Version Skew Strategy

n/a

### Operational Aspects of API Extensions

n/a

#### Failure Modes

Feature gating itself is straightforward install config validation, so there are no predictable, significant failures.

#### Support Procedures

We must clearly identify clusters that are installed with gated features. The only method for doing this that I am
aware of is by making the cluster non-upgradeable. If other resources should be written for tech preview clusters,
please advisde.

## Implementation History


## Alternatives
