---
title: centralized-tls-configuration
authors:
  - "@richardsonnick"
reviewers:
  - TBD (Control Plane Team, Core Component Teams, Layered Product teams)
approvers:
  - "@joelanford"
  - "@mrunalp"
api-approvers:
  - "@everettraven"
  - "@JoelSpeed"
creation-date: 2025-12-12
last-updated: 2025-12-12
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2611
see-also:
  - "/enhancements/kube-apiserver/tls-config.md"
  - "/enhancements/security/api-tls-curves-config.md"
replaces:
superseded-by:
---

# Centralized and Enforced TLS Configuration Throughout OpenShift

## Summary

This enhancement ensures that all OpenShift components (both core platform and layered products) honor the centralized TLS security profile configuration. Currently, many components either hardcode their TLS settings or rely on Go/library defaults rather than respecting the cluster-wide TLS configuration defined via the API Server, Kubelet, or Ingress configurations. This creates security vulnerabilities and prevents customers from enforcing consistent cryptographic policies across their clusters, which is critical for Post-Quantum Cryptography (PQC) readiness.

## Motivation

Hardcoding TLS configuration creates security vulnerabilities because it does not align with evolving, centrally managed security policies. As organizations prepare for Post-Quantum Cryptography (PQC), they need the ability to configure TLS settings uniformly across all platform components through a small number of well-documented configuration points.

Today, not all OpenShift components obey the central TLS configuration, leading to:
- Inconsistent TLS behavior across the platform
- Inability to enforce custom TLS profiles defined by customers
- Blocked path to PQC readiness, as PQC-resilient algorithms will only be available in TLS 1.3+
- Customer security compliance failures when components ignore configured cipher restrictions

### User Stories

#### Story 1: Security Administrator Enforcing TLS Policy
As a security administrator, I want all OpenShift platform components to respect the TLS security profile I configure at the cluster level so that I can ensure consistent cryptographic policy enforcement across my entire cluster without having to configure each component individually.

#### Story 2: PQC Preparation
As a cluster administrator preparing for Post-Quantum Cryptography requirements, I want to enforce TLS 1.3 across my entire platform so that when PQC algorithms become available (via the separate curves configuration enhancement), all components will be ready to use them consistently.

#### Story 3: Custom TLS Profile Enforcement
As a security-conscious enterprise customer, I want to define a custom TLS profile that disables algorithms my security team considers unsafe, and I want all OpenShift components (including operators and layered products) to honor this configuration so that I can meet my organization's security compliance requirements.

#### Story 4: Upgrade Safety
As a cluster administrator, I want to have control over when components begin honoring centralized TLS configuration after an upgrade so that I can test the impact on my workloads and clients before fully enabling strict TLS policy enforcement.

### Goals

1. All OpenShift core platform components honor the centralized TLS configuration from the API Server, Kubelet, or Ingress configuration (as appropriate for each component).
2. Layered products and operators also respect the centralized TLS configuration.
3. Components explicitly configure all TLS profile settings rather than relying on Go/library defaults.
4. Provide a safe upgrade path that allows customers to gradually adopt stricter TLS enforcement without breaking existing workloads or clients.

### Non-Goals

1. Modifying the existing TLS security profile API structure (Old, Intermediate, Modern, Custom profiles) - that API is already defined.
2. Adding the `curves` field to the TLS profile API - that is covered by a separate enhancement (see [api-tls-curves-config.md](/enhancements/security/api-tls-curves-config.md)).
3. Defining specific PQC algorithms or curves to be used - this enhancement focuses on the enforcement mechanism.
4. Changing how TLS certificates are managed or distributed.

## Proposal

This proposal introduces a mechanism for gradual adoption of centralized TLS configuration enforcement, allowing components to transition from their current behavior (often ignoring the central config) to fully respecting the configured TLS profile.

### TLS Adherence Mode

To provide a safe upgrade path and prevent breaking changes when components begin honoring TLS configuration they previously ignored, we propose adding a `tlsAdherence` field to the relevant configuration APIs:

```go
// TLSAdherence specifies how strictly components should adhere to the
// centralized TLS security profile configuration.
type TLSAdherence string

const (
    // TLSAdherenceLegacy indicates that components should use their existing
    // behavior, which may include hardcoded or default TLS settings that do
    // not respect the centralized configuration. This is the default during
    // the transition period.
    TLSAdherenceLegacy TLSAdherence = "Legacy"

    // TLSAdherenceStrict indicates that all components must honor the
    // centralized TLS security profile configuration. Components that
    // previously used hardcoded or default settings will now respect
    // the configured profile.
    TLSAdherenceStrict TLSAdherence = "Strict"
)
```

This field would be added to the `APIServerSpec` alongside the existing `tlsSecurityProfile` field:

```go
type APIServerSpec struct {
    // ... existing fields ...

    // tlsSecurityProfile specifies settings for TLS connections for
    // externally exposed servers.
    // +optional
    TLSSecurityProfile *TLSSecurityProfile `json:"tlsSecurityProfile,omitempty"`

    // tlsAdherence specifies how strictly components should adhere to
    // the tlsSecurityProfile configuration. When set to "Strict", all
    // platform components will use the TLS configuration defined in this
    // object. When set to "Legacy" (the default), components continue
    // using their existing TLS configuration mechanisms.
    //
    // This field requires the TLSAdherenceStrict feature gate to be
    // enabled. When the feature gate is disabled, this field is not
    // present in the API.
    // +optional
    TLSAdherence TLSAdherence `json:"tlsAdherence,omitempty"`
)
```

### Component Behavior

Component TLS behavior depends on the feature gate and `tlsAdherence` field value (see [Feature Gate](#feature-gate) section for the full matrix).

When the feature gate is enabled and `tlsAdherence` is set to `Strict`, each component must:

1. **Fetch TLS configuration** from the appropriate central source:
   - **API Server configuration** (`apiserver.config.openshift.io/cluster`) - For most components that should match the API server TLS profile
   - **Kubelet configuration** - For components running on nodes that need to match node-level TLS settings
   - **Ingress configuration** - For components serving ingress traffic

2. **Explicitly configure all TLS parameters** including:
   - Minimum TLS version
   - Cipher suites
   - TLS curves (when the curves field is available)
   - Do NOT rely on Go or library defaults

3. **NOT use any hardcoded TLS configurations** from the codebase

When the feature gate is disabled, or when `tlsAdherence` is set to `Legacy`, components continue using their existing TLS configuration mechanisms.

### Workflow Description

**Cluster Administrator** is responsible for configuring TLS settings and managing the transition to strict enforcement.

1. The cluster administrator reviews their current TLS security profile configuration:
   ```bash
   oc get apiserver cluster -o jsonpath='{.spec.tlsSecurityProfile}'
   ```

2. The administrator enables the `TLSAdherenceStrict` feature gate on a test cluster to evaluate the impact:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: FeatureGate
   metadata:
     name: cluster
   spec:
     featureSet: CustomNoUpgrade
     customNoUpgrade:
       enabled:
         - TLSAdherenceStrict
   ```

3. The administrator sets `tlsAdherence: Strict` on the API server configuration:
   ```bash
   oc patch apiserver cluster --type=merge -p '{"spec":{"tlsAdherence":"Strict"}}'
   ```

4. The administrator monitors for any TLS handshake failures or client compatibility issues using cluster logs and metrics.

5. Once validated, the administrator can apply the same configuration to production clusters.

### API Extensions

- Adds `tlsAdherence` field to `APIServerSpec` in `apiserver.config.openshift.io/v1`
- Introduces `TLSAdherenceStrict` feature gate
- No changes to existing TLS profile structure (Old, Intermediate, Modern, Custom)

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift deployments:
- The management cluster's TLS configuration applies to management cluster components
- Each hosted control plane has its own TLS configuration that applies to the hosted cluster's components
- The `tlsAdherence` field would be honored independently in each context

#### Standalone Clusters

This enhancement applies directly to standalone clusters with no special considerations.

#### Single-node Deployments or MicroShift

- Single-node OpenShift (SNO): The enhancement applies fully
- MicroShift: The `tlsAdherence` concept should be exposed in MicroShift's configuration file, defaulting to `Legacy` for backwards compatibility

### Implementation Details/Notes/Constraints

#### Feature Gate

The `TLSAdherenceStrict` feature gate controls two things:

1. **API field presence**: When the feature gate is disabled, the `tlsAdherence` field is not present in the API. When enabled, the field becomes available.

2. **Component behavior**: The feature gate and field value together determine how components configure TLS:

   | Feature Gate | tlsAdherence     | Component Behavior |
   |--------------|------------------|------------------ -|
   | Disabled     | N/A              | Components use their existing mechanisms for TLS configuration |
   | Enabled      | Legacy (default) | Components use their existing mechanisms for TLS configuration |
   | Enabled      | Strict           | Components use the TLS configuration defined in `apiserver.config.openshift.io/cluster` |

#### Component Onboarding

Each component team is responsible for:

1. Auditing their codebase for hardcoded TLS settings
2. Implementing logic to read the appropriate central TLS configuration
3. Respecting the `tlsAdherence` field (or the operator's propagated configuration)
4. Adding tests to verify TLS compliance
5. Updating documentation

#### TLS Scanner Integration (Internal)

The [tls-scanner](https://github.com/openshift/tls-scanner) tool is used internally for CI testing and component onboarding verification. It should be enhanced to:
- Report which components are respecting the centralized TLS configuration
- Identify components that are still using hardcoded or default settings
- Verify compliance with the configured TLS profile

### Risks and Mitigations

#### Risk: Breaking Existing Client Connections

**Risk**: When components begin honoring stricter TLS settings, existing clients that don't support the configured TLS version or ciphers will fail to connect.

**Mitigation**:
- The `tlsAdherence` field defaults to `Legacy` to maintain backwards compatibility
- Administrators must explicitly opt in to `Strict` mode
- Documentation will clearly explain the implications of enabling strict adherence

#### Risk: Component Teams Not Adopting

**Risk**: Some component teams may not prioritize implementing TLS configuration adherence.

**Mitigation**:
- OCPSTRAT-2611 is marked as a release blocker for OCP 4.22
- Product management has created tracking epics for all affected teams
- TLS scanner can identify non-compliant components

### Drawbacks

1. **Additional complexity**: Introducing the `tlsAdherence` field adds another configuration option that administrators must understand.

2. **Coordination overhead**: Requires coordinating changes across many teams and components.

3. **Potential for configuration drift**: Until all components are updated, the cluster may have inconsistent TLS behavior.

## Alternatives

### Alternative 1: Immediate Enforcement Without Opt-In

We could simply require all components to honor the TLS configuration starting in a specific release without any opt-in mechanism.

**Rejected because**: This could break existing clusters on upgrade if customers have configured strict TLS profiles (like Modern) and have clients that cannot handle those settings. The gradual adoption approach is safer.

### Alternative 2: Per-Component TLS Adherence Setting

We could allow administrators to configure TLS adherence on a per-component basis.

**Rejected because**: This adds significant complexity and goes against the goal of centralized configuration. The three existing configuration points (API Server, Kubelet, Ingress) already provide sufficient granularity.

## Open Questions

1. **Should the default change to `Strict` in a future release?** Once all components support strict adherence and sufficient time has passed, should `Legacy` mode be deprecated?

2. **How should layered products be notified of TLS configuration changes?** Should there be a standard webhook or event mechanism, or should each product poll the configuration?

4. **Should we use a separate API instead of adding `tlsAdherence` to the APIServer config?** Rather than extending `apiserver.config.openshift.io`, we could introduce a dedicated API object for configuring TLS across platform servers.

5. **Do we need three adherence modes instead of two?** Consider whether we need `Legacy`, `StrictByDefault`, and `Strict`:
   - **Legacy**: Components use their existing TLS configuration mechanisms
   - **StrictByDefault**: Components honor the centralized TLS configuration unless they have a component-specific TLS configuration that has been explicitly provided
   - **Strict**: Components honor the centralized TLS configuration unconditionally, ignoring any component-specific TLS configuration

## Test Plan

### Unit Tests
- Each component must have unit tests verifying that TLS configuration is correctly read and applied when `tlsAdherence: Strict`
- Tests for fallback behavior when `tlsAdherence: Legacy`

### Integration Tests
- End-to-end test that sets `tlsAdherence: Strict` and verifies components respect the TLS profile
- Test that custom TLS profiles (with restricted ciphers) are enforced
- Test upgrade scenarios from `Legacy` to `Strict`

### Origin cluster variant
- Run the entire origin suite of tests when `tlsAdherence: Strict` is in use with the `Modern` profile
- Verification that no other OCP feature is broken when custom TLS profiles are in use

## Graduation Criteria

### Dev Preview -> Tech Preview

- `TLSAdherenceStrict` feature gate implemented
- `tlsAdherence` field added to APIServerSpec
- TLS scanner can report compliance status
- Documentation for administrators on how to enable and test

### Tech Preview -> GA

- All core platform components honor `tlsAdherence: Strict`
- Layered products tracked in OCPSTRAT-2611 are compliant
- Upgrade testing completed with no regressions
- User-facing documentation in openshift-docs
- Feature gate enabled by default

### Removing a deprecated feature

If `Legacy` mode is deprecated in the future:
1. Announce deprecation in release N
2. Emit warnings and block upgrades to N+2 when `tlsAdherence: Legacy` is explicitly set in release N+1
3. Remove `Legacy` mode and default to `Strict` in release N+2

## Upgrade / Downgrade Strategy

### Upgrade

On upgrade to a version with this enhancement:
- The `tlsAdherence` field defaults to `Legacy` (or empty, which is treated as `Legacy`)
- Existing TLS behavior is preserved
- Administrators must explicitly enable `Strict` mode after upgrade
- Feature gate must be enabled before `Strict` mode can be used (during Tech Preview)

### Downgrade

On downgrade from a version with this enhancement:
- The `tlsAdherence` field will be ignored by older versions
- Components will revert to their previous TLS behavior
- No data loss or configuration corruption expected

## Version Skew Strategy

During an upgrade, there will be a period where some components are running the new version and others are running the old version:

- Old components: Will ignore the `tlsAdherence` field and use their existing behavior
- New components: Will respect the `tlsAdherence` field

This is acceptable because:
1. The default is `Legacy`, which matches the old behavior
2. Administrators should only set `Strict` after the upgrade is complete
3. Even with skew, the cluster remains functional (just with inconsistent TLS enforcement)

## Operational Aspects of API Extensions

### Failure Modes

1. **Invalid TLS configuration**: If a custom TLS profile specifies incompatible settings (e.g., ciphers that don't work with configured curves), the component should:
   - Pass through the behavior of the underlying crypto implementation
   - If possible:
      - Log a clear error message
      - Set a degraded operator condition

2. **Configuration unavailable**: If a component cannot read the central TLS configuration:
   - In `Legacy` mode: Continue with existing behavior
   - In `Strict` mode: Set a degraded condition and fail closed (refuse connections)

3. **Adherence mode unavailable**: If a component cannot read the configured TLS adherence configuration, it should set a degraded condition and fail closed (refuse connections)

## Support Procedures

### Detecting TLS Configuration Issues

1. Check operator conditions for TLS-related degraded states:
   ```bash
   oc get co -o json | jq '.items[] | select(.status.conditions[] | select(.type=="Degraded" and .status=="True")) | .metadata.name'
   ```

2. Check component logs for TLS errors:
   ```bash
   oc logs -n openshift-<component> deploy/<component> | grep -i tls
   ```

### Reverting to Legacy Mode

If strict TLS adherence causes issues:

1. Set adherence back to Legacy:
   ```bash
   oc patch apiserver cluster --type=merge -p '{"spec":{"tlsAdherence":"Legacy"}}'
   ```

2. Wait for components to reconcile and verify connectivity is restored.

## Infrastructure Needed

- CI jobs for TLS compliance testing
- Integration with the TLS scanner tool