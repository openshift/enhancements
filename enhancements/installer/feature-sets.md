---
title: installer-feature-sets
authors:
  - "@patrickdillon"
reviewers:
  - "@wking, installer, upgrades"
  - "@bparees, feature sets"
approvers:
  - "@sdodson"
  - "@zaneb"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@deads2k"
creation-date: 2022-09-07
last-updated: 2022-09-07
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CORS-2253
---

# Installer Feature Sets

## Summary

This enhancement proposes adding support for feature sets to `openshift-install`.
Users would be able to enable feature sets at install time. The Installer would support all feature sets in the OpenShift API,
and write the selected Feature Set to the cluster.

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

The goals of this proposal are:
* To provide an API that will allow users to enable feature sets in the Installer
* Allow the Installer to enable feature sets in the cluster
* Ensure the proposed implementation adheres to best practices for feature sets

### Non-Goals

None

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
5. Installer generates `FeatureGate` manifest
6. Installer emits warning message indicating the enabled feature set when provisioning infrastructure
7. Cluster installs successfully with TechPreview cluster 


### API Extensions

[OpenShift API Feature Sets](https://github.com/openshift/api/blob/master/config/v1/types_feature.go)
would become part of the install config API:

```go

// FeatureSet enables features that are not part of the default feature set.
	// +optional
	FeatureSet configv1.FeatureSet `json:"featureSet,omitempty"`
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

`explain` should handle feature-gated fields appropriately, either by skipping them (not printing them) or clearly marking
them as gated features. The initial implementation will simply include
comments denoting fields that are feature gated.

### Risks and Mitigations

I am not aware of any significant risks to introducing this feature. 

### Drawbacks

I am not aware of any significant drawbacks to introducing this feature.

## Design Details

### Open Questions [optional]

N/A

### Test Plan

Unit testing the new tech preview functionality and observing existing e2e tests should be sufficient.

### Graduation Criteria

n/a

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature


### Upgrade / Downgrade Strategy

Upgrades will be blocked based on the feature set. There is
discussion about whether it makes sense to block a cluster
upgrade based on Installer-only features, i.e. if the changes
are only in the Installer and don't affect the cluster, why
block upgrades?

The upgrade status of the cluster is based on the feature set,
so the current implementation would have the ability to accomodate
an Installer-only tech preview if we were able to add a new feature
set for that purpose. For discussion: if we added an `InstallerTechPreview` or `TechPreviewInstallOnly` feature set,
it could be possible that such a feature set would allow upgrades.

There could be a use case for supporting both feature sets in the
Installer: `InstallerTechPreview` would be for install-only
features, while `TechPreview` may apply to features that affect
both the Installer and the cluster.

The addition of such an `InstallerTechPreview` would need to be
added in a follow up enhancement.

### Version Skew Strategy

There is a small risk in unreleased Installers that there may be
a mismatch between the API versions in the installer and the cluster.
The Installer would need to be kept in sync with the OpenShift API
to make sure it is aware of current feature sets, which are updated
rarely.

### Operational Aspects of API Extensions

n/a

#### Failure Modes

Feature gating itself is straightforward install-config validation, so there are no predictable, significant failures.

#### Support Procedures

This should already be consistent with standard tech preview practices.

## Implementation History


## Alternatives
