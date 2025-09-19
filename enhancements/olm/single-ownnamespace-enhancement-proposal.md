# OLM v1 Single and OwnNamespace Install Mode Support

| Title                  | OLM v1 Single and OwnNamespace Install Mode Support                                    |
|------------------------|-----------------------------------------------------------------------------------------|
| Authors                | [@anbhatta]                                                                            |
| Reviewers              | [@joelanford], [@perdasilva], [@thetechnick]            |
| Approvers              |                                                                                        |
| API Approvers          |                                                                                        |
| Creation Date          | 2025-01-20                                                                             |
| Last Updated           | 2025-01-20                                                                             |
| Tracking Link          | [OPRUN-4133](https://issues.redhat.com/browse/OPRUN-4133)                             |

## Summary

This enhancement proposes the implementation of Single and OwnNamespace install modes in OpenShift OLM v1, to provide backward compatibility for operators that only support namespace-scoped installation patterns. These operators are shipped in Openshift within existing operator catalogs, and contain installation bundles installable with OLM v0 (colloquially identified as registry+v1 bundles). This feature enables the installation of registry+v1 bundles that were designed for namespace-specific operation while maintaining OLM v1's simplified operator management approach.

The enhancement introduces a configuration mechanism through the ClusterExtension API that allows users to specify a `watchNamespace` parameter, enabling operators to be installed in either SingleNamespace mode (watching a namespace different from the install namespace) or OwnNamespace mode (watching the same namespace where the operator is installed).

## Motivation

OLM v0 provided four install modes (AllNamespaces, MultiNamespace, SingleNamespace, OwnNamespace) as part of its multi-tenancy features. While OLM v1 deliberately simplified this by only supporting AllNamespaces mode to avoid the complexity and problems associated with multi-tenancy, there exists significant operator content in catalogs that only supports Single and OwnNamespace modes.

### User Stories

#### Story 1: Legacy Operator Migration
As a cluster administrator migrating from OLM v0 to OLM v1, I want to install operators that only support SingleNamespace or OwnNamespace install modes so that I can continue using existing operator content without requiring operator authors to modify their bundles.

#### Story 2: Security-Conscious Deployments
As a security-conscious administrator, I want to install operators that explicitly limit their scope to specific namespaces so that I can maintain strict RBAC boundaries and reduce the attack surface of operator deployments.

#### Story 2: Operator Author Requirements
As an operator developer, I have operators designed for namespace isolation that only support Single or OwnNamespace modes, and I want my customers to be able to deploy these operators in OpenShift with OLM v1 so that I can maintain the intended isolation boundaries while benefiting from OLM v1's simplified management.

### Goals

- Enable installation of registry+v1 bundles that only support SingleNamespace or OwnNamespace install modes
- Provide a configuration mechanism through the ClusterExtension API to specify watch namespaces
- Maintain backward compatibility with existing OLM v0 operator content
- Generate appropriate RBAC resources scoped to the target namespaces
- Support seamless migration path for customers moving from OLM v0 to OLM v1

### Non-Goals

- Re-introducing multi-tenancy features or supporting multiple installations of the same operator
- Supporting MultiNamespace install mode (watching multiple namespaces)
- Modifying the fundamental OLM v1 architecture or adding complex multi-tenancy logic
- Supporting install mode switching after initial installation as a first-class feature
- Enabling this feature for bundle formats other than registry+v1

## Proposal

Update the OLMv1 operator-controller to:
1. Validate bundle compatibility with the requested install mode during resolution
2. Parse the `watchNamespace` configuration from ClusterExtension.spec.config.inline
3. Determine the install mode based on the relationship between install namespace and watch namespace
4. Generate appropriate RBAC resources (Roles/RoleBindings vs ClusterRoles/ClusterRoleBindings) based on the determined install mode
5. Configure operator deployments with correct environment variables and RBAC for the target watch namespace

Add support for namespace-scoped operation by ensuring:
1. Bundle validation confirms the operator supports the requested install mode
2. RBAC resources are properly scoped to the watch namespace for Single/OwnNamespace modes
3. Operator deployment environment variables are configured for the specified watch namespace
4. Installation fails gracefully with clear error messages when bundles don't support the requested mode

Ensure parity with OLMv0 behavior by:
1. Generating identical RBAC and deployment configurations for equivalent install modes
2. Maintaining the same namespace isolation boundaries
3. Preserving operator functionality across install mode transitions

### Workflow Description

#### Administrator Workflow

1. **Enable Feature Gate**: Administrator enables the `SingleOwnNamespaceInstallSupport` feature gate on the operator-controller deployment
2. **Create ServiceAccount**: Administrator creates a ServiceAccount with appropriate permissions in the target namespace
3. **Configure ClusterExtension**: Administrator creates a ClusterExtension resource specifying:
   - `spec.namespace`: The installation namespace where the operator pod will run
   - `spec.config.inline.watchNamespace`: The namespace the operator should watch for resources
4. **Install Mode Detection**: The system automatically determines the install mode:
   - **AllNamespaces**: `watchNamespace` is empty or not specified
   - **OwnNamespace**: `watchNamespace` equals `spec.namespace`
   - **SingleNamespace**: `watchNamespace` differs from `spec.namespace`

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

#### ClusterExtension API Enhancement

The ClusterExtension API is being enhanced with a `config` field designed for this purpose:

```go
type ClusterExtensionConfig struct {
    ConfigType ClusterExtensionConfigType `json:"configType"`
    Inline     *apiextensionsv1.JSON      `json:"inline,omitempty"`
}
```

The `watchNamespace` configuration is specified within the `inline` JSON configuration:

```json
{
  "watchNamespace": "target-namespace"
}
```

#### Feature Gate

The feature is controlled by the `SingleOwnNamespaceInstallSupport` feature gate:
- **Alpha** status (default: disabled)
- Must be explicitly enabled via `--feature-gates=SingleOwnNamespaceInstallSupport=true`

### Topology Considerations

#### Hypershift

OLMv1 does not yet support Hypershift. Although no aspects of this feature's implementation stands out as at odds with the topology of Hypershift, it should be reviewed when OLMv1 is ready to be supported in Hypershift clusters.

#### Standalone Clusters
- **Full Compatibility**: Complete support for Single/OwnNamespace modes
- **Standard RBAC**: Normal Role/RoleBinding generation for namespace-scoped permissions

#### Single-node Deployments
- **Resource Efficiency**: Namespace-scoped operators can reduce resource overhead on constrained single-node deployments
- **Isolation Benefits**: Provides additional isolation even in single-node environments

#### MicroShift

OLMv1 does not yet support Microshift. Although no aspects of this feature's implementation stands out as odds with the topology of Microshift, it should be reviewed when OLMv1 is ready to be supported in Microshift clusters. 

### Implementation Details/Notes/Constraints

The ClusterExtension CRD used in the registry+v1 bundle installation process is enhanced to:
- Parse watchNamespace configuration from the inline JSON configuration
- Map install modes based on namespace relationships (install vs watch namespace)
- Generate appropriate RBAC resources scoped to the target namespace

The relevant generated resources will be:
- Role/RoleBinding resources for namespace-scoped permissions in Single/OwnNamespace modes
- ClusterRole/ClusterRoleBinding resources for cluster-scoped permissions (AllNamespaces mode)
- Deployment resources with correct environment variables for watch namespace configuration
- Service and other supporting resources in the install namespace

Feature gate `SingleOwnNamespaceInstallSupport` controls:
- Availability of watchNamespace configuration parsing
- Install mode detection and validation
- Generation of namespace-scoped RBAC resources

Bundle validation ensures:
- Requested install mode is supported by the bundle's CSV
- Target namespace exists and is accessible
- ServiceAccount has sufficient permissions for the install mode

#### Configuration Validation
- `watchNamespace` must be a valid DNS1123 subdomain
- Namespace must exist at installation time
- ServiceAccount must have sufficient permissions for the target namespace

#### Conversion Logic
The existing OLMv1 rukpak converter will support install mode detection and appropriate resource generation via a `WithTargetNamespaces()` option.

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

### Alpha (Current State)
- [ ] Feature-gated implementation behind `SingleOwnNamespaceInstallSupport`
- [ ] Basic functionality for Single and OwnNamespace modes
- [ ] Unit and integration test coverage
- [ ] Documentation for configuration and usage

### GA
- [ ] 1 OCP release of alpha feedback
- [ ] Production deployment validation
- [ ] Complete documentation including best practices
- [ ] Established support and maintenance processes

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

**Installation Success Rate:**

*   **Namespace Dependency Failures:** Single/OwnNamespace installations depend on target namespaces existing and being accessible. If the watch namespace is deleted or becomes inaccessible during installation, the ClusterExtension will fail to install. This creates a **dependency on namespace lifecycle management** that doesn't exist in AllNamespaces mode.
    *   Example: Installation fails when the specified `watchNamespace: "production"` namespace is deleted during the installation process.
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
- Bundle doesn't support requested install mode

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

1. **Verify Feature Gate**: Ensure `SingleOwnNamespaceInstallSupport` is enabled
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
- **operator-controller**: Must support the `SingleOwnNamespaceInstallSupport` feature gate
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
