---
title: multi-networkpolicy
authors:
  - "@s1061123"
reviewers:
  - "@dcbw"
  - "@danwinship"
  - "@dougbtv"
  - "@fepan"
  - "@zshi-redhat"
  - TBD
approvers:
  - TBD
creation-date: 2020-08-07
last-updated: yyyy-mm-dd
status: implementable
---

# NetworkPolicy for Multus Interfaces

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal to bring networkpolicy features for multus interface.

## Motivation

Customer wants to make their Kubernetes secure, by filtering traffic from
unexpected source. Kubernetes provides `NetworkPolicy` feature, however, it
is only applied to default network interface, not additional network interface
that is created by Multus CNI.  NetworkPolicy for Multus Interfaces brings the NetworkPolicy
mechanism to additional network interfaces and it provides customers the ability to filter
traffic from unexpected sources.

### Goals

- Introduce MultiNetworkPolicy CRD
- Introduce multi-networkpolicy which generates iptables from CRD as shown in the Design Details section
- Provide iptable based filtering mechanism for pod
- Gathering related information (iptables rules in pod network namespace) by must-gather

### Non-Goals

- Provide eBPF filtering mechanism
- Provide NetworkPolicy for userspace pod which uses DPDK and so on

## Proposal

### User Stories

#### Story 1

Customer can specify how groups of pods are allowed to communicate with each other
and other network endpoints as Kubernetes NetworkPolicy does.

### Implementation Details/Notes/Constraints

### Multus

No additional change is required because multi-networkpolicy only consumes 
network-attachment-definition CRD and it does not change net-attach-def CRD.

### How user enables multi-networkpolicy?

Currently we're going to bring it as Tech Preview first and then GA after a few release,
hence opt-in style configuration is planned at Tech Preview and make it default at GA.
Opt-in style configuration will be done at cluster-network-operator as boolean flag
(such as `useMultiNetworkPolicy: true`).


### Risks and Mitigations

## Design Details

### MultiNetworkPolicy samples

MultiNetworkPolicy CRD is defined in https://github.com/k8snetworkplumbingwg/multi-networkpolicy/blob/master/scheme.yml

MultiNetworkPolicy CRD has pretty similar schema to Kubernetes NetworkPolicy. The differences are
'apiVersion' and 'kind', to define it as CRD of MultiNetworkPolicy. Its semantics are same as well
as Kubernetes other than target interface annotation, `k8s.v1.cni.cncf.io/policy-for`. Target interface
is specified annotation, `k8s.v1.cni.cncf.io/policy-for`, as net-attach-def name.

```
apiVersion: k8s.cni.cncf.io/v1beta1
kind: MultiNetworkPolicy
metadata:
  name: policy-test1
  annotations:
    k8s.v1.cni.cncf.io/policy-for: macvlan-net1
spec:
  podSelector:
    matchLabels:
      role: http
  egress:
    - to:
      - podSelector:
          matchLabels:
            role: client
        namespaceSelector:
          matchLabels:
            test: default
    - to:
      - podSelector:
          matchLabels:
            role: not-client
      ports:
      - protocol: TCP
        port: 10080
      - protocol: TCP
        port: 20081
  ingress:
    - from:
      - podSelector:
          matchLabels:
            role: client
      ports:
      - protocol: TCP
        port: 80
    - from:
      - podSelector:
          matchLabels:
            role: not-client
      ports:
      - protocol: TCP
        port: 80
```

### Test Plan

Initially, multi-networkpolicy testing will be done via the upstream repo e2e test
using `kind` (in progress). Additionally we are going to use baremetal CI job.

Upstream repositories are:
 - https://github.com/k8snetworkplumbingwg/multi-networkpolicy (for scheme)
 - https://github.com/k8snetworkplumbingwg/multi-networkpolicy-iptables (implementation)

### Graduation Criteria

##### Dev Preview

- We have a mostly-passing periodic CI job at upstream

##### Dev Preview -> Tech Preview

- Gather feedback from users rather than just developers
- TBD

##### Tech Preview -> GA 

- More testing (upgrade, scale)
- Add CI job at baremetal OCP CI
- Sufficient time for feedback
- Available by default

### Upgrade / Downgrade Strategy

multi-networkpolicy just uses CRD, net-attach-def and MultiNetworkPolicy and no Kubernetes object is used,
hence we could say that it is not kubernetes-version sensitive.

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

Packet filtering could be done not only iptables also elsewhere (like ToR switch), but none is integrated
with Kubernetes secondary network interface.

## Infrastructure Needed

OpenShift 4.x cluster
