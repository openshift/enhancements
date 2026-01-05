---
title: olmv1-deployment-configuration-api
authors:
  - oceanc80
  - perdasilva
  - anbhatta
reviewers:
  - joelanford
approvers:
  - joelanford
api-approvers:
  - everettraven
creation-date: 2025-12-30
last-updated: 2025-12-30
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2305
---

# OLMv1: Deployment Configuration API

## Summary

This enhancement extends OLMv1's ClusterExtension API to support operator deployment customization through the configuration API. This provides feature parity with OLMv0's `SubscriptionConfig`, enabling users to configure resource limits, pod placement, environment variables, storage, and metadata annotations for operators installed via registry+v1 bundles.

## Motivation

OLMv0 provides users with deployment customization capabilities via the `Subscription.spec.config` field. This allows critical modifications to operator deployment behavior including node selectors, tolerations, resource requirements, environment variables, volumes, and affinity rules. OLMv1 currently lacks this functionality, creating a feature gap that prevents users from performing advanced customizations required for production-grade operator deployments.

Without deployment configuration support, users cannot:
- Schedule operators on dedicated infrastructure nodes
- Configure resource limits for operators
- Add environment-specific configuration through environment variables
- Attach custom storage volumes to operator pods
- Control pod placement using affinity and anti-affinity rules
- Add custom annotations to operator deployments and pods

This gap blocks migration from OLMv0 to OLMv1 for operators that require or whose user-base makes frequent use of customizations.

### User Stories

- As a cluster extension admin, I want to schedule operator pods on dedicated infrastructure nodes using node selectors and tolerations, so that I can isolate operator workloads from application workloads.
- As a cluster extension admin, I want to configure resource limits for operator pods, so that I can prevent operators from consuming excessive cluster resources.
- As a cluster extension admin, I want to configure environment variables for my operator deployment, so that I can customize operator behavior for different deployment contexts without rebuilding container images.
- As a cluster extension admin, I want to attach custom storage volumes to operator pods, so that I can provide persistent storage or configuration files to operators.
- As a cluster extension admin, I want to configure pod affinity rules for operator deployments, so that I can control how operator pods are distributed across cluster nodes.
- As a cluster extension admin, I want to add custom annotations to operator deployments, so that I can integrate with monitoring and observability tools.

### Goals

- Achieve feature parity with OLMv0's `SubscriptionConfig` for deployment customization
- Extend the ClusterExtension inline configuration to support a `deploymentConfig` field
- Support all deployment customization options available in OLMv0: nodeSelector, tolerations, resources, env, envFrom, volumes, volumeMounts, affinity, and annotations
- Provide JSON schema-based validation for deployment configuration
- Ensure deployment configuration is applied during bundle rendering
- Maintain the same merge and override semantics as OLMv0

### Non-Goals

- Introducing new configuration fields beyond those present in OLMv0's `SubscriptionConfig`
- Redesigning the OLMv1 renderer's core architecture; it is an additive change
- Supporting deployment configuration for bundle formats other than registry+v1

## Proposal

This proposal extends the existing ClusterExtension inline configuration structure to support deployment customization by adding a new `deploymentConfig` field that follows the same structure as [OLMv0's](https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/subscription_types.go#L42-L100) `SubscriptionConfig` and incorporates the OpenAPI specification from Kubernetes core v1 types. This will help OLMv1 maintain feature parity with OLMv0 for Deployment configuration without importing any OLMv0 packages and keeping OLMv1 self contained.

### Workflow Description

**cluster extension admin** is a user responsible for configuring and managing cluster extensions.

1. The cluster extension admin creates a ClusterExtension resource with inline configuration
2. The cluster extension admin specifies deployment customization in the `deploymentConfig` field
3. The ClusterExtension controller extracts and validates the `deploymentConfig` from the inline configuration
4. The controller passes the `deploymentConfig` to the bundle renderer during rendering
5. The renderer applies the deployment configuration to all Deployment resources generated from the bundle
6. The customized Deployment resources are applied to the cluster
7. The cluster extension admin can verify the deployment configuration by inspecting the running Deployments

#### Validation Failure

If the deployment configuration fails JSON schema validation:

1. The ClusterExtension controller rejects the configuration during admission or runtime validation
2. The ClusterExtension status is updated with a detailed error message indicating which fields failed validation
3. The cluster extension admin corrects the configuration based on the error message
4. The controller retries the installation with the corrected configuration

### API Extensions

The enhancement does not introduce new APIs, CRDs, webhooks, or aggregated API servers. As the inline configuration structure in the ClusterExtension API accepts any valid JSON object, the API will not be changed. This enhancement modifies the existing configuration schema for registry+v1 bundles to accept a `deploymentConfig` field. 
The configuration is validated using JSON schema generated from Kubernetes core v1 and apps v1 OpenAPI specifications.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement applies to operator deployments that run on the hosted cluster (guest cluster). The deployment configuration affects only the operator pods running in the guest cluster and has no impact on components in the management cluster.

Operators installed via ClusterExtension on hosted clusters can use deployment configuration to:
- Schedule operator pods on specific node pools in the guest cluster
- Configure resource requirements appropriate for the guest cluster's node capacity
- Add environment variables specific to the hosted cluster environment

#### Standalone Clusters

This enhancement is fully applicable to standalone OpenShift clusters and provides the same deployment customization capabilities as OLMv0.

#### Single-node Deployments or MicroShift

For single-node OpenShift (SNO) deployments, deployment configuration can be used to:
- Configure resource limits to prevent operators from consuming excessive resources on the single node
- Add environment variables to tune operator behavior for resource-constrained environments

For MicroShift, this enhancement applies to any operators that are installed via OLMv1. The deployment configuration provides the same customization capabilities, though node selector and affinity configuration may have limited utility in single-node scenarios.

### Implementation Details/Notes/Constraints

#### registry+v1 Bundle Configuration Schema Design

The registry+v1 bundle configuration will support a new `deploymentConfig` field that follows the same structure as OLMv0's `SubscriptionConfig`:

```go
// DeploymentConfig contains configuration specified for a
// ClusterExtension that follows the same structure and behavior as OLM
// v0's SubscriptionConfig.
//
// This enables v0-style deployment customization including environment variables,
// resource scheduling, storage, and pod placement for registry+v1 bundle installations.
type DeploymentConfig struct {
    // selector is the label selector for pods to be configured.
    // Existing ReplicaSets whose pods are selected by this will be the ones affected by this deployment.
    // It must match the pod template's labels.
    //
    // +optional
    Selector *metav1.LabelSelector `json:"selector,omitempty"`

    // nodeSelector is a selector which must be true for the pod to fit on a node.
    // Selector which must match a node's labels for the pod to be scheduled on that node.
    // More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
    //
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`

    // tolerations are the pod's tolerations.
    //
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

    // resources represents compute resources required by this container.
    // More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
    //
    // +optional
    Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

    // envFrom is a list of sources to populate environment variables in the container.
    // The keys defined within a source must be a C_IDENTIFIER. All invalid keys
    // will be reported as an event when the container is starting. When a key exists in multiple
    // sources, the value associated with the last source will take precedence.
    // Values defined by an Env with a duplicate key will take precedence.
    //
    // +optional
    EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

    // env is a list of environment variables to set in the container.
    // Cannot be updated.
    //
    // +patchMergeKey=name
    // +patchStrategy=merge
    // +optional
    Env []corev1.EnvVar `json:"env,omitempty" patchMergeKey:"name" patchStrategy:"merge"`

    // volumes is a list of Volumes to set in the podSpec.
    //
    // +optional
    Volumes []corev1.Volume `json:"volumes,omitempty"`

    // volumeMounts is a list of VolumeMounts to set in the container.
    //
    // +optional
    VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

    // affinity, if specified, overrides the pod's scheduling constraints.
    // nil sub-attributes will *not* override the original values in the pod.spec for those sub-attributes.
    // Use empty object ({}) to erase original sub-attribute values.
    //
    // +optional
    Affinity *corev1.Affinity `json:"affinity,omitempty"`

    // annotations is an unstructured key value map stored with each Deployment, Pod, APIService in the Operator.
    // Typically, annotations may be set by external tools to store and retrieve arbitrary metadata.
    // Use this field to pre-define annotations that OLM should add to each of the ClusterExtension's
    // deployments, pods, and apiservices.
    //
    // +optional
    Annotations map[string]string `json:"annotations,omitempty"`
}
```

Example ClusterExtension with deployment configuration:

```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: my-operator
spec:
  namespace: my-namespace
  serviceAccount:
    name: my-operator-sa
  source:
    sourceType: Catalog
    catalog:
      packageName: my-operator
  install:
    namespace: my-namespace
    serviceAccount:
      name: my-operator-sa
  config:
    inline:
      watchNamespace: "my-namespace"
      deploymentConfig:
        # Schedule pods on specific nodes
        nodeSelector:
          infrastructure: "dedicated"
        # Add tolerations for dedicated nodes
        tolerations:
          - key: "dedicated"
            operator: "Equal"
            value: "operators"
            effect: "NoSchedule"
        # Set resource requirements
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "200m"
        # Add environment variables
        env:
          - name: LOG_LEVEL
            value: "debug"
        # Add custom annotations
        annotations:
          monitoring.io/scrape: "true"
```

#### Renderer Modifications

The OLMv1 bundle renderer will be extended to accept and apply deployment configuration during the rendering process. For each configuration type (environment variables, resource limits, tolerations, etc.), functions to apply the configuration will be implemented that replicate [OLMv0's behavior](https://github.com/operator-framework/operator-lifecycle-manager/blob/d55d4899c17db9caeb90aac2ec86d5c82651593a/pkg/controller/operators/olm/overrides/inject/inject.go). 

The merge and override policies will also match OLMv0:

- **Resources & NodeSelector**: Complete replacement of existing values
- **Environment Variables**: Merge, with DeploymentConfig values overriding existing container variables of the same name
- **Tolerations & EnvFrom**: Append new values to existing values
- **Affinity**: Selective override of non-nil attributes
- **Annotations**: Merge, with existing annotations taking precedence over DeploymentConfig annotations

#### JSON Schema Validation and Controller Integration

The inline configuration will be validated using JSON schema-based validation. The JSON schema for `DeploymentConfig` will be based on a static snapshot of the Kubernetes apps v1 and core v1 OpenAPI specifications. This approach:

- Provides stability: The schema will not change unexpectedly with Kubernetes updates
- Ensures compatibility: Operators using current Kubernetes API types will continue to work
- Simplifies maintenance: No need for dynamic schema generation based on go.mod updates
- Is safe for the foreseeable future: Breaking changes are unlikely until apps v2 or core v2 API groups are introduced

When Kubernetes introduces new API versions, the schema can be updated deliberately as part of a planned OLMv1 enhancement.
Validation errors should provide clear user feedback that indicates the source of the error and why it is invalid.

The ClusterExtension controller will be updated to validate and extract deployment configuration from inline configuration, and pass it to the renderer.

#### Documentation

User-facing documentation and examples should be provided. The following topics should be covered:

- User guide for `deploymentConfig` usage
- Migration guide from OLMv0 `SubscriptionConfig` to OLMv1 `deploymentConfig`
- API reference documentation
- Example configurations/manifests

### Risks and Mitigations

**Risk**: Deployment configuration could conflict with operator-defined deployment specifications, leading to unexpected behavior.

**Mitigation**: Document the merge and override semantics clearly. Follow OLMv0 precedent for handling conflicts. Provide clear examples and migration guidance.

**Risk**: Schema validation may become outdated as new Kubernetes fields are added to core v1 and apps v1 types.

**Mitigation**: Use a static schema snapshot that is updated deliberately with OLMv1 releases. Monitor Kubernetes API changes and update the schema when necessary. Document the supported Kubernetes API version for deployment configuration.

**Risk**: Incorrect deployment configuration could cause operator pods to fail scheduling or startup or could cause performance issues.

**Mitigation**: Provide comprehensive validation through JSON schema. Surface validation errors clearly in ClusterExtension status. Provide documentation with working examples for common use cases and link to best practices (e.g. etcd should have a dedicated volume).

**Risk**: Users may set resource limits that are too restrictive, causing operator pods to crash or be OOMKilled.

**Mitigation**: Document best practices for setting resource limits. Recommend testing deployment configuration in non-production environments first. Provide guidance on monitoring operator pod resource usage.

### Drawbacks

- Adds complexity to the ClusterExtension inline configuration structure
- Requires maintaining JSON schema definitions based on Kubernetes core types
- Users must understand Kubernetes pod scheduling and resource management concepts to use effectively
- May encourage users to over-customize operator deployments, making troubleshooting more difficult

## Alternatives (Not Implemented)

### Alternative 1: Separate DeploymentConfig CRD

Instead of embedding deployment configuration in the inline configuration, introduce a separate `DeploymentConfig` CRD that references the ClusterExtension.

**Pros**:
- Cleaner separation of concerns
- Could be reused across multiple ClusterExtensions

**Cons**:
- Increases API surface area
- Adds complexity with additional resource lifecycle management
- Deviates from OLMv0 model where configuration is part of the Subscription
- Makes it harder to see the complete configuration in one place

**Rejected**: This approach increases complexity without clear benefits. The inline configuration model aligns with OLMv0 and keeps configuration co-located with the ClusterExtension.

### Alternative 2: Use Kustomize or Helm for Post-Processing

Allow users to apply Kustomize overlays or Helm post-renderers to modify generated Deployments.

**Pros**:
- Leverages existing tools that users may already be familiar with
- Very flexible customization model

**Cons**:
- Significantly more complex for users
- Requires external tooling and additional configuration
- Does not provide feature parity with OLMv0
- Makes it difficult to validate configuration at admission time

**Rejected**: This approach is too complex for the common use cases that OLMv0's SubscriptionConfig handles well. It would not provide a migration path from OLMv0.

### Alternative 3: Do Nothing

Keep the status quo and do not provide deployment configuration in OLMv1.

**Pros**:
- No additional implementation or maintenance burden

**Cons**:
- Blocks migration from OLMv0 to OLMv1 for operators requiring deployment customization
- Forces users to use workarounds like manual patching of Deployments
- Creates feature gap between OLMv0 and OLMv1

**Rejected**: This is not viable as current operator products rely on this functionality in OLMv0 and require it to migrate to OLMv1.

## Open Questions / Considerations

### Track changes to underlying kubernetes corev1 structures?
SubscriptionConfig uses many kubernetes corev1 structures from the standard kube lib. This means that the OLMv0 Subscription API would track changes to those structures (e.g. if a new Volume type is added to the API etc.). We need to think about whether we want the same behavior here, and if so how we’d like to implement it. E.g. we could have some process downloading and mining the openapi specs for the given kube lib version we have in go.mod, and having make verify fail when that changes. We’d want to think about how we’d handle any CEL expressions in those corev1 structures when doing the validation (and whether we want to handle them?).

#### Proposed Response
As these structures should change very rarely, we should use the latest definition of these structures and only update if there's a clear user ask. Ultimately, the goal for OLMv1 is to have Cluster Extension Authors define their own bundle configuration surface. Therefore, the extra complexity of building and maintaing a mechanism to automatically track and update these definitions is probably not warranted without clear customer demands.


## Test Plan

**Regression Tests**:
- Regression tests to ensure consistent rendering of bundle artifacts for different configurations

## Graduation Criteria

### Dev Preview -> Tech Preview

- Feature-gated implementation
- Ability to utilize deployment configuration end to end
- Documentation for configuration and usage
- Sufficient test coverage (unit, integration, and e2e tests)
- Verify feature parity with OLMv0 SubscriptionConfig

### Tech Preview -> GA

- More testing including upgrade and downgrade scenarios
- Sufficient time for feedback from Tech Preview users
- Production deployment validation
- End-user documentation including best practices and examples for common scenarios
- Address any issues found during Tech Preview

## Upgrade / Downgrade Strategy

### Upgrade

### Downgrade

## Version Skew Strategy

## Operational Aspects of API Extensions

This enhancement does not introduce new API extensions (webhooks, finalizers, aggregated API servers). It extends the existing ClusterExtension inline configuration structure, which is already validated at the API server level.

### Impact on Existing SLIs

- API throughput: Minimal impact. JSON schema validation adds negligible overhead during ClusterExtension creation/update
- Rendering time: Minimal increase due to applying deployment configuration to each Deployment.
- Resource usage: No significant impact. Deployment configuration is stored as part of the ClusterExtension inline configuration

### Failure Modes

- Invalid deployment configuration is provided

**Runtime Issues**:
- Incorrect deployment configuration is provided, e.g. node selector, tolerations or affinity rules don't match available nodes
- Resource limits are too restrictive for actual operator requirements, causing operator to crash

#### OCP Teams Likely to be Called Upon in Case of Escalation
1. OLM Team (primary)
2. Layered Product Team

## Support Procedures