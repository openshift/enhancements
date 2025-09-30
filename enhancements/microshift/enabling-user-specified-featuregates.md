---
title: enabling-user-specified-featuregates
authors:
  - copejon
reviewers:  
  - "@pacevedom, MicroShift Team Lead"
  - "@pmtk, MicroShift Team Engineer"
approvers:
  - "@jerpeter1" # MicroShift principal engineer
api-approvers:
  - None # Configuration file changes only, no API modifications
creation-date: 2025-09-24 # You'll need to fill in today's date
last-updated: 2025-09-29
tracking-link:
  -  https://issues.redhat.com/browse/USHIFT-6177
see-also:
  - ""
---

# Enabling User Specified FeatureGates

## Summary

MicroShift disables all feature gates from OpenShift by default while hardcoding only a few relevant ones, and lacks a controlled mechanism for users to experiment with additional feature gates or override defaults. This enhancement proposes adding configuration support for Kubernetes and OpenShift feature gates through the MicroShift configuration file. This capability will enable users to experiment with alpha and beta OpenShift and Kubernetes features like CPUManager's `prefer-align-cpus-by-uncorecache` in a supported and deterministic way, addressing edge computing use cases where users want to evaluate advanced resource management capabilities.

## Motivation

MicroShift users in edge computing environments want to experiment with upcoming Kubernetes features that are in alpha or beta stages to evaluate their potential benefits for specific use cases. Currently, users cannot configure feature gates in a supported way, preventing them from experimenting with capabilities like advanced CPU management, enhanced scheduling features, or experimental storage options that might improve performance in their resource-constrained edge environments.

### User Stories

* As a MicroShift administrator, I want to configure feature gates through the MicroShift configuration file so that I can experiment with alpha/beta OpenShift features in a controlled and supported manner.

### Goals

* Enable user configuration of Kubernetes and OpenShift feature gates through the MicroShift configuration file
* Provide a controlled and deterministic way to experiment with alpha and beta features

### Non-Goals

* Modify MicroShift's existing feature gate defaults
* Vetting custom feature gates for compatibility with MicroShift
* Validating custom feature gate settings for correctness, e.g. spelling, case, and punctuation
* Automatic enablement of experimental features without explicit user configuration
* Providing upgrade support to customized clusters

## Proposal

This enhancement proposes adding feature gate configuration support to MicroShift by extending `/etc/microshift/config.yaml` with a configuration schema inspired by OpenShift's FeatureGate custom resource specification. The configuration will support both predefined feature sets and custom feature gate combinations, ensuring consistency with OpenShift's FeatureGate API patterns.

The implementation includes:

1. **FeatureGate Configuration Schema**: Extend MicroShift's configuration file to include `featureGates` section inspired by OpenShift's FeatureGate CRD spec fields (`featureSet` and `customNoUpgrade`)
2. **Predefined Feature Sets**: Support for OpenShift's predefined feature sets like `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade`
3. **Custom Feature Gates**: Support for individual feature gate enablement/disablement via `customNoUpgrade` configuration

This approach ensures that users can experiment with the same feature gate capabilities as OpenShift while maintaining MicroShift's file-based configuration pattern. Default feature gate values will continue to be inherited from OpenShift to ensure consistency across the platform.

### Workflow Description

**MicroShift Administrator** is a human user responsible for configuring and managing MicroShift deployments.

#### User Configuration Workflow
1. MicroShift Administrator identifies a need for specific feature gates (e.g., `CPUManagerPolicyAlphaOptions`)
2. Administrator chooses between two configuration approaches:
   - **Predefined Feature Set**: Configure `featureGates.featureSet: TechPreviewNoUpgrade` or `DevPreviewNoUpgrade` for a curated set of preview features
   - **Custom Feature Gates**: Configure `featureGates.featureSet: CustomNoUpgrade` and specify individual features in `featureGates.customNoUpgrade.enabled/disabled` lists
3. Administrator updates `/etc/microshift/config.yaml` with the chosen configuration
4. Administrator restarts MicroShift service
5. MicroShift parses the FeatureGate configuration and passes settings to relevant Kubernetes components where validation occurs
6. The features are enabled / disabled according to the configured state

### API Extensions

This enhancement extends MicroShift's configuration file schema only. No new CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers are introduced. The configuration file structure will be extended to include a `featureGates` section inspired by the OpenShift FeatureGate CRD specification, providing consistency with OpenShift's feature gate configuration patterns while maintaining MicroShift's file-based configuration approach.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is not applicable to Hypershift/Hosted Control Planes as feature gate configuration in hosted environments would be managed through the hosting cluster's OpenShift FeatureGate API rather than through MicroShift configuration.

#### Standalone Clusters

This enhancement is primarily designed for standalone MicroShift deployments where administrators need direct control over feature gate configuration through the local configuration file.

#### Single-node Deployments or MicroShift

This enhancement is specific to MicroShift only and does not affect single-node OpenShift (SNO) deployments.

For MicroShift, feature gates configured through this mechanism will affect all Kubernetes components running within the MicroShift instance, including:

- kubelet
- kube-apiserver
- kube-controller-manager
- kube-scheduler

The resource consumption impact will be minimal as this enhancement only adds configuration parsing and pass-through functionality. The actual resource impact will depend on which feature gates are enabled by users and their specific behaviors.

### Implementation Details/Notes/Constraints

#### Configuration Schema Extension

The MicroShift configuration file will be extended to include a new `featureGates` section inspired by the OpenShift FeatureGate CRD specification:

**Predefined Feature Set Configuration:**
```yaml
featureGates:
  featureSet: TechPreviewNoUpgrade
```

**Custom Feature Gates Configuration:**
```yaml
featureGates:
  featureSet: CustomNoUpgrade
  customNoUpgrade:
    enabled:
      - "CPUManagerPolicyAlphaOptions"
      - "MemoryQoS"
    disabled:
      - "SomeDefaultEnabledFeature"
```

**Configuration Rules:**
- The `featureSet` field is required when configuring feature gates
- When using `customNoUpgrade`, the `featureSet` must be set to `CustomNoUpgrade`
- The `customNoUpgrade` field is only valid when `featureSet: CustomNoUpgrade`

This configuration will be parsed during MicroShift startup and the feature gate settings will be passed to the appropriate Kubernetes components via their command-line arguments or configuration files.

#### Component Integration

Feature gates will be applied to the following MicroShift components, which are integrated into the MicroShift runtime rather than running as separate processes:
- **kubelet**: Feature gates specified in kubelet configuration file
- **kube-apiserver**: Feature gates specified in kube-apiserver configuration file
- **kube-controller-manager**: Feature gates specified in kube-controller-manager configuration file
- **kube-scheduler**: Feature gates specified in kube-scheduler configuration file

MicroShift will generate or modify the appropriate configuration files for each component based on the user's feature gate settings in the MicroShift configuration file.

#### Validation and Error Handling

- Invalid feature gate names will be caught by the Kubernetes components themselves
- MicroShift will log configuration parsing errors but delegate feature gate validation to the components
- Conflicting feature gate settings between user configuration and component requirements will result in component startup failures with appropriate error messages

### Risks and Mitigations

**Risk: Experimenting with Unstable Alpha Features**
Users experimenting with alpha-stage feature gates may encounter instability or data loss in their MicroShift deployments.

*Mitigation:* Emphasize that experimentation should be conducted in non-production environments. Feature gate validation will be handled by the Kubernetes components themselves.

**Risk: Configuration Errors**
Invalid feature gate configurations could prevent MicroShift components from starting.

*Mitigation:* Leverage Kubernetes component validation for feature gate names and values. Provide clear error messages and documentation for troubleshooting configuration issues.

**Risk: Security Implications**
Some feature gates may expose new attack vectors or security vulnerabilities.

*Mitigation:* Security review will follow standard MicroShift processes. Feature gates that fundamentally conflict with MicroShift's security model will be documented as unsupported.

### Drawbacks

**Increased Configuration Complexity**
Adding feature gate configuration increases the complexity of MicroShift's configuration surface area. Users must understand both the feature gates themselves and their potential interactions, which could lead to misconfigurations in edge deployments where troubleshooting access is limited.

**Support Complexity**
Enabling alpha and beta features through user configuration means support teams may encounter issues related to experimental functionality that behaves differently across Kubernetes versions or has incomplete implementations.

**Edge Device Risk**
Edge deployments often have limited remote access for troubleshooting. If users enable experimental feature gates that cause instability, recovering these devices may require physical access or complex recovery procedures.

**Upgrade Limitations and Irreversible Changes**
Enabling `TechPreviewNoUpgrade`, `DevPreviewNoUpgrade`, or `CustomNoUpgrade` feature sets cannot be undone and prevents both minor version updates and major upgrades. Once enabled, the cluster permanently loses the ability to perform standard updates. These feature sets are explicitly not recommended for production clusters due to their irreversible nature and update limitations, which conflicts with the typical edge deployment requirement for reliable, long-term operation and maintenance.

## Alternatives (Not Implemented)

No significant alternatives were considered for this enhancement. The configuration file approach aligns with MicroShift's existing patterns and provides the required user-configurable feature gates with automated OpenShift alignment.

## Open Questions [optional]

1. **How does OpenShift handle upgrades when custom feature gates are configured?**

   This requires clarification of OpenShift's actual implementation behavior:
   - Does OpenShift actively **block/prevent** upgrades when TechPreviewNoUpgrade/DevPreviewNoUpgrade/CustomNoUpgrade is configured?
   - Or does OpenShift **allow** upgrades to proceed but the resulting cluster becomes unsupported?

   Understanding OpenShift's approach will inform whether MicroShift should implement active blocking logic (pre-upgrade checks that fail) or simply document that upgrades with custom feature gates are unsupported while allowing them to proceed technically.

2. **How should feature gate compatibility be validated across MicroShift versions?**

   Unlike OpenShift which has extensive CI testing across feature combinations, MicroShift may have limited resources for testing all feature gate combinations across version upgrades. The approach for ensuring compatibility and providing user guidance needs definition.

## Test Plan

The testing strategy focuses on verifying the passthrough functionality - that custom feature gate configurations are correctly parsed and passed to the appropriate Kubernetes components. Since this is strictly a configuration passthrough feature, testing validates the parsing and delivery mechanism rather than feature gate functionality itself.

### Unit Tests

**Configuration Parsing:**
- Validate parsing of `featureSet` values (TechPreviewNoUpgrade, DevPreviewNoUpgrade, CustomNoUpgrade, Default)
- Test parsing of `customNoUpgrade.enabled` and `customNoUpgrade.disabled` lists
- Verify configuration schema validation and error handling for malformed configurations
- Test default behavior when feature gates section is not configured

**Component Configuration Generation:**
- Test that feature gates are correctly written to kubelet configuration files
- Verify feature gates are properly formatted in kube-apiserver configuration
- Test feature gates are correctly applied to kube-controller-manager configuration
- Validate feature gates are properly set in kube-scheduler configuration
- Test that feature gates are applied to the correct components based on their scope

### Robot Framework Integration Tests

**Passthrough Verification:**
- Test that custom feature gates specified in MicroShift configuration appear in component configurations after service restart
- Verify TechPreviewNoUpgrade and DevPreviewNoUpgrade presets result in correct feature gates being passed to all components
- Test CustomNoUpgrade configuration with specific enabled/disabled lists are correctly applied to component configurations
- Validate that configuration changes only take effect after MicroShift service restart

**Configuration Error Handling:**
- Test MicroShift behavior with invalid feature gate names (passthrough with component validation)
- Verify appropriate error reporting when components reject invalid feature gate configurations
- Test handling of conflicting settings (same feature gate in both enabled and disabled lists)

### Testing Scope Limitations

**Component Behavior Verification:**
This enhancement does not test whether feature gates actually modify Kubernetes component behavior - that is the responsibility of upstream Kubernetes and OpenShift testing. Testing is limited to verifying the configuration passthrough mechanism works correctly.

**Upgrade Testing:**
Since upgrades are not supported when custom feature gates are configured, no additional upgrade testing is required for this enhancement. Default upgrade behavior without custom feature gates is already covered by existing MicroShift test suites.

## Graduation Criteria

The feature is planned to be released as GA directly.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

- Ability to utilize the enhancement end to end
- End user documentation completed and published
- Sufficient test coverage including Robot Framework integration tests
- Available by default
- End-to-end tests validating configuration passthrough functionality

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

**Default Configuration (no custom feature gates):**
Upgrades and downgrades proceed normally using standard MicroShift procedures with no additional considerations for feature gate handling.

**Custom Feature Gate Configurations:**
Upgrades and downgrades are not supported when custom feature gates are configured (TechPreviewNoUpgrade, DevPreviewNoUpgrade, or CustomNoUpgrade). Once custom feature gates are enabled, this configuration cannot be reverted - it is a permanent, one-way operation that permanently disables upgrade capability.

This limitation aligns with OpenShift's approach where TechPreviewNoUpgrade, DevPreviewNoUpgrade, and CustomNoUpgrade feature sets are irreversible and explicitly prevent cluster upgrades to avoid compatibility issues with experimental features.

## Version Skew Strategy

This enhancement introduces upgrade limitations when custom feature gates are configured to prevent compatibility issues across version boundaries.

### Default Configuration
When no custom feature gates are configured, standard MicroShift version skew handling applies with no additional considerations.

### Custom Feature Gate Limitations
When custom feature gates are configured (TechPreviewNoUpgrade, DevPreviewNoUpgrade, or CustomNoUpgrade), upgrades and downgrades between minor versions are not expected to work. Users must remove custom feature gate configurations before attempting minor version changes.

### Component Version Alignment
All Kubernetes components (kubelet, kube-apiserver, kube-controller-manager, kube-scheduler) are packaged together within each MicroShift release, eliminating internal component version skew concerns. Feature gate configuration is applied during startup with no runtime coordination required between components.

### Feature Gate Inconsistencies Between Components
It is possible that one component's feature gate settings disable an existing default feature gate while another component enables it, creating inconsistent behavior across components. However, resolving such inconsistencies is not within the scope of this proposal - this enhancement provides a passthrough mechanism only and does not validate feature gate compatibility between components.

## Operational Aspects of API Extensions

This enhancement does not introduce any API extensions (CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers). The feature operates entirely through configuration file changes and does not modify the OpenShift API surface or behavior.

All operational aspects are handled through existing MicroShift configuration mechanisms and component startup procedures.

## Support Procedures

### Detecting Feature Gate Configuration Issues

**MicroShift Service Startup Failures:**
- **Symptoms**: MicroShift service fails to start after configuration changes
- **Log locations**: `journalctl -u microshift.service`
- **Error patterns**: Component startup failures with feature gate validation errors
- **Detection**: Service status shows failed state, component logs show unknown feature gate names

**Component-Specific Failures:**
- **kubelet errors**: Check `journalctl -u microshift.service` for kubelet initialization failures
- **kube-apiserver errors**: Look for API server startup errors in MicroShift service logs
- **Controller/scheduler errors**: Component initialization failures logged in MicroShift service output

### Disabling Feature Gate Configuration

**Remove Custom Feature Gates:**
1. Edit `/etc/microshift/config.yaml`
2. Remove or comment out the `featureGates` section
3. Restart MicroShift service: `sudo systemctl restart microshift`

**Reset to Default Configuration:**
```yaml
# Remove entire featureGates section or set to:
featureGates:
  featureSet: Default
```

**Consequences of Disabling:**
- **Cluster health**: No impact on core MicroShift functionality
- **Existing workloads**: Workloads using experimental features may lose functionality
- **New workloads**: Will use default feature gate behavior only

### Edge Environment Troubleshooting

**Remote Diagnostics:**
- Feature gate configuration issues are logged in standard MicroShift service logs
- Use `microshift get nodes` to verify basic cluster functionality
- Check component status through `microshift get pods -A` for system pod health

**Recovery Procedures:**
- Configuration changes only require MicroShift service restart, not full system reboot
- Invalid configurations prevent service startup but do not affect system stability
- Greenboot integration ensures automatic rollback if feature gates prevent successful startup

### Graceful Failure and Recovery

**Configuration Changes:**
- Invalid feature gate configurations fail fast during service startup
- No partial application of settings - either all feature gates apply or none do
- Recovery is immediate upon fixing configuration and restarting service
- No data consistency risks from feature gate configuration changes

## Infrastructure Needed [optional]

No additional infrastructure is needed for this enhancement. The feature uses existing MicroShift configuration mechanisms and testing infrastructure.