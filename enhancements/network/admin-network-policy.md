---
title: admin-network-policy
authors:
  - "@tssurya"
reviewers:
  - "@jcaamano, OpenShift Networking"
  - "@astoycos, Office of the CTO"
  - "@npinaeva, OpenShift Networking"
approvers:
  - "@jcaamano"
api-approvers:
  - None
creation-date: 2023-05-31
last-updated: 2023-07-11
tracking-link:
  - https://issues.redhat.com/browse/SDN-2931
  - https://issues.redhat.com/browse/SDN-2932
see-also:
  - "https://github.com/kubernetes/enhancements/tree/master/keps/sig-network/2091-admin-network-policy"
replaces:
  - None
superseded-by:
  - None
---

# Add Support for Admin Network Policy in OVN-Kubernetes

## Summary

This enhancement outlines the design details to implement two new resources which are part
of the upstream [NetworkPolicy API](https://network-policy-api.sigs.k8s.io/) effort,
`AdminNetworkPolicy` and `BaselineAdminNetworkPolicy` in OVN-Kubernetes CNI plugin. They are
[currently in v1alpha1 version](https://github.com/kubernetes-sigs/network-policy-api/tree/master/apis/v1alpha1)
and maintained by Kubernetes `sig-network-policy-api` working group.

## Motivation

Today, OVN-Kubernetes supports  the core V1 NetworkPolicy resource which is being used
by all personas (administrators, operators, developers, application owners, tenant admins)
for enforcing network traffic regulations on their cluster workloads.
However Network Policies are namespace-scoped and were designed for developers
or namespace owners. They don't satisfy the requirements or use cases for cluster administrators.
Some of the main motivations for ANP and BANP are:

1. Cluster administrators don't have a way to define network policies on a
cluster-scoped level that is non-overridable by network policies.

2. Cluster administrators want network policies to be in place before the
namespaces are created.

3. Network Policies use an "implicit isolation" model that creates a default
deny ACL as soon as the policy is applied to the workload, thus automatically
isolating the workload. Any traffic that needs to be allowed, has to be called
out explicitly. For network administrators, they prefer an allowList and DenyList
that is explicit and an API that is similar to their traditonal firewall rules
that doesn't make any implicit assumptions.

### User Stories

* > "As an OpenShift cluster administrator I want to apply cluster-scoped policies
and guardrails for my entire cluster before namespaces are even created"

* > "As an OpenShift network administrator I want to enforce network traffic controls
that are non-overridable by users in the cluster in order to secure my cluster"

* > "As an OpenShift network administrator I want to enforce optional baseline network
traffic controls that are overridable by users in the cluster if need be"

Check out https://network-policy-api.sigs.k8s.io/user-stories/ where more fine grained
user stories for ANP and BANP are outlined.

### Goals

* Provide support for admins to use Admin Network Policy API in OVN-Kubernetes
* Provide support for admins to use Baseline Admin Network Policy API in OVN-Kubernetes

### Non-Goals

* v1alpha1 version of the API only supports east-west traffic scenarios for ANP and BANP. Egress
and ingress traffic policies at cluster-scoped level will be [iterated upon in the future
versions of the API](https://github.com/kubernetes-sigs/network-policy-api/pull/86)
* Two fields in the API will not be implemented in the first iteration because
they are subject to change in the future versions of the API. Hence we will hold off
until these parts are stable before implementing them downstream:
  * [`sameLabels`](https://github.com/kubernetes-sigs/network-policy-api/blob/7321d8509d9bf425c9c5b0338db3c0fb2816b4c5/apis/v1alpha1/shared_types.go#L134)
  * [`notSameLabels`](https://github.com/kubernetes-sigs/network-policy-api/blob/7321d8509d9bf425c9c5b0338db3c0fb2816b4c5/apis/v1alpha1/shared_types.go#L143)
* `NamedPorts` will not be implemented in the first iteration (will be supported in the future)
* ANP & BANP Policy logging (will be iterated upon in the future revisions)
* ANP & BANP Scale testing (will be iterated upon in the future revisions)
* Supporting ANP&BANP on Secondary Networks. Today Multi-Network Policies are
maintained by https://github.com/openshift/multus-networkpolicy and any future
support for cluster-scoped will have to come from those working groups to maintain parity.


Goal is to get a tech preview version of the current v1alpha1 API in OCP 4.14 which
will be improved upon in future releases based on scale testing and customer feedback.

## Proposal

A new level-driven controller will be added in OVN-Kubernetes which will
watch for the `AdminNetworkPolicy`, `BaselineAdminNetworkPolicy`, `Namespaces`
and `Pods` objects and create the:

* port groups for the subject pods selected in the policies - we will try to
investigate if shared port groups can be used here wherever possible
* address-sets for the peers defined in the policies - we will try and reuse
the address sets that are created for namespaces today and share them wherever possible
* acls for the ingress and egress policy rules that will connect these port-groups
and address-sets together into meaningful allow, deny or pass rules

for:

* ANP add/update/delete
* BANP add/update/delete
* pod add/update/delete (if it matches the policies; NOTE: updates are handled
only if pod labels, podIPs or podState (completed) changed)
* namespace add/update/delete (if it matches the policies; NOTE: updates are
handled only for label changes)

This controller will live in the `go-controller/pkg/ovn/controller` path
of OVNKubernetes repo under its own package `admin_network_policy`. Since these
are new APIs, we cannot reuse the [caches](https://github.com/ovn-org/ovn-kubernetes/blob/68295ec197b8eadd61ff872e293676fa1a68179d/go-controller/pkg/ovn/base_network_controller_policy.go#L123) that are present in network policy code.
The new controller will handle the implementation independent of the existing network
policy v1 code. A lot of performance fixes specially around locking have gone into
network policy v1 code and we don't want to destabilize those parts. Neither do we
want to add more things into the overloaded `oc` controller.

This controller hopes to be the first step towards breaking out network policy
construct management into its own set of standard controllers for the newer APIs.
Note that Network Policy V1 API is core and in-tree and potentially will never be
deprecated. Thus the existing code for network policy can be maintained as is in
the `oc` controller if need be. But new features are no longer added to that API.
So whenever Developer Network Policies come into play which is the future v2 of
network policy v1, we should be able to leverage the design of this new controller
and implement that easily. 

### Hierarchical ACLs and Pass Action

In order to implement the smooth co-existence of ANPs, NPs and BANPs in a cluster,
and ensure we provide clear precedence in the order of [ANP > NP > BANP](https://deploy-preview-54--kubernetes-sigs-network-policy-api.netlify.app/)
we will use a new RFE from OVN called [Hierarchical ACLs](https://bugzilla.redhat.com/show_bug.cgi?id=2134138).

This will let us define ACLs in stages or tiers. Currently we can support upto
4 tiers - 0, 1, 2 and 3. We will allot tier1 to ANP ACLs, tier2 to NP ACLs and
tier3 to BANP ACLs. Note that tier2 will be the default tier and other features
like egressFirewall will use that tier. ACLs in tier1 will be evaluated first
followed by ACLs in tier2 and finally ACLs in tier3. If a match is found in any
of the tiers, rest of the tiers will not be evaluated. Exception to this is if
a `Pass` action is set on an ACL. In that case we will skip all ACLs in that
specific tier and move to the next tier for processing.

Each tier supports upto 32000 priorities. Currently these are the priorities of
ACLs we plan to reserve for ANP & BANP:

* ANP: Tier1 - 30,000 - 20,000: In iteration1 we plan to support only upto max
100 ANPs (supported `.spec.priority` range: 0-99) in a cluster. We can expand
this later if needed. Since each ANP can have upto 100 egress or ingress rules
which hold precedence on account of how they are written in order, we will need
10,000 (100*100) priorities.
* BANP: Tier3 - 1750 - 1650: Since we can have only one default BANP in a cluster,
we need only 100 priorities for this.

### Workflow Description

The feature will be Tech Preview in 4.14 and will not be enabled by default
in the clusters. This will be behind a flag `enable-admin-network-policy` that
will be disabled by default in 4.14 using OCP Feature Gates. However anyone can
bring up a TechPreview OCP cluster and make this available there.
When we plan to GA the feature we will enable it via Cluster Network Operator.

**Cluster administrator or Network administrator** is responsible for
creating the ANPs or BANPs on a cluster. A sample yaml will look like
this for ANP (Note that BANP will be similar but it will not have `Pass`
action support or a priority field in its Spec since it is a singleton):

```yaml
apiVersion: policy.networking.k8s.io/v1alpha1
kind: AdminNetworkPolicy
metadata:
  name: priority-50-example
spec:
  priority: 50
  subject:
    namespaces:
      matchLabels:
          kubernetes.io/metadata.name: gryffindor
  ingress:
  - name: "deny-all-ingress-from-slytherin"
    action: "Deny"
    from:
    - pods:
        namespaces:
          namespaceSelector:
            matchLabels:
              conformance-house: slytherin
        podSelector:
          matchLabels:
            conformance-house: slytherin
  egress:
  - name: "pass-all-egress-to-slytherin"
    action: "Pass"
    to:
    - pods:
        namespaces:
          namespaceSelector:
            matchLabels:
              conformance-house: slytherin
        podSelector:
          matchLabels:
            conformance-house: slytherin
```

The OVN-K controller will convert these into NBDB logical entities (showing only egress, but same will be done for ingress):

```text
_uuid               : b29f0a44-30b2-4857-985f-a2e899ea72cd
acls                : [c02b4254-b1d0-48c1-a091-57ae6887bdc3, d4a487dd-3ac0-44d7-b07d-8d6eb0729567]
external_ids        : {name=priority-50-example_25000}
name                : a16480371363466183296
ports               : []
```
```text
_uuid               : 88a1905f-9733-479d-a23e-4e19aab9f95a
addresses           : []
external_ids        : {direction=ANPEgress, ip-family=v4, "k8s.ovn.org/id"="network-policy-controller:AdminNetworkPolicy:priority-50-example:ANPEgress:25000:v4", "k8s.ovn.org/name"=priority-50-example, "k8s.ovn.org/owner-controller"=network-policy-controller, "k8s.ovn.org/owner-type"=AdminNetworkPolicy, priority="25000"}
name                : a11410890234882820008
```
```text
_uuid               : d4a487dd-3ac0-44d7-b07d-8d6eb0729567
action              : pass
direction           : from-lport
external_ids        : {direction=ANPEgress, "k8s.ovn.org/id"="network-policy-controller:AdminNetworkPolicy:priority-50-example:ANPEgress:25000", "k8s.ovn.org/name"=priority-50-example, "k8s.ovn.org/owner-controller"=network-policy-controller, "k8s.ovn.org/owner-type"=AdminNetworkPolicy, priority="25000"}
label               : 0
log                 : false
match               : "((ip4.dst == $a11410890234882820008)) && (inport == @a16480371363466183296)"
meter               : acl-logging
name                : priority-50-example_ANPEgress_25000
options             : {apply-after-lb="true"}
priority            : 25000
severity            : debug
tier                : 1
```

#### Variation [optional]

N/A

### API Extensions

None

### Implementation Details/Notes/Constraints [optional]

Covered in the above sections

### Risks and Mitigations

N/A

### Drawbacks

N/A

## Design Details

### Open Questions [optional]

### Test Plan

* unit testing will be added for both ANP & BANP, pod label and namespace label changes in OVNK repo
* e2e conformance tests will be added for ANP & BANP into upstream `network-policy-api` repo
  * These e2e's will be brought down into OVN-Kubernetes CI and run as part of the presubmit jobs

### Graduation Criteria

#### Dev Preview -> Tech Preview

Tech Preview: 4.14

#### Tech Preview -> GA

GA: 4.16 (whenever we are sure the API has graduated to beta at least and hopefully GA)

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Since the feature is Tech Preview in 4.14 and uses alphav1 version
of the API, smooth upgrades when moving to betav1 version of the API is not
guaranteed and will not be tested. Once the API versioning is stablized in
upcoming versions, we will ensure upgrades are also stable.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

N/A

## Alternatives

We could have our own custom CRD for OVNK like other
plugins (Calico, Cilium, Antrea) have which might be faster but we
will loose the upstream first model. 

## Infrastructure Needed [optional]

N/A
