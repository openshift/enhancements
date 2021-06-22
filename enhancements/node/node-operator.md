---
title: node-operator
authors:
  - "@saschagrunert"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-06-22
last-updated: 2021-06-22
status: provisional
see-also: {}
replaces: {}
superseded-by: {}
---

# Node Operator (NOP)

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The overall goal of creating this new operator in OpenShift is the encapsulation
of CRI-O and kubelet related configurations, metrics and alerts within a single
domain and namespace.This provides on one hand a higher flexibility for the Node
team, while on the other hand the dependency on external teams gets reduced. In
the long-term the Node team would only rely on OpenShift internal APIs to manage
the container runtime and kubelet related configurations. Additional deployments
for proxying runtime metrics or providing telemetry access are part of this
operator, too.

## Motivation

Most of the node related configurations are today part of the [Machine Config
Operator (MCO)][mco]. Because the MCO got bigger over time, their long-term goal
is to provide an API for machine related configuration purposes. The new
operator would use this API in a later migration step.

The CRI-O metrics are today served by using a [`kubelet`
ServiceMonitor][kubelet-monitor] and a corresponding Prometheus relabeling. This
happens as part of the [Cluster Monitoring Operator (CMO)][cmo] project. The CMO
team also aims to provide an API for external teams to integrate their
metrics retrieval into the OpenShift cluster monitoring. In the same way as for
the MCO, the new node operator would use this API to integrate the runtime
metrics retrieval natively into OpenShift by using a dedicated ServiceMonitor
secured by TLS and RBAC.

[mco]: https://github.com/openshift/machine-config-operator
[cmo]: https://github.com/openshift/cluster-monitoring-operator
[kubelet-monitor]: https://github.com/openshift/cluster-monitoring-operator/blob/8ec92e1/assets/control-plane/service-monitor-kubelet.yaml#L95-L116

### Goals

- Near-term:
  - Create a new Node owned project to be prepared for future changes of the MCO
    and CMO.
  - Build in CRI-O metrics retrieval by using TLS and RBAC with the help of a
    kube-rbac-proxy DaemonSet.
  - Include TLS certificate rotation (on disk as well as in a secret) for the
    CRI-O metrics.
- Long-term:
  - Migrate the kubelet and CRI-O configurations into the operator.
  - Include [OpenTelemetry][opentelemetry] support for a node.

[opentelemetry]: https://opentelemetry.io

### Non-Goals

Re-implementing logic from the MCO or CMO, we will rely on their APIs if they're
available.

## Proposal

The main proposal is integrating a new operator into OpenShift, which is owned
by the Node team. This includes the whole supply chain around CI/CD and release
management.

The first implementation of the operator will consist of:

- A dedicated namespace where the operator lives and acts on.
- A controller managing the reconciliation of the statically provided assets,
  which are:
  - RBAC rules as well as a ServiceAccount for the kube-rbac-proxy based metrics
    server.
  - SecurityContextConstraints to allow HostPorts and HostNetwork for the
    DaemonSet.
  - A DaemonSet to retrieve the metrics for every node.
  - A secret for the TLS cert/key for the metrics retrieval (used by CRI-O as
    well as the kube-rbac-proxy).
  - A service to access the metrics.
  - A ServiceMonitor to integrate the metrics into OpenShift.
- A reconciler for the required TLS cert/key. This reconciliation loop will
  recreate the certificate if nearly expired and will sync them to disk (for
  CRI-O) as well as into the existing secret (for kube-rbac-proxy). CRI-O
  already supports on-the-fly certificate reload for its secure metrics
  endpoint.

A proof of concept integration can be found [within the CMO][poc].

Because the whole monitoring stack should be integrated into the OpenShift
cluster monitoring (not the user workload monitoring), it may be possible that
the CMO has to be modified to accept the ServiceMonitor within the CRI-O
operator namepsace. A corresponing RBAC cluster role binding to allow the
metrics retrieval between both namespaces will be required, too:

```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: crio-metrics-client
rules:
  - nonResourceURLs:
      - /metrics
    verbs:
      - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: crio-metrics-client
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: crio-metrics-client
subjects:
  - kind: ServiceAccount
    name: prometheus-k8s
    namespace: openshift-monitoring
```

[poc]: https://github.com/openshift/cluster-monitoring-operator/pull/1219

### User Stories

As a Node team, I want to have full control over how node related metrics and
alerts get managed.

As a Node team, I want to ensure future proofness for the latest development
efforts in OpenShift.

As a cluster operator, I want to ensure that all metrics endpoints are secure
by default and covered by RBAC rules.

### Risks and Mitigations

#### A new project to integrate

The highest risk for OpenShift as well as the Node team is that we have to
maintain and integrate a new operator into the product. This will add initially
higher efforts on the team, while gaining more flexibility in a mid-to-long
term. A mitigation around this to keep the focus on metrics in the first place,
while having in mind to change the operator later on to fulfill the APIs from
the MCO and CMO.

Another risk is that we have a new operator managed by CVO. There is no
mitigation about that yet other than not implementing the operator.

## Design Details

### Test Plan

**Note:** _Section not required until targeted at a release._

<!--
Consider the following in developing a test plan for this enhancement:

- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).
-->

### Graduation Criteria

**Note:** _Section not required until targeted at a release._

<!--
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
-->

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

<!--
The idea is to find the best form of an argument why this enhancement should _not_ be implemented.
-->

## Alternatives

The alternative of creating a new project is integrating the required changes
for CRI-O metrics into the MCO or CMO. This would not align with the long-term
goals of the operators.
