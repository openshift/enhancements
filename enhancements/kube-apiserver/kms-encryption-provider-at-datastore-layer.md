---
title: kms-encryption-provider-at-datastore-layer
authors:
  - "@ardaguclu"
  - "@dgrisonnet"
  - "@flavianmissi"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@ibihim"
  - "@sjenning"
  - "@tkashem"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@sjenning"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@JoelSpeed"
creation-date: 2025-10-17
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/OCPSTRAT-108"
  - "https://issues.redhat.com/browse/OCPSTRAT-1638"
see-also:
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
replaces:
  - ""
superseded-by:
  - ""
---

# KMS Encryption Provider at Datastore Layer

## Summary

Provide a user-configurable interface to support encryption of data stored in
etcd using a supported Key Management Service (KMS).

## Motivation

OpenShift supports AES encryption at the datastore layer using local keys.
It protects against etcd data leaks in the event of an etcd backup compromise.
However, aescbc and aesgcm, which are supported encryption technologies today
available in OpenShift do not protect against online host compromise i.e. in
such cases, attackers can decrypt encrypted data from etcd using local keys,
KMS managed keys protects against such scenarios.

### User Stories

* As a cluster admin, I want the APIServer config to be the single source of
  etcd encryption configuration for my cluster, so that I can easily manage all
  encryption related configuration in a single place
* As a cluster admin, I want the kas-operator to manage KMS plugin lifecycle on
  my behalf, so that I don’t need to do any manual work when configuring KMS
  etcd encryption for my cluster
* As a cluster admin, I want to easily understand the operations done by CKASO
  when managing the KMS plugin lifecycle via Conditions in the APIServer CR’s
  Status
* As a cluster admin, I want to be able to switch to a different KMS plugin,
  i.e. from AWS to a pre-installed Vault, by performing a single configuration
  change without needing to perform any other manual intervention
    * TODO: confirm this requirement
* As a cluster admin, I want to configure my chosen KMS to automatically rotate
  encryption keys and have OpenShift to automatically become aware of these new
  keys, without any manual intervention
* As a cluster admin, I want to know when anything goes wrong during key
  rotation, so that I can manually take the necessary actions to fix the state
  of the cluster

### Goals

* Users have an easy to use interface to configure KMS encryption
* Users will configure OpenShift clusters to use a specific KMS key, created by
  them
* Encryption keys managed by the KMS, and are not stored in the cluster
* Encryption keys are rotated by the KMS, and the configuration is managed by
  the user
* OpenShift clusters automatically detect KMS key rotation and react
  appropriately
* Users can disable encryption after enabling it
* Configuring KMS encryption should not meaningfully degrade the performance of
  the cluster
* OpenShift will manage KMS plugins' lifecycle on behalf of the users
* Provide users with the tools to monitor the state of KMS plugins and KMS
  itself

### Non-Goals

* Support for users to control what resources they want to encrypt
* Support for OpenShift managed encryption keys in KMS
* Direct support for hardware security models (these might still be supported
  via KMS plugins, i.e. Hashicorp Vault or Thales)
* Full data recovery in cases where the KMS key is lost
* Support for users to specify which resources they want to encrypt

## Proposal

To support KMS encryption in OpenShift, we will leverage the work done in
[upstream Kubernetes](https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/3299-kms-v2-improvements).
However, we will need to extend and adapt the encryption workflow in OpenShift
to support new constraints introduced by the externalization of encryption keys
in a KMS. Because OpenShift will not own the keys from the KMS, we will also
need to provide tools to users to detect KMS-related failures and take
action toward recovering their clusters whenever possible.

We focus on supporting KMS v2 only, as KMS v1 has considerable performance
impact in the cluster.

#### API Extensions

We will extend the APIServer config to add a new `kms` encryption type alongside
the existing `aescbc` and `aesgcm` types. Unlike `aescbc` and `aesgcm`, KMS
will require additional input from users to configure their KMS provider, such
as connection details, authentication credentials, and key references. From a
UX perspective, this is the only change the KMS feature introduces—it is
intentionally minimal to reduce user burden and potential for errors.

#### Encryption Controller Extensions

This feature will reuse existing encryption and migration workflows while
extending them to handle externally-managed keys. We will introduce a new
controller to manage KMS plugin pod lifecycle and integrate KMS plugin health
checks into the existing controller precondition system.

#### KMS Plugin Lifecycle

KMS encryption requires KMS plugin pods to bridge communication between the
kube-apiserver and the external KMS. In OpenShift, the kube-apiserver-operator
will manage these plugins on behalf of users, reducing operational complexity
and ensuring consistent behavior across the platform. The operator will handle
plugin deployment, health monitoring, and lifecycle management during key
rotation events.

### Workflow Description

#### Roles

**cluster admin** is a human user responsible for the overall configuration and
maintainenance of a cluster.

**KMS** the Key Management Service responsible automatic rotation of the Key
Encryption Key (KEK).

#### Initial Resource Encryption

1. The cluster admin creates an encryption key (KEK) in their KMS of choice
1. The cluster admin give the OpenShift apiservers access to the newly created
   KMS KEK
1. The cluster admiin updates the APIServer configuration resource, providing
   the necessary configuration options for the KMS of choice
1. The cluster admin observes the `kube-apiserver` `clusteroperator` resource,
   for progress on the configuration, as well as migration of resources

#### Key rotation

1. The cluster admin configures automatic periodic rotation of the KEK in KMS
1. KMS rotates the KEK
1. OpenShift detects the KEK has been rotated, and starts migrating encrypted
   data to use the new KEK
1. The cluster admin eventually checks the `kube-apiserver` `clusteroperator`
   resource, and sees that the KEK was rotated, and the status of the data
   migration

#### Change of KMS Provider

### API Extensions

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

#### Controller Preconditions and KMS Plugin Health

Encryption controllers should only run when the KMS provider plugin is up and
running. All the encryption controllers take in a preconditionsFulfilled
function as a parameter. The controllers use this to decide whether they should
sync or not. We can leverage this existing mechanism to check if the KMS plugin
is healthy, in addition to the existing checks.

#### Encryption Key Secret Management for KMS

The keyController will continue managing encryption key secrets as it does
today. The difference is that for the KMS encryption provider, the encryption
key secret contents will be empty. This secret must be empty because when the
KMS provider is used the root encryption key (KEK) is stored and managed by KMS
itself. We still want the encryption key secret to exist, even if empty, so
that we can leverage functionality in the existing encryption controllers, thus
having full feature parity between existing encryption providers and the new
KMS encryption provider.

#### Key Rotation Handling

Keys can be rotated in the following ways:
* Automatic periodic key rotation by the KMS, following user provided rotation
  policy in the KMS itself
* The user creates a new KMS key, and updates the KMS section of the APIServer
  config with the new key

OpenShift must detect the change and trigger re-encryption of affected
resources.

TODO: elaborate details about the two rotation scenarios,
how detection works, migration process, etc.

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

## Alternatives (Not Implemented)

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

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
