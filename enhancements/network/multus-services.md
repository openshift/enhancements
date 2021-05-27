---
title: multus service abstraction
authors:
  - "@s1061123"
reviewers:
  - TBD
  - "@alicedoe"
approvers:
  - TBD
  - "@oscardoe"
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
status: implementable
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Multus Service Abstraction


## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This propose introduces to implement Kubernetes service in secondaly network interface, which is created by multus CNI. Currently pods' secondary network interfaces made by multus is out of Kubernetes network, hence Kubernetes network functionality cannot be used, such as network policy and service. This proposal introduces several components into OpenShift and try to implement some of the Kubernetes service for pods' secondary network interfaces.

## Motivation

Kubernetes Service object is commonly used abstraction to access Pod workloads. User can define service with label selector and the service provides load-balancing mechanisms to access the pods. This is very useful for usual network services, such as HTTP server. Kubernetes provides various service modes to access, such as ClusterIP, LoadBalancer, headless service, ExternalName and NodePort, not only for inside cluster also for outside of cluster.

These functionality is implemented in various components (e.g. endpoint controller, kube-proxy/openvSwitch) and that is the reason we cannot use it for pods' secondary network because pods' secondary network is not under management of Kubernetes.

### Goals

- Provide a mechanism to access to endpoints (by virtual IP address, DNS names), which is organized by Kubernetes service objects, in phased approach.

### Non-Goals

Due to the variety of service types, this must be handled in a phased approach. Therefore this covers the initial implementation and the structural elements, some of which may share commonalities with the future enhancements.

- Provide whole Kubernetes service functionality for secondary network interface.
- Provide completely same semantics for Kubernetes service mechanism
- LoadBalancer features for pods' secondary network interface (this requires more discussion in upstream community)
- Ingress and other functionalities related to Kubernetes service (this will be addressed in another enhancement)
- Network verification for accesssing services (i.e. user need to make sure reachability to the target network)
- Service for universal forwarding mechanisms (e.g DPDK Pods are not supported), we only focus on the pods which uses Linux network stack (i.e. iptables)

## Proposal


### Target Service Functionality

We targets the following service functionality for this proposal:

- ClusterIP
- headless service
- NodePort

#### Service Reachability

The service can be accessed from the pods that can access the target service network. For example, if 'ServiceA' is created for 'net-attach-def1', then 'ServiceA' can be accessed from the pods which has 'net-attach-def1' network interface and we don't provide any gateway/routers for access from outside of network in this proposal.

#### Cluster IP for multus

When the service is created with 'type: ClusterIP', then Kubernetes assign cluster virtual IP for the services. User can access the services with given virtual IP. The request to virtual IP is automatically replaced with actual pods' network interface IP and send to the target services. User needs to make sure reachability to the target network, otherwise the request packet will be dropped.

#### Headless service

XXX: fill 

#### NodePort

XXX: Fill (note: NodePort only exposes port of pods' secondary network interface)

### User Stories

XXX: need to add user stories for cluseter IP/headless services (inside cluster access)
XXX: need to add user stories for NodePort services (outside cluster access)

### Implementation Details/Notes/Constraints [optional]

In this enhancement, we will introduce following components (components name might be changed at development/community discussion):

- Multus Service Controller
- Multus Proxy

Multus service controller watches all Pods' secondary interfaces, services and network attachment definitions and it creates EndpointSlice for the service. EndpointSlice contains secondary network IP address. Multus proxy watches. Service and EndpointSlice has special label, `service.kubernetes.io/service-proxy-name`, which is defined at [kube-proxy APIs](https://pkg.go.dev/k8s.io/kubernetes/pkg/proxy/apis), to make target service out of Kubernetes management.

#### Create Service

1. Service is created

User creates Kubernetes service object. At that time, user needs to add following:

- k8s.v1.cni.cncf.io/service-networks

`k8s.v1.cni.cncf.io/service-networks` specifies which network (i.e. net-attach-def) is the target to exporse the service.
XXXX: TBD

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

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

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
