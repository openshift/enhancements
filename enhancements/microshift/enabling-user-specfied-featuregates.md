---
title: enabling-user-specified-featuregates
authors:
  - TBD
reviewers:
  - TBD # MicroShift core team for configuration changes
  - TBD # Kubernetes upstream expert for feature gate implications
  - TBD # OpenShift platform team for alignment with OpenShift defaults
approvers:
  - TBD # MicroShift principal engineer
api-approvers:
  - None # Configuration file changes only, no API modifications
creation-date: 2025-01-XX # You'll need to fill in today's date
last-updated: 2025-01-XX
tracking-link:
  - TBD # Link to USHIFT-6080 or the main epic ticket
see-also:
  - TBD # Any related enhancements if applicable
---

# Enabling User-Specified FeatureGates in MicroShift

## Summary

MicroShift currently inherits feature gates from its Kubernetes and OpenShift upstream components but lacks a controlled mechanism for users to experiment with additional feature gates or override defaults. This enhancement proposes adding configuration support for Kubernetes and OpenShift feature gates through the MicroShift configuration file, while ensuring that default feature gates remain aligned with OpenShift during automated rebases. This capability will enable users to experiment with alpha and beta Kubernetes features like CPUManager's `prefer-align-cpus-by-uncorecache` in a supported and deterministic way, addressing edge computing use cases where users want to evaluate advanced resource management capabilities.

## Motivation

MicroShift users in edge computing environments want to experiment with upcoming Kubernetes features that are in alpha or beta stages to evaluate their potential benefits for specific use cases. Currently, users cannot configure feature gates in a supported way, preventing them from experimenting with capabilities like advanced CPU management, enhanced scheduling features, or experimental storage options that might improve performance in their resource-constrained edge environments.

Additionally, the lack of automated feature gate alignment during OpenShift rebases has caused issues like [USHIFT-2813](https://issues.redhat.com/browse/USHIFT-2813), where conflicting feature gate defaults between different Kubernetes components (such as kube-controller-manager and kube-apiserver) led to compatibility problems during version upgrades. This enhancement addresses both user configuration needs and operational stability requirements.

### User Stories

* As a MicroShift administrator, I want to experiment with the CPUManager `prefer-align-cpus-by-uncorecache` feature gate so that I can evaluate whether it improves CPU allocation for my high-performance computing workloads on edge devices with specific CPU topologies.

* As a MicroShift administrator, I want to configure feature gates through the MicroShift configuration file so that I can experiment with upcoming Kubernetes features in a controlled and supported manner.

* As a MicroShift developer, I want feature gates to remain automatically aligned with OpenShift defaults during rebases so that I can avoid compatibility issues and manual intervention during version upgrades.

* As an edge computing platform operator, I want to experiment with specific alpha/beta features across my development and testing environments so that I can evaluate their potential benefits before considering them for production use.

### Goals

* Enable user configuration of Kubernetes and OpenShift feature gates through the MicroShift configuration file
* Maintain automatic alignment of default feature gates with OpenShift during rebases
* Provide a controlled and deterministic way to experiment with alpha and beta features
* Prevent feature gate misalignment issues during Kubernetes version upgrades
* Support edge computing experimentation with advanced resource management features

### Non-Goals

* Modifying OpenShift's feature gate defaults or upstream Kubernetes behavior
* Supporting feature gates that fundamentally conflict with MicroShift's architecture
* Automatic enablement of experimental features without explicit user configuration for experimentation

## Proposal

This enhancement proposes adding feature gate configuration support to MicroShift by extending `/etc/microshift/config.yaml` to mirror OpenShift's FeatureGate custom resource specification. The configuration will support both predefined feature sets and custom feature gate combinations, ensuring consistency with OpenShift's FeatureGate API patterns.

The implementation includes:

1. **FeatureGate Configuration Schema**: Extend MicroShift's configuration file to include `featureGates` section matching the OpenShift FeatureGate CRD spec fields (`featureSet` and `customNoUpgrade`)
2. **Predefined Feature Sets**: Support for OpenShift's predefined feature sets like `TechPreviewNoUpgrade`
3. **Custom Feature Gates**: Support for individual feature gate enablement/disablement via `customNoUpgrade` configuration
4. **Automated Rebase Integration**: Maintain feature gate alignment with OpenShift defaults during rebases

This approach ensures that users can experiment with the same feature gate capabilities as OpenShift while maintaining MicroShift's file-based configuration pattern. Default feature gate values will continue to be inherited from OpenShift to ensure consistency across the platform.

### Workflow Description

**MicroShift Administrator** is a human user responsible for configuring and managing MicroShift deployments.

**MicroShift Developer** is a human user responsible for maintaining MicroShift codebase and rebases.

#### User Configuration Workflow
1. MicroShift Administrator identifies a need for specific feature gates (e.g., `CPUManagerPolicyAlphaOptions`)
2. Administrator chooses between two configuration approaches:
   - **Predefined Feature Set**: Configure `featureGates.featureSet: TechPreviewNoUpgrade` for a curated set of preview features
   - **Custom Feature Gates**: Configure `featureGates.featureSet: CustomNoUpgrade` and specify individual features in `featureGates.customNoUpgrade.enabled/disabled` lists
3. Administrator updates `/etc/microshift/config.yaml` with the chosen configuration
4. Administrator restarts MicroShift service
5. MicroShift parses the FeatureGate configuration and passes settings to relevant Kubernetes components where validation occurs
6. The features become available according to the configured state

#### Automated Rebase Workflow
1. CI automation initiates OpenShift rebase process
2. Automated tooling compares feature gate defaults between MicroShift and OpenShift components
3. Conflicts or misalignments are detected and flagged
4. Developer resolves conflicts to ensure MicroShift maintains the same default feature gates as OpenShift
5. Default feature gate alignment with OpenShift is maintained automatically

### API Extensions

This enhancement extends MicroShift's configuration file schema only. No new CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers are introduced. The configuration file structure will be extended to include a `featureGates` section that mirrors the OpenShift FeatureGate CRD specification, providing consistency with OpenShift's feature gate configuration patterns while maintaining MicroShift's file-based configuration approach.

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

The MicroShift configuration file will be extended to include a new `featureGates` section that mirrors the OpenShift FeatureGate CRD specification:

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

#### Rebase Automation

To address the requirements from [USHIFT-2813](https://issues.redhat.com/browse/USHIFT-2813), the rebase process will include:

1. **Feature Gate Inventory**: Automated tooling to extract default feature gate settings from each Kubernetes component
2. **Conflict Detection**: Comparison logic to identify conflicting defaults between components
3. **Alignment Verification**: CI checks to ensure MicroShift's defaults match OpenShift's defaults
4. **Override Mechanism**: Use of Kubernetes' `OverrideDefault` method where necessary to resolve conflicts

#### Validation and Error Handling

- Invalid feature gate names will be caught by the Kubernetes components themselves
- MicroShift will log configuration parsing errors but delegate feature gate validation to the components
- Conflicting feature gate settings between user configuration and component requirements will result in component startup failures with appropriate error messages

### Risks and Mitigations

**Risk: Feature Gate Conflicts Between Components**
Components may have conflicting default values for the same feature gate, as experienced in [USHIFT-2813](https://issues.redhat.com/browse/USHIFT-2813).

*Mitigation:* Implement automated detection during the rebase process to identify conflicts early and establish a consistent approach to resolve conflicting feature gate defaults across all components.

**Risk: Experimenting with Unstable Alpha Features**
Users experimenting with alpha-stage feature gates may encounter instability or data loss in their MicroShift deployments.

*Mitigation:* Emphasize that experimentation should be conducted in non-production environments. Feature gate validation will be handled by the Kubernetes components themselves.

**Risk: Feature Gate Misalignment During Rebases**
Manual rebase processes may miss feature gate changes in OpenShift, leading to divergent behavior.

*Mitigation:* Integrate automated feature gate alignment checks into the CI rebase process to ensure MicroShift maintains the same defaults as OpenShift automatically.

**Risk: Configuration Errors**
Invalid feature gate configurations could prevent MicroShift components from starting.

*Mitigation:* Leverage Kubernetes component validation for feature gate names and values. Provide clear error messages and documentation for troubleshooting configuration issues.

**Risk: Security Implications**
Some feature gates may expose new attack vectors or security vulnerabilities.

*Mitigation:* Security review will follow standard MicroShift processes. Feature gates that fundamentally conflict with MicroShift's security model will be documented as unsupported.

### Drawbacks

**Increased Configuration Complexity**
Adding feature gate configuration increases the complexity of MicroShift's configuration surface area. Users must understand both the feature gates themselves and their potential interactions, which could lead to misconfigurations in edge deployments where troubleshooting access is limited.

**Maintenance Burden**
This enhancement requires ongoing maintenance to keep feature gate handling aligned with OpenShift during rebases. The automated tooling and processes need to be maintained and updated as Kubernetes and OpenShift evolve their feature gate mechanisms.

**Support Complexity**
Enabling alpha and beta features through user configuration means support teams may encounter issues related to experimental functionality that behaves differently across Kubernetes versions or has incomplete implementations.

**Edge Device Risk**
Edge deployments often have limited remote access for troubleshooting. If users enable experimental feature gates that cause instability, recovering these devices may require physical access or complex recovery procedures.

**Upgrade Limitations and Irreversible Changes**
Enabling `TechPreviewNoUpgrade` feature set cannot be undone and prevents both minor version updates and major upgrades. Once enabled, the cluster permanently loses the ability to perform standard updates. Similarly, `CustomNoUpgrade` configurations prevent upgrades/updates until reset to default settings. These feature sets are explicitly not recommended for production clusters due to their irreversible nature and update limitations, which conflicts with the typical edge deployment requirement for reliable, long-term operation and maintenance.

## Alternatives (Not Implemented)

No significant alternatives were considered for this enhancement. The configuration file approach aligns with MicroShift's existing patterns and provides the required user-configurable feature gates with automated OpenShift alignment.

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

## Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.