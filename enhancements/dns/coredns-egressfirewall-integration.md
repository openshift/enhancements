---
title: coredns-egressfirewall-integration
authors:
  - '@arkadeepsen'
reviewers:
  - '@Miciah'
  - '@danwinship'
  - '@joelspeed'
approvers:
  - '@Miciah'
  - '@danwinship'
api-approvers:
  - '@joelspeed'
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
integration, EgressFirewall will be able to better support DNS names whose IPs change dynamically
and also will be able to provide support for wildcard DNS names.

## Motivation

Currently, EgressFirewall (OVN-K master) does a DNS lookup of a DNS name based on the TTL of the
previous lookup. OVN-K master then updates the underlying ACL rules. However, if a pod belonging to
the same Namespace as that of the EgressFirewall does a DNS lookup and is able to get the response before
the OVN-K master then the pod will be incorrectly allowed/denied access to the DNS name. The integration
between CoreDNS and EgressFirewall needs to be improved to avoid such a scenario.

If an administrator wants to specifically allow/deny access to all subdomains then currently the
administrator has to add all subdomains in the EgressFirewall rules. This becomes difficult when
subdomains are dynamically added/removed as each one has to be added/removed individually from the
EgressFirewall rules. Currently, wildcard DNS names are not supported in EgressFirewall. However,
even if the support is added to EgressFirewall, the integration between CoreDNS and EgressFirewall
needs to be improved as wildcard DNS names cannot be directly looked up through a query.


### User Stories

* As an OpenShift cluster administrator, I want to add DNS Names to EgressFirewall rules, so that I can allow/deny
access to them even if the IPs associated with the corresponding DNS records change dynamically.
* As an OpenShift cluster administrator, I want to add wildcard DNS Names to EgressFirewall rules, so that I can
allow/deny access to all the subdomains belonging to the wildcard DNS names.
* As an OpenShift engineer, I want to add a new Custom Resource, so that IPs and TTLs of DNS names which are used
in EgressFirewall rules can be tracked.
* As an OpenShift engineer, I want to add a new plugin to CoreDNS, so that DNS lookups of DNS names used in EgressFirewall
rules can be inspected and the current IPs and TTLs can be tracked in the corresponding new CR.
* As an OpenShift engineer, I want to modify Cluster DNS operator, so that CoreDNS can be deployed with the
new plugin enabled.

### Goals

* Support update of EgressFirewall ACL rules if IPs associated with the corresponding DNS names change
dynamically.
* Support usage of wildcard DNS name in EgressFirewall rules.
* Create CRs for each unique DNS name used in the EgressFirewall rules and use the CRs to track the current
IPs and the corresponding TTL information.

### Non-Goals

* Support additional DNS resolution functionality in the new CoreDNS plugin.
* Support allowing/denying of DNS lookups (core EgressFirewall functionality) in the new CoreDNS plugin.

## Proposal

This enhancement proposes to introduce a new CoreDNS plugin (``egressfirewall``) and a new Custom Resource
(``DNSName``) to improve the integration of CoreDNS with EgressFirewall. The ``DNSName`` CR will be created
for each unique DNS name (both regular and wildcard DNS names) used in the EgressFirewall rules. This CR will
be used to store the DNS name along with the current IPs and the correspodning TTL and the next lookup time
based on the TTL. The OVN-K master will be responsible for the creation of the ``DNSName`` CRs. The ``DNSName``
CR is meant for communication between CoreDNS and OVN-K master. Thus it cannot be created or modified by an
OpenShift cluster adminisrator.

The new plugin will inspect each DNS lookup and the corresponding response for the DNS lookup from other
plugins. If the DNS name in the query matches any ``DNSName`` CR(s) (regular or wildcard or both), then the
plugin will update the ``.status`` of the matching ``DNSName`` CR(s) with the DNS name along with the IPs and
the corresponding TTL and the next lookup time based on the TTL. The OVN-K master will watch the ``DNSName``
CRs. Whenever the IPs are updated for a ``DNSName`` CR, the OVN-K master will update the the underlying ACL
rules for the corresponding EgressFirewall(s).

OVN-K master will keep track of the TTL (or next lookup time) for each regular DNS name and send a DNS lookup
query to CoreDNS when the minimum TTL expires. However, for a wildcard DNS name a DNS lookup cannot be performed
directly on the DNS name as it will not return any IP. Thus, the lookups will be performed on the DNS names
which are updated in the ``.status`` of the corresponding wildcard ``DNSName`` CR.

### Workflow Description

The workflows for DNS name addition and deletion are explained in this section.

#### Addition

##### Regular DNS name
* An OpenShift cluster administrator creates an EgressFirewall resource for a Namespace and adds rule(s) containing
regular DNS name(s).
* The OVN-K master will create corrresponding `DNSName` CRs for each of the DNS names in the EgressFirewall rules, if not
already created. Each CR will be created in the ``openshift-dns`` Namespace and the Name of the CR will be same as the DNS
name (barring any trailinng `.`). The ``.spec.isregular`` field will be set to true, even if it already exists.
* The OVN-K master will then perform DNS lookup for each of the regular DNS names added to the EgressFirewall rules.
* The ``egressfirewall`` CoreDNS plugin will intercept the request and the response for the DNS lookup for each of the
regular DNS names.
* As these DNS names have corresponding ``DNSName`` CRs, the ``egressfirewall`` plugin will update the ``.status`` of
the `DNSName` CRs with the DNS name and the corresponding current IPs along with the TTL and the next lookup time based
on the TTL. However, this update will only take place if there is a change in the exisiting IP
addresses or TTL or both for the DNS name.
* The OVN-K master will watch the ``DNSName`` CRs. When the ``.status`` of a ``DNSName`` CR is updated, the OVN-K master
will update the ``AddressSet`` for the DNS name, which is linked with the ACL rule for the corresponding EgressFirewall(s).
* The OVN-K master will also receive the response of the DNS lookup query for the DNS name. The OVN-K master will check
the corresponding ``DNSName`` CR's ``.status`` and if the next lookup time in the status is greater than the next lookup
time based on the received TTL, then the corresponding CR's ``.status`` will be updated. The corresponding ``AddressSet``
will also be updated.

##### Wildcard DNS name
* An OpenShift cluster administrator creates an EgressFirewall resource for a Namespace and adds rule(s) containing
wildcard DNS name(s).
* The OVN-K master will create corrresponding `DNSName` CRs for each of the wildcard DNS names in the EgressFirewall rules, if not
already created. Each CR will be created in the ``openshift-dns`` Namespace. The Name of the CR will be set to the wildcard
DNS name after replacing the ``*`` with ``wildcard`` (barring any trailinng `.`). For example, if the wildcard DNS name is
``*.example.com``, then the Name of the corresponding CR will be ``wildcard.example.com``. This is done to adhere to the
[Kubernetes object naming validations](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names).
The ``.spec.iswildcard`` field will be set to true, even if it already exists.
* The OVN-K master will not perform any DNS lookup for the wildcard DNS names directly.
* The ``egressfirewall`` CoreDNS plugin will intercept the request and the response for the DNS lookups from all the pods. If
the regular DNS name in the lookup matches with a wildcard DNS name, then the ``egressfirewall`` plugin will update the
``.status`` of the corresponding `DNSName` CR with the regular DNS name and the corresponding current IPs along with the TTL
and the next lookup time based on the TTL. However, this update will only take place if there is a change in the exisiting IP
addresses or the next lookup time or both for the DNS name.
* The OVN-K master will watch the ``DNSName`` CRs. When the ``.status`` of a ``DNSName`` CR is updated, the OVN-K master
will update the ``AddressSet`` for the wildcard DNS name, which is linked with the ACL rule for the corresponding EgressFirewall(s).
* The OVN-K master will also store the regular DNS name and the corresponding current IPs along with the TTL and the next time
to lookup. Based on the next time to lookup, the OVN-K master will follow same method as that of regular DNS names to get the
latest IPs and TTL information.

#### Deletion

##### Regular DNS name
* An OpenShift cluster administrator deletes an EgressFirewall resource for a Namespace containing rul(e) for regular DNS name(s).
* The OVN-K master will delete the ACL rules corresponding to the EgressFirewall.
* The OVN-K master will then check if the same regular DNS names are also used in the EgressFirewall rules in other Namespaces. If
they are not used, then the OVN-K master will delete the corrresponding ``AdressSets`` for each of the DNS names in the EgressFirewall
rules. The OVN-K master will also delete the corresponding ``DNSName`` CRs, only if the ``.spec.isregular`` field is set to true and
the ``.spec.iswildcard`` field is set to false. If both the fields are set to true, then the CR will not be deleted and the
``.spec.isregular`` field will be set to false.


##### Wildcard DNS name
* An OpenShift cluster administrator deletes an EgressFirewall resource for a Namespace containing rul(e) for wildcard DNS name(s).
* The OVN-K master will delete the ACL rules corresponding to the EgressFirewall.
* The OVN-K master will then check if the same wildcard DNS names are also used in the EgressFirewall rules in other Namespaces. If
they are not used, then the OVN-K master will delete the corrresponding ``AdressSets`` for each of the DNS names in the EgressFirewall
rules. The OVN-K master will also delete the corresponding ``DNSName`` CRs, only if the ``.spec.isregular`` field is set to false and
the ``.spec.iswildcard`` field is set to true. If both the fields are set to true, then the CR will not be deleted and the
``.spec.iswildcard`` field will be set to false. The from the ``.status`` field all the other DNS names' details will be removed and
only the details of the DNS name will be kept which matches the Name field of the CR.


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
	// +kubebuilder:validation:Pattern=^(\*\.)?([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+\.?$
	DNSName string `json:"dnsName,omitempty"`
}
````

The following ``DNSName`` CRD will be added to the ``k8s.ovn.org`` api-group.

````go
// DNSName describes a DNS name used a EgressFirewall rule.
type DNSName struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of DNSName.
	Spec DNSNameSpec `json:"spec"`
	// Observed status of DNSName
	// +optional
	Status DNSNameStatus `json:"status,omitempty"`
}

// DNSNameSpec is a desired state description of DNSName.
type DNSNameSpec struct {
	IsRegular  bool `json:"isregular,omitempty"`
	IsWildcard bool `json:"iswildcard,omitempty"`
}

type DNSNameStatus struct {
	// The list of matching DNS names and their corresponding IPs along with TTL and next
	// time to DNS lookup.
	Items []DNSNameStatusItem `json:"items"`
}

type DNSNameStatusItem struct {
	DNSName string        `json:"dnsname"`
	Info    []DNSNameInfo `json:"info"`
}

type DNSNameInfo struct {
	IP             string `json:"ip"`
	TTL            string `json:"ttl"`
	NextLookupTime string `json:"nextlookuptime"`
}
````

### Implementation Details/Notes/Constraints [optional]



### Risks and Mitigations


### Drawbacks

* Whenever there's a change in the IPs or next time to DNS lookup for a DNS name, the additional step of updating the
related ``DNSName`` CRs will be executed. This might add some delay to the DNS lookup process. However, this will only
happen whenever there's a change in the DNS information.


## Design Details

### Open Questions [optional]

### Test Plan

* This enhancement will be tested through e2e tests by adding EgressFirewall rules containing regular DNS names
and wildcard DNS names.
* Testing the feature where IPs are dynamically changed may be a little bit tricky as this will probably include
creation of DNS records and then changing the IP adresses of the DNS records through the e2e tests.

### Graduation Criteria

This is a user facing change and will directly go to GA. This feature requires an update to Openshift Docs.

#### Dev Preview -> Tech Preview

N.A. This feature will go directly to GA.

#### Tech Preview -> GA

N.A. This feature will go directly to GA.

#### Removing a deprecated feature


### Upgrade / Downgrade Strategy

Upgrade expectations:
* On upgrade, the OVN-K master will create the corresponding ``DNSName`` CRs for each DNS name in the
existing EgressFirewall resources. The ``egressfirewall`` plugin will also start updating the ``.status``
fields of the ``DNSName`` CRs. The scearios arising out of order of update of the various components are
dicussed in [Version Skew Strategy](#version-skew-strategy)

Downgrade expectations:
* On downgrade, the ``DNSName`` CRs may still remain. However, these CRs would not have any impact on how
EgressFirewall ACL rules are implemented in the downgraded cluster. Deleting the CR Definition of ``DNSName``
from the cluster would remove all the ``DNSName`` CRs.

### Version Skew Strategy

The following 2 scenarios may occur during the upgrade process:
* Scenario 1: The Cluster DNS operator and the CoreDNS pods are upgraded first and then the OVN-K master pods.

  In this scenario, the ``egressfirewall`` CoreDNS plugin will start inspecting each DNS lookup before the ``DNSName``
  CRs are created by the OVN-K master. The plugin will just respond with the response received from other plugins for
  the DNS lookups. As OVN-K master will be continuing the DNS lookups for DNS names with expired TTLs, CoreDNS will
  also be responding with the corresponding IPs and the TTLs.

* Scenario 2: The OVN-K master pods are upgraded first and then the Cluster DNS operator and the CoreDNS pods.

  In this scenario, the OVN-K master will create ``DNSName`` CRs for each unique DNS name used in EgressFirewall rules.
  However, as the Cluster DNS operator and the CoreDNS pods are still not upgraded, CoreDNS pods will not run the
  ``egressfirewall`` plugin. The OVN-K master will still receive the response for the DNS lookup queries it will send
  to the CoreDNS.

### Operational Aspects of API Extensions


#### Failure Modes


#### Support Procedures


## Implementation History


## Alternatives


## Infrastructure Needed [optional]

