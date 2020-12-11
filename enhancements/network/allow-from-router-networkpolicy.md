---
title: "Allow-From-Router" NetworkPolicy
authors:
  - "@danwinship"
reviewers:
  - "@squeed"
  - "@trozet"
  - "@Miciah"
approvers:
  - "@knobunc"
  - "@squeed"
creation-date: 2020-12-10
last-updated: 2020-12-10
status: implementable
---

# "Allow-From-Router" NetworkPolicy

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Operational readiness criteria is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Many users want to create NetworkPolicies that say, effectively,
"allow traffic from routers". However, the router can run with one of
several different "endpoint publishing strategies", which use the
cluster network differently, and when using the `HostNetwork`
strategy, the NetworkPolicy-relevant behavior also varies depending on
the network plugin in use. This makes it impossible to create an
"allow traffic from routers" policy that will work in any cluster.
Indeed, in some cases, it is difficult to create such a policy even
just for a single cluster, even when you know exactly how the cluster
is configured.

## Motivation

### Goals

- Allow a user to create a policy which will allow traffic to their
  pods from routers (and not from arbitrary pod-network pods), which
  will work regardless of network plugin or router endpoint publishing
  strategy.

### Non-Goals

- Allowing a user to create a policy which is 100% guaranteed to allow
  traffic _only_ from routers (and, eg, not from other host-network
  pods).

## Proposal

### Background

When the router is run with the `NodePortService` or
`LoadBalancerService` endpoint publishing strategy, the router pods
run on the pod network, and so router traffic can be allowed by
creating NetworkPolicies that allow traffic based on an appropriate
`namespaceSelector`. We currently label the `openshift-ingress`
namespace with the label `network.openshift.io/policy-group: ingress`
for exactly this purpose.

However, when the router is run with the `HostNetwork` endpoint
publishing strategy, the router pods will be on the host network, and
so their traffic will not be recognized as coming from any specific
namespace.

In OpenShift SDN, for somewhat accidental historical reasons, traffic
coming from any non-pod source is treated as though it came from the
namespace "`default`". Thus, it's possible to allow traffic from
routers by adding a label to `default` and then allowing traffic based
on that label. (This will also allow traffic from any other
host-network pod, or node process.) However, this approach does not
work with any other network plugin.

In theory, with other plugins, if you knew the pod network CIDR, you
could create a policy saying "allow from anywhere (that can route to
the pod network) except the pod network", which is essentially
equivalent to "allow from `default`" in OpenShift SDN. eg:

    ipBlock:
      cidr: 0.0.0.0/0
      except:
        - 10.128.0.0/14

This is not ideal, because it requires you to know the pod network
CIDR, but even given that, it would not actually work with many
network plugins anyway. Eg, in ovn-kubernetes, when a node or a
host-network pod sends traffic to a pod-network pod, the source IP is
not the node's "primary" node IP but rather the IP associated with the
`ovn-k8s-mp0` interface, which is inside the pod network CIDR (eg, it
would be `10.128.6.2` on the node that has the `10.128.6.0/23`
subnet).

It would be possible to create a policy allowing specifically the
relevant source node IP of each node, but such a policy would need to
be updated any time nodes were added or removed from the cluster.

(There has been some discussion upstream about allowing [`nodeSelector`
in NetworkPolicies] but this is not going to happen any time soon, if
it happens at all.)

[`nodeSelector` in NetworkPolicies]: https://github.com/kubernetes/kubernetes/issues/51891

### Plan

There does not seem to be any good way to solve this completely
network-plugin-agnostically at the current time, so we won't worry
about Kuryr, Calico, etc, at this time. (Though Kuryr could choose to
implement the same hack I propose for OVN Kubernetes.)

We also don't want to diverge too much from upstream/"stock"
NetworkPolicy, so trying to add `nodeSelector` is out.

The simplest approach would be to formalize OpenShift SDN's ability to
"allow from host-network", and add a similar behavior to OVN
Kubernetes. I believe this could be implemented by having
ovn-kubernetes add the management port IP and "primary" node IP of
each node to the address set for some "host-network-matching"
namespace. (Management port IP would be needed to make node-to-pod
ingress policies work correctly, while the primary node IP would be
needed to make pod-to-node egress policies work correctly.)

The question then is what namespace this should be; I don't think it
should be "`default`" like in openshift-sdn, since that namespace is
not actually supposed to be "magic" in this way. In OCP, we'll want
the "host-network-matching" namespace to be something starting with
"`openshift-`", but we can't hardcode that upstream.

So the best solution seems to be that we add a config option
`host-network-namespace` to the `[kubernetes]` section of the config.
If this option is specified, and the indicated namespace exists, then
ovn-kubernetes will do whatever it is that it has to do, to make it so
that NetworkPolicies that select that namespace get applied to
host-network traffic.

So, then, the plan is:

  1. When using ovn-kubernetes, CNO will create a namespace
     `openshift-host-network`, and configure ovn-kubernetes in OCP
     with `host-network-namespace=openshift-host-network`.

  2. For consistency, we modify openshift-sdn to treat
     `openshift-host-network` the same way (see below for more
     details).

  3. CNO will watch the ingress configuration, and when the endpoint
     publishing strategy is `HostNetwork`, it will add the
     `network.openshift.io/policy-group: ingress` label to
     `openshift-host-network`, and when it's not `HostNetwork` it will
     remove that label.

Then users will be able to match router traffic in either
openshift-sdn or ovn-kubernetes with any router endpoint publishing
strategy via:

    ingress:
      - from:
          - namespaceSelector:
              matchLabels:
                network.openshift.io/policy-group: ingress

We can also formally specify "allow-from-host-network". (This is
basically free at this point, and also useful to users, so...) We just
have CNO also add the label `network.openshift.io/policy-group:
host-network` to `openshift-host-network`, and then we document that
(at least when using openshift-sdn or ovn-kubernetes), a NetworkPolicy
selecting namespaces with that label will match host-network traffic.

#### OpenShift SDN Modifications

We _could_ avoid modifying openshift-sdn, and just have CNO know that
the "host network" namespace is `openshift-host-network` with
ovn-kubernetes, but `default` with openshift-sdn. However, it would be
simpler if we just always used the same namespace as the "host network
namespace".

In NetworkPolicy mode, openshift-sdn doesn't let you have two
namespaces with the same VNID, but we could easily hack it so that
`openshift-host-network` was treated as VNID 0 _for purposes of
NetworkPolicy only_. This would cause it to behave strangely if there
were any pods in it, but we don't really care, since there _won't_ be
any pods in it.

### Risks and Mitigations

The biggest problem is that there is really no good way to distinguish
"traffic that's actually from a host-network router" from "random
other host-network traffic", so users might end up allowing more
traffic than they want to. This is mostly a documentation problem,
since there really isn't a workaround if they are using `HostNetwork`.

### Test Plan

We will add ovn-kubernetes-specific tests for the new ovn-kubernetes
feature upstream.

We will add tests to origin to ensure that the
`network.openshift.io/policy-group: host-network` label works as
described above, when using openshift-sdn or ovn-kubernetes.

We should also test that the `policy-group: ingress` label is applied
correctly in `HostNetwork` endpoint publishing mode, and that
NetworkPolicies based on that label work correctly regardless of
endpoint publishing mode. I'm not sure if we currently run the origin
e2e suite with `HostNetwork` endpoint publishing anywhere though...

### Graduation Criteria

#### Tech Preview -> GA

- Just one release of it being vaguely useful seems like enough? It's
  not that complicated a feature...

### Upgrade / Downgrade Strategy

N/A; if people rely on the feature and then downgrade, then their
policies will stop matching and their pods will become isolated, but
the fix is to just remove the policy until they upgrade again.

### Version Skew Strategy

N/A; no clusters should contain any policies using the new feature
until after they are fully upgraded to a release in which the feature
exists. At any rate, even if they did, CNO would be upgraded before
SDN/OVN, so SDN/OVN that implement the feature would never run in a
cluster with a CNO that doesn't implement the feature.

## Implementation History

- 2020-12-10: Initial proposal
