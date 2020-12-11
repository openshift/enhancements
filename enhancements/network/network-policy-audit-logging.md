---
title: audit-logging-of-network-policy-events
authors:
  - "@astoycos"
reviewers:
  - "@abhat" 
  - "@vpickard"
  - "@trozet" 
  - "@Billy99"
approvers:
  - "@knobunc"
creation-date: 2020-12-11
last-updated: 2020-12-11
status: implementable

---

# Audit Loggging of Network Policy Events

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The OVN-Kubernetes network type uses [OVN](https://www.ovn.org) to implement node overlay networks for Kubernetes. When OVN-Kubernetes is used as the network type for an Openshift cluster, OVN ACLs are used to implement Kubernetes' network policies (`NetworkPolicy` resources).  ACL's can either allow or deny traffic by matching on packets with specific rules. Built into the OVN ACL feature is
the ability to specify logging for each "allow" or "deny" rule.  This enhancement will activate the OVN ACL feature logging and allow the customer to manipulate the logging level, rate, and namespaces in which it is used, thereby showing valuable realtime information involving network policies.

## Motivation

Many customers require the ability to audit network policy related traffic events for regulatory and security policy compliance. Openshift currently does not have any features that satisfy this requirement. The ACL audit logging will allow customers to monitor NetworkPolicy events and identify patterns involving malicious activity with both allow and deny events. This is necessary in scenarios where
customers require a certain level of compliance, such as monitoring firewall activity, intrusion detection support, or to perform post-mortem analysis.

### Goals

- Activate configurable ACL allow/deny logging in OVN-Kubernetes on a per namespace basis.
- Allow the cluster administrator set the global logging via the Cluster Network Operator's configuration.
- Allow the cluster administrator set the logging level via a namespace's `.metadata.annotations` field.
- Collect the relevant data from the ovn-controller logs and present it to the cluster administrator.  

### Non-Goals

- Network Policy Object Logging, `oc describe <entity>` takes care of that.

## Proposal

To begin implementing ACL audit logging, first the [network.operator.openshift.io](https://github.com/openshift/api/blob/master/operator/v1/types_network.go) API needs to be updated.  These changes involve adding the optional `aclLoggingRateLimit` flag to the `OVNKubernetesConfig` struct as follows.

```go
// ovnKubernetesConfig contains the configuration parameters for networks
// using the ovn-kubernetes network project
type OVNKubernetesConfig struct {
 // mtu is the MTU to use for the tunnel interface. This must be 100
 // bytes smaller than the uplink mtu.
 // Default is 1400
 // +kubebuilder:validation:Minimum=0
 // +optional
 MTU *uint32 `json:"mtu,omitempty"`
 // geneve port is the UDP port to be used by geneve encapulation.
 // Default is 6081
 // +kubebuilder:validation:Minimum=1
 // +optional
 GenevePort *uint32 `json:"genevePort,omitempty"`
 // HybridOverlayConfig configures an additional overlay network for peers that are
 // not using OVN.
 // +optional
 HybridOverlayConfig *HybridOverlayConfig `json:"hybridOverlayConfig,omitempty"`
 // ipsecConfig enables and configures IPsec for pods on the pod network within the
 // cluster.
 // +optional
 IPsecConfig *IPsecConfig `json:"ipsecConfig,omitempty"`
 
 <--- BEGIN NEW CODE --->
 // aclLoggingRateLimit allows for the configuring of max acl logging rate
 // defaults to 20 messages per second
 // +optional
 aclLoggingRateLimit  *uint32 `json:"aclLoggingRateLimit,omitempty"`
 <--- END NEW CODE --->

}
```

The flag `aclLoggingRateLimit` defaults to `20` messages per second.

This will result in an updated OVNKubernetes Operator configuration object: `(.spec.defaultNetwork.ovnKubernetesConfig)` which will enable cluster-wide configuration of ACL logging at cluster installation time:

```go
spec:
  defaultNetwork:
    type: OVNKubernetes
    ovnKubernetesConfig:
      mtu: 1400
      genevePort: 6081
      ipsecConfig: {}
      aclLoggingRateLimit: 20
```

To enable the ACL logging on a per namespace basis using metadata annotations in the namespace's definition like the following

```go
kind: Namespace
apiVersion: v1
metadata:
  name: tenantA
  annotations:
    k8s.ovn.org/acl-logging: '{ "deny": "alert", "allow": "notice" }'
```

or

```go
kind: Namespace
apiVersion: v1
metadata:
  name: tenantB
  annotations:
    k8s.ovn.org/acl-logging: '{ "deny": "notice" }'
```

The logging can be activated for either allow, drop, or allow and drop actions. The severity must be one of `alert`, `warning`,
`notice`, `info`, or `debug` as described in [OVN documentation](http://www.openvswitch.org/support/dist-docs/ovn-nbctl.8.html).

each level is listed in order or descending severity

```go
alert    A major failure forced a process to abort.

warning  A high-level operation or a subsystem failed.  Attention
         is warranted.

notice   A low-level operation failed, but higher-level subsystems
         may be able to recover.

info     Information that may be useful in retrospect when
         investigating a problem.

debug    Information useful only to someone with intricate
         knowledge of the system, or that would commonly cause too-
         voluminous log output.  Log messages at this level are not
         logged by default.
```

### User Stories

#### Story 1

As an Openshift user I want to see what Network Policies and accompanied ACLs are dropping the most traffic in a cluster. For example, when a packet runs into a `drop` ACL it will log a message similar to the following:

 ```sh
  2021-01-05T17:34:02.675Z|00004|acl_log(ovn_pinctrl0)|INFO|name="<Network Policy Name>", verdict=drop, severity=info: icmp,vlan_tci=0x0000,dl_src=50:54:00:00:00:02,dl_dst=50:54:00:00:00:01,nw_src=192.168.0.3,nw_dst=192.168.0.2,nw_tos=0,nw_ecn=0,nw_ttl=64,icmp_type=8,icmp_code=0
 ```

#### Story 2

As an Openshift user I want to control the rate of ACL logging to ensure I can extract the useful information involving accept/reject actions in high hit rate scenarios.

#### Story 3

As an Openshift user I want to monitor in realtime traffic that is hitting any network policies I have implemented.

#### Story 4

As an Openshift user I want to be able to extract the ACL audit logs using a command similar to viewing the API audit logs

`oc adm node-logs --role=master --path=openshift-apiserver/`

### Implementation Details/Notes/Constraints

The following codebase changes will need to done before this feature becomes usable:  

- OVN Kubernetes

  - Move from a cluster wide namespace default deny port group to namespace specific ones.
  - Add logging flag and custom OVN meter to all created ACLs:

   ```sh
    fmt.Sprintf("log=%v", aclLogging != ""),
    fmt.Sprintf("severity=%s", getACLLoggingSeverity(aclLogging)), "meter=acl-logging",
   ```

  - Upstream work

- Cluster Network Operator
  
  - Digest the `aclLoggingRateLimit` Flag and start OVN-K with the configured value.

### Risks and Mitigations

- How do we control who is able to see the audit logs?
  - Only Admins should be able to see the audit logs since it could help expose holes in a cluster's network policy structure

- Adding meters, which can be used to limit the logging rate, to each ACL does add a slightly larger overhead in OVN, but based on early upstream testing with logging both enabled and disabled, there were negligible performance impacts detected.  

## Design Details

### Open Questions

1. How should the ACL logs be digested from the ovn-controller logs and presented to the user?

- Currently in OCP API audit logging is dumped directly onto the Nodes. should ACL logging do the same?
  - OCP's logging stack could be used to pull the logs and present them to the user with a Kiabana Dashboard, However the logging
          stack is an optional feature in an OCP cluster

2. Based on the solution to question 1.:

- Should a must-gather collect these logs, or should they be accessible only at runtime?

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

#### Examples

These are generalized examples to consider, in addition to the aforementioned [maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

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