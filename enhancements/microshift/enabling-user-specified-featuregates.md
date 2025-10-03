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

MicroShift disables most feature gates by default while hardcoding only a few relevant ones, and lacks a controlled mechanism for users to experiment with additional feature gates or override defaults. This enhancement proposes adding configuration support for feature gates through the MicroShift configuration file. In OpenShift, users configure feature gates through the FeatureGate API, where operators independently filter featureGates for their components based on the central FeatureGate API 'cluster' instance. In contrast, MicroShift users will specify feature gates directly in the configuration file (`/etc/microshift/config.yaml`), and MicroShift will pass all user-specified featureGates to the kube-apiserver, which then propagates them to other Kubernetes components (kubelet, kube-controller-manager, kube-scheduler). This capability will enable users to experiment with alpha and beta Kubernetes features like CPUManager's `prefer-align-cpus-by-uncorecache` in a supported and deterministic way, addressing edge computing use cases where users want to evaluate advanced resource management capabilities.

## Motivation

MicroShift users in edge computing environments want to experiment with upcoming Kubernetes features that are in alpha or beta stages to evaluate their potential benefits for specific use cases. Currently, users cannot configure feature gates in a supported way, preventing them from experimenting with capabilities like advanced CPU management, enhanced scheduling features, or experimental storage options that might improve performance in their resource-constrained edge environments.

### User Stories

* As a MicroShift administrator, I want to configure feature gates through the MicroShift configuration file (`/etc/microshift/config.yaml`), so that I can experiment with alpha/beta features in a controlled and supported manner consistent with MicroShift's file-based configuration approach.

### Goals

* Enable user configuration of feature gates through the MicroShift configuration file
* Provide a controlled and deterministic way to experiment with alpha and beta features

### Non-Goals

* Modify MicroShift's existing feature gate defaults
* Vetting custom feature gates for compatibility with MicroShift
* Validating custom feature gate settings for correctness, e.g. spelling, case, and punctuation
* Automatic enablement of experimental features without explicit user configuration
* Providing upgrade support to customized clusters

## Proposal

This enhancement proposes adding feature gate configuration support to MicroShift by extending `/etc/microshift/config.yaml` with a configuration schema inspired by OpenShift's FeatureGate custom resource specification. In OpenShift, users configure feature gates through the FeatureGate API, and operators independently filter featureGates before applying them to their components. MicroShift takes a different approach aligned with its file-based configuration philosophy: users specify feature gates directly in the configuration file, and MicroShift passes all user-specified featureGates to the kube-apiserver, which then handles propagation to other Kubernetes components.

The implementation includes:

1. **FeatureGate Configuration Schema**: Extend MicroShift's configuration file to include `featureGates` section with fields inspired by OpenShift's FeatureGate CRD spec (`featureSet` and `customNoUpgrade`)
2. **Predefined Feature Sets**: Support for predefined feature sets like `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade`
3. **Custom Feature Gates**: Support for individual feature gate enablement/disablement via `customNoUpgrade` configuration
4. **API Server Propagation**: All configured featureGates will be passed to the kube-apiserver, which handles propagation to other Kubernetes components (kubelet, kube-controller-manager, kube-scheduler)

This approach ensures that users can experiment with feature gate capabilities while maintaining MicroShift's file-based configuration pattern instead of requiring API interactions.

### Workflow Description

**MicroShift Administrator** is a human user responsible for configuring and managing MicroShift deployments.

#### User Configuration Workflow
1. MicroShift Administrator identifies a need for specific feature gates (e.g., `CPUManagerPolicyAlphaOptions`)
2. Administrator chooses between two configuration approaches:
   - **Predefined Feature Set**: Configure `featureGates.featureSet: TechPreviewNoUpgrade` or `DevPreviewNoUpgrade` for a curated set of preview features
   - **Custom Feature Gates**: Configure `featureGates.featureSet: CustomNoUpgrade` and specify individual features in `featureGates.customNoUpgrade.enabled/disabled` lists
3. Administrator updates `/etc/microshift/config.yaml` with the chosen configuration
4. Administrator restarts MicroShift service
5. MicroShift parses the FeatureGate configuration and passes all settings to the kube-apiserver
6. The kube-apiserver propagates the feature gates to other Kubernetes components (kubelet, kube-controller-manager, kube-scheduler)
7. Each component processes the featureGates and enables/disables the features it supports according to the configured state

### API Extensions

This enhancement extends MicroShift's configuration file schema only. No new CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers are introduced. Unlike OpenShift where users interact with the FeatureGate API to configure feature gates, MicroShift users will configure feature gates directly in the `/etc/microshift/config.yaml` file. The configuration file structure will be extended to include a `featureGates` section with a structure inspired by the OpenShift FeatureGate CRD specification, maintaining MicroShift's file-based configuration approach.

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

The MicroShift configuration file will be extended to include a new `featureGates` section with a structure inspired by the OpenShift FeatureGate CRD specification. While OpenShift users configure feature gates through the Kubernetes API (e.g., `oc edit featuregate cluster`), MicroShift users will configure them directly in `/etc/microshift/config.yaml`:

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

#### FeatureSet Definitions

Each OpenShift release image provides one manifest per FeatureSet profile. This enables the existing MicroShift rebase automation to keep current with OpenShift feature-set lists. The pertinent manifests for MicroShift are:

- `0000_50_cluster-config-api_featureGate-SelfManagedHA-Default.yaml`
- `0000_50_cluster-config-api_featureGate-SelfManagedHA-DevPreviewNoUpgrade.yaml`
- `0000_50_cluster-config-api_featureGate-SelfManagedHA-TechPreviewNoUpgrade.yaml`

#### Component Integration

In OpenShift, users configure feature gates by creating FeatureGate API objects and operators independently filter featureGates for their respective components. MicroShift adopts a different model aligned with its file-based configuration approach: users specify feature gates in `/etc/microshift/config.yaml`, and MicroShift passes all user-specified featureGates to the kube-apiserver, which then handles the propagation to other components. This approach ensures all components receive the necessary feature gate settings without requiring MicroShift to implement complex filtering logic.

The propagation flow works as follows:
1. **MicroShift → kube-apiserver**: MicroShift passes all configured feature gates to the kube-apiserver
2. **kube-apiserver → Other Components**: The kube-apiserver propagates feature gates to:
   - **kubelet**: Through the Node configuration
   - **kube-controller-manager**: Through internal cluster configuration
   - **kube-scheduler**: Through internal cluster configuration

Each component will then internally process these settings according to its capabilities. This leverages Kubernetes' native propagation mechanisms rather than requiring MicroShift to directly configure each component.

#### Comparison with OpenShift's FeatureGate Architecture

**OpenShift Approach:**
- Users configure feature gates through the FeatureGate API by creating/modifying FeatureGate instances
- The FeatureGate API instance named 'cluster' serves as the single source of truth for all featureGates across the cluster
- Each operator independently reads the 'cluster' FeatureGate instance and filters the featureGates relevant to its managed components
- Operators determine which featureGates to pass to their components and handle component restarts when featureGate values change
- This provides fine-grained control but requires complex operator logic for filtering and lifecycle management

**MicroShift Approach:**
- Users configure feature gates through the configuration file (`/etc/microshift/config.yaml`) rather than through an API
- Configuration file-based featureGate specification without a central API object
// TODO this is unclear on openshift. i saw that the MCO watches the FeatureGate API and will restart kubelets, but I don't know if this applies to all components. It's probably not worth mentioning here though since it doesn't really change the design
- Single-point propagation through kube-apiserver to all other Kubernetes components
- Simpler implementation leveraging kube-apiserver's native propagation mechanisms
- Component restart handled through MicroShift service restart rather than individual operator reconciliation

#### Validation and Error Handling

- **Configuration Parsing**: MicroShift will validate the structural correctness of the configuration (YAML syntax, required fields)
- **API Server Validation**: The kube-apiserver does not validate the feature gates it receives from MicroShift before propagating them
- **Component-level Validation**: Each Kubernetes component will validate the feature gates it recognizes
- **Error Reporting**: Components will log errors or warnings for invalid feature gate configurations
- **Startup Failures**: May occur when featureGate settings conflict (i.e. a featureGate is both enabled and disabled)

### Risks and Mitigations

**Risk: Experimenting with Unstable Alpha Features**
Users experimenting with alpha-stage feature gates may encounter instability or data loss in their MicroShift deployments.

*Mitigation:* Emphasize that experimentation should be conducted in non-production environments. Feature gate validation will be handled by the Kubernetes components themselves.

**Risk: Configuration Errors**
Invalid feature gate configurations in the MicroShift configuration file could prevent MicroShift components from starting.

*Mitigation:* Kubernetes components inherently ignore unrecognized feature gate names, so typos or incorrect names will not cause failures. Only invalid values for recognized gates can cause issues. Components provide clear error messages for such cases, and documentation will guide troubleshooting.

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

The testing strategy focuses on verifying the propagation functionality - that custom feature gate configurations are correctly parsed from the MicroShift configuration file and passed to the kube-apiserver, which then handles propagation to other Kubernetes components. Testing validates the parsing and delivery mechanism rather than feature gate functionality itself.

### Unit Tests

**Configuration Parsing:**
- Validate parsing of `featureSet` values (TechPreviewNoUpgrade, DevPreviewNoUpgrade, CustomNoUpgrade, Default)
- Test parsing of `customNoUpgrade.enabled` and `customNoUpgrade.disabled` lists
- Verify configuration schema validation and error handling for malformed configurations
- Test default behavior when feature gates section is not configured

**API Server Configuration:**
- Verify feature gates are properly formatted in the kube-apiserver configuration

### Robot Framework Integration Tests

**Universal Propagation Verification:**
- Test that custom feature gates specified in MicroShift configuration appear after service restart
- Verify TechPreviewNoUpgrade and DevPreviewNoUpgrade presets results in their feature gates being passed to kube-apiserver

**Configuration Error Handling:**
- Verify error reporting from embedded components in MicroShift logs  
- Test handling of conflicting settings (same feature gate in both enabled and disabled lists) at the kube-apiserver level
- Verify that configuration file parsing errors are clearly reported to users

### Testing Scope Limitations

**Component Behavior Verification:**
This enhancement does not test whether feature gates actually modify Kubernetes component behavior - that is the responsibility of upstream Kubernetes testing. Testing is limited to verifying that MicroShift correctly passes feature gates to the kube-apiserver and that the kube-apiserver's native propagation mechanism distributes them to other components correctly.

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

Similar to OpenShift, the TechPreviewNoUpgrade, DevPreviewNoUpgrade, and CustomNoUpgrade feature sets are irreversible and explicitly prevent cluster upgrades to avoid compatibility issues with experimental features.

## Version Skew Strategy

This enhancement introduces upgrade limitations when custom feature gates are configured to prevent compatibility issues across version boundaries.

### Default Configuration
When no custom feature gates are configured, standard MicroShift version skew handling applies with no additional considerations.

### Custom Feature Gate Limitations
When custom feature gates are configured (TechPreviewNoUpgrade, DevPreviewNoUpgrade, or CustomNoUpgrade), upgrades and downgrades between minor versions are not expected to work. Users must remove custom feature gate configurations before attempting minor version changes.

### Component Version Alignment
All Kubernetes components (kubelet, kube-apiserver, kube-controller-manager, kube-scheduler) are packaged together within each MicroShift release, eliminating internal component version skew concerns. Feature gate configuration is read from the MicroShift configuration file and passed to the kube-apiserver during startup, which then handles propagation to other components using Kubernetes' native mechanisms.

### Feature Gate Consistency Across Components
The kube-apiserver's native propagation mechanism ensures consistent feature gate distribution to all Kubernetes components. While individual components may recognize different subsets of feature gates based on their capabilities, the kube-apiserver ensures all components receive the same feature gate configuration from the MicroShift configuration file. This enhancement relies on the kube-apiserver's propagation logic and does not implement additional validation for feature gate compatibility between components.

## Operational Aspects of API Extensions

// TODO the configuration schema is being modified. Backwards compatibility must be maintained

All operational aspects are handled through existing MicroShift configuration mechanisms and component startup procedures.

## Support Procedures

### Detecting Feature Gate Configuration Issues

**MicroShift Service Startup Failures:**
- **Symptoms**: MicroShift service fails to start after configuration changes
- **Log locations**: `journalctl -u microshift.service`
- **Error patterns**: Component startup failures with feature gate validation errors
- **Detection**: Service status shows failed state, component logs show unknown feature gate names

**Component-Specific Failures:**
- **kube-apiserver errors**: Look for API server startup errors in `journalctl -u microshift.service` - these are critical as the apiserver handles propagation

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