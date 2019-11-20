---
title: ingress-nodeport-publishing
authors:
  - "@ironcladlou"
reviewers:
  - "@ironcladlou"
  - "@smarterclayton"
  - "@knobunc"
  - "@Miciah"
  - "@danehans"
  - "@frobware"
approvers:
  - "@smarterclayton"
  - "@knobunc"
creation-date: 2019-11-05
last-updated: 2019-11-05
status: provisional
see-also:
replaces:
superseded-by:
---

# IngressController NodePort Publishing Strategy

This enhancement proposes the addition of a new NodePort publishing strategy to the  [ingresscontrollers.operator.openshift.io API](https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go).

The NodePort strategy is positioned as a preferred alternative to most uses of the existing HostNetwork strategy, and is proposed as a new default in all contexts where HostNetwork is currently chosen by OpenShift.

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

The `HostNetwork` strategy typically does the job, but has significant operational drawbacks:

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
  // service. The node ports are dynamically allocated by OpenShift.
  //
  // To support static port allocations, user changes to the node port
  // field of the managed Service will preserved.
  NodePort *NodePortStrategy `json:"nodePort,omitempty"`
}

// NodePortStrategy has no additional configuration.
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

**Note:** It is also proposed that `NodePort` become the default publishing strategy for all platforms which currently default to `HostNetwork`.

### User Stories

#### Story 1

As an OpenShift administrator, I want to integrate the default ingresscontroller with a self-managed load balancer directly through node ports in a way that maximizes ingress utilization and ingresscontroller scheduling flexibility.

### Implementation Details/Notes/Constraints

One critical architectural detail of this proposal which demands scrutiny is the following constraint:

* The Ingress Operator will ignore any updates to `.spec.ports[].nodePort` fields of the Service.

By making explicit that users own the  `.spec.ports[].nodePort` field, no additional port configuration API should be required. By default, ports are allocated automatically and users can discover the actual port allocations for integrations. However, sometimes static port allocations are necessary to integrate with existing infrastructure which may not be easily reconfigured in response to dynamic ports. To achieve integrations with static node ports, users can update the managed `Service` resource directly.

Because OpenShift isn't managing anything connected to the NodePort service, the ports used to expose the IngressController are irrelevant and can be left to the discretion of the administrator (constrained only by the cluster node port CIDR configuration).

If in the future the `NodePort` strategy API gains a port configuration field, it should be possible in a future enhancement for the ingress operator to selectively assume ownership of `.spec.ports[].nodePort`.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

This new API should be covered by e2e tests similar to those which exist already for the other publishing strategies.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

#### Add port configuration to HostNetwork API

One alternative to a NodePort strategy is to add user-defined port configuration to the `HostNetwork` publishing strategy. Configurable ports would enable co-location of `HostNetwork` shards, but would not resolve co-location during rollout. Additionally, end-users would be responsible for port conflict resolution barring the specification of some  port allocation strategy (which is already solved by `NodePort`).


