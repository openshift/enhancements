---
title: centralized-tls-config
authors:
  - richardsonnick
reviewers:
  - dsalerno # OpenShift networking stack knowledge
approvers: 
  - JoelSpeed
api-approvers:
  - JoelSpeed
creation-date: 2026-01-20
last-updated: 2026-01-20
tracking-link:
  - TBD
---

# Cluster-wide TLS Security Profile API

## Summary

This enhancement proposes the creation of a cluster-wide TLS Security Profile API that will serve as the unified source of truth for TLS security settings across OpenShift clusters. The new API will establish strict validation to prevent silent configuration failures, support predefined profiles (Intermediate, Modern, FIPS, PQC), and provide a clean mechanism for Post-Quantum Cryptography (PQC) enablement. 

## Motivation

Currently, OpenShift utilizes multiple TLS configurations distributed across various components (e.g., Ingress Controller, API Server, Kubelet) rather than a single unified configuration. Coordinating these disparate configurations has created significant challenges:

**Difficulty Mapping API to TLS Implementations:** It is difficult to map the existing API structures to the varying constraints of underlying TLS implementations across different components. For example, the Golang TLS implementation is highly restrictive; setting the TLS version to TLS 1.3 causes Golang to ignore any specified cipher suites and strictly enforce its own defaults. This discrepancy creates a "silent failure" scenario where a user believes they have applied a specific cipher configuration, but the system is ignoring it. This leads to a misunderstanding of the actual state of OpenShift's TLS configuration.

**Upgrades:** There is difficulty in handling version transitions without breaking existing configurations across these multiple control points.

**Component Ambiguity:** It is currently unclear which components should honor kubelet vs. ingress configurations, specifically regarding TLS passthrough and decrypt/re-encrypt scenarios.

### User Stories

As a cluster administrator, I want to configure TLS security settings in a single location so that I can ensure consistent security policies across all cluster components without managing multiple configuration points.

As a security officer, I want the cluster to reject TLS configurations that are unsupported by underlying implementations so that I can avoid "silent failure" scenarios where I believe a security policy is enforced but it is actually being ignored.

As a compliance officer, I want to enable PQC-ready TLS profiles cluster-wide so that I can meet emerging regulatory requirements and future-proof communications against quantum computing threats.

As a platform operator managing layered products, I want layered products to inherit cluster-wide TLS settings by default so that I can maintain consistent security posture across the entire platform.

As an application developer, I want to understand clearly which TLS profile applies to my application's ingress so that I can support legacy clients when necessary while the cluster uses stricter internal profiles.

### Goals

1. **Separation of Concerns:** Establish a new, dedicated API for cluster-wide TLS security profiles, unifying the approach and separating it from the legacy component-specific configurations.

2. **Prevent Silent Failures:** Implement rigorous validation logic to reject configurations that are unsupported by the underlying TLS implementations (e.g., rejecting custom cipher suites if TLS 1.3 is selected).

3. **PQC Enablement:** Provide a clean mechanism to introduce PQC behaviors and profiles required for the 4.22 timeframe and regulatory compliance.

4. **Future-Proofing:** Align with the transition toward the Gateway API, avoiding technical debt associated with backporting fixes to legacy Ingress controllers.

### Non-Goals

1. Runtime monitoring of components for TLS compliance. The scope of the validation system is only within the configuration step.

2. Automatic migration of existing component-specific TLS configurations to the new cluster-wide API.

3. Removing the existing component-specific TLS configuration capabilities (e.g., IngressController-specific profiles will be retained for legacy support).

4. Backporting fixes to existing Ingress controllers (new development efforts will focus on the Gateway API).

## Proposal

I propose the creation of a cluster-wide TLS Security Profile API that will serve as the source of truth for TLS security settings across the cluster.

### API Design Principles

**Strictness:** This API will enforce strict security configurations and will not permit flexible or "best effort" settings that conflict with the underlying TLS implementation's capabilities. If a configuration is technically impossible in the underlying implementation (such as customizing TLS 1.3 ciphers in Go), the API will reject it at configuration time rather than ignoring it.

**Profile-Based:** The API will support predefined profiles (e.g., Intermediate, Modern, FIPS, PQC) to simplify adoption for layered products.

**Custom Profile:** A Custom profile will be available for users requiring granular control to set configuration parameters manually. However, this profile will be explicitly documented as high-risk. Users utilizing the Custom profile must strictly adhere to valid configurations; otherwise, they risk creating unsupported states or encountering the validation failures described above. Custom profiles will be validated at configuration time, and invalid configurations will fail immediately with descriptive error messages.

### Scope and Overrides

To address the need for specific components to deviate from the cluster-wide default (e.g., for legacy support), the following scoping rules apply:

**Cluster Default:** By default, all Core Components and the Kubelet will inherit the profile defined in the ClusterTLSProfile.

**Ingress Exception:** The Ingress Controller will retain its existing capability to define a specific `tlsSecurityProfile`. This allows Ingress to support legacy external clients (e.g., using Intermediate or Old profiles) even if the cluster internal communication (Kubelet/API) is set to Modern or PQC.

**Layered Products:** Layered products are expected to inherit the Cluster Default. Products unable to support the default (e.g., due to version incompatibility) will be considered on a case-by-case basis.

### Workflow Description

**cluster administrator** is a human user responsible for configuring cluster-wide TLS security settings.

**component operator** is an automated system that consumes the cluster-wide TLS configuration and applies it to managed components.

1. The cluster administrator creates or updates a `ClusterTLSProfile` resource specifying the desired TLS security profile.

2. The validation system validates the configuration against known TLS implementation constraints. If the configuration is invalid (e.g., custom cipher suites with TLS 1.3), the API rejects it with a descriptive error message.

3. Upon successful validation, the configuration is stored in the cluster.

4. Component operators (kube-apiserver-operator, kubelet, etc.) watch the `ClusterTLSProfile` resource and update their respective component configurations.

5. Each component applies the new TLS settings. Components report their status via operator conditions.

#### Ingress Override Workflow

1. The cluster administrator configures a cluster-wide profile (e.g., Modern or PQC).

2. For applications requiring legacy client support, the administrator configures an IngressController-specific `tlsSecurityProfile` (e.g., Intermediate or Old).

3. The Ingress Controller uses its specific profile for external client connections while internal cluster communication continues to use the cluster-wide profile.

#### Passthrough vs. Re-encrypt Scenarios

**Re-encrypt:** In re-encrypt scenarios, the Ingress controller (acting as a Man-in-the-Middle) must honor the cluster-wide profile for the backend connection. The frontend connection uses the IngressController-specific profile if configured.

**Passthrough:** In passthrough scenarios, the backend component is responsible for honoring the cluster-wide profile as the Ingress controller does not terminate TLS.

### API Extensions

This enhancement introduces a new cluster-scoped Custom Resource Definition:

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterTLSProfile
metadata:
  name: cluster
spec:
  tlsSecurityProfile:
    type: Modern  # One of: Old, Intermediate, Modern, FIPS, PQC, Custom
    # If type is Custom:
    custom:
      ciphers:
        - TLS_AES_128_GCM_SHA256
        - TLS_AES_256_GCM_SHA384
      minTLSVersion: VersionTLS13
      curves:
        - X25519MLKEM768
        - X25519
status:
  conditions:
    - type: Valid
      status: "True"
      reason: ConfigurationValid
      message: "TLS configuration validated successfully"
    - type: Applied
      status: "True"
      reason: AllComponentsUpdated
      message: "All components have applied the TLS configuration"
  componentStatuses:
    - component: kube-apiserver
      status: Applied
      lastUpdated: "2026-01-09T12:00:00Z"
    - component: kubelet
      status: Applied
      lastUpdated: "2026-01-09T12:01:00Z"
```

The API modifies existing behavior by:
- Establishing a new default source for TLS configuration that core components will consume
- Adding validation webhooks to reject invalid TLS configurations at admission time
- Core components will be updated to watch this new resource in addition to their existing configuration sources

### Topology Considerations

#### Hypershift / Hosted Control Planes

For Hypershift deployments:
- The management cluster will have its own `ClusterTLSProfile` governing management cluster components
- Each hosted control plane will have its own `ClusterTLSProfile` governing the hosted control plane components
- Guest cluster kubelets and other node components will respect the hosted control plane's `ClusterTLSProfile`

Coordination is needed to ensure:
- Management cluster TLS settings don't conflict with hosted control plane requirements
- The hosted control plane's TLS configuration properly propagates to guest cluster nodes

#### Standalone Clusters

This is the primary target for this enhancement. Standalone clusters will fully benefit from the unified TLS configuration approach.

#### Single-node Deployments or MicroShift

**Single-node OpenShift (SNO):** The enhancement applies fully. Resource consumption impact is minimal as the `ClusterTLSProfile` is a single lightweight resource watched by existing operators.

**MicroShift:** MicroShift maintains its own simplified configuration model. The `ClusterTLSProfile` concept should be evaluated for inclusion in MicroShift's configuration file as a simplified TLS profile setting. MicroShift teams should be consulted on appropriate configuration surface.

### Implementation Details/Notes/Constraints

#### Key Components

**Validation System:**
- This system will run at configuration time via admission webhooks
- It must maintain knowledge of what configurations are supported by underlying TLS implementations (Go crypto/tls, OpenSSL) and check against them when the TLS Profile is changed
- This system must fail early in the TLS configuration workflow to prevent deployment of a cluster with a malformed TLS configuration
- This system will not monitor components during runtime for TLS compliance

**Validation Rules:**
- Reject custom cipher suite configurations when `minTLSVersion` is TLS 1.3 (Go limitation)
- Reject cipher-curve combinations known to be incompatible
- Validate curve names against a known-good list for the Custom profile
- Warn (but allow) configurations that may have limited component support (e.g., PQC curves on older components)

#### Go crypto/tls Limitations

Components using Go's `crypto/tls` library have specific limitations:

**TLS 1.3 Cipher Suite Configuration:** Go's `crypto/tls` does not allow cipher suite configuration for TLS 1.3 ([golang/go#29349](https://github.com/golang/go/issues/29349)). The API validation must reject such configurations to prevent silent failures.

**Curve Preferences Ordering:** Starting in Go 1.24, `CurvePreferences` semantics are changing ([golang/go#69393](https://github.com/golang/go/issues/69393)). The validation system should account for these changes.

### Risks and Mitigations

**Risk:** Component teams may not adopt the new API in the required timeframe.
**Mitigation:** Establish clear adoption deadlines, provide implementation guidance, and require justification for any component that cannot adopt the new API.

**Risk:** Validation logic may not cover all edge cases of TLS implementation incompatibilities.
**Mitigation:** Start with known limitations (e.g., Go TLS 1.3 ciphers) and expand validation rules as new incompatibilities are discovered. Maintain comprehensive documentation of known limitations.

**Risk:** Breaking existing configurations during migration.
**Mitigation:** The cluster-wide API will be opt-in initially. Existing component-specific configurations will continue to work. Clear migration documentation will be provided.

**Risk:** Performance impact from validation webhooks.
**Mitigation:** Validation logic is lightweight (configuration parsing and rule checking). The `ClusterTLSProfile` resource changes infrequently, so webhook calls are rare.

### Drawbacks

**Complexity:** Adding another configuration layer increases system complexity. However, this is offset by the simplification of having a single source of truth.

**Migration Effort:** Existing clusters will need to consciously adopt the new API. This creates a transition period where both old and new configuration methods coexist.

**Ingress Exception:** Maintaining the Ingress exception creates a potential point of confusion. Clear documentation is required to explain when and why Ingress may differ from cluster-wide settings.

## Alternatives (Not Implemented)

**Alternative 1: Extend Existing Component-Specific APIs**
Rather than creating a new cluster-wide API, extend each existing component's API with better validation. This was rejected because:
- It doesn't solve the coordination problem
- Each component would need to independently implement the same validation logic
- No single source of truth for cluster TLS policy

**Alternative 2: Central Configuration with No Overrides**
Create a strict cluster-wide API with no component-level overrides. This was rejected because:
- Ingress needs flexibility to support legacy external clients
- Some components may have legitimate reasons for different TLS settings

**Alternative 3: Defer to Gateway API Only**
Wait for Gateway API adoption and implement TLS configuration only there. This was rejected because:
- Gateway API transition timeline is uncertain
- Current components need PQC support for 4.22
- Kubelet and API server are not affected by Gateway API

## Open Questions [optional]

1. What is the exact schema for the PQC profile? What curves and settings should it include by default?

2. How should the API handle components that report they cannot support the configured profile (e.g., older operator versions)?

3. Should there be a mechanism for administrators to acknowledge and override validation warnings for edge cases?

4. What is the interaction model with the existing `apiserver.config.openshift.io/cluster` TLS configuration?

## Test Plan

**Unit Tests:**
- Validation webhook correctly rejects invalid configurations
- Validation webhook accepts valid configurations
- Profile expansion (predefined profiles to actual TLS settings)

**Integration Tests:**
- Component operators correctly watch and respond to `ClusterTLSProfile` changes
- Components apply the correct TLS settings based on the profile
- Ingress override works correctly alongside cluster-wide settings

**E2E Tests:**
- Create cluster with each predefined profile and verify TLS settings with tls-scanner
- Change profile and verify components update correctly
- Configure Custom profile and verify validation rejects invalid configurations
- Test passthrough and re-encrypt scenarios with different profiles

### Dev Preview -> Tech Preview

- Ability to create and configure `ClusterTLSProfile` resource
- Validation system rejects known invalid configurations
- At least one core component (e.g., kube-apiserver) respects the cluster-wide profile
- End user documentation available

### Tech Preview -> GA

- All core components respect the cluster-wide profile
- Comprehensive validation for all known TLS implementation limitations
- PQC profile defined and functional
- Upgrade/downgrade testing complete
- Performance testing complete
- User-facing documentation in openshift-docs

### Removing a deprecated feature

Not applicable for initial implementation.

## Upgrade / Downgrade Strategy

**Upgrade:**
- Existing clusters upgrading to the version with `ClusterTLSProfile` will have no `ClusterTLSProfile` resource created by default
- Components will continue to use their existing configuration sources
- Administrators can opt-in by creating a `ClusterTLSProfile` resource
- Once created, components will begin honoring the cluster-wide profile
- Component-specific configurations (like IngressController) continue to override cluster defaults

**Downgrade:**
- If downgrading to a version without `ClusterTLSProfile` support, the resource will be orphaned
- Components will revert to their previous configuration sources (component-specific or defaults)
- Administrators should document their `ClusterTLSProfile` settings before downgrade if they wish to manually reconfigure components

## Version Skew Strategy

During upgrades, there will be a period where some components support `ClusterTLSProfile` and others do not:

- Components that support `ClusterTLSProfile` will read from it (if present) or fall back to their existing configuration sources
- Components that don't yet support `ClusterTLSProfile` will continue using their existing configuration
- The `ClusterTLSProfile` status will report which components have applied the configuration
- Operators should use the status to understand which components are respecting the cluster-wide profile

For n-2 kubelet skew:
- Older kubelets that don't support `ClusterTLSProfile` will continue using MachineConfig-based TLS settings
- This is acceptable as long as the MachineConfig settings are compatible with the cluster-wide profile
- Documentation will advise administrators to ensure compatibility during mixed-version periods

## Operational Aspects of API Extensions

**Admission Webhook:**
- A validating admission webhook will be deployed to validate `ClusterTLSProfile` resources
- SLI: `apiserver_admission_webhook_admission_duration_seconds` for the webhook
- The webhook should respond within 100ms for typical configurations

**Failure Modes:**
- If the webhook is unavailable, `ClusterTLSProfile` changes will be rejected (fail-closed)
- This is the desired behavior to prevent invalid configurations
- The webhook deployment will use standard HA practices (multiple replicas, PodDisruptionBudget)

**Impact on Existing SLIs:**
- Minimal impact as `ClusterTLSProfile` changes are infrequent (administrative action)
- No impact on pod scheduling or workload operations

**Escalation Teams:**
- OpenShift Security team for TLS validation logic issues
- Respective component teams for component-specific application issues

## Support Procedures

### Detecting Configuration Issues

**Symptoms:**
- `ClusterTLSProfile` resource shows `Valid=False` condition
- Component operator conditions indicate TLS configuration problems
- TLS handshake failures in component logs

**Metrics and Alerts:**
- `cluster_tls_profile_valid` metric (1 = valid, 0 = invalid)
- Alert: `ClusterTLSProfileInvalid` fires when configuration is invalid for > 5 minutes
- Alert: `ClusterTLSProfileNotApplied` fires when components haven't applied config for > 15 minutes

### Troubleshooting Steps

1. Check `ClusterTLSProfile` status:
```bash
oc get clustertlsprofile cluster -o yaml
```

2. Review conditions for validation errors:
```bash
oc get clustertlsprofile cluster -o jsonpath='{.status.conditions}'
```

3. Check component application status:
```bash
oc get clustertlsprofile cluster -o jsonpath='{.status.componentStatuses}'
```

4. Review component operator logs for TLS-related errors

### Recovery Procedures

**Invalid Configuration:**
- Edit the `ClusterTLSProfile` to fix validation errors
- Or delete the `ClusterTLSProfile` to revert components to their default configurations

**Component Not Applying Configuration:**
- Check component operator logs
- Restart the component operator if necessary
- Verify the component supports the configured profile

Components will revert to their individual configuration sources or defaults.

## Infrastructure Needed [optional]

No new infrastructure required. The feature will be implemented within existing OpenShift operator patterns.

