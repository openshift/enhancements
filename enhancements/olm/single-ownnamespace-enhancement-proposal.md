# OLM v1 Single and OwnNamespace Install Mode Support

title: single-ownnamespace-enhancement-proposal
authors:
  - anbhatta
reviewers:
  - perdasilva
  - joelanford
approvers:
  - joelanford
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - JoelSpeed
  - everettraven 
creation-date: 2025-09-19
last-updated: 2025-10-21
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OPRUN-4133


## Summary

This enhancement proposes adding limited compatibility support for operator bundles that only declare `Single` and `OwnNamespace` install modes. These bundles are shipped in OpenShift within existing operator catalogs and contain installation bundles installable with OLM v0 (registry+v1 format). This feature enables the rendering and installation of registry+v1 bundles that declare only namespace-scoped install modes, providing a migration path from OLM v0 while maintaining OLM v1's stance on NOT supporting OLMv0's multi-tenancy model.

The enhancement introduces a configuration mechanism through the ClusterExtension API that allows users to specify a `watchNamespace` parameter as opaque bundle configuration, enabling registry+v1 bundles to be rendered in either SingleNamespace mode (watching a namespace different from the install namespace) or OwnNamespace mode (watching the same namespace where the operator is installed).

## OLM v1 Design Principles and Scope

**This enhancement does NOT change OLM v1's core design principles.** OLM v1 maintains its fundamental stances:

- **No multi-tenancy support**: OLM v1 does not and will not support multi-tenancy features. As stated in the [upstream design decisions](https://operator-framework.github.io/operator-controller/project/olmv1_design_decisions/), "Kubernetes is not multi-tenant with respect to management of APIs (because APIs are global)."
- **No first-class watchNamespace configuration**: OLM v1 does not provide first-class API support for configuring which namespaces a controller watches.
- **Single ownership model**: Each extension is owned by exactly one ClusterExtension to avoid shared ownership complexity.

This enhancement provides **limited compatibility support for existing registry+v1 bundle content only**. The `watchNamespace` configuration is treated as opaque bundle-provided configuration used solely for rendering registry+v1 bundles into appropriate manifests. This is not a multi-tenancy feature but rather a bundle format compatibility layer to support migration from OLM v0 to OLM v1.

Future OLM v1 bundle formats will not include these legacy install mode concepts, as OLM v1 will not have first-class features or opinions on RBAC management in next-generation bundle formats.

## Motivation

The registry+v1 bundle format (used by OLM v0) includes install mode declarations (AllNamespaces, MultiNamespace, SingleNamespace, OwnNamespace) that affect how bundles are rendered into Kubernetes manifests. OLM v1 deliberately simplified operator installation by focusing on AllNamespaces mode to avoid complexity, but there exists significant existing registry+v1 bundle content in catalogs that only declares support for Single and OwnNamespace install modes.

### User Stories

#### Story 1: Legacy Operator Migration
As a cluster administrator migrating from OLM v0 to OLM v1, I want to install operators that only support SingleNamespace or OwnNamespace install modes so that I can continue using existing operator content without requiring operator authors to modify their bundles.

#### Story 2: Operator Author Requirements
As an operator developer, I have existing registry+v1 bundles that only support Single or OwnNamespace install modes, and I want my customers to be able to deploy these operators in OpenShift with OLM v1 so that the bundle content can be properly rendered and installed without requiring me to modify my existing bundle format during the migration to OLM v1.

### Goals

- Enable installation of registry+v1 bundles that only support SingleNamespace or OwnNamespace install modes
- Provide a configuration mechanism through the ClusterExtension API to pass opaque watchNamespace configuration for bundle rendering
- Maintain backward compatibility with existing registry+v1 bundle content from OLM v0 catalogs
- Generate appropriate RBAC resources scoped to the target namespaces for registry+v1 bundles only
- Support seamless migration path for customers moving from OLM v0 to OLM v1

#### Future Goals

- Expand the configuration surface for registry+v1 bundles to support OLMv0 SubscriptionConfig-type behavior, enabling broader operator configuration compatibility during the migration from OLMv0 to OLMv1 

### Non-Goals

- Re-introducing multi-tenancy features or supporting multiple installations of the same operator
- Supporting MultiNamespace install mode (watching multiple namespaces)
- Modifying the fundamental OLM v1 architecture or adding complex multi-tenancy logic
- Enabling this feature for bundle formats other than registry+v1
- Changes to OLM cache infrastructure: OLM will continue to watch bundle contents cluster-scope
- Explicit callout that MultiNamespace is NEVER supported

## Proposal

Update the OLMv1 operator-controller to support rendering registry+v1 bundles with Single and OwnNamespace install modes by:
1. Validate bundle compatibility with the requested install mode during reconciliation
2. Parse the `watchNamespace` configuration from ClusterExtension.spec.config.inline
3. Determine the install mode based on the relationship between install namespace and watch namespace
4. Validate the watchNamespace configuration against the install modes supported by the bundle (see below note for more details)
5. Generate appropriate RBAC resources (Roles/RoleBindings vs ClusterRoles/ClusterRoleBindings) based on the determined install mode, for registry+v1 bundles
6. Configure operator deployments with correct environment variables and RBAC for the target watch namespace

**Specific Resource Changes:**
- **ClusterRole/ClusterRoleBinding**: `clusterPermissions` entries in the CSV are always created as `ClusterRole` and `ClusterRoleBinding` resources regardless of install mode
- **Role/RoleBinding**: `permissions` entries in the CSV are created as `Role` and `RoleBinding` resources in the watch namespace(s). For AllNamespaces mode, these are instead created as `ClusterRole` and `ClusterRoleBinding` resources
- **Operator Configuration**: `olm.targetNamespaces` annotation gets set in the operator deployment's pod template, instructing the operator how to configure itself for the target namespace scope

`Note`: With watch namespace as a configuration value that can (at times, must) be provided for installation, the user input will be validated based on the bundle's supported install modes:

| AllNamespaces | SingleNamespace | OwnNamespace | WatchNamespace Configuration                                     |
|---------------|-----------------|--------------|------------------------------------------------------------------|
| -             | -               | -            | undefined/error (no supported install modes)                     |
| -             | -               | ✓            | no configuration                                                 |
| -             | ✓               | -            | required (must not be install namespace)                         |
| -             | ✓               | ✓            | optional (default: install namespace)                            | 
| ✓             | -               | -            | no configuration                                                 | 
| ✓             | -               | ✓            | optional. If set, must be install namespace (default: unset)     | 
| ✓             | ✓               | -            | optional. If set, must NOT be install namespace (default: unset) | 
| ✓             | ✓               | ✓            | optional (default: unset)                                        | 

Add support for namespace-scoped operation by ensuring:
1. Bundle validation confirms the operator supports the requested install mode
2. RBAC resources are properly scoped to the watch namespace for Single/OwnNamespace modes
3. Operator deployment environment variables are configured for the specified watch namespace
4. Installation fails gracefully with clear error messages when bundles don't support the requested mode
5. Usage of reasonable default install mode when possible: 
  - AllNamespaces by default (when available)
  - OwnNamespace by default (when no AllNamespaces and available)

Ensure parity with OLMv0 behavior by:
1. Generating identical RBAC and deployment configurations for equivalent install modes
2. Maintaining the same namespace isolation boundaries
3. Preserving operator functionality across install mode transitions

### Workflow Description

#### Administrator Workflow

1. **Create ServiceAccount**: Administrator creates a ServiceAccount with appropriate permissions in the target namespace
2. **Configure ClusterExtension**: Administrator creates a ClusterExtension resource specifying:
   - `spec.namespace`: The installation namespace where the operator pod will run
   - `spec.config.inline.watchNamespace`: The namespace the operator should watch for resources 

A scenario exists where the user must specify the watch namespace. Example workflow of a bundle that will require the watch namespace to be specified: 

1. User creates ClusterExtension for a bundle that only supportes SingleNamespace install mode but does not specify the watchNamespace
2. ClusterExtension does not install. The `Installing` and `Progressing` conditions will be set to false with an error that indicates the required `watchNamespace` is not specified
3. User updates the ClusterExtension specifying the `watchNamespace` configuration to be an exiting namespace on the cluster
4. ClusterExtension installs successfully

```
Note: Once a ClusterExtension is already installed, OLM will not prevent the `watchNamespace` parameter from being changed by the admin. OLM will reconcile again with the new parameter, however, whether the operator will then re-install successfully is dependent on the operator itself.  
```

#### Example Configuration

**OwnNamespace Mode**:
```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: argocd
spec:
  namespace: argocd                    # Install namespace
  serviceAccount:
    name: argocd-installer
  config:
    configType: Inline
    inline:
      watchNamespace: argocd           # Same as install namespace = OwnNamespace
  source:
    sourceType: Catalog
    catalog:
      packageName: argocd-operator
```

**SingleNamespace Mode**:
```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: argocd
spec:
  namespace: argocd                    # Install namespace
  serviceAccount:
    name: argocd-installer
  config:
    configType: Inline
    inline:
      watchNamespace: target-namespace # Different namespace = SingleNamespace
  source:
    sourceType: Catalog
    catalog:
      packageName: argocd-operator
```

### API Extensions

The ClusterExtension API will be expanded to contain a discriminated union `.spec.config`:

```
// ClusterExtensionConfig is a discriminated union which selects the source configuration values to be merged into
// the ClusterExtension's rendered manifests.
//
// +kubebuilder:validation:XValidation:rule="has(self.configType) && self.configType == 'Inline' ?has(self.inline) : !has(self.inline)",message="inline is required when configType is Inline, and forbidden otherwise"
// +union
type ClusterExtensionConfig struct {
	// configType is a required reference to the type of configuration source.
	//
	// Allowed values are "Inline"
	//
	// When this field is set to "Inline", the cluster extension configuration is defined inline within the
	// ClusterExtension resource.
	//
	// +unionDiscriminator
	// +kubebuilder:validation:Enum:="Inline"
	// +kubebuilder:validation:Required
	ConfigType ClusterExtensionConfigType `json:"configType"`

	// inline contains JSON or YAML values specified directly in the
	// ClusterExtension.
	//
	// inline must be set if configType is 'Inline'.
	// inline accepts arbitrary JSON/YAML objects.
	// inline is validation at runtime against the schema provided by the bundle if a schema is provided.
	//
	// +kubebuilder:validation:Type=object
	// +optional
	Inline *apiextensionsv1.JSON `json:"inline,omitempty"`
}
```

```
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: argocd
spec:
  namespace: argocd
  serviceAccount:
    name: argocd-installer
  config:
    inline:
      watchNamespace: argocd-pipelines
  source:
    sourceType: Catalog
    catalog:
      packageName: argocd-operator
      version: 0.6.0
```

Initially, only the `Inline` configType will be available. However, we leave it expandable in case further 
configuration sources (e.g. ConfigMaps, Secrets, etc.) become needed.

#### Feature Gate

This feature can be enabled via the feature gate `NewOLMOwnSingleNamespace` when the feature is in tech preview.  

### Topology Considerations

#### Hypershift / Hosted Control Planes

OLMv1 does not yet support Hypershift. Although no aspects of this feature's implementation stands out as at odds with the topology of Hypershift, it should be reviewed when OLMv1 is ready to be supported in Hypershift clusters.

#### Standalone Clusters
- **Full Compatibility**: Complete support for Single/OwnNamespace modes
- **Standard RBAC**: Normal Role/RoleBinding generation for namespace-scoped permissions

#### Single-node Deployments or MicroShift

OLMv1 does not yet support Microshift, but it should be noted that no aspects of this feature's implementation stands out as odds with the topology of Microshift.

### Implementation Details/Notes/Constraints

The ClusterExtension CRD used in the registry+v1 bundle installation process is enhanced to:
- Parse watchNamespace configuration from the inline JSON configuration
- Validate watchNamespace against the bundle's supported install modes
- Map install modes based on namespace relationships (install vs watch namespace)
- Generate appropriate RBAC resources scoped to the target namespace

The system determines the actual install mode based on the bundle's supported install modes and the user's configuration:

  | Bundle Supported Modes | watchNamespace Config    | Selected Install Mode | Rationale |
|-------------------------|--------------------------|-----------------------|-----------|
| AllNamespaces only      | Not specified            | AllNamespaces         | Default behavior, no namespace scoping |
| AllNamespaces only      | Specified                | Error                 | Bundle doesn't support namespace-scoped installation |
| SingleNamespace only    | Not specified            | OwnNamespace          | Default to most restrictive supported mode |
| SingleNamespace only    | Specified (≠ install ns) | SingleNamespace       | User's explicit choice, bundle supports it |
| SingleNamespace only    | Specified (= install ns) | Error                 | SingleNamespace cannot watch its own install namespace |
| OwnNamespace only       | Not specified            | OwnNamespace          | Use the only supported mode |
| OwnNamespace only       | Specified (= install ns) | OwnNamespace          | Explicit choice matches supported mode |
| OwnNamespace only       | Specified (≠ install ns) | Error                 | OwnNamespace can only watch install namespace |
| Single + Own            | Not specified            | OwnNamespace          | Default to more restrictive mode |
| Single + Own            | Specified (= install ns) | OwnNamespace          | Namespace relationship determines mode |
| Single + Own            | Specified (≠ install ns) | SingleNamespace       | Namespace relationship determines mode |
| All + Single + Own      | Not specified            | AllNamespaces         | Default to least restrictive mode for compatibility |
| All + Single + Own      | Specified (= install ns) | OwnNamespace          | Namespace relationship determines mode |
| All + Single + Own      | Specified (≠ install ns) | SingleNamespace       | Namespace relationship determines mode |

Default Mode Selection Rules are as follows:
  1. No watchNamespace specified: Use the most permissive mode supported by the bundle (AllNamespaces > OwnNamespace >
  SingleNamespace)
  2. watchNamespace = install namespace: Use OwnNamespace mode if supported, otherwise error
  3. watchNamespace ≠ install namespace: Use SingleNamespace mode if supported, otherwise error
  4. Unsupported mode requested: Installation fails with clear error message indicating supported modes

The relevant generated resources will be:
- Role/RoleBinding resources for namespace-scoped permissions in Single/OwnNamespace modes
- ClusterRole/ClusterRoleBinding resources for cluster-scoped permissions (AllNamespaces mode)
- Deployment resources with correct environment variables for watch namespace configuration
- Service and other supporting resources in the install namespace

Bundle validation ensures:
- Requested install mode is supported by the bundle's CSV
- Target namespace exists and is accessible
- ServiceAccount has sufficient permissions for the install mode

#### Configuration Validation
- `watchNamespace` must be a valid DNS1123 subdomain
- Namespace must exist at installation time
- ServiceAccount must have sufficient permissions for the target namespace

#### Conversion Logic

The existing OLMv1 bundle converter will support install mode detection and appropriate resource generation via a `WithTargetNamespaces()` option.

### Risks and Mitigations

Because we are enabling namespace-scoped operator installations, there are operational implications that could impact cluster management. These risks are mitigated by:

- **RBAC Misconfiguration**: Install mode validation ensures operators only receive permissions appropriate for their scope
- **Namespace Dependency**: Clear error messages when target namespaces don't exist or aren't accessible
- **Migration Complexity**: Comprehensive documentation and examples for transitioning between install modes
- **Permission Escalation**: ServiceAccount validation ensures adequate permissions without over-privileging

Currently, admins control the scope of operator installations through ClusterExtension RBAC. This enhancement adds namespace-level controls while maintaining existing security boundaries.

The feature is alpha and feature-gated, allowing administrators to:
- Control adoption timeline through feature gate management
- Test namespace-scoped installations in non-production environments
- Gradually migrate from AllNamespaces to more targeted install modes

Operators installed in Single/OwnNamespace modes have reduced blast radius compared to AllNamespaces installations, potentially improving cluster security posture.

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Increased Complexity** | Medium | Feature is alpha and feature-gated; clear documentation emphasizes this is transitional |
| **RBAC Misconfiguration** | High | Comprehensive validation and clear error messages; documentation provides RBAC examples |
| **Installation Failures** | Medium | Detailed preflight checks and validation; clear error reporting |
| **Security Boundaries** | Medium | Explicit validation of namespace permissions; RBAC properly scoped |
| **Feature Proliferation** | Low | Clear documentation that this is for legacy compatibility only |

### Drawbacks

- **Increased API Surface**: Adds configuration complexity to the ClusterExtension API
- **Maintenance Burden**: Requires ongoing support for legacy install modes
- **Potential Confusion**: Users might not understand when to use different install modes
- **Migration Complexity**: Organizations may delay moving to AllNamespaces mode

## Alternatives (Not Implemented)

### Alternative 1: Wait for Operator Authors to Migrate
**Description**: Do not implement Single/OwnNamespace support and require all operator authors to modify their bundles to support AllNamespaces mode.

**Why Not Selected**:
- Would create immediate migration blockers for customers
- Significant ecosystem impact requiring coordination across many operator teams
- Telco and other specialized operators have legitimate namespace isolation requirements

### Alternative 2: Support All Install Modes Including MultiNamespace
**Description**: Implement full OLM v0 install mode compatibility including MultiNamespace.

**Why Not Selected**:
- Would reintroduce the multi-tenancy complexity that OLM v1 explicitly avoided
- MultiNamespace mode was a primary source of problems in OLM v0
- Goes against core OLM v1 design principles

### Alternative 3: Separate CRD for Namespace-Scoped Operators
**Description**: Create a different API (e.g., NamespacedExtension) for namespace-scoped operator installations.

**Why Not Selected**:
- Would fragment the operator installation experience
- Adds unnecessary API complexity
- Configuration-based approach is more flexible and maintainable

## Open Questions
1. Do we want to generate validation schemas for registry+v1 bundles?
2. Should we allow schema files to be packaged in bundles? 

## Test Plan

### Integration Tests
- **End-to-End Installation**: Test complete installation flow for Single and OwnNamespace modes
- **Bundle Compatibility**: Verify handling of bundles with different install mode support
- **Permission Validation**: Verify ServiceAccount permission requirements

### Regression Tests
- **Conversion Compatibility**: Ensure generated manifests match OLM v0 output for equivalent configurations
- **Feature Gate Toggle**: Verify behavior when feature gate is disabled

### Test Data
https://github.com/openshift/operator-framework-operator-controller will include comprehensive test data in `/test/regression/convert/testdata/expected-manifests/` with separate directories for each install mode, providing reference manifests for validation.

## Graduation Criteria

### Dev Preview -> Tech Preview
- [ ] Feature-gated implementation behind `NewOLMOwnSingleNamespace`
- [ ] Basic functionality for Single and OwnNamespace modes
- [ ] Unit and integration test coverage
- [ ] Documentation for configuration and usage

### Tech Preview -> GA
- [ ] 1 OCP release of alpha feedback
- [ ] Production deployment validation
- [ ] Complete documentation including best practices
- [ ] Established support and maintenance processes

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

### Upgrade Strategy
- **Feature Gate Dependency**: Feature must be enabled via feature gate before configuration can be used
- **Backward Compatibility**: Existing AllNamespaces installations continue to work unchanged
- **Configuration Migration**: No automatic migration; users must explicitly install using OLMv1 ClusterExtension and configure `watchNamespace`

### Downgrade Strategy
- **Feature Gate Disable**: Disabling the feature gate prevents new Single/OwnNamespace installations
- **Existing Installations**: Already-installed Single/OwnNamespace operators continue to function
- **Configuration Removal**: Removing `watchNamespace` configuration reverts to AllNamespaces mode on next reconciliation

### Version Compatibility
- **Minimum Version**: Requires OpenShift 4.20+ 
- **Configuration Schema**: Uses existing ClusterExtension configuration schema for forward compatibility

## Operational Aspects of API Extensions

### Impact of Install Mode Extensions

**ClusterExtension API Enhancement:**
- **Architectural Impact:** The `config.inline.watchNamespace` field enables runtime install mode selection, moving from compile-time (bundle) to runtime (installation) configuration
- **Operational Impact:**
  - Administrators must understand namespace relationships and RBAC implications
  - Troubleshooting requires awareness of install vs watch namespace distinctions
  - Monitoring and alerting must account for namespace-scoped operator deployments

**RBAC Resource Generation:**
- **Architectural Impact:** Dynamic RBAC generation based on install mode creates different permission patterns for the same operator
- **Operational Impact:**
  - Permission debugging requires understanding of install mode impact on RBAC scope
  - Security auditing must consider namespace-level vs cluster-level permission grants
  - Upgrade scenarios may change RBAC scope if install mode changes


### Impact on Existing SLIs

With the removal of the install mode concept in olmv1, operator packages that want to continue to use this stop-gap feature are expected to surface configuration documentation, and call out if the `watchNamespace` parameter is a part of it, along with usage example etc. Until a broad percentage of operator pacakges, especially those that don't support `AllNamespaces` mode, take action to make such documentation avaialble, a spike in bad installation is expected.

**Installation Success Rate:** 

*   **RBAC Validation Complexity:** Namespace-scoped installations require more complex RBAC validation to ensure the ServiceAccount has appropriate permissions for the target namespace. RBAC misconfigurations that work in AllNamespaces mode may fail in Single/OwnNamespace modes.
    *   Example: ServiceAccount has cluster-wide read permissions but lacks namespace-specific write permissions, causing installation to fail.
*   **Bundle Compatibility Validation:** Additional validation layer to confirm bundles support the requested install mode. Bundles that only support AllNamespaces will fail when Single/OwnNamespace is requested.
    *   Example: Attempting to install a bundle with `watchNamespace: "test"` when the bundle CSV only declares support for AllNamespaces install mode.

**Installation Time:**

*   **Extended Validation Phase:** Additional validation steps for namespace existence, accessibility, and RBAC permissions add latency to the installation process. Each namespace-scoped installation must validate the target namespace and ServiceAccount permissions.
*   **RBAC Generation Complexity:** Converting cluster-scoped RBAC to namespace-scoped Role/RoleBinding resources requires additional processing time. Complex operators with extensive permission requirements will see increased installation duration.
*   **Cross-Namespace Connectivity Validation:** Single namespace mode requires validation that the operator in the install namespace can access resources in the watch namespace, adding network connectivity checks.

**Operator Availability:**

*   **Namespace Isolation Impact:** Operators installed in Single/OwnNamespace modes are more susceptible to namespace-level issues. Namespace deletion, network policies, or resource quotas can impact operator availability in ways that don't affect AllNamespaces operators.
    *   Example: A network policy blocking cross-namespace communication prevents a SingleNamespace operator from accessing its target resources.
*   **ServiceAccount Permission Dependencies:** Namespace-scoped operators depend on ServiceAccount permissions that may be modified by namespace administrators, creating additional failure points not present in cluster-scoped installations.
    *   Example: Namespace admin removes critical RoleBinding, causing operator to lose access to required resources.

**Resource Utilization:**

*   **RBAC Resource Proliferation:** Each Single/OwnNamespace installation creates namespace-scoped RBAC resources instead of reusing cluster-scoped ones. Multiple operators in different namespaces will create duplicate Role/RoleBinding resources rather than sharing ClusterRole/ClusterRoleBinding resources.
    *   Example: Installing the same operator in 10 different namespaces creates 10 sets of Role/RoleBinding resources instead of 1 set of ClusterRole/ClusterRoleBinding resources.
*   **Namespace Resource Quota Impact:** Operators and their RBAC resources count against namespace resource quotas, potentially causing quota exhaustion that doesn't occur with cluster-scoped installations.

### Possible Failure Modes

**Configuration Issues:**
- Invalid watchNamespace specification (DNS1123 validation failures)
- Target namespace doesn't exist or isn't accessible
- ServiceAccount lacks sufficient permissions for namespace access
- Bundle configuration does not include `watchNamespace`

**Runtime Issues:**
- Operator deployed in install namespace but cannot access watch namespace
- RBAC resources incorrectly scoped for actual operator requirements
- Network policies preventing cross-namespace access when needed

### OCP Teams Likely to be Called Upon in Case of Escalation

1. OLM Team (primary)
2. OpenShift API Server Team 
3. Networking Team (cross-namespace connectivity)
4. Authentication & Authorization Team (ServiceAccount/RBAC)
5. Layered Product Team

## Support Procedures

If there are problems with namespace-scoped operator installations:

1. **Verify Feature Gate**: Ensure `NewOLMOwnSingleNamespace` is enabled
2. **Check Namespace Existence**: Confirm target watch namespace exists and is accessible
3. **Validate ServiceAccount Permissions**: Verify ServiceAccount has required permissions for target namespace
4. **Review Bundle Compatibility**: Confirm bundle CSV supports the requested install mode
5. **Examine RBAC Resources**: Check generated Role/RoleBinding resources are correctly scoped

Common troubleshooting scenarios:
- **Installation Stuck**: Check namespace availability and ServiceAccount permissions
- **Operator Not Functioning**: Verify RBAC resources are correctly scoped to watch namespace
- **Permission Denied Errors**: Review ServiceAccount permissions and namespace access rights

For persistent issues, administrators can:
- Disable feature gate to fall back to AllNamespaces mode
- Modify watchNamespace configuration to change install mode
- Scale down operator-controller to manually intervene if needed

## Version Skew Strategy

### Component Interactions
- **operator-controller**: Must support the `NewOLMOwnSingleNamespace` feature gate
- **rukpak**: Uses existing conversion capabilities; no additional requirements
- **catalogs**: No changes required; 

### API Compatibility
- **ClusterExtension API**: Uses existing configuration schema; no API version changes required
- **Bundle Format**: Works with existing registry+v1 bundles without modification
- **Status Reporting**: Uses existing condition and status mechanisms

### Deployment Considerations
- **Feature Gate Synchronization**: All operator-controller replicas must have consistent feature gate configuration
- **Configuration Validation**: API server validates configuration schema regardless of feature gate state
- **Runtime Behavior**: Feature gate only affects installation behavior, not API acceptance

#### Feature Dependencies
- **Configuration Support**: This feature builds upon a ClusterExtension configuration infrastructure
- **RBAC Generation**: Leverages existing rukpak RBAC generation capabilities with enhanced scoping logic
- **Feature Gate Framework**: Uses established feature gate patterns for controlled rollout
