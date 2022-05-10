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
manage DNS using the required cloud provider integration. But in some
circumstances, the required DNS zone may be different from the cluster
DNS zone and may not even be hosted in the cloud provider. In such scenarios,
the operator reports a degraded state after provisioning the required
resources in the cloud provider because the provided DNS can't be created.

This enhancement aims to provide the end-user the ability to completely disable
DNS management on an Ingress Controller.

## Motivation

End users need to have an option to manage DNS on their own instead of relying
on the `cluster-ingress-operator`. This is quite a common requirement where
1. Customers may need to deploy ingress controllers on domains hosted outside of
   cloud providers.
2. Partners may need to deploy ingress controllers for domains from their own 
   customers (with domains not hosted in the same cloud provider).

With no support for such scenarios currently, customers end up with having the
`cluster-ingress-operator` in degraded state. The solution is to add support
in the operator to be able to optionally manage DNS record lifecycle. 

### User Stories

#### As a cluster admin, when configuring an ingresscontroller, I want to specify whether a DNS record should or should not be created for this ingress 

The cluster admin has the option to modify/create ingresscontroller to 
specify whether a DNS record should or should not be created. This is 
available as a Day-2 operation to the cluster admin.

#### As a cluster admin, when updating an ingresscontroller from managed DNS to unmanaged DNS, I want the previous DNS records to be deleted

Upon updating `.loadBalancer.dns` to `Unmanaged` the older DNS records that were created
must be deleted by the ingress operator.

### Goals

- Provide the ability to opt in for DNS management by the `cluster-ingress-operator`.
- Recover from degraded state of the `cluster-ingress-operator` during upgrades
  from OCP 3.x to 4.x where DNS was being managed externally.

### Non-Goals

- Provide a day-0 solution for cluster installations involving external DNS
  management by the customer.

## Proposal

Currently, there is partial support provided by the `installer` (in UPI installations)
to disable the `cluster-ingress-operator` from managing the DNS cluster-wide as documented
[here](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones). 

The proposed solution adds support for more fine-grained control over specific
ingresscontrollers on how they manage the wildcard DNS records associated with them.

### Workflow Description

This feature is designed as Day-2 solution and is geared towards cluster
admins. It mainly applies to scenarios where specific ingresscontrollers
need to be configured to not manage DNS records associated with them.

#### Modifying the default ingresscontroller

The default ingresscontroller will be created with domain present in
*ingress.config.openshift.io/cluster* `.spec.domain`. If we want to 
update the domain present on the ingresscontroller with a domain
outside the defined cluster DNS zone,
1. The new domain that needs to be associated with the ingresscontroller
    must be created prior to making any changes.
2. The ingresscontroller must be edited with the created domain and
    the `.loadBalancer.dns` field must be set to `Unmanaged`.
3. This will trigger a reconcile of the controller, resulting in updating
    the following conditions on the ingress operator

    - `DNSManaged` condition will be set to false and reason updated to
        UnmanagedDNS.
    - `DNSReady` condition will be set to true and reason updated to
        UnmanagedDNS.
4. If any *DNSRecord* was previously provisioned, it will be deleted. 
    
__Note__: Similarly, to create a new ingresscontroller where DNS is managed
          externally, the same workflow can be followed.

### API Extensions

The ingresscontroller CRD is extended by adding an optional field `DNS`
of type `LoadBalancerDNSManagementPolicy`. This field will default to `Managed`.

```go
// LoadBalancerStrategy holds parameters for a load balancer.
type LoadBalancerStrategy struct {
    // <snip>

    // dns indicates if the lifecyle of the wildcard DNS record
    // associated with the load balancer service will be managed by
    // the ingress operator.
    //
    // +kubebuilder:default:="Managed"
    // +kubebuilder:validation:Optional
    // +kubebuilder:validation:Enum=Managed;Unmanaged
    // +optional
    DNS LoadBalancerDNSManagementPolicy `json:"dns"`
}

// LoadBalancerDNSManagementPolicy is a policy for configuring how
// ingresscontrollers manage DNS.
//
// +kubebuilder:validation:Enum=Managed;Unmanaged
type LoadBalancerDNSManagementPolicy string

const (
    // ManagedLoadBalancerDNS configures the ingresscontroller to manage
    // the wildcard DNS records associated with it.
    ManagedLoadBalancerDNS LoadBalancerDNSManagementPolicy = "Managed"
    // UnmanagedLoadBalancerDNS configures the ingresscontroller to not
    // manage the wildcard DNS records associated with it.
    UnmanagedLoadBalancerDNS LoadBalancerDNSManagementPolicy = "Unmanaged"
)
```

Re-uses existing state `DNSManaged` of the controller to denote if
the wildcard *DNSRecord* associated with the controller is managed/unmanaged. 

### Implementation Details/Notes/Constraints

Based on `.loadBalancer.dns`, the controller decides whether to ensure creating
a wildcard *DNSRecord*. If the customer is updating `.loadBalancer.dns` from 
`"Managed"` to `"Unmanaged"` the previously created *DNSRecord* will be deleted.

The support provided by the installer and this new field aren't entirely
mutually exclusive, i.e. if customer has opted to disable DNS management cluster
wide (as documented
[here](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones)),
this field is of no value and need not be set since *DNSRecord* created is silently
reconciled by the DNS controller.

### Risks and Mitigations

The manual creation of the required DNS records will need to be done prior to
making this change as the canary controller will try to ensure that the ingress
domains are reachable, and therefore puts the `cluster-ingress-operator` gets in a 
degraded state. This needs to be documented to avoid unnecessary support cases.

### Drawbacks

N/A 

## Design Details

### Open Questions

Currently, as per documentation `.spec.domain` in the ingresscontroller cannot be
updated after creation since updating the [domain](https://github.com/openshift/cluster-ingress-operator/blob/972f09b9dbb181ae5c414da2a990b57c60fde9d8/pkg/operator/controller/ingress/controller.go#L342-L355)
in the ingresscontroller doesn't result in [updating](https://github.com/openshift/cluster-ingress-operator/blob/972f09b9dbb181ae5c414da2a990b57c60fde9d8/pkg/operator/controller/ingress/dns.go#L97)
the wildcard *DNSRecord*. 
> 1. Can this logic be changed to support moving from managed DNS to
     unmanaged DNS on the `default` ingresscontroller?

### Test Plan

- Test by updating the default ingresscontroller `domain` with a manually
  created DNS record and ensure correct working of test workload.
- Test to ensure clean up of old DNS records when updating `.loadBalancer.dns` to `Unmanaged`.

### Graduation Criteria

N/A.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

On upgrades, `.loadBalancer.dns` will default to `Managed` which is consistent with the
older versions of the ingress operator.

On downgrades, there are 2 possible variations
* When downgrading to an unsupported version, having the `.loadBalancer.dns` set to
  `Unmanaged`,
  * If the cluster domain is not the same as the domain in the ingresscontroller,
    could result in failing to create the wildcard *DNSRecord* associated on the 
    ingresscontroller and thereby result in the `cluster-ingress-operator` entering 
    a degraded state. To recover DNS management will have to be disabled cluster 
    wide ([docs](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones)).
  * When the cluster domain is the same as the domain in the ingresscontroller,
    this will not cause any issues during downgrades, but a new wildcard *DNSRecord*
    will be created by the operator.
* When downgrading to an unsupported version, having the `.loadBalancer.dns` set to
  `Managed` or leaving it as the default in the ingresscontroller will not result
  in any issue as it would be consistent with the older versions.

### Version Skew Strategy

N/A.

### Operational Aspects of API Extensions

N/A.

#### Failure Modes

- If the customer doesn't create the appropriate manual records needed
  for the `default` ingresscontroller, then the `cluster-ingress-operator`
  will result in a degraded state.

#### Support Procedures

N/A.

## Implementation History

N/A.

## Alternatives

N/A.
