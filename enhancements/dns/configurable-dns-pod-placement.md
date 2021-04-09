---
title: configurable-dns-pod-placement
authors:
  - "@Miciah"
reviewers:
  - "@candita"
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@miheer"
  - "@rfredette"
  - "@sgreene570"
approvers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
creation-date: 2021-02-23
last-updated: 2021-03-29
status: implementable
see-also: 
replaces:
superseded-by:
---

# Configurable DNS Pod Placement

This enhancement enables cluster administrators to configure the placement of
the CoreDNS Pods that provide cluster DNS service.

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The DNS operator in OpenShift 4.7 and prior versions manages a DaemonSet that
serves two functions: running CoreDNS and managing node hosts' `/etc/hosts`
files.  This enhancement, in OpenShift 4.8, replaces this single DaemonSet with
two: one DaemonSet for CoreDNS and one DaemonSet for managing `/etc/hosts`.
Additionally, this enhancement adds an API to enable cluster administrators to
configure the placement of the CoreDNS Pods.

## Motivation

OpenShift 4.7 uses a single DaemonSet for both CoreDNS and for managing node
hosts' `/etc/hosts` files.  Specifically, this DaemonSet has a container that
adds an entry for the cluster image registry to `/etc/hosts` to enable the
container runtime (which does not use the cluster DNS service) to resolve and
thus pull from the cluster image registry.

Because `/etc/hosts` needs to be managed on every node host, this DaemonSet must
run on every node host.  Moreover, management of `/etc/hosts` is a critical
service because the node host may fail to pull images (including those of core
components) unless `/etc/hosts` has an entry for the cluster image registry.
Consequently the DaemonSet has a toleration for all taints so that the DNS Pod
always runs on all nodes.

Some cluster administrators require the ability to configure DNS not to run on
certain nodes.  For example, security policies may prohibit communication
between certain pairs of nodes; a DNS query from an arbitrary Pod on some node A
to the DNS Pod on some other node B might fail if some security policy prohibits
communication between node A and node B.

Splitting CoreDNS and management of `/etc/hosts` into separate DaemonSets makes
it possible to remove the blanket toleration for all taints from the CoreDNS
DaemonSet while keeping the blanket toleration on the DaemonSet that manages
`/etc/hosts`.  Splitting the DaemonSet also makes it possible to enable use of a
custom node selector on the CoreDNS DaemonSet.

Another advantage of using separates DaemonSets is that the DaemonSet that
updates `/etc/hosts` can use the host network to avoid consuming SR-IOV devices
on nodes that have Smart NICs.

### Goals

1. Separate CoreDNS and management of `/etc/hosts` into separate DaemonSets.
2. Enable cluster administrators to control where the CoreDNS DaemonSet is scheduled.

### Non-Goals

1. Enable cluster administrators to control the placement of the DaemonSet that manages `/etc/hosts`.
2. Enforce security policies.

## Proposal

This enhancement has two distinct parts.  First, the DNS operator, which manages
the "dns-default" DaemonSet, is modified to manage an additional "node-resolver"
DaemonSet, and the "dns-node-resolver" container, which manages `/etc/hosts`, is
moved from the "dns-default" DaemonSet to a new "node-resolver" DaemonSet.  As
part of this change, the toleration for all taints is removed from the
"dns-default" DaemonSet.  The DaemonSet that manages `/etc/hosts` has blanket
toleration and uses the host network.  From the cluster administrator's
perspective, this DaemonSet split is an internal change.

Second, a new API is provided to enable cluster administrators to specify the
desired placement of the "dns-default" DaemonSet's Pods, which, due to the first
change, only run CoreDNS and no longer must be scheduled to every node.  This
new API is the user-facing part of this enhancement.

The DNS operator API is extended by adding an optional `NodePlacement` field
with type `DNSNodePlacement` to `DNSSpec`:

```go
// DNSSpec is the specification of the desired behavior of the DNS.
type DNSSpec struct {
	// ...

	// nodePlacement provides explicit control over the scheduling of DNS
	// pods.
	//
	// Generally, it is useful to run a DNS pod on every node so that DNS
	// queries are always handled by a local DNS pod instead of going over
	// the network to a DNS pod on another node.  However, security policies
	// may require restricting the placement of DNS pods to specific nodes.
	// For example, if a security policy prohibits pods on arbitrary nodes
	// from communicating with the API, a node selector can be specified to
	// restrict DNS pods to nodes that are permitted to communicate with the
	// API.  Conversely, if running DNS pods on nodes with a particular
	// taint is desired, a toleration can be specified for that taint.
	//
	// If unset, defaults are used. See nodePlacement for more details.
	//
	// +optional
	NodePlacement DNSNodePlacement `json:"nodePlacement,omitempty"`
}
```

The `DNSNodePlacement` type has fields to specify a node selector and
tolerations:

```go
// DNSNodePlacement describes the node scheduling configuration for DNS pods.
type DNSNodePlacement struct {
	// nodeSelector is the node selector applied to DNS pods.
	//
	// If empty, the default is used, which is currently the following:
	//
	//   beta.kubernetes.io/os: linux
	//
	// This default is subject to change.
	//
	// If set, the specified selector is used and replaces the default.
	//
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// tolerations is a list of tolerations applied to DNS pods.
	//
	// The default is an empty list.  This default is subject to change.
	//
	// See https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
	//
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}
```

By default, DNS Pods run on untainted Linux nodes.  The `NodePlacement` field
enables cluster administrators to specify alternative parameters.  For example,
the following DNS specifies that DNS Pods should run only on "infra" nodes
(i.e., nodes that have the "node-role.kubernetes.io/infra" label and taint):

```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  nodePlacement:
    nodeSelector:
      kubernetes.io/os: linux
      node-role.kubernetes.io/infra: ""
    tolerations:
    - effect: NoSchedule
      key: "node-role.kubernetes.io/infra"
      operator: Exists
```

Note that the new `NodeSelector` field has type `map[string]string`, rather than
`*metav1.LabelSelector`, to match the type on the DaemonSet's
`spec.template.spec.nodeSelector` field's type.  Thus the user can specify label
key-value pairs to match but cannot specify arbitrary match expressions as using
`metav1.LabelSelector` would allow.

### Validation

Omitting `spec.nodePlacement` or its subfields specifies the default behavior.

The API validates that `spec.nodePlacement.nodeSelector`, if specified, is a
valid node selector and that `spec.nodePlacement.tolerations`, if specified, is
a list of valid tolerations.

### User Stories

#### As a cluster administrator, I must comply with a security policy that prohibits communication among worker nodes

To satisfy this use-case, the cluster administrator can specify a node selector
that includes only control-plane nodes, using the new
`spec.nodePlacement.nodeSelector` API field as follows:

```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  nodePlacement:
    nodeSelector:
      kubernetes.io/os: linux
      node-role.kubernetes.io/master: ""
```

#### As a cluster administrator, I want to allow DNS Pods to run on nodes that have a taint that has key "dns-only" and effect `NoSchedule`

To satisfy this use-case, the cluster administrator can specify a toleration for
the taint in question as follows:

```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  nodePlacement:
    tolerations:
    - effect: NoSchedule
      key: "dns-only"
      operator: Exists
```

### Implementation Details

Implementing this enhancement requires changes in the following repositories:

* openshift/api
* openshift/cluster-dns-operator

The DNS operator is modified to manage both the "dns-default" DaemonSet and the
"node-resolver" DaemonSet, both in the "openshift-dns" namespace.  The
"dns-node-resolver" container is removed from the "dns-default" DaemonSet if it
already exists, as are any tolerations and label selectors that are not
configured per the new API.  The operator is modified to apply the configured
tolerations and node label selectors to the "dns-default" DaemonSet.  The
"dns-node-resolver" is configured to tolerate all taints (as the "dns-default"
DaemonSet does in OpenShift 4.7 and earlier) and run on all Linux nodes.

### Risks and Mitigations

A cluster administrator could configure a node selector, or taint all nodes, in
such a way that DNS Pods could not be scheduled to any node, rendering the DNS
service unavailable.  Because the DNS service is critical to other cluster
components including OAuth, fixing misconfigured DNS Pod placement parameters
could be impossible for the cluster administrator to do.

As a mitigation for this risk, this enhancement adds logic in the DNS operator
to validate the desired node-placement parameters, before applying them, by
listing nodes and verifying that at least one matches the specified criteria.
If the desired node-placement parameters would prevent any DNS Pods from being
scheduled to any node, the operator does not apply the new parameters to the
DaemonSet.

This mitigation has the drawback that it cannot prevent DNS Pods from being
removed if the cluster administrator removes, relabels, or taints nodes where
DNS Pods are already scheduled.

## Design Details

### Test Plan

Unit tests are added to verify the functionality of the new API.

Additionally, an end-to-end test is added that configures a node selector to
select master nodes and verifies that the operator implements the change, then
configures a node selector to select no nodes and verifies that the operator
ignores the change.

### Graduation Criteria

N/A.

### Upgrade / Downgrade Strategy

On upgrade, the DNS operator removes the "dns-node-resolver" container from the
existing "dns-default" DaemonSet and creates a new "node-resolver" DaemonSet.

On downgrade, the DNS operator leaves the "node-resolver" DaemonSet running,
which results in each node's having two "dns-node-resolver" containers: one in
the "dns-default" DaemonSet and one in the "node-resolver" DaemonSet.  However,
both "dns-node-resolver" containers write the same content to `/etc/hosts`, and
they write the file atomically, so the redundant updates should not cause
conflicts.

### Version Skew Strategy

N/A.

## Implementation History

- 2018-10-05, in OCP 4.0, [openshift/cluster-dns-operator#34 update resources to
  avoid openshift cycles by
  deads2k](https://github.com/openshift/cluster-dns-operator/pull/34) added a
  blanket toleration for all taints.
- 2019-11-06, in OCP 4.3, [openshift/cluster-dns-operator#140 Bug 1753059: Don't
  start DNS on NotReady nodes by
  ironcladlou](https://github.com/openshift/cluster-dns-operator/pull/140)
  changed the blanket toleration to a toleration for a narrower set of taints in
  order to avoid scheduling the DNS Pod on nodes without networking.
- 2020-05-29, in OCP 4.5, [openshift/cluster-dns-operator#171 Bug 1813479:
  Tolerate all taints by
  Miciah](https://github.com/openshift/cluster-dns-operator/pull/171) reverted
  #140 and restored the blanket toleration.  This change was then backported to
  OCP 4.4 with
  [#179](https://github.com/openshift/cluster-dns-operator/pull/179) and to OCP
  4.3 with [#186](https://github.com/openshift/cluster-dns-operator/pull/186),
  with the result that DNS Pods tolerate all taints with the latest z-stream
  release of every OpenShift release up to and including OpenShift 4.7.
- In OCP 4.8, [openshift/cluster-dns-operator#209 Add node-resolver
  daemonset](https://github.com/openshift/cluster-dns-operator/pull/209) splits
  the DNS DaemonSet into two DaemonSets and implements the DNS Pod placement
  API.

## Alternatives

Approaches to configure the DNS service to prefer a node-local DNS Pod have been
investigated.  However, preferring a node-local endpoint would not prevent
inter-node traffic if no node-local endpoint were available (for example, during
a rolling upgrade of the DNS Pods) and would not address other use-cases where
a cluster administrator does not want DNS Pods running on certain nodes.

Configuring the container runtime to use the cluster DNS service has been
considered.  If the container runtime used the cluster DNS service, then no
entry for the cluster image registry would be needed in `/etc/hosts`, and the
"dns-node-resolver" container could be removed entirely.  However, avoiding a
bootstrap problem would be difficult with this approach: The container runtime
requires DNS to pull images, but the DNS operator and DNS Pods cannot start
until the container runtime has pulled their images.

As described in "Upgrade / Downgrade Strategy", downgrades from OpenShift 4.8 to
4.7 restore the "dns-node-resolver" container in the "dns-default" DaemonSet and
leave the "node-resolver" DaemonSet with its own "dns-node-resolver" container
running, which redundantly updates `/etc/hosts`.  We could backport a change to
OpenShift 4.7 to clean up any "node-resolver" DaemonSet that a downgrade may
have left.  Even if this enhancement is implemented as currently planned, this
option to backport logic to clean up any old DaemonSet remains available to us
should the need for it arise.

As described in "Risks and Mitigations", the operator validates any provided
placement parameters before applying them to the DNS DaemonSet.  However, an
administrator could configure valid placement parameters (meaning the parameters
allow some DNS Pods to be scheduled) and then remove, relabel, or taint nodes in
such a way that no DNS Pods would remain running.  We considered making the DNS
operator revert user-specified placement parameters to the default node selector
and a blanket toleration for all taints if it detected that no DNS Pod were
scheduled to any node.  However, this would be difficult to do in a safe and
transparent way.  We may consider it as a follow-on improvement.
