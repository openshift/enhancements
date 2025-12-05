---
title: external-secrets-component-config
authors:
  - "@sbhor"
reviewers:
  - "@tgeer"
approvers:
  - "@tgeer"
api-approvers:
  - "@tgeer"
creation-date: 2025-12-1
last-updated: 2025-12-1
tracking-link: 
  - https://issues.redhat.com/browse/ESO-266
see-also:
  - NA
replaces:
  - NA
superseded-by:
  - NA
---

# Component Configuration for external-secrets Operator

## Summary

This document proposes an enhancement to the `ExternalSecretsConfig` API by introducing a `ComponentConfig` extension and global annotations support thorugh `Annotations` field. This allows administrators to specify component-specific overrides for deployment lifecycle settings and global custom annotations for external-secrets components (Controller, Webhook, CertController, BitwardenSDKServer). This change offers administrators greater control over the resource management and operational parameters of components.

## Motivation

Administrators often need to control deployment lifecycle settings and add custom annotations without directly modifying the underlying operator-managed Deployment resources.

### User Stories

- As an administrator, I want to customize deployment lifecycle properties for `external-secrets` operand components to manage their resource consumption and rollback behavior.
- As an operator user, I want to be able to set specific deployment override values independently for each component via ComponentConfig to meet unique operational requirements.
- As a platform engineer, I want to add custom annotations to external-secrets deployments without modifying operator-managed resources.

### Goals

- Provide a declarative API for specifying deployment lifecycle overrides for each component via `componentConfig`.
- Provide a declarative API for adding custom annotations globally to all component Deployments and Pod templates via `controllerConfig.annotations`.
- Support all four operand components: Controller, Webhook, CertController, and BitwardenSDKServer.

### Non-Goals

- Exhaustive validation of individual argument values. Users should consult upstream documentation. Only basic structural validation (non-empty strings, list length limits) will be performed. Invalid arguments will result in container runtime failures (CrashLoopBackOff) rather than API rejection.
- Resource limits, replica counts, environment variables, or other deployment-level settings except for the RevisionHistoryLimit which is specifically introduced by this proposal.

## Proposal

Extend the ControllerConfig API with:
1. A new `annotations` field for adding custom annotations globally to Deployments and Pod templates.
2. A new `componentConfig` field for per-component deployment lifecycle overrides.

### Workflow Description

**For Global Annotations:**

1. **User Configuration:** Administrator updates the `ExternalSecretsConfig` CR with the `controllerConfig.annotations` field containing custom key-value pairs.
2. **Validation:** The operator validates that annotation keys and values conform to Kubernetes annotation constraints.
3. **Reconciliation:** The operator merges user-specified annotations with any default annotations. User annotations take precedence in case of conflicts. Annotations are applied to both the Deployment metadata and Pod template metadata for all components.
4. **Rollout:** Kubernetes detects the annotation changes and performs updates as needed.

**For Component Configuration:**

1. **User Configuration:** Administrator updates the `ExternalSecretsConfig` CR, utilizing the new `componentConfig` list to specify configuration entries for a component (Controller, Webhook, etc.).
2. **Validation:** It verifies the `componentName` against the allowed enum values and enforces uniqueness across the list. It strictly validates the `OverrideArgs` field using the provided `XValidation` rule, ensuring every entry uses the specified format.
3. **Reconciliation:** It parses the `OverrideArgs` field to identify and extract the deployment override key and its corresponding value. It updates the component's underlying Kubernetes Deployment resource by setting the parsed override value in the appropriate `.spec` field.
4. **Rollout:** Kubernetes detects the change in the Deployment's spec and performs a rolling update, applying the new setting to the component.

### Implementation Details/Notes/Constraints

### API Extensions

```go
// ComponentConfig represents component-specific configuration overrides.
type ComponentConfig struct {
  // componentName specifies which deployment component this configuration applies to.
  // Valid values: Controller, Webhook, CertController, Bitwarden
  // +kubebuilder:validation:Enum:=ExternalSecretsCoreController;Webhook;CertController;BitwardenSDKServer
  // +kubebuilder:validation:Required
  ComponentName ComponentName `json:"componentName"`
  
  // overrideArgs allows setting deployment-level overrides for the component.
  //
  // Currently supported deployment-level overrides:
  // - RevisionHistoryLimit:<int> - Number of old ReplicaSets to retain (e.g., "RevisionHistoryLimit:12")
  //
  // +listType=atomic
  // +kubebuilder:validation:MinItems:=0
  // +kubebuilder:validation:MaxItems:=50
  // +kubebuilder:validation:XValidation:rule="self.all(x, x.matches('^RevisionHistoryLimit:\\d+$'))",message="Only deployment-level overrides for RevisionHistoryLimit are supported, which must be followed by a non-negative integer (e.g., RevisionHistoryLimit:5)."
  // +kubebuilder:validation:Optional
  // +optional
  OverrideArgs []string `json:"overrideArgs,omitempty"`
}

type ControllerConfig struct {
    // ... existing fields ...

    // annotations allows adding custom annotations to all external-secrets component
    // Deployments and Pod templates. These annotations are applied globally to all
    // operand components (Controller, Webhook, CertController, BitwardenSDKServer).
    // These annotations are merged with any default annotations set by the operator.
    // User-specified annotations take precedence over defaults in case of conflicts.
    //
    // +kubebuilder:validation:Optional
    // +optional
    Annotations map[string]string `json:"annotations,omitempty"`

    // componentConfig allows specifying component-specific configuration overrides
    // for individual components (Controller, Webhook, CertController, Bitwarden).
    // +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.componentName == y.componentName))",message="componentName must be unique across all componentConfig entries"
    // +kubebuilder:validation:MinItems:=0
    // +kubebuilder:validation:MaxItems:=4
    // +kubebuilder:validation:Optional
    // +listType=map
    // +listMapKey=componentName
    ComponentConfig []ComponentConfig `json:"componentConfig,omitempty"`
}
```

#### Example User Configuration

**Configure RevisionHistoryLimit for the Controller:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    componentConfig:
      - componentName: ExternalSecretsCoreController
        overrideArgs:
          - "RevisionHistoryLimit:5"
```

**Add custom annotations (applied to all components):**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    annotations:
      example.com/custom-annotation: "value"
```

**Combined: annotations (global) with component-specific overrideArgs:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    # Annotations applied to ALL components
    annotations:
      example.com/custom-annotation: "value"
    # Component-specific overrides
    componentConfig:
      - componentName: ExternalSecretsCoreController
        overrideArgs:
          - "RevisionHistoryLimit:10"
      - componentName: Webhook
        overrideArgs:
          - "RevisionHistoryLimit:3"
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

None

#### Standalone Clusters

None

#### Single-node Deployments or MicroShift

None

### Risks and Mitigations

* **Risk:** The primary risk lies in administrators setting the RevisionHistoryLimit value too low (for example, setting it to 0 or 1). Doing so severely limits or completely eliminates the component's ability to perform quick rollbacks to previous stable versions. If a new deployment fails, recovery will be slower and more complex if there are no historical ReplicaSets to instantly switch back to.
    * **Mitigation:** strongly recommend a safe minimum value (typically between 3 and 5) to ensure operational continuity and maintain reasonable rollback capabilities.

* **Risk:** Users may accidentally override critical arguments required for proper operation.
    * **Mitigation:** The operator will protect certain critical arguments from being overridden and will log warnings if users attempt to do so.

* **Risk:** Configuration changes may cause service disruption during rollout.
    * **Mitigation:** Standard Kubernetes rolling update strategies will minimize disruption. Users can control rollout behavior through the deployment's update strategy.

### Drawbacks

- Increased API surface complexity for users who don't need customization.
- Potential for misconfiguration leading to operational issues.

## Test Plan

* **Unit Tests:**
    1. Test validation of componentName uniqueness.
    2. Test validation of argument count limits (max 50).
    3. Test parsing of deployment-level overrides (e.g., "RevisionHistoryLimit:5").
    4. Test that invalid override formats are handled gracefully.
    5. Test annotation merging logic with defaults and user overrides.

* **Integration Tests:**
    1. Deploy the operator and create an `ExternalSecretsConfig` with component configuration.
    2. Verify that "RevisionHistoryLimit:X" is correctly applied to the deployment's `spec.revisionHistoryLimit`.
    3. Verify that specified annotations appear on both Deployment and Pod template.
    4. Update the configuration and verify the deployment is updated accordingly.
    5. Remove the configuration and verify defaults are restored.
    6. Attempt to apply a configuration that fails XValidation and verify the API server rejects the resource with the appropriate error message.
    7. Test annotation override behavior when user annotation conflicts with operator default.

* **End-to-End (E2E) Tests:**
    1. Test each component type (Controller, Webhook, CertController, BitwardenSDKServer) individually.
    2. Configure `RevisionHistoryLimit:X` and verify old ReplicaSets are cleaned up accordingly.
    3. Verify that the operator correctly handles invalid configurations gracefully.

## Graduation Criteria

This feature will be delivered as GA directly, as it uses stable Kubernetes APIs and provides essential operational flexibility.

* All API fields are implemented with proper validation.
* Argument merging logic is complete.
* Annotation merging logic is complete and applies to both Deployment and Pod template.
* All tests outlined in the Test Plan are passing.
* Documentation includes examples for common use cases.

### Dev Preview -> Tech Preview

Not applicable. This feature will be enabled by default at GA.

### Tech Preview -> GA

Not applicable. This feature will be enabled by default at GA.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

* **Upgrade:** On upgrade, the new `annotations` and `componentConfig` fields will be available. Existing installations without these configurations will continue to work with default settings. Users can optionally add annotations and component configurations after upgrade.

* **Downgrade:** If a user downgrades to a version that doesn't support `annotations` or `componentConfig`, these fields will be ignored by the older operator. Deployments will revert to default configurations, and custom annotations will be removed. Users should be aware that custom configurations will be lost on downgrade.

## Alternatives (Not Implemented)

* **Validating Webhook for Argument Semantics:** A validating admission webhook could be implemented to perform semantic validation of override values against upstream external-secrets component schemas. This would provide pre-flight validation of override keys and semantic values, enabling the early rejection of invalid configurations before deployment rollout, and offering user-friendly error messages. This could be reconsidered in future iterations if runtime validation failures (due to invalid values for supported keys) become a significant operational burden.

## Version Skew Strategy

NA

## Operational Aspects of API Extensions

The `annotations` and `componentConfig` API extensions follow standard Kubernetes patterns:

* **Failure Modes:** Invalid configurations will be rejected by the API server validation. Invalid annotation formats will be rejected at the API level. Runtime failures (e.g., invalid arguments causing pod crashes) will be visible through standard pod status and events.

* **Support Procedures:** Administrators can verify the applied configuration by inspecting the deployment specs and comparing them to the `ExternalSecretsConfig` resource. Custom annotations can be verified on both Deployment and Pod template metadata.

## Support Procedures

Support personnel debugging configuration issues should:

1. Verify the `ExternalSecretsConfig` resource: `oc get externalsecretconfigs cluster -o yaml`
2. Compare the deployment spec to the expected configuration: `oc get deployment external-secrets -n external-secrets -o yaml`
3. Verify custom annotations are applied to Deployment and Pod template: check `.metadata.annotations` and `.spec.template.metadata.annotations` in the deployment spec.
4. Check pod logs for argument parsing errors.
5. Review events for the deployment: `oc get events -n external-secrets`
6. If a pod is failing to start due to invalid arguments, check the container's termination message and logs.
