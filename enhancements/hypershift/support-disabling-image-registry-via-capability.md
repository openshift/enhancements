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
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - deads2k
  - sjenning
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

To get started with this template:
1. **Pick a domain.** Find the appropriate domain to discuss your enhancement.
1. **Make a copy of this template.** Copy this template into the directory for
   the domain.
1. **Fill out the metadata at the top.** The embedded YAML document is
   checked by the linter.
1. **Fill out the "overview" sections.** This includes the Summary and
   Motivation sections. These should be easy and explain why the community
   should desire this enhancement.
1. **Create a PR.** Assign it to folks with expertise in that domain to help
   sponsor the process.
1. **Merge after reaching consensus.** Merge when there is consensus
   that the design is complete and all reviewer questions have been
   answered so that work can begin.  Come back and update the document
   if important details (API field names, workflow, etc.) change
   during code review.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

See ../README.md for background behind these instructions.

Start by filling out the header with the metadata for this enhancement.

# Disable the Image Registry via Capability

## Summary

In OCP Classic it's possible to disable various optional components at install
time via [cluster capabilities](cluster-capabilities). This enhancement proposes
introducing a capabilities API to Hypershift, with initial support for a single
capability for disabling the `ImageRegistry`.

[cluster-capabilities]: https://docs.openshift.com/container-platform/4.17/installing/overview/cluster-capabilities.html

## Motivation

It is currently possible to disable the Image Registry by setting
`.spec.managementState` to `Removed` in the `configs.imageregistry.operator.openshift.io`
resource. Using this approach to disable the Image Registry has the undesired
side-effect of causing a pod redeployment in the hosted control plane. This is
because the pull-secrets controller must be disabled along with the Image
Registry, and for that a pod redeployment is necessary.

We propose a way to disable the Image Registry that conforms to
[Hypershift's design principles](hypershift-design-principles).

Introducing a capabilities API to Hypershift is the natural way of supporting
disabling optional components at install time. To constrain the scope of the
inital implementation, the `ImageRegistry` capability is the only one that will
be initially supported, although the capabilities API itself will allow room for
growth.

[hypershift-design-principles]: https://hypershift-docs.netlify.app/reference/goals-and-design-invariants/


### User Stories

See [Hypershift docs](hypershift-personas) for a definition of the personas used below.

* As a cluster instance admin, I want to provision hosted control planes and
clusters without the Image Registry, so that my hosted clusters do not contain
its resources, such as workloads, storage accounts, pull-secrets, etc

[hypershift-personas]: https://hypershift-docs.netlify.app/reference/concepts-and-personas/#personas

### Goals

* Cluster instance admins can create clusters with the Image Registry disabled
* Cluster instance admins can reenable the Image Registry without recreating
the cluster
* Components in the hosted control plane are not impacted by the Image Registry
absence
* Image Registry related assets such as workloads, storage account and
pull-secrets are not provisioned in clusters without the Image Registry
* OpenShift Engineers are able to extend the list of capabilities available
in Hypershift, allowing other optional components to be disabled in the future

### Non-Goals

* Supporting the full set of OpenShift cluster capabilities
* Moving management of hosted control plane components to CVO
([OTA-951](https://issues.redhat.com/browse/OTA-951))

## Proposal

`HostedCluster` grows an optional slice field `disabledCapabilities` in its
`spec`, indicating what components users want to disable at install time.

Initially, the only supported value in this slice is `ImageRegistry`.

Editing this field at runtime is only allowed if the user is removing
capabilities from the slice. Adding capabilities to the slice is not supported.
In other words, disabling capabilities at runtime is not supported.

The `control-plane-operator` is responsible for reconciling both, the Image
Registry and Cluster Version Operator. Both reconcilers will need to change.

The Image Registry reconciler needs to look for an entry matching the
`ImageRegistry` capability in the `HostedCluster.spec.disabledCapabilities`
field, and skip reconciliation when the `ImageRegistry` capability is found.

The `CVO` reconciler needs to edit the `ClusterVersion` resource, setting
`spec.capabilities.baselineCapabilitySet` to `"None"`, then calculating the
difference between `ClusterVersionCapabilitySets[ClusterVersionCapabilitySetCurrent]`
and `HostedCluster.spec.disabledCapabilities`, assigning the resulting set to
field `spec.capabilities.additionalEnabledCapabilities`.

The `hypershift` CLI grows a `--disable-cluster-capabilities` option. Initially
this option will only accept a single value: `ImageRegistry`. In the future it
can grow to support a comma separated list of cluster capabilities.

### Workflow Description

1. Cluster service consumer creates a hosted cluster via `hypershift` CLI, setting
`--disable-cluster-capabilities=ImageRegistry`
1. The `hypershift` CLI creates a `HostedCluster` resource, setting
`.spec.disabledCapabilities` to `["ImageRegistry"]` as requested by the user
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

const ImageRegistryCapability OptionalCapability = v1.ClusterVersionCapabilityImageRegistry

type HostedClusterSpec struct {
    // ... existing fields ...

    // +kubebuilder:validation:XValidation:rule="size(self.spec.disabledCapabilities) <= size(oldSelf.spec.disabledCapabilities)",message="Disabling capabilities on running clusters is not supported, only removing capabilities from disabledCapabilities (re-enabling capabilities) is supported."
    // +optional
    DisabledCapabilities []OptionalCapability
}
```

With the new field in place, the `cvoBootrapScript` needs to create the
`ClusterVersion` resource with a specific set of enabled capabilities, minus
the ones specified in `DisabledCapabilities`.
This function currently doesn't have access to the `HostedClusterSpec`.
`cvo.ReconcileDeployment` will need to change to allow it access to the set of
disabled capabilities specified by the customer.

TODO: all the reconcile functions in `hostedcontrolplane_controller.go` take a
`HostedControlPlane` object, not a `HostedCluster`. My assumption is that the
`HosterControlPlane` gets created from the `HostedCluster`. If this is correct,
where does it happen?


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

### Topology Considerations

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282

How does it affect any of the components running in the
management cluster? How does it affect any components running split
between the management cluster and guest cluster?

#### Standalone Clusters

Is the change relevant for standalone clusters?

#### Single-node Deployments or MicroShift

How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

How does this proposal affect MicroShift? For example, if the proposal
adds configuration options through API resources, should any of those
behaviors also be exposed to MicroShift admins through the
configuration file for MicroShift?

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

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

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
