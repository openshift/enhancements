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
last-updated: 2026-01-21
tracking-link:
  - TBD
---

# Cluster-wide TLS Security Profile Configuration

## Summary

This enhancement proposes extending the existing `apiserver.config.openshift.io/v1` API to serve as the unified source of truth for TLS security settings across OpenShift clusters. Rather than introducing a new Custom Resource, we will leverage the existing APIServer configuration and establish that all components (with specific exceptions) should honor its TLS settings. This enhancement introduces a new `tlsAdherence` field to control how strictly components follow the configured TLS profile and provides clear documentation around TLS 1.3 cipher behavior.

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

3. **TLS 1.3 Transparency:** Clearly document the behavior that TLS 1.3 uses a hardcoded set of ciphers as defined by the Go runtime, removing ambiguity about cipher configuration.

### Non-Goals

1. Runtime monitoring of components for TLS compliance.

2. Automatic migration of existing component-specific TLS configurations.

3. Removing the existing component-specific TLS configuration capabilities (e.g., IngressController-specific profiles will be retained for legacy support).

4. Backporting fixes to existing Ingress controllers (new development efforts will focus on the Gateway API).

## Proposal

We propose extending the existing `apiserver.config.openshift.io/v1` API to serve as the source of truth for TLS security settings across the cluster. All components (with specific documented exceptions) should honor the TLS configuration defined in this API.

### API Design Principles

**Profile-Based:** The API supports the existing predefined profiles (Old, Intermediate, Modern).

**Custom Profile:** A Custom profile will be available for users requiring granular control to set configuration parameters manually. However, this profile will be explicitly documented as high-risk. Users utilizing the Custom profile must understand the limitations of underlying TLS implementations. Custom profiles are subject to the same TLS 1.3 cipher behavior documented below.

### TLS Adherence Modes

The new `tlsAdherence` field controls how strictly the TLS configuration is enforced by components:

**`legacy` (default):** Provides backward-compatible behavior. Components will attempt to honor the configured TLS profile but may fall back to their individual defaults if conflicts arise. This mode is intended for clusters that need to maintain compatibility with existing configurations during migration.

**`strict`:** Enforces strict adherence to the TLS configuration. All components must honor the configured profile. This mode is recommended for security-conscious deployments and is required for certain compliance frameworks.

### TLS 1.3 Cipher Behavior

When the minimum TLS version is set to TLS 1.3, the following behavior applies:

**Hardcoded Cipher Suites:** Go's `crypto/tls` library does not allow cipher suite configuration for TLS 1.3. When TLS 1.3 is the minimum version, the following cipher suites are automatically used and cannot be overridden:

- `TLS_AES_128_GCM_SHA256`
- `TLS_AES_256_GCM_SHA384`
- `TLS_CHACHA20_POLY1305_SHA256`

**Behavior:** Any custom cipher suite configurations specified with `minTLSVersion: VersionTLS13` will be ignored. The Go runtime will use the hardcoded cipher suites listed above regardless of what is specified in the configuration.

**Rationale:** This behavior is mandated by [Go's crypto/tls implementation](https://github.com/golang/go/issues/29349), which intentionally does not expose TLS 1.3 cipher suite configuration. The TLS 1.3 cipher suites are considered secure and the Go team has decided that allowing configuration could lead to weaker security postures.

### Scope and Component Expectations

All OpenShift components should honor the TLS configuration defined in `apiserver.config.openshift.io/v1`. With some exceptions as needed.


**Components With Override Capability:**
- **Ingress Controller:** Retains its existing capability to define a specific `tlsSecurityProfile`. This allows Ingress to support legacy external clients (e.g., using Intermediate or Old profiles) even if the cluster internal communication is set to Modern.
- **Routes:** Individual routes may specify TLS settings that differ from the cluster-wide default for specific application requirements.
- **Layered Products:** Layered products are expected to inherit the cluster default. Products unable to support the default (e.g., due to version incompatibility) must document their deviation and provide a justification.

### Workflow Description

**cluster administrator** is a human user responsible for configuring cluster-wide TLS security settings.

**component operator** is an automated system that consumes the cluster-wide TLS configuration and applies it to managed components.

1. The cluster administrator updates the `apiserver.config.openshift.io/v1` resource specifying the desired TLS security profile and adherence mode.

2. The configuration is stored in the cluster.

3. Component operators watch the APIServer configuration and update their respective component configurations.

4. Each component applies the new TLS settings. Components report their status via operator conditions.

#### Ingress Override Workflow

1. The cluster administrator configures a cluster-wide profile (e.g., Modern) in the APIServer configuration.

2. For applications requiring legacy client support, the administrator configures an IngressController-specific `tlsSecurityProfile` (e.g., Intermediate or Old).

3. The Ingress Controller uses its specific profile for external client connections while internal cluster communication continues to use the cluster-wide profile.

#### Passthrough vs. Re-encrypt Scenarios

**Re-encrypt:** In re-encrypt scenarios, the Ingress controller (acting as a Man-in-the-Middle) must honor the cluster-wide profile for the backend connection. The frontend connection uses the IngressController-specific profile if configured.

**Passthrough:** In passthrough scenarios, the backend component is responsible for honoring the cluster-wide profile as the Ingress controller does not terminate TLS.

### API Extensions

This enhancement extends the existing `apiserver.config.openshift.io/v1` resource with new fields:

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
      minTLSVersion: VersionTLS12  # Note: Custom ciphers only apply to TLS 1.2
  # New field introduced by this enhancement
  tlsAdherence: strict  # One of: legacy, strict (default: legacy)
status:
  conditions:
    - type: TLSConfigurationApplied
      status: "True"
      reason: AllComponentsUpdated
      message: "All components have applied the TLS configuration"
```

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

#### TLS 1.3 with Custom Ciphers (Not Recommended)

The following configuration is technically valid but the custom ciphers will be ignored:

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
        - TLS_AES_128_GCM_SHA256  # This will be IGNORED - TLS 1.3 ciphers are hardcoded
      minTLSVersion: VersionTLS13
  tlsAdherence: strict
# Note: Custom cipher suites are ignored with TLS 1.3. 
# The Go runtime uses hardcoded cipher suites for TLS 1.3 connections.
```

The API modifies existing behavior by:
- Establishing the APIServer configuration as the default source for TLS configuration that all core components will consume
- Introducing the `tlsAdherence` field to control enforcement behavior
- Documenting expected component behavior regarding TLS configuration inheritance

### Topology Considerations

#### Hypershift / Hosted Control Planes

For Hypershift deployments:
**TBD**

#### Standalone Clusters

This is the primary target for this enhancement. Standalone clusters will fully benefit from the unified TLS configuration approach.

#### Single-node Deployments or MicroShift

**Single-node OpenShift (SNO):** The enhancement applies fully. Resource consumption impact is minimal as the APIServer configuration is an existing lightweight resource watched by existing operators.

**MicroShift:** **TBD**

### Implementation Details/Notes/Constraints

#### Go crypto/tls Limitations

Components using Go's `crypto/tls` library have specific limitations:

**TLS 1.3 Cipher Suite Configuration:** Go's `crypto/tls` does not allow cipher suite configuration for TLS 1.3 ([golang/go#29349](https://github.com/golang/go/issues/29349)). When TLS 1.3 is configured, the following ciphers are used automatically:
- `TLS_AES_128_GCM_SHA256`
- `TLS_AES_256_GCM_SHA384`
- `TLS_CHACHA20_POLY1305_SHA256`

Any specified cipher suites will be ignored for TLS 1.3 connections.

**Curve Preferences Ordering:** Starting in Go 1.24, `CurvePreferences` semantics are changing ([golang/go#69393](https://github.com/golang/go/issues/69393)). Components should account for these changes.

### Risks and Mitigations

**Risk:** Component teams may not adopt the unified approach in the required timeframe.
**Mitigation:** Establish clear adoption deadlines, provide implementation guidance, and require justification for any component that cannot adopt the new approach. The `legacy` adherence mode provides a migration path.

**Risk:** Breaking existing configurations during migration.
**Mitigation:** The `tlsAdherence: legacy` mode (default) maintains backward compatibility. Existing component-specific configurations will continue to work. Clear migration documentation will be provided.

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

## Test Plan

**Unit Tests:**
- Profile expansion (predefined profiles to actual TLS settings)
- `tlsAdherence` field correctly parsed and applied

**Integration Tests:**
- Component operators correctly watch and respond to APIServer TLS configuration changes
- Components apply the correct TLS settings based on the profile
- Ingress override works correctly alongside cluster-wide settings
- `tlsAdherence` mode correctly controls enforcement behavior

**E2E Tests:**
- Create cluster with each predefined profile and verify TLS settings with tls-scanner
- Change profile and verify components update correctly
- Test passthrough and re-encrypt scenarios with different profiles

### Dev Preview -> Tech Preview

- Ability to configure TLS profile and `tlsAdherence` in APIServer resource
- At least one core component (e.g., kube-apiserver) respects the cluster-wide profile
- Documentation of TLS 1.3 hardcoded cipher behavior
- End user documentation available

### Tech Preview -> GA

- All core components respect the cluster-wide profile
- Upgrade/downgrade testing complete
- Performance testing complete
- User-facing documentation in openshift-docs

### Removing a deprecated feature

Not applicable for initial implementation.

## Upgrade / Downgrade Strategy

**Upgrade:**
- Existing clusters upgrading will default to `tlsAdherence: legacy` for backward compatibility
- Components will continue to use their existing configuration sources
- Administrators can opt-in to strict enforcement by setting `tlsAdherence: strict`
- Component-specific configurations (like IngressController) continue to override cluster defaults

**Downgrade:**
- If downgrading to a version without `tlsAdherence` support, the field will be ignored
- Components will revert to their previous behavior
- No special handling required as the existing `tlsSecurityProfile` field is preserved

## Version Skew Strategy

During upgrades, there will be a period where some components support the enhanced configuration and others do not:

- Components that support the enhanced configuration will respect `tlsAdherence` mode
- Components that don't yet support enhanced configuration will continue using their existing behavior
- The APIServer status will report which components have applied the configuration
- Operators should use the status to understand which components are respecting the cluster-wide profile

For n-2 kubelet skew:
- Older kubelets that don't support the enhanced TLS configuration will continue using MachineConfig-based TLS settings
- This is acceptable as long as the MachineConfig settings are compatible with the cluster-wide profile
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
- Component operator conditions indicate TLS configuration problems
- TLS handshake failures in component logs

**Metrics and Alerts:**
- Alert: `ClusterTLSConfigurationNotApplied` fires when components haven't applied config for > 15 minutes

### Troubleshooting Steps

1. Check APIServer TLS configuration status:
```bash
oc get apiserver cluster -o yaml
```

2. Check the configured `tlsAdherence` mode:
```bash
oc get apiserver cluster -o jsonpath='{.spec.tlsAdherence}'
```

3. For TLS 1.3, understand that custom ciphers are ignored:
```bash
oc get apiserver cluster -o jsonpath='{.spec.tlsSecurityProfile}'
```

4. Review component operator logs for TLS-related errors

### Recovery Procedures

**Component Not Applying Configuration:**
- Check component operator logs
- Restart the component operator if necessary
- Verify the component supports the configured profile

## Infrastructure Needed [optional]

No new infrastructure required. The feature will be implemented within existing OpenShift operator patterns using the existing APIServer configuration resource.
