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
last-updated: 2025-12-15
tracking-link:
  - https://issues.redhat.com/browse/RFE-7842
  - https://issues.redhat.com/browse/OCPSTRAT-2419
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

The External Secrets Operator for Red Hat OpenShift provides limited configuration options via its `ExternalSecretsConfig` API, constraining user customization. This enhancement proposes extending the `ExternalSecretsConfig` API to allow comprehensive customization of the external-secrets deployment. The extended configuration options—including annotations, environment variables, and deployment/pod specifications will be available for all core components (Controller, Webhook, CertController, BitwardenSDKServer). This change provides administrators with greater control over the resource management and operational parameters of each component.

## Motivation

Administrators often need to control core operational parameters, lifecycle settings, and custom metadata without directly modifying the underlying operator-managed resources. Currently, any manual changes made directly to the operand resources or other component specifications are immediately overwritten by the operator, making persistent customization impossible. This hardening forces users to accept default settings that may not be optimal for their workloads. This proposal resolves this issue by providing a dedicated, supported configuration path through `ExternalSecretsConfig`, granting administrators the necessary flexibility to fine-tune essential specifications like revisionHistoryLimit, add crucial environment variables, and apply additional metadata for seamless and efficient integration into complex cluster environments.

### User Stories

- As an OpenShift administrator, I want to configure the deployment lifecycle properties (e.g., revisionHistoryLimit) for external-secrets operand components using the `ExternalSecretsConfig` API so that I can control their rollback behavior and optimize cluster resource consumption.
- As an OpenShift Administrator, I need to apply unique configuration overrides (e.g., revisionHistoryLimit) to individual external-secrets components (Controller, Webhook, etc.) so that I can meet the specific operational and resource requirements of each component efficiently.
- As an OpenShift Administrator, I want to define custom metadata (like annotations or labels) on the external-secrets component deployments via the `ExternalSecretsConfig` API so that the deployments correctly integrate with cluster policy tools, monitoring systems (e.g., Prometheus), and internal tooling without being overwritten.
- As an OpenShift Administrator, I need to set custom environment variables for specific components (e.g., the Controller) so that I can configure component behavior at runtime or securely integrate the operand with necessary external services.

### Goals

- Provide a declarative API for specifying deployment lifecycle overrides for each component via `ExternalSecretsConfig`.
- Provide a declarative API for adding custom annotations globally to all resources created for the `external-secrets` operand via `ExternalSecretsConfig`.
- Provide a declarative API for specifying custom environment variables for each component via `ExternalSecretsConfig`.
- Support all four operand components: Controller, Webhook, CertController, and BitwardenSDKServer.

### Non-Goals

- Exhaustive validation of individual configured values. Users should consult upstream documentation. Only basic structural validation (non-empty strings, list length limits) will be performed.
- Resource limits, replica counts, or other deployment-level settings except for the RevisionHistoryLimit which is specifically introduced by this proposal.

## Proposal

Extend the ExternalSecretsConfig API with:
1. A new `annotations` field for adding custom annotations globally to Deployments and Pod templates.
2. A new `componentConfig` field for per-component deployment lifecycle overrides.

### Workflow Description

**For Global Annotations:**

1. **User Configuration:** Administrator updates the `ExternalSecretsConfig` CR with the `controllerConfig.annotations` field containing custom key-value pairs.
2. **Validation:** The operator validates that annotation keys and values conform to Kubernetes annotation constraints.
3. **Reconciliation:** The operator merges user-specified annotations with any default annotations. User annotations take precedence in case of conflicts. Annotations are applied to both the Deployment metadata and Pod template metadata for all components.
4. **Rollout:** Kubernetes detects the annotation changes and performs updates as needed.

**For Component Configuration:**

1. **User Configuration:** Administrator updates the `ExternalSecretsConfig` CR, utilizing the new `componentConfig` list to specify configuration entries for a component (Controller, Webhook, etc.). This includes deployment-level overrides via `DeploymentConfig` and custom environment variables via `overrideEnv`.
2. **Validation:** It verifies the `componentName` against the allowed enum values and enforces uniqueness across the list. It strictly validates the `DeploymentConfig` field using the provided Kubernetes CEL validation rules, ensuring every entry uses the specified format. For `overrideEnv`, it validates that environment variable names and values conform to Kubernetes conventions.
3. **Reconciliation:** The operator applies the `deploymentConfigs` values (e.g., `revisionHistoryLimit`) directly to the component's underlying Kubernetes Deployment resource spec. For `overrideEnv`, the operator merges user-specified environment variables with default variables, with user values taking precedence in case of conflicts.
4. **Rollout:** Kubernetes detects the change in the Deployment's spec and performs a rolling update, applying the new setting to the component.

### Implementation Details/Notes/Constraints

### API Extensions

```go
type ComponentConfig struct {
    // componentName specifies which deployment component this configuration applies to.
    // Allowed values: Controller, Webhook, CertController, Bitwarden
    // +kubebuilder:validation:Enum:=ExternalSecretsCoreController;Webhook;CertController;BitwardenSDKServer
    // +kubebuilder:validation:Required
    ComponentName ComponentName `json:"componentName"`
    
    // deploymentConfigs allows specifying deployment-level configuration overrides.
    // +kubebuilder:validation:Optional
    // +optional
    DeploymentConfigs DeploymentConfig `json:"deploymentConfigs,omitempty"`
    
    // overrideEnv allows setting custom environment variables for the component's container.
    // These environment variables are merged with the default environment variables set by
    // the operator. User-specified variables take precedence in case of conflicts.
    // Environment variables starting with HOSTNAME, KUBERNETES_, or EXTERNAL_SECRETS_ are reserved
    // and cannot be overridden.
    //
    // +kubebuilder:validation:Optional
    // +kubebuilder:validation:XValidation:rule="self.all(e, !e.name.startsWith('HOSTNAME') && !e.name.startsWith('KUBERNETES_') && !e.name.startsWith('EXTERNAL_SECRETS_'))",message="Environment variable names cannot start with 'HOSTNAME', 'KUBERNETES_', or 'EXTERNAL_SECRETS_' as these are reserved"
    // +optional
    OverrideEnv []corev1.EnvVar `json:"overrideEnv,omitempty"`
}

type ControllerConfig struct {
    // annotations allows adding custom annotations to all external-secrets component
    // Deployments and Pod templates. These annotations are applied globally to all
    // operand components (Controller, Webhook, CertController, BitwardenSDKServer).
    // These annotations are merged with any default annotations set by the operator.
    // User-specified annotations take precedence over defaults in case of conflicts.
    // Annotations with keys starting with kubernetes.io, app.kubernetes, openshift.io, or k8s.io
    // are reserved and cannot be overridden.
    //
    // +kubebuilder:validation:Optional
    // +kubebuilder:validation:XValidation:rule="self.all(key, !key.startsWith('kubernetes.io') && !key.startsWith('app.kubernetes') && !key.startsWith('openshift.io') && !key.startsWith('k8s.io'))",message="Annotation keys cannot start with 'kubernetes.io', 'app.kubernetes', 'openshift.io', or 'k8s.io' as these are reserved"
    // +optional
    Annotations map[string]string `json:"annotations,omitempty"`
	
    // componentConfigs allows specifying component-specific (Controller, Webhook, CertController, Bitwarden) configuration overrides.
    // +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.componentName == y.componentName))",message="componentName must be unique across all componentConfig entries"
    // +kubebuilder:validation:MinItems:=0
    // +kubebuilder:validation:MaxItems:=4
    // +kubebuilder:validation:Optional
    // +listType=map
    // +listMapKey=componentName
    ComponentConfigs []ComponentConfig `json:"componentConfig,omitempty"`
}

type DeploymentConfig struct {
  // revisionHistoryLimit specifies the number of old ReplicaSets to retain for rollback.
  // Minimum value of 2 is enforced to ensure rollback capability.
  //
  // +kubebuilder:validation:Minimum=2
  // +kubebuilder:validation:Optional
  // +optional
  RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
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
        deploymentConfigs:
          revisionHistoryLimit: 5
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

**Set custom environment variables for a component:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    componentConfig:
      - componentName: ExternalSecretsCoreController
        overrideEnv:
          - name: GOMAXPROCS
            value: "4"
```

**Combined: annotations (global) with component-specific deployment config and overrideEnv:**

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
        deploymentConfigs:
          revisionHistoryLimit: 10
        overrideEnv:
          - name: GOMAXPROCS
            value: "4"
      - componentName: Webhook
        deploymentConfigs:
          revisionHistoryLimit: 3
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
    * **Mitigation:** The operator can protect certain critical arguments from being overridden and will log warnings if users attempt to do so.

* **Risk:** Users may override critical environment variables required for proper component operation.
    * **Mitigation:** The operator can protect certain critical environment variables from being overridden and will log warnings if users attempt to do so.

* **Risk:** Configuration changes may cause service disruption during rollout.
    * **Mitigation:** Standard Kubernetes rolling update strategies will minimize disruption. Users can control rollout behavior through the deployment's update strategy.

### Drawbacks

- Increased API surface complexity for users who don't need customization.
- Potential for misconfiguration leading to operational issues.

## Test Plan

* **Unit Tests:**
    1. Test validation of componentName uniqueness.
    2. Test validation of `deploymentConfigs.revisionHistoryLimit` values.
    3. Test that invalid `deploymentConfigs` values are handled gracefully.
    4. Test annotation merging logic with defaults and user overrides.
    5. Test that reserved annotation prefixes are rejected.
    6. Test environment variable merging logic with defaults and user overrides.
    7. Test that reserved environment variable prefixes are rejected.
    8. Test that environment variable names conform to Kubernetes conventions.

* **Integration Tests:**
    1. Deploy the operator and create an `ExternalSecretsConfig` with component configuration.
    2. Verify that `deploymentConfigs.revisionHistoryLimit` is correctly applied to the deployment's `spec.revisionHistoryLimit`.
    3. Verify that specified annotations appear on both Deployment and Pod template.
    4. Verify that specified environment variables appear in the container spec.
    5. Update the configuration and verify the deployment is updated accordingly.
    6. Remove the configuration and verify defaults are restored.
    7. Attempt to apply a configuration that fails XValidation and verify the API server rejects the resource with the appropriate error message.
    8. Test annotation override behavior when user annotation conflicts with operator default.
    9. Test environment variable override behavior when user variable conflicts with operator default.

* **End-to-End (E2E) Tests:**
    1. Test each component type (Controller, Webhook, CertController, BitwardenSDKServer) individually.
    2. Configure `deploymentConfigs.revisionHistoryLimit` and verify old ReplicaSets are cleaned up accordingly.
    3. Configure custom environment variables and verify they are available in the running container.
    4. Verify that the operator correctly handles invalid configurations gracefully.

## Graduation Criteria

This feature will be delivered as GA directly, as it uses stable Kubernetes APIs and provides essential operational flexibility.

* All API fields are implemented with proper validation.
* Deployment config application logic is complete (e.g., `revisionHistoryLimit`).
* Annotation merging logic is complete and applies to both Deployment and Pod template.
* Environment variable merging logic is complete and applies to the container spec.
* All tests outlined in the Test Plan are passing.
* Documentation includes examples for common use cases.

### Dev Preview -> Tech Preview

Not applicable. This feature will be enabled by default at GA.

### Tech Preview -> GA

Not applicable. This feature will be enabled by default at GA.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

* **Upgrade:** On upgrade, the new `annotations` and `componentConfig` fields (including `deploymentConfigs` and `overrideEnv`) will be available. Existing installations without these configurations will continue to work with default settings. Users can optionally add annotations, deployment overrides, and custom environment variables after upgrade.

* **Downgrade:** If a user downgrades to a version that doesn't support `annotations` or `componentConfig`, these fields will be ignored by the older operator. Deployments will revert to default configurations, custom annotations will be removed, and custom environment variables will be reset. Users should be aware that custom configurations will be lost on downgrade.

## Alternatives (Not Implemented)

* **Validating Webhook for Argument Semantics:** A validating admission webhook could be implemented to perform semantic validation of override values against upstream external-secrets component schemas. This would provide pre-flight validation of override keys and semantic values, enabling the early rejection of invalid configurations before deployment rollout, and offering user-friendly error messages. This could be reconsidered in future iterations if runtime validation failures (due to invalid values for supported keys) become a significant operational burden.

## Version Skew Strategy

NA

## Operational Aspects of API Extensions

The `annotations` and `componentConfig` API extensions follow standard Kubernetes patterns:

* **Failure Modes:** Invalid configurations will be rejected by the API server validation. Invalid annotation formats will be rejected at the API level. Invalid environment variable names will be rejected at the API level.

* **Support Procedures:** Administrators can verify the applied configuration by inspecting the deployment specs and comparing them to the `ExternalSecretsConfig` resource. Custom annotations can be verified on both Deployment and Pod template metadata. Custom environment variables can be verified in the container spec of the Deployment.

## Support Procedures

Support personnel debugging configuration issues should:

1. Verify the `ExternalSecretsConfig` resource: `oc get externalsecretconfigs cluster -o yaml`
2. Compare the deployment spec to the expected configuration: `oc get deployment external-secrets -n external-secrets -o yaml`
3. Verify custom annotations are applied to Deployment and Pod template: check `.metadata.annotations` and `.spec.template.metadata.annotations` in the deployment spec.
4. Verify custom environment variables are applied to containers: check `.spec.template.spec.containers[*].env` in the deployment spec.
5. Check pod logs for argument parsing errors or environment variable issues.
6. Review events for the deployment: `oc get events -n external-secrets`
7. If a pod is failing to start due to invalid arguments or environment variables, check the container's termination message and logs.
