---
title: ingress-nodeport-publishing
authors:
  - "@ironcladlou"
reviewers:
  - "@ironcladlou"
  - "@smarterclayton"
  - "@knobc"
  - "@Miciah"
  - "@danehans"
  - "@frobware"
approvers:
  - "@smarterclayton"
  - "@knobc"
creation-date: 2019-11-05
last-updated: 2019-11-05
status: provisional
see-also:
replaces:
superseded-by:
---

# IngressController NodePort Publishing Strategy

This enhancement proposes the addition of a new NodePort publishing strategy to the  [ingresscontrollers.operator.openshift.io API](https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go).

The NodePort strategy is positioned as a preferred alternative to most uses of the existing HostNetwork strategy.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

When possible, OpenShift will expose IngressControllers using the [LoadBalancerService publishing strategy](https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go). Some OpenShift administrators (even on cloud platforms) don't want OpenShift to manage a cloud load balancer and DNS for their IngressControllers. These administrators generally want IngressControllers to be exposed through node ports to enable custom integration with a front-end load balancing solution.

## Motivation

Administrators today have two alternatives to the `LoadBalancerService` publishing strategy:

1. Use the `Private` strategy and expose the IngressController manually.
2. Use the `HostNetwork` strategy and integrate with the resulting static node ports that expose the IngressController.

The `Private` strategy is not ideal because the administrator becomes responsible for managing Kubernetes Service and possibly other resources to expose the IngressController, and OpenShift is unable to provide any management value (e.g. upgrades, monitoring).

The `HostNetwork` typically does the job, but has significant operational drawbacks:

1. HA rollouts require node headroom to host new versions. Because the IngressController pods use statically defined ports on the host network interface, new revisions of the pods can't be colocated on the same node, requiring either additional nodes for scale-up or a toleration for reducing availability for a scale-down prior to scale-up.
2. IngressController shards require dedicated sets of nodes. Because of the static host port allocation prohibiting colocation, pods of discrete shards of IngressControllers can't live on the same node even when resources might allow it. This results in poor utilization.

A `NodePort` strategy gives administrators node ports for IngressControllers for integrations, but without the drawbacks of `HostNetwork`. IngressController pods exposed by `NodePort` can be colocated, solving the HA rollout and utilization problems of `HostNetwork`.

### Goals

* Create an API which supports the minimum viable NodePort publishing strategy that preserves the possibility of later adding more configuration to the strategy.

### Non-Goals

* To keep the API as focused as possible, this proposal does not specify a specific API to configure the node port allocation. However, nothing in this proposal should prevent a such a followup enhancement.

## Proposal

The following changes are proposed to the [ingresscontrollers.operator.openshift.io API](https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go).

```go
NodePortStrategyType EndpointPublishingStrategyType = "NodePort"

type EndpointPublishingStrategy struct {
  // <existing fields omitted>

  // nodePortStrategy exposes ingress controller pods using a NodePort
  // service. The node ports are dynamically allocated by OpenShift. Changes
  // to the node port field of the managed Service will honored.
  NodePort *NodePortStrategy `json:"nodePort,omitempty"`
}

type NodePortStrategy struct {
}
```

The [Ingress Operator](https://github.com/openshift/cluster-ingress-operator) will implement the `NodePort` strategy by exposing IngressControllers with Service like this:


```yaml
apiVersion: v1
kind: Service
metadata:
  name: router-default
  namespace: openshift-ingress
  annotations:
    operator.openshift.io/node-port-service-for: default
spec:
  type: NodePort
  externalTrafficPolicy: Local
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: http
  - name: https
    port: 443
    protocol: TCP
    targetPort: https
  selector:
    ingresscontroller.operator.openshift.io/deployment-ingresscontroller: default
```

The Ingress Operator will ignore any updates to `.spec.ports[].nodePort` fields of the Service.

### User Stories [optional]

Detail the things that people will be able to do if this is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they releate.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

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
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

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
