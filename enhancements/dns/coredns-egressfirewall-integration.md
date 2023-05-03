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
rule that way. The only reasonable way to use DNS-based firewall rules is to have a "deny all" and add DNS-based allow
exceptions on top of that. Considering this, henceforth only allow rule scenarios have been used in this enhancement
proposal.

## Motivation

Currently, EgressFirewall (OVN-K master) does a DNS lookup of a DNS name based on a default TTL or the
TTL of the previous lookup (as explained [here](https://docs.openshift.com/container-platform/4.12/networking/ovn_kubernetes_network_provider/configuring-egress-firewall-ovn.html#domain-name-server-resolution_configuring-egress-firewall-ovn)).
OVN-K master then updates the underlying ``AddressSet`` for the DNS name
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
even if the support is added to EgressFirewall to accepts wildcard DNS names, the integration between CoreDNS
and EgressFirewall needs to be improved for fully supporting it. A simple DNS lookup of a wildcard DNS name will
not be enough to get the IPs of all the subdomains of the wildcard DNS name as each subdomain will have a corresponding
`A` record. Additionally, the DNS lookup of the wildcard DNS name may also fail to fetch any IP if no `A` record exists
for the DNS name.


### User Stories

* As an OpenShift cluster administrator, I want to add regular DNS names to EgressFirewall rules, so that I can allow
access to them even if the IPs associated with the corresponding DNS records change. 
* As an OpenShift cluster administrator, I want to add wildcard DNS names to EgressFirewall rules, so that I can
allow access to all the subdomains belonging to the wildcard DNS names.


<!-- * As an OpenShift engineer, I want to add a new Custom Resource, so that IPs and TTLs of the DNS names which are used
in EgressFirewall rules can be tracked.
* As an OpenShift engineer, I want to add a new plugin to CoreDNS, so that DNS lookups of DNS names used in EgressFirewall
rules can be inspected and the current IPs and TTLs can be tracked in the corresponding new CR.
* As an OpenShift engineer, I want to modify Cluster DNS operator, so that CoreDNS can be deployed with the
new plugin enabled. -->

### Goals

<!-- * Support update of the underlying network rule corresponding to an EgressFirewall rule containing
a DNS name if the IP addresses associated with it change. -->

* Support allowing access to DNS names even if the IP addresses associated with them changes
* Support usage of wildcard DNS names in EgressFirewall rules.

<!-- * Create CRs for each unique DNS name used in the EgressFirewall rules and use the CRs to track the current
IPs and the corresponding TTL information. -->

### Non-Goals

* Support additional DNS resolution functionality in the new CoreDNS plugin. The new plugin will only inspect the
response of DNS resolution by the other existing plugins.
* Support denying of DNS lookups in the new CoreDNS plugin. The new plugin will not stop the DNS lookup itself and
respond with a `REFUSED` if a EgressFirewall rule denies access to the specific DNS name being queried for.

## Proposal

This enhancement proposes to introduce a new CoreDNS plugin (``egressfirewall``) and a new Custom Resource
(``EgressFirewallDNSName``) to improve the integration of CoreDNS with EgressFirewall. The OVN-K master will
create a ``EgressFirewallDNSName`` CR for each unique DNS name (both regular and wildcard DNS names) used in
the EgressFirewall rules. This CR will be used to store the DNS name along with the current IPs, the corresponding
TTL, and the last lookup time. The ``EgressFirewallDNSName`` CR is meant for communication between
CoreDNS and OVN-K master.

The new plugin will inspect each DNS lookup and the corresponding response for the DNS lookup from other
plugins. If the DNS name in the query matches any ``EgressFirewallDNSName`` CR(s) (regular or wildcard or both), then the
plugin will update the ``.status`` of the matching ``EgressFirewallDNSName`` CR(s) with the DNS name along with the IPs,
the corresponding TTL, and the last lookup time. The OVN-K master will watch the ``EgressFirewallDNSName``
CRs. Whenever the IPs are updated for a ``EgressFirewallDNSName`` CR, the OVN-K master will update the underlying ``AddressSet``
referenced by the ACL rule(s) for the corresponding EgressFirewall rule(s).

OVN-K master will keep track of the next lookup time (TTL + last lookup time) for each regular DNS name and send a DNS lookup
query to CoreDNS when the minimum TTL expires. However, for a wildcard DNS name a DNS lookup cannot be only performed
on the DNS name as it will not return the IPs of all the subdomains. The DNS lookup of the wildcard DNS name may fail to return
any IP as well. Thus, the lookups will be performed on the DNS names which are updated in the ``.status`` of the corresponding
wildcard ``EgressFirewallDNSName`` CRs.

The following ``EgressFirewallDNSName`` CRD will be added to the ``dns.openshift.io`` api-group.

````go
// EgressFirewallDNSName describes a DNS name used in a EgressFirewall rule.
type EgressFirewallDNSName struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of EgressFirewallDNSName.
	Spec EgressFirewallDNSNameSpec `json:"spec"`
	// Observed status of EgressFirewallDNSName
	// +optional
	Status EgressFirewallDNSNameStatus `json:"status,omitempty"`
}

// EgressFirewallDNSNameSpec is a desired state description of EgressFirewallDNSName.
type EgressFirewallDNSNameSpec struct {
	// Name is the DNS name used in a EgressFirewall rule.
	Name string `json:"name"`
}

// EgressFirewallDNSNameStatus defines the observed status of EgressFirewallDNSName.
type EgressFirewallDNSNameStatus struct {
	// The list of matching DNS names and their corresponding IPs along with TTL and last
	// DNS lookup time.
	ResolvedNames []EgressFirewallDNSNameStatusItem `json:"resolvedNames"`
}

// EgressFirewallDNSNameStatusItem describes the details of a resolved DNS name.
type EgressFirewallDNSNameStatusItem struct {
	// The resolved DNS name corresponding to the Name field of EgressFirewallDNSNameSpec.
	DNSName string `json:"dnsName"`
	// The list of corresponding associated IP addresses and the corresponding TTL and last
	// lookup time.
	Info []EgressFirewallDNSNameInfo `json:"info"`
	// conditions provide information about the state of the DNS name.
	//
	// These are the supported conditions:
	//
	//   * Available
	//   - True if the following conditions are met:
	//     * All the associated IP addresses are updated in the AddressSet, corresponding to the
	//       DNS name, used in the underlying ACL rules by OVN-K master.
	//   - False if any of those conditions are unsatisfied.
	Conditions []OperatorCondition `json:"conditions,omitempty"`
}

// EgressFirewallDNSNameInfo gives the details of an IP address associated with a DNS name with the
// corresponding TTL and the last DNS lookup time.
type EgressFirewallDNSNameInfo struct {
	// The IP address associated with a DNS name used in a EgressFirewall rule.
	IP string `json:"ip"`
	// Time-to-live value of the IP address.
	TTL int64 `json:"ttl"`
	// Timestamp when the last DNS lookup was successfully completed.
	LastLookupTime metav1.Time `json:"lastLookupTime"`
}
````

### Workflow Description

The workflows for Create, Delete and Update events for EgressFirewall related to DNS names are explained in this section.

#### Create/Update of Regular DNS name

* An OpenShift cluster administrator creates/updates an EgressFirewall resource for a namespace and adds rule(s) containing regular
DNS name(s).
* The OVN-K master will create corresponding `EgressFirewallDNSName` CRs for each of the DNS names in the EgressFirewall rules, if not
already created. Each CR will be created in the ``openshift-ovn-kubernetes`` namespace. The name of the CR will be assigned using a hash
function (similar to the ComputeHash [here](https://github.com/openshift/kubernetes/blob/master/pkg/controller/controller_utils.go#L1157-L1172))
prefixed by `dns-`. The `.spec.name` field of the CR will be set to the DNS name (along with a trailing `.`).
* The ``egressfirewall`` CoreDNS plugin will watch for the events related to `EgressFirewallDNSName` CRs and will store the DNS name along
with the corresponding CR name.
* The OVN-K master will then perform DNS lookup for each of the regular DNS names added to the EgressFirewall rules.
* The ``egressfirewall`` CoreDNS plugin will intercept the request and the response for the DNS lookup for each of the
regular DNS names.
* As these DNS names have corresponding ``EgressFirewallDNSName`` CRs, the ``egressfirewall`` plugin will update the ``.status`` of
the `EgressFirewallDNSName` CRs with the DNS name and the corresponding current IPs along with the TTL and the current time as the last lookup time.
However, this update will only take place if there is a change in the existing IP addresses or next time to lookup (TTL + last lookup time) or both
for the DNS name. If the update is applied to the `.status` of the `EgressFirewallDNSName` CRs, then `Available` condition will be set to `False` for
the corresponding `conditions` field by the plugin.
* The OVN-K master will watch the ``EgressFirewallDNSName`` CRs. When the ``.status`` of a ``EgressFirewallDNSName`` CR is updated, the OVN-K master
will update the ``AddressSet`` for the DNS name, which is linked with the ACL rule(s) for the corresponding EgressFirewall
rule(s). Once the update is applied to the `AddressSet`, the OVN-K master will set the `Available` condition to `True` for
the corresponding `conditions` field.
* The `egressfirewall` plugin will wait until the `Available` condition becomes `True` for the `EgressFirewallDNSName` CR. Once it is set
to `True`, the plugin will send the response to the DNS lookup query.
* The OVN-K master will receive the response of the DNS lookup query for the DNS name. The OVN-K master will check
the corresponding ``EgressFirewallDNSName`` CR's ``.status`` and if the next lookup time (TTL + last lookup time) in the status is greater than the next lookup
time based on the received TTL, then the corresponding CR's ``.status`` will be updated. The corresponding ``AddressSet``
will also be updated.
* The OVN-K master will store the regular DNS name and the corresponding current IPs along with the TTL and the next time
to lookup. Based on the next time to lookup, the OVN-K master will perform DNS lookups to get the latest IPs and TTL information.

#### Create/Update of Wildcard DNS name

* An OpenShift cluster administrator creates/updates an EgressFirewall resource for a namespace and adds rule(s) containing wildcard
DNS name(s).
* The OVN-K master will create corresponding `EgressFirewallDNSName` CRs for each of the wildcard DNS names in the EgressFirewall rules, if
not already created. Each CR will be created in the ``openshift-ovn-kubernetes`` namespace. The name of the CR will be assigned using a hash
function (similar to the ComputeHash [here](https://github.com/openshift/kubernetes/blob/master/pkg/controller/controller_utils.go#L1157-L1172))
prefixed by `dns-`. The `.spec.name` field of the CR will be set to the DNS name (along with a trailing `.`).
* The ``egressfirewall`` CoreDNS plugin will intercept the request and the response for the DNS lookup for each of the
regular DNS names.
* The OVN-K master will then perform DNS lookup for each of the wildcard DNS names added to the EgressFirewall rules.
* The ``egressfirewall`` CoreDNS plugin will intercept the request and the response for the DNS lookups from all the pods. If
the DNS lookup is for a wildcard DNS name and it matches with one of the wildcard DNS names used in a EgressFirewall rule, and the
lookup fails, then the plugin will not update the `.status` of the corresponding `EgressFirewallDNSName` CR. If the DNS lookup succeeds
then the ``egressfirewall`` plugin will update the ``.status`` of the corresponding `EgressFirewallDNSName` CR with the wildcard DNS name
and the corresponding current IPs along with the TTL and the current lookup time as the last lookup time. If the wildcard DNS name's
corresponding IP addresses and next lookup time (TTL + last lookup time) matches that of any other regular DNS name, added to the `.status`
of the `EgressFirewallDNSName` CR, then the details of the regular DNS name will be removed.
* If a regular DNS name in the lookup matches with a wildcard DNS name, then the ``egressfirewall`` plugin will update the
``.status`` of the corresponding `EgressFirewallDNSName` CR, if the IPs received in the response doesn't match with the IPs and the next lookup time
corresponding to the wildcard DNS name. Otherwise, the plugin will update the `.status` with the regular DNS name and the corresponding current IPs
along with the TTL and the next lookup time based on the TTL. 
* The updates will only take place if there is a change in the existing IP addresses or next time to lookup (TTL + last lookup time) or both
for the corresponding DNS names. If the update is applied to the `.status` of the `EgressFirewallDNSName` CRs, then `Available` condition will be set to `False` for
the corresponding `conditions` field by the plugin.
* The OVN-K master will watch the ``EgressFirewallDNSName`` CRs. When the ``.status`` of a ``EgressFirewallDNSName`` CR is updated, the OVN-K master
will update the ``AddressSet`` for the wildcard DNS name, which is linked with the ACL rule for the corresponding EgressFirewall
rule(s). Once the update is applied to the `AddressSet`, the OVN-K master will set the `Available` condition to `True` for
the corresponding `conditions` field.
* The OVN-K master will store the wildcard DNS name only if the DNS lookup succeeds. The DNS name will be stored with the corresponding current IPs
along with the TTL and the next time to lookup. The OVN-K master will also store the regular DNS names, matching the wildcard DNS name, with the corresponding
current IPs along with the TTL and the next time to lookup. Based on the next time to lookup, the OVN-K master will follow the same method as that of the
regular DNS names to get the latest IPs and TTL information.

#### Delete/Update of Regular DNS name

* An OpenShift cluster administrator deletes an EgressFirewall resource for a namespace containing rule(s) for regular DNS
name(s) OR updates an EgressFirewall resource for a namespace and deletes rule(s) containing regular DNS name(s).
* The OVN-K master will delete the ACL rule(s) corresponding to the EgressFirewall rule(s) containing the regular DNS name(s).
* The OVN-K master will then check if the same regular DNS names are also used in the EgressFirewall rules in other namespaces. If
they are not used, then the OVN-K master will delete the corresponding ``AddressSet`` for each of the DNS names in the EgressFirewall
rules. The OVN-K master will also delete the corresponding ``EgressFirewallDNSName`` CRs.
* On receiving the delete event, the `egressfirewall` plugin will remove the details stored regarding the ``EgressFirewallDNSName`` CRs.


#### Delete/Update of Wildcard DNS name

* An OpenShift cluster administrator deletes an EgressFirewall resource for a namespace containing rule(s) for wildcard DNS name(s)
OR updates an EgressFirewall resource for a namespace and deletes rule(s) containing wildcard DNS name(s).
* The OVN-K master will delete the ACL rule(s) corresponding to the EgressFirewall rule(s) containing the wildcard DNS name(s).
* The OVN-K master will then check if the same wildcard DNS names are also used in the EgressFirewall rules in other namespaces. If
they are not used, then the OVN-K master will delete the corresponding ``AddressSet`` for each of the DNS names in the EgressFirewall
rules. The OVN-K master will also delete the corresponding ``EgressFirewallDNSName`` CRs.
* On receiving the delete event, the `egressfirewall` plugin will remove the details stored regarding the ``EgressFirewallDNSName`` CRs.


#### Variation [optional]


### API Extensions

The validation of ``DNSName`` field in ``EgressFirewallDestination`` will be updated to accept wildcard DNS names as well.
It will be updated from ``^([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.?$`` which accepts only regular DNS names to
``^(\*\.)?([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.?$``.

````go
// EgressFirewallDestination is the endpoint that traffic is either allowed or denied to
type EgressFirewallDestination struct {
	// ..

	// dnsName is the domain name to allow/deny traffic to. If this is set, cidrSelector must be unset.
	// For a wildcard DNS name, the * will match only one label. additionally, only a single * can be
	// used at the beginning of the wildcard DNS name. For example, "*.example.com" will match "sub1.example.com"
	// but won't match "sub2.sub1.example.com"
	// +kubebuilder:validation:Pattern=^(\*\.)?([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.?$
	DNSName string `json:"dnsName,omitempty"`
}
````
The details of the ``EgressFirewallDNSName`` CRD can be found in the [Proposal](#proposal) section.


### Implementation Details/Notes/Constraints [optional]

The implementation changes needed for the proposed enhancement are documented in this section for each of the components.

#### Cluster DNS Operator

Cluster DNS Operator will deploy CoreDNS with the ``egressfirewall`` plugin enabled by adding it to the corefile. For the EgressFirewall rules to
apply consistently, even for DNS names that are resolved by custom upstreams, it will be added to all server blocks in the corefile. As the
plugin will watch and update the ``EgressFirewallDNSName`` CRs in the ``dns.openshift.io`` api-group, proper RBAC permissions will be needed
to be added to the ``ClusterRole`` for CoreDNS.

#### CoreDNS

The new plugin ``egressfirewall`` will be added to CoreDNS. As the plugin will inspect the DNS lookup queries and response from
other plugins, it needs to be added before the other plugins (namely ``forward`` plugin) in the ``plugin.cfg`` file.

The ``egressfirewall`` plugin will watch the ``EgressFirewallDNSName`` CRs and whenever there is a DNS lookup which matches one of the ``EgressFirewallDNSName``
CRs (either regular or wildcard DNS names or both), then it will update the ``.status`` of the ``EgressFirewallDNSName`` CR(s) if there's any change
in the corresponding IPs and/or the next lookup time information (TTL + last lookup time). The process is explained in the [Workflow Description](#workflow-description) section.

The details about the regular and wildcard DNS names will be stored in two separate maps by the plugin. Whenever there is a DNS lookup, if there is
no ``EgressFirewallDNSName`` CRs created, then the `egressfirewall` plugin will just send the received response to the lookup. A DNS name will be
checked for a match in both the maps. If there is no match then also the plugin will just send the received response to the lookup. However, when a
match is found (in either of the maps or in both), then the corresponding ``EgressFirewallDNSName`` CR is updated with the current IPs along with the
corresponding TTL and current time as the last lookup time, if the same information is not already available. If the `.status` of a ``EgressFirewallDNSName``
CR is updated, then the plugin will wait until the `Available` condition of the CR becomes true. Once the condition becomes true only then the response
to the DNS lookup will be sent.

#### OVN-K master

For every unique DNS name used in EgressFirewall rules, OVN-K master will create a corresponding ``EgressFirewallDNSName`` CR. The name of the CR will be assigned
using a hash function (similar to the ComputeHash [here](https://github.com/openshift/kubernetes/blob/master/pkg/controller/controller_utils.go#L1157-L1172))
prefixed by `dns-`.

The OVN-K master will also watch the ``EgressFirewallDNSName`` CRs. Whenever the ``.status`` of the CRs will be updated with new IPs and corresponding
TTL information for a DNS name, OVN-K master will update the ``AddressSet`` mapped to the DNS name. This ``AddressSet`` will be linked
to the ACL rule(s) for the EgressFirewall rule(s) in which the DNS name is used. This will ensure that the latest IPs are always updated in
the ``AddressSets``.

For wildcard DNS names, the OVN-K master will only query for the DNS names that get added to the ``.status`` of the corresponding ``EgressFirewallDNSName`` CR,
including the wildcard DNS name if it get added to the `.status`. However, the list of the DNS names to lookup for a wildcard DNS name should also
not become stale if a DNS name belonging its subdomain is removed. To achieve this a retry counter will be used for the DNS name
lookups. If the lookup fails for a DNS name listed in the ``.status`` of a ``EgressFirewallDNSName`` CR for threshold number of times (say 5),
then the DNS name will be removed from the ``.status``. However the DNS lookup will fail, if the corresponding wildcard DNS name does not have an A
record. Otherwise, the response will be same as the IP addresses associated with the A record of the wildcard DNS name.

### Risks and Mitigations

* The ``EgressFirewallDNSName`` CR will be created by OVN-K master whenever a new DNS name is used in a EgressFirewall rule. The CR will be deleted
when the corresponding DNS name is not used in any of the EgressFirewall rules. The ``EgressFirewallDNSName`` CR should not be modified (created or deleted or
updated) by an user. Doing so may lead to undesired behavior of EgressFirewall.

### Drawbacks

* Whenever there's a change in the IPs or the next lookup time (TTL + last lookup time) for a DNS name, the additional step of updating the
related ``EgressFirewallDNSName`` CRs will be executed. The `egressfirewall` plugin will wait until the IPs are updated in the corresponding
`AddressSet` and then send the response to the DNS lookup. This will add some delay to the DNS lookup process. However, this will only
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
* On upgrade, the OVN-K master will create the corresponding ``EgressFirewallDNSName`` CRs for each DNS name in the
existing EgressFirewall resources. The ``egressfirewall`` plugin will also start updating the ``.status``
fields of the ``EgressFirewallDNSName`` CRs. The scenarios arising out of the order of the update of the various components
are discussed in [Version Skew Strategy](#version-skew-strategy)

Downgrade expectations:
* On downgrade, the ``EgressFirewallDNSName`` CRs may still remain. However, these CRs would not have any impact on how
EgressFirewall ACL rules are implemented in the downgraded cluster. Deleting the CR Definition of ``EgressFirewallDNSName``
from the cluster would remove all the ``EgressFirewallDNSName`` CRs.

### Version Skew Strategy

The following 2 scenarios may occur during the upgrade process:
* Scenario 1: The Cluster DNS operator and the CoreDNS pods are upgraded first and then the OVN-K master pods.

  In this scenario, the ``egressfirewall`` CoreDNS plugin will start inspecting each DNS lookup before the ``EgressFirewallDNSName``
  CRs are created by the OVN-K master. The plugin will just respond with the response received from other plugins for
  the DNS lookups. As OVN-K master will be continuing the DNS lookups for DNS names with expired TTLs, CoreDNS will
  also be responding with the corresponding IPs and the TTLs.

* Scenario 2: The OVN-K master pods are upgraded first and then the Cluster DNS operator and the CoreDNS pods.

  In this scenario, the OVN-K master will create ``EgressFirewallDNSName`` CRs for each unique DNS name used in EgressFirewall rules.
  However, as the Cluster DNS operator and the CoreDNS pods are still not upgraded, CoreDNS pods will not run the
  ``egressfirewall`` plugin. The OVN-K master will still receive the response for the DNS lookup queries it will send
  to the CoreDNS.

### Operational Aspects of API Extensions


#### Failure Modes


#### Support Procedures


## Implementation History


## Alternatives


## Infrastructure Needed [optional]

