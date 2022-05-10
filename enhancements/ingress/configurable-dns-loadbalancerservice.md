---
title: configurable-dns-loadbalancerservice
authors:
  - "@thejasn"
reviewers:
  - "@alebedev87"
  - "@sherine-k"
approvers:
  - "@Miciah"
api-approvers:
  - "@Miciah"
creation-date: 2022-05-10
last-updated: 2022-05-10
tracking-link:
  - https://issues.redhat.com/browse/CFEPLAN-58
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Configurable DNS Management for LoadBalancerService Ingress Controllers

## Summary

The ingress operator currently assumes that it's going to create and
manage DNS using the required cloud provider integration, but in some
circumstances when the DNS zone may be different from the cluster DNS zone
and may not even be hosted in the cloud provider. In such scenarios the operator
after provisioning the required resources(load balancer, haproxy) in the cloud
provider reports a degraded state because the provided DNS can't be created.

This enhancement aims to provide the end-user the ability to completely disable
DNS management on an Ingress Controller.

## Motivation

End users need to have an option to manage DNS on their own instead of relying
on the `cluster-ingress-operator`. This is quite a common requirement where
1. Customers may need to deploy ingress controllers on domains hosted outside of
   cloud providers.
2. Partners may need to deploy ingress controllers for domains from their own 
   customers (with domains not hosted in the same cloud provider).

With no support for such scenarious currently, customers end up with clusters
in degraded state. The solution to this is to add support
in the `cluster-ingress-operator` to be able to optionally manage DNS record
creation. 

### User Stories

#### As a cluster admin, when configuring an ingresscontroller, I want to specify whether a DNS record should or should not be created for this ingress 

The cluster admin has the option to modify/create ingresscontroller to 
specify whether a DNS record should or should not be created. This is 
available as a Day-2 operation to the cluster admin.

#### As a cluster admin, when updating an ingresscontroller from managed DNS to unmanaged DNS, I want the previous DNS records to be deleted

Upon updating `manageDNS` to `false` the older DNS records that were created
must be deleted by the ingress operator.

### Goals

- Provide the ability to opt in for DNS management by the `cluster-ingress-operator`.
- Recover from degraded state of the `cluster-ingress-operator` during upgrades
  from OCP 3.x to 4.x where DNS was being managed externally,

### Non-Goals

- Provide a day-0 solution for cluster installations involving external DNS
  managaments by the customer.

## Proposal

Currently there is partial support provided by the `installer` (in UPI installations)
to disable the `cluster-ingress-operator` from managing the DNS cluster-wide as documented
[here](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones). 

The proposed solution adds support for more fine grained control over specific
ingresscontrollers on how they manage the wildcard DNS records associated with them.

### Workflow Description

This feature is designed as Day-2 solution and is geared toworads cluster
admins. It mainly applies to scenarios where specific ingresscontrollers
need to be configured to not manage DNS records associated with them.

#### Modifying the default ingresscontroller

The default ingresscontroller will be created with domain present in
*ingress.config.openshift.io/cluster* `.spec.domain`. If we want to 
update the domain present on the ingresscontroller with a domain
outside the defined cluster DNS zone,
1. The new domain that needs to be associated with the ingresscontroller
    must be created prior to making any changes.
2. The ingresscontroller must be edited with a new domain and
    the `manageDNS` field must be set to `false`.
3. This will trigger a reconcile of the controller resulting in updating
    the following conditions on the ingress operator

    - `DNSManaged` condition will be set to false and reason updated to
        UnmanagedDNS.
    - `DNSReady` condition will be set to true and reason updated to
        UnmanagedDNS.
4. If any *DNSRecord* was previously provisioned, it will be deleted. 
    
__Note__: Similarly,to create a new ingresscontroller where dns is managed
          externally the similar steps can be followed.

### API Extensions

The IngressController CRD is extended by adding an optional `ManageDNS`
field with type `bool`. This field will default to true.

```go
// LoadBalancerStrategy holds parameters for a load balancer.
type LoadBalancerStrategy struct {
    // <snip>

    // manageDNS indicates if the lifecyle of the wildcard DNS record
    // associated with the load balancer service will be managed by
    // the ingress operator.
    //
  // +kubebuilder:default:=true
  // +kubebuilder:validation:Optional 
    // +optional
    ManageDNS bool `json:"manageDNS"`
}
```

Re-uses existing state `DNSManaged` of the controller to denote if
the wildcard *DNSRecord* associated with the controller is managed/un-managed. 

### Implementation Details/Notes/Constraints

Based on `managedDNS`, the controller decides whether to ensure creating
the a wildcard *DNSRecord*. If the customer is updating `managedDNS` from 
`"true"` to `"false"` the previously created DNSRecord will be deleted.

The support provided by the installer and this new field aren't entriely
mutually exclusive i.e. if customer has opted to disable DNS management cluster
wide (as mentioned under UPI based installations), this additional flag is of
no value and need not be set since *DNSRecord* created is silently reconciled 
by the dns controller.

### Risks and Mitigations

The manual creation of the required DNS records will need to be done prior to
making this change as the canary controller will try to ensure the if the ingress
domains are reachable, otherwise the `cluster-ingress-operator` gets into a 
degraded state. This needs to be documented to avoid unnecessary support cases.

### Drawbacks

There is an inconsistency in how the existing support is added to disable
DNS management cluster-wide by the ingress controller i.e. the controller 
proceedes to create a wildcard *DNSRecord* and during reconcilliation the dns
controller [silently skips](https://github.com/openshift/cluster-ingress-operator/blob/972f09b9dbb181ae5c414da2a990b57c60fde9d8/pkg/operator/controller/dns/controller.go#L261-L311)
publishing the record to the zone. Where as in the proposed solution, 
a wildcard *DNSRecord* is **not** created when `manageDNS` is set to `false`. 

## Design Details

### Test Plan

- Test by updating the default ingresscontroller to be associated with a manually
  created dns record and ensure correct working of test workload.
- Test to ensure clean up old DNS records when updating `manageDNS` to false.


### Graduation Criteria

N/A.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

On upgrades, this field will default to `true` which is consistent with the
older versions of the ingress operator.

On downgrades, 
- If downgrading to an unsupported version while having the `manageDNS` set to
  `false` could result in failing to create a wildcard *DNSRecord* and therby
  resulting in the ingress *ClusterOperator* in a degraded state.
- If downgrading to an unsupported version while having the `manageDNS` set to
  `true` or leaving it as the default will not result in any issue as it would
  be consistent with the older versions.

### Version Skew Strategy

N/A.

### Operational Aspects of API Extensions

N/A.

#### Failure Modes

- If the customer doesn't create the appropriate manual records needed
  for the `default` ingresscontroller, then the ingress *ClusterOperator*
  will result in a degraded state.

#### Support Procedures

N/A.

## Implementation History

N/A.

## Alternatives

N/A.
