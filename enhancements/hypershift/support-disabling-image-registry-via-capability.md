---
title: support-disabling-image-registry-via-capability
authors:
  - flavianmissi
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - deads2k
  - jubittajohn
  - sanchezl
  - sjenning
  - tjungblu
  - wking # OTA/cvo expert, for awareness and potential feedback
  - enxebre
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - deads2k
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - deads2k
  - sjenning
creation-date: 2024-12-19
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/XCMSTRAT-771
see-also:
  - "/enhancements/installer/component-selection.md"
---

# Disable the Image Registry via Capability

## Summary

In OCP Classic it's possible to disable various optional components at install
time via [cluster capabilities][cluster-capabilities]. This enhancement proposes
introducing a capabilities API to Hypershift, with initial support for a single
capability for disabling the `ImageRegistry`.

[cluster-capabilities]: /enhancements/installer/component-selection.md


## Motivation

It is currently possible to disable the Image Registry by setting
`.spec.managementState` to `Removed` in the `configs.imageregistry.operator.openshift.io`
resource. Using this approach to disable the Image Registry has the undesired
side-effect of causing a pod redeployment in the hosted control plane. This is
because the pull-secrets controller must be disabled along with the Image
Registry, and for that a pod redeployment is necessary.

We propose a way to disable the Image Registry that conforms to
[Hypershift's design principles][hypershift-design-principles].

Introducing a capabilities API to Hypershift is the natural way of supporting
disabling optional components at install time. To constrain the scope of the
initial implementation, the `ImageRegistry` capability is the only one that will
be initially supported, although the capabilities API itself will allow room for
growth.

[hypershift-design-principles]: https://hypershift-docs.netlify.app/reference/goals-and-design-invariants/


### User Stories

#### Personas

The following personas are borrowed from [Hypershift docs][hypershift-personas]
used in the user stories below.

**Cluster service consumer**: The user empowered to request control planes,
request workers, and drive upgrades or modify externalized configuration. Likely
not empowered to manage or access cloud credentials or infrastructure encryption
keys. In the case of managed services, this is someone employed by the customer.

**Cluster service provider**: The user hosting cluster control planes,
responsible for up-time. UI for fleet wide alerts, configuring AWS account to
host control planes in, user provisioned infra (host awareness of available
compute), where to pull VMs from. Has cluster admin management. In the case of
managed services, this persona represents Red Hat SRE.

#### Stories

* As a cluster service consumer, I want to provision hosted control planes and
clusters without the Image Registry, so that my hosted clusters do not contain
resources from a component I do not use, such as workloads, storage accounts,
pull-secrets, etc, which allows me to save on computing resources
* As a cluster service provider, I want users to be able to disable the Image
Registry so that I don't need to maintain hosted control plane components that
users don't care about.

[hypershift-personas]: https://hypershift-docs.netlify.app/reference/concepts-and-personas/#personas

### Goals

* Cluster instance admins can create clusters with the Image Registry capability
disabled
* Components in the hosted control plane are not impacted by the abscense of
the Image Registry
* Image Registry related assets such as workloads, storage account and
pull-secrets are not provisioned in clusters where the Image Registry capability
is disabled
* OpenShift Engineers are able to extend the list of capabilities available
in Hypershift, allowing other optional components to be disabled in the future

### Non-Goals

* Supporting the full set of OpenShift cluster capabilities
* Moving management of hosted control plane components to CVO
([OTA-951](https://issues.redhat.com/browse/OTA-951))
* Add support for implicitly enabled capabilities in Hypershift
* Supporting enabling capabilities that were initially disabled

## Proposal

### Workflow Description

1. Cluster service consumer creates a hosted cluster via `hypershift` CLI, setting
`--disable-cluster-capabilities=ImageRegistry`
1. The `hypershift` CLI creates a `HostedCluster` resource, setting
`.spec.capabilities.disabledCapabilities` to `["ImageRegistry"]` as requested by
the user
1. Hypershift control-plane-operator detects the ImageRegistry capability is
disabled in the `HostedCluster`, and skips creating resources for the Image
Registry operator, ensuring the `ClusterVersion` resource reflects this
1. The guest cluster becomes available and functional without the image registry
1. The hosted control-plane becomes available and functional without the
cluster image registry operator running
1. Image Registry operator and operand resources are not present in guest or
management clusters
1. Image Registry infrastructure resources are not created in cloud provider
(i.e. storage bucket, IAM user, etc)
1. No pull-secrets are created for any service accounts, regardless of when
the service account is created (install time or runtime)

### API Extensions

```go
import "github.com/openshift/api/config/v1"

// +kubebuilder:validation:Enum=ImageRegistry
type OptionalCapability v1.ClusterVersionCapability

const ImageRegistryCapability OptionalCapability = OptionalCapability(configv1.ClusterVersionCapabilityImageRegistry)

type Capabilities struct {
    // +listType=atomic
    // +optional
    Disabled []OptionalCapability
}

type HostedClusterSpec struct {
    // ... existing fields ...

    // +kubebuilder:validation:XValidation:rule="self == oldSelf", message="Capabilities is immutable. Changes might result in unpredictable and disruptive behavior."
    Capabilities *Capabilities
}
```

With the new field in place, the `cvoBootrapScript` needs to create the
`ClusterVersion` resource with a specific set of enabled capabilities, minus
the ones specified in `DisabledCapabilities`.
To allow the cvo reconciliation access to the disabled capabilities, the field
also needs to be added to the `HostedControlPlaneSpec`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hypershift is the target topology of this enhancement.

#### Standalone Clusters

Standalone clusters already support capabilities - the changes in this
enhancement do not affect this topology.

#### Single-node Deployments or MicroShift

The changes in this enhancement target Hypershift specific API and CLI.
Single-node deployments and Microshift are unnaffected.

### Implementation Details/Notes/Constraints

This feature will be behind TechPreview feature gate.

#### Capabilities can be disabled at cluster install time

Cluster service consumers can set the `--disable-cluster-capabilities` command
line option in the `hypershift` CLI to `ImageRegistry`, i.e.
```
hypershift create cluster azure --disable-cluster-capabilities=ImageRegistry [other-options]
```

The only acceptable value for this flag is `ImageRegistry`.

The hypershift CLI will pass this option onto the newly created `HostedCluster`
via `.spec.capabilities.disabledCapabilities`. The hosted cluster controller
will then pass this forward into the `HostedControlPlane` resource.

The `CVO` bootstrapper will create the `ClusterVersion` resource, setting
`spec.capabilities.baselineCapabilitySet` to `"None"`, then calculating the
difference between `ClusterVersionCapabilitySets[ClusterVersionCapabilitySetCurrent]`
and `HostedCluster.spec.capabilities.disabledCapabilities`, assigning the
resulting set to field `spec.capabilities.additionalEnabledCapabilities`.

The hosted cluster config operator will skip creation of Image Registry related
resources when it detects the image registry capability has been disabled.

#### Once disabled, capabilities cannot be enabled on running clusters

There is no use case for enabling an initially disabled capability on a running
cluster. For now, this will be a one way trip.

This will be ensured by API level validation of field immutability.

#### Capabilities cannot be disabled on running clusters

In hopes of disabling capabilities on a running cluster, cluster service
consumers may add or edit entries to `.spec.capabilities.disabledCapabilities`.
This will not pass validation, as this field is immutable.

#### Cluster upgrades

During cluster upgrades, the default set of capabilities might change. To avoid
drifting from the defaults imposed in OCP, the control-plane-operator will
always reconcile the default set of capabilities (vCurrent) with the
`ClusterVersion` `.spec.capabilities` field, minus the capabilities the user
initially disabled at cluster install time.

#### Evolving the Hypershift capabilities API

With a capabilities framework in place, it's expected that more capabilities
become available for disabling at install time. Currently in OpenShift
Standalone all optional capabilities are enabled by default. In the future, that
might change and some optional capabilities might come disabled by default.

In that case, cluster service consumers might need a way to decide which
of the disabled optional capabilities they want to enable at cluster install
time. In this case, a `--enable-cluster-capabilities` flag may be added to the Hypershift
CLI, and an equivalent `EnabledCapabilities` added to the `HostedCluster`
resource.

A good example of this is the `Baremetal` capability, which should be disable
by default so clusters are install with out the CBO. This makes a use case to
capture capabilities opt-in semantic. A possible path forward:

```
type Capabilities struct {
	// disabledCapabilities when specified, sets the cluster version baselineCapabilitySet to None
	// and sets all additionalEnabledCapabilities BUT the ones supplied in disabledCapabilities.
	// This effectively disables that capability on the hosted cluster.
	//
	// When this is not supplied, the cluster will use the DefaultCapabilitySet defined for the respective
	// OpenShift version.
	//
	// Once set, this field cannot be changed.
	//
	// +listType=atomic
	// +optional
	DisabledCapabilities []OptionalCapability `json:"disabledCapabilities,omitempty"`

	// enabledCapabilities specifies opt-in for capabilities that are not enable by default.
	// Its values are mutually exclusive with thouse in disabledCapabilities.
	// +listType=atomic
	// +optional
	EnabledCapabilities []OptionalCapability `json:"enabledCapabilities,omitempty"`
}
```


When extending the list of supported capabilities in Hypershift, it is crucial
to use existing capability names from the CVO API. Standalone cluster
capability names should exactly match the ones supported by Hypershift, although
the set of supported capabilities is expected to differ.

### Risks and Mitigations

#### Lack of implicitly enabled capabilities

During hosted control plane upgrades, the CVO is currently unaware it's being
upgraded. This leads to the CVO being unable to calculate what capabilities
should be implicitly enabled for clusters during upgrades.

Lack of implicitly enabled capabilities support results in two issues:

* when a capability is removed from the `vCurrent` capability set, hosted
control planes upgrading to the new `vCurrent` would lose the removed
capability, potentially disrupting customers' workloads
* when setting `BaselineCapabilitySet` in the `ClusterVersion` to `None`
(`ClusterVersionCapabilitySetNone`), newly introduced capabilities would be not
be added to the list of `AdditionalEnabledCapabilities`

Both these issues already exist on GA Hypershift, and we chose to not address
them in this proposal. See https://issues.redhat.com/browse/OTA-823 for details
about functional CVO differences in Hypershift.

#### Functionality drift in managed services

Managed services must implement support for the new `DisabledCapabilities` field
in their own tooling before cluster service consumers can use them. Because of
this, behavior may drift among different implementations, such as ARO and ROSA.

#### Certain capabilities may depend on other capabilities

In the future, it may be possible to disable multiple capabilities in Hypershift.
When this happens, it's important that Hypershift starts policying that certain
capabilities may need others to function.

An example of this co-dependency are the Marketplace and the OLM capabilities.
Enabling the Marketplace capability without the OLM capability will result in a
non-functional cluster and must not be allowed.

The installer currently has [validation logic][installer-caps-validation] to
prevent users from ending up in this situation.

[installer-caps-validation]: https://github.com/openshift/installer/blob/main/pkg/types/validation/installconfig.go#L205-L253

### Drawbacks

#### ROSA

GA'd versions of ROSA HCP already have support for disabling the image registry
by setting `.spec.managementState` to `Removed` in the
`configs.imageregistry.operator.openshift.io` resource. Depending on the state
of this (GA/Tech Preview/etc), ROSA might need to continue allowing users to
disable the image registry this way to keep backwards compatibility.

#### API design is different than OCP standalone

Customers who are used to the capabilities API in OCP standalone would expect
the same API when trying to use the same feature in Hypershift, and will likely
be surprised when learning the APIs differ.

#### Partial support for capabilities in Hypershift

There is currently a big feature mismatch in support for capabilities in Hypershift
when compared to OCP classic.

OCP classic supports a bigger set of capabilities, and we'll only introduce a single.
In addition to that, GA Hypershift also does not support capabilities removal.
Although not directly related to this EP, this may cause Hypershift clusters to
break, which might lead to an overall distrust in the capabilities feature, and
maybe even on OpenShift itself.

An example situation where removing a capability might break Hypershift is the
[DeploymentConfig capability becoming disabled by default][deploymentconfig-jira].

[deploymentconfig-jira]: https://issues.redhat.com/browse/WRKLDS-1326?focusedId=26401199&page=com.atlassian.jira.plugin.system.issuetabpanels%3Acomment-tabpanel#comment-26401199

## Open Questions [optional]

1. What happens when a new capability is added to `ClusterVersionCapabilitySetCurrent`?
Hypershift will set `additionalEnabledCapabilities` in the `ClusterVersion`
resource to `ClusterVersionCapabilitySetCurrent` at install time. After an
upgrade, the set of current capabilities might change.
Should Hypershift reconcile the changes? And if so, what happens then?
    * A: For new installs everything will be transparent. The new capability
    will be part of the `ClusterVersionCapabilitySetCurrent`.
    * A: Upgrades are a different story, which we probably need some exploration
    tests to understand. TODO: create spike?
1. Does Hypershift's CVO need to support [implicity capabilities][implicit-capabilities]?
    * A: no, see [Lack of implicitly enabled capabilities][lack-of-implicit-capabilities] for details

[implicit-capabilities]: https://github.com/openshift/enhancements/blob/master/enhancements/installer/component-selection.md#updates
[lack-of-implicit-capabilities]: #lack-of-implicitly-enabled-capabilities

## Test Plan

* [TestCreateCluster][TestCreateCluster] with image reg disabled
* [TestCreateClusterV2][TestCreateClusterV2] with image reg disabled
* [TestUpgradeControlPlane][TestUpgradeControlPlane] with image reg disabled

The assertions in these tests are sufficient, we would just need to ensure the
image registry operator is not showing up in the status objects after setting
the capabilities.

An additional e2e test is needed to cover the case where the registry is re-enabled
on a running cluster, as well as a negative ensuring that once enabled it
cannot be disabled on running clusters.

[TestCreateCluster]: https://github.com/openshift/hypershift/blob/main/test/e2e/create_cluster_test.go#L1111
[TestCreateClusterV2]: https://github.com/openshift/hypershift/blob/main/test/e2e/create_cluster_test.go#L1165C6-L1165C25
[TestUpgradeControlPlane]: https://github.com/openshift/hypershift/blob/main/test/e2e/control_plane_upgrade_test.go#L16

## Graduation Criteria

TBD

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

TBD

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

TBD

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

TBD

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

TODO: we want to deprecate the existing method of using `.spec.managementState: Removed` on ROSA.

## Upgrade / Downgrade Strategy

- Upgrading a hosted cluster from a version that does not support the ImageRegistry
capability into a cluster with support for the capability should not result in
a change of the Image Registry component status - it should remain enabled
- Upgrading a hosted cluster with the ImageRegistry capability disabled should
result in an upgraded cluster where the Image Registry component remains disabled
- Downgrading ?

-------

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

TBD

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

TBD

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

TBD

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

## Alternatives (Not Implemented)

TBD

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

## Infrastructure Needed [optional]

TBD

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
