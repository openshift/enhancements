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
last-updated: 2026-01-26
tracking-link:
  - TBD
---

# Cluster-wide TLS Security Profile Configuration

## Summary

This enhancement proposes extending the existing `apiserver.config.openshift.io/v1` API to serve as the unified source of truth for TLS security settings across OpenShift clusters. Rather than introducing a new Custom Resource, we will leverage the existing APIServer configuration and establish that all components (with specific exceptions) should honor its TLS settings. This enhancement introduces a new `tlsAdherence` field to control how strictly components follow the configured TLS profile, adds validation to prevent invalid TLS 1.3 cipher configurations, and provides clear documentation around TLS 1.3 cipher behavior.

## Motivation

Currently, OpenShift utilizes multiple TLS configurations distributed across various components (e.g., Ingress Controller, API Server, Kubelet) rather than a single unified configuration.

It is currently unclear which components should honor kubelet vs. ingress configurations, specifically regarding TLS passthrough and decrypt/re-encrypt scenarios.

### User Stories

As a cluster administrator, I want to configure TLS security settings in a single location so that I can ensure consistent security policies across all cluster components without managing multiple configuration points.

As a platform operator managing layered products, I want layered products to inherit cluster-wide TLS settings by default so that I can maintain consistent security posture across the entire platform.

As an application developer, I want to understand clearly which TLS profile applies to my application's ingress so that I can support legacy clients when necessary while the cluster uses stricter internal profiles.

### Goals

1. **Unified Configuration:** Extend the existing `apiserver.config.openshift.io/v1` API to serve as the cluster-wide TLS security profile source of truth, avoiding the introduction of new Custom Resources.

2. **TLS Adherence Control:** Introduce a `tlsAdherence` field that allows administrators to choose between `legacy` behavior (for backward compatibility) and `strict` behavior (for enforced compliance).

3. **TLS 1.3 Transparency:** Clearly document and enforce the behavior that TLS 1.3 uses a hardcoded set of ciphers as defined by the Go runtime, removing ambiguity about cipher configuration.

4. **Validation:** Add validation to the APIServer TLS configuration to disallow cipher suite configuration when `minTLSVersion` is set to TLS 1.3.

### Non-Goals

1. Runtime monitoring of components for TLS compliance.

2. Automatic migration of existing component-specific TLS configurations.

3. Removing the existing component-specific TLS configuration capabilities (e.g., IngressController-specific profiles will be retained for legacy support).

4. Backporting fixes to existing Ingress controllers (new development efforts will focus on the Gateway API).

5. TLS curves configuration for the APIServer (this is being addressed in a separate Ingress TLS Curves enhancement).

6. Enforcing client TLS settings. This enhancement applies only to the server-side TLS configuration of managed components.

## Proposal

We propose extending the existing `apiserver.config.openshift.io/v1` API to serve as the source of truth for TLS security settings across the cluster. All components (with specific documented exceptions) should honor the TLS configuration defined in this API.

### API Design Principles

**Profile-Based:** The API supports the existing predefined profiles (Old, Intermediate, Modern).

**Custom Profile:** A Custom profile will be available for users requiring granular control to set configuration parameters manually. However, this profile will be explicitly documented as high-risk. Users utilizing the Custom profile must understand the limitations of underlying TLS implementations. Custom profiles are subject to the same TLS 1.3 cipher behavior documented below.

**Restrictive Validation:** The APIServer TLS configuration will validate that cipher suites cannot be specified when `minTLSVersion` is set to TLS 1.3, preventing silent failures where users believe they have configured ciphers but they are being ignored.

### TLS Adherence Modes

The new `tlsAdherence` field is a **sibling** to the existing `tlsSecurityProfile` field on the APIServer config object. It controls how strictly the TLS configuration is enforced by components:

**`legacy` (default):** Provides backward-compatible behavior. Components will attempt to honor the configured TLS profile but may fall back to their individual defaults if conflicts arise. This mode is intended for clusters that need to maintain compatibility with existing configurations during migration.

**`strict`:** Enforces strict adherence to the TLS configuration. All components must honor the configured profile. If a core component fails to honor the TLS configuration when `strict` is set, this is treated as a **bug** requiring fixes and backporting. This mode is recommended for security-conscious deployments and is required for certain compliance frameworks.

**Unknown Enum Handling:** If a component encounters an unknown value for `tlsAdherence`, it should treat it as `strict` and log a warning. This ensures forward compatibility while defaulting to the more secure behavior.

### TLS 1.3 Cipher Behavior

When the minimum TLS version is set to TLS 1.3, the following behavior applies:

**Hardcoded Cipher Suites:** Go's `crypto/tls` library does not allow cipher suite configuration for TLS 1.3. When TLS 1.3 is the minimum version, the following cipher suites are automatically used and cannot be overridden:

- `TLS_AES_128_GCM_SHA256`
- `TLS_AES_256_GCM_SHA384`
- `TLS_CHACHA20_POLY1305_SHA256`

**Validation:** The APIServer TLS configuration will reject attempts to specify custom cipher suites when `minTLSVersion` is set to `VersionTLS13`. This validation prevents the "silent failure" scenario where users believe they have configured specific ciphers but the Go runtime ignores them.

**Rationale:** This behavior is mandated by [Go's crypto/tls implementation](https://github.com/golang/go/issues/29349), which intentionally does not expose TLS 1.3 cipher suite configuration. The TLS 1.3 cipher suites are considered secure and the Go team has decided that allowing configuration could lead to weaker security postures.

### Scope and Component Expectations

All OpenShift components should honor the TLS configuration defined in `apiserver.config.openshift.io/v1`, with the following exceptions.

**Components Excluded from `tlsAdherence` Toggle:**

The `strict` vs `legacy` toggle applies to all components **except** the following, which already have their own TLS security profile configuration paths:

- **Kubelet:** Configured via `KubeletConfig` CR with its own `tlsSecurityProfile` field
- **Ingress Controller:** Configured via `IngressController` CR with its own `tlsSecurityProfile` field

These components will continue to use their existing configuration mechanisms. Fresh clusters will default to the APIServer config, while upgraded clusters will retain their existing configurations.

**Components With Override Capability:**

- **Ingress Controller:** Retains its existing capability to define a specific `tlsSecurityProfile`. This allows Ingress to support legacy external clients (e.g., using Intermediate or Old profiles) even if the cluster internal communication is set to Modern.
- **Routes:** Individual routes may specify TLS settings that differ from the cluster-wide default for specific application requirements.
- **Gateway Controller:** Will initially honor the APIServer TLS profile. No configurable TLS 1.3 suites or curves. Overrides may be added later if needed.

**Layered Products:** Layered products are expected to inherit the cluster default. Products unable to support the default (e.g., due to version incompatibility) must document their deviation and provide a justification. For non-metrics or non-webhook product servers, the expectation is to fall back to the APIServer's TLS configuration; offering specific override configuration is a product team decision.

### Workflow Description

**cluster administrator** is a human user responsible for configuring cluster-wide TLS security settings.

**component operator** is an automated system that consumes the cluster-wide TLS configuration and applies it to managed components.

1. The cluster administrator updates the `apiserver.config.openshift.io/v1` resource specifying the desired TLS security profile and adherence mode.

2. Validation checks the configuration. If cipher suites are specified with `minTLSVersion: VersionTLS13`, the configuration is rejected with a descriptive error.

3. Upon successful validation, the configuration is stored in the cluster.

4. Component operators watch the APIServer configuration and update their respective component configurations.

5. Each component applies the new TLS settings. Components report their status via operator conditions.

#### Ingress Override Workflow

1. The cluster administrator configures a cluster-wide profile (e.g., Modern) in the APIServer configuration.

2. For applications requiring legacy client support, the administrator configures an IngressController-specific `tlsSecurityProfile` (e.g., Intermediate or Old).

3. The Ingress Controller uses its specific profile for external client connections while internal cluster communication continues to use the cluster-wide profile.

#### Passthrough vs. Re-encrypt Scenarios

**Re-encrypt:** In re-encrypt scenarios, the Ingress controller (acting as a Man-in-the-Middle) must honor the cluster-wide profile for the backend connection. The frontend connection uses the IngressController-specific profile if configured.

**Passthrough:** In passthrough scenarios, the backend component is responsible for honoring the cluster-wide profile as the Ingress controller does not terminate TLS.

### API Extensions

This enhancement extends the existing `apiserver.config.openshift.io/v1` resource with a new `tlsAdherence` field as a sibling to the existing `tlsSecurityProfile`:

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  tlsSecurityProfile:
    type: Modern  # One of: Old, Intermediate, Modern, Custom
    # If type is Custom:
    custom:
      ciphers:
        - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
        - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
      minTLSVersion: VersionTLS12  # Custom ciphers only valid with TLS 1.2
  # New field introduced by this enhancement (sibling to tlsSecurityProfile)
  tlsAdherence: strict  # One of: legacy, strict (default: legacy)
```

**Note:** The APIServer config currently lacks a status field. Future work may add a status field for components to report observed configuration and flag non-compliance.

#### TLS 1.3 Example

When using TLS 1.3, cipher configuration is not applicable:

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  tlsSecurityProfile:
    type: Modern
    # Modern profile sets minTLSVersion: VersionTLS13
    # Cipher suites are automatically set by Go runtime:
    # - TLS_AES_128_GCM_SHA256
    # - TLS_AES_256_GCM_SHA384
    # - TLS_CHACHA20_POLY1305_SHA256
  tlsAdherence: strict
```

#### Custom Profile with TLS 1.2 Example

Custom cipher configuration is only supported with TLS 1.2:

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  tlsSecurityProfile:
    type: Custom
    custom:
      ciphers:
        - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
        - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
        - TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
        - TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
      minTLSVersion: VersionTLS12
  tlsAdherence: strict
```

#### Invalid Configuration (Rejected)

The following configuration will be **rejected by validation**:

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  tlsSecurityProfile:
    type: Custom
    custom:
      ciphers:
        - TLS_AES_128_GCM_SHA256
      minTLSVersion: VersionTLS13  # ERROR: Cannot specify ciphers with TLS 1.3
  tlsAdherence: strict
# Validation Error: Cipher suites cannot be configured when minTLSVersion is VersionTLS13.
# TLS 1.3 cipher suites are hardcoded by the Go runtime.
```

The API modifies existing behavior by:
- Establishing the APIServer configuration as the default source for TLS configuration that all core components will consume
- Introducing the `tlsAdherence` field to control enforcement behavior
- Adding validation to reject cipher suite configuration with TLS 1.3
- Documenting expected component behavior regarding TLS configuration inheritance

### Feature Gate

The `tlsAdherence` field will be introduced behind a feature gate:

- **Feature Gate Name:** `TLSAdherence`
- **Initial State:** Tech Preview
- **Promotion Path:** Promote to GA quickly once core components are confirmed to honor the field

### Related Work

**Ingress TLS Curves:** A separate enhancement (led by Davide Salerno) will create a divergent TLS security profile struct for Ingress, allowing configuration of TLS curves and cipher suites. This is necessary because:
- The Ingress controller uses HAProxy (not Go's crypto/tls)
- HAProxy supports TLS curve configuration
- The separated struct allows Ingress-specific fields without affecting the APIServer's restrictive validation

### Topology Considerations

#### Hypershift / Hosted Control Planes

For Hypershift deployments:
**TBD**

#### Standalone Clusters

This is the primary target for this enhancement. Standalone clusters will fully benefit from the unified TLS configuration approach.

#### Single-node Deployments or MicroShift

**Single-node OpenShift (SNO):** The enhancement applies fully. Resource consumption impact is minimal as the APIServer configuration is an existing lightweight resource watched by existing operators.

**MicroShift:** **TBD**

#### OpenShift Kubernetes Engine

This enhancement applies to OpenShift Kubernetes Engine (OKE) clusters. The TLS configuration mechanism works identically to standard OpenShift Container Platform clusters.

### Implementation Details/Notes/Constraints

#### Go crypto/tls Limitations

Components using Go's `crypto/tls` library have specific limitations:

**TLS 1.3 Cipher Suite Configuration:** Go's `crypto/tls` does not allow cipher suite configuration for TLS 1.3 ([golang/go#29349](https://github.com/golang/go/issues/29349)). When TLS 1.3 is configured, the following ciphers are used automatically:
- `TLS_AES_128_GCM_SHA256`
- `TLS_AES_256_GCM_SHA384`
- `TLS_CHACHA20_POLY1305_SHA256`

The APIServer validation will reject configurations that attempt to set cipher suites with TLS 1.3.

**Curve Preferences Ordering:** Starting in Go 1.24, `CurvePreferences` semantics are changing ([golang/go#69393](https://github.com/golang/go/issues/69393)). Components should account for these changes.

### Risks and Mitigations

**Risk:** Component teams may not adopt the unified approach in the required timeframe.
**Mitigation:** Establish clear adoption deadlines, provide implementation guidance, and require justification for any component that cannot adopt the new approach. The `legacy` adherence mode provides a migration path.

**Risk:** Breaking existing configurations during migration.
**Mitigation:** The `tlsAdherence: legacy` mode (default) maintains backward compatibility. Existing component-specific configurations will continue to work. Clear migration documentation will be provided.

**Risk:** Components may have bugs where they don't honor the TLS configuration.
**Mitigation:** When `tlsAdherence: strict` is set, non-compliance is treated as a bug requiring fixes and backporting. CI tests will probe TLS servers to verify compliance.

### Drawbacks

**Complexity:** Extending the existing APIServer API with new semantics adds complexity. However, this is offset by avoiding the introduction of yet another Custom Resource.

**Migration Effort:** Existing clusters will need to consciously adopt `strict` mode for full enforcement. This creates a transition period where both behaviors coexist.

## Alternatives (Not Implemented)

**Alternative 1: Create a New ClusterTLSProfile CRD**
Create a dedicated new Custom Resource for cluster-wide TLS configuration. This was rejected because:
- Introduces yet another configuration resource for administrators to manage
- The existing APIServer configuration already has TLS profile support
- Better to enhance existing APIs than proliferate new ones

## Open Questions [optional]

1. How should the API handle components that report they cannot support the configured profile (e.g., older operator versions)?

2. Should a status field be added to the APIServer config for components to report compliance?

## Graduation Criteria

### Dev Preview -> Tech Preview

- `TLSAdherence` feature gate implemented
- Ability to configure TLS profile and `tlsAdherence` in APIServer resource
- Validation rejects cipher suites when `minTLSVersion` is TLS 1.3
- At least one core component (e.g., kube-apiserver) respects the cluster-wide profile
- Documentation of TLS 1.3 hardcoded cipher behavior
- End user documentation available

### Tech Preview -> GA

- **Explicit list of components confirmed to honor the `tlsAdherence` field** (list TBD before GA)
- All core components respect the cluster-wide profile
- CI tests verify TLS server compliance
- Upgrade/downgrade testing complete
- Performance testing complete
- User-facing documentation in openshift-docs

### Removing a deprecated feature

Not applicable for initial implementation.

## Test Plan

**Unit Tests:**
- Validation correctly rejects cipher suites with TLS 1.3
- Validation accepts valid configurations (ciphers with TLS 1.2, no ciphers with TLS 1.3)
- Profile expansion (predefined profiles to actual TLS settings)
- `tlsAdherence` field correctly parsed and applied
- Unknown `tlsAdherence` enum values treated as `strict`

**Integration Tests:**
- Component operators correctly watch and respond to APIServer TLS configuration changes
- Components apply the correct TLS settings based on the profile
- Ingress override works correctly alongside cluster-wide settings
- `tlsAdherence` mode correctly controls enforcement behavior

**E2E Tests:**
- Create cluster with each predefined profile and verify TLS settings with tls-scanner
- Change profile and verify components update correctly
- Verify validation rejects invalid TLS 1.3 + cipher configurations
- Test passthrough and re-encrypt scenarios with different profiles
- CI tests probe TLS servers to verify they honor the configured profile (potentially leveraging existing ports-open-registry test patterns)

## Upgrade / Downgrade Strategy

**Upgrade:**
- Existing clusters upgrading will default to `tlsAdherence: legacy` for backward compatibility
- Components will continue to use their existing configuration sources
- Administrators can opt-in to strict enforcement by setting `tlsAdherence: strict`
- Component-specific configurations (like IngressController, KubeletConfig) continue to use their own TLS profile settings

**Downgrade:**
- If downgrading to a version without `tlsAdherence` support, the field will be ignored
- Components will revert to their previous behavior
- No special handling required as the existing `tlsSecurityProfile` field is preserved

## Version Skew Strategy

During upgrades, there will be a period where some components support the enhanced configuration and others do not:

- Components that support the enhanced configuration will respect `tlsAdherence` mode
- Components that don't yet support enhanced configuration will continue using their existing behavior
- Operators should check if the `tlsAdherence` field is set and honor it when supported

For n-2 kubelet skew:
- Older kubelets that don't support the enhanced TLS configuration will continue using KubeletConfig-based TLS settings
- This is acceptable as kubelet is explicitly excluded from the `tlsAdherence` toggle
- Documentation will advise administrators to ensure compatibility during mixed-version periods

## Operational Aspects of API Extensions

**Impact on Existing SLIs:**
- Minimal impact as APIServer configuration changes are infrequent (administrative action)
- No impact on pod scheduling or workload operations

**Escalation Teams:**
- OpenShift Security team for TLS configuration issues
- Respective component teams for component-specific application issues

## Support Procedures

### Detecting Configuration Issues

**Symptoms:**
- Validation errors when attempting to set cipher suites with TLS 1.3
- Component operator conditions indicate TLS configuration problems
- TLS handshake failures in component logs

**Metrics and Alerts:**
- Alert: `ClusterTLSConfigurationNotApplied` fires when components haven't applied config for > 15 minutes

### Troubleshooting Steps

1. Check APIServer TLS configuration:
```bash
oc get apiserver cluster -o yaml
```

2. Check the configured `tlsAdherence` mode:
```bash
oc get apiserver cluster -o jsonpath='{.spec.tlsAdherence}'
```

3. Verify no cipher suites are set with TLS 1.3:
```bash
oc get apiserver cluster -o jsonpath='{.spec.tlsSecurityProfile}'
```

4. Review component operator logs for TLS-related errors

### Recovery Procedures

**Validation Error (cipher suites with TLS 1.3):**
- Remove the cipher suite configuration, or
- Change `minTLSVersion` to `VersionTLS12` if custom ciphers are required

**Component Not Applying Configuration:**
- Check component operator logs
- Restart the component operator if necessary
- Verify the component supports the configured profile
- If `tlsAdherence: strict` is set and component is not compliant, file a bug

## Infrastructure Needed [optional]

No new infrastructure required. The feature will be implemented within existing OpenShift operator patterns using the existing APIServer configuration resource.
