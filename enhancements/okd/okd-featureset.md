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

This enhancement proposes the introduction of a new "OKD" feature set in the OpenShift API that will be enabled by default on all OKD clusters. This feature set allows OKD clusters to enable select TechPreview features in addition to all the features in the Default featureset while maintaining the ability to upgrade, differentiating the community OKD distribution from the supported OpenShift Container Platform product. This featureset will replace the Default featureset for OKD clusters.

## Motivation

OKD is the community distribution of Kubernetes that powers OpenShift. Currently, OKD users cannot adopt TechPreview features early without blocking upgrades 

By introducing an OKD-specific feature set, we enable the OKD community to test and provide feedback on new features while maintaining a stable upgrade path.

### User Stories

As an OKD cluster administrator, I want to enable TechPreview features on my cluster so that I can test upcoming OpenShift functionality and provide community feedback, without permanently blocking my ability to upgrade. I also want to adopt new features earlier in the lifecycle compared to OCP.

As an OKD developer, I want to help users adopt new features in the community distribution so that I can validate functionality before it reaches OpenShift customers.

As a feature developer, I want OKD clusters to adopt new capabilities so that I can gather early feedback from the community before promoting features to GA.

As an OpenShift engineer, I want to prevent the OKD feature set from being enabled on OpenShift clusters so that we maintain clear boundaries between community and supported distributions.

### Goals

The goals of this proposal are:

* Introduce a new "OKD" feature set in the OpenShift API
* Enable the OKD feature set by default on all OKD clusters
* Allow OKD clusters to enable features from TechPreview while supporting upgrades
* Prevent the OKD feature set from being enabled on OpenShift Container Platform clusters
* Ensure proper validation and immutability (inability to transition to default) of the OKD feature set once enabled
* Generate appropriate CRD manifests for all OpenShift API resources with the OKD feature set

### Non-Goals

* Modifying the behavior of existing feature sets (Default, TechPreviewNoUpgrade, DevPreviewNoUpgrade, CustomNoUpgrade)
* Changing how feature gates work in OpenShift Container Platform
* Providing a mechanism to disable the OKD feature set once enabled on an OKD cluster
* Supporting the OKD feature set on OpenShift Container Platform installations

## Proposal

This proposal introduces a new "OKD" feature set to the OpenShift API configuration types. The feature set will:

1. Be added as a new constant in `config/v1/types_feature.go`
2. Include validation rules that prevent changing the feature set once enabled
3. Block enablement on OpenShift clusters with appropriate error messages
4. Generate CRD manifests for all API versions supporting the OKD variant
5. Be enabled by default on OKD cluster installations
6. Be able to transition from OKD to TechPreviewNoUpgrade or DevPreviewNoUpgrade

### Workflow Description

**OKD cluster administrator** is a user deploying and managing an OKD cluster.

**OpenShift cluster administrator** is a user deploying and managing an OpenShift Container Platform cluster.

**Feature developer** is an engineer developing new features for OpenShift.

#### Scenario 1: Installing a new OKD cluster

1. OKD cluster administrator initiates a cluster installation using `openshift-install` built for OKD
2. The installer automatically sets the feature set to "OKD" in the cluster configuration
3. The cluster installs with the OKD feature set enabled by default
4. The cluster has access to TechPreview features while maintaining upgrade capability
5. The administrator can upgrade the cluster to newer OKD versions without changing the feature set

#### Scenario 2: Attempting to enable OKD feature set on OpenShift

1. OpenShift cluster administrator attempts to set `featureSet: OKD` in the cluster configuration
2. The API server validates the configuration and rejects the request with error: "OKD feature set is not supported on OpenShift Container Platform clusters"
3. The cluster continues operating with its existing feature set configuration
4. Upgrades proceed normally without the OKD feature set

#### Scenario 3: Attempting to change OKD feature set after enablement

1. OKD cluster administrator has a running cluster with `featureSet: OKD`
2. Administrator attempts to change the feature set to "Default" 
3. The API validation rejects the change with error: "OKD may not be changed"
4. The cluster continues operating with the OKD feature set
5. The administrator must reinstall the cluster if a different feature set is required

#### Scenario 4: Attempting to Change OKD to TechPreviewNoUpgrade or DevPreviewNoUpgrade
1. OKD cluster administrator has a running cluster with `featureSet: OKD`
2. Administrator attempts to change the feature set to either "TechPreviewNoUpgrade" or "DevPreviewNoUpgrade"
3. The cluster successfully changes to "TechPreviewNoUpgrade" or "DevPreviewNoUpgrade"

### API Extensions

This enhancement modifies the existing FeatureGate API in the `config.openshift.io/v1` API group:

#### Modified Resources

**FeatureGate** resource modifications:
- Adds new "OKD" value to the FeatureSet enum
- Adds validation rule: `oldSelf == 'OKD' ? self != '' : true` to ensure OKD cannot transition to default 
- Adds validation to prevent OKD from being enabled on OpenShift clusters  

#### CRD Manifests

The following CRD manifests will be generated with OKD-specific variants:

- ClusterVersion (config.openshift.io/v1)
- APIServer (config.openshift.io/v1)
- ClusterImagePolicy (config.openshift.io/v1)
- Authentication (config.openshift.io/v1)
- Build (config.openshift.io/v1)
- ClusterOperator (config.openshift.io/v1)
- Console (config.openshift.io/v1)
- DNS (config.openshift.io/v1)
- FeatureGate (config.openshift.io/v1)
- Image (config.openshift.io/v1)
- ImageContentPolicy (config.openshift.io/v1)
- ImageDigestMirrorSet (config.openshift.io/v1)
- ImageTagMirrorSet (config.openshift.io/v1)
- Infrastructure (config.openshift.io/v1)
- Ingress (config.openshift.io/v1)
- Network (config.openshift.io/v1)
- Node (config.openshift.io/v1)
- OAuth (config.openshift.io/v1)
- OperatorHub (config.openshift.io/v1)
- Project (config.openshift.io/v1)
- Proxy (config.openshift.io/v1)
- Scheduler (config.openshift.io/v1)
- ControllerConfig (machineconfiguration.openshift.io/v1)
- KubeletConfig (machineconfiguration.openshift.io/v1)
- MachineConfig (machineconfiguration.openshift.io/v1)
- MachineConfigPool (machineconfiguration.openshift.io/v1)

And various operator-specific CRDs across multiple API groups.

### Topology Considerations

#### Standalone Clusters

The OKD feature set is specifically designed for standalone OKD clusters and is the primary use case for this enhancement. The feature set will be enabled by default during installation and cannot be changed afterwards.

#### Single-node Deployments or MicroShift

**Single-node OKD deployments (SNO):**
- The OKD feature set applies equally to single-node and multi-node deployments
- Resource consumption impact should be minimal as the feature set itself only controls which features are enabled, not the features themselves
- Individual features enabled by the OKD feature set may have their own resource implications

**MicroShift:**
- MicroShift is a separate distribution and does not use the OpenShift FeatureGate API
- This enhancement does not affect MicroShift deployments
- If MicroShift adopts feature gates in the future, a separate consideration would be needed

### Implementation Details/Notes/Constraints

#### API Type Definition

The OKD feature set is defined in `config/v1/types_feature.go`:

```go
const (
  // OKD turns on features for OKD. Turning this feature set ON is supported for OKD clusters, but NOT for OpenShift clusters.
	// Once enabled, this feature set cannot be changed back to Default, but can be changed to other feature sets and it allows upgrades.
	OKD FeatureSet = "OKD"
)
```

The feature set is added to the list of all fixed feature sets:

```go
var AllFixedFeatureSets = []FeatureSet{
    Default,
    TechPreviewNoUpgrade,
    DevPreviewNoUpgrade,
    CustomNoUpgrade,
    OKD,
}
```

#### Validation Rules

The FeatureGate spec includes Kubernetes validation rules:

```go
// +kubebuilder:validation:Enum=CustomNoUpgrade;DevPreviewNoUpgrade;TechPreviewNoUpgrade;OKD;""
// +kubebuilder:validation:XValidation:rule="oldSelf == 'OKD' ? self != '' : true",message="OKD cannot transition to Default"
```

This ensures:
1. Only valid feature set values can be specified
2. Once OKD is set, it cannot be changed to default

#### Platform Detection

The installer and cluster operators must be able to detect whether they are running on OKD vs OpenShift to:
- Automatically enable the OKD feature set during installation on OKD clusters
- Prevent the OKD feature set from being enabled on OpenShift

This detection is typically done through:
- Build tags during compilation (`scos` for OKD, `ocp` for OpenShift)
- Cluster version metadata
- Installation metadata persisted during cluster creation

#### Feature Set Inheritance

The OKD feature set should inherit all features from the Default feature set, with the addition of selected TechPreview features that are deemed appropriate for community adoption. The specific set of enabled features beyond Default will be determined by:

1. Features that are stable enough for community adoption 
2. Features where early feedback would be valuable
3. Features that align with OKD's mission as a community distribution
4. Features that do not compromise cluster stability or security

#### CRD Generation

CRD manifests with the OKD feature set variant are generated using the same tooling as other feature sets. The generator creates manifests with appropriate feature gate annotations and validation rules for each API resource that supports feature gates.

### Risks and Mitigations

**Risk:** OKD clusters might enable unstable features that cause cluster failures.

**Mitigation:**
- Carefully curate which TechPreview features are included in the OKD feature set
- Maintain clear documentation about the stability expectations of OKD
- Leverage the OKD community for testing and feedback before features reach OpenShift
- Follow the same graduation criteria as other feature sets

**Risk:** Accidental enablement of the OKD feature set on OpenShift clusters could cause support issues.

**Mitigation:**
- Prevent OCP clusters from enabling the OKD featureset through validation in the OpenShift Kubernetes repo

**Risk:** Inability of the OKD feature set transitioning to default could prevent administrators from recovering from configuration issues.

**Mitigation:**
- Document the inability to transition to default clearly in installation guides
- Provide clear guidance on cluster reinstallation if feature set change is required
- Ensure the default configuration is appropriate for most use cases
- Consider providing an escape hatch for exceptional circumstances (future enhancement)

### Drawbacks

**Divergence between OKD and OpenShift:** Introducing an OKD-specific feature set creates a divergence point between the community and commercial distributions. However, this is intentional and aligns with OKD's role as a proving ground for new features.

**Maintenance burden:** Supporting an additional feature set requires maintaining additional CRD manifests and ensuring the OKD variant is tested alongside other feature sets. This is mitigated by the automated generation of manifests and existing CI infrastructure.

**Immutability constraints:** The inability to change the feature set to default after enablement might frustrate administrators who want to switch configurations. However, this ensures consistency and prevents unexpected behavior during the cluster lifecycle.

**Testing complexity:** The OKD feature set adds another configuration variant that needs testing. This is addressed by leveraging existing OKD CI infrastructure and the feature gate testing framework.

## Alternatives (Not Implemented)

### Alternative 1: Reuse TechPreviewNoUpgrade with Different Behavior on OKD

Instead of creating a new OKD feature set, we could modify the TechPreviewNoUpgrade feature set to allow upgrades when running on OKD clusters.

**Rejected because:**
- It would create inconsistent behavior for the same feature set across distributions
- Documentation and user expectations would be confusing
- API contracts would be violated (TechPreviewNoUpgrade explicitly blocks upgrades)
- Difficult to maintain and reason about platform-specific behavior

### Alternative 2: Use CustomNoUpgrade for OKD

Instead of creating a dedicated OKD feature set, use the existing CustomNoUpgrade feature set for OKD clusters.

**Rejected because:**
- CustomNoUpgrade requires manually specifying individual feature gates
- Maintaining a custom configuration for all OKD clusters would be burdensome
- No clear differentiation between OKD and custom configurations
- Does not provide a default, curated experience for OKD users
- Makes it harder to manage and communicate what features are enabled on OKD

### Alternative 3: Create an OKDTechPreview Feature Set

Create a separate OKDTechPreview feature set in addition to the OKD feature set to allow OKD users to choose between stable and experimental features.

**Rejected because:**
- Adds unnecessary complexity with multiple OKD-specific feature sets
- The OKD feature set can already include appropriate TechPreview features
- OKD's role as a community distribution means users expect to adopt new features
- Can be reconsidered in the future if the use case becomes clearer

## Open Questions [optional]

1. What specific TechPreview features should be included in the OKD feature set initially?
   - This will be determined through collaboration with feature owners and the OKD community
   - Initial implementation may start with all Default features and expand incrementally

2. Should there be a mechanism to override the automatic enablement of the OKD feature set during installation?
   - For the initial implementation, no override is planned
   - Can be added in a future enhancement if a compelling use case emerges

## Test Plan

**Unit Tests:**
- Validation logic for the OKD feature set enum value
- Immutability validation (rejecting changes from OKD to default)
- Platform detection logic (OKD vs OpenShift)
- CRD manifest generation for OKD variants

**Integration Tests:**
- API server correctly rejects attempts to enable OKD on OpenShift clusters
- API server correctly rejects attempts to change from OKD to other feature sets
- FeatureGate custom resource can be created with OKD feature set on OKD clusters
- Feature gates are correctly applied when OKD feature set is enabled

**E2E Tests:**
- OKD cluster installs successfully with OKD feature set enabled by default
- OKD cluster can be upgraded with OKD feature set enabled
- Features enabled by the OKD feature set function correctly
- OpenShift cluster installation/configuration rejects OKD feature set

**Upgrade Tests:**
- OKD clusters with OKD feature set can upgrade from version N to N+1
- Feature set remains OKD after upgrade
- Feature gates are correctly maintained across upgrades

## Graduation Criteria

The OKD feature set will be introduced as a stable feature, not following the typical Dev Preview -> Tech Preview -> GA progression, because:

1. It is part of the OpenShift API contract
2. The feature set mechanism is already GA
3. This is adding a new value to an existing, stable enum
4. OKD is already a mature distribution

### Initial Release

The OKD feature set will be considered stable when:

- [ ] All API changes are merged to openshift/api
- [ ] CRD manifests are generated for all relevant API resources
- [ ] Validation logic is implemented and tested
- [ ] Installer changes to enable OKD feature set by default are implemented
- [ ] CI jobs for OKD with the new feature set are passing
- [ ] Documentation is updated to describe the OKD feature set

### Ongoing Requirements

- Maintain compatibility with feature gate framework changes
- Keep CRD manifests in sync as new API versions are added
- Document any new features added to the OKD feature set
- Ensure CI continues to test OKD with the feature set enabled

## Upgrade / Downgrade Strategy

### Upgrade Strategy

**OKD Clusters:**
- Existing OKD clusters without the OKD feature set: During the upgrade to the first version supporting the OKD feature set, the feature set should be automatically enabled if the cluster is detected as OKD. This should be handled by the cluster-version-operator or similar component.
- OKD clusters with OKD feature set already enabled: No changes needed; the feature set persists across upgrades.
- Upgrades are explicitly supported with the OKD feature set enabled.

**OpenShift Clusters:**
- No changes; the OKD feature set cannot be enabled and will not affect OpenShift clusters.

### Downgrade Strategy

**Downgrading from a version with OKD feature set to a version without:**
- If an OKD cluster with the OKD feature set is downgraded to a version that does not recognize the OKD feature set:
  - The cluster-version-operator should handle the unknown feature set gracefully
  - Ideally, the cluster should continue operating but may log warnings about the unknown feature set
  - This scenario should be tested to ensure it does not break the cluster
  - If necessary, clusters may need to be reinstalled rather than downgraded

**General downgrade considerations:**
- Downgrades are generally not supported in OpenShift/OKD
- If a downgrade is attempted, the feature set value being unknown to older versions should not prevent the cluster from functioning
- Individual features enabled by the feature set may have their own downgrade considerations

### Migration Path for Existing OKD Clusters

OKD clusters deployed before the introduction of the OKD feature set will need a migration strategy:

**Option 1: Automatic migration during upgrade**
- Detect OKD clusters using build metadata or cluster version
- Automatically enable the OKD feature set during CVO upgrade
- Log the change clearly for administrator awareness

**Option 2: Manual migration**
- Require administrators to manually set the OKD feature set
- Provide clear documentation and tooling
- May be more transparent but requires user action

**Recommendation:** Implement Option 1 (automatic migration) with clear logging and documentation. This provides the smoothest upgrade experience for OKD users.

## Version Skew Strategy

The OKD feature set introduces version skew considerations:

**Control Plane and Node Version Skew:**
- Feature gates are primarily evaluated at the control plane (API server) level
- Nodes respect feature gates propagated through the kubelet configuration
- Standard OpenShift version skew policies apply (e.g., nodes can be N-2 versions behind control plane)
- The OKD feature set does not introduce new version skew constraints beyond existing feature gate behavior

**Component Version Skew:**
- All components must be aware of the OKD feature set enum value
- Components from older versions that do not recognize "OKD" as a valid value may fail validation
- This is mitigated by:
  - Synchronizing API changes across all components
  - Using CI to test component compatibility
  - Following standard OpenShift component versioning

**Operator Version Skew:**
- Operators must handle clusters with the OKD feature set enabled
- Operators should either:
  - Explicitly support the OKD feature set
  - Treat it equivalently to an appropriate existing feature set (likely Default + TechPreview)
  - Gracefully handle unknown feature sets

**API Client Version Skew:**
- Clients using older API definitions may not recognize the OKD feature set value
- This is acceptable as long as:
  - Clients do not actively reject unknown enum values
  - The API server continues to accept and persist the OKD value
  - Clients can read the raw value even if they don't understand it

## Operational Aspects of API Extensions

### Impact of API Extensions

The OKD feature set introduces changes to the FeatureGate API and generates numerous CRD manifests with OKD variants:

**API Server Load:**
- Minimal impact: The feature set validation is lightweight
- No additional webhooks or API servers are introduced
- CRD manifests are loaded at API server startup and do not affect runtime performance

**Validation Performance:**
- The kubebuilder validation (`oldSelf == 'OKD' ? self != '' : true`) is a simple comparison
- Executed only when FeatureGate resources are modified (infrequent operation)
- No measurable impact on API throughput or latency

**CRD Proliferation:**
- Each API resource with feature gate support gets an additional OKD-variant CRD manifest
- This increases the number of files in the repository and installation manifests
- No runtime impact as CRDs are registered once during cluster initialization
- Storage impact is negligible (additional CRD definitions are small)

### Service Level Indicators (SLIs)

The OKD feature set itself does not introduce new SLIs, but relies on existing indicators:

**API Availability:**
- Monitored through standard `kube-apiserver` availability metrics
- No impact expected from the OKD feature set addition

**Validation Latency:**
- Monitored through API request latency metrics
- OKD feature set validation adds negligible latency (< 1ms)

**Feature Gate Operator Health:**
- Existing operator health checks apply
- Conditions: `Available`, `Progressing`, `Degraded`
- Metrics: `cluster_operator_conditions`

### Failure Modes

**Failure Mode 1: Invalid feature set value**
- **Symptom:** API server rejects FeatureGate resource with validation error
- **Impact:** Cluster administrators cannot modify the FeatureGate configuration
- **Detection:** API server logs show validation errors; CLI commands return error messages
- **Recovery:** Correct the feature set value to a valid option (Default, TechPreviewNoUpgrade, CustomNoUpgrade, OKD, or empty string)

**Failure Mode 2: Attempt to change OKD feature set to Default**
- **Symptom:** API server rejects update with error "OKD cannot be transitioned to Default"
- **Impact:** Cluster administrators cannot change the feature set from OKD to default 
- **Detection:** API server logs show validation errors; CLI commands return error messages
- **Recovery:** This is expected behavior; cluster must be reinstalled if a the default featureset is required

**Failure Mode 3: Version skew in component awareness of OKD feature set**
- **Symptom:** Components fail to start or report degraded status due to unknown feature set value
- **Impact:** Specific operators or components may not function correctly
- **Detection:** Operator logs show errors about unknown feature set; operator conditions show Degraded=True
- **Recovery:** Upgrade affected components to versions that recognize the OKD feature set

## Support Procedures

### Detecting OKD Feature Set Issues

**Checking the current feature set:**
```bash
oc get featuregate cluster -o jsonpath='{.spec.featureSet}'
```

**Viewing FeatureGate resource details:**
```bash
oc describe featuregate cluster
```

**Checking cluster-version-operator logs for feature set related issues:**
```bash
oc logs -n openshift-cluster-version deployment/cluster-version-operator | grep -i featureset
```

**Checking API server audit logs for feature set validation failures:**
```bash
oc adm node-logs --role=master --path=kube-apiserver/audit.log | grep -i featuregate
```

### Symptoms and Alerts

**Symptom:** Cluster upgrade is blocked
- Check if OKD feature set is enabled on an OpenShift cluster
- Review cluster-version-operator logs for feature set related errors
- Verify the cluster type matches the feature set (OKD cluster should have OKD feature set; OpenShift should not)

**Symptom:** API server rejects FeatureGate updates
- Check if attempting to change from OKD to another feature set (not allowed)
- Check if attempting to enable OKD on OpenShift (not allowed)
- Review validation error messages in API server response

**Symptom:** Features not behaving as expected
- Verify the feature set is correctly configured
- Check if the expected features are enabled for the OKD feature set
- Review feature gate effective configuration: `oc get featuregate cluster -o yaml`

### Disabling the OKD Feature Set

**Important:** The OKD feature set cannot be disabled once enabled. This is by design to ensure cluster consistency.

**If OKD feature set must be removed:**
1. Back up all critical cluster data and configurations
2. Plan for cluster downtime
3. Reinstall the cluster with the desired feature set
4. Restore applications and data to the new cluster

### Graceful Degradation

The OKD feature set validation is enforced at the API level:

- If validation fails, the FeatureGate resource will not be updated
- The cluster continues operating with the existing feature set configuration
- No cascading failures result from feature set validation failures
- Individual features may have their own failure modes independent of the feature set

### Impact on Cluster Health

**When OKD feature set is functioning correctly:**
- No impact on cluster health indicators
- Cluster operates normally with features enabled according to the OKD feature set definition

**When OKD feature set is configured incorrectly:**
- API server prevents invalid configurations through validation
- Cluster continues operating with last known good configuration
- No automatic reconciliation or rollback occurs
- Manual intervention required to correct configuration issues

## Infrastructure Needed [optional]

**CI Infrastructure:**
- OKD build and test jobs must be updated to expect the OKD feature set by default
- E2E test suites should include scenarios with the OKD feature set enabled
- Upgrade test jobs for OKD should verify feature set persistence

**Build Infrastructure:**
- No changes needed; existing OKD build infrastructure can accommodate this change
- The `scos` build tag will continue to differentiate OKD from OpenShift builds

**Documentation:**
- Update OKD installation documentation to explain the OKD feature set
- Add troubleshooting guides for feature set related issues
- Document the differences between OKD and OpenShift feature sets
- Provide guidance on which features are enabled in the OKD feature set

**Repository Infrastructure:**
- No new repositories required
- Changes are made to existing openshift/api repository and will be vendored to other repos
- The OpenShift Kubernetes repo will need changes
- The Cluster Config Operator repo will need changes to allow the OKD featureset to allow upgrades
- Generated CRD manifests will be committed to the repository

## Implementation History

- 2025-08-12: Initial PR opened (https://github.com/openshift/api/pull/2451)
- 2025-12-02: Enhancement proposal created
