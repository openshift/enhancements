---
title: accepted-risks
authors:
  - "@hongkailiu"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@wking"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-06-11
last-updated: yyyy-mm-dd
tracking-link:
  - https://issues.redhat.com/browse/OTA-1544
see-also:
  - "/enhancements/update/targeted-update-edge-blocking.md"
---

# accepted-risks

## Summary

A cluster admin can express accepted risks for a cluster so that when [a conditional update](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/updating_clusters/understanding-openshift-updates-1#conditional-updates-overview_understanding-update-channels-releases) of cluster is requested, it can be accepted only if the risks exposed to the conditional update are all accepted.

## Motivation

It is to reduce the time and effort for cluster administrators who manage many clusters, enabling them to pre-approve certain risks that they deem acceptable across some or all of their environments. Although the evaluation of risks cannot be avoided, the evaluation result may be reused for other clusters.

### User Stories

The enhancement is useful, e.g., in the following scenario:

- A cluster admin gets conditional updates of a cluster and evaluates the risks in the conditional updates.
- After evaluation, the admin decides that some risks can be accepted on all or some of the managed clusters.
- The administrator configures relevant clusters to allow for updates, as long as any identified risks have all been accepted.

### Goals

- Reduce the overall effort of managing a great number of clusters and evaluating risks from a conditional update.

### Non-Goals

- It does not change which impact a specific update risk may bring to a cluster and how it is evaluated by a cluster administrator.

## Proposal

This enhancement proposes a way of specifying accepted risks regarding updating an OpenShift cluster.

### Workflow Description

We descibe here two workflows and their different implementations of the oc-cli based on the same API extension for the following scenario as the starting point: The cluster administrator decides that those risks `DualStackNeedsController`, `OldBootImagesPodmanMissingAuthFlag`, and `LeakedMachineConfigBlocksMCO` are acceptable and wants to tell the cluster version operator to accept an update if all its associated risks are among those three (otherwise block it).

#### Workflow 1

Provide the existing `oc adm upgrade` command with a new and optional `--accept` option to update a cluster, i.e., `--accept` take effective only when performing an cluster update.

> oc adm upgrade --to 4.18.16  --accept DualStackNeedsController,LeakedMachineConfigBlocksMCO,OldBootImagesPodmanMissingAuthFlag


The result of `--accept` in the above command is to set up values for the field `.spec.desiredUpdate.acceptRisks` on the `clusterversion/version` manifest:

```console
$ oc clusterversion version -o yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: candidate-4.18
  clusterID: 1c182977-5663-428d-92a3-3d2bdf3fffb6
  desiredUpdate:
    acceptRisks:
    - name: DualStackNeedsController
    - name: LeakedMachineConfigBlocksMCO
    - name: OldBootImagesPodmanMissingAuthFlag
    force: false
    version: 4.18.16
```

Note that missing `--accept` in the above command means accepting no risks at all and all existing accepted risks specified in `cv.spec.desiredUpdate.acceptRisks` is going to be removed. A cluster admin who chooses to do GitOps on the ClusterVersion manifest should not use `oc adm upgrade` to perform a cluster update. Currently each item in `cv.spec.desiredUpdate.acceptRisks` has only one field `name` but it may have more such as `reason` in the future if needed.

The cluster-version operator finds that the update to `4.18.16` is not recommended because of the risks `DualStackNeedsController` `OldBootImagesPodmanMissingAuthFlag`, and `RHELKernelHighLoadIOWait` and only the first two of them are accepted by the administrator. Thus, the cluster update to `4.18.16` is blocked. After a couple of weeks, `4.18.17` is released which contains the fixes of `DualStackNeedsController` and `RHELKernelHighLoadIOWait`. The only remained risk of `4.18.17` is `OldBootImagesPodmanMissingAuthFlag`. When the cluster is updated to `4.18.17`, e.g., by the following command:

> oc adm upgrade --to 4.18.17  --accept DualStackNeedsController,LeakedMachineConfigBlocksMCO,OldBootImagesPodmanMissingAuthFlag

the CVO accepts the upgrade to `4.18.17` because its only risk `OldBootImagesPodmanMissingAuthFlag` has been accepted by the administrator already.

#### Workflow 2

Provide a new (sub-)command `accept`, e.g.,

> oc adm upgrade accept DualStackNeedsController,LeakedMachineConfigBlocksMCO,OldBootImagesPodmanMissingAuthFlag

 whose only result is to append the provided values to the field `.spec.desiredUpdate.accept` on the `clusterversion/version` manifest:

```console
$ oc clusterversion version -o yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: candidate-4.18
  clusterID: 1c182977-5663-428d-92a3-3d2bdf3fffb6
  desiredUpdate:
    acceptRisks:
    - name: DualStackNeedsController
    - name: LeakedMachineConfigBlocksMCO
    - name: OldBootImagesPodmanMissingAuthFlag
    force: false
    version: 4.18.15
```

Then the upgrade to `4.18.16` by the command `oc adm upgrade --to 4.18.16` will be blocked and to `4.18.17` by `oc adm upgrade --to 4.18.17` will be accepted.

The risk name like `OldBootImagesPodmanMissingAuthFlag` is unique cross all releases and thus the growing accepted risks over time should not cause any problems. However, for any reason, if it is desired to clear them out, we may run the following command:

> oc adm upgrade accept --clear

and

> oc adm upgrade accept -Foo

can be used to remove `Foo` from the accepted risks which is no-op if `Foo` is currently not in `cv.spec.desiredUpdate.acceptRisks`.

No risks are provided to the command means that no risks are accepted.

The `--replace` in the following following command is to replace the current accept risks (instead of appending by default) with `RiskA` and `RiskB`:

> oc adm upgrade accept --replace=true RiskA,RiskB

Note that we have to modify the `patchDesiredUpdate`(https://github.com/openshift/oc/blob/f9d98d644110d3413dc4862002395d0c6dfc1da7/pkg/cli/admin/upgrade/upgrade.go#L682-L696) function so that it does not clobber the existing `acceptedRisks` property.

#### Comparing Two Workflows:

Benefits of `oc adm upgrade --to 4.y.z --accept RiskA,RiskB`:

* No need for a new subcommand, so manipulating `cv.spec.desiredUpdate` stays consolidated.
* Encourages cluster-admins to build their own systems for managing acceptance information (e.g. GitOps), because while the new `cv.spec.desiredUpdate.acceptRisks` allows admins to store the risks they accept, it doesn't have space for them to explain why they find those risks acceptable, and that seems like important information that a team of admins sharing risk-acceptance decision-making would want to have available.

Benefits of `oc adm upgrade accept [--replace] RiskA,RiskB`:

* Convenient appending, so an admin can roll forward with the things previous admins have already accepted, without having to worry about what those were.
* Convenient way to share accepted risk names (but not your reasoning for acceptance) via ClusterVersion, without having to build your own system to share those between multiple cluster-admins.

### API Extensions

This enhancement 
- adds a new field 'clusterversion.spec.desiredUpdate.acceptRisks': It contains
  the names of conditional update risks that are considered acceptable.
- moves `clusterversion.status.conditionalUpdates.risks` up and rename it as
  `clusterversion.status.conditionalUpdateRisks`. It contains all the risks
  for `clusterversion.status.conditionalUpdates`.
- adds a new field 'clusterversion.status.conditionalUpdates.riskNames': It
  contains the names of risks for the conditional update. It deprecates
  `clusterversion.status.conditionalUpdates.risks`.
- adds a new field 'clusterversion.status.conditionalUpdateRisks.conditions': It
  contains the observations of the conditional update risk's current status.

For example,

```console
$ oc clusterversion version -o yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: candidate-4.18
  clusterID: 1c182977-5663-428d-92a3-3d2bdf3fffb6
  desiredUpdate:
    acceptedRisks:
    - name: DualStackNeedsController
    - name: LeakedMachineConfigBlocksMCO
    - name: OldBootImagesPodmanMissingAuthFlag
    force: false
    version: 4.18.15
status:
  conditionalUpdateRisks:  # include every risk in the conditional updates (moved up and renamed)
  - name: DualStackNeedsController
    message: Upgrade can get stuck on clusters that use multiple networks together with dual stack.
    url: https://issues.redhat.com/browse/SDN-3996
    matchingRules:
    - type: Always
    conditions:
    - status: True  # Apply=True if the risk is applied to the current cluster
      type: Applies
      reason: MatchingRule
      message: The matchingRules[0] matches
      lastTransitionTime: 2021-09-13T17:03:05Z
  - name: LeakedMachineConfigBlocksMCO
    message: MCO update might be blocked if some condition holds
    url: https://issues.redhat.com/browse/SDN-2212
    matchingRules:
    - type: PromQL
      promql:
        promql: prom_query
    conditions:
    - status: False
      type: Applies
      reason: NoMatchingRule
      message: None of the matching rules matches
      lastTransitionTime: 2021-09-13T17:03:05Z
  - name: DualStackNeedsController
    ...
  - name: RHELKernelHighLoadIOWait
    ...
  conditionalUpdates:
  - release:
      version: 4.18.16
      image: quay.io/openshift-release-dev/ocp-release@sha256:abc123
    riskNames:
    - DualStackNeedsController
    - OldBootImagesPodmanMissingAuthFlag
    - RHELKernelHighLoadIOWait
    risks:  # deprecated by riskNames
  - release:
      version: 4.18.17
    riskNames:
    - OldBootImagesPodmanMissingAuthFlag
    risks:  # deprecated by riskNames
  - release:
      version: 4.19.1
    riskNames:
    - LeakedMachineConfigBlocksMCO
    risks:  # deprecated by riskNames
```

When a conditional update is accepted, the names of its associated risks are going to be merged into `clusterversion.status.history.acceptedRisks` which is an existing field before this enhancement. For example, CVO's acceptance of `4.18.17` leads to `OldBootImagesPodmanMissingAuthFlag` being a part of value of `clusterversion.status.history.acceptedRisks`: The wording might look like "The target release ... is exposed to the risks [OldBootImagesPodmanMissingAuthFlag] and accepted by CVO because all of them are considered acceptable".

When a conditional update is blocked, there is a condition in `clusterversion.status.conditions` with `ReleaseAccepted=False`, e.g.,

```yaml
- lastTransitionTime: 2022-10-11T14:16:13Z
  message: Payload loaded version="4.18.16" ... is blocked because it is exposed to unaccepted risks [RHELKernelHighLoadIOWait]
  reason: UnacceptedRisks
  status: False
  type: ReleaseAccepted
```

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

Yes, ClusterVersion and the `oc adm upgrade` changes we're proposing are directly applicable to standalone clusters, without needing further integration work in other components.

#### Single-node Deployments or MicroShift

This proposal is applicable to single-node but not to MicroShift which lacks a ClusterVersion and CVO, and manages updates via RPMs.

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
