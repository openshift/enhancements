---
title: FeatureGates, round 2
authors:
- @deads2k
reviewers:
- team-leads # so they know how featuregates are changing.  The old way will work, but it will be unnecessarily hard.
approvers:
- @jspeed # api-approvers are picking up some code maintenance and we have to merge FeatureGate changes
- staff-eng # so when asked by new team leads, they have some sense of how to do this.
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
- @jspeed
creation-date: 2023-03-29
last-updated: 2023-03-29
---

# FeatureGates, round 2

## Summary

This enhancement aims to reduce the effort required to add a feature gate to TechPreviewNoUpgrade and to promote
that feature gate to Default.
Feature gates in OpenShift are enabled and disabled in a particular FeatureSet, mixing and matching is not allowed.
Prior to this enhancement, it is necessary to vendor openshift/api into every impacted repository.
After this enhancement, it will only be necessary to vendor into cluster-config-operator.

## Motivation

Reduce the effort required to add and promote feature gates, to encourage and eventually require using
feature gates to introduce new features into OCP.

### User Stories

As a staff engineer or release manager, I want to have no-developer-action result in features of unproven reliability
inaccessible-by-default in upgradeable clusters.

As a developer, I want to have a low friction way to add a feature gate to TechPreviewNoUpgrade and to promote a feature
gate to on-by-default.

### Goals

1. have a low friction way to add a feature gate to TechPreviewNoUpgrade
2. have a low friction to promote a feature gate to on-by-default
3. allow feature gates to move from TechPreviewNoUpgrade to Default without releases as TechPreviewNoUpgrade first.

### Non-Goals

1. Change thresholds for requiring TechPreviewNoUpgrade.
   This is a possible future goal, but an update here would be required.
2. Change thresholds for promoting from TechPreviewNoUpgrade to Default.
   This is a possible future goal, but an update here would be required.

## Proposal
### Phase 1
For developers using this, the first phase will look like
1. Open PR to openshift/api to add your feature gate to [TechPreviewNoUpgrade](https://github.com/openshift/api/blob/master/config/v1/types_feature.go#L117).
   The PR should be confined to just the feature gate change and should include a link to a merged enhancement.
2. Nag api-approvers with link to your PR right away and then every 24h or so until they merge it.
3. Vendor the change into openshift/cluster-config-operator.
4. Nag api-approvers with link to your PR right away and then every 24h or so until they merge it.

Notice that this flow eliminates vendoring into multiple repositories and makes it possible to test a feature gate change
with a single PR, so /payload testing functions properly.

### Phase 2
1. Open PR to openshift/api to add your feature gate to [TechPreviewNoUpgrade](https://github.com/openshift/api/blob/master/config/v1/types_feature.go#L117).
   The PR should be confined to just the feature gate change and should include a link to a merged enhancement.
2. Nag api-approvers with link to your PR right away and then every 24h or so until they merge it.
3. Automation vendors openshift/api into cluster-config-operator and opens a PR in a few hours.
   If policy allows it, machine /lgtm-ing is also desired.

Notice that we're down to a single PR to add or promote a featuregate.

### Mechanics of how it works
The reason that the current state of determining which feature gates are enabled for which feature sets is handled
by vendoring code as opposed to API status, is that when feature gates are promoted between releases the vN-1 will
have a pre-GA version.
So if vN writes status that indicates a feature gate should be on, the vN-1 must not honor that.
At the time, we had so few feature gates (see the last 12 releases), that this didn't matter, but since we've added
TechPreviewNoUpgrade CI jobs, stabilized them, and developed means of inspecting reliability of tests and jobs, there
is now a benefit of using these capabilities to test our code before impacting real clusters.


To overcome the original limitation, we will need to key the enabled/disabled feature gates with the payload version
that they are for.
This will ensure that older versions do not enable feature gates that were not GA until the later version.
To do this we will
1. Update FeatureGateStatus to contain a list of enabled and disabled feature gates for up to every version listed
   in the CVO history.
2. Update cluster-config-operator to have a control loop owned by api-approvers to set the FeatureGateStatus
3. Update library-go to make it easy to
   1. wait for FeatureGates to be read from the cluster for this version
   2. make it easy to function with a development build on a patched cluster
   3. make it easy to read the current state of FeatureGates
   4. by default, exit when feature gates change (most processes don't react cleanly to changes at runtime)
   5. make it easy to wire a hook to handle FeatureGate updates if exit isn't desired
4. Update existing operators to use the library-go implementation to consume feature gates in this way
5. Update the installer to pass full FeatureGate manifests to the five-ish operators that participate in rendering
   This is needed to ensure that when cluster-config-operator is updated, all the bootstrap processes will have the same
   feature gates enabled.

#### openshift/api
The type change will look about like this.
Minor fitting expected during API review.
cluster-config-operator will be updated to maintain this status.
```go
type FeatureGateStatus struct {
	// conditions represent the observations of the current state.
	// Known .status.conditions.type are: "DeterminationDegraded"
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// featureGates contains a list of enabled and disabled featureGates that are keyed by payloadVersion.
	// Operators other than the CVO and cluster-config-operator, must read the .status.featureGates, locate
	// the version they are managing, find the enabled/disabled featuregates and make the operand and operator match.
	// The enabled/disabled values for a particular version may change during the life of the cluster as various
	// .spec.featureSet values are selected.
	// Operators may choose to restart their processes to pick up these changes, but remembering past enable/disable
	// lists is beyond the scope of this API and is the responsibility of individual operators.
	// Only featureGates with .version in the ClusterVersion.status will be present in this list.
	// +listType=map
	// +listMapKey=version
	FeatureGates []FeatureGateDetails `json:"featureGates"`
}

type FeatureGateDetails struct {
	// version matches the version provided by the ClusterVersion and in the ClusterOperator.Status.Versions field.
	// +kubebuilder:validation:Required
	// +required
	Version string `json:"version"`
	// enabled is a list of all feature gates that are enabled in the cluster for the named version
	// +optional
	Enabled []FeatureGateAttributes `json:"enabled"`
	// disabled is a list of all feature gates that are disabled in the cluster for the named version
	// +optional
	Disabled []FeatureGateAttributes `json:"disabled"`
}

type FeatureGateAttributes struct {
	// name is the name of the FeatureGate
	// +kubebuilder:validation:Pattern=`^([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.?$`
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`

	// possible (probable?) future additions include
	// 1. support level (Stable, ServiceDeliveryOnly, TechPreview, DevPreview)
	// 2. description
}
```

#### openshift/library-go
The existing config observer will be updated to be a drop-in replacement (this covers a dozen or so operators).
Operators that don't use that pattern can use one like this

```go
featureGateAccessor := NewFeatureGateAccess(args)
go featureGateAccessor.Run(ctx)

// wait for feature gates to observed.
select{
case <- featureGateAccessor.InitialFeatureGatesObserved():
	enabled, disabled, _ := featureGateAccessor.CurrentFeatureGates()
	klog.Infof("FeatureGates initialized: enabled=%v  disabled=%v", enabled, disabled)
case <- time.After(1*time.Minute):
	klog.Errorf("timed out waiting for FeatureGate detection")
	return fmt.Errorf("timed out waiting for FeatureGate detection")
}

// continue from here.
// also open to changes to wire up to k/k FeatureGates if there is demand.
```

library-go also has code allow a "default" payload version (one that hasn't been substituted) to mean, "give me the 
feature gates for the latest CVO version".
This will support the development patch use-cases.

### Workflow Description

#### Variation [optional]

### API Extensions

No admission webhooks are needed.
If complicated validation is required, there is already a FeatureGate admission plugin hardcoded in our openshift/kubernetes
fork and we'll use that.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

### Drawbacks

## Design Details

### Open Questions [optional]

### Test Plan

Unit tests are already added to implementation PRs.
Clusters will not install without the controller functional and we don't support changing feature gates back from
TechPreviewNoUpgrade, so e2e tests are impractical.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

Since this is actually controlling the gates, it is not practical to pre-test it.
Once unit tests pass and clusters successfully install equivalently using this mechanism, PRs will be opened against 
several initial operators, including
1. etcd
2. kube-apiserver
3. kube-controller-manager
4. kube-scheduler
5. openshift-apiserver
6. openshift-controller-manager
7. authentication
8. ingress
9. cloud-credential-operator
10. networking
11. storage

There is incentive to use this mechanism when introducing TechPreviewNoUpgrade features because consuming feature gates
this way is less work.

We're done when all operators using feature gates are reading from FeatureGate.Status instead of vendoring the list.

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

On upgrade to the first level with this change, the cluster-config-operator goes first, so the FeatureGateStatus will
be available.

On downgrade to the last level without this change, the components have hardcoded feature gates, so they don't need
FeatureGateStatus.

On upgrade to another level with this change, the cluster cluster-config-operator goes first, so the FeatureGateStatus will
be available.

On downgrade to another level with this change, the cluster cluster-config-operator goes first, so the FeatureGateStatus will
be available.

### Version Skew Strategy

Every version known to CVO will have FeatureGateStatus left in the FeatureGate resource, so all levels installed on a cluster
will have access to the feature gates they need for their version.
During version changes (upgrade and downgrade), the cluster-config-operator goes first, so feature gates will be set
for whatever target level we're after.

### Operational Aspects of API Extensions

No extensions.

#### Failure Modes

1. cluster-config-operator doesn't update the FeatureGate.
   In this case, the operator version never reaches the desired level, so the CVO doesn't progress to the next component.
   The cluster runs skewed (see above for how) and the cluster-config-operator is debugged.

#### Support Procedures

The cluster-config-operator will use ClusterOperatorStatus to communicate failures.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

None identified.

## Infrastructure Needed [optional]

Phase two automation requires a bot similar to the TRT bot to automatically open PRs.

## Appendix

### Why do we not mix and match feature gates?
1. Having a single set vastly reduces the testing matrix.
   This allows us to maintain stability of our TechPreview features and has been largely successful.
2. You can't mix and match schemas or other manifests.
   This applies to all manifests, but schemas make it obvious.
   If gate/A requires new-field/one in type/first and gate/B requires new-field/two in type/first, then you logically need
   need manifests for no gates, gate/A only, gate/B only, and gate/A and gate/B.
   This is conceptually the case for resource, leading to a significant increase in the manifests (more likely a new approach
   to manifest creation).
3. Makes it impossible to turn on a feature gate that you think is GA, but is actually TechPreview.