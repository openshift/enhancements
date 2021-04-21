---
title: add-kubernetes-nmstate-operator-to-cvo
authors:
  - "@bcrochet"
  - "@yboaron"
  - "@hardys"
reviewers:
  - "@cgwalters"
  - "@derekwaynecarr"
  - "@russellb"
approvers:
  - TBD
creation-date: 2021-03-16
last-updated: 2021-04-23
status: provisional
see-also:
  - "/enhancements/machine-config/mco-network-configuration.md"
---

# Add Kubernetes NMState Operator to release payload

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Enable Kubernetes NMstate by default for selected platforms.

Network interface configuration is a frequent requirement for some platforms, particularly baremetal.
In recent releases [work was completed]("/enhancements/machine-config/mco-network-configuration.md")
to enable Kubernetes NMstate (optionally via OLM) so that declarative configuration
of secondary network interfaces is possible.

To improve the user-experience it is desirable to enable the NMState API by default on selected platforms
where such configuration is common.

## Motivation

In previous discussion it was noted that we may [not want NMState enabled on all platforms](https://github.com/openshift/enhancements/pull/161#discussion_r433303754) and also that [all tech-preview features must require opt-in](https://github.com/openshift/enhancements/pull/161#discussion_r433315534).

Now, the support status of NMState is moving from tech-preview to fully supported, the main consideration is how to conditionally enable this only on platforms where it's likely to be needed.

If we can enable Kubernetes NMstate as part of the release payload, we can improve the user experience on platforms where it is required - for example allow for NodeNetworkConfigurationPolicy to be provided via manifests at install-time via openshift-installer.

Additionally, having the Kubernetes NMstate as part of the release payload will allow uniformity between the various platforms (CNV, IPI-Baremetal).

Currently, CNV uses [Hyperconverged Cluster Operator - HCO](https://github.com/kubevirt/hyperconverged-cluster-operator) to deploy [custom operator - CNAO](https://github.com/kubevirt/cluster-network-addons-operator#nmstate), the CNAO installs Kubernetes NMstate, among other network related items.
While IPI-Baremetal uses a [new operator](https://github.com/openshift/kubernetes-nmstate/blob/master/manifests/kubernetes-nmstate-operator.package.yaml) to install Kubernetes NMstate using OLM.

## Q&A

**Q:** why NMSTATE capability should run everywhere (on-prem, cloud, edge) in all delivery models (self-managed, hosted) ?

**A:** If it was supported/possible, it would have been better to deploy nmstate capability only in required platforms.


**Q:** Is the signal/noise ratio by having this API/controller everywhere worth the potential confusion for an SRE in a particular infra, location, or delivery model when debugging an issue?

**A:** For the tech-preview phase we have a lightweight controller that only runs a deployment (replica=1) and NMSTATE CRD definition ,
creating NMSTATE CR instance triggers the full API controller deployment.
With this approach, we shouldn't have the full NMSTATE API everywhere.


**Q:** What existing SLO could deliver the content and not confuse the user? e.g. could Cluster Network Operator deliver this content optionally based on its own local config like storage/csi?

**A:** TBD


**Q:** What is the resource budget for this API/controller?
To enable it by default, we should understand its impact to the control plane:
- write rate to the API server
- number of clients that write to the resource (is it per node? is there a single controller co-located with api-server?)
To enable it by default, we should understand its impact to each worker:
- what does it watch?
- per node daemon resource cost (cpu/memory)
- per node daemon priority (is it node critical?)
- per node authority to write to the corresponding resource (think node restriction admission plugin in k8s as example)
- any unique backup/restore semantics introduced by the component

**A:** The resources composed of operator and handler (operand) resources.
Check [this document](https://docs.google.com/document/d/1qFumb1P01NOIl1gCq79Q0I8BkeaA5UQY9spqlOJjy-I/edit?usp=sharing) for detailed description of resources.


**Q:** What is Nmstate  future impact to other topologies (single node or externalized control planes)?
For single node
understanding what namespace hosts the component, and ensuring it fits in the cpuset isolation for mgmt components
For externalized control planes
is this needed only for the 'first' mgmt cluster and its workers?
is this needed for every worker in all agnostic clusters, and therefore should be hosted with an externalized control plane because its needed to support any 'join worker' flow.

**A:** SNO deployments using none platform will be unaffected as this operator will not be enabled by default.

TODO - do we need a way to enable this for none-platform, particularly for UPI metal as ideally we’d want all metal customers to have access to the same day-2 API (even if not by default)?

For externalized controlplane deployments this would be needed for both the management cluster and the externalized controlplane, so that the configuration of NICs can be handled in a consistent way - it’s an open question how we’d handle this in the hybrid platform case though...


**Q:** is it ok to deliver the content in a release payload, but enable it via some other configuration knob?

**A:** The desire is to enable a common enabled-by-default API for network configuration (initially only for selected platforms where that is commonly needed).

If that approach is not acceptable then delivering via the release payload with some way to opt-in would be reasonable.


**Q:** What is the knob to turn on/off the capability? This is related to the SLO chosen to deliver this new meta-operator and what configuration it reads to know to turn it on. For example, if this is for selected platforms, the component would read the Infrastructure CR to determine if it should enable it all, but is there another knob that would allow opt-in/out?

**A:** The current proposal is to couple this to the platform detected in the Infrastructure CR, so it can be enabled by default on selected platforms (particularly baremetal).

There could also be some other interface at the SLO level to override that behavior and/or to enable opt-in for platforms where it’s not enabled by default.


### Goals

- Provide NMState APIs by default on desired platforms, [NetworkManager configuration should be updated](https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/mco-network-configuration.md#option-c-design) to support this capability on these platforms.
- Allow using NMState APIs at install-time, where NIC config is a common requirement.

### Non-Goals

- Make persistent networking changes to nodes. That should be handled by Machine Config Operator ( check [this link](https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/mco-network-configuration.md#option-c) for more details).
- Provide an API for controlplane network configuration via openshift-install (although in future a common API would be desirable for controlplane and secondary network interfaces)

## Proposal

### User Stories

#### Story 1

As a user of Baremetal IPI I want to provide NodeNetworkConfigurationPolicy resources via manifests at install-time, to simplify
my deployment workflow and avoid additional post-deploy steps.

I'd like to avoid dependencies on additional registries in disconnected environments and enable core platform functions
such as network configuration directly from the release payload.

#### Story 2

As a user of Openshift Virtualization I want to apply a NodeNetworkConfigurationPolicy to create network resources at nodes that will connet to VMs nics and use the already deployed kubernetes-nmstate version from openshift to reduce deployment complexity and maintenance.


#### Story 3

As a user of Baremetal IPI I want to configure (for example MTU, bonding) secondary network interfaces used for storage traffic.

### Risks and Mitigations

Exposes a method to modify host networking. If a configuration is applied that breaks connectivity to the API, then it will be rolled back automatically.

## Design Details

### Open Questions [optional]

1. Is it possible to install Kubernetes NMstate only on certain platforms (maybe using [cluster profiles](https://github.com/openshift/enhancements/blob/e598d529ba89e29bc0b48bfdb9818710a6392414/enhancements/update/cluster-profiles.md) )?

2. Is it acceptable to adopt the [Cluster Baremetal Operator approach](https://github.com/openshift/enhancements/blob/4f5171c5a10f5fdeebc37cb6c6db85e3222e3ffe/enhancements/baremetal/an-slo-for-baremetal.md#not-in-use-slo-behaviors) ?

### Test Plan

Will be tested with an E2E test suite that also runs upstream. There is also a unit test suite.

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

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Kubernetes NMState Operator is currently available in the Red Hat Catalog as Tech Preview

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Conduct load testing
- E2E testing

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
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to this should be
  identified and discussed here.
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

## Implementation History

- Kubernetes NMState Operator being built by ART.
- Kubernetes NMState Operator available in Red Hat Catalog as Tech Preview

## Drawbacks

1.

## Alternatives

1. CNV continues to install Kubernetes NMstate as they do today

## Infrastructure Needed [optional]

At a minimum, for e2e test suite, the worker nodes of the SUT would need 2 additional NICs available.
