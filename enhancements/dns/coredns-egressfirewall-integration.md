---
title: coredns-egressfirewall-integration
authors:
  - '@arkadeepsen'
reviewers:
  - '@Miciah'
  - '@danwinship'
  - '@JoelSpeed'
  - '@TrilokGeer'
  - '@jerpeter1'
approvers:
  - '@Miciah'
  - '@danwinship'
api-approvers:
  - '@JoelSpeed'
creation-date: 2023-01-31
last-updated: 2023-01-31
tracking-link:
  - https://issues.redhat.com/browse/CFE-748
see-also:
  - N.A.
replaces:
  - N.A.
superseded-by:
  - N.A.
---

# Improve CoreDNS Integration with EgressFirewall

## Summary

This enhancement improves the integration of CoreDNS with EgressFirewall. With this improved
integration, EgressFirewall will be able to better support DNS names whose associated IP addresses change
and also will be able to provide support for wildcard DNS names.

NOTE: Doing DNS-based denies may have security issues. If a DNS rule denies access to only "example.com", then a proxy
can be set upon some host outside the cluster that will redirect connections to "example.com", and bypass the firewall
rule that way. The only reasonable way to use DNS-based firewall rules is to have a "deny all" rule and add DNS-based allow
exceptions on top of that. Considering this, henceforth only allow rule scenarios have been used in this enhancement
proposal.

## Motivation

Currently, EgressFirewall (OVN-K master) does a DNS lookup of a DNS name based on a default time-to-live (TTL) or the
TTL of the previous lookup (as explained [here](https://docs.openshift.com/container-platform/4.12/networking/ovn_kubernetes_network_provider/configuring-egress-firewall-ovn.html#domain-name-server-resolution_configuring-egress-firewall-ovn)).
OVN-K master then updates the underlying `AddressSet` for the DNS name
referenced by the corresponding ACL rule of the EgressFirewall rule containing the DNS name. However,
if a pod, belonging to the same namespace as that of the EgressFirewall, does a DNS lookup and gets a
different response than the OVN-K master has, then the pod will be incorrectly denied access to
the host. With the current implementation of EgressFirewall, to avoid such a scenario OVN-K master has to
spend way too much time doing DNS lookups to ensure any changes in the IP addresses are not missed or else
the firewall will possibly get out of sync. Thus, the integration between CoreDNS and EgressFirewall needs
to be improved to avoid such a scenario.

If an administrator wants to specifically allow access to all subdomains of some domain then
currently the administrator has to add all the subdomains in the EgressFirewall rules. This becomes
difficult when subdomains are added/removed as each one has to be added/removed individually
from the EgressFirewall rules. Currently, wildcard DNS names are not supported in EgressFirewall. However,
even if the support is added to EgressFirewall to accept wildcard DNS names, the integration between CoreDNS
and EgressFirewall needs to be improved for fully supporting it. A simple DNS lookup of a wildcard DNS name will
not be enough to get the IP addresses of all the subdomains of the wildcard DNS name as each subdomain may have a corresponding
`A` or `AAAA` record. Additionally, the DNS lookup of the wildcard DNS name may also fail to fetch any IP if no `A` or
`AAAA` record exists for the DNS name.

### User Stories

* As an OpenShift cluster administrator, I want to add regular DNS names to EgressFirewall rules, so that I can allow
access to them even if the IP addresses associated with the corresponding DNS records change. 
* As an OpenShift cluster administrator, I want to add wildcard DNS names to EgressFirewall rules, so that I can
allow access to all the subdomains belonging to the wildcard DNS names.

### Goals

* Support allowing access to DNS names even if the IP addresses associated with them changes
* Support usage of wildcard DNS names in EgressFirewall rules.

### Non-Goals

* Support additional DNS resolution functionality in the new CoreDNS plugin. The new plugin will only inspect the
response of DNS resolution by the other existing plugins.
* Support denying of DNS lookups in the new CoreDNS plugin. The new plugin will not stop the DNS lookup itself and will not
respond with a `REFUSED`/`NXDOMAIN` response code if a EgressFirewall rule denies access to the specific DNS name being queried for.

## Proposal

This enhancement proposes to introduce a new CoreDNS [external plugin](https://coredns.io/explugins/) (`egressfirewall`) and a new Custom Resource
(`EgressFirewallDNSName`) to improve the integration of CoreDNS with EgressFirewall. This proposal takes the OVN Interconnect (OVN-IC)
architecture into consideration where there are (possibly) multiple OVN-K masters and a centralized OVN-K cluster manager. The OVN-K cluster manager will
create a `EgressFirewallDNSName` CR for each unique DNS name (both regular and wildcard DNS names) used in
the EgressFirewall rules. This CR will be used to store the DNS name along with the current IP addresses, the corresponding
TTL, and the last lookup time. The `EgressFirewallDNSName` CR is meant for communication between
CoreDNS and OVN-K master(s).

The new plugin will inspect each DNS lookup and the corresponding response for the DNS lookup from other
plugins. If the DNS name in the query matches any `EgressFirewallDNSName` CR(s) (regular or wildcard or both), then the
plugin will update the `.status` of the matching `EgressFirewallDNSName` CR(s) with the DNS name along with the IP addresses,
the corresponding TTL, and the last lookup time. The OVN-K master(s) will watch the `EgressFirewallDNSName`
CRs. Whenever the IP addresses are updated for a `EgressFirewallDNSName` CR, the OVN-K master(s) will update the underlying `AddressSet`
referenced by the ACL rule(s) for the corresponding EgressFirewall rule(s).

A new controller (`EgressFirewallDNSName` controller) will keep track of the next lookup time (TTL + last lookup time)
for each regular DNS name and send a DNS lookup
query to CoreDNS when the minimum TTL expires. However, for a wildcard DNS name a DNS lookup cannot be only performed
on the DNS name as it will not return the IP addresses of all the subdomains. The DNS lookup of the wildcard DNS name may fail to return
any IP address as well. If the lookup for the wildcard DNS name fails, then it will retried using the default TTL (30 minutes). If the lookup
succeeds then the details will be added to the `.status` of the corresponding CR. Thus, the lookups will be performed on the DNS names which
are updated in the `.status` of the corresponding wildcard `EgressFirewallDNSName` CRs.

The following `EgressFirewallDNSName` CRD will be added to the `dns.openshift.io` api-group.

````go
// EgressFirewallDNSName describes a DNS name used in a EgressFirewall rule. It is TechPreviewNoUpgrade only.
type EgressFirewallDNSName struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the specification of the desired behavior of the EgressFirewallDNSName.
	Spec EgressFirewallDNSNameSpec `json:"spec,omitempty"`
	// status is the most recently observed status of the EgressFirewallDNSName.
	Status EgressFirewallDNSNameStatus `json:"status,omitempty"`
}

// EgressFirewallDNSNameSpec is a desired state description of EgressFirewallDNSName.
type EgressFirewallDNSNameSpec struct {
	// name is the DNS name used in a EgressFirewall rule.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=^(\*\.)?([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.$
	Name string `json:"name"`
}

// EgressFirewallDNSNameStatus defines the observed status of EgressFirewallDNSName.
type EgressFirewallDNSNameStatus struct {
	// The list of matching DNS names and their corresponding IP addresses along with TTL and last
	// DNS lookup time.
	ResolvedNames []EgressFirewallDNSNameStatusItem `json:"resolvedNames,omitempty"`
}

// EgressFirewallDNSNameStatusItem describes the details of a resolved DNS name.
type EgressFirewallDNSNameStatusItem struct {
	// The resolved DNS name corresponding to the Name field of EgressFirewallDNSNameSpec.
	// +kubebuilder:validation:Pattern=^(\*\.)?([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.$
	DNSName string `json:"dnsName"`
	// The IP addresses associated with the DNS name used in a EgressFirewall rule.
	IPs []string `json:"ips"`
	// Minimum time-to-live value among all the IP addresses.
	TTL int64 `json:"ttl"`
	// Timestamp when the last DNS lookup was successfully completed.
	LastLookupTime metav1.Time `json:"lastLookupTime"`
}
````

### Workflow Description

The workflows for Create, Delete and Update events for EgressFirewall related to DNS names are explained in this section. The workflow
for those events is shown in the following diagram with an example `EgressFirewallDNSName` CR:

![Workflow](./coredns-egressfirewall-integration.png)

#### Create/Update of DNS name

* An OpenShift cluster administrator creates/updates an EgressFirewall resource for a namespace and adds rule(s) containing
DNS name(s).
* The OVN-K cluster manager will create corresponding `EgressFirewallDNSName` CRs for each of the DNS names in the EgressFirewall rules, if not
already created. Each CR will be created in the `openshift-ovn-kubernetes` namespace. The name of the CR will be assigned using a hash
function (similar to the ComputeHash [here](https://github.com/openshift/kubernetes/blob/master/pkg/controller/controller_utils.go#L1157-L1172))
prefixed by `dns-`. The `.spec.name` field of the CR will be set to the DNS name (along with a trailing `.`).
* The `egressfirewall` CoreDNS plugin will watch for the events related to `EgressFirewallDNSName` CRs and will store the DNS name along
with the corresponding CR name.
* The `EgressFirewallDNSName` controller will watch for the events related to `EgressFirewallDNSName` CRs. When it will receive the Create events,
it will perform DNS lookup for each of the DNS names corresponding to the `EgressFirewallDNSName` CRs.
* The `egressfirewall` CoreDNS plugin will intercept the request and the response for the DNS lookups from all the pods. If the DNS name matches
any of the `EgressFirewallDNSName` CRs, then the plugin will update the corresponding `.status`. The details of the update steps are explained below.
* The OVN-K master(s) will watch for the events related to the `EgressFirewallDNSName` CRs. When the `.status` of a `EgressFirewallDNSName` CR is
updated, the OVN-K master(s) will update the `AddressSet` for the DNS name, which is linked with the ACL rule(s) for the corresponding EgressFirewall
rule(s).
* The `EgressFirewallDNSName` controller will store the regular DNS name matching the `.spec.name` field of a `EgressFirewallDNSName` CR and the
corresponding current IP addresses along with the TTL and the next time to lookup. Based on the next time to lookup, the controller will perform DNS
lookups to get the latest IP addresses and TTL information. The DNS lookup will be intercepted by the `egressfirewall` plugin and will enforce an
update of the `.status` of the CR.
* The controller will also store the wildcard DNS name which matches the `.spec.name` field of a `EgressFirewallDNSName` CR. If the DNS lookup is
successful for the wildcard DNS name, then the DNS name will be stored with the corresponding current IP addresses along with the TTL and the next time to
lookup. If the DNS lookup is not successful, then it will be retried  after a default TTL (30 minutes). The `EgressFirewallDNSName` controller will also store
the regular DNS names, matching the wildcard DNS name's `.status.resolvedNames[*].dnsName` field, with the corresponding current IP addresses along with the
TTL and the next time to lookup. Based on the next time to lookup, the controller will follow the same method as that of the regular DNS names to get the
latest IP addresses and TTL information.


#### Delete/Update of DNS name

* An OpenShift cluster administrator deletes an EgressFirewall resource for a namespace containing rule(s) for DNS name(s)
OR updates an EgressFirewall resource for a namespace and deletes rule(s) containing DNS name(s).
* The OVN-K master(s) will delete the ACL rule(s) corresponding to the EgressFirewall rule(s) containing the DNS name(s).
* The OVN-K master(s) will then check if the same DNS names are also used in the EgressFirewall rules in other namespaces. If
they are not used, then the OVN-K master(s) will delete the corresponding `AddressSet` for each of the DNS names in the EgressFirewall
rules. 
* Similar checks will be done by the OVN-K cluster manager. If DNS name is not used in any other namespaces then it will delete the
corresponding `EgressFirewallDNSName` CRs.
* On receiving the delete event, the `egressfirewall` plugin and the `EgressFirewallDNSName` controller will remove the details stored
regarding the `EgressFirewallDNSName` CRs.


#### Update steps of `.status` of the `EgressFirewallDNSName` CRs by the `egressfirewall` plugin

* The plugin gets the response of a DNS lookup from other plugins and checks the response code returned. It proceeds with further processing, only
if a success response code is returned, else it just sends the same response received from the other plugins.
* The plugin then checks whether the DNS lookup matches any `EgressFirewallDNSName` CR belonging to a regular DNS name or a wildcard DNS name
or both (in the case of a regular DNS name lookup). If no match is found then it just sends the same response received from the other plugins.
* If a regular DNS name in the lookup matches with a `EgressFirewallDNSName` CR corresponding to a regular DNS name, then the `.status` of
the `EgressFirewallDNSName` CR will be updated with the DNS name and the corresponding current IP addresses along with the TTL and the current time
as the last lookup time.
* If a regular DNS name in the lookup matches with a `EgressFirewallDNSName` CR corresponding to a wildcard DNS name, then the `.status` of
the `EgressFirewallDNSName` CR will be updated, if the IP addresses received in the response doesn't match with the IP addresses and the next lookup time
corresponding to the wildcard DNS name. Otherwise, the plugin will update the `.status` with the regular DNS name and the corresponding current IP addresses
along with the TTL and the next lookup time based on the TTL.
* If the DNS lookup is for a wildcard DNS name and it matches with the `EgressFirewallDNSName` CR corresponding to the wildcard DNS name,
then the `egressfirewall` plugin will update the `.status` of the corresponding `EgressFirewallDNSName` CR with the wildcard DNS name
and the corresponding current IP addresses along with the TTL and the current lookup time as the last lookup time. If the wildcard DNS name's
corresponding IP addresses and next lookup time (TTL + last lookup time) matches that of any other regular DNS name, added to the `.status`
of the `EgressFirewallDNSName` CR, then the details of the regular DNS name will be removed.
* However, the updates will take place if there is a change in the next lookup time (TTL + last lookup time) for the DNS name or if the next
lookup time is same for the DNS name, but the current IP addresses are different from the existing IP addresses. For the latter, the current
IP addresses are added to the existing IP addresses of the DNS name. This will take care of the scenario where different CoreDNS pods gets
different subsets of the IP addresses in the response to the DNS lookup of the same DNS name. The next time to lookup will be same for them
though the IP addresses may differ due to upstream DNS load balancing.
* Additionally, the exact matching of the next lookup time may never be successful. If the existing next lookup time of a DNS name lies
within a threshold value (5 seconds) of the current lookup time, specifically lies in the range defined by
`[current lookup time - threshold duration, current lookup time + threshold duration]`, then they are considered as same. 
* For DNS names whose TTL is returned as zero, a minimum TTL value (5 seconds) is used to avoid immediate DNS lookups by the `EgressFirewallDNSName`
controller for the same DNS names.
* The plugin then returns the same response received from the other plugins.

#### Variation [optional]


### API Extensions

The validation of [`DNSName`](https://github.com/ovn-org/ovn-kubernetes/blob/master/go-controller/pkg/crd/egressfirewall/v1/types.go#L74-L76) field in
`EgressFirewallDestination` will be updated to accept wildcard DNS names as well. It will be updated from `^([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.?$` which
accepts only regular DNS names to `^(\*\.)?([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.?$`.

````go
// EgressFirewallDestination is the endpoint that traffic is either allowed or denied to
type EgressFirewallDestination struct {
	// ..

	// dnsName is the domain name to allow/deny traffic to. If this is set, cidrSelector must be unset.
	// For a wildcard DNS name, the '*' will match only one label. Additionally, only a single '*' can be
	// used at the beginning of the wildcard DNS name. For example, '*.example.com' will match 'sub1.example.com'
	// but won't match 'sub2.sub1.example.com'
	// +kubebuilder:validation:Pattern=^(\*\.)?([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.?$
	DNSName string `json:"dnsName,omitempty"`
	// ..
}
````
The details of the `EgressFirewallDNSName` CRD can be found in the [Proposal](#proposal) section.


### Implementation Details/Notes/Constraints [optional]

The implementation changes needed for the proposed enhancement are documented in this section for each of the components.

#### Cluster DNS Operator

Cluster DNS Operator will deploy CoreDNS with the `egressfirewall` plugin enabled by adding it to the corefile. For the EgressFirewall rules to
apply consistently, even for DNS names that are resolved by custom upstreams, it will be added to all server blocks in the corefile. As the
plugin will watch and update the `EgressFirewallDNSName` CRs in the `dns.openshift.io` api-group, proper RBAC permissions will be needed
to be added to the `ClusterRole` for CoreDNS.

The new `EgressFirewallDNSName` controller will be added to the Cluster DNS Operator. The controller will watch the `EgressFirewallDNSName` CRs,
and will send DNS lookup requests for the `spec.name` field. It will also re-resolve the `status.resolvedNames[*].dnsName` fields based on the
corresponding next lookup time(TTL + last lookup time).

For wildcard DNS names, the controller will query for the DNS names that get added to the `.status` of the corresponding `EgressFirewallDNSName` CR,
including the wildcard DNS name, even if it doesn't get added to the `.status`. However, the list of the DNS names to lookup for a wildcard DNS name
should also not become stale if a DNS name belonging its subdomain is removed. To achieve this a retry counter will be used for the DNS name
lookups. If the lookup fails for a DNS name listed in the `.status` of a `EgressFirewallDNSName` CR for a threshold number of times (say 5),
then the DNS name will be removed from the `.status` by the controller. However, the DNS lookup will only fail if the corresponding wildcard
DNS name does not have an `A` or `AAAA` record. Otherwise, the response will be same as the IP addresses associated with the `A` or `AAAA` record
of the wildcard DNS name.

#### CoreDNS

The new external plugin `egressfirewall` will be added to a new github repository. The plugin will be enabled by adding its details in the `plugin.cfg` file
of the CoreDNS repository. As the plugin will inspect the DNS lookup queries and response from other plugins, it needs to be added before the other plugins
(namely `forward` plugin) which takes care of the DNS lookups for the DNS names external to the cluster.

The `egressfirewall` plugin will watch the `EgressFirewallDNSName` CRs and whenever there is a DNS lookup which matches one of the `EgressFirewallDNSName`
CRs (either regular or wildcard DNS names or both), then it will update the `.status` of the `EgressFirewallDNSName` CR(s) if there's any change
in the corresponding IP addresses and/or the next lookup time information (TTL + last lookup time). The process is explained in the [Workflow Description](#workflow-description) section.

`SharedIndexInformer` will be used for tracking events related to `EgressFirewallDNSName` CRs. The details about the regular and wildcard DNS names will be stored
in two separate maps by the plugin. Whenever there is a DNS lookup, if there is no `EgressFirewallDNSName` CRs created, then the `egressfirewall` plugin will just
send the received response to the lookup. A DNS name will be checked for a match in both the maps. If there is no match then also the plugin will just send the
received response to the lookup. However, when a match is found (in either of the maps or in both), then the corresponding `EgressFirewallDNSName` CR is updated
with the current IP addresses along with the corresponding TTL and current time as the last lookup time, if the same information is not already available.

#### OVN-K cluster manager

For every unique DNS name used in EgressFirewall rules, OVN-K cluster manager will create a corresponding `EgressFirewallDNSName` CR.
The name of the CR will be assigned using a hash function (similar to the ComputeHash
[here](https://github.com/openshift/kubernetes/blob/master/pkg/controller/controller_utils.go#L1157-L1172))
prefixed by `dns-`. It will also delete a `EgressFirewallDNSName` CR, when all the rules containing the corresponding DNS name are deleted.

#### OVN-K master(s)

The OVN-K master(s) will watch the `EgressFirewallDNSName` CRs. Whenever the `.status` of the CRs will be updated with new IP addresses and corresponding
TTL information for a DNS name, OVN-K master(s) will update the `AddressSet` mapped to the DNS name. This `AddressSet` will be linked
to the ACL rule(s) for the EgressFirewall rule(s) in which the DNS name is used. This will ensure that the latest IP addresses are always updated in
the `AddressSets`.


### Risks and Mitigations

* The `EgressFirewallDNSName` CR will be created by OVN-K Cluster manager whenever a new DNS name is used in a EgressFirewall rule. The CR will be deleted
when the corresponding DNS name is not used in any of the EgressFirewall rules. The `EgressFirewallDNSName` CR should not be modified (created or deleted or
updated) by an user. Doing so may lead to undesired behavior of EgressFirewall.

### Drawbacks

* Whenever there's a change in the IP addresses or the next lookup time (TTL + last lookup time) for a DNS name, the additional step of updating the
related `EgressFirewallDNSName` CRs will be executed. This will add some delay to the DNS lookup process. However, this will only
happen whenever there's a change in the DNS information.


## Design Details

### Open Questions [optional]

### Test Plan

* This enhancement will be tested through e2e tests by adding EgressFirewall rules containing regular DNS names
and wildcard DNS names. The tests will be added to the `openshift/origin` repository.
* Testing the feature where IP addresses are changed may be a little bit tricky as this will probably include
creation of DNS records and then changing the IP addresses of the DNS records through the e2e tests.

### Graduation Criteria

This feature will initially be released as Tech Preview only.

#### Dev Preview -> Tech Preview

N.A. This feature will go directly to Tech Preview.

#### Tech Preview -> GA (Future work)

* Incorporate the feedback received on the Tech Preview version.
* OpenShift documentation is needed to be updated.
* UTs and e2e tests need to cover all the edge case scenarios.

#### Removing a deprecated feature


### Upgrade / Downgrade Strategy

Upgrade expectations:
* On upgrade, the OVN-K cluster manager will create the corresponding `EgressFirewallDNSName` CRs for each DNS name in the
existing EgressFirewall resources. The `EgressFirewallDNSName` controller will start the DNS lookups for the `EgressFirewallDNSName` CRs
and the `egressfirewall` plugin will also start updating the `.status` fields of the `EgressFirewallDNSName` CRs. The scenarios arising
out of the order of the update of the various components are discussed in [Version Skew Strategy](#version-skew-strategy)

Downgrade expectations:
* On downgrade, the `EgressFirewallDNSName` CRs may still remain. However, these CRs would not have any impact on how
EgressFirewall ACL rules are implemented in the downgraded cluster. Deleting the CR Definition of `EgressFirewallDNSName`
from the cluster would remove all the `EgressFirewallDNSName` CRs.

### Version Skew Strategy

The following 2 scenarios may occur during the upgrade process:
* Scenario 1: The Cluster DNS operator and the CoreDNS pods are upgraded first and then the OVN-K cluster manager and master pods.

  In this scenario, the `egressfirewall` CoreDNS plugin will start inspecting each DNS lookup before the `EgressFirewallDNSName`
  CRs are created by the OVN-K cluster manager. The plugin will just respond with the response received from other plugins for
  the DNS lookups. As OVN-K master will be continuing the DNS lookups for DNS names with expired TTLs, CoreDNS will
  also be responding with the corresponding IP addresses and the TTLs, and the EgressFirewall functionality will still continue to work as before
  the start of the upgrade.

* Scenario 2: The OVN-K cluster manager and master pods are upgraded first and then the Cluster DNS operator and the CoreDNS pods.

  In this scenario, the OVN-K cluster manager will create `EgressFirewallDNSName` CRs for each unique DNS name used in EgressFirewall rules.
  However, as the Cluster DNS operator and the CoreDNS pods are still not upgraded, CoreDNS pods will not run the
  `egressfirewall` plugin. Thus, the EgressFirewall functionality will be broken in this scenario.

### Operational Aspects of API Extensions


#### Failure Modes


#### Support Procedures


## Implementation History


## Alternatives

The following solutions are alternatives for the proposed solution.

### [The existing system](https://docs.openshift.com/container-platform/4.12/networking/ovn_kubernetes_network_provider/configuring-egress-firewall-ovn.html#domain-name-server-resolution_configuring-egress-firewall-ovn)

In the existing system, OVN-K polls for each DNS name used in EgressFirewall rules based on the corresponding TTL. When there is a change in the
associated IP addresses then the `AddressSet` corresponding to each DNS name is updated.

#### Pros of the existing system

* Works well for regular DNS names with infrequent IP changes.

#### Cons of the existing system

* Wildcard DNS names are not supported.
* EgressFirewall rules with DNS names with frequent IP change may not be properly enforced.

### SOCKS or HTTP proxy

Users/customers can use a SOCKS or HTTP proxy for DNS requests. The proxy can be configured to allow or deny DNS names.

#### Pros of SOCKS or HTTP proxy

* Works well for DNS names with frequent IP changes.

#### Cons of SOCKS or HTTP proxy

* Not part of core OpenShift services.
* Less transparent for clients.


### Modify DNS response

The new CoreDNS plugin not only snoops on the DNS requests, but modifies the response based on the EgressFirewall rules. If a client requests for
a denied DNS name then the DNS response with `REFUSED` error code.

#### Pros of modifying DNS response

* Works well for DNS names with frequent IP changes.

#### Cons of modifying DNS response

* As mentioned previously, deny rules for DNS names have some problems.
* If only allow rules are supported, then all the DNS requests for names which are not mentioned in the allow rules should be sent a response with
`REFUSED` error code. This may not be obvious for the users.
* If client uses different DNS resolver then this will not work.


### [DNS Flow](https://github.com/freedge/dnsflow)

DNS Flow uses [dnstap](https://coredns.io/plugins/dnstap/) CoreDNS plugin to mirror all the DNS traffic to the `dnsflow` DaemonSet. Every 10 seconds, a
`dnsflow` pod lists the pod IP addresses for a namespace and maps the DNS name, used in the EgressFirewall allow rule for that namespace, to all the
pod IP addresses for that namespace. If the DNS name is used in the EgressFirewall allow rule for other namespaces, then all pod IP addresses of those namespaces
will also be mapped to the DNS name. When the DNS name in the DNS traffic matches the DNS name in an EgressFirewall allow rule then ovs allow rule
is added by the `dnsflow` pod by directly calling ovs-ofctl.

#### Pros of DNS Flow

* Works well for DNS names with frequent IP changes.

#### Cons of DNS Flow

* Additional delay is added due to receiving the DNS traffic through a socket connection on a separate pod.
* The pod IP addresses and EgressFirewall allow rules are checked every 10 seconds. Any changes in between will not be reflected in the ovs rules immediately.
* The existing ovs rules are not removed if an EgressFirewall allow rule is removed.

### [Cilium](https://docs.cilium.io/en/latest/security/policy/language/#dns-based)

A Cilium agent runs on each node and a DNS Proxy is provided in each agent. The proxy records the IP addresses related to Egress DNS policies and uses
them to enforce the DNS policies. The Cilium agent also [re-resolves](https://github.com/cilium/cilium/blob/HEAD/pkg/policy/api/egress.go#L137-L161)
the DNS names on a short interval of time (5 seconds) ignoring their TTL. The IP addresses are used in the underlying rules for the Egress policies. Only DNS
allow rules are supported by Cilium.


#### Pros of Cilium

* Works well for DNS names with frequent IP changes.
* Wildcard DNS names are supported. 

#### Cons of Cilium

* Additional delay added for sending the DNS traffic to the DNS proxy on the Cilium agent for recording the IP addresses related to Egress DNS policies.
* Other limitations are mentioned [here](https://github.com/cilium/cilium/blob/HEAD/pkg/policy/api/egress.go#L151-L158)


### gRPC connection between OVN-K and CoreDNS

Communication between OVN-K master and CoreDNS happens over a gRPC connection rather than the proposed `EgressFirewallDNSName` CR. Whenever there's a DNS
lookup for a DNS Name which is used in an EgressFirewall rule and the IP addresses associated with DNS name changes, then CoreDNS sends this information to the
OVN-K master. After the underlying ACL rules are updated the OVN-K master responds to the same CoreDNS pod with an OK message. Then the CoreDNS
pod responds to the original DNS lookup request.

#### Pros of gRPC connection between OVN-K and CoreDNS

* Works well for DNS names with frequent IP changes.
* Wildcard DNS names are supported. 

#### Cons of gRPC connection between OVN-K and CoreDNS

* The `EgressFirewallDNSName` CR works as a common knowledge base for the CoreDNS pods and OVN-K master. Without it, the CoreDNS pods and OVN-K have to
independently store the same information. Since a DNS lookup request is handled by one CoreDNS pod, the updated IP information will only be available to
that CoreDNS pod. Thus there should be a way for the CoreDNS pods to share this information amongst each other.
* If the CoreDNS pods does not store the IP information of the DNS names, then whenever there is a DNS lookup for a DNS name used in an EgressFirewall rule,
the IP information will always be needed to be sent to OVN-K master. This will add delay to all the DNS lookups of the DNS names used in EgressFirewall rules.

## Infrastructure Needed [optional]

