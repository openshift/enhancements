---
title: okd-featureset
authors:
  - "@jatinsu"
reviewers:
  - "@JoelSpeed, for API review and feature gate validation"
  - "@Prashanth684, for OKD-specific implementation details"
  - "@sdodson, for OKD platform considerations"
  - "@wking, for the Cluster Version Operator"
approvers:
  - "@sdodson"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-12-02
last-updated: 2025-12-02
tracking-link:
  - https://issues.redhat.com/browse/OKD-259
see-also:
  - "/enhancements/installer/feature-sets.md"
replaces:
superseded-by:
---

# OKD Feature Set

## Summary

This enhancement introduces a new "OKD" feature set in the OpenShift API that will be enabled by default on all
OKD clusters. This feature set allows OKD clusters to enable select TechPreview features with stable APIs (v1 or
v1beta1) in addition to all features in the Default feature set while maintaining upgrade capability. This serves
dual purposes: providing early access to stable features for the OKD community and acting as a signal driver for
OpenShift through community adoption, feature validation in real-world usage, and nightly job testing of upgrades
and e2es. This feature set will replace the Default feature set for OKD clusters.

## Motivation

OKD is the community distribution of Kubernetes that powers OpenShift. Currently, OKD users cannot adopt
TechPreview features early without blocking upgrades. A key driver for this enhancement is to enable features
with stable APIs (v1 or v1beta1) early on OKD clusters, which serves multiple purposes:

1. **Community enablement**: Provide OKD users early access to stable features before they graduate to GA in
   OpenShift
2. **Signal driver for OpenShift**: OKD acts as a proving ground where the community adopts new features through
   upgrades and validates stability through real-world usage
3. **Quality signal**: Nightly CI jobs on OKD test upgrades and e2es with these features enabled, providing early
   signals for OpenShift releases

By introducing an OKD-specific feature set, we enable the OKD community to adopt and provide feedback on features
with stable API guarantees while maintaining a stable upgrade path.

### User Stories

As an OKD cluster administrator, I want to enable TechPreview features on my cluster so that I can adopt upcoming
OpenShift functionality and provide feedback, without permanently blocking my ability to upgrade. I also want to
adopt new features earlier in the lifecycle compared to OCP.

As an OKD developer, I want to help users adopt new features so I can validate functionality before it reaches OpenShift customers.

As a feature developer, I want OKD clusters to adopt new capabilities for early community feedback before promoting features to GA.

### Goals

* Introduce a new "OKD" feature set in the OpenShift API
* Enable the OKD feature set by default on all OKD clusters
* Allow OKD clusters to enable TechPreview features while supporting upgrades
* Prevent the OKD feature set from being enabled on OpenShift Container Platform clusters
* Ensure proper validation preventing transition to Default once enabled
* Generate appropriate CRD manifests for all OpenShift API resources with the OKD feature set

### Non-Goals

* Modifying the behavior of existing feature sets
* Changing how feature gates work in OpenShift Container Platform
* Providing a mechanism to disable the OKD feature set once enabled
* Supporting the OKD feature set on OpenShift Container Platform installations

## Proposal

This proposal introduces a new "OKD" feature set that:
1. Is added as a new constant in `config/v1/types_feature.go`
2. Includes validation rules preventing transition to Default
3. Blocks enablement on OpenShift clusters
4. Generates CRD manifests for all API versions
5. Is enabled by default on OKD cluster installations
6. Automatically migrates existing OKD clusters from Default to OKD feature set during upgrade
7. Can transition to TechPreviewNoUpgrade or DevPreviewNoUpgrade

### Workflow Description

#### Scenario 1: Installing a new OKD cluster
1. OKD cluster administrator initiates installation using OKD-built `openshift-install`
2. CVO detects that it has been built for OKD
3. CVO automatically enables OKD feature set
4. Cluster operates with OKD feature set, providing access to select TechPreview features with upgrade capability

#### Scenario 2: Attempting to enable OKD feature set on OpenShift
1. OpenShift administrator attempts to set `featureSet: OKD`
2. API server rejects with error: "OKD feature set is not supported on OpenShift Container Platform clusters"
3. The cluster continues operating with its existing feature set configuration

#### Scenario 3: Attempting to change OKD to Default
1. OKD administrator attempts to change feature set to Default
2. API validation rejects with error: "OKD cannot transition to Default"
3. Cluster must be reinstalled if Default is required

#### Scenario 4: Changing OKD to TechPreviewNoUpgrade or DevPreviewNoUpgrade
1. OKD administrator changes feature set to TechPreviewNoUpgrade or DevPreviewNoUpgrade
2. Change succeeds

#### Scenario 5: Upgrading an existing OKD cluster with Default feature set
1. OKD cluster running older version has Default ("") feature set configured
2. Administrator upgrades to new version with OKD feature set support
3. CVO detects that it has been built for OKD and sees Default feature set
4. CVO automatically migrates the FeatureGate resource from Default to OKD
5. Migration is logged for administrator awareness
6. Cluster operates with OKD feature set after migration

### API Extensions

**FeatureGate** resource modifications:
- Adds new "OKD" value to the FeatureSet enum
- Adds validation rule: `oldSelf == 'OKD' ? self != '' : true`
- Validates against OKD enablement on OpenShift clusters

#### CRD Manifests

OKD-specific CRD variants will be generated for all config.openshift.io/v1 resources (CClusterVersion, APIServer, ClusterImagePolicy, Authentication, Build, ClusterOperator, Console, DNS, FeatureGate, Image, ImageContentPolicy, ImageDigestMirrorSet, ImageTagMirrorSet, Infrastructure, Ingress, Network, Node, OAuth, OperatorHub, Project, Proxy, and Scheduler) and machineconfiguration.openshift.io/v1 resources (ControllerConfig, KubeletConfig, MachineConfig, MachineConfigPool), plus various operator-specific CRDs.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The OKD feature set applies equally to Hypershift with minimal resource impact

#### Standalone Clusters

The OKD feature set is designed for standalone OKD clusters. The feature set is enabled by default during installation and can transition to TechPreviewNoUpgrade or DevPreviewNoUpgrade.

#### Single-node Deployments or MicroShift

- **SNO:** The OKD feature set applies equally to single-node and multi-node deployments with minimal resource impact
- **MicroShift:** The OKD feature set applies equally to microshift with minimal resource impact

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

#### API Type Definition

```go
const (
  // OKD turns on features for OKD. Turning this feature set ON is supported for OKD clusters, but NOT for OpenShift clusters.
  // Once enabled, this feature set cannot be changed back to Default, but can be changed to other feature sets and it allows upgrades.
  OKD FeatureSet = "OKD"
)

var AllFixedFeatureSets = []FeatureSet{
    Default,
    TechPreviewNoUpgrade,
    DevPreviewNoUpgrade,
    CustomNoUpgrade,
    OKD,
}
```

Tests that ensures all Default featuregates are enabled in OKD:
```go
	// Check that all Default featuregates are in OKD
		missingInOKD := defaultEnabled.Difference(okdEnabled)

		if missingInOKD.Len() > 0 {
			missingList := missingInOKD.List()
			sort.Strings(missingList)

			t.Errorf("ClusterProfile %q: OKD featureset is missing %d featuregate(s) that are enabled in Default:\n  - %s\n\nAll featuregates enabled in Default must also be enabled in OKD.",
				clusterProfile,
				missingInOKD.Len(),
				strings.Join(missingList, "\n  - "),
```

#### Validation Rules

```go
// +kubebuilder:validation:Enum=CustomNoUpgrade;DevPreviewNoUpgrade;TechPreviewNoUpgrade;OKD;""
// +kubebuilder:validation:XValidation:rule="oldSelf == 'OKD' ? self != '' : true",message="OKD cannot transition to Default"
```

#### Platform Detection

The Kubernetes repo (hyperkube operator) detects whether it is running on OKD vs OpenShift by checking the build
tag set at compile time. When compiled with the `scos` build tag, the binary is identified as OKD. When compiled
with the `ocp` build tag, it is identified as OpenShift. This detection is used to automatically enable the OKD
feature set on new OKD cluster installations and prevent the OKD feature set from being enabled on OpenShift
clusters.

#### Feature Set Inheritance

OKD inherits all Default features plus selected TechPreview features deemed appropriate for community adoption
based on stability, feedback value, and alignment with OKD's mission. Only features with stable API guarantees
(v1 or v1beta1) will be included beyond the Default feature set to ensure cluster stability and upgrade
compatibility.

### Risks and Mitigations

The primary risk is unstable features causing cluster failures, which is mitigated by carefully curating
TechPreview features and only including those with stable API guarantees (v1 or v1beta1), while enabling
community adoption for real-world validation. To prevent accidental enablement on OpenShift clusters, validation
logic blocks the OKD feature set from being enabled on OCP. Finally, the inability to transition back to Default
may create recovery challenges, which is addressed by clearly documenting this limitation and providing
reinstallation guidance when needed.

### Drawbacks

Introducing an OKD-specific feature set creates a divergence point between the community and commercial
distributions, though this is intentional and aligns with OKD's role as a proving ground for new features.
Supporting an additional feature set requires maintaining additional CRD manifests and ensuring the OKD variant
is tested alongside other feature sets, which is mitigated by automated generation of manifests and existing CI
infrastructure. The inability to transition back to Default after enablement might frustrate administrators who
want to switch configurations, but this ensures consistency and prevents unexpected behavior during the cluster
lifecycle. Finally, the OKD feature set adds another configuration variant that needs testing, which is
addressed by leveraging existing OKD CI infrastructure and the feature gate testing framework.

## Alternatives (Not Implemented)

### Alternative 1: Reuse TechPreviewNoUpgrade with Different Behavior on OKD

Instead of creating a new OKD feature set, we could modify TechPreviewNoUpgrade to allow upgrades when running on
OKD clusters. This was rejected because it would create inconsistent behavior for the same feature set across
distributions. TechPreviewNoUpgrade explicitly blocks upgrades by contract, and violating this would confuse
users and documentation. Platform-specific behavior for a single feature set is difficult to maintain and
reason about.

### Alternative 2: Use CustomNoUpgrade for OKD

Instead of creating a dedicated OKD feature set, we could use the existing CustomNoUpgrade feature set for OKD
clusters. This was rejected because CustomNoUpgrade requires manually specifying individual feature gates, which
would be burdensome to maintain across all OKD clusters. There would be no clear differentiation between OKD and
custom configurations, and it doesn't provide a default curated experience for OKD users. Additionally, it would
block upgrades.

### Alternative 3: Create OKD ClusterProfile

Instead of creating an OKD feature set, we could create a new ClusterProfile that would enable certain
featuregates in addition to the default featuregates. This was rejected because ClusterProfiles are outside the
scope of this enhancement and affect far more components than required. Implementing a full cluster profile is
significantly more complex than needed, and the OKD feature set is conceptually a "featuregate profile" rather
than a cluster profile, making a feature set more appropriate.

## Open Questions [optional]

1. What specific TechPreview features should be included initially?
   - Determined through collaboration with feature owners and OKD community

2. Should there be an override mechanism for automatic enablement?
   - Not planned initially; can be added if compelling use case emerges

## Test Plan

**Unit Tests:**
- Validation logic for the OKD feature set enum value
- Immutability validation (rejecting changes from OKD to default)
- Platform detection logic (OKD vs OpenShift)
- CRD manifest generation for OKD variants

**Integration Tests:**
- API server correctly rejects attempts to enable OKD on OpenShift clusters
- API server correctly rejects attempts to change from OKD to default
- FeatureGate custom resource can be created with OKD feature set on OKD clusters
- Feature gates are correctly applied when OKD feature set is enabled

**Upgrade Tests:**
- OKD clusters with OKD feature set can upgrade from version N to N+1
- Feature set remains OKD after upgrade
- Feature gates are correctly maintained across upgrades

## Graduation Criteria

This enhancement does not follow the standard Dev Preview -> Tech Preview -> GA graduation process. The OKD feature set will be introduced directly as a stable feature of the OKD distribution, as it is part of the core API contract rather than a feature progressing through maturity levels.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

### Upgrade Strategy

**OKD Clusters:** During upgrade, the CVO checks its build tag at startup. If the CVO was compiled with the `scos`
build tag (indicating OKD) and detects that the existing FeatureGate resource has the Default ("") feature set,
it automatically patches the FeatureGate resource to set the feature set to OKD ([CVO implementation](https://github.com/openshift/cluster-version-operator/pull/1287)).
The migration is logged for administrator awareness. OKD clusters with the OKD feature set already enabled will
have no changes needed.

**OpenShift Clusters:** No changes; OKD feature set cannot be enabled.

### Downgrade Strategy

Downgrading from a version with OKD feature set to a version without is not supported. Downgrades are generally not supported in OpenShift/OKD.

### Migration Path for Existing OKD Clusters

CVO automatically migrates existing OKD clusters from Default to OKD during upgrade by checking the OKD build tag
and patching the FeatureGate resource. Migration is logged for awareness.

## Version Skew Strategy

We plan to deliver this as part of a single release so there will be no version skew.

## Operational Aspects of API Extensions

### Impact of API Extensions

**CRD Proliferation:** Additional OKD-variant manifests so there's no runtime impact and there's negligible storage

### Service Level Indicators (SLIs)

The primary SLI for this feature is the passing of nightly e2e tests on OKD clusters, which signals that newly
added features included in the OKD feature set are stable and functioning correctly.

### Failure Modes

**Failure Mode 1: Attempt to change OKD to Default**
- Symptom: API rejects with "OKD cannot transition to Default"
- Recovery: Reinstall cluster if Default is required

**Failure Mode 2: Version skew in component awareness**
- Symptom: Components fail or report degraded status
- Recovery: Upgrade components to versions recognizing OKD feature set

## Support Procedures

### Detecting Issues

Check current feature set:
```bash
oc get featuregate cluster -o jsonpath='{.spec.featureSet}'
```

Check CVO logs:
```bash
oc logs -n openshift-cluster-version deployment/cluster-version-operator | grep -i featureset
```

### Symptoms and Alerts

**Blocked upgrade:** Verify cluster type matches feature set

**Rejected FeatureGate updates:** Check if attempting invalid transition

**Unexpected feature behavior:** Verify feature set configuration and enabled features

### Disabling the OKD Feature Set

The OKD feature set cannot be disabled once enabled by design.

### Graceful Degradation

Validation enforced at API level; failed validation preserves existing configuration; no cascading failures.

### Impact on Cluster Health

Stability depends on features added beyond Default feature set. To ensure stability, only features with stable API guarantees (v1 or v1beta1) will be included to minimize risk and ensure upgrade compatibility.

## Infrastructure Needed [optional]

**N/A**

## Implementation History

- 2025-08-12: Initial PR opened (https://github.com/openshift/api/pull/2451)
- 2025-08-20: Kubernetes PR opened to prevent OKD being enabled on OCP clusters (https://github.com/openshift/kubernetes/pull/2420)
- 2025-12-02: Enhancement proposal created
- 2025-12-16: Machine-api-operator PR opened to vendor API (https://github.com/openshift/machine-api-operator/pull/1448)
- 2025-12-16: Cluster-ingress-operator PR opened to vendor API (https://github.com/openshift/cluster-ingress-operator/pull/1324)
- 2026-01-06: CVO PR opened to automatically migrate Default to OKD feature set (https://github.com/openshift/cluster-version-operator/pull/1287)
