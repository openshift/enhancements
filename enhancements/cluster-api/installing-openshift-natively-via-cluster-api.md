---
title: installing-openshift-natively-via-cluster-api
authors:
  - "@JoelSpeed"
reviewers:
  - "@patrickdillon - For Installer related changes"
  - "@vincepri - For Cluster API SME"
  - "@2uasimojo - For Hive related changes"
approvers:
  - "@vincepri"
api-approvers: 
  - "@deads2k"
creation-date: 2023-09-19
last-updated: 2023-09-19
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
see-also:
  - "TBD"
replaces:
  - "TBD"
superseded-by:
  - "TBD"
---

# Installing OpenShift natively via Cluster API

## Summary

The following enhancement describes how to install an OpenShift 4 cluster using Cluster API native concepts.
The enhancement assumes that a Cluster API control plane exists[^1][^2] and that it is running the OpenShift related providers described below.

[^1]: The Cluster API control plane is a set of controllers that manage the lifecycle of Cluster API objects.
[^2]: A separate enhancement (TBD) will detail how the temporary Cluster API control plane will be provided by the installer for cluster 0 installations.

## Motivation

OpenShift 4 currently relies on terraform, embedded within the installer binary, to provision infrastructure for OpenShift clusters in Installer-Provisioned-Infrastructure clusters.
This approach has a number of drawbacks that have led to a divergent set of installers, complexity in integrating new providers, and a lack of flexibility in how OpenShift is installed[^3].

It is desirable to have a single installation experience for OpenShift that is consistent across all providers, and can be used not only to bootstrap the first cluster, but also to install additional clusters.

Cluster API is a Kubernetes sub-project that provides a declarative API for managing the lifecycle of Kubernetes clusters and the infrastructure behind them.
It provides a multi-cluster management solution that will enable OpenShift to unify the single cluster and multi-cluster installation experiences.

[^3]: Leading to various UPI clusters and other installer variations.

### User Stories

As an OpenShift administrator, I want to be able to provision my first OpenShift cluster using the same tooling that I use to provision additional clusters, so that I can have a consistent experience.

As an OpenShift administrator, I want to be able to centrally manage the lifecycle and infrastructure of my OpenShift clusters, so that I do not have to delegate infrastructure management to other teams.

As an OpenShift developer, I want to leverage Cluster API to provision OpenShift cluster infrastructure, so that I can focus on the OpenShift specific components and leverage the community around Cluster API for the generic cluster Infrastructure.

As an OpenShift developer, I want to simplify the maintenance of the installer by removing the embedded terraform and platform specific logic, so that the installer is easier to maintain and extend.

As a Hive developer, I want to delegate as much of the cluster lifecycle to Cluster API as possible, so that Hive can focus on the OpenShift specific features.

### Goals

#### Phase 1

1. Leverage an existing Cluster API control plane to provision infrastructure for OpenShift clusters.
1. Delegate the provisioning and management of the cluster infrastructure bootstrap provisioning to Cluster API.
1. Delegate the orchestration of the OpenShift cluster bootstrap node process to Cluster API.
1. Supplement Cluster API infrastructure provisioning gaps with small, and focused supplemental infrastructure providers.
1. Convert the installer to generate Cluster API infrastructure templates instead of terraform resources.
1. Rely on the installer to generate configuration up to and including the `manifests` phase.
1. Leverage the desired payload installer to generate bootstrap ignition configs and kubeconfigs.

#### Phase 2

1. Provide an end to end OpenShift installation experience using Cluster API.
  1. This will allow a BYO infrastructure experience for OpenShift via Cluster API.
1. Integrate Hive directly with Cluster API to remove the need to execute the installer during Cluster Deployments.

### Non-Goals

1. Provisioning of the Cluster API control plane.[^2]
1. Generation in-tree of ignition data for the bootstrap node.

## Proposal

A Cluster API control plane and bootstrap provider will be created to handle the orchestration and configuration of the OpenShift cluster during the bootstrap process.
The control plane provider will be responsible for creating (and destroying) the bootstrap node, and provisioning the control plane nodes once the bootstrap node is ready.
The bootstrap provider will be responsible for generating the correct ignition data for the bootstrap node, control plane nodes and worker nodes

Where gaps exist in the Cluster API infrastructure providers, small, focused supplemental infrastructure providers will be created to fill those gaps.
For example, on AWS, where Cluster API does not manage DNS, a supplemental provider will be created to manage DNS records for the cluster.

Provide a configurable, opinionated and controller driven multi-cluster and single cluster provisioning experience:
* Driven by install config and Cluster API resources
* Cluster API resources generated by the installer for opinionated install
* Self crafted Cluster API resources for custom install or install on external platforms
* Customisation of bootstrap manifests via control plane provider configuration

### Workflow Description

#### Opinionated installer generated infrastructure definitions

The installer binary will be updated to transform the existing install config into Cluster API resources.

During the `manifests` phase, the installer will generate the following Cluster API resources into an `infrastructure-manifests` folder:
* Cluster
* OpenShiftControlPlane
* InfrastructureCluster (e.g. `AWSCluster`)
* InfrastructureMachineTemplate (e.g. `AWSMachineTemplate`)

Additionally supplemental resources will be required:
* Namespace (each cluster will be created in a unique namespace for isolation)
* Infrastructure Credentials secret (to allow the Cluster API control plane to authenticate to the cloud provider).

The installer will then diverge from the current path, which is to execute `ignition-configs` and then `cluster` phases.

The `ignition-configs` phase will become a terminal phase and will be used by the bootstrap provider to generate the ignition configs for the bootstrap node, control plane nodes and worker nodes.
No changes are expected to the `ignition-configs` phase and existing UPI workflows should continue to work.

The `cluster` phase will now skip the `ignition-configs` phase and will instead apply the Cluster API resources generated in the `manifests` phase to the Cluster API control plane.

The installer will directly apply the Cluster API resources to the Cluster API control plane.
If any modifications are made between the `manifests` and `cluster` phases, the installer will not overwrite these changes.
This allows the user to tweak the configuration generated by the installer should they wish to do so.

The installer will also create an `install-state` secret in the cluster namespace containing the current `.openshift_install_state.json` file.
This will then be consumed later by the bootstrap provider to generation ignition.
This secret will be referenced in the `OpenShiftControlPlane` spec.

To allow the user to customise manifests, the installer will take all manifests from the `manifests` and `openshift` folders and wrap them into secrets to be applied to the cluster namespace.
Each secret will be annotated to indicate that it should be included in the ignition generation phase, and to identify whether it was a `manifest` file or `openshift` file.
A selector on the `OpenShiftControlPlane` spec will be used to identify which secrets should be included in the ignition generation phase.

The bootstrap provider will read the `OpenShiftControlPlane` spec to determine the install state and manifests required to complete the bootstrap ignition generation and will reconstruct the required structure for the installer to complete the `ignition-configs` phase in cluster.

Once all resources are applied, the installer will watch the Cluster API control plane resource status to determine when the cluster is ready.
Once the cluster is ready it will wait for all ClusterOperators to be available before completing the `cluster` phase, as it does today.

#### Custom infrastructure definitions

Where a user wishes to customise the installation of OpenShift, they may create their own Cluster API resources and apply them to the Cluster API control plane.
They will be expected to create the same resources as the installer would have created, but with their own configuration.

There are 2 ways to achieve this:
* By using the installer and customising the Cluster API resources generated by the installer.
* By manually crafting the infrastructure resources and applying them to the Cluster API control plane, for example, for externally supported platforms.

In the second case, where the installer does not have an opinionated set of Cluster API resources to generate, the installer will generate all resources required except for the `InfrastructureCluster` and `InfrastructureMachineTemplate`.
It is expected that the user will provide these two resources and update the references in the `Cluster` and `OpenShiftControlPlane` resources to match.

##### Bring your own infrastructure

Cluster API allows users to bring their own infrastructure.
The various providers allow configuration of existing infrastructure, such as VPCs, subnets etc, and will supplement any missing infrastructure with new infrastructure.

If users wish to bring their own infrastructure, they will be expected to create the required infrastructure on the cloud provider, to their needs, and update the `InfrastructureCluster` and `InfrastructureMachineTemplate` resources to match.
When configured with bring your own infrastructure, the InfrastructureProvider will not modify the existing infrastructure, but will populate the `InfrastructureCluster` with details of the infrastructure that already exists, allowing operation of the rest of the system.

#### Hive integration

TODO

#### Variation and form factor considerations 

##### Interaction with Machine API

Until the Machine API supports migration to Cluster API, the installer will continue to provision Machine API resources into the guest clusters created by Cluster API.

For temporary Cluster API control planes, the Cluster API resources will be discarded once the cluster is ready and the resulting cluster will look exactly as it does today.
When the Machine API to Cluster API migration is supported, the installer will be updated to include an option to move the Cluster API resources into the guest cluster creating a joined or self managed cluster.

For permanent Cluster API control planes (e.g. Hive or mulit-cluster use cases), Machine API resources should be dropped and Cluster API resources should be used in the long term to manage the cluster.
This will mean Hive creating Cluster API MachineDeployments instead of MachineAPI MachineSets.

### API Extensions

#### OpenShiftControlPlane

The `OpenShiftControlPlane` resource will be the configuration for the control plane provider implementation for Cluster API.
It must adhere to the upstream Cluster API [control plane provider API contract][control-plane-api-contract].

It will contain all of the required configuration to bootstrap the OpenShift cluster, including:
* Initial payload to bootstrap the cluster
* A reference to an install state secret containing either the install config or the install state
* A reference to the infrastructure template to be used to create the bootstrap and control plane nodes

The `OpenShiftControlPlane` will be introduced initially as a `v1alpha1` in Technology Preview.
It will reside in the `cluster.openshift.io`  API group.

The `spec` and `status` of the object are detailed below.

```golang
// OpenShiftControlPlaneSpec is the spec of the OpenShift control plane resource.
type OpenShiftControlPlaneSpec struct {
  // machineTemplate defines the machine template used to create the initial bootstrap and control plane machines.
  // Continued management of the control plane machines will be handled by the control plane machine set.
  // The machine template is therefore immutable and only applicable during the bootstrap process.
  // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="machineTemplate is immutable"
  // +kubeubilder:validation:Required
  MachineTemplate OpenShiftControlPlaneMachineTemplate `json:"machineTemplate"`

  // installStateSecretRef is a reference to a secret containing the install state.
  // The install state secret must contain either the install config or the install state, or both.
  // The install state secret must be in the same namespace as the OpenShiftControlPlane.
  // The install config must be under the key `install-config.yaml` and the install state must be under the key `.openshift_install_state.json`.
  // These files will be passed to the installer to generate the ignition configs for the bootstrap node, control plane nodes and worker nodes.
  // +kubebuilder:validation:Required
  InstallStateSecretRef OpenShiftControlPlaneSecretRef `json:"installStateSecretRef"`

  // manifestsSelector is a selector to identify secrets containing manifests to be included in the ignition generation phase.
  // The selector must match the labels on the secrets to be injected.
  // Each key in the secret must be the path to a file to be injected into the ignition.
  // This path should start with either `manifests/` or `openshift/`.
  // When omitted, the default manifests generated by the installer will be used.
  // +optional
  ManifestsSelector metav1.LabelSelector `json:"manifestsSelector,omitempty"`
}

// OpenShiftControlPlaneMachineTemplate is the spec of the OpenShift control plane machines.
type OpenShiftControlPlaneMachineTemplate struct {
  // metadata is the standard object's metadata.
  // This allows for machine labels and annotations to be applied to the control plane machines.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta ObjectMeta `json:"metadata,omitempty"`

	// infrastructureRef is a required reference to a custom resource offered by an infrastructure provider.
  // The infrastructure reference should define a template for the infrastructure provider to create the bootstrap and control plane nodes.
  // +kubebuilder:validation:Required
	InfrastructureRef InfrastructureReference `json:"infrastructureRef"`

	// NodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// NodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// NodeDeletionTimeout defines how long the machine controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// If no value is provided, the default value for this property of the Machine resource will be used.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`
}

// ObjectMeta is a subset of metav1.ObjectMeta.
// We use this to customise the labels and annotations applied to control plane machines.
type ObjectMeta struct {
	// labels is a map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// InfrastructureReference is a reference to a custom resource offered by an infrastructure provider.
// This is a subset of corev1.ObjectReference.
// The namespace must be set to the same as the OpenShiftControlPlane, but is required by Cluster API.
// Upstream discussion: https://github.com/kubernetes-sigs/cluster-api/issues/6539
type InfrastructureReference struct {
  // kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`
  // namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
	// name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +kubebuilder:validation:Required
	Name string `json:"name"`
  // apiVersion of the referent.
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`
}

// OpenShiftControlPlaneSecretRef is the reference to a secret in the same namespace as the OpenShiftControlPlane.
type OpenShiftControlPlaneSecretRef struct {
  // name is the name of the secret.
  // It has a maximum length of 253 characters and must be a valid DNS subdomain name.
  // This means it must consist only of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.
  // +kubebuilder:validation:Pattern="[a-z0-9]([-.a-z0-9]{,251}[a-z0-9])?"
  // +kubebuilder:validation:MaxLength=253
  // +kubebuilder:validation:Required
  Name string `json:"name"`
}

type OpenShiftControlPlaneStatus struct {}
```

[control-plane-api-contract]: https://cluster-api.sigs.k8s.io/developer/architecture/controllers/control-plane#crd-contracts

API Extensions are CRDs, admission and conversion webhooks, aggregated API servers,
and finalizers, i.e. those mechanisms that change the OCP API surface and behaviour.

- Name the API extensions this enhancement adds or modifies.
- Does this enhancement modify the behaviour of existing resources, especially those owned
  by other parties than the authoring team (including upstream resources), and, if yes, how?
  Please add those other parties as reviewers to the enhancement.

  Examples:
  - Adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
  - Restricts the label format for objects to X.
  - Defaults field Y on object kind Z.

Fill in the operational impact of these API Extensions in the "Operational Aspects
of API Extensions" section.

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

#### Hypershift [optional]

Does the design and implementation require specific details to account for the Hypershift use case?
See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282


### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.  

What trade-offs (technical/efficiency cost, user experience, flexibility, 
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?  

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?


## Design Details

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

### Test Plan

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

### Graduation Criteria

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

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

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

### Version Skew Strategy

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

### Operational Aspects of API Extensions

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

#### Failure Modes

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

#### Support Procedures

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

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
