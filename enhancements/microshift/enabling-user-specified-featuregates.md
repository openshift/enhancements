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

MicroShift disables most feature gates by default while hardcoding only a few relevant ones, and lacks a controlled
mechanism for users to experiment with additional feature gates or override defaults. This enhancement proposes adding
configuration support for feature gates through the MicroShift configuration file. In OpenShift, users configure feature
gates through the FeatureGate API. In contrast, MicroShift users will specify feature gates directly in the
configuration file (`/etc/microshift/config.yaml`), and MicroShift will pass all user-specified featureGates to the
kube-apiserver, which then propagates them to other Kubernetes components (kubelet, kube-controller-manager,
kube-scheduler). This capability will enable users to experiment with alpha and beta Kubernetes features like
CPUManager's `prefer-align-cpus-by-uncorecache` in a supported and deterministic way, addressing edge computing use
cases where users want to evaluate advanced resource management capabilities.

## Motivation

MicroShift users in edge computing environments want to experiment with upcoming Kubernetes features that are in alpha
or beta stages to evaluate their potential benefits for specific use cases. Currently, users cannot configure feature
gates in a supported way, preventing them from experimenting with capabilities like advanced CPU management, enhanced
scheduling features, or experimental storage options that might improve performance in their resource-constrained edge
environments.

### User Stories

* As a MicroShift administrator, I want to configure feature gates through the MicroShift configuration file
  (`/etc/microshift/config.yaml`), so that I can experiment with alpha/beta features in a controlled and supported
  manner consistent with MicroShift's file-based configuration approach.

### Goals

* Enable user configuration of feature gates through the MicroShift configuration file

### Non-Goals

* Modify MicroShift's existing feature gate defaults
* Vet custom feature gates for compatibility with MicroShift
* Validate custom feature gate settings for correctness, e.g. spelling, case, and punctuation
* Providing upgrade support to customized nodes

## Proposal

This enhancement proposes adding feature gate configuration support to MicroShift by extending `/etc/microshift/config.yaml` with a configuration inspired by OpenShift's FeatureGate custom resource specification. In OpenShift, users configure feature gates through the FeatureGate API, which is then propogated to sub-components (e.g. kube-apiserver, kubelet). In some cases, sub-component operators are also involved in the propagation of feature gate configurations and service restarts, such as the MCO configuring and restarting kubelets.

MicroShift does not deploy these operators and must a different approach which is aligned with its file-based configuration philosophy: users specify feature gates directly in the configuration file, and MicroShift passes all user-specified featureGates to the kube-apiserver, which then handles propagation to other Kubernetes components. Service restarts are executed by the cluster admin by restarting the MicroShift process.

> **Important!** The use of custom feature gates on MicroShift is irreversible and renders a cluster unable to be upgraded. This feature should only be used for testing alpha/beta features and should never be used in productions. Upgraded clusters will fail to start.

The implementation includes:

1. **FeatureGate Configuration**: Extend MicroShift's configuration file to include `apiServer.featureGates` section with fields inspired by OpenShift's FeatureGate CRD spec (`featureSet` and `customNoUpgrade`)
2. **Predefined Feature Sets**: Support for predefined feature sets like `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade`
3. **Custom Feature Gates**: Support for individual feature gate enablement/disablement via `customNoUpgrade` configuration
4. **API Server Propagation**: All configured featureGates will be passed to the kube-apiserver, which handles propagation to other Kubernetes components (kubelet, kube-controller-manager, kube-scheduler). Service restarts are the responsibility of the cluster admin.
5. **Feature Gate Config Changes**: OpenShift prevents users from reverting custom feature gates via spec validation rules. This is an not option for the MicroShift config. Instead, MicroShift will check for custom feature gates at startup. If customizations exist, MicroShift will write the feature gate config data to `/var/lib/microshift/no-upgrade`. During initialization MicroShift will read `/var/lib/microshift/no-upgrade` and compare it to `/etc/microshift/config.yaml` and `/etc/microshift/config.d/`. If the user's feature gate configuration differs from `/var/lib/microshift/no-upgrade`, MicroShift will log a fatal error.
6. **Breaking Node Upgrades**: Upgrades on OpenShift are preempted at the cluster level by the cluster-version-operator, in conjunction with other OpenShift operators. The lack of OpenShift operators on MicroShift means there is no reliable way to preempt node upgrades. MicroShift will check for `/var/lib/microshift/no-upgrade` and check the current MicroShift version against `/var/lib/microshift/version`. If `/var/lib/microshift/no-upgrade` exists and the version differs, MicroShift will log the error and fail.

### Workflow Description

**MicroShift Administrator** is a human user responsible for configuring and managing MicroShift deployments.

#### User Configuration Workflow

##### First Time Configuring Feature Gates
1. MicroShift Administrator identifies a need for specific feature gates (e.g., `CPUManagerPolicyAlphaOptions`)
2. Administrator chooses between two configuration approaches:
   - **Predefined Feature Set**: Configure `apiServer.featureGates.featureSet: TechPreviewNoUpgrade` or `DevPreviewNoUpgrade` for a curated set of preview features
   - **Custom Feature Gates**: Configure `apiServer.featureGates.featureSet: CustomNoUpgrade` and specify individual features in `apiServer.featureGates.customNoUpgrade.enabled/disabled` lists
3. Administrator updates `/etc/microshift/config.yaml` with the chosen configuration
4. Administrator restarts MicroShift service
5. MicroShift detects the custom FeatureGate configuration.
6. MicroShift writes feature gate config to `/var/lib/microshift/no-upgrade`, and logs the event
7. The kube-apiserver propagates the feature gates to other Kubernetes components (kubelet, kube-controller-manager, kube-scheduler)
8. Each component processes the featureGates and enables/disables the features it supports according to the configured state

##### Attempt to Revert Custom Feature Gates
1. Administrator decides to revert custom feature gates (e.g., wants to return to default settings)
2. Administrator modifies `/etc/microshift/config.yaml` to remove or change feature gate configuration
3. Administrator restarts MicroShift service
4. MicroShift detects `/var/lib/microshift/no-upgrade` with differring feature gate config.
5. MicroShift logs a fatal error that custom feature gates cannot be reverted or changed once applied

##### Attempt to Upgrade Node with Custom Feature Gates
1. Administrator deploys a node upgrade.
2. Administrator restarts MicroShift service
3. MicroShift detects the custom FeatureGate configuration.
4. If `/var/lib/microshift/no-upgrade` exists (indicating custom feature gates are configured), then the upgrade is blocked with a fatal error indicating custom feature gates break upgrades
5. Administrator must either:
   - Revert the upgrade back to the prior version, OR
   - Wipe MicroShift's state (`$ sudo microshift-cleanup-data --all`) and restart MicroShift service (`$ sudo systemctl restart microshift`). This returns the node to a supported state.

### API Extensions

This enhancement extends MicroShift's configuration file schema only. No new CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers are introduced. Unlike OpenShift where users interact with the FeatureGate API to configure feature gates, MicroShift users will configure feature gates directly in the `/etc/microshift/config.yaml` file.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is not applicable to Hypershift/Hosted Control Planes as feature gate configuration in hosted environments would be managed through the hosting cluster's OpenShift FeatureGate API rather than through MicroShift configuration.

#### Standalone Clusters

This enhancement is not applicable to standalone clusters.

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

The MicroShift configuration file will be extended to include a new `apiServer.featureGates` section with a structure inspired by the OpenShift FeatureGate CRD specification. MicroShift users will configure feature gates in `/etc/microshift/config.yaml`:

**Predefined Feature Set Configuration:**
```yaml
apiServer:
  featureGates:
    featureSet: TechPreviewNoUpgrade
```

**Custom Feature Gates Configuration:**
```yaml
apiServer:
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

See [Validation and Error Handling](#validation-and-error-handling) config validation details

#### FeatureSet Definitions

Each OpenShift release image provides one manifest per FeatureSet profile. This enables the existing MicroShift rebase automation to keep current with OpenShift feature-set lists. The pertinent manifests for MicroShift are:

- `0000_50_cluster-config-api_featureGate-SelfManagedHA-DevPreviewNoUpgrade.yaml`
- `0000_50_cluster-config-api_featureGate-SelfManagedHA-TechPreviewNoUpgrade.yaml`

#### Component Integration

In OpenShift, users configure feature gates by creating FeatureGate API objects and operators independently filter featureGates for their respective components. MicroShift adopts a different model aligned with its file-based configuration approach: users specify feature gates in `/etc/microshift/config.yaml`, and MicroShift passes all user-specified featureGates to the kube-apiserver.

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
- The FeatureGate API instance named 'cluster' serves as the source of truth for all featureGates across the cluster
- The kube-apiserver detects a CRUD event on the FeatureGate API, parses all FeatureGate API instances, and communicates the FeatureGate values to cluster components
- Operators like the Machine Config Operator also detect the CRUD event and will restart the the operand component if necessary
- This provides fine-grained control but requires complex operator logic for filtering and lifecycle management

**MicroShift Approach:**
- Users configure feature gates through the configuration file (`/etc/microshift/config.yaml`) rather than through an API
- If the new config is custom feature gates, MicroShift passes this to the kube-apiserver via the kube-apiserver config file
- If the new config is for a feature set, MicroShift extracts the feature gates from the respective feature set manifest (embedded) and passes them to the kube-apiserver via the kube-apiserver config file
- Simpler implementation leveraging kube-apiserver's native propagation mechanisms
- Component restart handled through MicroShift service restart rather than individual operator reconciliation

#### Validation and Error Handling

- **Configuration Parsing**: MicroShift will replicate OpenShift's schema rules as start-time validation checks:
  - **Conflicting Feature Gate Settings**: A feature gate appears in both `.apiServer.featureGates.customNoUpgrade.enabled` and `.apiServer.featureGates.customNoUpgrade.disabled`
  - **Conflicting Feature Set Settings**: Feature gates are defined under `.apiServer.featureGates.customNoUpgrade.[enabled|disabled]` but `.apiServer.featureGates.featureSet:` is not `customNoUpgrade`.
- **API Server Validation**: The kube-apiserver does not validate the feature gates it receives from MicroShift before propagating them. This behavior is the same on OpenShift
- **Component-level Validation**: Unrecognized feature-gate values are ignored by components. The component will only log them as a warning
- **Startup Failures**: May occur when featureGate settings conflict (i.e. a featureGate is both enabled and disabled)
- **Upgrade Failure**: `/var/lib/microshift/no-upgrade` exists and MicroShift version does not equal `/var/lib/microshift/version`
- **Custom Features cannot be Reverted or Changed**: `/var/lib/microshift/no-upgrade` data does not match `/etc/microshift/config` and `/etc/microshift/config.d`

### Risks and Mitigations

**Risk: Experimenting with Features**
Users experimenting feature gates may encounter instability or data loss in their MicroShift deployments.

*Mitigation:* Emphasize that experimentation should be conducted in non-production environments. Feature gate validation will be handled by the Kubernetes components themselves.

**Risk: Configuration Errors**
Invalid feature gate configurations in the MicroShift configuration file could prevent MicroShift components from starting.

*Mitigation:* Kubernetes components inherently ignore unrecognized feature gate names, so typos or mispellings may not cause failures. Components provide clear warning messages for such cases, and documentation will guide troubleshooting.

### Drawbacks

**Increased Configuration Complexity**
Adding feature gate configuration increases the complexity of MicroShift's configuration surface area. Users must understand both the feature gates themselves and their potential interactions, which could lead to misconfigurations in edge deployments where troubleshooting access is limited. Again, users must be aware that custom feature gates are for experimentation only, are irreversible, and make a cluster un-upgradeable.

**Support Complexity**
Enabling alpha and beta features through user configuration means support teams may encounter issues related to experimental functionality that behaves differently across Kubernetes versions or has incomplete implementations.

**Edge Device Risk**
Edge deployments often have limited remote access for troubleshooting. If users enable experimental feature gates that cause instability, recovering these devices may require physical access or complex recovery procedures.

**Upgrade Limitations and Irreversible Changes**
Once enabled, `TechPreviewNoUpgrade`, `DevPreviewNoUpgrade`, or `CustomNoUpgrade` feature sets CANNOT be undone and the cluster CANNOT be upgraded. These feature sets are NOT RECOMMENDED FOR PRODUCTION CLUSTERS.

## Alternatives (Not Implemented)

Utilizing the FeatureGate API on MicroShift is rejected as an alternative approach because it requires additional operators to manage both the API and the kubernetes components. At best, this would increase the complexity of cluster component lifecycle management and increase cluster overhead. This approach would also be a departure from the current model for user-defined configuration.

## Open Questions [optional]

1. **How does OpenShift handle upgrades when custom feature gates are configured?**

   This requires clarification of OpenShift's actual implementation behavior:
   - Does OpenShift actively **block/prevent** upgrades when TechPreviewNoUpgrade/DevPreviewNoUpgrade/CustomNoUpgrade is configured?
   - Or does OpenShift **allow** upgrades to proceed but the resulting cluster becomes unsupported?

  OpenShift actively prevents upgrades of clusters with customize features gates. OpenShift operators work together to communicate if any component has a had a custom feature gate applied. If so, the cluster-version-operator marks the cluster as un-upgradeable.

2. **How should feature gate compatibility be validated across MicroShift versions?**

   Unlike OpenShift which has extensive CI testing across feature combinations, MicroShift may have limited resources for testing all feature gate combinations across version upgrades. The approach for ensuring compatibility and providing user guidance needs definition.

   **Answer:** OpenShift does not validate feature gate compatibility and designates any customization of feature gate flags as unsupported. MicroShift will adopt this philosphy as well.

## Test Plan

The testing strategy focuses on verifying the propagation functionality - that custom feature gate configurations are correctly parsed from the MicroShift configuration file and passed to the kube-apiserver, which then handles propagation to other Kubernetes components. Testing validates the parsing and delivery mechanism rather than feature gate functionality itself.

### Unit Tests

**Configuration Parsing:**
- Validate parsing of `featureSet` values (TechPreviewNoUpgrade, DevPreviewNoUpgrade, CustomNoUpgrade)
- Test parsing of `customNoUpgrade.enabled` and `customNoUpgrade.disabled` lists
- Verify configuration schema validation and error handling for malformed configurations

**API Server Configuration:**
- Verify feature gate pass-through retains string formating in the kube-apiserver configuration

### Documentation Validation

**Validating Example Configs**
- Validation testing of featureGate config example(s) in documentation for correct syntax.

### Robot Framework Integration Tests

**Universal Propagation Verification:**
- Test that custom feature gates specified in MicroShift configuration appear after service restart
- Verify TechPreviewNoUpgrade and DevPreviewNoUpgrade presets results in their feature gates being passed to kube-apiserver

**Configuration Error Handling:**
- Verify error reporting from embedded components in MicroShift logs
- Test handling of conflicting settings (same feature gate in both enabled and disabled lists) by MicroShift
- Verify that configuration file parsing errors are clearly reported to users

### Testing Scope Limitations

**Component Behavior Verification:**
This enhancement does not test whether feature gates actually modify Kubernetes component behavior - that is the responsibility of upstream Kubernetes testing. Testing is limited to verifying that MicroShift correctly passes feature gates to the kube-apiserver.

**Upgrade Breaking Test:**
A test scenario will verify that MicroShift fails after upgrades when custom feature gates are configured:
- Verify that nodes with TechPreviewNoUpgrade, DevPreviewNoUpgrade, or CustomNoUpgrade fail to start after upgrade
- Test that upgrade failures provide clear error messages indicating custom feature gates

**Custom Feature Gate Immutability Testing:**
A test scenario verifies that custom feature gate configurations cannot be modified or reverted once applied:
- Verify that customized feature gates are written to `/var/lib/microshift/no-upgrade` and that it's contents are correct
- Test that MicroShift logs fatal error when `/var/lib/microshift/no-upgrade` data does not match user config

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

The TechPreviewNoUpgrade, DevPreviewNoUpgrade, and CustomNoUpgrade feature sets are irreversible and
will cause MicroShift to fail after and upgrades.

## Version Skew Strategy

This enhancement introduces upgrade limitations when custom feature gates are configured to prevent compatibility issues across version boundaries.

Feature Sets defined by OpenShift are included in the OCP release image. Rebase automation will be extended to pull in these manifests and they will be embedded into the MicroShift binary at build time.

### Default Configuration
When no custom feature gates are configured, standard MicroShift version skew handling applies with no additional considerations.

### Custom Feature Gate Limitations
When custom feature gates are configured (TechPreviewNoUpgrade, DevPreviewNoUpgrade, or CustomNoUpgrade), upgrades and downgrades between minor versions are not expected to work. Users must remove custom feature gate configurations before attempting minor version changes.

### Feature Gate Consistency Across Components
Feature gate skew can occur between embedded components. On OpenShift, this is a non-issue. On MicroShift, it is a known issue that one component's default may be to disable a feature, while another comonpent enables it. This problem is tracked by [USHIFT-2813](https://issues.redhat.com/browse/USHIFT-2813). Solving this issue is outside the scope of this proposal.

## Operational Aspects of API Extensions

Any changes to the MicroShift configuration schema must be backwards compatible by at least y-2 minor versions.

## Support Procedures

### Detecting Feature Gate Configuration Issues

**MicroShift Service Startup Failures:**
- **Symptoms**: MicroShift service fails to start after configuration changes
- **Log locations**: `journalctl -u microshift.service`
- **Error patterns**: Component startup failures with feature gate validation errors
- **Detection**: Service status shows failed state, component logs show unknown feature gate names

### Reverting Custom Feature Gate Configurations To Default

**Recovery Procedures:**
- To restore MicroShift to a stable and supported state, users must run `$ sudo microshift-cleanup-data --all`, set `.apiServer.featureGates: {}`, and restart MicroShift

### Upgrade / Rollback
- Upgrades when custom feature gates are configured cause MicroShift to fail to start. See [Attempt to Upgrade Node with Custom Feature Gates](#attempt-to-upgrade-node-with-custom-feature-gates).
